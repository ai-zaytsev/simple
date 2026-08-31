package payment

import (
	"context"
	"errors"
	"math/big"
	"time"
)

type RefundStatus string

const (
	RefundStatusCreating  RefundStatus = "creating"
	RefundStatusPending   RefundStatus = "pending"
	RefundStatusSucceeded RefundStatus = "succeeded"
	RefundStatusCanceled  RefundStatus = "canceled"
	RefundStatusFailed    RefundStatus = "failed"
)

type RefundMode string

const (
	RefundModeFull    RefundMode = "full"
	RefundModeProRata RefundMode = "pro_rata"
)

const (
	RefundReasonPaymentUnconfirmed = "payment_unconfirmed"
	RefundReasonPeriodEnded        = "paid_period_ended"
	RefundReasonUnsupported        = "refund_not_supported"
	RefundReasonPartialUnsupported = "partial_refund_not_supported"
	RefundReasonBelowMinimum       = "refund_below_provider_minimum"
	RefundReasonAlreadySucceeded   = "already_refunded"
)

// RefundLimits describes provider mechanics, not our commercial policy. A
// payment adapter may support a full return but not a partial one, and it may
// reject amounts below a provider-owned minimum.
type RefundLimits struct {
	Full         bool
	Partial      bool
	MinimumMinor int64
}

type RefundQuote struct {
	PaymentID        string
	Available        bool
	Retry            bool
	Reason           string
	Mode             RefundMode
	AmountMinor      int64
	Currency         string
	CalculatedAt     time.Time
	FullRefundUntil  *time.Time
	PaidPeriodEndsAt *time.Time
}

type RefundAttempt struct {
	ID                 string
	IdempotencyKey     string
	ProviderRefundID   string
	Status             RefundStatus
	CancellationReason string
	CreatedAt          time.Time
}

// RefundRecord is provider-neutral durable state. ProviderPaymentID is kept
// only so the adapter can return money to the original operation.
type RefundRecord struct {
	ID                   string
	PaymentID            string
	AccountID            string
	Provider             string
	ProviderPaymentID    string
	AmountMinor          int64
	Currency             string
	Mode                 RefundMode
	Status               RefundStatus
	CancellationReason   string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	SucceededAt          *time.Time
	EntitlementRevokedAt *time.Time
	Attempt              RefundAttempt
}

type RefundCreateRequest struct {
	RefundID          string
	ProviderPaymentID string
	IdempotencyKey    string
	AmountMinor       int64
	Currency          string
	Description       string
}

type RefundOperation struct {
	ProviderRefundID string
}

type CanonicalRefund struct {
	ProviderRefundID   string
	ProviderPaymentID  string
	RefundID           string
	AmountMinor        int64
	Currency           string
	Status             RefundStatus
	CancellationReason string
	CreatedAt          *time.Time
}

// QuoteRefund is the complete commercial policy. The seven-day boundary is
// inclusive; after it the result is floor(amount * remaining / full * 75%).
// big.Int is intentional: multiplying kopecks by nanoseconds over a year does
// not fit in int64 even though the final amount does.
func QuoteRefund(record Record, now time.Time, limits RefundLimits) RefundQuote {
	now = now.UTC()
	quote := RefundQuote{
		PaymentID: record.ID, Currency: record.Product.Currency, CalculatedAt: now,
	}
	if record.PaidAt != nil {
		boundary := record.PaidAt.UTC().Add(7 * 24 * time.Hour)
		quote.FullRefundUntil = &boundary
	}
	if record.EntitlementEndsAt != nil {
		ends := record.EntitlementEndsAt.UTC()
		quote.PaidPeriodEndsAt = &ends
	}
	if record.Status != StatusSucceeded || record.EntitlementAppliedAt == nil ||
		record.PaidAt == nil || record.EntitlementStartedAt == nil || record.EntitlementEndsAt == nil {
		quote.Reason = RefundReasonPaymentUnconfirmed
		return quote
	}
	start := record.EntitlementStartedAt.UTC()
	end := record.EntitlementEndsAt.UTC()
	if !end.After(start) || !now.Before(end) {
		quote.Reason = RefundReasonPeriodEnded
		return quote
	}
	// A missing provider flag is not permission. Legacy rows can be reconciled
	// separately, but Core must never guess that money can be returned.
	if !limits.Full || record.ProviderRefundable == nil || !*record.ProviderRefundable {
		quote.Reason = RefundReasonUnsupported
		return quote
	}
	if !now.After(record.PaidAt.UTC().Add(7 * 24 * time.Hour)) {
		quote.Available = true
		quote.Mode = RefundModeFull
		quote.AmountMinor = record.Product.AmountMinor
		return quote
	}
	if !limits.Partial {
		quote.Reason = RefundReasonPartialUnsupported
		return quote
	}

	remaining := end.Sub(now)
	full := end.Sub(start)
	if remaining <= 0 || full <= 0 {
		quote.Reason = RefundReasonPeriodEnded
		return quote
	}
	numerator := new(big.Int).SetInt64(record.Product.AmountMinor)
	numerator.Mul(numerator, big.NewInt(remaining.Nanoseconds()))
	numerator.Mul(numerator, big.NewInt(75))
	denominator := new(big.Int).SetInt64(full.Nanoseconds())
	denominator.Mul(denominator, big.NewInt(100))
	numerator.Quo(numerator, denominator)
	if !numerator.IsInt64() {
		quote.Reason = RefundReasonUnsupported
		return quote
	}
	amount := numerator.Int64()
	if amount <= 0 {
		quote.Reason = RefundReasonPeriodEnded
		return quote
	}
	if amount < limits.MinimumMinor {
		quote.Reason = RefundReasonBelowMinimum
		return quote
	}
	quote.Available = true
	quote.Mode = RefundModeProRata
	quote.AmountMinor = amount
	return quote
}

func VerifyRefund(expected RefundRecord, got CanonicalRefund) error {
	if got.ProviderRefundID != expected.Attempt.ProviderRefundID {
		return errors.New("provider returned a different refund")
	}
	if got.ProviderPaymentID != expected.ProviderPaymentID {
		return errors.New("refund belongs to a different payment")
	}
	if got.RefundID != expected.ID {
		return errors.New("refund metadata does not identify the expected refund")
	}
	if got.AmountMinor != expected.AmountMinor || got.Currency != expected.Currency {
		return errors.New("refund amount does not match")
	}
	return nil
}

type RefundUnavailableError struct{ Quote RefundQuote }

func (e RefundUnavailableError) Error() string { return ErrRefundUnavailable.Error() }
func (e RefundUnavailableError) Unwrap() error { return ErrRefundUnavailable }

// RefundQuote returns the current policy decision without changing money or
// entitlement state.
func (s *Service) RefundQuote(ctx context.Context, accountID, paymentID string, now time.Time) (RefundQuote, error) {
	record, err := s.repo.PaymentForAccount(ctx, accountID, paymentID)
	if err != nil {
		return RefundQuote{}, err
	}
	provider, err := s.provider(record.Provider)
	if err != nil {
		return RefundQuote{}, err
	}
	quote := QuoteRefund(record, now, provider.RefundLimits(record.PaymentMethod))
	if existing, existingErr := s.repo.RefundByPayment(ctx, accountID, paymentID); existingErr == nil {
		switch existing.Status {
		case RefundStatusSucceeded:
			quote.Available = false
			quote.AmountMinor = existing.AmountMinor
			quote.Mode = existing.Mode
			quote.Reason = RefundReasonAlreadySucceeded
		case RefundStatusCreating, RefundStatusPending:
			quote.Available = false
			quote.AmountMinor = existing.AmountMinor
			quote.Mode = existing.Mode
			quote.Reason = string(existing.Status)
		case RefundStatusCanceled, RefundStatusFailed:
			quote.Retry = quote.Available
		}
		return quote, nil
	} else if !errors.Is(existingErr, ErrRefundNotFound) {
		return RefundQuote{}, existingErr
	}
	return quote, nil
}

// StartRefund creates or resumes exactly one logical refund for the payment.
// A canceled/failed operation advances to a new provider attempt only when the
// caller explicitly says retry; network retries of a creating attempt retain
// the original idempotency key.
func (s *Service) StartRefund(
	ctx context.Context, accountID, paymentID string, retry bool, now time.Time,
) (RefundRecord, error) {
	record, err := s.repo.PaymentForAccount(ctx, accountID, paymentID)
	if err != nil {
		return RefundRecord{}, err
	}
	provider, err := s.provider(record.Provider)
	if err != nil {
		return RefundRecord{}, err
	}
	quote := QuoteRefund(record, now, provider.RefundLimits(record.PaymentMethod))
	if !quote.Available {
		return RefundRecord{}, RefundUnavailableError{Quote: quote}
	}
	refund, err := s.repo.BeginRefund(ctx, accountID, quote, retry)
	if err != nil {
		return RefundRecord{}, err
	}
	if refund.Status == RefundStatusSucceeded || refund.Status == RefundStatusCanceled || refund.Status == RefundStatusFailed {
		return refund, nil
	}
	updated, _, err := s.resumeRefund(ctx, provider, refund, now.UTC())
	if errors.Is(err, ErrRefundOutcomeUnknown) {
		// This is a durable in-progress state, not proof of failure. Returning
		// it lets the client say "being verified" while the background worker
		// keeps searching without issuing an unsafe second money operation.
		return refund, nil
	}
	return updated, err
}

func (s *Service) RefreshRefund(ctx context.Context, accountID, paymentID string) (RefundRecord, error) {
	refund, err := s.repo.RefundByPayment(ctx, accountID, paymentID)
	if err != nil {
		return RefundRecord{}, err
	}
	if refund.Status == RefundStatusSucceeded || refund.Attempt.ProviderRefundID == "" {
		return refund, nil
	}
	provider, err := s.provider(refund.Provider)
	if err != nil {
		return RefundRecord{}, err
	}
	updated, _, err := s.reconcileRefund(ctx, provider, refund)
	return updated, err
}

func (s *Service) HandleRefund(ctx context.Context, providerName, providerRefundID string) (RefundRecord, bool, error) {
	provider, err := s.provider(providerName)
	if err != nil {
		return RefundRecord{}, false, err
	}
	refund, err := s.repo.RefundByProviderID(ctx, providerName, providerRefundID)
	if err != nil {
		return RefundRecord{}, false, err
	}
	return s.reconcileRefund(ctx, provider, refund)
}

func (s *Service) reconcileRefund(ctx context.Context, provider Provider, refund RefundRecord) (RefundRecord, bool, error) {
	canonical, err := provider.GetRefund(ctx, refund.Attempt.ProviderRefundID)
	if err != nil {
		return RefundRecord{}, false, err
	}
	if err := VerifyRefund(refund, canonical); err != nil {
		return RefundRecord{}, false, err
	}
	switch canonical.Status {
	case RefundStatusSucceeded:
		return s.repo.ApplyRefundSucceeded(ctx, refund.ID, refund.Attempt.ID, canonical)
	case RefundStatusPending, RefundStatusCanceled:
		updated, updateErr := s.repo.SetRefundStatus(ctx, refund.ID, refund.Attempt.ID, canonical)
		return updated, false, updateErr
	default:
		return RefundRecord{}, false, errors.New("provider returned an unsupported refund status")
	}
}

// resumeRefund is the lost-response boundary. Before repeating POST it asks
// the provider for refunds of the original payment and joins the object whose
// private metadata names this logical refund. YooKassa only guarantees a POST
// idempotency key for 24 hours, so once that provider-owned window has elapsed
// Core refuses to issue another POST unless it can first prove no old object
// exists. VIP remains unchanged while the outcome is unknown.
func (s *Service) resumeRefund(
	ctx context.Context, provider Provider, refund RefundRecord, now time.Time,
) (RefundRecord, bool, error) {
	if refund.Attempt.ProviderRefundID == "" {
		canonical, findErr := provider.FindRefund(ctx, refund.ProviderPaymentID, refund.ID)
		switch {
		case findErr == nil:
			expected := refund
			expected.Attempt.ProviderRefundID = canonical.ProviderRefundID
			if err := VerifyRefund(expected, canonical); err != nil {
				return RefundRecord{}, false, err
			}
			attached, err := s.repo.AttachRefund(ctx, refund.ID, refund.Attempt.ID, RefundOperation{
				ProviderRefundID: canonical.ProviderRefundID,
			})
			if err != nil {
				return RefundRecord{}, false, err
			}
			refund = attached
		case !errors.Is(findErr, ErrRefundNotFound):
			return RefundRecord{}, false, findErr
		default:
			window := provider.RefundIdempotencyWindow()
			if window <= 0 || refund.Attempt.CreatedAt.IsZero() ||
				!now.Before(refund.Attempt.CreatedAt.UTC().Add(window)) {
				return RefundRecord{}, false, ErrRefundOutcomeUnknown
			}
			operation, createErr := provider.CreateRefund(ctx, RefundCreateRequest{
				RefundID: refund.ID, ProviderPaymentID: refund.ProviderPaymentID,
				IdempotencyKey: refund.Attempt.IdempotencyKey,
				AmountMinor:    refund.AmountMinor, Currency: refund.Currency,
				Description: "Возврат VIP",
			})
			if createErr != nil {
				if errors.Is(createErr, ErrRejected) {
					_, _ = s.repo.FailRefundAttempt(ctx, refund.ID, refund.Attempt.ID)
				}
				return RefundRecord{}, false, createErr
			}
			attached, err := s.repo.AttachRefund(ctx, refund.ID, refund.Attempt.ID, operation)
			if err != nil {
				return RefundRecord{}, false, err
			}
			refund = attached
		}
	}
	return s.reconcileRefund(ctx, provider, refund)
}

// ReconcileRefunds closes pending operations even if Android never opens again.
// YooKassa emits refund.succeeded but no refund.canceled webhook for this API.
func (s *Service) ReconcileRefunds(ctx context.Context, limit int) (int, error) {
	rows, err := s.repo.UnresolvedRefunds(ctx, limit)
	if err != nil {
		return 0, err
	}
	completed := 0
	var outcomeErr error
	for _, refund := range rows {
		provider, providerErr := s.provider(refund.Provider)
		if providerErr != nil {
			outcomeErr = errors.Join(outcomeErr, providerErr)
			continue
		}
		updated, _, reconcileErr := s.resumeRefund(ctx, provider, refund, time.Now().UTC())
		if reconcileErr != nil {
			outcomeErr = errors.Join(outcomeErr, reconcileErr)
			continue
		}
		if updated.Status == RefundStatusSucceeded || updated.Status == RefundStatusCanceled {
			completed++
		}
	}
	return completed, outcomeErr
}
