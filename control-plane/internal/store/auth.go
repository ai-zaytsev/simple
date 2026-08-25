package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Attempt is one request to sign in, before anybody has proved anything.
type Attempt struct {
	ID        uuid.UUID
	ExpiresAt time.Time
}

var (
	// ErrNoSuchAttempt covers a link that was never issued, one that expired,
	// and one that was already followed. Deliberately one error rather than
	// three: the person holding a bad link learns only that it does not work,
	// and the difference between "expired" and "already used" tells an attacker
	// which addresses are real.
	ErrNoSuchAttempt = errors.New("no usable attempt")

	// ErrTooManyRequests stops an address being used to send mail at somebody.
	ErrTooManyRequests = errors.New("too many requests for this address")
)

// RecentAttempts counts requests for an address inside a window, for the rate
// limit. Counted per address rather than per device, because the mailbox is
// what gets flooded and a new device identifier costs an attacker nothing.
func (s *Store) RecentAttempts(ctx context.Context, email string, within time.Duration) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		select count(*) from login_attempts
		where lower(email) = lower($1) and created_at > now() - $2::interval`,
		email, within.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("cannot count recent attempts: %w", err)
	}
	return count, nil
}

// CreateAttempt records a request. No account is created here.
//
// Creating one at this point would let anybody bring an account into existence
// for an address they do not own, and would make the existence of an account a
// thing an attacker can cause rather than discover.
func (s *Store) CreateAttempt(
	ctx context.Context,
	email string,
	deviceID uuid.UUID,
	tokenHash []byte,
	lifetime time.Duration,
) (Attempt, error) {
	attempt := Attempt{ID: uuid.New(), ExpiresAt: time.Now().UTC().Add(lifetime)}

	_, err := s.pool.Exec(ctx, `
		insert into login_attempts (id, email, device_id, token_hash, expires_at)
		values ($1, $2, $3, $4, $5)`,
		attempt.ID, email, deviceID, tokenHash, attempt.ExpiresAt)
	if err != nil {
		return Attempt{}, fmt.Errorf("cannot record the attempt: %w", err)
	}
	return attempt, nil
}

// ConfirmAttempt is what happens when the link is followed.
//
// Everything here is one transaction, because the three things it does have to
// be true together: the link is spent, an account exists for the address, and
// the device belongs to it. A crash between them would leave a spent link with
// no account, and the person would be locked out with nothing to retry.
func (s *Store) ConfirmAttempt(ctx context.Context, tokenHash []byte) (uuid.UUID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("cannot begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		attemptID uuid.UUID
		email     string
		deviceID  uuid.UUID
	)

	// The row is locked while it is being spent, so that two clicks on the same
	// link cannot both find it unused.
	err = tx.QueryRow(ctx, `
		select id, email, device_id from login_attempts
		where token_hash = $1
		  and consumed_at is null
		  and expires_at > now()
		for update`, tokenHash).Scan(&attemptID, &email, &deviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNoSuchAttempt
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("cannot read the attempt: %w", err)
	}

	// An address that has been here before keeps its account, with everything
	// attached to it. This is what makes reinstalling a phone a sign-in rather
	// than a fresh start.
	var accountID uuid.UUID
	err = tx.QueryRow(ctx, `
		select id from accounts where lower(email) = lower($1)`, email).Scan(&accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		accountID = uuid.New()
		if _, err := tx.Exec(ctx, `
			insert into accounts (id, email, state) values ($1, $2, 'ACTIVE')`,
			accountID, email); err != nil {
			return uuid.Nil, fmt.Errorf("cannot create the account: %w", err)
		}
	} else if err != nil {
		return uuid.Nil, fmt.Errorf("cannot look up the account: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		insert into devices (id, account_id, last_seen_at) values ($1, $2, now())
		on conflict (id) do update set account_id = $2, last_seen_at = now()`,
		deviceID, accountID); err != nil {
		return uuid.Nil, fmt.Errorf("cannot bind the device: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		update login_attempts
		set consumed_at = now(), confirmed_at = now(), account_id = $2
		where id = $1`, attemptID, accountID); err != nil {
		return uuid.Nil, fmt.Errorf("cannot spend the attempt: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("cannot commit: %w", err)
	}
	return accountID, nil
}

// AttemptOutcome is what the waiting application is told.
type AttemptOutcome struct {
	Confirmed bool
	Expired   bool
	AccountID uuid.UUID
}

// PollAttempt answers the application waiting for the link to be followed.
//
// The device identifier is part of the lookup so that knowing an attempt
// identifier is not enough: only the device that asked can learn the answer.
func (s *Store) PollAttempt(ctx context.Context, attemptID, deviceID uuid.UUID) (AttemptOutcome, error) {
	var (
		confirmed *time.Time
		expiresAt time.Time
		accountID *uuid.UUID
	)

	err := s.pool.QueryRow(ctx, `
		select confirmed_at, expires_at, account_id from login_attempts
		where id = $1 and device_id = $2`, attemptID, deviceID).
		Scan(&confirmed, &expiresAt, &accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return AttemptOutcome{}, ErrNoSuchAttempt
	}
	if err != nil {
		return AttemptOutcome{}, fmt.Errorf("cannot read the attempt: %w", err)
	}

	outcome := AttemptOutcome{
		Confirmed: confirmed != nil,
		Expired:   confirmed == nil && time.Now().After(expiresAt),
	}
	if accountID != nil {
		outcome.AccountID = *accountID
	}
	return outcome, nil
}

// AccountOfDevice returns the account a device already belongs to.
//
// Used so that an application which has signed in once does not ask again on
// every start.
func (s *Store) AccountOfDevice(ctx context.Context, deviceID uuid.UUID) (uuid.UUID, error) {
	var accountID *uuid.UUID
	err := s.pool.QueryRow(ctx, `
		select account_id from devices where id = $1`, deviceID).Scan(&accountID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && accountID == nil) {
		return uuid.Nil, ErrNoSuchAttempt
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("cannot read the device: %w", err)
	}
	return *accountID, nil
}

// LiveAttempt finds this device's most recent unused link for an address.
//
// It exists for the moment someone presses "send again" once too often. The
// rate limit answers exactly like success, so without this the application
// would be handed an attempt that was never recorded, poll it, and tell the
// person their link had expired - while up to five working links sat in their
// mailbox. Returning the newest live one makes the screen tell the truth.
func (s *Store) LiveAttempt(ctx context.Context, email string, deviceID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		select id from login_attempts
		where lower(email) = lower($1)
		  and device_id = $2
		  and consumed_at is null
		  and expires_at > now()
		order by created_at desc
		limit 1`, email, deviceID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNoSuchAttempt
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("cannot find a live attempt: %w", err)
	}
	return id, nil
}
