package bybit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tishin-serg/kurskonverter/internal/provider"
	"github.com/tishin-serg/kurskonverter/internal/provider/httpclient"
)

func TestUSDUsesBuyAdvertisements(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if req["side"] != "0" || req["currencyId"] != "USD" {
			t.Error(req)
		}
		row := strings.ReplaceAll(adJSON, `"currencyId":"RUB"`, `"currencyId":"USD"`)
		row = strings.Replace(row, `"side":1`, `"side":0`, 1)
		fmt.Fprintf(w, `{"ret_code":0,"result":{"count":1,"items":[%s]}}`, row)
	}))
	defer s.Close()
	c := NewAuthenticated(httpclient.New(), "key", "secret")
	c.BaseURL = s.URL
	rows, err := (Official{Client: c}).GetOffers(context.Background(), provider.P2PRequest{Asset: "USDT", Fiat: "USD", SellAsset: true})
	if err != nil || len(rows) != 1 || !rows[0].TakerSells || rows[0].Fiat != "USD" {
		t.Fatal(rows, err)
	}
}
