package telegram

import "testing"

func TestParse(t *testing.T) {
	for _, s := range []string{"0.01", "0.01 BTC", " 0,01 btc ", "0.00000001"} {
		if _, e := ParseAmount(s); e != nil {
			t.Fatal(s, e)
		}
	}
	for _, s := range []string{"0", "-1", "NaN", "1e8", "0.000000001", "21000001", "1 BTC extra", ".01"} {
		if _, e := ParseAmount(s); e == nil {
			t.Fatal(s)
		}
	}
}
