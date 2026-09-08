package telegram

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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

func TestIndependentBankSelectionAndAny(t *testing.T) {
	ctx := context.Background()
	db, e := storage.Open(ctx, ":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer db.DB.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"}}}`))
	}))
	defer srv.Close()
	b, e := bot.New("test", bot.WithSkipGetMe(), bot.WithServerURL(srv.URL))
	if e != nil {
		t.Fatal(e)
	}
	m := market.New()
	m.Update(func(s *domain.MarketSnapshot) { *s = *demo.Snapshot(time.Now()) })
	u := &UI{Market: m, Storage: db, Engine: route.DefaultEngine(), Filter: domain.Filter{PaymentMethod: "Tbank"}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	choices := u.bankChoices("wallet")
	if len(choices) != 1 {
		t.Fatal(choices)
	}
	u.bankCallback(ctx, b, 1, 1, "bankset:wallet:"+choices[0].Token)
	saved, e := db.ProviderPayments(ctx, 1)
	if e != nil || saved["wallet"] != "demo-bank" {
		t.Fatal(saved, e)
	}
	other, _ := db.ProviderPayments(ctx, 2)
	if len(other) != 0 {
		t.Fatal("settings leaked to another user")
	}
	f := u.Filter
	f.PaymentByProvider = saved
	r := u.Engine.Calculate(ctx, decimal.New(1, -2), m.Read(), f, time.Now())
	if len(r.Quotes) != 1 || r.Quotes[0].RouteID != "Wallet → GRAM → OKX" {
		t.Fatal(r)
	}
	// Unknown/stale callback must not replace the saved setting.
	u.bankCallback(ctx, b, 1, 1, "bankset:wallet:missing")
	saved, _ = db.ProviderPayments(ctx, 1)
	if saved["wallet"] != "demo-bank" {
		t.Fatal(saved)
	}
	for _, text := range []string{"/settings any", "0.01"} {
		u.Handle(ctx, b, &models.Update{Message: &models.Message{From: &models.User{ID: 1}, Chat: models.Chat{ID: 1, Type: "private"}, Text: text}})
	}
	_, result, e := db.Load(ctx, 1, 1)
	if e != nil || len(result.Quotes) != 3 {
		t.Fatal(result, e)
	}
	saved, _ = db.ProviderPayments(ctx, 1)
	if len(saved) != 0 {
		t.Fatal("global reset left provider restrictions")
	}
}
