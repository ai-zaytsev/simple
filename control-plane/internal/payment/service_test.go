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
	checkout  Checkout
	canonical Canonical
	creates   int
	gets      int
}

func (p *fakeProvider) Name() string { return "fake" }
func (p *fakeProvider) Create(context.Context, CreateRequest) (Checkout, error) {
	p.creates++
	return p.checkout, nil
}
func (p *fakeProvider) Get(context.Context, string) (Canonical, error) {
	p.gets++
	return p.canonical, nil
}

type fakeRepo struct {
	record  Record
	applies int
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
	r.record.VIPExpiresAt = &expires
	r.record.ProviderTest = &canonical.Test
	return r.record, true, nil
}
