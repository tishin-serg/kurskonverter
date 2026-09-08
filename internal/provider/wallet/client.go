package wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/domain"
	"github.com/tishin-serg/kurskonverter/internal/provider"
	"github.com/tishin-serg/kurskonverter/internal/provider/httpclient"
)

type Client struct {
	HTTP          *httpclient.Client
	BaseURL       string
	key           string
	WithdrawalFee string
}

func New(h *httpclient.Client, key, fee string) *Client {
	return &Client{HTTP: h, BaseURL: "https://p2p.walletbot.me", key: key, WithdrawalFee: fee}
}
func (c *Client) GetOffers(ctx context.Context, req provider.P2PRequest) ([]domain.P2POffer, error) {
	if c.key == "" {
		return nil, provider.ErrNotConfigured
	}
	var out []domain.P2POffer
	apiAsset := req.Asset
	// Wallet renamed TON to GRAM in the UI; integration-api v1 retains TON.
	if apiAsset == "GRAM" {
		apiAsset = "TON"
	}
	seen := map[string]bool{}
	for page := 1; page <= 100; page++ {
		body, _ := json.Marshal(struct {
			CryptoCurrency string `json:"cryptoCurrency"`
			FiatCurrency   string `json:"fiatCurrency"`
			Side           string `json:"side"`
			Page           int    `json:"page"`
			PageSize       int    `json:"pageSize"`
		}{apiAsset, req.Fiat, "SELL", page, 50})
		var raw struct {
			Status string
			Data   []struct {
				ID, Nickname, CryptoCurrency, FiatCurrency, Side, Price, LastQuantity, MinAmount, MaxAmount, ExecuteRate string
				Payments                                                                                                 []string
				OrderNum                                                                                                 int
			}
		}
		err := c.HTTP.Read(ctx, func() (*http.Request, error) {
			r, e := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/p2p/integration-api/v1/item/online", bytes.NewReader(body))
			if e != nil {
				return nil, e
			}
			r.Header.Set("X-API-Key", c.key)
			r.Header.Set("Content-Type", "application/json")
			return r, nil
		}, &raw)
		if err != nil {
			return nil, err
		}
		if raw.Status != "SUCCESS" {
			return nil, fmt.Errorf("wallet API rejected ads request")
		}
		newIDs := 0
		for _, r := range raw.Data {
			if r.ID == "" {
				return nil, fmt.Errorf("wallet missing ad ID")
			}
			if seen[r.ID] {
				continue
			}
			newIDs++
			seen[r.ID] = true
			if r.CryptoCurrency != apiAsset || r.FiatCurrency != req.Fiat || r.Side != "SELL" {
				continue
			}
			o := domain.P2POffer{Provider: "Wallet", Asset: req.Asset, Fiat: req.Fiat, MerchantName: r.Nickname, PaymentMethods: r.Payments, OrdersCount: r.OrderNum, AssetStep: decimal.New(1, -9)}
			valid := true
			for _, x := range []struct {
				s string
				d *decimal.Decimal
			}{{r.Price, &o.Price}, {r.LastQuantity, &o.AvailableAsset}, {r.MinAmount, &o.MinFiat}, {r.MaxAmount, &o.MaxFiat}} {
				v, e := httpclient.Positive(x.s)
				if e != nil {
					valid = false
					break
				}
				*x.d = v
			}
			rate, e := decimal.NewFromString(r.ExecuteRate)
			if e != nil || rate.IsNegative() || rate.GreaterThan(decimal.NewFromInt(1)) || r.OrderNum < 0 {
				valid = false
			}
			if !valid || o.MinFiat.GreaterThan(o.MaxFiat) {
				continue
			}
			o.CompletionRate = rate.Mul(decimal.NewFromInt(100))
			out = append(out, o)
		}
		if len(raw.Data) >= 50 && newIDs == 0 {
			return nil, fmt.Errorf("wallet repeated page")
		}
		if len(raw.Data) < 50 {
			if len(out) == 0 {
				return nil, fmt.Errorf("wallet no valid offers")
			}
			return out, nil
		}
	}
	return nil, fmt.Errorf("wallet pagination limit exceeded")
}

// P2P API does not expose withdrawal fees. The configured tariff is degraded.
func (c *Client) GetWithdrawalFee(_ context.Context, asset, network string) (domain.Fee, error) {
	if asset != "GRAM" || network != "TON" || c.WithdrawalFee == "" {
		return domain.Fee{}, provider.ErrNotConfigured
	}
	fee, e := decimal.NewFromString(c.WithdrawalFee)
	if e != nil || fee.IsNegative() {
		return domain.Fee{}, fmt.Errorf("invalid wallet fallback withdrawal fee")
	}
	return domain.Fee{Fixed: fee, Min: decimal.RequireFromString("0.1"), Step: decimal.New(1, -9), Fallback: true, Enabled: true, UpdatedAt: time.Now()}, nil
}
