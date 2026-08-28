package store

import (
	"context"
	"fmt"
	"time"
)

// Condition is what the measurements say about something right now.
//
// Derived every time it is asked and never stored, because a stored condition
// is an opinion with a date on it: the moment it is written down it starts
// being wrong, and the thing it describes stops being watched.
type Condition string

const (
	// Nothing the measurements can see is wrong.
	Healthy Condition = "healthy"

	// Working, and worse than it should be. Still worth handing out: a slow
	// way in beats no way in, and refusing every degraded node during a bad
	// hour would leave nothing.
	Degraded Condition = "degraded"

	// Working perfectly, and being kept from the people who need it. This is
	// the state the whole stage is about, and it is invisible from any single
	// vantage point: from here a blocked domain and a broken one look the
	// same. They are told apart by asking devices, which stand where the
	// blocking is.
	Blocked Condition = "blocked"

	// Not working. Nobody can reach it, including us.
	Faulty Condition = "faulty"

	// Nothing has been measured yet. A new node is in this state for its first
	// minute, and saying so is better than calling it healthy on no evidence.
	Unknown Condition = "unknown"
)

// ConditionOf works out what is wrong with a node, if anything.
//
// The order of the checks is the meaning of the function. Blocked is decided
// before faulty, because a domain we can reach and devices cannot is not
// broken - sending somebody to restart it would waste their afternoon and fix
// nothing. Faulty is decided before degraded, because a node that has stopped
// reporting is not a slow node.
func ConditionOf(s NodeStanding, now time.Time) Condition {
	silent := s.LastSeen == nil || now.Sub(*s.LastSeen) > silentAfter

	switch s.DomainVerdict {
	case "likely blocked":
		return Blocked
	case "unreachable":
		// Unreachable for devices while the machine itself is reporting
		// happily to us is the same picture as blocked, whatever the probe
		// called it: something between them and us is refusing.
		if !silent {
			return Blocked
		}
		return Faulty
	}

	if silent {
		return Faulty
	}

	if s.LossPercent != nil && *s.LossPercent >= 5 {
		return Degraded
	}
	if s.UpstreamLatencyMS != nil && *s.UpstreamLatencyMS >= 250 {
		return Degraded
	}
	if s.CPUPercent != nil && *s.CPUPercent >= 85 {
		return Degraded
	}
	if s.MemoryPercent != nil && *s.MemoryPercent >= 90 {
		return Degraded
	}
	if s.DomainVerdict == "slower" {
		return Degraded
	}
	if s.DomainVerdict == "" && s.LastSeen == nil {
		return Unknown
	}
	return Healthy
}

// Standing is one server or domain, what was decided about it and what is
// observed about it, and the four answers those two produce together.
type Standing struct {
	Name      string    `json:"name"`
	Kind      string    `json:"kind"` // "server" or "domain"
	Lifecycle string    `json:"lifecycle"`
	Since     time.Time `json:"since"`
	Note      string    `json:"note"`
	Condition Condition `json:"condition"`

	// The word to show, which is the lifecycle unless the condition overrides
	// it. A node that is meant to be serving and is blocked is described as
	// blocked, because that is the thing somebody needs to act on.
	State string `json:"state"`

	// The four questions the stage says must have an unambiguous answer.
	MayHandOut     bool `json:"may_hand_out"`
	StopHandingOut bool `json:"stop_handing_out"`
	NeedsReplacing bool `json:"needs_replacing"`
	MayDelete      bool `json:"may_delete"`

	// Why, in one line, so that the answers above are readable rather than
	// merely correct.
	Because string `json:"because"`
}

// building is the part of a life before anything can be handed out.
var building = map[string]bool{
	"creating":             true,
	"configuring":          true,
	"awaiting-certificate": true,
	"verifying":            true,
}

// leaving is the part after it has been decided that this will not be used.
var leaving = map[string]bool{
	"draining":    true,
	"quarantined": true,
	"removing":    true,
	"removed":     true,
	"retired":     true,
}

// Decide answers the four questions from the two axes.
//
// Written as one function rather than four, because the answers are not
// independent and reading them together is the only way to see that they are
// consistent. "May hand out" and "stop handing out" are not opposites: a node
// that is still being built is neither, and treating them as opposites is how
// a half-built machine ends up in somebody's connection plan.
func Decide(st *Standing, now time.Time, idleFor time.Duration) {
	lifecycle := st.Lifecycle

	st.State = lifecycle
	switch {
	case building[lifecycle], leaving[lifecycle]:
		// The lifecycle is the story. What the measurements say about a
		// machine that is being built or taken away is not what anybody needs
		// to know about it.
	case st.Condition != Healthy && st.Condition != Unknown:
		st.State = string(st.Condition)
	}

	usable := (lifecycle == "ready" || lifecycle == "serving") &&
		(st.Condition == Healthy || st.Condition == Degraded || st.Condition == Unknown)

	st.MayHandOut = usable

	// Only for things that were supposed to be in service. A node that is
	// being built is not "stop handing out", it is "not yet", and reporting it
	// as the former would put it on a list of problems it does not belong on.
	st.StopHandingOut = !usable && (lifecycle == "ready" || lifecycle == "serving")

	// A blocked domain does not recover by being restarted; it is replaced.
	// That is the difference this stage exists to make, and it is the
	// difference between an afternoon spent on the wrong machine and a new
	// domain being raised.
	st.NeedsReplacing = st.Condition == Blocked ||
		(st.Condition == Faulty && now.Sub(st.Since) > faultyTooLong)

	// Deletable when it has been declared gone, or when it has been on its way
	// out long enough that nothing can still be using it. `removing` is not
	// enough on its own: something is removing it, and finishing the job twice
	// is how a half-deleted node loses the half that was still working.
	st.MayDelete = lifecycle == "removed" || lifecycle == "retired" ||
		(lifecycle == "draining" && idleFor > drainedFor) ||
		(lifecycle == "quarantined" && idleFor > drainedFor)

	st.Because = because(st, lifecycle)
}

func because(st *Standing, lifecycle string) string {
	switch {
	case building[lifecycle]:
		return "not finished being built"
	case lifecycle == "removed" || lifecycle == "retired":
		return "gone; kept so the name is never reused"
	case lifecycle == "removing":
		return "being taken away"
	case lifecycle == "draining":
		return "carrying the people already on it, given to nobody new"
	case lifecycle == "quarantined":
		return "held out of service deliberately"
	case st.Condition == Blocked:
		return "reachable from us and not from devices: this is a block, not a fault"
	case st.Condition == Faulty:
		return "nobody can reach it, including us"
	case st.Condition == Degraded:
		return "working, and worse than it should be"
	case st.Condition == Unknown:
		return "nothing measured yet"
	case lifecycle == "ready":
		return "finished and proven, held as a spare"
	default:
		return "working"
	}
}

const (
	// How long something has to be unreachable before replacing it is the
	// answer rather than waiting. Short enough to act within a working day,
	// long enough that a reboot is not a reason to build a new machine.
	faultyTooLong = 2 * time.Hour

	// How long a node on its way out has to be carrying nobody before it can
	// be deleted. An hour, because a device that has just lost its tunnel
	// tries again, and deleting the node underneath it turns a reconnection
	// into a failure.
	drainedFor = time.Hour
)

// Lifecycles reads the declared state of every server and domain, works out
// the condition of each, and answers the four questions.
func (s *Store) Lifecycles(ctx context.Context, now time.Time) ([]Standing, error) {
	standings, err := s.NodeStandings(ctx, "", defaultCapacityForPanel)
	if err != nil {
		return nil, err
	}
	measured := map[string]NodeStanding{}
	domainOf := map[string]string{}
	for _, st := range standings {
		measured[st.Node.Alias] = st
		if st.ServerName != "" {
			domainOf[st.ServerName] = st.Node.Alias
		}
	}

	out := []Standing{}

	rows, err := s.pool.Query(ctx, `
		select n.alias, n.state, n.state_since, n.state_note,
		       coalesce(l.sessions_online, 0), l.at
		from nodes n
		left join lateral (
			select sessions_online, at from metrics.node_samples s
			where s.node_alias = n.alias order by s.at desc limit 1
		) l on true
		order by n.alias`)
	if err != nil {
		return nil, fmt.Errorf("cannot read node lifecycles: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			st       Standing
			sessions int
			lastAt   *time.Time
		)
		if err := rows.Scan(&st.Name, &st.Lifecycle, &st.Since, &st.Note, &sessions, &lastAt); err != nil {
			return nil, fmt.Errorf("cannot read a node lifecycle: %w", err)
		}
		st.Kind = "server"
		st.Condition = ConditionOf(measured[st.Name], now)

		// How long it has been carrying nobody. Unknown counts as busy: a node
		// we have heard nothing from is not a node we know to be empty.
		idle := time.Duration(0)
		if lastAt != nil && sessions == 0 {
			idle = now.Sub(st.Since)
		}
		Decide(&st, now, idle)
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	domains, err := s.domainLifecycles(ctx, now, measured, domainOf)
	if err != nil {
		return nil, err
	}
	return append(out, domains...), nil
}

func (s *Store) domainLifecycles(
	ctx context.Context, now time.Time,
	measured map[string]NodeStanding, domainOf map[string]string,
) ([]Standing, error) {
	byName, err := s.domainConditions(ctx, now)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		select name, purpose, lifecycle, since, note from domains order by name`)
	if err != nil {
		return nil, fmt.Errorf("cannot read domain lifecycles: %w", err)
	}
	defer rows.Close()

	out := []Standing{}
	for rows.Next() {
		var st Standing
		var purpose string
		if err := rows.Scan(&st.Name, &purpose, &st.Lifecycle, &st.Since, &st.Note); err != nil {
			return nil, fmt.Errorf("cannot read a domain lifecycle: %w", err)
		}
		st.Kind = "domain"

		// A node's cover domain takes its condition from the node behind it:
		// the two are reachable or blocked together, and reporting them apart
		// would be reporting one thing twice with two different answers.
		//
		// A domain with no node - the way in to the Control Plane - is judged
		// on its own checks. It has to be: it is the domain whose blocking
		// matters most, because every recovery path depends on devices being
		// able to ask where to go next.
		own, checked := byName[st.Name]
		if !checked {
			own = Unknown
		}
		if alias, ok := domainOf[st.Name]; ok {
			st.Condition = CoverCondition(ConditionOf(measured[alias], now), own)
		} else {
			st.Condition = own
		}

		Decide(&st, now, 0)
		out = append(out, st)
	}
	return out, rows.Err()
}

// RememberDomain records a domain the service is serving, if it is not already
// known.
//
// Called where domains come into existence rather than by a person, because a
// domain that is in use and has no row is a domain nobody can retire: the
// question "should we stop using this" would have nowhere to be answered.
func (s *Store) RememberDomain(ctx context.Context, name, purpose string) error {
	if name == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		insert into domains (name, purpose, lifecycle) values (lower($1), $2, 'serving')
		on conflict (name) do nothing`, name, purpose)
	if err != nil {
		return fmt.Errorf("cannot remember the domain %s: %w", name, err)
	}
	return nil
}

// SetNodeLifecycle moves a node to a declared state.
func (s *Store) SetNodeLifecycle(ctx context.Context, alias, lifecycle, note string) error {
	tag, err := s.pool.Exec(ctx, `
		update nodes set state = $2, state_since = now(), state_note = $3
		where alias = $1 and state <> $2`, alias, lifecycle, note)
	if err != nil {
		return fmt.Errorf("cannot move %s to %s: %w", alias, lifecycle, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("node %s is not there, or is already %s", alias, lifecycle)
	}
	return nil
}

// SetDomainLifecycle moves a domain to a declared state.
func (s *Store) SetDomainLifecycle(ctx context.Context, name, lifecycle, note string) error {
	tag, err := s.pool.Exec(ctx, `
		update domains set lifecycle = $2, since = now(), note = $3
		where name = lower($1) and lifecycle <> $2`, name, lifecycle, note)
	if err != nil {
		return fmt.Errorf("cannot move %s to %s: %w", name, lifecycle, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("domain %s is not there, or is already %s", name, lifecycle)
	}
	return nil
}

// RememberServedDomains gives a row to every domain the service is actually
// serving.
//
// Called from the prober, which already walks that list every few minutes, so
// that a domain cannot be in use and unknown at the same time. A domain with
// no row is a domain nobody can retire: the question "should we stop using
// this" would have nowhere to be answered, and the answer would be discovered
// by somebody noticing that a node had gone quiet.
func (s *Store) RememberServedDomains(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		insert into domains (name, purpose, lifecycle)
		select distinct lower(params->>'server_name'), 'node', 'serving'
		from nodes
		where coalesce(params->>'server_name','') <> '' and state <> 'removed'
		on conflict (name) do nothing`)
	if err != nil {
		return fmt.Errorf("cannot remember node domains: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		insert into domains (name, purpose, lifecycle)
		select distinct lower(host), 'bootstrap', 'serving'
		from bootstrap_entries
		where enabled and kind = 'https-direct' and host <> ''
		on conflict (name) do nothing`)
	if err != nil {
		return fmt.Errorf("cannot remember bootstrap domains: %w", err)
	}
	return nil
}

// domainConditions works out what is wrong with each domain from the checks of
// that domain alone.
//
// Needed because not every domain has a node behind it. The way in to the
// Control Plane is a domain with no server of its own, and it is the one whose
// blocking matters most: if it goes, devices cannot be told where to connect
// next, and every recovery path this service has depends on it. Deriving its
// condition only from a node would have left it reading "nothing measured yet"
// for ever, while being checked every five minutes.
func (s *Store) domainConditions(ctx context.Context, now time.Time) (map[string]Condition, error) {
	// Two windows in one pass. The hour is what a domain is normally judged
	// on, so that one recovering is not held down by yesterday's failures.
	// The day is the fallback, and it exists because of the alternative:
	// treating a domain with nothing in the last hour as never measured, when
	// what it actually has is twenty-four hours of failing. "Nothing measured
	// yet" is a reason to hand a domain out, and that would be the wrong
	// answer about the one domain nobody can reach.
	rows, err := s.pool.Query(ctx, `
		select target,
			coalesce(avg(case when vantage = 'control-plane' and at >= $1
				then (ok::int) end) * 100, -1)::float8,
			coalesce(avg(case when vantage = 'device' and at >= $1
				then (ok::int) end) * 100, -1)::float8,
			count(*) filter (where vantage = 'device' and at >= $1)::int,

			coalesce(avg(case when vantage = 'control-plane'
				then (ok::int) end) * 100, -1)::float8,
			coalesce(avg(case when vantage = 'device'
				then (ok::int) end) * 100, -1)::float8,
			count(*) filter (where vantage = 'device')::int
		from metrics.endpoint_probes
		where at >= $2
		group by target`, now.Add(-time.Hour), now.Add(-24*time.Hour))
	if err != nil {
		return nil, fmt.Errorf("cannot read domain checks: %w", err)
	}
	defer rows.Close()

	conditions := map[string]Condition{}
	for rows.Next() {
		var target string
		var hour, day EndpointHealth
		if err := rows.Scan(&target,
			&hour.FromUs, &hour.FromDevices, &hour.DeviceChecks,
			&day.FromUs, &day.FromDevices, &day.DeviceChecks); err != nil {
			return nil, fmt.Errorf("cannot read a domain check: %w", err)
		}
		conditions[target] = conditionFromVerdict(endpointVerdict(recentOrElse(hour, day)))
	}
	return conditions, rows.Err()
}

// recentOrElse prefers the last hour and falls back to the last day.
//
// "Nothing in the last hour" and "nothing at all" are different situations
// that produce the same empty row, and only one of them means a domain has
// never been looked at.
func recentOrElse(hour, day EndpointHealth) EndpointHealth {
	if hour.FromUs >= 0 || hour.DeviceChecks > 0 {
		return hour
	}
	return day
}

// CoverCondition judges a domain that stands in front of a node.
//
// It follows the node, because the two usually fail together and reporting
// them apart would be reporting one thing twice with two different answers.
// Usually, not always: a node stays healthy by talking to us over a connection
// it opens itself, and it goes on doing that while the domain in front of it
// answers nobody. That is not a hypothetical shape - it is what being blocked
// looks like from here, and it is the first thing this whole lifecycle exists
// to tell apart.
//
// So the node's word is taken except where the domain's own checks say the way
// in is shut. Those two states are the ones that stop a domain being handed
// out, and they are exactly the ones the node cannot see.
func CoverCondition(node, own Condition) Condition {
	if own == Faulty || own == Blocked {
		return own
	}
	return node
}

// conditionFromVerdict maps the words the probes speak into the words a
// lifecycle speaks, so that one vocabulary describes a domain wherever it is
// mentioned.
func conditionFromVerdict(verdict string) Condition {
	switch verdict {
	case "likely blocked":
		return Blocked
	case "unreachable":
		return Faulty
	case "slower":
		return Degraded
	case "works":
		return Healthy
	default:
		return Unknown
	}
}
