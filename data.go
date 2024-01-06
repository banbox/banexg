package banexg

import (
	"github.com/anyongjin/banexg/utils"
)

const (
	ParamClientOrderId      = "clientOrderId"
	ParamOrderIds           = "orderIdList"
	ParamOrigClientOrderIDs = "origClientOrderIdList"
	ParamSor                = "sor" // smart order route, for create order in spot
	ParamPostOnly           = "postOnly"
	ParamTimeInForce        = "timeInForce"
	ParamTriggerPrice       = "triggerPrice"
	ParamStopLossPrice      = "stopLossPrice"
	ParamTakeProfitPrice    = "takeProfitPrice"
	ParamTrailingDelta      = "trailingDelta"
	ParamReduceOnly         = "reduceOnly"
	ParamCost               = "cost"
	ParamClosePosition      = "closePosition" // 触发后全部平仓
	ParamCallbackRate       = "callbackRate"  // 跟踪止损回调百分比
	ParamRolling            = "rolling"
	ParamTest               = "test"
	ParamMarginMode         = "marginMode"
	ParamSymbol             = "symbol"
	ParamPositionSide       = "positionSide"
	ParamProxy              = "proxy"
	ParamName               = "name"
	ParamMethod             = "method"
	ParamInterval           = "interval"

	UriEncodeSafe = utils.UriEncodeSafe
)

var (
	DefReqHeaders = map[string]string{
		"User-Agent": "Go-http-client/1.1",
		"Connection": "keep-alive",
		"Accept":     "application/json",
	}
	DefCurrCodeMap = map[string]string{
		"XBT":   "BTC",
		"BCC":   "BCH",
		"BCHSV": "BSV",
	}
	DefWsIntvs = map[string]int{
		"WatchOrderBooks": 100,
	}
	DefRetries = map[string]int{
		"FetchOrderBook":     1,
		"FetchPositionsRisk": 1,
	}
)

const (
	DefTimeInForce = TimeInForceGTC
)

const (
	HasFail = 1 << iota
	HasOk
	HasEmulated
)

const (
	BoolNull  = 0
	BoolFalse = -1
	BoolTrue  = 1
)

const (
	OptProxy           = "Proxy"
	OptApiKey          = "ApiKey"
	OptApiSecret       = "ApiSecret"
	OptUserAgent       = "UserAgent"
	OptReqHeaders      = "ReqHeaders"
	OptCareMarkets     = "CareMarkets"
	OptPrecisionMode   = "PrecisionMode"
	OptMarketType      = "MarketType"
	OptContractType    = "ContractType"
	OptTimeInForce     = "TimeInForce"
	OptWsIntvs         = "WsIntvs" // ws 订阅间隔
	OptRetries         = "Retries"
	OptWsConn          = "WsConn"
	OptAuthRefreshSecs = "AuthRefreshSecs"
	OptPositionMethod  = "PositionMethod"
)

const (
	RoundModeTruncate  = 0
	RoundModeRound     = 1
	RoundModeRoundUp   = 2
	RoundModeRoundDown = 3
)

const (
	PrecModeDecimalPlace = utils.PrecModeDecimalPlace
	PrecModeSignifDigits = utils.PrecModeSignifDigits
	PrecModeTickSize     = utils.PrecModeTickSize
)

const (
	PaddingNo   = 5
	PaddingZero = 6
)

const (
	MarketSpot    = "spot"   // 现货交易
	MarketMargin  = "margin" // 保证金杠杆现货交易 margin trade
	MarketLinear  = "linear"
	MarketInverse = "inverse"
	MarketOption  = "option" // 期权 for option contracts

	MarketSwap   = "swap"   // 永续合约 for perpetual swap futures that don't have a delivery date
	MarketFuture = "future" // 有交割日的期货 for expiring futures contracts that have a delivery/settlement date
)

const (
	MarginCross    = "cross"
	MarginIsolated = "isolated"
)

const (
	OdStatusOpen      = "open"
	OdStatusClosed    = "closed"
	OdStatusCanceled  = "canceled"
	OdStatusCanceling = "canceling"
	OdStatusRejected  = "rejected"
	OdStatusExpired   = "expired"
)

const (
	OdTypeMarket          = "MARKET"
	OdTypeLimit           = "LIMIT"
	OdTypeStopLoss        = "STOP_LOSS"
	OdTypeStopLossLimit   = "STOP_LOSS_LIMIT"
	OdTypeTakeProfit      = "TAKE_PROFIT"
	OdTypeTakeProfitLimit = "TAKE_PROFIT_LIMIT"
	OdTypeStop            = "STOP"
	OdTypeLimitMaker      = "LIMIT_MAKER"
)

const (
	OdSideBuy  = "buy"
	OdSideSell = "sell"
)

const (
	PosSideLong  = "long"
	PosSideShort = "short"
	PosSideBoth  = "both"
)

const (
	TimeInForceGTC = "GTC" // Good Till Cancel 一直有效，直到被成交或取消
	TimeInForceIOC = "IOC" // Immediate or Cancel 无法立即成交的部分取消
	TimeInForceFOK = "FOK" // Fill or Kill 无法全部立即成交就撤销
	TimeInForceGTX = "GTX" // Good Till Crossing 无法成为挂单方就取消
	TimeInForceGTD = "GTD" // Good Till Date 在特定时间前有效，到期自动取消
	TimeInForcePO  = "PO"  // Post Only
)

const (
	MidListenKey = "listenKey"
)
