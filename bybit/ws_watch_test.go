package bybit

import (
	"encoding/gob"
	"strings"
	"testing"

	"github.com/banbox/banexg"
)

func newBybitWsWatch(t *testing.T, marketID, symbol, marketType string) (*Bybit, *banexg.WsClient) {
	t.Helper()
	conn := newTestAsyncConn()
	exg, err := New(map[string]interface{}{banexg.OptWsConn: conn})
	if err != nil {
		t.Fatalf("new bybit failed: %v", err)
	}
	seedMarketIfNeeded(exg, marketID, symbol, marketType)
	exg.WsDecoder = gob.NewDecoder(strings.NewReader(""))
	exg.MarketType = marketType
	client, err := exg.getWsPublicClient(marketType, "")
	if err != nil {
		t.Fatalf("get ws client failed: %v", err)
	}
	return exg, client
}

func assertSubKey(t *testing.T, client *banexg.WsClient, key string, want bool) {
	t.Helper()
	_, ok := client.SubscribeKeys[key]
	if ok != want {
		t.Fatalf("subscribe key %s want=%v got=%v", key, want, ok)
	}
}

func TestWatchOrderBooksSubscribeUnsubscribe(t *testing.T) {
	exg, client := newBybitWsWatch(t, "BTCUSDT", "BTC/USDT", banexg.MarketSpot)

	out, err := exg.WatchOrderBooks([]string{"BTC/USDT"}, 0, nil)
	if err != nil || out == nil {
		t.Fatalf("WatchOrderBooks failed: %v", err)
	}
	key := "orderbook.50.BTCUSDT"
	assertSubKey(t, client, key, true)

	limits, lock := client.LockOdBookLimits()
	depth := limits["BTC/USDT"]
	lock.Unlock()
	if depth != 50 {
		t.Fatalf("unexpected orderbook depth: %d", depth)
	}

	if err := exg.UnWatchOrderBooks([]string{"BTC/USDT"}, nil); err != nil {
		t.Fatalf("UnWatchOrderBooks failed: %v", err)
	}
	assertSubKey(t, client, key, false)
	limits, lock = client.LockOdBookLimits()
	_, ok := limits["BTC/USDT"]
	lock.Unlock()
	if ok {
		t.Fatal("expected orderbook depth to be cleared")
	}
}

func TestWatchTradesSubscribeUnsubscribe(t *testing.T) {
	exg, client := newBybitWsWatch(t, "BTCUSDT", "BTC/USDT", banexg.MarketSpot)

	out, err := exg.WatchTrades([]string{"BTC/USDT"}, nil)
	if err != nil || out == nil {
		t.Fatalf("WatchTrades failed: %v", err)
	}
	key := "publicTrade.BTCUSDT"
	assertSubKey(t, client, key, true)

	if err := exg.UnWatchTrades([]string{"BTC/USDT"}, nil); err != nil {
		t.Fatalf("UnWatchTrades failed: %v", err)
	}
	assertSubKey(t, client, key, false)
}

func TestWatchOHLCVsSubscribeUnsubscribe(t *testing.T) {
	exg, client := newBybitWsWatch(t, "BTCUSDT", "BTC/USDT", banexg.MarketSpot)

	jobs := [][2]string{{"BTC/USDT", "1m"}}
	out, err := exg.WatchOHLCVs(jobs, nil)
	if err != nil || out == nil {
		t.Fatalf("WatchOHLCVs failed: %v", err)
	}
	key := "kline.1.BTCUSDT"
	assertSubKey(t, client, key, true)

	if err := exg.UnWatchOHLCVs(jobs, nil); err != nil {
		t.Fatalf("UnWatchOHLCVs failed: %v", err)
	}
	assertSubKey(t, client, key, false)
}

func TestWatchMarkPricesSubscribeUnsubscribe(t *testing.T) {
	exg, client := newBybitWsWatch(t, "BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)

	out, err := exg.WatchMarkPrices([]string{"BTC/USDT:USDT"}, nil)
	if err != nil || out == nil {
		t.Fatalf("WatchMarkPrices failed: %v", err)
	}
	key := "tickers.BTCUSDT"
	assertSubKey(t, client, key, true)

	if err := exg.UnWatchMarkPrices([]string{"BTC/USDT:USDT"}, nil); err != nil {
		t.Fatalf("UnWatchMarkPrices failed: %v", err)
	}
	assertSubKey(t, client, key, false)
}
