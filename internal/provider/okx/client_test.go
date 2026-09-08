package okx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tishin-serg/kurskonverter/internal/provider/httpclient"
)

func TestBook(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"0","data":[{"asks":[["100","2","0","1"]],"bids":[["99","1","0","1"]],"ts":"1700000000000"}]}`))
	}))
	defer s.Close()
	c := New(httpclient.New())
	c.BaseURL = s.URL
	b, e := c.GetOrderBook(context.Background(), "BTC-USDT")
	if e != nil || len(b.Asks) != 1 {
		t.Fatal(b, e)
	}
}
