package domain

import "strings"

// CanonicalPayment maps explicit bank names across APIs. Numeric provider IDs
// remain unchanged: a generic bank transfer must never imply a specific bank.
func CanonicalPayment(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "tbank", "t-bank", "tinkoff", "тинькофф", "т-банк", "т-банк rub", "tcsbrub":
		return "tbank"
	case "sbp", "сбп", "сбп rub", "sbprub":
		return "sbp"
	default:
		return s
	}
}
func PaymentMatches(wanted string, available []string) bool {
	if wanted == "" {
		return true
	}
	wanted = CanonicalPayment(wanted)
	for _, v := range available {
		if CanonicalPayment(v) == wanted {
			return true
		}
	}
	return false
}
