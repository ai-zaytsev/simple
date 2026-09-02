package npd

import (
	"context"
	"errors"

	"download.simplevpn/control-plane/internal/mail"
)

// Email is the channel these letters travel on.
//
// Its own small type rather than reuse of internal/alert.Email, because that
// one carries an Alert - a state, a headline, reasons - and these letters are
// not that shape. A tax receipt failure has no "state" to put in a subject
// line and no calm state to return to; it is one payment, once, with the
// fields somebody needs to fix it by hand.
type Email struct {
	Sender *mail.Sender
	To     string
}

func (e Email) Name() string { return "email" }

func (e Email) Send(_ context.Context, subject, body string) error {
	if e.Sender == nil || e.To == "" {
		return errors.New("почта для писем о чеках не настроена")
	}
	return e.Sender.Send(e.To, subject, body)
}
