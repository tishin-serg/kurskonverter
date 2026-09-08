package okx

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/tishin-serg/kurskonverter/internal/domain"
	"github.com/tishin-serg/kurskonverter/internal/provider/httpclient"
)

type Client struct {
	HTTP                    *httpclient.Client
	BaseURL                 string
	key, secret, passphrase string
}

func New(h *httpclient.Client) *Client { return &Client{HTTP: h, BaseURL: "https://www.okx.com"} }
func (c *Client) GetOrderBook(ctx context.Context, symbol string) (domain.OrderBook, error) {
	var raw struct {
		Code string
		Data []struct {
			Asks, Bids [][]string
			Ts         string
		}
	}
	var b domain.OrderBook
	if e := c.HTTP.Get(ctx, c.BaseURL+"/api/v5/market/books?sz=400&instId="+url.QueryEscape(symbol), &raw); e != nil {
		return b, e
	}
	if raw.Code != "0" || len(raw.Data) != 1 {
		return b, fmt.Errorf("okx book unavailable")
	}
	r := raw.Data[0]
	var e error
	b.Asks, e = httpclient.Levels(r.Asks)
	if e != nil {
		return b, e
	}
	b.Bids, e = httpclient.Levels(r.Bids)
	if e != nil {
		return b, e
	}
	b.UpdatedAt, e = httpclient.Timestamp(r.Ts)
	return b, e
}
func (c *Client) GetInstrument(ctx context.Context, symbol string) (domain.Instrument, error) {
	var raw struct {
		Code string
		Data []struct{ InstID, State, BaseCcy, QuoteCcy, LotSz, TickSz, MinSz, MaxMktSz, MaxMktAmt string }
	}
	var i domain.Instrument
	if e := c.HTTP.Get(ctx, c.BaseURL+"/api/v5/public/instruments?instType=SPOT&instId="+url.QueryEscape(symbol), &raw); e != nil {
		return i, e
	}
	if raw.Code != "0" || len(raw.Data) != 1 {
		return i, fmt.Errorf("okx instrument unavailable")
	}
	r := raw.Data[0]
	if r.InstID != symbol || r.State != "live" {
		return i, fmt.Errorf("okx instrument not live")
	}
	i.Base = r.BaseCcy
	i.Quote = r.QuoteCcy
	var e error
	i.Step, e = httpclient.Positive(r.LotSz)
	if e != nil {
		return i, e
	}
	i.Tick, e = httpclient.Positive(r.TickSz)
	if e != nil {
		return i, e
	}
	i.MinBase, e = httpclient.Positive(r.MinSz)
	if e != nil {
		return i, e
	}
	i.MaxBase, e = httpclient.Positive(r.MaxMktSz)
	if e != nil {
		return i, e
	}
	i.MaxQuote, e = httpclient.Positive(r.MaxMktAmt)
	i.UpdatedAt = time.Now()
	return i, e
}
