package money

import (
	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/domain"
	"testing"
)

func TestP2PCountryBeforePrice(t *testing.T) {
	d := decimal.RequireFromString
	base := domain.P2POffer{Provider: "Bybit", Asset: "USDT", Fiat: "RUB", OfferID: "open", Price: d("90"), MinFiat: d("1"), MaxFiat: d("100000"), AvailableAsset: d("1000"), CompletionRate: d("99")}
	cheap := base
	cheap.OfferID, cheap.Price, cheap.BlockedCountries = "restricted", d("50"), []string{"RUS", "AFG"}
	for _, tc := range []struct{ country, want string }{{"RUS", "open"}, {"", "open"}, {"GEO", "restricted"}, {"USA", "restricted"}} {
		got, _, err := SelectP2P(d("100"), "USDT", []domain.P2POffer{cheap, base}, domain.Filter{BybitCountry: tc.country})
		if err != nil || got.OfferID != tc.want {
			t.Fatal(tc, got, err)
		}
	}
}
