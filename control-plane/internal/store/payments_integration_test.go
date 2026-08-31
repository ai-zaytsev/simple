package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"download.simplevpn/control-plane/internal/payment"
	"download.simplevpn/control-plane/internal/purchase"
)

// This test is optional in ordinary CI and is run against a disposable real
// PostgreSQL database before publishing payment schema changes. The unit tests
// cover decisions; this catches SQL syntax, nullable scans and transaction
// behavior that a source assertion cannot.
func TestPaymentLifecycleOnPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	st, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := st.SetPurchases(ctx, purchase.Settings{Open: true, FreeDays: 7}, "payment-test"); err != nil {
		t.Fatal(err)
	}

	accountID := uuid.New()
	if _, err := st.pool.Exec(ctx, `
		insert into accounts (id, email, created_at)
		values ($1, $2, now() - interval '8 days')`, accountID, accountID.String()+"@example.test"); err != nil {
		t.Fatal(err)
	}

	record, err := st.BeginPayment(ctx, accountID.String(), "vip_1_month", "test-provider")
	if err != nil {
		t.Fatal(err)
	}
	record, err = st.AttachCheckout(ctx, record.ID, payment.Checkout{
		ProviderPaymentID: "provider-" + uuid.NewString(),
		URL:               "https://pay.example/checkout",
		Status:            payment.StatusPending,
		Test:              true,
	})
	if err != nil {
		t.Fatal(err)
	}
	canonical := payment.Canonical{
		ProviderPaymentID: record.ProviderPaymentID,
		PaymentID:         record.ID,
		AmountMinor:       record.Product.AmountMinor,
		Currency:          record.Product.Currency,
		Status:            payment.StatusSucceeded,
		Paid:              true,
		Test:              true,
	}

	first, applied, err := st.ApplySucceeded(ctx, record.ID, canonical)
	if err != nil || !applied || first.VIPExpiresAt == nil {
		t.Fatalf("first application: applied=%v record=%+v err=%v", applied, first, err)
	}
	second, applied, err := st.ApplySucceeded(ctx, record.ID, canonical)
	if err != nil || applied || second.VIPExpiresAt == nil || !second.VIPExpiresAt.Equal(*first.VIPExpiresAt) {
		t.Fatalf("duplicate application changed expiry: applied=%v record=%+v err=%v", applied, second, err)
	}

	if _, err := st.pool.Exec(ctx,
		`update accounts set vip_expires_at = now() - interval '1 minute' where id = $1`, accountID); err != nil {
		t.Fatal(err)
	}
	expired, err := st.ExpireVIPs(ctx)
	if err != nil || expired != 1 {
		t.Fatalf("expiry pass: expired=%d err=%v", expired, err)
	}
	var tier string
	var expiry *time.Time
	if err := st.pool.QueryRow(ctx,
		`select tier, vip_expires_at from accounts where id = $1`, accountID).Scan(&tier, &expiry); err != nil {
		t.Fatal(err)
	}
	if tier != "FREE" || expiry != nil {
		t.Fatalf("expired paid VIP became tier=%s expiry=%v", tier, expiry)
	}

	adminID := uuid.New()
	if _, err := st.pool.Exec(ctx, `
		insert into accounts (id, email, tier, vip_expires_at)
		values ($1, $2, 'VIP', null)`, adminID, adminID.String()+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if expired, err := st.ExpireVIPs(ctx); err != nil || expired != 0 {
		t.Fatalf("administrative VIP was treated as expired: expired=%d err=%v", expired, err)
	}
}

func TestRefundLifecycleOnPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	st, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := st.SetPurchases(ctx, purchase.Settings{Open: true, FreeDays: 7}, "refund-test"); err != nil {
		t.Fatal(err)
	}

	accountID := uuid.New()
	if _, err := st.pool.Exec(ctx, `
		insert into accounts (id, email, created_at)
		values ($1, $2, now() - interval '8 days')`, accountID, accountID.String()+"@example.test"); err != nil {
		t.Fatal(err)
	}
	record, err := st.BeginPayment(ctx, accountID.String(), "vip_1_month", "test-provider")
	if err != nil {
		t.Fatal(err)
	}
	record, err = st.AttachCheckout(ctx, record.ID, payment.Checkout{
		ProviderPaymentID: "provider-" + uuid.NewString(),
		URL:               "https://pay.example/refund-test", Status: payment.StatusPending, Test: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	paidAt := time.Now().UTC().Add(-time.Minute)
	paid, applied, err := st.ApplySucceeded(ctx, record.ID, payment.Canonical{
		ProviderPaymentID: record.ProviderPaymentID, PaymentID: record.ID,
		AmountMinor: record.Product.AmountMinor, Currency: record.Product.Currency,
		Status: payment.StatusSucceeded, Paid: true, Test: true, PaidAt: &paidAt,
		PaymentMethod: "bank_card", Refundable: true,
	})
	if err != nil || !applied {
		t.Fatalf("payment application: applied=%v record=%+v err=%v", applied, paid, err)
	}
	quote := payment.QuoteRefund(paid, paidAt, payment.RefundLimits{
		Full: true, Partial: true, MinimumMinor: 100,
	})
	refund, err := st.BeginRefund(ctx, accountID.String(), quote, false)
	if err != nil {
		t.Fatal(err)
	}
	refund, err = st.AttachRefund(ctx, refund.ID, refund.Attempt.ID, payment.RefundOperation{
		ProviderRefundID: "provider-refund-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	canceled := payment.CanonicalRefund{
		ProviderRefundID:  refund.Attempt.ProviderRefundID,
		ProviderPaymentID: refund.ProviderPaymentID, RefundID: refund.ID,
		AmountMinor: refund.AmountMinor, Currency: refund.Currency,
		Status: payment.RefundStatusCanceled, CancellationReason: "insufficient_funds",
	}
	refund, err = st.SetRefundStatus(ctx, refund.ID, refund.Attempt.ID, canceled)
	if err != nil || refund.Status != payment.RefundStatusCanceled {
		t.Fatalf("canceled refund: refund=%+v err=%v", refund, err)
	}
	var tier string
	if err := st.pool.QueryRow(ctx, `select tier from accounts where id = $1`, accountID).Scan(&tier); err != nil {
		t.Fatal(err)
	}
	if tier != "VIP" {
		t.Fatal("canceled refund revoked VIP")
	}
	if _, err := st.BeginRefund(ctx, accountID.String(), quote, false); !errors.Is(err, payment.ErrRefundRetryRequired) {
		t.Fatalf("canceled refund retried without explicit authority: %v", err)
	}
	retry, err := st.BeginRefund(ctx, accountID.String(), quote, true)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Attempt.ID == refund.Attempt.ID || retry.Attempt.IdempotencyKey == refund.Attempt.IdempotencyKey {
		t.Fatal("provider retry reused a canonically canceled attempt")
	}
	retry, err = st.AttachRefund(ctx, retry.ID, retry.Attempt.ID, payment.RefundOperation{
		ProviderRefundID: "provider-refund-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	succeeded := payment.CanonicalRefund{
		ProviderRefundID:  retry.Attempt.ProviderRefundID,
		ProviderPaymentID: retry.ProviderPaymentID, RefundID: retry.ID,
		AmountMinor: retry.AmountMinor, Currency: retry.Currency,
		Status: payment.RefundStatusSucceeded,
	}
	first, revoked, err := st.ApplyRefundSucceeded(ctx, retry.ID, retry.Attempt.ID, succeeded)
	if err != nil || !revoked || first.EntitlementRevokedAt == nil {
		t.Fatalf("successful refund: revoked=%v refund=%+v err=%v", revoked, first, err)
	}
	second, revoked, err := st.ApplyRefundSucceeded(ctx, retry.ID, retry.Attempt.ID, succeeded)
	if err != nil || revoked || second.EntitlementRevokedAt == nil {
		t.Fatalf("duplicate refund changed access: revoked=%v refund=%+v err=%v", revoked, second, err)
	}
	if err := st.pool.QueryRow(ctx, `select tier from accounts where id = $1`, accountID).Scan(&tier); err != nil {
		t.Fatal(err)
	}
	if tier != "FREE" {
		t.Fatalf("confirmed refund left tier=%s", tier)
	}
}
