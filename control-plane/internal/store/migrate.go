package store

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strings"
)

// The schema travels inside the binary.
//
// It used to be a folder of files someone applied by hand before a deploy,
// which meant the running code and the schema it needs were two things that
// had to be kept in step by memory. They are now one artefact: a binary that
// starts is a binary whose schema is already there, and moving the Control
// Plane to another machine no longer has a step that can be forgotten.
//
//go:embed migrations/*.sql
var migrations embed.FS

// migrationLock is an arbitrary constant, and only has to be the same in every
// instance. It stops two starting at once from both migrating.
const migrationLock = 8_274_119

// Migrate brings the database up to what this binary expects.
//
// Applied migrations are recorded, so a file runs once and never again. That
// record is what lets a later migration do something that cannot be repeated -
// backfill a column, drop a default - without the author having to write it in
// a way that survives being run twice.
func (s *Store) Migrate(ctx context.Context) ([]string, error) {
	files, err := migrations.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("cannot read the embedded schema: %w", err)
	}

	names := make([]string, 0, len(files))
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".sql") {
			names = append(names, file.Name())
		}
	}
	// Ordered by name, which is why they are numbered.
	sort.Strings(names)

	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot reach the database: %w", err)
	}
	defer conn.Release()

	// Held on this one connection for the whole run, and released with it.
	if _, err := conn.Exec(ctx, `select pg_advisory_lock($1)`, migrationLock); err != nil {
		return nil, fmt.Errorf("cannot take the migration lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx), `select pg_advisory_unlock($1)`, migrationLock)
	}()

	if _, err := conn.Exec(ctx, `
		create table if not exists schema_migrations (
			name       text primary key,
			applied_at timestamptz not null default now()
		)`); err != nil {
		return nil, fmt.Errorf("cannot record migrations: %w", err)
	}

	applied := make([]string, 0, len(names))
	for _, name := range names {
		var done bool
		if err := conn.QueryRow(ctx,
			`select exists (select 1 from schema_migrations where name = $1)`, name,
		).Scan(&done); err != nil {
			return nil, fmt.Errorf("cannot check migration %s: %w", name, err)
		}
		if done {
			continue
		}

		body, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			return nil, fmt.Errorf("cannot read migration %s: %w", name, err)
		}

		// One transaction per file, so a migration that fails half way leaves
		// the schema as it was rather than in a state no file describes.
		tx, err := conn.Begin(ctx)
		if err != nil {
			return nil, fmt.Errorf("cannot begin migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, string(body)); err != nil {
			_ = tx.Rollback(ctx)
			return nil, fmt.Errorf("migration %s failed: %w", name, err)
		}
		if _, err := tx.Exec(ctx,
			`insert into schema_migrations (name) values ($1)`, name,
		); err != nil {
			_ = tx.Rollback(ctx)
			return nil, fmt.Errorf("cannot record migration %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("cannot commit migration %s: %w", name, err)
		}

		applied = append(applied, name)
	}

	return applied, nil
}
