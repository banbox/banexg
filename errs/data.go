package errs

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
)

var (
	PrintErr func(e error) string // print string for common error
)
