package yahoo

import (
	"testing"
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
