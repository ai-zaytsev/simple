package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrNoSuchDevice covers a token nobody was issued, a token that was replaced,
// and a device that has been cut off. One error rather than three: the holder
// of a bad token learns only that it does not work.
var ErrNoSuchDevice = errors.New("no such device")

// Device is what a request has proved about itself.
type Device struct {
	ID        uuid.UUID
	AccountID uuid.UUID

	// Which tier the account behind this device is on.
	//
	// Read from accounts, never stored on the device, and that is the whole
	// mechanism by which a status belongs to a person rather than to a phone.
	// There is no tier column on devices for this to be filled from, so a
	// device cannot carry a status of its own even by mistake - which means
	// signing in on a second phone cannot produce a different answer, because
	// both answers come from the same row.
	Tier string
}

// IssueDeviceToken replaces this device's secret and returns the new one.
//
// Replaced rather than reused, so that a response lost on the way to the phone
// costs nothing: the next attempt issues another and the unheard one stops
// working. Only the hash is kept, so a copy of this table lets nobody connect.
func (s *Store) IssueDeviceToken(ctx context.Context, deviceID uuid.UUID, hash []byte) error {
	// Only to a device that still has access.
	//
	// Without this condition a spent link stayed useful for as long as the
	// attempt row lived: a device cut off a moment ago could be polled again,
	// be handed a fresh secret, ask for a plan, and be issued a new credential
	// because it no longer had one. Being cut off would have lasted until
	// somebody stopped asking.
	tag, err := s.pool.Exec(ctx, `
		update devices set token_hash = $2, last_seen_at = now()
		where id = $1
		  and exists (
		    select 1 from device_credentials c
		    where c.device_id = $1 and c.state = 'ACTIVE'
		  )`,
		deviceID, hash)
	if err != nil {
		return fmt.Errorf("cannot issue a device token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoSuchDevice
	}
	return nil
}

// DeviceByToken is the only way a request becomes a device.
//
// The identifier a client sends is not consulted anywhere. It was a claim
// before this existed, and a claim is exactly what an attacker supplies.
func (s *Store) DeviceByToken(ctx context.Context, hash []byte) (Device, error) {
	var device Device
	// The join is the point. A device is identified and its account's tier is
	// read in one statement, so there is no moment at which the two could be
	// about different accounts and no second lookup anybody could forget.
	err := s.pool.QueryRow(ctx, `
		select d.id, d.account_id, a.tier
		from devices d
		join accounts a on a.id = d.account_id
		where d.token_hash = $1 and d.account_id is not null`, hash).
		Scan(&device.ID, &device.AccountID, &device.Tier)
	if errors.Is(err, pgx.ErrNoRows) {
		return Device{}, ErrNoSuchDevice
	}
	if err != nil {
		return Device{}, fmt.Errorf("cannot identify the device: %w", err)
	}
	return device, nil
}

// EnsureCredential returns this device's VPN credential, creating one if it has
// none.
//
// Idempotent on purpose: a device asks for a plan every day, and each ask must
// not mint another credential. The unique index on live rows makes that true
// even if two requests arrive at once.
func (s *Store) EnsureCredential(ctx context.Context, deviceID uuid.UUID) (uuid.UUID, error) {
	var credential uuid.UUID
	err := s.pool.QueryRow(ctx, `
		select credential_uuid from device_credentials
		where device_id = $1 and state = 'ACTIVE'`, deviceID).Scan(&credential)
	if err == nil {
		return credential, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("cannot read the credential: %w", err)
	}

	credential = uuid.New()
	_, err = s.pool.Exec(ctx, `
		insert into device_credentials (id, device_id, credential_uuid, updated_seq)
		values ($1, $2, $3, next_seq('credentials'))
		on conflict (device_id) where state = 'ACTIVE' do nothing`,
		uuid.New(), deviceID, credential)
	if err != nil {
		return uuid.Nil, fmt.Errorf("cannot create a credential: %w", err)
	}

	// Read back rather than trust the insert: when two requests race, the one
	// that lost has to return what the winner wrote.
	err = s.pool.QueryRow(ctx, `
		select credential_uuid from device_credentials
		where device_id = $1 and state = 'ACTIVE'`, deviceID).Scan(&credential)
	if err != nil {
		return uuid.Nil, fmt.Errorf("cannot read the credential back: %w", err)
	}
	return credential, nil
}

// RevokeDevice cuts one device off and leaves every other one alone.
//
// The token goes with the credential. Removing only the credential would leave
// a device that can still ask for plans and be handed a fresh one; removing
// only the token would leave a credential the nodes still accept.
func (s *Store) RevokeDevice(ctx context.Context, deviceID uuid.UUID) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("cannot begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		update device_credentials
		set state = 'REVOKED', revoked_at = now(), updated_seq = next_seq('credentials')
		where device_id = $1 and state = 'ACTIVE'`, deviceID)
	if err != nil {
		return false, fmt.Errorf("cannot revoke the credential: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`update devices set token_hash = null where id = $1`, deviceID); err != nil {
		return false, fmt.Errorf("cannot clear the device token: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("cannot commit: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// DevicesOfAccount lists a person's devices so that one can be picked out.
type DeviceSummary struct {
	ID         uuid.UUID
	Credential *uuid.UUID
	State      string
	CreatedAt  string
	LastSeenAt string

	// Whether this is an installation of our application or something else
	// holding a link, and what the person calls it. Both are here so that one
	// list can show a phone and a television side by side: revoking the right
	// one must not require reading identifiers.
	Kind  string
	Label string
}

func (s *Store) DevicesOfAccount(ctx context.Context, accountID uuid.UUID) ([]DeviceSummary, error) {
	rows, err := s.pool.Query(ctx, `
		select d.id,
		       c.credential_uuid,
		       coalesce(c.state, 'NONE'),
		       to_char(d.created_at, 'YYYY-MM-DD"T"HH24:MI:SSZ'),
		       to_char(coalesce(d.last_seen_at, d.created_at), 'YYYY-MM-DD"T"HH24:MI:SSZ'),
		       d.kind, d.label
		from devices d
		left join device_credentials c
		       on c.device_id = d.id and c.state = 'ACTIVE'
		where d.account_id = $1
		order by d.created_at`, accountID)
	if err != nil {
		return nil, fmt.Errorf("cannot list devices: %w", err)
	}
	defer rows.Close()

	var out []DeviceSummary
	for rows.Next() {
		var d DeviceSummary
		if err := rows.Scan(&d.ID, &d.Credential, &d.State, &d.CreatedAt, &d.LastSeenAt,
			&d.Kind, &d.Label); err != nil {
			return nil, fmt.Errorf("cannot read a device: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// NodeByToken identifies a node asking for the list of who may connect.
func (s *Store) NodeByToken(ctx context.Context, hash []byte) (string, error) {
	var alias string
	err := s.pool.QueryRow(ctx,
		`select alias from nodes where token_hash = $1`, hash).Scan(&alias)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNoSuchDevice
	}
	if err != nil {
		return "", fmt.Errorf("cannot identify the node: %w", err)
	}
	return alias, nil
}

// LiveCredentials is every credential a node should currently accept.
//
// The whole list, not a delta. A node that applies a delta it half received is
// a node whose idea of who may connect drifts from ours and never comes back;
// the whole list is a few kilobytes and makes the node's state a function of
// this answer rather than of its own history.
func (s *Store) LiveCredentials(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		select c.credential_uuid::text
		from device_credentials c
		join devices d on d.id = c.device_id
		join accounts a on a.id = d.account_id
		where c.state = 'ACTIVE' and a.state = 'ACTIVE'
		order by c.credential_uuid`)
	if err != nil {
		return nil, fmt.Errorf("cannot list credentials: %w", err)
	}
	defer rows.Close()

	credentials := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cannot read a credential: %w", err)
		}
		credentials = append(credentials, id)
	}
	return credentials, rows.Err()
}

// SetNodeToken records the secret a node will authenticate with.
func (s *Store) SetNodeToken(ctx context.Context, alias string, hash []byte) error {
	tag, err := s.pool.Exec(ctx,
		`update nodes set token_hash = $2 where alias = $1`, alias, hash)
	if err != nil {
		return fmt.Errorf("cannot set the node token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoSuchDevice
	}
	return nil
}

// evictBeyondLimit cuts off the oldest devices when an account has more than
// its tier allows.
//
// Oldest by when their access was granted, and the device that has just signed
// in is never among them: a person who signs in has said which device they
// mean, and taking that one away would make signing in do the opposite of what
// it looks like.
//
// Both halves go, exactly as a deliberate revocation does. Leaving the token
// would let a cut-off device ask for a plan and be handed a fresh credential;
// leaving the credential would let it keep connecting with the one it has.
func evictBeyondLimit(ctx context.Context, tx pgx.Tx, accountID, keep uuid.UUID) ([]uuid.UUID, error) {
	var limit int
	err := tx.QueryRow(ctx, `
		select l.max_devices
		from accounts a join tier_limits l on l.tier = a.tier
		where a.id = $1`, accountID).Scan(&limit)
	if err != nil {
		return nil, fmt.Errorf("cannot read the device limit: %w", err)
	}

	// Only the application's own installations. A television is not competing
	// for the same slot as a phone: with one shared number, connecting a
	// television on a tier that allows one device would cut off the phone that
	// connected it, and the person would watch their own action sign them out.
	rows, err := tx.Query(ctx, `
		select c.device_id
		from device_credentials c
		join devices d on d.id = c.device_id
		where d.account_id = $1
		  and c.state = 'ACTIVE'
		  and c.device_id <> $2
		order by c.created_at desc
		offset $3`, accountID, keep, limit-1)
	if err != nil {
		return nil, fmt.Errorf("cannot find devices over the limit: %w", err)
	}

	var over []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("cannot read a device over the limit: %w", err)
		}
		over = append(over, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot finish reading devices: %w", err)
	}
	if len(over) == 0 {
		return nil, nil
	}

	if _, err := tx.Exec(ctx, `
		update device_credentials
		set state = 'REVOKED', revoked_at = now(), updated_seq = next_seq('credentials')
		where device_id = any($1) and state = 'ACTIVE'`, over); err != nil {
		return nil, fmt.Errorf("cannot revoke a credential: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`update devices set token_hash = null where id = any($1)`, over); err != nil {
		return nil, fmt.Errorf("cannot clear a device token: %w", err)
	}

	return over, nil
}

// AccountTier is what an account's status is, and what it currently means.
type AccountTier struct {
	AccountID  uuid.UUID
	Tier       string
	MaxDevices int
	Devices    int
}

// ErrNoSuchAccount is returned when nobody has signed in with that address.
var ErrNoSuchAccount = errors.New("no such account")

// SetAccountTier moves an account between statuses.
//
// By address, because that is the only handle a person has that an operator
// also has. The address is used to find the row and is not returned, so a
// caller that logs the answer logs an account identifier rather than somebody's
// mailbox.
//
// The database refuses a tier nobody has defined, through the foreign key on
// tier_limits. That refusal is left to the database rather than checked here:
// a list of allowed words in Go and a list of rows in the schema would be two
// lists, and the day they disagree is the day one of them is wrong.
func (s *Store) SetAccountTier(ctx context.Context, email, tier string) (AccountTier, error) {
	var out AccountTier

	err := s.pool.QueryRow(ctx, `
		update accounts set tier = $2
		where lower(email) = lower($1)
		returning id, tier`, email, tier).Scan(&out.AccountID, &out.Tier)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountTier{}, ErrNoSuchAccount
	}
	if err != nil {
		// The address is not wrapped into the error: an error travels into
		// logs, and this one would carry a mailbox with it.
		return AccountTier{}, fmt.Errorf("cannot set the tier: %w", err)
	}

	// What the new status means and how many devices are on it, so that the
	// answer says what happened rather than only that something did.
	if err := s.pool.QueryRow(ctx, `
		select l.max_devices,
		       (select count(*)::int from devices d where d.account_id = $1)
		from tier_limits l where l.tier = $2`, out.AccountID, out.Tier).
		Scan(&out.MaxDevices, &out.Devices); err != nil {
		return out, fmt.Errorf("cannot read what the tier allows: %w", err)
	}
	return out, nil
}

// AccountTierByEmail reads a status without changing it.
func (s *Store) AccountTierByEmail(ctx context.Context, email string) (AccountTier, error) {
	var out AccountTier
	err := s.pool.QueryRow(ctx, `
		select a.id, a.tier, l.max_devices,
		       (select count(*)::int from devices d where d.account_id = a.id)
		from accounts a join tier_limits l on l.tier = a.tier
		where lower(a.email) = lower($1)`, email).
		Scan(&out.AccountID, &out.Tier, &out.MaxDevices, &out.Devices)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountTier{}, ErrNoSuchAccount
	}
	if err != nil {
		return AccountTier{}, fmt.Errorf("cannot read the tier: %w", err)
	}
	return out, nil
}

// ErrTooManyExternal means the tier does not allow another one.
var ErrTooManyExternal = errors.New("no room for another external device")

// AddExternalDevice creates a router, a television or a computer.
//
// One statement's worth of difference from an application installation: no
// token, because it never asks us anything, and a label, because a person
// revoking one needs to know which is which.
//
// The limit is checked inside the transaction that does the insert, so two
// requests arriving together cannot both find room and both take it.
func (s *Store) AddExternalDevice(
	ctx context.Context, accountID uuid.UUID, label string,
) (DeviceSummary, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return DeviceSummary{}, fmt.Errorf("cannot begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var allowed, used int
	err = tx.QueryRow(ctx, `
		select l.max_external,
		       (select count(*)::int
		          from devices d
		          join device_credentials c
		            on c.device_id = d.id and c.state = 'ACTIVE'
		         where d.account_id = $1 and d.kind = 'external')
		from accounts a join tier_limits l on l.tier = a.tier
		where a.id = $1`, accountID).Scan(&allowed, &used)
	if err != nil {
		return DeviceSummary{}, fmt.Errorf("cannot read the external limit: %w", err)
	}
	if used >= allowed {
		return DeviceSummary{}, ErrTooManyExternal
	}

	deviceID := uuid.New()
	if _, err := tx.Exec(ctx, `
		insert into devices (id, account_id, kind, label)
		values ($1, $2, 'external', $3)`, deviceID, accountID, label); err != nil {
		return DeviceSummary{}, fmt.Errorf("cannot create the device: %w", err)
	}

	// Its own credential, from the same table every other credential lives in.
	// This is where "no shared key" stops being a promise: there is one row per
	// device and a unique index over the live ones, so a second device cannot
	// be given the first one's access even deliberately.
	credential := uuid.New()
	if _, err := tx.Exec(ctx, `
		insert into device_credentials (id, device_id, credential_uuid, updated_seq)
		values ($1, $2, $3, next_seq('credentials'))`,
		uuid.New(), deviceID, credential); err != nil {
		return DeviceSummary{}, fmt.Errorf("cannot create the credential: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return DeviceSummary{}, fmt.Errorf("cannot commit: %w", err)
	}

	return DeviceSummary{
		ID:         deviceID,
		Credential: &credential,
		State:      "ACTIVE",
		Kind:       "external",
		Label:      label,
	}, nil
}
