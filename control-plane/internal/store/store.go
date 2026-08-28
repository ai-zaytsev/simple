// Package store is the only place that talks to the database.
package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("cannot configure the database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database is not answering: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// NextSeq hands out the next number for a scope.
//
// The number comes from the database rather than from this process, and the
// reason is invariant I-2: a counter in memory would collide the moment a
// second instance existed, and clients would then reject valid configuration
// as an attempted rollback. It is transactional now, while there is one
// instance and it costs nothing, because retrofitting it later means a fleet
// of clients that have already recorded a number nobody can exceed.
func (s *Store) NextSeq(ctx context.Context, scope string) (int64, error) {
	var seq int64
	err := s.pool.QueryRow(ctx, `select next_seq($1)`, scope).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("cannot advance the sequence for %s: %w", scope, err)
	}
	return seq, nil
}

// Record keeps every issued document.
//
// This is what makes returning to a previous working configuration possible:
// a client refuses anything whose number moves backwards, so going back means
// issuing the older payload again under a new, higher number. Without the
// payload kept here there would be nothing to issue again.
func (s *Store) Record(ctx context.Context, kind, scope string, seq int64, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("cannot serialise the document for storage: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		insert into documents (kind, scope, seq, payload)
		values ($1, $2, $3, $4)`, kind, scope, seq, encoded)
	if err != nil {
		return fmt.Errorf("cannot record the issued document: %w", err)
	}
	return nil
}

// PreviousPayload returns the payload of an earlier document, for reissuing.
func (s *Store) PreviousPayload(ctx context.Context, kind, scope string, seq int64) ([]byte, error) {
	var payload []byte
	err := s.pool.QueryRow(ctx, `
		select payload from documents
		where kind = $1 and scope = $2 and seq = $3`, kind, scope, seq).Scan(&payload)
	if err != nil {
		return nil, fmt.Errorf("no %s document with sequence %d: %w", kind, seq, err)
	}
	return payload, nil
}

// TouchDevice records that a device asked for something, creating it and its
// account on first sight.
//
// Registration is implicit because the alternative - a separate call before
// the first plan - is one more thing that can fail between installing an
// application and it working.
func (s *Store) TouchDevice(ctx context.Context, deviceID, accountID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cannot begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		insert into accounts (id) values ($1)
		on conflict (id) do nothing`, accountID); err != nil {
		return fmt.Errorf("cannot record the account: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		insert into devices (id, account_id, last_seen_at)
		values ($1, $2, now())
		on conflict (id) do update set last_seen_at = now()`, deviceID, accountID); err != nil {
		return fmt.Errorf("cannot record the device: %w", err)
	}

	return tx.Commit(ctx)
}
