package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"download.simplevpn/control-plane/internal/payment"
	"download.simplevpn/control-plane/internal/purchase"
)

const maxWebhookBytes = 64 << 10

type createPaymentRequest struct {
	ProductID string `json:"product_id"`
}

// paymentReturn is deliberately static. YooKassa sends the browser here after
// success and failure alike; the page therefore makes no claim about either
// and performs no state change.
func (s *Server) paymentReturn(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("content-type", "text/html; charset=utf-8")
	w.Header().Set("cache-control", "no-store")
	w.Header().Set("content-security-policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	w.Header().Set("referrer-policy", "no-referrer")
	w.Header().Set("x-content-type-options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!doctype html><html lang="ru"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Simple VPN — оплата</title><style>body{margin:0;min-height:100vh;display:grid;place-items:center;background:#f4f7fb;color:#172033;font:18px system-ui,sans-serif}.card{max-width:34rem;margin:1.5rem;padding:2rem;border-radius:1.5rem;background:white;box-shadow:0 1rem 3rem #17203318}h1{font-size:1.6rem}p{line-height:1.55;color:#4a5568}</style><main class="card"><h1>Вернитесь в Simple VPN</h1><p>Сам возврат со страницы оплаты ничего не подтверждает. Приложение включит VIP только после подтверждения платёжной системой.</p></main></html>`))
}

// createPayment accepts a catalog identifier and nothing commercial from the
// phone. Account, amount, currency, duration and provider all come from Core.
func (s *Server) createPayment(w http.ResponseWriter, r *http.Request) {
	device, ok := s.device(w, r)
	if !ok {
		return
	}
	if s.payments == nil {
		writeError(w, http.StatusServiceUnavailable, "payments are not configured")
		return
	}

	var body createPaymentRequest
	if err := decode(w, r, &body); err != nil || strings.TrimSpace(body.ProductID) == "" {
		writeError(w, http.StatusBadRequest, "payment product is required")
		return
	}

	// The same decision the button received, read again at the point money is
	// about to be accepted. The repository repeats it under the account lock;
	// this first read provides a useful refusal without calling a provider.
	tier, created, err := s.store.TierOfAccount(r.Context(), device.AccountID)
	if err != nil {
		s.log.Error("cannot read account before payment", "error", err)
		writeError(w, http.StatusServiceUnavailable, "purchase is unavailable")
		return
	}
	state, err := s.store.LoadServiceState(r.Context())
	if err != nil || !purchase.Assess(time.Now().UTC(), created, tier, state.Purchases).Available {
		writeError(w, http.StatusConflict, "purchase is unavailable")
		return
	}

	record, err := s.payments.Start(r.Context(), device.AccountID.String(), strings.TrimSpace(body.ProductID))
	switch {
	case errors.Is(err, payment.ErrProductNotFound):
		writeError(w, http.StatusBadRequest, "payment product is unavailable")
		return
	case errors.Is(err, payment.ErrAlreadyVIP), errors.Is(err, payment.ErrPurchaseUnavailable):
		writeError(w, http.StatusConflict, "purchase is unavailable")
		return
	case errors.Is(err, payment.ErrPaymentInProgress):
		writeError(w, http.StatusConflict, "another payment is in progress")
		return
	case errors.Is(err, payment.ErrRejected):
		s.log.Error("payment provider rejected checkout creation")
		writeError(w, http.StatusBadGateway, "payment could not be created")
		return
	case err != nil:
		s.log.Error("cannot create payment", "error", err)
		writeError(w, http.StatusServiceUnavailable, "payment service is unavailable")
		return
	}

	writeJSON(w, http.StatusOK, paymentJSON(record, true))
}

// currentPayment reports only our durable state. It deliberately does not ask
// the provider: returning from checkout and calling this endpoint cannot turn
// a pending payment into a successful one.
func (s *Server) currentPayment(w http.ResponseWriter, r *http.Request) {
	device, ok := s.device(w, r)
	if !ok {
		return
	}
	if s.payments == nil {
		writeError(w, http.StatusServiceUnavailable, "payments are not configured")
		return
	}
	record, err := s.payments.Current(r.Context(), device.AccountID.String())
	if errors.Is(err, payment.ErrPaymentNotFound) {
		// "No payment" is a valid account state, not a failed entry point.
		// Android retries non-200 responses over every bootstrap address, so a
		// 404 here would turn one ordinary screen refresh into several needless
		// requests and misleading reachability log lines.
		writeJSON(w, http.StatusOK, map[string]string{"status": "none"})
		return
	}
	if err != nil {
		s.log.Error("cannot read current payment", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot read payment")
		return
	}
	writeJSON(w, http.StatusOK, paymentJSON(record, record.Status == payment.StatusPending))
}

// paymentWebhook trusts one field: which provider object should be checked.
// Every field that can grant access is then read through authenticated server
// API by payment.Service.
func (s *Server) paymentWebhook(w http.ResponseWriter, r *http.Request) {
	if s.payments == nil {
		writeError(w, http.StatusServiceUnavailable, "payments are not configured")
		return
	}
	var notification struct {
		Type   string `json:"type"`
		Event  string `json:"event"`
		Object struct {
			ID string `json:"id"`
		} `json:"object"`
	}
	if err := decodePaymentWebhook(w, r, &notification); err != nil {
		writeError(w, http.StatusBadRequest, "notification could not be read")
		return
	}
	if notification.Type != "notification" || strings.TrimSpace(notification.Object.ID) == "" {
		writeError(w, http.StatusBadRequest, "notification is incomplete")
		return
	}
	switch notification.Event {
	case "payment.succeeded", "payment.canceled", "payment.waiting_for_capture":
	default:
		writeJSON(w, http.StatusOK, map[string]bool{"received": true})
		return
	}

	_, applied, err := s.payments.Handle(r.Context(), strings.TrimSpace(notification.Object.ID))
	if err != nil {
		// Non-200 asks YooKassa to retry. This covers the narrow create race and
		// a temporary provider API outage without ever accepting webhook data as
		// truth. No provider response body or identifier is logged.
		s.log.Error("cannot verify payment notification")
		writeError(w, http.StatusServiceUnavailable, "notification cannot be verified")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"received": true, "applied": applied})
}

func decodePaymentWebhook(w http.ResponseWriter, r *http.Request, into any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxWebhookBytes))
	if err := decoder.Decode(into); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("notification contains more than one JSON value")
	}
	return nil
}

func paymentJSON(record payment.Record, includeCheckout bool) map[string]any {
	out := map[string]any{
		"payment_id": record.ID,
		"product_id": record.Product.ID,
		"status":     record.Status,
		"created_at": record.CreatedAt.UTC().Format(time.RFC3339),
	}
	if includeCheckout && record.CheckoutURL != "" {
		out["checkout_url"] = record.CheckoutURL
	}
	if record.VIPExpiresAt != nil {
		out["vip_expires_at"] = record.VIPExpiresAt.UTC().Format(time.RFC3339)
	}
	return out
}
