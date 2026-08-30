package store

import (
	"context"
	"encoding/json"
	"fmt"

	"download.simplevpn/control-plane/internal/appupdate"
	"download.simplevpn/control-plane/internal/purchase"
)

// ServiceState is what an operator can change while the service runs.
type ServiceState struct {
	KillSwitch             KillSwitch
	MinSupportedAppVersion int
	AppUpdates             appupdate.Policy
	Purchases              purchase.Settings
}

type KillSwitch struct {
	Enabled    bool   `json:"enabled"`
	MessageKey string `json:"message_key"`
}

// LoadServiceState reads the switches.
//
// A missing or unreadable row is an error rather than a default. Defaulting
// would mean that losing the row silently turns the kill switch off, and the
// one setting whose failure mode must never be "off by accident" is that one.
func (s *Store) LoadServiceState(ctx context.Context) (ServiceState, error) {
	rows, err := s.pool.Query(ctx, `select key, value from service_state`)
	if err != nil {
		return ServiceState{}, fmt.Errorf("cannot read service state: %w", err)
	}
	defer rows.Close()

	state := ServiceState{}
	seen := map[string]bool{}

	for rows.Next() {
		var (
			key string
			raw []byte
		)
		if err := rows.Scan(&key, &raw); err != nil {
			return ServiceState{}, fmt.Errorf("cannot read a setting: %w", err)
		}

		switch key {
		case "kill_switch":
			if err := json.Unmarshal(raw, &state.KillSwitch); err != nil {
				return ServiceState{}, fmt.Errorf("kill_switch is unreadable: %w", err)
			}
			seen[key] = true
		case "min_supported_app_version":
			if err := json.Unmarshal(raw, &state.MinSupportedAppVersion); err != nil {
				return ServiceState{}, fmt.Errorf("min_supported_app_version is unreadable: %w", err)
			}
			seen[key] = true
		case "app_updates":
			if err := json.Unmarshal(raw, &state.AppUpdates); err != nil {
				return ServiceState{}, fmt.Errorf("app_updates is unreadable: %w", err)
			}
			if err := state.AppUpdates.Validate(); err != nil {
				return ServiceState{}, fmt.Errorf("app_updates is invalid: %w", err)
			}
			seen[key] = true
		case "purchases":
			// Read into a local shape and copied across, rather than tagging
			// the purchase package's own struct with JSON names. The names in
			// this row belong to the database; the field names belong to the
			// decision. Tying them together would make renaming either one a
			// migration.
			var p struct {
				Open     bool `json:"open"`
				FreeDays int  `json:"free_days"`
			}
			if err := json.Unmarshal(raw, &p); err != nil {
				return ServiceState{}, fmt.Errorf("purchases is unreadable: %w", err)
			}
			state.Purchases = purchase.Settings{Open: p.Open, FreeDays: p.FreeDays}
			seen[key] = true
		}
	}
	if err := rows.Err(); err != nil {
		return ServiceState{}, fmt.Errorf("cannot finish reading service state: %w", err)
	}

	// Purchases is deliberately not required.
	//
	// An absent row leaves Open false, and false is the safe direction: the
	// service declines to sell rather than sells because nobody said it may
	// not. The kill switch is required for the mirror-image reason - its
	// dangerous default is off - and the asymmetry is the point rather than
	// an oversight.
	if !seen["kill_switch"] || !seen["min_supported_app_version"] || !seen["app_updates"] {
		return ServiceState{}, fmt.Errorf("service state is incomplete")
	}

	// The new policy is authoritative. The old row is kept and written with it
	// so a binary rollback still enforces the same minimum, while old clients
	// keep receiving the root field they already understand.
	state.MinSupportedAppVersion = state.AppUpdates.MinSupportedVersionCode
	return state, nil
}

// SetPurchases writes the two settings an operator may change about selling.
//
// One statement for both, because they are read as one row and a partial write
// would leave the pair inconsistent for as long as it took somebody to notice.
// The whole value is replaced rather than merged: merging a missing field
// silently keeps an old one, and "I turned the wait down and it did not
// change" is exactly the kind of failure this table exists to avoid.
func (s *Store) SetPurchases(ctx context.Context, p purchase.Settings, by string) error {
	value, err := json.Marshal(struct {
		Open     bool `json:"open"`
		FreeDays int  `json:"free_days"`
	}{Open: p.Open, FreeDays: p.FreeDays})
	if err != nil {
		return fmt.Errorf("cannot write the purchase settings: %w", err)
	}

	if _, err := s.pool.Exec(ctx, `
		insert into service_state (key, value, changed_by, updated_at)
		values ('purchases', $1::jsonb, $2, now())
		on conflict (key) do update
		set value = excluded.value,
		    changed_by = excluded.changed_by,
		    updated_at = now()`, value, by); err != nil {
		return fmt.Errorf("cannot save the purchase settings: %w", err)
	}
	return nil
}
