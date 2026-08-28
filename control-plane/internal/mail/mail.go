// Package mail sends the one message this product sends.
package mail

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Sender delivers sign-in links through the transactional provider.
//
// Plain text only, and that is a privacy decision rather than a stylistic one:
// an open-tracking pixel is an image in an HTML body, and a message with no
// HTML body cannot carry one. The provider's click tracking is off for the same
// reason - it would rewrite the link through a counting redirect and record who
// followed it and when.
type Sender struct {
	apiKey      string
	fromAddress string
	fromName    string
	client      *http.Client
}

func NewSender(apiKey, fromAddress, fromName string) *Sender {
	return &Sender{
		apiKey:      apiKey,
		fromAddress: fromAddress,
		fromName:    fromName,
		client:      &http.Client{Timeout: 15 * time.Second},
	}
}

type request struct {
	Sender      party   `json:"sender"`
	To          []party `json:"to"`
	Subject     string  `json:"subject"`
	TextContent string  `json:"textContent"`
}

type party struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

// SendSignInLink delivers the link and returns the provider's message id.
//
// The message id is kept so that a delivery report arriving later can be
// matched against something we actually sent; a report about a message we never
// sent is discarded rather than acted on.
func (s *Sender) SendSignInLink(to, link string, validFor time.Duration) (string, error) {
	minutes := int(validFor.Minutes())

	body := request{
		Sender:  party{Email: s.fromAddress, Name: s.fromName},
		To:      []party{{Email: to}},
		Subject: "Вход в приложение",
		TextContent: fmt.Sprintf(
			"Откройте ссылку, чтобы завершить вход:\n\n%s\n\n"+
				"Ссылка действует %d минут и срабатывает один раз.\n\n"+
				"Если вы не запрашивали вход, письмо можно не открывать: "+
				"без перехода по ссылке ничего не произойдёт.\n",
			link, minutes),
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("cannot build the message: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost,
		"https://api.brevo.com/v3/smtp/email", bytes.NewReader(encoded))
	if err != nil {
		return "", fmt.Errorf("cannot build the request: %w", err)
	}
	req.Header.Set("api-key", s.apiKey)
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("provider unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		// The body is not included. It echoes the recipient, and an address in
		// a log is the one thing this system must never write down casually.
		return "", fmt.Errorf("provider refused the message: HTTP %d", resp.StatusCode)
	}

	var answer struct {
		MessageID string `json:"messageId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		// Delivered but unreportable. Not an error for the caller: the message
		// is on its way, and only the ability to match a later report is lost.
		return "", nil
	}
	return answer.MessageID, nil
}

// Send delivers an ordinary message to one address.
//
// Separate from SendSignInLink because the two have different rules about
// their contents: a sign-in link is written once and never changes, while this
// carries whatever the sender wants. It is used for operational alerts, which
// go to us rather than to a user, and nothing in this package checks that -
// the caller decides who it is talking to.
func (s *Sender) Send(to, subject, text string) error {
	body := request{
		Sender:      party{Email: s.fromAddress, Name: s.fromName},
		To:          []party{{Email: to}},
		Subject:     subject,
		TextContent: text,
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("cannot build the message: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost,
		"https://api.brevo.com/v3/smtp/email", bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("cannot build the request: %w", err)
	}
	req.Header.Set("api-key", s.apiKey)
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("provider unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		// The status and nothing else. The body echoes the recipient, and an
		// address in a log is the one thing this system must never write down
		// casually.
		return fmt.Errorf("provider refused the message: HTTP %d", resp.StatusCode)
	}
	return nil
}
