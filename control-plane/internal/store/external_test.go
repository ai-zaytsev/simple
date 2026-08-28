package store

import (
	"regexp"
	"testing"
)

// Adding a television must not sign out the phone that added it.
//
// The device limit counted every device on the account. With a tier allowing
// one device, connecting a television would have evicted the phone in the same
// breath - the person's own action signing them out, which is the worst kind
// of defect because it looks like the feature working.
//
// Two limits counting two different things, and the eviction query has to say
// which one it is about.
func TestEvictionOnlyCountsTheApplication(t *testing.T) {
	source := readSource(t, "access.go")

	eviction := between(source,
		"func evictBeyondLimit", "\nfunc ")
	if eviction == "" {
		t.Fatal("cannot find the eviction")
	}

	if !regexp.MustCompile(`d\.kind\s*=\s*'app'`).MatchString(eviction) {
		t.Error("eviction counts every device, so a television and a phone " +
			"compete for the same slot and evict each other")
	}
}

// The two limits must stay two.
func TestExternalDevicesHaveTheirOwnAllowance(t *testing.T) {
	schema := allMigrations(t)

	if !regexp.MustCompile(`(?is)alter\s+table\s+tier_limits\s+add\s+column.*max_external`).
		MatchString(schema) {
		t.Error("there is no separate allowance for external devices")
	}

	// Nothing for FREE, which is what the stage says: the ability is VIP's.
	if !regexp.MustCompile(`(?is)update\s+tier_limits\s+set\s+max_external\s*=\s*0\s+where\s+tier\s*=\s*'FREE'`).
		MatchString(schema) {
		t.Error("FREE is not held at zero external devices")
	}
}

// VIP has no limits, and "no limit" has to be written as absence of a value.
//
// Zero is already taken and means the opposite: FREE has zero external
// devices, which is none at all. A single column cannot carry "none" and "no
// limit" as the same number - one reading locks a paying customer out, the
// other hands a free one everything.
func TestNoLimitIsAbsenceAndNotZero(t *testing.T) {
	schema := allMigrations(t)

	nulled := regexp.MustCompile(
		`(?is)update\s+tier_limits\s+set\s+max_devices\s*=\s*null\s*,\s*max_external\s*=\s*null\s+where\s+tier\s*=\s*'VIP'`)
	if !nulled.MatchString(schema) {
		t.Error("VIP still carries limits, or they are written as numbers")
	}

	if !regexp.MustCompile(`(?is)alter\s+column\s+max_devices\s+drop\s+not\s+null`).
		MatchString(schema) {
		t.Error("max_devices cannot hold the absence of a limit")
	}

	// FREE keeps its zero, which must go on meaning none rather than all.
	if !regexp.MustCompile(`(?is)update\s+tier_limits\s+set\s+max_external\s*=\s*0\s+where\s+tier\s*=\s*'FREE'`).
		MatchString(schema) {
		t.Error("FREE is no longer held at zero external devices")
	}
}

// The two places that read a limit must both treat absence as absence, not
// dereference it into a number.
func TestBothReadersSurviveNoLimit(t *testing.T) {
	source := readSource(t, "access.go")

	eviction := between(source, "func evictBeyondLimit", "\nfunc ")
	if !regexp.MustCompile(`limit\s*==\s*nil`).MatchString(eviction) {
		t.Error("eviction does not handle an account with no device limit")
	}

	adding := between(source, "func (s *Store) AddExternalDevice", "\nfunc ")
	if !regexp.MustCompile(`allowed\s*!=\s*nil`).MatchString(adding) {
		t.Error("adding an external device does not handle an account with no limit")
	}
}
