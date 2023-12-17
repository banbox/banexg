package banexg

import (
	"errors"
	"github.com/anyongjin/banexg/utils"
)

const (
	ParamNewClientOrderId   = "newClientOrderId"
	ParamOrderIds           = "orderIdList"
	ParamOrigClientOrderIDs = "origClientOrderIdList"

	UriEncodeSafe = utils.UriEncodeSafe
)

var (
	ErrMissingApiKey        = errors.New("ApiKey missing")
	ErrCredsRequired        = errors.New("credential fields missing")
	ErrUnSupportSign        = utils.ErrUnSupportSign
	ErrApiNotSupport        = errors.New("api not support")
	ErrSandboxApiNotSupport = errors.New("sandbox api not support")
	ErrUnsupportMarket      = errors.New("unsupported market type")
	ErrNoMarketForPair      = errors.New("no market found for pair")
	ErrMarketNotLoad        = errors.New("markets not loaded")
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
	IsUnitTest = false
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
	OptProxy         = "Proxy"
	OptApiKey        = "ApiKey"
	OptApiSecret     = "ApiSecret"
	OptUserAgent     = "UserAgent"
	OptReqHeaders    = "ReqHeaders"
	OptCareMarkets   = "CareMarkets"
	OptPrecisionMode = "PrecisionMode"
	OptTradeMode     = "TradeMode"
	OptTradeInverse  = "TradeInverse"
)

const (
	RoundModeTruncate  = 0
	RoundModeRound     = 1
	RoundModeRoundUp   = 2
	RoundModeRoundDown = 3
)

const (
	PrecModeDecimalPlace = 2
	PrecModeSignifDigits = 3
	PrecModeTickSize     = 4
)

const (
	PaddingNo   = 5
	PaddingZero = 6
)

const (
	TradeSpot   = "spot"   // 现货交易
	TradeMargin = "margin" // 保证金杠杆现货交易 margin trade
	TradeSwap   = "swap"   // 永续合约 for perpetual swap futures that don't have a delivery date
	TradeFuture = "future" // 有交割日的期货 for expiring futures contracts that have a delivery/settlement date
	TradeOption = "option" // 期权 for option contracts
)
