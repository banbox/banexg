package binance

import (
	"context"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
	"strconv"
	"strings"
)

func (e *Binance) FetchOrder(symbol, orderId string, params map[string]interface{}) (*banexg.Order, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	args["symbol"] = market.ID
	args["orderId"] = orderId
	marginMode := utils.PopMapVal(args, banexg.ParamMarginMode, "")
	method := MethodPrivateGetOrder
	if market.Option {
		method = MethodEapiPrivateGetOrder
	} else if market.Linear {
		method = MethodFapiPrivateGetOrder
	} else if market.Inverse {
		method = MethodDapiPrivateGetOrder
	} else if market.Type == banexg.MarketMargin || marginMode != "" {
		method = MethodSapiGetMarginOrder
		if marginMode == "isolated" {
			args["isIsolated"] = true
		}
	}
	clientOrderId := utils.PopMapVal(args, banexg.ParamClientOrderId, "")
	if clientOrderId != "" {
		if market.Option {
			args["clientOrderId"] = clientOrderId
		} else {
			args["origClientOrderId"] = clientOrderId
		}
	}
	tryNum := e.GetRetryNum("FetchOrder", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	var mapSymbol = func(mid string) string {
		return market.Symbol
	}
	switch method {
	case MethodPrivateGetOrder:
		return parseOrder[*SpotOrder](mapSymbol, rsp)
	case MethodEapiPrivateGetOrder:
		return parseOrder[*OptionOrder](mapSymbol, rsp)
	case MethodFapiPrivateGetOrder:
		return parseOrder[*FutureOrder](mapSymbol, rsp)
	case MethodDapiPrivateGetOrder:
		return parseOrder[*InverseOrder](mapSymbol, rsp)
	case MethodSapiGetMarginOrder:
		return parseOrder[*MarginOrder](mapSymbol, rsp)
	default:
		return nil, errs.NewMsg(errs.CodeNotSupport, "not support order method %s", method)
	}
}

/*
FetchOrders 获取自己的订单
symbol: 必填，币种
*/
func (e *Binance) FetchOrders(symbol string, since int64, limit int, params map[string]interface{}) ([]*banexg.Order, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	args["symbol"] = market.ID
	marginMode := utils.PopMapVal(args, banexg.ParamMarginMode, "")
	method := MethodPrivateGetAllOrders
	if market.Option {
		method = MethodEapiPrivateGetHistoryOrders
	} else if market.Linear {
		method = MethodFapiPrivateGetAllOrders
	} else if market.Inverse {
		method = MethodDapiPrivateGetAllOrders
	} else if market.Type == banexg.MarketMargin || marginMode != "" {
		method = MethodSapiGetMarginAllOrders
		if marginMode == "isolated" {
			args["isIsolated"] = true
		}
	}
	until := utils.PopMapVal(args, banexg.ParamUntil, int64(0))
	if until > 0 {
		args["endTime"] = until
	}
	if since > 0 {
		args["startTime"] = since
	}
	if limit > 0 {
		args["limit"] = limit
	}
	tryNum := e.GetRetryNum("FetchOrders", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	var mapSymbol = func(mid string) string {
		return market.Symbol
	}
	switch method {
	case MethodPrivateGetAllOrders:
		return parseOrders[*SpotOrder](mapSymbol, rsp)
	case MethodEapiPrivateGetHistoryOrders:
		return parseOrders[*OptionOrder](mapSymbol, rsp)
	case MethodFapiPrivateGetAllOrders:
		return parseOrders[*FutureOrder](mapSymbol, rsp)
	case MethodDapiPrivateGetAllOrders:
		return parseOrders[*InverseOrder](mapSymbol, rsp)
	case MethodSapiGetMarginAllOrders:
		return parseOrders[*MarginOrder](mapSymbol, rsp)
	default:
		return nil, errs.NewMsg(errs.CodeNotSupport, "not support order method %s", method)
	}
}

/*
FetchOpenOrders

:see: https://binance-docs.github.io/apidocs/spot/en/#cancel-an-existing-order-and-send-a-new-order-trade
:see: https://binance-docs.github.io/apidocs/futures/en/#current-all-open-orders-user_data
:see: https://binance-docs.github.io/apidocs/delivery/en/#current-all-open-orders-user_data
:see: https://binance-docs.github.io/apidocs/voptions/en/#query-current-open-option-orders-user_data
fetch all unfilled currently open orders
:see: https://binance-docs.github.io/apidocs/spot/en/#current-open-orders-user_data
:see: https://binance-docs.github.io/apidocs/futures/en/#current-all-open-orders-user_data
:see: https://binance-docs.github.io/apidocs/delivery/en/#current-all-open-orders-user_data
:see: https://binance-docs.github.io/apidocs/voptions/en/#query-current-open-option-orders-user_data
:see: https://binance-docs.github.io/apidocs/spot/en/#query-margin-account-39-s-open-orders-user_data
:param str symbol: unified market symbol
:param int [since]: the earliest time in ms to fetch open orders for
:param int [limit]: the maximum number of open orders structures to retrieve
:param dict [params]: extra parameters specific to the exchange API endpoint
:param str [params.marginMode]: 'cross' or 'isolated', for spot margin trading
:returns Order[]: a list of `order structures <https://docs.ccxt.com/#/?id=order-structure>`
*/
func (e *Binance) FetchOpenOrders(symbol string, since int64, limit int, params map[string]interface{}) ([]*banexg.Order, *errs.Error) {
	var args map[string]interface{}
	var marketType string
	if symbol != "" {
		argsIn, market, err := e.LoadArgsMarket(symbol, params)
		if err != nil {
			return nil, err
		}
		args = argsIn
		args["symbol"] = market.ID
		marketType = market.Type
	} else {
		args = utils.SafeParams(params)
		marketType, _ = e.GetArgsMarketType(args, "")
	}
	marginMode := utils.PopMapVal(args, banexg.ParamMarginMode, "")
	method := MethodPrivateGetOpenOrders
	if marketType == banexg.MarketOption {
		method = MethodEapiPrivateGetOpenOrders
		if since > 0 {
			args["startTime"] = since
		}
		if limit > 0 {
			args["limit"] = limit
		}
	} else if marketType == banexg.MarketLinear {
		method = MethodFapiPrivateGetOpenOrders
	} else if marketType == banexg.MarketInverse {
		method = MethodDapiPrivateGetOpenOrders
	} else if marketType == banexg.MarketMargin || marginMode != "" {
		method = MethodSapiGetMarginOpenOrders
		if marginMode == banexg.MarginIsolated {
			args["isIsolated"] = true
			if symbol == "" {
				return nil, errs.NewMsg(errs.CodeParamRequired, "FetchOpenOrders requires a symbol for isolated markets")
			}
		}
	}
	tryNum := e.GetRetryNum("FetchOpenOrders", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	var marketMap = make(map[string]*banexg.Market)
	var mapSymbol = func(mid string) string {
		if market, ok := marketMap[mid]; ok {
			return market.Symbol
		}
		market := e.GetMarketById(mid, marketType)
		marketMap[mid] = market
		return market.Symbol
	}
	switch method {
	case MethodPrivateGetOpenOrders:
		return parseOrders[*SpotOrder](mapSymbol, rsp)
	case MethodEapiPrivateGetOpenOrders:
		return parseOrders[*OptionOrder](mapSymbol, rsp)
	case MethodFapiPrivateGetOpenOrders:
		return parseOrders[*FutureOrder](mapSymbol, rsp)
	case MethodDapiPrivateGetOpenOrders:
		return parseOrders[*InverseOrder](mapSymbol, rsp)
	case MethodSapiGetMarginOpenOrders:
		return parseOrders[*MarginOrder](mapSymbol, rsp)
	default:
		return nil, errs.NewMsg(errs.CodeNotSupport, "not support order method %s", method)
	}
}

func (e *Binance) EditOrder(symbol, orderId, side string, amount, price float64, params map[string]interface{}) (*banexg.Order, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	clientOrderId := utils.PopMapVal(args, banexg.ParamClientOrderId, "")
	args["symbol"] = market.ID
	args["side"] = strings.ToUpper(side)
	args["quantity"] = amount
	args["price"] = price
	if clientOrderId != "" {
		args["origClientOrderId"] = clientOrderId
	} else {
		args["orderId"] = orderId
	}
	var method string
	if market.Option {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "EditOrder not available in option market")
	} else if market.Linear {
		method = MethodFapiPrivatePutOrder
	} else if market.Inverse {
		method = MethodDapiPrivatePutOrder
	} else {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "EditOrder not available in spot/margin market")
	}
	tryNum := e.GetRetryNum("EditOrder", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	var mapSymbol = func(mid string) string {
		return market.Symbol
	}
	if method == MethodFapiPrivatePutOrder {
		return parseOrder[*FutureOrder](mapSymbol, rsp)
	} else if method == MethodDapiPrivatePutOrder {
		return parseOrder[*InverseOrder](mapSymbol, rsp)
	} else {
		return nil, errs.NewMsg(errs.CodeRunTime, "invalid method for EditOrder: %s", method)
	}
}

/*
CancelOrder
cancels an open order

	:see: https://binance-docs.github.io/apidocs/spot/en/#cancel-order-trade
	:see: https://binance-docs.github.io/apidocs/futures/en/#cancel-order-trade
	:see: https://binance-docs.github.io/apidocs/delivery/en/#cancel-order-trade
	:see: https://binance-docs.github.io/apidocs/voptions/en/#cancel-option-order-trade
	:see: https://binance-docs.github.io/apidocs/spot/en/#margin-account-cancel-order-trade
	:param str id: order id
	:param str symbol: unified symbol of the market the order was made in
	:param dict [params]: extra parameters specific to the exchange API endpoint
	:returns dict: An `order structure <https://docs.ccxt.com/#/?id=order-structure>`
*/
func (e *Binance) CancelOrder(id string, symbol string, params map[string]interface{}) (*banexg.Order, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	marginMode := utils.PopMapVal(args, banexg.ParamMarginMode, "")
	clientOrderId := utils.PopMapVal(args, banexg.ParamClientOrderId, "")
	args["symbol"] = market.ID
	if clientOrderId != "" {
		if market.Option {
			args["clientOrderId"] = clientOrderId
		} else {
			args["origClientOrderId"] = clientOrderId
		}
	} else {
		args["orderId"] = id
	}
	method := MethodPrivateDeleteOrder
	if market.Option {
		method = MethodEapiPrivateDeleteOrder
	} else if market.Linear {
		method = MethodFapiPrivateDeleteOrder
	} else if market.Inverse {
		method = MethodDapiPrivateDeleteOrder
	} else if market.Type == banexg.MarketMargin || marginMode != "" {
		method = MethodSapiDeleteMarginOrder
		if marginMode == "isolated" {
			args["isIsolated"] = true
		}
	}
	tryNum := e.GetRetryNum("CancelOrder", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	var mapSymbol = func(mid string) string {
		return market.Symbol
	}
	if method == MethodFapiPrivateDeleteOrder {
		return parseOrder[*FutureOrder](mapSymbol, rsp)
	} else if method == MethodDapiPrivateDeleteOrder {
		return parseOrder[*InverseOrder](mapSymbol, rsp)
	} else if method == MethodEapiPrivateDeleteOrder {
		return parseOrder[*OptionOrder](mapSymbol, rsp)
	} else {
		// spot margin sor
		return parseOrder[*SpotOrder](mapSymbol, rsp)
	}
}

func parseOrders[T IBnbOrder](mapSymbol func(string) string, rsp *banexg.HttpRes) ([]*banexg.Order, *errs.Error) {
	var data = make([]T, 0)
	rawList, err := utils.UnmarshalStringMapArr(rsp.Content, &data)
	if err != nil {
		return nil, errs.New(errs.CodeUnmarshalFail, err)
	}
	var result = make([]*banexg.Order, len(data))
	for i, item := range data {
		result[i] = item.ToStdOrder(mapSymbol, rawList[i])
	}
	return result, nil
}

func parseOrder[T IBnbOrder](mapSymbol func(string) string, rsp *banexg.HttpRes) (*banexg.Order, *errs.Error) {
	var data = new(T)
	raw, err := utils.UnmarshalStringMap(rsp.Content, &data)
	if err != nil {
		return nil, errs.New(errs.CodeUnmarshalFail, err)
	}
	result := (*data).ToStdOrder(mapSymbol, raw)
	return result, nil
}

var orderStateMap = map[string]string{
	OdStatusNew:             banexg.OdStatusOpen,
	OdStatusAccept:          banexg.OdStatusOpen,
	OdStatusPartiallyFilled: banexg.OdStatusPartFilled,
	OdStatusFilled:          banexg.OdStatusFilled,
	OdStatusCanceled:        banexg.OdStatusCanceled,
	OdStatusCancelled:       banexg.OdStatusCanceled,
	OdStatusPendingCancel:   banexg.OdStatusCanceling,
	OdStatusReject:          banexg.OdStatusRejected,
	OdStatusExpired:         banexg.OdStatusExpired,
	OdStatusExpiredInMatch:  banexg.OdStatusExpired,
}

func mapOrderStatus(status string) string {
	if val, ok := orderStateMap[status]; ok {
		return val
	}
	return status
}

func (o *OrderBase) ToStdOrder(mapSymbol func(string) string, info map[string]interface{}) *banexg.Order {
	status := mapOrderStatus(o.Status)
	filled, _ := strconv.ParseFloat(o.ExecutedQty, 64)
	lastTradeTimestamp := int64(0)
	if filled > 0 && status == banexg.OdStatusOpen || status == banexg.OdStatusFilled {
		lastTradeTimestamp = o.UpdateTime
	}
	orderType := strings.ToLower(o.Type)
	postOnly := false
	if orderType == banexg.OdTypeLimitMaker {
		orderType = banexg.OdTypeLimit
		postOnly = true
	}
	timeInForce := o.TimeInForce
	if timeInForce == "GTX" {
		//GTX means "Good Till Crossing" and is an equivalent way of saying Post Only
		timeInForce = "PO"
	}
	if timeInForce == "PO" {
		postOnly = true
	}
	price, _ := strconv.ParseFloat(o.Price, 64)
	return &banexg.Order{
		ID:                  strconv.Itoa(o.OrderId),
		ClientOrderID:       o.ClientOrderId,
		LastTradeTimestamp:  lastTradeTimestamp,
		LastUpdateTimestamp: o.UpdateTime,
		Type:                orderType,
		TimeInForce:         timeInForce,
		PostOnly:            postOnly,
		Side:                strings.ToLower(o.Side),
		Price:               price,
		Filled:              filled,
		Status:              status,
		Symbol:              mapSymbol(o.Symbol),
		Fee:                 &banexg.Fee{},
		Trades:              make([]*banexg.Trade, 0),
	}
}

func (o *SpotBase) ToStdOrder(mapSymbol func(string) string, info map[string]interface{}) *banexg.Order {
	timeStamp := int64(0)
	if o.Time > 0 {
		timeStamp = o.Time
	} else if o.TransactTime > 0 {
		timeStamp = o.TransactTime
	} else if o.UpdateTime > 0 {
		timeStamp = o.UpdateTime
	}
	stopPrice, _ := strconv.ParseFloat(o.StopPrice, 64)
	amount, _ := strconv.ParseFloat(o.OrigQty, 64)
	cost, _ := strconv.ParseFloat(o.CummulativeQuoteQty, 64)
	result := o.OrderBase.ToStdOrder(mapSymbol, info)
	result.Timestamp = timeStamp
	result.Datetime = utils.ISO8601(timeStamp)
	result.TriggerPrice = stopPrice
	result.Amount = amount
	result.Cost = cost
	return result
}

func (o *SpotOrder) ToStdOrder(mapSymbol func(string) string, info map[string]interface{}) *banexg.Order {
	result := o.SpotBase.ToStdOrder(mapSymbol, info)
	result.Info = info
	timeStamp := int64(0)
	if o.Time > 0 {
		timeStamp = o.Time
	} else if o.WorkingTime > 0 {
		timeStamp = o.WorkingTime
	} else if o.TransactTime > 0 {
		timeStamp = o.TransactTime
	} else if o.UpdateTime > 0 {
		timeStamp = o.UpdateTime
	}
	result.Timestamp = timeStamp
	result.Datetime = utils.ISO8601(timeStamp)
	// calc fee and average from trades
	average, filled, feeAmt, feeAsset := calcFills(o.Fills)
	if filled > 0 {
		result.Average = average
		if result.Price == 0 {
			result.Price = average
		}
		result.Filled = filled
		if result.Fee == nil {
			result.Fee = &banexg.Fee{
				Currency: feeAsset,
				Cost:     feeAmt,
			}
		} else {
			result.Fee.Cost = feeAmt
			result.Fee.Currency = feeAsset
		}
	}
	return result
}

func (o *MarginOrder) ToStdOrder(mapSymbol func(string) string, info map[string]interface{}) *banexg.Order {
	result := o.SpotBase.ToStdOrder(mapSymbol, info)
	result.Info = info
	return result
}

func (o *OptionOrder) ToStdOrder(mapSymbol func(string) string, info map[string]interface{}) *banexg.Order {
	timeStamp := o.CreateTime
	if timeStamp == 0 {
		timeStamp = o.UpdateTime
	}
	avgPrice, _ := strconv.ParseFloat(o.AvgPrice, 64)
	result := o.OrderBase.ToStdOrder(mapSymbol, info)
	result.Info = info
	result.Timestamp = timeStamp
	result.Datetime = utils.ISO8601(timeStamp)
	result.ReduceOnly = o.ReduceOnly
	result.Average = avgPrice
	result.Amount = o.Quantity
	result.Fee.Currency = o.QuoteAsset
	result.Fee.Cost = o.Fee
	result.PostOnly = o.PostOnly
	return result
}

func (o *FutureBase) ToStdOrder(mapSymbol func(string) string, info map[string]interface{}) *banexg.Order {
	timeStamp := o.Time
	if timeStamp == 0 {
		timeStamp = o.UpdateTime
	}
	stopPrice, _ := strconv.ParseFloat(o.StopPrice, 64)
	avgPrice, _ := strconv.ParseFloat(o.AvgPrice, 64)
	amount, _ := strconv.ParseFloat(o.OrigQty, 64)
	result := o.OrderBase.ToStdOrder(mapSymbol, info)
	result.Timestamp = timeStamp
	result.Datetime = utils.ISO8601(timeStamp)
	result.ReduceOnly = o.ReduceOnly
	result.Average = avgPrice
	result.Amount = amount
	result.TriggerPrice = stopPrice
	result.PositionSide = strings.ToLower(o.PositionSide)
	return result
}

func (o *FutureOrder) ToStdOrder(mapSymbol func(string) string, info map[string]interface{}) *banexg.Order {
	cost, _ := strconv.ParseFloat(o.CumQuote, 64)
	result := o.FutureBase.ToStdOrder(mapSymbol, info)
	result.Info = info
	result.Cost = cost
	return result
}

func (o *InverseOrder) ToStdOrder(mapSymbol func(string) string, info map[string]interface{}) *banexg.Order {
	cost, _ := strconv.ParseFloat(o.CumBase, 64)
	result := o.FutureBase.ToStdOrder(mapSymbol, info)
	result.Info = info
	result.Cost = cost
	return result
}

func calcFills(fills []*SpotFill) (float64, float64, float64, string) {
	if len(fills) == 0 {
		return 0, 0, 0, ""
	}
	var totCost, totAmt, totFee = float64(0), float64(0), float64(0)
	var feeCurr string
	for _, f := range fills {
		price, _ := strconv.ParseFloat(f.Price, 64)
		qty, _ := strconv.ParseFloat(f.Qty, 64)
		feeCost, _ := strconv.ParseFloat(f.Commission, 64)
		feeCurr = f.CommissionAsset
		totAmt += qty
		totFee += feeCost
		totCost += qty * price
	}
	average := float64(0)
	if totAmt > 0 {
		average = totCost / totAmt
	}
	return average, totAmt, totFee, feeCurr
}
