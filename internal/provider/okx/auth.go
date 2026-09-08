package okx

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/domain"
	"github.com/tishin-serg/kurskonverter/internal/provider"
	"github.com/tishin-serg/kurskonverter/internal/provider/httpclient"
)

func NewAuthenticated(h *httpclient.Client, key, secret, passphrase string) *Client {
	c := New(h)
	c.key, c.secret, c.passphrase = key, secret, passphrase
	return c
}

func (c *Client) read(ctx context.Context, path string, out any) error {
	if c.key == "" || c.secret == "" || c.passphrase == "" {
		return provider.ErrNotConfigured
	}
	var envelope struct {
		Code string
		Data json.RawMessage
	}
	err := c.HTTP.Read(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
		if err != nil {
			return nil, err
		}
		ts := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		mac := hmac.New(sha256.New, []byte(c.secret))
		_, _ = mac.Write([]byte(ts + "GET" + path))
		req.Header.Set("OK-ACCESS-KEY", c.key)
		req.Header.Set("OK-ACCESS-PASSPHRASE", c.passphrase)
		req.Header.Set("OK-ACCESS-TIMESTAMP", ts)
		req.Header.Set("OK-ACCESS-SIGN", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
		return req, nil
	}, &envelope)
	if err != nil {
		return err
	}
	if envelope.Code != "0" {
		return fmt.Errorf("okx API rejected request (code %s)", safeCode(envelope.Code))
	}
	if err = json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("okx invalid response data")
	}
	return nil
}
func safeCode(s string) string {
	if _, e := strconv.Atoi(s); e != nil || len(s) > 8 {
		return "unknown"
	}
	return s
}

func (c *Client) GetTradingFee(ctx context.Context, symbol string) (domain.Fee, error) {
	var f domain.Fee
	var cfg []struct{ FeeType string }
	if err := c.read(ctx, "/api/v5/account/config", &cfg); err != nil {
		return f, err
	}
	if len(cfg) != 1 || cfg[0].FeeType != "0" {
		return f, fmt.Errorf("okx requires received-currency fee mode (feeType=0)")
	}
	var rows []struct {
		InstType, Taker string
		FeeGroup        []struct{ Taker string }
	}
	if err := c.read(ctx, "/api/v5/account/trade-fee?instType=SPOT&instId="+url.QueryEscape(symbol), &rows); err != nil {
		return f, err
	}
	if len(rows) != 1 || rows[0].InstType != "SPOT" {
		return f, fmt.Errorf("okx spot fee unavailable")
	}
	rate := rows[0].Taker
	if len(rows[0].FeeGroup) == 1 {
		rate = rows[0].FeeGroup[0].Taker
	} else if len(rows[0].FeeGroup) > 1 {
		return f, fmt.Errorf("okx ambiguous fee groups")
	}
	d, err := decimal.NewFromString(rate)
	if err != nil || d.IsPositive() || !d.GreaterThan(decimal.NewFromInt(-1)) {
		return f, fmt.Errorf("okx unsupported taker fee")
	}
	f.Rate = d.Neg()
	f.Enabled = true
	f.UpdatedAt = time.Now()
	return f, nil
}

type currency struct {
	Ccy, Chain, MinFee, MaxFee, Fee, MinWd, MaxWd, WdTickSz, MinDep string
	CanWd, CanDep                                                   bool
}

// GetGRAMDeposit verifies the native TON network, never an unrelated GRAM token.
func (c *Client) GetGRAMDeposit(ctx context.Context) (domain.Fee, error) {
	rows, err := c.currencies(ctx, "GRAM")
	if err != nil {
		return domain.Fee{}, err
	}
	for _, r := range rows {
		if r.Ccy == "GRAM" && r.Chain == "GRAM-The Open Network (TON)" {
			if !r.CanDep {
				return domain.Fee{}, fmt.Errorf("okx GRAM deposit suspended")
			}
			min, e := httpclient.Positive(r.MinDep)
			if e != nil {
				return domain.Fee{}, e
			}
			return domain.Fee{Min: min, Enabled: true, UpdatedAt: time.Now()}, nil
		}
	}
	return domain.Fee{}, fmt.Errorf("okx native GRAM-TON deposit network unavailable")
}
func (c *Client) currencies(ctx context.Context, asset string) ([]currency, error) {
	var rows []currency
	err := c.read(ctx, "/api/v5/asset/currencies?ccy="+url.QueryEscape(asset), &rows)
	return rows, err
}
func (c *Client) GetWithdrawalFee(ctx context.Context, asset, network string) (domain.Fee, error) {
	var f domain.Fee
	if asset != "BTC" || network != "BTC" {
		return f, fmt.Errorf("okx unsupported withdrawal network")
	}
	rows, err := c.currencies(ctx, asset)
	if err != nil {
		return f, err
	}
	for _, r := range rows {
		if r.Ccy != asset || r.Chain != "BTC-Bitcoin" {
			continue
		}
		if !r.CanWd {
			return f, fmt.Errorf("okx BTC withdrawal suspended")
		}
		rawFee := r.Fee
		if rawFee == "" {
			if r.MinFee != r.MaxFee {
				return f, fmt.Errorf("okx variable withdrawal fee unsupported")
			}
			rawFee = r.MinFee
		}
		f.Fixed, err = decimal.NewFromString(rawFee)
		if err != nil || f.Fixed.IsNegative() {
			return f, fmt.Errorf("okx invalid withdrawal fee")
		}
		f.Min, err = httpclient.Positive(r.MinWd)
		if err != nil {
			return f, err
		}
		f.Max, err = httpclient.Positive(r.MaxWd)
		if err != nil {
			return f, err
		}
		n, e := strconv.Atoi(r.WdTickSz)
		if e != nil || n < 0 || n > 18 {
			return f, fmt.Errorf("okx invalid withdrawal precision")
		}
		f.Step = decimal.New(1, -int32(n))
		f.Enabled = true
		f.UpdatedAt = time.Now()
		return f, nil
	}
	return f, fmt.Errorf("okx Bitcoin network unavailable")
}
