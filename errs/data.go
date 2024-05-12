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
)

var (
	PrintErr func(e error) string // print string for common error
)
