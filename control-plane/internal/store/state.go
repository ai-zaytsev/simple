package store

import (
	"context"
	"encoding/json"
	"fmt"
)

// ServiceState is what an operator can change while the service runs.
type ServiceState struct {
	KillSwitch             KillSwitch
	MinSupportedAppVersion int
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
		}
	}
	if err := rows.Err(); err != nil {
		return ServiceState{}, fmt.Errorf("cannot finish reading service state: %w", err)
	}

	if !seen["kill_switch"] || !seen["min_supported_app_version"] {
		return ServiceState{}, fmt.Errorf("service state is incomplete")
	}
	return state, nil
}
