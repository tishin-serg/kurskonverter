package money

import (
	"errors"
	"slices"

	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/domain"
)

var ErrLiquidity = errors.New("insufficient liquidity")
var ErrInvalid = errors.New("invalid market data or amount")
var One = decimal.NewFromInt(1)

// CeilRatio rounds an exact rational quotient upward to a positive step.
// QuoRem avoids Div's global precision and underfunding at step boundaries.
func CeilRatio(n, denominator, step decimal.Decimal) decimal.Decimal {
	q, r := n.QuoRem(denominator.Mul(step), 0)
	if r.IsPositive() {
		q = q.Add(One)
	}
	return q.Mul(step)
}
func Ceil(n, step decimal.Decimal) decimal.Decimal { return CeilRatio(n, One, step) }
func CalculateBuyCost(target decimal.Decimal, asks []domain.OrderBookLevel) (decimal.Decimal, error) {
	if !target.IsPositive() {
		return decimal.Zero, ErrInvalid
	}
	levels := slices.Clone(asks)
	for _, l := range levels {
		if !l.Price.IsPositive() || !l.Amount.IsPositive() {
			return decimal.Zero, ErrInvalid
		}
	}
	slices.SortFunc(levels, func(a, b domain.OrderBookLevel) int { return a.Price.Cmp(b.Price) })
	left, cost := target, decimal.Zero
	for _, l := range levels {
		take := decimal.Min(left, l.Amount)
		cost = cost.Add(take.Mul(l.Price))
		left = left.Sub(take)
		if left.IsZero() {
			return cost, nil
		}
	}
	return decimal.Zero, ErrLiquidity
}

// SellForQuote inverts bid execution; fee is charged in the received quote asset.
func SellForQuote(target, rate, step decimal.Decimal, bids []domain.OrderBookLevel) (decimal.Decimal, error) {
	if !target.IsPositive() || rate.IsNegative() || !rate.LessThan(One) || !step.IsPositive() {
		return decimal.Zero, ErrInvalid
	}
	levels := slices.Clone(bids)
	for _, l := range levels {
		if !l.Price.IsPositive() || !l.Amount.IsPositive() {
			return decimal.Zero, ErrInvalid
		}
	}
	slices.SortFunc(levels, func(a, b domain.OrderBookLevel) int { return b.Price.Cmp(a.Price) })
	left, base := target, decimal.Zero
	for _, l := range levels {
		net := l.Price.Mul(One.Sub(rate))
		capacity := l.Amount.Mul(net)
		if capacity.GreaterThanOrEqual(left) {
			base = CeilRatio(base.Mul(net).Add(left), net, step)
			return verifySell(base, target, rate, levels)
		}
		base = base.Add(l.Amount)
		left = left.Sub(capacity)
	}
	return decimal.Zero, ErrLiquidity
}
func verifySell(base, target, rate decimal.Decimal, levels []domain.OrderBookLevel) (decimal.Decimal, error) {
	left, received := base, decimal.Zero
	for _, l := range levels {
		take := decimal.Min(left, l.Amount)
		received = received.Add(take.Mul(l.Price).Mul(One.Sub(rate)))
		left = left.Sub(take)
	}
	if left.IsPositive() || received.LessThan(target) {
		return decimal.Zero, ErrLiquidity
	}
	return base, nil
}
func SelectP2P(amount decimal.Decimal, asset string, offers []domain.P2POffer, f domain.Filter) (domain.P2POffer, decimal.Decimal, error) {
	var best domain.P2POffer
	cost := decimal.Zero
	if !amount.IsPositive() {
		return best, cost, ErrInvalid
	}
	for _, o := range offers {
		needed := amount
		if o.AssetStep.IsPositive() {
			needed = Ceil(amount, o.AssetStep)
		}
		if o.Asset != asset || o.Fiat != "RUB" || !o.Price.IsPositive() || o.MinFiat.IsNegative() || o.MaxFiat.LessThan(o.MinFiat) || o.CompletionRate.LessThan(f.MinCompletionRate) || o.CompletionRate.GreaterThan(decimal.NewFromInt(100)) || o.OrdersCount < f.MinOrdersCount || o.AvailableAsset.LessThan(amount) {
			continue
		}
		if !domain.PaymentMatches(f.PaymentMethod, o.PaymentMethods) {
			continue
		}
		if o.AvailableAsset.LessThan(needed) {
			continue
		}
		rub := Ceil(needed.Mul(o.Price), decimal.New(1, -2))
		if rub.LessThan(o.MinFiat) || rub.GreaterThan(o.MaxFiat) {
			continue
		}
		if cost.IsZero() || rub.LessThan(cost) {
			best = o
			cost = rub
		}
	}
	if cost.IsZero() {
		return best, cost, ErrLiquidity
	}
	return best, cost, nil
}
