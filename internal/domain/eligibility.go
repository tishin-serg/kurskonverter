package domain

import "strings"

// CountryEligible checks the ad's KYC-country allowlist, not the bank location.
// With no personal country, only ads without country restrictions are eligible.
func CountryEligible(o P2POffer, f Filter) bool {
	if !strings.EqualFold(o.Provider, "Bybit") || len(o.AllowedCountries) == 0 {
		return true
	}
	if f.BybitCountry == "" {
		return false
	}
	for _, country := range o.AllowedCountries {
		if country == f.BybitCountry {
			return true
		}
	}
	return false
}
