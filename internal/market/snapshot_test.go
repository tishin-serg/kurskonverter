package market

import (
	"sync"
	"testing"

	"github.com/tishin-serg/kurskonverter/internal/domain"
)

func TestIsolation(t *testing.T) {
	s := New()
	var retained *domain.MarketSnapshot
	s.Update(func(n *domain.MarketSnapshot) {
		n.P2P["a"] = domain.Data[[]domain.P2POffer]{Value: []domain.P2POffer{{PaymentMethods: []string{"bank"}}}}
		n.Exchange.Value = []domain.ExchangeOffer{{Warnings: []string{"original"}}}
		retained = n
	})
	retained.P2P["a"].Value[0].PaymentMethods[0] = "bad"
	a := s.Read()
	a.P2P["a"].Value[0].PaymentMethods[0] = "changed"
	a.Exchange.Value[0].Warnings[0] = "changed"
	if s.Read().Exchange.Value[0].Warnings[0] != "original" {
		t.Fatal("mutable exchange warnings")
	}
	if s.Read().P2P["a"].Value[0].PaymentMethods[0] != "bank" {
		t.Fatal("mutable publication")
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Go(func() {
			for j := 0; j < 10; j++ {
				s.Update(func(n *domain.MarketSnapshot) { n.Books["a"] = domain.OrderBook{} })
				_ = s.Read()
			}
		})
	}
	wg.Wait()
}
