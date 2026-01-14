package okx

import (
	"math"
	"testing"

	"github.com/banbox/banexg"
)

func getAlgoPosition(t *testing.T, debug bool) (*OKX, *banexg.Position) {
	t.Helper()
	exg := getExchange(map[string]interface{}{
		banexg.OptMarketType: banexg.MarketLinear,
		banexg.OptDebugApi:   debug,
	})
	posList, err := exg.FetchAccountPositions(nil, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
	if err != nil {
		t.Fatalf("FetchAccountPositions: %v", err)
	}
	if len(posList) == 0 {
		t.Fatal("please create a linear position first")
	}
	return exg, posList[0]
}

func pickPosPrice(t *testing.T, pos *banexg.Position) float64 {
	t.Helper()
	price := pos.MarkPrice
	if price == 0 {
		price = pos.EntryPrice
	}
	if price == 0 && pos.Notional > 0 && pos.Contracts > 0 {
		price = pos.Notional / pos.Contracts
	}
	if price == 0 {
		t.Fatal("unable to determine position price")
	}
	return price
}

func pickPosQty(t *testing.T, pos *banexg.Position) float64 {
	t.Helper()
	qty := pos.Contracts
	if qty <= 0 {
		t.Fatal("position quantity is zero")
	}
	if qty > 1 {
		qty = 1
	}
	return qty
}

func algoOrderSide(pos *banexg.Position) string {
	if pos.Side == banexg.PosSideShort {
		return banexg.OdSideBuy
	}
	return banexg.OdSideSell
}

func algoPrices(pos *banexg.Position, price float64) (float64, float64) {
	if pos.Side == banexg.PosSideShort {
		return price * 1.1, price * 0.9
	}
	return price * 0.9, price * 1.1
}

func cancelAlgoOrder(t *testing.T, exg *OKX, symbol, id string) {
	t.Helper()
	_, err := exg.CancelOrder(id, symbol, map[string]interface{}{
		banexg.ParamAlgoOrder: true,
	})
	if err != nil {
		t.Logf("cancel algo order failed: %v", err)
	}
}

func TestMapAlgoOrderType(t *testing.T) {
	tests := []struct {
		name      string
		ordType   string
		tpTrigger float64
		tpOrd     float64
		slTrigger float64
		slOrd     float64
		trigger   float64
		ord       float64
		expected  string
	}{
		{"oco with both prices", "oco", 100, 99, 90, 91, 0, 0, "oco"},
		{"conditional sl market", "conditional", 0, 0, 90, -1, 0, 0, banexg.OdTypeStopLoss},
		{"conditional sl limit", "conditional", 0, 0, 90, 91, 0, 0, banexg.OdTypeStopLossLimit},
		{"conditional tp market", "conditional", 100, -1, 0, 0, 0, 0, banexg.OdTypeTakeProfitMarket},
		{"conditional tp limit", "conditional", 100, 99, 0, 0, 0, 0, banexg.OdTypeTakeProfitLimit},
		{"trigger market", "trigger", 0, 0, 0, 0, 95, -1, banexg.OdTypeStopMarket},
		{"trigger limit", "trigger", 0, 0, 0, 0, 95, 96, banexg.OdTypeStop},
		{"move_order_stop", "move_order_stop", 0, 0, 0, 0, 0, 0, banexg.OdTypeTrailingStopMarket},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapAlgoOrderType(tt.ordType, tt.tpTrigger, tt.tpOrd, tt.slTrigger, tt.slOrd, tt.trigger, tt.ord)
			if result != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestMapAlgoOrderStatus(t *testing.T) {
	tests := []struct {
		status   string
		expected string
	}{
		{"live", banexg.OdStatusOpen},
		{"pause", banexg.OdStatusOpen},
		{"partially_effective", banexg.OdStatusPartFilled},
		{"effective", banexg.OdStatusFilled},
		{"canceled", banexg.OdStatusCanceled},
		{"order_failed", banexg.OdStatusRejected},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			result := mapAlgoOrderStatus(tt.status)
			if result != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestIsAlgoOrderType(t *testing.T) {
	tests := []struct {
		odType   string
		expected bool
	}{
		{banexg.OdTypeStop, true},
		{banexg.OdTypeStopMarket, true},
		{banexg.OdTypeStopLoss, true},
		{banexg.OdTypeStopLossLimit, true},
		{banexg.OdTypeTakeProfit, true},
		{banexg.OdTypeTakeProfitLimit, true},
		{banexg.OdTypeTakeProfitMarket, true},
		{banexg.OdTypeTrailingStopMarket, true},
		{"conditional", true},
		{"oco", true},
		{"trigger", true},
		{"move_order_stop", true},
		{"twap", true},
		{"chase", true},
		{banexg.OdTypeLimit, false},
		{banexg.OdTypeMarket, false},
	}
	for _, tt := range tests {
		t.Run(tt.odType, func(t *testing.T) {
			result := isAlgoOrderType(tt.odType)
			if result != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// ============================================================================
// API Integration Tests - require local.json with valid credentials
// Run manually with: go test -run TestAPI_CreateAlgoOrderTPSL -v
// These tests are prefixed with TestAPI_ to distinguish them from unit tests.
// ============================================================================

func TestAPI_CreateAlgoOrderTPSL(t *testing.T) {
	exg, pos := getAlgoPosition(t, false)
	// If your account is in long/short mode, set ParamPositionSide in args to pos.Side.
	symbol := pos.Symbol
	curPrice := pickPosPrice(t, pos)
	quantity := pickPosQty(t, pos)
	side := algoOrderSide(pos)
	slPrice, tpPrice := algoPrices(pos, curPrice)

	t.Run("StopLossMarket", func(t *testing.T) {
		args := map[string]interface{}{
			banexg.ParamStopLossPrice: slPrice,
			banexg.ParamReduceOnly:    true,
			banexg.ParamPositionSide:  pos.Side,
		}
		order, err := exg.CreateOrder(symbol, banexg.OdTypeStopLoss, side, quantity, 0, args)
		if err != nil {
			t.Fatalf("Create stop loss order failed: %v", err)
		}
		defer cancelAlgoOrder(t, exg, symbol, order.ID)
		if order.ID == "" {
			t.Fatalf("empty order id")
		}
		fetched, err := exg.FetchOrder(symbol, order.ID, map[string]interface{}{
			banexg.ParamAlgoOrder: true,
		})
		if err != nil {
			t.Fatalf("Fetch stop loss order failed: %v", err)
		}
		if fetched.StopLossPrice == 0 {
			t.Fatalf("expected stop loss price, got 0")
		}
	})

	t.Run("TakeProfitMarket", func(t *testing.T) {
		args := map[string]interface{}{
			banexg.ParamTakeProfitPrice: tpPrice,
			banexg.ParamReduceOnly:      true,
			banexg.ParamPositionSide:    pos.Side,
		}
		order, err := exg.CreateOrder(symbol, banexg.OdTypeTakeProfitMarket, side, quantity, 0, args)
		if err != nil {
			t.Fatalf("Create take profit order failed: %v", err)
		}
		defer cancelAlgoOrder(t, exg, symbol, order.ID)
		fetched, err := exg.FetchOrder(symbol, order.ID, map[string]interface{}{
			banexg.ParamAlgoOrder: true,
		})
		if err != nil {
			t.Fatalf("Fetch take profit order failed: %v", err)
		}
		if fetched.TakeProfitPrice == 0 {
			t.Fatalf("expected take profit price, got 0")
		}
	})

	t.Run("OCO", func(t *testing.T) {
		args := map[string]interface{}{
			banexg.ParamStopLossPrice:   slPrice,
			banexg.ParamTakeProfitPrice: tpPrice,
			banexg.ParamReduceOnly:      true,
			banexg.ParamPositionSide:    pos.Side,
		}
		order, err := exg.CreateOrder(symbol, banexg.OdTypeStopLoss, side, quantity, 0, args)
		if err != nil {
			t.Fatalf("Create OCO order failed: %v", err)
		}
		defer cancelAlgoOrder(t, exg, symbol, order.ID)
		fetched, err := exg.FetchOrder(symbol, order.ID, map[string]interface{}{
			banexg.ParamAlgoOrder: true,
		})
		if err != nil {
			t.Fatalf("Fetch OCO order failed: %v", err)
		}
		if fetched.StopLossPrice == 0 || fetched.TakeProfitPrice == 0 {
			t.Fatalf("expected both stop loss and take profit prices, got sl=%v tp=%v", fetched.StopLossPrice, fetched.TakeProfitPrice)
		}
	})
}

func TestAPI_CreateAlgoOrderLimitTPSL(t *testing.T) {
	exg, pos := getAlgoPosition(t, false)
	symbol := pos.Symbol
	curPrice := pickPosPrice(t, pos)
	quantity := pickPosQty(t, pos)
	side := algoOrderSide(pos)
	slPrice, tpPrice := algoPrices(pos, curPrice)

	t.Run("StopLossLimit", func(t *testing.T) {
		price := math.Max(slPrice, 0)
		args := map[string]interface{}{
			banexg.ParamStopLossPrice: slPrice,
			banexg.ParamReduceOnly:    true,
			banexg.ParamPositionSide:  pos.Side,
		}
		order, err := exg.CreateOrder(symbol, banexg.OdTypeStopLossLimit, side, quantity, price, args)
		if err != nil {
			t.Fatalf("Create stop loss limit order failed: %v", err)
		}
		defer cancelAlgoOrder(t, exg, symbol, order.ID)
		fetched, err := exg.FetchOrder(symbol, order.ID, map[string]interface{}{
			banexg.ParamAlgoOrder: true,
		})
		if err != nil {
			t.Fatalf("Fetch stop loss limit order failed: %v", err)
		}
		if fetched.Type != banexg.OdTypeStopLossLimit {
			t.Fatalf("expected type %s, got %s", banexg.OdTypeStopLossLimit, fetched.Type)
		}
	})

	t.Run("TakeProfitLimit", func(t *testing.T) {
		price := math.Max(tpPrice, 0)
		args := map[string]interface{}{
			banexg.ParamTakeProfitPrice: tpPrice,
			banexg.ParamReduceOnly:      true,
			banexg.ParamPositionSide:    pos.Side,
		}
		order, err := exg.CreateOrder(symbol, banexg.OdTypeTakeProfitLimit, side, quantity, price, args)
		if err != nil {
			t.Fatalf("Create take profit limit order failed: %v", err)
		}
		defer cancelAlgoOrder(t, exg, symbol, order.ID)
		fetched, err := exg.FetchOrder(symbol, order.ID, map[string]interface{}{
			banexg.ParamAlgoOrder: true,
		})
		if err != nil {
			t.Fatalf("Fetch take profit limit order failed: %v", err)
		}
		if fetched.Type != banexg.OdTypeTakeProfitLimit {
			t.Fatalf("expected type %s, got %s", banexg.OdTypeTakeProfitLimit, fetched.Type)
		}
	})
}

func TestAPI_CreateAlgoOrderTrailingStop(t *testing.T) {
	exg, pos := getAlgoPosition(t, false)
	symbol := pos.Symbol
	quantity := pickPosQty(t, pos)
	side := algoOrderSide(pos)
	args := map[string]interface{}{
		banexg.ParamCallbackRate:    0.01,
		banexg.ParamActivationPrice: pickPosPrice(t, pos),
		banexg.ParamReduceOnly:      true,
		banexg.ParamPositionSide:    pos.Side,
	}
	order, err := exg.CreateOrder(symbol, banexg.OdTypeTrailingStopMarket, side, quantity, 0, args)
	if err != nil {
		panic(err)
	}
	defer cancelAlgoOrder(t, exg, symbol, order.ID)
	t.Logf("created trailing stop order: id=%s", order.ID)
	fetched, err := exg.FetchOrder(symbol, order.ID, map[string]interface{}{
		banexg.ParamAlgoOrder: true,
	})
	if err != nil {
		t.Fatalf("fetch trailing stop order failed: %v", err)
	}
	if fetched.Type != banexg.OdTypeTrailingStopMarket {
		t.Fatalf("expected type %s, got %s", banexg.OdTypeTrailingStopMarket, fetched.Type)
	}
}

func TestAPI_CreateAlgoOrderClosePosition(t *testing.T) {
	exg, pos := getAlgoPosition(t, false)
	symbol := pos.Symbol
	side := algoOrderSide(pos)
	curPrice := pickPosPrice(t, pos)
	slPrice, _ := algoPrices(pos, curPrice)
	args := map[string]interface{}{
		banexg.ParamStopLossPrice: slPrice,
		banexg.ParamClosePosition: true,
		banexg.ParamPositionSide:  pos.Side,
	}
	order, err := exg.CreateOrder(symbol, banexg.OdTypeStopLoss, side, 0, 0, args)
	if err != nil {
		panic(err)
	}
	defer cancelAlgoOrder(t, exg, symbol, order.ID)
	t.Logf("created close position order: id=%s", order.ID)
	fetched, err := exg.FetchOrder(symbol, order.ID, map[string]interface{}{
		banexg.ParamAlgoOrder: true,
	})
	if err != nil {
		t.Fatalf("fetch close position order failed: %v", err)
	}
	if !fetched.ReduceOnly {
		t.Fatalf("expected reduceOnly to be true")
	}
}

func TestAPI_FetchAlgoOrders(t *testing.T) {
	exg := getExchange(map[string]interface{}{
		banexg.OptMarketType: banexg.MarketLinear,
	})
	symbol := "ETH/USDT:USDT"
	orders, err := exg.FetchOpenOrders(symbol, 0, 10, map[string]interface{}{
		banexg.ParamAlgoOrder: true,
	})
	if err != nil {
		panic(err)
	}
	t.Logf("fetched %d algo orders", len(orders))
	for _, order := range orders {
		t.Logf("algo order: id=%s, type=%s, side=%s, status=%s",
			order.ID, order.Type, order.Side, order.Status)
	}
}
