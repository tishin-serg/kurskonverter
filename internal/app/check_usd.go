package app

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/config"
	"github.com/tishin-serg/kurskonverter/internal/market"
	"github.com/tishin-serg/kurskonverter/internal/route"
)

func CheckUSD(ctx context.Context, c config.Config, out io.Writer) error {
	s := market.New()
	for _, w := range Workers(c, s) {
		if w.Name == "bybit:p2p" || w.Name == "bybit:usd:p2p" || w.Name == "demo" {
			if err := w.Fetch(ctx); err != nil {
				return fmt.Errorf("%s: %w", w.Name, err)
			}
		}
	}
	q, err := route.CalculateUSD(decimal.NewFromInt(100), s.Read(), c.Filter, time.Now())
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "USD/TBC: OK; receive=%s USD; cost=%s RUB; buy=%s RUB/USDT; sell=%s USD/USDT; fallback=%t\n", q.ReceivedUSD.String(), q.RUBRequired.String(), q.Buy.Price.String(), q.Sell.Price.String(), q.Fallback)
	return nil
}
