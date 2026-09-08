package bybit

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/domain"
)

func nonnegative(s string) (decimal.Decimal, error) {
	d, e := decimal.NewFromString(s)
	if e != nil || d.IsNegative() {
		return decimal.Zero, fmt.Errorf("missing or invalid bybit amount")
	}
	return d, nil
}
func (c *Client) GetTradingFee(ctx context.Context, symbol string) (domain.Fee, error) {
	var result struct {
		Category string
		List     []struct{ Symbol, TakerFeeRate string }
	}
	var f domain.Fee
	if e := c.read(ctx, "/v5/account/fee-rate", url.Values{"category": {"spot"}, "symbol": {symbol}}, nil, &result); e != nil {
		return f, e
	}
	if result.Category != "spot" || len(result.List) != 1 || result.List[0].Symbol != symbol {
		return f, fmt.Errorf("bybit fee pair mismatch")
	}
	rate, e := nonnegative(result.List[0].TakerFeeRate)
	if e != nil || rate.GreaterThanOrEqual(decimal.NewFromInt(1)) {
		return f, fmt.Errorf("invalid taker fee")
	}
	return domain.Fee{Rate: rate, Enabled: true, UpdatedAt: time.Now()}, nil
}
func (c *Client) GetWithdrawalFee(ctx context.Context, asset, network string) (domain.Fee, error) {
	var result struct {
		Rows []struct {
			Coin   string
			Chains []struct{ Chain, ChainWithdraw, WithdrawFee, WithdrawPercentageFee, WithdrawMin, WithdrawMax, MinAccuracy string }
		}
	}
	var f domain.Fee
	if e := c.read(ctx, "/v5/asset/coin/query-info", url.Values{"coin": {asset}}, nil, &result); e != nil {
		return f, e
	}
	for _, row := range result.Rows {
		if row.Coin != asset {
			continue
		}
		for _, ch := range row.Chains {
			if ch.Chain != network {
				continue
			}
			if ch.ChainWithdraw != "1" {
				return f, fmt.Errorf("bybit withdrawal suspended")
			}
			fixed, e := nonnegative(ch.WithdrawFee)
			if e != nil {
				return f, e
			}
			rate, e := nonnegative(ch.WithdrawPercentageFee)
			if e != nil || !rate.IsZero() {
				return f, fmt.Errorf("unsupported bybit percentage withdrawal fee")
			}
			min, e := nonnegative(ch.WithdrawMin)
			if e != nil {
				return f, e
			}
			max := decimal.Zero
			if ch.WithdrawMax != "-1" {
				max, e = nonnegative(ch.WithdrawMax)
				if e != nil || !max.IsPositive() || max.LessThan(min) {
					return f, fmt.Errorf("invalid withdrawal maximum")
				}
			}
			precision, e := strconv.Atoi(ch.MinAccuracy)
			if e != nil || precision < 0 || precision > 18 {
				return f, fmt.Errorf("invalid withdrawal precision")
			}
			return domain.Fee{Fixed: fixed, Min: min, Max: max, Step: decimal.New(1, -int32(precision)), Enabled: true, UpdatedAt: time.Now()}, nil
		}
	}
	return f, fmt.Errorf("bybit withdrawal network unavailable")
}
