package binance

import (
	"net/http"
	"strings"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

func mapBinanceError(api *banexg.Entry, status int, content string) *errs.Error {
	if status == http.StatusTeapot {
		return errs.NewMsg(errs.CodeTemporarilyBanned, "exchange temporarily banned this client")
	}
	if status == http.StatusTooManyRequests {
		return errs.NewMsg(errs.CodeRateLimit, "exchange rate limit exceeded")
	}
	if status >= http.StatusInternalServerError && api != nil && api.Risky {
		return errs.NewMsg(errs.CodeExecutionUnknown, "exchange did not confirm whether the trading request executed")
	}
	var response struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if utils.UnmarshalString(content, &response, utils.JsonNumDefault) != nil || response.Code == 0 {
		return nil
	}
	return newBinanceError(response.Code, response.Msg)
}

func newBinanceError(nativeCode int, message string) *errs.Error {
	code := errs.CodeExchangeError
	msg := strings.ToLower(message)
	switch nativeCode {
	case -1002, -2014:
		code = errs.CodeUnauthorized
	case -1022:
		code = errs.CodeSignFail
	case -2015, -4056:
		code = errs.CodeAccKeyError
	case -1021, -1131, -5028:
		code = errs.CodeExpired
	case -1003, -1015, -12014:
		code = errs.CodeRateLimit
	case -1001, -1004, -1008, -1016:
		code = errs.CodeSystemBusy
	case -1006, -1007:
		code = errs.CodeExecutionUnknown
	case -1121, -3028, -4144:
		code = errs.CodeSymbolInvalid
	case -1112, -2016, -3021, -4141:
		code = errs.CodeMarketUnavailable
	case -2013:
		code = errs.CodeOrderNotFound
	case -2011:
		if strings.Contains(msg, "unknown") || strings.Contains(msg, "not found") || strings.Contains(msg, "does not exist") {
			code = errs.CodeOrderNotFound
		} else {
			code = errs.CodeOrderNotCancelable
		}
	case -2010:
		switch {
		case strings.Contains(msg, "insufficient") || strings.Contains(msg, "balance"):
			code = errs.CodeInsufficientFunds
		case strings.Contains(msg, "duplicate") || strings.Contains(msg, "client order id"):
			code = errs.CodeDuplicateRequest
		case strings.Contains(msg, "immediately trigger"):
			code = errs.CodeOrderWouldTrigger
		default:
			code = errs.CodeOrderRejected
		}
	case -2021:
		code = errs.CodeOrderWouldTrigger
	case -2022:
		code = errs.CodeReduceOnlyRejected
	case -3002, -2018:
		code = errs.CodeInsufficientFunds
	case -2019, -4050, -4051:
		code = errs.CodeInsufficientMargin
	case -2027, -2028, -4164, -4400, -4401, -4402, -4403:
		code = errs.CodeRiskLimit
	case -4061, -4062, -4067, -4068:
		code = errs.CodePositionModeConflict
	case -4115, -4116:
		code = errs.CodeDuplicateRequest
	case -4046, -4052, -4059, -5027:
		code = errs.CodeNoChange
	case -2023, -3022, -4192:
		code = errs.CodeAccountRestricted
	case -1125, -3038:
		code = errs.CodeStreamExpired
	default:
		if nativeCode <= -1100 && nativeCode >= -1134 || nativeCode <= -4000 && nativeCode >= -4999 {
			code = errs.CodeParamInvalid
		}
	}
	return errs.NewMsg(code, "%s", message)
}
