package okx

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tishin-serg/kurskonverter/internal/provider/httpclient"
)

func TestAuthenticatedFeesAndNetwork(t *testing.T) {
	mode := "0"
	canWd := true
	chain := "GRAM-The Open Network (TON)"
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts := r.Header.Get("OK-ACCESS-TIMESTAMP")
		mac := hmac.New(sha256.New, []byte("secret"))
		mac.Write([]byte(ts + "GET" + r.URL.RequestURI()))
		if ts == "" || r.Header.Get("OK-ACCESS-KEY") != "key" || r.Header.Get("OK-ACCESS-PASSPHRASE") != "pass" || r.Header.Get("OK-ACCESS-SIGN") != base64.StdEncoding.EncodeToString(mac.Sum(nil)) {
			t.Error("invalid signature")
		}
		switch r.URL.Path {
		case "/api/v5/account/config":
			fmt.Fprintf(w, `{"code":"0","data":[{"feeType":%q}]}`, mode)
		case "/api/v5/account/trade-fee":
			if r.URL.Query().Get("instId") != "BTC-USDT" {
				t.Error("missing symbol")
			}
			fmt.Fprint(w, `{"code":"0","data":[{"instType":"SPOT","feeGroup":[{"taker":"-0.001"}]}]}`)
		case "/api/v5/asset/currencies":
			if r.URL.Query().Get("ccy") == "GRAM" {
				fmt.Fprintf(w, `{"code":"0","data":[{"ccy":"GRAM","chain":%q,"canDep":true,"minDep":"0.001"}]}`, chain)
			} else {
				fmt.Fprintf(w, `{"code":"0","data":[{"ccy":"BTC","chain":"BTC-Bitcoin","canWd":%t,"minFee":"0.000029","maxFee":"0.000029","minWd":"0.00008","maxWd":"500","wdTickSz":"8"}]}`, canWd)
			}
		default:
			t.Error(r.URL.Path)
		}
	}))
	defer s.Close()
	c := NewAuthenticated(httpclient.New(), "key", "secret", "pass")
	c.BaseURL = s.URL
	f, e := c.GetTradingFee(context.Background(), "BTC-USDT")
	if e != nil || f.Rate.String() != "0.001" {
		t.Fatal(f, e)
	}
	mode = "1"
	if _, e = c.GetTradingFee(context.Background(), "BTC-USDT"); e == nil {
		t.Fatal("wrong fee currency accepted")
	}
	f, e = c.GetWithdrawalFee(context.Background(), "BTC", "BTC")
	if e != nil || f.Fixed.String() != "0.000029" || f.Step.String() != "0.00000001" {
		t.Fatal(f, e)
	}
	canWd = false
	if _, e = c.GetWithdrawalFee(context.Background(), "BTC", "BTC"); e == nil {
		t.Fatal("suspended withdrawal accepted")
	}
	if _, e = c.GetGRAMDeposit(context.Background()); e != nil {
		t.Fatal(e)
	}
	chain = "GRAM-ERC20"
	if _, e = c.GetGRAMDeposit(context.Background()); e == nil {
		t.Fatal("wrong chain accepted")
	}
}
