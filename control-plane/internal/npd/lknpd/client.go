// Package lknpd talks to lknpd.nalog.ru and is the only thing in Core that
// does.
//
// The API is unofficial. It can change shape, start asking for a CAPTCHA, or
// answer with a maintenance page, and none of that is a bug on our side - it
// is the weather. So this package has one job beyond making the calls: to tell
// the difference between "the tax service is unwell" and "we asked wrongly",
// because the first blocks tomorrow's sales and the second is ours to fix.
//
// Every path, field name and literal here was read out of the MIT project
// inache-su/moy-nalog-api rather than remembered. docs/integrations/lknpd.md
// records what was read and from which file. Nothing else in this repository
// can catch a mistake in these strings: the compiler does not know that the
// path is /income, and a wrong one fails only against the real service.
package lknpd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	apiV1 = "https://lknpd.nalog.ru/api/v1"

	// The reason text that goes into a cancellation. A Russian sentence, not a
	// code - the API takes the words themselves.
	reasonRefund = "Возврат средств"

	// A card payment by an individual. WIRE is for legal entities.
	paymentTypeCash = "CASH"

	incomeFromIndividual = "FROM_INDIVIDUAL"
)

// ErrServiceUnavailable is the tax service being unwell rather than us being
// wrong. It is the difference between waiting and fixing, so it has a name.
var ErrServiceUnavailable = errors.New("сервис ФНС временно недоступен")

// ErrUnauthorized means the session is not usable: expired, revoked, or the
// login itself was refused.
var ErrUnauthorized = errors.New("сессия ФНС недействительна")

// Session is everything needed to keep talking without logging in again.
//
// Carried in and out rather than kept in the client, so that the thing that
// stores it decides where it lives. None of these fields may be logged.
type Session struct {
	INN          string
	DeviceID     string
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

// Client is a thin adapter. It holds no state between calls beyond the session
// it was handed.
type Client struct {
	http      *http.Client
	userAgent string
	now       func() time.Time

	// The timezone receipts are dated in. The tax service reads the offset
	// from the timestamp, so this must be the taxpayer's own zone.
	location *time.Location

	// The API address. A constant in production; a stub in tests.
	baseURL string
}

// newAt exists so tests can aim the adapter at a stub. The real address is a
// constant and not configuration: a deployment pointed somewhere else would
// file somebody else's taxes.
func newAt(httpClient *http.Client, location *time.Location, baseURL string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if location == nil {
		location = time.FixedZone("MSK", 3*60*60)
	}
	return &Client{
		http:      httpClient,
		userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36",
		now:       time.Now,
		location:  location,
		baseURL:   baseURL,
	}
}

func (c *Client) deviceInfo(deviceID string) map[string]any {
	return map[string]any{
		"sourceType":     "WEB",
		"sourceDeviceId": deviceID,
		"appVersion":     "1.0.0",
		"metaDetails":    map[string]any{"userAgent": c.userAgent},
	}
}

// Login exchanges an INN and password for a session.
//
// Called only when there is no usable session: a login loop against an
// unofficial API is how an account earns a CAPTCHA.
func (c *Client) Login(ctx context.Context, inn, password, deviceID string) (Session, error) {
	body := map[string]any{
		"username":   inn,
		"password":   password,
		"deviceInfo": c.deviceInfo(deviceID),
	}
	var out authResponse
	if err := c.call(ctx, http.MethodPost, "/auth/lkfl", "", body, &out); err != nil {
		return Session{}, err
	}
	return c.sessionFrom(out, inn, deviceID)
}

// Refresh renews an access token without a password.
func (c *Client) Refresh(ctx context.Context, s Session) (Session, error) {
	if strings.TrimSpace(s.RefreshToken) == "" {
		return Session{}, fmt.Errorf("%w: обновлять нечем", ErrUnauthorized)
	}
	body := map[string]any{
		"refreshToken": s.RefreshToken,
		"deviceInfo":   c.deviceInfo(s.DeviceID),
	}
	var out authResponse
	if err := c.call(ctx, http.MethodPost, "/auth/token", "", body, &out); err != nil {
		return Session{}, err
	}
	renewed, err := c.sessionFrom(out, s.INN, s.DeviceID)
	if err != nil {
		return Session{}, err
	}
	// The refresh response may keep the old refresh token rather than issue a
	// new one. Dropping it would turn a working session into a dead one.
	if renewed.RefreshToken == "" {
		renewed.RefreshToken = s.RefreshToken
	}
	return renewed, nil
}

func (c *Client) sessionFrom(out authResponse, inn, deviceID string) (Session, error) {
	if strings.TrimSpace(out.Token) == "" {
		return Session{}, fmt.Errorf("%w: ответ без токена", ErrUnauthorized)
	}
	session := Session{
		INN:          inn,
		DeviceID:     deviceID,
		AccessToken:  out.Token,
		RefreshToken: out.RefreshToken,
	}
	if out.Profile.INN != "" {
		session.INN = out.Profile.INN
	}
	if out.TokenExpireIn != "" {
		if at, err := time.Parse(time.RFC3339, out.TokenExpireIn); err == nil {
			session.ExpiresAt = at
		}
	}
	return session, nil
}

// Alive is the one authorised read used to decide whether selling is allowed
// today. It asks for the profile: cheap, and it proves the token works rather
// than only that the host answers.
func (c *Client) Alive(ctx context.Context, s Session) error {
	var out map[string]any
	if err := c.call(ctx, http.MethodGet, "/user", s.AccessToken, nil, &out); err != nil {
		return err
	}
	if len(out) == 0 {
		return errors.New("профиль пуст")
	}
	return nil
}

// CreateReceipt registers income and returns the receipt id and its printable
// address.
//
// amountMinor is what the customer paid, before the payment provider's
// commission: the receipt is for their payment, not for our takings.
func (c *Client) CreateReceipt(
	ctx context.Context, s Session, name string, amountMinor int64, at time.Time,
) (string, string, error) {
	amount := formatAmount(amountMinor)
	body := map[string]any{
		"operationTime": at.In(c.location).Format(time.RFC3339),
		"requestTime":   c.now().In(c.location).Format(time.RFC3339),
		"services": []map[string]any{{
			"name":     name,
			"amount":   amount,
			"quantity": 1,
		}},
		"totalAmount": amount,
		"client": map[string]any{
			"incomeType":   incomeFromIndividual,
			"displayName":  nil,
			"contactPhone": nil,
			"inn":          nil,
		},
		"paymentType":                     paymentTypeCash,
		"ignoreMaxTotalIncomeRestriction": false,
	}
	var out receiptResponse
	if err := c.call(ctx, http.MethodPost, "/income", s.AccessToken, body, &out); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(out.ApprovedReceiptUUID) == "" {
		return "", "", errors.New("ФНС не вернула идентификатор чека")
	}
	return out.ApprovedReceiptUUID,
		fmt.Sprintf("%s/receipt/%s/%s/print", c.baseURL, s.INN, out.ApprovedReceiptUUID),
		nil
}

// CancelReceipt voids a receipt because money went back.
func (c *Client) CancelReceipt(ctx context.Context, s Session, receiptUUID string, at time.Time) error {
	body := map[string]any{
		"operationTime": at.In(c.location).Format(time.RFC3339),
		"requestTime":   c.now().In(c.location).Format(time.RFC3339),
		"comment":       reasonRefund,
		"receiptUuid":   receiptUUID,
		"partnerCode":   nil,
	}
	var out receiptResponse
	return c.call(ctx, http.MethodPost, "/cancel", s.AccessToken, body, &out)
}

// ReceiptExists answers whether a receipt is still there, for reconciliation
// after an outage. A 404 is an answer, not a failure.
func (c *Client) ReceiptExists(ctx context.Context, s Session, receiptUUID string) (bool, error) {
	var out map[string]any
	err := c.call(ctx, http.MethodGet,
		fmt.Sprintf("/receipt/%s/%s/json", s.INN, receiptUUID), s.AccessToken, nil, &out)
	var status statusError
	if errors.As(err, &status) && status.code == http.StatusNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

type authResponse struct {
	Token         string `json:"token"`
	RefreshToken  string `json:"refreshToken"`
	TokenExpireIn string `json:"tokenExpireIn"`
	Profile       struct {
		INN string `json:"inn"`
	} `json:"profile"`
}

type receiptResponse struct {
	ApprovedReceiptUUID string `json:"approvedReceiptUuid"`
}

// statusError carries the HTTP status so that a 404 can mean "no such
// receipt" without the caller parsing prose.
type statusError struct {
	code    int
	message string
}

func (e statusError) Error() string {
	return fmt.Sprintf("ФНС ответила %d: %s", e.code, e.message)
}

func (c *Client) call(ctx context.Context, method, path, token string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("не удалось собрать запрос: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("не удалось подготовить запрос: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Referer", "https://lknpd.nalog.ru/")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// A network failure is the service being unreachable, which is the
		// same decision as the service being unwell: wait, do not fix.
		return fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("%w: ответ не дочитан", ErrServiceUnavailable)
	}

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		if out == nil {
			return nil
		}
		if err := json.Unmarshal(raw, out); err != nil {
			// A body that is not the JSON we expect is how a changed API and a
			// CAPTCHA page both look. Neither is ours to fix in the moment,
			// and both must stop tomorrow's sales rather than be retried.
			return fmt.Errorf("%w: ответ не разобран как JSON", ErrServiceUnavailable)
		}
		return nil
	}

	message, code := problemFrom(raw)

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("%w: %s", ErrUnauthorized, message)
	case unavailable(resp.StatusCode, message, code):
		return fmt.Errorf("%w: %s", ErrServiceUnavailable, message)
	}
	return statusError{code: resp.StatusCode, message: message}
}

func problemFrom(raw []byte) (string, string) {
	var problem struct {
		Message          string `json:"message"`
		ExceptionMessage string `json:"exceptionMessage"`
		Code             string `json:"code"`
	}
	if err := json.Unmarshal(raw, &problem); err != nil {
		// Not JSON at all. Say so without echoing the body: a maintenance page
		// or a CAPTCHA is large and its contents are of no use in a log.
		return "ответ не в формате JSON", ""
	}
	message := problem.Message
	if message == "" {
		message = problem.ExceptionMessage
	}
	if message == "" {
		message = "без пояснения"
	}
	return message, problem.Code
}

// unavailable classifies a temporary outage from the answer itself rather than
// from a maintenance schedule nobody publishes.
func unavailable(status int, message, code string) bool {
	if status == http.StatusServiceUnavailable || status == http.StatusBadGateway ||
		status == http.StatusGatewayTimeout || status == http.StatusTooManyRequests {
		return true
	}
	haystack := strings.ToLower(message + " " + code)
	if strings.Contains(haystack, "техническ") && strings.Contains(haystack, "работ") {
		return true
	}
	for _, needle := range []string{
		"maintenance", "service unavailable", "service_unavailable", "временно недоступ",
	} {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

// formatAmount turns minor units into the string the API wants: "399.00".
func formatAmount(minor int64) string {
	return fmt.Sprintf("%d.%02d", minor/100, minor%100)
}

// New builds the adapter aimed at the real tax service.
func New(httpClient *http.Client, location *time.Location) *Client {
	return newAt(httpClient, location, apiV1)
}
