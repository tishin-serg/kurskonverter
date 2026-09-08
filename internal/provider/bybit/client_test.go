package bybit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tishin-serg/kurskonverter/internal/provider/httpclient"
)

func TestBook(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("category") != "spot" {
			t.Error("category")
		}
		_, _ = w.Write([]byte(`{"retCode":0,"result":{"s":"BTCUSDT","a":[["100","2"]],"b":[["99","1"]],"ts":1700000000000}}`))
	}))
	defer s.Close()
	c := New(httpclient.New())
	c.BaseURL = s.URL
	b, e := c.GetOrderBook(context.Background(), "BTCUSDT")
	if e != nil || len(b.Asks) != 1 || b.UpdatedAt.UnixMilli() != 1700000000000 {
		t.Fatal(b, e)
	}
}
