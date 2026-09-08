package route

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/domain"
	"github.com/tishin-serg/kurskonverter/internal/money"
)

type Calculator interface {
	ID() string
	Calculate(context.Context, decimal.Decimal, *domain.MarketSnapshot, domain.Filter, time.Time) (domain.Quote, error)
}
type Engine struct{ Routes []Calculator }
type Result struct {
	Equivalent  *domain.FiatEquivalent `json:",omitempty"`
	Quotes      []domain.Quote
	Unavailable map[string]string
}

func (e Engine) Calculate(ctx context.Context, target decimal.Decimal, s *domain.MarketSnapshot, f domain.Filter, now time.Time) Result {
	r := Result{Unavailable: map[string]string{}}
	if s == nil || !target.IsPositive() {
		r.Unavailable["input"] = "invalid request"
		return r
	}
	for _, c := range e.Routes {
		if ctx.Err() != nil {
			r.Unavailable[c.ID()] = "cancelled"
			continue
		}
		q, err := c.Calculate(ctx, target, s, f, now)
		if err != nil {
			r.Unavailable[c.ID()] = err.Error()
		} else {
			r.Quotes = append(r.Quotes, q)
		}
	}
	slices.SortFunc(r.Quotes, func(a, b domain.Quote) int {
		if n := a.RUBRequired.Cmp(b.RUBRequired); n != 0 {
			return n
		}
		return stringsCompare(a.RouteID, b.RouteID)
	})
	if len(r.Quotes) > 0 {
		best := r.Quotes[0].RUBRequired
		for i := range r.Quotes {
			q := &r.Quotes[i]
			q.DifferenceRub = q.RUBRequired.Sub(best)
			q.DifferencePercent = q.DifferenceRub.Mul(decimal.NewFromInt(100)).DivRound(best, 8)
		}
	}
	return r
}
func stringsCompare(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
func fresh(t, now time.Time, ttl time.Duration) bool {
	return !t.IsZero() && !t.After(now.Add(time.Second)) && now.Sub(t) <= ttl
}

type Step interface {
	Backward(decimal.Decimal, *domain.MarketSnapshot, time.Time) (decimal.Decimal, domain.QuoteStep, bool, error)
}

// Path steps are declared in forward order and evaluated in reverse.
type Path struct {
	Name, P2PKey, Asset string
	Steps               []Step
}

func (p Path) ID() string { return p.Name }
func (p Path) Calculate(ctx context.Context, target decimal.Decimal, s *domain.MarketSnapshot, f domain.Filter, now time.Time) (domain.Quote, error) {
	if method, ok := f.PaymentByProvider[p.P2PKey]; ok {
		f.PaymentMethod = method
	}
	q := domain.Quote{RouteID: p.Name, TargetBTC: target, BTCReceived: target, CalculatedAt: now}
	needed := target
	steps := make([]domain.QuoteStep, len(p.Steps)+1)
	for i := len(p.Steps) - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return q, err
		}
		n, st, degraded, err := p.Steps[i].Backward(needed, s, now)
		if err != nil {
			return q, fmt.Errorf("%s: %w", p.Name, err)
		}
		needed = n
		steps[i+1] = st
		q.IsDegraded = q.IsDegraded || degraded
		if degraded && st.Type == "withdrawal" && st.Provider == "Wallet" {
			q.Warnings = append(q.Warnings, "Вывод Wallet: резервная комиссия "+st.Fee.String()+" GRAM по опубликованному тарифу; проверьте её в Wallet перед переводом.")
		}
	}
	data := s.P2P[p.P2PKey]
	if !fresh(data.UpdatedAt, now, 60*time.Second) {
		return q, fmt.Errorf("P2P unavailable or stale")
	}
	if f.PaymentMethod != "" && !slices.ContainsFunc(data.Value, func(o domain.P2POffer) bool {
		return domain.PaymentMatches(f.PaymentMethod, o.PaymentMethods)
	}) {
		return q, fmt.Errorf("P2P payment unavailable: %s", f.PaymentMethod)
	}
	offer, rub, err := money.SelectP2P(needed, p.Asset, data.Value, f)
	if err != nil {
		return q, fmt.Errorf("P2P: %w", err)
	}
	if offer.AssetStep.IsPositive() {
		needed = money.Ceil(needed, offer.AssetStep)
	}
	steps[0] = domain.QuoteStep{Type: "p2p", FromAsset: "RUB", ToAsset: p.Asset, Input: rub, Output: needed, Price: offer.Price, Provider: offer.Provider, Description: offer.MerchantName}
	steps[0].MerchantName = offer.MerchantName
	steps[0].OfferID = offer.OfferID
	steps[0].PaymentMethods = slices.Clone(offer.PaymentMethods)
	steps[0].MinFiat, steps[0].MaxFiat = offer.MinFiat, offer.MaxFiat
	q.Steps = steps
	if offer.FeeFallback {
		q.IsDegraded = true
		q.Warnings = append(q.Warnings, "P2P: комиссия тейкера 0% задана в настройках; API не вернул ставку. Проверьте комиссию перед сделкой.")
	}
	q.RUBRequired = rub
	if len(p.Steps) > 0 {
		q.BTCReceived = steps[len(steps)-1].Output
		if q.BTCReceived.GreaterThan(target) {
			q.Warnings = append(q.Warnings, "Получение округлено вверх до точности вывода")
		}
	}
	if q.IsDegraded {
		q.Warnings = append(q.Warnings, "Использованы явно заданные резервные комиссии")
	}
	return q, nil
}

type Withdrawal struct{ Key, Asset, Provider string }

type Deposit struct{ Key, Asset, Provider string }

func (d Deposit) Backward(target decimal.Decimal, s *domain.MarketSnapshot, now time.Time) (decimal.Decimal, domain.QuoteStep, bool, error) {
	f, ok := s.Fees[d.Key]
	if !ok || !f.Enabled || !fresh(f.UpdatedAt, now, 15*time.Minute) || target.LessThan(f.Min) {
		return decimal.Zero, domain.QuoteStep{}, false, fmt.Errorf("deposit suspended/stale/below minimum")
	}
	return target, domain.QuoteStep{Type: "deposit", FromAsset: d.Asset, ToAsset: d.Asset, Input: target, Output: target, Provider: d.Provider, Description: "Сеть TON; проверьте адрес и memo/tag депозита"}, false, nil
}

func (w Withdrawal) Backward(target decimal.Decimal, s *domain.MarketSnapshot, now time.Time) (decimal.Decimal, domain.QuoteStep, bool, error) {
	fee, ok := s.Fees[w.Key]
	if !ok || !fee.Enabled || !fresh(fee.UpdatedAt, now, 15*time.Minute) || fee.Fixed.IsNegative() || !fee.Rate.IsZero() || !fee.Step.IsPositive() {
		return decimal.Zero, domain.QuoteStep{}, false, fmt.Errorf("withdrawal unavailable/stale/unsupported fee")
	}
	output := money.Ceil(target, fee.Step)
	if output.LessThan(fee.Min) {
		return decimal.Zero, domain.QuoteStep{}, false, fmt.Errorf("below withdrawal minimum")
	}
	input := output.Add(fee.Fixed)
	if fee.Max.IsPositive() && input.GreaterThan(fee.Max) {
		return decimal.Zero, domain.QuoteStep{}, false, fmt.Errorf("above withdrawal maximum")
	}
	return input, domain.QuoteStep{Type: "withdrawal", FromAsset: w.Asset, ToAsset: w.Asset, Input: input, Output: output, Fee: fee.Fixed, Provider: w.Provider}, fee.Fallback, nil
}

type Trade struct {
	Key, FeeKey, Provider string
	Buy                   bool
}

func (t Trade) Backward(target decimal.Decimal, s *domain.MarketSnapshot, now time.Time) (decimal.Decimal, domain.QuoteStep, bool, error) {
	book := s.Books[t.Key]
	inst := s.Instruments[t.Key]
	fee, ok := s.Fees[t.FeeKey]
	if !fresh(book.UpdatedAt, now, 15*time.Second) || !fresh(inst.UpdatedAt, now, 15*time.Minute) || !ok || !fee.Enabled || !fresh(fee.UpdatedAt, now, 15*time.Minute) || fee.Rate.IsNegative() || !fee.Rate.LessThan(money.One) || !fee.Fixed.IsZero() || !inst.Step.IsPositive() || !inst.Tick.IsPositive() {
		return decimal.Zero, domain.QuoteStep{}, false, fmt.Errorf("market/fee unavailable or stale")
	}
	for _, levels := range [][]domain.OrderBookLevel{book.Asks, book.Bids} {
		for _, l := range levels {
			if !l.Price.Mod(inst.Tick).IsZero() {
				return decimal.Zero, domain.QuoteStep{}, false, fmt.Errorf("price off tick")
			}
		}
	}
	st := domain.QuoteStep{Type: "spot", Provider: t.Provider}
	var input, base, quote decimal.Decimal
	var err error
	if t.Buy {
		base = money.CeilRatio(target, money.One.Sub(fee.Rate), inst.Step)
		quote, err = money.CalculateBuyCost(base, book.Asks)
		if err == nil && inst.QuoteStep.IsPositive() {
			quote = money.Ceil(quote, inst.QuoteStep)
		}
		input = quote
		st.FromAsset = inst.Quote
		st.ToAsset = inst.Base
		st.Output = base.Mul(money.One.Sub(fee.Rate))
		st.Fee = base.Mul(fee.Rate)
	} else {
		base, err = money.SellForQuote(target, fee.Rate, inst.Step, book.Bids)
		input = base
		quote = target
		st.FromAsset = inst.Base
		st.ToAsset = inst.Quote
		st.Output = target
		st.Description = "Продажа по bids; комиссия удерживается в quote"
		if err == nil {
			gross := decimal.Zero
			left := base
			levels := slices.Clone(book.Bids)
			slices.SortFunc(levels, func(a, b domain.OrderBookLevel) int { return b.Price.Cmp(a.Price) })
			for _, l := range levels {
				take := decimal.Min(left, l.Amount)
				gross = gross.Add(take.Mul(l.Price))
				left = left.Sub(take)
			}
			st.Fee = gross.Mul(fee.Rate)
			st.Output = gross.Sub(st.Fee)
			quote = gross
		}
	}
	if err != nil {
		return decimal.Zero, st, false, err
	}
	if base.LessThan(inst.MinBase) || quote.LessThan(inst.MinQuote) {
		return decimal.Zero, st, false, fmt.Errorf("below market minimum")
	}
	if (inst.MaxBase.IsPositive() && base.GreaterThan(inst.MaxBase)) || (inst.MaxQuote.IsPositive() && quote.GreaterThan(inst.MaxQuote)) {
		return decimal.Zero, st, false, fmt.Errorf("above market maximum")
	}
	st.Input = input
	return input, st, fee.Fallback, nil
}

type BestChange struct{}

func (BestChange) ID() string { return "BestChange" }
func (BestChange) Calculate(_ context.Context, target decimal.Decimal, s *domain.MarketSnapshot, f domain.Filter, now time.Time) (domain.Quote, error) {
	q := domain.Quote{RouteID: "BestChange", TargetBTC: target, BTCReceived: target, CalculatedAt: now}
	if !fresh(s.Exchange.UpdatedAt, now, 60*time.Second) {
		return q, fmt.Errorf("BestChange unavailable or stale")
	}
	for _, o := range s.Exchange.Value {
		if !o.Enabled || !o.Rate.IsPositive() || o.MinFiat.IsNegative() || o.MaxFiat.LessThan(o.MinFiat) || o.ReserveBTC.LessThan(target) || !domain.PaymentMatches(f.PaymentMethod, []string{o.PaymentMethod}) {
			continue
		}
		rub := money.Ceil(target.Mul(o.Rate), decimal.New(1, -2))
		if rub.LessThan(o.MinFiat) || rub.GreaterThan(o.MaxFiat) {
			continue
		}
		if q.RUBRequired.IsZero() || rub.LessThan(q.RUBRequired) {
			q.RUBRequired = rub
			q.Warnings = slices.Clone(o.Warnings)
			q.Steps = []domain.QuoteStep{{Type: "exchange", FromAsset: "RUB", ToAsset: "BTC", Input: rub, Output: target, Price: o.Rate, Provider: o.Exchanger, Description: o.PaymentMethod + "; курс за BTC на внешний адрес, без дополнительных комиссий обменника по данным API; " + o.URL}}
		}
	}
	if q.RUBRequired.IsZero() {
		return q, money.ErrLiquidity
	}
	return q, nil
}
func DefaultEngine() Engine {
	return Engine{Routes: []Calculator{
		BestChange{},
		Path{Name: "Bybit P2P → USDT → BTC", P2PKey: "bybit", Asset: "USDT", Steps: []Step{Trade{Key: "bybit:BTCUSDT", FeeKey: "bybit:trade", Provider: "Bybit", Buy: true}, Withdrawal{Key: "bybit:BTC", Asset: "BTC", Provider: "Bybit"}}},
		Path{Name: "Wallet → GRAM → OKX", P2PKey: "wallet", Asset: "GRAM", Steps: []Step{Withdrawal{Key: "wallet:GRAM", Asset: "GRAM", Provider: "Wallet"}, Deposit{Key: "okx:GRAM:deposit", Asset: "GRAM", Provider: "OKX"}, Trade{Key: "okx:GRAM-USDT", FeeKey: "okx:GRAM-USDT:trade", Provider: "OKX"}, Trade{Key: "okx:BTC-USDT", FeeKey: "okx:BTC-USDT:trade", Provider: "OKX", Buy: true}, Withdrawal{Key: "okx:BTC", Asset: "BTC", Provider: "OKX"}}},
	}}
}
