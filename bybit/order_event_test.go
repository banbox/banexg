package bybit

import (
	"context"

	"testing"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
)

func TestBuildPurposeClientOrderID(t *testing.T) {
	exg := &Bybit{}
	enter := exg.BuildPurposeClientOrderID("bot", 123, "abc", banexg.ClientOrderPurposeEnter)
	exit := exg.BuildPurposeClientOrderID("bot", 123, "abc", banexg.ClientOrderPurposeExit)
	if enter != "bot_123_z_abc" {
		t.Fatalf("unexpected enter id: %s", enter)
	}
	if exit != "bot_123_y_abc" {
		t.Fatalf("unexpected exit id: %s", exit)
	}
	if enter == exit {
		t.Fatal("entry and exit ids must differ for one order")
	}
	if exg.BuildPurposeClientOrderID("bot", 123, "abc", banexg.ClientOrderPurposeEnter) != enter {
		t.Fatal("purpose ids must be stable so resubmits stay idempotent")
	}
	if got := exg.ParseClientOrderID("bot", enter); got != 123 {
		t.Fatalf("purpose id must remain parseable, got %d", got)
	}
	// A randomized segment is numeric, so it can never reproduce a marker form.
	for i := 0; i < 200; i++ {
		if got := exg.BuildClientOrderID("bot", 123, "abc", true); got == enter || got == exit {
			t.Fatalf("randomized id %s collided with a purpose id", got)
		}
	}
}

func TestIndeterminateSubmit(t *testing.T) {
	exg := &Bybit{}
	uncertain := []int{errs.CodeNetFail, errs.CodeConnectFail, errs.CodeIOReadFail,
		errs.CodeIOWriteFail, errs.CodeTimeout, errs.CodeInvalidResponse,
		errs.CodeServerError, errs.CodeExecutionUnknown, errs.CodeDuplicateRequest}
	for _, code := range uncertain {
		if !exg.IndeterminateSubmit(errs.NewMsg(code, "boom")) {
			t.Fatalf("code %d must be indeterminate", code)
		}
	}
	certain := []int{errs.CodeInsufficientFunds, errs.CodeParamInvalid, errs.CodeOrderRejected}
	for _, code := range certain {
		if exg.IndeterminateSubmit(errs.NewMsg(code, "rejected")) {
			t.Fatalf("code %d must be conclusive", code)
		}
	}
	if exg.IndeterminateSubmit(nil) {
		t.Fatal("nil error is not indeterminate")
	}
}

func TestFindSubmittedOrder(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	params := map[string]interface{}{banexg.ParamAccount: "acc-1"}
	setBybitTestRequest(t, func(_ context.Context, endpoint string, got map[string]interface{}, _ int, _ bool, _ bool) *banexg.HttpRes {
		if endpoint != MethodPrivateGetV5OrderRealtime {
			t.Fatalf("unexpected endpoint: %s", endpoint)
		}
		if got["orderLinkId"] != "bot_123_z_abc" {
			t.Fatalf("expected orderLinkId, got %v", got["orderLinkId"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"list":[{"orderId":"order-1","orderLinkId":"bot_123_z_abc","symbol":"BTCUSDT","side":"Buy","orderType":"Market","orderStatus":"Accepted","timeInForce":"GTC","price":"0","qty":"1","cumExecQty":"0","cumExecValue":"0","avgPrice":"0","createdTime":"1700000000000","updatedTime":"1700000001000"}],"nextPageCursor":""},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	res, err := exg.FindSubmittedOrder("BTC/USDT:USDT", "bot_123_z_abc", params)
	if err != nil {
		t.Fatalf("FindSubmittedOrder failed: %v", err)
	}
	if res == nil || res.ID != "order-1" || res.ClientOrderID != "bot_123_z_abc" {
		t.Fatalf("unexpected order: %+v", res)
	}
	if _, ok := params[banexg.ParamClientOrderId]; ok {
		t.Fatal("caller params must not be mutated")
	}
}

func TestFindSubmittedOrderConfirmedAbsent(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	empty := `{"retCode":0,"retMsg":"OK","result":{"list":[],"nextPageCursor":""},"retExtInfo":{},"time":1700000000000}`
	setBybitTestRequest(t, func(_ context.Context, endpoint string, _ map[string]interface{}, _ int, _ bool, _ bool) *banexg.HttpRes {
		switch endpoint {
		case MethodPrivateGetV5OrderRealtime, MethodPrivateGetV5OrderHistory:
			return &banexg.HttpRes{Status: 200, Content: empty}
		default:
			t.Fatalf("unexpected endpoint: %s", endpoint)
			return nil
		}
	})
	res, err := exg.FindSubmittedOrder("BTC/USDT:USDT", "bot_123_z_abc", nil)
	if err != nil {
		t.Fatalf("absent order must not error: %v", err)
	}
	if res != nil {
		t.Fatalf("expected nil order, got %+v", res)
	}
}

func TestFindSubmittedOrderLookupFailure(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	setBybitTestRequest(t, func(_ context.Context, endpoint string, _ map[string]interface{}, _ int, _ bool, _ bool) *banexg.HttpRes {
		body := `{"retCode":10007,"retMsg":"not enough money","result":{},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})
	if _, err := exg.FindSubmittedOrder("BTC/USDT:USDT", "bot_123_z_abc", nil); err == nil {
		t.Fatal("expected lookup error")
	} else if exg.IndeterminateSubmit(err) {
		t.Fatalf("lookup error must be conclusive: %v", err)
	}
	if _, err := exg.FindSubmittedOrder("BTC/USDT:USDT", "", nil); err == nil || err.Code != errs.CodeParamRequired {
		t.Fatalf("expected required param error, got %v", err)
	}
}
