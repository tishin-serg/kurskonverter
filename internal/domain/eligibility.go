package domain

import "strings"

// CountryEligible checks the ad's KYC-country denylist, not the bank location.
// With no personal country, ads with country restrictions cannot be verified.
func CountryEligible(o P2POffer, f Filter) bool {
	if !strings.EqualFold(o.Provider, "Bybit") || len(o.BlockedCountries) == 0 {
		return true
	}
	// Bybit's public ad endpoint does not expose the caller's effective region.
	// An active nationalLimit can still be rejected by the account, even when
	// the configured KYC country is absent from the returned list. Do not show
	// such an ad as executable; only unrestricted ads are safe to calculate.
	return false
}
