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
