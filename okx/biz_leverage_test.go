package okx

import (
	"testing"

	"github.com/banbox/banexg"
	"github.com/sasha-s/go-deadlock"
)

func TestBuildLeverageBrackets(t *testing.T) {
	tiers := []PositionTier{
		{InstType: "SWAP", InstFamily: "BTC-USDT", Tier: "1", MinSz: "0", MaxSz: "100", Mmr: "0.01", MaxLever: "10"},
		{InstType: "SWAP", InstFamily: "BTC-USDT", Tier: "2", MinSz: "100", MaxSz: "200", Mmr: "0.02", MaxLever: "5"},
	}
	brackets := buildLeverageBrackets(tiers)
	info, ok := brackets["BTC-USDT"]
	if !ok || info == nil {
		t.Fatalf("missing brackets for instFamily")
	}
	if len(info.Brackets) != 2 {
		t.Fatalf("unexpected bracket count: %d", len(info.Brackets))
	}
	if info.Brackets[0].Cum != 0 {
		t.Fatalf("unexpected cum for first bracket: %v", info.Brackets[0].Cum)
	}
	if info.Brackets[1].Cum != 1 {
		t.Fatalf("unexpected cum for second bracket: %v", info.Brackets[1].Cum)
	}
}

func TestCalcMaintMargin(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	seedMarket(exg, "BTC-USDT-SWAP", "BTC/USDT:USDT", banexg.MarketLinear)
	exg.LeverageBrackets = buildLeverageBrackets([]PositionTier{
		{InstType: "SWAP", InstFamily: "BTC-USDT", Tier: "1", MinSz: "0", MaxSz: "100", Mmr: "0.01", MaxLever: "10"},
		{InstType: "SWAP", InstFamily: "BTC-USDT", Tier: "2", MinSz: "100", MaxSz: "200", Mmr: "0.02", MaxLever: "5"},
	})
	margin, err := exg.CalcMaintMargin("BTC/USDT:USDT", 150)
	if err != nil {
		t.Fatalf("CalcMaintMargin error: %v", err)
	}
	if margin != 2 {
		t.Fatalf("unexpected maint margin: %v", margin)
	}
}

func TestGetLeverage(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	seedMarket(exg, "BTC-USDT-SWAP", "BTC/USDT:USDT", banexg.MarketLinear)
	exg.LeverageBrackets = buildLeverageBrackets([]PositionTier{
		{InstType: "SWAP", InstFamily: "BTC-USDT", Tier: "1", MinSz: "0", MaxSz: "100", Mmr: "0.01", MaxLever: "10"},
		{InstType: "SWAP", InstFamily: "BTC-USDT", Tier: "2", MinSz: "100", MaxSz: "200", Mmr: "0.02", MaxLever: "5"},
	})
	exg.Accounts = map[string]*banexg.Account{
		"default": {
			Name:         "default",
			Leverages:    map[string]int{"BTC/USDT:USDT": 8},
			LockLeverage: &deadlock.Mutex{},
		},
	}
	exg.DefAccName = "default"
	cur, max := exg.GetLeverage("BTC/USDT:USDT", 150, "")
	if cur != 8 {
		t.Fatalf("unexpected current leverage: %v", cur)
	}
	if max != 5 {
		t.Fatalf("unexpected max leverage: %v", max)
	}
}

func TestMarketToInstTypeForLeverage(t *testing.T) {
	tests := []struct {
		name      string
		market    *banexg.Market
		expected  string
		expectErr bool
	}{
		{
			name:     "margin market",
			market:   &banexg.Market{Margin: true, Contract: false},
			expected: InstTypeMargin,
		},
		{
			name:     "option market",
			market:   &banexg.Market{Option: true},
			expected: InstTypeOption,
		},
		{
			name:     "swap market",
			market:   &banexg.Market{Swap: true},
			expected: InstTypeSwap,
		},
		{
			name:     "future market",
			market:   &banexg.Market{Future: true},
			expected: InstTypeFutures,
		},
		{
			name:      "spot market - should fail",
			market:    &banexg.Market{Spot: true},
			expectErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := marketToInstTypeForLeverage(tt.market)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// ============================================================================
// API Integration Tests - require local.json with valid credentials
// Run manually with: go test -run TestAPI_LoadLeverageBrackets -v
// These tests are prefixed with TestAPI_ to distinguish them from unit tests.
// ============================================================================

func TestAPI_SetLeverage(t *testing.T) {
	exg := getExchange(map[string]interface{}{
		banexg.OptMarketType: banexg.MarketLinear,
		//banexg.OptDebugApi:   true,
	})
	symbol := "BTC/USDT:USDT"
	leverage := 5.0
	res, err := exg.SetLeverage(leverage, symbol, nil)
	if err != nil {
		panic(err)
	}
	t.Logf("set leverage result: %+v", res)
}

func TestAPI_GetLeverage(t *testing.T) {
	exg := getExchange(map[string]interface{}{
		banexg.OptMarketType: banexg.MarketLinear,
		//banexg.OptDebugApi:   true,
	})
	symbol := "BTC/USDT:USDT"
	cur, max := exg.GetLeverage(symbol, 1000, "")
	t.Logf("current leverage: %v, max leverage: %v", cur, max)
	if max <= 0 {
		t.Fatalf("expected positive max leverage, got %v", max)
	}
}
