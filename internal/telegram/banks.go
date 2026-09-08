package telegram

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/tishin-serg/kurskonverter/internal/domain"
)

type bankChoice struct{ ID, Label, Token string }

func bankLabel(provider, id string) string {
	if domain.CanonicalPayment(id) == "tbank" {
		return "Т-Банк"
	}
	if domain.CanonicalPayment(id) == "sbp" {
		return "СБП"
	}
	if provider == "bybit" {
		if id == domain.BybitTBCPayment {
			return "TBC Bank"
		}
		if id == "416" {
			return "Баланс Bybit (не банк)"
		}
		if id == "14" {
			return "Банковский перевод (14)"
		}
		return "Способ Bybit " + id
	}
	switch id {
	case "sberbankru":
		return "Сбербанк"
	case "alfabank":
		return "Альфа-Банк"
	case "vtbbankru":
		return "ВТБ"
	default:
		return id
	}
}
func (u *UI) bankChoices(provider string) []bankChoice {
	data := u.Market.Read().P2P[provider]
	if data.UpdatedAt.IsZero() || time.Since(data.UpdatedAt) > 60*time.Second {
		return nil
	}
	unique := map[string]bool{}
	var choices []bankChoice
	for _, offer := range data.Value {
		for _, method := range offer.PaymentMethods {
			if method == "" || unique[method] {
				continue
			}
			unique[method] = true
			hash := sha256.Sum256([]byte(provider + "\x00" + method))
			choices = append(choices, bankChoice{ID: method, Label: bankLabel(provider, method), Token: fmt.Sprintf("%x", hash[:12])})
		}
	}
	sort.Slice(choices, func(i, j int) bool {
		if choices[i].Label == choices[j].Label {
			return choices[i].ID < choices[j].ID
		}
		return choices[i].Label < choices[j].Label
	})
	return choices
}
func (u *UI) bankMenu(ctx context.Context, b *bot.Bot, user, chat int64, provider string, page int) {
	settings, e := u.Storage.ProviderPayments(ctx, user)
	if e != nil {
		u.send(ctx, b, chat, "Не удалось прочитать настройки.", nil)
		return
	}
	legacy, e := u.Storage.Payment(ctx, user)
	if e != nil {
		u.send(ctx, b, chat, "Не удалось прочитать настройки.", nil)
		return
	}
	if legacy == "" {
		legacy = u.Filter.PaymentMethod
	}
	if legacy == "any" {
		legacy = ""
	}
	label := func(key string) string {
		v, ok := settings[key]
		if !ok {
			v = legacy
		}
		if v == "" {
			return "любой"
		}
		return bankLabel(key, v)
	}
	k := &models.InlineKeyboardMarkup{}
	if provider == "" {
		for _, key := range []string{"bybit", "wallet"} {
			k.InlineKeyboard = append(k.InlineKeyboard, []models.InlineKeyboardButton{{Text: key + ": " + label(key), CallbackData: "banks:" + key + ":0"}})
		}
		k.InlineKeyboard = append(k.InlineKeyboard, one("← Назад", "settings"))
		u.panel(ctx, b, user, chat, "banks", "Банки P2P\n\nВыберите площадку. Банк для BestChange задан отдельным направлением обмена.", k)
		return
	}
	if provider != "bybit" && provider != "wallet" {
		return
	}
	choices := u.bankChoices(provider)
	const size = 6
	pages := (len(choices) + size - 1) / size
	if pages == 0 {
		pages = 1
	}
	if page < 0 || page >= pages {
		page = 0
	}
	k.InlineKeyboard = append(k.InlineKeyboard, []models.InlineKeyboardButton{{Text: "Любой способ оплаты", CallbackData: "bankset:" + provider + ":any"}})
	for i := page * size; i < len(choices) && i < (page+1)*size; i++ {
		v := choices[i]
		k.InlineKeyboard = append(k.InlineKeyboard, []models.InlineKeyboardButton{{Text: v.Label, CallbackData: "bankset:" + provider + ":" + v.Token}})
	}
	var nav []models.InlineKeyboardButton
	if page > 0 {
		nav = append(nav, models.InlineKeyboardButton{Text: "←", CallbackData: fmt.Sprintf("banks:%s:%d", provider, page-1)})
	}
	if page+1 < pages {
		nav = append(nav, models.InlineKeyboardButton{Text: "→", CallbackData: fmt.Sprintf("banks:%s:%d", provider, page+1)})
	}
	if len(nav) > 0 {
		k.InlineKeyboard = append(k.InlineKeyboard, nav)
	}
	k.InlineKeyboard = append(k.InlineKeyboard, []models.InlineKeyboardButton{{Text: "← Назад", CallbackData: "banks"}})
	text := fmt.Sprintf("Банк · %s\nВыбран: %s\n\nДоступные способы оплаты · %d/%d", provider, label(provider), page+1, pages)
	if len(choices) == 0 {
		text += "\nСвежих объявлений пока нет. Попробуйте позже."
	}
	u.panel(ctx, b, user, chat, "banks:"+provider, text, k)
}
func (u *UI) bankCallback(ctx context.Context, b *bot.Bot, user, chat int64, data string) {
	if e := u.Storage.EnsureUser(ctx, user); e != nil {
		return
	}
	if data == "banks" {
		u.bankMenu(ctx, b, user, chat, "", 0)
		return
	}
	p := strings.Split(data, ":")
	if len(p) != 3 || (p[1] != "bybit" && p[1] != "wallet") {
		return
	}
	if p[0] == "banks" {
		page, e := strconv.Atoi(p[2])
		if e == nil {
			u.bankMenu(ctx, b, user, chat, p[1], page)
		}
		return
	}
	if p[0] != "bankset" {
		return
	}
	method := ""
	found := p[2] == "any"
	for _, choice := range u.bankChoices(p[1]) {
		if choice.Token == p[2] {
			method = choice.ID
			found = true
			break
		}
	}
	if !found {
		u.bankMenu(ctx, b, user, chat, p[1], 0)
		return
	}
	if e := u.Storage.SetProviderPayment(ctx, user, p[1], method); e != nil {
		u.send(ctx, b, chat, "Не удалось сохранить банк.", nil)
		return
	}
	u.settings(ctx, b, user, chat, "✅ Банк сохранён")
}
