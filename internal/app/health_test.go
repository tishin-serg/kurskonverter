package app

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tishin-serg/kurskonverter/internal/domain"
	"github.com/tishin-serg/kurskonverter/internal/market"
	"github.com/tishin-serg/kurskonverter/internal/provider/demo"
	"github.com/tishin-serg/kurskonverter/internal/storage"
)

func TestHealth(t *testing.T) {
	db, e := storage.Open(context.Background(), ":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer db.DB.Close()
	m := market.New()
	h := &Health{Storage: db, Market: m, Stats: market.NewStatistics()}
	handler := h.Handler()
	check := func(path string, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != want {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	check("/health", 200)
	check("/ready", 503)
	h.Started.Store(true)
	h.TelegramReady.Store(true)
	check("/ready", 503)
	m.Update(func(s *domain.MarketSnapshot) { *s = *demo.Snapshot(time.Now()) })
	check("/ready", 200)
	m.Update(func(s *domain.MarketSnapshot) { *s = *demo.Snapshot(time.Now().Add(-time.Hour)) })
	check("/ready", 503)
	check("/metrics", 200)
}
