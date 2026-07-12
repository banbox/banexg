package okx

import (
	"strings"
	"testing"

	"github.com/banbox/banexg/errs"
)

func TestMapOKXErrorUsesNeutralCodes(t *testing.T) {
	tests := []struct {
		native string
		msg    string
		want   int
	}{
		{"51008_1000", "Insufficient balance", errs.CodeInsufficientFunds},
		{"51008_1001", "Insufficient margin", errs.CodeInsufficientMargin},
		{"51063", "Order does not exist", errs.CodeOrderNotFound},
		{"51117", "Reduce-only rule", errs.CodeReduceOnlyRejected},
		{"50011", "Rate limit reached", errs.CodeRateLimit},
		{"99999", "New exchange failure", errs.CodeExchangeError},
	}
	for _, test := range tests {
		err := newOKXError(test.native, test.msg)
		if err.Code != test.want || err.BizCode != 0 {
			t.Fatalf("native %s: expected neutral code %d, got %#v", test.native, test.want, err)
		}
		if strings.Contains(err.Short(), test.native) {
			t.Fatalf("native code leaked in public error: %s", err.Short())
		}
	}
}
