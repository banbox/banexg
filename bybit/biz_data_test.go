package bybit

import (
	"context"
	"testing"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/utils"
)

func mustMarshal(t *testing.T, payload interface{}) string {
	t.Helper()
	text, err := utils.MarshalString(payload)
	if err != nil {
		t.Fatalf("marshal response failed: %v", err)
	}
	return text
}

func requireBybitReq(t *testing.T, endpoint string, params map[string]interface{}, wantEndpoint, wantCategory, wantSymbol string) {
	t.Helper()
	if endpoint != wantEndpoint {
		t.Fatalf("unexpected endpoint: %s", endpoint)
	}
	if wantCategory != "" && params["category"] != wantCategory {
		t.Fatalf("unexpected category: %v", params["category"])
	}
	if wantSymbol != "" && params["symbol"] != wantSymbol {
		t.Fatalf("unexpected symbol: %v", params["symbol"])
	}
}

func TestParseBybitOHLCV(t *testing.T) {
	rows := [][]string{
		{"3", "3", "4", "2", "3.5", "30", "300"},
		{"2", "2", "3", "1", "2.5", "20", "200"},
		{"1", "1", "2", "0.5", "1.5", "10", "100"},
	}
	out := parseBybitOHLCV(rows)
	if len(out) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(out))
	}
	if out[0].Time != 1 || out[1].Time != 2 || out[2].Time != 3 {
		t.Fatalf("expected ascending times, got %d,%d,%d", out[0].Time, out[1].Time, out[2].Time)
	}
}

func TestBybitFundingIntervalMS(t *testing.T) {
	if got := bybitFundingIntervalMS(nil); got != 0 {
		t.Fatalf("expected 0 for nil market, got %d", got)
	}
	market := &banexg.Market{Info: map[string]interface{}{"fundingInterval": "480"}}
	expect := int64(480 * 60 * 1000)
	if got := bybitFundingIntervalMS(market); got != expect {
		t.Fatalf("expected %d, got %d", expect, got)
	}
	market.Info["fundingInterval"] = 0
	if got := bybitFundingIntervalMS(market); got != 0 {
		t.Fatalf("expected 0 for zero fundingInterval, got %d", got)
	}
}

func TestFetchOHLCV_OptionNotSupported(t *testing.T) {
	market := &banexg.Market{Symbol: "BTC/USDT:USDT-30DEC22-18000-C", ID: "BTC-30DEC22-18000-C", Option: true}
	exg := &Bybit{Exchange: &banexg.Exchange{ExgInfo: &banexg.ExgInfo{
		Markets: banexg.MarketMap{market.Symbol: market},
	}}}
	_, err := exg.FetchOHLCV(market.Symbol, "1m", 0, 10, nil)
	if err == nil {
		t.Fatal("expected error for option kline")
	}
}

func TestFetchFundingRate_Errors(t *testing.T) {
	spot := &banexg.Market{Symbol: "BTC/USDT", ID: "BTCUSDT", Spot: true, Type: banexg.MarketSpot}
	exg := &Bybit{Exchange: &banexg.Exchange{ExgInfo: &banexg.ExgInfo{
		Markets: banexg.MarketMap{spot.Symbol: spot},
	}}}
	if _, err := exg.FetchFundingRate(spot.Symbol, nil); err == nil {
		t.Fatal("expected error for spot funding rate")
	}
}

func TestFetchFundingRates_Errors(t *testing.T) {
	spot := &banexg.Market{Symbol: "BTC/USDT", ID: "BTCUSDT", Spot: true, Type: banexg.MarketSpot}
	swap := &banexg.Market{Symbol: "BTC/USDT:USDT", ID: "BTCUSDT", Swap: true, Linear: true, Type: banexg.MarketLinear}
	future := &banexg.Market{Symbol: "BTC/USDT:USDT-230630", ID: "BTCUSDT", Swap: false, Linear: true, Type: banexg.MarketLinear}
	exg := &Bybit{Exchange: &banexg.Exchange{ExgInfo: &banexg.ExgInfo{
		Markets: banexg.MarketMap{
			spot.Symbol:   spot,
			swap.Symbol:   swap,
			future.Symbol: future,
		},
	}}}
	if _, err := exg.FetchFundingRates([]string{spot.Symbol, swap.Symbol}, nil); err == nil {
		t.Fatal("expected error for non-swap symbol")
	}
	if _, err := exg.FetchFundingRates([]string{future.Symbol, swap.Symbol}, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	}); err == nil {
		t.Fatal("expected error for non-swap contract funding rate")
	}
}

func TestFetchFundingRateHistory_Errors(t *testing.T) {
	exg := &Bybit{Exchange: &banexg.Exchange{ExgInfo: &banexg.ExgInfo{
		Markets: banexg.MarketMap{},
	}}}
	if _, err := exg.FetchFundingRateHistory("", 0, 1, nil); err == nil {
		t.Fatal("expected error when symbol is empty")
	}
	spot := &banexg.Market{Symbol: "BTC/USDT", ID: "BTCUSDT", Spot: true, Type: banexg.MarketSpot}
	exg.Markets[spot.Symbol] = spot
	if _, err := exg.FetchFundingRateHistory(spot.Symbol, 0, 1, nil); err == nil {
		t.Fatal("expected error for spot funding rate history")
	}
	swap := &banexg.Market{Symbol: "BTC/USDT:USDT", ID: "BTCUSDT", Swap: true, Linear: true, Contract: true, Type: banexg.MarketLinear}
	exg.Markets[swap.Symbol] = swap
	if _, err := exg.FetchFundingRateHistory(swap.Symbol, 1700000000000, 1, nil); err == nil {
		t.Fatal("expected error when fundingInterval is missing for startTime")
	}
}

func TestParseFundingRatesFiltersSwap(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	exg.Markets["BTC/USDT:USDT"].Swap = true
	seedMarket(exg, "ETHUSDT", "ETH/USDT:USDT", banexg.MarketLinear)
	exg.Markets["ETH/USDT:USDT"].Swap = false

	body := mustMarshal(t, map[string]interface{}{
		"retCode": 0,
		"retMsg":  "OK",
		"result": map[string]interface{}{
			"category": "linear",
			"list": []map[string]interface{}{
				{
					"symbol":          "BTCUSDT",
					"fundingRate":     "0.0001",
					"nextFundingTime": "1700000000000",
					"markPrice":       "30000",
					"indexPrice":      "29900",
				},
				{
					"symbol":          "ETHUSDT",
					"fundingRate":     "0.0002",
					"nextFundingTime": "1700000000000",
					"markPrice":       "2000",
					"indexPrice":      "1990",
				},
			},
		},
		"retExtInfo": map[string]interface{}{},
		"time":       1700000000000,
	})

	setBybitTestRequest(t, func(_ context.Context, endpoint string, params map[string]interface{}, _ int, _ bool, _ bool) *banexg.HttpRes {
		requireBybitReq(t, endpoint, params, MethodPublicGetV5MarketTickers, banexg.MarketLinear, "")
		return &banexg.HttpRes{Status: 200, Content: body}
	})

	args := map[string]interface{}{"category": banexg.MarketLinear}
	symbolSet := banexg.BuildSymbolSet([]string{"BTC/USDT:USDT", "ETH/USDT:USDT"})
	items, err := parseFundingRates(exg, banexg.MarketLinear, args, 1, symbolSet)
	if err != nil {
		t.Fatalf("parseFundingRates failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 funding rate, got %d", len(items))
	}
	if items[0].Symbol != "BTC/USDT:USDT" {
		t.Fatalf("unexpected funding rate symbol: %s", items[0].Symbol)
	}
	if items[0].FundingRate != 0.0001 {
		t.Fatalf("unexpected funding rate: %v", items[0].FundingRate)
	}
	if items[0].MarkPrice != 30000 || items[0].IndexPrice != 29900 {
		t.Fatalf("unexpected mark/index price: %v/%v", items[0].MarkPrice, items[0].IndexPrice)
	}
}

func TestFetchFundingRateHistoryParams(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	exg.Markets["BTC/USDT:USDT"].Swap = true
	exg.Markets["BTC/USDT:USDT"].Info = map[string]interface{}{"fundingInterval": 480}

	t.Run("autoEndTime", func(t *testing.T) {
		const since = int64(1700000000000)
		const limit = 2
		expectEnd := since + int64(limit)*int64(8*60*60*1000)

		setBybitTestRequest(t, func(_ context.Context, endpoint string, params map[string]interface{}, _ int, _ bool, _ bool) *banexg.HttpRes {
			requireBybitReq(t, endpoint, params, MethodPublicGetV5MarketFundingHistory, banexg.MarketLinear, "BTCUSDT")
			if params["limit"] != limit {
				t.Fatalf("unexpected limit: %v", params["limit"])
			}
			if params["startTime"] != since {
				t.Fatalf("unexpected startTime: %v", params["startTime"])
			}
			if params["endTime"] != expectEnd {
				t.Fatalf("unexpected endTime: %v", params["endTime"])
			}
			body := `{"retCode":0,"retMsg":"OK","result":{"category":"linear","list":[{"symbol":"BTCUSDT","fundingRate":"0.0001","fundingRateTimestamp":"1700000000000"}]},"retExtInfo":{},"time":1700000000000}`
			return &banexg.HttpRes{Status: 200, Content: body}
		})

		items, err := exg.FetchFundingRateHistory("BTC/USDT:USDT", since, limit, nil)
		if err != nil {
			t.Fatalf("FetchFundingRateHistory failed: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("expected 1 funding rate, got %d", len(items))
		}
		if items[0].Symbol != "BTC/USDT:USDT" {
			t.Fatalf("unexpected symbol: %s", items[0].Symbol)
		}
	})

	t.Run("explicitEndTime", func(t *testing.T) {
		const until = int64(1700000000000)
		setBybitTestRequest(t, func(_ context.Context, endpoint string, params map[string]interface{}, _ int, _ bool, _ bool) *banexg.HttpRes {
			requireBybitReq(t, endpoint, params, MethodPublicGetV5MarketFundingHistory, banexg.MarketLinear, "BTCUSDT")
			if params["limit"] != maxFundRateBatch {
				t.Fatalf("unexpected limit: %v", params["limit"])
			}
			if _, ok := params["startTime"]; ok {
				t.Fatalf("did not expect startTime, got %v", params["startTime"])
			}
			if params["endTime"] != until {
				t.Fatalf("unexpected endTime: %v", params["endTime"])
			}
			body := `{"retCode":0,"retMsg":"OK","result":{"category":"linear","list":[{"symbol":"BTCUSDT","fundingRate":"0.0002","fundingRateTimestamp":"1699990000000"}]},"retExtInfo":{},"time":1700000000000}`
			return &banexg.HttpRes{Status: 200, Content: body}
		})

		items, err := exg.FetchFundingRateHistory("BTC/USDT:USDT", 0, 0, map[string]interface{}{
			banexg.ParamUntil: until,
		})
		if err != nil {
			t.Fatalf("FetchFundingRateHistory failed: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("expected 1 funding rate, got %d", len(items))
		}
	})
}

func TestFetchOHLCVPriceEndpoints(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	cases := []struct {
		name   string
		price  string
		method string
	}{
		{name: "index", price: "index", method: MethodPublicGetV5MarketIndexPriceKline},
		{name: "premium", price: "premiumIndex", method: MethodPublicGetV5MarketPremiumIndexPriceKline},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			setBybitTestRequest(t, func(_ context.Context, endpoint string, params map[string]interface{}, _ int, _ bool, _ bool) *banexg.HttpRes {
				requireBybitReq(t, endpoint, params, tc.method, banexg.MarketLinear, "BTCUSDT")
				if params["interval"] != "1" {
					t.Fatalf("unexpected interval: %v", params["interval"])
				}
				body := `{"retCode":0,"retMsg":"OK","result":{"symbol":"BTCUSDT","category":"linear","list":[["2","2","3","1","2.5","20","200"]]},"retExtInfo":{},"time":1700000000000}`
				return &banexg.HttpRes{Status: 200, Content: body}
			})

			params := map[string]interface{}{"price": tc.price}
			klines, err := exg.FetchOHLCV("BTC/USDT:USDT", "1m", 0, 1, params)
			if err != nil {
				t.Fatalf("FetchOHLCV failed: %v", err)
			}
			if len(klines) != 1 {
				t.Fatalf("expected 1 kline, got %d", len(klines))
			}
		})
	}
}

func TestFetchOrderBookParams(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	setBybitTestRequest(t, func(_ context.Context, endpoint string, params map[string]interface{}, _ int, _ bool, _ bool) *banexg.HttpRes {
		requireBybitReq(t, endpoint, params, MethodPublicGetV5MarketOrderbook, banexg.MarketSpot, "BTCUSDT")
		if params["limit"] != 200 {
			t.Fatalf("unexpected limit: %v", params["limit"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"s":"BTCUSDT","a":[["101","1"]],"b":[["100","2"]],"ts":1700000000000,"u":123},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})

	book, err := exg.FetchOrderBook("BTC/USDT", 999, nil)
	if err != nil {
		t.Fatalf("FetchOrderBook failed: %v", err)
	}
	if book == nil || book.Limit != 200 {
		t.Fatalf("unexpected orderbook limit: %+v", book)
	}
	if len(book.Asks.Price) == 0 || len(book.Bids.Price) == 0 {
		t.Fatal("expected non-empty orderbook sides")
	}
}

func TestFetchOHLCVParams(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	setBybitTestRequest(t, func(_ context.Context, endpoint string, params map[string]interface{}, _ int, _ bool, _ bool) *banexg.HttpRes {
		requireBybitReq(t, endpoint, params, MethodPublicGetV5MarketMarkPriceKline, banexg.MarketLinear, "BTCUSDT")
		if params["interval"] != "1" {
			t.Fatalf("unexpected interval: %v", params["interval"])
		}
		if params["limit"] != 2 {
			t.Fatalf("unexpected limit: %v", params["limit"])
		}
		if params["start"] != int64(123) {
			t.Fatalf("unexpected start: %v", params["start"])
		}
		if params["end"] != int64(456) {
			t.Fatalf("unexpected end: %v", params["end"])
		}
		body := `{"retCode":0,"retMsg":"OK","result":{"symbol":"BTCUSDT","category":"linear","list":[["2","2","3","1","2.5","20","200"],["1","1","2","0.5","1.5","10","100"]]},"retExtInfo":{},"time":1700000000000}`
		return &banexg.HttpRes{Status: 200, Content: body}
	})

	klines, err := exg.FetchOHLCV("BTC/USDT:USDT", "1m", 123, 2, map[string]interface{}{
		"price":           "mark",
		banexg.ParamUntil: int64(456),
	})
	if err != nil {
		t.Fatalf("FetchOHLCV failed: %v", err)
	}
	if len(klines) != 2 {
		t.Fatalf("expected 2 klines, got %d", len(klines))
	}
	if klines[0].Time != 1 || klines[1].Time != 2 {
		t.Fatalf("unexpected kline order: %d,%d", klines[0].Time, klines[1].Time)
	}
}
