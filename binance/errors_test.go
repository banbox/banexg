package binance

import (
	"strings"
	"testing"

	"github.com/banbox/banexg/errs"
)

func TestMapBinanceErrorUsesNeutralCodes(t *testing.T) {
	tests := []struct {
		native  int
		message string
		want    int
	}{
		{-2013, "Order does not exist.", errs.CodeOrderNotFound},
		{-2022, "ReduceOnly Order is rejected.", errs.CodeReduceOnlyRejected},
		{-2021, "Order would immediately trigger.", errs.CodeOrderWouldTrigger},
		{-1007, "Timeout waiting for response.", errs.CodeExecutionUnknown},
		{-2010, "Account has insufficient balance for requested action.", errs.CodeInsufficientFunds},
		{-2010, "Duplicate order sent.", errs.CodeDuplicateRequest},
		{-999999, "New exchange failure", errs.CodeExchangeError},
	}
	for _, test := range tests {
		err := newBinanceError(test.native, test.message)
		if err.Code != test.want || err.BizCode != 0 {
			t.Fatalf("native %d: expected neutral code %d, got %#v", test.native, test.want, err)
		}
		if strings.Contains(err.Short(), "-201") || strings.Contains(err.Short(), "-1007") {
			t.Fatalf("native code leaked in public error: %s", err.Short())
		}
	}
}
