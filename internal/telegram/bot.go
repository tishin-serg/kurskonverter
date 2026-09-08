package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/tishin-serg/kurskonverter/internal/domain"
	"github.com/tishin-serg/kurskonverter/internal/market"
	"github.com/tishin-serg/kurskonverter/internal/route"
	"github.com/tishin-serg/kurskonverter/internal/storage"
)

type UI struct {
	Market   *market.Store
	Storage  *storage.Store
	Engine   route.Engine
	Filter   domain.Filter
	Demo     bool
	Log      *slog.Logger
	Requests atomic.Uint64
}

func (u *UI) New(token string) (*bot.Bot, error) {
	return bot.New(token, bot.WithAllowedUpdates(bot.AllowedUpdates{"message", "callback_query"}), bot.WithCheckInitTimeout(15*time.Second), bot.WithDefaultHandler(u.Handle), bot.WithWorkers(2), bot.WithNotAsyncHandlers(), bot.WithUpdatesChannelCap(64), bot.WithErrorsHandler(func(_ error) { u.Log.Warn("telegram API request failed") }))
}
func (u *UI) send(ctx context.Context, b *bot.Bot, chat int64, text string, k *models.InlineKeyboardMarkup) {
	chunks := splitMessage(text)
	for i, chunk := range chunks {
		params := &bot.SendMessageParams{ChatID: chat, Text: chunk}
		// A typed nil in the ReplyMarkup interface becomes reply_markup=null;
		// Telegram rejects it instead of treating the keyboard as omitted.
		if i == len(chunks)-1 && k != nil {
			params.ReplyMarkup = k
		}
		_, err := b.SendMessage(ctx, params)
		if err != nil {
			u.Log.Warn("telegram send failed", "part", i+1)
			return
		}
	}
}

// 1900 runes fit Telegram's limit even when every rune uses two UTF-16 units.
func splitMessage(text string) []string {
	var chunks []string
	runes := []rune(text)
	for len(runes) > 1900 {
		cut := 1900
		for i := cut - 1; i >= 950; i-- {
			if runes[i] == '\n' {
				cut = i + 1
				break
			}
		}
		chunks = append(chunks, string(runes[:cut]))
		runes = runes[cut:]
	}
	if len(runes) > 0 {
		chunks = append(chunks, string(runes))
	}
	return chunks
}
func (u *UI) Handle(ctx context.Context, b *bot.Bot, update *models.Update) {
	u.Requests.Add(1)
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if update.CallbackQuery != nil {
		u.callback(ctx, b, update.CallbackQuery)
		return
	}
	m := update.Message
	if m == nil || m.From == nil || m.Chat.Type != "private" {
		return
	}
	user := m.From.ID
	chat := m.Chat.ID
	if e := u.Storage.EnsureUser(ctx, user); e != nil {
		u.send(ctx, b, chat, "Хранилище временно недоступно.", nil)
		return
	}
	text := strings.TrimSpace(m.Text)
	if text == "/rate" || strings.HasPrefix(text, "/rate ") {
		u.rateCommand(ctx, b, user, chat, text)
		return
	}
	if text == "/banks" || text == "/settings" {
		u.bankMenu(ctx, b, user, chat, "", 0)
		return
	}
	if text == "/start" || text == "/help" {
		u.send(ctx, b, chat, "Отправьте количество BTC для получения на внешний адрес: 0.01 BTC.\nИли задайте /rate 7000000 и отправьте 5000 руб — рассчитаю BTC-эквивалент и стоимость покупки.\n/settings — банк и личный курс BTC/RUB.\nРасчёт учитывает стакан и комиссии; выполнение сделок ботом не производится.", nil)
		return
	}
	if strings.HasPrefix(text, "/settings") {
		parts := strings.Fields(text)
		if len(parts) > 2 {
			u.send(ctx, b, chat, "/settings <ID способа оплаты> или /settings any", nil)
			return
		}
		if len(parts) == 2 {
			p := parts[1]
			if len(p) > 64 {
				u.send(ctx, b, chat, "ID слишком длинный.", nil)
				return
			}
			if e := u.Storage.SetPayment(ctx, user, p); e != nil {
				u.send(ctx, b, chat, "Не удалось сохранить настройки.", nil)
				return
			}
		}
		p, e := u.Storage.Payment(ctx, user)
		if e != nil {
			u.send(ctx, b, chat, "Не удалось прочитать настройки.", nil)
			return
		}
		if p == "" {
			p = "любой (либо PAYMENT_METHOD сервера)"
		}
		u.send(ctx, b, chat, "Общий способ оплаты: "+p+"\nВыбрать отдельно для Bybit и Wallet: /banks\nСнять все фильтры оплаты: /settings any", nil)
		return
	}
	u.quote(ctx, b, user, chat, text)
}
func (u *UI) quote(ctx context.Context, b *bot.Bot, user, chat int64, text string) {
	target, equivalent, e := u.requestAmount(ctx, user, text)
	if e != nil {
		u.send(ctx, b, chat, e.Error(), nil)
		return
	}
	f := u.Filter
	p, e := u.Storage.Payment(ctx, user)
	if e != nil {
		u.send(ctx, b, chat, "Не удалось прочитать настройки.", nil)
		return
	}
	if p != "" {
		f.PaymentMethod = p
	}
	if p == "any" {
		f.PaymentMethod = ""
	}
	f.PaymentByProvider, e = u.Storage.ProviderPayments(ctx, user)
	if e != nil {
		u.send(ctx, b, chat, "Не удалось прочитать настройки банков.", nil)
		return
	}
	r := u.Engine.Calculate(ctx, target, u.Market.Read(), f, time.Now())
	r.Equivalent = equivalent
	id, e := u.Storage.Save(ctx, user, target.String(), r)
	if e != nil {
		u.send(ctx, b, chat, "Не удалось сохранить расчёт. Повторите запрос.", nil)
		return
	}
	k := &models.InlineKeyboardMarkup{}
	for i, q := range r.Quotes {
		k.InlineKeyboard = append(k.InlineKeyboard, []models.InlineKeyboardButton{{Text: "Подробнее: " + q.RouteID, CallbackData: fmt.Sprintf("d:%d:%d", id, i)}})
	}
	k.InlineKeyboard = append(k.InlineKeyboard, []models.InlineKeyboardButton{{Text: "Обновить", CallbackData: fmt.Sprintf("r:%d", id)}})
	k.InlineKeyboard = append(k.InlineKeyboard, []models.InlineKeyboardButton{{Text: "Выбрать банк P2P", CallbackData: "banks"}})
	u.send(ctx, b, chat, Summary(target, r, u.Demo), k)
}
func (u *UI) callback(ctx context.Context, b *bot.Bot, q *models.CallbackQuery) {
	_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID})
	if err != nil {
		u.Log.Debug("callback acknowledgment failed")
	}
	var source *models.Chat
	if q.Message.Message != nil {
		source = &q.Message.Message.Chat
	} else if q.Message.InaccessibleMessage != nil {
		source = &q.Message.InaccessibleMessage.Chat
	}
	if source == nil || source.Type != "private" {
		return
	}
	chat := source.ID
	if q.From.ID != chat {
		return
	}
	if q.Data == "rate" {
		u.rateCommand(ctx, b, q.From.ID, chat, "/rate")
		return
	}
	if q.Data == "banks" || strings.HasPrefix(q.Data, "banks:") || strings.HasPrefix(q.Data, "bankset:") {
		u.bankCallback(ctx, b, q.From.ID, chat, q.Data)
		return
	}
	parts := strings.Split(q.Data, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return
	}
	id, e := strconv.ParseInt(parts[1], 10, 64)
	if e != nil {
		return
	}
	target, r, e := u.Storage.Load(ctx, q.From.ID, id)
	if e != nil {
		u.send(ctx, b, chat, "Расчёт недоступен. Отправьте количество BTC заново.", nil)
		return
	}
	if parts[0] == "r" && len(parts) == 2 {
		if r.Equivalent != nil {
			target = r.Equivalent.RUB.String() + " RUB"
		}
		u.quote(ctx, b, q.From.ID, chat, target)
		return
	}
	if parts[0] == "d" && len(parts) == 3 {
		i, e := strconv.Atoi(parts[2])
		if e != nil || i < 0 || i >= len(r.Quotes) {
			return
		}
		u.send(ctx, b, chat, "Сохранённый расчёт; для актуальной цены нажмите «Обновить».\n\n"+equivalentText(r.Equivalent)+Breakdown(r.Quotes[i], u.Demo), nil)
	}
}
