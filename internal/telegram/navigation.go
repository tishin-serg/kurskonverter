package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/domain"
)

func button(text, action string) models.InlineKeyboardButton {
	return models.InlineKeyboardButton{Text: text, CallbackData: action}
}
func keyboard(rows ...[]models.InlineKeyboardButton) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}
func one(text, action string) []models.InlineKeyboardButton {
	return []models.InlineKeyboardButton{button(text, action)}
}

// Entry points outside the active panel must open a visible menu at the end
// of the chat. Editing an earlier panel does not scroll Telegram to it.
func (u *UI) newPanel(ctx context.Context, user int64) {
	v, err := u.Storage.UIState(ctx, user)
	if err != nil {
		return
	}
	v.PanelID = 0
	if err := u.Storage.SaveUI(ctx, user, v); err != nil {
		u.Log.Warn("menu state reset failed")
	}
}

// Only menu panels are edited; saved calculation messages never use this path.
func (u *UI) panel(ctx context.Context, b *bot.Bot, user, chat int64, state, text string, k *models.InlineKeyboardMarkup) {
	v, err := u.Storage.UIState(ctx, user)
	if err != nil {
		u.send(ctx, b, chat, "Не удалось открыть меню. Попробуйте /start.", nil)
		return
	}
	v.State = state
	v.Revision++
	for i := range k.InlineKeyboard {
		for j := range k.InlineKeyboard[i] {
			a := &k.InlineKeyboard[i][j]
			a.CallbackData = fmt.Sprintf("u:%d:%s", v.Revision, a.CallbackData)
		}
	}
	if err = u.Storage.SaveUI(ctx, user, v); err != nil {
		return
	}
	if v.PanelID != 0 {
		_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{ChatID: chat, MessageID: v.PanelID, Text: text, ReplyMarkup: k})
		if err == nil {
			return
		}
		u.Log.Debug("menu edit unavailable; opening a new panel")
	}
	m, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chat, Text: text, ReplyMarkup: k})
	if err != nil {
		u.Log.Warn("menu send failed")
		return
	}
	v.PanelID = m.ID
	if err = u.Storage.SaveUI(ctx, user, v); err != nil {
		u.Log.Warn("menu state save failed")
	}
}
func (u *UI) home(ctx context.Context, b *bot.Bot, user, chat int64, note string) {
	text := "Калькулятор переводов\n\nСравню стоимость BTC и долларов на TBC с учётом комиссий. Сделки вы выполняете сами."
	if note != "" {
		text = note + "\n\n" + text
	}
	u.panel(ctx, b, user, chat, "home", text, keyboard(one("Получить BTC", "btc"), one("USD на TBC", "usd"), one("⚙️ Настройки", "settings")))
}
func (u *UI) personalFilter(ctx context.Context, user int64) (domain.Filter, error) {
	f := u.Filter
	p, err := u.Storage.Preferences(ctx, user)
	if err != nil {
		return f, err
	}
	if p.MinOrders != nil {
		f.MinOrdersCount = *p.MinOrders
	}
	if p.MinSuccess != "" {
		f.MinCompletionRate, err = decimal.NewFromString(p.MinSuccess)
		if err != nil {
			return f, err
		}
	}
	f.DisallowFallback = !p.AllowFallback
	f.BybitCountry = p.BybitCountry
	legacy, err := u.Storage.Payment(ctx, user)
	if err != nil {
		return f, err
	}
	if legacy != "" {
		f.PaymentMethod = legacy
	}
	if legacy == "any" {
		f.PaymentMethod = ""
	}
	f.PaymentByProvider, err = u.Storage.ProviderPayments(ctx, user)
	return f, err
}
func (u *UI) settings(ctx context.Context, b *bot.Bot, user, chat int64, note string) {
	f, err := u.personalFilter(ctx, user)
	if err != nil {
		u.home(ctx, b, user, chat, "Настройки временно недоступны.")
		return
	}
	rate, err := u.Storage.ReferenceRate(ctx, user)
	if err != nil {
		u.home(ctx, b, user, chat, "Не удалось прочитать курс.")
		return
	}
	if rate == "" {
		rate = "не задан"
	} else {
		rate += " ₽"
	}
	label := func(provider string) string {
		m, ok := f.PaymentByProvider[provider]
		if !ok {
			m = f.PaymentMethod
		}
		if m == "" {
			return "любой"
		}
		return bankLabel(provider, m)
	}
	fallback := "разрешены"
	if f.DisallowFallback {
		fallback = "исключены"
	}
	text := fmt.Sprintf("⚙️ Настройки\n\nКурс за 1 BTC: %s\nСделок у продавца: от %d\nУспешных операций: от %s%%\nBybit: %s\nWallet: %s\nРезервные комиссии: %s", rate, f.MinOrdersCount, f.MinCompletionRate.String(), label("bybit"), label("wallet"), fallback)
	text += "\nСтрана KYC Bybit: " + countryLabel(f.BybitCountry)
	if f.BybitCountry == "" {
		text += "\nОбъявления с ограничением страны исключены."
	}
	if note != "" {
		text = note + "\n\n" + text
	}
	u.panel(ctx, b, user, chat, "settings", text, keyboard(one("Курс BTC/RUB", "input:rate"), []models.InlineKeyboardButton{button("Число сделок", "input:orders"), button("Успешность, %", "input:success")}, one("Банки P2P", "banks"), one("Страна KYC Bybit", "input:country"), one("Резервные комиссии", "fallback"), one("🏠 Главное меню", "home")))
}
func (u *UI) inputPanel(ctx context.Context, b *bot.Bot, user, chat int64, kind, note string) {
	var text string
	var rows [][]models.InlineKeyboardButton
	switch kind {
	case "country":
		text = "Страна KYC Bybit\n\nУкажите страну верификации аккаунта, а не страну банка или VPN.\nНапример: RUS — Россия, GEO — Грузия.\n\nФильтр проверяет регион объявления. Остальные требования продавца проверьте на Bybit."
		rows = append(rows, []models.InlineKeyboardButton{button("Россия", "set:country:RUS"), button("Грузия", "set:country:GEO")}, one("Страна не задана", "set:country:clear"))
	case "rate":
		text = "Курс BTC/RUB\n\nСколько рублей за 1 BTC?\nНапример: 7000000\n\nИспользуется для рублёвого эквивалента, а не как цена покупки."
	case "orders":
		text = "Минимум сделок у продавца\n\nВыберите значение или введите своё от 0 до 1000000.\n0 — без ограничения."
		rows = append(rows, []models.InlineKeyboardButton{button("50", "set:orders:50"), button("100", "set:orders:100")}, []models.InlineKeyboardButton{button("500", "set:orders:500"), button("Без ограничения", "set:orders:0")})
	case "success":
		text = "Успешные операции продавца\n\nВыберите процент или введите свой от 0 до 100.\nДанные относятся к периоду, который возвращает площадка."
		rows = append(rows, []models.InlineKeyboardButton{button("95%", "set:success:95"), button("98%", "set:success:98")}, []models.InlineKeyboardButton{button("99%", "set:success:99"), button("Без ограничения", "set:success:0")})
	default:
		u.home(ctx, b, user, chat, "Этот раздел больше недоступен.")
		return
	}
	if note != "" {
		text = note + "\n\n" + text
	}
	rows = append(rows, one("❌ Отмена", "settings"))
	u.panel(ctx, b, user, chat, "input:"+kind, text, keyboard(rows...))
}
func (u *UI) saveSetting(ctx context.Context, b *bot.Bot, user, chat int64, kind, value string) {
	value = strings.TrimSpace(value)
	var err error
	switch kind {
	case "country":
		var country string
		country, err = parseCountry(value)
		if err == nil {
			err = u.Storage.SetPreference(ctx, user, kind, country)
		}
	case "rate":
		var d decimal.Decimal
		d, err = parseRate(value)
		if err == nil {
			err = u.Storage.SetReferenceRate(ctx, user, d.String())
		}
	case "orders":
		var n int
		n, err = strconv.Atoi(value)
		if err != nil || n < 0 || n > 1000000 {
			err = fmt.Errorf("введите целое число от 0 до 1000000")
		} else {
			err = u.Storage.SetPreference(ctx, user, kind, strconv.Itoa(n))
		}
	case "success":
		var d decimal.Decimal
		value = strings.TrimSuffix(value, "%")
		if len(value) > 6 || !ratePattern.MatchString(value) {
			err = fmt.Errorf("invalid percentage")
		} else {
			d, err = decimal.NewFromString(strings.ReplaceAll(value, ",", "."))
		}
		if err != nil || d.IsNegative() || d.GreaterThan(decimal.NewFromInt(100)) || d.Exponent() < -2 {
			err = fmt.Errorf("введите процент от 0 до 100, например 98,5")
		} else {
			err = u.Storage.SetPreference(ctx, user, kind, d.String())
		}
	default:
		u.settings(ctx, b, user, chat, "Неизвестная настройка.")
		return
	}
	if err != nil {
		u.inputPanel(ctx, b, user, chat, kind, "Не удалось сохранить: "+err.Error())
		return
	}
	u.settings(ctx, b, user, chat, "✅ Настройка сохранена")
}
func (u *UI) amountPanel(ctx context.Context, b *bot.Bot, user, chat int64, mode, note string) {
	text := "Получить BTC\n\nВведите количество: 0.01 BTC\nИли сумму эквивалента: 5000 руб"
	if mode == "usd" {
		text = "USD на TBC\n\nСколько долларов должно поступить на счёт?\nНапример: 100 USD"
	}
	if note != "" {
		text = note + "\n\n" + text
	}
	rows := [][]models.InlineKeyboardButton{}
	if mode == "btc" {
		rows = append(rows, one("Курс BTC/RUB", "input:rate"))
	}
	rows = append(rows, one("❌ Отмена", "home"))
	u.panel(ctx, b, user, chat, "amount:"+mode, text, keyboard(rows...))
}
func (u *UI) uiCallback(ctx context.Context, b *bot.Bot, user, chat int64, data string) {
	p := strings.SplitN(data, ":", 3)
	if len(p) != 3 {
		u.home(ctx, b, user, chat, "Меню устарело.")
		return
	}
	rev, err := strconv.ParseInt(p[1], 10, 64)
	v, e := u.Storage.UIState(ctx, user)
	if err != nil || e != nil {
		return
	}
	if rev != v.Revision {
		if strings.HasPrefix(v.State, "input:") {
			u.inputPanel(ctx, b, user, chat, strings.TrimPrefix(v.State, "input:"), "Открыт актуальный экран.")
		} else if strings.HasPrefix(v.State, "amount:") {
			u.amountPanel(ctx, b, user, chat, strings.TrimPrefix(v.State, "amount:"), "Открыт актуальный экран.")
		} else {
			u.settings(ctx, b, user, chat, "Эта кнопка устарела. Вот актуальные настройки.")
		}
		return
	}
	a := p[2]
	switch {
	case a == "home":
		u.home(ctx, b, user, chat, "")
	case a == "settings":
		u.settings(ctx, b, user, chat, "")
	case a == "btc" || a == "usd":
		u.amountPanel(ctx, b, user, chat, a, "")
	case strings.HasPrefix(a, "input:"):
		u.inputPanel(ctx, b, user, chat, strings.TrimPrefix(a, "input:"), "")
	case strings.HasPrefix(a, "set:"):
		parts := strings.Split(a, ":")
		if len(parts) == 3 {
			u.saveSetting(ctx, b, user, chat, parts[1], parts[2])
		}
	case a == "fallback":
		u.panel(ctx, b, user, chat, "fallback", "Резервные комиссии\n\nНекоторые площадки не возвращают тариф через API. Можно учитывать заданные резервные значения с предупреждением или исключать такие маршруты.", keyboard(one("Разрешить с предупреждением", "fallback:1"), one("Исключить такие маршруты", "fallback:0"), one("← Назад", "settings")))
	case a == "fallback:1" || a == "fallback:0":
		if err := u.Storage.SetPreference(ctx, user, "fallback", strings.TrimPrefix(a, "fallback:")); err != nil {
			u.settings(ctx, b, user, chat, "Не удалось сохранить настройку.")
		} else {
			u.settings(ctx, b, user, chat, "✅ Настройка сохранена")
		}
	case a == "banks" || strings.HasPrefix(a, "banks:") || strings.HasPrefix(a, "bankset:"):
		u.bankCallback(ctx, b, user, chat, a)
	default:
		u.home(ctx, b, user, chat, "Этот раздел больше недоступен.")
	}
}
func (u *UI) pendingInput(ctx context.Context, b *bot.Bot, user, chat int64, text string) bool {
	v, err := u.Storage.UIState(ctx, user)
	if err != nil {
		return false
	}
	if !strings.HasPrefix(v.State, "input:") && !strings.HasPrefix(v.State, "amount:") {
		return false
	}
	if time.Now().Unix()-v.UpdatedAt > 1800 {
		u.home(ctx, b, user, chat, "Предыдущий ввод отменён после перерыва. Выберите новый расчёт.")
		return true
	}
	// Explicitly suffixed amounts always start a new quote, even during settings input.
	upper := strings.ToUpper(text)
	if strings.HasSuffix(upper, "USD") {
		u.usdQuote(ctx, b, user, chat, text)
		return true
	}
	if strings.HasSuffix(upper, "BTC") || rubPattern.MatchString(text) {
		u.quote(ctx, b, user, chat, text)
		return true
	}
	if strings.HasPrefix(v.State, "input:") {
		u.saveSetting(ctx, b, user, chat, strings.TrimPrefix(v.State, "input:"), text)
		return true
	}
	if v.State == "amount:usd" {
		u.usdQuote(ctx, b, user, chat, text)
		return true
	}
	u.quote(ctx, b, user, chat, text)
	return true
}
func (u *UI) clearInput(ctx context.Context, user int64) {
	v, err := u.Storage.UIState(ctx, user)
	if err == nil {
		v.State = "home"
		_ = u.Storage.SaveUI(ctx, user, v)
	}
}

func (u *UI) finishInput(ctx context.Context, b *bot.Bot, user, chat int64) {
	v, err := u.Storage.UIState(ctx, user)
	if err == nil && v.PanelID != 0 && (strings.HasPrefix(v.State, "input:") || strings.HasPrefix(v.State, "amount:")) {
		u.home(ctx, b, user, chat, "✅ Расчёт готов")
		return
	}
	u.clearInput(ctx, user)
}
