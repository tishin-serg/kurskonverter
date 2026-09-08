package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/domain"
	"github.com/tishin-serg/kurskonverter/internal/market"
	"github.com/tishin-serg/kurskonverter/internal/route"
	"github.com/tishin-serg/kurskonverter/internal/storage"
)

type Health struct {
	Storage                     *storage.Store
	Market                      *market.Store
	Stats                       *market.Statistics
	Started                     atomic.Bool
	TelegramReady               atomic.Bool
	Version, Commit, Date, Mode string
}

func (h *Health) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "alive", "version": h.Version, "commit": h.Commit, "date": h.Date, "mode": h.Mode})
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if !h.Started.Load() || !h.TelegramReady.Load() || h.Storage.DB.PingContext(ctx) != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		result := route.DefaultEngine().Calculate(ctx, decimal.New(1, -2), h.Market.Read(), domain.Filter{}, time.Now())
		if len(result.Quotes) == 0 {
			http.Error(w, "no usable market route for readiness probe", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ready\n"))
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		errs, last := h.Stats.Snapshot()
		for k, v := range errs {
			_, _ = fmt.Fprintf(w, "provider_errors_total{provider=%q} %d\n", k, v)
		}
		for k, v := range last {
			_, _ = fmt.Fprintf(w, "provider_last_success_timestamp{provider=%q} %d\n", k, v.Unix())
		}
	})
	return mux
}
