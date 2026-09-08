package bybit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/domain"
	"github.com/tishin-serg/kurskonverter/internal/provider"
	"github.com/tishin-serg/kurskonverter/internal/provider/httpclient"
	"github.com/tishin-serg/kurskonverter/internal/route"
)

const adJSON = `{"id":"a","tokenId":"USDT","currencyId":"RUB","side":1,"price":"90","lastQuantity":"10000","minAmount":"100","maxAmount":"1000000","nickName":"test seller","recentOrderNum":100,"recentExecuteRate":"99","payments":["bank"],"symbolInfo":{"buyFeeRate":"0","sellFeeRate":"0","token":{"scale":6}}}`

func TestBybitFullRoute(t *testing.T) {
	ctx := context.Background()
	key, secret := "test-key", "test-secret"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v5/market/") {
			body, _ := io.ReadAll(r.Body)
			payload := r.URL.RawQuery
			if r.Method == "POST" {
				payload = string(body)
				var b map[string]string
				if e := json.Unmarshal(body, &b); e != nil || b["side"] != "1" || b["tokenId"] != "USDT" {
					t.Error("incorrect P2P request")
				}
			}
			mac := hmac.New(sha256.New, []byte(secret))
			_, _ = mac.Write([]byte(r.Header.Get("X-BAPI-TIMESTAMP") + key + "5000" + payload))
			if r.Header.Get("X-BAPI-SIGN") != hex.EncodeToString(mac.Sum(nil)) || r.Header.Get("X-BAPI-API-KEY") != key {
				t.Error("invalid signature")
			}
		}
		var body string
		switch r.URL.Path {
		case "/v5/market/orderbook":
			body = fmt.Sprintf(`{"retCode":0,"result":{"s":"BTCUSDT","a":[["100000","1"]],"b":[["99999","1"]],"ts":%d}}`, time.Now().UnixMilli())
		case "/v5/market/instruments-info":
			body = `{"retCode":0,"result":{"list":[{"symbol":"BTCUSDT","status":"Trading","baseCoin":"BTC","quoteCoin":"USDT","lotSizeFilter":{"basePrecision":"0.00000001","quotePrecision":"0.000001","maxMarketOrderQty":"100","maxOrderAmt":"10000000","minOrderQty":"0.00001","minOrderAmt":"1"},"priceFilter":{"tickSize":"0.01"}}]}}`
		case "/v5/account/fee-rate":
			body = `{"retCode":0,"result":{"category":"spot","list":[{"symbol":"BTCUSDT","takerFeeRate":"0.001"}]}}`
		case "/v5/asset/coin/query-info":
			body = `{"retCode":0,"result":{"rows":[{"coin":"BTC","chains":[{"chain":"BTC","chainWithdraw":"1","withdrawFee":"0.00005","withdrawPercentageFee":"0","withdrawMin":"0.0001","withdrawMax":"1","minAccuracy":"8"}]}]}}`
		case "/v5/p2p/item/online":
			body = `{"ret_code":0,"result":{"count":1,"items":[` + adJSON + `]}}`
		default:
			t.Error("unexpected endpoint")
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	c := NewAuthenticated(httpclient.New(), key, secret)
	c.BaseURL = srv.URL
	s := domain.EmptySnapshot()
	var e error
	s.Books["bybit:BTCUSDT"], e = c.GetOrderBook(ctx, "BTCUSDT")
	if e != nil {
		t.Fatal(e)
	}
	s.Instruments["bybit:BTCUSDT"], e = c.GetInstrument(ctx, "BTCUSDT")
	if e != nil {
		t.Fatal(e)
	}
	s.Fees["bybit:trade"], e = c.GetTradingFee(ctx, "BTCUSDT")
	if e != nil {
		t.Fatal(e)
	}
	s.Fees["bybit:BTC"], e = c.GetWithdrawalFee(ctx, "BTC", "BTC")
	if e != nil {
		t.Fatal(e)
	}
	offers, e := (Official{Client: c}).GetOffers(ctx, provider.P2PRequest{Asset: "USDT", Fiat: "RUB"})
	if e != nil {
		t.Fatal(e)
	}
	s.P2P["bybit"] = domain.Data[[]domain.P2POffer]{Value: offers, UpdatedAt: time.Now()}
	r := route.DefaultEngine().Calculate(ctx, decimal.RequireFromString(".01"), s, domain.Filter{PaymentMethod: "bank", MinCompletionRate: decimal.NewFromInt(95), MinOrdersCount: 50}, time.Now())
	if len(r.Quotes) != 1 || !r.Quotes[0].RUBRequired.Equal(decimal.RequireFromString("90540.63")) {
		t.Fatal(r)
	}
}
func TestAuthErrorsAndRetries(t *testing.T) {
	for _, body := range []string{`{}`, `{"result":{}}`, `{"retCode":0,"result":null}`, `{"retCode":10005,"retMsg":"test-secret"}`} {
		t.Run(body, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer s.Close()
			c := NewAuthenticated(httpclient.New(), "test-key", "test-secret")
			c.BaseURL = s.URL
			_, e := c.GetTradingFee(context.Background(), "BTCUSDT")
			if e == nil || strings.Contains(e.Error(), "test-secret") {
				t.Fatal(e)
			}
		})
	}
	calls := 0
	var stamps []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		stamps = append(stamps, r.Header.Get("X-BAPI-TIMESTAMP"))
		if calls == 1 {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte(`{"retCode":0,"result":{}}`))
	}))
	defer s.Close()
	c := NewAuthenticated(httpclient.New(), "test-key", "test-secret")
	c.BaseURL = s.URL
	var v any
	if e := c.read(context.Background(), "/read", url.Values{}, nil, &v); e != nil || calls != 2 || stamps[0] == stamps[1] {
		t.Fatal(calls, e, stamps)
	}
	if _, e := New(httpclient.New()).GetTradingFee(context.Background(), "BTCUSDT"); e != ErrCredentials {
		t.Fatal(e)
	}
}
func TestAdValidation(t *testing.T) {
	for _, bad := range []string{strings.Replace(adJSON, `"side":1`, `"side":0`, 1), strings.Replace(adJSON, `"buyFeeRate":"0"`, `"buyFeeRate":"0.01"`, 1), strings.Replace(adJSON, `"recentExecuteRate":"99"`, `"recentExecuteRate":null`, 1), strings.Replace(adJSON, `"scale":6`, `"scale":null`, 1)} {
		var a ad
		e := json.Unmarshal([]byte(bad), &a)
		if e == nil {
			_, e = normalizeAd(a, provider.P2PRequest{Asset: "USDT", Fiat: "RUB"})
		}
		if e == nil {
			t.Fatal("accepted invalid ad")
		}
	}
}
func TestWithdrawalValidation(t *testing.T) {
	for _, tc := range []struct{ name, withdraw, fee, percent, max string }{{"suspended", "0", ".00005", "0", "1"}, {"missing fee", "1", "", "0", "1"}, {"percent", "1", ".00005", ".01", "1"}, {"max", "1", ".00005", "0", ""}} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = fmt.Fprintf(w, `{"retCode":0,"result":{"rows":[{"coin":"BTC","chains":[{"chain":"BTC","chainWithdraw":%q,"withdrawFee":%q,"withdrawPercentageFee":%q,"withdrawMin":".0001","withdrawMax":%q,"minAccuracy":"8"}]}]}}`, tc.withdraw, tc.fee, tc.percent, tc.max)
			}))
			defer s.Close()
			c := NewAuthenticated(httpclient.New(), "k", "s")
			c.BaseURL = s.URL
			if _, e := c.GetWithdrawalFee(context.Background(), "BTC", "BTC"); e == nil {
				t.Fatal("invalid withdrawal accepted")
			}
		})
	}
}
