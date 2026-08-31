// Package yookassa adapts YooKassa's server API to the general payment
// provider contract. It contains no mobile SDK and exposes no YooKassa shape
// to Android or entitlement code.
package yookassa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"download.simplevpn/control-plane/internal/payment"
)

const defaultBaseURL = "https://api.yookassa.ru/v3"

type Client struct {
	shopID    string
	secretKey string
	baseURL   string
	http      *http.Client
}

func New(shopID, secretKey string, client *http.Client) (*Client, error) {
	return newAt(shopID, secretKey, defaultBaseURL, client)
}

func newAt(shopID, secretKey, baseURL string, client *http.Client) (*Client, error) {
	if strings.TrimSpace(shopID) == "" || strings.TrimSpace(secretKey) == "" {
		return nil, errors.New("shop id and secret key are required")
	}
	for _, character := range strings.TrimSpace(shopID) {
		if character < '0' || character > '9' {
			return nil, errors.New("shop id must be numeric")
		}
	}
	if client == nil {
		client = &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				// API calls do not need browser redirects. Refusing them also means
				// Basic credentials never have to cross a redirected request.
				return http.ErrUseLastResponse
			},
		}
	}
	return &Client{
		shopID:    strings.TrimSpace(shopID),
		secretKey: strings.TrimSpace(secretKey),
		baseURL:   strings.TrimRight(baseURL, "/"),
		http:      client,
	}, nil
}

func (c *Client) Name() string { return "yookassa" }

// YooKassa documents POST idempotency for 24 hours. Core also searches by
// payment and refund metadata before every retry, and stops creating after
// this window if the previous outcome still cannot be established.
func (c *Client) RefundIdempotencyWindow() time.Duration { return 24 * time.Hour }

func (c *Client) Create(ctx context.Context, in payment.CreateRequest) (payment.Checkout, error) {
	payload := struct {
		Amount struct {
			Value    string `json:"value"`
			Currency string `json:"currency"`
		} `json:"amount"`
		Capture      bool `json:"capture"`
		Confirmation struct {
			Type      string `json:"type"`
			ReturnURL string `json:"return_url"`
		} `json:"confirmation"`
		Description string            `json:"description"`
		Metadata    map[string]string `json:"metadata"`
	}{}
	payload.Amount.Value = formatAmount(in.AmountMinor)
	payload.Amount.Currency = in.Currency
	payload.Capture = true
	payload.Confirmation.Type = "redirect"
	payload.Confirmation.ReturnURL = in.ReturnURL
	payload.Description = in.Description
	payload.Metadata = map[string]string{"payment_id": in.PaymentID}

	body, err := json.Marshal(payload)
	if err != nil {
		return payment.Checkout{}, fmt.Errorf("cannot encode create payment request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/payments", bytes.NewReader(body))
	if err != nil {
		return payment.Checkout{}, fmt.Errorf("cannot prepare create payment request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")
	req.Header.Set("Idempotence-Key", in.IdempotencyKey)
	c.authorize(req)

	var raw providerPayment
	if err := c.do(req, &raw); err != nil {
		return payment.Checkout{}, err
	}
	status, err := statusOf(raw.Status)
	if err != nil {
		return payment.Checkout{}, err
	}
	checkoutURL, err := url.Parse(raw.Confirmation.URL)
	if err != nil || checkoutURL.Scheme != "https" || checkoutURL.Host == "" {
		return payment.Checkout{}, errors.New("payment provider returned no safe checkout address")
	}
	if strings.TrimSpace(raw.ID) == "" {
		return payment.Checkout{}, errors.New("payment provider returned no payment id")
	}
	return payment.Checkout{
		ProviderPaymentID: raw.ID,
		URL:               checkoutURL.String(),
		Status:            status,
		Test:              raw.Test,
	}, nil
}

func (c *Client) Get(ctx context.Context, providerPaymentID string) (payment.Canonical, error) {
	if strings.TrimSpace(providerPaymentID) == "" {
		return payment.Canonical{}, errors.New("provider payment id is empty")
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		c.baseURL+"/payments/"+url.PathEscape(providerPaymentID),
		nil,
	)
	if err != nil {
		return payment.Canonical{}, fmt.Errorf("cannot prepare payment lookup: %w", err)
	}
	req.Header.Set("accept", "application/json")
	c.authorize(req)

	var raw providerPayment
	if err := c.do(req, &raw); err != nil {
		return payment.Canonical{}, err
	}
	status, err := statusOf(raw.Status)
	if err != nil {
		return payment.Canonical{}, err
	}
	amount, err := parseAmount(raw.Amount.Value)
	if err != nil {
		return payment.Canonical{}, errors.New("payment provider returned an unreadable amount")
	}

	var paidAt *time.Time
	if raw.CapturedAt != "" {
		parsed, parseErr := time.Parse(time.RFC3339Nano, raw.CapturedAt)
		if parseErr != nil {
			return payment.Canonical{}, errors.New("payment provider returned an unreadable capture time")
		}
		paidAt = &parsed
	}
	return payment.Canonical{
		ProviderPaymentID: raw.ID,
		PaymentID:         raw.Metadata.PaymentID,
		AmountMinor:       amount,
		Currency:          raw.Amount.Currency,
		Status:            status,
		Paid:              raw.Paid,
		Test:              raw.Test,
		PaidAt:            paidAt,
		PaymentMethod:     raw.PaymentMethod.Type,
		Refundable:        raw.Refundable,
	}, nil
}

// RefundLimits contains only capabilities verified for payment methods this
// test-store integration can actually exercise. Unknown methods are allowed a
// full refund but are not guessed to support a partial one.
func (c *Client) RefundLimits(paymentMethod string) payment.RefundLimits {
	return payment.RefundLimits{
		Full:         true,
		Partial:      paymentMethod == "bank_card",
		MinimumMinor: 100,
	}
}

func (c *Client) CreateRefund(
	ctx context.Context, in payment.RefundCreateRequest,
) (payment.RefundOperation, error) {
	payload := struct {
		Amount struct {
			Value    string `json:"value"`
			Currency string `json:"currency"`
		} `json:"amount"`
		PaymentID   string            `json:"payment_id"`
		Description string            `json:"description"`
		Metadata    map[string]string `json:"metadata"`
	}{}
	payload.Amount.Value = formatAmount(in.AmountMinor)
	payload.Amount.Currency = in.Currency
	payload.PaymentID = in.ProviderPaymentID
	payload.Description = in.Description
	payload.Metadata = map[string]string{"refund_id": in.RefundID}
	body, err := json.Marshal(payload)
	if err != nil {
		return payment.RefundOperation{}, fmt.Errorf("cannot encode create refund request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/refunds", bytes.NewReader(body))
	if err != nil {
		return payment.RefundOperation{}, fmt.Errorf("cannot prepare create refund request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")
	req.Header.Set("Idempotence-Key", in.IdempotencyKey)
	c.authorize(req)

	var raw providerRefund
	if err := c.do(req, &raw); err != nil {
		return payment.RefundOperation{}, err
	}
	if strings.TrimSpace(raw.ID) == "" {
		return payment.RefundOperation{}, errors.New("payment provider returned no refund id")
	}
	return payment.RefundOperation{ProviderRefundID: raw.ID}, nil
}

func (c *Client) GetRefund(ctx context.Context, providerRefundID string) (payment.CanonicalRefund, error) {
	if strings.TrimSpace(providerRefundID) == "" {
		return payment.CanonicalRefund{}, errors.New("provider refund id is empty")
	}
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, c.baseURL+"/refunds/"+url.PathEscape(providerRefundID), nil,
	)
	if err != nil {
		return payment.CanonicalRefund{}, fmt.Errorf("cannot prepare refund lookup: %w", err)
	}
	req.Header.Set("accept", "application/json")
	c.authorize(req)

	var raw providerRefund
	if err := c.do(req, &raw); err != nil {
		return payment.CanonicalRefund{}, err
	}
	return canonicalRefund(raw)
}

// FindRefund recovers a POST whose HTTP response was lost. YooKassa's list
// endpoint can filter by the original payment; the private refund_id metadata
// then identifies our one logical operation without trusting a webhook body.
func (c *Client) FindRefund(
	ctx context.Context, providerPaymentID, refundID string,
) (payment.CanonicalRefund, error) {
	if strings.TrimSpace(providerPaymentID) == "" || strings.TrimSpace(refundID) == "" {
		return payment.CanonicalRefund{}, payment.ErrRefundNotFound
	}
	cursor := ""
	for page := 0; page < 100; page++ {
		query := url.Values{}
		query.Set("payment_id", providerPaymentID)
		query.Set("limit", "100")
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		req, err := http.NewRequestWithContext(
			ctx, http.MethodGet, c.baseURL+"/refunds?"+query.Encode(), nil,
		)
		if err != nil {
			return payment.CanonicalRefund{}, fmt.Errorf("cannot prepare refund search: %w", err)
		}
		req.Header.Set("accept", "application/json")
		c.authorize(req)
		var raw struct {
			Items      []providerRefund `json:"items"`
			NextCursor string           `json:"next_cursor"`
		}
		if err := c.do(req, &raw); err != nil {
			return payment.CanonicalRefund{}, err
		}
		for _, item := range raw.Items {
			if item.PaymentID == providerPaymentID && item.Metadata.RefundID == refundID {
				return canonicalRefund(item)
			}
		}
		if strings.TrimSpace(raw.NextCursor) == "" {
			return payment.CanonicalRefund{}, payment.ErrRefundNotFound
		}
		cursor = strings.TrimSpace(raw.NextCursor)
	}
	return payment.CanonicalRefund{}, errors.New("payment provider refund list did not terminate")
}

func canonicalRefund(raw providerRefund) (payment.CanonicalRefund, error) {
	status, err := refundStatusOf(raw.Status)
	if err != nil {
		return payment.CanonicalRefund{}, err
	}
	amount, err := parseAmount(raw.Amount.Value)
	if err != nil {
		return payment.CanonicalRefund{}, errors.New("payment provider returned an unreadable refund amount")
	}
	var createdAt *time.Time
	if raw.CreatedAt != "" {
		parsed, parseErr := time.Parse(time.RFC3339Nano, raw.CreatedAt)
		if parseErr != nil {
			return payment.CanonicalRefund{}, errors.New("payment provider returned an unreadable refund time")
		}
		createdAt = &parsed
	}
	return payment.CanonicalRefund{
		ProviderRefundID: raw.ID, ProviderPaymentID: raw.PaymentID,
		RefundID: raw.Metadata.RefundID, AmountMinor: amount,
		Currency: raw.Amount.Currency, Status: status,
		CancellationReason: refundCancellationReason(raw.CancellationDetails.Reason),
		CreatedAt:          createdAt,
	}, nil
}

func (c *Client) authorize(req *http.Request) {
	// net/http constructs the Basic header in memory. Neither credential is an
	// argument, URL, request body or log field.
	req.SetBasicAuth(c.shopID, c.secretKey)
}

func (c *Client) do(req *http.Request, into any) error {
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: request failed", payment.ErrUnavailable)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		// Drain a bounded body for connection reuse and deliberately throw it
		// away. Provider error bodies can echo submitted metadata; they never
		// belong in our error or log.
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 32<<10))
		if res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500 {
			return fmt.Errorf("%w: HTTP %d", payment.ErrUnavailable, res.StatusCode)
		}
		return fmt.Errorf("%w: HTTP %d", payment.ErrRejected, res.StatusCode)
	}

	if err := json.NewDecoder(io.LimitReader(res.Body, 128<<10)).Decode(into); err != nil {
		return fmt.Errorf("%w: unreadable response", payment.ErrUnavailable)
	}
	return nil
}

type providerPayment struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Paid   bool   `json:"paid"`
	Test   bool   `json:"test"`
	Amount struct {
		Value    string `json:"value"`
		Currency string `json:"currency"`
	} `json:"amount"`
	Confirmation struct {
		URL string `json:"confirmation_url"`
	} `json:"confirmation"`
	Metadata struct {
		PaymentID string `json:"payment_id"`
	} `json:"metadata"`
	CapturedAt    string `json:"captured_at"`
	Refundable    bool   `json:"refundable"`
	PaymentMethod struct {
		Type string `json:"type"`
	} `json:"payment_method"`
}

type providerRefund struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	PaymentID string `json:"payment_id"`
	CreatedAt string `json:"created_at"`
	Amount    struct {
		Value    string `json:"value"`
		Currency string `json:"currency"`
	} `json:"amount"`
	Metadata struct {
		RefundID string `json:"refund_id"`
	} `json:"metadata"`
	CancellationDetails struct {
		Reason string `json:"reason"`
	} `json:"cancellation_details"`
}

func statusOf(raw string) (payment.Status, error) {
	switch raw {
	case "pending", "waiting_for_capture":
		return payment.StatusPending, nil
	case "succeeded":
		return payment.StatusSucceeded, nil
	case "canceled":
		return payment.StatusCanceled, nil
	default:
		return "", errors.New("payment provider returned an unknown status")
	}
}

func refundStatusOf(raw string) (payment.RefundStatus, error) {
	switch raw {
	case "pending":
		return payment.RefundStatusPending, nil
	case "succeeded":
		return payment.RefundStatusSucceeded, nil
	case "canceled":
		return payment.RefundStatusCanceled, nil
	default:
		return "", errors.New("payment provider returned an unknown refund status")
	}
}

func refundCancellationReason(raw string) string {
	switch raw {
	case "":
		return ""
	case "insufficient_funds", "general_decline", "rejected_by_payee",
		"rejected_by_timeout", "yoo_money_account_closed", "payment_expired":
		return raw
	default:
		return "provider_declined"
	}
}

func formatAmount(minor int64) string {
	return fmt.Sprintf("%d.%02d", minor/100, minor%100)
}

func parseAmount(value string) (int64, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 || len(parts[1]) != 2 || parts[0] == "" {
		return 0, errors.New("amount must have two decimal places")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole < 0 {
		return 0, errors.New("amount whole part is invalid")
	}
	fraction, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || fraction < 0 {
		return 0, errors.New("amount fraction is invalid")
	}
	return whole*100 + fraction, nil
}
