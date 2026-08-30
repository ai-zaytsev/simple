package yookassa

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"download.simplevpn/control-plane/internal/payment"
)

func TestCreateUsesOnlyTheServerRedirectAPI(t *testing.T) {
	const shop = "12345"
	const secret = "secret-never-returned"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/payments" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(shop+":"+secret))
		if r.Header.Get("Authorization") != wantAuth {
			t.Fatal("server API did not use shop credentials as Basic auth")
		}
		if r.Header.Get("Idempotence-Key") != "idem-1" {
			t.Fatal("idempotence key was not sent")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["capture"] != true {
			t.Error("payment is not one-stage")
		}
		confirmation := body["confirmation"].(map[string]any)
		if confirmation["type"] != "redirect" {
			t.Error("external redirect confirmation was not requested")
		}
		metadata := body["metadata"].(map[string]any)
		if metadata["payment_id"] != "our-1" {
			t.Error("provider cannot join the webhook to our payment")
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"provider-1","status":"pending","paid":false,"test":true,
			"amount":{"value":"399.00","currency":"RUB"},
			"confirmation":{"type":"redirect","confirmation_url":"https://yoomoney.test/pay/1"}
		}`))
	}))
	defer server.Close()

	client, err := newAt(shop, secret, server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	checkout, err := client.Create(context.Background(), payment.CreateRequest{
		PaymentID:      "our-1",
		IdempotencyKey: "idem-1",
		AmountMinor:    39900,
		Currency:       "RUB",
		Description:    "VIP",
		ReturnURL:      "https://simple-syncbridge.download/v1/payments/return",
	})
	if err != nil {
		t.Fatal(err)
	}
	if checkout.ProviderPaymentID != "provider-1" || checkout.Status != payment.StatusPending || !checkout.Test {
		t.Fatalf("unexpected checkout: %+v", checkout)
	}
}

func TestGetReturnsCanonicalProviderState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"id":"provider-1","status":"succeeded","paid":true,"test":true,
			"amount":{"value":"1090.00","currency":"RUB"},
			"metadata":{"payment_id":"our-1"},
			"captured_at":"2026-08-30T10:00:00.000Z"
		}`))
	}))
	defer server.Close()
	client, _ := newAt("12345", "secret", server.URL, server.Client())

	got, err := client.Get(context.Background(), "provider-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.PaymentID != "our-1" || got.AmountMinor != 109000 || !got.Paid || got.PaidAt == nil {
		t.Fatalf("canonical state lost data: %+v", got)
	}
}

func TestProviderErrorsNeverCarrySecretsOrBodies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"description":"secret-never-returned private-metadata"}`))
	}))
	defer server.Close()
	client, _ := newAt("12345", "secret-never-returned", server.URL, server.Client())

	_, err := client.Get(context.Background(), "provider-1")
	if !errors.Is(err, payment.ErrRejected) {
		t.Fatalf("wrong error class: %v", err)
	}
	for _, forbidden := range []string{"secret-never-returned", "private-metadata"} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("provider error leaked %q", forbidden)
		}
	}
}

func TestMoneyIsNeverParsedThroughFloat(t *testing.T) {
	for value, want := range map[string]int64{"399.00": 39900, "1090.00": 109000, "3490.00": 349000} {
		got, err := parseAmount(value)
		if err != nil || got != want {
			t.Fatalf("%s became %d, %v", value, got, err)
		}
	}
	for _, bad := range []string{"399", "399.0", "399.000", "-1.00", "NaN"} {
		if _, err := parseAmount(bad); err == nil {
			t.Fatalf("accepted malformed amount %q", bad)
		}
	}
}
