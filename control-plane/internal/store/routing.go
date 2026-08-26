package store

import (
	"context"
	"fmt"

	"download.simplevpn/control-plane/internal/document"
)

// LoadRouting reads the rules that decide where traffic goes.
//
// Sorted so that two clients asking a moment apart are told the same thing in
// the same order. Without it the lists would come back in whatever order the
// database happened to produce, every plan would differ from the last, and
// comparing two plans - the first thing anybody does when a route looks wrong -
// would be useless.
func (s *Store) LoadRouting(ctx context.Context) (document.Routing, error) {
	rows, err := s.pool.Query(ctx, `
		select kind, value from routing_rules
		where enabled
		order by kind, value`)
	if err != nil {
		return document.Routing{}, fmt.Errorf("cannot read routing rules: %w", err)
	}
	defer rows.Close()

	routing := document.Routing{
		Profile: "default-ru",
		// Every list is present and empty rather than absent, so a client that
		// finds no rules of a kind knows there are none rather than having to
		// decide what a missing field means.
		DirectApps:    []string{},
		DirectDomains: []string{},
		DirectIPs:     []string{},
		ProxyDomains:  []string{},
		ProxyIPs:      []string{},
		RussiaDirect:  true,
	}

	for rows.Next() {
		var kind, value string
		if err := rows.Scan(&kind, &value); err != nil {
			return document.Routing{}, fmt.Errorf("cannot read a routing rule: %w", err)
		}

		switch kind {
		case "direct_app":
			routing.DirectApps = append(routing.DirectApps, value)
		case "direct_domain":
			routing.DirectDomains = append(routing.DirectDomains, value)
		case "direct_ip":
			routing.DirectIPs = append(routing.DirectIPs, value)
		case "proxy_domain":
			routing.ProxyDomains = append(routing.ProxyDomains, value)
		case "proxy_ip":
			routing.ProxyIPs = append(routing.ProxyIPs, value)
		}
	}
	if err := rows.Err(); err != nil {
		return document.Routing{}, fmt.Errorf("cannot finish reading routing rules: %w", err)
	}

	return routing, nil
}
