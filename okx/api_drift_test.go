package okx

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/banbox/banexg"
)

func newMockOKX(t *testing.T, handler http.HandlerFunc, methods ...string) (*OKX, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	exg, err := New(map[string]interface{}{
		banexg.OptApiKey:    "key",
		banexg.OptApiSecret: "secret",
		banexg.OptPassword:  "pass",
	})
	if err != nil {
		server.Close()
		t.Fatalf("new okx: %v", err)
	}
	for _, method := range methods {
		api := exg.Apis[method]
		exg.Hosts.Prod[api.Host] = server.URL + "/api/v5"
		exg.Hosts.Test[api.Host] = server.URL + "/api/v5"
		api.Url = server.URL + "/api/v5/" + api.Path
	}
	t.Cleanup(server.Close)
	return exg, server
}

func TestFetchOrdersAlgoHistoryUsesOfficialQueriesAndFiltersLocally(t *testing.T) {
	var mu sync.Mutex
	queries := make([]url.Values, 0, len(okxAlgoOrderTypes)*len(okxAlgoHistoryStates))
	exg, _ := newMockOKX(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get(FldOrdType) == "" || q.Get("state") == "" {
			t.Errorf("missing required algo history query: %s", r.URL.RawQuery)
		}
		if q.Has(FldBegin) || q.Has(FldEnd) {
			t.Errorf("unsupported time query sent: %s", r.URL.RawQuery)
		}
		mu.Lock()
		queries = append(queries, q)
		mu.Unlock()

		data := "[]"
		if q.Get("state") == "effective" {
			switch q.Get(FldOrdType) {
			case "conditional":
				data = `[{"algoId":"old","instId":"BTC-USDT","ordType":"conditional","state":"effective","cTime":"100"},{"algoId":"keep","instId":"BTC-USDT","ordType":"conditional","state":"effective","cTime":"200"}]`
			case "oco":
				data = `[{"algoId":"keep","instId":"BTC-USDT","ordType":"oco","state":"effective","cTime":"200"}]`
			}
		}
		_, _ = fmt.Fprintf(w, `{"code":"0","msg":"","data":%s}`, data)
	}, MethodTradeGetOrdersAlgoHistory)
	seedMarket(exg, "BTC-USDT", "BTC/USDT", banexg.MarketSpot)

	orders, err := exg.FetchOrders("BTC/USDT", 150, 10, map[string]interface{}{
		banexg.ParamAlgoOrder: true,
		banexg.ParamUntil:     int64(250),
		banexg.ParamNoCache:   true,
	})
	if err != nil {
		t.Fatalf("fetch algo history: %v", err)
	}
	if len(queries) != len(okxAlgoOrderTypes)*len(okxAlgoHistoryStates) {
		t.Fatalf("unexpected query count: %d", len(queries))
	}
	if len(orders) != 1 || orders[0].ID != "algo:keep" {
		t.Fatalf("expected filtered deduplicated order, got %+v", orders)
	}
}

func TestFetchOpenOrdersAlgoFansOutAllCreatableTypes(t *testing.T) {
	var mu sync.Mutex
	seen := make([]string, 0, len(okxAlgoOrderTypes))
	exg, _ := newMockOKX(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.Query().Get(FldOrdType))
		mu.Unlock()
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[]}`))
	}, MethodTradeGetOrdersAlgoPending)
	seedMarket(exg, "BTC-USDT", "BTC/USDT", banexg.MarketSpot)

	_, err := exg.FetchOpenOrders("BTC/USDT", 0, 0, map[string]interface{}{
		banexg.ParamAlgoOrder: true,
		banexg.ParamNoCache:   true,
	})
	if err != nil {
		t.Fatalf("fetch open algo orders: %v", err)
	}
	sort.Strings(seen)
	want := append([]string(nil), okxAlgoOrderTypes...)
	sort.Strings(want)
	if fmt.Sprint(seen) != fmt.Sprint(want) {
		t.Fatalf("unexpected algo types: got %v want %v", seen, want)
	}
}

func TestFetchOpenOrdersFullSnapshotIncludesRegularAndAlgo(t *testing.T) {
	var mu sync.Mutex
	seen := make([]string, 0, len(okxAlgoOrderTypes)+1)
	exg, _ := newMockOKX(t, func(w http.ResponseWriter, r *http.Request) {
		ordType := r.URL.Query().Get(FldOrdType)
		mu.Lock()
		seen = append(seen, ordType)
		mu.Unlock()
		data := `[{"ordId":"regular","instId":"BTC-USDT","state":"live","ordType":"limit"}]`
		if ordType != "" {
			data = fmt.Sprintf(`[{"algoId":"%s","instId":"BTC-USDT","state":"live","ordType":"%s"}]`, ordType, ordType)
		}
		_, _ = fmt.Fprintf(w, `{"code":"0","msg":"","data":%s}`, data)
	}, MethodTradeGetOrdersPending, MethodTradeGetOrdersAlgoPending)
	seedMarket(exg, "BTC-USDT", "BTC/USDT", banexg.MarketSpot)

	orders, err := exg.FetchOpenOrders("BTC/USDT", 0, 1000, map[string]interface{}{
		banexg.ParamFullSnapshot: true,
		banexg.ParamNoCache:      true,
	})
	if err != nil {
		t.Fatalf("fetch full snapshot: %v", err)
	}
	if len(orders) != len(okxAlgoOrderTypes)+1 {
		t.Fatalf("orders = %d, want %d", len(orders), len(okxAlgoOrderTypes)+1)
	}
	if len(seen) != len(okxAlgoOrderTypes)+1 {
		t.Fatalf("queries = %v", seen)
	}
}

func TestFetchOpenOrdersFullSnapshotRejectsPartialAlgoResult(t *testing.T) {
	exg, _ := newMockOKX(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get(FldOrdType) == "trigger" {
			_, _ = w.Write([]byte(`{"code":"50000","msg":"failed","data":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[]}`))
	}, MethodTradeGetOrdersPending, MethodTradeGetOrdersAlgoPending)
	seedMarket(exg, "BTC-USDT", "BTC/USDT", banexg.MarketSpot)

	_, err := exg.FetchOpenOrders("BTC/USDT", 0, 1000, map[string]interface{}{
		banexg.ParamFullSnapshot: true,
		banexg.ParamNoCache:      true,
	})
	if err == nil {
		t.Fatal("partial algo snapshot was accepted")
	}
}

func TestFetchOpenOrdersFullSnapshotRejectsFullPage(t *testing.T) {
	exg, _ := newMockOKX(t, func(w http.ResponseWriter, _ *http.Request) {
		items := make([]string, 100)
		for i := range items {
			items[i] = fmt.Sprintf(`{"ordId":"%d","instId":"BTC-USDT","state":"live","ordType":"limit"}`, i)
		}
		_, _ = fmt.Fprintf(w, `{"code":"0","msg":"","data":[%s]}`, strings.Join(items, ","))
	}, MethodTradeGetOrdersPending, MethodTradeGetOrdersAlgoPending)
	seedMarket(exg, "BTC-USDT", "BTC/USDT", banexg.MarketSpot)

	_, err := exg.FetchOpenOrders("BTC/USDT", 0, 1000, map[string]interface{}{
		banexg.ParamFullSnapshot: true,
		banexg.ParamNoCache:      true,
	})
	if err == nil {
		t.Fatal("full OKX page was accepted as complete")
	}
}

func TestFetchFundingRatesSupportsFuturesXPerps(t *testing.T) {
	exg, _ := newMockOKX(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get(FldInstId) != InstIdAny || q.Has(banexg.ParamContract) {
			t.Errorf("unexpected funding query: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","fundingRate":"0.1"},{"instType":"FUTURES","instId":"BTC-USD-FUTURES","fundingRate":"0.2"}]}`))
	}, MethodPublicGetFundingRate)
	exg.Markets = banexg.MarketMap{}

	rates, err := exg.FetchFundingRates(nil, map[string]interface{}{
		banexg.ParamMarket:   banexg.MarketFuture,
		banexg.ParamContract: banexg.MarketFuture,
		banexg.ParamNoCache:  true,
	})
	if err != nil {
		t.Fatalf("fetch FUTURES funding rates: %v", err)
	}
	if len(rates) != 1 || rates[0].Symbol != "BTC-USD-FUTURES" || rates[0].FundingRate != 0.2 {
		t.Fatalf("unexpected FUTURES funding rates: %+v", rates)
	}
}

func TestFetchOHLCVClampsLimitTo300(t *testing.T) {
	exg, _ := newMockOKX(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get(FldLimit); got != "300" {
			t.Errorf("limit = %q, want 300", got)
		}
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[]}`))
	}, MethodMarketGetCandles)
	seedMarket(exg, "BTC-USDT", "BTC/USDT", banexg.MarketSpot)

	if _, err := exg.FetchOHLCV("BTC/USDT", "1m", 0, 500, map[string]interface{}{banexg.ParamNoCache: true}); err != nil {
		t.Fatalf("fetch OHLCV: %v", err)
	}
}
