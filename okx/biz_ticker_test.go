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
	lasts := tickersToLastPrices(tickers)
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
	prices := tickersToPriceMap(tickers)
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

func TestAPI_FetchOHLCV(t *testing.T) {
	exg := getExchange(nil)
	symbol := "BTC/USDT"
	since := int64(1704067200000) // 2024-01-01 00:00:00 UTC
	klines, err := exg.FetchOHLCV(symbol, "1d", since, 10, nil)
	if err != nil {
		panic(err)
	}
	t.Logf("fetched %d klines for %s", len(klines), symbol)
	for _, k := range klines {
		t.Logf("kline: time=%d, O=%v H=%v L=%v C=%v V=%v", k.Time, k.Open, k.High, k.Low, k.Close, k.Volume)
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

func TestAPI_FetchOHLCVHistory(t *testing.T) {
	exg := getExchange(nil)
	symbol := "BTC/USDT"
	since := int64(1672531200000) // 2023-01-01
	until := int64(1675209600000) // 2023-02-01
	klines, err := exg.FetchOHLCV(symbol, "1d", since, 10, map[string]interface{}{
		banexg.ParamUntil: until,
	})
	if err != nil {
		panic(err)
	}
	t.Logf("fetched %d history klines for %s", len(klines), symbol)
	if len(klines) > 0 {
		t.Logf("first kline: time=%d, O=%v H=%v L=%v C=%v", klines[0].Time, klines[0].Open, klines[0].High, klines[0].Low, klines[0].Close)
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
