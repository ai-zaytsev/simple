package store

import (
	"context"
	"errors"
	"fmt"
	"time"

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
		join tier_limits l on l.tier = a.tier
		where c.state = 'ACTIVE' and a.state = 'ACTIVE'
		  and (d.kind <> 'external' or l.max_external is null or l.max_external > 0)
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
	// Nullable, because a tier may have no limit at all. Null is not zero here
	// and the difference is the whole of it: zero would mean an account allowed
	// no devices, and every sign-in would evict everything the person had.
	// Read into a pointer so the two cannot be confused by a value that looks
	// like a number either way.
	var limit *int
	err := tx.QueryRow(ctx, `
		select l.max_devices
		from accounts a join tier_limits l on l.tier = a.tier
		where a.id = $1`, accountID).Scan(&limit)
	if err != nil {
		return nil, fmt.Errorf("cannot read the device limit: %w", err)
	}

	// No limit, so nothing is over it. Answered before the query rather than
	// inside it: the query builds its offset by subtracting one from the
	// limit, and there is no number here to subtract from.
	if limit == nil {
		return nil, nil
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
		  and d.kind = 'app'
		  and c.state = 'ACTIVE'
		  and c.device_id <> $2
		order by c.created_at desc
		offset $3`, accountID, keep, *limit-1)
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
//
// The limits are pointers because a tier may have none. An operator asking
// what VIP grants gets "no limit" and not a number that happens to be large,
// which is the difference between a policy and a guess about one.
type AccountTier struct {
	AccountID   uuid.UUID
	Tier        string
	MaxDevices  *int
	MaxExternal *int
	Devices     int
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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AccountTier{}, fmt.Errorf("cannot begin tier change: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var accountID uuid.UUID
	err = tx.QueryRow(ctx, `
		select id from accounts where lower(email) = lower($1) for update`, email).
		Scan(&accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountTier{}, ErrNoSuchAccount
	}
	if err != nil {
		// The address is not wrapped into the error: an error travels into
		// logs, and this one would carry a mailbox with it.
		return AccountTier{}, fmt.Errorf("cannot find the account: %w", err)
	}

	out, err := setAccountTier(ctx, tx, accountID, tier)
	if err != nil {
		return AccountTier{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AccountTier{}, fmt.Errorf("cannot commit tier change: %w", err)
	}
	return out, nil
}

// setAccountTier is the single operator transition. FREE is not only a word
// on the account: it is the set of access limits that must become visible in
// the same transaction, so a node cannot observe FREE with VIP-only access.
func setAccountTier(
	ctx context.Context, tx pgx.Tx, accountID uuid.UUID, tier string,
) (AccountTier, error) {
	if tier == "FREE" {
		if err := expireAccount(ctx, tx, accountID); err != nil {
			return AccountTier{}, err
		}
	} else if _, err := tx.Exec(ctx, `
		update accounts set tier = $2, vip_expires_at = null where id = $1`,
		accountID, tier); err != nil {
		return AccountTier{}, fmt.Errorf("cannot set the tier: %w", err)
	}

	out := AccountTier{AccountID: accountID, Tier: tier}
	// What the new status means and how many devices are on it, so that the
	// answer says what happened rather than only that something did.
	if err := tx.QueryRow(ctx, `
		select l.max_devices, l.max_external,
		       (select count(*)::int from devices d where d.account_id = $1)
		from tier_limits l where l.tier = $2`, out.AccountID, out.Tier).
		Scan(&out.MaxDevices, &out.MaxExternal, &out.Devices); err != nil {
		return AccountTier{}, fmt.Errorf("cannot read what the tier allows: %w", err)
	}
	return out, nil
}

// AccountTierByEmail reads a status without changing it.
func (s *Store) AccountTierByEmail(ctx context.Context, email string) (AccountTier, error) {
	var out AccountTier
	err := s.pool.QueryRow(ctx, `
		select a.id, a.tier, l.max_devices, l.max_external,
		       (select count(*)::int from devices d where d.account_id = a.id)
		from accounts a join tier_limits l on l.tier = a.tier
		where lower(a.email) = lower($1)`, email).
		Scan(&out.AccountID, &out.Tier, &out.MaxDevices, &out.MaxExternal, &out.Devices)
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

	// Asked for again, rather than made again.
	//
	// The application walks every way in until one answers, which is what
	// makes it survive a blocked address - and it means a request that
	// succeeded here and lost its answer on the way back is sent again. The
	// first live use of this produced two devices called "ноут" from one
	// press, and nothing on the screen could explain why.
	//
	// The name is the handle: it is what a person reads to tell one device
	// from another, and two rows sharing it are two rows nobody can revoke the
	// right one of. So the same name on the same account is the same device,
	// and a retry gets back what the first attempt made.
	var existing DeviceSummary
	err = tx.QueryRow(ctx, `
		select d.id, d.kind, d.label, c.credential_uuid
		from devices d
		join device_credentials c on c.device_id = d.id and c.state = 'ACTIVE'
		where d.account_id = $1 and d.kind = 'external' and d.label = $2`,
		accountID, label).Scan(&existing.ID, &existing.Kind, &existing.Label,
		&existing.Credential)
	if err == nil {
		existing.State = "ACTIVE"
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return DeviceSummary{}, fmt.Errorf("cannot look for an existing device: %w", err)
	}

	// Null allows any number; zero allows none. Both are real answers and they
	// are opposite ones, which is why this is a pointer rather than a number
	// with a sentinel value agreed on somewhere else in the file.
	var allowed *int
	var used int
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
	if allowed != nil && used >= *allowed {
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

// LimitedCredentials is every credential whose account is capped, with the cap.
//
// Returned as a list rather than a rate per credential, because the node needs
// a set to route on and one number to shape with. A rate per credential would
// mean a shaping class per person, which is a great many classes and no more
// truth: everybody on a tier is held to the same figure.
//
// The node learns which credentials are slowed and how slow. It does not learn
// what tier means, who is on one, or why - the same arrangement as the heavier
// group, and for the same reason.
func (s *Store) LimitedCredentials(ctx context.Context) ([]string, int, error) {
	// The lowest cap in force. One number because there is one shaping class;
	// with two paid speeds this becomes a class each, and that is a change to
	// the node rather than to this query.
	var speed *int
	if err := s.pool.QueryRow(ctx, `
		select min(speed_mbit) from tier_limits where speed_mbit is not null`).
		Scan(&speed); err != nil {
		return nil, 0, fmt.Errorf("cannot read the speed limits: %w", err)
	}
	if speed == nil {
		return []string{}, 0, nil
	}

	rows, err := s.pool.Query(ctx, `
		select c.credential_uuid::text
		from device_credentials c
		join devices d on d.id = c.device_id
		join accounts a on a.id = d.account_id
		join tier_limits l on l.tier = a.tier
		where c.state = 'ACTIVE' and l.speed_mbit is not null
		  and (d.kind <> 'external' or l.max_external is null or l.max_external > 0)
		order by c.credential_uuid`)
	if err != nil {
		return nil, 0, fmt.Errorf("cannot list limited credentials: %w", err)
	}
	defer rows.Close()

	limited := []string{}
	for rows.Next() {
		var credential string
		if err := rows.Scan(&credential); err != nil {
			return nil, 0, fmt.Errorf("cannot read a limited credential: %w", err)
		}
		limited = append(limited, credential)
	}
	return limited, *speed, rows.Err()
}

// TierOfAccount is the status an account is on and the day it began, and
// nothing else about it.
//
// Separate from AccountTierByEmail because the caller here is the application
// on somebody's phone, asking about itself. It has proved a device token, not
// an address, and it has no business naming one.
//
// The creation date comes back with the tier because the two are always wanted
// together: whether somebody may buy VIP depends on what they have now and on
// how long they have had it. Two queries for one decision would be two chances
// for them to disagree.
func (s *Store) TierOfAccount(
	ctx context.Context, accountID uuid.UUID,
) (string, time.Time, error) {
	var tier string
	var created time.Time
	if err := s.pool.QueryRow(ctx,
		`select tier, created_at from accounts where id = $1`, accountID).
		Scan(&tier, &created); err != nil {
		return "", time.Time{}, fmt.Errorf("cannot read the tier: %w", err)
	}
	return tier, created, nil
}

// ErrNotYours means the device named is not on the caller's account.
var ErrNotYours = errors.New("that device is not on this account")

// RotateExternalCredential gives one external device a new credential.
//
// For the link that stopped working. A person who cannot use their television
// any more needs a working address for that television - not a new device with
// a new name and a lost place in their own list.
//
// The old credential is revoked in the same transaction that creates the new
// one, so the two never both work. That is the whole point when the reason for
// asking is that somebody else has the old link.
//
// Refused unless the device belongs to the caller's account and is external.
// An application installation rotated this way would have its credential
// replaced while its token stayed valid, and the phone would keep asking for
// plans built on a credential the nodes no longer accept - a device that looks
// signed in and carries nothing.
func (s *Store) RotateExternalCredential(
	ctx context.Context, accountID, deviceID uuid.UUID,
) (DeviceSummary, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return DeviceSummary{}, fmt.Errorf("cannot begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var out DeviceSummary
	err = tx.QueryRow(ctx, `
		select id, kind, label from devices
		where id = $1 and account_id = $2 and kind = 'external'`,
		deviceID, accountID).Scan(&out.ID, &out.Kind, &out.Label)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeviceSummary{}, ErrNotYours
	}
	if err != nil {
		return DeviceSummary{}, fmt.Errorf("cannot find the device: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		update device_credentials
		set state = 'REVOKED', revoked_at = now(), updated_seq = next_seq('credentials')
		where device_id = $1 and state = 'ACTIVE'`, deviceID); err != nil {
		return DeviceSummary{}, fmt.Errorf("cannot revoke the old credential: %w", err)
	}

	credential := uuid.New()
	if _, err := tx.Exec(ctx, `
		insert into device_credentials (id, device_id, credential_uuid, updated_seq)
		values ($1, $2, $3, next_seq('credentials'))`,
		uuid.New(), deviceID, credential); err != nil {
		return DeviceSummary{}, fmt.Errorf("cannot create the new credential: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return DeviceSummary{}, fmt.Errorf("cannot commit: %w", err)
	}

	out.Credential = &credential
	out.State = "ACTIVE"
	return out, nil
}

// AccountBrief is one account, said without saying who it is.
type AccountBrief struct {
	// The first characters of the identifier, which is all an operator needs
	// to name one and less than a public log should carry.
	Prefix  string
	Tier    string
	Devices int

	// The day the account began, which is the day the free period counts
	// from.
	//
	// Here because the first argument about that period was conducted without
	// it: the application said a date, the Business Owner said it looked
	// wrong, and neither of us could see the number the date was computed
	// from. A setting whose effect depends on a value nobody can read is a
	// setting that gets argued about instead of checked.
	Created string
}

// prefixLength is how much of an identifier is enough to pick one out.
//
// Eight hexadecimal characters is four billion, which for a service with one
// account is absurd and for a service with a million is still comfortable: the
// chance that two of a million collide on eight characters is about one in
// eight. When it happens, the operation below refuses rather than guesses.
const prefixLength = 8

// Accounts lists what exists, with no address and no whole identifier.
//
// An operator needs to answer two questions before changing anything - how
// many accounts are there, and which tier is each on - and neither of them
// requires knowing who anybody is. This is what makes assigning a tier
// possible from a workflow whose log is public.
func (s *Store) Accounts(ctx context.Context) ([]AccountBrief, error) {
	rows, err := s.pool.Query(ctx, `
		select left(a.id::text, $1), a.tier,
		       (select count(*)::int from devices d where d.account_id = a.id),
		       to_char(a.created_at at time zone 'UTC', 'YYYY-MM-DD')
		from accounts a
		order by a.created_at`, prefixLength)
	if err != nil {
		return nil, fmt.Errorf("cannot list accounts: %w", err)
	}
	defer rows.Close()

	out := []AccountBrief{}
	for rows.Next() {
		var b AccountBrief
		if err := rows.Scan(&b.Prefix, &b.Tier, &b.Devices, &b.Created); err != nil {
			return nil, fmt.Errorf("cannot read an account: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ErrAmbiguousAccount means the prefix given matches more than one account.
var ErrAmbiguousAccount = errors.New("that prefix matches more than one account")

// SetAccountTierByPrefix moves an account between statuses, named by the start
// of its identifier.
//
// The point of the prefix is the log. Assigning a tier is an operator action,
// operator actions run through the pipeline, and the pipeline's log is public
// - so the handle has to be something that can be written there. An address
// cannot. A whole identifier could, and does not need to: it is useless
// without the database, but printing it costs something and buys nothing.
//
// Refused when the prefix matches more than one account. A tier granted to
// somebody who did not pay for it is not the kind of mistake that announces
// itself, so the ambiguous case has to fail rather than pick.
func (s *Store) SetAccountTierByPrefix(
	ctx context.Context, prefix, tier string,
) (AccountTier, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AccountTier{}, fmt.Errorf("cannot begin tier change: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		select id from accounts where id::text like $1 || '%' for update`, prefix)
	if err != nil {
		return AccountTier{}, fmt.Errorf("cannot find matching accounts: %w", err)
	}
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return AccountTier{}, fmt.Errorf("cannot read a matching account: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return AccountTier{}, fmt.Errorf("cannot finish matching accounts: %w", err)
	}

	switch len(ids) {
	case 0:
		return AccountTier{}, ErrNoSuchAccount
	case 1:
	default:
		return AccountTier{}, ErrAmbiguousAccount
	}

	out, err := setAccountTier(ctx, tx, ids[0], tier)
	if err != nil {
		return AccountTier{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AccountTier{}, fmt.Errorf("cannot commit tier change: %w", err)
	}
	return out, nil
}
