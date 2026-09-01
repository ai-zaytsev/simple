package api

import (
	"regexp"
	"testing"
)

// What /healthz answers, pinned.
//
// Three workflows decide whether a deployment, a restore or a rollback
// succeeded by asking this endpoint. One of them waited for 204 - a number I
// wrote from memory and never checked - and reported a working service as a
// broken one after a live restore that had actually succeeded.
//
// The check is now on the body rather than the code, which cannot drift the
// same way. This holds the other end of that agreement: the handler says ok,
// and if somebody changes it the workflows do not silently start lying.
func TestHealthAnswersOK(t *testing.T) {
	source := readHandler(t, "api.go")

	health := between(source, "func (s *Server) health", "\nfunc ")
	if health == "" {
		t.Fatal("there is no health handler")
	}

	if !regexp.MustCompile(`http\.StatusOK`).MatchString(health) {
		t.Error("health no longer answers 200; three workflows read this " +
			"endpoint to decide whether a deploy or a restore worked")
	}
	if !regexp.MustCompile(`"status":\s*"ok"|"status": "ok"`).MatchString(health) {
		t.Error(`health no longer says "ok"; the workflows match on that word ` +
			"rather than on a status code, precisely so it cannot drift")
	}
}
