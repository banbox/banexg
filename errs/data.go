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
}
