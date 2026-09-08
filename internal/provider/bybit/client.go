package bybit

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/tishin-serg/kurskonverter/internal/domain"
	"github.com/tishin-serg/kurskonverter/internal/provider"
	"github.com/tishin-serg/kurskonverter/internal/provider/httpclient"
)

type Client struct {
	HTTP        *httpclient.Client
	BaseURL     string
	key, secret string
}

func New(h *httpclient.Client) *Client { return &Client{HTTP: h, BaseURL: "https://api.bybit.com"} }
func (c *Client) GetOrderBook(ctx context.Context, symbol string) (domain.OrderBook, error) {
	var raw struct {
		RetCode int
		Result  struct {
			A, B [][]string
			Ts   int64
			S    string
		}
	}
	var b domain.OrderBook
	if e := c.HTTP.Get(ctx, c.BaseURL+"/v5/market/orderbook?category=spot&limit=200&symbol="+url.QueryEscape(symbol), &raw); e != nil {
		return b, e
	}
	if raw.RetCode != 0 || raw.Result.S != symbol || raw.Result.Ts <= 0 {
		return b, fmt.Errorf("bybit rejected book")
	}
	var e error
	b.Asks, e = httpclient.Levels(raw.Result.A)
	if e != nil {
		return b, e
	}
	b.Bids, e = httpclient.Levels(raw.Result.B)
	b.UpdatedAt = time.UnixMilli(raw.Result.Ts)
	return b, e
}
func (c *Client) GetInstrument(ctx context.Context, symbol string) (domain.Instrument, error) {
	var raw struct {
		RetCode int
		Result  struct {
			List []struct {
				Symbol, Status, BaseCoin, QuoteCoin string
				LotSizeFilter                       struct{ BasePrecision, QuotePrecision, MinOrderQty, MinOrderAmt, MaxMarketOrderQty, MaxOrderAmt string }
				PriceFilter                         struct{ TickSize string }
			}
		}
	}
	var i domain.Instrument
	if e := c.HTTP.Get(ctx, c.BaseURL+"/v5/market/instruments-info?category=spot&symbol="+url.QueryEscape(symbol), &raw); e != nil {
		return i, e
	}
	if raw.RetCode != 0 || len(raw.Result.List) != 1 {
		return i, fmt.Errorf("bybit instrument unavailable")
	}
	r := raw.Result.List[0]
	if r.Symbol != symbol || r.Status != "Trading" {
		return i, fmt.Errorf("bybit instrument not trading")
	}
	var e error
	i.Base = r.BaseCoin
	i.Quote = r.QuoteCoin
	i.Step, e = httpclient.Positive(r.LotSizeFilter.BasePrecision)
	if e != nil {
		return i, e
	}
	i.Tick, e = httpclient.Positive(r.PriceFilter.TickSize)
	if e != nil {
		return i, e
	}
	i.MinBase, e = httpclient.Positive(r.LotSizeFilter.MinOrderQty)
	if e != nil {
		return i, e
	}
	i.MinQuote, e = httpclient.Positive(r.LotSizeFilter.MinOrderAmt)
	if e != nil {
		return i, e
	}
	i.QuoteStep, e = httpclient.Positive(r.LotSizeFilter.QuotePrecision)
	if e != nil {
		return i, e
	}
	i.MaxBase, e = httpclient.Positive(r.LotSizeFilter.MaxMarketOrderQty)
	if e != nil {
		return i, e
	}
	i.MaxQuote, e = httpclient.Positive(r.LotSizeFilter.MaxOrderAmt)
	if e != nil {
		return i, e
	}
	i.UpdatedAt = time.Now()
	return i, e
}

type BybitP2PSource interface{ provider.P2PProvider }

// Web isolates the undocumented integration. No guessed endpoint is called.
type Web struct{}

func (Web) GetOffers(context.Context, provider.P2PRequest) ([]domain.P2POffer, error) {
	return nil, provider.ErrNotConfigured
}
