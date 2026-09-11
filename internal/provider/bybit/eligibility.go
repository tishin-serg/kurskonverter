package bybit

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The online API returns maker preferences even when the caller cannot trade.
// Decode per ad so malformed eligibility data cannot remove valid neighbouring ads.
func adCountries(raw json.RawMessage) ([]string, error) {
	var p struct {
		HasNationalLimit scalar
		NationalLimit    string
	}
	if len(raw) == 0 || json.Unmarshal(raw, &p) != nil {
		return nil, fmt.Errorf("missing or invalid P2P country restrictions")
	}
	switch p.HasNationalLimit {
	case "0":
		// Disabled preferences may retain their previous country list.
		return nil, nil
	case "1":
		if strings.TrimSpace(p.NationalLimit) == "" {
			return nil, fmt.Errorf("empty P2P country restriction")
		}
		var countries []string
		for _, part := range strings.Split(p.NationalLimit, ",") {
			code := strings.ToUpper(strings.TrimSpace(part))
			if len(code) != 3 || strings.IndexFunc(code, func(r rune) bool { return r < 'A' || r > 'Z' }) >= 0 {
				return nil, fmt.Errorf("invalid P2P country code")
			}
			countries = append(countries, code)
		}
		return countries, nil
	default:
		return nil, fmt.Errorf("unknown P2P country restriction status")
	}
}
