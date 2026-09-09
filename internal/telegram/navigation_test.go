package telegram

import (
	"context"
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
	"github.com/tishin-serg/kurskonverter/internal/market"
	"github.com/tishin-serg/kurskonverter/internal/provider/demo"
	"github.com/tishin-serg/kurskonverter/internal/route"
	"github.com/tishin-serg/kurskonverter/internal/storage"
)

func TestNavigationAndPersonalSettings(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.DB.Close()
	m := market.New()
	m.Update(func(s *domain.MarketSnapshot) { *s = *demo.Snapshot(time.Now()) })
	u := &UI{Storage: db, Market: m, Engine: route.DefaultEngine(), Filter: domain.Filter{MinOrdersCount: 50, MinCompletionRate: decimal.NewFromInt(95)}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	var sends, edits, nextID int
	var last string
	var failEdit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "answerCallbackQuery"):
			io.WriteString(w, `{"ok":true,"result":true}`)
			return
		case strings.HasSuffix(r.URL.Path, "editMessageText"):
			edits++
			if failEdit {
				io.WriteString(w, `{"ok":false,"error_code":400,"description":"message can't be edited"}`)
				return
			}
		case strings.HasSuffix(r.URL.Path, "sendMessage"):
			sends++
			nextID++
		}
		last = r.FormValue("text")
		fmt.Fprintf(w, `{"ok":true,"result":{"message_id":%d,"date":1,"chat":{"id":1,"type":"private"}}}`, nextID)
	}))
	defer srv.Close()
	b, err := bot.New("test", bot.WithSkipGetMe(), bot.WithServerURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	send := func(text string) {
		u.Handle(ctx, b, &models.Update{Message: &models.Message{From: &models.User{ID: 1}, Chat: models.Chat{ID: 1, Type: "private"}, Text: text}})
	}
	callback := func(data string) {
		u.Handle(ctx, b, &models.Update{CallbackQuery: &models.CallbackQuery{ID: "callback", From: models.User{ID: 1}, Message: models.MaybeInaccessibleMessage{Message: &models.Message{Chat: models.Chat{ID: 1, Type: "private"}}}, Data: data}})
	}
	click := func(action string) string {
		v, e := db.UIState(ctx, 1)
		if e != nil {
			t.Fatal(e)
		}
		data := fmt.Sprintf("u:%d:%s", v.Revision, action)
		callback(data)
		return data
	}
	send("/start")
	if sends != 1 || !strings.Contains(last, "Калькулятор") {
		t.Fatal(sends, last)
	}
	click("settings")
	if sends != 1 || edits != 1 || !strings.Contains(last, "от 50") {
		t.Fatal(sends, edits, last)
	}
	click("input:orders")
	send("не число")
	v, _ := db.UIState(ctx, 1)
	if v.State != "input:orders" || !strings.Contains(last, "введите целое") {
		t.Fatal(v, last)
	}
	old := click("set:orders:100")
	p, _ := db.Preferences(ctx, 1)
	if p.MinOrders == nil || *p.MinOrders != 100 {
		t.Fatal(p)
	}
	click("input:orders")
	click("set:orders:500")
	callback(old)
	p, _ = db.Preferences(ctx, 1)
	if *p.MinOrders != 500 {
		t.Fatal("stale button changed settings")
	}
	click("input:success")
	send("1e2147483647")
	v, _ = db.UIState(ctx, 1)
	if v.State != "input:success" {
		t.Fatal(v)
	}
	send("98,5%")
	f, e := u.personalFilter(ctx, 1)
	if e != nil || f.MinCompletionRate.String() != "98.5" || f.MinOrdersCount != 500 {
		t.Fatal(f, e)
	}
	other, e := u.personalFilter(ctx, 2)
	if e != nil || other.MinOrdersCount != 50 {
		t.Fatal("settings leaked", other, e)
	}
	click("input:rate")
	send("/cancel")
	v, _ = db.UIState(ctx, 1)
	if v.State != "home" {
		t.Fatal(v)
	}
	click("settings")
	click("input:orders")
	send("/start")
	v, _ = db.UIState(ctx, 1)
	if v.State != "home" {
		t.Fatal(v)
	}
	if sends != 2 {
		t.Fatal("menus flooded the chat", sends)
	}
	failEdit = true
	click("settings")
	if sends != 3 {
		t.Fatal("failed edit did not recover", sends)
	}
	failEdit = false
	click("input:orders")
	if _, err := db.DB.ExecContext(ctx, "UPDATE user_ui SET updated_at=0 WHERE user_id=1"); err != nil {
		t.Fatal(err)
	}
	send("123")
	v, _ = db.UIState(ctx, 1)
	if v.State != "home" {
		t.Fatal("expired input persisted")
	}
	click("settings")
	click("input:orders")
	send("100 USD")
	_, result, e := db.Load(ctx, 1, 1)
	if e != nil || result.USD == nil {
		t.Fatal("explicit USD did not switch scenarios", result, e)
	}
	if !strings.Contains(last, "TBC Bank") {
		t.Fatal(last)
	}
	beforeSends, beforeEdits := sends, edits
	callback("home")
	if sends != beforeSends+1 || edits != beforeEdits || !strings.Contains(last, "Калькулятор") {
		t.Fatal("result menu edited an invisible old panel", sends, edits, last)
	}
	click("settings")
	if sends != beforeSends+1 || edits != beforeEdits+1 {
		t.Fatal("navigation within the new panel must edit it")
	}
}
