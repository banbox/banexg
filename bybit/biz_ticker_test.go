package bybit

import (
	"context"
	"testing"

	"github.com/banbox/banexg"
)

func newBybitWithMarket(id, symbol, marketType string) *Bybit {
	exg, err := New(nil)
	if err != nil {
		panic(err)
	}
	seedMarket(exg, id, symbol, marketType)
	return exg
}

func seedMarketWithBase(exg *Bybit, marketID, symbol, marketType, base string) {
	seedMarket(exg, marketID, symbol, marketType)
	if exg == nil {
		return
	}
	market, ok := exg.Markets[symbol]
	if !ok || market == nil {
		return
	}
	market.Base = base
}

func setBybitV5MarketTickersResponse(t *testing.T, expectedCategory, expectedSymbol, body string) {
	t.Helper()
	setBybitTestRequest(t, func(_ context.Context, endpoint string, params map[string]interface{}, _ int, _ bool, _ bool) *banexg.HttpRes {
		if endpoint != MethodPublicGetV5MarketTickers {
			t.Fatalf("unexpected endpoint: %s", endpoint)
		}
		if params["category"] != expectedCategory {
			t.Fatalf("unexpected category: %v", params["category"])
		}
		if expectedSymbol != "" && params["symbol"] != expectedSymbol {
			t.Fatalf("unexpected symbol: %v", params["symbol"])
		}
		return &banexg.HttpRes{Status: 200, Content: body}
	})
}

func TestBaseTickerToStdTicker(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	ticker := (&BaseTicker{
		Symbol:       "BTCUSDT",
		Bid1Price:    "100",
		Bid1Size:     "1",
		Ask1Price:    "101",
		Ask1Size:     "2",
		LastPrice:    "102",
		HighPrice24h: "120",
		LowPrice24h:  "80",
		Turnover24h:  "1000",
		Volume24h:    "10",
	}).ToStdTicker(exg, banexg.MarketSpot, map[string]interface{}{})

	if ticker.Symbol != "BTC/USDT" {
		t.Fatalf("unexpected symbol: %s", ticker.Symbol)
	}
	if ticker.Bid != 100 || ticker.Ask != 101 || ticker.Last != 102 {
		t.Fatalf("unexpected prices: bid=%v ask=%v last=%v", ticker.Bid, ticker.Ask, ticker.Last)
	}
	if ticker.BaseVolume != 10 || ticker.QuoteVolume != 1000 {
		t.Fatalf("unexpected volumes: base=%v quote=%v", ticker.BaseVolume, ticker.QuoteVolume)
	}
}

func TestSpotTickerToStdTicker(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	info := map[string]interface{}{"raw": "spot"}
	ticker := (&SpotTicker{
		BaseTicker: BaseTicker{
			Symbol:       "BTCUSDT",
			Bid1Price:    "100",
			Bid1Size:     "1",
			Ask1Price:    "101",
			Ask1Size:     "2",
			LastPrice:    "102",
			HighPrice24h: "120",
			LowPrice24h:  "80",
			Turnover24h:  "1000",
			Volume24h:    "10",
		},
		PrevPrice24h: "90",
		Price24hPcnt: "0.1",
	}).ToStdTicker(exg, banexg.MarketSpot, info)

	if ticker.Open != 90 {
		t.Fatalf("unexpected open: %v", ticker.Open)
	}
	if ticker.Percentage != 10 {
		t.Fatalf("unexpected percentage: %v", ticker.Percentage)
	}
	if ticker.Info == nil {
		t.Fatal("expected info to be set")
	}
}

func TestFutureTickerToStdTicker(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	info := map[string]interface{}{"raw": "future"}
	ticker := (&FutureTicker{
		ContractTicker: ContractTicker{
			BaseTicker: BaseTicker{
				Symbol:       "BTCUSDT",
				Bid1Price:    "100",
				Bid1Size:     "1",
				Ask1Price:    "101",
				Ask1Size:     "2",
				LastPrice:    "102",
				HighPrice24h: "120",
				LowPrice24h:  "80",
				Turnover24h:  "1000",
				Volume24h:    "10",
			},
			IndexPrice: "99",
			MarkPrice:  "100.5",
		},
		PrevPrice24h: "95",
		Price24hPcnt: "0.2",
	}).ToStdTicker(exg, banexg.MarketLinear, info)

	if ticker.Symbol != "BTC/USDT:USDT" {
		t.Fatalf("unexpected symbol: %s", ticker.Symbol)
	}
	if ticker.IndexPrice != 99 || ticker.MarkPrice != 100.5 {
		t.Fatalf("unexpected index/mark: %v/%v", ticker.IndexPrice, ticker.MarkPrice)
	}
	if ticker.Open != 95 || ticker.Percentage != 20 {
		t.Fatalf("unexpected open/percentage: %v/%v", ticker.Open, ticker.Percentage)
	}
}

func TestOptionTickerToStdTicker(t *testing.T) {
	exg := newBybitWithMarket("BTC-30DEC22-18000-C", "BTC/USDT:USDT-30DEC22-18000-C", banexg.MarketOption)
	info := map[string]interface{}{"raw": "option"}
	ticker := (&OptionTicker{
		ContractTicker: ContractTicker{
			BaseTicker: BaseTicker{
				Symbol:       "BTC-30DEC22-18000-C",
				Bid1Price:    "1",
				Bid1Size:     "1",
				Ask1Price:    "2",
				Ask1Size:     "2",
				LastPrice:    "1.5",
				HighPrice24h: "3",
				LowPrice24h:  "1",
				Turnover24h:  "10",
				Volume24h:    "5",
			},
			IndexPrice: "25000",
			MarkPrice:  "25500",
		},
	}).ToStdTicker(exg, banexg.MarketOption, info)

	if ticker.Symbol == "" {
		t.Fatal("expected symbol to be set")
	}
	if ticker.Info == nil {
		t.Fatal("expected info to be set")
	}
}

func TestFetchTickersOptionRequireArg(t *testing.T) {
	exg := &Bybit{}
	_, err := exg.fetchTickers("FetchTickers", banexg.MarketOption, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error when option ticker has no symbol/baseCoin")
	}
}

func TestFetchTickersParams(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	body := `{"retCode":0,"retMsg":"OK","result":{"category":"linear","list":[{"symbol":"BTCUSDT","bid1Price":"100","bid1Size":"1","ask1Price":"101","ask1Size":"2","lastPrice":"102","highPrice24h":"120","lowPrice24h":"80","turnover24h":"1000","volume24h":"10","indexPrice":"99","markPrice":"100.5","prevPrice24h":"95","price24hPcnt":"0.1"}]},"retExtInfo":{},"time":1700000000000}`
	setBybitV5MarketTickersResponse(t, banexg.MarketLinear, "BTCUSDT", body)

	tickers, err := exg.FetchTickers([]string{"BTC/USDT:USDT"}, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
	if err != nil {
		t.Fatalf("FetchTickers failed: %v", err)
	}
	if len(tickers) != 1 {
		t.Fatalf("expected 1 ticker, got %d", len(tickers))
	}
	if tickers[0].Symbol != "BTC/USDT:USDT" {
		t.Fatalf("unexpected ticker symbol: %s", tickers[0].Symbol)
	}
}

func TestFetchTickerParams(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	body := `{"retCode":0,"retMsg":"OK","result":{"category":"spot","list":[{"symbol":"BTCUSDT","bid1Price":"100","bid1Size":"1","ask1Price":"101","ask1Size":"2","lastPrice":"102","highPrice24h":"120","lowPrice24h":"80","turnover24h":"1000","volume24h":"10","prevPrice24h":"90","price24hPcnt":"0.1"}]},"retExtInfo":{},"time":1700000000000}`
	setBybitV5MarketTickersResponse(t, banexg.MarketSpot, "BTCUSDT", body)

	ticker, err := exg.FetchTicker("BTC/USDT", nil)
	if err != nil {
		t.Fatalf("FetchTicker failed: %v", err)
	}
	if ticker.Symbol != "BTC/USDT" {
		t.Fatalf("unexpected ticker symbol: %s", ticker.Symbol)
	}
	if ticker.Last != 102 {
		t.Fatalf("unexpected last price: %v", ticker.Last)
	}
}

func TestFetchLastPricesFiltersSymbols(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	seedMarket(exg, "ETHUSDT", "ETH/USDT", banexg.MarketSpot)
	body := `{"retCode":0,"retMsg":"OK","result":{"category":"spot","list":[{"symbol":"BTCUSDT","lastPrice":"102"},{"symbol":"ETHUSDT","lastPrice":"203"}]},"retExtInfo":{},"time":1700000000000}`
	setBybitV5MarketTickersResponse(t, banexg.MarketSpot, "BTCUSDT", body)

	items, err := exg.FetchLastPrices([]string{"BTC/USDT"}, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketSpot,
	})
	if err != nil {
		t.Fatalf("FetchLastPrices failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 last price, got %d", len(items))
	}
	if items[0].Symbol != "BTC/USDT" {
		t.Fatalf("unexpected last price symbol: %s", items[0].Symbol)
	}
	if items[0].Price != 102 {
		t.Fatalf("unexpected last price: %v", items[0].Price)
	}
}

func TestFetchTickersOptionGroupByBaseCoin(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new bybit exchange failed: %v", err)
	}
	btcSymbol := "BTC/USDT:USDT-30DEC22-18000-C"
	ethSymbol := "ETH/USDT:USDT-30DEC22-1000-C"
	seedMarketWithBase(exg, "BTC-30DEC22-18000-C", btcSymbol, banexg.MarketOption, "BTC")
	seedMarketWithBase(exg, "ETH-30DEC22-1000-C", ethSymbol, banexg.MarketOption, "ETH")

	callCounts := map[string]int{}
	setBybitTestRequest(t, func(_ context.Context, endpoint string, params map[string]interface{}, _ int, _ bool, _ bool) *banexg.HttpRes {
		if endpoint != MethodPublicGetV5MarketTickers {
			t.Fatalf("unexpected endpoint: %s", endpoint)
		}
		if params["category"] != banexg.MarketOption {
			t.Fatalf("unexpected category: %v", params["category"])
		}
		base, ok := params["baseCoin"].(string)
		if !ok || base == "" {
			t.Fatalf("expected baseCoin for option tickers, got %v", params["baseCoin"])
		}
		callCounts[base]++
		var body string
		switch base {
		case "BTC":
			body = `{"retCode":0,"retMsg":"OK","result":{"category":"option","list":[{"symbol":"BTC-30DEC22-18000-C","lastPrice":"1.5","bid1Price":"1","bid1Size":"1","ask1Price":"2","ask1Size":"1","highPrice24h":"3","lowPrice24h":"1","turnover24h":"10","volume24h":"5","indexPrice":"25000","markPrice":"25500"}]},"retExtInfo":{},"time":1700000000000}`
		case "ETH":
			body = `{"retCode":0,"retMsg":"OK","result":{"category":"option","list":[{"symbol":"ETH-30DEC22-1000-C","lastPrice":"2","bid1Price":"1.8","bid1Size":"1","ask1Price":"2.2","ask1Size":"1","highPrice24h":"3","lowPrice24h":"1","turnover24h":"8","volume24h":"4","indexPrice":"1500","markPrice":"1550"}]},"retExtInfo":{},"time":1700000000000}`
		default:
			t.Fatalf("unexpected baseCoin: %s", base)
		}
		return &banexg.HttpRes{Status: 200, Content: body}
	})

	tickers, fetchErr := exg.FetchTickers([]string{btcSymbol, ethSymbol}, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketOption,
	})
	if fetchErr != nil {
		t.Fatalf("FetchTickers failed: %v", fetchErr)
	}
	if len(tickers) != 2 {
		t.Fatalf("expected 2 tickers, got %d", len(tickers))
	}
	gotSymbols := map[string]struct{}{}
	for _, ticker := range tickers {
		gotSymbols[ticker.Symbol] = struct{}{}
	}
	if _, ok := gotSymbols[btcSymbol]; !ok {
		t.Fatalf("missing btc option ticker: %v", gotSymbols)
	}
	if _, ok := gotSymbols[ethSymbol]; !ok {
		t.Fatalf("missing eth option ticker: %v", gotSymbols)
	}
	if callCounts["BTC"] != 1 || callCounts["ETH"] != 1 {
		t.Fatalf("expected 1 request per baseCoin, got %+v", callCounts)
	}
}
