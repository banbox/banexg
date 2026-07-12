package banexg

import (
	"strings"
	"testing"

	"github.com/banbox/banexg/errs"
)

func TestCheckWsErrorDoesNotExposeNativePayload(t *testing.T) {
	err := CheckWsError(map[string]string{"error": `{"code":12345,"msg":"bad request"}`})
	if err == nil || err.Code != errs.CodeExchangeError || strings.Contains(err.Short(), "12345") {
		t.Fatalf("expected neutral websocket error, got %v", err)
	}
}

func TestCheckWsErrorUsesExchangeMapper(t *testing.T) {
	err := CheckWsErrorWith(map[string]string{"error": `{"code":12345,"msg":"bad request"}`},
		func(_ int, _ string) *errs.Error {
			return errs.NewMsg(errs.CodeParamInvalid, "bad request")
		})
	if err == nil || err.Code != errs.CodeParamInvalid || err.BizCode != 0 {
		t.Fatalf("expected mapped websocket error, got %v", err)
	}
}
