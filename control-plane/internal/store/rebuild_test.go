package store

import (
	"os"
	"regexp"
	"testing"
)

// A measurement must not outlive the machine it was about.
//
// A domain outlives the node behind it: the name is kept and pointed at a new
// address. Every check made while the old machine was being destroyed stays in
// the table, and those checks failed.
//
// What that produced was not a wrong number on a screen. Devices could not
// reach the domains during a rebuild while the Control Plane could, which is
// precisely the shape of being blocked in Russia. The service concluded that
// both freshly built nodes were blocked and refused to hand out either - it
// locked itself out of its own fleet, in the same hour those nodes were built.
//
// And it would not have recovered on its own. A device with no plan checks
// nothing, and the judgement is waiting for checks.
func TestAJudgementIgnoresChecksFromBeforeTheMachine(t *testing.T) {
	lifecycle := readStoreSource(t, "lifecycle.go")

	conditions := between(lifecycle, "func (s *Store) domainConditions", "\nfunc ")
	if conditions == "" {
		t.Fatal("cannot find how a domain's condition is decided")
	}
	if !regexp.MustCompile(`p\.at\s*>=?\s*d\.since`).MatchString(conditions) {
		t.Error("a domain is judged on checks that predate what it now serves; " +
			"a rebuilt node inherits its predecessor's failures and is read as blocked")
	}

	choose := readStoreSource(t, "choose.go")
	standings := between(choose, "func (s *Store) NodeStandings", "\nfunc ")
	if standings == "" {
		t.Fatal("cannot find how a node is scored")
	}
	if !regexp.MustCompile(`p\.at\s*>=?\s*n\.state_since`).MatchString(standings) {
		t.Error("a node is scored on checks made before it existed")
	}
}

func readStoreSource(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("cannot read %s: %v", name, err)
	}
	return string(body)
}
