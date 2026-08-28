package store

import (
	"testing"
	"time"
)

var nowForTest = time.Date(2026, 8, 28, 22, 0, 0, 0, time.UTC)

// A cover domain follows the node behind it, because the two usually fail
// together. Usually is the word doing the work here.
//
// A node stays healthy by talking to us over a connection it opens itself, and
// it goes on doing that while the domain in front of it answers nobody. That
// is not a hypothetical shape: it is what being blocked looks like from here,
// and telling it apart from a broken machine is the reason this lifecycle
// exists at all.
func TestACoverThatAnswersNobodyIsNotHealthy(t *testing.T) {
	cases := []struct {
		name string
		node Condition
		own  Condition
		want Condition
	}{
		// The case found on the live panel: a working node behind a domain
		// nothing could reach, and the domain still being handed out.
		{"healthy node, unreachable domain", Healthy, Faulty, Faulty},
		{"healthy node, blocked domain", Healthy, Blocked, Blocked},

		// The node still speaks when its own checks are quiet. A domain
		// nobody has probed lately is not a domain known to be bad.
		{"healthy node, nothing measured", Healthy, Unknown, Healthy},
		{"healthy node, healthy domain", Healthy, Healthy, Healthy},

		// A degraded domain is not enough to override: slow is a reason to
		// prefer another way in, not to stop offering this one, and the
		// chooser already scores on latency.
		{"healthy node, slow domain", Healthy, Degraded, Healthy},

		// The node's own trouble still travels, as it always did.
		{"faulty node, healthy domain", Faulty, Healthy, Faulty},
		{"blocked node, nothing measured", Blocked, Unknown, Blocked},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CoverCondition(c.node, c.own); got != c.want {
				t.Errorf("got %s, want %s", got, c.want)
			}
		})
	}
}

// TestACoverThatAnswersNobodyIsNotHandedOut is the consequence, stated where
// somebody reading the lifecycle will find it.
func TestACoverThatAnswersNobodyIsNotHandedOut(t *testing.T) {
	standing := Standing{
		Kind:      "domain",
		Name:      "cover.example",
		Lifecycle: "serving",
		Condition: CoverCondition(Healthy, Faulty),
	}
	Decide(&standing, nowForTest, 0)

	if standing.MayHandOut {
		t.Error("a domain nothing can reach is still being handed out")
	}
	if !standing.StopHandingOut {
		t.Error("a domain nothing can reach is not on the list to stop handing out")
	}
}

// recentOrElse decides which window a domain is judged on, and the whole point
// is that an empty hour is not the same as an empty day.
func TestAnEmptyHourIsNotAnUnmeasuredDomain(t *testing.T) {
	hour := EndpointHealth{FromUs: -1, FromDevices: -1, DeviceChecks: 0}
	day := EndpointHealth{FromUs: 0, FromDevices: -1, DeviceChecks: 0}

	// Twenty-four hours of failing and nothing in the last one. Treating that
	// as "never measured" would hand the domain out, because "nothing measured
	// yet" is a reason to offer something rather than to withhold it.
	if got := recentOrElse(hour, day); got.FromUs != 0 {
		t.Errorf("a day of failures was thrown away in favour of an empty hour: %+v", got)
	}
	if conditionFromVerdict(endpointVerdict(recentOrElse(hour, day))) != Faulty {
		t.Error("a domain failing every check for a day is not judged faulty")
	}

	// A recovering domain is judged on the hour, not held down by the day.
	recovered := EndpointHealth{FromUs: 100, FromDevices: -1, DeviceChecks: 0}
	if got := recentOrElse(recovered, day); got.FromUs != 100 {
		t.Errorf("a recovered domain is still being judged on yesterday: %+v", got)
	}

	// Devices reporting is enough to call the hour measured, even when our own
	// checks have nothing in it.
	deviceOnly := EndpointHealth{FromUs: -1, FromDevices: 100, DeviceChecks: 12}
	if got := recentOrElse(deviceOnly, day); got.DeviceChecks != 12 {
		t.Errorf("an hour with device checks was treated as empty: %+v", got)
	}
}
