package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"download.simplevpn/control-plane/internal/appupdate"
)

var (
	ErrUpdateRollback      = errors.New("application version cannot move backwards")
	ErrMinimumAboveLatest  = errors.New("minimum supported version exceeds latest")
	ErrReleaseDisagreement = errors.New("version is already published with different material")
)

type AppRelease struct {
	VersionCode int
	VersionName string
	Channel     string
	Artifact    appupdate.Artifact
}

// PublishAppRelease advances latest after an immutable public artifact has
// been verified. It never changes the minimum: publishing is an offer, not a
// fleet-wide command to stop.
func (s *Store) PublishAppRelease(
	ctx context.Context,
	release AppRelease,
	by string,
) (appupdate.Policy, error) {
	if release.VersionCode < 1 || strings.TrimSpace(release.VersionName) == "" {
		return appupdate.Policy{}, errors.New("release identity is incomplete")
	}
	if strings.TrimSpace(release.Channel) == "" {
		return appupdate.Policy{}, errors.New("release channel is empty")
	}
	if release.Channel == appupdate.DirectAPK {
		if err := release.Artifact.Validate(); err != nil {
			return appupdate.Policy{}, fmt.Errorf("release artifact is invalid: %w", err)
		}
	} else if release.Artifact != (appupdate.Artifact{}) {
		return appupdate.Policy{}, errors.New("unknown channel cannot publish direct APK material")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return appupdate.Policy{}, fmt.Errorf("cannot begin app release: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	policy, err := updatesForChange(ctx, tx)
	if err != nil {
		return appupdate.Policy{}, err
	}
	switch {
	case release.VersionCode < policy.LatestVersionCode:
		return appupdate.Policy{}, ErrUpdateRollback
	case release.VersionCode == policy.LatestVersionCode:
		current, ok := policy.Channels[release.Channel]
		if policy.LatestVersionName != release.VersionName || (ok && current != release.Artifact) {
			return appupdate.Policy{}, ErrReleaseDisagreement
		}
		if ok {
			// Exactly the same publication is a safe retry.
			return policy, tx.Commit(ctx)
		}
		// The migration knows the first build's identity before its bytes are
		// published. The first official publication attaches that missing
		// channel material without pretending a new version exists.
		if policy.Channels == nil {
			policy.Channels = map[string]appupdate.Artifact{}
		}
		policy.Channels[release.Channel] = release.Artifact
		if err := saveUpdates(ctx, tx, policy, changedBy(by)); err != nil {
			return appupdate.Policy{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return appupdate.Policy{}, fmt.Errorf("cannot commit first app artifact: %w", err)
		}
		return policy, nil
	}

	policy.LatestVersionCode = release.VersionCode
	policy.LatestVersionName = strings.TrimSpace(release.VersionName)
	// Channel material belongs to one exact global latest version. Keeping an
	// artifact from the previous version would make that channel offer old
	// bytes while the policy describes the new version. Other channels attach
	// to this release through the equal-version branch above.
	policy.Channels = map[string]appupdate.Artifact{release.Channel: release.Artifact}
	if err := policy.Validate(); err != nil {
		return appupdate.Policy{}, fmt.Errorf("release would make policy invalid: %w", err)
	}
	if err := saveUpdates(ctx, tx, policy, changedBy(by)); err != nil {
		return appupdate.Policy{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return appupdate.Policy{}, fmt.Errorf("cannot commit app release: %w", err)
	}
	return policy, nil
}

// SetMinSupportedAppVersion changes the server stop line atomically in both
// the new policy and the legacy row used by a rolled-back Core binary.
func (s *Store) SetMinSupportedAppVersion(
	ctx context.Context,
	minimum int,
	by string,
) (appupdate.Policy, error) {
	if minimum < 1 {
		return appupdate.Policy{}, errors.New("minimum must be positive")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return appupdate.Policy{}, fmt.Errorf("cannot begin minimum update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	policy, err := updatesForChange(ctx, tx)
	if err != nil {
		return appupdate.Policy{}, err
	}
	if minimum > policy.LatestVersionCode {
		return appupdate.Policy{}, ErrMinimumAboveLatest
	}
	policy.MinSupportedVersionCode = minimum
	by = changedBy(by)
	if err := saveUpdates(ctx, tx, policy, by); err != nil {
		return appupdate.Policy{}, err
	}
	if _, err := tx.Exec(ctx, `
		update service_state
		set value = to_jsonb($1::int), changed_by = $2, updated_at = now()
		where key = 'min_supported_app_version'`, minimum, by); err != nil {
		return appupdate.Policy{}, fmt.Errorf("cannot keep legacy minimum in sync: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return appupdate.Policy{}, fmt.Errorf("cannot commit minimum update: %w", err)
	}
	return policy, nil
}

func updatesForChange(ctx context.Context, tx pgx.Tx) (appupdate.Policy, error) {
	var raw []byte
	if err := tx.QueryRow(ctx, `
		select value from service_state where key = 'app_updates' for update`).Scan(&raw); err != nil {
		return appupdate.Policy{}, fmt.Errorf("cannot lock app update policy: %w", err)
	}
	var policy appupdate.Policy
	if err := json.Unmarshal(raw, &policy); err != nil {
		return appupdate.Policy{}, fmt.Errorf("cannot read app update policy: %w", err)
	}
	if err := policy.Validate(); err != nil {
		return appupdate.Policy{}, fmt.Errorf("app update policy is invalid: %w", err)
	}
	return policy, nil
}

func saveUpdates(ctx context.Context, tx pgx.Tx, policy appupdate.Policy, by string) error {
	raw, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("cannot encode app update policy: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		update service_state
		set value = $1::jsonb, changed_by = $2, updated_at = now()
		where key = 'app_updates'`, raw, by); err != nil {
		return fmt.Errorf("cannot save app update policy: %w", err)
	}
	return nil
}

func changedBy(value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return "operator"
}
