package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

type D = decimal.Decimal

// FiatEquivalent records the user's conversion basis, not a market quote.
type FiatEquivalent struct{ RUB, Rate D }
type OrderBookLevel struct{ Price, Amount D }
type OrderBook struct {
	Asks, Bids []OrderBookLevel
	UpdatedAt  time.Time
}
type P2POffer struct {
	AllowedCountries                                        []string
	TakerSells                                              bool
	OfferID                                                 string
	Provider, Asset, Fiat, MerchantName                     string
	Price, MinFiat, MaxFiat, AvailableAsset, CompletionRate D
	OrdersCount                                             int
	AssetStep                                               D
	FeeFallback                                             bool
	PaymentMethods                                          []string
}
type Filter struct {
	BybitCountry      string
	DisallowFallback  bool
	PaymentByProvider map[string]string
	PaymentMethod     string
	MinCompletionRate D
	MinOrdersCount    int
}
type Fee struct {
	Fixed, Rate, Step, Min, Max D
	UpdatedAt                   time.Time
	Fallback                    bool
	Enabled                     bool
}
type Instrument struct {
	MaxBase, MaxQuote, QuoteStep  D
	Base, Quote                   string
	Step, Tick, MinBase, MinQuote D
	UpdatedAt                     time.Time
}
type ExchangeOffer struct {
	URL                                string
	Warnings                           []string
	Exchanger, PaymentMethod           string
	Rate, MinFiat, MaxFiat, ReserveBTC D
	Enabled                            bool
}
type QuoteStep struct {
	MerchantName, OfferID                           string
	PaymentMethods                                  []string
	MinFiat, MaxFiat                                D
	Type, FromAsset, ToAsset, Provider, Description string
	Input, Output, Price, Fee                       D
}
type Quote struct {
	RouteID                                                               string
	TargetBTC, RUBRequired, BTCReceived, DifferenceRub, DifferencePercent D
	Steps                                                                 []QuoteStep
	CalculatedAt                                                          time.Time
	IsDegraded                                                            bool
	Warnings                                                              []string
}
type Data[T any] struct {
	Value     T
	UpdatedAt time.Time
}
type MarketSnapshot struct {
	Books       map[string]OrderBook
	Instruments map[string]Instrument
	Fees        map[string]Fee
	P2P         map[string]Data[[]P2POffer]
	Exchange    Data[[]ExchangeOffer]
}

func EmptySnapshot() *MarketSnapshot {
	return &MarketSnapshot{Books: map[string]OrderBook{}, Instruments: map[string]Instrument{}, Fees: map[string]Fee{}, P2P: map[string]Data[[]P2POffer]{}}
}
