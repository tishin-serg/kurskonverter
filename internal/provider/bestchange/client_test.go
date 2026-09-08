package bestchange

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tishin-serg/kurskonverter/internal/provider/httpclient"
)

func TestRatesRejectUnknownFees(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v2/test-key/") {
			t.Error("missing key")
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/currencies/ru"):
			fmt.Fprint(w, `{"currencies":[{"id":21,"code":"SBPRUB","name":"СБП RUB"},{"id":93,"code":"BTC","crypto":true}]}`)
		case strings.HasSuffix(r.URL.Path, "/changers/ru"):
			fmt.Fprint(w, `{"changers":[{"id":1,"name":"active","active":true},{"id":2,"name":"inactive","active":false}]}`)
		case strings.HasSuffix(r.URL.Path, "/rates/21-93"):
			fmt.Fprint(w, `{"rates":{"21-93":[
 {"changer":1,"rate":"7000000.123456789","rankrate":"7000000.123456789","reserve":"2","inmin":"1000","inmax":"1000000","extra":[],"marks":["verifying"]},
 {"changer":1,"rate":"1","rankrate":"1","reserve":"2","inmin":"1000","inmax":"1000000","extra":{"tofee":{"value":"0.0001","type":"absolute"}}},
 {"changer":1,"rate":"1","rankrate":"2","reserve":"2","inmin":"1000","inmax":"1000000","extra":[]},
 {"changer":2,"rate":"1","rankrate":"1","reserve":"2","inmin":"1000","inmax":"1000000","extra":[]},
 {"changer":1,"rate":"1","rankrate":"1","reserve":"2","inmin":"1000","inmax":"1000000","extra":[],"marks":["percent"]}]}}`)
		default:
			t.Error(r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	c := New(httpclient.New(), "test-key", "SBPRUB")
	c.BaseURL = s.URL
	if err := c.RefreshChangers(context.Background()); err != nil {
		t.Fatal(err)
	}
	rates, e := c.GetRates(context.Background(), "RUB-BTC")
	if e != nil || len(rates) != 1 {
		t.Fatal(rates, e)
	}
	if rates[0].Rate.String() != "7000000.123456789" || rates[0].PaymentMethod != "СБП RUB" || len(rates[0].Warnings) != 1 {
		t.Fatal(rates[0])
	}
}
