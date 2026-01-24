package bybit

import (
	"testing"

	"github.com/banbox/banexg"
)

func bybitPrecPriceMust(t *testing.T, exg *Bybit, market *banexg.Market, price float64) float64 {
	t.Helper()
	v, err := exg.PrecPrice(market, price)
	if err != nil {
		t.Fatalf("PrecPrice failed: %v", err)
	}
	return v
}

func bybitPrecAmountMust(t *testing.T, exg *Bybit, market *banexg.Market, amount float64) float64 {
	t.Helper()
	v, err := exg.PrecAmount(market, amount)
	if err != nil {
		t.Fatalf("PrecAmount failed: %v", err)
	}
	return v
}
