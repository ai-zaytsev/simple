// Package payment contains the provider-neutral payment contract.
//
// Android and entitlement code speak only in these terms. A provider adapter
// knows how to turn them into one acquirer's HTTP payload, and nothing outside
// that adapter needs to know the acquirer's request shape.
package payment

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"
)

type Status string

const (
	StatusCreating  Status = "creating"
	StatusPending   Status = "pending"
	StatusSucceeded Status = "succeeded"
	StatusCanceled  Status = "canceled"
	StatusFailed    Status = "failed"
)

// Product is a server-owned commercial promise. AmountMinor is kopecks for
// RUB; using an integer means no binary rounding can change what is charged.
type Product struct {
	ID             string
	Title          string
	AmountMinor    int64
	Currency       string
	DurationMonths int
}

// Record is our durable view of a payment. Commercial fields are snapshots:
// editing the catalog tomorrow cannot alter what this row promised today.
type Record struct {
	ID                   string
	AccountID            string
	Product              Product
	Provider             string
	ProviderPaymentID    string
	IdempotencyKey       string
	Status               Status
	CheckoutURL          string
	ProviderTest         *bool
	CreatedAt            time.Time
	PaidAt               *time.Time
	EntitlementAppliedAt *time.Time
	EntitlementStartedAt *time.Time
	EntitlementEndsAt    *time.Time
	VIPExpiresAt         *time.Time
	PaymentMethod        string
	ProviderRefundable   *bool
}

// CreateRequest is everything a provider may be told when a checkout is made.
// Account identifiers and email addresses are intentionally absent. PaymentID
// is random and is enough to join the provider object back to our own row.
type CreateRequest struct {
	PaymentID      string
	IdempotencyKey string
	AmountMinor    int64
	Currency       string
	Description    string
	ReturnURL      string
}

type Checkout struct {
	ProviderPaymentID string
	URL               string
	Status            Status
	Test              bool
}

// Canonical is a payment read back from the provider over its authenticated
// server API. Webhook fields never become this type directly.
type Canonical struct {
	ProviderPaymentID string
	PaymentID         string
	AmountMinor       int64
	Currency          string
	Status            Status
	Paid              bool
	Test              bool
	PaidAt            *time.Time
	PaymentMethod     string
	Refundable        bool
}

type Provider interface {
	Name() string
	Create(context.Context, CreateRequest) (Checkout, error)
	Get(context.Context, string) (Canonical, error)
	RefundIdempotencyWindow() time.Duration
	RefundLimits(string) RefundLimits
	CreateRefund(context.Context, RefundCreateRequest) (RefundOperation, error)
	GetRefund(context.Context, string) (CanonicalRefund, error)
	FindRefund(context.Context, string, string) (CanonicalRefund, error)
}

var (
	ErrUnavailable          = errors.New("payment provider is unavailable")
	ErrRejected             = errors.New("payment provider rejected the request")
	ErrProductNotFound      = errors.New("payment product does not exist")
	ErrPaymentInProgress    = errors.New("another payment is already in progress")
	ErrAlreadyVIP           = errors.New("account is already VIP")
	ErrPurchaseUnavailable  = errors.New("VIP purchase is unavailable")
	ErrPaymentNotFound      = errors.New("payment does not exist")
	ErrProviderNotFound     = errors.New("payment provider is not configured")
	ErrRefundNotFound       = errors.New("refund does not exist")
	ErrRefundUnavailable    = errors.New("refund is unavailable")
	ErrRefundRetryRequired  = errors.New("a canceled refund requires an explicit retry")
	ErrRefundOutcomeUnknown = errors.New("refund outcome is still unknown")
)

// Repository is the provider-independent durable boundary. The PostgreSQL
// store implements it; tests can prove orchestration without a database or a
// real acquirer.
type Repository interface {
	Products(context.Context) ([]Product, error)
	BeginPayment(context.Context, string, string, string) (Record, error)
	AttachCheckout(context.Context, string, Checkout) (Record, error)
	CurrentPayment(context.Context, string) (Record, error)
	PaymentByProviderID(context.Context, string, string) (Record, error)
	SetPaymentStatus(context.Context, string, Status, bool) (Record, error)
	ApplySucceeded(context.Context, string, Canonical) (Record, bool, error)
	PaymentForAccount(context.Context, string, string) (Record, error)
	RefundByPayment(context.Context, string, string) (RefundRecord, error)
	BeginRefund(context.Context, string, RefundQuote, bool) (RefundRecord, error)
	AttachRefund(context.Context, string, string, RefundOperation) (RefundRecord, error)
	FailRefundAttempt(context.Context, string, string) (RefundRecord, error)
	RefundByProviderID(context.Context, string, string) (RefundRecord, error)
	SetRefundStatus(context.Context, string, string, CanonicalRefund) (RefundRecord, error)
	ApplyRefundSucceeded(context.Context, string, string, CanonicalRefund) (RefundRecord, bool, error)
	UnresolvedRefunds(context.Context, int) ([]RefundRecord, error)
}

type Service struct {
	repo           Repository
	activeProvider string
	providers      map[string]Provider
	returnURL      string
}

func NewService(repo Repository, provider Provider, returnURL string) (*Service, error) {
	return NewServiceWithProviders(repo, provider.Name(), []Provider{provider}, returnURL)
}

// NewServiceWithProviders keeps adapters for old providers available after
// new sales switch elsewhere. A refund is selected by the provider recorded
// on its payment, never by whichever provider happens to be active today.
func NewServiceWithProviders(
	repo Repository, activeProvider string, providers []Provider, returnURL string,
) (*Service, error) {
	parsed, err := url.Parse(returnURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, errors.New("payment return URL must be public HTTPS")
	}
	configured := make(map[string]Provider, len(providers))
	for _, provider := range providers {
		if provider == nil || provider.Name() == "" {
			return nil, errors.New("payment provider must have a name")
		}
		if _, exists := configured[provider.Name()]; exists {
			return nil, errors.New("payment provider name is configured twice")
		}
		configured[provider.Name()] = provider
	}
	if _, ok := configured[activeProvider]; !ok {
		return nil, errors.New("active payment provider is not configured")
	}
	return &Service{
		repo: repo, activeProvider: activeProvider, providers: configured,
		returnURL: parsed.String(),
	}, nil
}

func (s *Service) ProviderName() string { return s.activeProvider }

func (s *Service) provider(name string) (Provider, error) {
	provider, ok := s.providers[name]
	if !ok {
		return nil, ErrProviderNotFound
	}
	return provider, nil
}

func (s *Service) Products(ctx context.Context) ([]Product, error) {
	return s.repo.Products(ctx)
}

// Start creates or resumes the account's one open payment. The repository row
// exists before the provider call, and retries reuse its idempotency key.
func (s *Service) Start(ctx context.Context, accountID, productID string) (Record, error) {
	provider, err := s.provider(s.activeProvider)
	if err != nil {
		return Record{}, err
	}
	record, err := s.repo.BeginPayment(ctx, accountID, productID, provider.Name())
	if err != nil {
		return Record{}, err
	}
	if record.ProviderPaymentID != "" && record.CheckoutURL != "" {
		return record, nil
	}

	checkout, err := provider.Create(ctx, CreateRequest{
		PaymentID:      record.ID,
		IdempotencyKey: record.IdempotencyKey,
		AmountMinor:    record.Product.AmountMinor,
		Currency:       record.Product.Currency,
		Description:    record.Product.Title,
		ReturnURL:      s.returnURL,
	})
	if err != nil {
		if errors.Is(err, ErrRejected) {
			_, _ = s.repo.SetPaymentStatus(ctx, record.ID, StatusFailed, false)
		}
		return Record{}, err
	}
	return s.repo.AttachCheckout(ctx, record.ID, checkout)
}

func (s *Service) Current(ctx context.Context, accountID string) (Record, error) {
	return s.repo.CurrentPayment(ctx, accountID)
}

// Handle treats the webhook only as a wake-up signal. The object used below
// is fetched through the provider's authenticated server API.
func (s *Service) Handle(ctx context.Context, providerPaymentID string) (Record, bool, error) {
	return s.HandlePayment(ctx, s.activeProvider, providerPaymentID)
}

// HandlePayment names the provider whose webhook route woke Core. This keeps
// late notifications from an old provider valid after new sales have moved.
func (s *Service) HandlePayment(
	ctx context.Context, providerName, providerPaymentID string,
) (Record, bool, error) {
	provider, err := s.provider(providerName)
	if err != nil {
		return Record{}, false, err
	}
	record, err := s.repo.PaymentByProviderID(ctx, provider.Name(), providerPaymentID)
	if err != nil {
		return Record{}, false, err
	}
	canonical, err := provider.Get(ctx, providerPaymentID)
	if err != nil {
		return Record{}, false, err
	}
	if canonical.ProviderPaymentID != providerPaymentID {
		return Record{}, false, errors.New("provider returned a different payment")
	}
	if err := VerifyMatch(record.ID, record.Product.AmountMinor, record.Product.Currency, canonical); err != nil {
		return Record{}, false, err
	}

	switch canonical.Status {
	case StatusSucceeded:
		if !canonical.Paid {
			return Record{}, false, errors.New("succeeded payment is not paid")
		}
		return s.repo.ApplySucceeded(ctx, record.ID, canonical)
	case StatusCanceled, StatusPending:
		updated, updateErr := s.repo.SetPaymentStatus(ctx, record.ID, canonical.Status, canonical.Test)
		return updated, false, updateErr
	default:
		return Record{}, false, errors.New("provider returned an unsupported payment status")
	}
}

// VerifySucceeded proves that a canonical provider object is the exact object
// our row expected. A succeeded word alone is not proof: the amount, currency,
// internal identifier and paid flag all belong to the same assertion.
func VerifySucceeded(expectedID string, amountMinor int64, currency string, got Canonical) error {
	if err := VerifyMatch(expectedID, amountMinor, currency, got); err != nil {
		return err
	}
	if got.Status != StatusSucceeded || !got.Paid {
		return errors.New("payment is not confirmed")
	}
	return nil
}

// VerifyMatch applies to every terminal state. A forged canceled webhook must
// not be able to close another payment any more than a forged success may open
// VIP.
func VerifyMatch(expectedID string, amountMinor int64, currency string, got Canonical) error {
	if got.PaymentID != expectedID {
		return errors.New("payment metadata does not identify the expected payment")
	}
	if got.AmountMinor != amountMinor || got.Currency != currency {
		return fmt.Errorf("payment amount does not match")
	}
	return nil
}
