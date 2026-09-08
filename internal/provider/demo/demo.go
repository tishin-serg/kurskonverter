// Package demo contains synthetic, explicitly labelled offline fixtures only.
package demo

import (
	"time"

	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/domain"
)

func Snapshot(now time.Time) *domain.MarketSnapshot {
	d := decimal.RequireFromString
	s := domain.EmptySnapshot()
	for _, key := range []string{"bybit:BTCUSDT", "okx:BTC-USDT"} {
		s.Books[key] = domain.OrderBook{UpdatedAt: now, Asks: []domain.OrderBookLevel{{Price: d("100000"), Amount: d(".003")}, {Price: d("100010"), Amount: d(".005")}, {Price: d("100050"), Amount: d("1")}}}
		s.Instruments[key] = domain.Instrument{Base: "BTC", Quote: "USDT", Step: d(".00000001"), Tick: d(".01"), UpdatedAt: now}
	}
	s.Books["okx:GRAM-USDT"] = domain.OrderBook{UpdatedAt: now, Bids: []domain.OrderBookLevel{{Price: d("5"), Amount: d("100000")}}}
	s.Instruments["okx:GRAM-USDT"] = domain.Instrument{Base: "GRAM", Quote: "USDT", Step: d(".01"), Tick: d(".01"), UpdatedAt: now}
	for _, key := range []string{"bybit:trade", "okx:BTC-USDT:trade", "okx:GRAM-USDT:trade"} {
		s.Fees[key] = domain.Fee{Rate: d(".001"), Enabled: true, UpdatedAt: now}
	}
	for _, key := range []string{"bybit:BTC", "okx:BTC"} {
		s.Fees[key] = domain.Fee{Fixed: d(".00005"), Step: d(".00000001"), Enabled: true, UpdatedAt: now}
	}
	s.Fees["wallet:GRAM"] = domain.Fee{Fixed: d(".05"), Step: d(".01"), Enabled: true, UpdatedAt: now}
	s.Fees["okx:GRAM:deposit"] = domain.Fee{Min: d(".1"), Enabled: true, UpdatedAt: now}
	for _, x := range []struct{ key, asset, price string }{{"bybit", "USDT", "90"}, {"wallet", "GRAM", "445"}} {
		s.P2P[x.key] = domain.Data[[]domain.P2POffer]{UpdatedAt: now, Value: []domain.P2POffer{{Provider: x.key, Asset: x.asset, Fiat: "RUB", MerchantName: "DEMO merchant", Price: d(x.price), MinFiat: d("1000"), MaxFiat: d("10000000"), AvailableAsset: d("100000"), CompletionRate: d("99"), OrdersCount: 1000, PaymentMethods: []string{"demo-bank"}}}}
	}
	s.Exchange = domain.Data[[]domain.ExchangeOffer]{UpdatedAt: now, Value: []domain.ExchangeOffer{{Exchanger: "DEMO exchanger", PaymentMethod: "demo-bank", Rate: d("8800000"), MinFiat: d("1000"), MaxFiat: d("10000000"), ReserveBTC: d("10"), Enabled: true}}}
	return s
}
