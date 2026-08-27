package store

import (
	"context"
	"fmt"
	"time"
)

// Overview is everything the panel shows, in one answer.
//
// Assembled from summaries rather than from raw rows, and never by joining the
// two halves of the schema. Nothing in here can be narrowed to a person and a
// place, because nothing in the tables underneath holds the pair.
type Overview struct {
	GeneratedAt time.Time `json:"generated_at"`

	Now struct {
		ActiveUsersHour int     `json:"active_users_hour"`
		ActiveUsersDay  int     `json:"active_users_day"`
		SessionsOnline  int     `json:"sessions_online"`
		NodesReporting  int     `json:"nodes_reporting"`
		UplinkBps       float64 `json:"uplink_bps"`
		DownlinkBps     float64 `json:"downlink_bps"`
	} `json:"now"`

	Nodes     []NodeHealth     `json:"nodes"`
	Classes   []ClassShare     `json:"classes"`
	Usage     UsageShape       `json:"usage"`
	Connect   ConnectSummary   `json:"connect"`
	Endpoints []EndpointHealth `json:"endpoints"`
}

type NodeHealth struct {
	Alias          string     `json:"alias"`
	LastSeen       *time.Time `json:"last_seen"`
	SessionsOnline int        `json:"sessions_online"`
	Users          int        `json:"users"`
	CPUPercent     *float64   `json:"cpu_percent"`
	Load1          *float64   `json:"load1"`
	MemoryPercent  *float64   `json:"memory_percent"`
	Established    *int       `json:"established"`
	LatencyMS      *float64   `json:"latency_ms"`
	LossPercent    *float64   `json:"loss_percent"`
	UplinkBps      float64    `json:"uplink_bps"`
	DownlinkBps    float64    `json:"downlink_bps"`

	// One of: ok, busy, degraded, silent. Decided in Go from the numbers
	// above so that the rule is readable in one place instead of buried in
	// a case expression.
	Verdict string `json:"verdict"`
}

type ClassShare struct {
	Class   string  `json:"class"`
	Bytes   int64   `json:"bytes"`
	Percent float64 `json:"percent"`
}

// UsageShape is how load is distributed across people, without naming any.
type UsageShape struct {
	Period      string  `json:"period"`
	Users       int     `json:"users"`
	TotalBytes  int64   `json:"total_bytes"`
	MeanBytes   int64   `json:"mean_bytes"`
	P50         int64   `json:"p50"`
	P90         int64   `json:"p90"`
	P95         int64   `json:"p95"`
	P99         int64   `json:"p99"`
	HeavyUsers  int     `json:"heavy_users"`
	HeavyAbove  int64   `json:"heavy_above"`
	Top1Percent float64 `json:"top_1_percent_share"`

	// True when there are too few people for a top-percentile figure to mean
	// anything. Shown rather than hidden: a share computed over nine users is
	// not a small sample, it is a different quantity.
	TooFewForTop bool `json:"too_few_for_top"`
}

type ConnectSummary struct {
	Attempts       int     `json:"attempts"`
	Successes      int     `json:"successes"`
	Reconnects     int     `json:"reconnects"`
	SuccessRate    float64 `json:"success_rate"`
	AvgSessionSecs float64 `json:"avg_session_seconds"`
	AvgLatencyMS   float64 `json:"avg_latency_ms"`
}

type EndpointHealth struct {
	Target       string     `json:"target"`
	FromUs       float64    `json:"ok_from_us"`
	FromDevices  float64    `json:"ok_from_devices"`
	DeviceChecks int        `json:"device_checks"`
	LatencyMS    *float64   `json:"latency_ms"`
	LastOK       *time.Time `json:"last_ok"`

	// One of: works, slower, unreachable, likely blocked. The last one is the
	// point of keeping two vantages: an address we can reach and devices
	// cannot is not broken, it is being kept from them.
	Verdict string `json:"verdict"`
}

// Overview answers every question the panel asks, in one round of queries.
func (s *Store) Overview(ctx context.Context, now time.Time) (Overview, error) {
	var o Overview
	o.GeneratedAt = now

	hour := now.Truncate(time.Hour)
	if err := s.pool.QueryRow(ctx, `
		select
			count(distinct analytics_id) filter (where hour >= $1)::int,
			count(distinct analytics_id) filter (where hour >= $2)::int
		from metrics.active_hours`, hour, now.Add(-24*time.Hour)).
		Scan(&o.Now.ActiveUsersHour, &o.Now.ActiveUsersDay); err != nil {
		return o, fmt.Errorf("cannot count active users: %w", err)
	}

	nodes, err := s.nodeHealth(ctx, now)
	if err != nil {
		return o, err
	}
	o.Nodes = nodes
	for _, n := range nodes {
		o.Now.SessionsOnline += n.SessionsOnline
		o.Now.UplinkBps += n.UplinkBps
		o.Now.DownlinkBps += n.DownlinkBps
		if n.Verdict != "silent" {
			o.Now.NodesReporting++
		}
	}

	if o.Classes, err = s.classShares(ctx, now); err != nil {
		return o, err
	}
	if o.Usage, err = s.usageShape(ctx, now); err != nil {
		return o, err
	}
	if o.Connect, err = s.connectSummary(ctx, now); err != nil {
		return o, err
	}
	if o.Endpoints, err = s.endpointHealth(ctx, now); err != nil {
		return o, err
	}
	return o, nil
}

func (s *Store) nodeHealth(ctx context.Context, now time.Time) ([]NodeHealth, error) {
	rows, err := s.pool.Query(ctx, `
		select n.alias, l.at, l.sessions_online, l.users_configured,
		       l.cpu_percent, l.load1, l.memory_percent, l.established,
		       l.upstream_latency_ms, l.upstream_loss_percent,
		       l.uplink_bytes, l.downlink_bytes, l.window_s
		from nodes n
		left join lateral (
			select * from metrics.node_samples s
			where s.node_alias = n.alias
			order by s.at desc limit 1
		) l on true
		order by n.alias`)
	if err != nil {
		return nil, fmt.Errorf("cannot read node health: %w", err)
	}
	defer rows.Close()

	health := []NodeHealth{}
	for rows.Next() {
		var h NodeHealth
		var sessions, users, established *int
		var up, down *int64
		var window *int
		if err := rows.Scan(&h.Alias, &h.LastSeen, &sessions, &users,
			&h.CPUPercent, &h.Load1, &h.MemoryPercent, &established,
			&h.LatencyMS, &h.LossPercent, &up, &down, &window); err != nil {
			return nil, fmt.Errorf("cannot read a node row: %w", err)
		}
		if sessions != nil {
			h.SessionsOnline = *sessions
		}
		if users != nil {
			h.Users = *users
		}
		h.Established = established
		if up != nil && window != nil && *window > 0 {
			h.UplinkBps = float64(*up) * 8 / float64(*window)
			h.DownlinkBps = float64(*down) * 8 / float64(*window)
		}
		h.Verdict = nodeVerdict(h, now)
		health = append(health, h)
	}
	return health, rows.Err()
}

// nodeVerdict turns numbers into the four words the panel shows.
//
// Thresholds are here rather than in SQL because this is a judgement, and a
// judgement should be somewhere a person can read it and disagree.
func nodeVerdict(h NodeHealth, now time.Time) string {
	// Three windows without a word. A node reports every minute, so one late
	// sample is a hiccup and three is a node that has stopped talking.
	if h.LastSeen == nil || now.Sub(*h.LastSeen) > 3*time.Minute {
		return "silent"
	}
	if h.LossPercent != nil && *h.LossPercent >= 5 {
		return "degraded"
	}
	if h.LatencyMS != nil && *h.LatencyMS >= 250 {
		return "degraded"
	}
	if h.CPUPercent != nil && *h.CPUPercent >= 85 {
		return "busy"
	}
	if h.MemoryPercent != nil && *h.MemoryPercent >= 90 {
		return "busy"
	}
	if h.Load1 != nil && *h.Load1 >= 4 {
		return "busy"
	}
	return "ok"
}

func (s *Store) classShares(ctx context.Context, now time.Time) ([]ClassShare, error) {
	rows, err := s.pool.Query(ctx, `
		select class, sum(uplink_bytes + downlink_bytes)::bigint as bytes
		from metrics.traffic_classes
		where at >= $1
		group by class
		order by bytes desc`, now.Add(-24*time.Hour))
	if err != nil {
		return nil, fmt.Errorf("cannot read traffic classes: %w", err)
	}
	defer rows.Close()

	shares := []ClassShare{}
	var total int64
	for rows.Next() {
		var c ClassShare
		if err := rows.Scan(&c.Class, &c.Bytes); err != nil {
			return nil, fmt.Errorf("cannot read a class row: %w", err)
		}
		total += c.Bytes
		shares = append(shares, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range shares {
		if total > 0 {
			shares[i].Percent = float64(shares[i].Bytes) * 100 / float64(total)
		}
	}
	return shares, nil
}

func (s *Store) usageShape(ctx context.Context, now time.Time) (UsageShape, error) {
	period := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	u := UsageShape{Period: period.Format("2006-01")}

	err := s.pool.QueryRow(ctx, `
		select
			count(*)::int,
			coalesce(sum(bytes),0)::bigint,
			coalesce(avg(bytes),0)::bigint,
			coalesce(percentile_disc(0.50) within group (order by bytes),0),
			coalesce(percentile_disc(0.90) within group (order by bytes),0),
			coalesce(percentile_disc(0.95) within group (order by bytes),0),
			coalesce(percentile_disc(0.99) within group (order by bytes),0)
		from metrics.account_usage where period = $1`, period).
		Scan(&u.Users, &u.TotalBytes, &u.MeanBytes, &u.P50, &u.P90, &u.P95, &u.P99)
	if err != nil {
		return u, fmt.Errorf("cannot read usage: %w", err)
	}

	// Heavy is defined against the middle of the distribution rather than as a
	// fixed number of gigabytes, so it keeps meaning as the service grows and
	// as what people do with it changes.
	u.HeavyAbove = u.P50 * 5
	if u.HeavyAbove > 0 {
		if err := s.pool.QueryRow(ctx, `
			select count(*)::int from metrics.account_usage
			where period = $1 and bytes > $2`, period, u.HeavyAbove).Scan(&u.HeavyUsers); err != nil {
			return u, fmt.Errorf("cannot count heavy users: %w", err)
		}
	}

	// A hundred people is the point below which "the top one percent" stops
	// being a percentile and starts being one person.
	u.TooFewForTop = u.Users < 100
	if !u.TooFewForTop {
		if err := s.pool.QueryRow(ctx, `
			with ranked as (
				select bytes, ntile(100) over (order by bytes desc) as bucket
				from metrics.account_usage where period = $1
			)
			select coalesce(
				sum(bytes) filter (where bucket = 1)::float8
				/ nullif(sum(bytes),0), 0)
			from ranked`, period).Scan(&u.Top1Percent); err != nil {
			return u, fmt.Errorf("cannot compute the top share: %w", err)
		}
		u.Top1Percent *= 100
	}
	return u, nil
}

func (s *Store) connectSummary(ctx context.Context, now time.Time) (ConnectSummary, error) {
	var c ConnectSummary
	var latencySum, sessionSeconds int64
	var latencySamples, sessions int

	err := s.pool.QueryRow(ctx, `
		select
			coalesce(sum(attempts),0)::int, coalesce(sum(successes),0)::int,
			coalesce(sum(reconnects),0)::int, coalesce(sum(session_seconds),0)::bigint,
			coalesce(sum(latency_ms_sum),0)::bigint, coalesce(sum(latency_samples),0)::int
		from metrics.connect_reports where at >= $1`, now.Add(-24*time.Hour)).
		Scan(&c.Attempts, &c.Successes, &c.Reconnects, &sessionSeconds,
			&latencySum, &latencySamples)
	if err != nil {
		return c, fmt.Errorf("cannot read connect reports: %w", err)
	}

	if c.Attempts > 0 {
		c.SuccessRate = float64(c.Successes) * 100 / float64(c.Attempts)
	}
	sessions = c.Successes
	if sessions > 0 {
		c.AvgSessionSecs = float64(sessionSeconds) / float64(sessions)
	}
	if latencySamples > 0 {
		c.AvgLatencyMS = float64(latencySum) / float64(latencySamples)
	}
	return c, nil
}

func (s *Store) endpointHealth(ctx context.Context, now time.Time) ([]EndpointHealth, error) {
	rows, err := s.pool.Query(ctx, `
		select target,
			coalesce(avg(case when vantage = 'control-plane' then (ok::int) end) * 100, -1)::float8,
			coalesce(avg(case when vantage = 'device' then (ok::int) end) * 100, -1)::float8,
			count(*) filter (where vantage = 'device')::int,
			avg(latency_ms) filter (where ok)::float8,
			max(at) filter (where ok)
		from metrics.endpoint_probes
		where at >= $1
		group by target
		order by target`, now.Add(-24*time.Hour))
	if err != nil {
		return nil, fmt.Errorf("cannot read probes: %w", err)
	}
	defer rows.Close()

	health := []EndpointHealth{}
	for rows.Next() {
		var e EndpointHealth
		if err := rows.Scan(&e.Target, &e.FromUs, &e.FromDevices,
			&e.DeviceChecks, &e.LatencyMS, &e.LastOK); err != nil {
			return nil, fmt.Errorf("cannot read a probe row: %w", err)
		}
		e.Verdict = endpointVerdict(e)
		health = append(health, e)
	}
	return health, rows.Err()
}

// endpointVerdict names what is wrong with a way in, when something is.
//
// The whole reason for measuring the same address from two places is this
// function. An address that answers us and not the people who need it is not a
// broken server, and calling it one would send somebody to Helsinki to restart
// something that is working perfectly.
func endpointVerdict(e EndpointHealth) string {
	const enough = 10

	weCanReach := e.FromUs < 0 || e.FromUs >= 90

	if e.DeviceChecks >= enough {
		switch {
		case e.FromDevices < 30 && weCanReach:
			return "likely blocked"
		case e.FromDevices < 30:
			return "unreachable"
		case e.FromDevices < 90:
			return "slower"
		}
	}

	if e.FromUs >= 0 && e.FromUs < 30 {
		return "unreachable"
	}
	if e.FromUs >= 0 && e.FromUs < 90 {
		return "slower"
	}
	if e.LatencyMS != nil && *e.LatencyMS > 1500 {
		return "slower"
	}
	return "works"
}
