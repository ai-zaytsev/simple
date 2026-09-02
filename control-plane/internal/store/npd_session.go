package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"download.simplevpn/control-plane/internal/npd/lknpd"
)

// LoadSession reads the stored lknpd session. A missing row is not an error:
// it means we have never signed in.
//
// preferredDeviceID is the identifier that came with a configured refresh
// token. When it is set it wins over whatever is stored and over generating
// one, because a refresh token is issued for a device and is worthless beside
// any other identifier. Generating one is the last resort, for the password
// route where nobody handed us a pair.
func (s *Store) LoadSession(ctx context.Context, preferredDeviceID string) (lknpd.Session, error) {
	var (
		session   lknpd.Session
		inn       *string
		access    *string
		refresh   *string
		expiresAt *time.Time
	)
	err := s.pool.QueryRow(ctx, `
		select inn, device_id, access_token, refresh_token, expires_at
		from npd_session where id = true`).
		Scan(&inn, &session.DeviceID, &access, &refresh, &expiresAt)

	if errors.Is(err, pgx.ErrNoRows) {
		deviceID := preferredDeviceID
		if deviceID == "" {
			// Nobody handed us one, so make it once and keep it: it has to be
			// stable across restarts, since it is what binds a refresh token
			// to us.
			deviceID = uuid.NewString()
		}
		if _, err := s.pool.Exec(ctx, `
			insert into npd_session (id, device_id) values (true, $1)
			on conflict (id) do nothing`, deviceID); err != nil {
			return lknpd.Session{}, fmt.Errorf("cannot start an НПД session: %w", err)
		}
		return lknpd.Session{DeviceID: deviceID}, nil
	}
	if err != nil {
		return lknpd.Session{}, fmt.Errorf("cannot read the НПД session: %w", err)
	}

	if inn != nil {
		session.INN = *inn
	}
	if access != nil {
		session.AccessToken = *access
	}
	if refresh != nil {
		session.RefreshToken = *refresh
	}
	if expiresAt != nil {
		session.ExpiresAt = *expiresAt
	}

	// A configured device identifier that disagrees with the stored one means
	// the operator handed us a different pair. The stored tokens belonged to
	// the old device and cannot be used with the new identifier, so they are
	// dropped rather than carried forward into requests that would be refused.
	if preferredDeviceID != "" && session.DeviceID != preferredDeviceID {
		session.DeviceID = preferredDeviceID
		session.AccessToken = ""
		session.RefreshToken = ""
		session.ExpiresAt = time.Time{}
	}
	return session, nil
}
