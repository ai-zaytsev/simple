package store

import (
	"context"
	"fmt"

	"download.simplevpn/control-plane/internal/document"
)

// LoadBootstrapEntries reads every way a client may reach this service.
//
// Ordered so that two clients asking a moment apart are told the same thing.
// The client shuffles within weights when it tries them; the order here is only
// so that comparing two descriptors - the first thing anybody does when
// recovery misbehaves - is possible at all.
func (s *Store) LoadBootstrapEntries(ctx context.Context) ([]document.BootstrapEntry, error) {
	rows, err := s.pool.Query(ctx, `
		select kind, host, port, server_name, path_prefix, weight
		from bootstrap_entries
		where enabled
		order by weight desc, kind, host`)
	if err != nil {
		return nil, fmt.Errorf("cannot read bootstrap entries: %w", err)
	}
	defer rows.Close()

	entries := []document.BootstrapEntry{}
	for rows.Next() {
		var e document.BootstrapEntry
		if err := rows.Scan(&e.Kind, &e.Host, &e.Port, &e.ServerName, &e.PathPrefix, &e.Weight); err != nil {
			return nil, fmt.Errorf("cannot read a bootstrap entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot finish reading bootstrap entries: %w", err)
	}

	return entries, nil
}
