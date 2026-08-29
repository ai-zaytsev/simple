package store

import (
	"regexp"
	"testing"
)

// The old link stops working the moment the new one starts.
//
// The reason somebody asks for a new link is often that the old one is
// somewhere it should not be. Two working links would answer the request and
// not the problem - and it would look like it had worked, which is the part
// that makes it dangerous.
func TestReplacingALinkLeavesOnlyOne(t *testing.T) {
	source := readSource(t, "access.go")

	rotate := between(source, "func (s *Store) RotateExternalCredential", "\nfunc ")
	if rotate == "" {
		t.Fatal("there is no way to replace an external credential")
	}

	if !regexp.MustCompile(`(?s)state = 'REVOKED'.*insert into device_credentials`).
		MatchString(rotate) {
		t.Error("the old credential is not revoked before the new one is made")
	}

	// One transaction, so the two states never both exist and never both
	// fail to. A revoke that committed without its insert would leave a
	// television with no way back at all.
	if !regexp.MustCompile(`s\.pool\.Begin`).MatchString(rotate) {
		t.Error("the replacement is not done in one transaction")
	}
}

// An application installation must not be rotated this way.
//
// Its credential would be replaced while its token stayed valid, so the phone
// would go on asking for plans built on a credential no node accepts: signed
// in, connected, carrying nothing. External devices have no token, which is
// why the same operation is safe for them.
func TestOnlyAnExternalDeviceCanBeRotated(t *testing.T) {
	source := readSource(t, "access.go")
	rotate := between(source, "func (s *Store) RotateExternalCredential", "\nfunc ")

	if !regexp.MustCompile(`kind\s*=\s*'external'`).MatchString(rotate) {
		t.Error("an application installation could be rotated, which would " +
			"leave a phone that looks signed in and carries nothing")
	}
	if !regexp.MustCompile(`account_id\s*=\s*\$2`).MatchString(rotate) {
		t.Error("the device is not checked against the caller's account")
	}
}
