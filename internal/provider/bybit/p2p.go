package bybit

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/domain"
	"github.com/tishin-serg/kurskonverter/internal/provider"
)

// The official specification contains both JSON numbers and strings for side
// and reliability fields. Preserve the lexeme instead of converting to float.
type scalar string

func (s *scalar) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return fmt.Errorf("null numeric field")
	}
	if len(b) > 0 && b[0] == '"' {
		var v string
		if e := json.Unmarshal(b, &v); e != nil {
			return e
		}
		*s = scalar(v)
	} else {
		*s = scalar(b)
	}
	return nil
}

type ad struct {
	ID, TokenID, CurrencyID, NickName                                                  string
	Side, Price, LastQuantity, MinAmount, MaxAmount, RecentOrderNum, RecentExecuteRate scalar
	Payments                                                                           []string
	SymbolInfo                                                                         struct {
		BuyFeeRate, SellFeeRate scalar
		Token                   struct{ Scale *int }
	}
}
type Official struct {
	Client          *Client
	ZeroFeeFallback bool
}

func (o Official) GetOffers(ctx context.Context, req provider.P2PRequest) ([]domain.P2POffer, error) {
	if o.Client == nil {
		return nil, ErrCredentials
	}
	if req.Asset != "USDT" || req.Fiat != "RUB" {
		return nil, fmt.Errorf("bybit P2P adapter supports RUB/USDT only")
	}
	var offers []domain.P2POffer
	seen := map[string]bool{}
	read := 0
	rejected := map[string]int{}
	for page := 1; page <= 10; page++ {
		// side is the maker's side: buy USDT from sell advertisements.
		body := map[string]string{"tokenId": req.Asset, "currencyId": req.Fiat, "side": "1", "page": strconv.Itoa(page), "size": "300"}
		var result struct {
			Count *int
			Items []ad
		}
		if e := o.Client.read(ctx, "/v5/p2p/item/online", nil, body, &result); e != nil {
			return nil, e
		}
		if result.Count == nil || *result.Count < 0 || *result.Count > 3000 {
			return nil, fmt.Errorf("bybit P2P invalid or excessive page count")
		}
		uniqueBefore := len(seen)
		for _, a := range result.Items {
			if a.ID == "" {
				return nil, fmt.Errorf("bybit P2P missing ad ID")
			}
			// Ads can move between pages while the live order list changes.
			// Count rows for pagination, but never count an offer's liquidity twice.
			read++
			if seen[a.ID] {
				continue
			}
			seen[a.ID] = true
			fallback := false
			if o.ZeroFeeFallback {
				// Missing fields only; an explicit non-zero fee is never overridden.
				if a.SymbolInfo.BuyFeeRate == "" {
					a.SymbolInfo.BuyFeeRate = "0"
					fallback = true
				}
				if a.SymbolInfo.SellFeeRate == "" {
					a.SymbolInfo.SellFeeRate = "0"
					fallback = true
				}
			}
			v, e := normalizeAd(a, req)
			if e != nil {
				rejected[e.Error()]++
				continue
			}
			v.FeeFallback = fallback
			offers = append(offers, v)
		}
		if len(result.Items) > 0 && len(seen) == uniqueBefore {
			return nil, fmt.Errorf("bybit P2P repeated page without progress")
		}
		if read >= *result.Count {
			if read > 0 && len(offers) == 0 {
				return nil, fmt.Errorf("bybit P2P: all %d ads rejected: %v", read, rejected)
			}
			return offers, nil
		}
		if len(result.Items) == 0 {
			return nil, fmt.Errorf("bybit P2P incomplete pagination")
		}
	}
	return nil, fmt.Errorf("bybit P2P page limit reached")
}
func normalizeAd(a ad, req provider.P2PRequest) (domain.P2POffer, error) {
	var o domain.P2POffer
	if a.TokenID != req.Asset || a.CurrencyID != req.Fiat || a.Side != "1" {
		return o, fmt.Errorf("wrong ad direction")
	}
	// Non-zero or missing P2P fees are not silently assumed free. This adapter
	// accepts only explicit zero rates on both sides until fee units are modeled.
	for _, r := range []scalar{a.SymbolInfo.BuyFeeRate, a.SymbolInfo.SellFeeRate} {
		v, e := nonnegative(string(r))
		if e != nil {
			return o, fmt.Errorf("missing or invalid P2P fee")
		}
		if !v.IsZero() {
			return o, fmt.Errorf("nonzero P2P fee (%s)", v.String())
		}
	}
	precision := a.SymbolInfo.Token.Scale
	if precision == nil || *precision < 0 || *precision > 18 {
		return o, fmt.Errorf("missing P2P asset precision")
	}
	var e error
	o.Price, e = nonnegative(string(a.Price))
	if e != nil || !o.Price.IsPositive() {
		return o, fmt.Errorf("invalid P2P price")
	}
	o.MinFiat, e = nonnegative(string(a.MinAmount))
	if e != nil {
		return o, e
	}
	o.MaxFiat, e = nonnegative(string(a.MaxAmount))
	if e != nil || o.MaxFiat.LessThan(o.MinFiat) {
		return o, fmt.Errorf("invalid P2P limits")
	}
	o.AvailableAsset, e = nonnegative(string(a.LastQuantity))
	if e != nil {
		return o, e
	}
	o.CompletionRate, e = nonnegative(string(a.RecentExecuteRate))
	if e != nil || o.CompletionRate.GreaterThan(decimal.NewFromInt(100)) {
		return o, fmt.Errorf("invalid P2P reliability")
	}
	o.OrdersCount, e = strconv.Atoi(string(a.RecentOrderNum))
	if e != nil || o.OrdersCount < 0 {
		return o, fmt.Errorf("invalid P2P order count")
	}
	o.Provider = "Bybit"
	o.OfferID = a.ID
	o.Asset = req.Asset
	o.Fiat = req.Fiat
	o.MerchantName = a.NickName
	o.PaymentMethods = a.Payments
	o.AssetStep = decimal.New(1, -int32(*precision))
	return o, nil
}
