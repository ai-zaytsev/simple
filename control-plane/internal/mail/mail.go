// Package mail sends the messages this product sends.
package mail

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Sender delivers messages through the transactional provider.
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

	// Where a reply goes. The address messages are sent from is a no-reply
	// subdomain that accepts nothing, so without this a person answering a
	// message from us is writing into a hole and has no way to know it.
	replyTo string

	client *http.Client
}

// NewSender builds a sender. An empty replyTo means messages carry no
// Reply-To, which is the old behaviour and is still correct where nobody is
// meant to answer.
func NewSender(apiKey, fromAddress, fromName, replyTo string) *Sender {
	return &Sender{
		apiKey:      apiKey,
		fromAddress: fromAddress,
		fromName:    fromName,
		replyTo:     replyTo,
		client:      &http.Client{Timeout: 15 * time.Second},
	}
}

type request struct {
	Sender      party   `json:"sender"`
	To          []party `json:"to"`
	ReplyTo     *party  `json:"replyTo,omitempty"`
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

	return s.post(s.message(to, "Вход в приложение", fmt.Sprintf(
		"Откройте ссылку, чтобы завершить вход:\n\n%s\n\n"+
			"Ссылка действует %d минут и срабатывает один раз.\n\n"+
			"Если вы не запрашивали вход, письмо можно не открывать: "+
			"без перехода по ссылке ничего не произойдёт.\n",
		link, minutes)))
}

// Send delivers an ordinary message to one address.
//
// Separate from SendSignInLink because the two have different rules about
// their contents: a sign-in link is written once and never changes, while this
// carries whatever the caller wants. It is used for operational alerts, which
// go to us rather than to a user, and nothing in this package checks that -
// the caller decides who it is talking to.
func (s *Sender) Send(to, subject, text string) error {
	_, err := s.post(s.message(to, subject, text))
	return err
}

func (s *Sender) message(to, subject, text string) request {
	body := request{
		Sender:      party{Email: s.fromAddress, Name: s.fromName},
		To:          []party{{Email: to}},
		Subject:     subject,
		TextContent: text,
	}
	if s.replyTo != "" {
		body.ReplyTo = &party{Email: s.replyTo, Name: s.fromName}
	}
	return body
}

// post is the one place that talks to the provider.
//
// One place rather than two, because the rules that matter here - never log
// the body, treat a missing message id as success - are easy to get right once
// and easy to let drift when they are written twice.
func (s *Sender) post(body request) (string, error) {
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
		// The status and nothing else. The body echoes the recipient, and an
		// address in a log is the one thing this system must never write down
		// casually.
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
