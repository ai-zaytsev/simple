package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"download.simplevpn/control-plane/internal/npd"
	"download.simplevpn/control-plane/internal/npd/lknpd"
)

// enqueueReceipt records that a payment's tax position has changed and needs
// putting right.
//
// Takes a transaction rather than opening one: the obligation to issue a
// receipt is written in the same commit as the money it belongs to. Anything
// else has a window where VIP is granted and nothing remembers that a receipt
// is owed - and that window is exactly where an obligation would be lost.
func enqueueReceipt(ctx context.Context, tx pgx.Tx, paymentID uuid.UUID) error {
	// One open operation per payment. A refund arriving before the first
	// receipt was issued joins the existing row rather than making a second:
	// the amount is worked out when the row is processed, not when it is made.
	_, err := tx.Exec(ctx, `
		insert into npd_operations (payment_id)
		values ($1)
		on conflict (payment_id) where state = 'pending' do nothing`, paymentID)
	if err != nil {
		return fmt.Errorf("cannot record the receipt obligation: %w", err)
	}
	return nil
}

func (s *Store) SaveSession(ctx context.Context, session lknpd.Session) error {
	var expires any
	if !session.ExpiresAt.IsZero() {
		expires = session.ExpiresAt
	}
	_, err := s.pool.Exec(ctx, `
		insert into npd_session (id, inn, device_id, access_token, refresh_token, expires_at, updated_at)
		values (true, $1, $2, $3, $4, $5, now())
		on conflict (id) do update set
		    inn = excluded.inn,
		    device_id = excluded.device_id,
		    access_token = excluded.access_token,
		    refresh_token = excluded.refresh_token,
		    expires_at = excluded.expires_at,
		    updated_at = now()`,
		session.INN, session.DeviceID, session.AccessToken, session.RefreshToken, expires)
	if err != nil {
		// The message must not carry the row: every field in it is a secret.
		return errors.New("cannot store the НПД session")
	}
	return nil
}

func (s *Store) SetAvailability(ctx context.Context, ok bool, detail string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `
		insert into npd_availability (id, ok, checked_at, detail)
		values (true, $1, $2, $3)
		on conflict (id) do update set ok = excluded.ok,
		    checked_at = excluded.checked_at, detail = excluded.detail`,
		ok, at.UTC(), detail)
	if err != nil {
		return fmt.Errorf("cannot record НПД availability: %w", err)
	}
	return nil
}

// TaxAvailability is what the panel and the purchase gate read.
type TaxAvailability struct {
	OK        bool       `json:"ok"`
	CheckedAt *time.Time `json:"checked_at"`
	Detail    string     `json:"detail"`
	Pending   int        `json:"pending"`
}

func (s *Store) TaxAvailability(ctx context.Context) (TaxAvailability, error) {
	var out TaxAvailability
	var detail *string
	err := s.pool.QueryRow(ctx, `
		select a.ok, a.checked_at, a.detail,
		       (select count(*) from npd_operations where state = 'pending')
		from npd_availability a where a.id = true`).
		Scan(&out.OK, &out.CheckedAt, &detail, &out.Pending)
	if errors.Is(err, pgx.ErrNoRows) {
		return TaxAvailability{Detail: "проверка ещё не выполнялась"}, nil
	}
	if err != nil {
		return TaxAvailability{}, fmt.Errorf("cannot read НПД availability: %w", err)
	}
	if detail != nil {
		out.Detail = *detail
	}
	return out, nil
}

func (s *Store) PendingOperations(ctx context.Context, limit int) ([]npd.Operation, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		select id::text, payment_id::text, attempts, alerted_at is not null
		from npd_operations
		where state = 'pending'
		order by created_at
		limit $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("cannot read the receipt queue: %w", err)
	}
	defer rows.Close()

	var out []npd.Operation
	for rows.Next() {
		var op npd.Operation
		if err := rows.Scan(&op.ID, &op.PaymentID, &op.Attempts, &op.Alerted); err != nil {
			return nil, fmt.Errorf("cannot read a queued receipt: %w", err)
		}
		out = append(out, op)
	}
	return out, rows.Err()
}

func (s *Store) PendingCount(ctx context.Context) (int, error) {
	var count int
	if err := s.pool.QueryRow(ctx,
		`select count(*) from npd_operations where state = 'pending'`).Scan(&count); err != nil {
		return 0, fmt.Errorf("cannot count the receipt queue: %w", err)
	}
	return count, nil
}

func (s *Store) OperationDone(ctx context.Context, operationID string) error {
	id, err := uuid.Parse(operationID)
	if err != nil {
		return fmt.Errorf("cannot read the operation id: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		update npd_operations set state = 'done', last_error = null, updated_at = now()
		where id = $1 and state = 'pending'`, id)
	if err != nil {
		return fmt.Errorf("cannot close the receipt operation: %w", err)
	}
	return nil
}

func (s *Store) OperationFailed(ctx context.Context, operationID, message string, alerted bool) error {
	id, err := uuid.Parse(operationID)
	if err != nil {
		return fmt.Errorf("cannot read the operation id: %w", err)
	}
	// Still pending. A failed receipt is retried, because the obligation does
	// not go away by being difficult.
	_, err = s.pool.Exec(ctx, `
		update npd_operations
		set attempts = attempts + 1,
		    last_error = left($2, 1000),
		    alerted_at = case when $3 and alerted_at is null then now() else alerted_at end,
		    updated_at = now()
		where id = $1`, id, message, alerted)
	if err != nil {
		return fmt.Errorf("cannot record the receipt failure: %w", err)
	}
	return nil
}

// Settlement gathers what the tax decision needs about one payment: what was
// paid, what has been given back, and which receipt stands.
//
// One query rather than three, so the decision cannot be made against a
// half-updated picture where a refund has landed but the receipt has not.
func (s *Store) Settlement(ctx context.Context, paymentID string) (npd.Settlement, error) {
	id, err := uuid.Parse(paymentID)
	if err != nil {
		return npd.Settlement{}, fmt.Errorf("cannot read the payment id: %w", err)
	}

	var (
		out          npd.Settlement
		receiptRow   *string
		receiptUUID  *string
		receiptMinor *int64
		creating     bool
		paidAt       *time.Time
		email        *string
	)
	err = s.pool.QueryRow(ctx, `
		select p.id::text, p.account_id::text, a.email, p.product_id,
		       p.amount_minor, p.paid_at,
		       coalesce((select sum(r.amount_minor) from refunds r
		                 where r.payment_id = p.id and r.status = 'succeeded'), 0),
		       r.id::text, r.receipt_uuid, r.amount_minor,
		       coalesce(r.state = 'creating', false)
		from payments p
		join accounts a on a.id = p.account_id
		left join npd_receipts r
		       on r.payment_id = p.id and r.state in ('creating', 'active')
		where p.id = $1`, id).
		Scan(&out.PaymentID, &out.AccountID, &email, &out.ProductName,
			&out.PaidMinor, &paidAt, &out.RefundedMinor,
			&receiptRow, &receiptUUID, &receiptMinor, &creating)
	if errors.Is(err, pgx.ErrNoRows) {
		return npd.Settlement{}, fmt.Errorf("платёж %s не найден", paymentID)
	}
	if err != nil {
		return npd.Settlement{}, fmt.Errorf("cannot read the settlement: %w", err)
	}

	if email != nil {
		out.AccountEmail = *email
	}
	if paidAt != nil {
		out.PaidAt = *paidAt
	} else {
		out.PaidAt = time.Now().UTC()
	}
	out.Unresolved = creating
	if !creating && receiptRow != nil && receiptUUID != nil && receiptMinor != nil {
		out.Active = &npd.Receipt{
			RowID: *receiptRow, UUID: *receiptUUID, AmountMinor: *receiptMinor,
		}
	}
	return out, nil
}

// BeginReceipt opens the row that will hold a receipt, before ФНС is asked for
// one. The unique index refuses a second open row for the same payment, so two
// receipts for one payment cannot be born from a race or a retry.
func (s *Store) BeginReceipt(ctx context.Context, paymentID string, amountMinor int64) (string, error) {
	id, err := uuid.Parse(paymentID)
	if err != nil {
		return "", fmt.Errorf("cannot read the payment id: %w", err)
	}
	var rowID string
	err = s.pool.QueryRow(ctx, `
		insert into npd_receipts (payment_id, amount_minor, state)
		values ($1, $2, 'creating')
		returning id::text`, id, amountMinor).Scan(&rowID)
	if err != nil {
		return "", fmt.Errorf("cannot open a receipt row: %w", err)
	}
	return rowID, nil
}

func (s *Store) FinishReceipt(ctx context.Context, rowID, receiptUUID, printURL string) error {
	id, err := uuid.Parse(rowID)
	if err != nil {
		return fmt.Errorf("cannot read the receipt row id: %w", err)
	}
	tag, err := s.pool.Exec(ctx, `
		update npd_receipts set state = 'active', receipt_uuid = $2, print_url = nullif($3, '')
		where id = $1 and state = 'creating'`, id, receiptUUID, printURL)
	if err != nil {
		return fmt.Errorf("cannot record the issued receipt: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.New("строка чека уже не в состоянии создания")
	}
	return nil
}

func (s *Store) CancelReceipt(ctx context.Context, rowID string, at time.Time) error {
	id, err := uuid.Parse(rowID)
	if err != nil {
		return fmt.Errorf("cannot read the receipt row id: %w", err)
	}
	// Only an active receipt can be cancelled. A second cancellation of the
	// same receipt changes nothing here and never reaches ФНС twice.
	tag, err := s.pool.Exec(ctx, `
		update npd_receipts set state = 'cancelled', cancelled_at = $2
		where id = $1 and state = 'active'`, id, at.UTC())
	if err != nil {
		return fmt.Errorf("cannot record the cancelled receipt: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.New("чек уже не действует")
	}
	return nil
}

// SettleReceiptByHand closes an operation that a person dealt with in «Мой
// налог» themselves.
//
// Needed because an operation stuck on "we do not know whether a receipt was
// created" can only be resolved by somebody looking. Without this the queue
// would stay blocked forever and, since an unfinished queue keeps sales shut,
// so would selling.
func (s *Store) SettleReceiptByHand(
	ctx context.Context, paymentID, receiptUUID string, amountMinor int64,
) error {
	id, err := uuid.Parse(paymentID)
	if err != nil {
		return fmt.Errorf("cannot read the payment id: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cannot begin the manual settlement: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if receiptUUID == "" {
		// Nothing stands: the person checked and there is no receipt. Drop the
		// unresolved row so the queue can try again properly.
		if _, err := tx.Exec(ctx,
			`delete from npd_receipts where payment_id = $1 and state = 'creating'`, id); err != nil {
			return fmt.Errorf("cannot clear the unresolved receipt: %w", err)
		}
	} else {
		tag, err := tx.Exec(ctx, `
			update npd_receipts
			set state = 'active', receipt_uuid = $2,
			    amount_minor = case when $3 > 0 then $3 else amount_minor end
			where payment_id = $1 and state = 'creating'`, id, receiptUUID, amountMinor)
		if err != nil {
			return fmt.Errorf("cannot record the manual receipt: %w", err)
		}
		if tag.RowsAffected() == 0 {
			// No unresolved row. Record the receipt as it stands now.
			if _, err := tx.Exec(ctx, `
				insert into npd_receipts (payment_id, receipt_uuid, amount_minor, state)
				values ($1, $2, $3, 'active')`, id, receiptUUID, amountMinor); err != nil {
				return fmt.Errorf("cannot record the manual receipt: %w", err)
			}
		}
	}

	if _, err := tx.Exec(ctx, `
		update npd_operations set state = 'done', updated_at = now()
		where payment_id = $1 and state = 'pending'`, id); err != nil {
		return fmt.Errorf("cannot close the operation: %w", err)
	}
	return tx.Commit(ctx)
}
