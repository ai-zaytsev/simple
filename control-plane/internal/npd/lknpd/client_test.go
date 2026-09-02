package lknpd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func clientAt(t *testing.T, handler http.HandlerFunc) (*Client, func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	c := newAt(server.Client(), time.FixedZone("MSK", 3*60*60), server.URL)

	return c, func() {

		server.Close()
	}
}

func TestMaintenanceIsToldApartFromOurMistake(t *testing.T) {
	// This is the whole reason the adapter exists as its own layer. The tax
	// service being unwell stops tomorrow's sales and is nobody's bug; a
	// rejected request is ours and waiting will not cure it.
	cases := []struct {
		name        string
		status      int
		body        string
		unavailable bool
	}{
		{"503", http.StatusServiceUnavailable, `{"message":"nope"}`, true},
		{"429", http.StatusTooManyRequests, `{"message":"slow down"}`, true},
		{"502", http.StatusBadGateway, `{"message":"bad gateway"}`, true},
		{"технические работы", http.StatusInternalServerError,
			`{"message":"Проводятся технические работы"}`, true},
		{"временно недоступен", http.StatusInternalServerError,
			`{"message":"Сервис временно недоступен"}`, true},
		{"maintenance", http.StatusInternalServerError, `{"message":"Under maintenance"}`, true},
		{"наша ошибка", http.StatusBadRequest, `{"message":"Неверная сумма"}`, false},
		{"не найдено", http.StatusNotFound, `{"message":"Нет такого чека"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, done := clientAt(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			defer done()

			err := c.Alive(context.Background(), Session{AccessToken: "t"})
			if errors.Is(err, ErrServiceUnavailable) != tc.unavailable {
				t.Fatalf("unavailable=%v for %v", errors.Is(err, ErrServiceUnavailable), err)
			}
		})
	}
}

func TestAChangedApiCountsAsAnOutage(t *testing.T) {
	// A CAPTCHA page and a redesigned API both arrive as 200 with a body that
	// is not the JSON we expect. Neither is fixable in the moment and both
	// must stop selling rather than be retried into a loop.
	c, done := clientAt(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<html><body>Введите код с картинки</body></html>")
	})
	defer done()

	err := c.Alive(context.Background(), Session{AccessToken: "t"})
	if !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("expected an outage, got %v", err)
	}
	if strings.Contains(err.Error(), "картинки") {
		t.Fatal("the page body must not travel into the error and from there into a log")
	}
}

func TestExpiredTokenIsItsOwnAnswer(t *testing.T) {
	c, done := clientAt(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"message":"token expired"}`)
	})
	defer done()

	err := c.Alive(context.Background(), Session{AccessToken: "stale"})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected an authorisation problem, got %v", err)
	}
	if errors.Is(err, ErrServiceUnavailable) {
		t.Fatal("a dead token is not an outage: refreshing helps, waiting does not")
	}
}

func TestCreateReceiptSendsWhatTheApiWants(t *testing.T) {
	var got map[string]any
	c, done := clientAt(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/income" {
			t.Errorf("income goes to /income, went to %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Error("income must be authorised")
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, `{"approvedReceiptUuid":"20abcdef"}`)
	})
	defer done()

	at := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	uuid, printURL, err := c.CreateReceipt(
		context.Background(),
		Session{AccessToken: "tok", INN: "123456789012"},
		"Simple VPN, VIP на месяц", 39900, at,
	)
	if err != nil {
		t.Fatal(err)
	}
	if uuid != "20abcdef" {
		t.Fatalf("receipt id not read back: %q", uuid)
	}
	if !strings.HasSuffix(printURL, "/receipt/123456789012/20abcdef/print") {
		t.Fatalf("printable address is built, not returned: %q", printURL)
	}

	// Amounts travel as strings with two decimals. 39900 minor units is
	// "399.00" and never 399 or 39900.
	if got["totalAmount"] != "399.00" {
		t.Fatalf("totalAmount was %v", got["totalAmount"])
	}
	if got["paymentType"] != "CASH" {
		t.Fatalf("a card payment by a person is CASH, got %v", got["paymentType"])
	}
	// The offset matters: the tax service dates the receipt by what we send.
	if ts, _ := got["operationTime"].(string); !strings.HasSuffix(ts, "+03:00") {
		t.Fatalf("operationTime must carry the taxpayer's offset, got %v", got["operationTime"])
	}
}

func TestCancelSendsTheRussianReasonNotACode(t *testing.T) {
	// The API takes the words themselves. "REFUND" would be accepted as a
	// comment and mean nothing to the tax service.
	var got map[string]any
	c, done := clientAt(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cancel" {
			t.Errorf("cancellation goes to /cancel, went to %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, `{"approvedReceiptUuid":"20abcdef"}`)
	})
	defer done()

	err := c.CancelReceipt(context.Background(),
		Session{AccessToken: "tok"}, "20abcdef", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got["comment"] != "Возврат средств" {
		t.Fatalf("cancellation reason was %v", got["comment"])
	}
	if got["receiptUuid"] != "20abcdef" {
		t.Fatalf("receipt id was %v", got["receiptUuid"])
	}
}

func TestRefreshKeepsTheOldRefreshTokenWhenNoneIsIssued(t *testing.T) {
	// The service may renew only the access token. Dropping the refresh token
	// we already hold would turn a working session into one that has to log in
	// with a password again - which against an unofficial API is how an
	// account earns a CAPTCHA.
	c, done := clientAt(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"token":"new-access"}`)
	})
	defer done()

	renewed, err := c.Refresh(context.Background(), Session{
		RefreshToken: "keep-me", INN: "123456789012", DeviceID: "d",
	})
	if err != nil {
		t.Fatal(err)
	}
	if renewed.RefreshToken != "keep-me" {
		t.Fatalf("refresh token was lost: %q", renewed.RefreshToken)
	}
	if renewed.AccessToken != "new-access" {
		t.Fatalf("access token not taken: %q", renewed.AccessToken)
	}
}

func TestMissingReceiptIsAnAnswerNotAFailure(t *testing.T) {
	c, done := clientAt(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"not found"}`)
	})
	defer done()

	exists, err := c.ReceiptExists(context.Background(),
		Session{AccessToken: "tok", INN: "123456789012"}, "20abcdef")
	if err != nil {
		t.Fatalf("a missing receipt is not an error: %v", err)
	}
	if exists {
		t.Fatal("expected the receipt to be reported absent")
	}
}

func TestAmountFormatting(t *testing.T) {
	for _, tc := range []struct {
		minor int64
		want  string
	}{
		{39900, "399.00"},
		{22201, "222.01"},
		{100, "1.00"},
		{5, "0.05"},
		{120000, "1200.00"},
	} {
		if got := formatAmount(tc.minor); got != tc.want {
			t.Errorf("formatAmount(%d) = %q, want %q", tc.minor, got, tc.want)
		}
	}
}
