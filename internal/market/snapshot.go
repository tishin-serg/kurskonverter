package market

import (
	"maps"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/tishin-serg/kurskonverter/internal/domain"
)

// Store never exposes the published pointer: both writers and readers get owned
// copies. Decimal values are immutable and can safely be shared.
type Store struct {
	mu  sync.Mutex
	ptr atomic.Pointer[domain.MarketSnapshot]
}

func New() *Store { s := &Store{}; s.ptr.Store(domain.EmptySnapshot()); return s }
func clone(s *domain.MarketSnapshot) *domain.MarketSnapshot {
	out := *s
	out.Books = maps.Clone(s.Books)
	for k, b := range out.Books {
		b.Asks = slices.Clone(b.Asks)
		b.Bids = slices.Clone(b.Bids)
		out.Books[k] = b
	}
	out.Instruments = maps.Clone(s.Instruments)
	out.Fees = maps.Clone(s.Fees)
	out.P2P = maps.Clone(s.P2P)
	for k, d := range out.P2P {
		d.Value = slices.Clone(d.Value)
		for i := range d.Value {
			d.Value[i].PaymentMethods = slices.Clone(d.Value[i].PaymentMethods)
		}
		out.P2P[k] = d
	}
	out.Exchange.Value = slices.Clone(s.Exchange.Value)
	for i := range out.Exchange.Value {
		out.Exchange.Value[i].Warnings = slices.Clone(out.Exchange.Value[i].Warnings)
	}
	return &out
}
func (s *Store) Read() *domain.MarketSnapshot { return clone(s.ptr.Load()) }

// Update takes a short local mutation only. Network IO must happen before it.
func (s *Store) Update(f func(*domain.MarketSnapshot)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := clone(s.ptr.Load())
	f(next)
	s.ptr.Store(clone(next))
}
