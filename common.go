package banexg

import (
	"github.com/banbox/banexg/base"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

type FuncNewExchange = func(map[string]interface{}) (BanExchange, *errs.Error)

var newExgs map[string]FuncNewExchange

type BanExchange = base.BanExchange

var (
	ParamClientOrderId      = base.ParamClientOrderId
	ParamOrderIds           = base.ParamOrderIds
	ParamOrigClientOrderIDs = base.ParamOrigClientOrderIDs
	ParamSor                = base.ParamSor
	ParamPostOnly           = base.ParamPostOnly
	ParamTimeInForce        = base.ParamTimeInForce
	ParamTriggerPrice       = base.ParamTriggerPrice
	ParamStopLossPrice      = base.ParamStopLossPrice
	ParamTakeProfitPrice    = base.ParamTakeProfitPrice
	ParamTrailingDelta      = base.ParamTrailingDelta
	ParamReduceOnly         = base.ParamReduceOnly
	ParamCost               = base.ParamCost
	ParamClosePosition      = base.ParamClosePosition
	ParamCallbackRate       = base.ParamCallbackRate
	ParamRolling            = base.ParamRolling
	ParamTest               = base.ParamTest
	ParamMarginMode         = base.ParamMarginMode
	ParamSymbol             = base.ParamSymbol
	ParamPositionSide       = base.ParamPositionSide
	ParamProxy              = base.ParamProxy
	ParamName               = base.ParamName
	ParamMethod             = base.ParamMethod
	ParamInterval           = base.ParamInterval
	ParamAccount            = base.ParamAccount
)

const (
	HasFail     = base.HasFail
	HasOk       = base.HasOk
	HasEmulated = base.HasEmulated
)

const (
	BoolNull  = base.BoolNull
	BoolFalse = base.BoolFalse
	BoolTrue  = base.BoolTrue
)

const (
	OptProxy           = base.OptProxy
	OptApiKey          = base.OptApiKey
	OptApiSecret       = base.OptApiSecret
	OptAccCreds        = base.OptAccCreds
	OptAccName         = base.OptAccName
	OptUserAgent       = base.OptUserAgent
	OptReqHeaders      = base.OptReqHeaders
	OptCareMarkets     = base.OptCareMarkets
	OptPrecisionMode   = base.OptPrecisionMode
	OptMarketType      = base.OptMarketType
	OptContractType    = base.OptContractType
	OptTimeInForce     = base.OptTimeInForce
	OptWsIntvs         = base.OptWsIntvs
	OptRetries         = base.OptRetries
	OptWsConn          = base.OptWsConn
	OptAuthRefreshSecs = base.OptAuthRefreshSecs
	OptPositionMethod  = base.OptPositionMethod
)

const (
	PrecModeDecimalPlace = utils.PrecModeDecimalPlace
	PrecModeSignifDigits = utils.PrecModeSignifDigits
	PrecModeTickSize     = utils.PrecModeTickSize
)

const (
	MarketSpot    = base.MarketSpot   // 现货交易
	MarketMargin  = base.MarketMargin // 保证金杠杆现货交易 margin trade
	MarketLinear  = base.MarketLinear
	MarketInverse = base.MarketInverse
	MarketOption  = base.MarketOption // 期权 for option contracts
	MarketSwap    = base.MarketSwap   // 永续合约 for perpetual swap futures that don't have a delivery date
	MarketFuture  = base.MarketFuture // 有交割日的期货 for expiring futures contracts that have a delivery/settlement date
)

const (
	MarginCross    = base.MarginCross
	MarginIsolated = base.MarginIsolated
)

const (
	OdStatusOpen      = base.OdStatusOpen
	OdStatusClosed    = base.OdStatusClosed
	OdStatusCanceled  = base.OdStatusCanceled
	OdStatusCanceling = base.OdStatusCanceling
	OdStatusRejected  = base.OdStatusRejected
	OdStatusExpired   = base.OdStatusExpired
)

const (
	OdTypeMarket          = base.OdTypeMarket
	OdTypeLimit           = base.OdTypeLimit
	OdTypeStopLoss        = base.OdTypeStopLoss
	OdTypeStopLossLimit   = base.OdTypeStopLossLimit
	OdTypeTakeProfit      = base.OdTypeTakeProfit
	OdTypeTakeProfitLimit = base.OdTypeTakeProfitLimit
	OdTypeStop            = base.OdTypeStop
	OdTypeLimitMaker      = base.OdTypeLimitMaker
)

const (
	OdSideBuy  = base.OdSideBuy
	OdSideSell = base.OdSideSell
)

const (
	PosSideLong  = base.PosSideLong
	PosSideShort = base.PosSideShort
	PosSideBoth  = base.PosSideBoth
)

const (
	TimeInForceGTC = base.TimeInForceGTC // Good Till Cancel 一直有效，直到被成交或取消
	TimeInForceIOC = base.TimeInForceIOC // Immediate or Cancel 无法立即成交的部分取消
	TimeInForceFOK = base.TimeInForceFOK // Fill or Kill 无法全部立即成交就撤销
	TimeInForceGTX = base.TimeInForceGTX // Good Till Crossing 无法成为挂单方就取消
	TimeInForceGTD = base.TimeInForceGTD // Good Till Date 在特定时间前有效，到期自动取消
	TimeInForcePO  = base.TimeInForcePO  // Post Only
)

var (
	ParamHandshakeTimeout = base.ParamHandshakeTimeout
	ParamChanCaps         = base.ParamChanCaps
	ParamChanCap          = base.ParamChanCap
)

type FuncSign = base.FuncSign
type FuncFetchCurr = base.FuncFetchCurr
type FuncFetchMarkets = base.FuncFetchMarkets
type FuncAuth = base.FuncAuth
type FuncOnWsMsg = base.FuncOnWsMsg
type FuncOnWsMethod = base.FuncOnWsMethod
type FuncOnWsErr = base.FuncOnWsErr
type FuncOnWsClose = base.FuncOnWsClose
type FuncGetWsJob = base.FuncGetWsJob
type Exchange = base.Exchange
type Account = base.Account
type ExgHosts = base.ExgHosts
type ExgFee = base.ExgFee
type TradeFee = base.TradeFee
type FeeTiers = base.FeeTiers
type FeeTierItem = base.FeeTierItem
type Entry = base.Entry
type Credential = base.Credential
type HttpReq = base.HttpReq
type HttpRes = base.HttpRes
type CurrencyMap = base.CurrencyMap
type Currency = base.Currency
type ChainNetwork = base.ChainNetwork
type CodeLimits = base.CodeLimits
type LimitRange = base.LimitRange
type Market = base.Market
type Precision = base.Precision
type MarketLimits = base.MarketLimits
type MarketMap = base.MarketMap
type MarketArrMap = base.MarketArrMap
type Ticker = base.Ticker
type OhlcvArr = base.OhlcvArr
type Kline = base.Kline
type SymbolKline = base.SymbolKline
type Balances = base.Balances
type Asset = base.Asset
type Position = base.Position
type Order = base.Order
type Trade = base.Trade
type MyTrade = base.MyTrade
type Fee = base.Fee
type OrderBook = base.OrderBook
type OrderBookSide = base.OrderBookSide
type WsJobInfo = base.WsJobInfo
type WsMsg = base.WsMsg
type WsClient = base.WsClient
type WebSocket = base.WebSocket
