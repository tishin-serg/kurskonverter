package bybit

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/tishin-serg/kurskonverter/internal/provider/httpclient"
)

var ErrCredentials = errors.New("bybit: BYBIT_API_KEY and BYBIT_API_SECRET required")

type APIError struct{ Code int }

func (e *APIError) Error() string {
	return fmt.Sprintf("bybit API code %d (check API permissions, advertiser access and clock)", e.Code)
}
func NewAuthenticated(h *httpclient.Client, key, secret string) *Client {
	c := New(h)
	c.key = key
	c.secret = secret
	return c
}

type envelope struct {
	Code       *int            `json:"retCode"`
	LegacyCode *int            `json:"ret_code"`
	Result     json.RawMessage `json:"result"`
}

func (c *Client) read(ctx context.Context, path string, query url.Values, body any, out any) error {
	if c.key == "" || c.secret == "" {
		return ErrCredentials
	}
	method, payload := http.MethodGet, query.Encode()
	var data []byte
	if body != nil {
		method = http.MethodPost
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode bybit request")
		}
		payload = string(data)
	}
	var env envelope
	err := c.HTTP.Read(ctx, func() (*http.Request, error) {
		endpoint := c.BaseURL + path
		if method == http.MethodGet {
			endpoint += "?" + payload
		}
		r, e := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(data))
		if e != nil {
			return nil, e
		}
		ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
		mac := hmac.New(sha256.New, []byte(c.secret))
		_, _ = mac.Write([]byte(ts + c.key + "5000" + payload))
		r.Header.Set("X-BAPI-API-KEY", c.key)
		r.Header.Set("X-BAPI-TIMESTAMP", ts)
		r.Header.Set("X-BAPI-RECV-WINDOW", "5000")
		r.Header.Set("X-BAPI-SIGN", hex.EncodeToString(mac.Sum(nil)))
		r.Header.Set("Content-Type", "application/json")
		return r, nil
	}, &env)
	if err != nil {
		return err
	}
	code := env.Code
	if code == nil {
		code = env.LegacyCode
	}
	if code == nil {
		return fmt.Errorf("bybit response missing status")
	}
	if *code != 0 {
		return &APIError{Code: *code}
	}
	if env.LegacyCode != nil && *env.LegacyCode != 0 {
		return &APIError{Code: *env.LegacyCode}
	}
	if len(env.Result) == 0 || string(env.Result) == "null" {
		return fmt.Errorf("bybit response missing result")
	}
	if json.Unmarshal(env.Result, out) != nil {
		return fmt.Errorf("invalid bybit result")
	}
	return nil
}
