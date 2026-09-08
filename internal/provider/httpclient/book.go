package httpclient

import (
	"fmt"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tishin-serg/kurskonverter/internal/domain"
)

func Levels(rows [][]string) ([]domain.OrderBookLevel, error) {
	out := make([]domain.OrderBookLevel, 0, len(rows))
	for _, r := range rows {
		if len(r) < 2 {
			return nil, fmt.Errorf("invalid level")
		}
		p, e := decimal.NewFromString(r[0])
		if e != nil || !p.IsPositive() {
			return nil, fmt.Errorf("invalid price")
		}
		a, e := decimal.NewFromString(r[1])
		if e != nil || !a.IsPositive() {
			return nil, fmt.Errorf("invalid amount")
		}
		out = append(out, domain.OrderBookLevel{Price: p, Amount: a})
	}
	return out, nil
}
func Timestamp(s string) (time.Time, error) {
	n, e := strconv.ParseInt(s, 10, 64)
	if e != nil || n <= 0 {
		return time.Time{}, fmt.Errorf("invalid source timestamp")
	}
	t := time.UnixMilli(n)
	if t.After(time.Now().Add(time.Second * 5)) {
		return time.Time{}, fmt.Errorf("future source timestamp")
	}
	return t, nil
}
func Positive(s string) (decimal.Decimal, error) {
	d, e := decimal.NewFromString(s)
	if e != nil || !d.IsPositive() {
		return d, fmt.Errorf("invalid instrument precision")
	}
	return d, nil
}
