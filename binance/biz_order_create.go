package binance

import (
	"context"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
	"strings"
)

func isBnbOrderType(market *banexg.Market, odType string) bool {
	if m, ok := market.Info.(*BnbMarket); ok {
		return utils.ArrContains(m.OrderTypes, odType)
	}
	return false
}

/*
CreateOrder 提交订单到交易所

:see: https://binance-docs.github.io/apidocs/spot/en/#new-order-trade

	:see: https://binance-docs.github.io/apidocs/spot/en/#test-new-order-trade
	:see: https://binance-docs.github.io/apidocs/futures/en/#new-order-trade
	:see: https://binance-docs.github.io/apidocs/delivery/en/#new-order-trade
	:see: https://binance-docs.github.io/apidocs/voptions/en/#new-order-trade
	:see: https://binance-docs.github.io/apidocs/spot/en/#new-order-using-sor-trade
	:see: https://binance-docs.github.io/apidocs/spot/en/#test-new-order-using-sor-trade
	:param str symbol: unified symbol of the market to create an order in
	:param str type: 'MARKET' or 'LIMIT' or 'STOP_LOSS' or 'STOP_LOSS_LIMIT' or 'TAKE_PROFIT' or 'TAKE_PROFIT_LIMIT' or 'STOP'
	:param str side: 'buy' or 'sell'
	:param float amount: how much of currency you want to trade in units of base currency
	:param float [price]: the price at which the order is to be fullfilled, in units of the quote currency, ignored in market orders
	:param dict [params]: extra parameters specific to the exchange API endpoint
	:param str [params.marginMode]: 'cross' or 'isolated', for spot margin trading
	:param boolean [params.sor]: *spot only* whether to use SOR(Smart Order Routing) or not, default is False
	:param boolean [params.test]: *spot only* whether to use the test endpoint or not, default is False
	:returns dict: an `order structure <https://docs.ccxt.com/#/?id=order-structure>`
*/
func (e *Binance) CreateOrder(symbol, odType, side string, amount float64, price float64, params map[string]interface{}) (*banexg.Order, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	marginMode := utils.PopMapVal(args, banexg.ParamMarginMode, "")
	sor := utils.PopMapVal(args, banexg.ParamSor, false)
	clientOrderId := utils.PopMapVal(args, banexg.ParamClientOrderId, "")
	postOnly := utils.PopMapVal(args, banexg.ParamPostOnly, false)
	timeInForce := utils.GetMapVal(args, banexg.ParamTimeInForce, "")
	if postOnly || timeInForce == banexg.TimeInForcePO || odType == banexg.OdTypeLimitMaker {
		if timeInForce == banexg.TimeInForceIOC || timeInForce == banexg.TimeInForceFOK {
			return nil, errs.NewMsg(errs.CodeParamInvalid, "postOnly orders cannot have timeInForce: %s", timeInForce)
		} else if odType == banexg.OdTypeMarket {
			return nil, errs.NewMsg(errs.CodeParamInvalid, "market orders cannot be postOnly")
		}
		postOnly = true
	}
	isMarket := odType == banexg.OdTypeMarket
	isLimit := odType == banexg.OdTypeLimit
	triggerPrice := utils.PopMapVal(args, banexg.ParamTriggerPrice, float64(0))
	stopLossPrice := utils.PopMapVal(args, banexg.ParamStopLossPrice, float64(0))
	if stopLossPrice == 0 {
		stopLossPrice = triggerPrice
	}
	takeProfitPrice := utils.PopMapVal(args, banexg.ParamTakeProfitPrice, float64(0))
	trailingDelta := utils.PopMapVal(args, banexg.ParamTrailingDelta, 0)
	isStopLoss := stopLossPrice != float64(0) || trailingDelta != 0
	isTakeProfit := takeProfitPrice != float64(0)
	args["symbol"] = market.ID
	args["side"] = strings.ToUpper(side)
	if postOnly && (market.Spot || market.Type == banexg.MarketMargin) {
		odType = banexg.OdTypeLimitMaker
	}
	if market.Type == banexg.MarketMargin || marginMode != "" {
		reduceOnly := utils.PopMapVal(args, banexg.ParamReduceOnly, false)
		if reduceOnly {
			args["sideEffectType"] = "AUTO_REPAY"
		}
	}
	stopPrice := float64(0)
	if isStopLoss {
		stopPrice = stopLossPrice
		if isMarket {
			odType = banexg.OdTypeStopLoss
			if market.Contract {
				odType = banexg.OdTypeStopMarket
			}
		} else if isLimit {
			odType = banexg.OdTypeStopLossLimit
			if market.Contract {
				odType = banexg.OdTypeStop
			}
		}
	} else if isTakeProfit {
		stopPrice = takeProfitPrice
		if isMarket {
			odType = banexg.OdTypeTakeProfit
			if market.Contract {
				odType = banexg.OdTypeTakeProfitMarket
			}
		} else if isLimit {
			odType = banexg.OdTypeTakeProfitLimit
			if market.Contract {
				odType = banexg.OdTypeTakeProfit
			}
		}
	}
	if marginMode == banexg.MarginIsolated {
		args["isIsolated"] = true
	}
	if clientOrderId == "" {
		broker := "x-R4BD3S82"
		if market.Contract {
			broker = "x-xcKtGhcu"
		}
		clientOrderId = broker + utils.UUID(22)
	}
	args["newClientOrderId"] = clientOrderId
	odRspType := "RESULT"
	if market.Spot || market.Type == banexg.MarketMargin {
		if rspType, ok := e.newOrderRespType[odType]; ok {
			odRspType = rspType
		}
	}
	// 'ACK' for order id, 'RESULT' for full order or 'FULL' for order with fills
	args["newOrderRespType"] = odRspType
	exgOdType := strings.ToUpper(odType)
	if market.Option {
		if odType == banexg.OdTypeMarket {
			return nil, errs.NewMsg(errs.CodeParamInvalid, "market order is invalid for option")
		}
	} else if !isBnbOrderType(market, exgOdType) {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "invalid order type %s for %s market", exgOdType, market.Type)
	}
	args["type"] = exgOdType
	timeInForceRequired, priceRequired, stopPriceRequired, quantityRequired := false, false, false, false
	/*
	   # spot/margin
	   #
	   #     LIMIT                timeInForce, quantity, price
	   #     MARKET               quantity or quoteOrderQty
	   #     STOP_LOSS            quantity, stopPrice
	   #     STOP_LOSS_LIMIT      timeInForce, quantity, price, stopPrice
	   #     TAKE_PROFIT          quantity, stopPrice
	   #     TAKE_PROFIT_LIMIT    timeInForce, quantity, price, stopPrice
	   #     LIMIT_MAKER          quantity, price
	   #
	   # futures
	   #
	   #     LIMIT                timeInForce, quantity, price
	   #     MARKET               quantity
	   #     STOP/TAKE_PROFIT     quantity, price, stopPrice
	   #     STOP_MARKET          stopPrice
	   #     TAKE_PROFIT_MARKET   stopPrice
	   #     TRAILING_STOP_MARKET callbackRate
	*/
	if odType == banexg.OdTypeMarket {
		quantityRequired = true
		if market.Spot {
			cost := utils.PopMapVal(args, banexg.ParamCost, 0.0)
			if cost == 0 && price != 0 {
				cost = amount * price
			}
			if cost != 0 {
				precRes, err := e.PrecCost(market, cost)
				if err != nil {
					return nil, err
				}
				args["quoteOrderQty"] = precRes
			}
		}
	} else if odType == banexg.OdTypeLimit {
		priceRequired = true
		timeInForceRequired = true
		quantityRequired = true
	} else if odType == banexg.OdTypeStopLoss || odType == banexg.OdTypeTakeProfit {
		stopPriceRequired = true
		quantityRequired = true
		if market.Linear || market.Inverse {
			priceRequired = true
		}
	} else if odType == banexg.OdTypeStopLossLimit || odType == banexg.OdTypeTakeProfitLimit {
		quantityRequired = true
		stopPriceRequired = true
		priceRequired = true
		timeInForceRequired = true
	} else if odType == banexg.OdTypeLimitMaker {
		priceRequired = true
		quantityRequired = true
	} else if odType == banexg.OdTypeStop {
		quantityRequired = true
		stopPriceRequired = true
		priceRequired = true
	} else if odType == banexg.OdTypeStopMarket || odType == banexg.OdTypeTakeProfitMarket {
		closePosition := utils.GetMapVal(args, banexg.ParamClosePosition, false)
		if !closePosition {
			quantityRequired = true
		}
		stopPriceRequired = true
	} else if odType == banexg.OdTypeTrailingStopMarket {
		quantityRequired = true
		callBackRate := utils.GetMapVal(args, banexg.ParamCallbackRate, 0.0)
		if callBackRate == 0 {
			return nil, errs.NewMsg(errs.CodeParamRequired, "createOrder require callbackRate for %s order", odType)
		}
	}
	if quantityRequired {
		amtStr, err := e.PrecAmount(market, amount)
		if err != nil {
			return nil, err
		}
		args["quantity"] = amtStr
	}
	if priceRequired {
		if price == 0 {
			return nil, errs.NewMsg(errs.CodeParamRequired, "createOrder require price for %s order", odType)
		}
		priceStr, err := e.PrecPrice(market, price)
		if err != nil {
			return nil, err
		}
		args["price"] = priceStr
	}
	if timeInForceRequired {
		if timeInForce == "" {
			timeInForce = e.TimeInForce
		}
		args["timeInForce"] = timeInForce
	}
	if market.Contract && postOnly {
		args["timeInForce"] = banexg.TimeInForceGTX
	}
	if stopPriceRequired {
		if market.Contract {
			if stopPrice == 0 {
				return nil, errs.NewMsg(errs.CodeParamRequired, "createOrder require stopPrice for %s order", odType)
			}
		} else if trailingDelta == 0 && stopPrice == 0 {
			return nil, errs.NewMsg(errs.CodeParamRequired, "createOrder require stopPrice/trailingDelta for %s order", odType)
		}
		if stopPrice != 0 {
			stopPriceStr, err := e.PrecPrice(market, stopPrice)
			if err != nil {
				return nil, err
			}
			args["stopPrice"] = stopPriceStr
		}
	}
	if timeInForce == banexg.TimeInForcePO {
		delete(args, banexg.ParamTimeInForce)
	}
	method := "privatePostOrder"
	if sor {
		method = "privatePostSorOrder"
	} else if market.Linear {
		method = "fapiPrivatePostOrder"
	} else if market.Inverse {
		method = "dapiPrivatePostOrder"
	} else if market.Type == banexg.MarketMargin || marginMode != "" {
		method = "sapiPostMarginOrder"
	} else if market.Option {
		method = "eapiPrivatePostOrder"
	}
	if market.Spot || market.Type == banexg.MarketMargin {
		test := utils.GetMapVal(args, banexg.ParamTest, false)
		if test {
			method += "Test"
		}
	}
	tryNum := e.GetRetryNum("CreateOrder", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	var mapSymbol = func(mid string) string {
		return market.Symbol
	}
	if method == "fapiPrivatePostOrder" {
		return parseOrder[*FutureOrder](mapSymbol, rsp)
	} else if method == "dapiPrivatePostOrder" {
		return parseOrder[*InverseOrder](mapSymbol, rsp)
	} else if method == "eapiPrivatePostOrder" {
		return parseOrder[*OptionOrder](mapSymbol, rsp)
	} else {
		// spot margin sor
		return parseOrder[*SpotOrder](mapSymbol, rsp)
	}
}
