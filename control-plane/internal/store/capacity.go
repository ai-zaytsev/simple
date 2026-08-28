package store

import (
	"context"
	"fmt"
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
	var r capacity.Reading

	standings, err := s.NodeStandings(ctx, "", defaultCapacity)
	if err != nil {
		return r, err
	}

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
