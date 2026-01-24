package bybit

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"strings"
	"testing"
)

// ---- api_account_access_test.go ----
func logAccountAccess(t *testing.T, res *banexg.AccountAccess) {
	t.Helper()
	if res == nil {
		t.Log("AccountAccess: <nil>")
		return
	}
	t.Logf("TradeKnown: %v, TradeAllowed: %v", res.TradeKnown, res.TradeAllowed)
	t.Logf("WithdrawKnown: %v, WithdrawAllowed: %v", res.WithdrawKnown, res.WithdrawAllowed)
	t.Logf("IPKnown: %v, IPAny: %v", res.IPKnown, res.IPAny)
	t.Logf("PosMode: %s", res.PosMode)
	t.Logf("AcctLv: %s, AcctMode: %s", res.AcctLv, res.AcctMode)
	t.Logf("MarginMode: %s", res.MarginMode)
	if res.Info == nil {
		t.Log("Info: <nil>")
	} else {
		t.Logf("Info keys: %d", len(res.Info))
	}
}

func requireAccountAccessNonEmpty(t *testing.T, res *banexg.AccountAccess) {
	t.Helper()
	if res == nil {
		t.Fatal("expected AccountAccess, got nil")
	}
	// Bybit FetchAccountAccess should populate at least IP and trade fields from /v5/user/query-api.
	if !res.TradeKnown {
		t.Fatalf("expected TradeKnown=true, got: %#v", res)
	}
	if !res.IPKnown {
		t.Fatalf("expected IPKnown=true, got: %#v", res)
	}
}

func TestApi_FetchAccountAccess(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	res, err := exg.FetchAccountAccess(nil)
	if err != nil {
		t.Fatalf("FetchAccountAccess: %v", err)
	}
	logAccountAccess(t, res)
	requireAccountAccessNonEmpty(t, res)
}

func TestApi_FetchAccountAccess_WithBalance(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	// Fetch balance first to pass balance.Info into FetchAccountAccess (internal ParamBalance).
	bal := fetchBalanceMust(t, exg, nil)
	res, err := exg.FetchAccountAccess(map[string]interface{}{
		banexg.ParamBalance: bal,
	})
	if err != nil {
		t.Fatalf("FetchAccountAccess: %v", err)
	}
	logAccountAccess(t, res)
	requireAccountAccessNonEmpty(t, res)
}

// ---- resp_test.go ----
func TestBybitTimeUnmarshal(t *testing.T) {
	var ts BybitTime
	if err := json.Unmarshal([]byte(`"1700000000000"`), &ts); err != nil {
		t.Fatalf("unmarshal string timestamp failed: %v", err)
	}
	if int64(ts) != 1700000000000 {
		t.Fatalf("unexpected timestamp from string: %d", ts)
	}
	if err := json.Unmarshal([]byte(`1700000000001`), &ts); err != nil {
		t.Fatalf("unmarshal numeric timestamp failed: %v", err)
	}
	if int64(ts) != 1700000000001 {
		t.Fatalf("unexpected timestamp from number: %d", ts)
	}
	ts = 1700000000002
	if err := json.Unmarshal([]byte(`null`), &ts); err != nil {
		t.Fatalf("unmarshal null timestamp failed: %v", err)
	}
	if int64(ts) != 1700000000002 {
		t.Fatalf("unexpected timestamp after null: %d", ts)
	}
}

func TestMapBybitRetCode(t *testing.T) {
	if err := mapBybitRetCode(0, "OK"); err != nil {
		t.Fatalf("expected nil for retCode 0, got %v", err)
	}
	cases := []struct {
		retCode  int
		wantCode int
	}{
		{10001, errs.CodeParamInvalid},
		{10003, errs.CodeAccKeyError},
		{10004, errs.CodeSignFail},
		{10005, errs.CodeUnauthorized},
		{10006, errs.CodeSystemBusy},
		{10028, errs.CodeForbidden},
		{110001, errs.CodeDataNotFound},
		{110004, errs.CodeNoTrade},
		{3100326, errs.CodeParamRequired},
	}
	for _, tc := range cases {
		err := mapBybitRetCode(tc.retCode, "msg")
		if err == nil {
			t.Fatalf("expected error for retCode %d", tc.retCode)
		}
		if err.Code != tc.wantCode {
			t.Fatalf("retCode %d expected code %d, got %d", tc.retCode, tc.wantCode, err.Code)
		}
		if err.BizCode != tc.retCode {
			t.Fatalf("retCode %d expected bizCode %d, got %d", tc.retCode, tc.retCode, err.BizCode)
		}
	}

	unknown := mapBybitRetCode(999999, "unknown")
	if unknown == nil {
		t.Fatal("expected error for unknown retCode")
	}
	if unknown.Code != errs.CodeRunTime {
		t.Fatalf("expected runtime code for unknown retCode, got %d", unknown.Code)
	}
	if unknown.BizCode != 999999 {
		t.Fatalf("expected bizCode 999999, got %d", unknown.BizCode)
	}
}

func TestParseBybitPct(t *testing.T) {
	if got := parseBybitPct("1.5"); got != 0.015 {
		t.Fatalf("expected 0.015, got %v", got)
	}
	if got := parseBybitPct("0"); got != 0 {
		t.Fatalf("expected 0 for 0 input, got %v", got)
	}
	if got := parseBybitPct("bad"); got != 0 {
		t.Fatalf("expected 0 for invalid input, got %v", got)
	}
}

func TestDecodeBybitList(t *testing.T) {
	type row struct {
		Symbol string `json:"symbol"`
		Price  string `json:"price"`
	}
	items := []map[string]interface{}{
		{"symbol": "BTCUSDT", "price": "123.45"},
		{"symbol": "ETHUSDT", "price": "234.56"},
	}
	decoded, err := decodeBybitList[row](items)
	if err != nil {
		t.Fatalf("decodeBybitList failed: %v", err)
	}
	if len(decoded) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(decoded))
	}
	if decoded[0].Symbol != "BTCUSDT" || decoded[1].Price != "234.56" {
		t.Fatalf("unexpected decode result: %#v", decoded)
	}

	empty, err := decodeBybitList[row](nil)
	if err != nil {
		t.Fatalf("decodeBybitList empty failed: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty slice, got %d", len(empty))
	}
}

func TestDecodeBybitListError(t *testing.T) {
	type row struct {
		Symbol int `json:"symbol"`
	}
	items := []map[string]interface{}{
		{"symbol": "bad"},
	}
	_, err := decodeBybitList[row](items)
	if err == nil {
		t.Fatal("expected decodeBybitList error")
	}
	if err.Code != errs.CodeUnmarshalFail {
		t.Fatalf("expected unmarshal fail code, got %d", err.Code)
	}
}

func TestV5RespUnmarshal(t *testing.T) {
	type result struct {
		TimeSecond string `json:"timeSecond"`
		TimeNano   string `json:"timeNano"`
	}
	raw := `{"retCode":0,"retMsg":"OK","result":{"timeSecond":"1688639403","timeNano":"1688639403423213947"},"retExtInfo":{},"time":"1688639403423"}`
	var rsp V5Resp[result]
	if err := json.Unmarshal([]byte(raw), &rsp); err != nil {
		t.Fatalf("unmarshal V5Resp failed: %v", err)
	}
	if rsp.RetCode != 0 || rsp.RetMsg != "OK" {
		t.Fatalf("unexpected retCode/retMsg: %d/%s", rsp.RetCode, rsp.RetMsg)
	}
	if rsp.Result.TimeSecond != "1688639403" || rsp.Result.TimeNano == "" {
		t.Fatalf("unexpected result: %#v", rsp.Result)
	}
	if int64(rsp.Time) != 1688639403423 {
		t.Fatalf("unexpected time: %d", rsp.Time)
	}
}

func TestRequestRetryParsing(t *testing.T) {
	exg := &Bybit{Exchange: &banexg.Exchange{}}
	setBybitTestRequest(t, func(_ context.Context, endpoint string, params map[string]interface{}, _ int, _, _ bool) *banexg.HttpRes {
		if endpoint != MethodPublicGetV5MarketTime {
			t.Fatalf("unexpected endpoint: %s", endpoint)
		}
		if params == nil {
			t.Fatal("params should not be nil")
		}
		content := `{"retCode":0,"retMsg":"OK","result":{"timeSecond":"1688639403","timeNano":"1688639403423213947"},"retExtInfo":{},"time":1688639403423}`
		return &banexg.HttpRes{Content: content}
	})
	res := requestRetry[struct {
		TimeSecond string `json:"timeSecond"`
		TimeNano   string `json:"timeNano"`
	}](exg, MethodPublicGetV5MarketTime, map[string]interface{}{}, 1)
	if res.Error != nil {
		t.Fatalf("requestRetry failed: %v", res.Error)
	}
	if res.Result.TimeSecond == "" || res.Result.TimeNano == "" {
		t.Fatalf("unexpected result: %#v", res.Result)
	}
}

func TestRequestRetryErrors(t *testing.T) {
	exg := &Bybit{Exchange: &banexg.Exchange{}}
	setBybitTestRequest(t, func(_ context.Context, endpoint string, params map[string]interface{}, _ int, _, _ bool) *banexg.HttpRes {
		content := `{"retCode":10001,"retMsg":"invalid","result":{},"retExtInfo":{},"time":1688639403423}`
		return &banexg.HttpRes{Content: content}
	})
	res := requestRetry[map[string]interface{}](exg, MethodPublicGetV5MarketTime, map[string]interface{}{}, 1)
	if res.Error == nil {
		t.Fatal("expected error for non-zero retCode")
	}
	if res.Error.Code != errs.CodeParamInvalid {
		t.Fatalf("expected param invalid code, got %d", res.Error.Code)
	}
	if res.Error.BizCode != 10001 {
		t.Fatalf("expected bizCode 10001, got %d", res.Error.BizCode)
	}

	setBybitTestRequest(t, func(_ context.Context, endpoint string, params map[string]interface{}, _ int, _, _ bool) *banexg.HttpRes {
		return &banexg.HttpRes{Content: `not-json`}
	})
	res = requestRetry[map[string]interface{}](exg, MethodPublicGetV5MarketTime, map[string]interface{}{}, 1)
	if res.Error == nil || res.Error.Code != errs.CodeUnmarshalFail {
		t.Fatalf("expected unmarshal error, got %v", res.Error)
	}
}

func TestFetchV5ListAll(t *testing.T) {
	exg := &Bybit{Exchange: &banexg.Exchange{}}
	call := 0
	setBybitTestRequest(t, func(_ context.Context, endpoint string, params map[string]interface{}, _ int, _, _ bool) *banexg.HttpRes {
		if endpoint != MethodPublicGetV5MarketTickers {
			t.Fatalf("unexpected endpoint: %s", endpoint)
		}
		call++
		if call == 1 {
			if _, ok := params["cursor"]; ok {
				t.Fatalf("expected no cursor for first call, got %v", params["cursor"])
			}
			content := `{"retCode":0,"retMsg":"OK","result":{"category":"spot","list":[{"symbol":"BTCUSDT"}],"nextPageCursor":"next"},"retExtInfo":{},"time":1700000000000}`
			return &banexg.HttpRes{Content: content}
		}
		if params["cursor"] != "next" {
			t.Fatalf("expected cursor=next, got %v", params["cursor"])
		}
		content := `{"retCode":0,"retMsg":"OK","result":{"category":"spot","list":[{"symbol":"ETHUSDT"}],"nextPageCursor":""},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Content: content}
	})

	args := map[string]interface{}{"category": "spot"}
	items, err := fetchV5List(exg, MethodPublicGetV5MarketTickers, args, 1, 0, 0)
	if err != nil {
		t.Fatalf("fetchV5ListAll failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0]["symbol"] != "BTCUSDT" || items[1]["symbol"] != "ETHUSDT" {
		t.Fatalf("unexpected items: %#v", items)
	}
}

func TestFetchV5ListAllError(t *testing.T) {
	exg := &Bybit{Exchange: &banexg.Exchange{}}
	setBybitTestRequest(t, func(_ context.Context, endpoint string, params map[string]interface{}, _ int, _, _ bool) *banexg.HttpRes {
		content := `{"retCode":10001,"retMsg":"invalid","result":{},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Content: content}
	})
	_, err := fetchV5List(exg, MethodPublicGetV5MarketTickers, map[string]interface{}{}, 1, 0, 0)
	if err == nil {
		t.Fatal("expected fetchV5ListAll error")
	}
	if err.Code != errs.CodeParamInvalid {
		t.Fatalf("expected param invalid error, got %v", err)
	}
}

// ---- ws_replay_test.go ----
func TestBybitReplayHandlesPublic(t *testing.T) {
	exg, _ := newBybitWsTest(t, "BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	exg.WsDecoder = gob.NewDecoder(strings.NewReader(""))
	exg.regReplayHandles()
	seedMarket(exg, "BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	if exg.WsReplayFn == nil {
		t.Fatal("WsReplayFn not initialized")
	}
	if err := exg.WsReplayFn["WatchOrderBooks"](&banexg.WsLog{Content: `["BTC/USDT"]`}); err != nil {
		t.Fatalf("replay WatchOrderBooks failed: %v", err)
	}
	if err := exg.WsReplayFn["WatchTrades"](&banexg.WsLog{Content: `["BTC/USDT"]`}); err != nil {
		t.Fatalf("replay WatchTrades failed: %v", err)
	}
	if err := exg.WsReplayFn["WatchOHLCVs"](&banexg.WsLog{Content: `[["BTC/USDT","1m"]]`}); err != nil {
		t.Fatalf("replay WatchOHLCVs failed: %v", err)
	}
	if err := exg.WsReplayFn["WatchMarkPrices"](&banexg.WsLog{Content: `["BTC/USDT:USDT"]`}); err != nil {
		t.Fatalf("replay WatchMarkPrices failed: %v", err)
	}
}

func TestBybitReplayWsMsg(t *testing.T) {
	exg, client := newBybitWsTest(t, "BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	exg.regReplayHandles()
	client.SubscribeKeys["publicTrade.BTCUSDT"] = 0
	out := wsOutChan[*banexg.Trade](exg, client, "trades")
	msg := `{"topic":"publicTrade.BTCUSDT","type":"snapshot","ts":1700000000000,"data":[{"T":1700000000000,"s":"BTCUSDT","S":"Buy","v":"0.1","p":"100","i":"t1"}]}`
	content, err := json.Marshal([]string{"wss://test", banexg.MarketSpot, "", msg})
	if err != nil {
		t.Fatalf("marshal wsMsg content: %v", err)
	}
	handle := exg.WsReplayFn["wsMsg"]
	if handle == nil {
		t.Fatal("wsMsg handle missing")
	}
	if err := handle(&banexg.WsLog{Content: string(content)}); err != nil {
		t.Fatalf("replay wsMsg failed: %v", err)
	}
	trade := readChan(t, out)
	if trade.Symbol != "BTC/USDT" || trade.Price != 100 || trade.Amount != 0.1 {
		t.Fatalf("unexpected trade from wsMsg: %+v", trade)
	}
}
