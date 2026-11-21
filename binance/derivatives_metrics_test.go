package binance

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/banbox/banexg"
)

func newMetricsTestExchange(t *testing.T, visionHandler, restHandler http.HandlerFunc) *Binance {
	t.Helper()
	var restSrv *httptest.Server
	if restHandler != nil {
		restSrv = httptest.NewServer(restHandler)
		t.Cleanup(restSrv.Close)
	}
	if visionHandler != nil {
		visionSrv := httptest.NewServer(visionHandler)
		t.Cleanup(visionSrv.Close)
		oldVisionURL := binanceVisionBaseURL
		binanceVisionBaseURL = visionSrv.URL
		t.Cleanup(func() { binanceVisionBaseURL = oldVisionURL })
	}

	e, err := New(map[string]interface{}{
		banexg.OptMarketType: banexg.MarketLinear,
	})
	if err != nil {
		t.Fatalf("New binance failed: %v", err)
	}
	if restSrv != nil {
		e.Hosts.Prod[HostFApiData] = restSrv.URL
		e.Hosts.Prod[HostDApiData] = restSrv.URL
		for _, code := range []string{
			MethodFapiDataGetOpenInterestHist,
			MethodFapiDataGetTopLongShortPositionRatio,
			MethodFapiDataGetTopLongShortAccountRatio,
			MethodFapiDataGetGlobalLongShortAccountRatio,
			MethodFapiDataGetTakerlongshortRatio,
		} {
			if entry := e.Apis[code]; entry != nil {
				entry.Url = restSrv.URL + "/" + entry.Path
			}
		}
	}
	market := &banexg.Market{
		ID:       "BTCUSDT",
		Symbol:   "BTC/USDT:USDT",
		Base:     "BTC",
		Quote:    "USDT",
		Settle:   "USDT",
		BaseID:   "BTC",
		QuoteID:  "USDT",
		SettleID: "USDT",
		Type:     banexg.MarketLinear,
		Linear:   true,
		Swap:     true,
		Contract: true,
		Active:   true,
	}
	e.Markets = banexg.MarketMap{market.Symbol: market}
	e.MarketsById = banexg.MarketArrMap{market.ID: []*banexg.Market{market}}
	return e
}

func TestFetchDerivativesMetricsArchiveParsesAndResamples5mTo1hLast(t *testing.T) {
	dayStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	const fiveMin = 5 * time.Minute
	// Build two full hours of 5m metrics (00:05..02:00 = 24 rows) plus one row with empty ratios.
	csvRows := []string{
		"create_time,symbol,sum_open_interest,sum_open_interest_value,count_toptrader_long_short_ratio,sum_toptrader_long_short_ratio,count_long_short_ratio,sum_taker_long_short_vol_ratio",
	}
	for i := 1; i <= 24; i++ {
		ts := dayStart.Add(time.Duration(i) * fiveMin)
		oi := 1000.0 + float64(i)
		oiVal := oi * 42000.0
		if i == 24 {
			// Verify missing/empty columns become NaN while closing OI is preserved!
			csvRows = append(csvRows, ts.Format("2006-01-02 15:04:05")+",BTCUSDT,"+
				strconv.FormatFloat(oi, 'f', 2, 64)+","+
				strconv.FormatFloat(oiVal, 'f', 2, 64)+",,,,")
			continue
		}
		csvRows = append(csvRows, ts.Format("2006-01-02 15:04:05")+",BTCUSDT,"+
			strconv.FormatFloat(oi, 'f', 2, 64)+","+
			strconv.FormatFloat(oiVal, 'f', 2, 64)+","+
			strconv.FormatFloat(1.1+float64(i)*0.01, 'f', 4, 64)+","+
			strconv.FormatFloat(1.2+float64(i)*0.01, 'f', 4, 64)+","+
			strconv.FormatFloat(0.9+float64(i)*0.01, 'f', 4, 64)+","+
			strconv.FormatFloat(0.8+float64(i)*0.01, 'f', 4, 64))
	}
	zipData := makeVisionZip(t, "BTCUSDT-metrics-2024-01-01.csv", csvRows)

	e := newMetricsTestExchange(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/futures/um/daily/metrics/BTCUSDT/BTCUSDT-metrics-2024-01-01.zip") {
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipData)
			return
		}
		http.NotFound(w, r)
	}, nil)

	startMS := dayStart.Add(time.Hour).UnixMilli()     // 01:00
	endMS := dayStart.Add(2 * time.Hour).UnixMilli()   // 02:00
	items, avail, err := e.FetchDerivativesMetricsArchive(context.Background(), "BTC/USDT:USDT", "1h", startMS, endMS)
	if err != nil || !avail {
		t.Fatalf("FetchDerivativesMetricsArchive failed: avail=%v err=%v", avail, err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 resampled 1h snapshots, got %d", len(items))
	}

	// Bucket 1 [00:00, 01:00]: last 5m row is i=12 (01:00:00), OI = 1012 (NOT sum!).
	if items[0].Timestamp != startMS || math.Abs(items[0].OpenInterest-1012.0) > 1e-9 {
		t.Fatalf("unexpected 01:00 snapshot: %+v", items[0])
	}
	if math.Abs(items[0].TopTraderLongShortPosRatio-1.32) > 1e-6 || math.Abs(items[0].TakerLongShortVolRatio-0.92) > 1e-6 {
		t.Fatalf("unexpected 01:00 ratios: %+v", items[0])
	}

	// Bucket 2 [01:00, 02:00]: last 5m row is i=24 (02:00:00), OI = 1024, ratios empty -> NaN!
	if items[1].Timestamp != endMS || math.Abs(items[1].OpenInterest-1024.0) > 1e-9 {
		t.Fatalf("unexpected 02:00 snapshot: %+v", items[1])
	}
	if !math.IsNaN(items[1].TopTraderLongShortPosRatio) || !math.IsNaN(items[1].GlobalLongShortAccountRatio) {
		t.Fatalf("expected missing CSV ratios at 02:00 to be NaN, got %+v", items[1])
	}
}

func TestFetchDerivativesMetricsStitchesVisionArchiveAndRecentRESTWithoutDupes(t *testing.T) {
	day1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	const fiveMin = 5 * time.Minute

	// Day 1 is in Vision (288 5m rows covering 2024-01-01 00:05 .. 2024-01-02 00:00).
	csvRows := []string{
		"create_time,symbol,sum_open_interest,sum_open_interest_value,count_toptrader_long_short_ratio,sum_toptrader_long_short_ratio,count_long_short_ratio,sum_taker_long_short_vol_ratio",
	}
	for i := 1; i <= 288; i++ {
		ts := day1.Add(time.Duration(i) * fiveMin)
		oi := 2000.0 + float64(i)
		csvRows = append(csvRows, ts.Format("2006-01-02 15:04:05")+",BTCUSDT,"+
			strconv.FormatFloat(oi, 'f', 2, 64)+","+
			strconv.FormatFloat(oi*40000.0, 'f', 2, 64)+",1.10,1.25,0.95,1.05")
	}
	day1Zip := makeVisionZip(t, "BTCUSDT-metrics-2024-01-01.csv", csvRows)

	visionHits := 0
	restOIHits := 0
	// REST serves an overlapping boundary point at 2024-01-02 00:00 plus 01:00 and 02:00 on Day 2.
	restStamps := []int64{
		day2.UnixMilli(),
		day2.Add(1 * time.Hour).UnixMilli(),
		day2.Add(2 * time.Hour).UnixMilli(),
	}

	e := newMetricsTestExchange(t,
		func(w http.ResponseWriter, r *http.Request) {
			visionHits++
			if strings.HasSuffix(r.URL.Path, "BTCUSDT-metrics-2024-01-01.zip") {
				w.Header().Set("Content-Type", "application/zip")
				_, _ = w.Write(day1Zip)
				return
			}
			// Day 2 (2024-01-02.zip) is 404 (not yet archived), triggering REST tail stitch!
			http.NotFound(w, r)
		},
		func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			reqStart, _ := strconv.ParseInt(q.Get("startTime"), 10, 64)
			reqEnd, _ := strconv.ParseInt(q.Get("endTime"), 10, 64)
			var rows []map[string]interface{}
			for idx, ts := range restStamps {
				if (reqStart > 0 && ts < reqStart) || (reqEnd > 0 && ts > reqEnd) {
					continue
				}
				switch {
				case strings.HasSuffix(r.URL.Path, "/openInterestHist"):
					restOIHits++
					oi := 5000.0 + float64(idx)
					rows = append(rows, map[string]interface{}{
						"symbol":               "BTCUSDT",
						"sumOpenInterest":      strconv.FormatFloat(oi, 'f', 2, 64),
						"sumOpenInterestValue": strconv.FormatFloat(oi*41000.0, 'f', 2, 64),
						"timestamp":            ts,
					})
				case strings.HasSuffix(r.URL.Path, "/topLongShortPositionRatio"):
					rows = append(rows, map[string]interface{}{
						"symbol":         "BTCUSDT",
						"longShortRatio": strconv.FormatFloat(1.50+float64(idx)*0.1, 'f', 4, 64),
						"timestamp":      strconv.FormatInt(ts, 10), // string timestamp variant
					})
				case strings.HasSuffix(r.URL.Path, "/topLongShortAccountRatio"):
					rows = append(rows, map[string]interface{}{
						"symbol":         "BTCUSDT",
						"longShortRatio": "1.35",
						"timestamp":      ts,
					})
				case strings.HasSuffix(r.URL.Path, "/globalLongShortAccountRatio"):
					rows = append(rows, map[string]interface{}{
						"symbol":         "BTCUSDT",
						"longShortRatio": "0.88",
						"timestamp":      ts,
					})
				case strings.HasSuffix(r.URL.Path, "/takerlongshortRatio"):
					rows = append(rows, map[string]interface{}{
						"symbol":       "BTCUSDT",
						"buySellRatio": "1.12",
						"timestamp":    ts,
					})
				}
			}
			_ = json.NewEncoder(w).Encode(rows)
		},
	)

	startMS := day1.Add(22 * time.Hour).UnixMilli() // 2024-01-01 22:00 -> 3 archive hours: 22:00, 23:00, 24:00
	endMS := day2.Add(2 * time.Hour).UnixMilli()    // 2024-01-02 02:00 -> +2 REST hours: 01:00, 02:00
	items, supported, err := e.FetchDerivativesMetrics(context.Background(), "BTC/USDT:USDT", "1h", startMS, endMS, 0, nil)
	if err != nil || !supported {
		t.Fatalf("FetchDerivativesMetrics failed: supported=%v err=%v", supported, err)
	}
	if visionHits == 0 || restOIHits == 0 {
		t.Fatalf("expected both Vision and REST to be used, got visionHits=%d restOIHits=%d", visionHits, restOIHits)
	}
	if len(items) != 5 {
		t.Fatalf("expected 5 stitched deduplicated hourly snapshots (22:00..02:00), got %d", len(items))
	}
	for i := 0; i < 5; i++ {
		wantTS := startMS + int64(i)*int64(time.Hour/time.Millisecond)
		if items[i].Timestamp != wantTS {
			t.Fatalf("item[%d].Timestamp = %d, want %d", i, items[i].Timestamp, wantTS)
		}
	}
	// 2024-01-02 00:00 (index 2) comes from Vision i=288 -> OI = 2288;
	// 2024-01-02 01:00 (index 3) comes from REST idx=1 -> OI = 5001, TopPosRatio = 1.60, TakerVolRatio = 1.12.
	if math.Abs(items[2].OpenInterest-2288.0) > 1e-9 {
		t.Fatalf("expected Day 1 24:00 OI=2288 from Vision, got %v", items[2].OpenInterest)
	}
	if math.Abs(items[3].OpenInterest-5001.0) > 1e-9 ||
		math.Abs(items[3].TopTraderLongShortPosRatio-1.60) > 1e-9 ||
		math.Abs(items[3].TakerLongShortVolRatio-1.12) > 1e-9 {
		t.Fatalf("unexpected REST stitched item[3]: %+v", items[3])
	}
}
