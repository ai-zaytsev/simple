package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
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
