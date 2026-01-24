package bybit

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/utils"
)

func TestParseBybitWsTradeItem(t *testing.T) {
	exg := mustNewBybit(t, "Bybit")
	seedMarket(exg, "BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	item := map[string]interface{}{
		"T": int64(1700000000000),
		"s": "BTCUSDT",
		"S": "Buy",
		"v": "0.1",
		"p": "100",
		"i": "t1",
	}
	trade := parseBybitWsTradeItem(exg, item, banexg.MarketSpot)
	if trade == nil {
		t.Fatalf("unexpected nil trade")
	}
	if trade.Symbol != "BTC/USDT" || trade.Price != 100 || trade.Amount != 0.1 {
		t.Fatalf("unexpected trade: %+v", trade)
	}
	if trade.Side != banexg.OdSideBuy {
		t.Fatalf("unexpected side: %s", trade.Side)
	}
}

func TestParseBybitWsKlineItem(t *testing.T) {
	item := map[string]interface{}{
		"start":    int64(1700000000000),
		"open":     "100",
		"high":     "110",
		"low":      "90",
		"close":    "105",
		"volume":   "12.5",
		"turnover": "200",
	}
	kline := parseBybitWsKlineItem(item)
	if kline == nil {
		t.Fatalf("unexpected nil kline")
	}
	if kline.Open != 100 || kline.High != 110 || kline.Low != 90 || kline.Close != 105 {
		t.Fatalf("unexpected kline: %+v", kline)
	}
	if kline.Info != 200 {
		t.Fatalf("unexpected kline info: %+v", kline.Info)
	}
}

func TestApplyBybitWsOrderBook(t *testing.T) {
	exg := mustNewBybit(t, "Bybit")
	seedMarket(exg, "BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	market := exg.Markets["BTC/USDT"]
	if market == nil {
		t.Fatalf("missing market")
	}
	data := &orderBookSnapshot{
		Symbol: "BTCUSDT",
		Bids:   [][]string{{"100", "1"}},
		Asks:   [][]string{{"101", "2"}},
		Ts:     1700000000000,
		Update: 10,
	}
	book := applyBybitWsOrderBook(exg, market, data, "snapshot", 50)
	if book == nil {
		t.Fatalf("unexpected nil orderbook")
	}
	if book.Bids == nil || len(book.Bids.Price) != 1 || book.Bids.Price[0] != 100 {
		t.Fatalf("unexpected bids: %+v", book.Bids)
	}
	if book.Asks == nil || len(book.Asks.Price) != 1 || book.Asks.Price[0] != 101 {
		t.Fatalf("unexpected asks: %+v", book.Asks)
	}
}

func TestParseBybitWsMyTrade(t *testing.T) {
	exg := mustNewBybit(t, "Bybit")
	seedMarket(exg, "BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	item := map[string]interface{}{
		"symbol":      "BTCUSDT",
		"orderId":     "o1",
		"orderLinkId": "c1",
		"side":        "Sell",
		"orderType":   "Limit",
		"orderPrice":  "100",
		"orderQty":    "0.5",
		"leavesQty":   "0.4",
		"execId":      "e1",
		"execPrice":   "99.5",
		"execQty":     "0.1",
		"execValue":   "9.95",
		"execFee":     "0.01",
		"feeCurrency": "USDT",
		"execTime":    "1700000000000",
		"isMaker":     true,
	}
	trade := parseBybitWsMyTrade(exg, item, banexg.MarketSpot)
	if trade == nil {
		t.Fatalf("unexpected nil trade")
	}
	if trade.Symbol != "BTC/USDT" || trade.Price != 99.5 || trade.Amount != 0.1 {
		t.Fatalf("unexpected trade: %+v", trade)
	}
	if !utils.EqualNearly(trade.Filled, 0.1) || trade.State != banexg.OdStatusPartFilled {
		t.Fatalf("unexpected trade state: %+v", trade)
	}
	if trade.Fee == nil || trade.Fee.Currency != "USDT" {
		t.Fatalf("unexpected fee: %+v", trade.Fee)
	}
}

func TestParseBybitWsBalance(t *testing.T) {
	exg := mustNewBybit(t, "Bybit")
	info := map[string]interface{}{}
	bal := &WalletBalance{
		Coin: []WalletBalanceCoin{
			{
				Coin:            "USDT",
				WalletBalance:   "10",
				SpotBorrow:      "1",
				Locked:          "2",
				TotalOrderIM:    "1",
				TotalPositionIM: "0",
				Equity:          "9",
			},
		},
	}
	res := parseBybitBalance(exg, bal, info, true)
	if res == nil {
		t.Fatalf("unexpected nil balance")
	}
	asset := res.Assets["USDT"]
	if asset == nil {
		t.Fatalf("missing USDT asset")
	}
	if asset.Total != 9 {
		t.Fatalf("unexpected total: %+v", asset)
	}
}

func TestParseBybitWsPositions(t *testing.T) {
	exg := mustNewBybit(t, "Bybit")
	seedMarket(exg, "BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	info := map[string]interface{}{}
	item := &wsPositionInfo{
		Category: "linear",
		PositionInfo: PositionInfo{
			Symbol:        "BTCUSDT",
			Side:          "Buy",
			Size:          "1",
			AvgPrice:      "20000",
			MarkPrice:     "19900",
			PositionValue: "20000",
			Leverage:      "5",
			PositionIM:    "100",
			PositionMM:    "10",
			UnrealisedPnl: "-5",
			LiqPrice:      "15000",
			UpdatedTime:   "1700000000000",
		},
	}
	pos := parseBybitPosition(exg, &item.PositionInfo, info, banexg.MarketLinear)
	if pos == nil {
		t.Fatalf("unexpected nil position")
	}
	if pos.Symbol != "BTC/USDT:USDT" || pos.Side != banexg.PosSideLong {
		t.Fatalf("unexpected position: %+v", pos)
	}
}

func TestBybitWsOrderBookDepth(t *testing.T) {
	cases := []struct {
		marketType string
		limit      int
		want       int
	}{
		{banexg.MarketSpot, 0, 50},
		{banexg.MarketSpot, 1, 1},
		{banexg.MarketSpot, 120, 200},
		{banexg.MarketSpot, 999, 1000},
		{banexg.MarketOption, 0, 25},
		{banexg.MarketOption, 10, 25},
		{banexg.MarketOption, 30, 100},
		{banexg.MarketLinear, 0, 50},
		{banexg.MarketLinear, 200, 200},
		{banexg.MarketLinear, 999, 1000},
	}
	for _, c := range cases {
		if got := bybitWsOrderBookDepth(c.marketType, c.limit); got != c.want {
			t.Fatalf("depth mismatch market=%s limit=%d: got %d want %d", c.marketType, c.limit, got, c.want)
		}
	}
}

func TestBybitWsTradeSymbol(t *testing.T) {
	spot := &banexg.Market{ID: "BTCUSDT", Symbol: "BTC/USDT", Type: banexg.MarketSpot}
	if got := bybitWsTradeSymbol(spot); got != "BTCUSDT" {
		t.Fatalf("spot trade symbol mismatch: %s", got)
	}
	opt := &banexg.Market{
		ID:     "BTC-30JUN23-30000-P",
		Symbol: "BTC/USDT:BTC-30JUN23-30000-P",
		Type:   banexg.MarketOption,
		Option: true,
		Base:   "BTC",
	}
	if got := bybitWsTradeSymbol(opt); got != "BTC" {
		t.Fatalf("option trade symbol mismatch: %s", got)
	}
	optNoBase := &banexg.Market{
		ID:     "ETH-30JUN23-2000-C",
		Symbol: "ETH/USDT:ETH-30JUN23-2000-C",
		Type:   banexg.MarketOption,
		Option: true,
	}
	if got := bybitWsTradeSymbol(optNoBase); got != "ETH" {
		t.Fatalf("option trade symbol fallback mismatch: %s", got)
	}
	if got := bybitWsTradeSymbol(nil); got != "" {
		t.Fatalf("nil market should return empty symbol, got %s", got)
	}
}

func TestBybitWsOpSuccess(t *testing.T) {
	ok := true
	fail := false
	success, err := bybitWsOpSuccess(&wsBaseMsg{Success: &ok})
	if !success || err != nil {
		t.Fatalf("expected success without error, got %v %v", success, err)
	}
	success, err = bybitWsOpSuccess(&wsBaseMsg{Success: &fail, RetMsg: "fail"})
	if success || err == nil {
		t.Fatalf("expected failure with error, got %v %v", success, err)
	}
	success, err = bybitWsOpSuccess(&wsBaseMsg{Success: &fail, RetMsgAlt: "alt-fail"})
	if success || err == nil || err.Message() != "alt-fail" {
		t.Fatalf("expected failure with alt message, got %v %v", success, err)
	}
	success, err = bybitWsOpSuccess(&wsBaseMsg{RetCode: 0})
	if !success || err != nil {
		t.Fatalf("expected retCode=0 success, got %v %v", success, err)
	}
	success, err = bybitWsOpSuccess(&wsBaseMsg{RetCode: 20001})
	if !success || err != nil {
		t.Fatalf("expected retCode=20001 success, got %v %v", success, err)
	}
	success, err = bybitWsOpSuccess(&wsBaseMsg{RetCode: 10001, RetMsg: "invalid"})
	if success || err == nil {
		t.Fatalf("expected retCode error, got %v %v", success, err)
	}
	success, err = bybitWsOpSuccess(&wsBaseMsg{RetCodeAlt: 10001, RetMsgAlt: "invalid-alt"})
	if success || err == nil || !strings.Contains(err.Message(), "invalid-alt") {
		t.Fatalf("expected retCode alt error, got %v %v", success, err)
	}
	var base wsBaseMsg
	if err := utils.UnmarshalString(`{"op":"auth","success":false,"ret_msg":"bad"}`, &base, utils.JsonNumDefault); err != nil {
		t.Fatalf("unmarshal ws base msg failed: %v", err)
	}
	success, err = bybitWsOpSuccess(&base)
	if success || err == nil || err.Message() != "bad" {
		t.Fatalf("expected ret_msg to be used, got %v %v", success, err)
	}
}

func TestFillBybitWsOrderBookTs(t *testing.T) {
	base := &wsBaseMsg{Ts: 1700000000123}
	data := &orderBookSnapshot{Symbol: "BTCUSDT"}
	fillBybitWsOrderBookTs(base, data)
	if data.Ts != base.Ts {
		t.Fatalf("unexpected orderbook ts: %d", data.Ts)
	}
}

func TestDecodeWsTickerData(t *testing.T) {
	obj := []byte(`{"symbol":"BTCUSDT","markPrice":"100"}`)
	items, err := decodeWsTickerData(obj)
	if err != nil {
		t.Fatalf("decode ticker object failed: %v", err)
	}
	if len(items) != 1 || items[0]["symbol"] != "BTCUSDT" {
		t.Fatalf("unexpected ticker object decode: %+v", items)
	}
	list := []byte(`[{"symbol":"BTCUSDT","markPrice":"100"},{"symbol":"ETHUSDT","markPrice":"200"}]`)
	items, err = decodeWsTickerData(list)
	if err != nil {
		t.Fatalf("decode ticker list failed: %v", err)
	}
	if len(items) != 2 || items[1]["symbol"] != "ETHUSDT" {
		t.Fatalf("unexpected ticker list decode: %+v", items)
	}
}

func TestDecodeWsList(t *testing.T) {
	if items, err := decodeWsList(nil); err != nil || items != nil {
		t.Fatalf("expected nil items on empty input, got %v err=%v", items, err)
	}
	items, err := decodeWsList([]byte(`[]`))
	if err != nil {
		t.Fatalf("decode ws list failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty items, got %v", items)
	}
	_, err = decodeWsList([]byte(`{"bad":"json"}`))
	if err == nil {
		t.Fatal("expected error for non-list json")
	}
}

func TestBybitWsString(t *testing.T) {
	if got := bybitWsString("ok"); got != "ok" {
		t.Fatalf("expected ok, got %q", got)
	}
	if got := bybitWsString(json.Number("12.34")); got != "12.34" {
		t.Fatalf("expected 12.34, got %q", got)
	}
	if got := bybitWsString(float64(1.25)); got != "1.25" {
		t.Fatalf("expected 1.25, got %q", got)
	}
	if got := bybitWsString(int64(42)); got != "42" {
		t.Fatalf("expected 42, got %q", got)
	}
	if got := bybitWsString(int(7)); got != "7" {
		t.Fatalf("expected 7, got %q", got)
	}
	if got := bybitWsString(nil); got != "" {
		t.Fatalf("expected empty for nil, got %q", got)
	}
	type custom struct{ A int }
	if got := bybitWsString(custom{A: 1}); got == "" {
		t.Fatalf("expected non-empty string for custom type")
	}
}

func TestBybitWsBool(t *testing.T) {
	if !bybitWsBool(true) {
		t.Fatal("expected true for bool true")
	}
	if !bybitWsBool("true") || !bybitWsBool("1") {
		t.Fatal("expected true for string true/1")
	}
	if bybitWsBool("false") || bybitWsBool("0") {
		t.Fatal("expected false for string false/0")
	}
}

func TestSplitBybitTopic(t *testing.T) {
	prefix, mid, symbol := splitBybitTopic("kline.1.BTCUSDT")
	if prefix != "kline" || mid != "1" || symbol != "BTCUSDT" {
		t.Fatalf("unexpected split: %s %s %s", prefix, mid, symbol)
	}
	prefix, mid, symbol = splitBybitTopic("tickers.BTCUSDT")
	if prefix != "tickers" || mid != "BTCUSDT" || symbol != "" {
		t.Fatalf("unexpected split: %s %s %s", prefix, mid, symbol)
	}
}

func TestBybitTimeFrameFromInterval(t *testing.T) {
	if got := bybitTimeFrameFromInterval("1"); got != "1m" {
		t.Fatalf("expected 1m, got %q", got)
	}
	if got := bybitTimeFrameFromInterval("240"); got != "4h" {
		t.Fatalf("expected 4h, got %q", got)
	}
	got := bybitTimeFrameFromInterval("D")
	if got != "D" && got != "1d" {
		t.Fatalf("expected D or 1d, got %q", got)
	}
	if got := bybitTimeFrameFromInterval(""); got != "" {
		t.Fatalf("expected empty interval to return empty, got %q", got)
	}
}

func TestBybitWsKlineTopics(t *testing.T) {
	exg := mustNewBybit(t, "Bybit")
	seedMarket(exg, "BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	jobs := [][2]string{{"BTC/USDT", "1m"}}
	keys, refKeys, err := bybitWsKlineTopics(exg, jobs)
	if err != nil {
		t.Fatalf("kline topics error: %v", err)
	}
	if len(keys) != 1 || keys[0] != "kline.1.BTCUSDT" {
		t.Fatalf("unexpected kline keys: %+v", keys)
	}
	if len(refKeys) != 1 || refKeys[0] != "BTC/USDT@1" {
		t.Fatalf("unexpected kline ref keys: %+v", refKeys)
	}
}

func TestBybitWsMarkPriceTopics(t *testing.T) {
	exg := mustNewBybit(t, "Bybit")
	seedMarket(exg, "BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	keys, err := bybitWsMarkPriceTopics(exg, []string{"BTC/USDT:USDT"})
	if err != nil {
		t.Fatalf("mark price topics error: %v", err)
	}
	if len(keys) != 1 || keys[0] != "tickers.BTCUSDT" {
		t.Fatalf("unexpected mark price keys: %+v", keys)
	}
}

func TestBybitWsMarkPriceTopicsRejectsSpot(t *testing.T) {
	exg := mustNewBybit(t, "Bybit")
	seedMarket(exg, "BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	if _, err := bybitWsMarkPriceTopics(exg, []string{"BTC/USDT"}); err == nil {
		t.Fatal("expected error for spot mark price topics")
	}
}

func TestBybitMarketTypeFromCategory(t *testing.T) {
	if got := bybitMarketTypeFromCategory("spot"); got != banexg.MarketSpot {
		t.Fatalf("expected spot, got %q", got)
	}
	if got := bybitMarketTypeFromCategory("LINEAR"); got != banexg.MarketLinear {
		t.Fatalf("expected linear, got %q", got)
	}
	if got := bybitMarketTypeFromCategory("inverse"); got != banexg.MarketInverse {
		t.Fatalf("expected inverse, got %q", got)
	}
	if got := bybitMarketTypeFromCategory("option"); got != banexg.MarketOption {
		t.Fatalf("expected option, got %q", got)
	}
	if got := bybitMarketTypeFromCategory("unknown"); got != "" {
		t.Fatalf("expected empty for unknown, got %q", got)
	}
}

func TestBybitWsTradeTopics(t *testing.T) {
	exg := mustNewBybit(t, "Bybit")
	seedMarket(exg, "BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	keys, err := bybitWsTradeTopics(exg, []string{"BTC/USDT"})
	if err != nil {
		t.Fatalf("bybitWsTradeTopics failed: %v", err)
	}
	if len(keys) != 1 || keys[0] != "publicTrade.BTCUSDT" {
		t.Fatalf("unexpected trade topics: %#v", keys)
	}
	if _, err := bybitWsTradeTopics(exg, []string{"ETH/USDT"}); err == nil {
		t.Fatal("expected error for missing market")
	}
}
