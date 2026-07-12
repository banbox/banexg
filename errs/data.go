package errs

import (
	"github.com/sasha-s/go-deadlock"
)

const (
	CodeNetFail = -1*iota - 1
	CodeNotSupport
	CodeInvalidRequest
	CodeAccKeyError
	CodeMissingApiKey
	CodeCredsRequired
	CodeSignFail
	CodeRunTime
	CodeNotImplement
	CodeMarketNotLoad
	CodeNoMarketForPair
	CodeUnsupportMarket
	CodeSandboxApiNotSupport
	CodeApiNotSupport
	CodeInvalidResponse
	CodeMarshalFail
	CodeUnmarshalFail
	CodeParamRequired
	CodeParamInvalid
	CodeWsInvalidMsg
	CodeWsReadFail
	CodeConnectFail
	CodeInvalidTimeFrame
	CodePrecDecFail
	CodeBadExgName
	CodeIOWriteFail
	CodeIOReadFail
	CodeInvalidData
	CodeExpired
	CodeNetDisable
	CodeCancel
	CodeShutdown
	CodeTimeout
	CodeOOM
	CodeSystemBusy
	CodeUnauthorized
	CodeForbidden
	CodeDataNotFound
	CodeServerError
	CodeNoTrade
	CodeRateLimit
	CodeTemporarilyBanned
	CodeExecutionUnknown
	CodeExchangeError
	CodeSymbolInvalid
	CodeMarketUnavailable
	CodeOrderNotFound
	CodeOrderRejected
	CodeInsufficientFunds
	CodeInsufficientMargin
	CodeRiskLimit
	CodePositionModeConflict
	CodeReduceOnlyRejected
	CodeDuplicateRequest
	CodeNoChange
	CodeAccountRestricted
	CodeStreamExpired
	CodeOrderWouldTrigger
	CodeOrderNotCancelable
	CodeLeverageInvalid
	CodePrecisionViolation
)

var (
	PrintErr     func(e error) string // print string for common error
	codeNameLock = deadlock.Mutex{}
)

var errCodeNames = map[int]string{
	CodeNetFail:              "NetFail",
	CodeNotSupport:           "NotSupport",
	CodeInvalidRequest:       "InvalidRequest",
	CodeAccKeyError:          "AccKeyError",
	CodeMissingApiKey:        "MissingApiKey",
	CodeCredsRequired:        "CredsRequired",
	CodeSignFail:             "SignFail",
	CodeRunTime:              "RunTime",
	CodeNotImplement:         "NotImplement",
	CodeMarketNotLoad:        "MarketNotLoad",
	CodeNoMarketForPair:      "NoMarketForPair",
	CodeUnsupportMarket:      "UnsupportMarket",
	CodeSandboxApiNotSupport: "SandboxApiNotSupport",
	CodeApiNotSupport:        "ApiNotSupport",
	CodeInvalidResponse:      "InvalidResponse",
	CodeMarshalFail:          "MarshalFail",
	CodeUnmarshalFail:        "UnmarshalFail",
	CodeParamRequired:        "ParamRequired",
	CodeParamInvalid:         "ParamInvalid",
	CodeWsInvalidMsg:         "WsInvalidMsg",
	CodeWsReadFail:           "WsReadFail",
	CodeConnectFail:          "ConnectFail",
	CodeInvalidTimeFrame:     "InvalidTimeFrame",
	CodePrecDecFail:          "PrecDecFail",
	CodeBadExgName:           "BadExgName",
	CodeIOWriteFail:          "IOWriteFail",
	CodeIOReadFail:           "IOReadFail",
	CodeInvalidData:          "InvalidData",
	CodeExpired:              "Expired",
	CodeNetDisable:           "NetDisable",
	CodeCancel:               "Cancel",
	CodeShutdown:             "Shutdown",
	CodeTimeout:              "Timeout",
	CodeOOM:                  "OOM",
	CodeSystemBusy:           "SystemBusy",
	CodeUnauthorized:         "Unauthorized",
	CodeForbidden:            "Forbidden",
	CodeDataNotFound:         "DataNotFound",
	CodeServerError:          "ServerError",
	CodeNoTrade:              "NoTrade",
	CodeRateLimit:            "RateLimit",
	CodeTemporarilyBanned:    "TemporarilyBanned",
	CodeExecutionUnknown:     "ExecutionUnknown",
	CodeExchangeError:        "ExchangeError",
	CodeSymbolInvalid:        "SymbolInvalid",
	CodeMarketUnavailable:    "MarketUnavailable",
	CodeOrderNotFound:        "OrderNotFound",
	CodeOrderRejected:        "OrderRejected",
	CodeInsufficientFunds:    "InsufficientFunds",
	CodeInsufficientMargin:   "InsufficientMargin",
	CodeRiskLimit:            "RiskLimit",
	CodePositionModeConflict: "PositionModeConflict",
	CodeReduceOnlyRejected:   "ReduceOnlyRejected",
	CodeDuplicateRequest:     "DuplicateRequest",
	CodeNoChange:             "NoChange",
	CodeAccountRestricted:    "AccountRestricted",
	CodeStreamExpired:        "StreamExpired",
	CodeOrderWouldTrigger:    "OrderWouldTrigger",
	CodeOrderNotCancelable:   "OrderNotCancelable",
	CodeLeverageInvalid:      "LeverageInvalid",
	CodePrecisionViolation:   "PrecisionViolation",
}
