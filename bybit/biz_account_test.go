package bybit

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"go.uber.org/zap"
)

func TestParseBybitBalance(t *testing.T) {
	exg := &Bybit{Exchange: &banexg.Exchange{ExgInfo: &banexg.ExgInfo{}}}
	bal := &WalletBalance{
		Coin: []WalletBalanceCoin{
			{
				Coin:            "USDT",
				Equity:          "120",
				WalletBalance:   "100",
				Locked:          "2",
				BorrowAmount:    "7",
				SpotBorrow:      "5",
				TotalOrderIM:    "3",
				TotalPositionIM: "10",
				UnrealisedPnl:   "1.5",
			},
			{
				Coin: "BTC",
			},
		},
	}
	info := map[string]interface{}{"raw": true}

	parsed := parseBybitBalance(exg, bal, info, false)
	if parsed == nil {
		t.Fatal("expected balance, got nil")
	}
	if _, ok := parsed.Assets["USDT"]; !ok {
		t.Fatal("expected USDT asset")
	}
	if _, ok := parsed.Assets["BTC"]; ok {
		t.Fatal("expected BTC asset to be filtered out when keepZero=false")
	}

	asset := parsed.Assets["USDT"]
	assertFloatNear(t, asset.Free, 80)
	assertFloatNear(t, asset.Used, 15)
	assertFloatNear(t, asset.Total, 120)
	assertFloatNear(t, asset.Debt, 7)
	assertFloatNear(t, asset.UPol, 1.5)
	if got := parsed.Free["USDT"]; got != asset.Free {
		t.Fatalf("expected Free map to match asset.Free, got %v", got)
	}

	parsedKeepZero := parseBybitBalance(exg, bal, info, true)
	if parsedKeepZero == nil {
		t.Fatal("expected balance, got nil")
	}
	if _, ok := parsedKeepZero.Assets["BTC"]; !ok {
		t.Fatal("expected BTC asset when keepZero=true")
	}
}

func TestParseBybitPosition(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	info := map[string]interface{}{"raw": true}

	pos := parseBybitPosition(exg, &PositionInfo{
		PositionIdx:   0,
		Symbol:        "BTCUSDT",
		Side:          "Buy",
		Size:          "2",
		AvgPrice:      "100",
		MarkPrice:     "110",
		PositionValue: "0",
		Leverage:      "5",
		PositionIM:    "20",
		PositionMM:    "5",
		UnrealisedPnl: "10",
		LiqPrice:      "80",
		TradeMode:     1,
		UpdatedTime:   "1700000000000",
	}, info, banexg.MarketLinear)

	if pos == nil {
		t.Fatal("expected position, got nil")
	}
	if pos.Symbol != "BTC/USDT:USDT" {
		t.Fatalf("unexpected symbol: %s", pos.Symbol)
	}
	if pos.Side != banexg.PosSideLong {
		t.Fatalf("unexpected side: %s", pos.Side)
	}
	if !pos.Isolated || pos.MarginMode != banexg.MarginIsolated {
		t.Fatalf("expected isolated margin mode, got isolated=%v mode=%s", pos.Isolated, pos.MarginMode)
	}
	if pos.Hedged {
		t.Fatal("expected hedged=false for PositionIdx=0")
	}
	assertFloatNear(t, pos.Notional, 200)
	assertFloatNear(t, pos.Collateral, 30)
	expMarginRatio, _ := utils.PrecFloat64(5.0/30.0, 4, true, 0)
	assertFloatNear(t, pos.MarginRatio, expMarginRatio)
	expPct, _ := utils.PrecFloat64(10*100/20.0, 2, true, 0)
	assertFloatNear(t, pos.Percentage, expPct)

	pos2 := parseBybitPosition(exg, &PositionInfo{
		PositionIdx: 2,
		Symbol:      "BTCUSDT",
		Side:        "",
		Size:        "1",
		TradeMode:   0,
	}, info, banexg.MarketLinear)
	if pos2 == nil {
		t.Fatal("expected position for positionIdx mapping")
	}
	if pos2.Side != banexg.PosSideShort {
		t.Fatalf("expected short side from positionIdx=2, got %s", pos2.Side)
	}
	if !pos2.Hedged {
		t.Fatal("expected hedged position when positionIdx!=0")
	}
	if pos2.Isolated || pos2.MarginMode != banexg.MarginCross {
		t.Fatalf("expected cross margin mode, got isolated=%v mode=%s", pos2.Isolated, pos2.MarginMode)
	}

	if got := parseBybitPosition(exg, &PositionInfo{Symbol: "BTCUSDT"}, info, banexg.MarketLinear); got != nil {
		t.Fatal("expected nil position when size=0 and side empty")
	}
}

func TestFetchPositionsValidation(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)

	if _, err := exg.FetchPositions(nil, map[string]interface{}{banexg.ParamMarket: banexg.MarketSpot}); err == nil || err.Code != errs.CodeUnsupportMarket {
		t.Fatalf("expected unsupported market error, got %v", err)
	}

	_, err := exg.FetchPositions([]string{"BTC/USDT:USDT", "ETH/USDT:USDT"}, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
	if err == nil || err.Code != errs.CodeParamInvalid {
		t.Fatalf("expected invalid param error for multiple symbols, got %v", err)
	}

	_, err = exg.FetchPositions(nil, map[string]interface{}{banexg.ParamMarket: banexg.MarketLinear})
	if err == nil || err.Code != errs.CodeParamRequired {
		t.Fatalf("expected missing symbol/settleCoin error, got %v", err)
	}
}

func TestFetchBalanceStub(t *testing.T) {
	exg := &Bybit{Exchange: &banexg.Exchange{ExgInfo: &banexg.ExgInfo{}}}
	setBybitTestRequest(t, func(_ context.Context, endpoint string, params map[string]interface{}, _ int, _, _ bool) *banexg.HttpRes {
		if endpoint != MethodPrivateGetV5AccountWalletBalance {
			t.Fatalf("unexpected endpoint: %s", endpoint)
		}
		if params["accountType"] != "UNIFIED" {
			t.Fatalf("expected accountType=UNIFIED, got %v", params["accountType"])
		}
		if params["coin"] != "USDT" {
			t.Fatalf("expected coin=USDT, got %v", params["coin"])
		}
		content := `{"retCode":0,"retMsg":"OK","result":{"list":[{"accountType":"UNIFIED","coin":[{"coin":"USDT","walletBalance":"0","locked":"0","borrowAmount":"0","spotBorrow":"0","totalOrderIM":"0","totalPositionIM":"0","unrealisedPnl":"0","equity":"0"}]}]},"time":1700000000000}`
		return &banexg.HttpRes{Content: content}
	})

	bal, err := exg.FetchBalance(map[string]interface{}{banexg.ParamCurrency: "USDT"})
	if err != nil {
		t.Fatalf("FetchBalance failed: %v", err)
	}
	if bal == nil {
		t.Fatal("expected balance")
	}
	if _, ok := bal.Assets["USDT"]; !ok {
		t.Fatal("expected USDT asset in balance")
	}
}

func TestFetchPositionsStub(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	setBybitTestRequest(t, func(_ context.Context, endpoint string, params map[string]interface{}, _ int, _, _ bool) *banexg.HttpRes {
		if endpoint != MethodPrivateGetV5PositionList {
			t.Fatalf("unexpected endpoint: %s", endpoint)
		}
		if params["category"] != banexg.MarketLinear {
			t.Fatalf("expected category=linear, got %v", params["category"])
		}
		if params["symbol"] != "BTCUSDT" {
			t.Fatalf("expected symbol=BTCUSDT, got %v", params["symbol"])
		}
		content := `{"retCode":0,"retMsg":"OK","result":{"list":[{"symbol":"BTCUSDT","side":"Buy","size":"2","avgPrice":"100","markPrice":"110","positionValue":"0","leverage":"5","positionIM":"20","positionMM":"5","unrealisedPnl":"10","liqPrice":"80","tradeMode":1,"updatedTime":"1700000000000","positionIdx":0}],"nextPageCursor":""},"time":1700000000000}`
		return &banexg.HttpRes{Content: content}
	})

	items, err := exg.FetchPositions([]string{"BTC/USDT:USDT"}, map[string]interface{}{banexg.ParamMarket: banexg.MarketLinear})
	if err != nil {
		t.Fatalf("FetchPositions failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 position, got %d", len(items))
	}
	if items[0].Symbol != "BTC/USDT:USDT" {
		t.Fatalf("unexpected position symbol: %s", items[0].Symbol)
	}
}

func TestBuildBybitLeverageBrackets(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	items := []RiskLimitInfo{
		{ID: 1, Symbol: "BTCUSDT", RiskLimitValue: "100", MaintenanceMargin: "1", MaxLeverage: "10"},
		{ID: 2, Symbol: "BTCUSDT", RiskLimitValue: "200", MaintenanceMargin: "2", MaxLeverage: "5"},
	}
	brackets := buildBybitLeverageBrackets(exg, banexg.MarketLinear, items)
	info := brackets["BTC/USDT:USDT"]
	if info == nil || len(info.Brackets) != 2 {
		t.Fatalf("expected 2 brackets, got %v", info)
	}
	first := info.Brackets[0]
	if first.Floor != 0 || first.Capacity != 100 {
		t.Fatalf("unexpected first bracket floor/capacity: %v/%v", first.Floor, first.Capacity)
	}
	if math.Abs(first.MaintMarginRatio-0.01) > 1e-9 {
		t.Fatalf("unexpected first bracket mmr: %v", first.MaintMarginRatio)
	}
	second := info.Brackets[1]
	if second.Floor != 100 || second.Capacity != 200 {
		t.Fatalf("unexpected second bracket floor/capacity: %v/%v", second.Floor, second.Capacity)
	}
	if math.Abs(second.Cum-1) > 1e-9 {
		t.Fatalf("unexpected second bracket cum: %v", second.Cum)
	}
}

func TestCalcMaintMargin(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	items := []RiskLimitInfo{
		{ID: 1, Symbol: "BTCUSDT", RiskLimitValue: "100", MaintenanceMargin: "1", MaxLeverage: "10"},
		{ID: 2, Symbol: "BTCUSDT", RiskLimitValue: "200", MaintenanceMargin: "2", MaxLeverage: "5"},
	}
	exg.LeverageBrackets = buildBybitLeverageBrackets(exg, banexg.MarketLinear, items)

	mm, err := exg.CalcMaintMargin("BTC/USDT:USDT", 150)
	if err != nil {
		t.Fatalf("CalcMaintMargin failed: %v", err)
	}
	if math.Abs(mm-2) > 1e-9 {
		t.Fatalf("unexpected maint margin: %v", mm)
	}

	if _, err := exg.CalcMaintMargin("UNKNOWN", 100); err == nil {
		t.Fatal("expected error for missing bracket")
	}
}

// Tests migrated from api_account_test.go

func TestApi_FetchBalance(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	bal := fetchBalanceMust(t, exg, map[string]interface{}{
		banexg.ParamCurrency: "USDT",
	})
	log.Info("balance", zap.Any("balance", bal))
	requireBalanceHasAssets(t, bal, "USDT")
}

func TestApi_FetchBalance_DefaultParams(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	bal := fetchBalanceMust(t, exg, nil)
	if bal.Info == nil {
		t.Fatal("expected balance.Info")
	}
	if bal.Assets == nil {
		t.Fatal("expected balance.Assets")
	}
}

func TestApi_FetchBalance_ExplicitAccountType(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	bal := fetchBalanceMust(t, exg, map[string]interface{}{
		"accountType": "UNIFIED",
	})
	if bal.Info == nil {
		t.Fatal("expected balance.Info")
	}
	if bal.Assets == nil {
		t.Fatal("expected balance.Assets")
	}
}

func TestApi_FetchBalance_MultiCoin_DefaultAccountType(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	bal := fetchBalanceMust(t, exg, map[string]interface{}{
		banexg.ParamCurrency: "USDT,BTC",
	})
	requireBalanceHasAssets(t, bal, "USDT", "BTC")
}

func TestApi_FetchBalance_MultiCoin_ExplicitAccountType(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	bal := fetchBalanceMust(t, exg, map[string]interface{}{
		"accountType":        "UNIFIED",
		banexg.ParamCurrency: "USDT,BTC",
	})
	requireBalanceHasAssets(t, bal, "USDT", "BTC")
}

func TestApi_LoadLeverageBrackets_Linear_Default(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	if err := exg.LoadLeverageBrackets(false, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	}); err != nil {
		t.Fatalf("LoadLeverageBrackets failed: %v", err)
	}
	requireLeverageBracketsNonEmpty(t, exg)
}

func TestApi_CalcMaintMargin(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	_, err := exg.LoadMarkets(false, nil)
	if err != nil {
		t.Fatalf("LoadMarkets failed: %v", err)
	}
	if err := exg.LoadLeverageBrackets(false, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	}); err != nil {
		t.Fatalf("LoadLeverageBrackets failed: %v", err)
	}

	mm, err := exg.CalcMaintMargin("BTC/USDT:USDT", 1000)
	if err != nil {
		t.Fatalf("CalcMaintMargin failed: %v", err)
	}
	if mm <= 0 {
		t.Fatalf("expected positive maint margin, got %v", mm)
	}
}

func TestApi_CalcMaintMargin_Inverse(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	symbol := pickBybitInverseSymbolForLeverage(t, exg)

	if err := exg.LoadLeverageBrackets(false, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketInverse,
	}); err != nil {
		t.Fatalf("LoadLeverageBrackets failed: %v", err)
	}

	mm, err := exg.CalcMaintMargin(symbol, 1000)
	if err != nil {
		t.Fatalf("CalcMaintMargin failed: %v", err)
	}
	if mm <= 0 {
		t.Fatalf("expected positive maint margin, got %v", mm)
	}
}

func TestApi_CloseAllPositions(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	positions, err := exg.FetchAccountPositions(nil, map[string]interface{}{
		banexg.ParamMarket:      banexg.MarketLinear,
		banexg.ParamSettleCoins: []string{"USDT"},
	})
	if err != nil {
		t.Fatalf("FetchAccountPositions failed: %v", err)
	}
	if len(positions) == 0 {
		t.Log("no positions to close")
		return
	}

	for _, pos := range positions {
		if pos.Contracts == 0 {
			continue
		}
		closeSide := banexg.OdSideSell
		if pos.Side == banexg.PosSideShort {
			closeSide = banexg.OdSideBuy
		}

		order, err := exg.CreateOrder(pos.Symbol, banexg.OdTypeMarket, closeSide, pos.Contracts, 0, map[string]interface{}{
			banexg.ParamReduceOnly: true,
		})
		if err != nil {
			t.Errorf("close position %s failed: %v", pos.Symbol, err)
			continue
		}
		_ = order
	}
}

func TestApi_CancelAllOpenOrders(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	openOrders, err := exg.FetchOpenOrders("", 0, 100, map[string]interface{}{
		banexg.ParamMarket:      banexg.MarketLinear,
		banexg.ParamSettleCoins: []string{"USDT"},
	})
	if err != nil {
		t.Fatalf("FetchOpenOrders failed: %v", err)
	}
	if len(openOrders) == 0 {
		t.Log("no open orders to cancel")
		return
	}

	for _, order := range openOrders {
		_, err := exg.CancelOrder(order.ID, order.Symbol, nil)
		if err != nil {
			t.Errorf("cancel order %s failed: %v", order.ID, err)
			continue
		}
	}
}

func fetchBalanceMust(t *testing.T, exg *Bybit, params map[string]interface{}) *banexg.Balances {
	t.Helper()
	bal, err := exg.FetchBalance(params)
	if err != nil {
		t.Fatalf("FetchBalance failed: %v", err)
	}
	if bal == nil {
		t.Fatal("expected balance")
	}
	return bal
}

func requireBalanceHasAssets(t *testing.T, bal *banexg.Balances, codes ...string) {
	t.Helper()
	if bal == nil || bal.Assets == nil {
		t.Fatalf("expected balance with assets, got: %#v", bal)
	}
	keys := make([]string, 0, len(bal.Assets))
	for k := range bal.Assets {
		keys = append(keys, k)
	}
	for _, code := range codes {
		if _, ok := bal.Assets[code]; !ok {
			t.Fatalf("expected %s asset in balance, got keys: %v", code, keys)
		}
	}
}

// Tests migrated from api_leverage_test.go, api_setleverage_test.go, api_getleverage_test.go

func requireLeverageBracketsNonEmpty(t *testing.T, exg *Bybit) {
	t.Helper()
	if exg == nil {
		t.Fatal("bybit exchange not initialized")
	}
	exg.LeverageBracketsLock.Lock()
	defer exg.LeverageBracketsLock.Unlock()
	if len(exg.LeverageBrackets) == 0 {
		t.Fatal("expected non-empty leverage brackets")
	}
}

func requireLeverageBracketForSymbol(t *testing.T, exg *Bybit, marketType, rawID string) {
	t.Helper()
	if exg == nil {
		t.Fatal("bybit exchange not initialized")
	}
	if rawID == "" {
		t.Fatal("expected non-empty raw symbol id")
	}

	safe := exg.SafeSymbol(rawID, "", marketType)
	if safe == "" {
		safe = rawID
	}

	exg.LeverageBracketsLock.Lock()
	defer exg.LeverageBracketsLock.Unlock()

	if _, ok := exg.LeverageBrackets[safe]; ok {
		return
	}
	if _, ok := exg.LeverageBrackets[rawID]; ok {
		return
	}
	t.Fatalf("expected leverage bracket for %q (raw %q)", safe, rawID)
}

func TestApi_LoadLeverageBrackets_Linear_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	if err := exg.LoadLeverageBrackets(true, map[string]interface{}{
		banexg.ParamMarket:  banexg.MarketLinear,
		banexg.ParamSymbol:  "BTCUSDT",
		banexg.ParamNoCache: true,
	}); err != nil {
		t.Fatalf("LoadLeverageBrackets failed: %v", err)
	}
	requireLeverageBracketsNonEmpty(t, exg)
	requireLeverageBracketForSymbol(t, exg, banexg.MarketLinear, "BTCUSDT")
}

func TestApi_LoadLeverageBrackets_Linear_Symbol_After(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	first := fetchV5MarketRiskLimit(t, exg, map[string]interface{}{
		"category": banexg.MarketLinear,
	})
	requireRiskLimitListNonEmpty(t, first)
	if first.NextPageCursor == "" {
		t.Skip("risk-limit did not return nextPageCursor; cannot validate ParamAfter mapping in this environment")
	}

	if err := exg.LoadLeverageBrackets(true, map[string]interface{}{
		banexg.ParamMarket:  banexg.MarketLinear,
		banexg.ParamSymbol:  "BTCUSDT",
		banexg.ParamAfter:   first.NextPageCursor,
		banexg.ParamNoCache: true,
	}); err != nil {
		t.Fatalf("LoadLeverageBrackets failed: %v", err)
	}
	requireLeverageBracketsNonEmpty(t, exg)
	requireLeverageBracketForSymbol(t, exg, banexg.MarketLinear, "BTCUSDT")
}

func TestApi_LoadLeverageBrackets_Inverse_Default(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	if err := exg.LoadLeverageBrackets(true, map[string]interface{}{
		banexg.ParamMarket:  banexg.MarketInverse,
		banexg.ParamNoCache: true,
	}); err != nil {
		t.Fatalf("LoadLeverageBrackets failed: %v", err)
	}
	requireLeverageBracketsNonEmpty(t, exg)
}

func TestApi_LoadLeverageBrackets_Inverse_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	if err := exg.LoadLeverageBrackets(true, map[string]interface{}{
		banexg.ParamMarket:  banexg.MarketInverse,
		banexg.ParamSymbol:  "BTCUSD",
		banexg.ParamNoCache: true,
	}); err != nil {
		t.Fatalf("LoadLeverageBrackets failed: %v", err)
	}
	requireLeverageBracketsNonEmpty(t, exg)
	requireLeverageBracketForSymbol(t, exg, banexg.MarketInverse, "BTCUSD")
}

func setBybitLeverageMust(t *testing.T, exg *Bybit, symbol string, leverage float64, params map[string]interface{}) {
	t.Helper()
	if exg == nil {
		t.Fatal("bybit exchange not initialized")
	}
	if symbol == "" {
		t.Fatal("symbol required")
	}
	if _, err := exg.SetLeverage(leverage, symbol, params); err != nil {
		t.Fatalf("SetLeverage failed: symbol=%s leverage=%v params=%v err=%v", symbol, leverage, params, err)
	}
}

func TestApi_SetLeverage_Default_Linear(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	symbol := pickBybitLinearSymbolForLeverage(t, exg)
	setBybitLeverageMust(t, exg, symbol, 2, nil)
}

func TestApi_SetLeverage_Default_Inverse(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	symbol := pickBybitInverseSymbolForLeverage(t, exg)
	setBybitLeverageMust(t, exg, symbol, 2, nil)
}

func TestApi_SetLeverage_OverrideBuyLeverage_Linear(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	symbol := pickBybitLinearSymbolForLeverage(t, exg)
	setBybitLeverageMust(t, exg, symbol, 2, map[string]interface{}{
		"buyLeverage": "2",
	})
}

func TestApi_SetLeverage_OverrideSellLeverage_Linear(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	symbol := pickBybitLinearSymbolForLeverage(t, exg)
	setBybitLeverageMust(t, exg, symbol, 2, map[string]interface{}{
		"sellLeverage": "2",
	})
}

func TestApi_SetLeverage_OverrideBuySellLeverage_Linear(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	symbol := pickBybitLinearSymbolForLeverage(t, exg)
	setBybitLeverageMust(t, exg, symbol, 2, map[string]interface{}{
		"buyLeverage":  "2",
		"sellLeverage": "2",
	})
}

func TestApi_SetLeverage_OverrideBuyLeverage_Inverse(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	symbol := pickBybitInverseSymbolForLeverage(t, exg)
	setBybitLeverageMust(t, exg, symbol, 2, map[string]interface{}{
		"buyLeverage": "2",
	})
}

func TestApi_SetLeverage_OverrideSellLeverage_Inverse(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	symbol := pickBybitInverseSymbolForLeverage(t, exg)
	setBybitLeverageMust(t, exg, symbol, 2, map[string]interface{}{
		"sellLeverage": "2",
	})
}

func TestApi_SetLeverage_OverrideBuySellLeverage_Inverse(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	symbol := pickBybitInverseSymbolForLeverage(t, exg)
	setBybitLeverageMust(t, exg, symbol, 2, map[string]interface{}{
		"buyLeverage":  "2",
		"sellLeverage": "2",
	})
}

func isBybitPortfolioMargin(exg *Bybit) bool {
	if exg == nil {
		return false
	}
	access, err := exg.FetchAccountAccess(nil)
	if err != nil || access == nil {
		return false
	}
	return access.AcctMode == banexg.AcctModePortfolioMargin
}

func pickBybitLinearSymbolForLeverage(t *testing.T, exg *Bybit) string {
	t.Helper()
	markets := loadBybitMarketsForType(t, exg, banexg.MarketLinear)
	for _, m := range markets {
		if m != nil && m.Symbol == "BTC/USDT:USDT" {
			return m.Symbol
		}
	}
	if m := pickBybitMarketByType(markets, banexg.MarketLinear); m != nil && m.Symbol != "" {
		return m.Symbol
	}
	t.Skip("no linear markets available")
	return ""
}

func pickBybitInverseSymbolForLeverage(t *testing.T, exg *Bybit) string {
	t.Helper()
	markets := loadBybitMarketsForType(t, exg, banexg.MarketInverse)
	if m := pickBybitMarketByType(markets, banexg.MarketInverse); m != nil && m.Symbol != "" {
		return m.Symbol
	}
	t.Skip("no inverse markets available")
	return ""
}

func clearBybitLeverageCache(t *testing.T, exg *Bybit, account string) {
	t.Helper()
	if exg == nil {
		t.Fatal("bybit exchange not initialized")
	}
	acc, err := exg.GetAccount(account)
	if err != nil || acc == nil || acc.LockLeverage == nil {
		t.Skipf("cannot resolve account for leverage cache reset: %v", err)
		return
	}
	acc.LockLeverage.Lock()
	acc.Leverages = nil
	acc.LockLeverage.Unlock()
}

func clearBybitLeverageBracketsCache(t *testing.T, exg *Bybit) {
	t.Helper()
	if exg == nil {
		t.Fatal("bybit exchange not initialized")
	}
	exg.LeverageBracketsLock.Lock()
	exg.LeverageBrackets = nil
	exg.LeverageBracketsLock.Unlock()
}

func requireNear(t *testing.T, got, want, tol float64, name string) {
	t.Helper()
	if tol < 0 {
		t.Fatalf("invalid tol: %v", tol)
	}
	if math.Abs(got-want) > tol {
		t.Fatalf("%s mismatch: got=%v want=%v tol=%v", name, got, want, tol)
	}
}

func TestApi_GetLeverage_Linear(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	symbol := pickBybitLinearSymbolForLeverage(t, exg)
	isPM := isBybitPortfolioMargin(exg)
	if !isPM {
		if _, err := exg.SetLeverage(2, symbol, nil); err != nil {
			t.Fatalf("SetLeverage failed: %v", err)
		}
	}

	cur, max := exg.GetLeverage(symbol, 1000, "")
	if max <= 0 {
		t.Fatalf("expected positive max leverage, got %v", max)
	}
	if !isPM {
		if cur <= 0 {
			t.Fatalf("expected positive current leverage, got %v", cur)
		}
		requireNear(t, cur, 2, 0.1, "current leverage")
	}
}

func TestApi_GetLeverage_Linear_LoadsMaxLeverage_OnDemand(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	symbol := pickBybitLinearSymbolForLeverage(t, exg)

	clearBybitLeverageBracketsCache(t, exg)

	_, max := exg.GetLeverage(symbol, 1000, "")
	if max <= 0 {
		t.Fatalf("expected positive max leverage, got %v", max)
	}
}

func TestApi_GetLeverage_Linear_FetchCurrentLeverage_OnCacheMiss(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	symbol := pickBybitLinearSymbolForLeverage(t, exg)

	isPM := isBybitPortfolioMargin(exg)
	if !isPM {
		if _, err := exg.SetLeverage(3, symbol, nil); err != nil {
			t.Fatalf("SetLeverage failed: %v", err)
		}
	}
	clearBybitLeverageCache(t, exg, "")

	cur, max := exg.GetLeverage(symbol, 1000, "")
	if max <= 0 {
		t.Fatalf("expected positive max leverage, got %v", max)
	}
	if !isPM {
		if cur <= 0 {
			t.Fatalf("expected positive current leverage, got %v", cur)
		}
		requireNear(t, cur, 3, 0.1, "current leverage")
	}
}

func TestApi_GetLeverage_Inverse(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	symbol := pickBybitInverseSymbolForLeverage(t, exg)
	isPM := isBybitPortfolioMargin(exg)
	if !isPM {
		if _, err := exg.SetLeverage(2, symbol, nil); err != nil {
			t.Fatalf("SetLeverage failed: %v", err)
		}
	}

	cur, max := exg.GetLeverage(symbol, 1000, "")
	if max <= 0 {
		t.Fatalf("expected positive max leverage, got %v", max)
	}
	if !isPM {
		if cur <= 0 {
			t.Fatalf("expected positive current leverage, got %v", cur)
		}
		requireNear(t, cur, 2, 0.1, "current leverage")
	}
}

func TestApi_GetLeverage_Inverse_LoadsMaxLeverage_OnDemand(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	symbol := pickBybitInverseSymbolForLeverage(t, exg)

	clearBybitLeverageBracketsCache(t, exg)

	_, max := exg.GetLeverage(symbol, 1000, "")
	if max <= 0 {
		t.Fatalf("expected positive max leverage, got %v", max)
	}
}

func TestApi_GetLeverage_Inverse_FetchCurrentLeverage_OnCacheMiss(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	symbol := pickBybitInverseSymbolForLeverage(t, exg)

	isPM := isBybitPortfolioMargin(exg)
	if !isPM {
		if _, err := exg.SetLeverage(3, symbol, nil); err != nil {
			t.Fatalf("SetLeverage failed: %v", err)
		}
	}
	clearBybitLeverageCache(t, exg, "")

	cur, max := exg.GetLeverage(symbol, 1000, "")
	if max <= 0 {
		t.Fatalf("expected positive max leverage, got %v", max)
	}
	if !isPM {
		if cur <= 0 {
			t.Fatalf("expected positive current leverage, got %v", cur)
		}
		requireNear(t, cur, 3, 0.1, "current leverage")
	}
}

// Tests migrated from api_account_positions_test.go

func fetchAccountPositionsMust(t *testing.T, exg *Bybit, symbols []string, params map[string]interface{}) []*banexg.Position {
	t.Helper()
	positions, err := exg.FetchAccountPositions(symbols, params)
	if err != nil {
		t.Fatalf("FetchAccountPositions failed: %v", err)
	}
	if positions == nil {
		t.Fatal("expected positions slice")
	}
	return positions
}

func TestApi_FetchAccountPositions_Linear_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fetchAccountPositionsMust(t, exg, []string{"BTC/USDT:USDT"}, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
}

func TestApi_FetchAccountPositions_Linear_SymbolAndSettleCoin(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fetchAccountPositionsMust(t, exg, []string{"BTC/USDT:USDT"}, map[string]interface{}{
		banexg.ParamMarket:      banexg.MarketLinear,
		banexg.ParamSettleCoins: []string{"USDT"},
	})
}

func TestApi_FetchAccountPositions_Linear_SettleCoin(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fetchAccountPositionsMust(t, exg, nil, map[string]interface{}{
		banexg.ParamMarket:      banexg.MarketLinear,
		banexg.ParamSettleCoins: []string{"USDT"},
	})
}

func TestApi_FetchAccountPositions_Linear_SettleCoin_RawKey(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fetchAccountPositionsMust(t, exg, nil, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
		"settleCoin":       "USDT",
	})
}

func TestApi_FetchAccountPositions_Linear_MultiSettleCoins(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	fetchAccountPositionsMust(t, exg, nil, map[string]interface{}{
		banexg.ParamMarket:      banexg.MarketLinear,
		banexg.ParamSettleCoins: []string{"USDT", "USDT"},
	})
}

func TestApi_FetchAccountPositions_Linear_Limit(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fetchAccountPositionsMust(t, exg, nil, map[string]interface{}{
		banexg.ParamMarket:      banexg.MarketLinear,
		banexg.ParamSettleCoins: []string{"USDT"},
		banexg.ParamLimit:       1,
	})
}

func TestApi_FetchAccountPositions_Linear_After(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	first := fetchV5PositionList(t, exg, map[string]interface{}{
		"category":   banexg.MarketLinear,
		"settleCoin": "USDT",
		"limit":      1,
	})
	if first.NextPageCursor == "" {
		t.Skip("position/list did not return nextPageCursor; cannot validate ParamAfter mapping in this environment")
	}

	fetchAccountPositionsMust(t, exg, nil, map[string]interface{}{
		banexg.ParamMarket:      banexg.MarketLinear,
		banexg.ParamSettleCoins: []string{"USDT"},
		banexg.ParamLimit:       1,
		banexg.ParamAfter:       first.NextPageCursor,
	})
}

func TestApi_FetchAccountPositions_Inverse_Default(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fetchAccountPositionsMust(t, exg, nil, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketInverse,
	})
}

func TestApi_FetchAccountPositions_Inverse_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	markets := loadBybitMarketsForType(t, exg, banexg.MarketInverse)
	market := pickBybitMarketByType(markets, banexg.MarketInverse)
	if market == nil || market.Symbol == "" {
		t.Skip("no inverse markets available")
	}

	fetchAccountPositionsMust(t, exg, []string{market.Symbol}, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketInverse,
	})
}

func TestApi_FetchAccountPositions_Option_Default(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fetchAccountPositionsMust(t, exg, nil, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketOption,
		banexg.ParamLimit:  1,
	})
}

func TestApi_FetchAccountPositions_Option_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	markets := loadBybitMarketsForType(t, exg, banexg.MarketOption)
	market := pickBybitMarketByType(markets, banexg.MarketOption)
	if market == nil || market.Symbol == "" {
		t.Skip("no option markets available")
	}

	fetchAccountPositionsMust(t, exg, []string{market.Symbol}, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketOption,
	})
}

func TestApi_FetchAccountPositions_Option_BaseCoin(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	markets := loadBybitMarketsForType(t, exg, banexg.MarketOption)
	market := pickBybitMarketByType(markets, banexg.MarketOption)
	if market == nil || market.Base == "" {
		t.Skip("no option markets available")
	}

	fetchAccountPositionsMust(t, exg, nil, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketOption,
		"baseCoin":         market.Base,
		banexg.ParamLimit:  1,
	})
}

// Tests migrated from api_positionlist_test.go, api_risklimit_test.go, api_fee_rate_test.go, api_calculate_fee_test.go, api_incomehistory_test.go

func fetchPositionsMust(t *testing.T, exg *Bybit, symbols []string, params map[string]interface{}) []*banexg.Position {
	t.Helper()
	positions, err := exg.FetchPositions(symbols, params)
	if err != nil {
		t.Fatalf("FetchPositions failed: %v", err)
	}
	if positions == nil {
		t.Fatal("expected positions slice")
	}
	return positions
}

func fetchV5PositionList(t *testing.T, exg *Bybit, args map[string]interface{}) V5ListResult {
	t.Helper()
	if exg == nil {
		t.Fatal("bybit exchange not initialized")
	}
	tryNum := exg.GetRetryNum("V5PositionList", 1)
	res := requestRetry[V5ListResult](exg, MethodPrivateGetV5PositionList, args, tryNum)
	if res.Error != nil {
		t.Fatalf("v5 position/list failed: %v", res.Error)
	}
	return res.Result
}

func TestApi_FetchPositions_Linear_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	fetchPositionsMust(t, exg, []string{"BTC/USDT:USDT"}, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
}

func TestApi_FetchPositions_Linear_SymbolAndSettleCoin(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	fetchPositionsMust(t, exg, []string{"BTC/USDT:USDT"}, map[string]interface{}{
		banexg.ParamMarket:      banexg.MarketLinear,
		banexg.ParamSettleCoins: []string{"USDT"},
	})
}

func TestApi_FetchPositions_Linear_SettleCoin(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fetchPositionsMust(t, exg, nil, map[string]interface{}{
		banexg.ParamMarket:      banexg.MarketLinear,
		banexg.ParamSettleCoins: []string{"USDT"},
	})
}

func TestApi_FetchPositions_Linear_SettleCoin_RawKey(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fetchPositionsMust(t, exg, nil, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
		"settleCoin":       "USDT",
	})
}

func TestApi_FetchPositions_Linear_MultiSettleCoins(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	// Trigger the multi-settleCoin request loop; use duplicates to avoid relying on extra settle coins.
	fetchPositionsMust(t, exg, nil, map[string]interface{}{
		banexg.ParamMarket:      banexg.MarketLinear,
		banexg.ParamSettleCoins: []string{"USDT", "USDT"},
	})
}

func TestApi_FetchPositions_Linear_Limit(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	fetchPositionsMust(t, exg, nil, map[string]interface{}{
		banexg.ParamMarket:      banexg.MarketLinear,
		banexg.ParamSettleCoins: []string{"USDT"},
		banexg.ParamLimit:       1,
	})
}

func TestApi_FetchPositions_Linear_After(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	first := fetchV5PositionList(t, exg, map[string]interface{}{
		"category":   banexg.MarketLinear,
		"settleCoin": "USDT",
		"limit":      1,
	})
	if first.NextPageCursor == "" {
		t.Skip("position/list did not return nextPageCursor; cannot validate ParamAfter mapping in this environment")
	}

	fetchPositionsMust(t, exg, nil, map[string]interface{}{
		banexg.ParamMarket:      banexg.MarketLinear,
		banexg.ParamSettleCoins: []string{"USDT"},
		banexg.ParamLimit:       1,
		banexg.ParamAfter:       first.NextPageCursor,
	})
}

func TestApi_FetchPositions_Inverse_Default(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	fetchPositionsMust(t, exg, nil, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketInverse,
	})
}

func TestApi_FetchPositions_Inverse_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	markets := loadBybitMarketsForType(t, exg, banexg.MarketInverse)
	market := pickBybitMarketByType(markets, banexg.MarketInverse)
	if market == nil || market.Symbol == "" {
		t.Skip("no inverse markets available")
	}

	fetchPositionsMust(t, exg, []string{market.Symbol}, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketInverse,
	})
}

func TestApi_FetchPositions_Option_Default(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fetchPositionsMust(t, exg, nil, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketOption,
		banexg.ParamLimit:  1,
	})
}

func TestApi_FetchPositions_Option_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	markets := loadBybitMarketsForType(t, exg, banexg.MarketOption)
	market := pickBybitMarketByType(markets, banexg.MarketOption)
	if market == nil || market.Symbol == "" {
		t.Skip("no option markets available")
	}

	fetchPositionsMust(t, exg, []string{market.Symbol}, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketOption,
	})
}

func TestApi_FetchPositions_Option_BaseCoin(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	markets := loadBybitMarketsForType(t, exg, banexg.MarketOption)
	market := pickBybitMarketByType(markets, banexg.MarketOption)
	if market == nil || market.Base == "" {
		t.Skip("no option markets available")
	}

	fetchPositionsMust(t, exg, nil, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketOption,
		"baseCoin":         market.Base,
		banexg.ParamLimit:  1,
	})
}

func fetchV5MarketRiskLimit(t *testing.T, exg *Bybit, args map[string]interface{}) V5ListResult {
	t.Helper()
	if exg == nil {
		t.Fatal("bybit exchange not initialized")
	}
	tryNum := exg.GetRetryNum("V5MarketRiskLimit", 1)
	res := requestRetry[V5ListResult](exg, MethodPublicGetV5MarketRiskLimit, args, tryNum)
	if res.Error != nil {
		t.Fatalf("v5 market risk-limit failed: %v", res.Error)
	}
	return res.Result
}

func requireRiskLimitListNonEmpty(t *testing.T, rr V5ListResult) {
	t.Helper()
	if len(rr.List) == 0 {
		t.Fatal("expected non-empty risk-limit list")
	}
}

func requireRiskLimitAllSymbols(t *testing.T, rr V5ListResult, symbol string) {
	t.Helper()
	if symbol == "" {
		t.Fatal("expected non-empty symbol")
	}
	for i, row := range rr.List {
		got, _ := row["symbol"].(string)
		if got == "" {
			t.Fatalf("risk-limit row[%d] missing symbol", i)
		}
		if got != symbol {
			t.Fatalf("risk-limit row[%d] symbol mismatch: got=%q want=%q", i, got, symbol)
		}
	}
}

func TestApi_V5MarketRiskLimit_Linear_Default(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	rr := fetchV5MarketRiskLimit(t, exg, map[string]interface{}{
		"category": banexg.MarketLinear,
	})
	requireRiskLimitListNonEmpty(t, rr)
}

func TestApi_V5MarketRiskLimit_Linear_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	rr := fetchV5MarketRiskLimit(t, exg, map[string]interface{}{
		"category": banexg.MarketLinear,
		"symbol":   "BTCUSDT",
	})
	requireRiskLimitListNonEmpty(t, rr)
	requireRiskLimitAllSymbols(t, rr, "BTCUSDT")
}

func TestApi_V5MarketRiskLimit_Linear_Cursor(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	first := fetchV5MarketRiskLimit(t, exg, map[string]interface{}{
		"category": banexg.MarketLinear,
	})
	requireRiskLimitListNonEmpty(t, first)
	if first.NextPageCursor == "" {
		t.Skip("risk-limit did not return nextPageCursor; cannot validate cursor pagination in this environment")
	}

	second := fetchV5MarketRiskLimit(t, exg, map[string]interface{}{
		"category": banexg.MarketLinear,
		"cursor":   first.NextPageCursor,
	})
	requireRiskLimitListNonEmpty(t, second)
}

func TestApi_V5MarketRiskLimit_Inverse_Default(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	rr := fetchV5MarketRiskLimit(t, exg, map[string]interface{}{
		"category": banexg.MarketInverse,
	})
	requireRiskLimitListNonEmpty(t, rr)
}

func TestApi_V5MarketRiskLimit_Inverse_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	rr := fetchV5MarketRiskLimit(t, exg, map[string]interface{}{
		"category": banexg.MarketInverse,
		"symbol":   "BTCUSD",
	})
	requireRiskLimitListNonEmpty(t, rr)
	requireRiskLimitAllSymbols(t, rr, "BTCUSD")
}

func TestApi_V5MarketRiskLimit_Inverse_Cursor(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	first := fetchV5MarketRiskLimit(t, exg, map[string]interface{}{
		"category": banexg.MarketInverse,
	})
	requireRiskLimitListNonEmpty(t, first)
	if first.NextPageCursor == "" {
		t.Skip("risk-limit did not return nextPageCursor; cannot validate cursor pagination in this environment")
	}

	second := fetchV5MarketRiskLimit(t, exg, map[string]interface{}{
		"category": banexg.MarketInverse,
		"cursor":   first.NextPageCursor,
	})
	requireRiskLimitListNonEmpty(t, second)
}

func fetchFeeRateV5(t *testing.T, exg *Bybit, params map[string]interface{}) V5ListResult {
	t.Helper()
	params = ensureNoCache(params)
	tryNum := exg.GetRetryNum("FetchFeeRate", 1)
	res := requestRetry[V5ListResult](exg, MethodPrivateGetV5AccountFeeRate, params, tryNum)
	if res.Error != nil {
		t.Fatalf("fetch fee rate failed: %v", res.Error)
	}
	if len(res.Result.List) == 0 {
		t.Fatalf("expected non-empty fee rate list, got: %#v", res.Result)
	}
	return res.Result
}

func requireFeeRateHasMakerTaker(t *testing.T, items []map[string]interface{}) {
	t.Helper()
	for i, it := range items {
		_, okMaker := it["makerFeeRate"]
		_, okTaker := it["takerFeeRate"]
		if !okMaker || !okTaker {
			t.Fatalf("fee rate item[%d] missing maker/taker fields: %#v", i, it)
		}
		// Bybit returns numbers as strings; allow 0 for VIPs.
		_ = parseBybitNum(it["makerFeeRate"])
		_ = parseBybitNum(it["takerFeeRate"])
	}
}

func TestApi_GetFeeRate_Spot_Default(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	res := fetchFeeRateV5(t, exg, map[string]interface{}{
		"category":          "spot",
		banexg.ParamNoCache: true,
	})
	requireFeeRateHasMakerTaker(t, res.List)
}

func TestApi_GetFeeRate_Spot_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	res := fetchFeeRateV5(t, exg, map[string]interface{}{
		"category":          "spot",
		"symbol":            "BTCUSDT",
		banexg.ParamNoCache: true,
	})
	requireFeeRateHasMakerTaker(t, res.List)
	if got := utils.GetMapVal(res.List[0], "symbol", ""); got != "BTCUSDT" {
		t.Fatalf("unexpected symbol in fee rate response: %s", got)
	}
}

func TestApi_GetFeeRate_Linear_Default(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	res := fetchFeeRateV5(t, exg, map[string]interface{}{
		"category":          "linear",
		banexg.ParamNoCache: true,
	})
	requireFeeRateHasMakerTaker(t, res.List)
}

func TestApi_GetFeeRate_Linear_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	res := fetchFeeRateV5(t, exg, map[string]interface{}{
		"category":          "linear",
		"symbol":            "BTCUSDT",
		banexg.ParamNoCache: true,
	})
	requireFeeRateHasMakerTaker(t, res.List)
	if got := utils.GetMapVal(res.List[0], "symbol", ""); got != "BTCUSDT" {
		t.Fatalf("unexpected symbol in fee rate response: %s", got)
	}
}

func TestApi_GetFeeRate_Inverse_Default(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	res := fetchFeeRateV5(t, exg, map[string]interface{}{
		"category":          "inverse",
		banexg.ParamNoCache: true,
	})
	requireFeeRateHasMakerTaker(t, res.List)
}

func TestApi_GetFeeRate_Inverse_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	res := fetchFeeRateV5(t, exg, map[string]interface{}{
		"category":          "inverse",
		"symbol":            "BTCUSD",
		banexg.ParamNoCache: true,
	})
	requireFeeRateHasMakerTaker(t, res.List)
	if got := utils.GetMapVal(res.List[0], "symbol", ""); got != "BTCUSD" {
		t.Fatalf("unexpected symbol in fee rate response: %s", got)
	}
}

func TestApi_GetFeeRate_Option_Default(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	res := fetchFeeRateV5(t, exg, map[string]interface{}{
		"category":          "option",
		banexg.ParamNoCache: true,
	})
	requireFeeRateHasMakerTaker(t, res.List)
}

func TestApi_GetFeeRate_Option_BaseCoin(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	res := fetchFeeRateV5(t, exg, map[string]interface{}{
		"category": "option",
		"baseCoin": "BTC",
		// Keep symbol empty; V5 docs say options use baseCoin instead of symbol.
		banexg.ParamNoCache: true,
	})
	requireFeeRateHasMakerTaker(t, res.List)
}

func newBybitPublicNoCurr(t *testing.T, name string, careMarkets []string) *Bybit {
	t.Helper()
	exg := getBybitOrSkipNoCurr(t, nil)
	if exg == nil {
		t.Skip("bybit exchange not initialized")
		return nil
	}
	exg.Name = name
	exg.CareMarkets = careMarkets
	return exg
}

func pickMarketSymbolByID(markets banexg.MarketMap, wantID string) string {
	for sym, m := range markets {
		if m != nil && m.ID == wantID {
			return sym
		}
	}
	return ""
}

func pickFirstMarketSymbol(markets banexg.MarketMap, pred func(m *banexg.Market) bool) string {
	for sym, m := range markets {
		if m != nil && pred(m) {
			return sym
		}
	}
	return ""
}

func tolFor(want float64) float64 {
	return math.Max(1e-12, math.Abs(want)*1e-9)
}

func expectedFeeForMarket(exg *Bybit, market *banexg.Market, odType, side string, amount, price float64, isMaker bool) (ccy string, cost float64, quoteCost float64, rate float64) {
	feeSide := market.FeeSide
	if feeSide == "" {
		if exg.Fees != nil {
			if market.Spot || market.Margin {
				feeSide = exg.Fees.Main.FeeSide
			} else if market.Linear {
				feeSide = exg.Fees.Linear.FeeSide
			} else if market.Inverse {
				feeSide = exg.Fees.Inverse.FeeSide
			}
		}
		if feeSide == "" {
			feeSide = "quote"
		}
	}

	useQuote := false
	if feeSide == "get" {
		useQuote = side == banexg.OdSideSell
	} else if feeSide == "give" {
		useQuote = side == banexg.OdSideBuy
	} else {
		useQuote = feeSide == "quote"
	}

	quoteCost = amount * price
	if useQuote {
		ccy = market.Quote
		cost = quoteCost
	} else {
		ccy = market.Base
		cost = amount
	}
	if !market.Spot {
		ccy = market.Settle
	}

	if isMaker {
		rate = market.Maker
	} else {
		rate = market.Taker
	}
	cost *= rate
	quoteCost *= rate
	return ccy, cost, quoteCost, rate
}

func fetchTickerLastPriceMust(t *testing.T, exg *Bybit, symbol string) float64 {
	t.Helper()
	ticker, err := exg.FetchTicker(symbol, map[string]interface{}{banexg.ParamNoCache: true})
	if err != nil {
		t.Fatalf("FetchTicker failed: %v", err)
	}
	if ticker == nil || ticker.Last <= 0 {
		t.Fatalf("unexpected ticker: %#v", ticker)
	}
	return ticker.Last
}

func requireCalculateFeeMatch(t *testing.T, exg *Bybit, symbol, odType, side string, amount, price float64, isMaker bool) {
	t.Helper()

	market, err := exg.GetMarket(symbol)
	if err != nil {
		t.Fatalf("GetMarket failed: %v", err)
	}

	got, err2 := exg.CalculateFee(symbol, odType, side, amount, price, isMaker, nil)
	if err2 != nil {
		t.Fatalf("CalculateFee failed: %v", err2)
	}
	if got == nil {
		t.Fatal("expected fee")
	}

	wantCcy, wantCost, wantQuoteCost, wantRate := expectedFeeForMarket(exg, market, odType, side, amount, price, isMaker)
	if got.Currency != wantCcy {
		t.Fatalf("fee currency mismatch: got=%s want=%s", got.Currency, wantCcy)
	}
	requireNear(t, got.Cost, wantCost, tolFor(wantCost), "fee cost")
	requireNear(t, got.QuoteCost, wantQuoteCost, tolFor(wantQuoteCost), "fee quote cost")
	requireNear(t, got.Rate, wantRate, tolFor(wantRate), "fee rate")
	if got.IsMaker != isMaker {
		t.Fatalf("fee IsMaker mismatch: got=%v want=%v", got.IsMaker, isMaker)
	}
}

func TestApi_CalculateFee_Spot(t *testing.T) {
	exg := newBybitPublicNoCurr(t, "BybitTestApiCalculateFeeSpot", []string{banexg.MarketSpot})
	markets := loadMarkets(t, exg, nil, false)

	symbol := pickMarketSymbolByID(markets, "BTCUSDT")
	if symbol == "" {
		symbol = "BTC/USDT"
	}
	if _, ok := markets[symbol]; !ok {
		t.Skipf("spot symbol not found in markets: %s", symbol)
	}

	price := fetchTickerLastPriceMust(t, exg, symbol)
	amount := 0.001

	// Same parameter-shape, different values; keep coverage here rather than duplicating test functions.
	requireCalculateFeeMatch(t, exg, symbol, banexg.OdTypeLimit, banexg.OdSideBuy, amount, price, false)
	requireCalculateFeeMatch(t, exg, symbol, banexg.OdTypeLimit, banexg.OdSideSell, amount, price, false)
	requireCalculateFeeMatch(t, exg, symbol, banexg.OdTypeLimit, banexg.OdSideBuy, amount, price, true)
	requireCalculateFeeMatch(t, exg, symbol, banexg.OdTypeLimit, banexg.OdSideSell, amount, price, true)
	requireCalculateFeeMatch(t, exg, symbol, banexg.OdTypeMarket, banexg.OdSideBuy, amount, price, false)
	requireCalculateFeeMatch(t, exg, symbol, banexg.OdTypeMarket, banexg.OdSideSell, amount, price, false)

	if _, err := exg.CalculateFee(symbol, banexg.OdTypeMarket, banexg.OdSideBuy, amount, price, true, nil); err == nil {
		t.Fatal("expected error for market order with isMaker=true")
	}
}

func TestApi_CalculateFee_Linear(t *testing.T) {
	exg := newBybitPublicNoCurr(t, "BybitTestApiCalculateFeeLinear", []string{banexg.MarketLinear})
	markets := loadMarkets(t, exg, nil, false)

	symbol := pickMarketSymbolByID(markets, "BTCUSDT")
	if symbol == "" {
		symbol = pickFirstMarketSymbol(markets, func(m *banexg.Market) bool { return m != nil && m.Linear })
	}
	if symbol == "" {
		t.Skip("no linear market found")
	}

	price := fetchTickerLastPriceMust(t, exg, symbol)
	amount := 0.001

	requireCalculateFeeMatch(t, exg, symbol, banexg.OdTypeLimit, banexg.OdSideBuy, amount, price, false)
	requireCalculateFeeMatch(t, exg, symbol, banexg.OdTypeLimit, banexg.OdSideBuy, amount, price, true)
	requireCalculateFeeMatch(t, exg, symbol, banexg.OdTypeMarket, banexg.OdSideBuy, amount, price, false)
}

func TestApi_CalculateFee_Inverse(t *testing.T) {
	exg := newBybitPublicNoCurr(t, "BybitTestApiCalculateFeeInverse", []string{banexg.MarketInverse})
	markets := loadMarkets(t, exg, nil, false)

	symbol := pickMarketSymbolByID(markets, "BTCUSD")
	if symbol == "" {
		symbol = pickFirstMarketSymbol(markets, func(m *banexg.Market) bool { return m != nil && m.Inverse })
	}
	if symbol == "" {
		t.Skip("no inverse market found")
	}

	price := fetchTickerLastPriceMust(t, exg, symbol)
	amount := 0.001

	requireCalculateFeeMatch(t, exg, symbol, banexg.OdTypeLimit, banexg.OdSideBuy, amount, price, false)
	requireCalculateFeeMatch(t, exg, symbol, banexg.OdTypeLimit, banexg.OdSideBuy, amount, price, true)
	requireCalculateFeeMatch(t, exg, symbol, banexg.OdTypeMarket, banexg.OdSideBuy, amount, price, false)
}

func TestApi_CalculateFee_Option(t *testing.T) {
	exg := newBybitPublicNoCurr(t, "BybitTestApiCalculateFeeOption", []string{banexg.MarketOption})
	markets := loadMarkets(t, exg, nil, false)

	symbol := pickFirstMarketSymbol(markets, func(m *banexg.Market) bool {
		return m != nil && m.Option && m.Base == "BTC"
	})
	if symbol == "" {
		symbol = pickFirstMarketSymbol(markets, func(m *banexg.Market) bool { return m != nil && m.Option })
	}
	if symbol == "" {
		t.Skip("no option market found")
	}

	price := fetchTickerLastPriceMust(t, exg, symbol)
	amount := 0.001

	requireCalculateFeeMatch(t, exg, symbol, banexg.OdTypeLimit, banexg.OdSideBuy, amount, price, false)
}

func fetchIncomeHistoryOrFail(t *testing.T, exg *Bybit, inType, symbol string, since int64, limit int, params map[string]interface{}) []*banexg.Income {
	t.Helper()
	items, err := exg.FetchIncomeHistory(inType, symbol, since, limit, params)
	if err != nil {
		t.Fatalf("FetchIncomeHistory failed: %v", err)
	}
	if items == nil {
		t.Fatal("expected income items slice")
	}
	return items
}

func TestApi_FetchIncomeHistory_SinceOnly(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	since := time.Now().UTC().Add(-2 * time.Hour).UnixMilli()
	fetchIncomeHistoryOrFail(t, exg, "", "", since, 10, nil)
}

func TestApi_FetchIncomeHistory_UntilOnly(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	until := time.Now().UTC().UnixMilli()
	fetchIncomeHistoryOrFail(t, exg, "", "", 0, 10, map[string]interface{}{
		banexg.ParamUntil: until,
	})
}

func TestApi_FetchIncomeHistory_SinceAndUntil(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	now := time.Now().UTC()
	since := now.Add(-2 * time.Hour).UnixMilli()
	until := now.UnixMilli()
	fetchIncomeHistoryOrFail(t, exg, "", "", since, 10, map[string]interface{}{
		banexg.ParamUntil: until,
	})
}

func TestApi_FetchIncomeHistory_WithSymbolAndCurrency(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	if _, err := exg.LoadMarkets(false, nil); err != nil {
		t.Fatalf("LoadMarkets failed: %v", err)
	}
	// transaction-log does not accept symbol, but banexg uses symbol to derive category/baseCoin.
	fetchIncomeHistoryOrFail(t, exg, "", "BTC/USDT:USDT", 0, 10, map[string]interface{}{
		banexg.ParamCurrency: "USDT",
	})
}

func TestApi_FetchIncomeHistory_WithTypeTrade(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	// See docs/bybit_v5/enum.md "type(uta-translog)" for valid values.
	fetchIncomeHistoryOrFail(t, exg, "TRADE", "", 0, 10, nil)
}

func TestApi_FetchIncomeHistory_WithMarketTypeSpot(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	fetchIncomeHistoryOrFail(t, exg, "", "", 0, 10, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketSpot,
	})
}

func TestApi_FetchIncomeHistory_WithBaseCoin(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	fetchIncomeHistoryOrFail(t, exg, "", "", 0, 10, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
		"baseCoin":         "BTC",
	})
}

func TestApi_FetchIncomeHistory_WithAccountTypeExplicit(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	fetchIncomeHistoryOrFail(t, exg, "", "", 0, 10, map[string]interface{}{
		"accountType": "UNIFIED",
	})
}

func TestApi_FetchIncomeHistory_WithTransSubType(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	fetchIncomeHistoryOrFail(t, exg, "", "", 0, 10, map[string]interface{}{
		"transSubType": "movePosition",
	})
}

func TestApi_FetchIncomeHistory_WithCursorAfter(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	// Fetch a cursor from the raw endpoint first; FetchIncomeHistory only returns the list.
	args, _, _, _, err := exg.loadBybitOrderArgs("", nil)
	if err != nil {
		t.Fatalf("loadBybitOrderArgs failed: %v", err)
	}
	if _, ok := args["accountType"]; !ok {
		args["accountType"] = "UNIFIED"
	}
	// Expand the time range to improve the chance of getting at least 2 records.
	// Bybit requires endTime-startTime <= 7 days for this endpoint.
	now := time.Now().UTC()
	args["startTime"] = now.Add(-7*24*time.Hour + time.Minute).UnixMilli()
	args["endTime"] = now.UnixMilli()
	args["limit"] = 1
	tryNum := exg.GetRetryNum("FetchIncomeHistory", 1)
	res := requestRetry[V5ListResult](exg, MethodPrivateGetV5AccountTransactionLog, args, tryNum)
	if res.Error != nil {
		t.Fatalf("request transaction-log failed: %v", res.Error)
	}
	if res.Result.NextPageCursor == "" {
		t.Skip("no nextPageCursor returned; skip cursor pagination test")
	}
	fetchIncomeHistoryOrFail(t, exg, "", "", 0, 5, map[string]interface{}{
		banexg.ParamAfter: res.Result.NextPageCursor,
	})
}
