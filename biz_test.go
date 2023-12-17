package banexg

import "testing"

func TestSetOptions(t *testing.T) {
	FakeApiKey := "123"
	e := Exchange{
		Options: map[string]interface{}{
			OptTradeMode:     TradeMargin,
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
	if e.TradeMode == TradeMargin {
		t.Logf("Pass TradeMode")
	} else {
		t.Errorf("Fail TradeMode, cur %v, expect: %v", e.TradeMode, TradeMargin)
	}
}
