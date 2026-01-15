package binance

import (
	"strings"
	"testing"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"github.com/banbox/bntp"
	"go.uber.org/zap"
)

func getPosition(t *testing.T) (*Binance, *banexg.Position) {
	bntp.LangCode = bntp.LangZhCN
	exg := getBinance(nil)

	pos, err := exg.FetchAccountPositions(nil, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
	if err != nil {
		panic(err)
	}
	if len(pos) == 0 {
		t.Fatal("please create position first")
	}
	return exg, pos[0]
}

// TestCreateAlgoOrder 测试创建策略单
// 对应接口: POST /fapi/v1/algoOrder
func TestCreateAlgoOrder(t *testing.T) {
	exg, pos := getPosition(t)
	symbol := pos.Symbol
	quantity := pos.Contracts
	curPrice := pos.Notional / quantity
	log.Info("get positions", zap.String("pair", symbol), zap.Float64("price", curPrice), zap.Float64("quantity", quantity))

	// 1. 测试创建止损单 (STOP_MARKET)
	slPrice := curPrice * 0.8
	tpPrice := curPrice * 1.2
	args := map[string]interface{}{
		banexg.ParamStopLossPrice: slPrice,
		banexg.ParamPositionSide:  strings.ToUpper(banexg.PosSideLong),
	}
	// CreateOrder 内部会根据 Linear 市场和 OrderType 自动路由到 createAlgoOrder
	res, err := exg.CreateOrder(symbol, banexg.OdTypeStopMarket, banexg.OdSideSell, quantity, 0, args)
	if err != nil {
		t.Fatalf("Create STOP_MARKET Order Failed: %v", err)
	}
	resStr, _ := utils.MarshalString(res)
	t.Logf("Create STOP_MARKET Result: %s", resStr)

	// 2. 测试创建止盈单 (TAKE_PROFIT_MARKET)
	args2 := map[string]interface{}{
		banexg.ParamTakeProfitPrice: tpPrice,
		banexg.ParamPositionSide:    strings.ToUpper(banexg.PosSideLong),
	}
	res2, err := exg.CreateOrder(symbol, banexg.OdTypeTakeProfitMarket, banexg.OdSideSell, quantity, 0, args2)
	if err != nil {
		t.Fatalf("Create TAKE_PROFIT_MARKET Order Failed: %v", err)
	}
	resStr2, _ := utils.MarshalString(res2)
	t.Logf("Create TAKE_PROFIT_MARKET Result: %s", resStr2)
}

// TestFetchOpenAlgoOrders 测试查询当前挂单的策略单
// 对应接口: GET /fapi/v1/openAlgoOrders
func TestFetchOpenAlgoOrders(t *testing.T) {
	exg, pos := getPosition(t)
	symbol := pos.Symbol

	args := map[string]interface{}{
		banexg.ParamAlgoOrder: true, // 关键参数，指示查询策略单
	}

	res, err := exg.FetchOpenOrders(symbol, 0, 0, args)
	if err != nil {
		t.Fatalf("Fetch Open Algo Orders Failed: %v", err)
	}
	resStr, _ := utils.MarshalString(res)
	t.Logf("Fetch Open Algo Orders Result: %s", resStr)

	if len(res) > 0 {
		// 测试获取条件单
		algoRes, err := exg.FetchOrder(symbol, res[0].ID, args)
		if err != nil {
			t.Logf("Fetch Algo Order Failed (Might be invalid ID): %v", err)
		} else {
			resStr, _ = utils.MarshalString(algoRes)
			t.Logf("Fetch Algo Order Result: %s", resStr)
		}
	}
}

// TestFetchAlgoOrdersHistory 测试查询策略单历史
// 对应接口: GET /fapi/v1/allAlgoOrders
func TestFetchAlgoOrdersHistory(t *testing.T) {
	exg, pos := getPosition(t)
	symbol := pos.Symbol

	args := map[string]interface{}{
		banexg.ParamAlgoOrder: true, // 关键参数，指示查询策略单
		// banexg.ParamLimit: 10,
	}

	res, err := exg.FetchOrders(symbol, 0, 10, args)
	if err != nil {
		t.Fatalf("Fetch Algo Orders History Failed: %v", err)
	}
	resStr, _ := utils.MarshalString(res)
	t.Logf("Fetch Algo Orders History Result: %s", resStr)

	if len(res) > 0 {
		// 测试获取历史单详情
		algoRes, err := exg.FetchOrder(symbol, res[0].ID, args)
		if err != nil {
			t.Logf("Fetch Algo Order History Detail Failed: %v", err)
		} else {
			resStr, _ = utils.MarshalString(algoRes)
			t.Logf("Fetch Algo Order History Detail Result: %s", resStr)
		}
	}
}

// TestCancelAlgoOrder 测试撤销策略单
// 对应接口: DELETE /fapi/v1/algoOrder
func TestCancelAlgoOrder(t *testing.T) {
	exg, pos := getPosition(t)
	symbol := pos.Symbol
	quantity := pos.Contracts
	curPrice := pos.Notional / quantity

	// 1. 先创建一个策略单
	slPrice := curPrice * 0.8
	createArgs := map[string]interface{}{
		banexg.ParamStopLossPrice: slPrice,
		banexg.ParamPositionSide:  strings.ToUpper(banexg.PosSideLong),
	}
	order, err := exg.CreateOrder(symbol, banexg.OdTypeStopMarket, banexg.OdSideSell, quantity, 0, createArgs)
	if err != nil {
		t.Fatalf("Setup: Create Order Failed: %v", err)
	}
	algoId := order.ID
	t.Logf("Setup: Created order %s to cancel", algoId)

	// 2. 撤销该策略单
	args := map[string]interface{}{
		banexg.ParamAlgoOrder: true, // 关键参数，指示操作策略单
	}

	res, err := exg.CancelOrder(algoId, symbol, args)
	if err != nil {
		t.Logf("Cancel Algo Order Failed: %v", err)
	} else {
		resStr, _ := utils.MarshalString(res)
		t.Logf("Cancel Algo Order Result: %s", resStr)
	}
}

// TestAlgoOrderLifecycle 综合测试：下单 -> 查询挂单 -> 撤单 -> 查询历史
func TestAlgoOrderLifecycle(t *testing.T) {
	exg, pos := getPosition(t)
	symbol := pos.Symbol
	quantity := pos.Contracts
	curPrice := pos.Notional / quantity

	// 1. 下单
	slPrice := curPrice * 0.9
	createArgs := map[string]interface{}{
		banexg.ParamStopLossPrice: slPrice,
		banexg.ParamPositionSide:  strings.ToUpper(banexg.PosSideLong),
	}
	order, err := exg.CreateOrder(symbol, banexg.OdTypeStopMarket, banexg.OdSideSell, quantity, 0, createArgs)
	if err != nil {
		t.Fatalf("Lifecycle: Create Order Failed: %v", err)
	}
	resStr, _ := utils.MarshalString(order.Info)
	t.Logf("Lifecycle: Created Order ID: %s, price: %f, res: %s", order.ID, slPrice, resStr)

	algoId := order.ID

	// 2. 查询 Open Orders 确认存在
	openArgs := map[string]interface{}{
		banexg.ParamAlgoOrder: true,
	}
	openOrders, err := exg.FetchOpenOrders(symbol, 0, 0, openArgs)
	if err != nil {
		t.Fatalf("Lifecycle: Fetch Open Orders Failed: %v", err)
	}
	found := false
	for _, o := range openOrders {
		if o.ID == algoId {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Lifecycle: Order %s not found in %d open orders", algoId, len(openOrders))
	} else {
		t.Logf("Lifecycle: Order %s found in open orders", algoId)
	}

	// 3. 查询单个订单详情
	fetchArgs := map[string]interface{}{
		banexg.ParamAlgoOrder: true,
	}
	fetchedOrder, err := exg.FetchOrder(symbol, algoId, fetchArgs)
	if err != nil {
		t.Fatalf("Lifecycle: Fetch Order Failed: %v", err)
	}
	if fetchedOrder.ID != algoId {
		t.Errorf("Lifecycle: Fetched Order ID mismatch, got %s want %s", fetchedOrder.ID, algoId)
	}
	t.Logf("Lifecycle: Fetched Order Details confirmed")

	// 4. 撤单
	cancelArgs := map[string]interface{}{
		banexg.ParamAlgoOrder: true,
	}
	cancelRes, err := exg.CancelOrder(algoId, symbol, cancelArgs)
	if err != nil {
		t.Fatalf("Lifecycle: Cancel Order Failed: %v", err)
	}
	t.Logf("Lifecycle: Cancelled Order %s, Status: %s", cancelRes.ID, cancelRes.Status)

	// 5. 查询历史订单确认状态 (可能需要稍微等待一点时间)
	// time.Sleep(time.Second)
	// historyArgs := map[string]interface{}{
	// 	banexg.ParamAlgoOrder: true,
	// 	banexg.ParamLimit: 20,
	// }
	// historyOrders, err := exg.FetchOrders(symbol, 0, 20, historyArgs)
	// if err != nil {
	// 	t.Fatalf("Lifecycle: Fetch History Failed: %v", err)
	// }
	// foundHistory := false
	// for _, o := range historyOrders {
	// 	if o.ID == algoId {
	// 		foundHistory = true
	// 		t.Logf("Lifecycle: Order found in history with status: %s", o.Status)
	// 		break
	// 	}
	// }
	// if !foundHistory {
	// 	t.Logf("Lifecycle: Order %s not found in history (might be delay)", algoId)
	// }
}
