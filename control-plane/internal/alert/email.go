package alert

import (
	"context"
	"fmt"

	"download.simplevpn/control-plane/internal/mail"
)

// Email is the first channel, and deliberately the least interesting one.
//
// It is thirty lines because that is all a channel should be. The next one -
// a messenger, a webhook, a phone - is another file this size, and nothing
// outside it changes: not the rule that decided to warn, not the wording, not
// the decision about whether to repeat.
type Email struct {
	Sender *mail.Sender
	To     string
}

func (e Email) Name() string { return "email" }

func (e Email) Send(_ context.Context, a Alert) error {
	if e.Sender == nil || e.To == "" {
		return fmt.Errorf("email is not configured")
	}

	// The state in the subject line, because the first thing somebody needs
	// from a message on a phone screen is whether to open it now.
	subject := fmt.Sprintf("[%s] %s", a.State, a.Headline)
	return e.Sender.Send(e.To, subject, a.Text())
}
