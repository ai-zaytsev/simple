package api

import (
	"regexp"
	"testing"
)

// The decision is made here and sent, never left to the phone.
//
// That is the whole requirement of the stage: the wait and the switch change
// from the server without an APK update. A device applying its own copy of the
// rule would keep offering a purchase for exactly as long as it took somebody
// to install a new version - which for some people is never.
func TestTheOfferIsDecidedByTheService(t *testing.T) {
	source := readHandler(t, "access.go")

	listing := between(source, "func (s *Server) listDevices", "\nfunc ")
	if listing == "" {
		t.Fatal("cannot find the device listing")
	}

	if !regexp.MustCompile(`purchase\.Assess\(`).MatchString(listing) {
		t.Error("the offer is not assessed on the server")
	}
	if !regexp.MustCompile(`"purchase"`).MatchString(listing) {
		t.Error("the answer does not carry the offer, so the application " +
			"has nothing to draw but its own guess")
	}
}

// A failed read must close the offer, not open it.
//
// The one outcome worth refusing outright: a database hiccup that puts a buy
// button in front of somebody on a service that cannot take money.
func TestAFailedReadClosesTheOffer(t *testing.T) {
	source := readHandler(t, "access.go")
	listing := between(source, "func (s *Server) listDevices", "\nfunc ")

	if !regexp.MustCompile(`offer := purchase\.Offer\{Reason: purchase\.ReasonClosed\}`).
		MatchString(listing) {
		t.Error("the offer does not start closed, so a failed settings read " +
			"could leave it open")
	}
}

// Writing the settings replaces both or neither.
//
// Half a body would quietly keep the other half, and "I turned the wait down
// and nothing changed" is the failure this refuses to have.
func TestPurchaseSettingsAreWrittenWhole(t *testing.T) {
	source := readHandler(t, "purchases.go")

	if !regexp.MustCompile(`body\.Open == nil \|\| body\.FreeDays == nil`).
		MatchString(source) {
		t.Error("a half-given body is accepted, so one setting can be " +
			"changed while the other is silently kept")
	}
	if !regexp.MustCompile(`\*body\.FreeDays < 0`).MatchString(source) {
		t.Error("a negative wait is accepted")
	}
}

// The settings endpoint is an operator endpoint.
func TestOnlyAnOperatorChangesSelling(t *testing.T) {
	source := readHandler(t, "purchases.go")

	if !regexp.MustCompile(`if !s\.admin\(w, r\)`).MatchString(source) {
		t.Error("selling can be switched by anybody who can reach the service")
	}
}
