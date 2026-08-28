package store

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A status belongs to an account, and the schema is what makes that true.
//
// The stage asked for it in the negative: not the phone, not the build, not a
// Google account, not a payment method. Three of those four the system has no
// concept of at all, so the one that could actually happen is the phone - a
// tier column on devices, added by somebody in a hurry, after which two
// devices of one person could disagree about what that person paid for.
//
// A comment would not stop that. A test that reads the migrations does.
func TestATierBelongsToAnAccountAndNotToADevice(t *testing.T) {
	schema := allMigrations(t)

	if !hasColumn(schema, "accounts", "tier") {
		t.Error("accounts has no tier; a status has nowhere to belong")
	}
	if hasColumn(schema, "devices", "tier") {
		t.Error("devices has a tier column; a status would belong to a phone, " +
			"and two phones of one person could then disagree")
	}
}

// Both words have to exist as rows, because the foreign key on accounts.tier
// means a word with no row is a status no account can hold.
func TestBothStatusesExist(t *testing.T) {
	schema := allMigrations(t)

	for _, tier := range []string{"FREE", "VIP"} {
		inserted := regexp.MustCompile(
			`(?is)insert\s+into\s+tier_limits.*?'` + tier + `'`)
		if !inserted.MatchString(schema) {
			t.Errorf("%s is never inserted into tier_limits, so no account can be on it", tier)
		}
	}
}

// allMigrations is every statement the schema is built from, as one string.
func allMigrations(t *testing.T) string {
	t.Helper()

	entries, err := os.ReadDir("migrations")
	if err != nil {
		t.Fatalf("cannot read migrations: %v", err)
	}

	var all strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		body, err := os.ReadFile(filepath.Join("migrations", entry.Name()))
		if err != nil {
			t.Fatalf("cannot read %s: %v", entry.Name(), err)
		}
		all.Write(body)
		all.WriteString("\n")
	}
	return all.String()
}

// hasColumn asks whether a table ever gains a column of that name, whether in
// its create statement or in a later alter.
func hasColumn(schema, table, column string) bool {
	altered := regexp.MustCompile(
		`(?is)alter\s+table\s+` + table + `\s+add\s+column(\s+if\s+not\s+exists)?\s+` + column + `\b`)
	if altered.MatchString(schema) {
		return true
	}

	created := regexp.MustCompile(
		`(?is)create\s+table(\s+if\s+not\s+exists)?\s+` + table + `\s*\((.*?)\n\);`)
	body := created.FindStringSubmatch(schema)
	if body == nil {
		return false
	}
	declared := regexp.MustCompile(`(?im)^\s*` + column + `\s`)
	return declared.MatchString(body[2])
}
