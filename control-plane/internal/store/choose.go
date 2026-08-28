package store

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"download.simplevpn/control-plane/internal/document"
)

// NodeStanding is everything known about one node at the moment somebody asks
// where to connect.
//
// Assembled from the measurements the service already takes rather than from
// anything new: how the machine is doing, whether its domain answers, what it
// costs to reach, and how often connecting to it has failed lately. Nothing
// here is about any user.
type NodeStanding struct {
	Node       document.Node
	ServerName string

	// Capacity is how many tunnel connections this node is sized for. A
	// declared property of the machine, not a measurement: the point of it is
	// to know how much room is left before the numbers get bad, which is too
	// late to find out by watching them get bad.
	Capacity int

	LastSeen      *time.Time
	CPUPercent    *float64
	MemoryPercent *float64
	Sessions      int
	LossPercent   *float64

	// UpstreamLatencyMS is the node's own network health. DomainLatencyMS is
	// what it costs somebody to reach its domain, preferring what devices
	// measured over what we measured: the second is the number users live
	// with, and from Helsinki the two can differ by everything.
	UpstreamLatencyMS *float64
	DomainLatencyMS   *float64

	// One of the words from endpointVerdict, or empty when nothing has checked
	// this domain yet.
	DomainVerdict string

	Attempts  int
	Successes int
}

// Score is how good a place this is to send somebody, from 0 to 1.
//
// Two factors multiplied, because they answer different questions and either
// one being bad is enough. Room says whether the node can take more without
// getting worse; quality says how well it is serving the people already on it.
// A node with plenty of room that loses a tenth of its packets is not a good
// place to send anybody, and neither is a healthy node that is full.
// typicalHandshake is what reaching a node's domain costs across the fleet.
//
// Passed into scoring rather than assumed, because the number it is compared
// against is not a network round trip. A device measures a whole handshake -
// connect, TLS, upgrade - which on real phones runs to hundreds of
// milliseconds where the node's own ping is one. Judging both against one
// fixed threshold penalised every node anybody had actually measured, so the
// node nobody had checked came out ahead of the node proven to be working.
// That was live on the panel: 0.629 for the unmeasured node, 0.119 for the
// good one.
//
// So a node is compared against the others rather than against a number
// somebody chose. A node at the fleet's usual cost pays nothing; one that is
// twice as expensive to reach pays half. If every node is slow, none is
// penalised - which is right for a chooser, whose whole question is which of
// these to use. Whether the fleet as a whole is slow is a different question
// and the panel already answers it.
func (s NodeStanding) Score(typicalHandshake float64) float64 {
	return s.Room() * s.Quality(typicalHandshake)
}

// TypicalHandshake is the middle of what devices measured, or zero when
// nothing has been measured yet.
func TypicalHandshake(standings []NodeStanding) float64 {
	measured := []float64{}
	for _, s := range standings {
		if s.DomainLatencyMS != nil && *s.DomainLatencyMS > 0 {
			measured = append(measured, *s.DomainLatencyMS)
		}
	}
	if len(measured) == 0 {
		return 0
	}
	sort.Float64s(measured)
	return measured[len(measured)/2]
}

// Room is how much of this node is still free, from 0 to 1.
//
// The worst of the three pressures rather than their average: a node at 95%
// memory is full whatever its processor is doing, and averaging would hide
// exactly the number that is about to matter.
func (s NodeStanding) Room() float64 {
	load := 0.0
	if s.CPUPercent != nil {
		load = math.Max(load, *s.CPUPercent/100)
	}
	if s.MemoryPercent != nil {
		load = math.Max(load, *s.MemoryPercent/100)
	}
	if s.Capacity > 0 {
		load = math.Max(load, float64(s.Sessions)/float64(s.Capacity))
	}
	return math.Max(0, math.Min(1, 1-load))
}

// Quality is how well this node is serving the people already on it.
func (s NodeStanding) Quality(typicalHandshake float64) float64 {
	quality := 1.0

	// What it costs a device to reach this node, against what it costs to
	// reach the others. Absence of a measurement is not an advantage: a node
	// nobody has checked simply pays nothing here, and so does a node at the
	// usual cost.
	if typicalHandshake > 0 && s.DomainLatencyMS != nil && *s.DomainLatencyMS > 0 {
		worse := *s.DomainLatencyMS/typicalHandshake - 1
		if worse > 0 {
			quality *= 1 / (1 + worse)
		}
	}

	// The node's own round trip, which is a round trip and can be judged
	// against a number. Smooth rather than a threshold: a threshold makes a
	// node flip between chosen and shunned over one millisecond.
	if s.UpstreamLatencyMS != nil && *s.UpstreamLatencyMS > 0 {
		quality *= 1 / (1 + *s.UpstreamLatencyMS/roundTripHalvesAt)
	}

	if s.LossPercent != nil {
		quality *= math.Max(0, 1-*s.LossPercent/100)
	}

	// How often devices actually got through. Ignored below a handful of
	// attempts, because three failures out of four attempts is not a failure
	// rate, it is four attempts.
	if s.Attempts >= minAttemptsToJudge {
		quality *= float64(s.Successes) / float64(s.Attempts)
	}

	// A domain that answers slowly for devices is worth less than one that
	// does not, and worth more than nothing.
	if s.DomainVerdict == "slower" {
		quality *= 0.5
	}

	return math.Max(0, math.Min(1, quality))
}

// Usable says whether this node should be offered at all.
//
// Separate from the score because these are not degrees of worse: a node that
// has stopped reporting, or whose domain devices cannot reach, is not a poor
// choice but a wrong one. Scoring them low would still put them in front of
// somebody the moment every other node had a bad minute.
func (s NodeStanding) Usable(now time.Time) bool {
	if s.LastSeen == nil || now.Sub(*s.LastSeen) > silentAfter {
		return false
	}
	switch s.DomainVerdict {
	case "unreachable", "likely blocked":
		return false
	}
	return true
}

const (
	// Where a node's own round trip has cost it half its quality. Two hundred
	// milliseconds is roughly the point at which a person notices the service
	// rather than the internet.
	//
	// This is a round trip and only a round trip. What a device measures when
	// it checks a domain is a whole handshake - connect, TLS, upgrade - and is
	// judged against the other nodes instead; see TypicalHandshake.
	roundTripHalvesAt = 200.0

	// Below this many connection attempts, a success rate is noise.
	minAttemptsToJudge = 20

	// A node reports every minute, so three missed windows is a node that has
	// stopped talking rather than one that was late.
	silentAfter = 3 * time.Minute
)

// Rank puts the nodes in the order this device should try them.
//
// Two properties matter and they pull against each other. The order has to be
// good, or people end up on a node that is failing. And it has to differ
// between devices, or every device piles onto whichever node is currently best
// and makes it the worst - the classic way an automatic chooser produces the
// outage it was built to avoid.
//
// So the order is drawn at random, weighted by score, from a random number
// that depends only on the device and the node. Good nodes are picked far more
// often; the load spreads in proportion to how much room each one has; and one
// device asking twice gets the same answer, because a plan that reshuffles on
// every refresh is a plan that reconnects people for no reason.
//
// Nodes that are not usable go last rather than being dropped. A plan naming
// somewhere bad is worse than a good plan and far better than no plan: the
// client probes before it commits and moves on by itself, and a device with an
// empty plan has nothing to move on to.
func Rank(standings []NodeStanding, deviceKey string, now time.Time) []NodeStanding {
	type keyed struct {
		standing NodeStanding
		key      float64
		usable   bool
	}

	typical := TypicalHandshake(standings)

	ranked := make([]keyed, 0, len(standings))
	for _, s := range standings {
		score := s.Score(typical)

		// Never exactly zero: a node with no measurements at all has not
		// earned a place at the front, and it has not earned exclusion
		// either. This is the state every node is in for its first minute.
		if score <= 0 {
			score = 0.001
		}

		ranked = append(ranked, keyed{
			standing: s,
			usable:   s.Usable(now),
			key:      math.Pow(uniform(deviceKey, s.Node.Alias), 1/score),
		})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].usable != ranked[j].usable {
			return ranked[i].usable
		}
		return ranked[i].key > ranked[j].key
	})

	order := make([]NodeStanding, 0, len(ranked))
	for _, r := range ranked {
		order = append(order, r.standing)
	}
	return order
}

// uniform is a number in (0,1] drawn from the pair, and the same every time.
func uniform(deviceKey, alias string) float64 {
	sum := sha256.Sum256([]byte(deviceKey + "\x00" + alias))
	// Shifted off zero so that the power below is always defined.
	return (float64(binary.BigEndian.Uint64(sum[:8])) + 1) / (math.MaxUint64 + 2)
}

// NodeStandings reads what is known about every node that can serve this
// transport.
//
// One query rather than one per node, and left joins throughout: a node that
// has never reported is a node with no row, and it has to come back with
// nothing rather than not come back.
func (s *Store) NodeStandings(ctx context.Context, kind string, defaultCapacity int) ([]NodeStanding, error) {
	rows, err := s.pool.Query(ctx, `
		select
			n.alias, n.host, n.port, n.transport_kind, n.params,
			coalesce(n.params->>'server_name', ''),
			coalesce((n.params->>'capacity')::int, $2),
			sample.at, sample.cpu_percent, sample.memory_percent,
			coalesce(sample.sessions_online, 0), sample.upstream_loss_percent,
			sample.upstream_latency_ms,
			probe.latency_ms, coalesce(probe.ok_rate, -1),
			coalesce(tries.attempts, 0), coalesce(tries.successes, 0)
		from nodes n

		left join lateral (
			select * from metrics.node_samples s
			where s.node_alias = n.alias
			order by s.at desc limit 1
		) sample on true

		-- What devices found when they tried this node's domain, preferred
		-- over our own checks because it is the vantage that matters.
		left join lateral (
			select avg(p.latency_ms)::float8 as latency_ms,
			       (avg((p.ok)::int) * 100)::float8 as ok_rate
			from metrics.endpoint_probes p
			where p.target = lower(coalesce(n.params->>'server_name',''))
			  and p.vantage = 'device'
			  and p.at > now() - interval '1 hour'
		) probe on true

		left join lateral (
			select sum(r.attempts)::int as attempts, sum(r.successes)::int as successes
			from metrics.connect_reports r
			where r.node_alias = n.alias and r.at > now() - interval '1 hour'
		) tries on true

		-- An empty kind means every kind, so that the panel can show the same
		-- numbers the chooser used rather than a second opinion about them.
		where n.state = 'active' and ($1 = '' or n.transport_kind = $1)
		order by n.alias`, kind, defaultCapacity)
	if err != nil {
		return nil, fmt.Errorf("cannot read node standings: %w", err)
	}
	defer rows.Close()

	standings := []NodeStanding{}
	for rows.Next() {
		var (
			st     NodeStanding
			tkind  string
			raw    []byte
			okRate float64
		)
		if err := rows.Scan(
			&st.Node.Alias, &st.Node.Host, &st.Node.Port, &tkind, &raw,
			&st.ServerName, &st.Capacity,
			&st.LastSeen, &st.CPUPercent, &st.MemoryPercent,
			&st.Sessions, &st.LossPercent, &st.UpstreamLatencyMS,
			&st.DomainLatencyMS, &okRate,
			&st.Attempts, &st.Successes,
		); err != nil {
			return nil, fmt.Errorf("cannot read a node standing: %w", err)
		}

		params := map[string]any{}
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, fmt.Errorf("node %s has unreadable parameters: %w", st.Node.Alias, err)
		}
		st.Node.Transport = document.Transport{Kind: tkind, Params: params}
		st.DomainVerdict = verdictFromRate(okRate)

		standings = append(standings, st)
	}
	return standings, rows.Err()
}

// verdictFromRate turns what devices reported into the same words the panel
// uses, so that one vocabulary describes a domain wherever it is mentioned.
func verdictFromRate(okRate float64) string {
	switch {
	case okRate < 0:
		return ""
	case okRate < 30:
		return "unreachable"
	case okRate < 90:
		return "slower"
	default:
		return "works"
	}
}

// defaultCapacityForPanel is what the panel assumes when a node does not say.
//
// The panel is a view, not a decision, so it does not take the configured
// number: what it shows is "how full is this, roughly", and being a little
// wrong about a node nobody configured is better than the panel needing to be
// told the same thing twice.
const defaultCapacityForPanel = 500
