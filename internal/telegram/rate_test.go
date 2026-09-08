package telegram

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/domain"
	"github.com/tishin-serg/kurskonverter/internal/market"
	"github.com/tishin-serg/kurskonverter/internal/provider/demo"
	"github.com/tishin-serg/kurskonverter/internal/route"
	"github.com/tishin-serg/kurskonverter/internal/storage"
)

func TestRubleEquivalentRounding(t *testing.T) {
	for _, tc := range []struct{ rub, rate, btc string }{{"5000", "7000000", "0.00071429"}, {"5000", "8000000", "0.000625"}, {"0.01", "999999999999", "0.00000001"}} {
		rub, rate := decimal.RequireFromString(tc.rub), decimal.RequireFromString(tc.rate)
		btc, err := equivalentBTC(rub, rate)
		if err != nil || btc.String() != tc.btc || btc.Mul(rate).LessThan(rub) || !btc.Sub(decimal.New(1, -8)).Mul(rate).LessThan(rub) {
			t.Fatal(tc, btc, err)
		}
	}
	for _, raw := range []string{"0", "-1", "1e8", "NaN", "7000000 BTC", "1.000000001"} {
		if _, err := parseRate(raw); err == nil {
			t.Fatal(raw)
		}
	}
}

func TestPersonalRatePersistenceAndRefresh(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "bot.db")
	db, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	m := market.New()
	m.Update(func(s *domain.MarketSnapshot) { *s = *demo.Snapshot(time.Now()) })
	u := &UI{Storage: db, Market: m, Engine: route.DefaultEngine(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	var texts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "sendMessage") {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
			}
			texts = append(texts, r.FormValue("text"))
			io.WriteString(w, `{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"}}}`)
		} else {
			io.WriteString(w, `{"ok":true,"result":true}`)
		}
	}))
	defer srv.Close()
	b, err := bot.New("test", bot.WithSkipGetMe(), bot.WithServerURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	send := func(text string) {
		u.Handle(ctx, b, &models.Update{Message: &models.Message{From: &models.User{ID: 1}, Chat: models.Chat{ID: 1, Type: "private"}, Text: text}})
	}
	send("5000 руб")
	if !strings.Contains(texts[len(texts)-1], "сначала задайте") {
		t.Fatal(texts)
	}
	send("/rate 7000000")
	send("5000 руб")
	target, r, err := db.Load(ctx, 1, 1)
	if err != nil || target != "0.00071429" || r.Equivalent == nil || r.Equivalent.Rate.String() != "7000000" {
		t.Fatal(target, r, err)
	}
	if !strings.Contains(Summary(decimal.RequireFromString(target), r, false), "5000.00 ₽") {
		t.Fatal(r)
	}
	if err := db.DB.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.DB.Close()
	u.Storage = db
	if rate, err := db.ReferenceRate(ctx, 1); err != nil || rate != "7000000" {
		t.Fatal(rate, err)
	}
	if rate, err := db.ReferenceRate(ctx, 2); err != nil || rate != "" {
		t.Fatal("rate leaked", rate, err)
	}
	for _, text := range []string{"5000 RUB", "5000₽", "5000,00 руб."} {
		if n, e, err := u.requestAmount(ctx, 1, text); err != nil || n.String() != "0.00071429" || e == nil {
			t.Fatal(text, n, err)
		}
	}
	send("/rate 8000000")
	u.Handle(ctx, b, &models.Update{CallbackQuery: &models.CallbackQuery{ID: "refresh", From: models.User{ID: 1}, Message: models.MaybeInaccessibleMessage{Message: &models.Message{Chat: models.Chat{ID: 1, Type: "private"}}}, Data: "r:1"}})
	target, r, err = db.Load(ctx, 1, 2)
	if err != nil || target != "0.000625" || r.Equivalent == nil || r.Equivalent.RUB.String() != "5000" || r.Equivalent.Rate.String() != "8000000" {
		t.Fatal(target, r, err)
	}
	_, old, err := db.Load(ctx, 1, 1)
	if err != nil || old.Equivalent.Rate.String() != "7000000" {
		t.Fatal("saved calculation changed")
	}
}
