package domain

import "testing"

func TestPaymentAliases(t *testing.T) {
	for _, s := range []string{"Tbank", "Т-Банк", "tinkoff", "TCSBRUB", "Т-Банк RUB"} {
		if !PaymentMatches(s, []string{"tinkoff"}) {
			t.Fatal(s)
		}
	}
	if PaymentMatches("Tbank", []string{"14", "Bank Transfer", "sbp"}) {
		t.Fatal("generic transfer is not a specific bank")
	}
	if !PaymentMatches("14", []string{"14"}) {
		t.Fatal("raw IDs must remain usable")
	}
}
