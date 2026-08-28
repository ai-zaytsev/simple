// Package auth turns an email address into a signed-in device.
//
// There is no password anywhere in this package, and no code for the user to
// copy. The only proof of identity is that somebody opened a link delivered to
// the address they claimed, which is the same proof a password reset gives -
// used directly instead of as a fallback.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
)

// TokenBytes is the length of the secret in a link.
//
// Thirty two bytes because the link is the entire credential: there is no
// second factor to fall back on, and a link short enough to guess is an account
// short enough to steal.
const TokenBytes = 32

// NewToken returns a link secret and the hash to store for it.
//
// Only the hash is ever written down. Somebody who reads the database finds
// hashes, and a hash cannot be pasted into a browser.
func NewToken() (token string, hash []byte, err error) {
	raw := make([]byte, TokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	// URL-safe and unpadded, because this string goes into a link that people
	// will see, copy and occasionally retype.
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, HashToken(token), nil
}

// HashToken is how a presented link is matched against a stored attempt.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// NormaliseEmail returns the form used for comparison.
//
// Case is folded and surrounding space removed, so that somebody who signs up
// with one spelling and returns with another reaches the same account instead
// of a second one nobody can find. Nothing more is done: stripping dots or
// plus-tags would merge addresses that some providers treat as distinct, and
// merging two people's accounts is worse than keeping one person's twice.
func NormaliseEmail(value string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return "", errors.New("address is empty")
	}

	at := strings.IndexByte(trimmed, '@')
	if at <= 0 || at == len(trimmed)-1 {
		return "", errors.New("address has no local part or no domain")
	}
	if strings.Contains(trimmed[at+1:], "@") {
		return "", errors.New("address has more than one at sign")
	}
	if !strings.Contains(trimmed[at+1:], ".") {
		return "", errors.New("domain has no dot")
	}
	if len(trimmed) > 254 {
		return "", errors.New("address is too long")
	}
	return trimmed, nil
}

// SameSecret compares two secrets without letting the comparison say how far
// it got.
//
// Hashed first so that the comparison is over a fixed length whatever the
// inputs were: a constant-time compare of two different lengths still tells
// the caller they were different lengths, which for a secret typed by a person
// is a start.
func SameSecret(offered, expected string) bool {
	if offered == "" || expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare(HashToken(offered), HashToken(expected)) == 1
}
