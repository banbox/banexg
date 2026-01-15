package okx

import (
	"testing"

	"github.com/banbox/banexg"
)

func TestApi_FetchAccountAccess(t *testing.T) {
	exg := getExchange(map[string]interface{}{
		banexg.OptDebugApi: true,
	})

	res, err := exg.FetchAccountAccess(nil)
	if err != nil {
		t.Fatalf("FetchAccountAccess: %v", err)
	}

	t.Logf("TradeKnown: %v, TradeAllowed: %v", res.TradeKnown, res.TradeAllowed)
	t.Logf("WithdrawKnown: %v, WithdrawAllowed: %v", res.WithdrawKnown, res.WithdrawAllowed)
	t.Logf("IPKnown: %v, IPAny: %v", res.IPKnown, res.IPAny)
	t.Logf("PosMode: %s", res.PosMode)
	t.Logf("AcctLv: %s, AcctMode: %s", res.AcctLv, res.AcctMode)
}

func TestApi_FetchAccountAccess_Linear(t *testing.T) {
	exg := getExchange(map[string]interface{}{
		banexg.OptDebugApi:   true,
		banexg.OptMarketType: banexg.MarketLinear,
	})

	res, err := exg.FetchAccountAccess(nil)
	if err != nil {
		t.Fatalf("FetchAccountAccess: %v", err)
	}

	t.Logf("TradeKnown: %v, TradeAllowed: %v", res.TradeKnown, res.TradeAllowed)
	t.Logf("WithdrawKnown: %v, WithdrawAllowed: %v", res.WithdrawKnown, res.WithdrawAllowed)
	t.Logf("IPKnown: %v, IPAny: %v", res.IPKnown, res.IPAny)
	t.Logf("PosMode: %s", res.PosMode)
}

func TestApi_FetchAccountAccess_WithBalance(t *testing.T) {
	exg := getExchange(map[string]interface{}{
		banexg.OptDebugApi:   true,
		banexg.OptMarketType: banexg.MarketLinear,
	})

	balance, err := exg.FetchBalance(nil)
	if err != nil {
		t.Fatalf("FetchBalance: %v", err)
	}

	res, err := exg.FetchAccountAccess(map[string]interface{}{
		banexg.ParamBalance: balance,
	})
	if err != nil {
		t.Fatalf("FetchAccountAccess: %v", err)
	}

	t.Logf("TradeKnown: %v, TradeAllowed: %v", res.TradeKnown, res.TradeAllowed)
	t.Logf("WithdrawKnown: %v, WithdrawAllowed: %v", res.WithdrawKnown, res.WithdrawAllowed)
	t.Logf("IPKnown: %v, IPAny: %v", res.IPKnown, res.IPAny)
	t.Logf("PosMode: %s", res.PosMode)
}
