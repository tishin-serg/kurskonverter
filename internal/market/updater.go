package market

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

type Worker struct {
	Name     string
	Interval time.Duration
	Fetch    func(context.Context) error
}
type Statistics struct {
	mu          sync.Mutex
	Errors      map[string]uint64
	LastSuccess map[string]time.Time
}

func NewStatistics() *Statistics {
	return &Statistics{Errors: map[string]uint64{}, LastSuccess: map[string]time.Time{}}
}
func (s *Statistics) Snapshot() (map[string]uint64, map[string]time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := map[string]uint64{}
	l := map[string]time.Time{}
	for k, v := range s.Errors {
		e[k] = v
	}
	for k, v := range s.LastSuccess {
		l[k] = v
	}
	return e, l
}
func Run(ctx context.Context, workers []Worker, log *slog.Logger, stats *Statistics) error {
	group, ctx := errgroup.WithContext(ctx)
	for _, w := range workers {
		group.Go(func() error {
			if w.Interval <= 0 {
				return nil
			}
			ticker := time.NewTicker(w.Interval)
			defer ticker.Stop()
			for {
				started := time.Now()
				req, cancel := context.WithTimeout(ctx, 12*time.Second)
				err := w.Fetch(req)
				cancel()
				stats.mu.Lock()
				if err != nil {
					stats.Errors[w.Name]++
				} else {
					stats.LastSuccess[w.Name] = time.Now()
				}
				stats.mu.Unlock()
				if ctx.Err() != nil {
					return nil
				}
				if err != nil {
					log.Warn("provider refresh failed", "provider", w.Name, "error", err.Error(), "duration", time.Since(started))
				} else {
					log.Debug("provider updated", "provider", w.Name, "duration", time.Since(started))
				}
				select {
				case <-ctx.Done():
					return nil
				case <-ticker.C:
				}
			}
		})
	}
	return group.Wait()
}
