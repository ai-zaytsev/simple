package store

import (
	"context"
	"fmt"
	"sort"
	"time"

	"download.simplevpn/control-plane/internal/capacity"
)

// CapacityReading gathers what the service knows about its own room.
//
// Assembled from what is already collected: the samples nodes send every
// minute and the daily summaries made from them. Nothing is measured specially
// for the alert, which is why the alert can be trusted - it is looking at the
// same numbers the panel is.
func (s *Store) CapacityReading(ctx context.Context, now time.Time, defaultCapacity int) (capacity.Reading, error) {
	standings, err := s.NodeStandings(ctx, "", defaultCapacity)
	if err != nil {
		return capacity.Reading{}, err
	}
	return s.readingFrom(ctx, now, standings)
}

// readingFrom is the reading itself, taking standings the caller already has.
//
// Split out so the panel can build the same picture without asking the
// database a second time for something it is holding.
func (s *Store) readingFrom(ctx context.Context, now time.Time, standings []NodeStanding) (capacity.Reading, error) {
	var r capacity.Reading
	var err error

	for _, standing := range standings {
		switch {
		case standing.Usable(now):
			r.NodesUsable++
			// A spare is a finished node nobody has yet decided to lean on.
			if standing.Lifecycle == "ready" {
				r.NodesSpare++
			}
			r.CapacityTotal += standing.Capacity
			r.SessionsNow += standing.Sessions
		default:
			switch ConditionOf(standing, now) {
			case Blocked:
				r.NodesBlocked++
			case Faulty:
				r.NodesFaulty++
			}
		}
	}

	// The busiest minute of the day and of the week, across the fleet at once
	// rather than per node: capacity is what the group has, and a peak on one
	// node while another sits idle is a distribution problem, not a shortage.
	err = s.pool.QueryRow(ctx, `
		with fleet as (
			select at, sum(coalesce(sessions_online, 0))::int as sessions
			from metrics.node_samples
			where at >= $1
			group by at
		)
		select
			coalesce(max(sessions) filter (where at >= $2), 0),
			coalesce(max(sessions), 0)
		from fleet`, now.Add(-7*24*time.Hour), now.Add(-24*time.Hour)).
		Scan(&r.PeakToday, &r.PeakWeek)
	if err != nil {
		return r, fmt.Errorf("cannot read the peaks: %w", err)
	}

	// The utilisation all but one minute in twenty stayed below. A single
	// spike is an incident; a high figure here is a service that is full most
	// of the time that matters.
	if r.CapacityTotal > 0 {
		var p95 float64
		err = s.pool.QueryRow(ctx, `
			with fleet as (
				select at, sum(coalesce(sessions_online, 0))::int as sessions
				from metrics.node_samples
				where at >= $1
				group by at
			)
			select coalesce(
				percentile_disc(0.95) within group (order by sessions), 0)::float8
			from fleet`, now.Add(-7*24*time.Hour)).Scan(&p95)
		if err != nil {
			return r, fmt.Errorf("cannot read the busy line: %w", err)
		}
		r.P95Utilisation = p95 / float64(r.CapacityTotal)
	}

	// Growth, only where there is a week to compare against. A fortnight-old
	// service cannot say how it grew last week, and a trend invented from two
	// days is worse than no trend: it would be acted on.
	var thisWeek, lastWeek *int
	err = s.pool.QueryRow(ctx, `
		select
			(select max(peak_sessions)::int from metrics.node_days
			 where day >= $1::date and day < $2::date),
			(select max(peak_sessions)::int from metrics.node_days
			 where day >= $3::date and day < $1::date)`,
		now.AddDate(0, 0, -7), now.AddDate(0, 0, 1), now.AddDate(0, 0, -14)).
		Scan(&thisWeek, &lastWeek)
	if err != nil {
		return r, fmt.Errorf("cannot read the trend: %w", err)
	}
	if thisWeek != nil && lastWeek != nil && *lastWeek > 0 {
		growth := float64(*thisWeek-*lastWeek) / float64(*lastWeek)
		r.GrowthWeekOnWeek = &growth
	}

	// Domains ready and unused. A new node needs one, and getting one is the
	// slow part - it is not something that happens in an afternoon.
	err = s.pool.QueryRow(ctx, `
		select count(*)::int from domains
		where lifecycle = 'ready' and purpose = 'node'`).Scan(&r.DomainsSpare)
	if err != nil {
		return r, fmt.Errorf("cannot count spare domains: %w", err)
	}

	return r, nil
}

// LastSaid is what was last announced about a subject, and when.
func (s *Store) LastSaid(ctx context.Context, subject string) (string, time.Time, error) {
	var state string
	var at time.Time

	err := s.pool.QueryRow(ctx, `
		select state, at from metrics.capacity_alerts
		where subject = $1 order by at desc limit 1`, subject).Scan(&state, &at)
	if err != nil {
		// Nothing said yet is an answer, not a failure: it is the state every
		// deployment starts in.
		return "", time.Time{}, nil
	}
	return state, at, nil
}

// RecordAlert writes down that something was said, and whether it arrived.
func (s *Store) RecordAlert(ctx context.Context, subject, state, channel string, ok bool, detail string) error {
	_, err := s.pool.Exec(ctx, `
		insert into metrics.capacity_alerts (at, state, subject, channel, ok, detail)
		values (now(), $1, $2, $3, $4, $5)
		on conflict (at, subject, channel) do nothing`,
		state, subject, channel, ok, trimDetail(detail))
	if err != nil {
		return fmt.Errorf("cannot record the alert: %w", err)
	}
	return nil
}

func trimDetail(detail string) string {
	if len(detail) > 300 {
		return detail[:300]
	}
	return detail
}

// CapacityView is the same judgement the alert makes, with the breakdowns a
// person needs in order to act on it.
//
// The state and the reasons come from capacity.Assess and are not recomputed
// here: a panel that judged the fleet by its own rule would eventually
// disagree with the message that woke somebody up, and then neither could be
// trusted.
type CapacityView struct {
	State   string   `json:"state"`
	Reasons []string `json:"reasons"`

	SessionsNow   int     `json:"sessions_now"`
	CapacityTotal int     `json:"capacity_total"`
	Utilisation   float64 `json:"utilisation"`
	SpareRoom     int     `json:"spare_room"`

	PeakToday   int     `json:"peak_today"`
	PeakWeek    int     `json:"peak_week"`
	PeakUsed    float64 `json:"peak_used"`
	PeakDay     int     `json:"peak_day"`
	PeakEvening int     `json:"peak_evening"`

	// The busiest each hour of the day has been over the week, Moscow time.
	// A shape rather than a number, because "we are full at nine in the
	// evening and empty at four in the morning" is a different problem from
	// "we are full", and it is not visible in an average.
	ByHour []int `json:"by_hour"`

	P95Utilisation float64  `json:"p95_utilisation"`
	Growth         *float64 `json:"growth_week_on_week"`

	// How much history the peaks and the busy line were computed from.
	//
	// Reported because without it they read as weekly figures whatever they
	// are: a peak over ten hours and a peak over seven days are shown in the
	// same box, and only one of them means what the label says. Growth already
	// refuses to answer without two weeks; these two cannot refuse, because a
	// peak over ten hours is still a real peak - so they say their span
	// instead.
	HistoryHours float64 `json:"history_hours"`

	NodesUsable  int `json:"nodes_usable"`
	NodesSpare   int `json:"nodes_spare"`
	NodesBlocked int `json:"nodes_blocked"`
	NodesFaulty  int `json:"nodes_faulty"`
	DomainsSpare int `json:"domains_spare"`

	Groups  []GroupCapacity `json:"groups"`
	Domains []DomainLoad    `json:"domains"`
}

// GroupCapacity is one pool of machines and how full it is.
type GroupCapacity struct {
	Group       string  `json:"group"`
	Nodes       int     `json:"nodes"`
	Spare       int     `json:"spare"`
	Sessions    int     `json:"sessions"`
	Capacity    int     `json:"capacity"`
	Utilisation float64 `json:"utilisation"`
}

// DomainLoad is how much of the load one way in is carrying.
//
// Named by domain rather than by node because that is the unit that gets
// blocked, and a domain carrying most of the traffic is the one whose loss
// would be felt.
type DomainLoad struct {
	Domain   string  `json:"domain"`
	Node     string  `json:"node"`
	Sessions int     `json:"sessions"`
	Share    float64 `json:"share"`
	Verdict  string  `json:"verdict"`
}

// capacityView builds the panel's capacity picture from standings the caller
// already has, so that the panel and the chooser read one set of numbers.
func (s *Store) capacityView(
	ctx context.Context, now time.Time, standings []NodeStanding,
) (CapacityView, error) {
	var v CapacityView

	reading, err := s.readingFrom(ctx, now, standings)
	if err != nil {
		return v, err
	}
	verdict := capacity.Assess(reading)

	v.State = string(verdict.State)
	v.Reasons = verdict.Reasons
	v.SessionsNow = reading.SessionsNow
	v.CapacityTotal = reading.CapacityTotal
	v.Utilisation = verdict.Utilisation
	v.SpareRoom = reading.CapacityTotal - reading.SessionsNow
	if v.SpareRoom < 0 {
		v.SpareRoom = 0
	}
	v.PeakToday = reading.PeakToday
	v.PeakWeek = reading.PeakWeek
	v.PeakUsed = verdict.PeakUsed
	v.P95Utilisation = reading.P95Utilisation
	v.Growth = reading.GrowthWeekOnWeek
	v.NodesUsable = reading.NodesUsable
	v.NodesSpare = reading.NodesSpare
	v.NodesBlocked = reading.NodesBlocked
	v.NodesFaulty = reading.NodesFaulty
	v.DomainsSpare = reading.DomainsSpare

	if v.ByHour, err = s.hourlyPeaks(ctx, now); err != nil {
		return v, err
	}
	if v.HistoryHours, err = s.historyHours(ctx, now); err != nil {
		return v, err
	}
	// Daytime and evening as people live them, not as UTC has them. The hours
	// are Moscow's because that is where the load is; an evening peak read off
	// a UTC clock lands in the afternoon and looks like nothing.
	v.PeakDay = maxOfHours(v.ByHour, 9, 17)
	v.PeakEvening = maxOfHours(v.ByHour, 18, 23)

	v.Groups = groupCapacity(standings, now)
	v.Domains = domainLoad(standings, now)
	return v, nil
}

// hourlyPeaks is the busiest each hour of the day has been this week.
func (s *Store) hourlyPeaks(ctx context.Context, now time.Time) ([]int, error) {
	rows, err := s.pool.Query(ctx, `
		with fleet as (
			select at, sum(coalesce(sessions_online, 0))::int as sessions
			from metrics.node_samples
			where at >= $1
			group by at
		)
		select extract(hour from (at at time zone 'Europe/Moscow'))::int,
		       max(sessions)::int
		from fleet group by 1`, now.Add(-7*24*time.Hour))
	if err != nil {
		return nil, fmt.Errorf("cannot read the daily shape: %w", err)
	}
	defer rows.Close()

	hours := make([]int, 24)
	for rows.Next() {
		var hour, sessions int
		if err := rows.Scan(&hour, &sessions); err != nil {
			return nil, fmt.Errorf("cannot read an hour: %w", err)
		}
		if hour >= 0 && hour < 24 {
			hours[hour] = sessions
		}
	}
	return hours, rows.Err()
}

// historyHours is how far back the samples the peaks are drawn from reach.
//
// Capped at the same week the peaks look at, because "we have four months of
// history" says nothing about a figure computed from seven days of it.
func (s *Store) historyHours(ctx context.Context, now time.Time) (float64, error) {
	var oldest *time.Time
	err := s.pool.QueryRow(ctx, `
		select min(at) from metrics.node_samples where at >= $1`,
		now.Add(-7*24*time.Hour)).Scan(&oldest)
	if err != nil {
		return 0, fmt.Errorf("cannot tell how much history there is: %w", err)
	}
	if oldest == nil {
		return 0, nil
	}
	return now.Sub(*oldest).Hours(), nil
}

func maxOfHours(hours []int, from, to int) int {
	best := 0
	for hour := from; hour <= to && hour < len(hours); hour++ {
		if hours[hour] > best {
			best = hours[hour]
		}
	}
	return best
}

func groupCapacity(standings []NodeStanding, now time.Time) []GroupCapacity {
	index := map[string]*GroupCapacity{}
	order := []string{}
	for _, standing := range standings {
		if !standing.Usable(now) {
			continue
		}
		name := standing.Group
		if name == "" {
			name = "default"
		}
		group, ok := index[name]
		if !ok {
			group = &GroupCapacity{Group: name}
			index[name] = group
			order = append(order, name)
		}
		group.Nodes++
		group.Capacity += standing.Capacity
		group.Sessions += standing.Sessions
		if standing.Lifecycle == "ready" {
			group.Spare++
		}
	}

	sort.Strings(order)
	out := make([]GroupCapacity, 0, len(order))
	for _, name := range order {
		group := index[name]
		if group.Capacity > 0 {
			group.Utilisation = float64(group.Sessions) / float64(group.Capacity)
		}
		out = append(out, *group)
	}
	return out
}

func domainLoad(standings []NodeStanding, now time.Time) []DomainLoad {
	total := 0
	for _, standing := range standings {
		if standing.Usable(now) {
			total += standing.Sessions
		}
	}

	out := []DomainLoad{}
	for _, standing := range standings {
		name := standing.ServerName
		if name == "" {
			name = standing.Node.Host
		}
		if name == "" {
			continue
		}
		load := DomainLoad{
			Domain:   name,
			Node:     standing.Node.Alias,
			Sessions: standing.Sessions,
			Verdict:  standing.DomainVerdict,
		}
		if total > 0 {
			load.Share = float64(standing.Sessions) / float64(total)
		}
		out = append(out, load)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Sessions != out[j].Sessions {
			return out[i].Sessions > out[j].Sessions
		}
		return out[i].Domain < out[j].Domain
	})
	return out
}
