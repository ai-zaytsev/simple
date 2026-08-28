package probe

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"download.simplevpn/control-plane/internal/alert"
	"download.simplevpn/control-plane/internal/capacity"
	"download.simplevpn/control-plane/internal/store"
)

// CapacityWatch looks at how much room the service has and says so when it
// matters.
//
// Three things kept apart on purpose: the numbers come from the store, the
// judgement from internal/capacity, and where the message goes from
// internal/alert. This is only the loop that puts them together, which is why
// it is short - and why changing any of the three does not touch the others.
type CapacityWatch struct {
	store    *store.Store
	notifier *alert.Notifier
	log      *slog.Logger

	every    time.Duration
	capacity int
}

func NewCapacityWatch(
	st *store.Store, notifier *alert.Notifier,
	every time.Duration, defaultCapacity int, log *slog.Logger,
) *CapacityWatch {
	if every <= 0 {
		every = 10 * time.Minute
	}
	return &CapacityWatch{
		store: st, notifier: notifier, log: log,
		every: every, capacity: defaultCapacity,
	}
}

func (w *CapacityWatch) Run(ctx context.Context) {
	ticker := time.NewTicker(w.every)
	defer ticker.Stop()

	// Not immediately. A service that has just started has heard from no node
	// yet, and its first honest reading is "nothing can be handed out" - which
	// would announce an emergency every time anybody deployed.
	select {
	case <-ctx.Done():
		return
	case <-time.After(3 * time.Minute):
	}

	w.once(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.once(ctx)
		}
	}
}

func (w *CapacityWatch) once(ctx context.Context) {
	reading, err := w.store.CapacityReading(ctx, time.Now().UTC(), w.capacity)
	if err != nil {
		w.log.Error("cannot read how much room there is", "error", err)
		return
	}

	verdict := capacity.Assess(reading)

	sent, err := w.notifier.Consider(ctx, alert.Alert{
		State:    string(verdict.State),
		Subject:  "capacity",
		Headline: headline(verdict, reading),
		Reasons:  verdict.Reasons,
		Facts:    facts(verdict, reading),
	}, string(capacity.Normal))
	if err != nil {
		w.log.Error("cannot decide whether to warn", "error", err)
		return
	}
	if sent {
		w.log.Info("capacity alert sent", "state", verdict.State)
	}
}

func headline(v capacity.Verdict, r capacity.Reading) string {
	switch v.State {
	case capacity.Critical:
		return "the service is out of room"
	case capacity.ScaleRequired:
		return "add a server"
	case capacity.Watch:
		return "capacity is trending towards a decision"
	default:
		return fmt.Sprintf("capacity is comfortable on %d node(s)", r.NodesUsable)
	}
}

func facts(v capacity.Verdict, r capacity.Reading) []string {
	lines := []string{
		fmt.Sprintf("now:      %d connection(s) of %d, %.0f%%",
			r.SessionsNow, r.CapacityTotal, v.Utilisation*100),
		fmt.Sprintf("peaks:    %d today, %d this week (%.0f%% of capacity)",
			r.PeakToday, r.PeakWeek, v.PeakUsed*100),
		fmt.Sprintf("busy:     %.0f%% at the ninety-fifth percentile", r.P95Utilisation*100),
		fmt.Sprintf("servers:  %d usable, %d spare, %d blocked, %d faulty",
			r.NodesUsable, r.NodesSpare, r.NodesBlocked, r.NodesFaulty),
		fmt.Sprintf("domains:  %d spare", r.DomainsSpare),
	}

	if r.GrowthWeekOnWeek != nil {
		lines = append(lines, fmt.Sprintf("growth:   %+.0f%% on last week", *r.GrowthWeekOnWeek*100))
	} else {
		lines = append(lines, "growth:   not enough history to say")
	}
	return lines
}
