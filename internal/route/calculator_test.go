package route

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/domain"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }
func fixture(now time.Time) *domain.MarketSnapshot {
	s := domain.EmptySnapshot()
	s.Books["bybit:BTCUSDT"] = domain.OrderBook{UpdatedAt: now, Asks: []domain.OrderBookLevel{{Price: d("100000"), Amount: d("1")}}}
	s.Instruments["bybit:BTCUSDT"] = domain.Instrument{Base: "BTC", Quote: "USDT", Step: d(".00000001"), Tick: d(".01"), UpdatedAt: now}
	s.Fees["bybit:trade"] = domain.Fee{Rate: d(".001"), Enabled: true, UpdatedAt: now}
	s.Fees["bybit:BTC"] = domain.Fee{Fixed: d(".00005"), Step: d(".00000001"), Enabled: true, UpdatedAt: now}
	s.P2P["bybit"] = domain.Data[[]domain.P2POffer]{UpdatedAt: now, Value: []domain.P2POffer{{Asset: "USDT", Fiat: "RUB", Price: d("90"), MinFiat: d("100"), MaxFiat: d("1000000"), AvailableAsset: d("10000"), CompletionRate: d("100"), OrdersCount: 100}}}
	return s
}
func TestRoute(t *testing.T) {
	now := time.Now()
	s := fixture(now)
	r := DefaultEngine().Calculate(context.Background(), d(".01"), s, domain.Filter{}, now)
	if len(r.Quotes) != 1 || len(r.Unavailable) != 2 {
		t.Fatal(r)
	}
	q := r.Quotes[0]
	if !q.RUBRequired.Equal(d("90540.63")) {
		t.Fatal(q.RUBRequired)
	}
	if q.Steps[1].Output.LessThan(d(".01005")) {
		t.Fatal("underfunded fee")
	}
	if !q.Steps[2].Input.Equal(d(".01005")) {
		t.Fatal(q.Steps[2])
	}
	fee := s.Fees["bybit:trade"]
	fee.Fallback = true
	s.Fees["bybit:trade"] = fee
	r = DefaultEngine().Calculate(context.Background(), d(".01"), s, domain.Filter{}, now)
	if !r.Quotes[0].IsDegraded {
		t.Fatal("unmarked fallback")
	}
	r = DefaultEngine().Calculate(context.Background(), d(".01"), s, domain.Filter{}, now.Add(16*time.Second))
	if len(r.Quotes) != 0 {
		t.Fatal("stale book")
	}
}
func TestBestChangeRanking(t *testing.T) {
	now := time.Now()
	s := fixture(now)
	s.Exchange = domain.Data[[]domain.ExchangeOffer]{UpdatedAt: now, Value: []domain.ExchangeOffer{{Exchanger: "valid", Enabled: true, Rate: d("8000000"), MinFiat: d("1000"), MaxFiat: d("1000000"), ReserveBTC: d("1")}, {Exchanger: "empty", Enabled: true, Rate: d("1"), ReserveBTC: d(".001")}}}
	r := DefaultEngine().Calculate(context.Background(), d(".01"), s, domain.Filter{}, now)
	if len(r.Quotes) != 2 || r.Quotes[0].RouteID != "BestChange" || !r.Quotes[1].DifferenceRub.Equal(d("10540.63")) {
		t.Fatal(r)
	}
}
func TestWalletRoute(t *testing.T) {
	now := time.Now()
	s := fixture(now)
	s.Books["okx:BTC-USDT"] = s.Books["bybit:BTCUSDT"]
	s.Instruments["okx:BTC-USDT"] = s.Instruments["bybit:BTCUSDT"]
	s.Fees["okx:BTC-USDT:trade"] = s.Fees["bybit:trade"]
	s.Fees["okx:GRAM-USDT:trade"] = s.Fees["bybit:trade"]
	s.Fees["okx:BTC"] = s.Fees["bybit:BTC"]
	s.Fees["wallet:GRAM"] = domain.Fee{Fixed: d(".05"), Step: d(".01"), UpdatedAt: now, Enabled: true}
	s.Fees["okx:GRAM:deposit"] = domain.Fee{Min: d(".001"), Enabled: true, UpdatedAt: now}
	s.Instruments["okx:GRAM-USDT"] = domain.Instrument{Base: "GRAM", Quote: "USDT", Step: d(".01"), Tick: d(".01"), UpdatedAt: now}
	s.Books["okx:GRAM-USDT"] = domain.OrderBook{UpdatedAt: now, Bids: []domain.OrderBookLevel{{Price: d("5"), Amount: d("10000")}}}
	s.P2P["wallet"] = domain.Data[[]domain.P2POffer]{UpdatedAt: now, Value: []domain.P2POffer{{Asset: "GRAM", Fiat: "RUB", Price: d("450"), MaxFiat: d("1000000"), AvailableAsset: d("10000"), CompletionRate: d("100"), OrdersCount: 100}}}
	r := DefaultEngine().Calculate(context.Background(), d(".01"), s, domain.Filter{}, now)
	if len(r.Quotes) != 2 {
		t.Fatal(r)
	}
	q := r.Quotes[1]
	if !q.RUBRequired.Equal(d("90657.00")) {
		t.Fatal(q.RUBRequired)
	}
	if len(q.Steps) != 6 {
		t.Fatal(q)
	}
}

func TestDepositRejectsSuspensionAndMinimum(t *testing.T) {
	now := time.Now()
	s := fixture(now)
	step := Deposit{Key: "deposit", Asset: "GRAM", Provider: "OKX"}
	for _, fee := range []domain.Fee{{Enabled: false, UpdatedAt: now}, {Enabled: true, UpdatedAt: now.Add(-16 * time.Minute)}, {Enabled: true, UpdatedAt: now, Min: d("2")}} {
		s.Fees["deposit"] = fee
		if _, _, _, e := step.Backward(d("1"), s, now); e == nil {
			t.Fatal("invalid deposit accepted")
		}
	}
}
