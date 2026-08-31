package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"download.simplevpn/control-plane/internal/payment"
)

func TestReturnPageNeverClaimsThePaymentSucceeded(t *testing.T) {
	recorder := httptest.NewRecorder()
	(&Server{}).paymentReturn(recorder, httptest.NewRequest(http.MethodGet, "/v1/payments/return", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("return page answered %d", recorder.Code)
	}
	body := strings.ToLower(recorder.Body.String())
	if strings.Contains(body, "оплата прошла") || strings.Contains(body, "успешн") {
		t.Fatal("browser return claims a success it cannot know")
	}
	if !strings.Contains(body, "ничего не подтверждает") {
		t.Fatal("return page does not explain the source of truth")
	}
}

func TestWebhookUsesOnlyTheObjectIDBeforeCanonicalVerification(t *testing.T) {
	body, err := os.ReadFile("payments.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	webhook := between(source, "func (s *Server) paymentWebhook", "\nfunc ")
	if webhook == "" {
		t.Fatal("payment webhook is missing")
	}
	if !strings.Contains(webhook, "s.payments.Handle") {
		t.Fatal("webhook never reads canonical provider state")
	}
	for _, forbidden := range []string{"notification.Object.Status", "notification.Object.Paid", "notification.Object.Amount"} {
		if strings.Contains(webhook, forbidden) {
			t.Fatalf("webhook trusts %s from the incoming body", forbidden)
		}
	}
}

func TestPaymentEndpointsHaveTheRequiredAuthority(t *testing.T) {
	body, err := os.ReadFile("payments.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, handler := range []string{"createPayment", "currentPayment"} {
		part := between(source, "func (s *Server) "+handler, "\nfunc ")
		if !strings.Contains(part, "s.device(w, r)") {
			t.Fatalf("%s is not bound to the authenticated account", handler)
		}
	}
	webhook := between(source, "func (s *Server) paymentWebhook", "\nfunc ")
	if strings.Contains(webhook, "s.device(w, r)") {
		t.Fatal("provider webhook incorrectly requires an Android device token")
	}
}

func TestCurrentPaymentUsesCanonicalServiceRecovery(t *testing.T) {
	body, err := os.ReadFile("payments.go")
	if err != nil {
		t.Fatal(err)
	}
	current := between(string(body), "func (s *Server) currentPayment", "\nfunc ")
	if !strings.Contains(current, "s.payments.Current") {
		t.Fatal("payment check bypasses the provider-neutral payment service")
	}
	for _, forbidden := range []string{"decode(", "URL.Query(", "FormValue(", "checkout_url", "return_url"} {
		if strings.Contains(current, forbidden) {
			t.Fatalf("payment check trusts client or browser field %q", forbidden)
		}
	}
}

func TestWebhookRejectsMoreThanOneJSONValue(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/payments/webhooks/yookassa",
		strings.NewReader(`{"type":"notification"}{"object":{"id":"other"}}`),
	)
	var notification any
	if err := decodePaymentWebhook(recorder, request, &notification); err == nil {
		t.Fatal("webhook accepted trailing JSON")
	}
}

func TestRefundAPIAcceptsNoCommercialOrProviderFields(t *testing.T) {
	body, err := os.ReadFile("payments.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	request := between(source, "type refundRequest struct", "\n}")
	for _, allowed := range []string{"PaymentID", "Retry"} {
		if !strings.Contains(request, allowed) {
			t.Fatalf("refund request lacks %s", allowed)
		}
	}
	for _, forbidden := range []string{"Amount", "Currency", "Provider", "Duration"} {
		if strings.Contains(request, forbidden) {
			t.Fatalf("Android can submit refund field %s", forbidden)
		}
	}
	for _, handler := range []string{"refundQuote", "createRefund", "currentRefund"} {
		part := between(source, "func (s *Server) "+handler, "\nfunc ")
		if !strings.Contains(part, "refundDeviceRequest") {
			t.Fatalf("%s is not bound to the authenticated account", handler)
		}
	}
	helper := between(source, "func (s *Server) refundDeviceRequest", "\nfunc ")
	if !strings.Contains(helper, "s.device(w, r)") {
		t.Fatal("refund helper does not authenticate a device")
	}
}

func TestRefundResponsesExposeNoProviderIdentifiers(t *testing.T) {
	quote := refundQuoteJSON(payment.RefundQuote{
		PaymentID: "payment-1", Available: true, AmountMinor: 60000,
		Currency: "RUB", Mode: payment.RefundModeProRata, CalculatedAt: time.Now(),
	})
	refund := refundJSON(payment.RefundRecord{
		ID: "refund-1", PaymentID: "payment-1", Provider: "secret-provider-shape",
		ProviderPaymentID: "provider-payment-1", AmountMinor: 60000,
		Currency: "RUB", Mode: payment.RefundModeProRata,
		Status: payment.RefundStatusPending, CreatedAt: time.Now(),
		Attempt: payment.RefundAttempt{ProviderRefundID: "provider-refund-1"},
	})
	for name, object := range map[string]map[string]any{"quote": quote, "refund": refund} {
		for _, forbidden := range []string{"provider", "provider_payment_id", "provider_refund_id", "idempotency_key"} {
			if _, exists := object[forbidden]; exists {
				t.Fatalf("%s response exposes %s", name, forbidden)
			}
		}
	}
}

func TestRefundWebhookIsCanonicallyVerified(t *testing.T) {
	body, err := os.ReadFile("payments.go")
	if err != nil {
		t.Fatal(err)
	}
	webhook := between(string(body), "func (s *Server) paymentWebhook", "\nfunc ")
	if !strings.Contains(webhook, `case "refund.succeeded"`) ||
		!strings.Contains(webhook, "s.payments.HandleRefund") {
		t.Fatal("refund webhook does not trigger canonical refund verification")
	}
	if strings.Contains(webhook, "notification.Object.Status") ||
		strings.Contains(webhook, "notification.Object.Amount") {
		t.Fatal("refund webhook trusts provider body fields")
	}
}
