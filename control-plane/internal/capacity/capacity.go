// Package capacity decides whether the service is running out of room, and
// says so in four words a person can act on.
//
// The decision is separated from where the message goes on purpose. What makes
// a service short of capacity does not change when somebody moves from email
// to a messenger, and a rule tangled up with a transport has to be re-read and
// re-tested every time the transport changes. See internal/alert for the other
// half.
package capacity

import "fmt"

// State is what somebody is being asked to do about it.
type State string

const (
	// Nothing to do.
	Normal State = "NORMAL"

	// Nothing to do today, and worth knowing. Something is trending towards a
	// decision - room shrinking, spares thinning, a domain pool running down.
	Watch State = "WATCH"

	// A decision is due. There is still room, and there will not be for long,
	// and adding a server takes longer than the warning gives if it is left.
	ScaleRequired State = "SCALE_REQUIRED"

	// The room is gone or about to go. People are being affected, or will be
	// within the hour.
	Critical State = "CRITICAL"
)

// Reading is what the service knows about its own room right now.
//
// Every field is a number the service already collects; nothing here is
// measured specially for the alert. Pointers where history may be missing: a
// fortnight-old service cannot say how it grew last week, and saying so is
// better than inventing a trend from two days.
type Reading struct {
	// How many nodes could be handed out, and how many of those are spares
	// nobody is leaning on.
	NodesUsable int
	NodesSpare  int

	// Connections now, against what the group is sized for.
	SessionsNow   int
	CapacityTotal int

	// The busiest minute of the last day and of the last week. Peaks are what
	// capacity has to cover; an average covers nothing.
	PeakToday int
	PeakWeek  int

	// The utilisation that all but one minute in twenty stayed below. A single
	// spike is an incident; a high P95 is a service that is full most of the
	// time it matters.
	P95Utilisation float64

	// The busiest minute this week against the busiest minute the week before,
	// when there is a week before. Nil when there is not enough history.
	GrowthWeekOnWeek *float64

	// Consumable domains sitting ready and unused. A new node needs one, and
	// buying one is not something that happens in an afternoon.
	DomainsSpare int

	// Whether anything is currently blocked or broken. A fleet with no room
	// and no fault is a different situation from a fleet with no room because
	// half of it is unreachable.
	NodesBlocked int
	NodesFaulty  int
}

// Verdict is the state and the sentences that produced it.
type Verdict struct {
	State   State    `json:"state"`
	Reasons []string `json:"reasons"`

	// What the numbers were when this was decided, so that a message read an
	// hour later still means something.
	Utilisation float64 `json:"utilisation"`
	PeakUsed    float64 `json:"peak_used"`
}

const (
	// Where a peak stops being comfortable. Below this there is room for a
	// node to fail and the rest to carry its people.
	watchAbove = 0.60

	// Where adding a server has to be decided rather than considered. Chosen
	// against how long adding one takes: a build is minutes, but a domain, a
	// certificate and a provider's quota are not, and the last of those has
	// already taken longer than a day on this fleet.
	scaleAbove = 0.80

	// Where people are being affected or are about to be.
	criticalAbove = 0.95

	// Below this many unused domains, raising a node stops being possible
	// quickly. Two, because one is what you use and none is what you have
	// afterwards.
	domainsLow = 2
)

// Assess turns the numbers into one of the four words.
//
// Written as a sequence of named conditions rather than a formula, because
// somebody woken by this needs to know which one fired. A number that is high
// for two different reasons is two different problems.
func Assess(r Reading) Verdict {
	v := Verdict{State: Normal}

	if r.CapacityTotal > 0 {
		v.Utilisation = float64(r.SessionsNow) / float64(r.CapacityTotal)
		v.PeakUsed = float64(r.PeakWeek) / float64(r.CapacityTotal)
	}

	raise := func(to State, why string) {
		v.Reasons = append(v.Reasons, why)
		if rank(to) > rank(v.State) {
			v.State = to
		}
	}

	// Nowhere to send anybody is the worst thing this can say, and it does not
	// depend on any threshold.
	if r.NodesUsable == 0 {
		raise(Critical, "no node can be handed out")
	}

	switch {
	case v.PeakUsed >= criticalAbove:
		raise(Critical, fmt.Sprintf("the week's peak used %.0f%% of capacity", v.PeakUsed*100))
	case v.PeakUsed >= scaleAbove:
		raise(ScaleRequired, fmt.Sprintf("the week's peak used %.0f%% of capacity", v.PeakUsed*100))
	case v.PeakUsed >= watchAbove:
		raise(Watch, fmt.Sprintf("the week's peak used %.0f%% of capacity", v.PeakUsed*100))
	}

	if r.P95Utilisation >= scaleAbove {
		raise(ScaleRequired, fmt.Sprintf(
			"nineteen minutes in twenty are above %.0f%% of capacity", r.P95Utilisation*100))
	} else if r.P95Utilisation >= watchAbove {
		raise(Watch, fmt.Sprintf(
			"nineteen minutes in twenty are above %.0f%% of capacity", r.P95Utilisation*100))
	}

	// A fleet with no spare is a fleet where one machine failing is an outage,
	// however much room the rest has.
	if r.NodesUsable > 0 && r.NodesSpare == 0 {
		raise(ScaleRequired, "no working spare: one node failing takes its people with it")
	}

	// Growth, only where there is a week to compare with. Doubling from a
	// small number is not an emergency, so this raises the state only when
	// there is also something to fill.
	if r.GrowthWeekOnWeek != nil && *r.GrowthWeekOnWeek > 0.5 && v.PeakUsed >= watchAbove/2 {
		raise(ScaleRequired, fmt.Sprintf(
			"the peak grew %.0f%% on last week", *r.GrowthWeekOnWeek*100))
	} else if r.GrowthWeekOnWeek != nil && *r.GrowthWeekOnWeek > 0.25 {
		raise(Watch, fmt.Sprintf("the peak grew %.0f%% on last week", *r.GrowthWeekOnWeek*100))
	}

	// Domains are the slow part of adding a node, so they are warned about
	// before they run out rather than when they do.
	switch {
	case r.DomainsSpare == 0:
		raise(ScaleRequired, "no consumable domain is spare: a new node cannot be raised")
	case r.DomainsSpare < domainsLow:
		raise(Watch, fmt.Sprintf("%d consumable domain spare", r.DomainsSpare))
	}

	if r.NodesBlocked > 0 {
		raise(ScaleRequired, fmt.Sprintf(
			"%d node reachable by us and not by devices: replace rather than repair", r.NodesBlocked))
	}
	if r.NodesFaulty > 0 {
		raise(Watch, fmt.Sprintf("%d node nobody can reach", r.NodesFaulty))
	}

	if len(v.Reasons) == 0 {
		v.Reasons = append(v.Reasons, "room to spare, nothing trending")
	}
	return v
}

func rank(s State) int {
	switch s {
	case Critical:
		return 3
	case ScaleRequired:
		return 2
	case Watch:
		return 1
	default:
		return 0
	}
}
