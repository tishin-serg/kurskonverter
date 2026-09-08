package telegram

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/route"
)

var usdPattern = regexp.MustCompile(`(?i)^([0-9]{1,10}(?:[.,][0-9]{1,2})?)\s*(?:USD|\$)?$`)

func parseUSD(text string) (decimal.Decimal, error) {
	m := usdPattern.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return decimal.Zero, fmt.Errorf("введите сумму долларов, например 100 или 100,50 USD")
	}
	n, e := decimal.NewFromString(strings.ReplaceAll(m[1], ",", "."))
	if e != nil || !n.IsPositive() {
		return decimal.Zero, fmt.Errorf("сумма USD должна быть больше нуля")
	}
	return n, nil
}
func (u *UI) usdQuote(ctx context.Context, b *bot.Bot, user, chat int64, text string) {
	n, err := parseUSD(text)
	if err != nil {
		u.amountPanel(ctx, b, user, chat, "usd", err.Error())
		return
	}
	f, err := u.personalFilter(ctx, user)
	if err != nil {
		u.settings(ctx, b, user, chat, "Не удалось прочитать настройки.")
		return
	}
	q, err := route.CalculateUSD(n, u.Market.Read(), f, time.Now())
	if err != nil {
		u.panel(ctx, b, user, chat, "amount:usd", "USD на TBC\n\n"+err.Error(), keyboard(one("Повторить", "usd"), one("⚙️ Настройки", "settings"), one("← Назад", "home")))
		return
	}
	id, err := u.Storage.Save(ctx, user, n.String(), route.Result{USD: &q})
	if err != nil {
		u.amountPanel(ctx, b, user, chat, "usd", "Не удалось сохранить расчёт. Повторите ввод.")
		return
	}
	u.finishInput(ctx, b, user, chat)
	u.send(ctx, b, chat, usdSummary(q, u.Demo), keyboard(one("Этапы и заявки", fmt.Sprintf("ud:%d", id)), []models.InlineKeyboardButton{button("Обновить", fmt.Sprintf("ur:%d", id)), button("🏠 Меню", "home")}))
}
func usdSummary(q route.USDQuote, demo bool) string {
	prefix := ""
	if demo {
		prefix = "ДЕМО · вымышленные данные\n\n"
	}
	s := fmt.Sprintf("%sUSD на TBC Bank\n\nПолучить: %s USD\nПотребуется: %s ₽\nИтоговый курс: %s ₽ за 1 USD\n\nRUB → USDT → USD\nРасчёт: %s UTC\nБез возможной комиссии банка за зачисление.", prefix, q.ReceivedUSD.StringFixed(2), q.RUBRequired.StringFixed(2), q.EffectiveRate.StringFixed(2), q.CalculatedAt.UTC().Format("15:04:05"))
	if q.Fallback {
		s += "\n⚠️ P2P-комиссия 0% задана в настройках; API не вернул тариф."
	}
	return s
}
func usdDetails(q route.USDQuote, demo bool) string {
	var out strings.Builder
	out.WriteString(usdSummary(q, demo))
	fmt.Fprintf(&out, "\n\n1. Покупка USDT за рубли\nОтдаю: %s RUB\nПолучаю: %s USDT\nКурс: %s RUB за 1 USDT\nПродавец: %s\nЗаявка: %s\nЛимиты: %s–%s RUB", q.RUBRequired.String(), q.USDTBought.String(), q.Buy.Price.String(), q.Buy.MerchantName, q.Buy.OfferID, q.Buy.MinFiat.String(), q.Buy.MaxFiat.String())
	fmt.Fprintf(&out, "\n\n2. Продажа USDT за USD\nОтдаю: %s USDT\nПолучаю: %s USD на TBC Bank\nКурс: %s USD за 1 USDT\nПокупатель: %s\nЗаявка: %s\nЛимиты: %s–%s USD", q.USDTRequired.String(), q.ReceivedUSD.StringFixed(2), q.Sell.Price.String(), q.Sell.MerchantName, q.Sell.OfferID, q.Sell.MinFiat.String(), q.Sell.MaxFiat.String())
	if change := q.USDTBought.Sub(q.USDTRequired); change.IsPositive() {
		fmt.Fprintf(&out, "\nОстаток после округления покупки: %s USDT", change.String())
	}
	out.WriteString("\n\nДве P2P-сделки в Bybit, без вывода USDT между площадками. Проверьте условия обеих заявок перед первой сделкой.")
	return out.String()
}
