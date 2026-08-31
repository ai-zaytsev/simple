package payment

import (
	"context"
	"errors"
	"testing"
	"time"
)

func refundableRecord() Record {
	paid := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := paid.Add(30 * 24 * time.Hour)
	applied := paid
	refundable := true
	return Record{
		ID: "our-1", AccountID: "account-1", Provider: "fake",
		ProviderPaymentID: "provider-1", Status: StatusSucceeded,
		Product: Product{ID: "vip_1_month", AmountMinor: 120000, Currency: "RUB", DurationMonths: 1},
		PaidAt:  &paid, EntitlementAppliedAt: &applied,
		EntitlementStartedAt: &paid, EntitlementEndsAt: &end,
		VIPExpiresAt:  &end,
		PaymentMethod: "bank_card", ProviderRefundable: &refundable,
	}
}

func TestRefundPolicyBoundaries(t *testing.T) {
	record := refundableRecord()
	limits := RefundLimits{Full: true, Partial: true, MinimumMinor: 100}
	paid := *record.PaidAt

	for name, now := range map[string]time.Time{
		"immediately":        paid,
		"exactly seven days": paid.Add(7 * 24 * time.Hour),
	} {
		t.Run(name, func(t *testing.T) {
			quote := QuoteRefund(record, now, limits)
			if !quote.Available || quote.Mode != RefundModeFull || quote.AmountMinor != 120000 {
				t.Fatalf("full-refund boundary was calculated as %+v", quote)
			}
		})
	}

	quote := QuoteRefund(record, paid.Add(10*24*time.Hour), limits)
	if !quote.Available || quote.Mode != RefundModeProRata || quote.AmountMinor != 60000 {
		t.Fatalf("20/30 * 75%% must be 600.00 RUB, got %+v", quote)
	}

	almostEnd := QuoteRefund(record, record.EntitlementEndsAt.Add(-time.Minute), limits)
	if almostEnd.Available || almostEnd.Reason != RefundReasonBelowMinimum {
		t.Fatalf("sub-provider-minimum result was not refused: %+v", almostEnd)
	}

	ended := QuoteRefund(record, *record.EntitlementEndsAt, limits)
	if ended.Available || ended.Reason != RefundReasonPeriodEnded {
		t.Fatalf("ended period was refundable: %+v", ended)
	}
}

func TestPartialRefundIsNotInventedForUnsupportedMethod(t *testing.T) {
	record := refundableRecord()
	quote := QuoteRefund(record, record.PaidAt.Add(8*24*time.Hour), RefundLimits{
		Full: true, Partial: false, MinimumMinor: 100,
	})
	if quote.Available || quote.Reason != RefundReasonPartialUnsupported {
		t.Fatalf("unsupported partial refund was offered: %+v", quote)
	}
}

func TestRefundRequiresCanonicalProviderPermission(t *testing.T) {
	record := refundableRecord()
	limits := RefundLimits{Full: true, Partial: true, MinimumMinor: 100}
	for name, allowed := range map[string]*bool{
		"missing": nil,
		"refused": func() *bool { value := false; return &value }(),
	} {
		t.Run(name, func(t *testing.T) {
			record.ProviderRefundable = allowed
			quote := QuoteRefund(record, *record.PaidAt, limits)
			if quote.Available || quote.Reason != RefundReasonUnsupported {
				t.Fatalf("provider permission %s produced %+v", name, quote)
			}
		})
	}
}

func TestConfirmedRefundAloneRevokesVIPAndDuplicateIsIdempotent(t *testing.T) {
	record := refundableRecord()
	repo := newFakeRepo()
	repo.record = record
	provider := &fakeProvider{
		refundOperation: RefundOperation{ProviderRefundID: "provider-refund-1"},
		refundCanonical: CanonicalRefund{
			ProviderRefundID: "provider-refund-1", ProviderPaymentID: record.ProviderPaymentID,
			RefundID: "refund-1", AmountMinor: record.Product.AmountMinor,
			Currency: "RUB", Status: RefundStatusSucceeded,
		},
	}
	service, err := NewService(repo, provider, "https://simple-syncbridge.download/v1/payments/return")
	if err != nil {
		t.Fatal(err)
	}

	first, err := service.StartRefund(context.Background(), record.AccountID, record.ID, false, *record.PaidAt)
	if err != nil || first.Status != RefundStatusSucceeded || repo.record.VIPExpiresAt != nil {
		t.Fatalf("confirmed refund did not revoke VIP: refund=%+v err=%v", first, err)
	}
	second, applied, err := service.HandleRefund(context.Background(), "fake", "provider-refund-1")
	if err != nil || applied || second.Status != RefundStatusSucceeded {
		t.Fatalf("duplicate refund applied twice: applied=%v refund=%+v err=%v", applied, second, err)
	}
	if provider.refundCreates != 1 {
		t.Fatalf("provider refund was created %d times", provider.refundCreates)
	}
}

func TestLostCreateResponseIsRecoveredWithoutSecondMoneyOperation(t *testing.T) {
	record := refundableRecord()
	repo := newFakeRepo()
	repo.record = record
	provider := &fakeProvider{refundCreateErr: ErrUnavailable}
	service, _ := NewService(repo, provider, "https://simple-syncbridge.download/v1/payments/return")

	_, err := service.StartRefund(context.Background(), record.AccountID, record.ID, false, *record.PaidAt)
	if !errors.Is(err, ErrUnavailable) || provider.refundCreates != 1 || repo.refund == nil {
		t.Fatalf("lost response was not preserved: creates=%d refund=%+v err=%v", provider.refundCreates, repo.refund, err)
	}
	canonical := CanonicalRefund{
		ProviderRefundID: "provider-refund-1", ProviderPaymentID: record.ProviderPaymentID,
		RefundID: repo.refund.ID, AmountMinor: record.Product.AmountMinor,
		Currency: "RUB", Status: RefundStatusSucceeded,
	}
	provider.refundCreateErr = nil
	provider.refundFind = &canonical
	provider.refundCanonical = canonical
	completed, err := service.ReconcileRefunds(context.Background(), 100)
	if err != nil || completed != 1 || repo.refund.Status != RefundStatusSucceeded || provider.refundCreates != 1 {
		t.Fatalf("lost response created money twice: creates=%d completed=%d refund=%+v err=%v", provider.refundCreates, completed, repo.refund, err)
	}
}

func TestUnknownRefundIsNotRepostedAfterProviderIdempotencyWindow(t *testing.T) {
	record := refundableRecord()
	repo := newFakeRepo()
	repo.record = record
	provider := &fakeProvider{refundCreateErr: ErrUnavailable}
	service, _ := NewService(repo, provider, "https://simple-syncbridge.download/v1/payments/return")

	_, _ = service.StartRefund(context.Background(), record.AccountID, record.ID, false, *record.PaidAt)
	visible, visibleErr := service.StartRefund(
		context.Background(), record.AccountID, record.ID, false,
		record.PaidAt.Add(24*time.Hour),
	)
	if visibleErr != nil || visible.Status != RefundStatusCreating {
		t.Fatalf("unknown outcome was presented as a failure: refund=%+v err=%v", visible, visibleErr)
	}
	completed, err := service.ReconcileRefunds(context.Background(), 100)
	if !errors.Is(err, ErrRefundOutcomeUnknown) || provider.refundCreates != 1 {
		t.Fatalf("old unknown refund was reposted: creates=%d completed=%d err=%v", provider.refundCreates, completed, err)
	}
}

func TestPendingAndCanceledRefundNeverRevokesVIP(t *testing.T) {
	for name, status := range map[string]RefundStatus{
		"pending":  RefundStatusPending,
		"canceled": RefundStatusCanceled,
	} {
		t.Run(name, func(t *testing.T) {
			record := refundableRecord()
			expires := *record.EntitlementEndsAt
			record.VIPExpiresAt = &expires
			repo := newFakeRepo()
			repo.record = record
			provider := &fakeProvider{
				refundOperation: RefundOperation{ProviderRefundID: "provider-refund-1"},
				refundCanonical: CanonicalRefund{
					ProviderRefundID: "provider-refund-1", ProviderPaymentID: record.ProviderPaymentID,
					RefundID: "refund-1", AmountMinor: record.Product.AmountMinor,
					Currency: "RUB", Status: status, CancellationReason: "insufficient_funds",
				},
			}
			service, _ := NewService(repo, provider, "https://simple-syncbridge.download/v1/payments/return")
			got, err := service.StartRefund(context.Background(), record.AccountID, record.ID, false, *record.PaidAt)
			if err != nil || got.Status != status || repo.record.VIPExpiresAt == nil {
				t.Fatalf("%s refund changed VIP: refund=%+v err=%v", status, got, err)
			}
		})
	}
}

func TestRefundCanonicalMustMatchPaymentAmountAndMetadata(t *testing.T) {
	expected := RefundRecord{
		ID: "refund-1", ProviderPaymentID: "payment-1", AmountMinor: 60000, Currency: "RUB",
		Attempt: RefundAttempt{ProviderRefundID: "provider-refund-1"},
	}
	good := CanonicalRefund{
		ProviderRefundID: "provider-refund-1", ProviderPaymentID: "payment-1",
		RefundID: "refund-1", AmountMinor: 60000, Currency: "RUB", Status: RefundStatusSucceeded,
	}
	if err := VerifyRefund(expected, good); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]CanonicalRefund{
		"provider id": withRefund(good, func(r *CanonicalRefund) { r.ProviderRefundID = "other" }),
		"payment":     withRefund(good, func(r *CanonicalRefund) { r.ProviderPaymentID = "other" }),
		"metadata":    withRefund(good, func(r *CanonicalRefund) { r.RefundID = "other" }),
		"amount":      withRefund(good, func(r *CanonicalRefund) { r.AmountMinor++ }),
		"currency":    withRefund(good, func(r *CanonicalRefund) { r.Currency = "USD" }),
	} {
		t.Run(name, func(t *testing.T) {
			if VerifyRefund(expected, bad) == nil {
				t.Fatal("mismatched refund was accepted")
			}
		})
	}
}

func withRefund(base CanonicalRefund, change func(*CanonicalRefund)) CanonicalRefund {
	change(&base)
	return base
}
