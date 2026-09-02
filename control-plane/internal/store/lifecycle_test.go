package store

import (
	"testing"
	"time"

	"download.simplevpn/control-plane/internal/document"
)

func working(now time.Time) NodeStanding {
	cpu, mem, latency, loss := 5.0, 30.0, 20.0, 0.0
	return NodeStanding{
		Node:              document.Node{Alias: "n-1"},
		Lifecycle:         "serving",
		LastSeen:          at(now, 30*time.Second),
		CPUPercent:        &cpu,
		MemoryPercent:     &mem,
		UpstreamLatencyMS: &latency,
		LossPercent:       &loss,
		DomainVerdict:     "works",
	}
}

// TestBrokenAndBlockedAreDifferent is the distinction this whole stage exists
// to make, and the one no single vantage point can see.
//
// A server that has fallen over and a server that works perfectly while being
// kept from the people who need it look identical from Helsinki: both are
// silence. Confusing them costs an afternoon spent restarting a healthy
// machine while the actual answer - raise a new domain - goes undone.
func TestBrokenAndBlockedAreDifferent(t *testing.T) {
	now := time.Date(2026, 8, 28, 14, 0, 0, 0, time.UTC)

	broken := working(now)
	broken.LastSeen = nil
	broken.DomainVerdict = "unreachable"

	blocked := working(now)
	blocked.DomainVerdict = "likely blocked" // reporting fine, devices cannot reach it

	if got := ConditionOf(broken, now); got != Faulty {
		t.Errorf("a server nobody can reach was called %q, want %q", got, Faulty)
	}
	if got := ConditionOf(blocked, now); got != Blocked {
		t.Errorf("a server that works and is being blocked was called %q, want %q", got, Blocked)
	}

	// And the difference has to reach the answers, not stop at the label. One
	// of these is replaced; the other is investigated.
	brokenAnswer := Standing{Lifecycle: "serving", Condition: Faulty, Since: now.Add(-time.Minute)}
	blockedAnswer := Standing{Lifecycle: "serving", Condition: Blocked, Since: now.Add(-time.Minute)}
	Decide(&brokenAnswer, now, 0)
	Decide(&blockedAnswer, now, 0)

	if brokenAnswer.NeedsReplacing {
		t.Error("a server that broke a minute ago was already marked for replacement")
	}
	if !blockedAnswer.NeedsReplacing {
		t.Error("a blocked domain was not marked for replacement; blocking does not heal")
	}
}

// TestBeingBuiltIsNotBeingBroken keeps half-finished machines off the list of
// problems and out of connection plans at the same time.
func TestBeingBuiltIsNotBeingBroken(t *testing.T) {
	now := time.Now()

	for _, lifecycle := range []string{"creating", "configuring", "awaiting-certificate", "verifying"} {
		st := Standing{Lifecycle: lifecycle, Condition: Unknown, Since: now}
		Decide(&st, now, 0)

		if st.MayHandOut {
			t.Errorf("%s: a machine still being built was offered to somebody", lifecycle)
		}
		// The important half: not a problem either. "May hand out" and "stop
		// handing out" are not opposites, and treating them as such puts every
		// new machine on somebody's list of faults.
		if st.StopHandingOut {
			t.Errorf("%s: a machine still being built was reported as needing to be withdrawn", lifecycle)
		}
		if st.State != lifecycle {
			t.Errorf("%s: shown as %q instead of where it is in its life", lifecycle, st.State)
		}
	}
}

func TestTheFourAnswers(t *testing.T) {
	now := time.Date(2026, 8, 28, 14, 0, 0, 0, time.UTC)
	long := now.Add(-6 * time.Hour)
	recent := now.Add(-time.Minute)

	cases := []struct {
		name      string
		standing  Standing
		idle      time.Duration
		hand      bool
		stop      bool
		replace   bool
		mayDelete bool
	}{
		{"working", Standing{Lifecycle: "serving", Condition: Healthy, Since: long}, 0,
			true, false, false, false},

		{"a spare", Standing{Lifecycle: "ready", Condition: Healthy, Since: long}, 0,
			true, false, false, false},

		// Worse than it should be is still better than nothing. Refusing every
		// degraded node during a bad hour would leave nowhere to send anybody.
		{"degraded", Standing{Lifecycle: "serving", Condition: Degraded, Since: long}, 0,
			true, false, false, false},

		{"blocked", Standing{Lifecycle: "serving", Condition: Blocked, Since: recent}, 0,
			false, true, true, false},

		{"broken just now", Standing{Lifecycle: "serving", Condition: Faulty, Since: recent}, 0,
			false, true, false, false},

		{"broken for hours", Standing{Lifecycle: "serving", Condition: Faulty, Since: long}, 0,
			false, true, true, false},

		{"draining, still carrying people", Standing{Lifecycle: "draining", Condition: Healthy, Since: long}, 0,
			false, false, false, false},

		{"draining and empty for hours", Standing{Lifecycle: "draining", Condition: Healthy, Since: long}, 3 * time.Hour,
			false, false, false, true},

		{"quarantined", Standing{Lifecycle: "quarantined", Condition: Healthy, Since: long}, 0,
			false, false, false, false},

		// Gone. Nothing about it is work: it is not replaced, because it has
		// already been dealt with, and it is not deleted, because there is
		// nothing left to delete.
		{"removed", Standing{Lifecycle: "removed", Condition: Unknown, Since: long}, 0,
			false, false, false, false},

		// The case the fleet actually had and the table did not. A removed node
		// keeps whatever condition it was last seen in, and nothing measures it
		// again, so "faulty for longer than an hour" stays true for ever. Twelve
		// of these sat under "needs replacing" and hid the number that meant
		// something. The old test used Unknown here, where the wrong branch
		// could not be reached.
		{"removed while broken", Standing{Lifecycle: "removed", Condition: Faulty, Since: long}, 0,
			false, false, false, false},

		{"removed while blocked", Standing{Lifecycle: "removed", Condition: Blocked, Since: long}, 0,
			false, false, false, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := c.standing
			Decide(&st, now, c.idle)

			if st.MayHandOut != c.hand {
				t.Errorf("may hand out: got %v, want %v", st.MayHandOut, c.hand)
			}
			if st.StopHandingOut != c.stop {
				t.Errorf("stop handing out: got %v, want %v", st.StopHandingOut, c.stop)
			}
			if st.NeedsReplacing != c.replace {
				t.Errorf("needs replacing: got %v, want %v", st.NeedsReplacing, c.replace)
			}
			if st.MayDelete != c.mayDelete {
				t.Errorf("may delete: got %v, want %v", st.MayDelete, c.mayDelete)
			}
			if st.Because == "" {
				t.Error("no reason given; the answers should be readable, not only correct")
			}
		})
	}
}

// TestHandingOutAndDeletingAreNeverBothTrue is a consistency check rather than
// a rule: whatever the answers are, they must not contradict each other.
func TestHandingOutAndDeletingAreNeverBothTrue(t *testing.T) {
	now := time.Now()
	lifecycles := []string{
		"creating", "configuring", "awaiting-certificate", "verifying",
		"ready", "serving", "draining", "quarantined", "removing", "removed",
	}
	conditions := []Condition{Healthy, Degraded, Blocked, Faulty, Unknown}

	for _, lifecycle := range lifecycles {
		for _, condition := range conditions {
			for _, idle := range []time.Duration{0, 5 * time.Hour} {
				st := Standing{Lifecycle: lifecycle, Condition: condition, Since: now.Add(-time.Hour)}
				Decide(&st, now, idle)

				if st.MayHandOut && st.MayDelete {
					t.Errorf("%s/%s: may be handed out and may be deleted at the same time", lifecycle, condition)
				}

				// Nothing that is gone is work. This is the invariant the
				// fleet report broke: twelve removed servers counted as
				// "needs replacing" because the verdict was computed from
				// condition alone, and a removed node keeps the condition it
				// was last seen in for ever.
				if st.Gone && (st.NeedsReplacing || st.MayDelete || st.MayHandOut || st.StopHandingOut) {
					t.Errorf("%s/%s: gone, yet still counted as something to do",
						lifecycle, condition)
				}
				if st.MayHandOut && st.StopHandingOut {
					t.Errorf("%s/%s: may be handed out and must stop being handed out", lifecycle, condition)
				}
			}
		}
	}
}

func TestLifecycleGovernsWhetherANodeIsOffered(t *testing.T) {
	now := time.Now()

	// A perfectly healthy machine that somebody has taken out of service stays
	// out of service. The measurements do not get a vote on intent.
	for _, lifecycle := range []string{"draining", "quarantined", "removing", "removed", "creating"} {
		node := working(now)
		node.Lifecycle = lifecycle
		if node.Usable(now) {
			t.Errorf("%s: a node taken out of service was still offered", lifecycle)
		}
	}

	for _, lifecycle := range []string{"ready", "serving"} {
		node := working(now)
		node.Lifecycle = lifecycle
		if !node.Usable(now) {
			t.Errorf("%s: a healthy node in service was not offered", lifecycle)
		}
	}
}

// TestADomainWithNoServerStillHasACondition covers the way in to the Control
// Plane, which is a domain with no node behind it.
//
// It read "nothing measured yet" on the panel while being checked every five
// minutes, because the condition was only ever derived from a node. That is
// the one domain whose blocking matters most: if devices cannot reach it they
// cannot be told where to go next, and every recovery path this service has
// runs through it.
func TestADomainWithNoServerStillHasACondition(t *testing.T) {
	cases := []struct {
		name    string
		verdict string
		want    Condition
	}{
		{"answering everybody", "works", Healthy},
		{"answering slowly", "slower", Degraded},
		{"we can reach it, devices cannot", "likely blocked", Blocked},
		{"nobody can reach it", "unreachable", Faulty},
		{"never checked", "", Unknown},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := conditionFromVerdict(c.verdict); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}

	// And the blocked case has to reach the answers: a blocked way in is
	// replaced, not restarted.
	now := time.Now()
	st := Standing{Kind: "domain", Lifecycle: "serving", Condition: Blocked, Since: now.Add(-time.Minute)}
	Decide(&st, now, 0)
	if !st.NeedsReplacing || st.MayHandOut {
		t.Errorf("a blocked way in: replace=%v, hand out=%v", st.NeedsReplacing, st.MayHandOut)
	}
}
