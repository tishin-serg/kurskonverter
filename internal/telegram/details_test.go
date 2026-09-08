package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/domain"
	"github.com/tishin-serg/kurskonverter/internal/provider/demo"
	"github.com/tishin-serg/kurskonverter/internal/route"
	"github.com/tishin-serg/kurskonverter/internal/storage"
)

func TestDetailsCallbacksFromTelegramJSON(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.DB.Close()
	if err := db.EnsureUser(ctx, 7); err != nil {
		t.Fatal(err)
	}
	result := route.DefaultEngine().Calculate(ctx, decimal.RequireFromString("0.01"), demo.Snapshot(time.Now()), domain.Filter{}, time.Now())
	id, err := db.Save(ctx, 7, "0.01", result)
	if err != nil {
		t.Fatal(err)
	}
	var sent []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "sendMessage") {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
			}
			if _, present := r.Form["reply_markup"]; present {
				t.Error("details must omit reply_markup entirely, not send a typed nil")
				fmt.Fprint(w, `{"ok":false,"error_code":400,"description":"Bad Request: object expected as reply markup"}`)
				return
			}
			sent = append(sent, r.FormValue("text"))
			fmt.Fprint(w, `{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":7,"type":"private"}}}`)
		} else {
			fmt.Fprint(w, `{"ok":true,"result":true}`)
		}
	}))
	defer srv.Close()
	b, err := bot.New("test", bot.WithSkipGetMe(), bot.WithServerURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	u := &UI{Storage: db, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for _, date := range []int{1, 0} {
		for i, quote := range result.Quotes {
			sent = nil
			var update models.Update
			raw := fmt.Sprintf(`{"callback_query":{"id":"callback","from":{"id":7},"message":{"message_id":1,"date":%d,"chat":{"id":7,"type":"private"}},"data":"d:%d:%d"}}`, date, id, i)
			if err := json.Unmarshal([]byte(raw), &update); err != nil {
				t.Fatal(err)
			}
			u.Handle(ctx, b, &update)
			text := strings.Join(sent, "")
			if !strings.Contains(text, quote.RouteID) || !strings.Contains(text, "Итого:") {
				t.Fatal(date, text)
			}
			for _, step := range quote.Steps {
				if !strings.Contains(text, "Отдаю: "+step.Input.String()+" "+step.FromAsset) || !strings.Contains(text, "Получаю: "+step.Output.String()+" "+step.ToAsset) {
					t.Fatal(text)
				}
			}
			// Neither a different chat nor a different owner may expose history.
			sent = nil
			update.CallbackQuery.From.ID = 8
			u.Handle(ctx, b, &update)
			if len(sent) != 0 {
				t.Fatal("cross-user callback answered")
			}
		}
	}
}

func TestP2PDetailsAndLosslessMessageSplitting(t *testing.T) {
	q := domain.Quote{Steps: []domain.QuoteStep{{Type: "p2p", Provider: "Bybit", FromAsset: "RUB", ToAsset: "USDT", Input: decimal.NewFromInt(9000), Output: decimal.NewFromInt(100), Price: decimal.NewFromInt(90), MerchantName: "seller-test", OfferID: "ad-42", PaymentMethods: []string{"14"}, MinFiat: decimal.NewFromInt(100), MaxFiat: decimal.NewFromInt(10000)}}}
	text := Breakdown(q, false)
	for _, want := range []string{"Курс: 1 USDT = 90 RUB", "Продавец: seller-test", "Заявка: ad-42", "100–10000 ₽", "Банковский перевод (14)"} {
		if !strings.Contains(text, want) {
			t.Fatal(text)
		}
	}
	long := strings.Repeat("Этап 🪙\n", 1500) + "Последний этап: BTC"
	chunks := splitMessage(long)
	if len(chunks) < 2 || strings.Join(chunks, "") != long {
		t.Fatal("details lost")
	}
	for _, chunk := range chunks {
		if len([]rune(chunk)) > 1900 {
			t.Fatal("message too long")
		}
	}
}
