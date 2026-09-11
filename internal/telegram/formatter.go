package telegram

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/domain"
	"github.com/tishin-serg/kurskonverter/internal/route"
)

var amountPattern = regexp.MustCompile(`(?i)^([0-9]{1,8})(?:[.,]([0-9]{1,8}))?(?:\s+BTC)?$`)

func routeLabel(id string) string {
	switch id {
	case "Bybit P2P → USDT → BTC":
		return "Bybit"
	case "Wallet → GRAM → OKX":
		return "Wallet → OKX"
	default:
		return id
	}
}

func ParseAmount(text string) (decimal.Decimal, error) {
	text = strings.TrimSpace(text)
	if len(text) > 40 || !amountPattern.MatchString(text) {
		return decimal.Zero, fmt.Errorf("введите положительное число BTC, не более 8 знаков после запятой: 0.01 BTC")
	}
	text = strings.TrimSpace(strings.TrimSuffix(strings.ToUpper(text), "BTC"))
	n, e := decimal.NewFromString(strings.ReplaceAll(text, ",", "."))
	if e != nil || !n.IsPositive() || n.GreaterThan(decimal.NewFromInt(21000000)) {
		return decimal.Zero, fmt.Errorf("недопустимое количество BTC")
	}
	return n, nil
}
func Summary(target decimal.Decimal, r route.Result, demo bool) string {
	var b strings.Builder
	if demo {
		b.WriteString("🧪 ДЕМО — вымышленные данные\n\n")
	}
	fmt.Fprintf(&b, "💰 Получить %s BTC\n\n", target.StringFixed(8))
	b.WriteString(equivalentText(r.Equivalent))
	medals := []string{"🥇", "🥈", "🥉"}
	for i, q := range r.Quotes {
		label := "•"
		if i < len(medals) {
			label = medals[i]
		}
		fmt.Fprintf(&b, "%s %s — %s ₽\n", label, routeLabel(q.RouteID), q.RUBRequired.StringFixed(2))
		if q.RouteID == "BestChange" && len(q.Steps) > 0 {
			fmt.Fprintf(&b, "Обменник: %s\n", q.Steps[0].Provider)
		}
		if i > 0 {
			fmt.Fprintf(&b, "+%s ₽ / +%s%%\n", q.DifferenceRub.StringFixed(2), q.DifferencePercent.StringFixed(2))
		}
		if q.IsDegraded {
			b.WriteString("⚠️ Резервные данные\n")
			for _, warning := range q.Warnings {
				if strings.HasPrefix(warning, "P2P:") || strings.HasPrefix(warning, "Вывод Wallet:") {
					fmt.Fprintln(&b, warning)
				}
			}
		}
		b.WriteString("\n")
	}
	if len(r.Quotes) == 0 {
		b.WriteString("Нет маршрутов с подходящими актуальными данными.\n")
	}
	if len(r.Unavailable) > 0 {
		names := make([]string, 0, len(r.Unavailable))
		for name := range r.Unavailable {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(&b, "⚠️ %s: %s\n", name, unavailableReason(r.Unavailable[name]))
		}
	}
	if len(r.Quotes) > 0 {
		fmt.Fprintf(&b, "Расчёт: %s UTC\nОценка по стакану; цена может измениться до сделки.", r.Quotes[0].CalculatedAt.UTC().Format("15:04:05"))
	}
	return b.String()
}
func Breakdown(q domain.Quote, demo bool) string {
	var b strings.Builder
	if demo {
		b.WriteString("🧪 ДЕМО — вымышленные данные\n")
	}
	fmt.Fprintf(&b, "%s\nПолучить: %s BTC\n\n", q.RouteID, q.TargetBTC.StringFixed(8))
	for i, s := range q.Steps {
		kind := map[string]string{"p2p": "Покупка P2P", "spot": "Обмен на бирже", "withdrawal": "Вывод", "deposit": "Зачисление", "exchange": "Обменник"}[s.Type]
		if kind == "" {
			kind = s.Type
		}
		fmt.Fprintf(&b, "%d. %s · %s\nОтдаю: %s %s\nПолучаю: %s %s\n", i+1, s.Provider, kind, s.Input.String(), s.FromAsset, s.Output.String(), s.ToAsset)
		if s.Price.IsPositive() {
			fmt.Fprintf(&b, "Курс: 1 %s = %s %s\n", s.ToAsset, s.Price.String(), s.FromAsset)
		}
		if s.Type == "p2p" {
			merchant := s.MerchantName
			if merchant == "" {
				merchant = s.Description
			}
			if merchant != "" {
				fmt.Fprintf(&b, "Продавец: %s\n", merchant)
			}
			if s.OfferID != "" {
				fmt.Fprintf(&b, "Заявка: %s\n", s.OfferID)
			}
			if len(s.PaymentMethods) > 0 {
				labels := make([]string, 0, len(s.PaymentMethods))
				for _, method := range s.PaymentMethods {
					labels = append(labels, bankLabel(strings.ToLower(s.Provider), method))
				}
				fmt.Fprintf(&b, "Оплата в заявке: %s\n", strings.Join(labels, ", "))
			}
			if s.MaxFiat.IsPositive() {
				fmt.Fprintf(&b, "Лимиты заявки: %s–%s ₽\n", s.MinFiat.String(), s.MaxFiat.String())
			}
		}
		if !s.Fee.IsZero() {
			asset := s.ToAsset
			fmt.Fprintf(&b, "Комиссия: %s %s\n", s.Fee.String(), asset)
		}
		if s.Description != "" && s.Type != "p2p" {
			fmt.Fprintln(&b, s.Description)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Итого: %s ₽\nРасчёт: %s UTC", q.RUBRequired.StringFixed(2), q.CalculatedAt.UTC().Format("15:04:05"))
	for _, w := range q.Warnings {
		fmt.Fprintf(&b, "\n⚠️ %s", w)
	}
	return b.String()
}

func unavailableReason(reason string) string {
	switch {
	case reason == "fallback disabled":
		return "исключён настройкой резервных комиссий."
	case strings.HasPrefix(reason, "P2P payment unavailable: "):
		return "в заявках нет выбранного способа оплаты «" + strings.TrimPrefix(reason, "P2P payment unavailable: ") + "». Выберите доступный банк через /banks."
	case strings.Contains(reason, "insufficient liquidity"):
		return "нет заявки под эту сумму и выбранные фильтры (страна KYC, банк, лимиты, резерв, надёжность). Проверьте /settings или измените сумму."
	case strings.Contains(reason, "below withdrawal minimum"):
		return "сумма ниже минимального вывода биржи."
	case strings.Contains(reason, "below market minimum"):
		return "сумма ниже минимальной сделки на бирже."
	case strings.Contains(reason, "stale"), strings.Contains(reason, "unavailable"):
		return "нет свежих данных или операция временно недоступна. Нажмите «Обновить»."
	default:
		return reason
	}
}
