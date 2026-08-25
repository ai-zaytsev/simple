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
}

// IssueDeviceToken replaces this device's secret and returns the new one.
//
// Replaced rather than reused, so that a response lost on the way to the phone
// costs nothing: the next attempt issues another and the unheard one stops
// working. Only the hash is kept, so a copy of this table lets nobody connect.
func (s *Store) IssueDeviceToken(ctx context.Context, deviceID uuid.UUID, hash []byte) error {
	tag, err := s.pool.Exec(ctx,
		`update devices set token_hash = $2, last_seen_at = now() where id = $1`,
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
	err := s.pool.QueryRow(ctx, `
		select d.id, d.account_id
		from devices d
		where d.token_hash = $1 and d.account_id is not null`, hash).
		Scan(&device.ID, &device.AccountID)
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
}

func (s *Store) DevicesOfAccount(ctx context.Context, accountID uuid.UUID) ([]DeviceSummary, error) {
	rows, err := s.pool.Query(ctx, `
		select d.id,
		       c.credential_uuid,
		       coalesce(c.state, 'NONE'),
		       to_char(d.created_at, 'YYYY-MM-DD"T"HH24:MI:SSZ'),
		       to_char(coalesce(d.last_seen_at, d.created_at), 'YYYY-MM-DD"T"HH24:MI:SSZ')
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
		if err := rows.Scan(&d.ID, &d.Credential, &d.State, &d.CreatedAt, &d.LastSeenAt); err != nil {
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
