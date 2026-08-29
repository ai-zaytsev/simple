package store

import (
	"regexp"
	"testing"
)

// An absent purchases row must leave selling closed.
//
// The kill switch is required to be present, because its dangerous default is
// "off" and losing the row would turn it off silently. This setting is the
// mirror image: its dangerous default is "on", so a missing row has to leave
// the service declining to sell rather than selling because nobody said it
// may not.
func TestMissingPurchaseSettingsSellNothing(t *testing.T) {
	source := readSource(t, "state.go")

	loading := between(source, "func (s *Store) LoadServiceState", "\nfunc ")
	if loading == "" {
		t.Fatal("cannot find the service state loader")
	}

	// Not in the completeness check, deliberately. If it were, a missing row
	// would make the whole load fail - and every caller treats that as an
	// error, which is a harsher outcome than the safe default.
	if regexp.MustCompile(`seen\["purchases"\]\s*\|\||\|\|\s*!seen\["purchases"\]`).
		MatchString(loading) {
		t.Error("a missing purchases row fails the whole load rather than " +
			"leaving selling closed")
	}
	if !regexp.MustCompile(`case "purchases":`).MatchString(loading) {
		t.Error("the purchases row is never read")
	}
}

// The setting is stored as one value and replaced whole.
func TestPurchaseSettingsAreOneRow(t *testing.T) {
	source := readSource(t, "state.go")

	writing := between(source, "func (s *Store) SetPurchases", "\nfunc ")
	if writing == "" {
		t.Fatal("there is no way to change the purchase settings")
	}

	if !regexp.MustCompile(`on conflict \(key\) do update`).MatchString(writing) {
		t.Error("the setting cannot be changed after it is first written")
	}
	if !regexp.MustCompile(`changed_by`).MatchString(writing) {
		t.Error("nothing records who changed selling, which is the change " +
			"somebody will want to date later")
	}
}

// The default in the schema is closed.
func TestSellingStartsClosed(t *testing.T) {
	schema := allMigrations(t)

	if !regexp.MustCompile(`(?is)'purchases',\s*'\{"open":\s*false`).MatchString(schema) {
		t.Error("selling is open by default, on a service that cannot yet " +
			"take a payment")
	}
	if !regexp.MustCompile(`(?is)"free_days":\s*7`).MatchString(schema) {
		t.Error("the default free period is not seven days")
	}
}
