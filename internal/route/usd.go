package route

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/domain"
	"github.com/tishin-serg/kurskonverter/internal/money"
)

type USDQuote struct {
	TargetUSD, ReceivedUSD, RUBRequired, USDTRequired, USDTBought, EffectiveRate decimal.Decimal
	Buy, Sell                                                                    domain.P2POffer
	CalculatedAt                                                                 time.Time
	Fallback                                                                     bool
}

// Buy USDT for RUB and sell it on the same exchange to receive USD at TBC.
// Both advertisements must cover the entire corresponding leg; no split orders.
func CalculateUSD(target decimal.Decimal, s *domain.MarketSnapshot, f domain.Filter, now time.Time) (USDQuote, error) {
	var best USDQuote
	if !target.IsPositive() || s == nil || !target.Mod(decimal.New(1, -2)).IsZero() {
		return best, fmt.Errorf("введите USD с точностью до цента")
	}
	rub, usd := s.P2P["bybit"], s.P2P["bybit:usd"]
	if !fresh(rub.UpdatedAt, now, 60*time.Second) || !fresh(usd.UpdatedAt, now, 60*time.Second) {
		return best, fmt.Errorf("нет свежих P2P-данных. Повторите расчёт через несколько секунд")
	}
	if method, ok := f.PaymentByProvider["bybit"]; ok {
		f.PaymentMethod = method
	}
	buyOffers := make([]domain.P2POffer, 0, len(rub.Value))
	for _, a := range rub.Value {
		if !a.TakerSells && (!f.DisallowFallback || !a.FeeFallback) {
			buyOffers = append(buyOffers, a)
		}
	}
	for _, sell := range usd.Value {
		if !sell.TakerSells || sell.Fiat != "USD" || sell.Asset != "USDT" || !domain.PaymentMatches(domain.BybitTBCPayment, sell.PaymentMethods) || !sell.Price.IsPositive() || !sell.AssetStep.IsPositive() || sell.MinFiat.IsNegative() || sell.MaxFiat.LessThan(sell.MinFiat) || sell.CompletionRate.LessThan(f.MinCompletionRate) || sell.CompletionRate.GreaterThan(decimal.NewFromInt(100)) || sell.OrdersCount < f.MinOrdersCount || (f.DisallowFallback && sell.FeeFallback) {
			continue
		}
		needed := money.CeilRatio(target, sell.Price, sell.AssetStep)
		received := needed.Mul(sell.Price).Truncate(2)
		if needed.GreaterThan(sell.AvailableAsset) || received.LessThan(target) || received.LessThan(sell.MinFiat) || received.GreaterThan(sell.MaxFiat) {
			continue
		}
		buy, cost, err := money.SelectP2P(needed, "USDT", buyOffers, f)
		if err != nil {
			continue
		}
		if best.RUBRequired.IsZero() || cost.LessThan(best.RUBRequired) {
			bought := needed
			if buy.AssetStep.IsPositive() {
				bought = money.Ceil(needed, buy.AssetStep)
			}
			best = USDQuote{TargetUSD: target, ReceivedUSD: received, RUBRequired: cost, USDTRequired: needed, USDTBought: bought, EffectiveRate: cost.DivRound(received, 8), Buy: buy, Sell: sell, CalculatedAt: now, Fallback: buy.FeeFallback || sell.FeeFallback}
		}
	}
	if best.RUBRequired.IsZero() {
		return best, fmt.Errorf("нет пары заявок под эту сумму: проверьте банк оплаты, лимиты и фильтры продавцов. Зачисление — только TBC Bank")
	}
	return best, nil
}
