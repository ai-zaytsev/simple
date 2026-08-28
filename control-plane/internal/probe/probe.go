// Package probe checks our own ways in, so that blocking is noticed by
// testing rather than by reading what users failed to do.
//
// The stage that asked for this was explicit: problems with domains and
// servers must be found through separate technical checks, not through the
// traffic history of real people. So this asks the questions itself, against a
// list of addresses that comes from our own configuration and can contain
// nothing else.
//
// It answers from one vantage point, which is the wrong one. Blocking happens
// between a user in Russia and us; from Helsinki everything looks fine right
// up until nobody can connect. That is why the same addresses are also checked
// by the devices themselves, and why the panel shows the two side by side: an
// address we can reach and devices cannot is not a broken server.
package probe

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"download.simplevpn/control-plane/internal/store"
)

type Runner struct {
	store *store.Store
	log   *slog.Logger

	// every is how often the whole list is walked. Minutes rather than
	// seconds: this is looking for an address that has stopped working, not
	// for a packet that went missing, and every check is a request somebody
	// could count.
	every time.Duration

	client *http.Client
}

func New(st *store.Store, every time.Duration, log *slog.Logger) *Runner {
	return &Runner{
		store: st,
		every: every,
		log:   log,
		client: &http.Client{
			Timeout: 10 * time.Second,
			// A redirect is an answer. Following it would measure wherever it
			// led instead of the address that was asked about.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Run walks the list until the context ends.
func (r *Runner) Run(ctx context.Context) {
	ticker := time.NewTicker(r.every)
	defer ticker.Stop()

	r.round(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.round(ctx)
		}
	}
}

func (r *Runner) round(ctx context.Context) {
	// Every domain in use gets a row before it is checked, so that a domain
	// cannot be serving people and unknown to the lifecycle at the same time.
	// A domain with no row is a domain nobody can decide to retire.
	if err := r.store.RememberServedDomains(ctx); err != nil {
		r.log.Error("cannot record the domains in use", "error", err)
	}

	names, err := r.store.ServedNames(ctx)
	if err != nil {
		r.log.Error("cannot list what to check", "error", err)
		return
	}

	for _, name := range names {
		ok, latency, detail := r.check(ctx, name)
		var ms *int
		if latency > 0 {
			value := int(latency.Milliseconds())
			ms = &value
		}
		if err := r.store.RecordProbe(ctx, time.Now().UTC(), name, "control-plane", ok, ms, detail); err != nil {
			r.log.Error("cannot record a check", "error", err)
		}
	}
}

// check asks one address whether it is still there.
//
// What comes back is reduced to a word from a fixed list before it is stored.
// A server's own error text is not written down: it can contain an address, a
// hostname or a chain of them, and a database with no column for a destination
// should not acquire one through a message somebody else wrote.
func (r *Runner) check(ctx context.Context, name string) (bool, time.Duration, string) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+name+"/", nil)
	if err != nil {
		return false, 0, "bad-target"
	}

	started := time.Now()
	answer, err := r.client.Do(request)
	elapsed := time.Since(started)
	if err != nil {
		if ctx.Err() != nil {
			return false, 0, "stopped"
		}
		return false, elapsed, "unreachable"
	}
	defer answer.Body.Close()

	if answer.StatusCode >= 500 {
		return false, elapsed, "server-error"
	}
	return true, elapsed, ""
}

// Housekeeper folds yesterday into summaries and drops what is past keeping.
//
// Retention that depends on somebody remembering to run something is not
// retention, and the promise made in the schema - that raw minute rows do not
// survive - is only true if something enforces it.
type Housekeeper struct {
	store       *store.Store
	log         *slog.Logger
	rawKeep     time.Duration
	summaryKeep time.Duration
}

func NewHousekeeper(st *store.Store, rawKeep, summaryKeep time.Duration, log *slog.Logger) *Housekeeper {
	return &Housekeeper{store: st, log: log, rawKeep: rawKeep, summaryKeep: summaryKeep}
}

func (h *Housekeeper) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	h.once(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.once(ctx)
		}
	}
}

func (h *Housekeeper) once(ctx context.Context) {
	now := time.Now().UTC()

	// Today as well as yesterday: today's summary is incomplete and will be
	// rewritten on the next pass, which is better than a panel that shows
	// nothing for the day it is being read on.
	for _, day := range []time.Time{now.AddDate(0, 0, -1), now} {
		if err := h.store.RollUpDay(ctx, day.Truncate(24*time.Hour)); err != nil {
			h.log.Error("cannot roll up a day", "error", err)
		}
	}

	if err := h.store.DropOldSamples(ctx, h.rawKeep, h.summaryKeep); err != nil {
		h.log.Error("cannot apply retention", "error", err)
	}
}
