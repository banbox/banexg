package okx

import (
	"strconv"
	"strings"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

func setOrderID(args map[string]interface{}, orderId string) *errs.Error {
	clientOrderId := utils.PopMapVal(args, banexg.ParamClientOrderId, "")
	if orderId != "" {
		args[FldOrdId] = orderId
		return nil
	}
	if clientOrderId != "" {
		args[FldClOrdId] = clientOrderId
		return nil
	}
	return errs.NewMsg(errs.CodeParamRequired, "order id or clientOrderId required")
}

func parseOrders(e *OKX, items []map[string]interface{}, marketType, symbol string) ([]*banexg.Order, *errs.Error) {
	arr, err := decodeResult[Order](items)
	if err != nil {
		return nil, err
	}
	result := make([]*banexg.Order, 0, len(arr))
	for i, item := range arr {
		order := parseOrder(e, &item, items[i], marketType)
		if order == nil {
			continue
		}
		if symbol != "" && order.Symbol != symbol {
			continue
		}
		result = append(result, order)
	}
	return result, nil
}

func pickOrdersHistoryMethod(args map[string]interface{}, since, until int64) string {
	return pickArchiveMethod(args, since, until, MethodTradeGetOrdersHistory, MethodTradeGetOrdersHistoryArchive)
}

func (e *OKX) CreateOrder(symbol, odType, side string, amount, price float64, params map[string]interface{}) (*banexg.Order, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	triggerPrice := utils.PopMapVal(args, banexg.ParamTriggerPrice, float64(0))
	stopLossPrice := utils.PopMapVal(args, banexg.ParamStopLossPrice, float64(0))
	if stopLossPrice == 0 {
		stopLossPrice = triggerPrice
	}
	takeProfitPrice := utils.PopMapVal(args, banexg.ParamTakeProfitPrice, float64(0))
	algoOrder := utils.PopMapVal(args, banexg.ParamAlgoOrder, false)
	ordType, ok := orderTypeMap[odType]
	if !ok {
		ordType = odType
	}
	if ordType == "limit" {
		tif := utils.PopMapVal(args, banexg.ParamTimeInForce, "")
		switch tif {
		case banexg.TimeInForceIOC:
			ordType = "ioc"
		case banexg.TimeInForceFOK:
			ordType = "fok"
		case banexg.TimeInForcePO:
			ordType = "post_only"
		}
	}
	args[FldInstId] = market.ID
	args[FldSide] = strings.ToLower(side)
	args[FldOrdType] = ordType

	if market.Type == banexg.MarketSpot {
		args[FldTdMode] = TdModeCash
	} else {
		mgnMode := utils.PopMapVal(args, banexg.ParamMarginMode, banexg.MarginCross)
		args[FldTdMode] = mgnMode
	}
	if clOrdId := utils.PopMapVal(args, banexg.ParamClientOrderId, ""); clOrdId != "" {
		if !validateClOrdId(clOrdId) {
			return nil, errs.NewMsg(errs.CodeParamInvalid, "clOrdId must be 1-32 alphanumeric characters")
		}
		args[FldClOrdId] = clOrdId
	}
	if reduceOnly := utils.PopMapVal(args, banexg.ParamReduceOnly, false); reduceOnly {
		args[FldReduceOnly] = true
	}
	if market.Spot {
		if tradeQuoteCcy := getTradeQuoteCcy(market); tradeQuoteCcy != "" {
			args[FldTradeQuoteCcy] = tradeQuoteCcy
		}
	}
	if market.Contract {
		posSide := utils.PopMapVal(args, banexg.ParamPositionSide, "")
		if posSide == "" {
			return nil, errs.NewMsg(errs.CodeParamRequired, "positionSide required for contract order")
		}
		args[FldPosSide] = strings.ToLower(posSide)
	}
	if ordType == "market" && market.Spot {
		cost := utils.PopMapVal(args, banexg.ParamCost, 0.0)
		if cost > 0 {
			args[FldTgtCcy] = TgtCcyQuote
			args[FldSz] = strconv.FormatFloat(cost, 'f', -1, 64)
		} else {
			args[FldSz] = strconv.FormatFloat(amount, 'f', -1, 64)
		}
	} else {
		args[FldSz] = strconv.FormatFloat(amount, 'f', -1, 64)
	}
	if price > 0 && ordType != "market" {
		args[FldPx] = strconv.FormatFloat(price, 'f', -1, 64)
	}

	if algoOrder || isAlgoOrderType(odType) || stopLossPrice != 0 || takeProfitPrice != 0 {
		return e.createAlgoOrder(market, odType, side, amount, price, args, stopLossPrice, takeProfitPrice)
	}

	tryNum := e.GetRetryNum("CreateOrder", 1)
	res := requestRetry[[]OrderResult](e, MethodTradePostOrder, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	if len(res.Result) == 0 {
		return nil, errs.NewMsg(errs.CodeDataNotFound, "empty order result")
	}
	ord := res.Result[0]
	if ord.SCode != "0" {
		return nil, errs.NewMsg(errs.CodeRunTime, "[%s] %s", ord.SCode, ord.SMsg)
	}
	return &banexg.Order{
		ID:            ord.OrdId,
		ClientOrderID: ord.ClOrdId,
		Symbol:        symbol,
		Type:          odType,
		Side:          side,
		Amount:        amount,
		Price:         price,
		Status:        banexg.OdStatusOpen,
	}, nil
}

func (e *OKX) EditOrder(symbol, orderId, side string, amount, price float64, params map[string]interface{}) (*banexg.Order, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	args[FldInstId] = market.ID
	if err := setOrderID(args, orderId); err != nil {
		return nil, err
	}
	if amount > 0 {
		args[FldNewSz] = strconv.FormatFloat(amount, 'f', -1, 64)
	}
	if price > 0 {
		args[FldNewPx] = strconv.FormatFloat(price, 'f', -1, 64)
	}
	tryNum := e.GetRetryNum("EditOrder", 1)
	res := requestRetry[[]OrderResult](e, MethodTradePostAmendOrder, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	if len(res.Result) == 0 {
		return nil, errs.NewMsg(errs.CodeDataNotFound, "empty amend result")
	}
	item := res.Result[0]
	if item.SCode != "0" {
		return nil, errs.NewMsg(errs.CodeRunTime, "[%s] %s", item.SCode, item.SMsg)
	}
	return &banexg.Order{
		ID:            item.OrdId,
		ClientOrderID: item.ClOrdId,
		Symbol:        symbol,
		Side:          side,
		Amount:        amount,
		Price:         price,
		Status:        banexg.OdStatusOpen,
	}, nil
}

func (e *OKX) CancelOrder(id string, symbol string, params map[string]interface{}) (*banexg.Order, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	algoOrder := utils.PopMapVal(args, banexg.ParamAlgoOrder, false)
	if algoOrder || strings.HasPrefix(id, "algo:") {
		return e.cancelAlgoOrder(id, market, args)
	}
	args[FldInstId] = market.ID
	if err := setOrderID(args, id); err != nil {
		return nil, err
	}
	tryNum := e.GetRetryNum("CancelOrder", 1)
	res := requestRetry[[]OrderResult](e, MethodTradePostCancelOrder, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	if len(res.Result) == 0 {
		return nil, errs.NewMsg(errs.CodeDataNotFound, "empty cancel result")
	}
	item := res.Result[0]
	if item.SCode != "0" {
		return nil, errs.NewMsg(errs.CodeRunTime, "[%s] %s", item.SCode, item.SMsg)
	}
	return &banexg.Order{
		ID:            item.OrdId,
		ClientOrderID: item.ClOrdId,
		Symbol:        symbol,
		Status:        banexg.OdStatusCanceled,
	}, nil
}

func (e *OKX) FetchOrder(symbol, id string, params map[string]interface{}) (*banexg.Order, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	algoOrder := utils.PopMapVal(args, banexg.ParamAlgoOrder, false)
	if algoOrder || strings.HasPrefix(id, "algo:") {
		return e.fetchAlgoOrder(id, market, args)
	}
	args[FldInstId] = market.ID
	if err := setOrderID(args, id); err != nil {
		return nil, err
	}
	tryNum := e.GetRetryNum("FetchOrder", 1)
	res := requestRetry[[]map[string]interface{}](e, MethodTradeGetOrder, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	if len(res.Result) == 0 {
		return nil, errs.NewMsg(errs.CodeDataNotFound, "empty order result")
	}
	arr, err := decodeResult[Order](res.Result)
	if err != nil {
		return nil, err
	}
	return parseOrder(e, &arr[0], res.Result[0], market.Type), nil
}

func (e *OKX) FetchOpenOrders(symbol string, since int64, limit int, params map[string]interface{}) ([]*banexg.Order, *errs.Error) {
	args := utils.SafeParams(params)
	algoOrder := utils.PopMapVal(args, banexg.ParamAlgoOrder, false)
	marketType := ""
	contractType := ""
	var err *errs.Error
	var market *banexg.Market
	if symbol != "" {
		_, market, err = e.LoadArgsMarket(symbol, args)
		if err != nil {
			return nil, err
		}
		args[FldInstId] = market.ID
		marketType = market.Type
	}
	if marketType == "" {
		marketType, contractType, err = e.LoadArgsMarketType(args)
		if err != nil {
			return nil, err
		}
		instType := instTypeByMarket(marketType, contractType)
		if instType != "" {
			args[FldInstType] = instType
		}
	}
	if limit > 0 {
		if limit > 100 {
			limit = 100
		}
		args[FldLimit] = strconv.Itoa(limit)
	}
	tryNum := e.GetRetryNum("FetchOpenOrders", 1)
	method := MethodTradeGetOrdersPending
	if algoOrder {
		method = MethodTradeGetOrdersAlgoPending
	}
	res := requestRetry[[]map[string]interface{}](e, method, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	if algoOrder {
		return parseAlgoOrders(e, res.Result, marketType, symbol)
	}
	return parseOrders(e, res.Result, marketType, symbol)
}

func (e *OKX) FetchOrders(symbol string, since int64, limit int, params map[string]interface{}) ([]*banexg.Order, *errs.Error) {
	args := utils.SafeParams(params)
	algoOrder := utils.PopMapVal(args, banexg.ParamAlgoOrder, false)
	marketType := ""
	contractType := ""
	var err *errs.Error
	var market *banexg.Market
	if symbol != "" {
		args, market, err = e.LoadArgsMarket(symbol, args)
		if err != nil {
			return nil, err
		}
		args[FldInstId] = market.ID
		marketType = market.Type
		if instType := instTypeFromMarket(market); instType != "" {
			args[FldInstType] = instType
		}
	}
	if marketType == "" {
		marketType, contractType, err = e.LoadArgsMarketType(args)
		if err != nil {
			return nil, err
		}
		instType := instTypeByMarket(marketType, contractType)
		if instType == "" {
			return nil, errs.NewMsg(errs.CodeParamInvalid, "unsupported market: %v", marketType)
		}
		args[FldInstType] = instType
	}
	if limit > 0 {
		if limit > 100 {
			limit = 100
		}
		args[FldLimit] = strconv.Itoa(limit)
	}
	until := utils.PopMapVal(args, banexg.ParamUntil, int64(0))
	if since > 0 {
		args[FldBegin] = strconv.FormatInt(since, 10)
	}
	if until > 0 {
		args[FldEnd] = strconv.FormatInt(until, 10)
	}
	method := pickOrdersHistoryMethod(args, since, until)
	if algoOrder {
		method = MethodTradeGetOrdersAlgoHistory
	}
	tryNum := e.GetRetryNum("FetchOrders", 1)
	res := requestRetry[[]map[string]interface{}](e, method, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	if algoOrder {
		return parseAlgoOrders(e, res.Result, marketType, symbol)
	}
	return parseOrders(e, res.Result, marketType, symbol)
}

func parseOrder(e *OKX, item *Order, info map[string]interface{}, marketType string) *banexg.Order {
	if item == nil {
		return nil
	}
	if info == nil {
		info = map[string]interface{}{}
	}
	status := mapOrderStatus(item.State)
	orderType, tif, postOnly := mapOrderType(item.OrdType)
	price := parseFloat(item.Px)
	avgPx := parseFloat(item.AvgPx)
	amount := parseFloat(item.Sz)
	filled := parseFloat(item.AccFillSz)
	remaining := 0.0
	if amount > 0 {
		remaining = amount - filled
	}
	ts := parseInt(item.CTime)
	lastUpdate := parseInt(item.UTime)
	lastTrade := int64(0)
	if filled > 0 {
		if lastUpdate > 0 {
			lastTrade = lastUpdate
		} else {
			lastTrade = ts
		}
	}
	symbol := item.InstId
	if market := getMarketByIDAny(e, item.InstId, marketType); market != nil {
		symbol = market.Symbol
	}
	feeCost := parseFloat(item.Fee)
	var fee *banexg.Fee
	if feeCost != 0 {
		fee = &banexg.Fee{
			Currency: item.FeeCcy,
			Cost:     feeCost,
		}
	}
	reduceOnly := false
	if val, ok := info["reduceOnly"]; ok {
		switch v := val.(type) {
		case bool:
			reduceOnly = v
		case string:
			reduceOnly = parseBoolStr(v)
		}
	}
	return &banexg.Order{
		Info:                info,
		ID:                  item.OrdId,
		ClientOrderID:       item.ClOrdId,
		Timestamp:           ts,
		LastTradeTimestamp:  lastTrade,
		LastUpdateTimestamp: lastUpdate,
		Status:              status,
		Symbol:              symbol,
		Type:                orderType,
		TimeInForce:         tif,
		PositionSide:        strings.ToLower(item.PosSide),
		Side:                strings.ToLower(item.Side),
		Price:               price,
		Average:             avgPx,
		Amount:              amount,
		Filled:              filled,
		Remaining:           remaining,
		PostOnly:            postOnly,
		ReduceOnly:          reduceOnly,
		Fee:                 fee,
	}
}

func mapOrderStatus(status string) string {
	if val, ok := orderStatusMap[status]; ok {
		return val
	}
	return status
}

func mapOrderType(ordType string) (string, string, bool) {
	switch ordType {
	case "fok":
		return banexg.OdTypeLimit, banexg.TimeInForceFOK, false
	case "ioc":
		return banexg.OdTypeLimit, banexg.TimeInForceIOC, false
	case "post_only":
		return banexg.OdTypeLimitMaker, banexg.TimeInForcePO, true
	default:
		if v, ok := okxOrderTypeMap[ordType]; ok {
			return v, "", ordType == "post_only"
		}
	}
	return ordType, "", false
}
