package banexg

import (
	"net/http"
	"strings"
	"testing"

	"github.com/banbox/banexg/errs"
)

func TestMapHTTPError(t *testing.T) {
	tests := []struct {
		status int
		code   int
	}{
		{http.StatusUnauthorized, errs.CodeUnauthorized},
		{http.StatusForbidden, errs.CodeForbidden},
		{http.StatusTeapot, errs.CodeTemporarilyBanned},
		{http.StatusTooManyRequests, errs.CodeRateLimit},
		{http.StatusBadGateway, errs.CodeServerError},
	}
	for _, test := range tests {
		err := mapHTTPError(nil, test.status)
		if err.Code != test.code || err.BizCode != 0 {
			t.Fatalf("status %d: expected code %d without biz code, got %#v", test.status, test.code, err)
		}
	}
	if err := mapHTTPError(&Entry{Risky: true}, http.StatusBadGateway); err.Code != errs.CodeExecutionUnknown {
		t.Fatalf("risky HTTP failure must have unknown execution status, got %v", err)
	}
}

func TestDeprecatedBizCodeIsNotExposed(t *testing.T) {
	err := errs.NewMsg(errs.CodeOrderNotFound, "order missing")
	err.BizCode = -2013
	if strings.Contains(err.Short(), "2013") || strings.Contains(err.Error(), "2013") {
		t.Fatalf("deprecated native biz code leaked: %s", err.Short())
	}
}

func TestSetOptions(t *testing.T) {
	FakeApiKey := "123"
	e := Exchange{
		ExgInfo: &ExgInfo{},
		Options: map[string]interface{}{
			OptMarketType: MarketMargin,
			OptApiKey:     FakeApiKey,
		},
	}
	e.Init()
	_, creds, err := e.GetAccountCreds("default")
	if err != nil {
		panic(err)
	}
	if creds.ApiKey == FakeApiKey {
		t.Logf("Pass ApiKey")
	} else {
		t.Errorf("Fail ApiKey, cur %v, expect: %v", creds.ApiKey, FakeApiKey)
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
		ExgInfo: &ExgInfo{
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
						Price:      8,
						Amount:     8,
						ModePrice:  PrecModeDecimalPlace,
						ModeAmount: PrecModeDecimalPlace,
					},
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
