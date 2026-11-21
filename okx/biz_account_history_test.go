package okx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/banbox/banexg"
)

func TestParseIncome(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	seedMarket(exg, "BTC-USDT-SWAP", "BTC/USDT:USDT", banexg.MarketLinear)
	bill := &Bill{
		InstType: "SWAP",
		InstId:   "BTC-USDT-SWAP",
		BillId:   "623950854533513219",
		Type:     "8",
		SubType:  "173",
		Ts:       "1695033476167",
		BalChg:   "0.021933823221",
		Ccy:      "USDT",
		TradeId:  "586760148",
	}
	inc := parseIncome(exg, bill, nil)
	if inc == nil {
		t.Fatalf("unexpected nil income")
	}
	if inc.Income != 0.021933823221 {
		t.Fatalf("unexpected income: %v", inc.Income)
	}
	if inc.IncomeType != "8" || inc.Asset != "USDT" {
		t.Fatalf("unexpected income fields: %+v", inc)
	}
	if inc.TranID != bill.BillId || inc.TradeID != bill.TradeId {
		t.Fatalf("unexpected ids: %+v", inc)
	}
	if inc.Time != 1695033476167 {
		t.Fatalf("unexpected time: %v", inc.Time)
	}
	if inc.Symbol == "" {
		t.Fatalf("unexpected empty symbol")
	}
}

func TestParseIncomeFallback(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	seedMarket(exg, "ETH-USDT-SWAP", "ETH/USDT:USDT", banexg.MarketLinear)
	bill := &Bill{
		InstType: "SWAP",
		InstId:   "ETH-USDT-SWAP",
		BillId:   "1",
		Type:     "2",
		Ts:       "1700000000000",
		Pnl:      "-1.5",
		Ccy:      "USDT",
	}
	inc := parseIncome(exg, bill, nil)
	if inc == nil {
		t.Fatalf("unexpected nil income")
	}
	if inc.Income != -1.5 {
		t.Fatalf("unexpected income fallback: %v", inc.Income)
	}
}

func TestParseFundingRate(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	seedMarket(exg, "BTC-USDT-SWAP", "BTC/USDT:USDT", banexg.MarketLinear)
	item := &FundingRate{
		InstId:          "BTC-USDT-SWAP",
		FundingRate:     "0.0001",
		FundingTime:     "1743609600000",
		NextFundingTime: "1743638400000",
		InterestRate:    "0.0003",
		Ts:              "1743588686291",
	}
	res := parseFundingRate(exg, item, nil)
	if res == nil {
		t.Fatalf("unexpected nil funding rate")
	}
	if res.FundingRate != 0.0001 || res.NextFundingRate != 0 {
		t.Fatalf("unexpected rates: %+v", res)
	}
	if res.FundingTimestamp != 1743609600000 || res.NextFundingTimestamp != 1743638400000 {
		t.Fatalf("unexpected timestamps: %+v", res)
	}
}

func TestParseFundingRateHistory(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	seedMarket(exg, "BTC-USDT-SWAP", "BTC/USDT:USDT", banexg.MarketLinear)
	item := &FundingRateHistory{
		InstId:      "BTC-USDT-SWAP",
		FundingRate: "0.0000746604960499",
		FundingTime: "1703059200000",
		Method:      "next_period",
	}
	res := parseFundingRateHistory(exg, item, map[string]interface{}{"realizedRate": "0.0000746572360545"})
	if res == nil {
		t.Fatalf("unexpected nil funding rate history")
	}
	if res.FundingRate != 0.0000746604960499 {
		t.Fatalf("unexpected funding rate: %+v", res)
	}
	if res.Timestamp != 1703059200000 {
		t.Fatalf("unexpected timestamp: %+v", res)
	}
	if res.Symbol != "BTC/USDT:USDT" {
		t.Fatalf("unexpected symbol: %s", res.Symbol)
	}
}

func TestShouldContinueFundRateHistory(t *testing.T) {
	list := []*banexg.FundingRate{
		{Timestamp: 300},
		{Timestamp: 200},
		{Timestamp: 100},
	}
	next, ok := shouldContinueFundRateHistory(list, 3, 0, 0)
	if !ok || next != 100 {
		t.Fatalf("expected continue with next=100, got %v %v", next, ok)
	}
	next, ok = shouldContinueFundRateHistory(list[:2], 3, 0, 0)
	if ok {
		t.Fatalf("expected stop when page not full, got %v %v", next, ok)
	}
	next, ok = shouldContinueFundRateHistory(list, 3, 150, 0)
	if ok {
		t.Fatalf("expected stop when reaching since bound, got %v %v", next, ok)
	}
	next, ok = shouldContinueFundRateHistory(list, 3, 0, 100)
	if ok {
		t.Fatalf("expected stop when no progress, got %v %v", next, ok)
	}
}

func TestFetchFundingRateHistoryRangePaginationAndAscendingOrder(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	seedMarket(exg, "BTC-USDT-SWAP", "BTC/USDT:USDT", banexg.MarketLinear)
	exg.Markets["BTC/USDT:USDT"].Swap = true

	const (
		since    = int64(1700000000000)
		interval = int64(8 * 3600 * 1000)
		total    = 130
	)
	until := since + (total-1)*interval
	server := newOKXFundingTestServer(t, since, interval, total)
	defer server.Close()
	exg.Hosts.Prod[HostPublic] = server.URL
	exg.HttpClient = server.Client()

	items, fetchErr := exg.FetchFundingRateHistory("BTC/USDT:USDT", since, 0, map[string]interface{}{
		banexg.ParamUntil: until,
	})
	if fetchErr != nil {
		t.Fatalf("FetchFundingRateHistory failed: %v", fetchErr)
	}
	if len(items) != total {
		t.Fatalf("expected %d items across pages, got %d", total, len(items))
	}
	if items[0].Timestamp != since || items[len(items)-1].Timestamp != until {
		t.Fatalf("inclusive bounds lost: [%d, %d] vs [%d, %d]", items[0].Timestamp, items[len(items)-1].Timestamp, since, until)
	}
	for i := 1; i < len(items); i++ {
		if items[i-1].Timestamp >= items[i].Timestamp {
			t.Fatalf("items not ascending at %d: %d >= %d", i, items[i-1].Timestamp, items[i].Timestamp)
		}
	}
}

func newOKXFundingTestServer(t *testing.T, since, interval int64, total int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		after, _ := strconv.ParseInt(q.Get("after"), 10, 64)
		before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit <= 0 || limit > 100 {
			limit = 100
		}
		data := make([]map[string]interface{}, 0, limit)
		for i := total - 1; i >= 0 && len(data) < limit; i-- {
			ts := since + int64(i)*interval
			if after > 0 && ts >= after {
				continue
			}
			if before > 0 && ts <= before {
				continue
			}
			data = append(data, map[string]interface{}{
				"instType":     "SWAP",
				"instId":       "BTC-USDT-SWAP",
				"fundingRate":  "0.0001",
				"realizedRate": "0.0001",
				"fundingTime":  strconv.FormatInt(ts, 10),
				"method":       "current_period",
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code": "0",
			"msg":  "",
			"data": data,
		})
	}))
}
