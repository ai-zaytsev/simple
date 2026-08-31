package payment

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWebhookFetchesCanonicalStateAndAppliesOnlyOnce(t *testing.T) {
	repo := newFakeRepo()
	provider := &fakeProvider{canonical: Canonical{
		ProviderPaymentID: "provider-1",
		PaymentID:         "our-1",
		AmountMinor:       39900,
		Currency:          "RUB",
		Status:            StatusSucceeded,
		Paid:              true,
		Test:              true,
	}}
	service, _ := NewService(repo, provider, "https://simple-syncbridge.download/v1/payments/return")

	first, applied, err := service.Handle(context.Background(), "provider-1")
	if err != nil || !applied || first.EntitlementAppliedAt == nil {
		t.Fatalf("first webhook did not apply: applied=%v record=%+v err=%v", applied, first, err)
	}
	expiry := *first.VIPExpiresAt
	second, applied, err := service.Handle(context.Background(), "provider-1")
	if err != nil || applied {
		t.Fatalf("duplicate webhook applied: applied=%v err=%v", applied, err)
	}
	if !second.VIPExpiresAt.Equal(expiry) {
		t.Fatal("duplicate webhook added the duration twice")
	}
	if provider.gets != 2 {
		t.Fatal("each webhook must be checked with the provider")
	}
}

func TestReturnOrPendingStateCannotActivateVIP(t *testing.T) {
	repo := newFakeRepo()
	provider := &fakeProvider{canonical: Canonical{
		ProviderPaymentID: "provider-1", PaymentID: "our-1",
		AmountMinor: 39900, Currency: "RUB", Status: StatusPending,
	}}
	service, _ := NewService(repo, provider, "https://simple-syncbridge.download/v1/payments/return")

	current, err := service.Current(context.Background(), "account-1")
	if err != nil || current.EntitlementAppliedAt != nil || provider.gets != 0 {
		t.Fatal("reading after return changed the payment")
	}

	updated, applied, err := service.Handle(context.Background(), "provider-1")
	if err != nil || applied || updated.Status != StatusPending || updated.VIPExpiresAt != nil {
		t.Fatalf("pending payment activated VIP: %+v applied=%v err=%v", updated, applied, err)
	}
}

func TestCanceledPaymentClosesWithoutVIP(t *testing.T) {
	repo := newFakeRepo()
	provider := &fakeProvider{canonical: Canonical{
		ProviderPaymentID: "provider-1", PaymentID: "our-1",
		AmountMinor: 39900, Currency: "RUB", Status: StatusCanceled,
	}}
	service, _ := NewService(repo, provider, "https://simple-syncbridge.download/v1/payments/return")

	updated, applied, err := service.Handle(context.Background(), "provider-1")
	if err != nil || applied || updated.Status != StatusCanceled || updated.VIPExpiresAt != nil {
		t.Fatalf("canceled payment changed VIP: %+v applied=%v err=%v", updated, applied, err)
	}
}

func TestMismatchNeverReachesEntitlement(t *testing.T) {
	for name, mutate := range map[string]func(*Canonical){
		"amount":   func(p *Canonical) { p.AmountMinor++ },
		"currency": func(p *Canonical) { p.Currency = "USD" },
		"metadata": func(p *Canonical) { p.PaymentID = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			repo := newFakeRepo()
			canonical := Canonical{
				ProviderPaymentID: "provider-1", PaymentID: "our-1",
				AmountMinor: 39900, Currency: "RUB", Status: StatusSucceeded, Paid: true,
			}
			mutate(&canonical)
			service, _ := NewService(repo, &fakeProvider{canonical: canonical}, "https://simple-syncbridge.download/v1/payments/return")
			if _, _, err := service.Handle(context.Background(), "provider-1"); err == nil {
				t.Fatal("mismatched provider object was accepted")
			}
			if repo.applies != 0 {
				t.Fatal("repository was asked to apply a mismatched payment")
			}
		})
	}
}

func TestStartRetriesOneProviderOperation(t *testing.T) {
	repo := newFakeRepo()
	repo.record.ProviderPaymentID = ""
	repo.record.CheckoutURL = ""
	provider := &fakeProvider{checkout: Checkout{
		ProviderPaymentID: "provider-1", URL: "https://pay.example/one", Status: StatusPending,
	}}
	service, _ := NewService(repo, provider, "https://simple-syncbridge.download/v1/payments/return")

	first, err := service.Start(context.Background(), "account-1", "vip_1_month")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Start(context.Background(), "account-1", "vip_1_month")
	if err != nil {
		t.Fatal(err)
	}
	if provider.creates != 1 || first.ProviderPaymentID != second.ProviderPaymentID {
		t.Fatalf("retry created another provider payment: creates=%d", provider.creates)
	}
}

type fakeProvider struct {
	checkout        Checkout
	canonical       Canonical
	refundOperation RefundOperation
	refundCanonical CanonicalRefund
	refundLimits    RefundLimits
	creates         int
	gets            int
	refundCreates   int
	refundGets      int
	refundFinds     int
	refundFind      *CanonicalRefund
	refundCreateErr error
}

func (p *fakeProvider) Name() string { return "fake" }

func (p *fakeProvider) RefundIdempotencyWindow() time.Duration { return 24 * time.Hour }
func (p *fakeProvider) Create(context.Context, CreateRequest) (Checkout, error) {
	p.creates++
	return p.checkout, nil
}
func (p *fakeProvider) Get(context.Context, string) (Canonical, error) {
	p.gets++
	return p.canonical, nil
}

func (p *fakeProvider) RefundLimits(string) RefundLimits {
	if p.refundLimits.MinimumMinor == 0 {
		return RefundLimits{Full: true, Partial: true, MinimumMinor: 100}
	}
	return p.refundLimits
}

func (p *fakeProvider) CreateRefund(context.Context, RefundCreateRequest) (RefundOperation, error) {
	p.refundCreates++
	return p.refundOperation, p.refundCreateErr
}

func (p *fakeProvider) GetRefund(context.Context, string) (CanonicalRefund, error) {
	p.refundGets++
	return p.refundCanonical, nil
}

func (p *fakeProvider) FindRefund(context.Context, string, string) (CanonicalRefund, error) {
	p.refundFinds++
	if p.refundFind == nil {
		return CanonicalRefund{}, ErrRefundNotFound
	}
	return *p.refundFind, nil
}

type fakeRepo struct {
	record  Record
	applies int
	refund  *RefundRecord
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{record: Record{
		ID: "our-1", AccountID: "account-1", Provider: "fake", ProviderPaymentID: "provider-1",
		IdempotencyKey: "idem-1", Status: StatusPending, CheckoutURL: "https://pay.example/one",
		Product: Product{ID: "vip_1_month", Title: "VIP", AmountMinor: 39900, Currency: "RUB", DurationMonths: 1},
	}}
}

func (r *fakeRepo) Products(context.Context) ([]Product, error) {
	return []Product{r.record.Product}, nil
}
func (r *fakeRepo) BeginPayment(_ context.Context, accountID, productID, provider string) (Record, error) {
	if accountID != r.record.AccountID || productID != r.record.Product.ID || provider != r.record.Provider {
		return Record{}, errors.New("unexpected begin")
	}
	return r.record, nil
}
func (r *fakeRepo) AttachCheckout(_ context.Context, _ string, checkout Checkout) (Record, error) {
	r.record.ProviderPaymentID = checkout.ProviderPaymentID
	r.record.CheckoutURL = checkout.URL
	r.record.Status = checkout.Status
	r.record.ProviderTest = &checkout.Test
	return r.record, nil
}
func (r *fakeRepo) CurrentPayment(context.Context, string) (Record, error) { return r.record, nil }
func (r *fakeRepo) PaymentByProviderID(context.Context, string, string) (Record, error) {
	return r.record, nil
}
func (r *fakeRepo) SetPaymentStatus(_ context.Context, _ string, status Status, test bool) (Record, error) {
	r.record.Status = status
	r.record.ProviderTest = &test
	return r.record, nil
}
func (r *fakeRepo) ApplySucceeded(_ context.Context, _ string, canonical Canonical) (Record, bool, error) {
	if r.record.EntitlementAppliedAt != nil {
		return r.record, false, nil
	}
	r.applies++
	now := time.Now().UTC()
	expires := now.AddDate(0, r.record.Product.DurationMonths, 0)
	r.record.Status = StatusSucceeded
	r.record.PaidAt = &now
	r.record.EntitlementAppliedAt = &now
	r.record.EntitlementStartedAt = &now
	r.record.EntitlementEndsAt = &expires
	r.record.VIPExpiresAt = &expires
	r.record.ProviderTest = &canonical.Test
	return r.record, true, nil
}

func (r *fakeRepo) PaymentForAccount(_ context.Context, accountID, paymentID string) (Record, error) {
	if accountID != r.record.AccountID || paymentID != r.record.ID {
		return Record{}, ErrPaymentNotFound
	}
	return r.record, nil
}

func (r *fakeRepo) RefundByPayment(_ context.Context, accountID, paymentID string) (RefundRecord, error) {
	if r.refund == nil || accountID != r.record.AccountID || paymentID != r.record.ID {
		return RefundRecord{}, ErrRefundNotFound
	}
	return *r.refund, nil
}

func (r *fakeRepo) BeginRefund(_ context.Context, accountID string, quote RefundQuote, retry bool) (RefundRecord, error) {
	if accountID != r.record.AccountID {
		return RefundRecord{}, ErrPaymentNotFound
	}
	if r.refund != nil {
		if (r.refund.Status == RefundStatusCanceled || r.refund.Status == RefundStatusFailed) && !retry {
			return RefundRecord{}, ErrRefundRetryRequired
		}
		return *r.refund, nil
	}
	r.refund = &RefundRecord{
		ID: "refund-1", PaymentID: r.record.ID, AccountID: r.record.AccountID,
		Provider: r.record.Provider, ProviderPaymentID: r.record.ProviderPaymentID,
		AmountMinor: quote.AmountMinor, Currency: quote.Currency, Mode: quote.Mode,
		Status:    RefundStatusCreating,
		CreatedAt: quote.CalculatedAt,
		Attempt: RefundAttempt{
			ID: "attempt-1", IdempotencyKey: "refund-idem-1",
			Status: RefundStatusCreating, CreatedAt: quote.CalculatedAt,
		},
	}
	return *r.refund, nil
}

func (r *fakeRepo) AttachRefund(_ context.Context, _, _ string, operation RefundOperation) (RefundRecord, error) {
	r.refund.Attempt.ProviderRefundID = operation.ProviderRefundID
	r.refund.Attempt.Status = RefundStatusPending
	r.refund.Status = RefundStatusPending
	return *r.refund, nil
}

func (r *fakeRepo) FailRefundAttempt(context.Context, string, string) (RefundRecord, error) {
	r.refund.Status = RefundStatusFailed
	r.refund.Attempt.Status = RefundStatusFailed
	return *r.refund, nil
}

func (r *fakeRepo) RefundByProviderID(_ context.Context, provider, providerRefundID string) (RefundRecord, error) {
	if r.refund == nil || provider != r.refund.Provider || providerRefundID != r.refund.Attempt.ProviderRefundID {
		return RefundRecord{}, ErrRefundNotFound
	}
	return *r.refund, nil
}

func (r *fakeRepo) SetRefundStatus(_ context.Context, _, _ string, canonical CanonicalRefund) (RefundRecord, error) {
	r.refund.Status = canonical.Status
	r.refund.CancellationReason = canonical.CancellationReason
	r.refund.Attempt.Status = canonical.Status
	r.refund.Attempt.CancellationReason = canonical.CancellationReason
	return *r.refund, nil
}

func (r *fakeRepo) ApplyRefundSucceeded(_ context.Context, _, _ string, canonical CanonicalRefund) (RefundRecord, bool, error) {
	if r.refund.EntitlementRevokedAt != nil {
		return *r.refund, false, nil
	}
	if err := VerifyRefund(*r.refund, canonical); err != nil {
		return RefundRecord{}, false, err
	}
	now := time.Now().UTC()
	r.refund.Status = RefundStatusSucceeded
	r.refund.Attempt.Status = RefundStatusSucceeded
	r.refund.SucceededAt = &now
	r.refund.EntitlementRevokedAt = &now
	r.record.VIPExpiresAt = nil
	return *r.refund, true, nil
}

func (r *fakeRepo) UnresolvedRefunds(context.Context, int) ([]RefundRecord, error) {
	if r.refund != nil && (r.refund.Status == RefundStatusCreating || r.refund.Status == RefundStatusPending) {
		return []RefundRecord{*r.refund}, nil
	}
	return nil, nil
}
