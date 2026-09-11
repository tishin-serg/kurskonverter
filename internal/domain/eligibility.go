package domain

import "strings"

// CountryEligible checks the ad's KYC-country denylist, not the bank location.
// With no personal country, ads with country restrictions cannot be verified.
func CountryEligible(o P2POffer, f Filter) bool {
	if !strings.EqualFold(o.Provider, "Bybit") || len(o.BlockedCountries) == 0 {
		return true
	}
	if f.BybitCountry == "" {
		return false
	}
	for _, country := range o.BlockedCountries {
		if country == f.BybitCountry {
			return false
		}
	}
	return true
}
