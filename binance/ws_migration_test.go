package binance

import (
	"net/url"
	"testing"
	"time"

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

func TestLinearUserDataWsURL(t *testing.T) {
	got := linearUserDataWsURL("wss://fstream.binance.com/ws", "listen-key")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/private/ws" {
		t.Fatalf("path = %q, want /private/ws", parsed.Path)
	}
	query := parsed.Query()
	if query.Get("listenKey") != "listen-key" {
		t.Fatalf("listenKey = %q", query.Get("listenKey"))
	}
	if query.Get("events") != linearUserDataEvents {
		t.Fatalf("events = %q, want %q", query.Get("events"), linearUserDataEvents)
	}
	wantRawQuery := "listenKey=listen-key&events=" + url.QueryEscape(linearUserDataEvents)
	if parsed.RawQuery != wantRawQuery {
		t.Fatalf("raw query = %q, want %q", parsed.RawQuery, wantRawQuery)
	}
}

func TestListenKeyRetryDelay(t *testing.T) {
	want := []time.Duration{3 * time.Second, 6 * time.Second, 12 * time.Second, 30 * time.Second, 30 * time.Second}
	for attempt, expected := range want {
		if got := listenKeyRetryDelay(attempt); got != expected {
			t.Fatalf("attempt %d: got %s, want %s", attempt, got, expected)
		}
	}
}

func TestAlgoOrderUpdateLinksTriggeredTrade(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &banexg.WsClient{AccName: "acc", Key: "acc@old-listen-key", MarketType: banexg.MarketLinear}
	exg.handleAlgoOrderUpdate(client, map[string]string{
		"o": `{"aid":2148719,"ai":"1087109971","s":"BNBUSDT","X":"TRIGGERED"}`,
	})
	rotatedClient := &banexg.WsClient{AccName: "acc", Key: "acc@new-listen-key", MarketType: banexg.MarketLinear}
	trade := banexg.MyTrade{
		Trade: banexg.Trade{Order: "1087109971", Symbol: "BNBUSDT"},
		State: banexg.OdStatusFilled,
	}
	exg.linkAlgoOrder(rotatedClient, &trade)
	if trade.AlgoId != "2148719" {
		t.Fatalf("AlgoId = %q, want 2148719", trade.AlgoId)
	}
	again := banexg.MyTrade{Trade: banexg.Trade{Order: "1087109971", Symbol: "BNBUSDT"}}
	exg.linkAlgoOrder(rotatedClient, &again)
	if again.AlgoId != "" {
		t.Fatalf("terminal mapping remains: %q", again.AlgoId)
	}

	exg.handleAlgoOrderUpdate(client, map[string]string{
		"ao": `{"aid":99,"ai":"same-order","s":"ETHUSDT","X":"TRIGGERED"}`,
	})
	wrongSymbol := banexg.MyTrade{Trade: banexg.Trade{Order: "same-order", Symbol: "BNBUSDT"}}
	exg.linkAlgoOrder(client, &wrongSymbol)
	if wrongSymbol.AlgoId != "" {
		t.Fatalf("mapping crossed symbols: %q", wrongSymbol.AlgoId)
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
