package telegram

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/tishin-serg/kurskonverter/internal/domain"
	"github.com/tishin-serg/kurskonverter/internal/market"
	"github.com/tishin-serg/kurskonverter/internal/provider/demo"
	"github.com/tishin-serg/kurskonverter/internal/route"
	"github.com/tishin-serg/kurskonverter/internal/storage"
)

func TestUIOffline(t *testing.T) {
	var mu sync.Mutex
	var messages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "sendMessage") {
			if e := r.ParseMultipartForm(1 << 20); e != nil {
				t.Error(e)
			}
			mu.Lock()
			messages = append(messages, r.FormValue("text"))
			mu.Unlock()
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer srv.Close()
	b, e := bot.New("test", bot.WithSkipGetMe(), bot.WithServerURL(srv.URL))
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	db, e := storage.Open(ctx, ":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer db.DB.Close()
	m := market.New()
	m.Update(func(s *domain.MarketSnapshot) { *s = *demo.Snapshot(time.Now()) })
	ui := &UI{Market: m, Storage: db, Engine: route.DefaultEngine(), Demo: true, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for _, text := range []string{"/start", "/settings demo-bank", "0.01 BTC"} {
		ui.Handle(ctx, b, &models.Update{Message: &models.Message{From: &models.User{ID: 1}, Chat: models.Chat{ID: 1, Type: "private"}, Text: text}})
	}
	mu.Lock()
	defer mu.Unlock()
	if len(messages) != 3 || !strings.Contains(messages[2], "ДЕМО") || !strings.Contains(messages[2], "88000.00") {
		t.Fatal(messages)
	}
	_, r, e := db.Load(ctx, 1, 1)
	if e != nil || len(r.Quotes) != 3 {
		t.Fatal(r, e)
	}
	if text := Breakdown(r.Quotes[1], true); !strings.Contains(text, "Комиссия:") {
		t.Fatal(text)
	}
}
