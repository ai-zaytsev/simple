package store

import (
	"regexp"
	"testing"
)

// A prefix that matches more than one account must be refused, not resolved.
//
// This exists because the handle is deliberately short. Assigning a tier runs
// through the pipeline, the pipeline's log is public, and an address cannot
// appear there - so the handle is the start of an identifier. Short handles
// collide, and the collision has to fail loudly: a tier granted to somebody
// who did not pay for it is not a mistake that announces itself, and the
// person who did pay would go on seeing FREE while somebody else got VIP.
func TestAnAmbiguousPrefixIsRefused(t *testing.T) {
	source := readSource(t, "access.go")

	setting := between(source, "func (s *Store) SetAccountTierByPrefix", "\nfunc ")
	if setting == "" {
		t.Fatal("there is no way to set a tier by prefix")
	}

	if !regexp.MustCompile(`ErrAmbiguousAccount`).MatchString(setting) {
		t.Error("more than one match is not refused, so a tier could land " +
			"on an account nobody chose")
	}

	// Counted before the update rather than after it. An update that hit two
	// rows has already hit them.
	counted := regexp.MustCompile(`(?s)select count\(\*\).*update accounts set tier`)
	if !counted.MatchString(setting) {
		t.Error("the matches are counted after the update, which is too late")
	}
}

// The listing must not carry an address.
//
// It is read into a public log. That is the whole reason this pair of
// operations exists rather than the ones that take an email, and it would be
// undone by one column.
func TestTheAccountListingSaysNothingAboutWho(t *testing.T) {
	source := readSource(t, "access.go")

	listing := between(source, "func (s *Store) Accounts", "\nfunc ")
	if listing == "" {
		t.Fatal("there is no account listing")
	}

	if regexp.MustCompile(`(?i)email`).MatchString(listing) {
		t.Error("the listing selects an address, which then reaches a public log")
	}
	if !regexp.MustCompile(`left\(a\.id::text`).MatchString(listing) {
		t.Error("the listing returns whole identifiers where a prefix would do")
	}
}
