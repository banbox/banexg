package okx

import (
	"testing"

	"github.com/banbox/banexg"
)

func TestParseBalance(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	bal := &Balance{
		UTime:   "1705474164160",
		TotalEq: "100",
		Details: []BalanceDetail{
			{Ccy: "USDT", Eq: "10", CashBal: "10", AvailBal: "8", FrozenBal: "2"},
		},
	}
	res := parseBalance(exg, bal, nil)
	if res == nil {
		t.Fatalf("unexpected nil balance")
	}
	ast := res.Assets["USDT"]
	if ast == nil {
		t.Fatalf("missing asset")
	}
	if ast.Free != 8 || ast.Used != 2 || ast.Total != 10 {
		t.Fatalf("unexpected asset: %+v", ast)
	}
}

func TestParsePosition(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	seedMarket(exg, "BTC-USDT-SWAP", "BTC/USDT:USDT", banexg.MarketLinear)
	pos := &Position{
		InstType: "SWAP",
		InstId:   "BTC-USDT-SWAP",
		MgnMode:  "cross",
		PosId:    "1",
		PosSide:  "net",
		Pos:      "-2",
		AvgPx:    "20000",
		Lever:    "5",
		LiqPx:    "15000",
		MarkPx:   "19900",
		Margin:   "100",
		MgnRatio: "0.1",
		Upl:      "-10",
		UTime:    "1700000000000",
	}
	res := parsePosition(exg, pos, nil)
	if res == nil {
		t.Fatalf("unexpected nil position")
	}
	if res.Side != "short" {
		t.Fatalf("unexpected side: %s", res.Side)
	}
	if res.Contracts != 2 {
		t.Fatalf("unexpected contracts: %v", res.Contracts)
	}
	if res.EntryPrice != 20000 || res.MarkPrice != 19900 {
		t.Fatalf("unexpected prices: %+v", res)
	}
}

func TestParsePositionsFilter(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	seedMarket(exg, "BTC-USDT-SWAP", "BTC/USDT:USDT", banexg.MarketLinear)
	seedMarket(exg, "ETH-USDT-SWAP", "ETH/USDT:USDT", banexg.MarketLinear)
	items := []map[string]interface{}{
		{
			"instType": "SWAP",
			"instId":   "BTC-USDT-SWAP",
			"mgnMode":  "cross",
			"posId":    "1",
			"posSide":  "net",
			"pos":      "1",
			"avgPx":    "30000",
			"lever":    "5",
			"liqPx":    "20000",
			"markPx":   "30100",
			"margin":   "10",
			"mgnRatio": "0.1",
			"upl":      "1",
			"uTime":    "1700000000000",
		},
		{
			"instType": "SWAP",
			"instId":   "ETH-USDT-SWAP",
			"mgnMode":  "cross",
			"posId":    "2",
			"posSide":  "net",
			"pos":      "-2",
			"avgPx":    "2000",
			"lever":    "3",
			"liqPx":    "1500",
			"markPx":   "1990",
			"margin":   "20",
			"mgnRatio": "0.2",
			"upl":      "-1",
			"uTime":    "1700000000001",
		},
	}
	filter := map[string]struct{}{
		"BTC/USDT:USDT": {},
	}
	res, err2 := parsePositions(exg, items, filter)
	if err2 != nil {
		t.Fatalf("parsePositions: %v", err2)
	}
	if len(res) != 1 {
		t.Fatalf("unexpected positions length: %d", len(res))
	}
	if res[0].Symbol != "BTC/USDT:USDT" {
		t.Fatalf("unexpected symbol: %s", res[0].Symbol)
	}
}

func TestParsePositionsAll(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	seedMarket(exg, "BTC-USDT-SWAP", "BTC/USDT:USDT", banexg.MarketLinear)
	seedMarket(exg, "ETH-USDT-SWAP", "ETH/USDT:USDT", banexg.MarketLinear)
	items := []map[string]interface{}{
		{
			"instType": "SWAP",
			"instId":   "BTC-USDT-SWAP",
			"mgnMode":  "cross",
			"posId":    "1",
			"posSide":  "net",
			"pos":      "1",
			"avgPx":    "30000",
			"lever":    "5",
			"liqPx":    "20000",
			"markPx":   "30100",
			"margin":   "10",
			"mgnRatio": "0.1",
			"upl":      "1",
			"uTime":    "1700000000000",
		},
		{
			"instType": "SWAP",
			"instId":   "ETH-USDT-SWAP",
			"mgnMode":  "cross",
			"posId":    "2",
			"posSide":  "net",
			"pos":      "-2",
			"avgPx":    "2000",
			"lever":    "3",
			"liqPx":    "1500",
			"markPx":   "1990",
			"margin":   "20",
			"mgnRatio": "0.2",
			"upl":      "-1",
			"uTime":    "1700000000001",
		},
	}
	res, err2 := parsePositions(exg, items, nil)
	if err2 != nil {
		t.Fatalf("parsePositions: %v", err2)
	}
	if len(res) != 2 {
		t.Fatalf("unexpected positions length: %d", len(res))
	}
	if res[1].Side != "short" {
		t.Fatalf("unexpected side: %s", res[1].Side)
	}
}

// ============================================================================
// API Integration Tests - require local.json with valid credentials
// Run manually with: go test -run TestAPI_FetchBalance -v
// These tests are prefixed with TestAPI_ to distinguish them from unit tests.
// ============================================================================

func TestAPI_FetchBalance(t *testing.T) {
	exg := getExchange(nil)
	balance, err := exg.FetchBalance(nil)
	if err != nil {
		panic(err)
	}
	t.Logf("fetched balance, total assets: %d", len(balance.Assets))
	for ccy, asset := range balance.Assets {
		if asset.Total > 0 {
			t.Logf("asset: %s, free=%v, used=%v, total=%v", ccy, asset.Free, asset.Used, asset.Total)
		}
	}
}

func TestAPI_FetchPositions(t *testing.T) {
	exg := getExchange(nil)
	// 设置市场为U本位合约市场
	params := map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	}
	positions, err := exg.FetchPositions(nil, params)
	if err != nil {
		panic(err)
	}
	t.Logf("fetched %d positions", len(positions))
	for _, pos := range positions {
		t.Logf("position: symbol=%s, side=%s, contracts=%v, entryPrice=%v, markPrice=%v, unrealizedPnl=%v",
			pos.Symbol, pos.Side, pos.Contracts, pos.EntryPrice, pos.MarkPrice, pos.UnrealizedPnl)
	}
}
