package binance

import (
	"context"
	"fmt"
	"github.com/anyongjin/banexg"
	"github.com/anyongjin/banexg/utils"
	"github.com/bytedance/sonic"
	"strconv"
	"strings"
)

/*
FetchOrders 获取自己的订单
symbol: 必填，币种
*/
func (e *Binance) FetchOrders(symbol string, since int64, limit int, params *map[string]interface{}) ([]*banexg.Order, error) {
	_, err := e.LoadMarkets(false, nil)
	if err != nil {
		return nil, fmt.Errorf("load markets fail: %v", err)
	}
	var args = utils.SafeParams(params)
	marketType, marketInverse := e.GetArgsMarketType(args, symbol)
	market, err := e.GetMarket(symbol)
	if err != nil {
		return nil, fmt.Errorf("get market fail: %v", err)
	}
	args["symbol"] = market.ID
	marginMode := utils.PopMapVal(args, "marginMode", "")
	method := "privateGetAllOrders"
	if marketType == banexg.MarketOption {
		method = "eapiPrivateGetHistoryOrders"
	} else if marketType == banexg.MarketFuture {
		method = "fapiPrivateGetAllOrders"
	} else if marketInverse {
		method = "dapiPrivateGetAllOrders"
	} else if marketType == banexg.MarketMargin || marginMode != "" {
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
	rsp := e.RequestApi(context.Background(), method, &args)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	if !strings.HasPrefix(rsp.Content, "[") {
		return nil, fmt.Errorf(rsp.Content)
	}
	switch method {
	case "privateGetAllOrders":
		return parseOrders[*SpotOrder](market, rsp)
	case "eapiPrivateGetHistoryOrders":
		return parseOrders[*OptionOrder](market, rsp)
	case "fapiPrivateGetAllOrders":
		return parseOrders[*FutureOrder](market, rsp)
	case "dapiPrivateGetAllOrders":
		return parseOrders[*InverseOrder](market, rsp)
	case "sapiGetMarginAllOrders":
		return parseOrders[*MarginOrder](market, rsp)
	default:
		return nil, fmt.Errorf("not support order method %s", method)
	}
}

func parseOrders[T IBnbOrder](m *banexg.Market, rsp *banexg.HttpRes) ([]*banexg.Order, error) {
	var data = make([]T, 0)
	err := sonic.UnmarshalString(rsp.Content, &data)
	if err != nil {
		return nil, err
	}
	var result = make([]*banexg.Order, len(data))
	for i, item := range data {
		result[i] = item.ToStdOrder(m)
	}
	return result, nil
}

func parseOrder[T IBnbOrder](m *banexg.Market, rsp *banexg.HttpRes) (*banexg.Order, error) {
	var data = new(T)
	err := sonic.UnmarshalString(rsp.Content, &data)
	if err != nil {
		return nil, err
	}
	result := (*data).ToStdOrder(m)
	return result, nil
}

var orderStateMap = map[string]string{
	OdStatusNew:             banexg.OdStatusOpen,
	OdStatusPartiallyFilled: banexg.OdStatusOpen,
	OdStatusAccept:          banexg.OdStatusOpen,
	OdStatusFilled:          banexg.OdStatusClosed,
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

func (o *OrderBase) ToStdOrder(m *banexg.Market) *banexg.Order {
	status := mapOrderStatus(o.Status)
	filled, _ := strconv.ParseFloat(o.ExecutedQty, 64)
	lastTradeTimestamp := int64(0)
	if filled > 0 && status == banexg.OdStatusOpen || status == banexg.OdStatusClosed {
		lastTradeTimestamp = o.UpdateTime
	}
	orderType := strings.ToLower(o.Type)
	postOnly := false
	if orderType == "limit_maker" {
		orderType = "limit"
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
		Symbol:              m.Symbol,
		Fee:                 &banexg.Fee{},
		Trades:              make([]*banexg.Trade, 0),
	}
}

func (o *SpotBase) ToStdOrder(m *banexg.Market) *banexg.Order {
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
	result := o.OrderBase.ToStdOrder(m)
	result.Timestamp = timeStamp
	result.Datetime = utils.ISO8601(timeStamp)
	result.TriggerPrice = stopPrice
	result.Amount = amount
	result.Cost = cost
	return result
}

func (o *SpotOrder) ToStdOrder(m *banexg.Market) *banexg.Order {
	result := o.SpotBase.ToStdOrder(m)
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
	return result
}

func (o *MarginOrder) ToStdOrder(m *banexg.Market) *banexg.Order {
	result := o.SpotBase.ToStdOrder(m)
	result.Info = o
	return result
}

func (o *OptionOrder) ToStdOrder(m *banexg.Market) *banexg.Order {
	timeStamp := o.CreateTime
	if timeStamp == 0 {
		timeStamp = o.UpdateTime
	}
	avgPrice, _ := strconv.ParseFloat(o.AvgPrice, 64)
	result := o.OrderBase.ToStdOrder(m)
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

func (o *FutureBase) ToStdOrder(m *banexg.Market) *banexg.Order {
	timeStamp := o.Time
	if timeStamp == 0 {
		timeStamp = o.UpdateTime
	}
	stopPrice, _ := strconv.ParseFloat(o.StopPrice, 64)
	avgPrice, _ := strconv.ParseFloat(o.AvgPrice, 64)
	amount, _ := strconv.ParseFloat(o.OrigQty, 64)
	result := o.OrderBase.ToStdOrder(m)
	result.Timestamp = timeStamp
	result.Datetime = utils.ISO8601(timeStamp)
	result.ReduceOnly = o.ReduceOnly
	result.Average = avgPrice
	result.Amount = amount
	result.TriggerPrice = stopPrice
	return result
}

func (o *FutureOrder) ToStdOrder(m *banexg.Market) *banexg.Order {
	cost, _ := strconv.ParseFloat(o.CumQuote, 64)
	result := o.FutureBase.ToStdOrder(m)
	result.Info = o
	result.Cost = cost
	return result
}

func (o *InverseOrder) ToStdOrder(m *banexg.Market) *banexg.Order {
	cost, _ := strconv.ParseFloat(o.CumBase, 64)
	result := o.FutureBase.ToStdOrder(m)
	result.Info = o
	result.Cost = cost
	return result
}
