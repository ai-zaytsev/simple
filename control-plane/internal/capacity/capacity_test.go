package capacity

import (
	"strings"
	"testing"
)

// comfortable is a fleet with room: two nodes, one of them a spare, a quiet
// week and domains in hand.
func comfortable() Reading {
	return Reading{
		NodesUsable:   2,
		NodesSpare:    1,
		SessionsNow:   120,
		CapacityTotal: 1000,
		PeakToday:     200,
		PeakWeek:      300,
		DomainsSpare:  2,
	}
}

func TestTheFourStates(t *testing.T) {
	growingFast, growingSome := 0.8, 0.3

	cases := []struct {
		name string
		make func(Reading) Reading
		want State
	}{
		{"room to spare", func(r Reading) Reading { return r }, Normal},

		{"the week's peak took two thirds", func(r Reading) Reading {
			r.PeakWeek = 650
			return r
		}, Watch},

		{"the week's peak took four fifths", func(r Reading) Reading {
			r.PeakWeek = 850
			return r
		}, ScaleRequired},

		{"the week's peak took nearly everything", func(r Reading) Reading {
			r.PeakWeek = 960
			return r
		}, Critical},

		// A fleet with no spare is a fleet where one machine failing is an
		// outage, however much room the rest of it has.
		{"no spare, plenty of room", func(r Reading) Reading {
			r.NodesSpare = 0
			return r
		}, ScaleRequired},

		{"nothing can be handed out", func(r Reading) Reading {
			r.NodesUsable = 0
			r.NodesSpare = 0
			return r
		}, Critical},

		// The slow part of adding a node is not the node.
		{"one domain left", func(r Reading) Reading {
			r.DomainsSpare = 1
			return r
		}, Watch},

		{"no domain left", func(r Reading) Reading {
			r.DomainsSpare = 0
			return r
		}, ScaleRequired},

		{"a blocked node is replaced, not repaired", func(r Reading) Reading {
			r.NodesBlocked = 1
			return r
		}, ScaleRequired},

		{"a broken node is looked at", func(r Reading) Reading {
			r.NodesFaulty = 1
			return r
		}, Watch},

		// Growth on a nearly empty fleet is not an emergency. Doubling from
		// forty connections to eighty says nothing about capacity.
		{"fast growth on an empty fleet", func(r Reading) Reading {
			r.PeakWeek = 80
			r.GrowthWeekOnWeek = &growingFast
			return r
		}, Watch},

		{"fast growth on a filling fleet", func(r Reading) Reading {
			r.PeakWeek = 400
			r.GrowthWeekOnWeek = &growingFast
			return r
		}, ScaleRequired},

		{"some growth", func(r Reading) Reading {
			r.GrowthWeekOnWeek = &growingSome
			return r
		}, Watch},

		{"most minutes are busy", func(r Reading) Reading {
			r.P95Utilisation = 0.85
			return r
		}, ScaleRequired},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := Assess(c.make(comfortable()))
			if v.State != c.want {
				t.Errorf("got %s, want %s (reasons: %s)", v.State, c.want, strings.Join(v.Reasons, "; "))
			}
			if len(v.Reasons) == 0 {
				t.Error("no reason given; a state nobody can explain is a state nobody acts on")
			}
		})
	}
}

// TestTheWorstReasonWins checks that several problems at once produce the
// state of the worst, not of the last one looked at.
func TestTheWorstReasonWins(t *testing.T) {
	r := comfortable()
	r.PeakWeek = 650  // watch
	r.DomainsSpare = 0 // scale required
	r.NodesUsable = 0  // critical

	v := Assess(r)
	if v.State != Critical {
		t.Errorf("got %s, want %s", v.State, Critical)
	}
	if len(v.Reasons) < 3 {
		t.Errorf("three things were wrong and %d were named: %s",
			len(v.Reasons), strings.Join(v.Reasons, "; "))
	}
}

// TestNoHistoryIsNotNoGrowth guards the case a young service is in.
//
// A trend invented from two days would be acted on, and acting on it means
// renting a machine that is not needed. Absence of history has to stay absent
// rather than becoming a zero.
func TestNoHistoryIsNotNoGrowth(t *testing.T) {
	r := comfortable()
	r.GrowthWeekOnWeek = nil

	v := Assess(r)
	if v.State != Normal {
		t.Errorf("a service with no history was judged %s", v.State)
	}
	for _, reason := range v.Reasons {
		if strings.Contains(reason, "вырос") {
			t.Errorf("growth was reported with no history to compute it from: %q", reason)
		}
	}
}

// TestAnEmptyFleetIsCritical is the one case with no threshold in it.
func TestAnEmptyFleetIsCritical(t *testing.T) {
	v := Assess(Reading{})
	if v.State != Critical {
		t.Errorf("a fleet with nothing usable was judged %s", v.State)
	}
}
