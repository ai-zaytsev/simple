package store

import (
	"regexp"
	"testing"
)

// Adding the same device twice must add it once.
//
// The application walks every way in until one answers - that is what makes it
// survive a blocked address - so a request that succeeded here and lost its
// answer on the way back is sent again. The first live use of this produced
// two devices with the same name from one press, and nothing on the screen
// could explain why.
//
// The name is the handle. Two rows sharing it are two rows nobody can revoke
// the right one of, which makes this a correctness problem and not tidiness.
func TestAddingTheSameDeviceTwiceAddsItOnce(t *testing.T) {
	source := readSource(t, "access.go")

	adding := between(source, "func (s *Store) AddExternalDevice", "\nfunc ")
	if adding == "" {
		t.Fatal("cannot find where an external device is added")
	}

	// Looked for before the insert, inside the same transaction that does it.
	// Outside the transaction, two requests arriving together would both find
	// nothing and both insert.
	if !regexp.MustCompile(`(?s)tx\.QueryRow.*d\.label = \$2.*insert into devices`).
		MatchString(adding) {
		t.Error("an existing device with the same name is not looked for " +
			"before inserting, so a retried request creates a second one")
	}
}
