package route

import (
	"testing"
	"time"
)

func TestMarketAndWithdrawalMax(t *testing.T) {
	now := time.Now()
	s := fixture(now)
	inst := s.Instruments["bybit:BTCUSDT"]
	inst.MaxBase = d(".001")
	s.Instruments["bybit:BTCUSDT"] = inst
	trade := Trade{Key: "bybit:BTCUSDT", FeeKey: "bybit:trade", Buy: true}
	if _, _, _, e := trade.Backward(d(".01"), s, now); e == nil {
		t.Fatal("market maximum ignored")
	}
	f := s.Fees["bybit:BTC"]
	f.Max = d(".001")
	s.Fees["bybit:BTC"] = f
	if _, _, _, e := (Withdrawal{Key: "bybit:BTC"}).Backward(d(".01"), s, now); e == nil {
		t.Fatal("withdrawal maximum ignored")
	}
}
