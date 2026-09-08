package money

import (
	"testing"

	"github.com/tishin-serg/kurskonverter/internal/domain"
)

func TestP2PAssetPrecision(t *testing.T) {
	o := domain.P2POffer{Asset: "USDT", Fiat: "RUB", Price: d("100"), MaxFiat: d("10000"), AvailableAsset: d("10"), AssetStep: d(".01")}
	_, rub, e := SelectP2P(d("1.001"), "USDT", []domain.P2POffer{o}, domain.Filter{})
	if e != nil || !rub.Equal(d("101")) {
		t.Fatal(rub, e)
	}
	o.AvailableAsset = d("1.001")
	if _, _, e = SelectP2P(d("1.001"), "USDT", []domain.P2POffer{o}, domain.Filter{}); e == nil {
		t.Fatal("rounded amount exceeds stock")
	}
}
