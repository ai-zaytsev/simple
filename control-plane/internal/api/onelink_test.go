package api

import (
	"regexp"
	"testing"
)

// One device, one link.
//
// The first version built a link for every node in the fleet, reasoning that a
// client holding several survives one of them being retired. The Business
// Owner refused it and the arithmetic is the argument: with a hundred nodes
// that is a hundred links for one television, and nobody pastes a hundred of
// anything. A second connection is a second device with a name.
func TestOneDeviceGetsOneLink(t *testing.T) {
	source := readHandler(t, "external.go")

	building := between(source, "func (s *Server) linkFor", "\nfunc ")
	if building == "" {
		t.Fatal("cannot find where the link is built")
	}

	// Returned from inside the loop, which is what makes it one rather than
	// all of them. A version that collected and returned a slice would pass
	// every other check in this file.
	if !regexp.MustCompile(`(?s)for _, standing := range.*return built, nil`).
		MatchString(building) {
		t.Error("the link is not taken from the first usable node; this is " +
			"how a fleet of a hundred nodes hands out a hundred links")
	}
	if regexp.MustCompile(`links\s*=\s*append`).MatchString(building) {
		t.Error("links are being collected into a list again")
	}
}

// The node is drawn, not taken in whatever order the database returned.
//
// Taking the first row would put every external device in the fleet on one
// node, which is the load problem the ranking exists to avoid - and it would
// make "new link" hand back the same node it just replaced.
func TestTheNodeIsDrawnFromTheCredential(t *testing.T) {
	source := readHandler(t, "external.go")
	building := between(source, "func (s *Server) linkFor", "\nfunc ")

	if !regexp.MustCompile(`store\.Rank\(standings, device\.Credential\.String\(\)`).
		MatchString(building) {
		t.Error("the node is not drawn by the ranking from the credential, " +
			"so a replaced link can come back on the same node")
	}
}
