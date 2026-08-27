package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// IssuesThisWeek counts how many certificates were granted for a name.
//
// Against the authority's limit of five for the same set of names in seven
// days. Hitting it means a node cannot renew for days, and a limit reached by
// accident is indistinguishable from one reached by an attack - neither can be
// argued with once it is hit.
func (s *Store) IssuesThisWeek(ctx context.Context, name string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		select count(*) from certificate_issues
		where name = lower($1) and not refused and issued_at > now() - interval '7 days'`,
		name).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("cannot count issues for %s: %w", name, err)
	}
	return count, nil
}

// RecordIssue writes down that a certificate was granted.
func (s *Store) RecordIssue(ctx context.Context, name, nodeAlias string, expires time.Time) error {
	_, err := s.pool.Exec(ctx, `
		insert into certificate_issues (id, name, node_alias, expires_at)
		values ($1, lower($2), $3, $4)`,
		uuid.New(), name, nodeAlias, expires)
	if err != nil {
		return fmt.Errorf("cannot record the issue: %w", err)
	}
	return nil
}

// RecordRefusal writes down that one was not granted, and why.
//
// Kept alongside the successes because a node that keeps being told no is a
// node nobody would otherwise notice until its certificate expired.
func (s *Store) RecordRefusal(ctx context.Context, name, nodeAlias, reason string) {
	_, err := s.pool.Exec(ctx, `
		insert into certificate_issues (id, name, node_alias, refused, reason)
		values ($1, lower($2), $3, true, $4)`,
		uuid.New(), name, nodeAlias, reason)
	if err != nil {
		// Not worth failing the request over: the refusal itself has already
		// been decided, and losing the note is smaller than losing the answer.
		return
	}
}

// CertificateName is the domain a node is entitled to ask for.
//
// Taken from what the node was configured with rather than from what it says
// it wants. A node that could name its own domain could ask us to prove
// ownership of any domain we hold and hand it the result.
func (s *Store) CertificateName(ctx context.Context, alias string) (string, error) {
	var name string
	err := s.pool.QueryRow(ctx, `
		select coalesce(params->>'server_name', '')
		from nodes where alias = $1`, alias).Scan(&name)
	if err != nil {
		return "", fmt.Errorf("cannot read the node: %w", err)
	}
	if name == "" {
		return "", fmt.Errorf("node %s has no domain", alias)
	}
	return name, nil
}
