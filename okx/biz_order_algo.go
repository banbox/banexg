package okx

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

func isAlgoOrderType(odType string) bool {
	switch odType {
	case banexg.OdTypeStop, banexg.OdTypeStopMarket, banexg.OdTypeStopLoss, banexg.OdTypeStopLossLimit,
		banexg.OdTypeTakeProfit, banexg.OdTypeTakeProfitLimit, banexg.OdTypeTakeProfitMarket,
		banexg.OdTypeTrailingStopMarket, "conditional", "oco", "trigger", "move_order_stop", "twap", "chase":
		return true
	default:
		return false
	}
}

func mapAlgoOrderStatus(status string) string {
	status = strings.ToLower(status)
	if val, ok := algoOrderStatusMap[status]; ok {
		return val
	}
	return status
}

func mapAlgoOrderType(ordType string, tpTriggerPx, tpOrdPx, slTriggerPx, slOrdPx, triggerPx, ordPx float64) string {
	ordType = strings.ToLower(ordType)
	switch ordType {
	case "conditional", "oco":
		if tpTriggerPx != 0 && slTriggerPx != 0 && ordType == "oco" {
			return "oco"
		}
		if slTriggerPx != 0 {
			if slOrdPx == 0 || slOrdPx == -1 {
				return banexg.OdTypeStopLoss
			}
			return banexg.OdTypeStopLossLimit
		}
		if tpTriggerPx != 0 {
			if tpOrdPx == 0 || tpOrdPx == -1 {
				return banexg.OdTypeTakeProfitMarket
			}
			return banexg.OdTypeTakeProfitLimit
		}
	case "trigger":
		if ordPx == 0 || ordPx == -1 {
			return banexg.OdTypeStopMarket
		}
		return banexg.OdTypeStop
	case "move_order_stop":
		return banexg.OdTypeTrailingStopMarket
	}
	return ordType
}

func mapStr(info map[string]interface{}, key string) string {
	if info == nil {
		return ""
	}
	if val, ok := info[key]; ok {
		switch v := val.(type) {
		case string:
			return v
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		case float32:
			return strconv.FormatFloat(float64(v), 'f', -1, 64)
		case int:
			return strconv.Itoa(v)
		case int64:
			return strconv.FormatInt(v, 10)
		case json.Number:
			return v.String()
		case bool:
			if v {
				return "true"
			}
			return "false"
		}
	}
	return ""
}

func mapFloat(info map[string]interface{}, key string) float64 {
	return parseFloat(mapStr(info, key))
}

func mapFloatAny(info map[string]interface{}, keys ...string) float64 {
	for _, key := range keys {
		if val := mapStr(info, key); val != "" {
			return parseFloat(val)
		}
	}
	return 0
}

func parseAlgoOrders(e *OKX, items []map[string]interface{}, marketType, symbol string) ([]*banexg.Order, *errs.Error) {
	result := make([]*banexg.Order, 0, len(items))
	for _, item := range items {
		order := parseAlgoOrder(e, item, marketType)
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

func parseAlgoOrder(e *OKX, info map[string]interface{}, marketType string) *banexg.Order {
	if info == nil {
		return nil
	}
	instId := mapStr(info, FldInstId)
	algoId := mapStr(info, FldAlgoId)
	if algoId == "" {
		return nil
	}
	state := mapStr(info, "state")
	ordType := mapStr(info, FldOrdType)
	side := strings.ToLower(mapStr(info, FldSide))
	posSide := strings.ToLower(mapStr(info, FldPosSide))
	amount := mapFloat(info, FldSz)
	ordPx := mapFloatAny(info, FldOrdPx, FldOrderPx, FldPx)
	triggerPx := mapFloat(info, FldTriggerPx)
	tpTriggerPx := mapFloat(info, FldTpTriggerPx)
	tpOrdPx := mapFloat(info, FldTpOrdPx)
	slTriggerPx := mapFloat(info, FldSlTriggerPx)
	slOrdPx := mapFloat(info, FldSlOrdPx)
	ts := parseInt(mapStr(info, "cTime"))
	lastUpdate := parseInt(mapStr(info, "uTime"))
	clientID := mapStr(info, FldAlgoClOrdId)
	if clientID == "" {
		clientID = mapStr(info, FldClOrdId)
	}
	status := mapAlgoOrderStatus(state)
	orderType := mapAlgoOrderType(ordType, tpTriggerPx, tpOrdPx, slTriggerPx, slOrdPx, triggerPx, ordPx)
	symbol := instId
	market := getMarketByIDAny(e, instId, marketType)
	if market != nil {
		symbol = market.Symbol
		// For contract markets, convert contracts to coins
		if market.Contract && market.ContractSize > 0 && market.ContractSize != 1 {
			amount = amount * market.ContractSize
		}
	}
	trigger := triggerPx
	if trigger == 0 {
		if slTriggerPx != 0 {
			trigger = slTriggerPx
		} else if tpTriggerPx != 0 {
			trigger = tpTriggerPx
		}
	}
	reduceOnly := false
	if val := mapStr(info, FldReduceOnly); val != "" {
		reduceOnly = parseBoolStr(val)
	}
	return &banexg.Order{
		Info:                info,
		ID:                  "algo:" + algoId,
		ClientOrderID:       clientID,
		Timestamp:           ts,
		LastUpdateTimestamp: lastUpdate,
		Status:              status,
		Symbol:              symbol,
		Type:                orderType,
		PositionSide:        posSide,
		Side:                side,
		Price:               ordPx,
		Amount:              amount,
		TriggerPrice:        trigger,
		StopPrice:           trigger,
		TakeProfitPrice:     tpTriggerPx,
		StopLossPrice:       slTriggerPx,
		ReduceOnly:          reduceOnly,
	}
}

func setAlgoOrderID(args map[string]interface{}, algoId string) *errs.Error {
	clientOrderId := utils.PopMapVal(args, banexg.ParamClientOrderId, "")
	if algoId != "" {
		algoId = strings.TrimPrefix(algoId, "algo:")
		args[FldAlgoId] = algoId
		return nil
	}
	if clientOrderId != "" {
		if !validateClOrdId(clientOrderId) {
			return errs.NewMsg(errs.CodeParamInvalid, "clOrdId must be 1-32 alphanumeric characters")
		}
		args[FldAlgoClOrdId] = clientOrderId
		return nil
	}
	return errs.NewMsg(errs.CodeParamRequired, "algo id or clientOrderId required")
}

func (e *OKX) createAlgoOrder(market *banexg.Market, odType, side string, amount, price float64, params map[string]interface{}, stopLossPrice, takeProfitPrice float64) (*banexg.Order, *errs.Error) {
	args := utils.SafeParams(params)
	args[FldInstId] = market.ID
	args[FldSide] = strings.ToLower(side)
	if _, ok := args[FldTdMode]; !ok {
		if market.Type == banexg.MarketSpot {
			args[FldTdMode] = TdModeCash
		} else {
			mgnMode := utils.PopMapVal(args, banexg.ParamMarginMode, banexg.MarginCross)
			args[FldTdMode] = mgnMode
		}
	}
	if market.Contract {
		if _, ok := args[FldPosSide]; !ok {
			posSide := utils.PopMapVal(args, banexg.ParamPositionSide, "net")
			args[FldPosSide] = strings.ToLower(posSide)
		}
	}
	if _, ok := args[FldReduceOnly]; !ok {
		if reduceOnly := utils.PopMapVal(args, banexg.ParamReduceOnly, false); reduceOnly {
			args[FldReduceOnly] = true
		}
	}
	closePosition := utils.PopMapVal(args, banexg.ParamClosePosition, false)
	if closePosition {
		args[FldCloseFraction] = "1"
		if _, ok := args[FldReduceOnly]; !ok {
			args[FldReduceOnly] = true
		}
		delete(args, FldSz)
	}
	if _, ok := args[FldCallbackRatio]; !ok {
		if callbackRate := utils.PopMapVal(args, banexg.ParamCallbackRate, 0.0); callbackRate > 0 {
			args[FldCallbackRatio] = strconv.FormatFloat(callbackRate, 'f', -1, 64)
		}
	}
	if _, ok := args[FldCallbackSpread]; !ok {
		if trailingDelta := utils.PopMapVal(args, banexg.ParamTrailingDelta, 0.0); trailingDelta > 0 {
			precSpread, err := e.PrecPrice(market, trailingDelta)
			if err != nil {
				return nil, err
			}
			args[FldCallbackSpread] = strconv.FormatFloat(precSpread, 'f', -1, 64)
		}
	}
	if _, ok := args[FldActivePx]; !ok {
		if activationPrice := utils.PopMapVal(args, banexg.ParamActivationPrice, 0.0); activationPrice > 0 {
			precPx, err := e.PrecPrice(market, activationPrice)
			if err != nil {
				return nil, err
			}
			args[FldActivePx] = strconv.FormatFloat(precPx, 'f', -1, 64)
		}
	}
	if clOrdId, ok := args[FldClOrdId]; ok {
		if s, isStr := clOrdId.(string); isStr {
			if !validateClOrdId(s) {
				return nil, errs.NewMsg(errs.CodeParamInvalid, "clOrdId must be 1-32 alphanumeric characters")
			}
			args[FldAlgoClOrdId] = s
		} else {
			args[FldAlgoClOrdId] = clOrdId
		}
		delete(args, FldClOrdId)
	} else if clOrdId := utils.PopMapVal(args, banexg.ParamClientOrderId, ""); clOrdId != "" {
		if !validateClOrdId(clOrdId) {
			return nil, errs.NewMsg(errs.CodeParamInvalid, "clOrdId must be 1-32 alphanumeric characters")
		}
		args[FldAlgoClOrdId] = clOrdId
	}
	if market.Spot {
		if tradeQuoteCcy := getTradeQuoteCcy(market); tradeQuoteCcy != "" {
			args[FldTradeQuoteCcy] = tradeQuoteCcy
		}
	}
	if !closePosition {
		if market.Spot && odType == banexg.OdTypeMarket {
			cost := utils.PopMapVal(args, banexg.ParamCost, 0.0)
			if cost > 0 {
				args[FldTgtCcy] = TgtCcyQuote
				precCost, err := e.PrecCost(market, cost)
				if err != nil {
					return nil, err
				}
				args[FldSz] = strconv.FormatFloat(precCost, 'f', -1, 64)
			} else {
				precAmt, err := e.PrecAmount(market, amount)
				if err != nil {
					return nil, err
				}
				args[FldSz] = strconv.FormatFloat(precAmt, 'f', -1, 64)
			}
		} else {
			// For contract markets, convert coin amount to contracts
			szAmount := amount
			if market.Contract && market.ContractSize > 0 && market.ContractSize != 1 {
				szAmount = amount / market.ContractSize
			}
			precAmt, err := e.PrecAmount(market, szAmount)
			if err != nil {
				return nil, err
			}
			args[FldSz] = strconv.FormatFloat(precAmt, 'f', -1, 64)
		}
	}
	algoOrdType := strings.ToLower(odType)
	switch algoOrdType {
	case banexg.OdTypeStop, banexg.OdTypeStopMarket:
		algoOrdType = "trigger"
	case banexg.OdTypeTrailingStopMarket:
		algoOrdType = "move_order_stop"
	case banexg.OdTypeStopLoss, banexg.OdTypeStopLossLimit, banexg.OdTypeTakeProfit, banexg.OdTypeTakeProfitLimit, banexg.OdTypeTakeProfitMarket:
		algoOrdType = ""
	}
	if algoOrdType == "" || algoOrdType == banexg.OdTypeStop || algoOrdType == banexg.OdTypeStopMarket {
		if stopLossPrice == 0 && takeProfitPrice == 0 {
			return nil, errs.NewMsg(errs.CodeParamRequired, "createOrder require stopLossPrice/takeProfitPrice for algo order")
		}
		algoOrdType = "conditional"
		if stopLossPrice != 0 && takeProfitPrice != 0 {
			algoOrdType = "oco"
		}
		args[FldOrdType] = algoOrdType
		if takeProfitPrice != 0 {
			tpPx, err := e.PrecPrice(market, takeProfitPrice)
			if err != nil {
				return nil, err
			}
			args[FldTpTriggerPx] = strconv.FormatFloat(tpPx, 'f', -1, 64)
			if _, ok := args[FldTpOrdPx]; !ok {
				if odType == banexg.OdTypeTakeProfitLimit && price > 0 {
					ordPx, err := e.PrecPrice(market, price)
					if err != nil {
						return nil, err
					}
					args[FldTpOrdPx] = strconv.FormatFloat(ordPx, 'f', -1, 64)
				} else {
					args[FldTpOrdPx] = "-1"
				}
			}
		}
		if stopLossPrice != 0 {
			slPx, err := e.PrecPrice(market, stopLossPrice)
			if err != nil {
				return nil, err
			}
			args[FldSlTriggerPx] = strconv.FormatFloat(slPx, 'f', -1, 64)
			if _, ok := args[FldSlOrdPx]; !ok {
				if odType == banexg.OdTypeStopLossLimit && price > 0 {
					ordPx, err := e.PrecPrice(market, price)
					if err != nil {
						return nil, err
					}
					args[FldSlOrdPx] = strconv.FormatFloat(ordPx, 'f', -1, 64)
				} else {
					args[FldSlOrdPx] = "-1"
				}
			}
		}
	} else {
		args[FldOrdType] = algoOrdType
		switch algoOrdType {
		case "trigger":
			if _, ok := args[FldTriggerPx]; !ok {
				if stopLossPrice == 0 {
					return nil, errs.NewMsg(errs.CodeParamRequired, "triggerPx required for trigger order")
				}
				triggerPx, err := e.PrecPrice(market, stopLossPrice)
				if err != nil {
					return nil, err
				}
				args[FldTriggerPx] = strconv.FormatFloat(triggerPx, 'f', -1, 64)
			}
			if _, ok := args[FldOrderPx]; !ok {
				if price > 0 {
					ordPx, err := e.PrecPrice(market, price)
					if err != nil {
						return nil, err
					}
					args[FldOrderPx] = strconv.FormatFloat(ordPx, 'f', -1, 64)
				} else {
					args[FldOrderPx] = "-1"
				}
			}
		case "move_order_stop":
			if _, ok := args[FldCallbackRatio]; !ok {
				if _, ok := args[FldCallbackSpread]; !ok {
					return nil, errs.NewMsg(errs.CodeParamRequired, "callbackRatio/callbackSpread required for move_order_stop")
				}
			}
		case "twap":
			if _, ok := args[FldSzLimit]; !ok {
				return nil, errs.NewMsg(errs.CodeParamRequired, "szLimit required for twap order")
			}
			if _, ok := args[FldPxLimit]; !ok {
				return nil, errs.NewMsg(errs.CodeParamRequired, "pxLimit required for twap order")
			}
			if _, ok := args[FldTimeInterval]; !ok {
				return nil, errs.NewMsg(errs.CodeParamRequired, "timeInterval required for twap order")
			}
		}
	}
	if workingType := utils.PopMapVal(args, banexg.ParamWorkingType, ""); workingType != "" {
		if takeProfitPrice != 0 {
			args[FldTpTriggerPxType] = workingType
		}
		if stopLossPrice != 0 {
			args[FldSlTriggerPxType] = workingType
		}
	}
	delete(args, FldPx)
	delete(args, banexg.ParamTimeInForce)

	tryNum := e.GetRetryNum("CreateOrder", 1)
	res := requestRetry[[]map[string]interface{}](e, MethodTradePostOrderAlgo, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	if len(res.Result) == 0 {
		return nil, errs.NewMsg(errs.CodeDataNotFound, "empty algo order result")
	}
	item := res.Result[0]
	if scode := mapStr(item, "sCode"); scode != "" && scode != "0" {
		return nil, errs.NewMsg(errs.CodeRunTime, "[%s] %s", scode, mapStr(item, "sMsg"))
	}
	algoId := mapStr(item, FldAlgoId)
	if algoId == "" {
		return nil, errs.NewMsg(errs.CodeDataNotFound, "empty algoId")
	}
	clientID := mapStr(item, FldAlgoClOrdId)
	if clientID == "" {
		clientID = mapStr(item, FldClOrdId)
	}
	return &banexg.Order{
		ID:              "algo:" + algoId,
		ClientOrderID:   clientID,
		Symbol:          market.Symbol,
		Type:            odType,
		Side:            side,
		Amount:          amount,
		Price:           price,
		Status:          banexg.OdStatusOpen,
		TakeProfitPrice: takeProfitPrice,
		StopLossPrice:   stopLossPrice,
	}, nil
}

func (e *OKX) fetchAlgoOrder(id string, market *banexg.Market, args map[string]interface{}) (*banexg.Order, *errs.Error) {
	if err := setAlgoOrderID(args, id); err != nil {
		return nil, err
	}
	args[FldInstId] = market.ID
	tryNum := e.GetRetryNum("FetchOrder", 1)
	res := requestRetry[[]map[string]interface{}](e, MethodTradeGetOrderAlgo, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	if len(res.Result) == 0 {
		return nil, errs.NewMsg(errs.CodeDataNotFound, "empty algo order result")
	}
	return parseAlgoOrder(e, res.Result[0], market.Type), nil
}

func (e *OKX) cancelAlgoOrder(id string, market *banexg.Market, args map[string]interface{}) (*banexg.Order, *errs.Error) {
	if err := setAlgoOrderID(args, id); err != nil {
		return nil, err
	}
	args[FldInstId] = market.ID
	tryNum := e.GetRetryNum("CancelOrder", 1)
	res := requestRetry[[]map[string]interface{}](e, MethodTradePostCancelAlgos, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	if len(res.Result) == 0 {
		return nil, errs.NewMsg(errs.CodeDataNotFound, "empty cancel algo result")
	}
	item := res.Result[0]
	if scode := mapStr(item, "sCode"); scode != "" && scode != "0" {
		return nil, errs.NewMsg(errs.CodeRunTime, "[%s] %s", scode, mapStr(item, "sMsg"))
	}
	algoId := mapStr(item, FldAlgoId)
	if algoId == "" {
		algoId = strings.TrimPrefix(id, "algo:")
	}
	clientID := mapStr(item, FldAlgoClOrdId)
	if clientID == "" {
		clientID = mapStr(item, FldClOrdId)
	}
	return &banexg.Order{
		ID:            "algo:" + algoId,
		ClientOrderID: clientID,
		Symbol:        market.Symbol,
		Status:        banexg.OdStatusCanceled,
	}, nil
}
