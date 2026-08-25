// Package signing turns a document into something a client will believe.
//
// The whole trust model of the product rests here: an application accepts a
// connection plan because it carries a signature from a key compiled into it,
// and for no other reason. Nothing in this package should ever gain an option
// that produces an unsigned or differently-signed envelope.
package signing

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// Envelope is what leaves the server. The payload travels encoded rather than
// as nested JSON so that the bytes a client verifies are exactly the bytes it
// parses: re-serialising a decoded document can reorder fields or change
// spacing, and the signature would then fail for a document nobody tampered
// with.
type Envelope struct {
	PayloadB64 string `json:"payload_b64"`
	Alg        string `json:"alg"`
	KeyID      string `json:"key_id"`
	SigB64     string `json:"sig_b64"`
}

const algorithm = "ed25519"

// Signer holds one key and the identifier clients use to select it.
type Signer struct {
	keyID string
	key   ed25519.PrivateKey
}

// NewSigner builds a signer from a base64 seed, which is how the key is
// carried in the pipeline's secret store.
//
// The seed is 32 bytes, not the 64-byte expanded form: a seed is what can be
// generated, written down once and never printed again, and expanding it is
// this function's job rather than the operator's.
func NewSigner(keyID, seedB64 string) (*Signer, error) {
	if keyID == "" {
		return nil, errors.New("key id is empty")
	}

	seed, err := base64.StdEncoding.DecodeString(seedB64)
	if err != nil {
		return nil, fmt.Errorf("seed for %s is not base64: %w", keyID, err)
	}
	if len(seed) != ed25519.SeedSize {
		// Deliberately does not say what was found beyond its length: a
		// message quoting key material is a message that leaks it.
		return nil, fmt.Errorf("seed for %s is %d bytes, expected %d",
			keyID, len(seed), ed25519.SeedSize)
	}

	return &Signer{keyID: keyID, key: ed25519.NewKeyFromSeed(seed)}, nil
}

// PublicKeyB64 is what belongs in the client, and is safe to print anywhere.
func (s *Signer) PublicKeyB64() string {
	pub := s.key.Public().(ed25519.PublicKey)
	return base64.StdEncoding.EncodeToString(pub)
}

// KeyID identifies which key signed a document.
func (s *Signer) KeyID() string { return s.keyID }

// Seal serialises a document, signs the exact bytes, and wraps both.
func (s *Signer) Seal(document any) (*Envelope, error) {
	payload, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("document cannot be serialised: %w", err)
	}

	signature := ed25519.Sign(s.key, payload)

	return &Envelope{
		PayloadB64: base64.StdEncoding.EncodeToString(payload),
		Alg:        algorithm,
		KeyID:      s.keyID,
		SigB64:     base64.StdEncoding.EncodeToString(signature),
	}, nil
}
