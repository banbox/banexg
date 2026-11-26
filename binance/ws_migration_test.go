package binance

import (
	"net/url"
	"strings"
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

func TestUserDataEventEnvelope(t *testing.T) {
	msg, err := unwrapUserDataEvent(`{"subscriptionId":1,"event":{"e":"executionReport","s":"BTCUSDT","i":9007199254740993}}`)
	if err != nil {
		t.Fatal(err)
	}
	if msg == nil || msg.Event != "executionReport" || msg.Object["s"] != "BTCUSDT" || msg.Object["i"] != "9007199254740993" {
		t.Fatalf("unexpected event: %+v", msg)
	}
}

func TestMarginAndOptionUserDataUsePrivateRoute(t *testing.T) {
	exg, err := New(map[string]interface{}{banexg.OptApiKey: "key", banexg.OptApiSecret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	post, put := listenKeyMethods(banexg.MarketMargin)
	for method, verb := range map[string]string{post: "POST", put: "PUT"} {
		api := exg.Apis[method]
		if api == nil || api.Path != "margin/listen-key" || api.Method != verb || api.Host != HostSApiV2 {
			t.Fatalf("unexpected margin listenKey api %s: %+v", method, api)
		}
		api.Url = exg.GetHost(api.Host) + "/" + api.Path
		req := exg.Sign(api, map[string]interface{}{banexg.MidListenKey: "abc"})
		if req.Headers.Get("X-MBX-APIKEY") != "key" || strings.Contains(req.Url+req.Body, "signature") {
			t.Fatalf("margin listenKey is an api-key only request: %s %s", req.Url, req.Body)
		}
	}
	for _, marketType := range []string{banexg.MarketMargin, banexg.MarketOption} {
		if got := exg.userDataWsURL(marketType, "abc"); got != "wss://fstream.binance.com/private/ws/abc" {
			t.Fatalf("%s user data url = %s", marketType, got)
		}
	}
	if post, _ := listenKeyMethods(banexg.MarketOption); post != MethodEapiPrivatePostListenKey {
		t.Fatalf("option listenKey method = %s", post)
	}

	var order OptionOrder
	if err := utils.UnmarshalString(`{"selfTradePreventionMode":"EXPIRE_TAKER"}`, &order, utils.JsonNumDefault); err != nil {
		t.Fatal(err)
	}
	if order.SelfTradePreventionMode != "EXPIRE_TAKER" {
		t.Fatalf("selfTradePreventionMode = %q", order.SelfTradePreventionMode)
	}
}

func TestOptionStreamsUseRoutedHost(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	exg.OnWsMsg = func(*banexg.WsClient, *banexg.WsMsg) {}
	for msgHash, route := range map[string]string{"depth": "public", "kline": "market"} {
		host := linearWsHost(exg.GetHost(banexg.MarketOption), linearWsRoute(msgHash))
		if host != "wss://fstream.binance.com/"+route+"/ws" {
			t.Fatalf("option %s host = %s", msgHash, host)
		}
	}
	test, _ := New(map[string]interface{}{banexg.OptEnv: "test"})
	for key, host := range test.Hosts.Test {
		if strings.Contains(host, "binancefuture.com") || strings.Contains(host, "nbstream") {
			t.Fatalf("retired testnet host %s: %s", key, host)
		}
	}
}

func TestSpotUserDataUsesSignedWsApiSubscription(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	if spotURL := exg.userDataWsURL(banexg.MarketSpot, ""); spotURL != exg.GetHost(WssApi) {
		t.Fatalf("spot user data must use the WebSocket API, got %s", spotURL)
	}
	// the retired listenKey REST endpoint must not be called for spot
	if err := exg.postListenKey(&banexg.Account{Data: map[string]interface{}{}}, map[string]interface{}{banexg.ParamMarket: banexg.MarketSpot}); err != nil {
		t.Fatalf("spot auth should be a no-op: %v", err)
	}

	creds := &banexg.Credential{ApiKey: "key", Secret: "secret"}
	req, err := spotUserDataRequest(3, creds, 1700000000000, 5000)
	if err != nil {
		t.Fatal(err)
	}
	params := req["params"].(map[string]interface{})
	want, _ := utils.Signature("apiKey=key&recvWindow=5000&timestamp=1700000000000", "secret", "hmac", "sha256", "hex")
	if req["method"] != spotUserDataSubKey || params["signature"] != want || params["apiKey"] != "key" {
		t.Fatalf("unexpected spot subscribe request: %+v", req)
	}
}

func TestSignedRequestSignsPercentEncodedPayload(t *testing.T) {
	exg, err := New(map[string]interface{}{banexg.OptApiKey: "key", banexg.OptApiSecret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	api := exg.Apis[MethodFapiPrivateDeleteBatchOrders]
	api.Url = exg.GetHost(api.Host) + "/" + api.Path
	req := exg.Sign(api, map[string]interface{}{"symbol": "币安人生USDT", banexg.ParamOrderIds: []string{"1", "2"}})
	if req.Error != nil {
		t.Fatal(req.Error)
	}
	query := req.Url[strings.Index(req.Url, "?")+1:]
	signed, sign, ok := strings.Cut(query, "&signature=")
	want, _ := utils.Signature(signed, "secret", "hmac", "sha256", "hex")
	if !ok || sign != want {
		t.Fatalf("signature must cover the exact payload sent: %s", query)
	}
	if strings.ContainsAny(signed, "[],币") || !strings.Contains(signed, "orderidlist=%5B1%2C2%5D") {
		t.Fatalf("payload must be percent-encoded before signing: %s", signed)
	}
}
