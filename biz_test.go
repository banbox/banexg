package banexg

import "testing"

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
