package banexg

import (
	"github.com/banbox/banexg/utils"
	"sync"
	"time"
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
	ParamSymbols            = "symbols"
	ParamPositionSide       = "positionSide"
	ParamProxy              = "proxy"
	ParamName               = "name"
	ParamMethod             = "method"
	ParamInterval           = "interval"
	ParamAccount            = "account"
	ParamMarket             = "market"
	ParamContract           = "contract"
	ParamBrokerId           = "brokerId"
	ParamLimit              = "limit"
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
	HostRetryWaits  = map[string]int64{}
	hostWaitLock    sync.Mutex
	hostFlowChans   = make(map[string]chan struct{})
	hostFlowLock    sync.Mutex
	HostHttpConcurr = 3 // Maximum concurrent number of HTTP requests per domain name 每个域名发起http请求最大并发数
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
	OptAccCreds        = "Creds"
	OptAccName         = "AccName"
	OptUserAgent       = "UserAgent"
	OptReqHeaders      = "ReqHeaders"
	OptCareMarkets     = "CareMarkets"
	OptMarketType      = "MarketType"
	OptContractType    = "ContractType"
	OptTimeInForce     = "TimeInForce"
	OptWsIntvs         = "WsIntvs" // ws 订阅间隔
	OptRetries         = "Retries"
	OptWsConn          = "WsConn"
	OptAuthRefreshSecs = "AuthRefreshSecs"
	OptPositionMethod  = "PositionMethod"
	OptDebugWS         = "DebugWS"
	OptDebugAPI        = "DebugAPI"
	OptApiCaches       = "ApiCaches"
	OptFees            = "Fees"
	OptDumpPath        = "DumpPath"
	OptDumpBatchSize   = "DumpBatchSize"
	OptReplayPath      = "ReplayPath"
)

const (
	PrecModeDecimalPlace = utils.PrecModeDecimalPlace // 保留小数点后位数
	PrecModeSignifDigits = utils.PrecModeSignifDigits // 保留有效数字位数
	PrecModeTickSize     = utils.PrecModeTickSize     // 返回给定数字的整数倍
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
	OdStatusOpen       = "open"
	OdStatusPartFilled = "part_filled"
	OdStatusFilled     = "filled"
	OdStatusCanceled   = "canceled"
	OdStatusCanceling  = "canceling"
	OdStatusRejected   = "rejected"
	OdStatusExpired    = "expired"
)

// 此处订单类型全部使用币安订单类型小写
const (
	OdTypeMarket             = "market"
	OdTypeLimit              = "limit"
	OdTypeLimitMaker         = "limit_maker"
	OdTypeStop               = "stop"
	OdTypeStopMarket         = "stop_market"
	OdTypeStopLoss           = "stop_loss"
	OdTypeStopLossLimit      = "stop_loss_limit"
	OdTypeTakeProfit         = "take_profit"
	OdTypeTakeProfitLimit    = "take_profit_limit"
	OdTypeTakeProfitMarket   = "take_profit_market"
	OdTypeTrailingStopMarket = "trailing_stop_market"
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

const (
	ApiFetchTicker           = "FetchTicker"
	ApiFetchTickers          = "FetchTickers"
	ApiFetchTickerPrice      = "FetchTickerPrice"
	ApiLoadLeverageBrackets  = "LoadLeverageBrackets"
	ApiFetchCurrencies       = "FetchCurrencies"
	ApiGetLeverage           = "GetLeverage"
	ApiFetchOHLCV            = "FetchOHLCV"
	ApiFetchOrderBook        = "FetchOrderBook"
	ApiFetchOrder            = "FetchOrder"
	ApiFetchOrders           = "FetchOrders"
	ApiFetchBalance          = "FetchBalance"
	ApiFetchAccountPositions = "FetchAccountPositions"
	ApiFetchPositions        = "FetchPositions"
	ApiFetchOpenOrders       = "FetchOpenOrders"
	ApiCreateOrder           = "CreateOrder"
	ApiEditOrder             = "EditOrder"
	ApiCancelOrder           = "CancelOrder"
	ApiSetLeverage           = "SetLeverage"
	ApiCalcMaintMargin       = "CalcMaintMargin"
	ApiWatchOrderBooks       = "WatchOrderBooks"
	ApiUnWatchOrderBooks     = "UnWatchOrderBooks"
	ApiWatchOHLCVs           = "WatchOHLCVs"
	ApiUnWatchOHLCVs         = "UnWatchOHLCVs"
	ApiWatchMarkPrices       = "WatchMarkPrices"
	ApiUnWatchMarkPrices     = "UnWatchMarkPrices"
	ApiWatchTrades           = "WatchTrades"
	ApiUnWatchTrades         = "UnWatchTrades"
	ApiWatchMyTrades         = "WatchMyTrades"
	ApiWatchBalance          = "WatchBalance"
	ApiWatchPositions        = "WatchPositions"
	ApiWatchAccountConfig    = "WatchAccountConfig"
)

var (
	AllMarketTypes = map[string]struct{}{
		MarketSpot:    {},
		MarketMargin:  {},
		MarketLinear:  {},
		MarketInverse: {},
		MarketOption:  {},
	}
	AllContractTypes = map[string]struct{}{
		MarketSwap:   {},
		MarketFuture: {},
	}
)

var (
	exgCacheMarkets     = map[string]MarketMap{}   // cache markets for exchanges before expired
	exgCacheCurrs       = map[string]CurrencyMap{} // cache currencies for exchanges before expired
	exgCareMarkets      = map[string][]string{}    // what market types was cached for exchanges
	exgMarketTS         = map[string]int64{}       // when was markets cached
	exgMarketExpireMins = 360                      // ttl minutes for markets cache
	marketsLock         sync.RWMutex               // 访问缓存的读写锁
	LocUTC, _           = time.LoadLocation("UTC")
)
