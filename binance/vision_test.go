package binance

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/banbox/banexg"
)

func TestVisionTimeframe(t *testing.T) {
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{input: "1m", want: "1m", ok: true},
		{input: "2h", want: "2h", ok: true},
		{input: "4h", ok: false},
		{input: "6h", ok: false},
		{input: "8h", ok: false},
		{input: "12h", ok: false},
		{input: "1d", ok: false},
		{input: "3d", ok: false},
		{input: "1w", ok: false},
		{input: "1M", ok: false},
		{input: "1mo", ok: false},
		{input: "2m", ok: false},
	}
	for _, test := range tests {
		got, ok := visionTimeframe(test.input)
		if got != test.want || ok != test.ok {
			t.Fatalf("visionTimeframe(%q) = %q, %v; want %q, %v", test.input, got, ok, test.want, test.ok)
		}
	}
}

func TestVisionArchiveFilesUsesMonthlyFilesForCompleteMonths(t *testing.T) {
	start := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	end := time.Date(2021, 3, 3, 0, 0, 0, 0, time.UTC).UnixMilli()
	want := []visionArchiveFile{
		{granularity: "monthly", period: "2021-01"},
		{granularity: "monthly", period: "2021-02"},
		{granularity: "daily", period: "2021-03-01"},
		{granularity: "daily", period: "2021-03-02"},
	}
	if got := visionArchiveFiles(start, end); !reflect.DeepEqual(got, want) {
		t.Fatalf("visionArchiveFiles() = %#v, want %#v", got, want)
	}
}

func TestParseVisionKlines(t *testing.T) {
	linear := makeVisionZip(t, "ATOMUSDT-1m-2021-01.csv", []string{
		"open_time,open,high,low,close,volume,close_time,quote_asset_volume,number_of_trades,taker_buy_base_asset_volume,taker_buy_quote_asset_volume,ignore",
		"1609459200000000,1,2,0.5,1.5,10,1609459259999999,20,7,4,8,0",
		"1609459200000000,9,10,8,9.5,11,1609459259999999,21,8,5,9,0",
		"1609459260000,1.5,2.5,1,2,12,1609459319999,24,9,6,10,0",
	})
	got, err := parseVisionKlines(linear, 1609459200000, 1609459320000, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Time != 1609459200000 || got[0].Open != 9 || got[0].Volume != 11 ||
		got[0].Quote != 21 || got[0].BuyVolume != 5 || got[0].TradeNum != 8 {
		t.Fatalf("linear Vision rows were not parsed/deduplicated correctly: %+v", got)
	}

	inverse := makeVisionZip(t, "BTCUSD_PERP-1m-2021-01.csv", []string{
		"1609459200000,2,3,1,2.5,50,1609459259999,60,17,18,19,0",
	})
	got, err = parseVisionKlines(inverse, 1609459200000, 1609459260000, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Volume != 60 || got[0].Quote != 60 || got[0].BuyVolume != 19 || got[0].TradeNum != 17 {
		t.Fatalf("inverse Vision row mapping is wrong: %+v", got)
	}
}

func TestParseVisionKlinesRejectsMalformedRows(t *testing.T) {
	data := makeVisionZip(t, "bad.csv", []string{"not-a-kline"})
	if _, err := parseVisionKlines(data, 1, 2, false); err == nil {
		t.Fatal("malformed Vision row was accepted")
	}
}

func TestFetchOHLCVArchiveUsesVisionFiles(t *testing.T) {
	jan := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	feb := time.Date(2021, 2, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	end := time.Date(2021, 2, 2, 0, 0, 0, 0, time.UTC).UnixMilli()
	data := makeVisionZip(t, "ATOMUSDT-1m.csv", []string{
		"1609459200000,1,2,0.5,1.5,10,1609459259999,20,7,4,8,0",
		"1612137600000,2,3,1.5,2.5,11,1612137659999,22,8,5,9,0",
	})
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(data)
	}))
	defer server.Close()
	oldBaseURL := binanceVisionBaseURL
	binanceVisionBaseURL = server.URL
	t.Cleanup(func() { binanceVisionBaseURL = oldBaseURL })

	market := &banexg.Market{
		ID:     "ATOMUSDT",
		Symbol: "ATOM/USDT:USDT",
		Type:   banexg.MarketLinear,
		Linear: true,
		Swap:   true,
	}
	exchange := &Binance{Exchange: &banexg.Exchange{
		ExgInfo: &banexg.ExgInfo{
			ID:         "binance",
			MarketType: banexg.MarketLinear,
			Markets:    banexg.MarketMap{market.Symbol: market},
		},
		HttpClient: server.Client(),
	}}
	got, available, err := exchange.FetchOHLCVArchive(context.Background(), market.Symbol, "1m", jan, end)
	if err != nil || !available {
		t.Fatalf("FetchOHLCVArchive() = %v, %v, %v", got, available, err)
	}
	if len(got) != 2 || got[0].Time != jan || got[1].Time != feb {
		t.Fatalf("unexpected archive rows: %+v", got)
	}
	wantPaths := []string{
		"/futures/um/monthly/klines/ATOMUSDT/1m/ATOMUSDT-1m-2021-01.zip",
		"/futures/um/daily/klines/ATOMUSDT/1m/ATOMUSDT-1m-2021-02-01.zip",
	}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Fatalf("requested Vision paths = %#v, want %#v", paths, wantPaths)
	}
}

func TestFetchOHLCVArchiveFallsBackWhenFileMissing(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	oldBaseURL := binanceVisionBaseURL
	binanceVisionBaseURL = server.URL
	t.Cleanup(func() { binanceVisionBaseURL = oldBaseURL })
	market := &banexg.Market{ID: "ATOMUSDT", Symbol: "ATOM/USDT:USDT", Linear: true, Type: banexg.MarketLinear}
	exchange := &Binance{Exchange: &banexg.Exchange{
		ExgInfo: &banexg.ExgInfo{
			ID:         "binance",
			MarketType: banexg.MarketLinear,
			Markets:    banexg.MarketMap{market.Symbol: market},
		},
		HttpClient: server.Client(),
	}}
	_, available, err := exchange.FetchOHLCVArchive(context.Background(), market.Symbol, "1m", 1609459200000, 1609545600000)
	if err != nil || available {
		t.Fatalf("missing Vision file = available %v, err %v; want unavailable without error", available, err)
	}
}

func TestFetchOHLCVArchiveRetriesTruncatedFile(t *testing.T) {
	data := makeVisionZip(t, "ATOMUSDT-1m.csv", []string{
		"1609459200000,1,2,0.5,1.5,10,1609459259999,20,7,4,8,0",
	})
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		if attempts == 1 {
			_, _ = w.Write(data[:len(data)/2])
			return
		}
		_, _ = w.Write(data)
	}))
	defer server.Close()
	oldBaseURL := binanceVisionBaseURL
	binanceVisionBaseURL = server.URL
	t.Cleanup(func() { binanceVisionBaseURL = oldBaseURL })
	market := &banexg.Market{ID: "ATOMUSDT", Symbol: "ATOM/USDT:USDT", Linear: true, Type: banexg.MarketLinear}
	exchange := &Binance{Exchange: &banexg.Exchange{
		ExgInfo: &banexg.ExgInfo{
			ID:         "binance",
			MarketType: banexg.MarketLinear,
			Markets:    banexg.MarketMap{market.Symbol: market},
		},
		HttpClient: server.Client(),
	}}
	start := int64(1609459200000)
	got, available, err := exchange.FetchOHLCVArchive(context.Background(), market.Symbol, "1m", start, start+24*60*60*1000)
	if err != nil || !available || len(got) != 1 || attempts != 2 {
		t.Fatalf("truncated Vision file = rows %d, available %v, attempts %d, err %v; want one row, available, two attempts", len(got), available, attempts, err)
	}
}

func TestFetchOHLCVArchiveRetriesRequestTimeout(t *testing.T) {
	data := makeVisionZip(t, "ATOMUSDT-1m.csv", []string{
		"1609459200000,1,2,0.5,1.5,10,1609459259999,20,7,4,8,0",
	})
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(data)
	}))
	defer server.Close()
	oldBaseURL := binanceVisionBaseURL
	oldTimeout := binanceVisionRequestTimeout
	binanceVisionBaseURL = server.URL
	binanceVisionRequestTimeout = 20 * time.Millisecond
	t.Cleanup(func() {
		binanceVisionBaseURL = oldBaseURL
		binanceVisionRequestTimeout = oldTimeout
	})
	market := &banexg.Market{ID: "ATOMUSDT", Symbol: "ATOM/USDT:USDT", Linear: true, Type: banexg.MarketLinear}
	exchange := &Binance{Exchange: &banexg.Exchange{
		ExgInfo: &banexg.ExgInfo{
			ID:         "binance",
			MarketType: banexg.MarketLinear,
			Markets:    banexg.MarketMap{market.Symbol: market},
		},
		HttpClient: server.Client(),
	}}
	start := int64(1609459200000)
	got, available, err := exchange.FetchOHLCVArchive(context.Background(), market.Symbol, "1m", start, start+24*60*60*1000)
	if err != nil || !available || len(got) != 1 || attempts != 2 {
		t.Fatalf("request timeout retry = rows %d, available %v, attempts %d, err %v; want one row, available, two attempts", len(got), available, attempts, err)
	}
}

func TestFetchOHLCVArchiveSkipsLeadingMissingFiles(t *testing.T) {
	data := makeVisionZip(t, "ATOMUSDT-1m.csv", []string{
		"1612137600000,1,2,0.5,1.5,10,1612137659999,20,7,4,8,0",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "monthly/klines/ATOMUSDT/1m/ATOMUSDT-1m-2021-02.zip") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(data)
	}))
	defer server.Close()
	oldBaseURL := binanceVisionBaseURL
	binanceVisionBaseURL = server.URL
	t.Cleanup(func() { binanceVisionBaseURL = oldBaseURL })
	market := &banexg.Market{ID: "ATOMUSDT", Symbol: "ATOM/USDT:USDT", Linear: true, Type: banexg.MarketLinear}
	exchange := &Binance{Exchange: &banexg.Exchange{
		ExgInfo: &banexg.ExgInfo{
			ID:         "binance",
			MarketType: banexg.MarketLinear,
			Markets:    banexg.MarketMap{market.Symbol: market},
		},
		HttpClient: server.Client(),
	}}
	start := time.Date(2021, 1, 15, 0, 0, 0, 0, time.UTC).UnixMilli()
	end := time.Date(2021, 3, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	got, available, err := exchange.FetchOHLCVArchive(context.Background(), market.Symbol, "1m", start, end)
	if err != nil || !available || len(got) != 1 || got[0].Time != time.Date(2021, 2, 1, 0, 0, 0, 0, time.UTC).UnixMilli() {
		t.Fatalf("leading missing Vision files = rows %+v, available %v, err %v; want one February row", got, available, err)
	}
}

func makeVisionZip(t *testing.T, name string, rows []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	file, err := archive.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if _, err := file.Write([]byte(row + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
