package provider

import (
	"context"
	"errors"

	"github.com/tishin-serg/kurskonverter/internal/domain"
)

var ErrNotConfigured = errors.New("TODO: verified API contract and credentials required")

type P2PRequest struct {
	Asset, Fiat, PaymentMethod string
	SellAsset                  bool
}
type OrderBookProvider interface {
	GetOrderBook(context.Context, string) (domain.OrderBook, error)
}
type P2PProvider interface {
	GetOffers(context.Context, P2PRequest) ([]domain.P2POffer, error)
}
type WithdrawalFeeProvider interface {
	GetWithdrawalFee(context.Context, string, string) (domain.Fee, error)
}

// TradingFeeProvider returns the pair-specific taker rate with the normalized
// convention: buy fee in base, sell fee in quote. Other fee assets must fail.
type TradingFeeProvider interface {
	GetTradingFee(context.Context, string) (domain.Fee, error)
}
type RatesProvider interface {
	GetRates(context.Context, string) ([]domain.ExchangeOffer, error)
}
