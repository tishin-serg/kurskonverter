package app

import (
	"context"
	"time"

	"github.com/tishin-serg/kurskonverter/internal/config"
	"github.com/tishin-serg/kurskonverter/internal/domain"
	"github.com/tishin-serg/kurskonverter/internal/market"
	"github.com/tishin-serg/kurskonverter/internal/provider"
	"github.com/tishin-serg/kurskonverter/internal/provider/bestchange"
	"github.com/tishin-serg/kurskonverter/internal/provider/bybit"
	"github.com/tishin-serg/kurskonverter/internal/provider/demo"
	"github.com/tishin-serg/kurskonverter/internal/provider/httpclient"
	"github.com/tishin-serg/kurskonverter/internal/provider/okx"
	"github.com/tishin-serg/kurskonverter/internal/provider/wallet"
)

type spot interface {
	provider.OrderBookProvider
	GetInstrument(context.Context, string) (domain.Instrument, error)
}

func Workers(c config.Config, s *market.Store) []market.Worker {
	if c.Mode == "demo" {
		return []market.Worker{{Name: "demo", Interval: 5 * time.Second, Fetch: func(context.Context) error {
			v := demo.Snapshot(time.Now())
			s.Update(func(n *domain.MarketSnapshot) { *n = *v })
			return nil
		}}}
	}
	h := httpclient.New()
	bb := bybit.NewAuthenticated(h, c.BybitKey, c.BybitSecret)
	ox := okx.NewAuthenticated(h, c.OKXKey, c.OKXSecret, c.OKXPassphrase)
	wl := wallet.New(h, c.WalletKey, c.WalletWithdrawalFee)
	bc := bestchange.New(h, c.BestChangeKey, c.BestChangeFrom)
	var out []market.Worker
	out = append(out, market.Worker{Name: "bestchange:names", Interval: time.Hour, Fetch: bc.RefreshChangers})
	out = append(out, market.Worker{Name: "okx:GRAM:deposit", Interval: c.FeeRefresh, Fetch: func(ctx context.Context) error {
		v, e := ox.GetGRAMDeposit(ctx)
		if e != nil {
			s.Update(func(n *domain.MarketSnapshot) { delete(n.Fees, "okx:GRAM:deposit") })
			return e
		}
		s.Update(func(n *domain.MarketSnapshot) { n.Fees["okx:GRAM:deposit"] = v })
		return nil
	}})
	for _, x := range []struct {
		key, symbol string
		p           spot
	}{{"bybit:BTCUSDT", "BTCUSDT", bb}, {"okx:BTC-USDT", "BTC-USDT", ox}, {"okx:GRAM-USDT", "GRAM-USDT", ox}} {
		out = append(out, market.Worker{Name: x.key, Interval: c.BookRefresh, Fetch: func(ctx context.Context) error {
			b, e := x.p.GetOrderBook(ctx, x.symbol)
			if e != nil {
				return e
			}
			s.Update(func(n *domain.MarketSnapshot) { n.Books[x.key] = b })
			return nil
		}}, market.Worker{Name: x.key + ":instrument", Interval: c.FeeRefresh, Fetch: func(ctx context.Context) error {
			i, e := x.p.GetInstrument(ctx, x.symbol)
			if e != nil {
				s.Update(func(n *domain.MarketSnapshot) { delete(n.Instruments, x.key) })
				return e
			}
			s.Update(func(n *domain.MarketSnapshot) { n.Instruments[x.key] = i })
			return nil
		}})
	}
	var p2p bybit.BybitP2PSource = bybit.Official{Client: bb, ZeroFeeFallback: c.BybitP2PZeroFeeFallback}
	if c.P2PSource == "web" {
		p2p = bybit.Web{}
	}
	for _, x := range []struct {
		key, asset string
		interval   time.Duration
		p          provider.P2PProvider
	}{{"bybit", "USDT", c.BybitRefresh, p2p}, {"wallet", "GRAM", c.WalletRefresh, wl}} {
		out = append(out, market.Worker{Name: x.key + ":p2p", Interval: x.interval, Fetch: func(ctx context.Context) error {
			v, e := x.p.GetOffers(ctx, provider.P2PRequest{Asset: x.asset, Fiat: "RUB"})
			if e != nil {
				return e
			}
			s.Update(func(n *domain.MarketSnapshot) {
				n.P2P[x.key] = domain.Data[[]domain.P2POffer]{Value: v, UpdatedAt: time.Now()}
			})
			return nil
		}})
	}
	out = append(out, market.Worker{Name: "bestchange", Interval: c.BestRefresh, Fetch: func(ctx context.Context) error {
		v, e := bc.GetRates(ctx, "RUB-BTC")
		if e != nil {
			return e
		}
		s.Update(func(n *domain.MarketSnapshot) {
			n.Exchange = domain.Data[[]domain.ExchangeOffer]{Value: v, UpdatedAt: time.Now()}
		})
		return nil
	}})
	for _, x := range []struct {
		key, asset, network string
		p                   provider.WithdrawalFeeProvider
	}{{"bybit:BTC", "BTC", "BTC", bb}, {"okx:BTC", "BTC", "BTC", ox}, {"wallet:GRAM", "GRAM", "TON", wl}} {
		out = append(out, market.Worker{Name: x.key + ":fee", Interval: c.FeeRefresh, Fetch: func(ctx context.Context) error {
			v, e := x.p.GetWithdrawalFee(ctx, x.asset, x.network)
			if e != nil {
				s.Update(func(n *domain.MarketSnapshot) { delete(n.Fees, x.key) })
				return e
			}
			s.Update(func(n *domain.MarketSnapshot) { n.Fees[x.key] = v })
			return nil
		}})
	}
	for _, x := range []struct {
		key, symbol string
		p           provider.TradingFeeProvider
	}{
		{"bybit:trade", "BTCUSDT", bb}, {"okx:BTC-USDT:trade", "BTC-USDT", ox}, {"okx:GRAM-USDT:trade", "GRAM-USDT", ox},
	} {
		out = append(out, market.Worker{Name: x.key, Interval: c.FeeRefresh, Fetch: func(ctx context.Context) error {
			v, e := x.p.GetTradingFee(ctx, x.symbol)
			if e != nil {
				s.Update(func(n *domain.MarketSnapshot) { delete(n.Fees, x.key) })
				return e
			}
			s.Update(func(n *domain.MarketSnapshot) { n.Fees[x.key] = v })
			return nil
		}})
	}
	return out
}
