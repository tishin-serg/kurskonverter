package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/domain"
)

type Config struct {
	BestChangeFrom                                                                  string
	OKXKey, OKXSecret, OKXPassphrase, WalletKey, WalletWithdrawalFee, BestChangeKey string
	BybitP2PZeroFeeFallback                                                         bool
	BybitKey, BybitSecret                                                           string
	Token, DBPath, HTTPAddr, Mode, P2PSource                                        string
	Filter                                                                          domain.Filter
	BybitRefresh, WalletRefresh, BestRefresh, FeeRefresh, BookRefresh               time.Duration
}

func Load() (Config, error) {
	c := Config{Token: os.Getenv("TELEGRAM_BOT_TOKEN"), DBPath: value("SQLITE_PATH", "data/bot.db"), HTTPAddr: value("HTTP_ADDR", ":8080"), Mode: value("DATA_MODE", "live"), P2PSource: value("BYBIT_P2P_SOURCE", "official")}
	c.BybitKey = os.Getenv("BYBIT_API_KEY")
	c.BestChangeFrom = value("BESTCHANGE_FROM_CODE", "SBPRUB")
	c.OKXKey, c.OKXSecret, c.OKXPassphrase = os.Getenv("OKX_API_KEY"), os.Getenv("OKX_API_SECRET"), os.Getenv("OKX_API_PASSPHRASE")
	c.WalletKey, c.WalletWithdrawalFee, c.BestChangeKey = os.Getenv("WALLET_API_KEY"), os.Getenv("WALLET_GRAM_WITHDRAWAL_FEE_FALLBACK"), os.Getenv("BESTCHANGE_API_KEY")
	if c.OKXKey != "" || c.OKXSecret != "" || c.OKXPassphrase != "" {
		if c.OKXKey == "" || c.OKXSecret == "" || c.OKXPassphrase == "" {
			return c, fmt.Errorf("set all OKX_API_KEY, OKX_API_SECRET, OKX_API_PASSPHRASE")
		}
	}
	if c.WalletWithdrawalFee != "" {
		v, e := decimal.NewFromString(c.WalletWithdrawalFee)
		if e != nil || v.IsNegative() {
			return c, fmt.Errorf("invalid WALLET_GRAM_WITHDRAWAL_FEE_FALLBACK")
		}
	}
	c.BybitSecret = os.Getenv("BYBIT_API_SECRET")
	if v := os.Getenv("BYBIT_P2P_TAKER_FEE_FALLBACK"); v != "" {
		if v != "0" {
			return c, fmt.Errorf("BYBIT_P2P_TAKER_FEE_FALLBACK supports only explicit 0 or empty")
		}
		c.BybitP2PZeroFeeFallback = true
	}
	if (c.BybitKey == "") != (c.BybitSecret == "") {
		return c, fmt.Errorf("set both BYBIT_API_KEY and BYBIT_API_SECRET")
	}
	if c.Mode != "live" && c.Mode != "demo" {
		return c, fmt.Errorf("DATA_MODE must be live or demo")
	}
	if c.P2PSource != "official" && c.P2PSource != "web" {
		return c, fmt.Errorf("invalid BYBIT_P2P_SOURCE")
	}
	var err error
	c.Filter.MinCompletionRate, err = decimal.NewFromString(value("MIN_COMPLETION_RATE", "95"))
	if err != nil || c.Filter.MinCompletionRate.IsNegative() || c.Filter.MinCompletionRate.GreaterThan(decimal.NewFromInt(100)) {
		return c, fmt.Errorf("invalid MIN_COMPLETION_RATE")
	}
	c.Filter.MinOrdersCount, err = strconv.Atoi(value("MIN_ORDERS_COUNT", "50"))
	if err != nil || c.Filter.MinOrdersCount < 0 {
		return c, fmt.Errorf("invalid MIN_ORDERS_COUNT")
	}
	c.Filter.PaymentMethod = os.Getenv("PAYMENT_METHOD")
	if os.Getenv("BESTCHANGE_FROM_CODE") == "" && domain.CanonicalPayment(c.Filter.PaymentMethod) == "tbank" {
		c.BestChangeFrom = "TCSBRUB"
	}
	for _, x := range []struct {
		k, v string
		p    *time.Duration
	}{{"BYBIT_P2P_REFRESH", "20s", &c.BybitRefresh}, {"WALLET_P2P_REFRESH", "30s", &c.WalletRefresh}, {"BESTCHANGE_REFRESH", "20s", &c.BestRefresh}, {"FEE_REFRESH", "5m", &c.FeeRefresh}, {"ORDERBOOK_REFRESH", "5s", &c.BookRefresh}} {
		*x.p, err = time.ParseDuration(value(x.k, x.v))
		if err != nil || *x.p < time.Second || *x.p > time.Hour {
			return c, fmt.Errorf("invalid %s (1s..1h)", x.k)
		}
	}
	return c, nil
}
func value(k, def string) string {
	if s := os.Getenv(k); s != "" {
		return s
	}
	return def
}
