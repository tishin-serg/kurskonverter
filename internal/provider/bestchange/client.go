package bestchange

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/domain"
	"github.com/tishin-serg/kurskonverter/internal/provider"
	"github.com/tishin-serg/kurskonverter/internal/provider/httpclient"
)

type Client struct {
	HTTP          *httpclient.Client
	BaseURL       string
	key, fromCode string
	mu            sync.Mutex
	metadataAt    time.Time
	pair, payment string
	changers      map[int]string
}

func New(h *httpclient.Client, key, fromCode string) *Client {
	return &Client{HTTP: h, BaseURL: "https://mirror2.bestchange.app", key: key, fromCode: fromCode}
}
func (c *Client) read(ctx context.Context, path string, out any) error {
	if c.key == "" {
		return provider.ErrNotConfigured
	}
	return c.HTTP.Read(ctx, func() (*http.Request, error) {
		r, e := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v2/"+url.PathEscape(c.key)+path, nil)
		// Some official mirrors stall on reused connections. At this polling
		// frequency a fresh TLS connection is preferable to incomplete data.
		if e == nil {
			r.Close = true
		}
		return r, e
	}, out)
}
func (c *Client) metadata(ctx context.Context) error {
	if time.Since(c.metadataAt) < time.Hour {
		return nil
	}
	var currencies struct {
		Currencies []struct {
			ID           int
			Code, Name   string
			Cash, Crypto bool
		}
	}
	if e := c.read(ctx, "/currencies/ru", &currencies); e != nil {
		return fmt.Errorf("bestchange currencies: %w", e)
	}
	from, to := 0, 0
	payment := ""
	for _, v := range currencies.Currencies {
		if v.Code == c.fromCode && !v.Cash && !v.Crypto && strings.HasSuffix(v.Code, "RUB") {
			from = v.ID
			payment = v.Name
		}
		if v.Code == "BTC" && v.Crypto && !v.Cash {
			to = v.ID
		}
	}
	if from <= 0 || to <= 0 {
		return fmt.Errorf("bestchange RUB payment currency/BTC unavailable")
	}
	c.pair = strconv.Itoa(from) + "-" + strconv.Itoa(to)
	c.payment = payment
	c.metadataAt = time.Now()
	return nil
}

// Names are optional presentation data; live rates identify each exchanger
// independently. A failed large directory download must not suppress quotes.
func (c *Client) RefreshChangers(ctx context.Context) error {
	var changers struct {
		Changers []struct {
			ID     int
			Name   string
			Active bool
		}
	}
	if e := c.read(ctx, "/changers/ru", &changers); e != nil {
		return e
	}
	names := map[int]string{}
	for _, v := range changers.Changers {
		if v.ID > 0 {
			if v.Active {
				names[v.ID] = v.Name
			} else {
				names[v.ID] = ""
			}
		}
	}
	if len(names) == 0 {
		return fmt.Errorf("bestchange no active exchangers")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.changers = names
	return nil
}
func (c *Client) GetRates(ctx context.Context, pair string) ([]domain.ExchangeOffer, error) {
	if pair != "RUB-BTC" {
		return nil, fmt.Errorf("bestchange unsupported pair")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := c.metadata(ctx); e != nil {
		return nil, e
	}
	var raw struct {
		Rates map[string][]struct {
			Changer                               int
			Rate, Rankrate, Reserve, Inmin, Inmax decimal.Decimal
			Marks                                 []string
			Extra                                 json.RawMessage
		}
	}
	if e := c.read(ctx, "/rates/"+c.pair, &raw); e != nil {
		return nil, fmt.Errorf("bestchange rates: %w", e)
	}
	var out []domain.ExchangeOffer
	for _, v := range raw.Rates[c.pair] {
		name, ok := c.changers[v.Changer]
		if v.Changer <= 0 || (ok && name == "") {
			continue
		}
		if !ok {
			name = "Обменник BestChange №" + strconv.Itoa(v.Changer)
		}
		// rankrate is computed for $300, never for the requested amount. The public
		// v2 schema does not define extra fees: reject them instead of guessing.
		extra := strings.TrimSpace(string(v.Extra))
		if extra != "" && extra != "[]" && extra != "{}" && extra != "null" {
			continue
		}
		if !v.Rate.Equal(v.Rankrate) || slices.Contains(v.Marks, "percent") || slices.Contains(v.Marks, "unstable") || slices.Contains(v.Marks, "atm") || slices.Contains(v.Marks, "purse") {
			continue
		}
		if !v.Rate.IsPositive() || !v.Reserve.IsPositive() || v.Inmin.IsNegative() || !v.Inmax.IsPositive() || v.Inmin.GreaterThan(v.Inmax) {
			continue
		}
		out = append(out, domain.ExchangeOffer{Exchanger: name, URL: "https://www.bestchange.ru/click.php?id=" + strconv.Itoa(v.Changer), PaymentMethod: c.payment, Rate: v.Rate, MinFiat: v.Inmin, MaxFiat: v.Inmax, ReserveBTC: v.Reserve, Enabled: true, Warnings: markWarnings(v.Marks)})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("bestchange no offers with fully known fees")
	}
	return out, nil
}
func markWarnings(marks []string) []string {
	var out []string
	for _, m := range marks {
		switch m {
		case "floating":
			out = append(out, "Обменник не фиксирует курс при создании заявки")
		case "cardverify":
			out = append(out, "Возможна проверка банковской карты")
		case "verifying":
			out = append(out, "Возможна проверка документов обменником")
		case "reg":
			out = append(out, "У обменника требуется регистрация")
		case "manual":
			out = append(out, "Обмен проводится вручную или полуавтоматически")
		case "delay":
			out = append(out, "Обменник может задерживать перевод")
		case "card2card", "otherin":
			out = append(out, "Возможна отдельная комиссия вашего банка/платёжной системы")
		}
	}
	return out
}
