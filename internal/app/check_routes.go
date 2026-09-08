package app

import (
	"context"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/config"
	"github.com/tishin-serg/kurskonverter/internal/market"
	"github.com/tishin-serg/kurskonverter/internal/route"
	"github.com/tishin-serg/kurskonverter/internal/telegram"
)

// CheckRoutes uses the production workers, without Telegram polling or writes.
func CheckRoutes(ctx context.Context, c config.Config, out io.Writer) error {
	s := market.New()
	workers := Workers(c, s)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var reports []string
	for _, w := range workers {
		if w.Name == "bestchange:names" {
			continue
		}
		wg.Go(func() {
			err := w.Fetch(ctx)
			line := w.Name + ": OK"
			if err != nil {
				line = w.Name + ": " + err.Error()
			}
			mu.Lock()
			reports = append(reports, line)
			mu.Unlock()
		})
	}
	wg.Wait()
	sort.Strings(reports)
	for _, v := range reports {
		fmt.Fprintln(out, v)
	}
	// Books must be fresh after paginated API calls.
	for _, w := range workers {
		if w.Name == "bybit:BTCUSDT" || w.Name == "okx:BTC-USDT" || w.Name == "okx:GRAM-USDT" {
			wg.Go(func() { _ = w.Fetch(ctx) })
		}
	}
	wg.Wait()
	amount := decimal.New(1, -2)
	result := route.DefaultEngine().Calculate(ctx, amount, s.Read(), c.Filter, time.Now())
	fmt.Fprintln(out, telegram.Summary(amount, result, false))
	for name, reason := range result.Unavailable {
		fmt.Fprintf(out, "%s: %s\n", name, reason)
	}
	if len(result.Quotes) != 3 {
		return fmt.Errorf("only %d of 3 routes available", len(result.Quotes))
	}
	return nil
}
