// Package purchase answers one question: may this account buy VIP right now.
//
// A package of its own, with no database and no HTTP in it, because the
// question is a decision and decisions are the part worth testing exhaustively.
// Everything that makes it hard to test - reading settings, reading an account,
// writing JSON - lives outside and hands this the four facts it needs.
//
// The same shape as internal/capacity, and for the same reason: a rule that can
// only be exercised by standing up a service is a rule that gets exercised
// once.
package purchase

import "time"

// Settings are what an operator can change without a deploy.
type Settings struct {
	// Whether anybody at all may buy. The whole of the switch: turning it off
	// stops new purchases and touches nothing that has already been bought.
	Open bool

	// Whether receipts can be issued at all.
	//
	// A separate reason from Open, and not folded into it, because the two are
	// different facts with different owners: Open is a decision somebody made,
	// this is a condition somebody found. Folding them together would let a
	// morning check quietly overwrite the operator's own switch, and would
	// leave nobody able to say which of the two closed the door.
	ReceiptsWorking bool

	// How long a new account waits on FREE before it may buy.
	//
	// Days rather than a timestamp per account, so that changing it moves
	// everybody at once - including the people already waiting, which is the
	// point of being able to change it at all.
	FreeDays int
}

// Why an account may not buy. Empty when it may.
const (
	// The account already has what it would be buying.
	ReasonAlreadyVIP = "already_vip"

	// Sales are off. Ours, not theirs, and the wording a person sees has to
	// reflect that: nothing they do brings this forward.
	ReasonClosed = "closed"

	// Receipts cannot be issued, so nothing may be sold. Selling without a
	// receipt is not a degraded sale, it is one we are not allowed to make.
	ReasonNoReceipts = "no_receipts"

	// The free period has not finished. This one has a date, which is what
	// makes it different from every other refusal here.
	ReasonTooSoon = "too_soon"
)

// Offer is the answer, in the shape the application needs to draw it.
type Offer struct {
	// Whether the button does anything when pressed.
	Available bool

	// Why not, when it does not. One of the reasons above.
	Reason string

	// When the wait ends. Only set for ReasonTooSoon: a date on any other
	// refusal would be a promise we have not made, and "closed" in particular
	// has no date because nobody has decided one.
	AvailableAt *time.Time
}

// Assess decides whether this account may buy VIP.
//
// The order of the tests is the whole of the design, so it is written out
// rather than left to the reader:
//
// Already being VIP comes first, and it comes first even when sales are off.
// An account that has paid must never be told its status is unavailable - the
// switch stops new purchases and says nothing about existing ones, and reading
// them in the other order would turn a support question into a refund request.
//
// Sales being off comes next, before the wait. A person three days into their
// free period, on a service that is not selling, should not be shown a
// countdown to a date on which nothing will happen.
//
// Receipts follow the switch, and are separate from it. The operator turning
// sales off is a decision; the tax service being unreachable is a condition. A
// person needs to know which one they are looking at, and an operator needs
// their own switch not to be silently rewritten by a morning check.
//
// The wait comes last, because it is the only refusal that resolves by itself.
func Assess(now, accountCreated time.Time, tier string, settings Settings) Offer {
	if tier == "VIP" {
		return Offer{Reason: ReasonAlreadyVIP}
	}
	if !settings.Open {
		return Offer{Reason: ReasonClosed}
	}
	if !settings.ReceiptsWorking {
		return Offer{Reason: ReasonNoReceipts}
	}

	// Zero and negative both mean no wait. Negative cannot be stored - the
	// column refuses it - but arriving here from a hand-edited row should
	// open the door rather than compute a date in the past and call it the
	// future.
	if settings.FreeDays <= 0 {
		return Offer{Available: true}
	}

	opens := accountCreated.Add(time.Duration(settings.FreeDays) * 24 * time.Hour)
	if !now.Before(opens) {
		return Offer{Available: true}
	}
	return Offer{Reason: ReasonTooSoon, AvailableAt: &opens}
}
