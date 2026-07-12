package binance

import (
	"testing"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/utils"
)

func TestLinearWsRoute(t *testing.T) {
	tests := map[string]string{
		"linear@depth":      linearWsRoutePublic,
		"linear@bookTicker": linearWsRoutePublic,
		"linear@aggTrade":   linearWsRouteMarket,
		"linear@kline":      linearWsRouteMarket,
		"linear@markPrice":  linearWsRouteMarket,
		"linear@ticker":     linearWsRouteMarket,
	}
	for msgHash, want := range tests {
		if got := linearWsRoute(msgHash); got != want {
			t.Errorf("linearWsRoute(%q) = %q, want %q", msgHash, got, want)
		}
	}
	if got := linearWsHost("wss://fstream.binance.com/ws", linearWsRoutePublic); got != "wss://fstream.binance.com/public/ws" {
		t.Fatalf("linearWsHost() = %q", got)
	}
	if got := linearPrivateWsHost("wss://fstream.binance.com/ws"); got != "wss://fstream.binance.com/private/ws" {
		t.Fatalf("linearPrivateWsHost() = %q", got)
	}
}

func TestDefaultTradeStream(t *testing.T) {
	tests := []struct {
		market *banexg.Market
		want   string
	}{
		{market: &banexg.Market{Spot: true}, want: "trade"},
		{market: &banexg.Market{Contract: true, Linear: true}, want: "aggTrade"},
		{market: &banexg.Market{Contract: true, Inverse: true}, want: "aggTrade"},
		{market: &banexg.Market{Contract: true, Option: true}, want: "optionTrade"},
	}
	for _, test := range tests {
		if got := defaultTradeStream(test.market); got != test.want {
			t.Errorf("defaultTradeStream(%+v) = %q, want %q", test.market, got, test.want)
		}
	}
}

func TestMarginUserDataRequestAndEvent(t *testing.T) {
	req := marginUserDataRequest(7, "token-1")
	if req["method"] != marginUserDataSubKey {
		t.Fatalf("method = %v", req["method"])
	}
	params := req["params"].(map[string]interface{})
	if params["listenToken"] != "token-1" {
		t.Fatalf("listenToken = %v", params["listenToken"])
	}

	msg, err := unwrapUserDataEvent(`{"subscriptionId":1,"event":{"e":"executionReport","s":"BTCUSDT","i":9007199254740993}}`)
	if err != nil {
		t.Fatal(err)
	}
	if msg == nil || msg.Event != "executionReport" || msg.Object["s"] != "BTCUSDT" || msg.Object["i"] != "9007199254740993" {
		t.Fatalf("unexpected event: %+v", msg)
	}
}

func TestMarginUserListenTokenEndpointAndOptionSTP(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	api := exg.Apis[MethodSapiPostUserListenToken]
	if api == nil || api.Path != "userListenToken" || api.Method != "POST" || api.Host != HostSApi {
		t.Fatalf("unexpected user listen token API: %+v", api)
	}

	var order OptionOrder
	if err := utils.UnmarshalString(`{"selfTradePreventionMode":"EXPIRE_TAKER"}`, &order, utils.JsonNumDefault); err != nil {
		t.Fatal(err)
	}
	if order.SelfTradePreventionMode != "EXPIRE_TAKER" {
		t.Fatalf("selfTradePreventionMode = %q", order.SelfTradePreventionMode)
	}
}
