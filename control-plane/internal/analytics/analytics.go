// Package analytics turns an account into a key that measurement can use and
// nothing else can reverse.
package analytics

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Deriver produces the only user key that analytics is allowed to hold.
//
// The rule from docs/architecture/identity-model.md is that no measurement
// carries an address, an account, a device, or a VPN credential. A key derived
// here satisfies that and still allows everything the measurement is for -
// heavy and light users, session lengths, reconnect rates - because within one
// epoch the same account always produces the same key.
//
// Between epochs it does not. That is the point rather than a shortcoming:
// once an epoch key is gone, nobody holding both databases can join a person's
// behaviour across the boundary, and nobody can be asked to.
type Deriver struct {
	secret     []byte
	epochHours int
}

func NewDeriver(secretHex string, epoch time.Duration) (*Deriver, error) {
	secret, err := hex.DecodeString(secretHex)
	if err != nil {
		return nil, fmt.Errorf("analytics key is not hex: %w", err)
	}
	if len(secret) < 32 {
		return nil, errors.New("analytics key must be at least 32 bytes")
	}
	if epoch < time.Hour {
		return nil, errors.New("an epoch shorter than an hour measures nothing")
	}
	return &Deriver{secret: secret, epochHours: int(epoch.Hours())}, nil
}

// ID is HMAC(epoch key, account), truncated to 16 bytes.
//
// Truncated because 128 bits is beyond any collision that matters at this
// scale, and a shorter key is less tempting to treat as an identifier for
// something it is not.
func (d *Deriver) ID(accountID uuid.UUID, at time.Time) string {
	epoch := at.UTC().Unix() / int64(d.epochHours*3600)

	mac := hmac.New(sha256.New, d.secret)
	fmt.Fprintf(mac, "%d:", epoch)
	mac.Write(accountID[:])
	return hex.EncodeToString(mac.Sum(nil)[:16])
}
