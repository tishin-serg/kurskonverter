package route

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/domain"
	"github.com/tishin-serg/kurskonverter/internal/provider/demo"
)

func TestUSDTBCJointQuote(t *testing.T) {
	now := time.Now()
	s := demo.Snapshot(now)
	d := decimal.RequireFromString
	q, err := CalculateUSD(d("100"), s, domain.Filter{}, now)
	if err != nil || q.ReceivedUSD.LessThan(d("100")) || q.USDTRequired.String() != "99.009901" || q.RUBRequired.String() != "8910.9" {
		t.Fatal(q, err)
	}
	if q.EffectiveRate.Equal(d("90")) {
		t.Fatal("USD was incorrectly treated as USDT")
	}
	// A cheaper sell quote with insufficient capacity must not displace the valid pair.
	data := s.P2P["bybit:usd"]
	bad := data.Value[0]
	bad.Price = d("2")
	bad.AvailableAsset = d("1")
	data.Value = append(data.Value, bad)
	s.P2P["bybit:usd"] = data
	q2, err := CalculateUSD(d("100"), s, domain.Filter{}, now)
	if err != nil || !q2.RUBRequired.Equal(q.RUBRequired) {
		t.Fatal(q2, err)
	}
	// A coarser purchase lot leaves a visible USDT balance; it must still be funded.
	rub := s.P2P["bybit"]
	rub.Value[0].AssetStep = d("1")
	s.P2P["bybit"] = rub
	q, err = CalculateUSD(d("100"), s, domain.Filter{}, now)
	if err != nil || q.USDTBought.String() != "100" || q.RUBRequired.String() != "9000" {
		t.Fatal(q, err)
	}
}
func TestUSDTBCExclusions(t *testing.T) {
	for _, kind := range []string{"wrong-bank", "wrong-direction", "stale", "min-usd", "max-usd", "reserve", "success", "orders", "rub-bank", "rub-reserve", "fallback"} {
		t.Run(kind, func(t *testing.T) {
			now := time.Now()
			s := demo.Snapshot(now)
			d := decimal.RequireFromString
			f := domain.Filter{}
			usd := s.P2P["bybit:usd"]
			rub := s.P2P["bybit"]
			switch kind {
			case "wrong-bank":
				usd.Value[0].PaymentMethods = []string{"14"}
			case "wrong-direction":
				usd.Value[0].TakerSells = false
			case "stale":
				usd.UpdatedAt = now.Add(-61 * time.Second)
			case "min-usd":
				usd.Value[0].MinFiat = d("200")
			case "max-usd":
				usd.Value[0].MaxFiat = d("50")
			case "reserve":
				usd.Value[0].AvailableAsset = d("1")
			case "success":
				f.MinCompletionRate = d("100")
			case "orders":
				f.MinOrdersCount = 1001
			case "rub-bank":
				f.PaymentMethod = "absent"
			case "rub-reserve":
				rub.Value[0].AvailableAsset = d("1")
			case "fallback":
				usd.Value[0].FeeFallback = true
				f.DisallowFallback = true
			}
			s.P2P["bybit:usd"] = usd
			s.P2P["bybit"] = rub
			if _, err := CalculateUSD(d("100"), s, f, now); err == nil {
				t.Fatal("invalid pair accepted")
			}
		})
	}
}
