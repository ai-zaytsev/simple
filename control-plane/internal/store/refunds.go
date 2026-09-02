package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"download.simplevpn/control-plane/internal/payment"
)

func (s *Store) PaymentForAccount(
	ctx context.Context, account, id string,
) (payment.Record, error) {
	accountID, accountErr := uuid.Parse(account)
	paymentID, paymentErr := uuid.Parse(id)
	if accountErr != nil || paymentErr != nil {
		return payment.Record{}, payment.ErrPaymentNotFound
	}
	var record payment.Record
	err := s.pool.QueryRow(ctx, paymentRecordSQL+`
		where p.id = $1 and p.account_id = $2`, paymentID, accountID).
		Scan(paymentScan(&record)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return payment.Record{}, payment.ErrPaymentNotFound
	}
	if err != nil {
		return payment.Record{}, fmt.Errorf("cannot read the account payment: %w", err)
	}
	return record, nil
}

func (s *Store) RefundByPayment(
	ctx context.Context, account, id string,
) (payment.RefundRecord, error) {
	accountID, accountErr := uuid.Parse(account)
	paymentID, paymentErr := uuid.Parse(id)
	if accountErr != nil || paymentErr != nil {
		return payment.RefundRecord{}, payment.ErrRefundNotFound
	}
	return refundByPayment(ctx, s.pool, accountID, paymentID, false)
}

// BeginRefund serializes on the payment row. It creates one logical refund
// and one provider attempt, resumes live attempts unchanged, and advances the
// attempt number only after an explicit retry of a terminal failure.
func (s *Store) BeginRefund(
	ctx context.Context, account string, quote payment.RefundQuote, retry bool,
) (payment.RefundRecord, error) {
	accountID, accountErr := uuid.Parse(account)
	paymentID, paymentErr := uuid.Parse(quote.PaymentID)
	if accountErr != nil || paymentErr != nil {
		return payment.RefundRecord{}, payment.ErrPaymentNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return payment.RefundRecord{}, fmt.Errorf("cannot begin refund: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var paid payment.Record
	err = tx.QueryRow(ctx, paymentRecordSQL+`
		where p.id = $1 and p.account_id = $2 for update of p`, paymentID, accountID).
		Scan(paymentScan(&paid)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return payment.RefundRecord{}, payment.ErrPaymentNotFound
	}
	if err != nil {
		return payment.RefundRecord{}, fmt.Errorf("cannot lock the refunded payment: %w", err)
	}
	if !quote.Available || quote.PaymentID != paid.ID || quote.Currency != paid.Product.Currency ||
		quote.AmountMinor <= 0 || quote.AmountMinor > paid.Product.AmountMinor ||
		(quote.Mode != payment.RefundModeFull && quote.Mode != payment.RefundModeProRata) ||
		paid.Status != payment.StatusSucceeded || paid.EntitlementAppliedAt == nil {
		return payment.RefundRecord{}, payment.ErrRefundUnavailable
	}

	existing, existingErr := refundByPayment(ctx, tx, accountID, paymentID, true)
	if existingErr == nil {
		switch existing.Status {
		case payment.RefundStatusCreating, payment.RefundStatusPending, payment.RefundStatusSucceeded:
			if err := tx.Commit(ctx); err != nil {
				return payment.RefundRecord{}, fmt.Errorf("cannot resume refund: %w", err)
			}
			return existing, nil
		case payment.RefundStatusCanceled, payment.RefundStatusFailed:
			if !retry {
				return payment.RefundRecord{}, payment.ErrRefundRetryRequired
			}
		default:
			return payment.RefundRecord{}, payment.ErrRefundUnavailable
		}
		attemptID := uuid.New()
		idempotency := uuid.New()
		var attemptNo int
		if err := tx.QueryRow(ctx,
			`select coalesce(max(attempt_no), 0) + 1 from refund_attempts where refund_id = $1`,
			existing.ID,
		).Scan(&attemptNo); err != nil {
			return payment.RefundRecord{}, fmt.Errorf("cannot number refund attempt: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			update refunds set amount_minor = $2, currency = $3, mode = $4,
			    status = 'creating', cancellation_reason = null,
			    calculated_at = $5, updated_at = now()
			where id = $1`, existing.ID, quote.AmountMinor, quote.Currency,
			quote.Mode, quote.CalculatedAt); err != nil {
			return payment.RefundRecord{}, fmt.Errorf("cannot reset refund: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			insert into refund_attempts (
			    id, refund_id, provider, attempt_no, idempotency_key, status
			) values ($1, $2, $3, $4, $5, 'creating')`,
			attemptID, existing.ID, paid.Provider, attemptNo, idempotency); err != nil {
			return payment.RefundRecord{}, fmt.Errorf("cannot create refund retry: %w", err)
		}
		updated, err := refundByID(ctx, tx, uuid.MustParse(existing.ID), false)
		if err != nil {
			return payment.RefundRecord{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return payment.RefundRecord{}, fmt.Errorf("cannot commit refund retry: %w", err)
		}
		return updated, nil
	}
	if !errors.Is(existingErr, payment.ErrRefundNotFound) {
		return payment.RefundRecord{}, existingErr
	}

	refundID := uuid.New()
	attemptID := uuid.New()
	idempotency := uuid.New()
	if _, err := tx.Exec(ctx, `
		insert into refunds (
		    id, payment_id, account_id, provider, amount_minor, currency,
		    mode, status, calculated_at
		) values ($1, $2, $3, $4, $5, $6, $7, 'creating', $8)`,
		refundID, paymentID, accountID, paid.Provider, quote.AmountMinor,
		quote.Currency, quote.Mode, quote.CalculatedAt); err != nil {
		return payment.RefundRecord{}, fmt.Errorf("cannot create refund: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		insert into refund_attempts (
		    id, refund_id, provider, attempt_no, idempotency_key, status
		) values ($1, $2, $3, 1, $4, 'creating')`,
		attemptID, refundID, paid.Provider, idempotency); err != nil {
		return payment.RefundRecord{}, fmt.Errorf("cannot create refund attempt: %w", err)
	}
	created, err := refundByID(ctx, tx, refundID, false)
	if err != nil {
		return payment.RefundRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return payment.RefundRecord{}, fmt.Errorf("cannot commit refund: %w", err)
	}
	return created, nil
}

func (s *Store) AttachRefund(
	ctx context.Context, refund, attempt string, operation payment.RefundOperation,
) (payment.RefundRecord, error) {
	refundID, refundErr := uuid.Parse(refund)
	attemptID, attemptErr := uuid.Parse(attempt)
	if refundErr != nil || attemptErr != nil || operation.ProviderRefundID == "" {
		return payment.RefundRecord{}, payment.ErrRefundNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return payment.RefundRecord{}, fmt.Errorf("cannot attach refund: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var existing *string
	var status payment.RefundStatus
	if err := tx.QueryRow(ctx, `
		select provider_refund_id, status from refund_attempts
		where id = $1 and refund_id = $2 for update`, attemptID, refundID).Scan(&existing, &status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return payment.RefundRecord{}, payment.ErrRefundNotFound
		}
		return payment.RefundRecord{}, fmt.Errorf("cannot lock refund attempt: %w", err)
	}
	if existing != nil && *existing != operation.ProviderRefundID {
		return payment.RefundRecord{}, errors.New("refund attempt already has a different provider object")
	}
	if status == payment.RefundStatusSucceeded {
		updated, readErr := refundByID(ctx, tx, refundID, false)
		if readErr != nil {
			return payment.RefundRecord{}, readErr
		}
		if err := tx.Commit(ctx); err != nil {
			return payment.RefundRecord{}, fmt.Errorf("cannot keep completed refund: %w", err)
		}
		return updated, nil
	}
	if _, err := tx.Exec(ctx, `
		update refund_attempts
		set provider_refund_id = $3, status = 'pending', updated_at = now()
		where id = $1 and refund_id = $2 and status <> 'succeeded'`,
		attemptID, refundID, operation.ProviderRefundID); err != nil {
		return payment.RefundRecord{}, fmt.Errorf("cannot attach provider refund: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		update refunds set status = 'pending', updated_at = now()
		where id = $1 and status <> 'succeeded'`, refundID); err != nil {
		return payment.RefundRecord{}, fmt.Errorf("cannot mark refund pending: %w", err)
	}
	updated, err := refundByID(ctx, tx, refundID, false)
	if err != nil {
		return payment.RefundRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return payment.RefundRecord{}, fmt.Errorf("cannot commit provider refund: %w", err)
	}
	return updated, nil
}

func (s *Store) FailRefundAttempt(
	ctx context.Context, refund, attempt string,
) (payment.RefundRecord, error) {
	refundID, refundErr := uuid.Parse(refund)
	attemptID, attemptErr := uuid.Parse(attempt)
	if refundErr != nil || attemptErr != nil {
		return payment.RefundRecord{}, payment.ErrRefundNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return payment.RefundRecord{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		update refund_attempts set status = 'failed', updated_at = now()
		where id = $1 and refund_id = $2 and status = 'creating'
		  and provider_refund_id is null`, attemptID, refundID); err != nil {
		return payment.RefundRecord{}, fmt.Errorf("cannot fail refund attempt: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		update refunds set status = 'failed', updated_at = now()
		where id = $1 and status = 'creating'`, refundID); err != nil {
		return payment.RefundRecord{}, fmt.Errorf("cannot fail refund: %w", err)
	}
	updated, err := refundByID(ctx, tx, refundID, false)
	if err != nil {
		return payment.RefundRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return payment.RefundRecord{}, err
	}
	return updated, nil
}

func (s *Store) RefundByProviderID(
	ctx context.Context, provider, providerRefundID string,
) (payment.RefundRecord, error) {
	var record payment.RefundRecord
	err := s.pool.QueryRow(ctx, refundRecordSQL+`
		where ra.provider = $1 and ra.provider_refund_id = $2`, provider, providerRefundID).
		Scan(refundScan(&record)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return payment.RefundRecord{}, payment.ErrRefundNotFound
	}
	if err != nil {
		return payment.RefundRecord{}, fmt.Errorf("cannot find provider refund: %w", err)
	}
	return record, nil
}

func (s *Store) SetRefundStatus(
	ctx context.Context, refund, attempt string, canonical payment.CanonicalRefund,
) (payment.RefundRecord, error) {
	refundID, refundErr := uuid.Parse(refund)
	attemptID, attemptErr := uuid.Parse(attempt)
	if refundErr != nil || attemptErr != nil {
		return payment.RefundRecord{}, payment.ErrRefundNotFound
	}
	if canonical.Status != payment.RefundStatusPending && canonical.Status != payment.RefundStatusCanceled {
		return payment.RefundRecord{}, errors.New("refund status is not pending or canceled")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return payment.RefundRecord{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		update refund_attempts set status = $3,
		    cancellation_reason = nullif($4, ''), updated_at = now()
		where id = $1 and refund_id = $2 and status <> 'succeeded'`, attemptID, refundID,
		canonical.Status, canonical.CancellationReason); err != nil {
		return payment.RefundRecord{}, fmt.Errorf("cannot update refund attempt: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		update refunds set status = $2, cancellation_reason = nullif($3, ''),
		    updated_at = now()
		where id = $1 and status <> 'succeeded'`, refundID,
		canonical.Status, canonical.CancellationReason); err != nil {
		return payment.RefundRecord{}, fmt.Errorf("cannot update refund: %w", err)
	}
	updated, err := refundByID(ctx, tx, refundID, false)
	if err != nil {
		return payment.RefundRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return payment.RefundRecord{}, err
	}
	return updated, nil
}

// ApplyRefundSucceeded is the money/access commit boundary. Provider success,
// logical success and paid-VIP removal become visible together; a duplicate
// webhook sees entitlement_revoked_at and cannot repeat either action.
func (s *Store) ApplyRefundSucceeded(
	ctx context.Context, refund, attempt string, canonical payment.CanonicalRefund,
) (payment.RefundRecord, bool, error) {
	refundID, refundErr := uuid.Parse(refund)
	attemptID, attemptErr := uuid.Parse(attempt)
	if refundErr != nil || attemptErr != nil {
		return payment.RefundRecord{}, false, payment.ErrRefundNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return payment.RefundRecord{}, false, fmt.Errorf("cannot begin refund completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var paymentID uuid.UUID
	if err := tx.QueryRow(ctx, `select payment_id from refunds where id = $1`, refundID).
		Scan(&paymentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return payment.RefundRecord{}, false, payment.ErrRefundNotFound
		}
		return payment.RefundRecord{}, false, fmt.Errorf("cannot find refunded payment: %w", err)
	}
	var paidAmount int64
	if err := tx.QueryRow(ctx, `select amount_minor from payments where id = $1 for update`, paymentID).
		Scan(&paidAmount); err != nil {
		return payment.RefundRecord{}, false, fmt.Errorf("cannot lock refunded payment: %w", err)
	}
	expected, err := refundByAttemptForUpdate(ctx, tx, refundID, attemptID)
	if err != nil {
		return payment.RefundRecord{}, false, err
	}
	if expected.EntitlementRevokedAt != nil {
		return expected, false, tx.Commit(ctx)
	}
	if err := payment.VerifyRefund(expected, canonical); err != nil {
		return payment.RefundRecord{}, false, err
	}
	if canonical.Status != payment.RefundStatusSucceeded {
		return payment.RefundRecord{}, false, errors.New("refund is not confirmed")
	}

	var alreadyRefunded int64
	if err := tx.QueryRow(ctx, `
		select coalesce(sum(amount_minor), 0) from refunds
		where payment_id = $1 and status = 'succeeded' and id <> $2`,
		expected.PaymentID, refundID).Scan(&alreadyRefunded); err != nil {
		return payment.RefundRecord{}, false, fmt.Errorf("cannot total payment refunds: %w", err)
	}
	if alreadyRefunded+expected.AmountMinor > paidAmount {
		return payment.RefundRecord{}, false, errors.New("refunds exceed the original payment")
	}

	if _, err := tx.Exec(ctx, `
		update refund_attempts set status = 'succeeded', cancellation_reason = null,
		    updated_at = now() where id = $1 and refund_id = $2`, attemptID, refundID); err != nil {
		return payment.RefundRecord{}, false, fmt.Errorf("cannot complete refund attempt: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		update refunds set status = 'succeeded', cancellation_reason = null,
		    succeeded_at = now(), entitlement_revoked_at = now(), updated_at = now()
		where id = $1`, refundID); err != nil {
		return payment.RefundRecord{}, false, fmt.Errorf("cannot complete refund: %w", err)
	}

	var tier string
	var expiry *time.Time
	if err := tx.QueryRow(ctx, `
		select tier, vip_expires_at from accounts where id = $1 for update`, expected.AccountID).
		Scan(&tier, &expiry); err != nil {
		return payment.RefundRecord{}, false, fmt.Errorf("cannot lock refunded account: %w", err)
	}
	// Null expiry is an independent administrative VIP. A refund reverses the
	// paid entitlement and must not silently revoke a separate manual grant.
	if tier == "VIP" && expiry != nil {
		if err := expireAccount(ctx, tx, uuid.MustParse(expected.AccountID)); err != nil {
			return payment.RefundRecord{}, false, err
		}
	}

	// The tax position changed with the money. Same commit, same reason as on
	// the payment side: an obligation recorded anywhere else can be lost.
	if err := enqueueReceipt(ctx, tx, uuid.MustParse(expected.PaymentID)); err != nil {
		return payment.RefundRecord{}, false, err
	}
	updated, err := refundByID(ctx, tx, refundID, false)
	if err != nil {
		return payment.RefundRecord{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return payment.RefundRecord{}, false, fmt.Errorf("cannot commit refund and VIP removal: %w", err)
	}
	return updated, true, nil
}

func (s *Store) UnresolvedRefunds(ctx context.Context, limit int) ([]payment.RefundRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, refundRecordSQL+`
		where r.status in ('creating', 'pending')
		  and ra.status in ('creating', 'pending')
		order by r.updated_at asc limit $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("cannot list unresolved refunds: %w", err)
	}
	defer rows.Close()
	result := []payment.RefundRecord{}
	for rows.Next() {
		var record payment.RefundRecord
		if err := rows.Scan(refundScan(&record)...); err != nil {
			return nil, fmt.Errorf("cannot read unresolved refund: %w", err)
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

const refundRecordSQL = `
	select r.id::text, r.payment_id::text, r.account_id::text,
	       r.provider, coalesce(p.provider_payment_id, ''),
	       r.amount_minor, r.currency, r.mode, r.status,
	       coalesce(r.cancellation_reason, ''), r.created_at, r.updated_at,
	       r.succeeded_at, r.entitlement_revoked_at,
	       ra.id::text, ra.idempotency_key::text,
	       coalesce(ra.provider_refund_id, ''), ra.status,
	       coalesce(ra.cancellation_reason, ''), ra.created_at
	from refunds r
	join payments p on p.id = r.payment_id
	join refund_attempts ra on ra.refund_id = r.id
`

func refundScan(record *payment.RefundRecord) []any {
	return []any{
		&record.ID, &record.PaymentID, &record.AccountID,
		&record.Provider, &record.ProviderPaymentID,
		&record.AmountMinor, &record.Currency, &record.Mode, &record.Status,
		&record.CancellationReason, &record.CreatedAt, &record.UpdatedAt,
		&record.SucceededAt, &record.EntitlementRevokedAt,
		&record.Attempt.ID, &record.Attempt.IdempotencyKey,
		&record.Attempt.ProviderRefundID, &record.Attempt.Status,
		&record.Attempt.CancellationReason, &record.Attempt.CreatedAt,
	}
}

func refundByPayment(
	ctx context.Context, q queryer, accountID, paymentID uuid.UUID, forUpdate bool,
) (payment.RefundRecord, error) {
	query := refundRecordSQL + `
		where r.account_id = $1 and r.payment_id = $2
		order by ra.attempt_no desc limit 1`
	if forUpdate {
		query += ` for update of r, ra`
	}
	var record payment.RefundRecord
	err := q.QueryRow(ctx, query, accountID, paymentID).Scan(refundScan(&record)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return payment.RefundRecord{}, payment.ErrRefundNotFound
	}
	if err != nil {
		return payment.RefundRecord{}, fmt.Errorf("cannot read payment refund: %w", err)
	}
	return record, nil
}

func refundByID(
	ctx context.Context, q queryer, refundID uuid.UUID, forUpdate bool,
) (payment.RefundRecord, error) {
	query := refundRecordSQL + `
		where r.id = $1 order by ra.attempt_no desc limit 1`
	if forUpdate {
		query += ` for update of r, ra`
	}
	var record payment.RefundRecord
	err := q.QueryRow(ctx, query, refundID).Scan(refundScan(&record)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return payment.RefundRecord{}, payment.ErrRefundNotFound
	}
	if err != nil {
		return payment.RefundRecord{}, fmt.Errorf("cannot read refund: %w", err)
	}
	return record, nil
}

func refundByAttemptForUpdate(
	ctx context.Context, tx pgx.Tx, refundID, attemptID uuid.UUID,
) (payment.RefundRecord, error) {
	var record payment.RefundRecord
	err := tx.QueryRow(ctx, refundRecordSQL+`
		where r.id = $1 and ra.id = $2 for update of r, ra`, refundID, attemptID).
		Scan(refundScan(&record)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return payment.RefundRecord{}, payment.ErrRefundNotFound
	}
	if err != nil {
		return payment.RefundRecord{}, fmt.Errorf("cannot lock refund attempt: %w", err)
	}
	return record, nil
}
