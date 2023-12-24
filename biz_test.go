package banexg

import (
	"testing"
)

func TestSetOptions(t *testing.T) {
	FakeApiKey := "123"
	e := Exchange{
		Options: map[string]interface{}{
			OptMarketType:    MarketMargin,
			OptApiKey:        FakeApiKey,
			OptPrecisionMode: PrecModeTickSize,
		},
	}
	e.Init()
	if e.PrecisionMode == PrecModeTickSize {
		t.Logf("Pass PrecisionMode")
	} else {
		t.Errorf("Fail PrecisionMode, cur %v, expect: %v", e.PrecisionMode, PrecModeTickSize)
	}
	if e.Creds.ApiKey == FakeApiKey {
		t.Logf("Pass ApiKey")
	} else {
		t.Errorf("Fail ApiKey, cur %v, expect: %v", e.Creds.ApiKey, FakeApiKey)
	}
	if e.MarketType == MarketMargin {
		t.Logf("Pass MarketType")
	} else {
		t.Errorf("Fail MarketType, cur %v, expect: %v", e.MarketType, MarketMargin)
	}
}

func TestCalcFee(t *testing.T) {
	symbol := "FOO/BAR"
	exg := Exchange{
		Markets: map[string]*Market{
			symbol: {
				ID:     "foobar",
				Symbol: symbol,
				Base:   "FOO",
				Quote:  "BAR",
				Settle: "BAR",
				Taker:  0.002,
				Maker:  0.001,
				Precision: &Precision{
					Price:  8,
					Amount: 8,
				},
			},
		},
	}
	amount := 10.
	price := 100.
	fee, err := exg.CalculateFee(symbol, OdTypeLimit, OdSideBuy, amount, price, false, nil)
	if err != nil {
		panic(err)
	}
	if fee.Cost != 2.0 {
		t.Errorf("taker fee: %v", fee)
	}
	fee, err = exg.CalculateFee(symbol, OdTypeLimit, OdSideBuy, amount, price, true, nil)
	if err != nil {
		panic(err)
	}
	if fee.Cost != 1.0 {
		t.Errorf("maker fee: %v", fee)
	}
}
