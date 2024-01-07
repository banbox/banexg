package errs

const (
	CodeNetFail = -1*iota - 1
	CodeNotSupport
	CodeInvalidRequest
	CodeAccKeyError
	CodeMissingApiKey
	CodeCredsRequired
	CodeSignFail
	CodeNotImplement
	CodeMarketNotLoad
	CodeNoMarketForPair
	CodeUnsupportMarket
	CodeSandboxApiNotSupport
	CodeApiNotSupport
	CodeInvalidResponse
	CodeUnmarshalFail
	CodeParamRequired
	CodeParamInvalid
	CodeWsInvalidMsg
	CodeWsReadFail
	CodeConnectFail
	CodeInvalidTimeFrame
	CodePrecDecFail
	CodeBadExgName
)

var (
	MissingApiKey        = NewMsg(CodeMissingApiKey, "ApiKey missing")
	CredsRequired        = NewMsg(CodeCredsRequired, "credential fields missing")
	ApiNotSupport        = NewMsg(CodeApiNotSupport, "api not support")
	SandboxApiNotSupport = NewMsg(CodeSandboxApiNotSupport, "sandbox api not support")
	UnsupportMarket      = NewMsg(CodeUnsupportMarket, "unsupported market type")
	NoMarketForPair      = NewMsg(CodeNoMarketForPair, "no market found for pair")
	MarketNotLoad        = NewMsg(CodeMarketNotLoad, "markets not loaded")
	NotImplement         = NewMsg(CodeNotImplement, "method not implement")
	InvalidTimeFrame     = NewMsg(CodeInvalidTimeFrame, "invalid timeframe")
)
