package payment

import "testing"

func TestOnlyTheWholeCanonicalPaymentConfirms(t *testing.T) {
	good := Canonical{
		ProviderPaymentID: "provider-1",
		PaymentID:         "our-1",
		AmountMinor:       39900,
		Currency:          "RUB",
		Status:            StatusSucceeded,
		Paid:              true,
	}
	if err := VerifySucceeded("our-1", 39900, "RUB", good); err != nil {
		t.Fatalf("valid payment refused: %v", err)
	}

	cases := map[string]Canonical{
		"pending":        with(good, func(p *Canonical) { p.Status = StatusPending }),
		"not paid":       with(good, func(p *Canonical) { p.Paid = false }),
		"wrong metadata": with(good, func(p *Canonical) { p.PaymentID = "somebody-else" }),
		"wrong amount":   with(good, func(p *Canonical) { p.AmountMinor++ }),
		"wrong currency": with(good, func(p *Canonical) { p.Currency = "USD" }),
	}
	for name, candidate := range cases {
		t.Run(name, func(t *testing.T) {
			if err := VerifySucceeded("our-1", 39900, "RUB", candidate); err == nil {
				t.Fatal("unconfirmed payment was accepted")
			}
		})
	}
}

func with(base Canonical, change func(*Canonical)) Canonical {
	change(&base)
	return base
}
