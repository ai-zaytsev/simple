package api

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The status a plan carries must come from the account, never from this file.
//
// It was a literal for several stages and nobody noticed, because with one
// tier in existence the literal and the truth were the same string. An
// ordinary test would have been green for the same reason: it would have
// asserted that a plan says FREE, and a plan did say FREE.
//
// So this reads the source instead. Crude, and it catches exactly the mistake
// that was actually made - which is more than a test of the value would have
// done.
func TestThePlanDoesNotInventATier(t *testing.T) {
	source, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("cannot read the handler: %v", err)
	}

	assignment := regexp.MustCompile(`AccountTier:\s*"`)
	if assignment.Match(source) {
		t.Error("AccountTier is being assigned a literal; " +
			"a status belongs to the account and must be read from it")
	}

	if !strings.Contains(string(source), "AccountTier: device.Tier") {
		t.Error("AccountTier is not being filled from the device's account")
	}
}

// The handler that changes a status must not write an address anywhere it
// could be read later.
//
// The address is the handle an operator uses and the one thing in this
// exchange that identifies a person. It is used to find a row and must not
// travel any further than that.
func TestChangingATierDoesNotLogAnAddress(t *testing.T) {
	source, err := os.ReadFile("tier.go")
	if err != nil {
		t.Fatalf("cannot read the handler: %v", err)
	}

	for _, line := range strings.Split(string(source), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		if !strings.Contains(trimmed, "s.log.") {
			continue
		}
		if strings.Contains(trimmed, "email") || strings.Contains(trimmed, "Email") {
			t.Errorf("an address is being written to the log: %s", trimmed)
		}
	}

	// The answer must not echo it either: a caller that saves the response
	// would be saving a mailbox next to an account identifier.
	answer := string(source)
	start := strings.Index(answer, "writeJSON(w, http.StatusOK")
	if start < 0 {
		t.Fatal("cannot find the answer this handler sends")
	}
	if strings.Contains(answer[start:], "email") {
		t.Error("the answer echoes the address it was given")
	}
}
