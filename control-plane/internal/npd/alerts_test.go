package npd

import (
	"context"
	"strings"
	"testing"
	"time"
)

type capturingChannel struct {
	subject string
	body    string
	err     error
}

func (c *capturingChannel) Name() string { return "test" }
func (c *capturingChannel) Send(_ context.Context, subject, body string) error {
	c.subject, c.body = subject, body
	return c.err
}

func TestFailedReceiptLetterCarriesEverythingNeededToActOnIt(t *testing.T) {
	// The stage lists these fields. Each is here because without it the letter
	// cannot be acted on the same day, which is its only purpose.
	channel := &capturingChannel{}
	alerter := MailAlerter{Channels: []Channel{channel}}

	err := alerter.ReceiptFailed(context.Background(), Settlement{
		PaymentID:    "16100000-0000-4000-8000-000000000001",
		AccountID:    "acc-1",
		AccountEmail: "someone@example.com",
		PaidMinor:    39900,
		PaidAt:       time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC),
	}, "ФНС ответила 500", 1)
	if err != nil {
		t.Fatal(err)
	}

	for _, needed := range []string{
		"Payment ID:", "16100000-0000-4000-8000-000000000001",
		"Пользователь:", "someone@example.com",
		"Сумма:", "399,00 ₽",
		"Время платежа:", "2026-09-02 12:00 МСК",
		"Ошибка НПД:", "ФНС ответила 500",
		"Статус очереди:",
	} {
		if !strings.Contains(channel.body, needed) {
			t.Errorf("letter is missing %q:\n%s", needed, channel.body)
		}
	}
}

func TestFailedReceiptLetterSaysTheVipIsUntouched(t *testing.T) {
	// Business Owner reading this at speed must not think they have to undo
	// anything. The payment stands and so does the VIP.
	channel := &capturingChannel{}
	_ = MailAlerter{Channels: []Channel{channel}}.ReceiptFailed(
		context.Background(), Settlement{PaymentID: "p", PaidMinor: 39900}, "сбой", 1)

	if !strings.Contains(channel.body, "VIP выдан") {
		t.Fatalf("the letter must say the payment and VIP stand:\n%s", channel.body)
	}
}

func TestALetterAboutARefundedPaymentSaysWhatTheReceiptShouldBe(t *testing.T) {
	channel := &capturingChannel{}
	_ = MailAlerter{Channels: []Channel{channel}}.ReceiptFailed(
		context.Background(), Settlement{
			PaymentID: "p", PaidMinor: 120000, RefundedMinor: 60000,
		}, "сбой", 1)

	if !strings.Contains(channel.body, "600,00 ₽") {
		t.Fatalf("the amount owed is the remainder, not the payment:\n%s", channel.body)
	}
	if !strings.Contains(channel.body, "1200,00 ₽") {
		t.Fatalf("the original payment has to be visible to check the arithmetic:\n%s", channel.body)
	}
}

func TestAnUnresolvedReceiptWarnsBeforeIssuingAnother(t *testing.T) {
	channel := &capturingChannel{}
	_ = MailAlerter{Channels: []Channel{channel}}.ReceiptFailed(
		context.Background(), Settlement{PaymentID: "p", PaidMinor: 39900, Unresolved: true},
		"неизвестно", 1)

	if !strings.Contains(channel.body, "проверить в «Мой налог»") {
		t.Fatalf("a person about to issue a receipt by hand must be warned first:\n%s", channel.body)
	}
}

func TestTheOutageLetterSaysExistingVipKeepsWorking(t *testing.T) {
	channel := &capturingChannel{}
	_ = MailAlerter{Channels: []Channel{channel}}.TaxServiceDown(
		context.Background(), "ФНС недоступна", 0)

	if !strings.Contains(channel.body, "Действующие VIP продолжают работать") {
		t.Fatalf("the letter must say what is not affected:\n%s", channel.body)
	}
}

func TestNoChannelsIsNotAFailure(t *testing.T) {
	// A deployment nobody gave an address to still runs. It just cannot speak.
	if err := (MailAlerter{}).TaxServiceDown(context.Background(), "x", 0); err != nil {
		t.Fatal(err)
	}
}

func TestRublesFormatting(t *testing.T) {
	for _, tc := range []struct {
		minor int64
		want  string
	}{{39900, "399,00 ₽"}, {22201, "222,01 ₽"}, {5, "0,05 ₽"}} {
		if got := rubles(tc.minor); got != tc.want {
			t.Errorf("rubles(%d) = %q, want %q", tc.minor, got, tc.want)
		}
	}
}
