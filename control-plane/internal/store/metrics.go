package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// NodeSample is one node's report about itself for one window.
//
// Pointers where a number may be missing, because a node that could not read
// its own CPU should say so rather than report zero. Zero is a measurement.
type NodeSample struct {
	At               time.Time
	WindowSeconds    int
	UsersConfigured  *int
	SessionsOnline   *int
	CPUPercent       *float64
	Load1            *float64
	MemoryPercent    *float64
	Established      *int
	UplinkBytes      int64
	DownlinkBytes    int64
	UpstreamLatency  *float64
	UpstreamLoss     *float64
	Goroutines       *int
	HeapBytes        *int64
	XrayUptimeS      *int64
}

// ClassBytes is traffic of one kind, for a whole node, over one window.
type ClassBytes struct {
	Class    string
	Uplink   int64
	Downlink int64
}

// RecordNodeSample writes one window of a node's own health.
func (s *Store) RecordNodeSample(ctx context.Context, alias string, m NodeSample) error {
	_, err := s.pool.Exec(ctx, `
		insert into metrics.node_samples (
			at, node_alias, window_s,
			users_configured, sessions_online,
			cpu_percent, load1, memory_percent, established,
			uplink_bytes, downlink_bytes,
			upstream_latency_ms, upstream_loss_percent,
			goroutines, heap_bytes, xray_uptime_s)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		on conflict (node_alias, at) do nothing`,
		m.At, alias, m.WindowSeconds,
		m.UsersConfigured, m.SessionsOnline,
		m.CPUPercent, m.Load1, m.MemoryPercent, m.Established,
		m.UplinkBytes, m.DownlinkBytes,
		m.UpstreamLatency, m.UpstreamLoss,
		m.Goroutines, m.HeapBytes, m.XrayUptimeS)
	if err != nil {
		return fmt.Errorf("cannot record a node sample: %w", err)
	}
	return nil
}

// RecordTrafficClasses writes traffic by kind for one window.
//
// No user reaches this function, and there is no parameter through which one
// could. The separation the schema promises is kept here by the signature.
func (s *Store) RecordTrafficClasses(ctx context.Context, alias string, at time.Time, classes []ClassBytes) error {
	for _, c := range classes {
		_, err := s.pool.Exec(ctx, `
			insert into metrics.traffic_classes (at, node_alias, class, uplink_bytes, downlink_bytes)
			values ($1,$2,$3,$4,$5)
			on conflict (node_alias, at, class) do update
			set uplink_bytes = metrics.traffic_classes.uplink_bytes + excluded.uplink_bytes,
			    downlink_bytes = metrics.traffic_classes.downlink_bytes + excluded.downlink_bytes`,
			at, alias, c.Class, c.Uplink, c.Downlink)
		if err != nil {
			return fmt.Errorf("cannot record traffic classes: %w", err)
		}
	}
	return nil
}

// AccountsForCredentials turns what a node knows into what analytics may use.
//
// This is the only place the two halves meet, and it meets them in memory for
// the length of one request. The node knows credentials and nothing else; what
// is written afterwards is a pseudonym and nothing else. Neither side of that
// exchange is stored.
func (s *Store) AccountsForCredentials(ctx context.Context, credentials []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	found := map[uuid.UUID]uuid.UUID{}
	if len(credentials) == 0 {
		return found, nil
	}

	rows, err := s.pool.Query(ctx, `
		select c.credential_uuid, d.account_id
		from device_credentials c
		join devices d on d.id = c.device_id
		where c.credential_uuid = any($1) and d.account_id is not null`,
		credentials)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve credentials: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var credential, account uuid.UUID
		if err := rows.Scan(&credential, &account); err != nil {
			return nil, fmt.Errorf("cannot read a credential: %w", err)
		}
		found[credential] = account
	}
	return found, rows.Err()
}

// AddAccountUsage adds bytes to one pseudonym's total for a month.
func (s *Store) AddAccountUsage(ctx context.Context, period time.Time, analyticsID string, bytes int64) error {
	_, err := s.pool.Exec(ctx, `
		insert into metrics.account_usage (period, analytics_id, bytes)
		values ($1,$2,$3)
		on conflict (period, analytics_id) do update
		set bytes = metrics.account_usage.bytes + excluded.bytes`,
		period, analyticsID, bytes)
	if err != nil {
		return fmt.Errorf("cannot add usage: %w", err)
	}
	return nil
}

// MarkActive notes that somebody was using the service in an hour.
func (s *Store) MarkActive(ctx context.Context, hour time.Time, analyticsID string) error {
	_, err := s.pool.Exec(ctx, `
		insert into metrics.active_hours (hour, analytics_id) values ($1,$2)
		on conflict do nothing`, hour, analyticsID)
	if err != nil {
		return fmt.Errorf("cannot mark activity: %w", err)
	}
	return nil
}

// ConnectReport is a device's own summary of getting through to us.
type ConnectReport struct {
	NodeAlias      string
	EntryKind      string
	Attempts       int
	Successes      int
	Reconnects     int
	SessionSeconds int64
	LatencySumMs   int64
	LatencySamples int
}

// RecordConnectReport stores one already-summed report.
func (s *Store) RecordConnectReport(ctx context.Context, at time.Time, r ConnectReport) error {
	_, err := s.pool.Exec(ctx, `
		insert into metrics.connect_reports (
			at, node_alias, entry_kind, attempts, successes, reconnects,
			session_seconds, latency_ms_sum, latency_samples)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		at, r.NodeAlias, r.EntryKind, r.Attempts, r.Successes, r.Reconnects,
		r.SessionSeconds, r.LatencySumMs, r.LatencySamples)
	if err != nil {
		return fmt.Errorf("cannot record a connect report: %w", err)
	}
	return nil
}

// RecordProbe stores the result of testing one of our own addresses.
func (s *Store) RecordProbe(ctx context.Context, at time.Time, target, vantage string, ok bool, latencyMS *int, detail string) error {
	_, err := s.pool.Exec(ctx, `
		insert into metrics.endpoint_probes (at, target, vantage, ok, latency_ms, detail)
		values ($1,$2,$3,$4,$5,$6)`,
		at, target, vantage, ok, latencyMS, detail)
	if err != nil {
		return fmt.Errorf("cannot record a probe: %w", err)
	}
	return nil
}

// ServedNames lists the addresses this service is willing to test.
//
// The prober takes its targets from here rather than from anything a client or
// a node sends, so the one domain column in the schema can only ever hold a
// name we serve. A target that arrives from outside is refused, which is what
// keeps a probe table from becoming a list of places people go.
func (s *Store) ServedNames(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		select distinct params->>'server_name' from nodes
		where coalesce(params->>'server_name','') <> ''
		union
		select distinct host from bootstrap_entries
		where enabled and kind = 'https-direct' and host <> ''`)
	if err != nil {
		return nil, fmt.Errorf("cannot list served names: %w", err)
	}
	defer rows.Close()

	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("cannot read a served name: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// DropOldSamples enforces retention as code.
//
// Minute rows are the bulk and the least useful once a day has passed, so they
// go first; the daily summaries stay. Retention that depends on somebody
// remembering to run something is not retention.
func (s *Store) DropOldSamples(ctx context.Context, rawKeep, summaryKeep time.Duration) error {
	statements := []struct {
		sql  string
		keep time.Duration
	}{
		{"delete from metrics.node_samples where at < $1", rawKeep},
		{"delete from metrics.traffic_classes where at < $1", rawKeep},
		{"delete from metrics.connect_reports where at < $1", rawKeep},
		{"delete from metrics.endpoint_probes where at < $1", rawKeep},
		{"delete from metrics.active_hours where hour < $1", summaryKeep},
		{"delete from metrics.node_days where day < $1::date", summaryKeep},
		{"delete from metrics.traffic_class_days where day < $1::date", summaryKeep},
		{"delete from metrics.account_usage where period < $1::date", summaryKeep},
	}
	for _, st := range statements {
		if _, err := s.pool.Exec(ctx, st.sql, time.Now().Add(-st.keep)); err != nil {
			return fmt.Errorf("cannot apply retention: %w", err)
		}
	}
	return nil
}

// RollUpDay folds a day of minute rows into one row per node and one per class.
func (s *Store) RollUpDay(ctx context.Context, day time.Time) error {
	_, err := s.pool.Exec(ctx, `
		insert into metrics.node_days (
			day, node_alias, uplink_bytes, downlink_bytes, peak_sessions,
			avg_cpu_percent, max_cpu_percent, avg_latency_ms, max_loss_percent, samples)
		select $1::date, node_alias,
		       coalesce(sum(uplink_bytes),0), coalesce(sum(downlink_bytes),0),
		       coalesce(max(sessions_online),0),
		       avg(cpu_percent), max(cpu_percent),
		       avg(upstream_latency_ms), max(upstream_loss_percent),
		       count(*)
		from metrics.node_samples
		where at >= $1::date and at < $1::date + interval '1 day'
		group by node_alias
		on conflict (node_alias, day) do update set
			uplink_bytes = excluded.uplink_bytes,
			downlink_bytes = excluded.downlink_bytes,
			peak_sessions = excluded.peak_sessions,
			avg_cpu_percent = excluded.avg_cpu_percent,
			max_cpu_percent = excluded.max_cpu_percent,
			avg_latency_ms = excluded.avg_latency_ms,
			max_loss_percent = excluded.max_loss_percent,
			samples = excluded.samples`, day)
	if err != nil {
		return fmt.Errorf("cannot roll up nodes: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		insert into metrics.traffic_class_days (day, class, uplink_bytes, downlink_bytes)
		select $1::date, class, coalesce(sum(uplink_bytes),0), coalesce(sum(downlink_bytes),0)
		from metrics.traffic_classes
		where at >= $1::date and at < $1::date + interval '1 day'
		group by class
		on conflict (day, class) do update set
			uplink_bytes = excluded.uplink_bytes,
			downlink_bytes = excluded.downlink_bytes`, day)
	if err != nil {
		return fmt.Errorf("cannot roll up classes: %w", err)
	}
	return nil
}
