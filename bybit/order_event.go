package bybit

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

var _ banexg.OrderEventCapability = (*Bybit)(nil)
var _ banexg.ClientOrderCapability = (*Bybit)(nil)
var _ banexg.ClientOrderPurposeCapability = (*Bybit)(nil)
var _ banexg.SubmitReconcileCapability = (*Bybit)(nil)

// Bybit's orderLinkId layout is {namespace}_{orderID}_{segment}_{clientID},
// where the random segment is up to three digits. The purpose markers are
// letters, so they can never collide with a randomized id of the same order.
const (
	bybitEnterClientOrderMarker = "z"
	bybitExitClientOrderMarker  = "y"
)

// ParseClientOrderID parses Bybit's {botName}_{orderID}_... format.
func (*Bybit) ParseClientOrderID(botName, clientID string) int64 {
	if clientID == "" || botName == "" {
		return 0
	}
	prefix := botName + "_"
	if !strings.HasPrefix(clientID, prefix) {
		return 0
	}
	value := clientID[len(prefix):]
	if separator := strings.IndexByte(value, '_'); separator >= 0 {
		value = value[:separator]
	}
	orderID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || orderID <= 0 {
		return 0
	}
	return orderID
}

// BuildClientOrderID keeps Bybit's underscore-separated orderLinkId layout at
// the adapter boundary. The client ID is an optional persisted suffix.
func (*Bybit) BuildClientOrderID(namespace string, orderID int64, clientID string, randomize bool) string {
	if randomize {
		return fmt.Sprintf("%s_%d_%d_%s", namespace, orderID, rand.Intn(1000), clientID)
	}
	return fmt.Sprintf("%s_%d_%s", namespace, orderID, clientID)
}

// BuildPurposeClientOrderID swaps the random segment of BuildClientOrderID for
// a fixed per-purpose marker. Entry and exit of one local order therefore can
// not draw the same orderLinkId, and rebuilding the id for the same order is
// idempotent, which is what lets an uncertain submission be reconciled.
func (*Bybit) BuildPurposeClientOrderID(namespace string, orderID int64, clientID string,
	purpose banexg.ClientOrderPurpose) string {
	marker := bybitEnterClientOrderMarker
	if purpose == banexg.ClientOrderPurposeExit {
		marker = bybitExitClientOrderMarker
	}
	return fmt.Sprintf("%s_%d_%s_%s", namespace, orderID, marker, clientID)
}

// IndeterminateSubmit reports submissions where Bybit may have accepted the
// order even though the client never received a conclusive response.
func (*Bybit) IndeterminateSubmit(err *errs.Error) bool {
	if err == nil {
		return false
	}
	switch err.Code {
	case errs.CodeNetFail, errs.CodeConnectFail, errs.CodeIOReadFail, errs.CodeIOWriteFail,
		errs.CodeTimeout, errs.CodeInvalidResponse, errs.CodeServerError,
		errs.CodeExecutionUnknown, errs.CodeDuplicateRequest:
		return true
	default:
		return false
	}
}

// FindSubmittedOrder resolves an orderLinkId via v5 order realtime, falling
// back to order history inside FetchOrder. A confirmed unknown order returns
// (nil, nil) so the caller can distinguish it from a failed lookup.
func (e *Bybit) FindSubmittedOrder(symbol, clientOrderID string,
	params map[string]interface{}) (*banexg.Order, *errs.Error) {
	if clientOrderID == "" {
		return nil, errs.NewMsg(errs.CodeParamRequired, "client order id is required")
	}
	args := utils.SafeParams(params)
	args[banexg.ParamClientOrderId] = clientOrderID
	res, err := e.FetchOrder(symbol, "", args)
	if err != nil {
		if err.Code == errs.CodeDataNotFound || err.Code == errs.CodeOrderNotFound {
			return nil, nil
		}
		return nil, err
	}
	return res, nil
}
