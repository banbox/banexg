package bybit

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

func assertFloatNear(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("expected %.10f, got %.10f", want, got)
	}
}

func TestBybitCategoryFromMarket(t *testing.T) {
	cases := []struct {
		name    string
		market  *banexg.Market
		expect  string
		wantErr bool
	}{
		{name: "spot", market: &banexg.Market{Spot: true, Type: banexg.MarketSpot}, expect: banexg.MarketSpot},
		{name: "linear", market: &banexg.Market{Linear: true, Type: banexg.MarketLinear}, expect: banexg.MarketLinear},
		{name: "inverse", market: &banexg.Market{Inverse: true, Type: banexg.MarketInverse}, expect: banexg.MarketInverse},
		{name: "option", market: &banexg.Market{Option: true, Type: banexg.MarketOption}, expect: banexg.MarketOption},
		{name: "margin", market: &banexg.Market{Type: banexg.MarketMargin}, expect: banexg.MarketSpot},
		{name: "empty", market: &banexg.Market{Type: ""}, expect: banexg.MarketSpot},
		{name: "nil", market: nil, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := bybitCategoryFromMarket(tc.market)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expect {
				t.Fatalf("expected %s, got %s", tc.expect, got)
			}
		})
	}
}

func TestBybitCategoryFromType(t *testing.T) {
	cases := []struct {
		name      string
		market    string
		expect    string
		expectErr bool
	}{
		{name: "spot", market: banexg.MarketSpot, expect: banexg.MarketSpot},
		{name: "margin", market: banexg.MarketMargin, expect: banexg.MarketSpot},
		{name: "linear", market: banexg.MarketLinear, expect: banexg.MarketLinear},
		{name: "inverse", market: banexg.MarketInverse, expect: banexg.MarketInverse},
		{name: "option", market: banexg.MarketOption, expect: banexg.MarketOption},
		{name: "invalid", market: "unknown", expectErr: true},
		{name: "empty", market: "", expectErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := bybitCategoryFromType(tc.market)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expect {
				t.Fatalf("expected %s, got %s", tc.expect, got)
			}
		})
	}
}

func TestBybitOrderBookLimit(t *testing.T) {
	cases := []struct {
		name       string
		marketType string
		limit      int
		expect     int
	}{
		{name: "spotNeg", marketType: banexg.MarketSpot, limit: -1, expect: 0},
		{name: "spotZero", marketType: banexg.MarketSpot, limit: 0, expect: 0},
		{name: "spot1", marketType: banexg.MarketSpot, limit: 1, expect: 1},
		{name: "spotCap", marketType: banexg.MarketSpot, limit: 999, expect: 200},
		{name: "linearCap", marketType: banexg.MarketLinear, limit: 999, expect: 500},
		{name: "optionCap", marketType: banexg.MarketOption, limit: 999, expect: 25},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := bybitOrderBookLimit(tc.marketType, tc.limit); got != tc.expect {
				t.Fatalf("expected %d, got %d", tc.expect, got)
			}
		})
	}
}

func TestMinPositive(t *testing.T) {
	if got := minPositive(-1, 0, 5, 2); got != 2 {
		t.Fatalf("expected 2, got %v", got)
	}
	if got := minPositive(0, -1); got != 0 {
		t.Fatalf("expected 0, got %v", got)
	}
}

func TestLotSizeFtParse(t *testing.T) {
	ft := &LotSizeFt{
		BasePrecision:     "0.001",
		MinOrderQty:       "1",
		MaxOrderQty:       "100",
		MinOrderAmt:       "5",
		MaxOrderAmt:       "1000",
		MaxLimitOrderQty:  "80",
		MaxMarketOrderQty: "50",
	}
	amtPrec, minQty, maxQty, minAmt, maxAmt := ft.parse()
	assertFloatNear(t, amtPrec, 0.001)
	assertFloatNear(t, minQty, 1)
	assertFloatNear(t, maxQty, 50)
	assertFloatNear(t, minAmt, 5)
	assertFloatNear(t, maxAmt, 1000)
}

func TestOptionLotSizeFtParse(t *testing.T) {
	ft := &OptionLotSizeFt{
		MinOrderQty: "0.1",
		MaxOrderQty: "10",
		QtyStep:     "0.01",
	}
	minQty, maxQty, step := ft.parse()
	assertFloatNear(t, minQty, 0.1)
	assertFloatNear(t, maxQty, 10)
	assertFloatNear(t, step, 0.01)
}

func TestFutureLotSizeFtParse(t *testing.T) {
	ft := &FutureLotSizeFt{
		OptionLotSizeFt: OptionLotSizeFt{
			MinOrderQty: "1",
			MaxOrderQty: "100",
			QtyStep:     "0.5",
		},
		MaxMktOrderQty: "50",
	}
	minQty, maxQty, step, maxMkt := ft.parse()
	assertFloatNear(t, minQty, 1)
	assertFloatNear(t, maxQty, 100)
	assertFloatNear(t, step, 0.5)
	assertFloatNear(t, maxMkt, 50)
}

func TestBaseMarketToStdMarket(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	market := &BaseMarket{
		Symbol:    "BTCUSDT",
		BaseCoin:  "BTC",
		QuoteCoin: "USDT",
		Status:    "Trading",
	}
	std := market.ToStdMarket(exg)
	if std.Symbol != "BTC/USDT" {
		t.Fatalf("expected symbol BTC/USDT, got %s", std.Symbol)
	}
	if std.ID != "BTCUSDT" {
		t.Fatalf("expected ID BTCUSDT, got %s", std.ID)
	}
	if !std.Active {
		t.Fatal("expected active market")
	}
	if std.BaseID != "BTC" || std.QuoteID != "USDT" {
		t.Fatalf("unexpected base/quote IDs: %s/%s", std.BaseID, std.QuoteID)
	}
}

func TestContractMarketToStdMarket(t *testing.T) {
	exg := newBybitWithMarket("BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	delivery := int64(1700000000000)
	market := &ContractMarket{
		BaseMarket: BaseMarket{
			Symbol:    "BTCUSDT",
			BaseCoin:  "BTC",
			QuoteCoin: "USDT",
			Status:    "Trading",
		},
		SettleCoin:   "USDT",
		LaunchTime:   "1690000000000",
		DeliveryTime: "1700000000000",
		PriceFilter: &PriceFt{
			MinPrice: "1",
			MaxPrice: "100",
			TickSize: "0.5",
		},
	}
	std := market.ToStdMarket(exg)
	expectSymbol := "BTC/USDT:USDT-" + utils.YMD(delivery, "", false)
	if std.Symbol != expectSymbol {
		t.Fatalf("expected symbol %s, got %s", expectSymbol, std.Symbol)
	}
	if !std.Contract || !std.Future {
		t.Fatalf("expected contract future market, got contract=%v future=%v", std.Contract, std.Future)
	}
	if std.Settle != "USDT" || std.SettleID != "USDT" {
		t.Fatalf("unexpected settle: %s/%s", std.Settle, std.SettleID)
	}
	if std.Expiry != delivery {
		t.Fatalf("expected expiry %d, got %d", delivery, std.Expiry)
	}
	if std.ExpiryDatetime != utils.ISO8601(delivery) {
		t.Fatalf("unexpected expiry datetime: %s", std.ExpiryDatetime)
	}
	assertFloatNear(t, std.Precision.Price, 0.5)
	assertFloatNear(t, std.Limits.Price.Min, 1)
	assertFloatNear(t, std.Limits.Price.Max, 100)
}

func TestParseBybitNumInt(t *testing.T) {
	assertFloatNear(t, parseBybitNum("1.25"), 1.25)
	if got := parseBybitNum("bad"); got != 0 {
		t.Fatalf("expected 0 for invalid num, got %v", got)
	}
	if got := parseBybitInt("123"); got != 123 {
		t.Fatalf("expected 123, got %d", got)
	}
	if got := parseBybitInt("bad"); got != 0 {
		t.Fatalf("expected 0 for invalid int, got %d", got)
	}
}

func TestParseBybitBookSide(t *testing.T) {
	levels := [][]string{{"10", "1.5"}, {"9", "2"}, {"bad"}}
	got := bybitParseBookSide(levels)
	if len(got) != 2 {
		t.Fatalf("expected 2 levels, got %d", len(got))
	}
	if got[0][0] != 10 || got[0][1] != 1.5 {
		t.Fatalf("unexpected first level: %#v", got[0])
	}
}

func TestParseBybitOrderBook(t *testing.T) {
	if parseBybitOrderBook(nil, nil, 0) != nil {
		t.Fatal("expected nil orderbook when input is nil")
	}
	market := &banexg.Market{Symbol: "BTC/USDT"}
	ob := &orderBookSnapshot{
		Symbol: "BTCUSDT",
		Asks:   [][]string{{"11", "1"}, {"10", "2"}},
		Bids:   [][]string{{"9", "3"}, {"10", "4"}},
		Ts:     1700000000000,
		Update: 123,
	}
	book := parseBybitOrderBook(market, ob, 2)
	if book == nil {
		t.Fatal("expected orderbook")
	}
	if book.Symbol != "BTC/USDT" {
		t.Fatalf("unexpected symbol: %s", book.Symbol)
	}
	if len(book.Asks.Price) != 2 || len(book.Bids.Price) != 2 {
		t.Fatalf("unexpected depth: asks=%d bids=%d", len(book.Asks.Price), len(book.Bids.Price))
	}
	if book.Asks.Price[0] != 10 {
		t.Fatalf("asks not sorted ascending: %v", book.Asks.Price)
	}
	if book.Bids.Price[0] != 10 {
		t.Fatalf("bids not sorted descending: %v", book.Bids.Price)
	}
}

func TestSetBybitSymbolArg(t *testing.T) {
	market := &banexg.Market{Symbol: "BTC/USDT", ID: "BTCUSDT", Type: banexg.MarketSpot, Spot: true}
	exg := &Bybit{Exchange: &banexg.Exchange{ExgInfo: &banexg.ExgInfo{
		Markets: banexg.MarketMap{"BTC/USDT": market},
	}}}

	args := map[string]interface{}{}
	if err := setBybitSymbolArg(exg, args, []string{"BTC/USDT"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args["symbol"] != "BTCUSDT" {
		t.Fatalf("expected BTCUSDT, got %v", args["symbol"])
	}

	args2 := map[string]interface{}{"symbol": "ETHUSDT"}
	if err := setBybitSymbolArg(exg, args2, []string{"BTC/USDT"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args2["symbol"] != "ETHUSDT" {
		t.Fatalf("expected symbol to be preserved, got %v", args2["symbol"])
	}

	args3 := map[string]interface{}{}
	if err := setBybitSymbolArg(exg, args3, []string{"BTC/USDT", "ETH/USDT"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := args3["symbol"]; ok {
		t.Fatalf("did not expect symbol to be set for multiple symbols")
	}

	miss := &Bybit{Exchange: &banexg.Exchange{ExgInfo: &banexg.ExgInfo{
		Markets: banexg.MarketMap{},
	}}}
	err := setBybitSymbolArg(miss, map[string]interface{}{}, []string{"BTC/USDT"})
	if err == nil {
		t.Fatal("expected error for missing market")
	}
}

func TestApi_LoadMarkets(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	markets, err := exg.LoadMarkets(false, nil)
	if err != nil {
		t.Fatalf("LoadMarkets failed: %v", err)
	}
	if len(markets) == 0 {
		t.Fatal("expected non-empty markets")
	}
}

type firstErrRecorder struct {
	mu  sync.Mutex
	msg string
}

func (r *firstErrRecorder) Recordf(format string, args ...interface{}) {
	r.mu.Lock()
	if r.msg == "" {
		r.msg = fmt.Sprintf(format, args...)
	}
	r.mu.Unlock()
}

func (r *firstErrRecorder) Message() string {
	r.mu.Lock()
	msg := r.msg
	r.mu.Unlock()
	return msg
}

func mustNewBybit(t *testing.T, name string) *Bybit {
	t.Helper()
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new bybit exchange failed: %v", err)
	}
	exg.Name = name
	return exg
}

func mustInstrumentsInfoResp(t *testing.T, category string, list []map[string]interface{}, nextPageCursor string, timestamp int64) string {
	t.Helper()
	return mustMarshal(t, map[string]interface{}{
		"retCode": 0,
		"retMsg":  "OK",
		"result": map[string]interface{}{
			"category":       category,
			"nextPageCursor": nextPageCursor,
			"list":           list,
		},
		"retExtInfo": map[string]interface{}{},
		"time":       timestamp,
	})
}

func ensureParamAbsent(rec *firstErrRecorder, params map[string]interface{}, key, name string) {
	if _, ok := params[key]; ok {
		rec.Recordf("spot request should not include %s", name)
	}
}

func TestFetchMarkets_SpotMapping(t *testing.T) {
	exg := mustNewBybit(t, "BybitTestFetchMarketsSpot")

	spotList := []map[string]interface{}{
		{
			"symbol":        "BTCUSDT",
			"baseCoin":      "BTC",
			"quoteCoin":     "USDT",
			"status":        "Trading",
			"marginTrading": "both",
			"lotSizeFilter": map[string]interface{}{
				"basePrecision":     "0.001",
				"quotePrecision":    "0.01",
				"minOrderQty":       "0.01",
				"maxOrderQty":       "100",
				"minOrderAmt":       "5",
				"maxOrderAmt":       "1000",
				"maxLimitOrderQty":  "50",
				"maxMarketOrderQty": "20",
			},
			"priceFilter": map[string]interface{}{
				"tickSize": "0.1",
			},
		},
	}
	spotResp := mustInstrumentsInfoResp(t, "spot", spotList, "", 1700000000000)
	var rec firstErrRecorder

	setBybitTestRequest(t, func(_ context.Context, endpoint string, params map[string]interface{}, _ int, _, _ bool) *banexg.HttpRes {
		if endpoint != MethodPublicGetV5MarketInstrumentsInfo {
			rec.Recordf("unexpected endpoint: %s", endpoint)
		}
		if params["category"] != "spot" {
			rec.Recordf("expected spot category, got %v", params["category"])
		}
		ensureParamAbsent(&rec, params, banexg.ParamLimit, "limit")
		ensureParamAbsent(&rec, params, banexg.ParamAfter, "after")
		ensureParamAbsent(&rec, params, "cursor", "cursor")
		return &banexg.HttpRes{Content: spotResp}
	})

	markets, err := exg.FetchMarkets([]string{banexg.MarketSpot}, map[string]interface{}{
		banexg.ParamLimit: 999,
		banexg.ParamAfter: "cursor-a",
		"cursor":          "cursor-b",
	})
	if err != nil {
		t.Fatalf("FetchMarkets failed: %v", err)
	}
	if errMsg := rec.Message(); errMsg != "" {
		t.Fatalf("request validation failed: %s", errMsg)
	}
	if len(markets) != 1 {
		t.Fatalf("expected 1 market, got %d", len(markets))
	}
	market := markets["BTC/USDT"]
	if market == nil {
		t.Fatalf("expected BTC/USDT market")
	}
	if !market.Spot || market.Type != banexg.MarketSpot {
		t.Fatalf("expected spot market, got type=%s spot=%v", market.Type, market.Spot)
	}
	if !market.Margin {
		t.Fatalf("expected margin trading enabled")
	}
	assertFloatNear(t, market.Precision.Amount, 0.001)
	assertFloatNear(t, market.Precision.Price, 0.1)
	assertFloatNear(t, market.Precision.Quote, 0.01)
	assertFloatNear(t, market.Limits.Amount.Min, 0.01)
	assertFloatNear(t, market.Limits.Amount.Max, 20)
	assertFloatNear(t, market.Limits.Cost.Min, 5)
	assertFloatNear(t, market.Limits.Cost.Max, 1000)
	if market.Limits.Market == nil {
		t.Fatalf("expected market limits")
	}
	assertFloatNear(t, market.Limits.Market.Max, 20)
}

func TestFetchFutureMarkets_CursorPagination(t *testing.T) {
	exg := mustNewBybit(t, "BybitTestFetchMarketsFuture")

	futureList1 := []map[string]interface{}{
		{
			"symbol":       "BTCUSDT",
			"baseCoin":     "BTC",
			"quoteCoin":    "USDT",
			"settleCoin":   "USDT",
			"status":       "Trading",
			"contractType": "LinearPerpetual",
			"launchTime":   "1690000000000",
			"deliveryTime": "0",
			"priceFilter": map[string]interface{}{
				"minPrice": "1",
				"maxPrice": "100000",
				"tickSize": "0.5",
			},
			"lotSizeFilter": map[string]interface{}{
				"minOrderQty":      "0.001",
				"maxOrderQty":      "0",
				"qtyStep":          "0.001",
				"maxMktOrderQty":   "200",
				"minNotionalValue": "5",
			},
			"leverageFilter": map[string]interface{}{
				"minLeverage": "1",
				"maxLeverage": "50",
			},
		},
	}
	futureList2 := []map[string]interface{}{
		{
			"symbol":       "ETHUSDT",
			"baseCoin":     "ETH",
			"quoteCoin":    "USDT",
			"settleCoin":   "USDT",
			"status":       "Trading",
			"contractType": "LinearPerpetual",
			"launchTime":   "1690000000001",
			"deliveryTime": "0",
			"priceFilter": map[string]interface{}{
				"minPrice": "0.5",
				"maxPrice": "50000",
				"tickSize": "0.05",
			},
			"lotSizeFilter": map[string]interface{}{
				"minOrderQty":      "0.01",
				"maxOrderQty":      "10",
				"qtyStep":          "0.01",
				"maxMktOrderQty":   "5",
				"minNotionalValue": "1",
			},
			"leverageFilter": map[string]interface{}{
				"minLeverage": "1",
				"maxLeverage": "25",
			},
		},
	}

	page1 := mustInstrumentsInfoResp(t, "linear", futureList1, "next-page", 1700000000000)
	page2 := mustInstrumentsInfoResp(t, "linear", futureList2, "", 1700000000001)

	call := 0
	setBybitTestRequest(t, func(_ context.Context, endpoint string, params map[string]interface{}, _ int, _, _ bool) *banexg.HttpRes {
		if endpoint != MethodPublicGetV5MarketInstrumentsInfo {
			t.Fatalf("unexpected endpoint: %s", endpoint)
		}
		if params["category"] != "linear" {
			t.Fatalf("expected linear category, got %v", params["category"])
		}
		if params[banexg.ParamLimit] != 1000 {
			t.Fatalf("expected default limit 1000, got %v", params[banexg.ParamLimit])
		}
		cursor, _ := params["cursor"].(string)
		call++
		switch call {
		case 1:
			if cursor != "start" {
				t.Fatalf("expected cursor 'start', got %q", cursor)
			}
			return &banexg.HttpRes{Content: page1}
		case 2:
			if cursor != "next-page" {
				t.Fatalf("expected cursor 'next-page', got %q", cursor)
			}
			return &banexg.HttpRes{Content: page2}
		default:
			t.Fatalf("unexpected request count: %d", call)
			return &banexg.HttpRes{Content: page2}
		}
	})

	markets, err := exg.fetchFutureMarkets(map[string]interface{}{
		"category":        "linear",
		banexg.ParamAfter: "start",
	})
	if err != nil {
		t.Fatalf("fetchFutureMarkets failed: %v", err)
	}
	if len(markets) != 2 {
		t.Fatalf("expected 2 markets, got %d", len(markets))
	}
	market := markets["BTC/USDT:USDT"]
	if market == nil {
		t.Fatalf("expected BTC/USDT:USDT market")
	}
	assertFloatNear(t, market.Precision.Amount, 0.001)
	assertFloatNear(t, market.Precision.Price, 0.5)
	assertFloatNear(t, market.Limits.Amount.Min, 0.001)
	assertFloatNear(t, market.Limits.Amount.Max, 200)
	assertFloatNear(t, market.Limits.Cost.Min, 5)
	if market.Limits.Market == nil {
		t.Fatalf("expected market limits")
	}
	assertFloatNear(t, market.Limits.Market.Max, 200)
	assertFloatNear(t, market.Limits.Leverage.Min, 1)
	assertFloatNear(t, market.Limits.Leverage.Max, 50)
}

func TestLoadMarkets_Spot(t *testing.T) {
	exg := mustNewBybit(t, "BybitTestLoadMarkets")
	exg.CareMarkets = []string{banexg.MarketSpot}
	exg.FetchCurrencies = func(_ map[string]interface{}) (banexg.CurrencyMap, *errs.Error) {
		return banexg.CurrencyMap{}, nil
	}

	spotList := []map[string]interface{}{
		{
			"symbol":        "BTCUSDT",
			"baseCoin":      "BTC",
			"quoteCoin":     "USDT",
			"status":        "Trading",
			"marginTrading": "none",
			"lotSizeFilter": map[string]interface{}{
				"basePrecision":     "0.001",
				"quotePrecision":    "0.01",
				"minOrderQty":       "0.01",
				"maxOrderQty":       "100",
				"minOrderAmt":       "5",
				"maxOrderAmt":       "1000",
				"maxLimitOrderQty":  "100",
				"maxMarketOrderQty": "100",
			},
			"priceFilter": map[string]interface{}{
				"tickSize": "0.1",
			},
		},
	}
	spotResp := mustInstrumentsInfoResp(t, "spot", spotList, "", 1700000000002)
	var rec firstErrRecorder

	setBybitTestRequest(t, func(_ context.Context, endpoint string, params map[string]interface{}, _ int, _, _ bool) *banexg.HttpRes {
		if endpoint != MethodPublicGetV5MarketInstrumentsInfo {
			rec.Recordf("unexpected endpoint: %s", endpoint)
		}
		if params["category"] != "spot" {
			rec.Recordf("expected spot category, got %v", params["category"])
		}
		return &banexg.HttpRes{Content: spotResp}
	})

	markets, err := exg.LoadMarkets(true, nil)
	if err != nil {
		t.Fatalf("LoadMarkets failed: %v", err)
	}
	if errMsg := rec.Message(); errMsg != "" {
		t.Fatalf("request validation failed: %s", errMsg)
	}
	if len(markets) != 1 {
		t.Fatalf("expected 1 market, got %d", len(markets))
	}
	if _, ok := exg.MarketsById["BTCUSDT"]; !ok {
		t.Fatalf("expected MarketsById to contain BTCUSDT")
	}
}

// Tests migrated from api_loadmarkets_test.go and api_market_test.go

func bybitTestExgName(t *testing.T) string {
	t.Helper()
	// Keep names stable while avoiding repetition in each test.
	// Example: "TestApi_LoadMarkets_Spot_Default" -> "BybitTestApiLoadMarketsSpotDefault"
	name := strings.NewReplacer("_", "", "/", "_").Replace(t.Name())
	return "Bybit" + name
}

func newBybitLoadMarketsTestExg(t *testing.T, care []string, withCurr bool) *Bybit {
	t.Helper()
	return newBybitLoadMarketsExg(t, bybitTestExgName(t), care, withCurr)
}

func newBybitLoadMarketsExg(t *testing.T, name string, care []string, withCurr bool) *Bybit {
	t.Helper()
	var exg *Bybit
	if withCurr {
		exg = getBybitAuthed(t, nil)
	} else {
		exg = getBybitAuthedNoCurr(t, nil)
	}
	if exg == nil {
		t.Skip("bybit exchange not initialized")
		return nil
	}
	exg.Name = name
	exg.CareMarkets = care
	return exg
}

func ensureNoCache(params map[string]interface{}) map[string]interface{} {
	if params == nil {
		params = map[string]interface{}{}
	}
	if _, ok := params[banexg.ParamNoCache]; !ok {
		params[banexg.ParamNoCache] = true
	}
	return params
}

func loadMarkets(t *testing.T, exg *Bybit, params map[string]interface{}, allowAuthSkip bool) banexg.MarketMap {
	t.Helper()
	return loadMarketsMin(t, exg, params, allowAuthSkip, 1)
}

func loadMarketsMin(t *testing.T, exg *Bybit, params map[string]interface{}, allowAuthSkip bool, minMarkets int) banexg.MarketMap {
	t.Helper()
	params = ensureNoCache(params)
	markets, err := exg.LoadMarkets(true, params)
	if err != nil {
		if allowAuthSkip && isAuthError(err) {
			t.Skipf("skip due to auth error: %v", err)
		}
		t.Fatalf("LoadMarkets failed: %v", err)
	}
	if minMarkets > 0 && len(markets) < minMarkets {
		t.Fatal("expected non-empty markets")
	}
	return markets
}

func isAuthError(err *errs.Error) bool {
	if err == nil {
		return false
	}
	switch err.Code {
	case errs.CodeAccKeyError, errs.CodeUnauthorized, errs.CodeForbidden:
		return true
	default:
		return false
	}
}

func pickSymbolType(markets banexg.MarketMap) string {
	for _, m := range markets {
		if m == nil || m.Info == nil {
			continue
		}
		if val, ok := m.Info["symbolType"]; ok {
			if symType, ok2 := val.(string); ok2 && symType != "" {
				return symType
			}
		}
	}
	return ""
}

func pickMarketID(markets banexg.MarketMap) string {
	for _, m := range markets {
		if m != nil && m.ID != "" {
			return m.ID
		}
	}
	return ""
}

func assertMarketID(t *testing.T, markets banexg.MarketMap, id string) {
	t.Helper()
	for _, m := range markets {
		if m != nil && m.ID == id {
			return
		}
	}
	t.Fatalf("expected market id %s in results", id)
}

func fetchInstrumentsCursor(t *testing.T, exg *Bybit, params map[string]interface{}) string {
	t.Helper()
	args := ensureNoCache(utils.SafeParams(params))
	tryNum := exg.GetRetryNum("FetchMarkets", 1)
	res := requestRetry[V5ListResult](exg, MethodPublicGetV5MarketInstrumentsInfo, args, tryNum)
	if res.Error != nil {
		t.Fatalf("fetch cursor failed: %v", res.Error)
	}
	return res.Result.NextPageCursor
}

func TestApi_LoadMarkets_Spot_Default(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketSpot}, false)
	loadMarkets(t, exg, nil, false)
}

func TestApi_LoadMarkets_Spot_Symbol(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketSpot}, false)
	markets := loadMarkets(t, exg, map[string]interface{}{
		banexg.ParamSymbol: "BTCUSDT",
	}, false)
	assertMarketID(t, markets, "BTCUSDT")
}

func TestApi_LoadMarkets_Spot_Status(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketSpot}, false)
	loadMarkets(t, exg, map[string]interface{}{
		"status": "Trading",
	}, false)
}

func TestApi_LoadMarkets_Spot_SymbolType(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketSpot}, false)
	base := loadMarkets(t, exg, map[string]interface{}{
		"status": "Trading",
	}, false)
	symbolType := pickSymbolType(base)
	if symbolType == "" {
		t.Skip("no symbolType found in spot markets")
	}
	loadMarkets(t, exg, map[string]interface{}{
		"symbolType": symbolType,
	}, false)
}

func TestApi_LoadMarkets_Linear_Default(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketLinear}, false)
	loadMarkets(t, exg, nil, false)
}

func TestApi_LoadMarkets_Linear_Symbol(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketLinear}, false)
	markets := loadMarkets(t, exg, map[string]interface{}{
		banexg.ParamSymbol: "BTCUSDT",
	}, false)
	assertMarketID(t, markets, "BTCUSDT")
}

func TestApi_LoadMarkets_Linear_BaseCoin(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketLinear}, false)
	loadMarkets(t, exg, map[string]interface{}{
		"baseCoin": "BTC",
	}, false)
}

func TestApi_LoadMarkets_Linear_Status(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketLinear}, false)
	loadMarkets(t, exg, map[string]interface{}{
		"status": "Trading",
	}, false)
}

func TestApi_LoadMarkets_Linear_SymbolType(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketLinear}, false)
	base := loadMarkets(t, exg, nil, false)
	symbolType := pickSymbolType(base)
	if symbolType == "" {
		t.Skip("no symbolType found in linear markets")
	}
	loadMarkets(t, exg, map[string]interface{}{
		"symbolType": symbolType,
	}, false)
}

func TestApi_LoadMarkets_Linear_Limit(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketLinear}, false)
	loadMarkets(t, exg, map[string]interface{}{
		banexg.ParamLimit: 1000,
	}, false)
}

func TestApi_LoadMarkets_Linear_After(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketLinear}, false)
	cursor := fetchInstrumentsCursor(t, exg, map[string]interface{}{
		"category":        "linear",
		"baseCoin":        "BTC",
		banexg.ParamLimit: 1,
	})
	if cursor == "" {
		t.Skip("no cursor available for linear markets")
	}
	loadMarkets(t, exg, map[string]interface{}{
		"baseCoin":        "BTC",
		banexg.ParamLimit: 1,
		banexg.ParamAfter: cursor,
	}, false)
}

func TestApi_LoadMarkets_Inverse_Default(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketInverse}, false)
	loadMarkets(t, exg, nil, false)
}

func TestApi_LoadMarkets_Inverse_Symbol(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketInverse}, false)
	markets := loadMarkets(t, exg, map[string]interface{}{
		banexg.ParamSymbol: "BTCUSD",
	}, false)
	assertMarketID(t, markets, "BTCUSD")
}

func TestApi_LoadMarkets_Inverse_BaseCoin(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketInverse}, false)
	loadMarkets(t, exg, map[string]interface{}{
		"baseCoin": "BTC",
	}, false)
}

func TestApi_LoadMarkets_Inverse_Status(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketInverse}, false)
	loadMarkets(t, exg, map[string]interface{}{
		"status": "Trading",
	}, false)
}

func TestApi_LoadMarkets_Inverse_SymbolType(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketInverse}, false)
	base := loadMarkets(t, exg, nil, false)
	symbolType := pickSymbolType(base)
	if symbolType == "" {
		symbolType = "innovation"
	}
	markets := loadMarketsMin(t, exg, map[string]interface{}{
		"symbolType": symbolType,
	}, false, 0)
	if len(markets) == 0 {
		t.Logf("no inverse markets for symbolType=%s", symbolType)
	}
}

func TestApi_LoadMarkets_Inverse_Limit(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketInverse}, false)
	loadMarkets(t, exg, map[string]interface{}{
		banexg.ParamLimit: 1000,
	}, false)
}

func TestApi_LoadMarkets_Option_Default(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketOption}, false)
	loadMarkets(t, exg, nil, false)
}

func TestApi_LoadMarkets_Option_BaseCoin(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketOption}, false)
	loadMarkets(t, exg, map[string]interface{}{
		"baseCoin": "BTC",
	}, false)
}

func TestApi_LoadMarkets_Option_Status(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketOption}, false)
	loadMarkets(t, exg, map[string]interface{}{
		"status": "Trading",
	}, false)
}

func TestApi_LoadMarkets_Option_Symbol(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketOption}, false)
	base := loadMarkets(t, exg, map[string]interface{}{
		"baseCoin": "BTC",
	}, false)
	symbol := pickMarketID(base)
	if symbol == "" {
		t.Skip("no option symbol found")
	}
	loadMarkets(t, exg, map[string]interface{}{
		banexg.ParamSymbol: symbol,
	}, false)
}

func TestApi_LoadMarkets_Option_Limit(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketOption}, false)
	loadMarkets(t, exg, map[string]interface{}{
		banexg.ParamLimit: 1000,
	}, false)
}

func TestApi_LoadMarkets_Currency_Filter(t *testing.T) {
	exg := newBybitLoadMarketsTestExg(t, []string{banexg.MarketSpot}, true)
	loadMarkets(t, exg, map[string]interface{}{
		banexg.ParamCurrency: "USDT",
	}, true)
	if exg.CurrenciesByCode == nil || exg.CurrenciesByCode["USDT"] == nil {
		t.Fatal("expected USDT currency info")
	}
}

func assertMarketLimits(t *testing.T, exg *Bybit, symbol string) {
	t.Helper()
	market := exg.Markets[symbol]
	if market == nil {
		t.Fatalf("expected market for symbol %s", symbol)
	}
	if market.Limits.Amount == nil || market.Limits.Amount.Min == 0 {
		t.Errorf("expected Amount.Min for %s", symbol)
	}
}

func loadBybitMarketsForTypes(t *testing.T, exg *Bybit, marketTypes ...string) banexg.MarketMap {
	t.Helper()
	origin := exg.CareMarkets
	exg.CareMarkets = append([]string(nil), marketTypes...)
	defer func() {
		exg.CareMarkets = origin
	}()
	return loadMarkets(t, exg, nil, false)
}

func loadBybitMarketsForType(t *testing.T, exg *Bybit, marketType string) banexg.MarketMap {
	t.Helper()
	return loadBybitMarketsForTypes(t, exg, marketType)
}

func bybitMarketMatchesType(market *banexg.Market, marketType string) bool {
	if market == nil {
		return false
	}
	switch marketType {
	case banexg.MarketSpot:
		return market.Spot
	case banexg.MarketLinear:
		return market.Linear
	case banexg.MarketInverse:
		return market.Inverse
	case banexg.MarketOption:
		return market.Option
	default:
		return false
	}
}

func bybitSwapMarketMatchesType(market *banexg.Market, marketType string) bool {
	if market == nil || !market.Swap {
		return false
	}
	return bybitMarketMatchesType(market, marketType)
}

func pickBybitMarketByType(markets banexg.MarketMap, marketType string) *banexg.Market {
	for _, market := range markets {
		if bybitMarketMatchesType(market, marketType) {
			return market
		}
	}
	return nil
}

func pickBybitMarketsByType(markets banexg.MarketMap, marketType string, limit int) []*banexg.Market {
	items := make([]*banexg.Market, 0, limit)
	for _, market := range markets {
		if !bybitMarketMatchesType(market, marketType) {
			continue
		}
		items = append(items, market)
		if limit > 0 && len(items) >= limit {
			break
		}
	}
	return items
}

func bybitOptionExpDate(market *banexg.Market) string {
	if market == nil || market.ID == "" {
		return ""
	}
	parts := strings.Split(market.ID, "-")
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

func tickerHasSymbol(tickers []*banexg.Ticker, symbol string) bool {
	for _, item := range tickers {
		if item != nil && item.Symbol == symbol {
			return true
		}
	}
	return false
}

func lastPricesHasSymbol(items []*banexg.LastPrice, symbol string) bool {
	for _, item := range items {
		if item != nil && item.Symbol == symbol {
			return true
		}
	}
	return false
}

func limitSortedStrings(keys []string, limit int) []string {
	if len(keys) == 0 || limit <= 0 {
		return nil
	}
	sort.Strings(keys)
	if len(keys) > limit {
		keys = keys[:limit]
	}
	return keys
}

func samplePriceKeys(prices map[string]float64, limit int) []string {
	if len(prices) == 0 || limit <= 0 {
		return nil
	}
	keys := make([]string, 0, len(prices))
	for k := range prices {
		keys = append(keys, k)
	}
	return limitSortedStrings(keys, limit)
}

func requirePriceMapHasSymbol(t *testing.T, prices map[string]float64, symbol string) {
	t.Helper()
	if len(prices) == 0 {
		t.Fatal("expected non-empty prices")
	}
	if symbol == "" {
		t.Fatal("expected non-empty symbol")
	}
	if _, ok := prices[symbol]; !ok {
		t.Fatalf("expected %s price, got %d keys (sample: %v)", symbol, len(prices), samplePriceKeys(prices, 10))
	}
}

func requireLastPricesHasSymbol(t *testing.T, items []*banexg.LastPrice, symbol string) {
	t.Helper()
	if len(items) == 0 {
		t.Fatal("expected non-empty last prices")
	}
	if symbol == "" {
		t.Fatal("expected non-empty symbol")
	}
	if !lastPricesHasSymbol(items, symbol) {
		t.Fatalf("expected %s last price in results", symbol)
	}
}

func pickBybitSwapMarketByType(markets banexg.MarketMap, marketType string) *banexg.Market {
	for _, market := range markets {
		if bybitSwapMarketMatchesType(market, marketType) {
			return market
		}
	}
	return nil
}

func bybitSwapSymbolForSpot(spot *banexg.Market) string {
	if spot == nil || spot.Symbol == "" || spot.Quote == "" {
		return ""
	}
	// In banexg, linear swap symbol is "{spotSymbol}:{quote}" (e.g. "BTC/USDT:USDT").
	return fmt.Sprintf("%s:%s", spot.Symbol, spot.Quote)
}

func bybitSwapMarketForSpot(markets banexg.MarketMap, spot *banexg.Market, swapType string) *banexg.Market {
	if spot == nil || !spot.Spot {
		return nil
	}
	swapSymbol := bybitSwapSymbolForSpot(spot)
	if swapSymbol == "" {
		return nil
	}
	swap, ok := markets[swapSymbol]
	if !ok {
		return nil
	}
	if !bybitSwapMarketMatchesType(swap, swapType) {
		return nil
	}
	return swap
}

func pickBybitSpotWithSwapPair(markets banexg.MarketMap, swapType string) (*banexg.Market, *banexg.Market) {
	for _, spot := range markets {
		swap := bybitSwapMarketForSpot(markets, spot, swapType)
		if swap != nil {
			return spot, swap
		}
	}
	return nil, nil
}

func requireFundingRateCur(t *testing.T, item *banexg.FundingRateCur, wantSymbol string) {
	t.Helper()
	if item == nil || item.Symbol == "" {
		t.Fatal("expected funding rate item")
	}
	if wantSymbol != "" && item.Symbol != wantSymbol {
		t.Fatalf("expected symbol %s, got %s", wantSymbol, item.Symbol)
	}
	// fundingRate can be 0.0 at times, so we only assert fields that should exist.
	if item.NextFundingTimestamp <= 0 {
		t.Fatalf("expected positive NextFundingTimestamp, got %d", item.NextFundingTimestamp)
	}
}

func fundingRateCurBySymbol(items []*banexg.FundingRateCur, symbol string) *banexg.FundingRateCur {
	for _, item := range items {
		if item != nil && item.Symbol == symbol {
			return item
		}
	}
	return nil
}

func sampleFundingRateSymbols(items []*banexg.FundingRateCur, limit int) []string {
	if len(items) == 0 || limit <= 0 {
		return nil
	}
	keys := make([]string, 0, len(items))
	for _, it := range items {
		if it != nil && it.Symbol != "" {
			keys = append(keys, it.Symbol)
		}
	}
	return limitSortedStrings(keys, limit)
}

func requireFundingRatesHasSymbol(t *testing.T, items []*banexg.FundingRateCur, symbol string) {
	t.Helper()
	if len(items) == 0 {
		t.Fatal("expected non-empty funding rates")
	}
	if symbol == "" {
		t.Fatal("expected non-empty symbol")
	}
	item := fundingRateCurBySymbol(items, symbol)
	if item == nil {
		t.Fatalf("expected %s funding rate in results (got %d items, sample: %v)", symbol, len(items), sampleFundingRateSymbols(items, 10))
	}
	requireFundingRateCur(t, item, symbol)
}

func requireFundingRatesOnlySymbols(t *testing.T, items []*banexg.FundingRateCur, wantSymbols []string) {
	t.Helper()
	if len(wantSymbols) == 0 {
		t.Fatal("expected non-empty wantSymbols")
	}
	want := make(map[string]struct{}, len(wantSymbols))
	for _, sym := range wantSymbols {
		if sym != "" {
			want[sym] = struct{}{}
		}
	}
	if len(items) == 0 {
		t.Fatal("expected non-empty funding rates")
	}
	got := make(map[string]struct{}, len(items))
	for _, item := range items {
		requireFundingRateCur(t, item, "")
		if _, ok := want[item.Symbol]; !ok {
			t.Fatalf("unexpected symbol %s in results (want=%v)", item.Symbol, wantSymbols)
		}
		got[item.Symbol] = struct{}{}
	}
	for _, sym := range wantSymbols {
		if _, ok := got[sym]; !ok {
			t.Fatalf("missing symbol %s in results (got %d items, sample: %v)", sym, len(items), sampleFundingRateSymbols(items, 10))
		}
	}
}

func fetchFundingRateHistoryOrFail(t *testing.T, exg *Bybit, symbol string, since int64, limit int, params map[string]interface{}) []*banexg.FundingRate {
	t.Helper()
	items, err := exg.FetchFundingRateHistory(symbol, since, limit, params)
	if err != nil {
		t.Fatalf("FetchFundingRateHistory failed: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected non-empty funding rate history")
	}
	for _, it := range items {
		if it == nil {
			t.Fatal("expected non-nil funding rate item")
		}
		if it.Symbol == "" {
			t.Fatal("expected non-empty symbol in funding rate history")
		}
		// Bybit funding history endpoint returns funding rate timestamp in ms.
		if it.Timestamp <= 0 {
			t.Fatalf("expected positive timestamp in funding rate history, got %d", it.Timestamp)
		}
	}
	return items
}

func pickBybitSwapMarketsByType(markets banexg.MarketMap, marketType string, limit int) []*banexg.Market {
	items := make([]*banexg.Market, 0, limit)
	for _, market := range markets {
		if !bybitSwapMarketMatchesType(market, marketType) {
			continue
		}
		items = append(items, market)
		if limit > 0 && len(items) >= limit {
			break
		}
	}
	return items
}

func pickBybitSpotWithSwapPairs(markets banexg.MarketMap, swapType string, limit int) ([]*banexg.Market, []*banexg.Market) {
	spots := make([]*banexg.Market, 0, limit)
	swaps := make([]*banexg.Market, 0, limit)
	for _, spot := range markets {
		swap := bybitSwapMarketForSpot(markets, spot, swapType)
		if swap == nil {
			continue
		}
		spots = append(spots, spot)
		swaps = append(swaps, swap)
		if limit > 0 && len(spots) >= limit {
			break
		}
	}
	return spots, swaps
}

func TestApi_FetchTickers(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketSpot)
	market := pickBybitMarketByType(markets, banexg.MarketSpot)
	if market == nil || market.Symbol == "" {
		t.Skip("no spot market found")
	}

	tickers, err := exg.FetchTickers(nil, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketSpot,
	})
	if err != nil {
		t.Fatalf("FetchTickers failed: %v", err)
	}
	if len(tickers) == 0 {
		t.Fatal("expected non-empty tickers")
	}
	if tickers[0].Symbol == "" {
		t.Fatal("ticker symbol is empty")
	}
	if !tickerHasSymbol(tickers, market.Symbol) {
		t.Fatalf("expected spot tickers to include %s", market.Symbol)
	}
}

func TestApi_FetchTickers_Spot_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketSpot)
	market := pickBybitMarketByType(markets, banexg.MarketSpot)
	if market == nil || market.Symbol == "" {
		t.Skip("no spot market found")
	}
	tickers, err := exg.FetchTickers([]string{market.Symbol}, nil)
	if err != nil {
		t.Fatalf("FetchTickers spot symbol failed: %v", err)
	}
	if len(tickers) == 0 {
		t.Fatal("expected non-empty tickers")
	}
	if !tickerHasSymbol(tickers, market.Symbol) {
		t.Fatalf("expected spot ticker for %s", market.Symbol)
	}
}

func TestApi_FetchTickers_Linear_All(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketLinear)
	market := pickBybitMarketByType(markets, banexg.MarketLinear)
	if market == nil || market.Symbol == "" {
		t.Skip("no linear market found")
	}
	tickers, err := exg.FetchTickers(nil, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
	if err != nil {
		t.Fatalf("FetchTickers linear failed: %v", err)
	}
	if len(tickers) == 0 {
		t.Fatal("expected non-empty tickers")
	}
	if !tickerHasSymbol(tickers, market.Symbol) {
		t.Fatalf("expected linear tickers to include %s", market.Symbol)
	}
}

func TestApi_FetchTickers_Linear_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketLinear)
	market := pickBybitMarketByType(markets, banexg.MarketLinear)
	if market == nil || market.Symbol == "" {
		t.Skip("no linear market found")
	}
	tickers, err := exg.FetchTickers([]string{market.Symbol}, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
	if err != nil {
		t.Fatalf("FetchTickers linear symbol failed: %v", err)
	}
	if len(tickers) == 0 {
		t.Fatal("expected non-empty tickers")
	}
	if !tickerHasSymbol(tickers, market.Symbol) {
		t.Fatalf("expected linear ticker for %s", market.Symbol)
	}
}

func TestApi_FetchTickers_Inverse_All(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketInverse)
	market := pickBybitMarketByType(markets, banexg.MarketInverse)
	if market == nil || market.Symbol == "" {
		t.Skip("no inverse market found")
	}
	tickers, err := exg.FetchTickers(nil, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketInverse,
	})
	if err != nil {
		t.Fatalf("FetchTickers inverse failed: %v", err)
	}
	if len(tickers) == 0 {
		t.Fatal("expected non-empty tickers")
	}
	if !tickerHasSymbol(tickers, market.Symbol) {
		t.Fatalf("expected inverse tickers to include %s", market.Symbol)
	}
}

func TestApi_FetchTickers_Inverse_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketInverse)
	market := pickBybitMarketByType(markets, banexg.MarketInverse)
	if market == nil || market.Symbol == "" {
		t.Skip("no inverse market found")
	}
	tickers, err := exg.FetchTickers([]string{market.Symbol}, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketInverse,
	})
	if err != nil {
		t.Fatalf("FetchTickers inverse symbol failed: %v", err)
	}
	if len(tickers) == 0 {
		t.Fatal("expected non-empty tickers")
	}
	if !tickerHasSymbol(tickers, market.Symbol) {
		t.Fatalf("expected inverse ticker for %s", market.Symbol)
	}
}

func TestApi_FetchTickers_Option_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketOption)
	market := pickBybitMarketByType(markets, banexg.MarketOption)
	if market == nil || market.Symbol == "" {
		t.Skip("no option market found")
	}
	tickers, err := exg.FetchTickers([]string{market.Symbol}, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketOption,
	})
	if err != nil {
		t.Fatalf("FetchTickers option symbol failed: %v", err)
	}
	if len(tickers) == 0 {
		t.Fatal("expected non-empty tickers")
	}
	if !tickerHasSymbol(tickers, market.Symbol) {
		t.Fatalf("expected option ticker for %s", market.Symbol)
	}
}

func TestApi_FetchTickers_Option_BaseCoin(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketOption)
	market := pickBybitMarketByType(markets, banexg.MarketOption)
	if market == nil || market.Symbol == "" || market.Base == "" {
		t.Skip("no option market found")
	}
	tickers, err := exg.FetchTickers(nil, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketOption,
		"baseCoin":         market.Base,
	})
	if err != nil {
		t.Fatalf("FetchTickers option baseCoin failed: %v", err)
	}
	if len(tickers) == 0 {
		t.Fatal("expected non-empty tickers")
	}
	if !tickerHasSymbol(tickers, market.Symbol) {
		t.Fatalf("expected option tickers to include %s", market.Symbol)
	}
}

func TestApi_FetchTickers_Option_BaseCoinExpDate(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketOption)
	market := pickBybitMarketByType(markets, banexg.MarketOption)
	if market == nil || market.Symbol == "" || market.Base == "" {
		t.Skip("no option market found")
	}
	expDate := bybitOptionExpDate(market)
	if expDate == "" {
		t.Skip("option market missing expDate")
	}
	tickers, err := exg.FetchTickers(nil, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketOption,
		"baseCoin":         market.Base,
		"expDate":          expDate,
	})
	if err != nil {
		t.Fatalf("FetchTickers option baseCoin expDate failed: %v", err)
	}
	if len(tickers) == 0 {
		t.Fatal("expected non-empty tickers")
	}
	if !tickerHasSymbol(tickers, market.Symbol) {
		t.Fatalf("expected option tickers to include %s", market.Symbol)
	}
}

func TestApi_FetchTickers_Option_MultiSymbols(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketOption)
	items := pickBybitMarketsByType(markets, banexg.MarketOption, 2)
	if len(items) < 2 {
		t.Skip("not enough option markets")
	}
	symbols := []string{items[0].Symbol, items[1].Symbol}
	tickers, err := exg.FetchTickers(symbols, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketOption,
	})
	if err != nil {
		t.Fatalf("FetchTickers option multi symbols failed: %v", err)
	}
	if len(tickers) == 0 {
		t.Fatal("expected non-empty tickers")
	}
	if !tickerHasSymbol(tickers, items[0].Symbol) || !tickerHasSymbol(tickers, items[1].Symbol) {
		t.Fatalf("expected option tickers to include %v", symbols)
	}
}

func TestApi_FetchTicker(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	ticker, err := exg.FetchTicker("BTC/USDT", nil)
	if err != nil {
		t.Fatalf("FetchTicker failed: %v", err)
	}
	if ticker.Symbol == "" {
		t.Fatal("ticker symbol is empty")
	}
}

func TestApi_FetchTicker_Linear(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	markets := loadBybitMarketsForType(t, exg, banexg.MarketLinear)
	market := pickBybitMarketByType(markets, banexg.MarketLinear)
	if market == nil || market.Symbol == "" {
		t.Skip("no linear market found")
	}
	ticker, err := exg.FetchTicker(market.Symbol, nil)
	if err != nil {
		t.Fatalf("FetchTicker linear failed: %v", err)
	}
	if ticker == nil || ticker.Symbol == "" {
		t.Fatal("ticker symbol is empty")
	}
	if ticker.Symbol != market.Symbol {
		t.Fatalf("unexpected ticker symbol: %s", ticker.Symbol)
	}
}

func TestApi_FetchTicker_Inverse(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	markets := loadBybitMarketsForType(t, exg, banexg.MarketInverse)
	market := pickBybitMarketByType(markets, banexg.MarketInverse)
	if market == nil || market.Symbol == "" {
		t.Skip("no inverse market found")
	}
	ticker, err := exg.FetchTicker(market.Symbol, nil)
	if err != nil {
		t.Fatalf("FetchTicker inverse failed: %v", err)
	}
	if ticker == nil || ticker.Symbol == "" {
		t.Fatal("ticker symbol is empty")
	}
	if ticker.Symbol != market.Symbol {
		t.Fatalf("unexpected ticker symbol: %s", ticker.Symbol)
	}
}

func TestApi_FetchTicker_Option(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	markets := loadBybitMarketsForType(t, exg, banexg.MarketOption)
	market := pickBybitMarketByType(markets, banexg.MarketOption)
	if market == nil || market.Symbol == "" {
		t.Skip("no option market found")
	}
	ticker, err := exg.FetchTicker(market.Symbol, nil)
	if err != nil {
		t.Fatalf("FetchTicker option failed: %v", err)
	}
	if ticker == nil || ticker.Symbol == "" {
		t.Fatal("ticker symbol is empty")
	}
	if ticker.Symbol != market.Symbol {
		t.Fatalf("unexpected ticker symbol: %s", ticker.Symbol)
	}
}

func TestApi_FetchTickerPrice(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	prices, err := exg.FetchTickerPrice("BTC/USDT", map[string]interface{}{
		banexg.ParamMarket: banexg.MarketSpot,
	})
	if err != nil {
		t.Fatalf("FetchTickerPrice failed: %v", err)
	}
	requirePriceMapHasSymbol(t, prices, "BTC/USDT")
}

func TestApi_FetchTickerPrice_Spot_All(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketSpot)
	market := pickBybitMarketByType(markets, banexg.MarketSpot)
	if market == nil || market.Symbol == "" {
		t.Skip("no spot market found")
	}
	prices, err := exg.FetchTickerPrice("", map[string]interface{}{
		banexg.ParamMarket: banexg.MarketSpot,
	})
	if err != nil {
		t.Fatalf("FetchTickerPrice spot all failed: %v", err)
	}
	requirePriceMapHasSymbol(t, prices, market.Symbol)
}

func TestApi_FetchTickerPrice_Linear_All(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketLinear)
	market := pickBybitMarketByType(markets, banexg.MarketLinear)
	if market == nil || market.Symbol == "" {
		t.Skip("no linear market found")
	}
	prices, err := exg.FetchTickerPrice("", map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
	if err != nil {
		t.Fatalf("FetchTickerPrice linear all failed: %v", err)
	}
	requirePriceMapHasSymbol(t, prices, market.Symbol)
}

func TestApi_FetchTickerPrice_Linear_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketLinear)
	market := pickBybitMarketByType(markets, banexg.MarketLinear)
	if market == nil || market.Symbol == "" {
		t.Skip("no linear market found")
	}
	prices, err := exg.FetchTickerPrice(market.Symbol, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
	if err != nil {
		t.Fatalf("FetchTickerPrice linear symbol failed: %v", err)
	}
	requirePriceMapHasSymbol(t, prices, market.Symbol)
}

func TestApi_FetchTickerPrice_Inverse_All(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketInverse)
	market := pickBybitMarketByType(markets, banexg.MarketInverse)
	if market == nil || market.Symbol == "" {
		t.Skip("no inverse market found")
	}
	prices, err := exg.FetchTickerPrice("", map[string]interface{}{
		banexg.ParamMarket: banexg.MarketInverse,
	})
	if err != nil {
		t.Fatalf("FetchTickerPrice inverse all failed: %v", err)
	}
	requirePriceMapHasSymbol(t, prices, market.Symbol)
}

func TestApi_FetchTickerPrice_Inverse_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketInverse)
	market := pickBybitMarketByType(markets, banexg.MarketInverse)
	if market == nil || market.Symbol == "" {
		t.Skip("no inverse market found")
	}
	prices, err := exg.FetchTickerPrice(market.Symbol, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketInverse,
	})
	if err != nil {
		t.Fatalf("FetchTickerPrice inverse symbol failed: %v", err)
	}
	requirePriceMapHasSymbol(t, prices, market.Symbol)
}

func TestApi_FetchTickerPrice_Option_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketOption)
	market := pickBybitMarketByType(markets, banexg.MarketOption)
	if market == nil || market.Symbol == "" {
		t.Skip("no option market found")
	}
	prices, err := exg.FetchTickerPrice(market.Symbol, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketOption,
	})
	if err != nil {
		t.Fatalf("FetchTickerPrice option symbol failed: %v", err)
	}
	requirePriceMapHasSymbol(t, prices, market.Symbol)
}

func TestApi_FetchTickerPrice_Option_BaseCoin(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketOption)
	market := pickBybitMarketByType(markets, banexg.MarketOption)
	if market == nil || market.Symbol == "" || market.Base == "" {
		t.Skip("no option market found")
	}
	prices, err := exg.FetchTickerPrice("", map[string]interface{}{
		banexg.ParamMarket: banexg.MarketOption,
		"baseCoin":         market.Base,
	})
	if err != nil {
		t.Fatalf("FetchTickerPrice option baseCoin failed: %v", err)
	}
	requirePriceMapHasSymbol(t, prices, market.Symbol)
}

func TestApi_FetchTickerPrice_Option_BaseCoinExpDate(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketOption)
	market := pickBybitMarketByType(markets, banexg.MarketOption)
	if market == nil || market.Symbol == "" || market.Base == "" {
		t.Skip("no option market found")
	}
	expDate := bybitOptionExpDate(market)
	if expDate == "" {
		t.Skip("option market missing expDate")
	}
	prices, err := exg.FetchTickerPrice("", map[string]interface{}{
		banexg.ParamMarket: banexg.MarketOption,
		"baseCoin":         market.Base,
		"expDate":          expDate,
	})
	if err != nil {
		t.Fatalf("FetchTickerPrice option baseCoin expDate failed: %v", err)
	}
	requirePriceMapHasSymbol(t, prices, market.Symbol)
}

func TestApi_FetchLastPrices_Spot_All(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketSpot)
	market := pickBybitMarketByType(markets, banexg.MarketSpot)
	if market == nil || market.Symbol == "" {
		t.Skip("no spot market found")
	}

	items, err := exg.FetchLastPrices(nil, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketSpot,
	})
	if err != nil {
		t.Fatalf("FetchLastPrices spot all failed: %v", err)
	}
	requireLastPricesHasSymbol(t, items, market.Symbol)
}

func TestApi_FetchLastPrices_Spot_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketSpot)
	market := pickBybitMarketByType(markets, banexg.MarketSpot)
	if market == nil || market.Symbol == "" {
		t.Skip("no spot market found")
	}

	items, err := exg.FetchLastPrices([]string{market.Symbol}, nil)
	if err != nil {
		t.Fatalf("FetchLastPrices spot symbol failed: %v", err)
	}
	requireLastPricesHasSymbol(t, items, market.Symbol)
}

func TestApi_FetchLastPrices_Spot_MultiSymbols(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketSpot)
	items := pickBybitMarketsByType(markets, banexg.MarketSpot, 2)
	if len(items) < 2 {
		t.Skip("not enough spot markets")
	}
	symbols := []string{items[0].Symbol, items[1].Symbol}

	prices, err := exg.FetchLastPrices(symbols, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketSpot,
	})
	if err != nil {
		t.Fatalf("FetchLastPrices spot multi symbols failed: %v", err)
	}
	if len(prices) != len(symbols) {
		t.Fatalf("expected %d last prices, got %d", len(symbols), len(prices))
	}
	requireLastPricesHasSymbol(t, prices, items[0].Symbol)
	requireLastPricesHasSymbol(t, prices, items[1].Symbol)
}

func TestApi_FetchLastPrices_Linear_All(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketLinear)
	market := pickBybitMarketByType(markets, banexg.MarketLinear)
	if market == nil || market.Symbol == "" {
		t.Skip("no linear market found")
	}

	items, err := exg.FetchLastPrices(nil, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
	if err != nil {
		t.Fatalf("FetchLastPrices linear all failed: %v", err)
	}
	requireLastPricesHasSymbol(t, items, market.Symbol)
}

func TestApi_FetchLastPrices_Linear_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketLinear)
	market := pickBybitMarketByType(markets, banexg.MarketLinear)
	if market == nil || market.Symbol == "" {
		t.Skip("no linear market found")
	}

	items, err := exg.FetchLastPrices([]string{market.Symbol}, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
	if err != nil {
		t.Fatalf("FetchLastPrices linear symbol failed: %v", err)
	}
	requireLastPricesHasSymbol(t, items, market.Symbol)
}

func TestApi_FetchLastPrices_Linear_MultiSymbols(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketLinear)
	items := pickBybitMarketsByType(markets, banexg.MarketLinear, 2)
	if len(items) < 2 {
		t.Skip("not enough linear markets")
	}
	symbols := []string{items[0].Symbol, items[1].Symbol}

	prices, err := exg.FetchLastPrices(symbols, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
	if err != nil {
		t.Fatalf("FetchLastPrices linear multi symbols failed: %v", err)
	}
	if len(prices) != len(symbols) {
		t.Fatalf("expected %d last prices, got %d", len(symbols), len(prices))
	}
	requireLastPricesHasSymbol(t, prices, items[0].Symbol)
	requireLastPricesHasSymbol(t, prices, items[1].Symbol)
}

func TestApi_FetchLastPrices_Inverse_All(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketInverse)
	market := pickBybitMarketByType(markets, banexg.MarketInverse)
	if market == nil || market.Symbol == "" {
		t.Skip("no inverse market found")
	}

	items, err := exg.FetchLastPrices(nil, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketInverse,
	})
	if err != nil {
		t.Fatalf("FetchLastPrices inverse all failed: %v", err)
	}
	requireLastPricesHasSymbol(t, items, market.Symbol)
}

func TestApi_FetchLastPrices_Inverse_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketInverse)
	market := pickBybitMarketByType(markets, banexg.MarketInverse)
	if market == nil || market.Symbol == "" {
		t.Skip("no inverse market found")
	}

	items, err := exg.FetchLastPrices([]string{market.Symbol}, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketInverse,
	})
	if err != nil {
		t.Fatalf("FetchLastPrices inverse symbol failed: %v", err)
	}
	requireLastPricesHasSymbol(t, items, market.Symbol)
}

func TestApi_FetchLastPrices_Inverse_MultiSymbols(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketInverse)
	items := pickBybitMarketsByType(markets, banexg.MarketInverse, 2)
	if len(items) < 2 {
		t.Skip("not enough inverse markets")
	}
	symbols := []string{items[0].Symbol, items[1].Symbol}

	prices, err := exg.FetchLastPrices(symbols, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketInverse,
	})
	if err != nil {
		t.Fatalf("FetchLastPrices inverse multi symbols failed: %v", err)
	}
	if len(prices) != len(symbols) {
		t.Fatalf("expected %d last prices, got %d", len(symbols), len(prices))
	}
	requireLastPricesHasSymbol(t, prices, items[0].Symbol)
	requireLastPricesHasSymbol(t, prices, items[1].Symbol)
}

func TestApi_FetchLastPrices_Option_Symbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketOption)
	market := pickBybitMarketByType(markets, banexg.MarketOption)
	if market == nil || market.Symbol == "" {
		t.Skip("no option market found")
	}

	items, err := exg.FetchLastPrices([]string{market.Symbol}, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketOption,
	})
	if err != nil {
		t.Fatalf("FetchLastPrices option symbol failed: %v", err)
	}
	requireLastPricesHasSymbol(t, items, market.Symbol)
}

func TestApi_FetchLastPrices_Option_BaseCoin(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketOption)
	market := pickBybitMarketByType(markets, banexg.MarketOption)
	if market == nil || market.Symbol == "" || market.Base == "" {
		t.Skip("no option market found")
	}

	items, err := exg.FetchLastPrices(nil, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketOption,
		"baseCoin":         market.Base,
	})
	if err != nil {
		t.Fatalf("FetchLastPrices option baseCoin failed: %v", err)
	}
	requireLastPricesHasSymbol(t, items, market.Symbol)
}

func TestApi_FetchLastPrices_Option_BaseCoinExpDate(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketOption)
	market := pickBybitMarketByType(markets, banexg.MarketOption)
	if market == nil || market.Symbol == "" || market.Base == "" {
		t.Skip("no option market found")
	}
	expDate := bybitOptionExpDate(market)
	if expDate == "" {
		t.Skip("option market missing expDate")
	}

	items, err := exg.FetchLastPrices(nil, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketOption,
		"baseCoin":         market.Base,
		"expDate":          expDate,
	})
	if err != nil {
		t.Fatalf("FetchLastPrices option baseCoin expDate failed: %v", err)
	}
	requireLastPricesHasSymbol(t, items, market.Symbol)
}

func TestApi_FetchLastPrices_Option_MultiSymbols(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketOption)
	items := pickBybitMarketsByType(markets, banexg.MarketOption, 2)
	if len(items) < 2 {
		t.Skip("not enough option markets")
	}
	symbols := []string{items[0].Symbol, items[1].Symbol}

	prices, err := exg.FetchLastPrices(symbols, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketOption,
	})
	if err != nil {
		t.Fatalf("FetchLastPrices option multi symbols failed: %v", err)
	}
	if len(prices) != len(symbols) {
		t.Fatalf("expected %d last prices, got %d", len(symbols), len(prices))
	}
	requireLastPricesHasSymbol(t, prices, items[0].Symbol)
	requireLastPricesHasSymbol(t, prices, items[1].Symbol)
}

func requireBybitOrderBookNonEmpty(t *testing.T, book *banexg.OrderBook) {
	t.Helper()
	if book == nil || book.Asks == nil || book.Bids == nil {
		t.Fatal("expected orderbook data")
	}
	if len(book.Asks.Price) == 0 || len(book.Bids.Price) == 0 {
		t.Fatal("orderbook has empty sides")
	}
}

func scoreBybitOrderBookMarket(m *banexg.Market, marketType string) int {
	if m == nil || m.Symbol == "" {
		return -1
	}
	if !bybitMarketMatchesType(m, marketType) {
		return -1
	}

	score := 0
	// Prefer continuous markets where orderbooks are usually active.
	if (marketType == banexg.MarketLinear || marketType == banexg.MarketInverse) && m.Swap {
		score += 50
	}
	// Prefer BTC pairs, then ETH pairs.
	if strings.HasPrefix(m.Symbol, "BTC/") {
		score += 40
	} else if strings.HasPrefix(m.Symbol, "ETH/") {
		score += 20
	}
	// Prefer quote/settle in USDT/USD.
	if strings.Contains(m.Symbol, "/USDT") {
		score += 15
	}
	if strings.Contains(m.Symbol, "/USD") {
		score += 10
	}
	// Options often have many strikes/expiries; prefer BTC options by base.
	if marketType == banexg.MarketOption && strings.HasPrefix(m.ID, "BTC-") {
		score += 25
	}
	// Avoid expiring futures unless we're explicitly testing options.
	if (marketType == banexg.MarketSpot || marketType == banexg.MarketLinear || marketType == banexg.MarketInverse) && (m.Future || m.Option) {
		score -= 30
	}
	return score
}

func pickBybitOrderBookMarkets(markets banexg.MarketMap, marketType string, limit int) []*banexg.Market {
	items := make([]*banexg.Market, 0, len(markets))
	for _, m := range markets {
		if scoreBybitOrderBookMarket(m, marketType) >= 0 {
			items = append(items, m)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		si := scoreBybitOrderBookMarket(items[i], marketType)
		sj := scoreBybitOrderBookMarket(items[j], marketType)
		if si == sj {
			return items[i].Symbol < items[j].Symbol
		}
		return si > sj
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items
}

func bybitFetchOrderBookByType(t *testing.T, marketType string, limit int) (*banexg.Market, *banexg.OrderBook) {
	t.Helper()
	exg := getBybitAuthed(t, nil)
	if exg == nil {
		t.Skip("bybit exchange not initialized")
		return nil, nil
	}

	markets := loadBybitMarketsForType(t, exg, marketType)
	candidates := pickBybitOrderBookMarkets(markets, marketType, 8)
	if len(candidates) == 0 {
		t.Skipf("no %s markets found", marketType)
		return nil, nil
	}

	var lastErr error
	for _, m := range candidates {
		book, err := exg.FetchOrderBook(m.Symbol, limit, map[string]interface{}{
			banexg.ParamMarket: marketType,
		})
		if err != nil {
			lastErr = err
			continue
		}
		if book != nil && len(book.Asks.Price) > 0 && len(book.Bids.Price) > 0 {
			if book.Symbol != m.Symbol {
				t.Fatalf("unexpected orderbook symbol: got %s want %s", book.Symbol, m.Symbol)
			}
			return m, book
		}
		lastErr = err
	}
	if lastErr != nil {
		t.Fatalf("FetchOrderBook failed for %s candidates: %v", marketType, lastErr)
	}
	t.Fatalf("FetchOrderBook returned empty orderbook for %s candidates", marketType)
	return nil, nil
}

func requireBybitOrderBookLimit(t *testing.T, book *banexg.OrderBook, want int) {
	t.Helper()
	if book == nil {
		t.Fatal("expected orderbook data")
	}
	if book.Limit != want {
		t.Fatalf("unexpected orderbook limit: got %d want %d", book.Limit, want)
	}
}

func TestApi_FetchOrderBook(t *testing.T) {
	_, book := bybitFetchOrderBookByType(t, banexg.MarketSpot, 5)
	requireBybitOrderBookNonEmpty(t, book)
	requireBybitOrderBookLimit(t, book, 5)
}

func TestApi_FetchOrderBook_Spot_DefaultLimit(t *testing.T) {
	_, book := bybitFetchOrderBookByType(t, banexg.MarketSpot, 0)
	requireBybitOrderBookNonEmpty(t, book)
	requireBybitOrderBookLimit(t, book, 0)
}

func TestApi_FetchOrderBook_Spot_LimitClamp(t *testing.T) {
	_, book := bybitFetchOrderBookByType(t, banexg.MarketSpot, 999)
	requireBybitOrderBookNonEmpty(t, book)
	requireBybitOrderBookLimit(t, book, 200)
}

func TestApi_FetchOrderBook_Linear_DefaultLimit(t *testing.T) {
	_, book := bybitFetchOrderBookByType(t, banexg.MarketLinear, 0)
	requireBybitOrderBookNonEmpty(t, book)
	requireBybitOrderBookLimit(t, book, 0)
}

func TestApi_FetchOrderBook_Linear_LimitClamp(t *testing.T) {
	_, book := bybitFetchOrderBookByType(t, banexg.MarketLinear, 999)
	requireBybitOrderBookNonEmpty(t, book)
	requireBybitOrderBookLimit(t, book, 500)
}

func TestApi_FetchOrderBook_Inverse_DefaultLimit(t *testing.T) {
	_, book := bybitFetchOrderBookByType(t, banexg.MarketInverse, 0)
	requireBybitOrderBookNonEmpty(t, book)
	requireBybitOrderBookLimit(t, book, 0)
}

func TestApi_FetchOrderBook_Inverse_LimitClamp(t *testing.T) {
	_, book := bybitFetchOrderBookByType(t, banexg.MarketInverse, 999)
	requireBybitOrderBookNonEmpty(t, book)
	requireBybitOrderBookLimit(t, book, 500)
}

func TestApi_FetchOrderBook_Option_DefaultLimit(t *testing.T) {
	_, book := bybitFetchOrderBookByType(t, banexg.MarketOption, 0)
	requireBybitOrderBookNonEmpty(t, book)
	requireBybitOrderBookLimit(t, book, 0)
}

func TestApi_FetchOrderBook_Option_LimitClamp(t *testing.T) {
	_, book := bybitFetchOrderBookByType(t, banexg.MarketOption, 999)
	requireBybitOrderBookNonEmpty(t, book)
	requireBybitOrderBookLimit(t, book, 25)
}

func bybitOHLCVTestWindow1m() (int64, int64) {
	// Keep the window fairly recent so testnet/prod both have data.
	// We avoid "now" to reduce flakiness for the last in-progress candle.
	now := time.Now().UTC().Truncate(time.Minute)
	until := now.Add(-5 * time.Minute).UnixMilli()
	since := now.Add(-65 * time.Minute).UnixMilli()
	if since >= until {
		since = until - 60*60*1000
	}
	return since, until
}

func scoreBybitOHLCVMarket(m *banexg.Market, marketType string) int {
	if m == nil || m.Symbol == "" {
		return -1
	}
	score := 0
	// Prefer perpetual swaps for contract klines (more likely to have continuous data).
	if marketType == banexg.MarketLinear || marketType == banexg.MarketInverse {
		if m.Swap {
			score += 50
		}
	}
	// Prefer BTC pairs, then USDT/USD.
	if strings.HasPrefix(m.Symbol, "BTC/") {
		score += 40
	} else if strings.HasPrefix(m.Symbol, "ETH/") {
		score += 20
	}
	if strings.Contains(m.Symbol, "/USDT") {
		score += 15
	}
	if strings.Contains(m.Symbol, "/USD") {
		score += 10
	}
	// Avoid expiring futures/options for these basic API tests.
	if m.Future || m.Option {
		score -= 30
	}
	return score
}

func pickBybitMarketForOHLCV(t *testing.T, markets banexg.MarketMap, marketType string) *banexg.Market {
	t.Helper()
	if marketType != banexg.MarketSpot && marketType != banexg.MarketLinear && marketType != banexg.MarketInverse {
		t.Skipf("no suitable market found for %s", marketType)
	}
	var best *banexg.Market
	bestScore := -1
	for _, m := range markets {
		if !bybitMarketMatchesType(m, marketType) {
			continue
		}
		sc := scoreBybitOHLCVMarket(m, marketType)
		if sc > bestScore {
			bestScore = sc
			best = m
		}
	}
	if best == nil || best.Symbol == "" {
		t.Skipf("no suitable market found for %s", marketType)
	}
	return best
}

func runBybitFetchOHLCVCombo(t *testing.T, exg *Bybit, market *banexg.Market, timeframe string, since int64, until int64, price string) {
	t.Helper()
	var params map[string]interface{}
	if until > 0 || price != "" {
		params = map[string]interface{}{}
		if until > 0 {
			params[banexg.ParamUntil] = until
		}
		if price != "" {
			params["price"] = price
		}
	}
	klines, err := exg.FetchOHLCV(market.Symbol, timeframe, since, 2, params)
	if err != nil {
		t.Fatalf("FetchOHLCV failed: symbol=%s price=%s since=%d until=%d err=%v", market.Symbol, price, since, until, err)
	}
	if len(klines) == 0 {
		t.Fatalf("expected non-empty klines: symbol=%s price=%s since=%d until=%d", market.Symbol, price, since, until)
	}
	if len(klines) >= 2 && klines[0].Time > klines[1].Time {
		t.Fatalf("klines not in ascending order: %d > %d", klines[0].Time, klines[1].Time)
	}
	if since > 0 && klines[0].Time < since {
		t.Fatalf("expected first kline time >= since: got=%d since=%d", klines[0].Time, since)
	}
	if until > 0 && klines[len(klines)-1].Time > until {
		t.Fatalf("expected last kline time <= until: got=%d until=%d", klines[len(klines)-1].Time, until)
	}
}

type bybitOHLCVTimeCombo struct {
	name  string
	since int64
	until int64
}

func bybitOHLCVTimeCombos(since int64, until int64) []bybitOHLCVTimeCombo {
	return []bybitOHLCVTimeCombo{
		{name: "default", since: 0, until: 0},
		{name: "since", since: since, until: 0},
		{name: "until", since: 0, until: until},
		{name: "since_until", since: since, until: until},
	}
}

func testBybitFetchOHLCVParamCombos(t *testing.T, marketType string, price string) {
	t.Helper()
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, marketType)
	market := pickBybitMarketForOHLCV(t, markets, marketType)

	since, until := bybitOHLCVTestWindow1m()
	for _, tc := range bybitOHLCVTimeCombos(since, until) {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runBybitFetchOHLCVCombo(t, exg, market, "1m", tc.since, tc.until, price)
		})
	}
}

func TestApi_FetchOHLCV_Spot_ParamCombos(t *testing.T) {
	testBybitFetchOHLCVParamCombos(t, banexg.MarketSpot, "")
}

func TestApi_FetchOHLCV_Linear_Trade_ParamCombos(t *testing.T) {
	testBybitFetchOHLCVParamCombos(t, banexg.MarketLinear, "")
}

func TestApi_FetchOHLCV_Linear_MarkPrice_ParamCombos(t *testing.T) {
	testBybitFetchOHLCVParamCombos(t, banexg.MarketLinear, "mark")
}

func TestApi_FetchOHLCV_Linear_IndexPrice_ParamCombos(t *testing.T) {
	testBybitFetchOHLCVParamCombos(t, banexg.MarketLinear, "index")
}

func TestApi_FetchOHLCV_Linear_PremiumIndexPrice_ParamCombos(t *testing.T) {
	testBybitFetchOHLCVParamCombos(t, banexg.MarketLinear, "premiumIndex")
}

func TestApi_FetchOHLCV_Inverse_Trade_ParamCombos(t *testing.T) {
	testBybitFetchOHLCVParamCombos(t, banexg.MarketInverse, "")
}

func TestApi_FetchOHLCV_Inverse_MarkPrice_ParamCombos(t *testing.T) {
	testBybitFetchOHLCVParamCombos(t, banexg.MarketInverse, "mark")
}

func TestApi_FetchOHLCV_Inverse_IndexPrice_ParamCombos(t *testing.T) {
	testBybitFetchOHLCVParamCombos(t, banexg.MarketInverse, "index")
}

func TestApi_FetchFundingRate(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketLinear)
	market := pickBybitSwapMarketByType(markets, banexg.MarketLinear)
	if market == nil || market.Symbol == "" {
		t.Skip("no linear swap market found")
	}

	item, err := exg.FetchFundingRate(market.Symbol, nil)
	if err != nil {
		t.Fatalf("FetchFundingRate failed: %v", err)
	}
	requireFundingRateCur(t, item, market.Symbol)
}

func TestApi_FetchFundingRate_Linear_SwapSymbol_WithMarketParam(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketLinear)
	market := pickBybitSwapMarketByType(markets, banexg.MarketLinear)
	if market == nil || market.Symbol == "" {
		t.Skip("no linear swap market found")
	}

	item, err := exg.FetchFundingRate(market.Symbol, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
	if err != nil {
		t.Fatalf("FetchFundingRate linear swap symbol with ParamMarket failed: %v", err)
	}
	requireFundingRateCur(t, item, market.Symbol)
}

func TestApi_FetchFundingRate_Linear_MarketId_WithMarketParam(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketLinear)
	market := pickBybitSwapMarketByType(markets, banexg.MarketLinear)
	if market == nil || market.ID == "" || market.Symbol == "" {
		t.Skip("no linear swap market found")
	}

	item, err := exg.FetchFundingRate(market.ID, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
	if err != nil {
		t.Fatalf("FetchFundingRate linear market id with ParamMarket failed: %v", err)
	}
	requireFundingRateCur(t, item, market.Symbol)
}

func TestApi_FetchFundingRate_Linear_SpotSymbol_WithMarketParam(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForTypes(t, exg, banexg.MarketSpot, banexg.MarketLinear)
	spot, swap := pickBybitSpotWithSwapPair(markets, banexg.MarketLinear)
	if spot == nil || swap == nil {
		t.Skip("no spot market with matching linear swap market found")
	}

	item, err := exg.FetchFundingRate(spot.Symbol, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
	if err != nil {
		t.Fatalf("FetchFundingRate spot symbol with ParamMarket failed: %v", err)
	}
	requireFundingRateCur(t, item, swap.Symbol)
}

func TestApi_FetchFundingRate_Inverse_SwapSymbol_NoParams(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketInverse)
	market := pickBybitSwapMarketByType(markets, banexg.MarketInverse)
	if market == nil || market.Symbol == "" {
		t.Skip("no inverse swap market found")
	}

	item, err := exg.FetchFundingRate(market.Symbol, nil)
	if err != nil {
		t.Fatalf("FetchFundingRate inverse swap symbol failed: %v", err)
	}
	requireFundingRateCur(t, item, market.Symbol)
}

func TestApi_FetchFundingRate_Inverse_MarketId_WithMarketParam(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForType(t, exg, banexg.MarketInverse)
	market := pickBybitSwapMarketByType(markets, banexg.MarketInverse)
	if market == nil || market.ID == "" || market.Symbol == "" {
		t.Skip("no inverse swap market found")
	}

	item, err := exg.FetchFundingRate(market.ID, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketInverse,
	})
	if err != nil {
		t.Fatalf("FetchFundingRate inverse market id with ParamMarket failed: %v", err)
	}
	requireFundingRateCur(t, item, market.Symbol)
}

func TestApi_FetchFundingRates_EmptySymbols_WithMarketParam(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	for _, marketType := range []string{banexg.MarketLinear, banexg.MarketInverse} {
		marketType := marketType
		t.Run(marketType, func(t *testing.T) {
			markets := loadBybitMarketsForType(t, exg, marketType)
			sample := pickBybitSwapMarketByType(markets, marketType)
			if sample == nil || sample.Symbol == "" {
				t.Skipf("no %s swap market found", marketType)
			}
			items, err := exg.FetchFundingRates(nil, map[string]interface{}{
				banexg.ParamMarket: marketType,
			})
			if err != nil {
				t.Fatalf("FetchFundingRates empty symbols with ParamMarket failed: %v", err)
			}
			requireFundingRatesHasSymbol(t, items, sample.Symbol)
		})
	}
}

func TestApi_FetchFundingRates_OneSymbol_SwapSymbol(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	for _, marketType := range []string{banexg.MarketLinear, banexg.MarketInverse} {
		marketType := marketType
		t.Run(marketType, func(t *testing.T) {
			markets := loadBybitMarketsForType(t, exg, marketType)
			market := pickBybitSwapMarketByType(markets, marketType)
			if market == nil || market.Symbol == "" {
				t.Skipf("no %s swap market found", marketType)
			}
			items, err := exg.FetchFundingRates([]string{market.Symbol}, nil)
			if err != nil {
				t.Fatalf("FetchFundingRates one symbol failed: %v", err)
			}
			requireFundingRatesOnlySymbols(t, items, []string{market.Symbol})
		})
	}
}

func TestApi_FetchFundingRates_OneSymbol_SpotSymbol_WithMarketParam(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForTypes(t, exg, banexg.MarketSpot, banexg.MarketLinear)

	spot, swap := pickBybitSpotWithSwapPair(markets, banexg.MarketLinear)
	if spot == nil || swap == nil {
		t.Skip("no spot market with matching linear swap market found")
	}
	items, err := exg.FetchFundingRates([]string{spot.Symbol}, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
	if err != nil {
		t.Fatalf("FetchFundingRates one spot symbol with ParamMarket failed: %v", err)
	}
	requireFundingRatesOnlySymbols(t, items, []string{swap.Symbol})
}

func TestApi_FetchFundingRates_Multi_SwapSymbols_NoParams(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	for _, marketType := range []string{banexg.MarketLinear, banexg.MarketInverse} {
		marketType := marketType
		t.Run(marketType, func(t *testing.T) {
			markets := loadBybitMarketsForType(t, exg, marketType)
			picks := pickBybitSwapMarketsByType(markets, marketType, 2)
			if len(picks) < 2 {
				t.Skipf("need >=2 %s swap markets", marketType)
			}
			want := []string{picks[0].Symbol, picks[1].Symbol}
			items, err := exg.FetchFundingRates(want, nil)
			if err != nil {
				t.Fatalf("FetchFundingRates multi symbols failed: %v", err)
			}
			requireFundingRatesOnlySymbols(t, items, want)
		})
	}
}

func TestApi_FetchFundingRates_Multi_SwapSymbols_WithMarketParam(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	for _, marketType := range []string{banexg.MarketLinear, banexg.MarketInverse} {
		marketType := marketType
		t.Run(marketType, func(t *testing.T) {
			markets := loadBybitMarketsForType(t, exg, marketType)
			picks := pickBybitSwapMarketsByType(markets, marketType, 2)
			if len(picks) < 2 {
				t.Skipf("need >=2 %s swap markets", marketType)
			}
			want := []string{picks[0].Symbol, picks[1].Symbol}
			items, err := exg.FetchFundingRates(want, map[string]interface{}{
				banexg.ParamMarket: marketType,
			})
			if err != nil {
				t.Fatalf("FetchFundingRates multi symbols with ParamMarket failed: %v", err)
			}
			requireFundingRatesOnlySymbols(t, items, want)
		})
	}
}

func TestApi_FetchFundingRates_Multi_SpotSymbols_WithMarketParam(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	markets := loadBybitMarketsForTypes(t, exg, banexg.MarketSpot, banexg.MarketLinear)

	spots, swaps := pickBybitSpotWithSwapPairs(markets, banexg.MarketLinear, 2)
	if len(spots) < 2 || len(swaps) < 2 {
		t.Skip("need >=2 spot markets with matching linear swap markets")
	}
	items, err := exg.FetchFundingRates([]string{spots[0].Symbol, spots[1].Symbol}, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
	if err != nil {
		t.Fatalf("FetchFundingRates multi spot symbols with ParamMarket failed: %v", err)
	}
	requireFundingRatesOnlySymbols(t, items, []string{swaps[0].Symbol, swaps[1].Symbol})
}

func TestApi_FetchFundingRateHistory(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	fetchFundingRateHistoryOrFail(t, exg, "BTC/USDT:USDT", 0, 5, nil)
}

func TestApi_FetchFundingRateHistoryWithUntil(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	until := time.Now().UTC().UnixMilli()
	fetchFundingRateHistoryOrFail(t, exg, "BTC/USDT:USDT", 0, 5, map[string]interface{}{
		banexg.ParamUntil: until,
	})
}

func TestApi_FetchFundingRateHistoryWithSinceNoUntil(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	// When only startTime is provided, Bybit returns an error. banexg/bybit will
	// auto-fill endTime based on the instrument's fundingInterval.
	now := time.Now().UTC()
	since := now.Add(-40 * time.Hour).UnixMilli()
	fetchFundingRateHistoryOrFail(t, exg, "BTC/USDT:USDT", since, 5, nil)
}

func TestApi_FetchFundingRateHistoryWithSince(t *testing.T) {
	exg := getBybitAuthed(t, nil)

	now := time.Now().UTC()
	since := now.Add(-8 * time.Hour).UnixMilli()
	until := now.UnixMilli()
	fetchFundingRateHistoryOrFail(t, exg, "BTC/USDT:USDT", since, 5, map[string]interface{}{
		banexg.ParamUntil: until,
	})
}
