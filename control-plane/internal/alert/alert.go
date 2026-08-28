// Package alert carries a message somewhere, and knows nothing about what the
// message is about.
//
// The point of the separation is that where a message goes changes and what
// makes it worth sending does not. Today it is email; a messenger, a webhook
// and a phone are all the same shape of thing, and adding one has to be one
// new file rather than an edit to the rule that decided to say something.
//
// So a channel is an interface with one method, the decision about whether to
// speak at all lives here rather than in any channel, and the thing being
// announced is a value with no idea how it travels.
package alert

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Alert is something worth telling a person.
type Alert struct {
	// One of the four words. Carried as a string because this package has no
	// business knowing what states exist.
	State string

	// What it is about - "capacity", "domains" - so that two different worries
	// do not silence one another.
	Subject string

	// One line somebody can act on, and the reasoning behind it.
	Headline string
	Reasons  []string

	// The numbers as they were, so that a message read an hour later still
	// means something.
	Facts []string
}

// Text renders the alert for a channel that wants plain words.
//
// Here rather than in each channel: three channels rendering the same alert
// three ways is three places for it to be worded badly, and the difference
// between them would be noise rather than adaptation.
func (a Alert) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s\n", a.State, a.Headline)
	if len(a.Reasons) > 0 {
		b.WriteString("\nWhy:\n")
		for _, line := range a.Reasons {
			fmt.Fprintf(&b, "  - %s\n", line)
		}
	}
	if len(a.Facts) > 0 {
		b.WriteString("\nAs it stands:\n")
		for _, line := range a.Facts {
			fmt.Fprintf(&b, "  %s\n", line)
		}
	}
	return b.String()
}

// Channel is somewhere a message can go.
//
// One method, and it takes a whole Alert rather than a rendered string, so a
// channel that can do better than plain text - buttons, colour, a thread - is
// free to, without the sender knowing.
type Channel interface {
	Name() string
	Send(ctx context.Context, a Alert) error
}

// Memory is where the notifier remembers what it has already said.
//
// An interface so that this package does not depend on the database. The only
// thing it needs to know is when something was last said about a subject and
// in what state.
type Memory interface {
	LastSaid(ctx context.Context, subject string) (state string, at time.Time, err error)
	RecordAlert(ctx context.Context, subject, state, channel string, ok bool, detail string) error
}

// Notifier decides whether to speak, and then speaks everywhere at once.
type Notifier struct {
	channels []Channel
	memory   Memory
	log      *slog.Logger

	// How long a state has to have been said before it is said again. An
	// alert that repeats every five minutes is an alert people filter, and a
	// filtered alert is no alert with extra steps.
	repeatAfter time.Duration
}

func New(memory Memory, repeatAfter time.Duration, log *slog.Logger, channels ...Channel) *Notifier {
	if repeatAfter <= 0 {
		repeatAfter = 12 * time.Hour
	}
	return &Notifier{channels: channels, memory: memory, log: log, repeatAfter: repeatAfter}
}

// Consider sends the alert if it is worth sending, and says whether it did.
//
// Worth sending means the state has changed, or enough time has passed that
// somebody who missed it deserves reminding. A return to the calm state is
// worth sending once, because "it is over" is information; being calm for a
// week is not.
func (n *Notifier) Consider(ctx context.Context, a Alert, calm string) (bool, error) {
	if len(n.channels) == 0 {
		return false, nil
	}

	previous, at, err := n.memory.LastSaid(ctx, a.Subject)
	if err != nil {
		return false, fmt.Errorf("cannot tell what was already said: %w", err)
	}

	changed := previous != a.State
	stale := !at.IsZero() && time.Since(at) > n.repeatAfter

	switch {
	case changed:
		// Say it.
	case a.State == calm:
		// Calm and unchanged. Nothing has happened; saying so is noise.
		return false, nil
	case stale:
		// Still wrong, and long enough that it is worth saying again.
	default:
		return false, nil
	}

	// Nothing has been said before and everything is fine: the first thing a
	// new deployment does should not be to announce that it is well.
	if previous == "" && a.State == calm {
		return false, n.memory.RecordAlert(ctx, a.Subject, a.State, "none", true, "first look, nothing wrong")
	}

	sent := false
	for _, channel := range n.channels {
		err := channel.Send(ctx, a)
		detail := ""
		if err != nil {
			detail = err.Error()
			n.log.Error("could not send an alert",
				"channel", channel.Name(), "subject", a.Subject, "error", err)
		} else {
			sent = true
		}
		if recordErr := n.memory.RecordAlert(ctx, a.Subject, a.State, channel.Name(), err == nil, detail); recordErr != nil {
			n.log.Error("could not record an alert", "error", recordErr)
		}
	}
	return sent, nil
}

// Logged is the channel that always exists.
//
// Not a fallback for when another one fails: it is there so that a deployment
// with no channel configured still leaves a trace of what it would have said,
// and so that "did it decide to warn me" can be answered without a mailbox.
type Logged struct{ Log *slog.Logger }

func (l Logged) Name() string { return "log" }

func (l Logged) Send(_ context.Context, a Alert) error {
	l.Log.Warn("capacity", "state", a.State, "subject", a.Subject, "headline", a.Headline,
		"reasons", strings.Join(a.Reasons, "; "))
	return nil
}
