// Package npd decides what must exist in "Мой налог" for one payment.
//
// No database, no HTTP, no clock. The whole tax question is "given what the
// customer actually paid us and what we have already given back, which receipt
// should stand right now" - and that is arithmetic, not I/O.
//
// Keeping it here is what makes the awkward cases ordinary. A crash between
// cancelling the old receipt and creating the new one leaves the world in a
// state this function can read; run it again and it asks for exactly the step
// that is missing. There is no separate recovery path because there is no
// separate question.
package npd

import (
	"errors"
	"fmt"
)

// Receipt is a receipt as it stands in НПД right now.
type Receipt struct {
	// RowID is our own row, so the caller can mark exactly this receipt
	// cancelled without matching on the provider identifier.
	RowID       string
	UUID        string
	AmountMinor int64
}

// StepKind is what has to be done to lknpd.
type StepKind string

const (
	// StepCancel voids a receipt. The reason is always REFUND: the only thing
	// that makes us void a receipt here is money going back.
	StepCancel StepKind = "cancel"

	// StepCreate registers income.
	StepCreate StepKind = "create"
)

type Step struct {
	Kind StepKind

	// Cancel: which receipt. Create: how much, in minor units.
	ReceiptUUID string
	AmountMinor int64
}

// ErrRefundExceedsPayment means the numbers cannot both be true.
//
// Loud rather than clamped to zero. A refund larger than its payment is either
// a bug in our arithmetic or money moved outside the flow we can see, and both
// are things a person has to look at - quietly registering a receipt for zero
// would hide it under a correct-looking result.
var ErrRefundExceedsPayment = errors.New("возвращено больше, чем заплачено")

// Plan says what to do to make НПД match the money.
//
// paidMinor is what the customer paid before the provider's commission: the
// receipt is for the customer's payment, not for our takings.
//
// refundedMinor is the sum of every confirmed refund. Recomputed from the
// payment each time rather than accumulated step by step, so that a second and
// third partial refund need no memory of the first.
//
// active is the receipt currently standing for this payment, or nil.
func Plan(paidMinor, refundedMinor int64, active *Receipt) ([]Step, error) {
	if paidMinor <= 0 {
		return nil, fmt.Errorf("сумма платежа должна быть положительной, получено %d", paidMinor)
	}
	if refundedMinor < 0 {
		return nil, fmt.Errorf("сумма возвратов не может быть отрицательной, получено %d", refundedMinor)
	}
	if refundedMinor > paidMinor {
		return nil, fmt.Errorf("%w: %d из %d", ErrRefundExceedsPayment, refundedMinor, paidMinor)
	}

	remaining := paidMinor - refundedMinor

	if active == nil {
		if remaining == 0 {
			// Fully refunded before a receipt ever existed. Nothing stands, so
			// nothing has to be voided, and registering income that was
			// entirely returned would be a lie in the other direction.
			return nil, nil
		}
		return []Step{{Kind: StepCreate, AmountMinor: remaining}}, nil
	}

	if active.AmountMinor == remaining {
		// Already right. This is the case that makes retries free: an
		// operation that succeeded but whose bookkeeping did not land comes
		// back here and asks for nothing.
		return nil, nil
	}

	steps := []Step{{Kind: StepCancel, ReceiptUUID: active.UUID}}
	if remaining > 0 {
		// The new receipt belongs to the original payment, not to a new sale.
		// It is the same money, corrected downwards.
		steps = append(steps, Step{Kind: StepCreate, AmountMinor: remaining})
	}
	return steps, nil
}
