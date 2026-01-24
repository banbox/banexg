package okx

import (
	"testing"

	"github.com/banbox/banexg"
)

func TestParseTicker(t *testing.T) {
	ok, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	seedMarket(ok, "BTC-USDT", "BTC/USDT", banexg.MarketSpot)
	item := &Ticker{
		InstId:    "BTC-USDT",
		BidPx:     "8888.88",
		BidSz:     "5",
		AskPx:     "9999.99",
		AskSz:     "11",
		Last:      "9999.99",
		Open24h:   "9000",
		High24h:   "10000",
		Low24h:    "8000",
		Vol24h:    "2222",
		VolCcy24h: "3333",
		Ts:        "1597026383085",
	}
	ticker := parseTicker(ok, item, nil, "spot")
	if ticker.Symbol != "BTC/USDT" {
		t.Fatalf("unexpected symbol: %s", ticker.Symbol)
	}
	if ticker.Bid != 8888.88 || ticker.Ask != 9999.99 || ticker.Last != 9999.99 {
		t.Fatalf("unexpected price fields: %+v", ticker)
	}
	if ticker.BaseVolume != 2222 || ticker.QuoteVolume != 3333 {
		t.Fatalf("unexpected volume fields: %+v", ticker)
	}
	if ticker.TimeStamp != 1597026383085 {
		t.Fatalf("unexpected timestamp: %d", ticker.TimeStamp)
	}
}

func TestParseOrderBook(t *testing.T) {
	ob := &OrderBook{
		Asks: [][]string{{"41006.8", "0.60038921", "0", "1"}},
		Bids: [][]string{{"41006.3", "0.30178218", "0", "2"}},
		Ts:   "1629966436396",
	}
	market := &banexg.Market{Symbol: "BTC/USDT"}
	res := parseOrderBook(market, ob, 1)
	if res.Symbol != "BTC/USDT" {
		t.Fatalf("unexpected symbol: %s", res.Symbol)
	}
	if res.Asks.Depth != 1 || res.Bids.Depth != 1 {
		t.Fatalf("unexpected depth: %d/%d", res.Asks.Depth, res.Bids.Depth)
	}
	p, s := res.Asks.Level(0)
	if p != 41006.8 || s != 0.60038921 {
		t.Fatalf("unexpected ask level: %v %v", p, s)
	}
}

func TestParseOHLCV(t *testing.T) {
	rows := [][]string{{
		"1597026383085",
		"3.721",
		"3.743",
		"3.677",
		"3.708",
		"8422410",
		"22698348.04828491",
		"12698348.04828491",
		"0",
	}}
	klines := parseOHLCV(rows)
	if len(klines) != 1 {
		t.Fatalf("unexpected kline len: %d", len(klines))
	}
	k := klines[0]
	if k.Time != 1597026383085 || k.Open != 3.721 || k.Close != 3.708 {
		t.Fatalf("unexpected kline: %+v", k)
	}
	if k.Volume != 8422410 || k.Info != 12698348.04828491 {
		t.Fatalf("unexpected volume/info: %+v", k)
	}
}

func TestTickersToLastPrices(t *testing.T) {
	tickers := []*banexg.Ticker{
		{Symbol: "BTC/USDT", Last: 100.5, TimeStamp: 1700000000000, Info: map[string]interface{}{"src": "okx"}},
		{Symbol: "ETH/USDT", Last: 200.1, TimeStamp: 1700000001000},
		nil,
	}
	lasts := banexg.TickersToLastPrices(tickers, nil)
	if len(lasts) != 2 {
		t.Fatalf("unexpected last prices len: %d", len(lasts))
	}
	if lasts[0].Symbol != "BTC/USDT" || lasts[0].Price != 100.5 || lasts[0].Timestamp != 1700000000000 {
		t.Fatalf("unexpected last price: %+v", lasts[0])
	}
	if lasts[0].Info == nil || lasts[0].Info["src"] != "okx" {
		t.Fatalf("unexpected info: %+v", lasts[0].Info)
	}
}

func TestTickersToPriceMap(t *testing.T) {
	tickers := []*banexg.Ticker{
		{Symbol: "BTC/USDT", Last: 100.5},
		{Symbol: "ETH/USDT", Last: 200.1},
		nil,
	}
	prices := banexg.TickersToPriceMap(tickers, nil)
	if len(prices) != 2 {
		t.Fatalf("unexpected price map len: %d", len(prices))
	}
	if prices["BTC/USDT"] != 100.5 || prices["ETH/USDT"] != 200.1 {
		t.Fatalf("unexpected price map: %+v", prices)
	}
}

func TestMapTickerInstType(t *testing.T) {
	tests := []struct {
		name         string
		marketType   string
		contractType string
		expected     string
	}{
		{"spot", banexg.MarketSpot, "", InstTypeSpot},
		{"margin", banexg.MarketMargin, "", InstTypeSpot},
		{"linear", banexg.MarketLinear, banexg.MarketLinear, InstTypeSwap},
		{"inverse", banexg.MarketInverse, banexg.MarketInverse, InstTypeSwap},
		{"option", banexg.MarketOption, "", InstTypeOption},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapTickerInstType(tt.marketType, tt.contractType)
			if result != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// ============================================================================
// API Integration Tests - require local.json with valid credentials
// Run manually with: go test -run TestAPI_FetchTicker -v
// These tests are prefixed with TestAPI_ to distinguish them from unit tests.
// ============================================================================

func assertKlinesAsc(t *testing.T, klines []*banexg.Kline) {
	t.Helper()
	for i := 1; i < len(klines); i++ {
		if klines[i] == nil || klines[i-1] == nil {
			t.Fatalf("nil kline at %d/%d", i-1, i)
		}
		if klines[i].Time <= klines[i-1].Time {
			t.Fatalf("klines not ascending at %d: prev=%d cur=%d", i, klines[i-1].Time, klines[i].Time)
		}
	}
}

func mustFetchOHLCV(t *testing.T, exg *OKX, symbol, timeframe string, since int64, limit int, params map[string]interface{}) []*banexg.Kline {
	t.Helper()
	klines, err := exg.FetchOHLCV(symbol, timeframe, since, limit, params)
	if err != nil {
		t.Fatalf("FetchOHLCV(symbol=%s, tf=%s, since=%d, limit=%d, params=%v): %v", symbol, timeframe, since, limit, params, err)
	}
	if len(klines) == 0 {
		t.Fatalf("FetchOHLCV returned empty result (symbol=%s, tf=%s, since=%d, limit=%d, params=%v)", symbol, timeframe, since, limit, params)
	}
	assertKlinesAsc(t, klines)
	return klines
}

func TestAPI_FetchTicker(t *testing.T) {
	exg := getExchange(nil)
	symbol := "BTC/USDT"
	ticker, err := exg.FetchTicker(symbol, nil)
	if err != nil {
		panic(err)
	}
	t.Logf("ticker: symbol=%s, bid=%v, ask=%v, last=%v, volume=%v",
		ticker.Symbol, ticker.Bid, ticker.Ask, ticker.Last, ticker.BaseVolume)
}

func TestAPI_FetchTickers(t *testing.T) {
	exg := getExchange(nil)
	symbols := []string{"BTC/USDT", "ETH/USDT"}
	tickers, err := exg.FetchTickers(symbols, nil)
	if err != nil {
		panic(err)
	}
	t.Logf("fetched %d tickers", len(tickers))
	for _, ticker := range tickers {
		t.Logf("ticker: %s, last=%v", ticker.Symbol, ticker.Last)
	}
}

func TestApi_FetchOHLCV_Basic(t *testing.T) {
	exg := getExchange(nil)
	symbol := "BTC/USDT"
	since := int64(1735689600000)
	klines := mustFetchOHLCV(t, exg, symbol, "1h", since, 10, nil)
	if len(klines) > 10 {
		t.Fatalf("expected <= 10 klines, got %d", len(klines))
	}
	firstTime, lastTime := int64(0), int64(0)
	if len(klines) > 0 {
		firstTime = klines[0].Time
		lastTime = klines[len(klines)-1].Time
	}
	if firstTime != since {
		t.Fatalf("expected first kline time == since (inclusive). since=%d first=%d", since, firstTime)
	}
	t.Logf("fetched %d klines for %s", len(klines), symbol)
	t.Logf("firstTime=%d, lastTime=%d", firstTime, lastTime)
}

func TestApi_FetchOHLCV_SinceOnly(t *testing.T) {
	exg := getExchange(nil)
	symbol := "BTC/USDT"
	// Use server-returned ts to avoid timezone/alignment issues.
	ref := mustFetchOHLCV(t, exg, symbol, "4h", 0, 20, nil)
	since := ref[len(ref)-1].Time
	klines := mustFetchOHLCV(t, exg, symbol, "4h", since, 100, nil)
	if klines[0].Time != since {
		t.Fatalf("expected first kline time == since (inclusive). since=%d first=%d", since, klines[0].Time)
	}
	for _, k := range klines {
		if k.Time < since {
			t.Fatalf("unexpected kline time < since: since=%d k=%d", since, k.Time)
		}
	}
}

func TestAPI_FetchOrderBook(t *testing.T) {
	exg := getExchange(nil)
	symbol := "BTC/USDT"
	ob, err := exg.FetchOrderBook(symbol, 5, nil)
	if err != nil {
		panic(err)
	}
	t.Logf("orderbook: symbol=%s, asks=%d, bids=%d", ob.Symbol, ob.Asks.Depth, ob.Bids.Depth)
	if ob.Asks.Depth > 0 {
		p, s := ob.Asks.Level(0)
		t.Logf("best ask: price=%v, size=%v", p, s)
	}
	if ob.Bids.Depth > 0 {
		p, s := ob.Bids.Level(0)
		t.Logf("best bid: price=%v, size=%v", p, s)
	}
}

func TestApi_FetchOHLCV_UntilOnly(t *testing.T) {
	exg := getExchange(nil)
	symbol := "BTC/USDT"
	ref := mustFetchOHLCV(t, exg, symbol, "4h", 0, 20, nil)
	until := ref[len(ref)-1].Time
	klines := mustFetchOHLCV(t, exg, symbol, "4h", 0, 50, map[string]interface{}{
		banexg.ParamUntil: until,
	})
	// OKX docs: after=请求此时间戳之前的数据(ts < after)
	if klines[len(klines)-1].Time >= until {
		t.Fatalf("expected last kline time < until (exclusive). until=%d last=%d", until, klines[len(klines)-1].Time)
	}
}

func TestApi_FetchOHLCV_SinceUntil(t *testing.T) {
	exg := getExchange(nil)
	symbol := "BTC/USDT"
	ref := mustFetchOHLCV(t, exg, symbol, "1h", 0, 80, nil)
	if len(ref) < 60 {
		t.Fatalf("need >= 60 reference klines, got %d", len(ref))
	}
	since := ref[10].Time
	until := ref[50].Time
	if until <= since {
		t.Fatalf("invalid ref range: since=%d until=%d", since, until)
	}
	klines := mustFetchOHLCV(t, exg, symbol, "1h", since, 200, map[string]interface{}{
		banexg.ParamUntil: until,
	})
	if klines[0].Time != since {
		t.Fatalf("expected first kline time == since (inclusive). since=%d first=%d", since, klines[0].Time)
	}
	for _, k := range klines {
		if k.Time < since || k.Time >= until {
			t.Fatalf("kline time out of range: since=%d until=%d got=%d", since, until, k.Time)
		}
	}
	if klines[len(klines)-1].Time >= until {
		t.Fatalf("expected last kline time < until (exclusive). until=%d last=%d", until, klines[len(klines)-1].Time)
	}
}

func TestApi_FetchOHLCV_DefaultLimit(t *testing.T) {
	exg := getExchange(nil)
	symbol := "BTC/USDT"
	// limit<=0 should default to 100 internally.
	klines := mustFetchOHLCV(t, exg, symbol, "1m", 0, 0, nil)
	if len(klines) != 100 {
		t.Fatalf("expected default limit to return 100 klines, got %d", len(klines))
	}
}

func TestAPI_FetchOrderBookFull(t *testing.T) {
	exg := getExchange(nil)
	symbol := "BTC/USDT"
	limit := 500 // Should trigger books-full
	ob, err := exg.FetchOrderBook(symbol, limit, nil)
	if err != nil {
		panic(err)
	}
	t.Logf("fetched full orderbook: symbol=%s, asks=%d, bids=%d, limit=%d", ob.Symbol, ob.Asks.Depth, ob.Bids.Depth, ob.Limit)
	if ob.Asks.Depth > 400 || ob.Bids.Depth > 400 {
		t.Logf("successfully fetched deep orderbook (>400 levels)")
	}
}
