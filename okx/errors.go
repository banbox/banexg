package okx

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

func mapOKXHTTPError(api *banexg.Entry, status int, content string) *errs.Error {
	if status == http.StatusTooManyRequests {
		return errs.NewMsg(errs.CodeRateLimit, "exchange rate limit exceeded")
	}
	if status >= http.StatusInternalServerError && api != nil && api.Risky {
		return errs.NewMsg(errs.CodeExecutionUnknown, "exchange did not confirm whether the trading request executed")
	}
	var response struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}
	if utils.UnmarshalString(content, &response, utils.JsonNumDefault) != nil || response.Code == "" || response.Code == "0" {
		return nil
	}
	return newOKXError(response.Code, response.Msg)
}

func newOKXError(nativeCode, message string) *errs.Error {
	code := errs.CodeExchangeError
	baseText := strings.SplitN(nativeCode, "_", 2)[0]
	base, _ := strconv.Atoi(baseText)
	msg := strings.ToLower(message)
	switch {
	case nativeCode == "51008_1000" || nativeCode == "51008_1002" || nativeCode == "51008_1015" || nativeCode == "51008_1016" || nativeCode == "51008_1020":
		code = errs.CodeInsufficientFunds
	case nativeCode == "51008_1001" || nativeCode == "51008_1003" || nativeCode == "51008_1009" || nativeCode == "51008_1010":
		code = errs.CodeInsufficientMargin
	case base >= 50101 && base <= 50114 || base == 50119 || base >= 60004 && base <= 60009 || base == 60024 || base == 60032:
		code = errs.CodeAccKeyError
	case base == 50030 || base == 50035 || base == 50120 || base == 50121 || base == 64003:
		code = errs.CodeForbidden
	case base == 50011 || base == 50040 || base == 50061 || base == 58102 || base == 60014 || base == 60023:
		code = errs.CodeRateLimit
	case base == 50001 || base == 50013 || base == 50026 || base == 63999 || base == 64007 || base == 64008:
		code = errs.CodeSystemBusy
	case base == 50004 || base == 51412:
		code = errs.CodeExecutionUnknown
	case base == 51001 || base == 51002 || base == 51069 || base == 52000 || base == 54007 || base == 60018:
		code = errs.CodeSymbolInvalid
	case base == 51011 || base == 51016 || base == 51065 || base == 51068 || base == 50042 || base == 50071:
		code = errs.CodeDuplicateRequest
	case base == 51063 || base == 51603:
		code = errs.CodeOrderNotFound
	case base >= 51400 && base <= 51402 || base == 51503 || base == 51509 || base == 51510:
		code = errs.CodeOrderNotCancelable
	case base == 51023 || base == 51043 || base == 51058:
		code = errs.CodeDataNotFound
	case base == 51025 || base == 51165 || base == 51399 || base == 54006 || base == 54030 || base == 54031 || base == 54035:
		code = errs.CodeRiskLimit
	case base == 51117 || base == 51148 || base == 51205 || base == 51206 || base == 51328 || base == 51333 || base == 51521 || base == 51522:
		code = errs.CodeReduceOnlyRejected
	case base == 51008:
		if strings.Contains(msg, "margin") {
			code = errs.CodeInsufficientMargin
		} else {
			code = errs.CodeInsufficientFunds
		}
	case base == 51005 || base == 51007 || base == 51020 || base == 51101 || base == 51121 || base >= 51201 && base <= 51204:
		code = errs.CodePrecisionViolation
	case base == 51006 || base == 51031 || base == 51250 || base >= 51046 && base <= 51053:
		code = errs.CodeParamInvalid
	case base == 50000 || base == 50002 || base == 50006 || base == 50014 || base == 50015 || base == 50016 || base == 50024 || base == 50025 || base == 51000 || base == 60012 || base == 60013 || base == 60019 || base == 60027 || base == 60033:
		code = errs.CodeParamInvalid
	}
	return errs.NewMsg(code, "%s", message)
}
