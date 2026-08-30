package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"download.simplevpn/control-plane/internal/payment"
	"download.simplevpn/control-plane/internal/purchase"
)

// Products is the catalog Android may draw. Only active rows leave Core; the
// client never owns a fallback price or duration.
func (s *Store) Products(ctx context.Context) ([]payment.Product, error) {
	rows, err := s.pool.Query(ctx, `
		select id, title, amount_minor, currency, duration_months
		from payment_products where active
		order by duration_months`)
	if err != nil {
		return nil, fmt.Errorf("cannot read payment products: %w", err)
	}
	defer rows.Close()

	products := []payment.Product{}
	for rows.Next() {
		var product payment.Product
		if err := rows.Scan(&product.ID, &product.Title, &product.AmountMinor,
			&product.Currency, &product.DurationMonths); err != nil {
			return nil, fmt.Errorf("cannot read a payment product: %w", err)
		}
		products = append(products, product)
	}
	return products, rows.Err()
}

// BeginPayment creates the durable operation before the provider is called.
// The account row is the lock: two devices on one account cannot both create a
// checkout, even when their requests arrive together through different entry
// paths.
func (s *Store) BeginPayment(
	ctx context.Context, account, productID, provider string,
) (payment.Record, error) {
	accountID, err := uuid.Parse(account)
	if err != nil {
		return payment.Record{}, payment.ErrPaymentNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return payment.Record{}, fmt.Errorf("cannot begin payment: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		tier      string
		created   time.Time
		expiresAt *time.Time
	)
	err = tx.QueryRow(ctx, `
		select tier, created_at, vip_expires_at
		from accounts where id = $1 for update`, accountID).
		Scan(&tier, &created, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return payment.Record{}, payment.ErrPaymentNotFound
	}
	if err != nil {
		return payment.Record{}, fmt.Errorf("cannot lock the paying account: %w", err)
	}

	now := time.Now().UTC()
	if tier == "VIP" && expiresAt != nil && !expiresAt.After(now) {
		if err := expireAccount(ctx, tx, accountID); err != nil {
			return payment.Record{}, err
		}
		tier = "FREE"
		expiresAt = nil
	}
	if tier == "VIP" {
		return payment.Record{}, payment.ErrAlreadyVIP
	}

	// Re-evaluate the existing sales switch and FREE wait under the same
	// transaction as the insert. Turning sales off therefore closes the door
	// for every operation that has not already created its row.
	var raw []byte
	if err := tx.QueryRow(ctx,
		`select value from service_state where key = 'purchases' for share`).Scan(&raw); err != nil {
		return payment.Record{}, payment.ErrPurchaseUnavailable
	}
	var settings struct {
		Open     bool `json:"open"`
		FreeDays int  `json:"free_days"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return payment.Record{}, payment.ErrPurchaseUnavailable
	}
	offer := purchase.Assess(now, created, tier, purchase.Settings{
		Open: settings.Open, FreeDays: settings.FreeDays,
	})
	if !offer.Available {
		return payment.Record{}, payment.ErrPurchaseUnavailable
	}

	// A retry of the same product resumes. A different choice is refused while
	// the first checkout can still be paid; otherwise two payable promises
	// would exist for one account at once.
	existing, err := paymentByAccount(ctx, tx, accountID, true)
	if err == nil {
		if existing.Product.ID != productID {
			return payment.Record{}, payment.ErrPaymentInProgress
		}
		return existing, tx.Commit(ctx)
	}
	if !errors.Is(err, payment.ErrPaymentNotFound) {
		return payment.Record{}, err
	}

	var product payment.Product
	err = tx.QueryRow(ctx, `
		select id, title, amount_minor, currency, duration_months
		from payment_products where id = $1 and active`, productID).
		Scan(&product.ID, &product.Title, &product.AmountMinor,
			&product.Currency, &product.DurationMonths)
	if errors.Is(err, pgx.ErrNoRows) {
		return payment.Record{}, payment.ErrProductNotFound
	}
	if err != nil {
		return payment.Record{}, fmt.Errorf("cannot read the payment product: %w", err)
	}

	id := uuid.New()
	idempotency := uuid.New()
	var made time.Time
	if err := tx.QueryRow(ctx, `
		insert into payments (
			id, account_id, product_id, provider, idempotency_key,
			amount_minor, currency, duration_months, status
		) values ($1, $2, $3, $4, $5, $6, $7, $8, 'creating')
		returning created_at`,
		id, accountID, product.ID, provider, idempotency,
		product.AmountMinor, product.Currency, product.DurationMonths).
		Scan(&made); err != nil {
		return payment.Record{}, fmt.Errorf("cannot create the payment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return payment.Record{}, fmt.Errorf("cannot commit the payment: %w", err)
	}
	return payment.Record{
		ID: id.String(), AccountID: accountID.String(), Product: product,
		Provider: provider, IdempotencyKey: idempotency.String(),
		Status: payment.StatusCreating, CreatedAt: made,
	}, nil
}

func (s *Store) AttachCheckout(
	ctx context.Context, id string, checkout payment.Checkout,
) (payment.Record, error) {
	paymentID, err := uuid.Parse(id)
	if err != nil {
		return payment.Record{}, payment.ErrPaymentNotFound
	}
	// Redirect checkout is open until canonical webhook handling says
	// otherwise. Even an unusual create response carrying "succeeded" must
	// not release the one-open-payment guard before entitlement is applied.
	status := payment.StatusPending
	tag, err := s.pool.Exec(ctx, `
		update payments
		set provider_payment_id = $2, checkout = $3, status = $4,
		    provider_test = $5, updated_at = now()
		where id = $1`, paymentID, checkout.ProviderPaymentID, checkout.URL, status, checkout.Test)
	if err != nil {
		return payment.Record{}, fmt.Errorf("cannot attach the provider checkout: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return payment.Record{}, payment.ErrPaymentNotFound
	}
	return paymentByID(ctx, s.pool, paymentID)
}

// CurrentPayment is read-only. In particular it does not call the provider or
// apply VIP, which is why returning from a browser cannot confirm anything.
func (s *Store) CurrentPayment(ctx context.Context, account string) (payment.Record, error) {
	accountID, err := uuid.Parse(account)
	if err != nil {
		return payment.Record{}, payment.ErrPaymentNotFound
	}
	return paymentByAccount(ctx, s.pool, accountID, false)
}

func (s *Store) PaymentByProviderID(
	ctx context.Context, provider, providerPaymentID string,
) (payment.Record, error) {
	var record payment.Record
	err := s.pool.QueryRow(ctx, paymentRecordSQL+`
		where p.provider = $1 and p.provider_payment_id = $2`, provider, providerPaymentID).
		Scan(paymentScan(&record)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return payment.Record{}, payment.ErrPaymentNotFound
	}
	if err != nil {
		return payment.Record{}, fmt.Errorf("cannot find the provider payment: %w", err)
	}
	return record, nil
}

func (s *Store) SetPaymentStatus(
	ctx context.Context, id string, status payment.Status, providerTest bool,
) (payment.Record, error) {
	paymentID, err := uuid.Parse(id)
	if err != nil {
		return payment.Record{}, payment.ErrPaymentNotFound
	}
	if _, err := s.pool.Exec(ctx, `
		update payments
		set status = case when entitlement_applied_at is null then $2 else status end,
		    provider_test = $3,
		    updated_at = now()
		where id = $1`, paymentID, status, providerTest); err != nil {
		return payment.Record{}, fmt.Errorf("cannot update payment status: %w", err)
	}
	return paymentByID(ctx, s.pool, paymentID)
}

// ApplySucceeded is the exactly-once boundary. The payment row lock, applied
// timestamp and account update commit together. A duplicate webhook sees the
// timestamp and returns the same expiry without adding the duration again.
func (s *Store) ApplySucceeded(
	ctx context.Context, id string, canonical payment.Canonical,
) (payment.Record, bool, error) {
	paymentID, err := uuid.Parse(id)
	if err != nil {
		return payment.Record{}, false, payment.ErrPaymentNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return payment.Record{}, false, fmt.Errorf("cannot begin entitlement: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	record, err := paymentByIDForUpdate(ctx, tx, paymentID)
	if err != nil {
		return payment.Record{}, false, err
	}
	if record.EntitlementAppliedAt != nil {
		return record, false, tx.Commit(ctx)
	}
	if err := payment.VerifySucceeded(
		record.ID, record.Product.AmountMinor, record.Product.Currency, canonical,
	); err != nil {
		return payment.Record{}, false, err
	}

	paidAt := time.Now().UTC()
	if canonical.PaidAt != nil {
		paidAt = canonical.PaidAt.UTC()
	}
	if _, err := tx.Exec(ctx, `
		update payments
		set status = 'succeeded', provider_test = $2, paid_at = $3,
		    entitlement_applied_at = now(), updated_at = now()
		where id = $1`, paymentID, canonical.Test, paidAt); err != nil {
		return payment.Record{}, false, fmt.Errorf("cannot complete the payment: %w", err)
	}

	// An administrative VIP has no expiry and is never shortened. Every paid
	// VIP starts at now; the greatest expression also preserves paid time in
	// the defensive case of a second distinct successful payment.
	if _, err := tx.Exec(ctx, `
		update accounts
		set tier = 'VIP',
		    vip_expires_at = case
		      when tier = 'VIP' and vip_expires_at is null then null
		      else greatest(coalesce(vip_expires_at, now()), now())
		           + make_interval(months => $2)
		    end
		where id = $1`, record.AccountID, record.Product.DurationMonths); err != nil {
		return payment.Record{}, false, fmt.Errorf("cannot activate VIP: %w", err)
	}

	updated, err := paymentByID(ctx, tx, paymentID)
	if err != nil {
		return payment.Record{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return payment.Record{}, false, fmt.Errorf("cannot commit VIP: %w", err)
	}
	return updated, true, nil
}

// ExpireVIPs returns paid VIP accounts to FREE and immediately applies FREE's
// access limits. Administrative VIP has a null expiry and is untouched.
func (s *Store) ExpireVIPs(ctx context.Context) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("cannot begin VIP expiry: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		select id from accounts
		where tier = 'VIP' and vip_expires_at <= now()
		for update`)
	if err != nil {
		return 0, fmt.Errorf("cannot find expired VIP accounts: %w", err)
	}
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, fmt.Errorf("cannot read an expired VIP account: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, id := range ids {
		if err := expireAccount(ctx, tx, id); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("cannot commit VIP expiry: %w", err)
	}
	return len(ids), nil
}

func expireAccount(ctx context.Context, tx pgx.Tx, accountID uuid.UUID) error {
	if _, err := tx.Exec(ctx, `
		update accounts set tier = 'FREE', vip_expires_at = null
		where id = $1`, accountID); err != nil {
		return fmt.Errorf("cannot expire VIP: %w", err)
	}

	// FREE cannot use external links. Revoke them in the same transaction as
	// the tier so nodes never observe FREE with VIP-only credentials.
	if _, err := tx.Exec(ctx, `
		update device_credentials c
		set state = 'REVOKED', revoked_at = now(), updated_seq = next_seq('credentials')
		from devices d
		where c.device_id = d.id and d.account_id = $1 and d.kind = 'external'
		  and c.state = 'ACTIVE'`, accountID); err != nil {
		return fmt.Errorf("cannot revoke expired external access: %w", err)
	}

	// Preserve the most recently granted application access and apply FREE's
	// configured device limit to the rest.
	var keep uuid.UUID
	err := tx.QueryRow(ctx, `
		select c.device_id
		from device_credentials c join devices d on d.id = c.device_id
		where d.account_id = $1 and d.kind = 'app' and c.state = 'ACTIVE'
		order by c.created_at desc limit 1`, accountID).Scan(&keep)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot choose the device kept after VIP: %w", err)
	}
	_, err = evictBeyondLimit(ctx, tx, accountID, keep)
	return err
}

const paymentRecordSQL = `
	select p.id::text, p.account_id::text, p.product_id, pr.title,
	       p.amount_minor, p.currency, p.duration_months,
	       p.provider, coalesce(p.provider_payment_id, ''),
	       p.idempotency_key::text, p.status, coalesce(p.checkout, ''),
	       p.provider_test, p.created_at, p.paid_at,
	       p.entitlement_applied_at, a.vip_expires_at
	from payments p
	join payment_products pr on pr.id = p.product_id
	join accounts a on a.id = p.account_id
`

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func paymentScan(record *payment.Record) []any {
	return []any{
		&record.ID, &record.AccountID, &record.Product.ID, &record.Product.Title,
		&record.Product.AmountMinor, &record.Product.Currency, &record.Product.DurationMonths,
		&record.Provider, &record.ProviderPaymentID, &record.IdempotencyKey,
		&record.Status, &record.CheckoutURL, &record.ProviderTest, &record.CreatedAt,
		&record.PaidAt, &record.EntitlementAppliedAt, &record.VIPExpiresAt,
	}
}

func paymentByAccount(
	ctx context.Context, q queryer, accountID uuid.UUID, openOnly bool,
) (payment.Record, error) {
	where := "where p.account_id = $1"
	if openOnly {
		where += " and p.status in ('creating', 'pending')"
	}
	var record payment.Record
	err := q.QueryRow(ctx, paymentRecordSQL+where+` order by p.created_at desc limit 1`, accountID).
		Scan(paymentScan(&record)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return payment.Record{}, payment.ErrPaymentNotFound
	}
	if err != nil {
		return payment.Record{}, fmt.Errorf("cannot read the account payment: %w", err)
	}
	return record, nil
}

func paymentByID(ctx context.Context, q queryer, id uuid.UUID) (payment.Record, error) {
	var record payment.Record
	err := q.QueryRow(ctx, paymentRecordSQL+`where p.id = $1`, id).
		Scan(paymentScan(&record)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return payment.Record{}, payment.ErrPaymentNotFound
	}
	if err != nil {
		return payment.Record{}, fmt.Errorf("cannot read the payment: %w", err)
	}
	return record, nil
}

func paymentByIDForUpdate(ctx context.Context, tx pgx.Tx, id uuid.UUID) (payment.Record, error) {
	var record payment.Record
	err := tx.QueryRow(ctx, paymentRecordSQL+`where p.id = $1 for update of p`, id).
		Scan(paymentScan(&record)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return payment.Record{}, payment.ErrPaymentNotFound
	}
	if err != nil {
		return payment.Record{}, fmt.Errorf("cannot lock the payment: %w", err)
	}
	return record, nil
}
