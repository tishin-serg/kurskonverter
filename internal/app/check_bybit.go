package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/config"
	"github.com/tishin-serg/kurskonverter/internal/domain"
	"github.com/tishin-serg/kurskonverter/internal/provider"
	"github.com/tishin-serg/kurskonverter/internal/provider/bybit"
	"github.com/tishin-serg/kurskonverter/internal/provider/httpclient"
	"github.com/tishin-serg/kurskonverter/internal/route"
	"github.com/tishin-serg/kurskonverter/internal/telegram"
)

// CheckBybit reads only market/configuration data and never sends Telegram
// messages, creates orders, transfers funds or requests withdrawals.
func CheckBybit(ctx context.Context, c config.Config, out io.Writer) error {
	client := bybit.NewAuthenticated(httpclient.New(), c.BybitKey, c.BybitSecret)
	s := domain.EmptySnapshot()
	var e error
	var failures []error
	report := func(stage string, err error) {
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", stage, err))
			fmt.Fprintf(out, "%s: FAILED\n", stage)
		} else {
			fmt.Fprintf(out, "%s: OK\n", stage)
		}
	}
	if c.BybitKey == "" || c.BybitSecret == "" {
		return bybit.ErrCredentials
	}
	if c.P2PSource != "official" {
		return fmt.Errorf("check-bybit requires BYBIT_P2P_SOURCE=official")
	}
	s.Books["bybit:BTCUSDT"], e = client.GetOrderBook(ctx, "BTCUSDT")
	if e != nil {
		return fmt.Errorf("orderbook: %w", e)
	}
	fmt.Fprintln(out, "Orderbook: OK")
	s.Instruments["bybit:BTCUSDT"], e = client.GetInstrument(ctx, "BTCUSDT")
	if e != nil {
		return fmt.Errorf("instrument: %w", e)
	}
	fmt.Fprintln(out, "Instrument: OK")
	s.Fees["bybit:trade"], e = client.GetTradingFee(ctx, "BTCUSDT")
	report("Trading fee", e)
	s.Fees["bybit:BTC"], e = client.GetWithdrawalFee(ctx, "BTC", "BTC")
	report("Withdrawal conditions", e)
	offers, e := (bybit.Official{Client: client, ZeroFeeFallback: c.BybitP2PZeroFeeFallback}).GetOffers(ctx, provider.P2PRequest{Asset: "USDT", Fiat: "RUB"})
	report("P2P", e)
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	s.P2P["bybit"] = domain.Data[[]domain.P2POffer]{Value: offers, UpdatedAt: time.Now()}
	fmt.Fprintf(out, "P2P: %d validated offers\n", len(offers))
	// Refresh the book after slow P2P pagination; do not relabel the old book.
	s.Books["bybit:BTCUSDT"], e = client.GetOrderBook(ctx, "BTCUSDT")
	if e != nil {
		return e
	}
	r := route.Engine{Routes: []route.Calculator{route.DefaultEngine().Routes[1]}}.Calculate(ctx, decimal.New(1, -2), s, c.Filter, time.Now())
	if len(r.Quotes) == 0 {
		return fmt.Errorf("no quote: %s", r.Unavailable["Bybit P2P → USDT → BTC"])
	}
	fmt.Fprintln(out, telegram.Summary(decimal.New(1, -2), r, false))
	return nil
}
