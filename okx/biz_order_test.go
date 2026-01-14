package okx

import (
	"testing"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/utils"
)

func TestParseOrder(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	seedMarket(exg, "BTC-USDT", "BTC/USDT", banexg.MarketSpot)
	ord := &Order{
		InstType:  "SPOT",
		InstId:    "BTC-USDT",
		OrdId:     "123",
		ClOrdId:   "c1",
		Px:        "100",
		Sz:        "2",
		Side:      "buy",
		PosSide:   "net",
		OrdType:   "post_only",
		State:     "live",
		AvgPx:     "0",
		AccFillSz: "0",
		Fee:       "-0.01",
		FeeCcy:    "USDT",
		CTime:     "1700000000000",
		UTime:     "1700000001000",
	}
	res := parseOrder(exg, ord, map[string]interface{}{"reduceOnly": "false"}, "spot")
	if res == nil {
		t.Fatalf("unexpected nil order")
	}
	if res.Type != "limit_maker" || !res.PostOnly {
		t.Fatalf("unexpected type/postOnly: %s %v", res.Type, res.PostOnly)
	}
	if res.Status != "open" {
		t.Fatalf("unexpected status: %s", res.Status)
	}
	if res.Fee == nil || res.Fee.Cost != -0.01 {
		t.Fatalf("unexpected fee: %+v", res.Fee)
	}
}

func TestSetOrderID(t *testing.T) {
	args := map[string]interface{}{
		banexg.ParamClientOrderId: "c1",
	}
	if err := setOrderID(args, "o1"); err != nil {
		t.Fatalf("set order id: %v", err)
	}
	if args[FldOrdId] != "o1" {
		t.Fatalf("unexpected ordId: %v", args[FldOrdId])
	}
	if _, ok := args[FldClOrdId]; ok {
		t.Fatalf("unexpected clOrdId set")
	}
	if _, ok := args[banexg.ParamClientOrderId]; ok {
		t.Fatalf("clientOrderId should be popped")
	}

	args2 := map[string]interface{}{
		banexg.ParamClientOrderId: "c2",
	}
	if err := setOrderID(args2, ""); err != nil {
		t.Fatalf("set client order id: %v", err)
	}
	if args2[FldClOrdId] != "c2" {
		t.Fatalf("unexpected clOrdId: %v", args2[FldClOrdId])
	}

	if err := setOrderID(map[string]interface{}{}, ""); err == nil {
		t.Fatalf("expected error for empty order id")
	}
}

func TestParseOrderHistory(t *testing.T) {
	items := []map[string]interface{}{
		{
			"instType":   "SPOT",
			"instId":     "BTC-USDT",
			FldOrdId:     "680800019749904384",
			"ordType":    "market",
			"side":       "buy",
			"state":      "filled",
			"sz":         "100",
			"px":         "",
			"avgPx":      "51858",
			"accFillSz":  "100",
			"fee":        "-0.00000192834",
			"feeCcy":     "BTC",
			"reduceOnly": "false",
			"cTime":      "1708587373361",
			"uTime":      "1708587373362",
		},
	}
	var arr []Order
	if err := utils.DecodeStructMap(items, &arr, "json"); err != nil {
		t.Fatalf("decode order: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("unexpected order len: %d", len(arr))
	}
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	seedMarket(exg, "BTC-USDT", "BTC/USDT", banexg.MarketSpot)
	order := parseOrder(exg, &arr[0], items[0], "spot")
	if order == nil {
		t.Fatalf("unexpected nil order")
	}
	if order.Status != banexg.OdStatusFilled {
		t.Fatalf("unexpected status: %s", order.Status)
	}
	if order.Type != banexg.OdTypeMarket {
		t.Fatalf("unexpected type: %s", order.Type)
	}
	if order.Filled != 100 || order.Amount != 100 {
		t.Fatalf("unexpected filled/amount: %v/%v", order.Filled, order.Amount)
	}
	if order.Fee == nil || order.Fee.Cost != -0.00000192834 {
		t.Fatalf("unexpected fee: %+v", order.Fee)
	}
	if order.Symbol != "BTC/USDT" {
		t.Fatalf("unexpected symbol: %s", order.Symbol)
	}
}

// ============================================================================
// API Integration Tests - require local.json with valid credentials
// Run manually with: go test -run TestAPI_FetchOrder -v
// These tests are prefixed with TestAPI_ to distinguish them from unit tests.
// ============================================================================

func TestAPI_FetchOrder(t *testing.T) {
	exg := getExchange(nil)
	// Replace with a valid order ID from your account
	orderId := "3211916470425083904"
	symbol := "ETH/USDT"
	order, err := exg.FetchOrder(symbol, orderId, nil)
	if err != nil {
		panic(err)
	}
	t.Logf("order: id=%s, symbol=%s, type=%s, side=%s, status=%s, amount=%v, filled=%v, price=%v",
		order.ID, order.Symbol, order.Type, order.Side, order.Status, order.Amount, order.Filled, order.Price)
}

func TestAPI_FetchOrders(t *testing.T) {
	exg := getExchange(nil)
	symbol := "ETH/USDT"
	since := int64(0) // Fetch all
	orders, err := exg.FetchOrders(symbol, since, 10, nil)
	if err != nil {
		panic(err)
	}
	t.Logf("fetched %d orders for %s", len(orders), symbol)
	for _, order := range orders {
		t.Logf("order: id=%s, type=%s, side=%s, status=%s, amount=%v, filled=%v",
			order.ID, order.Type, order.Side, order.Status, order.Amount, order.Filled)
	}
}

func TestAPI_FetchOpenOrders(t *testing.T) {
	exg := getExchange(nil)
	symbol := "ETH/USDT"
	orders, err := exg.FetchOpenOrders(symbol, 0, 0, nil)
	if err != nil {
		panic(err)
	}
	t.Logf("fetched %d open orders for %s", len(orders), symbol)
	for _, order := range orders {
		t.Logf("open order: id=%s, type=%s, side=%s, price=%v, amount=%v",
			order.ID, order.Type, order.Side, order.Price, order.Amount)
	}
}

// TestAPI_CreateOrder creates a limit order. Use with caution!
// This test will actually submit an order to OKX.
func TestAPI_CreateOrder(t *testing.T) {
	exg := getExchange(nil)
	symbol := "ETH/USDT"
	// Create a limit buy order at a very low price (unlikely to fill)
	price := 3000.0 // Very low price
	amount := 0.001 // Minimum amount
	order, err := exg.CreateOrder(symbol, banexg.OdTypeLimit, banexg.OdSideBuy, amount, price, nil)
	if err != nil {
		panic(err)
	}
	t.Logf("created order: id=%s, symbol=%s, type=%s, side=%s, status=%s",
		order.ID, order.Symbol, order.Type, order.Side, order.Status)
	// Cancel the order immediately
	cancelled, err := exg.CancelOrder(order.ID, symbol, nil)
	if err != nil {
		t.Logf("failed to cancel order: %v", err)
	} else {
		t.Logf("cancelled order: id=%s, status=%s", cancelled.ID, cancelled.Status)
	}
}

func TestAPI_CancelOrder(t *testing.T) {
	exg := getExchange(nil)
	// Replace with a valid order ID from your account
	orderId := "your-order-id"
	symbol := "BTC/USDT"
	order, err := exg.CancelOrder(orderId, symbol, nil)
	if err != nil {
		panic(err)
	}
	t.Logf("cancelled order: id=%s, status=%s", order.ID, order.Status)
}

func TestAPI_CreateOrderWithStpMode(t *testing.T) {
	exg := getExchange(nil)
	symbol := "ETH/USDT"
	price := 3000.0
	amount := 0.001
	params := map[string]interface{}{
		banexg.ParamSelfTradePreventionMode: "cancel_maker",
		banexg.ParamTag:                     "testtag",
	}
	order, err := exg.CreateOrder(symbol, banexg.OdTypeLimit, banexg.OdSideBuy, amount, price, params)
	if err != nil {
		panic(err)
	}
	t.Logf("created order with stpMode: id=%s, symbol=%s", order.ID, order.Symbol)
	// Cancel immediately
	if _, err := exg.CancelOrder(order.ID, symbol, nil); err != nil {
		t.Logf("failed to cancel: %v", err)
	}
}
