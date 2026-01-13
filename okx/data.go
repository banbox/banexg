package okx

import "github.com/banbox/banexg"

const (
	HostPublic     = "public"
	HostPrivate    = "private"
	HostWsPublic   = "ws_public"
	HostWsPrivate  = "ws_private"
	HostWsBusiness = "ws_business"
)

// OKX API field keys
const (
	FldInstType        = "instType"
	FldInstId          = "instId"
	FldInstFamily      = "instFamily"
	FldUly             = "uly"
	FldMgnMode         = "mgnMode"
	FldTdMode          = "tdMode"
	FldPosSide         = "posSide"
	FldOrdType         = "ordType"
	FldSide            = "side"
	FldSz              = "sz"
	FldPx              = "px"
	FldOrdId           = "ordId"
	FldClOrdId         = "clOrdId"
	FldCcy             = "ccy"
	FldLever           = "lever"
	FldBar             = "bar"
	FldLimit           = "limit"
	FldAfter           = "after"
	FldBefore          = "before"
	FldBegin           = "begin"
	FldEnd             = "end"
	FldType            = "type"
	FldChannel         = "channel"
	FldTgtCcy          = "tgtCcy"
	FldNewSz           = "newSz"
	FldNewPx           = "newPx"
	FldReduceOnly      = "reduceOnly"
	FldAlgoId          = "algoId"
	FldAlgoClOrdId     = "algoClOrdId"
	FldTpTriggerPx     = "tpTriggerPx"
	FldTpOrdPx         = "tpOrdPx"
	FldSlTriggerPx     = "slTriggerPx"
	FldSlOrdPx         = "slOrdPx"
	FldTpTriggerPxType = "tpTriggerPxType"
	FldSlTriggerPxType = "slTriggerPxType"
	FldTriggerPx       = "triggerPx"
	FldOrderPx         = "orderPx"
	FldOrdPx           = "ordPx"
)

// OKX WebSocket channel names
const (
	WsChanTrades          = "trades"
	WsChanBooks           = "books"
	WsChanBooks5          = "books5"
	WsChanBalancePosition = "balance_and_position"
	WsChanOrders          = "orders"
	WsChanMarkPrice       = "mark-price"
	WsChanCandlePrefix    = "candle"
)

// OKX instType values
const (
	InstTypeSpot    = "SPOT"
	InstTypeMargin  = "MARGIN"
	InstTypeSwap    = "SWAP"
	InstTypeFutures = "FUTURES"
	InstTypeOption  = "OPTION"
)

// OKX tdMode values
const (
	TdModeCash = "cash"
)

// OKX special values
const (
	TgtCcyQuote = "quote_ccy"
	InstIdAny   = "ANY"
)

var (
	DefCareMarkets = []string{
		banexg.MarketSpot,
		banexg.MarketLinear,
		banexg.MarketInverse,
	}

	timeFrameMap = map[string]string{
		"1m": "1m", "3m": "3m", "5m": "5m", "15m": "15m", "30m": "30m",
		"1h": "1H", "2h": "2H", "4h": "4H", "6h": "6H", "12h": "12H",
		"1d": "1D", "1w": "1W", "1M": "1M",
	}

	orderStatusMap = map[string]string{
		"live":             banexg.OdStatusOpen,
		"partially_filled": banexg.OdStatusPartFilled,
		"filled":           banexg.OdStatusFilled,
		"cancelling":       banexg.OdStatusCanceling,
		"canceled":         banexg.OdStatusCanceled,
		"mmp_canceled":     banexg.OdStatusCanceled,
	}

	orderTypeMap = map[string]string{
		banexg.OdTypeLimit:      "limit",
		banexg.OdTypeMarket:     "market",
		banexg.OdTypeLimitMaker: "post_only",
	}

	okxOrderTypeMap = map[string]string{
		"limit":     banexg.OdTypeLimit,
		"market":    banexg.OdTypeMarket,
		"post_only": banexg.OdTypeLimitMaker,
	}

	algoOrderStatusMap = map[string]string{
		"live":                banexg.OdStatusOpen,
		"pause":               banexg.OdStatusOpen,
		"partially_effective": banexg.OdStatusPartFilled,
		"effective":           banexg.OdStatusFilled,
		"canceled":            banexg.OdStatusCanceled,
		"order_failed":        banexg.OdStatusRejected,
	}
)

const (
	MethodPublicGetInstruments         = "publicGetInstruments"
	MethodMarketGetTicker              = "marketGetTicker"
	MethodMarketGetTickers             = "marketGetTickers"
	MethodMarketGetBooks               = "marketGetBooks"
	MethodMarketGetCandles             = "marketGetCandles"
	MethodPublicGetFundingRate         = "publicGetFundingRate"
	MethodPublicGetFundingRateHistory  = "publicGetFundingRateHistory"
	MethodPublicGetPositionTiers       = "publicGetPositionTiers"
	MethodAccountGetBalance            = "accountGetBalance"
	MethodAccountGetBills              = "accountGetBills"
	MethodAccountGetBillsArchive       = "accountGetBillsArchive"
	MethodAccountGetPositions          = "accountGetPositions"
	MethodAccountGetLeverageInfo       = "accountGetLeverageInfo"
	MethodAccountGetPositionTiers      = "accountGetPositionTiers"
	MethodAccountSetLeverage           = "accountSetLeverage"
	MethodTradePostOrder               = "tradePostOrder"
	MethodTradePostCancelOrder         = "tradePostCancelOrder"
	MethodTradePostAmendOrder          = "tradePostAmendOrder"
	MethodTradeGetOrder                = "tradeGetOrder"
	MethodTradeGetOrdersPending        = "tradeGetOrdersPending"
	MethodTradeGetOrdersHistory        = "tradeGetOrdersHistory"
	MethodTradeGetOrdersHistoryArchive = "tradeGetOrdersHistoryArchive"
	MethodTradePostOrderAlgo           = "tradePostOrderAlgo"
	MethodTradePostCancelAlgos         = "tradePostCancelAlgos"
	MethodTradePostAmendAlgos          = "tradePostAmendAlgos"
	MethodTradeGetOrderAlgo            = "tradeGetOrderAlgo"
	MethodTradeGetOrdersAlgoPending    = "tradeGetOrdersAlgoPending"
	MethodTradeGetOrdersAlgoHistory    = "tradeGetOrdersAlgoHistory"
)
