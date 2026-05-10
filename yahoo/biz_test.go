package yahoo

import (
	"os"
	"testing"
	"time"

	"github.com/banbox/banexg"
)

// liveOrSkip skips tests that hit the real Yahoo Finance API unless
// BANEXG_YAHOO_LIVE=1 is set, so the package is testable in offline CI.
func liveOrSkip(t *testing.T) *Yahoo {
	t.Helper()
	if os.Getenv("BANEXG_YAHOO_LIVE") != "1" {
		t.Skip("set BANEXG_YAHOO_LIVE=1 to run live Yahoo Finance tests")
	}
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return exg
}

func TestNewExchange_NoNetwork(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if exg.ID != "yahoo" {
		t.Errorf("ID=%q, want yahoo", exg.ID)
	}
	if !exg.HasApi(banexg.ApiFetchOHLCV, "") {
		t.Error("FetchOHLCV must be advertised as supported")
	}
	if exg.HasApi(banexg.ApiCreateOrder, "") {
		t.Error("CreateOrder must NOT be advertised as supported")
	}
}

func TestMapMarket_Lazy(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m, err := exg.MapMarket("AAPL", 0)
	if err != nil {
		t.Fatalf("MapMarket: %v", err)
	}
	if !m.Spot || m.Symbol != "AAPL" || m.Base != "AAPL" || m.Quote != "USD" {
		t.Errorf("market: %+v", m)
	}
}

func TestFetchOHLCV_Daily_AAPL(t *testing.T) {
	exg := liveOrSkip(t)
	since := time.Now().AddDate(-10, 0, 0).UnixMilli()
	klines, err := exg.FetchOHLCV("AAPL", "1d", since, 0, nil)
	if err != nil {
		t.Fatalf("FetchOHLCV: %v", err)
	}
	if len(klines) < 2000 {
		t.Errorf("want >=2000 daily bars over 10y, got %d", len(klines))
	}
	for i := 1; i < len(klines); i++ {
		if klines[i].Time <= klines[i-1].Time {
			t.Fatalf("timestamps not strictly increasing at %d", i)
		}
	}
	last := klines[len(klines)-1]
	if last.Open <= 0 || last.Close <= 0 || last.High <= 0 || last.Low <= 0 {
		t.Errorf("invalid last bar: %+v", last)
	}
}

func TestFetchOHLCV_1m_AAPL(t *testing.T) {
	exg := liveOrSkip(t)
	klines, err := exg.FetchOHLCV("AAPL", "1m", 0, 0, nil)
	if err != nil {
		t.Fatalf("FetchOHLCV 1m: %v", err)
	}
	if len(klines) == 0 {
		t.Fatal("got 0 1-minute bars; weekend?")
	}
	for _, k := range klines {
		if k.Open <= 0 || k.Close <= 0 {
			t.Errorf("invalid bar %+v", k)
		}
	}
}

func TestFetchOHLCV_Hourly_AAPL(t *testing.T) {
	exg := liveOrSkip(t)
	klines, err := exg.FetchOHLCV("AAPL", "1h", 0, 0, nil)
	if err != nil {
		t.Fatalf("FetchOHLCV 1h: %v", err)
	}
	if len(klines) < 100 {
		t.Errorf("want >=100 hourly bars, got %d", len(klines))
	}
}

func TestFetchTickers(t *testing.T) {
	exg := liveOrSkip(t)
	tickers, err := exg.FetchTickers([]string{"AAPL", "MSFT", "GOOG"}, nil)
	if err != nil {
		t.Fatalf("FetchTickers: %v", err)
	}
	if len(tickers) != 3 {
		t.Errorf("want 3 tickers, got %d", len(tickers))
	}
	for _, tk := range tickers {
		if tk.Last <= 0 {
			t.Errorf("%s last=%v", tk.Symbol, tk.Last)
		}
	}
}

func TestFetchTicker_Single(t *testing.T) {
	exg := liveOrSkip(t)
	tk, err := exg.FetchTicker("AAPL", nil)
	if err != nil {
		t.Fatalf("FetchTicker: %v", err)
	}
	if tk.Symbol != "AAPL" || tk.Last <= 0 {
		t.Errorf("ticker: %+v", tk)
	}
}

func TestFetchOHLCV_BadSymbol(t *testing.T) {
	exg := liveOrSkip(t)
	_, err := exg.FetchOHLCV("ZZZZ_NOT_A_SYMBOL_X", "1d", 0, 0, nil)
	if err == nil {
		t.Fatal("expected error for unknown symbol")
	}
}

// TestFetchOHLCV_AAPL_AllTimeframes hits Yahoo for AAPL across every
// timeframe the project promises: 1m, 5m, 1h, 4h, 1D, 1W, 1M.
// Skips when BANEXG_YAHOO_LIVE != 1.
func TestFetchOHLCV_AAPL_AllTimeframes(t *testing.T) {
	exg := liveOrSkip(t)
	cases := []struct {
		tf      string
		minBars int
	}{
		{"1m", 1},   // intraday recent only (5d window); empty on weekends
		{"5m", 1},   // last 60d
		{"1h", 100}, // last 730d
		{"4h", 25},  // aggregated from 1h
		{"1D", 200}, // 1y default
		{"1W", 50},  // weekly, full max
		{"1M", 12},  // monthly, full max
	}
	for _, c := range cases {
		c := c
		t.Run(c.tf, func(t *testing.T) {
			klines, err := exg.FetchOHLCV("AAPL", c.tf, 0, 0, nil)
			if err != nil {
				t.Fatalf("FetchOHLCV %s: %v", c.tf, err)
			}
			if len(klines) < c.minBars {
				t.Fatalf("%s: want >=%d bars, got %d", c.tf, c.minBars, len(klines))
			}
			// Sanity: monotonically increasing timestamps, positive OHLC.
			for i, k := range klines {
				if k.Open <= 0 || k.High <= 0 || k.Low <= 0 || k.Close <= 0 {
					t.Errorf("%s bar %d non-positive OHLC: %+v", c.tf, i, k)
				}
				if k.High < k.Low {
					t.Errorf("%s bar %d high<low: %+v", c.tf, i, k)
				}
				if i > 0 && k.Time <= klines[i-1].Time {
					t.Fatalf("%s timestamps not strictly increasing at %d", c.tf, i)
				}
			}
			first, last := klines[0], klines[len(klines)-1]
			t.Logf("%-3s  %d bars  first=%s O=%.2f C=%.2f  last=%s O=%.2f C=%.2f",
				c.tf, len(klines),
				time.UnixMilli(first.Time).UTC().Format("2006-01-02 15:04"), first.Open, first.Close,
				time.UnixMilli(last.Time).UTC().Format("2006-01-02 15:04"), last.Open, last.Close)
		})
	}
}
