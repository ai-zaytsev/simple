package npd

import (
	"errors"
	"testing"
)

func TestSuccessfulPaymentGetsOneReceipt(t *testing.T) {
	steps, err := Plan(39900, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0].Kind != StepCreate || steps[0].AmountMinor != 39900 {
		t.Fatalf("expected one receipt for the full amount, got %+v", steps)
	}
}

func TestFullRefundCancelsAndCreatesNothing(t *testing.T) {
	steps, err := Plan(120000, 120000, &Receipt{UUID: "u", AmountMinor: 120000})
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0].Kind != StepCancel || steps[0].ReceiptUUID != "u" {
		t.Fatalf("a full refund voids the receipt and creates no other, got %+v", steps)
	}
}

func TestPartialRefundReplacesTheReceiptWithTheRemainder(t *testing.T) {
	// The example from the stage: 1 200 ₽ paid, 600 ₽ returned.
	steps, err := Plan(120000, 60000, &Receipt{UUID: "u", AmountMinor: 120000})
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 {
		t.Fatalf("expected cancel then create, got %+v", steps)
	}
	if steps[0].Kind != StepCancel || steps[0].ReceiptUUID != "u" {
		t.Fatalf("the standing receipt must be voided first, got %+v", steps[0])
	}
	if steps[1].Kind != StepCreate || steps[1].AmountMinor != 60000 {
		t.Fatalf("the new receipt is the remainder of the original payment, got %+v", steps[1])
	}
}

func TestSecondPartialRefundCountsFromTheOriginalPayment(t *testing.T) {
	// 1 200 paid, 600 returned, then 300 more. The standing receipt is 600.
	// The next one must be 300 - the remainder of the payment, not of the
	// receipt, and not 600-300 arrived at by luck.
	steps, err := Plan(120000, 90000, &Receipt{UUID: "second", AmountMinor: 60000})
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 || steps[1].AmountMinor != 30000 {
		t.Fatalf("the remainder is counted from the payment, got %+v", steps)
	}
}

func TestNothingToDoWhenTheReceiptIsAlreadyRight(t *testing.T) {
	// This is what makes a retry safe. An operation whose result never got
	// written down comes back here and is told there is nothing to do, rather
	// than issuing a second receipt for the same money.
	steps, err := Plan(39900, 0, &Receipt{UUID: "u", AmountMinor: 39900})
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 0 {
		t.Fatalf("expected no steps, got %+v", steps)
	}
}

func TestCrashBetweenCancelAndCreateResumes(t *testing.T) {
	// Cancelled, then the process died before the replacement was made. There
	// is no active receipt and money is still owed a receipt.
	steps, err := Plan(120000, 60000, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0].Kind != StepCreate || steps[0].AmountMinor != 60000 {
		t.Fatalf("expected only the missing step, got %+v", steps)
	}
}

func TestFullRefundWithNoReceiptDoesNothing(t *testing.T) {
	steps, err := Plan(39900, 39900, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 0 {
		t.Fatalf("nothing stands and nothing is owed, got %+v", steps)
	}
}

func TestRefundLargerThanPaymentIsRefused(t *testing.T) {
	// Not clamped to zero: that would hide a real disagreement under a
	// plausible-looking receipt.
	_, err := Plan(39900, 40000, nil)
	if !errors.Is(err, ErrRefundExceedsPayment) {
		t.Fatalf("expected the arithmetic to be refused, got %v", err)
	}
}

func TestNonPositivePaymentIsRefused(t *testing.T) {
	if _, err := Plan(0, 0, nil); err == nil {
		t.Fatal("a payment of zero has no receipt to register")
	}
}
