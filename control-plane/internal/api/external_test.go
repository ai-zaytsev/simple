package api

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// A link is built for an external device and for nothing else.
//
// The requirement is stated in the negative and belongs to the tier
// arrangement: connecting a router, a television or a third-party client is
// something only VIP may do, so a FREE account must never come into
// possession of a link. It cannot, because it may own no external device -
// but that is three facts in a chain, and this is the explicit one at the end.
func TestOnlyAnExternalDeviceHasALink(t *testing.T) {
	source := readHandler(t, "external.go")

	building := between(source, "func (s *Server) linksFor", "\nfunc ")
	if building == "" {
		t.Fatal("cannot find where links are built")
	}
	if !regexp.MustCompile(`device\.Kind\s*!=\s*"external"`).MatchString(building) {
		t.Error("a link can be built for a device that is not external, " +
			"which is how a FREE account would come to hold one")
	}

	// And the listing must not offer them for anything else either. Belt and
	// braces on purpose: this is the endpoint a person calls, and the refusal
	// above only stops a mistake rather than announcing one.
	listing := between(source, "func (s *Server) externalLinks", "\nfunc ")
	if !strings.Contains(listing, `d.Kind != "external"`) {
		t.Error("the listing does not restrict itself to external devices")
	}
}

// Adding one must go through the allowance, which is zero for FREE.
func TestAddingAnExternalDeviceGoesThroughTheAllowance(t *testing.T) {
	source := readHandler(t, "external.go")

	adding := between(source, "func (s *Server) addExternal", "\nfunc ")
	if !strings.Contains(adding, "AddExternalDevice") {
		t.Error("adding does not go through the store, so it does not go " +
			"through the allowance either")
	}
	if !strings.Contains(adding, "ErrTooManyExternal") {
		t.Error("a refused allowance is not handled, so a tier with none " +
			"would get an error page instead of a refusal")
	}
}

func readHandler(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("cannot read %s: %v", name, err)
	}
	return string(body)
}

// between returns the text from the first marker to the next occurrence of the
// second one after it, or empty when the first is absent.
func between(source, from, to string) string {
	start := strings.Index(source, from)
	if start < 0 {
		return ""
	}
	rest := source[start+len(from):]
	end := strings.Index(rest, to)
	if end < 0 {
		return rest
	}
	return rest[:end]
}
