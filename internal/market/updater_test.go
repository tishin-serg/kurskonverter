package market

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestWorkerFailureIsolated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stats := NewStatistics()
	healthy := make(chan struct{}, 1)
	bad := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, []Worker{{Name: "bad", Interval: time.Hour, Fetch: func(context.Context) error { bad <- struct{}{}; return errors.New("offline") }}, {Name: "good", Interval: time.Hour, Fetch: func(context.Context) error { healthy <- struct{}{}; return nil }}}, slog.New(slog.NewTextHandler(io.Discard, nil)), stats)
	}()
	defer cancel()
	for _, ch := range []chan struct{}{bad, healthy} {
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatal("worker blocked")
		}
	}
	cancel()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown blocked")
	}
	errs, last := stats.Snapshot()
	if errs["bad"] != 1 || last["good"].IsZero() {
		t.Fatal(errs, last)
	}
}
