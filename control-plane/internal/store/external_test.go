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
