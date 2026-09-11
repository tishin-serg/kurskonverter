package bybit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/tishin-serg/kurskonverter/internal/provider"
	"github.com/tishin-serg/kurskonverter/internal/provider/httpclient"
)

func TestAdCountries(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      []string
		bad       bool
	}{
		{"disabled", `{"hasNationalLimit":0,"nationalLimit":"AFG"}`, nil, false},
		{"number", `{"hasNationalLimit":1,"nationalLimit":"RUS,GEO"}`, []string{"RUS", "GEO"}, false},
		{"string", `{"hasNationalLimit":"1","nationalLimit":" rus , GEO "}`, []string{"RUS", "GEO"}, false},
		{"missing", ``, nil, true},
		{"null", `null`, nil, true},
		{"empty", `{}`, nil, true},
		{"missing flag", `{"nationalLimit":"RUS"}`, nil, true},
		{"null flag", `{"hasNationalLimit":null}`, nil, true},
		{"invalid flag", `{"hasNationalLimit":2}`, nil, true},
		{"empty list", `{"hasNationalLimit":1,"nationalLimit":""}`, nil, true},
		{"wrong type", `{"hasNationalLimit":1,"nationalLimit":["RUS"]}`, nil, true},
		{"invalid code", `{"hasNationalLimit":1,"nationalLimit":"RUS,RU"}`, nil, true},
		{"trailing comma", `{"hasNationalLimit":1,"nationalLimit":"RUS,"}`, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := adCountries(json.RawMessage(tc.raw))
			if (err != nil) != tc.bad || !reflect.DeepEqual(got, tc.want) {
				t.Fatal(got, err)
			}
		})
	}
}

func TestP2PAvailabilityFiltering(t *testing.T) {
	// One blocked and one malformed ad must not remove the valid neighbour.
	blocked := strings.Replace(adJSON, `"id":"a"`, `"id":"blocked"`, 1)
	blocked = strings.Replace(blocked, `"blocked":"N"`, `"blocked":"Y"`, 1)
	unknown := strings.Replace(adJSON, `"id":"a"`, `"id":"unknown"`, 1)
	unknown = strings.Replace(unknown, `"hasNationalLimit":0`, `"hasNationalLimit":null`, 1)
	valid := strings.Replace(adJSON, `"hasNationalLimit":0,"nationalLimit":""`, `"hasNationalLimit":1,"nationalLimit":"RUS,GEO"`, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"ret_code":0,"result":{"count":3,"items":[%s,%s,%s]}}`, blocked, unknown, valid)
	}))
	defer srv.Close()
	c := NewAuthenticated(httpclient.New(), "key", "secret")
	c.BaseURL = srv.URL
	ads, err := (Official{Client: c}).GetOffers(context.Background(), provider.P2PRequest{Asset: "USDT", Fiat: "RUB"})
	if err != nil || len(ads) != 1 || !reflect.DeepEqual(ads[0].AllowedCountries, []string{"RUS", "GEO"}) {
		t.Fatal(ads, err)
	}
}
