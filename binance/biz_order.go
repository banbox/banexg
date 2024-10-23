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
	method := "privateGetOrder"
	if market.Option {
		method = "eapiPrivateGetOrder"
	} else if market.Linear {
		method = "fapiPrivateGetOrder"
	} else if market.Inverse {
		method = "dapiPrivateGetOrder"
	} else if market.Type == banexg.MarketMargin || marginMode != "" {
		method = "sapiGetMarginOrder"
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
	case "privateGetOrder":
		return parseOrder[*SpotOrder](mapSymbol, rsp)
	case "eapiPrivateGetOrder":
		return parseOrder[*OptionOrder](mapSymbol, rsp)
	case "fapiPrivateGetOrder":
		return parseOrder[*FutureOrder](mapSymbol, rsp)
	case "dapiPrivateGetOrder":
		return parseOrder[*InverseOrder](mapSymbol, rsp)
	case "sapiGetMarginOrder":
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
	method := "privateGetAllOrders"
	if market.Option {
		method = "eapiPrivateGetHistoryOrders"
	} else if market.Linear {
		method = "fapiPrivateGetAllOrders"
	} else if market.Inverse {
		method = "dapiPrivateGetAllOrders"
	} else if market.Type == banexg.MarketMargin || marginMode != "" {
		method = "sapiGetMarginAllOrders"
		if marginMode == "isolated" {
			args["isIsolated"] = true
		}
	}
	until := utils.PopMapVal(args, "until", int64(0))
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
	case "privateGetAllOrders":
		return parseOrders[*SpotOrder](mapSymbol, rsp)
	case "eapiPrivateGetHistoryOrders":
		return parseOrders[*OptionOrder](mapSymbol, rsp)
	case "fapiPrivateGetAllOrders":
		return parseOrders[*FutureOrder](mapSymbol, rsp)
	case "dapiPrivateGetAllOrders":
		return parseOrders[*InverseOrder](mapSymbol, rsp)
	case "sapiGetMarginAllOrders":
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
	method := "privateGetOpenOrders"
	if marketType == banexg.MarketOption {
		method = "eapiPrivateGetOpenOrders"
		if since > 0 {
			args["startTime"] = since
		}
		if limit > 0 {
			args["limit"] = limit
		}
	} else if marketType == banexg.MarketLinear {
		method = "fapiPrivateGetOpenOrders"
	} else if marketType == banexg.MarketInverse {
		method = "dapiPrivateGetOpenOrders"
	} else if marketType == banexg.MarketMargin || marginMode != "" {
		method = "sapiGetMarginOpenOrders"
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
	case "privateGetOpenOrders":
		return parseOrders[*SpotOrder](mapSymbol, rsp)
	case "eapiPrivateGetOpenOrders":
		return parseOrders[*OptionOrder](mapSymbol, rsp)
	case "fapiPrivateGetOpenOrders":
		return parseOrders[*FutureOrder](mapSymbol, rsp)
	case "dapiPrivateGetOpenOrders":
		return parseOrders[*InverseOrder](mapSymbol, rsp)
	case "sapiGetMarginOpenOrders":
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
		method = "fapiPrivatePutOrder"
	} else if market.Inverse {
		method = "dapiPrivatePutOrder"
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
	if method == "fapiPrivatePutOrder" {
		return parseOrder[*FutureOrder](mapSymbol, rsp)
	} else if method == "dapiPrivatePutOrder" {
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
	method := "privateDeleteOrder"
	if market.Option {
		method = "eapiPrivateDeleteOrder"
	} else if market.Linear {
		method = "fapiPrivateDeleteOrder"
	} else if market.Inverse {
		method = "dapiPrivateDeleteOrder"
	} else if market.Type == banexg.MarketMargin || marginMode != "" {
		method = "sapiDeleteMarginOrder"
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
	if method == "fapiPrivateDeleteOrder" {
		return parseOrder[*FutureOrder](mapSymbol, rsp)
	} else if method == "dapiPrivateDeleteOrder" {
		return parseOrder[*InverseOrder](mapSymbol, rsp)
	} else if method == "eapiPrivateDeleteOrder" {
		return parseOrder[*OptionOrder](mapSymbol, rsp)
	} else {
		// spot margin sor
		return parseOrder[*SpotOrder](mapSymbol, rsp)
	}
}

func parseOrders[T IBnbOrder](mapSymbol func(string) string, rsp *banexg.HttpRes) ([]*banexg.Order, *errs.Error) {
	var data = make([]T, 0)
	err := utils.UnmarshalString(rsp.Content, &data)
	if err != nil {
		return nil, errs.New(errs.CodeUnmarshalFail, err)
	}
	var result = make([]*banexg.Order, len(data))
	for i, item := range data {
		result[i] = item.ToStdOrder(mapSymbol)
	}
	return result, nil
}

func parseOrder[T IBnbOrder](mapSymbol func(string) string, rsp *banexg.HttpRes) (*banexg.Order, *errs.Error) {
	var data = new(T)
	err := utils.UnmarshalString(rsp.Content, &data)
	if err != nil {
		return nil, errs.New(errs.CodeUnmarshalFail, err)
	}
	result := (*data).ToStdOrder(mapSymbol)
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

func (o *OrderBase) ToStdOrder(mapSymbol func(string) string) *banexg.Order {
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

func (o *SpotBase) ToStdOrder(mapSymbol func(string) string) *banexg.Order {
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
	result := o.OrderBase.ToStdOrder(mapSymbol)
	result.Timestamp = timeStamp
	result.Datetime = utils.ISO8601(timeStamp)
	result.TriggerPrice = stopPrice
	result.Amount = amount
	result.Cost = cost
	return result
}

func (o *SpotOrder) ToStdOrder(mapSymbol func(string) string) *banexg.Order {
	result := o.SpotBase.ToStdOrder(mapSymbol)
	result.Info = o
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

func (o *MarginOrder) ToStdOrder(mapSymbol func(string) string) *banexg.Order {
	result := o.SpotBase.ToStdOrder(mapSymbol)
	result.Info = o
	return result
}

func (o *OptionOrder) ToStdOrder(mapSymbol func(string) string) *banexg.Order {
	timeStamp := o.CreateTime
	if timeStamp == 0 {
		timeStamp = o.UpdateTime
	}
	avgPrice, _ := strconv.ParseFloat(o.AvgPrice, 64)
	result := o.OrderBase.ToStdOrder(mapSymbol)
	result.Info = o
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

func (o *FutureBase) ToStdOrder(mapSymbol func(string) string) *banexg.Order {
	timeStamp := o.Time
	if timeStamp == 0 {
		timeStamp = o.UpdateTime
	}
	stopPrice, _ := strconv.ParseFloat(o.StopPrice, 64)
	avgPrice, _ := strconv.ParseFloat(o.AvgPrice, 64)
	amount, _ := strconv.ParseFloat(o.OrigQty, 64)
	result := o.OrderBase.ToStdOrder(mapSymbol)
	result.Timestamp = timeStamp
	result.Datetime = utils.ISO8601(timeStamp)
	result.ReduceOnly = o.ReduceOnly
	result.Average = avgPrice
	result.Amount = amount
	result.TriggerPrice = stopPrice
	result.PositionSide = strings.ToLower(o.PositionSide)
	return result
}

func (o *FutureOrder) ToStdOrder(mapSymbol func(string) string) *banexg.Order {
	cost, _ := strconv.ParseFloat(o.CumQuote, 64)
	result := o.FutureBase.ToStdOrder(mapSymbol)
	result.Info = o
	result.Cost = cost
	return result
}

func (o *InverseOrder) ToStdOrder(mapSymbol func(string) string) *banexg.Order {
	cost, _ := strconv.ParseFloat(o.CumBase, 64)
	result := o.FutureBase.ToStdOrder(mapSymbol)
	result.Info = o
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
