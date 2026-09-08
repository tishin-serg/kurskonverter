package config

import "testing"

func TestConfig(t *testing.T) {
	for _, key := range []string{"DATA_MODE", "MIN_COMPLETION_RATE", "MIN_ORDERS_COUNT", "BYBIT_P2P_SOURCE", "ORDERBOOK_REFRESH"} {
		t.Setenv(key, "")
	}
	c, e := Load()
	if e != nil || c.Filter.MinOrdersCount != 50 || c.Mode != "live" {
		t.Fatal(c, e)
	}
	for _, tc := range []struct{ k, v string }{{"DATA_MODE", "other"}, {"MIN_COMPLETION_RATE", "101"}, {"MIN_ORDERS_COUNT", "-1"}, {"BYBIT_P2P_SOURCE", "guess"}, {"ORDERBOOK_REFRESH", "0s"}} {
		t.Run(tc.k, func(t *testing.T) {
			t.Setenv(tc.k, tc.v)
			if _, e := Load(); e == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}
