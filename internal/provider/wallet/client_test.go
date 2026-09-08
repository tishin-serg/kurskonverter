package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tishin-serg/kurskonverter/internal/provider"
	"github.com/tishin-serg/kurskonverter/internal/provider/httpclient"
)

func TestOffers(t *testing.T) {
	status := "SUCCESS"
	rate := "0.9875"
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("X-API-Key") != "test-key" {
			t.Error("request authentication")
		}
		var body struct {
			CryptoCurrency, FiatCurrency, Side string
			Page, PageSize                     int
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.CryptoCurrency != "TON" || body.Side != "SELL" || body.FiatCurrency != "RUB" || body.PageSize != 50 {
			t.Error(body)
		}
		fmt.Fprintf(w, `{"status":%q,"data":[{"id":"1","nickname":"merchant","cryptoCurrency":"TON","fiatCurrency":"RUB","side":"SELL","price":"350.25","lastQuantity":"200","minAmount":"1000","maxAmount":"50000","executeRate":%q,"orderNum":156,"payments":["sberbank"]}]}`, status, rate)
	}))
	defer s.Close()
	c := New(httpclient.New(), "test-key", "0.05")
	c.BaseURL = s.URL
	offers, e := c.GetOffers(context.Background(), provider.P2PRequest{Asset: "GRAM", Fiat: "RUB"})
	if e != nil || len(offers) != 1 {
		t.Fatal(e, offers)
	}
	o := offers[0]
	if o.Asset != "GRAM" || o.CompletionRate.String() != "98.75" || o.OrdersCount != 156 || o.AvailableAsset.String() != "200" {
		t.Fatal(o)
	}
	rate = "98.75"
	if _, e = c.GetOffers(context.Background(), provider.P2PRequest{Asset: "GRAM", Fiat: "RUB"}); e == nil {
		t.Fatal("accepted invalid rating scale")
	}
	status = "ERROR"
	if _, e = c.GetOffers(context.Background(), provider.P2PRequest{Asset: "GRAM", Fiat: "RUB"}); e == nil {
		t.Fatal("accepted failure")
	}
	fee, e := c.GetWithdrawalFee(context.Background(), "GRAM", "TON")
	if e != nil || !fee.Fallback || fee.Fixed.String() != "0.05" {
		t.Fatal(fee, e)
	}
	c.WithdrawalFee = ""
	if _, e = c.GetWithdrawalFee(context.Background(), "GRAM", "TON"); e == nil {
		t.Fatal("hidden fallback")
	}
}
