package binance

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
)

func newPriceKlineTestExchange(t *testing.T, visionHandler, restHandler http.HandlerFunc) *Binance {
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
		e.HttpClient = restSrv.Client()
		e.EnableRateLimit = banexg.BoolFalse
		e.Hosts.Prod[HostFApiPublic] = restSrv.URL
		e.Hosts.Prod[HostDApiPublic] = restSrv.URL
		e.Hosts.Prod[HostPublic] = restSrv.URL
	}
	linearSwap := &banexg.Market{
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
	}
	inverseSwap := &banexg.Market{
		ID:       "BTCUSD_PERP",
		Symbol:   "BTC/USD:BTC",
		Base:     "BTC",
		Quote:    "USD",
		Settle:   "BTC",
		BaseID:   "BTC",
		QuoteID:  "USD",
		SettleID: "BTC",
		Type:     banexg.MarketInverse,
		Inverse:  true,
		Swap:     true,
		Contract: true,
	}
	spotMarket := &banexg.Market{
		ID:     "SPOTONLYUSDT",
		Symbol: "SPOTONLY/USDT",
		Base:   "SPOTONLY",
		Quote:  "USDT",
		Type:   banexg.MarketSpot,
		Spot:   true,
	}
	deliveryFuture := &banexg.Market{
		ID:       "BTCUSDT_240329",
		Symbol:   "BTC/USDT:USDT-240329",
		BaseID:   "BTC",
		QuoteID:  "USDT",
		Type:     banexg.MarketLinear,
		Linear:   true,
		Future:   true,
		Contract: true,
	}
	e.Markets = banexg.MarketMap{
		linearSwap.Symbol:     linearSwap,
		inverseSwap.Symbol:    inverseSwap,
		spotMarket.Symbol:     spotMarket,
		deliveryFuture.Symbol: deliveryFuture,
	}
	return e
}

func TestFetchOHLCV_PremiumMarkAndIndexREST(t *testing.T) {
	var seenPath string
	var seenSymbol, seenPair string
	exg := newPriceKlineTestExchange(t, nil, func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenSymbol = r.URL.Query().Get("symbol")
		seenPair = r.URL.Query().Get("pair")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			[1704067200000,"-0.00120000","-0.00040000","-0.00250000","-0.00150000","0",1704070799999,"0",60,"0","0","0"],
			[1704070800000,"-0.00150000","0.00080000","-0.00180000","0.00030000","0",1704074399999,"0",60,"0","0","0"]
		]`))
	})

	// 1) USD-M premiumIndex (and "premium" alias) preserves negative basis and zero volume.
	for _, priceArg := range []string{"premiumIndex", "premium"} {
		rows, err := exg.FetchOHLCV("BTC/USDT:USDT", "1h", 1704067200000, 2, map[string]interface{}{
			"price":             priceArg,
			banexg.ParamNoCache: true,
		})
		if err != nil {
			t.Fatalf("FetchOHLCV(%s) failed: %v", priceArg, err)
		}
		if seenPath != "/premiumIndexKlines" || seenSymbol != "BTCUSDT" || seenPair != "" {
			t.Fatalf("unexpected query for %s: path=%s symbol=%s pair=%s", priceArg, seenPath, seenSymbol, seenPair)
		}
		if len(rows) != 2 {
			t.Fatalf("expected 2 rows, got %d", len(rows))
		}
		if rows[0].Time != 1704067200000 ||
			math.Abs(rows[0].Open - -0.0012) > 1e-9 ||
			math.Abs(rows[0].High - -0.0004) > 1e-9 ||
			math.Abs(rows[0].Low - -0.0025) > 1e-9 ||
			math.Abs(rows[0].Close - -0.0015) > 1e-9 ||
			rows[0].Volume != 0 {
			t.Fatalf("unexpected negative basis row[0]: %+v", rows[0])
		}
	}

	// 2) COIN-M index kline resolves underlying pair="BTCUSD" instead of contract ID "BTCUSD_PERP".
	if _, err := exg.FetchOHLCV("BTC/USD:BTC", "1h", 1704067200000, 2, map[string]interface{}{
		"price":             "index",
		banexg.ParamNoCache: true,
	}); err != nil {
		t.Fatalf("FetchOHLCV(inverse index) failed: %v", err)
	}
	if seenPath != "/indexPriceKlines" || seenPair != "BTCUSD" || seenSymbol != "" {
		t.Fatalf("expected inverse index pair=BTCUSD, got path=%s pair=%s symbol=%s", seenPath, seenPair, seenSymbol)
	}

	// 3) COIN-M premiumIndex uses dapiPublicGetPremiumIndexKlines with symbol="BTCUSD_PERP".
	if _, err := exg.FetchOHLCV("BTC/USD:BTC", "1h", 1704067200000, 2, map[string]interface{}{
		"price":             "premiumIndex",
		banexg.ParamNoCache: true,
	}); err != nil {
		t.Fatalf("FetchOHLCV(inverse premiumIndex) failed: %v", err)
	}
	if seenPath != "/premiumIndexKlines" || seenSymbol != "BTCUSD_PERP" {
		t.Fatalf("expected inverse premiumIndex symbol=BTCUSD_PERP, got path=%s symbol=%s", seenPath, seenSymbol)
	}

	// 4) Spot and delivery future premiumIndex are rejected cleanly with CodeNotSupport.
	if _, err := exg.FetchOHLCV("SPOTONLY/USDT", "1h", 0, 2, map[string]interface{}{"price": "premiumIndex", banexg.ParamMarket: banexg.MarketSpot}); err == nil || err.Code != errs.CodeNotSupport {
		t.Fatalf("expected CodeNotSupport on spot premiumIndex, got %v", err)
	}
	if _, err := exg.FetchOHLCV("BTC/USDT:USDT-240329", "1h", 0, 2, map[string]interface{}{"price": "premiumIndex"}); err == nil || err.Code != errs.CodeNotSupport {
		t.Fatalf("expected CodeNotSupport on delivery future premiumIndex, got %v", err)
	}
}

func TestFetchPriceOHLCVArchive_PremiumMarkAndIndexVision(t *testing.T) {
	jan := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	feb := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	end := time.Date(2024, 2, 2, 0, 0, 0, 0, time.UTC).UnixMilli()

	// Include header, microsecond & millisecond timestamps, negative basis values, and float/empty ignore columns!
	zipBody := makeVisionZip(t, "price-kline.csv", []string{
		"open_time,open,high,low,close,ignore,close_time,ignore,ignore,ignore,ignore,ignore",
		"1704067200000000,-0.0011,-0.0002,-0.0029,-0.0015,0.0,1704070799999999,0.0,0.0,,,",
		"1706745600000,-0.0015,0.0005,-0.0020,0.0002,0,1706749199999,0,0,0,0,0",
	})

	var paths []string
	exg := newPriceKlineTestExchange(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(zipBody)
	}, nil)

	// 1) USD-M premiumIndexKlines (4h timeframe supported on price klines!)
	paths = nil
	rows, avail, err := exg.FetchPriceOHLCVArchive(context.Background(), "premiumIndex", "BTC/USDT:USDT", "4h", jan, end)
	if err != nil || !avail || len(rows) != 2 {
		t.Fatalf("FetchPriceOHLCVArchive(premiumIndex) = len %d, avail %v, err %v", len(rows), avail, err)
	}
	if rows[0].Time != jan || math.Abs(rows[0].Close - -0.0015) > 1e-9 || rows[0].Volume != 0 || rows[1].Time != feb {
		t.Fatalf("unexpected parsed rows: %+v", rows)
	}
	wantPremPaths := []string{
		"/futures/um/monthly/premiumIndexKlines/BTCUSDT/4h/BTCUSDT-4h-2024-01.zip",
		"/futures/um/daily/premiumIndexKlines/BTCUSDT/4h/BTCUSDT-4h-2024-02-01.zip",
	}
	if !reflect.DeepEqual(paths, wantPremPaths) {
		t.Fatalf("premiumIndex Vision paths = %#v, want %#v", paths, wantPremPaths)
	}

	// 2) USD-M markPriceKlines
	paths = nil
	if _, avail, err := exg.FetchPriceOHLCVArchive(context.Background(), "mark", "BTC/USDT:USDT", "1h", jan, feb); err != nil || !avail {
		t.Fatalf("FetchPriceOHLCVArchive(mark) failed: avail=%v err=%v", avail, err)
	}
	if want := []string{"/futures/um/monthly/markPriceKlines/BTCUSDT/1h/BTCUSDT-1h-2024-01.zip"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("mark Vision paths = %#v, want %#v", paths, want)
	}

	// 3) USD-M & COIN-M indexPriceKlines use underlying index pair ("BTCUSDT" and "BTCUSD")
	paths = nil
	if _, avail, err := exg.FetchPriceOHLCVArchive(context.Background(), "index", "BTC/USDT:USDT-240329", "5m", jan, feb); err != nil || !avail {
		t.Fatalf("FetchPriceOHLCVArchive(linear index) failed: avail=%v err=%v", avail, err)
	}
	if _, avail, err := exg.FetchPriceOHLCVArchive(context.Background(), "index", "BTC/USD:BTC", "5m", jan, feb); err != nil || !avail {
		t.Fatalf("FetchPriceOHLCVArchive(inverse index) failed: avail=%v err=%v", avail, err)
	}
	wantIdxPaths := []string{
		"/futures/um/monthly/indexPriceKlines/BTCUSDT/5m/BTCUSDT-5m-2024-01.zip",
		"/futures/cm/monthly/indexPriceKlines/BTCUSD/5m/BTCUSD-5m-2024-01.zip",
	}
	if !reflect.DeepEqual(paths, wantIdxPaths) {
		t.Fatalf("index Vision paths = %#v, want %#v", paths, wantIdxPaths)
	}

	// 4) Spot returns available=false without error.
	exg.MarketType = banexg.MarketSpot
	if _, avail, err := exg.FetchPriceOHLCVArchive(context.Background(), "mark", "SPOTONLY/USDT", "1m", jan, feb); err != nil || avail {
		t.Fatalf("expected spot FetchPriceOHLCVArchive unavailable without error, got avail=%v err=%v", avail, err)
	}
}
