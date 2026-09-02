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
	// Selling also needs the tax service to be answering. This fixture line
	// is the gate working: without it BeginPayment refuses, which is exactly
	// what it should do on a deployment that has never checked.
	if err := st.SetAvailability(ctx, true, "integration test", time.Now()); err != nil {
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

	// The receipt obligation is committed with the VIP, not after it. Only
	// Postgres can prove that: it is a property of one transaction, and a fake
	// repository would agree with whatever the code happened to do.
	queued, err := st.PendingCount(ctx)
	if err != nil || queued != 1 {
		t.Fatalf("a paid payment must owe exactly one receipt: queued=%d err=%v", queued, err)
	}

	// And a duplicate webhook must not owe a second one.
	settlement, err := st.Settlement(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if settlement.PaidMinor != record.Product.AmountMinor {
		t.Fatalf("receipt would be for %d, payment was %d",
			settlement.PaidMinor, record.Product.AmountMinor)
	}
	if settlement.Active != nil {
		t.Fatal("nothing has been issued yet")
	}

	// Two receipts for one payment are refused by the database, not by care.
	if _, err := st.BeginReceipt(ctx, record.ID, settlement.PaidMinor); err != nil {
		t.Fatal(err)
	}
	if _, err := st.BeginReceipt(ctx, record.ID, settlement.PaidMinor); err == nil {
		t.Fatal("the database allowed a second open receipt for one payment")
	}
	appCredential := insertCredential(t, ctx, st, accountID, "app")
	external, err := st.AddExternalDevice(ctx, accountID, "expiry-test")
	if err != nil || external.Credential == nil {
		t.Fatalf("cannot add expiry-test external device: device=%+v err=%v", external, err)
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
	assertCredentialState(t, ctx, st, *external.Credential, "REVOKED")
	assertNodeCredentials(t, ctx, st, []uuid.UUID{appCredential}, []uuid.UUID{*external.Credential})

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
	// Selling also needs the tax service to be answering. This fixture line
	// is the gate working: without it BeginPayment refuses, which is exactly
	// what it should do on a deployment that has never checked.
	if err := st.SetAvailability(ctx, true, "integration test", time.Now()); err != nil {
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
	external, err := st.AddExternalDevice(ctx, accountID, "refund-test")
	if err != nil || external.Credential == nil {
		t.Fatalf("cannot add refund-test external device: device=%+v err=%v", external, err)
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
	assertCredentialState(t, ctx, st, *external.Credential, "ACTIVE")
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
	assertCredentialState(t, ctx, st, *external.Credential, "REVOKED")
	assertNodeCredentials(t, ctx, st, nil, []uuid.UUID{*external.Credential})
}

func TestManualTierDowngradeRevokesExternalAccessOnPostgres(t *testing.T) {
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

	for _, by := range []string{"email", "prefix"} {
		t.Run(by, func(t *testing.T) {
			accountID := uuid.New()
			email := accountID.String() + "@example.test"
			if _, err := st.pool.Exec(ctx, `
				insert into accounts (id, email, tier, vip_expires_at)
				values ($1, $2, 'VIP', null)`, accountID, email); err != nil {
				t.Fatal(err)
			}
			appCredential := insertCredential(t, ctx, st, accountID, "app")
			external, err := st.AddExternalDevice(ctx, accountID, "manual-"+by)
			if err != nil || external.Credential == nil {
				t.Fatalf("cannot add external device: device=%+v err=%v", external, err)
			}
			assertNodeCredentials(t, ctx, st,
				[]uuid.UUID{appCredential, *external.Credential}, nil)

			if by == "email" {
				_, err = st.SetAccountTier(ctx, email, "FREE")
			} else {
				_, err = st.SetAccountTierByPrefix(ctx, accountID.String()[:8], "FREE")
			}
			if err != nil {
				t.Fatal(err)
			}
			assertCredentialState(t, ctx, st, *external.Credential, "REVOKED")
			assertNodeCredentials(t, ctx, st,
				[]uuid.UUID{appCredential}, []uuid.UUID{*external.Credential})
		})
	}
}

func TestNodeListRejectsAStaleFreeExternalCredentialOnPostgres(t *testing.T) {
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

	accountID := uuid.New()
	if _, err := st.pool.Exec(ctx, `
		insert into accounts (id, email, tier)
		values ($1, $2, 'FREE')`, accountID, accountID.String()+"@example.test"); err != nil {
		t.Fatal(err)
	}
	appCredential := insertCredential(t, ctx, st, accountID, "app")
	// Deliberately bypass AddExternalDevice to reproduce the live drift: the
	// row says ACTIVE even though the account's current limit says zero.
	staleExternal := insertCredential(t, ctx, st, accountID, "external")
	assertCredentialState(t, ctx, st, staleExternal, "ACTIVE")
	assertNodeCredentials(t, ctx, st,
		[]uuid.UUID{appCredential}, []uuid.UUID{staleExternal})

	limited, _, err := st.LimitedCredentials(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !containsCredential(limited, appCredential) || containsCredential(limited, staleExternal) {
		t.Fatalf("limited list contains forbidden external access: %v", limited)
	}
}

func insertCredential(
	t *testing.T, ctx context.Context, st *Store, accountID uuid.UUID, kind string,
) uuid.UUID {
	t.Helper()
	deviceID := uuid.New()
	credential := uuid.New()
	if _, err := st.pool.Exec(ctx, `
		insert into devices (id, account_id, kind, label) values ($1, $2, $3, $4)`,
		deviceID, accountID, kind, kind+"-test"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, `
		insert into device_credentials (id, device_id, credential_uuid, updated_seq)
		values ($1, $2, $3, next_seq('credentials'))`,
		uuid.New(), deviceID, credential); err != nil {
		t.Fatal(err)
	}
	return credential
}

func assertCredentialState(
	t *testing.T, ctx context.Context, st *Store, credential uuid.UUID, want string,
) {
	t.Helper()
	var got string
	if err := st.pool.QueryRow(ctx, `
		select state from device_credentials where credential_uuid = $1`, credential).
		Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("credential %s has state %s, want %s", credential, got, want)
	}
}

func assertNodeCredentials(
	t *testing.T, ctx context.Context, st *Store, included, excluded []uuid.UUID,
) {
	t.Helper()
	live, err := st.LiveCredentials(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, credential := range included {
		if !containsCredential(live, credential) {
			t.Fatalf("node list does not contain allowed credential %s", credential)
		}
	}
	for _, credential := range excluded {
		if containsCredential(live, credential) {
			t.Fatalf("node list contains forbidden credential %s", credential)
		}
	}
}

func containsCredential(credentials []string, wanted uuid.UUID) bool {
	for _, credential := range credentials {
		if credential == wanted.String() {
			return true
		}
	}
	return false
}
