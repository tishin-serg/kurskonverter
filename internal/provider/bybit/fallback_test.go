package bybit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tishin-serg/kurskonverter/internal/provider"
	"github.com/tishin-serg/kurskonverter/internal/provider/httpclient"
)

func TestExplicitZeroFeeFallback(t *testing.T) {
	for _, tc := range []struct {
		name, fee     string
		enabled, want bool
	}{{"disabled", "", false, false}, {"missing", "", true, true}, {"nonzero", "0.01", true, false}, {"invalid", "bad", true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.Replace(adJSON, `"buyFeeRate":"0"`, `"buyFeeRate":"`+tc.fee+`"`, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = fmt.Fprintf(w, `{"retCode":0,"result":{"count":1,"items":[%s]}}`, body)
			}))
			defer srv.Close()
			c := NewAuthenticated(httpclient.New(), "k", "s")
			c.BaseURL = srv.URL
			ads, e := (Official{Client: c, ZeroFeeFallback: tc.enabled}).GetOffers(context.Background(), provider.P2PRequest{Asset: "USDT", Fiat: "RUB"})
			if tc.want {
				if e != nil || len(ads) != 1 || !ads[0].FeeFallback {
					t.Fatal(ads, e)
				}
			} else if e == nil {
				t.Fatal("unexpected fallback")
			}
		})
	}
}
