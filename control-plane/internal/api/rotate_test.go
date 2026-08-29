package api

import (
	"regexp"
	"testing"
)

// Replacing a link must not be a delete and an add.
//
// The stage asks for "a new link if the old one stopped working". Done by
// removing the device and creating another, the person loses the name they
// gave it and its place in their own list - they are asked to redo the part
// they got right in order to fix the part they did not. Worse, on a list of
// several televisions, the one that comes back is not obviously the one that
// went away.
func TestReplacingALinkKeepsTheDevice(t *testing.T) {
	source := readHandler(t, "external.go")

	rotate := between(source, "func (s *Server) rotateExternal", "\nfunc ")
	if rotate == "" {
		t.Fatal("there is no way to replace a link")
	}

	if regexp.MustCompile(`RevokeDevice|AddExternalDevice`).MatchString(rotate) {
		t.Error("replacing a link goes through revoke-and-add, which loses " +
			"the device's name and its place in the list")
	}
	if !regexp.MustCompile(`RotateExternalCredential`).MatchString(rotate) {
		t.Error("the credential is not being replaced in place")
	}
}

// The account comes from the token, never from the request.
//
// A caller naming somebody else's device must fail because it is not theirs,
// not because they guessed an identifier that does not exist. The difference
// matters when the identifier does exist.
func TestReplacingALinkCannotReachAnotherAccount(t *testing.T) {
	source := readHandler(t, "external.go")
	rotate := between(source, "func (s *Server) rotateExternal", "\nfunc ")

	if !regexp.MustCompile(`caller\.AccountID`).MatchString(rotate) {
		t.Error("the account is not taken from the proven token")
	}
	if regexp.MustCompile(`body\.AccountID|body\.Account`).MatchString(rotate) {
		t.Error("the account is being read from the request body")
	}
}

// What the application is told about itself.
//
// The screen for routers and televisions is VIP's, and an application that
// cannot tell which account it is on has two bad options: show the section to
// everybody and refuse on use, which teaches people the product is broken, or
// infer the tier from a failure, which is worse.
func TestTheApplicationIsToldItsTier(t *testing.T) {
	source := readHandler(t, "access.go")
	listing := between(source, "func (s *Server) listDevices", "\nfunc ")
	if listing == "" {
		t.Fatal("cannot find the device listing")
	}

	if !regexp.MustCompile(`"tier"`).MatchString(listing) {
		t.Error("the application is never told which tier it is on")
	}

	// By account, not by address. The caller proved a device token; it has not
	// named a mailbox and must not be handed one back.
	if regexp.MustCompile(`AccountTierByEmail|"email"`).MatchString(listing) {
		t.Error("the tier is being looked up by address on a call that " +
			"authenticated with a device token")
	}
}
