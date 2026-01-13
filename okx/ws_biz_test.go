package okx

import (
	"testing"

	"github.com/banbox/banexg"
	"github.com/sasha-s/go-deadlock"
)

func TestParseWsTradeItem(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	seedMarket(exg, "BTC-USDT", "BTC/USDT", banexg.MarketSpot)
	item := map[string]interface{}{
		"instId":  "BTC-USDT",
		"tradeId": "100",
		"px":      "20000",
		"sz":      "0.1",
		"side":    "buy",
		"ts":      "1700000000000",
	}
	trade := parseWsTradeItem(exg, item)
	if trade == nil {
		t.Fatalf("unexpected nil trade")
	}
	if trade.Symbol != "BTC/USDT" || trade.Price != 20000 || trade.Amount != 0.1 {
		t.Fatalf("unexpected trade: %+v", trade)
	}
}

func TestParseWsBalanceData(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	items := []map[string]interface{}{
		{"ccy": "USDT", "cashBal": "10", "availBal": "8", "frozenBal": "2", "eq": "10"},
	}
	bal := parseWsBalanceData(exg, items)
	if bal == nil {
		t.Fatalf("unexpected nil balance")
	}
	ast := bal.Assets["USDT"]
	if ast == nil || ast.Free != 8 || ast.Used != 2 || ast.Total != 10 {
		t.Fatalf("unexpected asset: %+v", ast)
	}
}

func TestParseWsPositions(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	seedMarket(exg, "BTC-USDT-SWAP", "BTC/USDT:USDT", banexg.MarketLinear)
	items := []map[string]interface{}{
		{
			"instType": "SWAP",
			"instId":   "BTC-USDT-SWAP",
			"mgnMode":  "cross",
			"posId":    "1",
			"posSide":  "net",
			"pos":      "-2",
			"avgPx":    "20000",
			"lever":    "5",
			"liqPx":    "15000",
			"markPx":   "19900",
			"margin":   "100",
			"mgnRatio": "0.1",
			"upl":      "-10",
			"uTime":    "1700000000000",
		},
	}
	positions := parseWsPositions(exg, items)
	if len(positions) != 1 {
		t.Fatalf("unexpected positions len: %d", len(positions))
	}
	if positions[0].Side != "short" {
		t.Fatalf("unexpected side: %s", positions[0].Side)
	}
}

func TestParseWsMyTrade(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	seedMarket(exg, "BTC-USDT", "BTC/USDT", banexg.MarketSpot)
	item := map[string]interface{}{
		"instType":   "SPOT",
		"instId":     "BTC-USDT",
		"ordId":      "1",
		"clOrdId":    "c1",
		"fillPx":     "30000",
		"fillSz":     "0.2",
		"fillTime":   "1700000001000",
		"tradeId":    "t1",
		"side":       "buy",
		"ordType":    "limit",
		"avgPx":      "30000",
		"accFillSz":  "0.2",
		"state":      "filled",
		"posSide":    "net",
		"reduceOnly": "false",
		"fee":        "-0.001",
		"feeCcy":     "BTC",
	}
	trade := parseWsMyTrade(exg, item, "")
	if trade == nil {
		t.Fatalf("unexpected nil trade")
	}
	if trade.Symbol != "BTC/USDT" || trade.Amount != 0.2 || trade.Price != 30000 {
		t.Fatalf("unexpected trade: %+v", trade)
	}
	if trade.Type != banexg.OdTypeLimit {
		t.Fatalf("unexpected trade type: %s", trade.Type)
	}
	if trade.Fee == nil || trade.Fee.Currency != "BTC" {
		t.Fatalf("unexpected fee: %+v", trade.Fee)
	}
}

func TestParseWsBookSide(t *testing.T) {
	levels := []map[string]interface{}{
		{"0": "100", "1": "2"},
		{"0": "101", "1": "3"},
	}
	out := parseWsBookSide(levels)
	if len(out) != 2 {
		t.Fatalf("unexpected len: %d", len(out))
	}
	if out[0][0] != 100 || out[0][1] != 2 {
		t.Fatalf("unexpected level: %+v", out[0])
	}
}

func TestParseWsCandleItem(t *testing.T) {
	item := map[string]interface{}{
		"0": "1700000000000",
		"1": "100",
		"2": "110",
		"3": "90",
		"4": "105",
		"5": "12.5",
		"6": "200",
		"7": "300",
		"8": "0",
	}
	kline := parseWsCandleItem(item)
	if kline == nil {
		t.Fatalf("unexpected nil kline")
	}
	if kline.Open != 100 || kline.High != 110 || kline.Low != 90 || kline.Close != 105 {
		t.Fatalf("unexpected kline: %+v", kline)
	}
	if kline.Info != 300 {
		t.Fatalf("unexpected kline info: %+v", kline.Info)
	}
}

func TestParseWsMarkPriceItem(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	seedMarket(exg, "BTC-USDT-SWAP", "BTC/USDT:USDT", banexg.MarketLinear)
	item := map[string]interface{}{
		"instType": "SWAP",
		"instId":   "BTC-USDT-SWAP",
		"markPx":   "30000",
	}
	symbol, price, marketType, instId := parseWsMarkPriceItem(exg, item)
	if symbol != "BTC/USDT:USDT" || instId != "BTC-USDT-SWAP" || price != 30000 {
		t.Fatalf("unexpected mark price item: %s %s %f", symbol, instId, price)
	}
	if marketType != banexg.MarketLinear {
		t.Fatalf("unexpected market type: %s", marketType)
	}
}

func TestUpdateAccLeverages(t *testing.T) {
	acc := &banexg.Account{
		Leverages:    map[string]int{},
		LockLeverage: &deadlock.Mutex{},
	}
	positions := []*banexg.Position{
		{Symbol: "BTC/USDT:USDT", Leverage: 5},
		{Symbol: "ETH/USDT:USDT", Leverage: 3},
	}
	updates := updateAccLeverages(acc, positions)
	if len(updates) != 2 {
		t.Fatalf("unexpected updates len: %d", len(updates))
	}
	if acc.Leverages["BTC/USDT:USDT"] != 5 || acc.Leverages["ETH/USDT:USDT"] != 3 {
		t.Fatalf("unexpected leverages: %+v", acc.Leverages)
	}
	updates = updateAccLeverages(acc, positions)
	if len(updates) != 0 {
		t.Fatalf("unexpected updates after second call: %d", len(updates))
	}
}

// ============================================================================
// WebSocket Integration Tests - require local.json with valid credentials
// Run manually with: go test -run TestAPI_WatchOrderBooks -v
// These tests are prefixed with TestAPI_ to distinguish them from unit tests.
// ============================================================================

func TestAPI_WatchOrderBooks(t *testing.T) {
	exg := getExchange(nil)
	symbols := []string{"BTC/USDT", "ETH/USDT"}
	out, err := exg.WatchOrderBooks(symbols, 5, nil)
	if err != nil {
		panic(err)
	}
	t.Logf("watching order books for %v", symbols)
	// Read a few order book updates
	for i := 0; i < 5; i++ {
		select {
		case ob := <-out:
			t.Logf("orderbook update: symbol=%s, asks=%d, bids=%d", ob.Symbol, ob.Asks.Depth, ob.Bids.Depth)
		case <-make(chan struct{}):
			// Timeout
		}
	}
}

func TestAPI_WatchTrades(t *testing.T) {
	exg := getExchange(nil)
	symbols := []string{"BTC/USDT"}
	out, err := exg.WatchTrades(symbols, nil)
	if err != nil {
		panic(err)
	}
	t.Logf("watching trades for %v", symbols)
	// Read a few trade updates
	for i := 0; i < 5; i++ {
		select {
		case trade := <-out:
			t.Logf("trade: symbol=%s, price=%v, amount=%v, side=%s", trade.Symbol, trade.Price, trade.Amount, trade.Side)
		case <-make(chan struct{}):
			// Timeout
		}
	}
}

func TestAPI_WatchOHLCVs(t *testing.T) {
	exg := getExchange(nil)
	jobs := [][2]string{{"BTC/USDT", "1m"}}
	out, err := exg.WatchOHLCVs(jobs, nil)
	if err != nil {
		panic(err)
	}
	t.Logf("watching OHLCVs for %v", jobs)
	// Read a few kline updates
	for i := 0; i < 3; i++ {
		select {
		case kline := <-out:
			t.Logf("kline: symbol=%s, time=%d, O=%v H=%v L=%v C=%v", kline.Symbol, kline.Time, kline.Open, kline.High, kline.Low, kline.Close)
		case <-make(chan struct{}):
			// Timeout
		}
	}
}

func TestAPI_WatchMyTrades(t *testing.T) {
	exg := getExchange(nil)
	out, err := exg.WatchMyTrades(nil)
	if err != nil {
		panic(err)
	}
	t.Logf("watching my trades, waiting for updates...")
	// This will wait for incoming trade updates from your account
	select {
	case trade := <-out:
		t.Logf("my trade: symbol=%s, orderId=%s, amount=%v, price=%v", trade.Symbol, trade.Order, trade.Amount, trade.Price)
	case <-make(chan struct{}):
		t.Logf("no trades received")
	}
}

func TestAPI_WatchBalance(t *testing.T) {
	exg := getExchange(nil)
	out, err := exg.WatchBalance(nil)
	if err != nil {
		panic(err)
	}
	t.Logf("watching balance, waiting for updates...")
	// This will wait for balance updates
	select {
	case balance := <-out:
		t.Logf("balance update: assets=%d", len(balance.Assets))
		for ccy, asset := range balance.Assets {
			if asset.Total > 0 {
				t.Logf("asset: %s, total=%v", ccy, asset.Total)
			}
		}
	case <-make(chan struct{}):
		t.Logf("no balance updates received")
	}
}

func TestAPI_WatchPositions(t *testing.T) {
	exg := getExchange(nil)
	out, err := exg.WatchPositions(nil)
	if err != nil {
		panic(err)
	}
	t.Logf("watching positions, waiting for updates...")
	// This will wait for position updates
	select {
	case positions := <-out:
		t.Logf("position update: count=%d", len(positions))
		for _, pos := range positions {
			t.Logf("position: symbol=%s, side=%s, contracts=%v", pos.Symbol, pos.Side, pos.Contracts)
		}
	case <-make(chan struct{}):
		t.Logf("no position updates received")
	}
}
