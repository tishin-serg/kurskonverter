package money

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/domain"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }
func TestBuy(t *testing.T) {
	for _, tc := range []struct {
		name, target, want string
		levels             []domain.OrderBookLevel
		fail               bool
	}{
		{"one", "0.01", "1000", []domain.OrderBookLevel{{Price: d("100000"), Amount: d("1")}}, false},
		{"multiple", "0.01", "1000.15", []domain.OrderBookLevel{{Price: d("100050"), Amount: d(".01")}, {Price: d("100000"), Amount: d(".003")}, {Price: d("100010"), Amount: d(".005")}}, false},
		{"liquidity", "2", "", []domain.OrderBookLevel{{Price: d("1"), Amount: d("1")}}, true},
		{"negative", "-1", "", nil, true},
		{"invalid level", "1", "", []domain.OrderBookLevel{{Price: d("0"), Amount: d("2")}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CalculateBuyCost(d(tc.target), tc.levels)
			if (err != nil) != tc.fail {
				t.Fatal(err)
			}
			if !tc.fail && !got.Equal(d(tc.want)) {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}
func TestCeil(t *testing.T) {
	if got := CeilRatio(d("1.0000000000000000000000001"), d("1"), d(".01")); !got.Equal(d("1.01")) {
		t.Fatal(got)
	}
}
func TestSell(t *testing.T) {
	bids := []domain.OrderBookLevel{{Price: d("10"), Amount: d("1")}, {Price: d("5"), Amount: d("10")}}
	got, err := SellForQuote(d("13.5"), d(".1"), d(".1"), bids)
	if err != nil || !got.Equal(d("2")) {
		t.Fatal(got, err)
	}
	if _, err = SellForQuote(d("1000"), d("0"), d(".1"), bids); err == nil {
		t.Fatal("liquidity")
	}
}
func TestP2P(t *testing.T) {
	base := domain.P2POffer{Asset: "USDT", Fiat: "RUB", Price: d("90"), MinFiat: d("50"), MaxFiat: d("1000"), AvailableAsset: d("10"), CompletionRate: d("99"), OrdersCount: 100, PaymentMethods: []string{"bank"}}
	f := domain.Filter{PaymentMethod: "bank", MinCompletionRate: d("95"), MinOrdersCount: 50}
	for _, name := range []string{"min", "max", "available", "completion", "orders", "payment", "asset", "fiat"} {
		t.Run(name, func(t *testing.T) {
			o := base
			switch name {
			case "min":
				o.MinFiat = d("200")
			case "max":
				o.MaxFiat = d("80")
			case "available":
				o.AvailableAsset = d(".5")
			case "completion":
				o.CompletionRate = d("90")
			case "orders":
				o.OrdersCount = 1
			case "payment":
				o.PaymentMethods = nil
			case "asset":
				o.Asset = "BTC"
			case "fiat":
				o.Fiat = "USD"
			}
			if _, _, err := SelectP2P(d("1"), "USDT", []domain.P2POffer{o}, f); err == nil {
				t.Fatal("accepted invalid")
			}
		})
	}
	expensive := base
	expensive.Price = d("100")
	bad := base
	bad.Price = d("80")
	bad.OrdersCount = 0
	_, cost, err := SelectP2P(d("1"), "USDT", []domain.P2POffer{expensive, bad, base}, f)
	if err != nil || !cost.Equal(d("90")) {
		t.Fatal(cost, err)
	}
}
