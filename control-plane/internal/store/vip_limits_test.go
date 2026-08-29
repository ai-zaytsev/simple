package store

import (
	"regexp"
	"testing"
)

// VIP has no device limits at all.
//
// Said plainly by the Business Owner, in the same breath as the speed limit.
// Migrations 0014 and 0015 had left placeholders - one device, five external -
// and both said in their own comments that the number belonged to whoever
// sells the product. This is the test that the decision arrived.
//
// It is written against the schema rather than against a running database
// because that is where the policy lives: one statement, no deploy. A test of
// the Go code would pass while the rows said something else.
func TestVIPIsHeldToNothing(t *testing.T) {
	schema := allMigrations(t)

	if !regexp.MustCompile(
		`(?is)update\s+tier_limits\s+set\s+max_devices\s*=\s*null\s*,\s*max_external\s*=\s*null\s+where\s+tier\s*=\s*'VIP'`).
		MatchString(schema) {
		t.Error("VIP still carries a device limit; the stage says it has none")
	}
}

// Null has to be allowed to mean it.
//
// The column was created NOT NULL with a check demanding at least one device.
// Setting it to null against that schema does not produce an unlimited tier -
// it produces a migration that fails, and a service that will not start.
func TestNoLimitCanBeWrittenDown(t *testing.T) {
	schema := allMigrations(t)

	for _, column := range []string{"max_devices", "max_external"} {
		pattern := `(?is)alter\s+table\s+tier_limits\s+alter\s+column\s+` +
			column + `\s+drop\s+not\s+null`
		if !regexp.MustCompile(pattern).MatchString(schema) {
			t.Errorf("%s cannot hold null, so 'no limit' has no way to be said",
				column)
		}
	}

	// And the check that refused it is gone. Left in place, it would refuse
	// the null on the way past regardless of the column being nullable.
	if !regexp.MustCompile(
		`(?is)drop\s+constraint\s+if\s+exists\s+tier_limits_max_devices_check`).
		MatchString(schema) {
		t.Error("the check demanding at least one device is still there")
	}
}

// Zero and null are opposite answers, and the code must not read one as the
// other.
//
// Zero external devices is FREE's actual policy. If a null limit were read as
// zero, VIP - the tier that is supposed to have no ceiling - would be the one
// tier unable to add a television at all. The bug would present as the
// feature being broken exactly where it was paid for.
func TestUnlimitedIsNotReadAsNone(t *testing.T) {
	source := readSource(t, "access.go")

	adding := between(source, "func (s *Store) AddExternalDevice", "\nfunc ")
	if adding == "" {
		t.Fatal("cannot find the external device path")
	}

	if !regexp.MustCompile(`allowed\s*!=\s*nil\s*&&`).MatchString(adding) {
		t.Error("the external limit is compared without asking whether there " +
			"is one, so an unlimited tier is refused at zero")
	}

	eviction := between(source, "func evictBeyondLimit", "\nfunc ")
	if eviction == "" {
		t.Fatal("cannot find the eviction")
	}

	if !regexp.MustCompile(`limit\s*==\s*nil`).MatchString(eviction) {
		t.Error("eviction does not check for an absent limit; on a tier with " +
			"none it would subtract from nothing")
	}
}
