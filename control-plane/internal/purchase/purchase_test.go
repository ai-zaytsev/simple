package purchase

import (
	"testing"
	"time"
)

var (
	now     = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	open    = Settings{Open: true, FreeDays: 7}
	shut    = Settings{Open: false, FreeDays: 7}
	instant = Settings{Open: true, FreeDays: 0}
)

// Every combination of the three facts, written out.
//
// The Business Owner cannot exercise this from the application - the only
// account there is has already been given VIP, so the FREE side of every rule
// below is unreachable by hand. That makes this table the only place these
// answers are ever checked, which is why it is exhaustive rather than
// representative.
func TestWhoMayBuy(t *testing.T) {
	day := 24 * time.Hour

	cases := []struct {
		name      string
		created   time.Time
		tier      string
		settings  Settings
		available bool
		reason    string
	}{
		{
			"a new FREE account waits",
			now.Add(-1 * day), "FREE", open,
			false, ReasonTooSoon,
		},
		{
			"a FREE account on its seventh day still waits",
			now.Add(-7*day + time.Minute), "FREE", open,
			false, ReasonTooSoon,
		},
		{
			"a FREE account exactly seven days old may buy",
			now.Add(-7 * day), "FREE", open,
			true, "",
		},
		{
			"an older FREE account may buy",
			now.Add(-30 * day), "FREE", open,
			true, "",
		},
		{
			"no wait configured means no wait",
			now, "FREE", instant,
			true, "",
		},

		// The switch, and what it does not touch.
		{
			"sales off refuses a FREE account that has waited",
			now.Add(-30 * day), "FREE", shut,
			false, ReasonClosed,
		},
		{
			"sales off refuses a FREE account still waiting, without a date",
			now.Add(-1 * day), "FREE", shut,
			false, ReasonClosed,
		},

		// VIP, which is the row that must not move.
		{
			"a VIP account is not offered what it has",
			now.Add(-30 * day), "VIP", open,
			false, ReasonAlreadyVIP,
		},
		{
			"a VIP account is unaffected by sales being off",
			now.Add(-30 * day), "VIP", shut,
			false, ReasonAlreadyVIP,
		},
		{
			"a VIP account younger than the free period is still VIP",
			now.Add(-1 * day), "VIP", shut,
			false, ReasonAlreadyVIP,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Assess(now, c.created, c.tier, c.settings)
			if got.Available != c.available {
				t.Errorf("available: got %v, want %v", got.Available, c.available)
			}
			if got.Reason != c.reason {
				t.Errorf("reason: got %q, want %q", got.Reason, c.reason)
			}
		})
	}
}

// A date is given exactly when it means something.
//
// "Too soon" is the one refusal that resolves on its own, so it is the one
// that may carry a date. A date on "closed" would be a promise nobody has
// made - sales reopen when somebody decides they do - and a date on
// "already VIP" would be nonsense.
func TestOnlyTheWaitCarriesADate(t *testing.T) {
	day := 24 * time.Hour

	waiting := Assess(now, now.Add(-2*day), "FREE", open)
	if waiting.AvailableAt == nil {
		t.Fatal("a waiting account is not told when the wait ends")
	}
	if want := now.Add(5 * day); !waiting.AvailableAt.Equal(want) {
		t.Errorf("the wait ends at %v, want %v", waiting.AvailableAt, want)
	}

	for _, c := range []struct {
		name  string
		offer Offer
	}{
		{"sales off", Assess(now, now.Add(-2*day), "FREE", shut)},
		{"already VIP", Assess(now, now.Add(-2*day), "VIP", open)},
		{"may buy now", Assess(now, now.Add(-30*day), "FREE", open)},
	} {
		if c.offer.AvailableAt != nil {
			t.Errorf("%s: carries a date it has not earned: %v", c.name, c.offer.AvailableAt)
		}
	}
}

// Changing the wait moves the people already waiting.
//
// That is the point of the setting being a number of days rather than a date
// stamped onto each account when it was made. An operator shortening the
// period must open the door for everybody at once, including somebody who
// signed up yesterday - otherwise the change applies only to accounts that do
// not exist yet, which is the opposite of a switch.
func TestChangingTheWaitMovesTheWaiting(t *testing.T) {
	day := 24 * time.Hour
	created := now.Add(-3 * day)

	if Assess(now, created, "FREE", Settings{Open: true, FreeDays: 7}).Available {
		t.Fatal("a three-day-old account may buy under a seven-day wait")
	}
	if !Assess(now, created, "FREE", Settings{Open: true, FreeDays: 2}).Available {
		t.Error("shortening the wait did not reach an account already waiting")
	}
	if Assess(now, created, "FREE", Settings{Open: true, FreeDays: 14}).Available {
		t.Error("lengthening the wait did not reach an account already waiting")
	}
}

// Being VIP is decided before sales are, and the order is the rule.
//
// Read the other way round, an account that has paid would be told the thing
// it paid for is unavailable the moment sales were switched off - which turns
// an operator's routine action into a support queue, and some of those into
// refund requests.
func TestPayingCustomersDoNotNoticeTheSwitch(t *testing.T) {
	day := 24 * time.Hour
	vip := Assess(now, now.Add(-90*day), "VIP", shut)

	if vip.Reason != ReasonAlreadyVIP {
		t.Errorf("a VIP account sees %q when sales are off, want %q",
			vip.Reason, ReasonAlreadyVIP)
	}
}

// A wait that has been removed opens the door immediately, and a nonsensical
// one does not compute a date in the past.
func TestNoWaitMeansNoWait(t *testing.T) {
	for _, days := range []int{0, -1, -365} {
		got := Assess(now, now, "FREE", Settings{Open: true, FreeDays: days})
		if !got.Available {
			t.Errorf("FreeDays=%d refuses an account: %q", days, got.Reason)
		}
		if got.AvailableAt != nil {
			t.Errorf("FreeDays=%d invents a date: %v", days, got.AvailableAt)
		}
	}
}
