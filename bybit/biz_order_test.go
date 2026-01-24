package bybit

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"go.uber.org/zap"
)

func ensureBybitMarketPrecision(exg *Bybit, symbol string) *banexg.Market {
	market := exg.Markets[symbol]
	if market.Precision == nil {
		market.Precision = &banexg.Precision{
			Price:      0.1,
			Amount:     0.001,
			ModePrice:  banexg.PrecModeTickSize,
			ModeAmount: banexg.PrecModeTickSize,
		}
	}
	if market.Linear || market.Inverse || market.Option {
		market.Contract = true
	}
	return market
}

func setBybitTestRequestWithEndpoint(t *testing.T, wantEndpoint string, fn func(params map[string]interface{}) *banexg.HttpRes) {
	t.Helper()
	setBybitTestRequest(t, func(_ context.Context, endpoint string, params map[string]interface{}, _ int, _ bool, _ bool) *banexg.HttpRes {
		if endpoint != wantEndpoint {
			t.Fatalf("unexpected endpoint: %s", endpoint)
		}
		return fn(params)
	})
}

func bybitPrecCostMust(t *testing.T, exg *Bybit, market *banexg.Market, cost float64) float64 {
	t.Helper()
	v, err := exg.PrecCost(market, cost)
	if err != nil {
		t.Fatalf("PrecCost failed: %v", err)
	}
	return v
}

func bybitPrecAmountStrMust(t *testing.T, exg *Bybit, market *banexg.Market, amount float64) string {
	t.Helper()
	prec := bybitPrecAmountMust(t, exg, market, amount)
	return strconv.FormatFloat(prec, 'f', -1, 64)
}

func bybitPrecPriceStrMust(t *testing.T, exg *Bybit, market *banexg.Market, price float64) string {
	t.Helper()
	prec := bybitPrecPriceMust(t, exg, market, price)
	return strconv.FormatFloat(prec, 'f', -1, 64)
}

func bybitPrecCostStrMust(t *testing.T, exg *Bybit, market *banexg.Market, cost float64) string {
	t.Helper()
	prec := bybitPrecCostMust(t, exg, market, cost)
	return strconv.FormatFloat(prec, 'f', -1, 64)
}

func TestEditOrderAllowsZeroTpsl(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	ensureBybitMarketPrecision(exg, "BTC/USDT:USDT")
	setBybitTestRequestWithEndpoint(t, MethodPrivatePostV5OrderAmend, func(params map[string]interface{}) *banexg.HttpRes {
		if params["category"] != banexg.MarketLinear {
			t.Fatalf("unexpected category: %v", params["category"])
		}
		if params["symbol"] != "BTCUSDT" {
			t.Fatalf("unexpected symbol: %v", params["symbol"])
		}
		if params["orderId"] != "order-1" {
			t.Fatalf("unexpected orderId: %v", params["orderId"])
		}
		if params["takeProfit"] != "0" {
			t.Fatalf("expected takeProfit=0, got %v", params["takeProfit"])
		}
		if params["stopLoss"] != "0" {
			t.Fatalf("expected stopLoss=0, got %v", params["stopLoss"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"orderId":"order-1","orderLinkId":"link-1"},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})

	_, err := exg.EditOrder("BTC/USDT:USDT", "order-1", "buy", 0, 0, map[string]interface{}{
		banexg.ParamTakeProfitPrice: float64(0),
		banexg.ParamStopLossPrice:   float64(0),
	})
	if err != nil {
		t.Fatalf("EditOrder failed: %v", err)
	}
}

func TestEditOrderUsesClientOrderID(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	market := ensureBybitMarketPrecision(exg, "BTC/USDT:USDT")
	amount := 0.1234
	price := 101.23
	expectedAmt := bybitPrecAmountStrMust(t, exg, market, amount)
	expectedPrice := bybitPrecPriceStrMust(t, exg, market, price)
	setBybitTestRequestWithEndpoint(t, MethodPrivatePostV5OrderAmend, func(params map[string]interface{}) *banexg.HttpRes {
		if params["orderLinkId"] != "link-1" {
			t.Fatalf("unexpected orderLinkId: %v", params["orderLinkId"])
		}
		if _, ok := params["orderId"]; ok {
			t.Fatalf("unexpected orderId: %v", params["orderId"])
		}
		if params["qty"] != expectedAmt {
			t.Fatalf("unexpected qty: %v", params["qty"])
		}
		if params["price"] != expectedPrice {
			t.Fatalf("unexpected price: %v", params["price"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"orderId":"order-7","orderLinkId":"link-1"},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})

	_, err := exg.EditOrder("BTC/USDT:USDT", "", "buy", amount, price, map[string]interface{}{
		banexg.ParamClientOrderId: "link-1",
	})
	if err != nil {
		t.Fatalf("EditOrder failed: %v", err)
	}
}

func TestEditOrderLinearExtraParams(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	market := ensureBybitMarketPrecision(exg, "BTC/USDT:USDT")
	tpLimit := 120.1
	slLimit := 90.2
	expectedTp := bybitPrecPriceStrMust(t, exg, market, tpLimit)
	expectedSl := bybitPrecPriceStrMust(t, exg, market, slLimit)
	setBybitTestRequestWithEndpoint(t, MethodPrivatePostV5OrderAmend, func(params map[string]interface{}) *banexg.HttpRes {
		if params["triggerBy"] != "MarkPrice" {
			t.Fatalf("unexpected triggerBy: %v", params["triggerBy"])
		}
		if params["tpTriggerBy"] != "IndexPrice" {
			t.Fatalf("unexpected tpTriggerBy: %v", params["tpTriggerBy"])
		}
		if params["slTriggerBy"] != "LastPrice" {
			t.Fatalf("unexpected slTriggerBy: %v", params["slTriggerBy"])
		}
		if params["tpslMode"] != "Partial" {
			t.Fatalf("unexpected tpslMode: %v", params["tpslMode"])
		}
		if params["tpOrderType"] != "Limit" || params["slOrderType"] != "Limit" {
			t.Fatalf("unexpected tp/sl order types: %#v", params)
		}
		if params["tpLimitPrice"] != expectedTp {
			t.Fatalf("unexpected tpLimitPrice: %v", params["tpLimitPrice"])
		}
		if params["slLimitPrice"] != expectedSl {
			t.Fatalf("unexpected slLimitPrice: %v", params["slLimitPrice"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"orderId":"order-9","orderLinkId":""},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})

	_, err := exg.EditOrder("BTC/USDT:USDT", "order-9", banexg.OdSideBuy, 0, 0, map[string]interface{}{
		"triggerBy":    "MarkPrice",
		"tpTriggerBy":  "IndexPrice",
		"slTriggerBy":  "LastPrice",
		"tpslMode":     "Partial",
		"tpOrderType":  "Limit",
		"slOrderType":  "Limit",
		"tpLimitPrice": tpLimit,
		"slLimitPrice": slLimit,
	})
	if err != nil {
		t.Fatalf("EditOrder extra params failed: %v", err)
	}
}

func TestEditOrderOptionOrderIv(t *testing.T) {
	exg := newBybitWithMarket("BTC-30DEC22-18000-C", "BTC/USDT:USDT-30DEC22-18000-C", banexg.MarketOption)
	ensureBybitMarketPrecision(exg, "BTC/USDT:USDT-30DEC22-18000-C")
	setBybitTestRequestWithEndpoint(t, MethodPrivatePostV5OrderAmend, func(params map[string]interface{}) *banexg.HttpRes {
		if params["orderIv"] != "0.1" {
			t.Fatalf("unexpected orderIv: %v", params["orderIv"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"orderId":"order-opt-iv","orderLinkId":""},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})

	_, err := exg.EditOrder("BTC/USDT:USDT-30DEC22-18000-C", "order-opt-iv", banexg.OdSideBuy, 0, 0, map[string]interface{}{
		"orderIv": 0.1,
	})
	if err != nil {
		t.Fatalf("EditOrder option orderIv failed: %v", err)
	}
}

func TestEditOrderOptionRejectsTpSlParams(t *testing.T) {
	exg := newBybitWithMarket("BTC-30DEC22-18000-C", "BTC/USDT:USDT-30DEC22-18000-C", banexg.MarketOption)
	ensureBybitMarketPrecision(exg, "BTC/USDT:USDT-30DEC22-18000-C")
	_, err := exg.EditOrder("BTC/USDT:USDT-30DEC22-18000-C", "order-opt-1", banexg.OdSideBuy, 0, 0, map[string]interface{}{
		"tpLimitPrice": 120.0,
	})
	if err == nil || err.Code != errs.CodeParamInvalid {
		t.Fatalf("expected option tp/sl param error, got %v", err)
	}
}

func TestBybitSideAndPositionIdx(t *testing.T) {
	side, err := bybitSide("buy")
	if err != nil || side != "Buy" {
		t.Fatalf("expected Buy, got %v err=%v", side, err)
	}
	side, err = bybitSide(" Sell ")
	if err != nil || side != "Sell" {
		t.Fatalf("expected Sell, got %v err=%v", side, err)
	}
	if _, err = bybitSide("hold"); err == nil {
		t.Fatal("expected error for invalid side")
	}

	cases := map[string]struct {
		idx int
		ok  bool
	}{
		"":                  {0, true},
		"net":               {0, true},
		"both":              {0, true},
		banexg.PosSideLong:  {1, true},
		banexg.PosSideShort: {2, true},
		"bad":               {0, false},
	}
	for input, tc := range cases {
		idx, ok := bybitPositionIdx(input)
		if idx != tc.idx || ok != tc.ok {
			t.Fatalf("positionIdx(%q) expected (%d,%v), got (%d,%v)", input, tc.idx, tc.ok, idx, ok)
		}
	}
}

func TestBybitTimeInForceMapping(t *testing.T) {
	if got := normalizeBybitTimeInForce(""); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := normalizeBybitTimeInForce(banexg.TimeInForcePO); got != "PostOnly" {
		t.Fatalf("expected PostOnly, got %q", got)
	}
	if got := parseBybitTimeInForce("PostOnly"); got != banexg.TimeInForcePO {
		t.Fatalf("expected PO, got %q", got)
	}
	if got := parseBybitTimeInForce(banexg.TimeInForceGTC); got != banexg.TimeInForceGTC {
		t.Fatalf("expected GTC, got %q", got)
	}
}

func TestBybitOrderTypeMapping(t *testing.T) {
	if got := bybitOrderTypeFrom(banexg.OdTypeStopLoss, 0); got != "Market" {
		t.Fatalf("expected Market for stop loss without price, got %q", got)
	}
	if got := bybitOrderTypeFrom(banexg.OdTypeLimit, 0); got != "Limit" {
		t.Fatalf("expected Limit, got %q", got)
	}
	if got := parseBybitOrderType("Limit", "StopLoss", 0); got != banexg.OdTypeStopLossLimit {
		t.Fatalf("expected StopLossLimit, got %q", got)
	}
	if got := parseBybitOrderType("Market", "TakeProfit", 0); got != banexg.OdTypeTakeProfitMarket {
		t.Fatalf("expected TakeProfitMarket, got %q", got)
	}
	if got := parseBybitOrderType("Market", "TrailingStop", 0); got != banexg.OdTypeTrailingStopMarket {
		t.Fatalf("expected TrailingStopMarket, got %q", got)
	}
	if got := parseBybitOrderType("Market", "", 100); got != banexg.OdTypeStopMarket {
		t.Fatalf("expected StopMarket for triggerPrice, got %q", got)
	}
}

func TestIsBybitStopOrderType(t *testing.T) {
	cases := map[string]bool{
		banexg.OdTypeStop:               true,
		banexg.OdTypeStopMarket:         true,
		banexg.OdTypeStopLoss:           true,
		banexg.OdTypeStopLossLimit:      true,
		banexg.OdTypeTakeProfit:         true,
		banexg.OdTypeTakeProfitLimit:    true,
		banexg.OdTypeTakeProfitMarket:   true,
		banexg.OdTypeTrailingStopMarket: false,
		banexg.OdTypeLimit:              false,
		banexg.OdTypeMarket:             false,
	}
	for input, expected := range cases {
		if got := isBybitStopOrderType(input); got != expected {
			t.Fatalf("isBybitStopOrderType(%q) expected %v, got %v", input, expected, got)
		}
	}
}

func TestParseBybitOrderStatus(t *testing.T) {
	cases := map[string]string{
		"New":                     banexg.OdStatusOpen,
		"Untriggered":             banexg.OdStatusOpen,
		"Triggered":               banexg.OdStatusOpen,
		"PartiallyFilled":         banexg.OdStatusPartFilled,
		"Filled":                  banexg.OdStatusFilled,
		"Cancelled":               banexg.OdStatusCanceled,
		"Deactivated":             banexg.OdStatusCanceled,
		"Rejected":                banexg.OdStatusRejected,
		"PartiallyFilledCanceled": banexg.OdStatusCanceled,
	}
	for input, expected := range cases {
		if got := parseBybitOrderStatus(input); got != expected {
			t.Fatalf("status %q expected %q, got %q", input, expected, got)
		}
	}
	if got := parseBybitOrderStatus("Unknown"); got != "Unknown" {
		t.Fatalf("expected unknown status passthrough, got %q", got)
	}
}

func TestParseBybitOrderFee(t *testing.T) {
	info := map[string]interface{}{
		"cumFeeDetail": map[string]interface{}{
			"USDT": "0.5",
			"BTC":  "0",
		},
	}
	fee := parseBybitOrderFee(nil, info)
	if fee == nil || fee.Currency != "USDT" || fee.Cost != 0.5 {
		t.Fatalf("unexpected fee: %#v", fee)
	}

	info = map[string]interface{}{
		"cumExecFee":  "0.01",
		"feeCurrency": "USDT",
	}
	fee = parseBybitOrderFee(nil, info)
	if fee == nil || fee.Currency != "USDT" || fee.Cost != 0.01 {
		t.Fatalf("unexpected fee from cumExecFee: %#v", fee)
	}
}

func TestSetBybitPriceArg(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	market := ensureBybitMarketPrecision(exg, "BTC/USDT:USDT")
	args := map[string]interface{}{}

	if err := setBybitPriceArg(exg, market, args, "price", 0, false); err != nil {
		t.Fatalf("setBybitPriceArg failed: %v", err)
	}
	if _, ok := args["price"]; ok {
		t.Fatal("expected price to be omitted when allowZero=false")
	}
	if err := setBybitPriceArg(exg, market, args, "price", 0, true); err != nil {
		t.Fatalf("setBybitPriceArg allowZero failed: %v", err)
	}
	if args["price"] != "0" {
		t.Fatalf("expected price=0, got %v", args["price"])
	}
	delete(args, "price")
	if err := setBybitPriceArg(exg, market, args, "price", -1, true); err != nil {
		t.Fatalf("setBybitPriceArg negative failed: %v", err)
	}
	if _, ok := args["price"]; ok {
		t.Fatal("expected negative price to be omitted")
	}
	if err := setBybitPriceArg(exg, market, args, "price", 100.1, false); err != nil {
		t.Fatalf("setBybitPriceArg precision failed: %v", err)
	}
	if args["price"] != "100.1" {
		t.Fatalf("unexpected price value: %v", args["price"])
	}
}

func TestPopBybitFloatArgAndSetPriceArgs(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	market := ensureBybitMarketPrecision(exg, "BTC/USDT:USDT")
	args := map[string]interface{}{banexg.ParamTriggerPrice: 12.3}
	val, ok := popBybitFloatArg(args, banexg.ParamTriggerPrice)
	if !ok || val != 12.3 {
		t.Fatalf("popBybitFloatArg expected 12.3 true, got %v %v", val, ok)
	}
	if _, exists := args[banexg.ParamTriggerPrice]; exists {
		t.Fatal("expected ParamTriggerPrice to be removed")
	}

	args = map[string]interface{}{
		banexg.ParamTriggerPrice:  0.0,
		banexg.ParamStopLossPrice: 100.2,
	}
	if err := popAndSetBybitPriceArgs(exg, market, args, true,
		bybitPriceParam{param: banexg.ParamTriggerPrice, key: "triggerPrice"},
		bybitPriceParam{param: banexg.ParamStopLossPrice, key: "stopLoss"},
	); err != nil {
		t.Fatalf("popAndSetBybitPriceArgs failed: %v", err)
	}
	if args["triggerPrice"] != "0" {
		t.Fatalf("expected triggerPrice=0, got %v", args["triggerPrice"])
	}
	if args["stopLoss"] != "100.2" {
		t.Fatalf("expected stopLoss=100.2, got %v", args["stopLoss"])
	}
	if _, exists := args[banexg.ParamStopLossPrice]; exists {
		t.Fatal("expected ParamStopLossPrice to be removed after pop")
	}
}

func TestParseBybitOrder(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	ensureBybitMarketPrecision(exg, "BTC/USDT:USDT")
	item := &OrderInfo{
		orderRef:      orderRef{OrderId: "order-1", OrderLinkId: "link-1"},
		Symbol:        "BTCUSDT",
		Side:          "Buy",
		OrderType:     "Limit",
		OrderStatus:   "PartiallyFilled",
		TimeInForce:   "PostOnly",
		Price:         "100",
		Qty:           "2",
		LeavesQty:     "0",
		CumExecQty:    "1.5",
		CumExecValue:  "150",
		AvgPrice:      "100",
		TriggerPrice:  "95",
		TakeProfit:    "110",
		StopLoss:      "90",
		StopOrderType: "Stop",
		ReduceOnly:    true,
		PositionIdx:   2,
		CreatedTime:   "1700000000000",
		UpdatedTime:   "1700000001000",
	}
	info := map[string]interface{}{
		"cumFeeDetail": map[string]interface{}{"USDT": "0.1"},
	}
	order := parseBybitOrder(exg, item, info, banexg.MarketLinear)
	if order == nil {
		t.Fatal("expected order")
	}
	if order.Symbol != "BTC/USDT:USDT" {
		t.Fatalf("unexpected symbol: %s", order.Symbol)
	}
	if order.Type != banexg.OdTypeStop {
		t.Fatalf("unexpected order type: %s", order.Type)
	}
	if order.Status != banexg.OdStatusPartFilled {
		t.Fatalf("unexpected status: %s", order.Status)
	}
	if order.Remaining != 0.5 {
		t.Fatalf("unexpected remaining: %v", order.Remaining)
	}
	if order.Cost != 150 {
		t.Fatalf("unexpected cost: %v", order.Cost)
	}
	if order.TimeInForce != banexg.TimeInForcePO || !order.PostOnly {
		t.Fatalf("unexpected timeInForce/postOnly: %s/%v", order.TimeInForce, order.PostOnly)
	}
	if order.PositionSide != banexg.PosSideShort {
		t.Fatalf("unexpected position side: %s", order.PositionSide)
	}
	if order.LastTradeTimestamp != 1700000001000 {
		t.Fatalf("unexpected lastTradeTimestamp: %d", order.LastTradeTimestamp)
	}
	if order.Fee == nil || order.Fee.Currency != "USDT" || order.Fee.Cost != 0.1 {
		t.Fatalf("unexpected fee: %#v", order.Fee)
	}
}

func TestParseBybitOrderCostFromAverage(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	ensureBybitMarketPrecision(exg, "BTC/USDT:USDT")
	item := &OrderInfo{
		orderRef:     orderRef{OrderId: "order-cost"},
		Symbol:       "BTCUSDT",
		Side:         "Buy",
		OrderType:    "Limit",
		OrderStatus:  "Filled",
		Price:        "100",
		Qty:          "2",
		CumExecQty:   "2",
		CumExecValue: "0",
		AvgPrice:     "150",
		CreatedTime:  "1700000000000",
		UpdatedTime:  "1700000001000",
	}
	order := parseBybitOrder(exg, item, map[string]interface{}{}, banexg.MarketLinear)
	if order == nil {
		t.Fatal("expected order")
	}
	if order.Cost != 300 {
		t.Fatalf("expected cost 300 from avg*filled, got %v", order.Cost)
	}
}

func TestParseBybitOrderAverageFromCost(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	ensureBybitMarketPrecision(exg, "BTC/USDT:USDT")
	item := &OrderInfo{
		orderRef:     orderRef{OrderId: "order-avg"},
		Symbol:       "BTCUSDT",
		Side:         "Buy",
		OrderType:    "Limit",
		OrderStatus:  "Filled",
		Price:        "100",
		Qty:          "2",
		CumExecQty:   "2",
		CumExecValue: "300",
		AvgPrice:     "0",
		CreatedTime:  "1700000000000",
		UpdatedTime:  "1700000001000",
	}
	order := parseBybitOrder(exg, item, map[string]interface{}{}, banexg.MarketLinear)
	if order == nil {
		t.Fatal("expected order")
	}
	if order.Average != 150 {
		t.Fatalf("expected average 150, got %v", order.Average)
	}
}

func TestParseBybitOrdersSymbolFilter(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	ensureBybitMarketPrecision(exg, "BTC/USDT:USDT")
	ethMarket := &banexg.Market{ID: "ETHUSDT", Symbol: "ETH/USDT:USDT", Type: banexg.MarketLinear, Linear: true, Contract: true}
	exg.Markets[ethMarket.Symbol] = ethMarket
	exg.MarketsById[ethMarket.ID] = []*banexg.Market{ethMarket}

	items := []map[string]interface{}{
		{
			"orderId":     "order-1",
			"symbol":      "BTCUSDT",
			"side":        "Buy",
			"orderType":   "Limit",
			"orderStatus": "New",
			"timeInForce": "GTC",
			"price":       "100",
			"qty":         "1",
			"cumExecQty":  "0",
			"leavesQty":   "1",
			"createdTime": "1700000000000",
			"updatedTime": "1700000001000",
		},
		{
			"orderId":     "order-2",
			"symbol":      "ETHUSDT",
			"side":        "Sell",
			"orderType":   "Limit",
			"orderStatus": "New",
			"timeInForce": "GTC",
			"price":       "200",
			"qty":         "2",
			"cumExecQty":  "0",
			"leavesQty":   "2",
			"createdTime": "1700000000000",
			"updatedTime": "1700000001000",
		},
	}
	orders, err := parseBybitOrders(exg, items, banexg.MarketLinear, "BTC/USDT:USDT")
	if err != nil {
		t.Fatalf("parseBybitOrders failed: %v", err)
	}
	if len(orders) != 1 || orders[0].ID != "order-1" || orders[0].Symbol != "BTC/USDT:USDT" {
		t.Fatalf("unexpected orders: %#v", orders)
	}
}

func TestParseBybitMyTrade(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	item := &ExecutionInfo{
		Symbol:      "BTCUSDT",
		orderRef:    orderRef{OrderId: "order-1", OrderLinkId: "link-1"},
		Side:        "Sell",
		OrderType:   "Limit",
		OrderQty:    "1.2",
		LeavesQty:   "0.2",
		ExecId:      "exec-1",
		ExecPrice:   "100",
		ExecQty:     "0.5",
		ExecFee:     "0.01",
		FeeCurrency: "USDT",
		FeeRate:     "0.001",
		ExecType:    "Trade",
		ExecTime:    "1700000002000",
		IsMaker:     true,
	}
	trade := parseBybitMyTrade(exg, item, map[string]interface{}{"raw": true}, banexg.MarketLinear)
	if trade == nil {
		t.Fatal("expected trade")
	}
	if trade.Symbol != "BTC/USDT:USDT" {
		t.Fatalf("unexpected symbol: %s", trade.Symbol)
	}
	if trade.Price != 100 || trade.Amount != 0.5 || trade.Cost != 50 {
		t.Fatalf("unexpected trade values: price=%v amount=%v cost=%v", trade.Price, trade.Amount, trade.Cost)
	}
	if trade.Filled != 1.0 {
		t.Fatalf("unexpected filled: %v", trade.Filled)
	}
	if trade.Fee == nil || trade.Fee.Currency != "USDT" || trade.Fee.Cost != 0.01 || !trade.Fee.IsMaker {
		t.Fatalf("unexpected fee: %#v", trade.Fee)
	}
	if trade.State != banexg.OdStatusPartFilled {
		t.Fatalf("unexpected state: %s", trade.State)
	}
}

func TestParseBybitMyTradeUsesExecValue(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	item := &ExecutionInfo{
		Symbol:    "BTCUSDT",
		orderRef:  orderRef{OrderId: "order-2"},
		Side:      "Buy",
		OrderType: "Limit",
		OrderQty:  "1",
		LeavesQty: "0",
		ExecId:    "exec-2",
		ExecPrice: "100",
		ExecQty:   "0.5",
		ExecValue: "40",
		ExecType:  "Trade",
		ExecTime:  "1700000004000",
	}
	trade := parseBybitMyTrade(exg, item, map[string]interface{}{}, banexg.MarketLinear)
	if trade == nil {
		t.Fatal("expected trade")
	}
	if trade.Cost != 40 {
		t.Fatalf("expected cost 40 from execValue, got %v", trade.Cost)
	}
}

func TestParseBybitIncome(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	item := &TransLogInfo{
		ID:              "income-1",
		Symbol:          "BTCUSDT",
		TransactionTime: "1700000003000",
		Type:            "FUNDING",
		Currency:        "USDT",
		CashFlow:        "100",
		Funding:         "5",
		Fee:             "1",
		Change:          "0",
		TradeId:         "trade-1",
	}
	income := parseBybitIncome(exg, item, map[string]interface{}{"raw": true}, banexg.MarketLinear)
	if income == nil {
		t.Fatal("expected income")
	}
	if income.Symbol != "BTC/USDT:USDT" {
		t.Fatalf("unexpected symbol: %s", income.Symbol)
	}
	if income.Income != 104 {
		t.Fatalf("unexpected income: %v", income.Income)
	}
	if income.Asset != "USDT" {
		t.Fatalf("unexpected asset: %s", income.Asset)
	}
}

func TestParseBybitIncomeUsesChange(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	item := &TransLogInfo{
		ID:              "income-2",
		Symbol:          "BTCUSDT",
		TransactionTime: "1700000005000",
		Type:            "FUNDING",
		Currency:        "USDT",
		Change:          "12.5",
		CashFlow:        "100",
		Funding:         "5",
		Fee:             "1",
	}
	income := parseBybitIncome(exg, item, map[string]interface{}{}, banexg.MarketLinear)
	if income == nil {
		t.Fatal("expected income")
	}
	if income.Income != 12.5 {
		t.Fatalf("expected income 12.5 from change, got %v", income.Income)
	}
}

func TestCreateOrderSpotMarketByCost(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	market := ensureBybitMarketPrecision(exg, "BTC/USDT")
	cost := 12.34
	expectedQty := bybitPrecCostStrMust(t, exg, market, cost)
	setBybitTestRequestWithEndpoint(t, MethodPrivatePostV5OrderCreate, func(params map[string]interface{}) *banexg.HttpRes {
		if params["category"] != banexg.MarketSpot || params["symbol"] != "BTCUSDT" {
			t.Fatalf("unexpected market params: %#v", params)
		}
		if params["side"] != "Buy" || params["orderType"] != "Market" {
			t.Fatalf("unexpected side/orderType: %#v", params)
		}
		if params["qty"] != expectedQty {
			t.Fatalf("unexpected qty: %v", params["qty"])
		}
		if params["marketUnit"] != "quoteCoin" {
			t.Fatalf("expected marketUnit=quoteCoin, got %v", params["marketUnit"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"orderId":"order-1","orderLinkId":"link-1"},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	if _, err := exg.CreateOrder("BTC/USDT", banexg.OdTypeMarket, banexg.OdSideBuy, 0, 0, map[string]interface{}{
		banexg.ParamCost: cost,
	}); err != nil {
		t.Fatalf("CreateOrder failed: %v", err)
	}
}

func TestCreateOrderStopLossTriggerPrice(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	market := ensureBybitMarketPrecision(exg, "BTC/USDT:USDT")
	stopLoss := 123.45
	expectedStop := bybitPrecPriceStrMust(t, exg, market, stopLoss)
	setBybitTestRequestWithEndpoint(t, MethodPrivatePostV5OrderCreate, func(params map[string]interface{}) *banexg.HttpRes {
		if params["orderType"] != "Market" {
			t.Fatalf("unexpected orderType: %v", params["orderType"])
		}
		if params["triggerPrice"] != expectedStop {
			t.Fatalf("unexpected triggerPrice: %v", params["triggerPrice"])
		}
		if _, ok := params["stopLoss"]; ok {
			t.Fatalf("unexpected attached stopLoss for conditional order: %v", params["stopLoss"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"orderId":"order-4","orderLinkId":""},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	if _, err := exg.CreateOrder("BTC/USDT:USDT", banexg.OdTypeStopLoss, banexg.OdSideBuy, 1, 0, map[string]interface{}{
		banexg.ParamStopLossPrice: stopLoss,
	}); err != nil {
		t.Fatalf("CreateOrder stop loss failed: %v", err)
	}
}

func TestCreateOrderTpSlLimitPriceFormatting(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	market := ensureBybitMarketPrecision(exg, "BTC/USDT")
	tpLimit := 123.456
	slLimit := 120.123
	expectedTp := bybitPrecPriceStrMust(t, exg, market, tpLimit)
	expectedSl := bybitPrecPriceStrMust(t, exg, market, slLimit)
	setBybitTestRequestWithEndpoint(t, MethodPrivatePostV5OrderCreate, func(params map[string]interface{}) *banexg.HttpRes {
		if params["tpLimitPrice"] != expectedTp {
			t.Fatalf("unexpected tpLimitPrice: %v", params["tpLimitPrice"])
		}
		if params["slLimitPrice"] != expectedSl {
			t.Fatalf("unexpected slLimitPrice: %v", params["slLimitPrice"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"orderId":"order-tpsl-limit","orderLinkId":""},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	if _, err := exg.CreateOrder("BTC/USDT", banexg.OdTypeLimit, banexg.OdSideBuy, 1, 100, map[string]interface{}{
		banexg.ParamTakeProfitPrice: 130.0,
		banexg.ParamStopLossPrice:   110.0,
		"tpLimitPrice":              tpLimit,
		"slLimitPrice":              slLimit,
		"tpOrderType":               "Limit",
		"slOrderType":               "Limit",
	}); err != nil {
		t.Fatalf("CreateOrder tp/sl limit price failed: %v", err)
	}
}

func TestCreateOrderLinearExtraParams(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	market := ensureBybitMarketPrecision(exg, "BTC/USDT:USDT")
	tpLimit := 120.1
	slLimit := 90.2
	expectedTp := bybitPrecPriceStrMust(t, exg, market, tpLimit)
	expectedSl := bybitPrecPriceStrMust(t, exg, market, slLimit)
	setBybitTestRequestWithEndpoint(t, MethodPrivatePostV5OrderCreate, func(params map[string]interface{}) *banexg.HttpRes {
		if params["triggerDirection"] != 1 {
			t.Fatalf("unexpected triggerDirection: %v", params["triggerDirection"])
		}
		if params["triggerBy"] != "MarkPrice" {
			t.Fatalf("unexpected triggerBy: %v", params["triggerBy"])
		}
		if params["tpTriggerBy"] != "IndexPrice" {
			t.Fatalf("unexpected tpTriggerBy: %v", params["tpTriggerBy"])
		}
		if params["slTriggerBy"] != "LastPrice" {
			t.Fatalf("unexpected slTriggerBy: %v", params["slTriggerBy"])
		}
		if params["tpslMode"] != "Partial" {
			t.Fatalf("unexpected tpslMode: %v", params["tpslMode"])
		}
		if params["tpOrderType"] != "Limit" || params["slOrderType"] != "Limit" {
			t.Fatalf("unexpected tp/sl order types: %#v", params)
		}
		if params["tpLimitPrice"] != expectedTp {
			t.Fatalf("unexpected tpLimitPrice: %v", params["tpLimitPrice"])
		}
		if params["slLimitPrice"] != expectedSl {
			t.Fatalf("unexpected slLimitPrice: %v", params["slLimitPrice"])
		}
		if params["closeOnTrigger"] != true {
			t.Fatalf("expected closeOnTrigger=true, got %v", params["closeOnTrigger"])
		}
		if params["bboSideType"] != "Queue" || params["bboLevel"] != "2" {
			t.Fatalf("unexpected bbo params: %#v", params)
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"orderId":"order-extra-1","orderLinkId":""},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	if _, err := exg.CreateOrder("BTC/USDT:USDT", banexg.OdTypeLimit, banexg.OdSideBuy, 1, 100, map[string]interface{}{
		banexg.ParamTakeProfitPrice: 130.0,
		banexg.ParamStopLossPrice:   110.0,
		"triggerDirection":          1,
		"triggerBy":                 "MarkPrice",
		"tpTriggerBy":               "IndexPrice",
		"slTriggerBy":               "LastPrice",
		"tpslMode":                  "Partial",
		"tpOrderType":               "Limit",
		"slOrderType":               "Limit",
		"tpLimitPrice":              tpLimit,
		"slLimitPrice":              slLimit,
		"closeOnTrigger":            true,
		"bboSideType":               "Queue",
		"bboLevel":                  "2",
	}); err != nil {
		t.Fatalf("CreateOrder extra params failed: %v", err)
	}
}

func TestCreateOrderRejectsSpotTriggerDirection(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	ensureBybitMarketPrecision(exg, "BTC/USDT")
	_, err := exg.CreateOrder("BTC/USDT", banexg.OdTypeLimit, banexg.OdSideBuy, 1, 100, map[string]interface{}{
		"triggerDirection": 1,
	})
	if err == nil || err.Code != errs.CodeParamInvalid {
		t.Fatalf("expected triggerDirection invalid error, got %v", err)
	}
}

func TestCreateOrderRejectsOrderFilterOnLinear(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	ensureBybitMarketPrecision(exg, "BTC/USDT:USDT")
	_, err := exg.CreateOrder("BTC/USDT:USDT", banexg.OdTypeLimit, banexg.OdSideBuy, 1, 100, map[string]interface{}{
		"orderFilter": "StopOrder",
	})
	if err == nil || err.Code != errs.CodeParamInvalid {
		t.Fatalf("expected orderFilter invalid error, got %v", err)
	}
}

func TestCreateOrderOptionOrderIvAndMmp(t *testing.T) {
	exg := newBybitWithMarket("BTC-30DEC22-18000-C", "BTC/USDT:USDT-30DEC22-18000-C", banexg.MarketOption)
	ensureBybitMarketPrecision(exg, "BTC/USDT:USDT-30DEC22-18000-C")
	setBybitTestRequestWithEndpoint(t, MethodPrivatePostV5OrderCreate, func(params map[string]interface{}) *banexg.HttpRes {
		if params["orderIv"] != "0.1" {
			t.Fatalf("unexpected orderIv: %v", params["orderIv"])
		}
		if params["mmp"] != true {
			t.Fatalf("expected mmp=true, got %v", params["mmp"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"orderId":"order-opt-2","orderLinkId":"link-opt-2"},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	if _, err := exg.CreateOrder("BTC/USDT:USDT-30DEC22-18000-C", banexg.OdTypeLimit, banexg.OdSideBuy, 1, 100, map[string]interface{}{
		banexg.ParamClientOrderId: "link-opt-2",
		"orderIv":                 0.1,
		"mmp":                     true,
	}); err != nil {
		t.Fatalf("CreateOrder option orderIv/mmp failed: %v", err)
	}
}

func TestCreateOrderOptionRejectsTpSlParams(t *testing.T) {
	exg := newBybitWithMarket("BTC-30DEC22-18000-C", "BTC/USDT:USDT-30DEC22-18000-C", banexg.MarketOption)
	ensureBybitMarketPrecision(exg, "BTC/USDT:USDT-30DEC22-18000-C")
	_, err := exg.CreateOrder("BTC/USDT:USDT-30DEC22-18000-C", banexg.OdTypeLimit, banexg.OdSideBuy, 1, 100, map[string]interface{}{
		banexg.ParamClientOrderId: "link-opt-3",
		"tpLimitPrice":            120.0,
	})
	if err == nil || err.Code != errs.CodeParamInvalid {
		t.Fatalf("expected option tp/sl param error, got %v", err)
	}
}

func TestCreateOrderLimitRejectsSlippageTolerance(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	ensureBybitMarketPrecision(exg, "BTC/USDT")
	_, err := exg.CreateOrder("BTC/USDT", banexg.OdTypeLimit, banexg.OdSideBuy, 1, 100, map[string]interface{}{
		"slippageToleranceType": "Percent",
		"slippageTolerance":     0.5,
	})
	if err == nil || err.Code != errs.CodeParamInvalid {
		t.Fatalf("expected slippageTolerance invalid error, got %v", err)
	}
}

func TestCreateOrderMarketSlippageTolerance(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	ensureBybitMarketPrecision(exg, "BTC/USDT")
	setBybitTestRequestWithEndpoint(t, MethodPrivatePostV5OrderCreate, func(params map[string]interface{}) *banexg.HttpRes {
		if params["slippageToleranceType"] != "Percent" {
			t.Fatalf("unexpected slippageToleranceType: %v", params["slippageToleranceType"])
		}
		if params["slippageTolerance"] != "0.5" {
			t.Fatalf("unexpected slippageTolerance: %v", params["slippageTolerance"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"orderId":"order-slip-1","orderLinkId":""},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	if _, err := exg.CreateOrder("BTC/USDT", banexg.OdTypeMarket, banexg.OdSideBuy, 0, 0, map[string]interface{}{
		banexg.ParamCost:        10.0,
		"slippageTolerance":     0.5,
		"slippageToleranceType": "Percent",
	}); err != nil {
		t.Fatalf("CreateOrder slippage tolerance failed: %v", err)
	}
}

func TestCreateOrderTrailingStopParams(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	market := ensureBybitMarketPrecision(exg, "BTC/USDT:USDT")
	trailing := 12.3
	active := 100.1
	expectedTrail := bybitPrecPriceStrMust(t, exg, market, trailing)
	expectedActive := bybitPrecPriceStrMust(t, exg, market, active)
	setBybitTestRequestWithEndpoint(t, MethodPrivatePostV5PositionTradingStop, func(params map[string]interface{}) *banexg.HttpRes {
		if params["trailingStop"] != expectedTrail {
			t.Fatalf("unexpected trailingStop: %v", params["trailingStop"])
		}
		if params["activePrice"] != expectedActive {
			t.Fatalf("unexpected activePrice: %v", params["activePrice"])
		}
		if params["tpslMode"] != "Full" {
			t.Fatalf("expected tpslMode Full, got %v", params["tpslMode"])
		}
		// ParamPositionSide is mapped to positionIdx (1=long, 2=short). Default is 0 (one-way) when not provided.
		if params["positionIdx"] != 1 {
			t.Fatalf("expected positionIdx=1 (positionSide long), got %v", params["positionIdx"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	// positionSide should map to positionIdx=1 (hedge long).
	if _, err := exg.CreateOrder("BTC/USDT:USDT", banexg.OdTypeTrailingStopMarket, banexg.OdSideBuy, 0, 0, map[string]interface{}{
		banexg.ParamTrailingDelta:   trailing,
		banexg.ParamActivationPrice: active,
		banexg.ParamPositionSide:    banexg.PosSideLong,
	}); err != nil {
		t.Fatalf("CreateOrder trailing stop failed: %v", err)
	}
}

func TestCreateOrderTrailingStopRequiresDelta(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	ensureBybitMarketPrecision(exg, "BTC/USDT:USDT")
	_, err := exg.CreateOrder("BTC/USDT:USDT", banexg.OdTypeTrailingStopMarket, banexg.OdSideBuy, 0, 0, map[string]interface{}{})
	if err == nil || err.Code != errs.CodeParamRequired {
		t.Fatalf("expected trailingDelta required error, got %v", err)
	}
}

func TestCreateOrderTrailingStopRejectsCallbackRate(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	ensureBybitMarketPrecision(exg, "BTC/USDT:USDT")
	_, err := exg.CreateOrder("BTC/USDT:USDT", banexg.OdTypeTrailingStopMarket, banexg.OdSideBuy, 0, 0, map[string]interface{}{
		banexg.ParamTrailingDelta: 10.0,
		banexg.ParamCallbackRate:  0.1,
	})
	if err == nil || err.Code != errs.CodeParamInvalid {
		t.Fatalf("expected callbackRate invalid error, got %v", err)
	}
}

func TestCreateOrderTrailingParamsRequireTrailingType(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	ensureBybitMarketPrecision(exg, "BTC/USDT:USDT")
	_, err := exg.CreateOrder("BTC/USDT:USDT", banexg.OdTypeLimit, banexg.OdSideBuy, 1, 100, map[string]interface{}{
		banexg.ParamTrailingDelta: 10.0,
	})
	if err == nil || err.Code != errs.CodeParamInvalid {
		t.Fatalf("expected trailing params invalid error, got %v", err)
	}
}

func TestCreateOrderReduceOnlyRejectsTpsl(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	ensureBybitMarketPrecision(exg, "BTC/USDT:USDT")
	_, err := exg.CreateOrder("BTC/USDT:USDT", banexg.OdTypeLimit, banexg.OdSideSell, 1, 100, map[string]interface{}{
		banexg.ParamReduceOnly:    true,
		banexg.ParamStopLossPrice: 95.0,
	})
	if err == nil || err.Code != errs.CodeParamInvalid {
		t.Fatalf("expected reduceOnly with tpsl error, got %v", err)
	}
}

func TestCreateOrderPostOnlyTimeInForce(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	ensureBybitMarketPrecision(exg, "BTC/USDT")
	setBybitTestRequestWithEndpoint(t, MethodPrivatePostV5OrderCreate, func(params map[string]interface{}) *banexg.HttpRes {
		if params["orderType"] != "Limit" {
			t.Fatalf("unexpected orderType: %v", params["orderType"])
		}
		if params["timeInForce"] != "PostOnly" {
			t.Fatalf("expected PostOnly, got %v", params["timeInForce"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"orderId":"order-5","orderLinkId":""},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	if _, err := exg.CreateOrder("BTC/USDT", banexg.OdTypeLimit, banexg.OdSideSell, 1, 100, map[string]interface{}{
		banexg.ParamPostOnly:    true,
		banexg.ParamTimeInForce: banexg.TimeInForceGTC,
	}); err != nil {
		t.Fatalf("CreateOrder postOnly failed: %v", err)
	}
}

func TestCreateOrderSpotMarginModeSetsIsLeverage(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	ensureBybitMarketPrecision(exg, "BTC/USDT")
	setBybitTestRequestWithEndpoint(t, MethodPrivatePostV5OrderCreate, func(params map[string]interface{}) *banexg.HttpRes {
		if params["isLeverage"] != 1 {
			t.Fatalf("expected isLeverage=1, got %v", params["isLeverage"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"orderId":"order-margin","orderLinkId":""},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	if _, err := exg.CreateOrder("BTC/USDT", banexg.OdTypeLimit, banexg.OdSideBuy, 1, 100, map[string]interface{}{
		banexg.ParamMarginMode: "cross",
	}); err != nil {
		t.Fatalf("CreateOrder margin mode failed: %v", err)
	}
}

func TestCreateOrderPositionIdxReduceOnly(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	ensureBybitMarketPrecision(exg, "BTC/USDT:USDT")
	// Test 1: Default positionIdx=0 for one-way mode (bybit default) when user does not provide positionSide/positionIdx.
	setBybitTestRequestWithEndpoint(t, MethodPrivatePostV5OrderCreate, func(params map[string]interface{}) *banexg.HttpRes {
		if params["positionIdx"] != 0 {
			t.Fatalf("expected positionIdx=0 (one-way mode default), got %v", params["positionIdx"])
		}
		if params["reduceOnly"] != true {
			t.Fatalf("expected reduceOnly=true, got %v", params["reduceOnly"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"orderId":"order-6","orderLinkId":""},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	if _, err := exg.CreateOrder("BTC/USDT:USDT", banexg.OdTypeLimit, banexg.OdSideBuy, 1, 100, map[string]interface{}{
		banexg.ParamReduceOnly: true,
	}); err != nil {
		t.Fatalf("CreateOrder positionIdx/reduceOnly failed: %v", err)
	}

	// Test 2: positionSide should map to positionIdx for hedge mode.
	setBybitTestRequestWithEndpoint(t, MethodPrivatePostV5OrderCreate, func(params map[string]interface{}) *banexg.HttpRes {
		if params["positionIdx"] != 1 {
			t.Fatalf("expected positionIdx=1 (positionSide long), got %v", params["positionIdx"])
		}
		if params["reduceOnly"] != true {
			t.Fatalf("expected reduceOnly=true, got %v", params["reduceOnly"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"orderId":"order-6-2","orderLinkId":""},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	if _, err := exg.CreateOrder("BTC/USDT:USDT", banexg.OdTypeLimit, banexg.OdSideBuy, 1, 100, map[string]interface{}{
		banexg.ParamPositionSide: banexg.PosSideLong,
		banexg.ParamReduceOnly:   true,
	}); err != nil {
		t.Fatalf("CreateOrder positionSide mapping failed: %v", err)
	}

	// Test 3: Explicit positionIdx for hedge mode.
	setBybitTestRequestWithEndpoint(t, MethodPrivatePostV5OrderCreate, func(params map[string]interface{}) *banexg.HttpRes {
		if params["positionIdx"] != 1 {
			t.Fatalf("expected explicit positionIdx=1, got %v", params["positionIdx"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"orderId":"order-7","orderLinkId":""},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	if _, err := exg.CreateOrder("BTC/USDT:USDT", banexg.OdTypeLimit, banexg.OdSideBuy, 1, 100, map[string]interface{}{
		"positionIdx":          1, // Explicit for hedge mode
		banexg.ParamReduceOnly: true,
	}); err != nil {
		t.Fatalf("CreateOrder explicit positionIdx failed: %v", err)
	}
}

func TestCreateOrderSpotMarketByAmount(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	market := ensureBybitMarketPrecision(exg, "BTC/USDT")
	amount := 0.1234
	expectedQty := bybitPrecAmountStrMust(t, exg, market, amount)
	setBybitTestRequestWithEndpoint(t, MethodPrivatePostV5OrderCreate, func(params map[string]interface{}) *banexg.HttpRes {
		if params["orderType"] != "Market" {
			t.Fatalf("unexpected orderType: %v", params["orderType"])
		}
		if params["qty"] != expectedQty {
			t.Fatalf("unexpected qty: %v", params["qty"])
		}
		if params["marketUnit"] != "baseCoin" {
			t.Fatalf("expected marketUnit=baseCoin, got %v", params["marketUnit"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"orderId":"order-2","orderLinkId":"link-2"},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	if _, err := exg.CreateOrder("BTC/USDT", banexg.OdTypeMarket, banexg.OdSideBuy, amount, 0, nil); err != nil {
		t.Fatalf("CreateOrder failed: %v", err)
	}
}

func TestCreateOrderMarketSpotRequiresAmountOrCost(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	ensureBybitMarketPrecision(exg, "BTC/USDT")
	_, err := exg.CreateOrder("BTC/USDT", banexg.OdTypeMarket, banexg.OdSideBuy, 0, 0, nil)
	if err == nil || err.Code != errs.CodeParamRequired {
		t.Fatalf("expected amount or cost required error, got %v", err)
	}
}

func TestCreateOrderMarketSpotPostOnlyInvalid(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	ensureBybitMarketPrecision(exg, "BTC/USDT")
	_, err := exg.CreateOrder("BTC/USDT", banexg.OdTypeMarket, banexg.OdSideBuy, 1, 0, map[string]interface{}{
		banexg.ParamPostOnly: true,
	})
	if err == nil || err.Code != errs.CodeParamInvalid {
		t.Fatalf("expected postOnly invalid error, got %v", err)
	}
}

func TestCreateOrderClosePosition(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	ensureBybitMarketPrecision(exg, "BTC/USDT:USDT")
	setBybitTestRequestWithEndpoint(t, MethodPrivatePostV5OrderCreate, func(params map[string]interface{}) *banexg.HttpRes {
		if params["category"] != banexg.MarketLinear {
			t.Fatalf("unexpected category: %v", params["category"])
		}
		if params["qty"] != "0" {
			t.Fatalf("expected qty=0, got %v", params["qty"])
		}
		if params["reduceOnly"] != true || params["closeOnTrigger"] != true {
			t.Fatalf("expected reduceOnly/closeOnTrigger true, got %#v", params)
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"orderId":"order-3","orderLinkId":""},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	if _, err := exg.CreateOrder("BTC/USDT:USDT", banexg.OdTypeMarket, banexg.OdSideSell, 0, 0, map[string]interface{}{
		banexg.ParamClosePosition: true,
	}); err != nil {
		t.Fatalf("CreateOrder closePosition failed: %v", err)
	}
}

func TestCreateOrderClosePositionInvalidMarket(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	ensureBybitMarketPrecision(exg, "BTC/USDT")
	_, err := exg.CreateOrder("BTC/USDT", banexg.OdTypeMarket, banexg.OdSideSell, 0, 0, map[string]interface{}{
		banexg.ParamClosePosition: true,
	})
	if err == nil || err.Code != errs.CodeParamInvalid {
		t.Fatalf("expected closePosition invalid error, got %v", err)
	}
}

func TestCreateOrderSpotStopUsesOrderFilter(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	ensureBybitMarketPrecision(exg, "BTC/USDT")
	setBybitTestRequestWithEndpoint(t, MethodPrivatePostV5OrderCreate, func(params map[string]interface{}) *banexg.HttpRes {
		if params["orderFilter"] != "StopOrder" {
			t.Fatalf("expected orderFilter StopOrder, got %v", params["orderFilter"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"orderId":"order-stop","orderLinkId":""},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	if _, err := exg.CreateOrder("BTC/USDT", banexg.OdTypeStopLoss, banexg.OdSideSell, 1, 0, map[string]interface{}{
		banexg.ParamStopLossPrice: float64(100),
	}); err != nil {
		t.Fatalf("CreateOrder stop order failed: %v", err)
	}
}

func TestCreateOrderSpotTriggerPriceUsesOrderFilter(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	ensureBybitMarketPrecision(exg, "BTC/USDT")
	setBybitTestRequestWithEndpoint(t, MethodPrivatePostV5OrderCreate, func(params map[string]interface{}) *banexg.HttpRes {
		if params["orderFilter"] != "StopOrder" {
			t.Fatalf("expected orderFilter StopOrder, got %v", params["orderFilter"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"orderId":"order-trigger","orderLinkId":""},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	if _, err := exg.CreateOrder("BTC/USDT", banexg.OdTypeLimit, banexg.OdSideBuy, 1, 101, map[string]interface{}{
		banexg.ParamTriggerPrice: float64(100),
	}); err != nil {
		t.Fatalf("CreateOrder triggerPrice order failed: %v", err)
	}
}

func TestFetchOrderFallbackHistory(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	setBybitTestRequest(t, func(_ context.Context, endpoint string, params map[string]interface{}, _ int, _ bool, _ bool) *banexg.HttpRes {
		switch endpoint {
		case MethodPrivateGetV5OrderRealtime:
			content := `{"retCode":0,"retMsg":"OK","result":{"list":[],"nextPageCursor":""},"retExtInfo":{},"time":1700000000000}`
			return &banexg.HttpRes{Status: 200, Content: content}
		case MethodPrivateGetV5OrderHistory:
			content := `{"retCode":0,"retMsg":"OK","result":{"list":[{"orderId":"order-1","orderLinkId":"link-1","symbol":"BTCUSDT","side":"Buy","orderType":"Limit","orderStatus":"Filled","timeInForce":"GTC","price":"100","qty":"1","cumExecQty":"1","cumExecValue":"100","avgPrice":"100","createdTime":"1700000000000","updatedTime":"1700000001000"}],"nextPageCursor":""},"retExtInfo":{},"time":1700000000000}`
			return &banexg.HttpRes{Status: 200, Content: content}
		default:
			t.Fatalf("unexpected endpoint: %s", endpoint)
		}
		return nil
	})
	order, err := exg.FetchOrder("BTC/USDT:USDT", "order-1", map[string]interface{}{})
	if err != nil {
		t.Fatalf("FetchOrder failed: %v", err)
	}
	if order.ID != "order-1" {
		t.Fatalf("unexpected order ID: %s", order.ID)
	}
	if order.Status != banexg.OdStatusFilled {
		t.Fatalf("unexpected order status: %s", order.Status)
	}
}

func TestCreateOrderRequiresPriceForLimit(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	ensureBybitMarketPrecision(exg, "BTC/USDT")
	_, err := exg.CreateOrder("BTC/USDT", banexg.OdTypeLimit, banexg.OdSideBuy, 1, 0, nil)
	if err == nil || err.Code != errs.CodeParamRequired {
		t.Fatalf("expected price required error, got %v", err)
	}
}

func TestSetBybitOrderID(t *testing.T) {
	args := map[string]interface{}{banexg.ParamClientOrderId: "link-1"}
	if err := setBybitOrderID(args, ""); err != nil {
		t.Fatalf("setBybitOrderID failed: %v", err)
	}
	if args["orderLinkId"] != "link-1" {
		t.Fatalf("expected orderLinkId link-1, got %v", args["orderLinkId"])
	}
	if _, ok := args[banexg.ParamClientOrderId]; ok {
		t.Fatal("expected ParamClientOrderId to be removed")
	}

	args = map[string]interface{}{}
	if err := setBybitOrderID(args, "order-1"); err != nil {
		t.Fatalf("setBybitOrderID with orderId failed: %v", err)
	}
	if args["orderId"] != "order-1" {
		t.Fatalf("expected orderId order-1, got %v", args["orderId"])
	}

	if err := setBybitOrderID(map[string]interface{}{}, ""); err == nil || err.Code != errs.CodeParamRequired {
		t.Fatalf("expected order id required error, got %v", err)
	}
}

func TestApplyBybitClientOrderID(t *testing.T) {
	args := map[string]interface{}{banexg.ParamClientOrderId: "link-1"}
	applyBybitClientOrderID(args)
	if args["orderLinkId"] != "link-1" {
		t.Fatalf("expected orderLinkId link-1, got %v", args["orderLinkId"])
	}
	if _, ok := args[banexg.ParamClientOrderId]; ok {
		t.Fatal("expected ParamClientOrderId to be removed")
	}

	args = map[string]interface{}{
		banexg.ParamClientOrderId: "link-2",
		"orderLinkId":             "keep-1",
	}
	applyBybitClientOrderID(args)
	if args["orderLinkId"] != "keep-1" {
		t.Fatalf("expected orderLinkId keep-1, got %v", args["orderLinkId"])
	}
	if _, ok := args[banexg.ParamClientOrderId]; ok {
		t.Fatal("expected ParamClientOrderId to be removed when orderLinkId exists")
	}
}

func TestApplyBybitSmpType(t *testing.T) {
	args := map[string]interface{}{
		banexg.ParamSelfTradePreventionMode: "CancelMaker",
	}
	applyBybitSmpType(args)
	if args["smpType"] != "CancelMaker" {
		t.Fatalf("expected smpType CancelMaker, got %v", args["smpType"])
	}
	if _, ok := args[banexg.ParamSelfTradePreventionMode]; ok {
		t.Fatal("expected ParamSelfTradePreventionMode to be removed")
	}

	args = map[string]interface{}{
		banexg.ParamSelfTradePreventionMode: "CancelMaker",
		"smpType":                           "CancelTaker",
	}
	applyBybitSmpType(args)
	if args["smpType"] != "CancelTaker" {
		t.Fatalf("expected smpType to stay CancelTaker, got %v", args["smpType"])
	}
	if _, ok := args[banexg.ParamSelfTradePreventionMode]; ok {
		t.Fatal("expected ParamSelfTradePreventionMode to be removed when smpType exists")
	}
}

func TestHasBybitTpslArgs(t *testing.T) {
	if hasAnyBybitArgs(map[string]interface{}{}, bybitTpslKeys...) {
		t.Fatal("expected false for empty args")
	}
	if !hasAnyBybitArgs(map[string]interface{}{"takeProfit": "1"}, bybitTpslKeys...) {
		t.Fatal("expected true for takeProfit")
	}
	if !hasAnyBybitArgs(map[string]interface{}{"slOrderType": "Limit"}, bybitTpslKeys...) {
		t.Fatal("expected true for slOrderType")
	}
}

func TestRejectBybitBefore(t *testing.T) {
	args := map[string]interface{}{banexg.ParamBefore: int64(123)}
	err := rejectBybitBefore(args)
	if err == nil || err.Code != errs.CodeNotSupport {
		t.Fatalf("expected not support error, got %v", err)
	}
	if _, ok := args[banexg.ParamBefore]; ok {
		t.Fatal("expected ParamBefore to be removed")
	}
	if err := rejectBybitBefore(map[string]interface{}{}); err != nil {
		t.Fatalf("unexpected error for empty args: %v", err)
	}
}

func TestApplyBybitTimeRange(t *testing.T) {
	args := map[string]interface{}{banexg.ParamUntil: int64(2000)}
	applyBybitTimeRange(args, 1000)
	if args["startTime"] != int64(1000) {
		t.Fatalf("expected startTime 1000, got %v", args["startTime"])
	}
	if args["endTime"] != int64(2000) {
		t.Fatalf("expected endTime 2000, got %v", args["endTime"])
	}
	if _, ok := args[banexg.ParamUntil]; ok {
		t.Fatal("expected ParamUntil to be removed")
	}
}

func TestValidateBybitTimeWindow(t *testing.T) {
	args := map[string]interface{}{
		"startTime": int64(1),
		"endTime":   bybitHistoryWindowMS + 2,
	}
	err := validateBybitTimeWindow(args)
	if err == nil || err.Code != errs.CodeParamInvalid {
		t.Fatalf("expected time window invalid error, got %v", err)
	}
	if err := validateBybitTimeWindow(map[string]interface{}{
		"startTime": int64(0),
		"endTime":   bybitHistoryWindowMS,
	}); err != nil {
		t.Fatalf("expected time window ok, got %v", err)
	}
}

func TestCreateOrderOptionRequiresOrderLinkId(t *testing.T) {
	exg := newBybitWithMarket("BTC-30DEC22-18000-C", "BTC/USDT:USDT-30DEC22-18000-C", banexg.MarketOption)
	ensureBybitMarketPrecision(exg, "BTC/USDT:USDT-30DEC22-18000-C")
	_, err := exg.CreateOrder("BTC/USDT:USDT-30DEC22-18000-C", banexg.OdTypeLimit, banexg.OdSideBuy, 1, 100, nil)
	if err == nil || err.Code != errs.CodeParamRequired {
		t.Fatalf("expected orderLinkId required error, got %v", err)
	}
}

func TestCreateOrderOptionUsesClientOrderID(t *testing.T) {
	exg := newBybitWithMarket("BTC-30DEC22-18000-C", "BTC/USDT:USDT-30DEC22-18000-C", banexg.MarketOption)
	ensureBybitMarketPrecision(exg, "BTC/USDT:USDT-30DEC22-18000-C")
	setBybitTestRequestWithEndpoint(t, MethodPrivatePostV5OrderCreate, func(params map[string]interface{}) *banexg.HttpRes {
		if params["orderLinkId"] != "link-opt-1" {
			t.Fatalf("expected orderLinkId link-opt-1, got %v", params["orderLinkId"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"orderId":"order-opt-1","orderLinkId":"link-opt-1"},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	if _, err := exg.CreateOrder("BTC/USDT:USDT-30DEC22-18000-C", banexg.OdTypeLimit, banexg.OdSideBuy, 1, 100, map[string]interface{}{
		banexg.ParamClientOrderId: "link-opt-1",
	}); err != nil {
		t.Fatalf("CreateOrder option clientOrderId failed: %v", err)
	}
}

func TestCancelOrderByClientOrderID(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	setBybitTestRequestWithEndpoint(t, MethodPrivatePostV5OrderCancel, func(params map[string]interface{}) *banexg.HttpRes {
		if params["category"] != banexg.MarketSpot || params["symbol"] != "BTCUSDT" {
			t.Fatalf("unexpected category/symbol: %#v", params)
		}
		if params["orderLinkId"] != "link-1" {
			t.Fatalf("unexpected orderLinkId: %v", params["orderLinkId"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"orderId":"order-1","orderLinkId":"link-1"},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	if _, err := exg.CancelOrder("", "BTC/USDT", map[string]interface{}{
		banexg.ParamClientOrderId: "link-1",
	}); err != nil {
		t.Fatalf("CancelOrder failed: %v", err)
	}
}

func TestFetchOpenOrdersLinearRequiresSymbol(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	_, err := exg.FetchOpenOrders("", 0, 10, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
	if err == nil || err.Code != errs.CodeParamRequired {
		t.Fatalf("expected missing symbol/base/settle error, got %v", err)
	}
}

func TestFetchOpenOrdersLinearWithSettleCoin(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	setBybitTestRequestWithEndpoint(t, MethodPrivateGetV5OrderRealtime, func(params map[string]interface{}) *banexg.HttpRes {
		if params["category"] != banexg.MarketLinear {
			t.Fatalf("unexpected category: %#v", params["category"])
		}
		if params["settleCoin"] != "USDT" {
			t.Fatalf("unexpected settleCoin: %#v", params["settleCoin"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"list":[],"nextPageCursor":""},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	orders, err := exg.FetchOpenOrders("", 0, 10, map[string]interface{}{
		banexg.ParamMarket:      banexg.MarketLinear,
		banexg.ParamSettleCoins: []string{"USDT"},
	})
	if err != nil {
		t.Fatalf("FetchOpenOrders failed: %v", err)
	}
	if len(orders) != 0 {
		t.Fatalf("expected empty orders, got %#v", orders)
	}
}

func TestFetchOpenOrdersParams(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	setBybitTestRequestWithEndpoint(t, MethodPrivateGetV5OrderRealtime, func(params map[string]interface{}) *banexg.HttpRes {
		if params["category"] != banexg.MarketLinear || params["symbol"] != "BTCUSDT" {
			t.Fatalf("unexpected category/symbol: %#v", params)
		}
		if params["orderLinkId"] != "link-1" {
			t.Fatalf("unexpected orderLinkId: %v", params["orderLinkId"])
		}
		if params["limit"] != 50 {
			t.Fatalf("expected limit 50, got %v", params["limit"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"list":[{"orderId":"order-1","orderLinkId":"link-1","symbol":"BTCUSDT","side":"Buy","orderType":"Limit","orderStatus":"New","timeInForce":"GTC","price":"100","qty":"1","cumExecQty":"0","leavesQty":"1","createdTime":"1700000000000","updatedTime":"1700000001000"}],"nextPageCursor":""},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	orders, err := exg.FetchOpenOrders("BTC/USDT:USDT", 0, 60, map[string]interface{}{
		banexg.ParamClientOrderId: "link-1",
	})
	if err != nil {
		t.Fatalf("FetchOpenOrders failed: %v", err)
	}
	if len(orders) != 1 || orders[0].ID != "order-1" {
		t.Fatalf("unexpected orders: %#v", orders)
	}
}

func TestFetchOrdersParams(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	setBybitTestRequestWithEndpoint(t, MethodPrivateGetV5OrderHistory, func(params map[string]interface{}) *banexg.HttpRes {
		if params["startTime"] != int64(1000) || params["endTime"] != int64(2000) {
			t.Fatalf("unexpected time range: %#v", params)
		}
		if params["orderLinkId"] != "link-1" {
			t.Fatalf("unexpected orderLinkId: %v", params["orderLinkId"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"list":[{"orderId":"order-2","orderLinkId":"link-1","symbol":"BTCUSDT","side":"Buy","orderType":"Limit","orderStatus":"Filled","timeInForce":"GTC","price":"100","qty":"1","cumExecQty":"1","leavesQty":"0","createdTime":"1700000000000","updatedTime":"1700000001000"}],"nextPageCursor":""},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	orders, err := exg.FetchOrders("BTC/USDT:USDT", 1000, 10, map[string]interface{}{
		banexg.ParamClientOrderId: "link-1",
		banexg.ParamUntil:         int64(2000),
	})
	if err != nil {
		t.Fatalf("FetchOrders failed: %v", err)
	}
	if len(orders) != 1 || orders[0].ID != "order-2" {
		t.Fatalf("unexpected orders: %#v", orders)
	}
}

func TestFetchOrdersUsesParamAfterCursor(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	setBybitTestRequestWithEndpoint(t, MethodPrivateGetV5OrderHistory, func(params map[string]interface{}) *banexg.HttpRes {
		if params["cursor"] != "cur-1" {
			t.Fatalf("expected cursor=cur-1, got %v", params["cursor"])
		}
		if _, ok := params[banexg.ParamAfter]; ok {
			t.Fatalf("expected ParamAfter to be removed, got %v", params[banexg.ParamAfter])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"list":[],"nextPageCursor":""},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	orders, err := exg.FetchOrders("BTC/USDT:USDT", 0, 10, map[string]interface{}{
		banexg.ParamAfter: "cur-1",
	})
	if err != nil {
		t.Fatalf("FetchOrders cursor failed: %v", err)
	}
	if len(orders) != 0 {
		t.Fatalf("expected empty orders, got %#v", orders)
	}
}

func TestFetchMyTradesParams(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	setBybitTestRequestWithEndpoint(t, MethodPrivateGetV5ExecutionList, func(params map[string]interface{}) *banexg.HttpRes {
		if params["startTime"] != int64(1000) || params["endTime"] != int64(2000) {
			t.Fatalf("unexpected time range: %#v", params)
		}
		if params["orderLinkId"] != "link-1" {
			t.Fatalf("unexpected orderLinkId: %v", params["orderLinkId"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"list":[{"symbol":"BTCUSDT","orderId":"order-3","orderLinkId":"link-1","side":"Buy","orderType":"Limit","orderQty":"1","leavesQty":"0.4","execId":"exec-1","execPrice":"100","execQty":"0.6","execFee":"0.001","feeCurrency":"USDT","execType":"Trade","execTime":"1700000002000","isMaker":true}],"nextPageCursor":""},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	trades, err := exg.FetchMyTrades("BTC/USDT:USDT", 1000, 10, map[string]interface{}{
		banexg.ParamClientOrderId: "link-1",
		banexg.ParamUntil:         int64(2000),
	})
	if err != nil {
		t.Fatalf("FetchMyTrades failed: %v", err)
	}
	if len(trades) != 1 || trades[0].Order != "order-3" {
		t.Fatalf("unexpected trades: %#v", trades)
	}
}

func TestFetchIncomeHistoryParams(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	exg.Markets["BTC/USDT:USDT"].Base = "BTC"
	setBybitTestRequestWithEndpoint(t, MethodPrivateGetV5AccountTransactionLog, func(params map[string]interface{}) *banexg.HttpRes {
		if params["accountType"] != "UNIFIED" {
			t.Fatalf("expected accountType UNIFIED, got %v", params["accountType"])
		}
		if _, ok := params["symbol"]; ok {
			t.Fatalf("unexpected symbol in params: %#v", params["symbol"])
		}
		if params["baseCoin"] != "BTC" {
			t.Fatalf("expected baseCoin BTC, got %v", params["baseCoin"])
		}
		if params["currency"] != "USDT" || params["type"] != "FUNDING" {
			t.Fatalf("unexpected currency/type: %#v", params)
		}
		if params["startTime"] != int64(1000) || params["endTime"] != int64(2000) {
			t.Fatalf("unexpected time range: %#v", params)
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"list":[{"id":"income-1","symbol":"BTCUSDT","transactionTime":"1700000003000","type":"FUNDING","currency":"USDT","cashFlow":"100","funding":"5","fee":"1","change":"0","tradeId":"trade-1"}],"nextPageCursor":""},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	items, err := exg.FetchIncomeHistory("FUNDING", "BTC/USDT:USDT", 1000, 10, map[string]interface{}{
		banexg.ParamCurrency: "USDT",
		banexg.ParamUntil:    int64(2000),
	})
	if err != nil {
		t.Fatalf("FetchIncomeHistory failed: %v", err)
	}
	if len(items) != 1 || items[0].TranID != "income-1" {
		t.Fatalf("unexpected income items: %#v", items)
	}
}

// Tests migrated from api_cancelorder_test.go

type bybitCancelOrderFixture struct {
	Symbol      string
	OrderID     string
	ClientID    string
	OrderFilter string
}

func bybitSetupCancelOrderFixture(t *testing.T, exg *Bybit, marketType, clientPrefix, orderFilter string) bybitCancelOrderFixture {
	t.Helper()

	markets := loadBybitMarketsForType(t, exg, marketType)
	if len(markets) == 0 {
		t.Skipf("no %s markets available", marketType)
	}

	prefer := bybitPreferredSymbolsForMarketType(marketType)
	market := bybitPickMarketPrefer(t, markets, marketType, prefer...)
	if market == nil || market.Symbol == "" {
		t.Skipf("no %s markets available", marketType)
	}
	symbol := market.Symbol

	refPrice := bybitFetchRefPrice(t, exg, symbol)
	qty := bybitCalcTestQty(market, refPrice)

	clientID := bybitTestClientOrderID(clientPrefix)
	args := map[string]interface{}{
		banexg.ParamClientOrderId: clientID,
	}

	odType := banexg.OdTypeLimit
	price := refPrice * 0.95
	if orderFilter != "" {
		trigger := refPrice * 1.05
		args[banexg.ParamTriggerPrice] = trigger
		args["orderFilter"] = orderFilter
		price = trigger * 1.01
	}

	od, err := exg.CreateOrder(symbol, odType, banexg.OdSideBuy, qty, price, args)
	if err != nil {
		bybitSkipOnTradePermission(t, err)
		t.Fatalf("CreateOrder failed: %v", err)
	}
	if od == nil || od.ID == "" {
		t.Fatalf("expected created order id, got: %#v", od)
	}

	t.Cleanup(func() { bybitCancelOrderBestEffort(t, exg, symbol, od.ID) })
	_ = fetchBybitOrderEventually(t, exg, symbol, od.ID)

	return bybitCancelOrderFixture{
		Symbol:      symbol,
		OrderID:     od.ID,
		ClientID:    clientID,
		OrderFilter: orderFilter,
	}
}

func bybitRequireOrderNotOpenEventually(t *testing.T, exg *Bybit, symbol, orderID string) *banexg.Order {
	t.Helper()
	return retryBybitEventually(t, "FetchOrder (post-cancel)", func() (*banexg.Order, error) {
		od, err := exg.FetchOrder(symbol, orderID, nil)
		if err != nil {
			return nil, err
		}
		if od == nil {
			return nil, errors.New("empty order")
		}
		if od.Status == banexg.OdStatusOpen || od.Status == banexg.OdStatusPartFilled {
			return nil, errors.New("order not closed yet")
		}
		return od, nil
	})
}

func TestApi_CancelOrder_Spot_ByOrderID(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fx := bybitSetupCancelOrderFixture(t, exg, banexg.MarketSpot, "cancel-spot-oid", "")

	if _, err := exg.CancelOrder(fx.OrderID, fx.Symbol, nil); err != nil {
		t.Fatalf("CancelOrder by orderId failed: %v", err)
	}
	_ = bybitRequireOrderNotOpenEventually(t, exg, fx.Symbol, fx.OrderID)
}

func TestApi_CancelOrder_Spot_ByClientOrderID(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fx := bybitSetupCancelOrderFixture(t, exg, banexg.MarketSpot, "cancel-spot-cloid", "")

	if _, err := exg.CancelOrder("", fx.Symbol, map[string]interface{}{
		banexg.ParamClientOrderId: fx.ClientID,
	}); err != nil {
		t.Fatalf("CancelOrder by clientOrderId failed: %v", err)
	}
	_ = bybitRequireOrderNotOpenEventually(t, exg, fx.Symbol, fx.OrderID)
}

func TestApi_CancelOrder_Spot_ByOrderIDAndClientOrderID_OrderIDWins(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fx := bybitSetupCancelOrderFixture(t, exg, banexg.MarketSpot, "cancel-spot-both", "")

	if _, err := exg.CancelOrder(fx.OrderID, fx.Symbol, map[string]interface{}{
		banexg.ParamClientOrderId: "non-existent-client-id",
	}); err != nil {
		t.Fatalf("CancelOrder by orderId (with clientOrderId) failed: %v", err)
	}
	_ = bybitRequireOrderNotOpenEventually(t, exg, fx.Symbol, fx.OrderID)
}

func TestApi_CancelOrder_Spot_WithOrderFilter_ByOrderID(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fx := bybitSetupCancelOrderFixture(t, exg, banexg.MarketSpot, "cancel-spot-filter-oid", "tpslOrder")

	if _, err := exg.CancelOrder(fx.OrderID, fx.Symbol, map[string]interface{}{
		"orderFilter": fx.OrderFilter,
	}); err != nil {
		t.Fatalf("CancelOrder with orderFilter by orderId failed: %v", err)
	}
	_ = bybitRequireOrderNotOpenEventually(t, exg, fx.Symbol, fx.OrderID)
}

func TestApi_CancelOrder_Spot_WithOrderFilter_ByClientOrderID(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fx := bybitSetupCancelOrderFixture(t, exg, banexg.MarketSpot, "cancel-spot-filter-cloid", "tpslOrder")

	if _, err := exg.CancelOrder("", fx.Symbol, map[string]interface{}{
		banexg.ParamClientOrderId: fx.ClientID,
		"orderFilter":             fx.OrderFilter,
	}); err != nil {
		t.Fatalf("CancelOrder with orderFilter by clientOrderId failed: %v", err)
	}
	_ = bybitRequireOrderNotOpenEventually(t, exg, fx.Symbol, fx.OrderID)
}

func TestApi_CancelOrder_Linear_ByOrderID(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fx := bybitSetupCancelOrderFixture(t, exg, banexg.MarketLinear, "cancel-linear-oid", "")

	if _, err := exg.CancelOrder(fx.OrderID, fx.Symbol, nil); err != nil {
		t.Fatalf("CancelOrder linear by orderId failed: %v", err)
	}
	_ = bybitRequireOrderNotOpenEventually(t, exg, fx.Symbol, fx.OrderID)
}

func TestApi_CancelOrder_Linear_ByClientOrderID(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fx := bybitSetupCancelOrderFixture(t, exg, banexg.MarketLinear, "cancel-linear-cloid", "")

	if _, err := exg.CancelOrder("", fx.Symbol, map[string]interface{}{
		banexg.ParamClientOrderId: fx.ClientID,
	}); err != nil {
		t.Fatalf("CancelOrder linear by clientOrderId failed: %v", err)
	}
	_ = bybitRequireOrderNotOpenEventually(t, exg, fx.Symbol, fx.OrderID)
}

func TestApi_CancelOrder_Linear_ByOrderIDAndClientOrderID_OrderIDWins(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fx := bybitSetupCancelOrderFixture(t, exg, banexg.MarketLinear, "cancel-linear-both", "")

	if _, err := exg.CancelOrder(fx.OrderID, fx.Symbol, map[string]interface{}{
		banexg.ParamClientOrderId: "non-existent-client-id",
	}); err != nil {
		t.Fatalf("CancelOrder linear by orderId (with clientOrderId) failed: %v", err)
	}
	_ = bybitRequireOrderNotOpenEventually(t, exg, fx.Symbol, fx.OrderID)
}

// Tests migrated from api_fetchorder_test.go

const bybitFetchOrderTestSymbol = "XRP/USDT:USDT"

func ensureBybitMarketsLoaded(t *testing.T, exg *Bybit) {
	t.Helper()
	if exg == nil {
		t.Skip("bybit exchange not initialized")
		return
	}
	if len(exg.Markets) > 0 {
		return
	}
	if _, err := exg.LoadMarkets(false, nil); err != nil {
		t.Fatalf("LoadMarkets failed: %v", err)
	}
}

func bybitTestClientOrderID(prefix string) string {
	ts := time.Now().UnixNano()
	suffix := fmt.Sprintf("-%d", ts)
	maxPrefix := 45 - len(suffix)
	if maxPrefix < 1 {
		maxPrefix = 1
	}
	if len(prefix) > maxPrefix {
		prefix = prefix[:maxPrefix]
	}
	return prefix + suffix
}

func bybitPlacePostOnlyLimitOrder(t *testing.T, exg *Bybit, symbol, clientOrderID string) *banexg.Order {
	t.Helper()
	ensureBybitMarketsLoaded(t, exg)

	ticker, err := exg.FetchTicker(symbol, nil)
	if err != nil {
		t.Fatalf("FetchTicker failed: %v", err)
	}
	basePrice := ticker.Bid
	if basePrice == 0 {
		basePrice = ticker.Last
	}
	if basePrice == 0 {
		basePrice = ticker.Ask
	}
	if basePrice == 0 {
		t.Fatalf("ticker price is empty")
	}

	price := basePrice * 0.98
	amount := 10.0

	args := map[string]interface{}{
		banexg.ParamPostOnly:      true,
		banexg.ParamClientOrderId: clientOrderID,
	}
	order, err := exg.CreateOrder(symbol, banexg.OdTypeLimit, banexg.OdSideBuy, amount, price, args)
	if err != nil {
		t.Fatalf("CreateOrder postOnly limit failed: %v", err)
	}
	if order == nil || order.ID == "" {
		t.Fatalf("expected created order id, got: %#v", order)
	}

	_ = fetchBybitOrderEventually(t, exg, symbol, order.ID)
	return order
}

func bybitCancelOrderBestEffort(t *testing.T, exg *Bybit, symbol, orderID string) {
	t.Helper()
	var lastErr error
	for i := 0; i < bybitOrderRetryCount; i++ {
		_, err := exg.CancelOrder(orderID, symbol, nil)
		if err == nil {
			return
		}
		if err.BizCode == 170213 || err.BizCode == 110001 {
			return
		}
		lastErr = err
		time.Sleep(bybitOrderRetryWait)
	}
	t.Logf("CancelOrder best-effort failed after retries (orderID=%s): %v", orderID, lastErr)
}

func TestApi_FetchOrder_ByOrderID(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	clientID := bybitTestClientOrderID("fetchorder-oid")
	order := bybitPlacePostOnlyLimitOrder(t, exg, bybitFetchOrderTestSymbol, clientID)
	defer bybitCancelOrderBestEffort(t, exg, bybitFetchOrderTestSymbol, order.ID)

	fetched := fetchBybitOrderEventually(t, exg, bybitFetchOrderTestSymbol, order.ID)
	if fetched.ID != order.ID {
		t.Fatalf("expected order id %s, got %s", order.ID, fetched.ID)
	}
}

func TestApi_FetchOrder_ByClientOrderID(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	clientID := bybitTestClientOrderID("fetchorder-cloid")
	order := bybitPlacePostOnlyLimitOrder(t, exg, bybitFetchOrderTestSymbol, clientID)
	defer bybitCancelOrderBestEffort(t, exg, bybitFetchOrderTestSymbol, order.ID)

	fetched := fetchBybitOrderWithParamsEventually(t, exg, bybitFetchOrderTestSymbol, "", map[string]interface{}{
		banexg.ParamClientOrderId: clientID,
	})
	if fetched.ID != order.ID {
		t.Fatalf("expected order id %s, got %s", order.ID, fetched.ID)
	}
	if fetched.ClientOrderID != "" && fetched.ClientOrderID != clientID {
		t.Fatalf("expected clientOrderId %s, got %s", clientID, fetched.ClientOrderID)
	}
}

func TestApi_FetchOrder_ByOrderIDAndClientOrderID_OrderIDWins(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	clientID := bybitTestClientOrderID("fetchorder-both")
	order := bybitPlacePostOnlyLimitOrder(t, exg, bybitFetchOrderTestSymbol, clientID)
	defer bybitCancelOrderBestEffort(t, exg, bybitFetchOrderTestSymbol, order.ID)

	fetched := fetchBybitOrderWithParamsEventually(t, exg, bybitFetchOrderTestSymbol, order.ID, map[string]interface{}{
		banexg.ParamClientOrderId: "non-existent-client-id",
	})
	if fetched.ID != order.ID {
		t.Fatalf("expected order id %s, got %s", order.ID, fetched.ID)
	}
}

func TestApi_FetchOrder_ByOrderID_WithOpenOnlyParam(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	clientID := bybitTestClientOrderID("fetchorder-openonly-oid")
	order := bybitPlacePostOnlyLimitOrder(t, exg, bybitFetchOrderTestSymbol, clientID)

	bybitCancelOrderBestEffort(t, exg, bybitFetchOrderTestSymbol, order.ID)

	fetched := fetchBybitOrderWithParamsEventually(t, exg, bybitFetchOrderTestSymbol, order.ID, map[string]interface{}{
		"openOnly": 0,
	})
	if fetched.ID != order.ID {
		t.Fatalf("expected order id %s, got %s", order.ID, fetched.ID)
	}
}

func TestApi_FetchOrder_ByClientOrderID_WithOpenOnlyParam(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	clientID := bybitTestClientOrderID("fetchorder-openonly-cloid")
	order := bybitPlacePostOnlyLimitOrder(t, exg, bybitFetchOrderTestSymbol, clientID)

	bybitCancelOrderBestEffort(t, exg, bybitFetchOrderTestSymbol, order.ID)

	fetched := fetchBybitOrderWithParamsEventually(t, exg, bybitFetchOrderTestSymbol, "", map[string]interface{}{
		banexg.ParamClientOrderId: clientID,
		"openOnly":                0,
	})
	if fetched.ID != order.ID {
		t.Fatalf("expected order id %s, got %s", order.ID, fetched.ID)
	}
}

// Tests migrated from api_createorder_params_test.go, api_editorder_test.go, etc.

func bybitFetchRefPrice(t *testing.T, exg *Bybit, symbol string) float64 {
	t.Helper()
	ticker, err := exg.FetchTicker(symbol, nil)
	if err != nil {
		t.Fatalf("FetchTicker failed: %v", err)
	}
	price := ticker.Last
	if price == 0 {
		price = ticker.Ask
	}
	if price == 0 {
		price = ticker.Bid
	}
	if price == 0 {
		t.Fatalf("empty ticker price for %s", symbol)
	}
	return price
}

func bybitPickMarketPrefer(t *testing.T, markets banexg.MarketMap, marketType string, preferredSymbols ...string) *banexg.Market {
	t.Helper()
	for _, sym := range preferredSymbols {
		if sym == "" {
			continue
		}
		if m, ok := markets[sym]; ok && m != nil {
			return m
		}
	}
	return pickBybitMarketByType(markets, marketType)
}

func bybitLoadAndPickMarket(t *testing.T, exg *Bybit, marketType string, preferredSymbols ...string) *banexg.Market {
	t.Helper()
	markets := loadBybitMarketsForType(t, exg, marketType)
	market := bybitPickMarketPrefer(t, markets, marketType, preferredSymbols...)
	if market == nil || market.Symbol == "" {
		t.Skipf("no %s markets available", marketType)
	}
	return market
}

func bybitCalcTestQty(market *banexg.Market, refPrice float64) float64 {
	qty := 0.001
	if market != nil && market.Limits != nil && market.Limits.Amount != nil && market.Limits.Amount.Min > 0 {
		qty = market.Limits.Amount.Min
	}
	if market != nil && market.Limits != nil && market.Limits.Cost != nil && market.Limits.Cost.Min > 0 && refPrice > 0 {
		need := market.Limits.Cost.Min / refPrice
		if need > qty {
			qty = need
		}
	}
	// Add buffer to avoid min-size edge cases after precision rounding.
	qty *= 2
	if qty <= 0 {
		qty = 0.001
	}
	return qty
}

func bybitSkipOnTradePermission(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	// For some environments, certain categories (spot/option/margin) or features might be disabled.
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "permission") ||
		strings.Contains(msg, "forbidden") ||
		strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "read-only") ||
		strings.Contains(msg, "api key") {
		t.Skipf("skip due to permission/auth error: %v", err)
	}
}

func bybitRequireFilledOrder(t *testing.T, exg *Bybit, symbol, orderID string) *banexg.Order {
	t.Helper()
	od := fetchBybitOrderEventually(t, exg, symbol, orderID)
	if od.Filled <= 0 {
		t.Fatalf("expected filled amount, got %v", od.Filled)
	}
	return od
}

func bybitSpotSellBack(t *testing.T, exg *Bybit, symbol string, amount float64) {
	t.Helper()
	_, err := exg.CreateOrder(symbol, banexg.OdTypeMarket, banexg.OdSideSell, amount, 0, nil)
	if err != nil {
		t.Fatalf("CreateOrder spot market sell failed: %v", err)
	}
}

func bybitCancelOpenOrdersForSymbol(t *testing.T, exg *Bybit, symbol string) {
	t.Helper()
	orders, err := exg.FetchOpenOrders(symbol, 0, 50, nil)
	if err != nil {
		t.Logf("FetchOpenOrders skipped: %v", err)
		return
	}
	for _, od := range orders {
		if od == nil || od.ID == "" {
			continue
		}
		bybitCancelOrderBestEffort(t, exg, symbol, od.ID)
	}
}

func bybitCleanupLinearOrdersAndPositions(t *testing.T, exg *Bybit, symbol string) {
	t.Helper()
	t.Cleanup(func() {
		bybitCancelOpenOrdersForSymbol(t, exg, symbol)
		bybitCloseLinearPositionBestEffort(t, exg, symbol)
	})
}

func bybitOpenLinearLongAndCleanup(t *testing.T, exg *Bybit, symbol string, qty float64) float64 {
	t.Helper()
	openOd, err := exg.CreateOrder(symbol, banexg.OdTypeMarket, banexg.OdSideBuy, qty, 0, nil)
	if err != nil {
		t.Fatalf("open linear position failed: %v", err)
	}
	openDetail := bybitRequireFilledOrder(t, exg, symbol, openOd.ID)
	bybitCleanupLinearOrdersAndPositions(t, exg, symbol)
	return openDetail.Filled
}

func bybitCreateLinearMarketWithAttachedTpSlPartialLimit(t *testing.T, exg *Bybit, symbol string, qty, refPrice float64) *banexg.Order {
	t.Helper()
	tp := refPrice * 1.05
	sl := refPrice * 0.95
	tpLimit := refPrice * 1.06
	slLimit := refPrice * 0.94

	openOd, err := exg.CreateOrder(symbol, banexg.OdTypeMarket, banexg.OdSideBuy, qty, 0, map[string]interface{}{
		banexg.ParamTakeProfitPrice: tp,
		banexg.ParamStopLossPrice:   sl,
		"tpslMode":                  "Partial",
		"tpOrderType":               "Limit",
		"slOrderType":               "Limit",
		"tpLimitPrice":              tpLimit,
		"slLimitPrice":              slLimit,
		"tpTriggerBy":               "LastPrice",
		"slTriggerBy":               "LastPrice",
	})
	if err != nil {
		t.Fatalf("CreateOrder linear market with attached TP/SL failed: %v", err)
	}
	return openOd
}

func bybitCloseLinearPositionBestEffort(t *testing.T, exg *Bybit, symbol string) {
	t.Helper()
	positions, err := exg.FetchAccountPositions([]string{symbol}, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
	if err != nil {
		t.Logf("FetchAccountPositions skipped: %v", err)
		return
	}
	for _, pos := range positions {
		if pos == nil || pos.Contracts == 0 {
			continue
		}
		closeSide := banexg.OdSideSell
		if pos.Side == banexg.PosSideShort {
			closeSide = banexg.OdSideBuy
		}
		_, err := exg.CreateOrder(symbol, banexg.OdTypeMarket, closeSide, pos.Contracts, 0, map[string]interface{}{
			banexg.ParamReduceOnly: true,
		})
		if err != nil {
			t.Logf("close position skipped: %v", err)
		}
	}
}

func TestApi_CreateOrder_Spot_MarketByCost(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	market := bybitLoadAndPickMarket(t, exg, banexg.MarketSpot, "BTC/USDT", "ETH/USDT", "XRP/USDT")
	symbol := market.Symbol
	refPrice := bybitFetchRefPrice(t, exg, symbol)

	// Use quote-based market buy (qty=cost, marketUnit=quoteCoin).
	cost := 10.0
	if market.Limits != nil && market.Limits.Cost != nil && market.Limits.Cost.Min > 0 && cost < market.Limits.Cost.Min*2 {
		cost = market.Limits.Cost.Min * 2
	}
	buyOrder, err := exg.CreateOrder(symbol, banexg.OdTypeMarket, banexg.OdSideBuy, 0, 0, map[string]interface{}{
		banexg.ParamCost: cost,
	})
	if err != nil {
		bybitSkipOnTradePermission(t, err)
		t.Fatalf("CreateOrder spot market buy by cost failed: %v", err)
	}

	buyDetail := bybitRequireFilledOrder(t, exg, symbol, buyOrder.ID)

	// Sell back to keep account clean.
	bybitSpotSellBack(t, exg, symbol, buyDetail.Filled)
	_ = refPrice // keep reference for debugging when needed
}

func TestApi_CreateOrder_Spot_MarketByQty(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	market := bybitLoadAndPickMarket(t, exg, banexg.MarketSpot, "BTC/USDT", "ETH/USDT", "XRP/USDT")
	symbol := market.Symbol
	refPrice := bybitFetchRefPrice(t, exg, symbol)

	// Use base-qty market buy (qty=amount, marketUnit=baseCoin).
	targetCost := 10.0
	amount := targetCost / refPrice
	if market.Limits != nil && market.Limits.Amount != nil && market.Limits.Amount.Min > 0 && amount < market.Limits.Amount.Min*2 {
		amount = market.Limits.Amount.Min * 2
	}
	buyOrder, err := exg.CreateOrder(symbol, banexg.OdTypeMarket, banexg.OdSideBuy, amount, 0, nil)
	if err != nil {
		bybitSkipOnTradePermission(t, err)
		t.Fatalf("CreateOrder spot market buy by qty failed: %v", err)
	}

	buyDetail := bybitRequireFilledOrder(t, exg, symbol, buyOrder.ID)
	bybitSpotSellBack(t, exg, symbol, buyDetail.Filled)
}

func TestApi_CreateOrder_Spot_Market_SlippageTolerance(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	market := bybitLoadAndPickMarket(t, exg, banexg.MarketSpot, "BTC/USDT", "ETH/USDT", "XRP/USDT")
	symbol := market.Symbol

	buyOrder, err := exg.CreateOrder(symbol, banexg.OdTypeMarket, banexg.OdSideBuy, 0, 0, map[string]interface{}{
		banexg.ParamCost:        10.0,
		"slippageToleranceType": "Percent",
		"slippageTolerance":     0.5,
	})
	if err != nil {
		bybitSkipOnTradePermission(t, err)
		t.Fatalf("CreateOrder spot slippageTolerance failed: %v", err)
	}
	buyDetail := bybitRequireFilledOrder(t, exg, symbol, buyOrder.ID)
	bybitSpotSellBack(t, exg, symbol, buyDetail.Filled)
}

func TestApi_CreateOrder_Spot_Limit_PostOnly(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	market := bybitLoadAndPickMarket(t, exg, banexg.MarketSpot, "BTC/USDT", "ETH/USDT", "XRP/USDT")
	symbol := market.Symbol
	refPrice := bybitFetchRefPrice(t, exg, symbol)

	qty := bybitCalcTestQty(market, refPrice)
	limitPrice := refPrice * 0.95
	od, err := exg.CreateOrder(symbol, banexg.OdTypeLimit, banexg.OdSideBuy, qty, limitPrice, map[string]interface{}{
		banexg.ParamPostOnly: true,
	})
	if err != nil {
		bybitSkipOnTradePermission(t, err)
		t.Fatalf("CreateOrder spot postOnly limit failed: %v", err)
	}
	t.Cleanup(func() { bybitCancelOrderBestEffort(t, exg, symbol, od.ID) })

	_ = fetchBybitOrderEventually(t, exg, symbol, od.ID)
	bybitCancelOrderBestEffort(t, exg, symbol, od.ID)
}

func TestApi_CreateOrder_Spot_Conditional_Market(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	market := bybitLoadAndPickMarket(t, exg, banexg.MarketSpot, "BTC/USDT", "ETH/USDT", "XRP/USDT")
	symbol := market.Symbol
	refPrice := bybitFetchRefPrice(t, exg, symbol)

	qty := bybitCalcTestQty(market, refPrice)
	trigger := refPrice * 1.05

	od, err := exg.CreateOrder(symbol, banexg.OdTypeMarket, banexg.OdSideBuy, qty, 0, map[string]interface{}{
		banexg.ParamTriggerPrice: trigger,
	})
	if err != nil {
		bybitSkipOnTradePermission(t, err)
		t.Fatalf("CreateOrder spot conditional market failed: %v", err)
	}
	t.Cleanup(func() { bybitCancelOrderBestEffort(t, exg, symbol, od.ID) })

	_ = fetchBybitOrderEventually(t, exg, symbol, od.ID)
	bybitCancelOrderBestEffort(t, exg, symbol, od.ID)
}

func TestApi_CreateOrder_Spot_Conditional_Limit(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	market := bybitLoadAndPickMarket(t, exg, banexg.MarketSpot, "BTC/USDT", "ETH/USDT", "XRP/USDT")
	symbol := market.Symbol
	refPrice := bybitFetchRefPrice(t, exg, symbol)

	qty := bybitCalcTestQty(market, refPrice)
	trigger := refPrice * 1.05
	limitPrice := trigger * 1.01

	od, err := exg.CreateOrder(symbol, banexg.OdTypeLimit, banexg.OdSideBuy, qty, limitPrice, map[string]interface{}{
		banexg.ParamTriggerPrice: trigger,
	})
	if err != nil {
		bybitSkipOnTradePermission(t, err)
		t.Fatalf("CreateOrder spot conditional limit failed: %v", err)
	}
	t.Cleanup(func() { bybitCancelOrderBestEffort(t, exg, symbol, od.ID) })

	_ = fetchBybitOrderEventually(t, exg, symbol, od.ID)
	bybitCancelOrderBestEffort(t, exg, symbol, od.ID)
}

func TestApi_CreateOrder_Linear_Conditional_StopMarket_ReduceOnly(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	market := bybitLoadAndPickMarket(t, exg, banexg.MarketLinear, "BTC/USDT:USDT", "ETH/USDT:USDT")
	symbol := market.Symbol
	refPrice := bybitFetchRefPrice(t, exg, symbol)
	qty := bybitCalcTestQty(market, refPrice)

	// Open a small long position.
	filled := bybitOpenLinearLongAndCleanup(t, exg, symbol, qty)

	// Place a reduce-only conditional stop-market sell far below current to keep it pending.
	trigger := refPrice * 0.95
	od, err := exg.CreateOrder(symbol, banexg.OdTypeStopMarket, banexg.OdSideSell, filled, 0, map[string]interface{}{
		banexg.ParamTriggerPrice: trigger,
		banexg.ParamReduceOnly:   true,
	})
	if err != nil {
		t.Fatalf("CreateOrder linear conditional stop-market reduceOnly failed: %v", err)
	}
	bybitCancelOrderBestEffort(t, exg, symbol, od.ID)
}

func TestApi_CreateOrder_Linear_Conditional_StopMarket_TriggerBy(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	market := bybitLoadAndPickMarket(t, exg, banexg.MarketLinear, "BTC/USDT:USDT", "ETH/USDT:USDT")
	symbol := market.Symbol
	refPrice := bybitFetchRefPrice(t, exg, symbol)
	qty := bybitCalcTestQty(market, refPrice)

	filled := bybitOpenLinearLongAndCleanup(t, exg, symbol, qty)

	trigger := refPrice * 0.95
	od, err := exg.CreateOrder(symbol, banexg.OdTypeStopMarket, banexg.OdSideSell, filled, 0, map[string]interface{}{
		banexg.ParamTriggerPrice: trigger,
		banexg.ParamReduceOnly:   true,
		"triggerBy":              "MarkPrice",
	})
	if err != nil {
		t.Fatalf("CreateOrder linear conditional stop-market with triggerBy failed: %v", err)
	}
	bybitCancelOrderBestEffort(t, exg, symbol, od.ID)
}

func TestApi_CreateOrder_Linear_Conditional_TakeProfitMarket_ReduceOnly(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	market := bybitLoadAndPickMarket(t, exg, banexg.MarketLinear, "BTC/USDT:USDT", "ETH/USDT:USDT")
	symbol := market.Symbol
	refPrice := bybitFetchRefPrice(t, exg, symbol)
	qty := bybitCalcTestQty(market, refPrice)

	filled := bybitOpenLinearLongAndCleanup(t, exg, symbol, qty)

	// Place a reduce-only conditional take-profit market sell far above current to keep it pending.
	trigger := refPrice * 1.05
	od, err := exg.CreateOrder(symbol, banexg.OdTypeTakeProfitMarket, banexg.OdSideSell, filled, 0, map[string]interface{}{
		banexg.ParamTriggerPrice: trigger,
		banexg.ParamReduceOnly:   true,
	})
	if err != nil {
		t.Fatalf("CreateOrder linear conditional take-profit market reduceOnly failed: %v", err)
	}
	bybitCancelOrderBestEffort(t, exg, symbol, od.ID)
}

func TestApi_CreateOrder_Linear_Conditional_StopMarket_ClosePosition(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	market := bybitLoadAndPickMarket(t, exg, banexg.MarketLinear, "BTC/USDT:USDT", "ETH/USDT:USDT")
	symbol := market.Symbol
	refPrice := bybitFetchRefPrice(t, exg, symbol)
	qty := bybitCalcTestQty(market, refPrice)

	// Open a small long position.
	_ = bybitOpenLinearLongAndCleanup(t, exg, symbol, qty)

	trigger := refPrice * 0.95
	od, err := exg.CreateOrder(symbol, banexg.OdTypeStopMarket, banexg.OdSideSell, 0, 0, map[string]interface{}{
		banexg.ParamTriggerPrice:  trigger,
		banexg.ParamClosePosition: true,
	})
	if err != nil {
		t.Fatalf("CreateOrder linear closePosition stop-market failed: %v", err)
	}
	bybitCancelOrderBestEffort(t, exg, symbol, od.ID)
}

func TestApi_CreateOrder_Linear_StopLoss_FromStopLossPrice_ReduceOnly(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	market := bybitLoadAndPickMarket(t, exg, banexg.MarketLinear, "BTC/USDT:USDT", "ETH/USDT:USDT")
	symbol := market.Symbol
	refPrice := bybitFetchRefPrice(t, exg, symbol)
	qty := bybitCalcTestQty(market, refPrice)

	filled := bybitOpenLinearLongAndCleanup(t, exg, symbol, qty)

	stopLoss := refPrice * 0.95
	od, err := exg.CreateOrder(symbol, banexg.OdTypeStopLoss, banexg.OdSideSell, filled, 0, map[string]interface{}{
		banexg.ParamStopLossPrice: stopLoss,
		banexg.ParamReduceOnly:    true,
	})
	if err != nil {
		t.Fatalf("CreateOrder linear stop_loss reduceOnly failed: %v", err)
	}
	bybitCancelOrderBestEffort(t, exg, symbol, od.ID)
}

func TestApi_CreateOrder_Linear_TakeProfit_FromTakeProfitPrice_ReduceOnly(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	market := bybitLoadAndPickMarket(t, exg, banexg.MarketLinear, "BTC/USDT:USDT", "ETH/USDT:USDT")
	symbol := market.Symbol
	refPrice := bybitFetchRefPrice(t, exg, symbol)
	qty := bybitCalcTestQty(market, refPrice)

	filled := bybitOpenLinearLongAndCleanup(t, exg, symbol, qty)

	takeProfit := refPrice * 1.05
	od, err := exg.CreateOrder(symbol, banexg.OdTypeTakeProfit, banexg.OdSideSell, filled, 0, map[string]interface{}{
		banexg.ParamTakeProfitPrice: takeProfit,
		banexg.ParamReduceOnly:      true,
	})
	if err != nil {
		t.Fatalf("CreateOrder linear take_profit reduceOnly failed: %v", err)
	}
	bybitCancelOrderBestEffort(t, exg, symbol, od.ID)
}

func TestApi_CreateOrder_Linear_Market_AttachTpSl_Params(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	market := bybitLoadAndPickMarket(t, exg, banexg.MarketLinear, "BTC/USDT:USDT", "ETH/USDT:USDT")
	symbol := market.Symbol
	refPrice := bybitFetchRefPrice(t, exg, symbol)
	qty := bybitCalcTestQty(market, refPrice)

	openOd := bybitCreateLinearMarketWithAttachedTpSlPartialLimit(t, exg, symbol, qty, refPrice)
	_ = bybitRequireFilledOrder(t, exg, symbol, openOd.ID)
	bybitCleanupLinearOrdersAndPositions(t, exg, symbol)

	// Allow TP/SL orders to be created.
	time.Sleep(800 * time.Millisecond)
}

func TestApi_CreateOrder_Linear_TrailingStopMarket_Params(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	market := bybitLoadAndPickMarket(t, exg, banexg.MarketLinear, "BTC/USDT:USDT", "ETH/USDT:USDT")
	symbol := market.Symbol
	refPrice := bybitFetchRefPrice(t, exg, symbol)
	qty := bybitCalcTestQty(market, refPrice)

	// Open a small long position.
	_ = bybitOpenLinearLongAndCleanup(t, exg, symbol, qty)

	// For Bybit, trailingStop is an absolute price offset.
	trailingDelta := refPrice * 0.002 // 0.2%
	if trailingDelta < 1 {
		trailingDelta = 1
	}
	activation := refPrice * 1.01
	if _, err := exg.CreateOrder(symbol, banexg.OdTypeTrailingStopMarket, banexg.OdSideBuy, 0, 0, map[string]interface{}{
		banexg.ParamTrailingDelta:   trailingDelta,
		banexg.ParamActivationPrice: activation,
	}); err != nil {
		t.Fatalf("CreateOrder trailing stop failed: %v", err)
	}
}

func TestApi_CreateOrder_Linear_PositionSide_HedgeMode(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	market := bybitLoadAndPickMarket(t, exg, banexg.MarketLinear, "BTC/USDT:USDT", "ETH/USDT:USDT")
	symbol := market.Symbol
	refPrice := bybitFetchRefPrice(t, exg, symbol)
	qty := bybitCalcTestQty(market, refPrice)

	// Many accounts run in one-way mode; in that case, positionIdx=1/2 is rejected.
	openOd, err := exg.CreateOrder(symbol, banexg.OdTypeMarket, banexg.OdSideBuy, qty, 0, map[string]interface{}{
		banexg.ParamPositionSide: banexg.PosSideLong,
	})
	if err != nil {
		msg := strings.ToLower(err.Message())
		if strings.Contains(msg, "position idx") || strings.Contains(msg, "position mode") || strings.Contains(msg, "hedge") {
			t.Skipf("skip hedge-mode positionSide mapping: %v", err)
		}
		t.Fatalf("CreateOrder with positionSide failed: %v", err)
	}
	openDetail := bybitRequireFilledOrder(t, exg, symbol, openOd.ID)
	t.Cleanup(func() {
		bybitCancelOpenOrdersForSymbol(t, exg, symbol)
		_, _ = exg.CreateOrder(symbol, banexg.OdTypeMarket, banexg.OdSideSell, openDetail.Filled, 0, map[string]interface{}{
			banexg.ParamReduceOnly:   true,
			banexg.ParamPositionSide: banexg.PosSideLong,
		})
	})

	_, err = exg.CreateOrder(symbol, banexg.OdTypeMarket, banexg.OdSideSell, openDetail.Filled, 0, map[string]interface{}{
		banexg.ParamReduceOnly:   true,
		banexg.ParamPositionSide: banexg.PosSideLong,
	})
	if err != nil {
		t.Fatalf("close hedge long failed: %v", err)
	}
}

func TestApi_CreateOrder_Spot_SmpType_FromSelfTradePreventionMode(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	market := bybitLoadAndPickMarket(t, exg, banexg.MarketSpot, "BTC/USDT", "ETH/USDT", "XRP/USDT")
	symbol := market.Symbol

	// Use a post-only order to avoid accidental fills; we only need the request accepted.
	refPrice := bybitFetchRefPrice(t, exg, symbol)
	qty := bybitCalcTestQty(market, refPrice)
	limitPrice := refPrice * 0.95

	od, err := exg.CreateOrder(symbol, banexg.OdTypeLimit, banexg.OdSideBuy, qty, limitPrice, map[string]interface{}{
		banexg.ParamPostOnly:                true,
		banexg.ParamSelfTradePreventionMode: "CancelBoth",
	})
	if err != nil {
		bybitSkipOnTradePermission(t, err)
		t.Fatalf("CreateOrder spot SMP failed: %v", err)
	}
	t.Cleanup(func() { bybitCancelOrderBestEffort(t, exg, symbol, od.ID) })
	bybitCancelOrderBestEffort(t, exg, symbol, od.ID)
}

func TestApi_CreateOrder_Linear_Market_AttachedTpSl_PartialLimit(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	market := bybitLoadAndPickMarket(t, exg, banexg.MarketLinear, "BTC/USDT:USDT", "ETH/USDT:USDT")
	symbol := market.Symbol
	refPrice := bybitFetchRefPrice(t, exg, symbol)
	qty := bybitCalcTestQty(market, refPrice)

	openOd := bybitCreateLinearMarketWithAttachedTpSlPartialLimit(t, exg, symbol, qty, refPrice)
	_ = bybitRequireFilledOrder(t, exg, symbol, openOd.ID)
	bybitCleanupLinearOrdersAndPositions(t, exg, symbol)

	// Allow TP/SL orders to be created.
	time.Sleep(800 * time.Millisecond)
}

func bybitCreateSpotLimitWithAttachedTpSl(t *testing.T, exg *Bybit, useLimitTpSl bool) {
	t.Helper()

	markets := loadBybitMarketsForType(t, exg, banexg.MarketSpot)
	market := bybitPickMarketPrefer(t, markets, banexg.MarketSpot, "BTC/USDT", "ETH/USDT", "XRP/USDT")
	if market == nil || market.Symbol == "" {
		t.Skip("no spot markets available")
	}
	symbol := market.Symbol
	refPrice := bybitFetchRefPrice(t, exg, symbol)
	qty := bybitCalcTestQty(market, refPrice)

	// Keep it safely away from market price to avoid fills.
	limitPrice := refPrice * 0.95
	tp := refPrice * 1.05
	sl := refPrice * 0.90

	params := map[string]interface{}{
		banexg.ParamTimeInForce:     banexg.TimeInForcePO,
		banexg.ParamTakeProfitPrice: tp,
		banexg.ParamStopLossPrice:   sl,
	}
	if useLimitTpSl {
		params["tpOrderType"] = "Limit"
		params["slOrderType"] = "Limit"
		params["tpLimitPrice"] = tp
		params["slLimitPrice"] = sl
	} else {
		params["tpOrderType"] = "Market"
		params["slOrderType"] = "Market"
	}

	od, err := exg.CreateOrder(symbol, banexg.OdTypeLimit, banexg.OdSideBuy, qty, limitPrice, params)
	if err != nil {
		bybitSkipOnTradePermission(t, err)
		// Spot TP/SL attachment support depends on account/product settings; treat as environment-specific.
		// See docs/bybit_v5/order/create-order.md.
		if err.BizCode == 176009 || err.BizCode == 170033 || err.BizCode == 110044 {
			t.Skipf("skip due to bybit limitations (bizCode=%d): %v", err.BizCode, err)
		}
		t.Fatalf("CreateOrder spot limit with attached TP/SL failed: %v", err)
	}
	if od == nil || od.ID == "" {
		t.Fatalf("expected order id, got %#v", od)
	}
	t.Cleanup(func() { bybitCancelOrderBestEffort(t, exg, symbol, od.ID) })

	// Best-effort: ensure the order can be queried.
	_ = fetchBybitOrderEventually(t, exg, symbol, od.ID)
}

func TestApi_CreateOrder_Spot_Limit_AttachedTpSl_MarketTpSl(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	bybitCreateSpotLimitWithAttachedTpSl(t, exg, false)
}

func TestApi_CreateOrder_Spot_Limit_AttachedTpSl_LimitTpSl(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	bybitCreateSpotLimitWithAttachedTpSl(t, exg, true)
}

func TestApi_CreateOrder_Linear_Limit_BboParams(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	markets := loadBybitMarketsForType(t, exg, banexg.MarketLinear)
	market := bybitPickMarketPrefer(t, markets, banexg.MarketLinear, "BTC/USDT:USDT", "ETH/USDT:USDT")
	if market == nil || market.Symbol == "" {
		t.Skip("no linear markets available")
	}
	symbol := market.Symbol
	refPrice := bybitFetchRefPrice(t, exg, symbol)
	qty := bybitCalcTestQty(market, refPrice)

	// Use PostOnly + BBO Queue to reduce the chance of getting filled.
	od, err := exg.CreateOrder(symbol, banexg.OdTypeLimit, banexg.OdSideBuy, qty, refPrice*0.95, map[string]interface{}{
		banexg.ParamTimeInForce: banexg.TimeInForcePO,
		"bboSideType":           "Queue",
		"bboLevel":              "1",
	})
	if err != nil {
		bybitSkipOnTradePermission(t, err)
		t.Fatalf("CreateOrder linear bbo params failed: %v", err)
	}
	if od == nil || od.ID == "" {
		t.Fatalf("expected order id, got %#v", od)
	}
	bybitCancelOrderBestEffort(t, exg, symbol, od.ID)
}

func TestApi_CreateOrder_Spot_Limit_MarginMode_IsLeverage(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	markets := loadBybitMarketsForType(t, exg, banexg.MarketSpot)
	market := bybitPickMarketPrefer(t, markets, banexg.MarketSpot, "BTC/USDT", "ETH/USDT", "XRP/USDT")
	if market == nil || market.Symbol == "" {
		t.Skip("no spot markets available")
	}
	symbol := market.Symbol
	refPrice := bybitFetchRefPrice(t, exg, symbol)
	qty := bybitCalcTestQty(market, refPrice)

	od, err := exg.CreateOrder(symbol, banexg.OdTypeLimit, banexg.OdSideBuy, qty, refPrice*0.95, map[string]interface{}{
		banexg.ParamTimeInForce: banexg.TimeInForcePO,
		banexg.ParamMarginMode:  banexg.MarginCross,
	})
	if err != nil {
		bybitSkipOnTradePermission(t, err)
		// Spot margin may not be enabled for the account/testnet; skip on known Bybit errors.
		if err.BizCode == 176009 || err.BizCode == 182021 || err.BizCode == 170033 || err.BizCode == 110044 {
			t.Skipf("skip spot margin order (bizCode=%d): %v", err.BizCode, err)
		}
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "margin") && strings.Contains(msg, "enable") {
			t.Skipf("skip spot margin order: %v", err)
		}
		t.Fatalf("CreateOrder spot margin-mode failed: %v", err)
	}
	if od == nil || od.ID == "" {
		t.Fatalf("expected order id, got %#v", od)
	}
	bybitCancelOrderBestEffort(t, exg, symbol, od.ID)
}

func bybitAlmostEqual(got, want float64) bool {
	// Price/qty in Bybit are typically rounded to tick/step size; tolerate tiny float error.
	const absEps = 1e-10
	relEps := 1e-9 * math.Max(1, math.Abs(want))
	return math.Abs(got-want) <= math.Max(absEps, relEps)
}

func bybitRequireAlmostEqual(t *testing.T, field string, got, want float64) {
	t.Helper()
	if !bybitAlmostEqual(got, want) {
		t.Fatalf("%s mismatch: got=%v want=%v", field, got, want)
	}
}

func bybitSetupEditOrderMarket(t *testing.T, exg *Bybit, marketType string) (*banexg.Market, string, float64, float64, float64) {
	t.Helper()
	markets := loadBybitMarketsForType(t, exg, marketType)
	if len(markets) == 0 {
		t.Skipf("no %s markets available", marketType)
	}

	prefer := bybitPreferredSymbolsForMarketType(marketType)

	market := bybitPickMarketPrefer(t, markets, marketType, prefer...)
	if market == nil || market.Symbol == "" {
		t.Skipf("no %s markets available", marketType)
	}
	symbol := market.Symbol
	refPrice := bybitFetchRefPrice(t, exg, symbol)
	qty := bybitCalcTestQty(market, refPrice)

	// Keep it away from market to reduce accidental fills.
	limitPrice := refPrice * 0.9
	return market, symbol, refPrice, qty, limitPrice
}

type bybitEditOrderLimitFixture struct {
	Market     *banexg.Market
	Symbol     string
	RefPrice   float64
	Qty        float64
	LimitPrice float64
	OrderID    string
	ClientID   string
}

func bybitSetupEditOrderLimitFixture(t *testing.T, exg *Bybit, marketType string) bybitEditOrderLimitFixture {
	t.Helper()

	market, symbol, refPrice, qty, limitPrice := bybitSetupEditOrderMarket(t, exg, marketType)
	clientID := bybitTestClientOrderID("editorder")
	od, err := exg.CreateOrder(symbol, banexg.OdTypeLimit, banexg.OdSideBuy, qty, limitPrice, map[string]interface{}{
		banexg.ParamPostOnly:      true,
		banexg.ParamClientOrderId: clientID,
	})
	if err != nil {
		bybitSkipOnTradePermission(t, err)
		t.Fatalf("CreateOrder limit postOnly failed: %v", err)
	}
	if od == nil || od.ID == "" {
		t.Fatalf("expected order id, got %#v", od)
	}
	orderID := od.ID
	_ = fetchBybitOrderEventually(t, exg, symbol, orderID)

	t.Cleanup(func() {
		bybitCancelOrderBestEffort(t, exg, symbol, orderID)
		bybitCancelOpenOrdersForSymbol(t, exg, symbol)
	})

	return bybitEditOrderLimitFixture{
		Market:     market,
		Symbol:     symbol,
		RefPrice:   refPrice,
		Qty:        qty,
		LimitPrice: limitPrice,
		OrderID:    orderID,
		ClientID:   clientID,
	}
}

type bybitEditOrderStopMarketFixture struct {
	Market   *banexg.Market
	Symbol   string
	RefPrice float64
	Qty      float64
	OrderID  string
	ClientID string
	Trigger  float64
}

func bybitSetupEditOrderStopMarketFixture(t *testing.T, exg *Bybit) bybitEditOrderStopMarketFixture {
	t.Helper()

	market, symbol, refPrice, qty, _ := bybitSetupEditOrderMarket(t, exg, banexg.MarketLinear)
	clientID := bybitTestClientOrderID("editorder-stop")

	// Buy stop-market far above current so it remains pending.
	trigger := refPrice * 1.2
	od, err := exg.CreateOrder(symbol, banexg.OdTypeStopMarket, banexg.OdSideBuy, qty, 0, map[string]interface{}{
		banexg.ParamClientOrderId: clientID,
		banexg.ParamTriggerPrice:  trigger,
	})
	if err != nil {
		bybitSkipOnTradePermission(t, err)
		t.Fatalf("CreateOrder stop-market failed: %v", err)
	}
	if od == nil || od.ID == "" {
		t.Fatalf("expected order id, got %#v", od)
	}
	orderID := od.ID
	_ = fetchBybitOrderEventually(t, exg, symbol, orderID)

	t.Cleanup(func() {
		bybitCancelOrderBestEffort(t, exg, symbol, orderID)
		bybitCancelOpenOrdersForSymbol(t, exg, symbol)
	})

	return bybitEditOrderStopMarketFixture{
		Market:   market,
		Symbol:   symbol,
		RefPrice: refPrice,
		Qty:      qty,
		OrderID:  orderID,
		ClientID: clientID,
		Trigger:  trigger,
	}
}

func bybitEditOrderMust(t *testing.T, exg *Bybit, symbol, orderID, side string, amount, price float64, params map[string]interface{}, action string) {
	t.Helper()
	if _, err := exg.EditOrder(symbol, orderID, side, amount, price, params); err != nil {
		t.Fatalf("EditOrder %s failed: %v", action, err)
	}
}

func bybitInfoFloat(t *testing.T, info map[string]interface{}, key string) float64 {
	t.Helper()
	if info == nil {
		t.Fatalf("expected info contains %s, got nil info", key)
	}
	raw, ok := info[key]
	if !ok {
		t.Fatalf("expected info contains %s, keys=%v", key, utils.KeysOfMap(info))
	}
	f, err := utils.ParseNum(raw)
	if err != nil {
		t.Fatalf("ParseNum(%s) failed: %v (raw=%#v)", key, err, raw)
	}
	return f
}

func TestApi_EditOrder_Linear_PriceOnly_ByOrderID(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	fx := bybitSetupEditOrderLimitFixture(t, exg, banexg.MarketLinear)

	newPrice := fx.LimitPrice * 0.95
	precNewPrice := bybitPrecPriceMust(t, exg, fx.Market, newPrice)
	bybitEditOrderMust(t, exg, fx.Symbol, fx.OrderID, banexg.OdSideBuy, 0, newPrice, nil, "price-only")

	got := fetchBybitOrderEventually(t, exg, fx.Symbol, fx.OrderID)
	bybitRequireAlmostEqual(t, "order.price", got.Price, precNewPrice)
}

func TestApi_EditOrder_Linear_QtyOnly_ByOrderID(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	fx := bybitSetupEditOrderLimitFixture(t, exg, banexg.MarketLinear)

	newQty := fx.Qty * 1.5
	precNewQty := bybitPrecAmountMust(t, exg, fx.Market, newQty)
	bybitEditOrderMust(t, exg, fx.Symbol, fx.OrderID, banexg.OdSideBuy, newQty, 0, nil, "qty-only")

	got := fetchBybitOrderEventually(t, exg, fx.Symbol, fx.OrderID)
	bybitRequireAlmostEqual(t, "order.amount", got.Amount, precNewQty)
}

func TestApi_EditOrder_Linear_QtyAndPrice_ByClientOrderID(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	fx := bybitSetupEditOrderLimitFixture(t, exg, banexg.MarketLinear)

	newQty := fx.Qty * 1.3
	newPrice := fx.LimitPrice * 0.97
	precNewQty := bybitPrecAmountMust(t, exg, fx.Market, newQty)
	precNewPrice := bybitPrecPriceMust(t, exg, fx.Market, newPrice)

	// Use ParamClientOrderId instead of orderId.
	bybitEditOrderMust(t, exg, fx.Symbol, "", banexg.OdSideBuy, newQty, newPrice, map[string]interface{}{
		banexg.ParamClientOrderId: fx.ClientID,
	}, "qty+price by clientOrderId")

	got := fetchBybitOrderEventually(t, exg, fx.Symbol, fx.OrderID)
	bybitRequireAlmostEqual(t, "order.amount", got.Amount, precNewQty)
	bybitRequireAlmostEqual(t, "order.price", got.Price, precNewPrice)
}

func TestApi_EditOrder_Linear_TriggerPriceOnly(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	fx := bybitSetupEditOrderStopMarketFixture(t, exg)

	newTrigger := fx.RefPrice * 1.25
	precNewTrigger := bybitPrecPriceMust(t, exg, fx.Market, newTrigger)
	bybitEditOrderMust(t, exg, fx.Symbol, fx.OrderID, banexg.OdSideBuy, 0, 0, map[string]interface{}{
		banexg.ParamTriggerPrice: newTrigger,
	}, "triggerPrice-only")

	got := fetchBybitOrderEventually(t, exg, fx.Symbol, fx.OrderID)
	bybitRequireAlmostEqual(t, "order.triggerPrice", got.TriggerPrice, precNewTrigger)
}

func TestApi_EditOrder_Linear_TriggerPrice_WithTriggerBy(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	fx := bybitSetupEditOrderStopMarketFixture(t, exg)

	newTrigger := fx.RefPrice * 1.3
	precNewTrigger := bybitPrecPriceMust(t, exg, fx.Market, newTrigger)
	bybitEditOrderMust(t, exg, fx.Symbol, fx.OrderID, banexg.OdSideBuy, 0, 0, map[string]interface{}{
		banexg.ParamTriggerPrice: newTrigger,
		"triggerBy":              "MarkPrice",
	}, "triggerPrice+triggerBy")

	got := fetchBybitOrderEventually(t, exg, fx.Symbol, fx.OrderID)
	bybitRequireAlmostEqual(t, "order.triggerPrice", got.TriggerPrice, precNewTrigger)
}

func TestApi_EditOrder_Linear_SetTakeProfitOnly(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	fx := bybitSetupEditOrderLimitFixture(t, exg, banexg.MarketLinear)

	tp := fx.RefPrice * 1.2
	precTP := bybitPrecPriceMust(t, exg, fx.Market, tp)
	bybitEditOrderMust(t, exg, fx.Symbol, fx.OrderID, banexg.OdSideBuy, 0, 0, map[string]interface{}{
		banexg.ParamTakeProfitPrice: tp,
		"tpTriggerBy":               "LastPrice",
	}, "takeProfit-only")

	got := fetchBybitOrderEventually(t, exg, fx.Symbol, fx.OrderID)
	bybitRequireAlmostEqual(t, "order.takeProfitPrice", got.TakeProfitPrice, precTP)
}

func TestApi_EditOrder_Linear_SetStopLossOnly(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	fx := bybitSetupEditOrderLimitFixture(t, exg, banexg.MarketLinear)

	sl := fx.RefPrice * 0.8
	precSL := bybitPrecPriceMust(t, exg, fx.Market, sl)
	bybitEditOrderMust(t, exg, fx.Symbol, fx.OrderID, banexg.OdSideBuy, 0, 0, map[string]interface{}{
		banexg.ParamStopLossPrice: sl,
		"slTriggerBy":             "LastPrice",
	}, "stopLoss-only")

	got := fetchBybitOrderEventually(t, exg, fx.Symbol, fx.OrderID)
	bybitRequireAlmostEqual(t, "order.stopLossPrice", got.StopLossPrice, precSL)
}

func TestApi_EditOrder_Linear_SetTakeProfitAndStopLoss(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	fx := bybitSetupEditOrderLimitFixture(t, exg, banexg.MarketLinear)

	tp := fx.RefPrice * 1.18
	sl := fx.RefPrice * 0.82
	precTP := bybitPrecPriceMust(t, exg, fx.Market, tp)
	precSL := bybitPrecPriceMust(t, exg, fx.Market, sl)

	bybitEditOrderMust(t, exg, fx.Symbol, fx.OrderID, banexg.OdSideBuy, 0, 0, map[string]interface{}{
		banexg.ParamTakeProfitPrice: tp,
		banexg.ParamStopLossPrice:   sl,
		"tpTriggerBy":               "LastPrice",
		"slTriggerBy":               "LastPrice",
	}, "takeProfit+stopLoss")

	got := fetchBybitOrderEventually(t, exg, fx.Symbol, fx.OrderID)
	bybitRequireAlmostEqual(t, "order.takeProfitPrice", got.TakeProfitPrice, precTP)
	bybitRequireAlmostEqual(t, "order.stopLossPrice", got.StopLossPrice, precSL)
}

func TestApi_EditOrder_Linear_PartialLimitTpSl_Params(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	fx := bybitSetupEditOrderLimitFixture(t, exg, banexg.MarketLinear)

	tp := fx.RefPrice * 1.21
	sl := fx.RefPrice * 0.79
	tpLimit := tp
	slLimit := sl * 0.99

	precTP := bybitPrecPriceMust(t, exg, fx.Market, tp)
	precSL := bybitPrecPriceMust(t, exg, fx.Market, sl)
	precTpLimit := bybitPrecPriceMust(t, exg, fx.Market, tpLimit)
	precSlLimit := bybitPrecPriceMust(t, exg, fx.Market, slLimit)

	bybitEditOrderMust(t, exg, fx.Symbol, fx.OrderID, banexg.OdSideBuy, 0, 0, map[string]interface{}{
		banexg.ParamTakeProfitPrice: tp,
		banexg.ParamStopLossPrice:   sl,
		"tpTriggerBy":               "LastPrice",
		"slTriggerBy":               "LastPrice",
		"tpslMode":                  "Partial",
		"tpLimitPrice":              tpLimit,
		"slLimitPrice":              slLimit,
	}, "partial limit TP/SL params")

	// Amend is async; give it a brief moment to propagate.
	time.Sleep(500 * time.Millisecond)

	got := fetchBybitOrderEventually(t, exg, fx.Symbol, fx.OrderID)
	bybitRequireAlmostEqual(t, "order.takeProfitPrice", got.TakeProfitPrice, precTP)
	bybitRequireAlmostEqual(t, "order.stopLossPrice", got.StopLossPrice, precSL)

	gotTpLimit := bybitInfoFloat(t, got.Info, "tpLimitPrice")
	gotSlLimit := bybitInfoFloat(t, got.Info, "slLimitPrice")
	bybitRequireAlmostEqual(t, "info.tpLimitPrice", gotTpLimit, precTpLimit)
	bybitRequireAlmostEqual(t, "info.slLimitPrice", gotSlLimit, precSlLimit)
}

func TestApi_EditOrder_Option_OrderIvOnly(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	market, symbol, _, _, limitPrice := bybitSetupEditOrderMarket(t, exg, banexg.MarketOption)

	// Option accounts are often unfunded in dev environments. Keep this order tiny and skip on "insufficient margin".
	qty := 0.1
	if market.Limits != nil && market.Limits.Amount != nil && market.Limits.Amount.Min > 0 {
		qty = market.Limits.Amount.Min
	}

	clientID := bybitTestClientOrderID("editorder-opt")
	od, err := exg.CreateOrder(symbol, banexg.OdTypeLimit, banexg.OdSideBuy, qty, limitPrice*0.8, map[string]interface{}{
		banexg.ParamClientOrderId: clientID,
	})
	if err != nil {
		bybitSkipOnTradePermission(t, err)
		msg := strings.ToLower(err.Error())
		if err.BizCode == 110044 || strings.Contains(msg, "insufficient") {
			t.Skipf("skip option orderIv test due to insufficient margin: %v", err)
		}
		t.Fatalf("CreateOrder option limit failed: %v", err)
	}
	if od == nil || od.ID == "" {
		t.Fatalf("expected order id, got %#v", od)
	}
	t.Cleanup(func() {
		bybitCancelOrderBestEffort(t, exg, symbol, od.ID)
		bybitCancelOpenOrdersForSymbol(t, exg, symbol)
	})

	detail := fetchBybitOrderEventually(t, exg, symbol, od.ID)
	curIv := 0.0
	if detail.Info != nil {
		if raw, ok := detail.Info["orderIv"]; ok {
			if v, err := utils.ParseNum(raw); err == nil {
				curIv = v
			}
		}
	}
	newIv := 0.5
	if curIv > 0 {
		newIv = curIv * 1.01
	}
	t.Logf("option orderIv: current=%v new=%v", curIv, newIv)

	// Prefer orderLinkId for option amend to reduce potential "order not exist" flakiness.
	if _, err := exg.EditOrder(symbol, "", banexg.OdSideBuy, 0, 0, map[string]interface{}{
		banexg.ParamClientOrderId: clientID,
		"orderIv":                 newIv,
	}); err != nil {
		bybitSkipOnTradePermission(t, err)
		msg := strings.ToLower(err.Error())
		if err.BizCode == 10001 {
			t.Skipf("skip option orderIv test due to ParamInvalid: %v", err)
		}
		if err.BizCode == 110001 {
			t.Skipf("skip option orderIv test due to DataNotFound: %v", err)
		}
		if err.BizCode == 110044 || strings.Contains(msg, "insufficient") {
			t.Skipf("skip option orderIv amend due to insufficient margin: %v", err)
		}
		t.Fatalf("EditOrder option orderIv-only failed: %v", err)
	}
}

func fetchBybitOpenOrdersNoError(t *testing.T, exg *Bybit, symbol string, limit int, params map[string]interface{}) []*banexg.Order {
	t.Helper()
	items, err := exg.FetchOpenOrders(symbol, 0, limit, params)
	if err != nil {
		t.Fatalf("FetchOpenOrders failed: %v", err)
	}
	if items == nil {
		t.Fatal("expected orders slice")
	}
	return items
}

func fetchBybitOpenOrdersContainEventually(t *testing.T, exg *Bybit, symbol string, params map[string]interface{}, wantIDs ...string) []*banexg.Order {
	t.Helper()
	return retryBybitEventually(t, "FetchOpenOrders (contain)", func() ([]*banexg.Order, error) {
		orders, err := exg.FetchOpenOrders(symbol, 0, 50, params)
		if err != nil {
			return nil, err
		}
		for _, wantID := range wantIDs {
			found := false
			for _, od := range orders {
				if od != nil && od.ID == wantID {
					found = true
					break
				}
			}
			if !found {
				return nil, errNotFound
			}
		}
		return orders, nil
	})
}

func bybitMarketIDMust(t *testing.T, exg *Bybit, symbol string) string {
	t.Helper()
	ensureBybitMarketsLoaded(t, exg)
	market := exg.Markets[symbol]
	if market == nil || market.ID == "" {
		t.Fatalf("market not found for symbol %s", symbol)
	}
	return market.ID
}

func bybitOpenOrdersCursorEventually(t *testing.T, exg *Bybit, symbol string, extraArgs map[string]interface{}) string {
	t.Helper()
	tryNum := exg.GetRetryNum("FetchOpenOrders", 1)
	return retryBybitEventually(t, "Get open orders cursor", func() (string, error) {
		args, _, _, _, err := exg.loadBybitOrderArgs(symbol, extraArgs)
		if err != nil {
			return "", err
		}
		args["limit"] = 1

		res := requestRetry[V5ListResult](exg, MethodPrivateGetV5OrderRealtime, args, tryNum)
		if res.Error != nil {
			return "", res.Error
		}
		if res.Result.NextPageCursor == "" {
			return "", errNotFound
		}
		return res.Result.NextPageCursor, nil
	})
}

func bybitNewOpenOrder(t *testing.T, exg *Bybit, symbol string, idPrefix string) (*banexg.Order, string, func()) {
	t.Helper()
	clientID := bybitTestClientOrderID(idPrefix)
	order := bybitPlacePostOnlyLimitOrder(t, exg, symbol, clientID)
	cleanup := func() {
		t.Helper()
		bybitCancelOrderBestEffort(t, exg, symbol, order.ID)
	}
	return order, clientID, cleanup
}

func TestApi_FetchOpenOrders_SymbolOnly(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	order, _, cleanup := bybitNewOpenOrder(t, exg, bybitFetchOrderTestSymbol, "fetchopenorders-symbol")
	defer cleanup()

	items := fetchBybitOpenOrdersContainEventually(t, exg, bybitFetchOrderTestSymbol, nil, order.ID)
	requireOrdersContainID(t, items, order.ID, "symbol")
}

func TestApi_FetchOpenOrders_SymbolOnly_OrderFilter(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	order, _, cleanup := bybitNewOpenOrder(t, exg, bybitFetchOrderTestSymbol, "fetchopenorders-of")
	defer cleanup()

	params := map[string]interface{}{"orderFilter": "Order"}
	items := fetchBybitOpenOrdersContainEventually(t, exg, bybitFetchOrderTestSymbol, params, order.ID)
	requireOrdersContainID(t, items, order.ID, "orderFilter=Order")
}

func TestApi_FetchOpenOrders_SymbolOnly_OpenOnlyClosed_NoError(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	// openOnly=1 switches to returning recent closed records. We only assert the request succeeds.
	fetchBybitOpenOrdersNoError(t, exg, bybitFetchOrderTestSymbol, 10, map[string]interface{}{"openOnly": 1})
}

func TestApi_FetchOpenOrders_NoSymbol_MarketTypeOnly_Spot(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fetchBybitOpenOrdersNoError(t, exg, "", 10, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketSpot,
	})
}

func TestApi_FetchOpenOrders_NoSymbol_Linear_BaseCoin(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	order, _, cleanup := bybitNewOpenOrder(t, exg, bybitFetchOrderTestSymbol, "fetchopenorders-base")
	defer cleanup()

	params := map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
		"baseCoin":         "XRP",
	}
	items := fetchBybitOpenOrdersContainEventually(t, exg, "", params, order.ID)
	requireOrdersContainID(t, items, order.ID, "baseCoin=XRP")
}

func TestApi_FetchOpenOrders_NoSymbol_Linear_SettleCoin(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	order, _, cleanup := bybitNewOpenOrder(t, exg, bybitFetchOrderTestSymbol, "fetchopenorders-settle")
	defer cleanup()

	params := map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
		"settleCoin":       "USDT",
	}
	items := fetchBybitOpenOrdersContainEventually(t, exg, "", params, order.ID)
	requireOrdersContainID(t, items, order.ID, "settleCoin=USDT")
}

func TestApi_FetchOpenOrders_NoSymbol_Linear_SymbolParam(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	order, _, cleanup := bybitNewOpenOrder(t, exg, bybitFetchOrderTestSymbol, "fetchopenorders-nosym")
	defer cleanup()

	marketID := bybitMarketIDMust(t, exg, bybitFetchOrderTestSymbol)
	params := map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
		"symbol":           marketID,
	}
	items := fetchBybitOpenOrdersContainEventually(t, exg, "", params, order.ID)
	requireOrdersContainID(t, items, order.ID, "symbol="+marketID)
}

func TestApi_FetchOpenOrders_ByOrderID(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	order, _, cleanup := bybitNewOpenOrder(t, exg, bybitFetchOrderTestSymbol, "fetchopenorders-oid")
	defer cleanup()

	items := fetchBybitOpenOrdersContainEventually(t, exg, bybitFetchOrderTestSymbol, map[string]interface{}{"orderId": order.ID}, order.ID)
	requireOrdersContainID(t, items, order.ID, "orderId")
}

func TestApi_FetchOpenOrders_ByOrderLinkID(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	order, clientID, cleanup := bybitNewOpenOrder(t, exg, bybitFetchOrderTestSymbol, "fetchopenorders-olid")
	defer cleanup()

	items := fetchBybitOpenOrdersContainEventually(t, exg, bybitFetchOrderTestSymbol, map[string]interface{}{"orderLinkId": clientID}, order.ID)
	requireOrdersContainID(t, items, order.ID, "orderLinkId")
}

func TestApi_FetchOpenOrders_ByClientOrderID(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	order, clientID, cleanup := bybitNewOpenOrder(t, exg, bybitFetchOrderTestSymbol, "fetchopenorders-cloid")
	defer cleanup()

	items := fetchBybitOpenOrdersContainEventually(t, exg, bybitFetchOrderTestSymbol, map[string]interface{}{banexg.ParamClientOrderId: clientID}, order.ID)
	requireOrdersContainID(t, items, order.ID, "clientOrderId")
}

func TestApi_FetchOpenOrders_LimitZero_WithSymbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	order, _, cleanup := bybitNewOpenOrder(t, exg, bybitFetchOrderTestSymbol, "fetchopenorders-limit0")
	defer cleanup()

	items := fetchBybitOpenOrdersContainEventually(t, exg, bybitFetchOrderTestSymbol, nil, order.ID)
	requireOrdersContainID(t, items, order.ID, "limit=0")

	// limit=0: bybit list APIs will use a default/max page size internally; ensure banexg wrapper still works.
	items2 := fetchBybitOpenOrdersNoError(t, exg, bybitFetchOrderTestSymbol, 0, nil)
	requireOrdersContainID(t, items2, order.ID, "limit=0")
}

func TestApi_FetchOpenOrders_SettleCoinsMulti_Dedup(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	order, _, cleanup := bybitNewOpenOrder(t, exg, bybitFetchOrderTestSymbol, "fetchopenorders-settlecoins")
	defer cleanup()

	// Use duplicates to force the multi-settleCoins code path without relying on multiple settlement coins being enabled.
	params := map[string]interface{}{
		banexg.ParamMarket:      banexg.MarketLinear,
		banexg.ParamSettleCoins: []string{"USDT", "USDT"},
	}
	items := fetchBybitOpenOrdersContainEventually(t, exg, "", params, order.ID)

	cnt := 0
	for _, od := range items {
		if od != nil && od.ID == order.ID {
			cnt++
		}
	}
	if cnt != 1 {
		t.Fatalf("expected order %s once in response, got %d occurrences", order.ID, cnt)
	}
}

func TestApi_FetchOpenOrders_NoSymbol_Linear_SettleCoinsSingle(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	order, _, cleanup := bybitNewOpenOrder(t, exg, bybitFetchOrderTestSymbol, "fetchopenorders-settlecoins1")
	defer cleanup()

	params := map[string]interface{}{
		banexg.ParamMarket:      banexg.MarketLinear,
		banexg.ParamSettleCoins: []string{"USDT"},
	}
	items := fetchBybitOpenOrdersContainEventually(t, exg, "", params, order.ID)
	requireOrdersContainID(t, items, order.ID, "settleCoins=[USDT]")
}

func TestApi_FetchOpenOrders_AfterCursor(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	od1, _, cleanup1 := bybitNewOpenOrder(t, exg, bybitFetchOrderTestSymbol, "fetchopenorders-cursor1")
	defer cleanup1()

	od2, _, cleanup2 := bybitNewOpenOrder(t, exg, bybitFetchOrderTestSymbol, "fetchopenorders-cursor2")
	defer cleanup2()

	// Ensure both orders are visible before proceeding.
	_ = fetchBybitOpenOrdersContainEventually(t, exg, bybitFetchOrderTestSymbol, nil, od1.ID, od2.ID)

	page1 := fetchBybitOpenOrdersNoError(t, exg, bybitFetchOrderTestSymbol, 1, nil)
	if len(page1) == 0 || page1[0] == nil || page1[0].ID == "" {
		t.Fatalf("expected first page order, got: %#v", page1)
	}

	cursor := bybitOpenOrdersCursorEventually(t, exg, bybitFetchOrderTestSymbol, nil)

	page2 := fetchBybitOpenOrdersNoError(t, exg, bybitFetchOrderTestSymbol, 1, map[string]interface{}{
		banexg.ParamAfter: cursor,
	})
	if len(page2) == 0 || page2[0] == nil || page2[0].ID == "" {
		t.Fatalf("expected second page order, got: %#v", page2)
	}
	if page2[0].ID == page1[0].ID {
		t.Fatalf("expected cursor page to differ from first page, got same orderId %s", page2[0].ID)
	}
}

func TestApi_FetchOpenOrders_CursorParam(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	od1, _, cleanup1 := bybitNewOpenOrder(t, exg, bybitFetchOrderTestSymbol, "fetchopenorders-cursorp1")
	defer cleanup1()

	od2, _, cleanup2 := bybitNewOpenOrder(t, exg, bybitFetchOrderTestSymbol, "fetchopenorders-cursorp2")
	defer cleanup2()

	_ = fetchBybitOpenOrdersContainEventually(t, exg, bybitFetchOrderTestSymbol, nil, od1.ID, od2.ID)
	cursor := bybitOpenOrdersCursorEventually(t, exg, bybitFetchOrderTestSymbol, nil)

	fetchBybitOpenOrdersNoError(t, exg, bybitFetchOrderTestSymbol, 1, map[string]interface{}{
		"cursor": cursor,
	})
}

func fetchBybitOrdersNoError(t *testing.T, exg *Bybit, symbol string, since int64, limit int, params map[string]interface{}) []*banexg.Order {
	t.Helper()
	items, err := exg.FetchOrders(symbol, since, limit, params)
	if err != nil {
		t.Fatalf("FetchOrders failed: %v", err)
	}
	if items == nil {
		t.Fatal("expected orders slice")
	}
	return items
}

func requireOrdersContainID(t *testing.T, items []*banexg.Order, wantID string, ctx string) {
	t.Helper()
	for _, od := range items {
		if od != nil && od.ID == wantID {
			return
		}
	}
	if ctx != "" {
		t.Fatalf("expected orderId %s in response (%s)", wantID, ctx)
	}
	t.Fatalf("expected orderId %s in response", wantID)
}

func bybitCreateCancelledPostOnlyLimitOrder(t *testing.T, exg *Bybit, symbol string, idPrefix string) (orderID string, clientOrderID string) {
	t.Helper()
	clientID := bybitTestClientOrderID(idPrefix)
	order := bybitPlacePostOnlyLimitOrder(t, exg, symbol, clientID)
	bybitCancelOrderAndWaitHistory(t, exg, symbol, order.ID, clientID)
	return order.ID, clientID
}

func bybitCancelOrderAndWaitHistory(t *testing.T, exg *Bybit, symbol string, orderID string, clientOrderID string) {
	t.Helper()

	bybitCancelOrderBestEffort(t, exg, symbol, orderID)

	// Ensure the cancelled order is queryable in /v5/order/history before returning.
	since := time.Now().Add(-30 * time.Minute).UnixMilli()
	retryBybitEventually(t, "FetchOrders (confirm cancelled order in history)", func() ([]*banexg.Order, error) {
		params := map[string]interface{}{}
		if clientOrderID != "" {
			params["orderLinkId"] = clientOrderID
		} else {
			params["orderId"] = orderID
		}
		items, err := exg.FetchOrders(symbol, since, 20, params)
		if err != nil {
			return nil, err
		}
		for _, od := range items {
			if od != nil && od.ID == orderID {
				return items, nil
			}
		}
		return nil, errNotFound
	})
}

var errNotFound = errors.New("not found")

func bybitOrderHistoryCursorEventually(t *testing.T, exg *Bybit, symbol string, since, until int64, extraArgs map[string]interface{}) string {
	t.Helper()
	tryNum := exg.GetRetryNum("FetchOrders", 1)

	return retryBybitEventually(t, "Get order history cursor", func() (string, error) {
		args, _, _, _, err := exg.loadBybitOrderArgs(symbol, extraArgs)
		if err != nil {
			return "", err
		}
		if since > 0 {
			args["startTime"] = since
		}
		if until > 0 {
			args["endTime"] = until
		}
		args["limit"] = 1

		res := requestRetry[V5ListResult](exg, MethodPrivateGetV5OrderHistory, args, tryNum)
		if res.Error != nil {
			return "", res.Error
		}
		if res.Result.NextPageCursor == "" {
			return "", errNotFound
		}
		return res.Result.NextPageCursor, nil
	})
}

func TestApi_FetchOrders_SymbolOnly_DefaultWindow(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fetchBybitOrdersNoError(t, exg, bybitFetchOrderTestSymbol, 0, 10, nil)
}

func TestApi_FetchOrders_SymbolOnly_StartTimeOnly(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	since := time.Now().Add(-2 * time.Hour).UnixMilli()
	fetchBybitOrdersNoError(t, exg, bybitFetchOrderTestSymbol, since, 10, nil)
}

func TestApi_FetchOrders_SymbolOnly_EndTimeOnly(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	until := time.Now().UnixMilli()
	fetchBybitOrdersNoError(t, exg, bybitFetchOrderTestSymbol, 0, 10, map[string]interface{}{
		banexg.ParamUntil: until,
	})
}

func TestApi_FetchOrders_NoSymbol_MarketTypeOnly(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fetchBybitOrdersNoError(t, exg, "", 0, 5, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
}

func TestApi_FetchOrders_NoSymbol_BaseCoin(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fetchBybitOrdersNoError(t, exg, "", 0, 5, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
		"baseCoin":         "XRP",
	})
}

func TestApi_FetchOrders_NoSymbol_SettleCoin(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fetchBybitOrdersNoError(t, exg, "", 0, 5, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
		"settleCoin":       "USDT",
	})
}

func TestApi_FetchOrders_ByOrderID(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	orderID, _ := bybitCreateCancelledPostOnlyLimitOrder(t, exg, bybitFetchOrderTestSymbol, "fetchorders-oid")

	since := time.Now().Add(-2 * time.Hour).UnixMilli()
	items := fetchBybitOrdersNoError(t, exg, bybitFetchOrderTestSymbol, since, 20, map[string]interface{}{
		"orderId": orderID,
	})
	requireOrdersContainID(t, items, orderID, "orderId")
}

func TestApi_FetchOrders_ByOrderLinkID(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	orderID, clientID := bybitCreateCancelledPostOnlyLimitOrder(t, exg, bybitFetchOrderTestSymbol, "fetchorders-olid")

	since := time.Now().Add(-2 * time.Hour).UnixMilli()
	items := fetchBybitOrdersNoError(t, exg, bybitFetchOrderTestSymbol, since, 20, map[string]interface{}{
		"orderLinkId": clientID,
	})
	requireOrdersContainID(t, items, orderID, "orderLinkId="+clientID)
}

func TestApi_FetchOrders_ByClientOrderID(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	orderID, clientID := bybitCreateCancelledPostOnlyLimitOrder(t, exg, bybitFetchOrderTestSymbol, "fetchorders-cloid")

	since := time.Now().Add(-2 * time.Hour).UnixMilli()
	items := fetchBybitOrdersNoError(t, exg, bybitFetchOrderTestSymbol, since, 20, map[string]interface{}{
		banexg.ParamClientOrderId: clientID,
	})
	requireOrdersContainID(t, items, orderID, "clientOrderId="+clientID)
}

func TestApi_FetchOrders_LimitZero_WithOrderID(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	orderID, _ := bybitCreateCancelledPostOnlyLimitOrder(t, exg, bybitFetchOrderTestSymbol, "fetchorders-limit0")

	// limit=0: bybit list APIs will use a default/max page size internally; ensure banexg wrapper still works.
	since := time.Now().Add(-2 * time.Hour).UnixMilli()
	items := fetchBybitOrdersNoError(t, exg, bybitFetchOrderTestSymbol, since, 0, map[string]interface{}{
		"orderId": orderID,
	})
	requireOrdersContainID(t, items, orderID, "limit=0 orderId")
}

func TestApi_FetchOrders_OrderFilter(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fetchBybitOrdersNoError(t, exg, bybitFetchOrderTestSymbol, 0, 10, map[string]interface{}{
		"orderFilter": "Order",
	})
}

func TestApi_FetchOrders_OrderStatus(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fetchBybitOrdersNoError(t, exg, bybitFetchOrderTestSymbol, 0, 10, map[string]interface{}{
		"orderStatus": "Filled",
	})
}

func TestApi_FetchOrders_AfterCursor(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	// Ensure there are at least two Cancelled orders within the time window so nextPageCursor exists.
	_, _ = bybitCreateCancelledPostOnlyLimitOrder(t, exg, bybitFetchOrderTestSymbol, "fetchorders-cursor1")
	_, _ = bybitCreateCancelledPostOnlyLimitOrder(t, exg, bybitFetchOrderTestSymbol, "fetchorders-cursor2")

	now := time.Now().UTC()
	since := now.Add(-30 * time.Minute).UnixMilli()
	until := now.UnixMilli()
	cursor := bybitOrderHistoryCursorEventually(t, exg, bybitFetchOrderTestSymbol, since, until, map[string]interface{}{
		"orderStatus": "Cancelled",
	})

	fetchBybitOrdersNoError(t, exg, bybitFetchOrderTestSymbol, since, 1, map[string]interface{}{
		banexg.ParamUntil: until,
		banexg.ParamAfter: cursor,
		// Keep the same filters used to obtain the cursor.
		"orderStatus": "Cancelled"})
}

const (
	bybitOrderRetryCount = 5
	bybitOrderRetryWait  = 800 * time.Millisecond
)

func retryBybitEventually[T any](t *testing.T, action string, fn func() (T, error)) T {
	t.Helper()
	var lastErr error
	for i := 0; i < bybitOrderRetryCount; i++ {
		result, err := fn()
		if err == nil {
			return result
		}
		lastErr = err
		time.Sleep(bybitOrderRetryWait)
	}
	t.Fatalf("%s failed after retries: %v", action, lastErr)
	var zero T
	return zero
}

func fetchBybitOrderEventually(t *testing.T, exg *Bybit, symbol, orderID string) *banexg.Order {
	return fetchBybitOrderWithParamsEventually(t, exg, symbol, orderID, nil)
}

func fetchBybitOrderWithParamsEventually(t *testing.T, exg *Bybit, symbol, orderID string, params map[string]interface{}) *banexg.Order {
	return retryBybitEventually(t, "FetchOrder", func() (*banexg.Order, error) {
		order, err := exg.FetchOrder(symbol, orderID, params)
		if err != nil {
			return nil, err
		}
		if order == nil {
			return nil, errors.New("empty order")
		}
		return order, nil
	})
}

func fetchBybitOrdersContainEventually(t *testing.T, exg *Bybit, symbol, orderID string, since int64) []*banexg.Order {
	return retryBybitEventually(t, "FetchOrders", func() ([]*banexg.Order, error) {
		orders, err := exg.FetchOrders(symbol, since, 50, nil)
		if err != nil {
			return nil, err
		}
		for _, od := range orders {
			if od.ID == orderID {
				return orders, nil
			}
		}
		return nil, errors.New("order not found in history")
	})
}

func fetchBybitTradesForOrderEventually(t *testing.T, exg *Bybit, symbol, orderID string, since int64) []*banexg.MyTrade {
	return retryBybitEventually(t, "FetchMyTrades", func() ([]*banexg.MyTrade, error) {
		trades, err := exg.FetchMyTrades(symbol, since, 50, nil)
		if err != nil {
			return nil, err
		}
		for _, tr := range trades {
			if tr.Order == orderID {
				return trades, nil
			}
		}
		return nil, errors.New("order not found in trades")
	})
}

func TestApi_OrderLifecycleSpot(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	if _, err := exg.LoadMarkets(false, nil); err != nil {
		t.Fatalf("LoadMarkets failed: %v", err)
	}

	symbol := "XRP/USDT:USDT"
	ticker, err := exg.FetchTicker(symbol, nil)
	if err != nil {
		t.Fatalf("FetchTicker failed: %v", err)
	}
	price := ticker.Last
	if price == 0 {
		price = ticker.Ask
	}
	if price == 0 {
		price = ticker.Bid
	}
	if price == 0 {
		t.Fatalf("ticker price is empty")
	}

	buyCost := 10.0
	buyAmount := buyCost / price
	buyOrder, err := exg.CreateOrder(symbol, banexg.OdTypeMarket, banexg.OdSideBuy, buyAmount, 0, nil)
	if err != nil {
		t.Fatalf("CreateOrder market buy failed: %v", err)
	}
	buyDetail := fetchBybitOrderEventually(t, exg, symbol, buyOrder.ID)
	if buyDetail.Filled <= 0 {
		t.Fatalf("expected filled amount, got %v", buyDetail.Filled)
	}
	buyTimestamp := buyDetail.Timestamp
	if buyTimestamp == 0 {
		buyTimestamp = time.Now().Add(-2 * time.Minute).UnixMilli()
	}
	limitPrice := price * 1.02

	sellOrder, err := exg.CreateOrder(symbol, banexg.OdTypeLimit, banexg.OdSideSell, buyDetail.Filled, limitPrice, nil)
	if err != nil {
		t.Fatalf("CreateOrder limit sell failed: %v", err)
	}
	sellDetail := fetchBybitOrderEventually(t, exg, symbol, sellOrder.ID)

	if sellDetail.Status == banexg.OdStatusOpen || sellDetail.Status == banexg.OdStatusPartFilled {
		newPrice := limitPrice * 1.01
		if _, err := exg.EditOrder(symbol, sellDetail.ID, banexg.OdSideSell, 0, newPrice, nil); err != nil {
			t.Fatalf("EditOrder failed: %v", err)
		}
		if _, err := exg.CancelOrder(sellDetail.ID, symbol, nil); err != nil {
			t.Logf("CancelOrder skipped: %v", err)
		}
	} else {
		t.Logf("sell order status is %s; skip amend/cancel", sellDetail.Status)
	}

	if _, err := exg.FetchOpenOrders(symbol, 0, 5, nil); err != nil {
		t.Fatalf("FetchOpenOrders failed: %v", err)
	}

	since := buyTimestamp - int64(2*time.Minute/time.Millisecond)
	fetchBybitOrdersContainEventually(t, exg, symbol, buyOrder.ID, since)
	fetchBybitTradesForOrderEventually(t, exg, symbol, buyOrder.ID, since)
}

func TestApi_FetchOrderHistory(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	items, err := exg.FetchOrders("XRP/USDT:USDT", 0, 50, nil)
	if err != nil {
		t.Fatalf("FetchOrders failed: %v", err)
	}
	if items == nil {
		t.Fatal("expected order items slice")
	}
	log.Info("order history", zap.Any("data", items))
}

func TestApi_FetchClosedPnlHistory(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	args := map[string]interface{}{
		"category": "linear",
	}
	tryNum := exg.GetRetryNum("FetchClosedPnl", 1)
	res := requestRetry[V5ListResult](exg, MethodPrivateGetV5PositionClosedPnl, args, tryNum)
	if res.Error != nil {
		t.Fatalf("FetchClosedPnl failed: %v", res.Error)
	}
	if res.Result.List == nil {
		t.Fatal("expected closed pnl items slice")
	}
	log.Info("closed pnl history", zap.Int("count", len(res.Result.List)), zap.Any("data", res.Result.List))
}

func TestApi_FetchIncomeHistory(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	items, err := exg.FetchIncomeHistory("", "", 0, 50, nil)
	if err != nil {
		t.Fatalf("FetchIncomeHistory failed: %v", err)
	}
	if items == nil {
		t.Fatal("expected income items slice")
	} else {
		log.Info("income history", zap.Any("data", items))
	}
}

func TestApi_FetchOrdersTimeRange(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	now := time.Now().UTC()
	since := now.Add(-2 * time.Hour).UnixMilli()
	until := now.UnixMilli()
	items, err := exg.FetchOrders("BTC/USDT:USDT", since, 10, map[string]interface{}{
		banexg.ParamUntil: until,
	})
	if err != nil {
		t.Fatalf("FetchOrders time range failed: %v", err)
	}
	if items == nil {
		t.Fatal("expected orders slice")
	}
}

func TestApi_FetchMyTradesTimeRange(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	now := time.Now().UTC()
	since := now.Add(-2 * time.Hour).UnixMilli()
	until := now.UnixMilli()
	items, err := exg.FetchMyTrades("BTC/USDT:USDT", since, 10, map[string]interface{}{
		banexg.ParamUntil: until,
	})
	if err != nil {
		t.Fatalf("FetchMyTrades time range failed: %v", err)
	}
	if items == nil {
		t.Fatal("expected trades slice")
	}
}
