package yahoo

import (
	"testing"

	"github.com/banbox/banexg"
)

func TestSplitTicker(t *testing.T) {
	cases := []struct {
		in      string
		base    string
		quote   string
		isIndex bool
	}{
		{"AAPL", "AAPL", "USD", false},
		{"^GSPC", "^GSPC", "USD", true},
		{"BTC-USD", "BTC", "USD", false},
		{"EURUSD=X", "EUR", "USD", false},
		{"ES=F", "ES", "USD", false},
		{"BRK-B", "BRK", "B", false}, // BRK-B will look like a pair; documented limitation
		{"", "", "USD", false},
	}
	for _, c := range cases {
		b, q, ix := splitTicker(c.in)
		if b != c.base || q != c.quote || ix != c.isIndex {
			t.Errorf("splitTicker(%q)=(%q,%q,%v), want (%q,%q,%v)",
				c.in, b, q, ix, c.base, c.quote, c.isIndex)
		}
	}
}

func TestParseChart_OK(t *testing.T) {
	body := `{
      "chart": {
        "result": [{
          "meta": {"symbol":"AAPL","currency":"USD"},
          "timestamp": [1700000000, 1700086400, 1700172800],
          "indicators": {
            "quote": [{
              "open":   [150.0, 151.5, null],
              "high":   [152.0, 152.5, 153.0],
              "low":    [149.0, 150.5, 151.0],
              "close":  [151.5, 152.0, 152.5],
              "volume": [1000000, 1200000, null]
            }]
          }
        }],
        "error": null
      }
    }`
	klines, err := parseChart(body)
	if err != nil {
		t.Fatalf("parseChart error: %v", err)
	}
	if len(klines) != 2 {
		t.Fatalf("want 2 klines (3rd has null open), got %d", len(klines))
	}
	if klines[0].Time != 1700000000*1000 || klines[0].Open != 150.0 || klines[0].Close != 151.5 {
		t.Errorf("first kline mismatch: %+v", klines[0])
	}
	if klines[1].Volume != 1200000 {
		t.Errorf("second volume want 1.2M, got %v", klines[1].Volume)
	}
}

func TestParseChart_Error(t *testing.T) {
	body := `{"chart":{"result":null,"error":{"code":"Not Found","description":"No data"}}}`
	if _, err := parseChart(body); err == nil {
		t.Fatal("expected error for chart error response")
	}
}

func TestParseQuote_OK(t *testing.T) {
	body := `{
      "quoteResponse": {
        "result": [
          {"symbol":"AAPL","regularMarketPrice":190.5,"regularMarketDayHigh":191.0,
           "regularMarketDayLow":189.0,"regularMarketOpen":190.0,
           "regularMarketPreviousClose":189.5,"regularMarketChange":1.0,
           "regularMarketChangePercent":0.53,"regularMarketVolume":50000000,
           "bid":190.4,"ask":190.6,"regularMarketTime":1700000000,
           "currency":"USD","exchange":"NMS","quoteType":"EQUITY"}
        ],
        "error": null
      }
    }`
	tickers, err := parseQuote(body)
	if err != nil {
		t.Fatalf("parseQuote error: %v", err)
	}
	if len(tickers) != 1 {
		t.Fatalf("want 1 ticker, got %d", len(tickers))
	}
	tk := tickers[0]
	if tk.Symbol != "AAPL" || tk.Last != 190.5 || tk.High != 191.0 || tk.Low != 189.0 {
		t.Errorf("ticker mismatch: %+v", tk)
	}
	if tk.TimeStamp != 1700000000*1000 {
		t.Errorf("ts want %d, got %d", 1700000000*1000, tk.TimeStamp)
	}
}

func TestAggregate_4h(t *testing.T) {
	// 8 hourly bars starting at 2024-01-01 00:00 UTC → expect 2 bars of 4h.
	hour := int64(60 * 60 * 1000)
	t0 := int64(1704067200000) // 2024-01-01T00:00:00Z, aligned to 4h boundary
	in := []*banexg.Kline{
		{Time: t0 + 0*hour, Open: 100, High: 102, Low: 99, Close: 101, Volume: 10},
		{Time: t0 + 1*hour, Open: 101, High: 105, Low: 100, Close: 104, Volume: 20},
		{Time: t0 + 2*hour, Open: 104, High: 106, Low: 103, Close: 105, Volume: 30},
		{Time: t0 + 3*hour, Open: 105, High: 107, Low: 102, Close: 106, Volume: 40},
		{Time: t0 + 4*hour, Open: 106, High: 108, Low: 105, Close: 107, Volume: 50},
		{Time: t0 + 5*hour, Open: 107, High: 109, Low: 106, Close: 108, Volume: 60},
		{Time: t0 + 6*hour, Open: 108, High: 110, Low: 107, Close: 109, Volume: 70},
		{Time: t0 + 7*hour, Open: 109, High: 111, Low: 108, Close: 110, Volume: 80},
	}
	out := aggregate(in, 4*hour)
	if len(out) != 2 {
		t.Fatalf("want 2 4h bars, got %d", len(out))
	}
	if out[0].Time != t0 || out[0].Open != 100 || out[0].Close != 106 ||
		out[0].High != 107 || out[0].Low != 99 || out[0].Volume != 100 {
		t.Errorf("bucket0 wrong: %+v", out[0])
	}
	if out[1].Time != t0+4*hour || out[1].Open != 106 || out[1].Close != 110 ||
		out[1].High != 111 || out[1].Low != 105 || out[1].Volume != 260 {
		t.Errorf("bucket1 wrong: %+v", out[1])
	}
}

func TestAggregate_Empty(t *testing.T) {
	if got := aggregate(nil, 4*60*60*1000); got != nil {
		t.Errorf("want nil for empty input, got %v", got)
	}
}

func TestChooseRange(t *testing.T) {
	cases := map[string]string{
		"1m":  "5d",
		"5m":  "60d",
		"60m": "730d",
		"1d":  "10y",
		"1wk": "max",
	}
	for tf, want := range cases {
		if got := chooseRange(tf, 0); got != want {
			t.Errorf("chooseRange(%q)=%q, want %q", tf, got, want)
		}
	}
	if got := chooseRange("1d", 50); got != "1y" {
		t.Errorf("chooseRange(1d,50)=%q, want 1y", got)
	}
}
