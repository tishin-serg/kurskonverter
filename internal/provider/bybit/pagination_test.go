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

func TestPagination(t *testing.T) {
	for _, scenario := range []string{"complete", "overlap", "duplicate", "empty", "too-many"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				body := adJSON
				count := 2
				if calls == 2 && scenario != "duplicate" {
					body = strings.Replace(adJSON, `"id":"a"`, `"id":"b"`, 1)
				}
				if scenario == "overlap" {
					count = 3
					if calls == 2 {
						body = adJSON + "," + body
					}
				}
				if calls == 2 && scenario == "empty" {
					body = ""
				}
				if scenario == "too-many" {
					count = 3001
				}
				_, _ = fmt.Fprintf(w, `{"ret_code":0,"result":{"count":%d,"items":[%s]}}`, count, body)
			}))
			defer srv.Close()
			c := NewAuthenticated(httpclient.New(), "key", "secret")
			c.BaseURL = srv.URL
			offers, e := (Official{Client: c}).GetOffers(context.Background(), provider.P2PRequest{Asset: "USDT", Fiat: "RUB"})
			if scenario == "complete" || scenario == "overlap" {
				if e != nil || len(offers) != 2 || calls != 2 {
					t.Fatal(offers, calls, e)
				}
			} else if e == nil {
				t.Fatal("partial or invalid market accepted")
			}
		})
	}
}
