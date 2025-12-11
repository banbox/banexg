package binance

import (
	"context"
	"strconv"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

func (e *Binance) createAlgoOrder(market *banexg.Market, args map[string]interface{}, tryNum int) (*banexg.Order, *errs.Error) {
	utils.PopMapVal(args, banexg.ParamAlgoOrder, false)
	args["symbol"] = market.ID
	args["algoType"] = "CONDITIONAL"
	if v, ok := args["newClientOrderId"]; ok {
		args["clientAlgoId"] = v
		delete(args, "newClientOrderId")
	}
	if v, ok := args["stopPrice"]; ok {
		args["triggerPrice"] = v
		delete(args, "stopPrice")
	}

	if utils.GetMapVal(args, banexg.ParamClosePosition, false) {
		delete(args, "quantity")
		delete(args, banexg.ParamReduceOnly)
	}
	// Convert boolean params to string if necessary
	banexg.SetBoolArg(args, banexg.ParamClosePosition, banexg.BoolLower)
	banexg.SetBoolArg(args, banexg.ParamReduceOnly, banexg.BoolLower)
	banexg.SetBoolArg(args, banexg.ParamPriceProtect, banexg.BoolUpper)

	if _, ok := args[banexg.ParamPriceMatch]; ok {
		delete(args, "price")
	}
	if v, ok := args[banexg.ParamActivationPrice]; ok {
		if val, ok := v.(float64); ok && val > 0 {
			if pStr, err := e.PrecPrice(market, val); err == nil {
				args[banexg.ParamActivationPrice] = pStr
			}
		}
	}

	rsp := e.RequestApiRetry(context.Background(), MethodFapiPrivatePostAlgoOrder, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	var mapSymbol = func(mid string) string {
		return market.Symbol
	}
	return parseOrder[*AlgoOrder](mapSymbol, rsp)
}

func (e *Binance) fetchAlgoOrder(id, clientOrderId string, args map[string]interface{}) (*banexg.Order, *errs.Error) {
	if clientOrderId != "" {
		args["clientAlgoId"] = clientOrderId
	} else {
		args["algoId"] = id
	}
	tryNum := e.GetRetryNum("FetchOrder", 1)
	rsp := e.RequestApiRetry(context.Background(), MethodFapiPrivateGetAlgoOrder, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	var mapSymbol = func(mid string) string {
		market := e.GetMarketById(mid, banexg.MarketLinear)
		return market.Symbol
	}
	return parseOrder[*AlgoOrder](mapSymbol, rsp)
}

func (e *Binance) fetchAlgoOrders(args map[string]interface{}, market *banexg.Market) ([]*banexg.Order, *errs.Error) {
	tryNum := e.GetRetryNum("FetchOrders", 1)
	rsp := e.RequestApiRetry(context.Background(), MethodFapiPrivateGetAllAlgoOrders, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	var mapSymbol = func(mid string) string {
		return market.Symbol
	}
	return parseOrders[*AlgoOrder](mapSymbol, rsp)
}

func (e *Binance) fetchAlgoOpenOrders(args map[string]interface{}, market *banexg.Market) ([]*banexg.Order, *errs.Error) {
	if market != nil {
		args["symbol"] = market.ID
	}
	tryNum := e.GetRetryNum("FetchOpenOrders", 1)
	rsp := e.RequestApiRetry(context.Background(), MethodFapiPrivateGetOpenAlgoOrders, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	var mapSymbol func(mid string) string
	if market != nil {
		mapSymbol = func(mid string) string {
			return market.Symbol
		}
	} else {
		var marketMap = make(map[string]*banexg.Market)
		mapSymbol = func(mid string) string {
			if market, ok := marketMap[mid]; ok {
				return market.Symbol
			}
			market := e.GetMarketById(mid, banexg.MarketLinear)
			marketMap[mid] = market
			return market.Symbol
		}
	}
	return parseOrders[*AlgoOrder](mapSymbol, rsp)
}

func (e *Binance) cancelAlgoOrder(id string, clientOrderId string, market *banexg.Market) (*banexg.Order, *errs.Error) {
	args := make(map[string]interface{})
	args["symbol"] = market.ID
	if clientOrderId != "" {
		args["clientAlgoId"] = clientOrderId
	} else {
		args["algoId"] = id
	}
	method := MethodFapiPrivateDeleteAlgoOrder
	tryNum := e.GetRetryNum("CancelAlgoOrder", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	var data = new(DeleteAlgoOrderRsp)
	_, err := utils.UnmarshalStringMap(rsp.Content, &data)
	if err != nil {
		return nil, errs.New(errs.CodeUnmarshalFail, err)
	}
	if data.Code != "200" {
		return nil, errs.NewMsg(errs.CodeServerError, "cancel algo order failed: %s", data.Msg)
	}
	return &banexg.Order{
		ID:            strconv.FormatInt(data.AlgoId, 10),
		ClientOrderID: data.ClientAlgoId,
		Status:        banexg.OdStatusCanceled,
		Symbol:        market.Symbol,
	}, nil
}
