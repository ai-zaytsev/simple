// Package dnsedit writes the one record a certificate authority asks for.
//
// It exists so that a node never holds a key to our DNS. A node that could
// edit DNS is a node whose compromise hands over every domain we own; a node
// that only receives a finished certificate hands over one certificate, for
// one machine, which is replaced by destroying that machine.
//
// Two providers, because our domains live at two registrars on purpose - a
// registrar is a single point of failure like any other, and the recovery
// channels depend on them being different.
package dnsedit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Editor writes and removes the TXT record used to prove we own a domain.
//
// Deliberately narrow: it can touch one name under one domain and nothing
// else. A general DNS client would be a more useful tool and a worse one to
// hold the credentials for.
type Editor struct {
	cloudflareToken string
	spaceshipKey    string
	spaceshipSecret string
	client          *http.Client
}

func New(cloudflareToken, spaceshipKey, spaceshipSecret string) *Editor {
	return &Editor{
		cloudflareToken: cloudflareToken,
		spaceshipKey:    spaceshipKey,
		spaceshipSecret: spaceshipSecret,
		client:          &http.Client{Timeout: 30 * time.Second},
	}
}

// The subdomain a certificate authority reads. Fixed by the ACME standard.
const challengePrefix = "_acme-challenge"

// Present publishes the proof for a domain.
//
// Which provider is asked is decided by which one knows the domain rather than
// by configuration: a list of domains per provider is one more thing to keep
// in step with reality, and it goes stale the first time a domain moves.
func (e *Editor) Present(ctx context.Context, domain, value string) error {
	if e.cloudflareHas(ctx, domain) {
		return e.cloudflarePresent(ctx, domain, value)
	}
	return e.spaceshipPresent(ctx, domain, value)
}

// CleanUp removes the proof.
//
// Failure here is reported but is not fatal to issuance: a certificate that has
// already been granted is not withdrawn because a record was left behind. The
// record is harmless on its own - it proves nothing to anybody who does not
// already control the domain - but it is removed because a name littered with
// stale proofs tells a reader how often we issue.
func (e *Editor) CleanUp(ctx context.Context, domain string) error {
	if e.cloudflareHas(ctx, domain) {
		return e.cloudflareRemove(ctx, domain)
	}
	return e.spaceshipRemove(ctx, domain)
}

// ---- Cloudflare ----------------------------------------------------------

func (e *Editor) cloudflareZone(ctx context.Context, domain string) (string, bool) {
	if e.cloudflareToken == "" {
		return "", false
	}

	endpoint := "https://api.cloudflare.com/client/v4/zones?name=" + url.QueryEscape(domain)
	var answer struct {
		Success bool `json:"success"`
		Result  []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := e.call(ctx, http.MethodGet, endpoint, nil, e.cloudflareHeaders(), &answer); err != nil {
		return "", false
	}
	if !answer.Success || len(answer.Result) == 0 {
		return "", false
	}
	return answer.Result[0].ID, true
}

func (e *Editor) cloudflareHas(ctx context.Context, domain string) bool {
	_, ok := e.cloudflareZone(ctx, domain)
	return ok
}

func (e *Editor) cloudflarePresent(ctx context.Context, domain, value string) error {
	zone, ok := e.cloudflareZone(ctx, domain)
	if !ok {
		return fmt.Errorf("cloudflare does not know %s", domain)
	}

	// Stale proofs go first, explicitly. A second record beside the first does
	// not replace it: the authority may read either, and reading the old one
	// fails the check for a reason nothing in the logs explains.
	if err := e.cloudflareRemoveIn(ctx, zone, domain); err != nil {
		return err
	}

	body := map[string]any{
		"type":    "TXT",
		"name":    challengePrefix + "." + domain,
		"content": value,
		// As short as the provider allows. The record lives for the length of
		// one check and a long one would keep answering after the proof has
		// stopped being true.
		"ttl": 60,
	}
	endpoint := "https://api.cloudflare.com/client/v4/zones/" + zone + "/dns_records"
	return e.call(ctx, http.MethodPost, endpoint, body, e.cloudflareHeaders(), nil)
}

func (e *Editor) cloudflareRemove(ctx context.Context, domain string) error {
	zone, ok := e.cloudflareZone(ctx, domain)
	if !ok {
		return nil
	}
	return e.cloudflareRemoveIn(ctx, zone, domain)
}

func (e *Editor) cloudflareRemoveIn(ctx context.Context, zone, domain string) error {
	name := challengePrefix + "." + domain
	endpoint := "https://api.cloudflare.com/client/v4/zones/" + zone +
		"/dns_records?type=TXT&name=" + url.QueryEscape(name)

	var listing struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := e.call(ctx, http.MethodGet, endpoint, nil, e.cloudflareHeaders(), &listing); err != nil {
		return err
	}

	for _, record := range listing.Result {
		remove := "https://api.cloudflare.com/client/v4/zones/" + zone + "/dns_records/" + record.ID
		if err := e.call(ctx, http.MethodDelete, remove, nil, e.cloudflareHeaders(), nil); err != nil {
			return err
		}
	}
	return nil
}

func (e *Editor) cloudflareHeaders() map[string]string {
	return map[string]string{
		"authorization": "Bearer " + e.cloudflareToken,
		"content-type":  "application/json",
	}
}

// ---- Spaceship -----------------------------------------------------------

type spaceshipRecord struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
	TTL   int    `json:"ttl,omitempty"`
}

func (e *Editor) spaceshipPresent(ctx context.Context, domain, value string) error {
	if e.spaceshipKey == "" {
		return fmt.Errorf("no provider knows %s", domain)
	}

	if err := e.spaceshipRemove(ctx, domain); err != nil {
		return err
	}

	body := map[string]any{
		// The flag means "overwrite a matching record", not "be the only
		// record here" - a distinction this project has already paid for once,
		// when a domain ended up answering with one live address and two dead
		// ones. Hence the removal above rather than trust in the flag.
		"force": true,
		"items": []spaceshipRecord{
			{Type: "TXT", Name: challengePrefix, Value: value, TTL: 60},
		},
	}
	endpoint := "https://spaceship.dev/api/v1/dns/records/" + domain
	return e.call(ctx, http.MethodPut, endpoint, body, e.spaceshipHeaders(), nil)
}

func (e *Editor) spaceshipRemove(ctx context.Context, domain string) error {
	if e.spaceshipKey == "" {
		return nil
	}

	endpoint := "https://spaceship.dev/api/v1/dns/records/" + domain + "?take=100&skip=0"
	var listing struct {
		Items []spaceshipRecord `json:"items"`
	}
	if err := e.call(ctx, http.MethodGet, endpoint, nil, e.spaceshipHeaders(), &listing); err != nil {
		return err
	}

	stale := []spaceshipRecord{}
	for _, record := range listing.Items {
		if record.Type == "TXT" && strings.HasPrefix(record.Name, challengePrefix) {
			stale = append(stale, record)
		}
	}
	if len(stale) == 0 {
		return nil
	}

	return e.call(ctx, http.MethodDelete, "https://spaceship.dev/api/v1/dns/records/"+domain,
		stale, e.spaceshipHeaders(), nil)
}

func (e *Editor) spaceshipHeaders() map[string]string {
	return map[string]string{
		"x-api-key":    strings.TrimSpace(e.spaceshipKey),
		"x-api-secret": strings.TrimSpace(e.spaceshipSecret),
		"content-type": "application/json",
		"accept":       "application/json",
	}
}

// ---- shared --------------------------------------------------------------

func (e *Editor) call(ctx context.Context, method, endpoint string, body any, headers map[string]string, into any) error {
	var payload *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("cannot build the request: %w", err)
		}
		payload = bytes.NewReader(encoded)
	} else {
		payload = bytes.NewReader(nil)
	}

	request, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
	if err != nil {
		return fmt.Errorf("cannot build the request: %w", err)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}

	response, err := e.client.Do(request)
	if err != nil {
		return fmt.Errorf("provider unreachable: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode >= 300 {
		// The body is not included. A provider echoes what it was sent, and
		// what it was sent includes the proof; a log holding it would let a
		// reader of the log answer a challenge we are in the middle of.
		return fmt.Errorf("provider refused: HTTP %d", response.StatusCode)
	}

	if into == nil {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(into); err != nil {
		return fmt.Errorf("provider answered unreadably: %w", err)
	}
	return nil
}
