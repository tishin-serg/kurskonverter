package telegram

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/domain"
	"github.com/tishin-serg/kurskonverter/internal/money"
)

var ratePattern = regexp.MustCompile(`^[0-9]{1,12}(?:[.,][0-9]{1,8})?$`)
var rubPattern = regexp.MustCompile(`(?i)^([0-9]{1,12}(?:[.,][0-9]{1,2})?)\s*(?:RUB|руб\.?|₽)$`)

func parseRate(raw string) (decimal.Decimal, error) {
	if !ratePattern.MatchString(raw) {
		return decimal.Zero, fmt.Errorf("укажите положительный курс числом: /rate 7000000")
	}
	rate, err := decimal.NewFromString(strings.ReplaceAll(raw, ",", "."))
	if err != nil || !rate.IsPositive() {
		return decimal.Zero, fmt.Errorf("курс должен быть больше нуля")
	}
	return rate, nil
}

func equivalentBTC(rub, rate decimal.Decimal) (decimal.Decimal, error) {
	if !rub.IsPositive() || !rate.IsPositive() {
		return decimal.Zero, fmt.Errorf("сумма и курс должны быть больше нуля")
	}
	btc := money.CeilRatio(rub, rate, decimal.New(1, -8))
	if btc.GreaterThan(decimal.NewFromInt(21000000)) {
		return decimal.Zero, fmt.Errorf("слишком большая сумма BTC по этому курсу")
	}
	return btc, nil
}

func (u *UI) rateCommand(ctx context.Context, b *bot.Bot, user, chat int64, text string) {
	parts := strings.Fields(text)
	if len(parts) > 2 {
		u.send(ctx, b, chat, "Формат: /rate 7000000 — рублей за 1 BTC.", nil)
		return
	}
	if len(parts) == 2 {
		rate, err := parseRate(parts[1])
		if err != nil {
			u.send(ctx, b, chat, err.Error(), nil)
			return
		}
		if err = u.Storage.SetReferenceRate(ctx, user, rate.String()); err != nil {
			u.send(ctx, b, chat, "Не удалось сохранить курс.", nil)
			return
		}
	}
	raw, err := u.Storage.ReferenceRate(ctx, user)
	if err != nil {
		u.send(ctx, b, chat, "Не удалось прочитать курс.", nil)
		return
	}
	current := "Личный курс BTC/RUB ещё не задан."
	if raw != "" {
		current = "Ваш курс: 1 BTC = " + raw + " ₽."
	}
	u.send(ctx, b, chat, current+"\nИзменить: /rate 7000000\nЗатем отправьте 5000 руб — рассчитаю BTC-эквивалент и фактическую стоимость маршрутов.\nЭтот курс используется только для определения суммы BTC, цены покупки берутся с рынков.", nil)
}

func (u *UI) requestAmount(ctx context.Context, user int64, text string) (decimal.Decimal, *domain.FiatEquivalent, error) {
	match := rubPattern.FindStringSubmatch(strings.TrimSpace(text))
	if match == nil {
		n, err := ParseAmount(text)
		return n, nil, err
	}
	raw, err := u.Storage.ReferenceRate(ctx, user)
	if err != nil {
		return decimal.Zero, nil, fmt.Errorf("не удалось прочитать ваш курс")
	}
	if raw == "" {
		return decimal.Zero, nil, fmt.Errorf("сначала задайте личный курс BTC/RUB: /rate 7000000")
	}
	rate, err := parseRate(raw)
	if err != nil {
		return decimal.Zero, nil, err
	}
	rub, err := decimal.NewFromString(strings.ReplaceAll(match[1], ",", "."))
	if err != nil {
		return decimal.Zero, nil, err
	}
	n, err := equivalentBTC(rub, rate)
	return n, &domain.FiatEquivalent{RUB: rub, Rate: rate}, err
}

func equivalentText(e *domain.FiatEquivalent) string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("Эквивалент для отправки: %s ₽\nВаш курс: 1 BTC = %s ₽\nBTC округлено вверх до сатоши; стоимость покупки ниже рассчитана по рынку.\n\n", e.RUB.StringFixed(2), e.Rate.String())
}
