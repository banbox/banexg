package yahoo

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

// makeSign builds the final HTTP request for Yahoo Finance public endpoints.
// Yahoo requires no auth; we substitute the {symbol} path placeholder and
// URL-encode remaining params into a query string.
func makeSign(_ *Yahoo) banexg.FuncSign {
	return func(api *banexg.Entry, args map[string]interface{}) *banexg.HttpReq {
		params := utils.SafeParams(args)
		raw := api.Url
		if sym, ok := params["symbol"]; ok {
			s, _ := sym.(string)
			raw = strings.Replace(raw, "{symbol}", url.PathEscape(s), 1)
			delete(params, "symbol")
		}
		if len(params) > 0 {
			raw += "?" + utils.UrlEncodeMap(params, true)
		}
		return &banexg.HttpReq{
			Url:     raw,
			Method:  api.Method,
			Headers: http.Header{},
			Private: false,
		}
	}
}

// buildMarket constructs a Spot Market for a Yahoo ticker.
// Yahoo tickers cover stocks (AAPL), indices (^GSPC), crypto (BTC-USD),
// futures (ES=F), forex (EURUSD=X). For v1 we treat all as Spot.
func buildMarket(ticker string) *banexg.Market {
	base, quote, isIndex := splitTicker(ticker)
	return &banexg.Market{
		ID:          ticker,
		LowercaseID: strings.ToLower(ticker),
		Symbol:      ticker,
		Base:        base,
		Quote:       quote,
		Type:        banexg.MarketSpot,
		Spot:        true,
		Active:      true,
		FeeSide:     "quote",
		Precision: &banexg.Precision{
			Amount:     1,
			Price:      0.01,
			Base:       1,
			Quote:      0.01,
			ModeAmount: banexg.PrecModeDecimalPlace,
			ModePrice:  banexg.PrecModeTickSize,
			ModeBase:   banexg.PrecModeDecimalPlace,
			ModeQuote:  banexg.PrecModeTickSize,
		},
		Limits: &banexg.MarketLimits{
			Amount: &banexg.LimitRange{Min: 1},
		},
		Info: map[string]interface{}{
			"isIndex": isIndex,
			"ticker":  ticker,
		},
	}
}

// splitTicker derives a Base/Quote pair from a Yahoo ticker symbol.
func splitTicker(t string) (base, quote string, isIndex bool) {
	if t == "" {
		return "", "USD", false
	}
	if t[0] == '^' {
		return t, "USD", true
	}
	if strings.HasSuffix(t, "=X") {
		core := strings.TrimSuffix(t, "=X")
		if len(core) == 6 {
			return core[:3], core[3:], false
		}
		return core, "USD", false
	}
	if strings.HasSuffix(t, "=F") {
		return strings.TrimSuffix(t, "=F"), "USD", false
	}
	if strings.Contains(t, "-") {
		parts := strings.SplitN(t, "-", 2)
		if len(parts) == 2 {
			return parts[0], parts[1], false
		}
	}
	return t, "USD", false
}

// chooseRange picks a sensible Yahoo `range` parameter when no explicit
// since/until is provided. Yahoo accepts: 1d/5d/1mo/3mo/6mo/1y/2y/5y/10y/ytd/max
// plus the day-count strings 60d/730d for intraday lookback.
func chooseRange(interval string, limit int) string {
	switch interval {
	case "1m":
		return "5d"
	case "2m", "5m", "15m", "30m":
		return "60d"
	case "60m", "90m", "4h":
		return "730d"
	case "1d":
		if limit > 0 && limit <= 252 {
			return "1y"
		}
		return "10y"
	case "5d", "1wk", "1mo", "3mo":
		return "max"
	}
	return "10y"
}

// parseChart converts the v8 chart JSON body to []*banexg.Kline.
// Uses the raw OHLCV under indicators.quote[0]; adjclose is ignored to match
// the unadjusted convention used by other banexg exchanges.
func parseChart(body string) ([]*banexg.Kline, *errs.Error) {
	var resp chartResp
	if err := utils.UnmarshalString(body, &resp, utils.JsonNumDefault); err != nil {
		return nil, errs.New(errs.CodeUnmarshalFail, err)
	}
	if resp.Chart.Error != nil {
		return nil, errs.NewMsg(errs.CodeInvalidResponse, "yahoo chart error: %s %s",
			resp.Chart.Error.Code, resp.Chart.Error.Description)
	}
	if len(resp.Chart.Result) == 0 {
		return nil, errs.NewMsg(errs.CodeDataNotFound, "yahoo chart empty result")
	}
	res := resp.Chart.Result[0]
	if len(res.Indicators.Quote) == 0 {
		return nil, errs.NewMsg(errs.CodeDataNotFound, "yahoo chart missing indicators")
	}
	q := res.Indicators.Quote[0]
	n := len(res.Timestamp)
	out := make([]*banexg.Kline, 0, n)
	for i := 0; i < n; i++ {
		// Skip bars where any OHLC element is null (e.g. trading halts).
		if i >= len(q.Open) || q.Open[i] == nil ||
			i >= len(q.High) || q.High[i] == nil ||
			i >= len(q.Low) || q.Low[i] == nil ||
			i >= len(q.Close) || q.Close[i] == nil {
			continue
		}
		var vol float64
		if i < len(q.Volume) && q.Volume[i] != nil {
			vol = *q.Volume[i]
		}
		out = append(out, &banexg.Kline{
			Time:   res.Timestamp[i] * 1000,
			Open:   *q.Open[i],
			High:   *q.High[i],
			Low:    *q.Low[i],
			Close:  *q.Close[i],
			Volume: vol,
		})
	}
	return out, nil
}

// coerceSymbols extracts a []string of tickers from a heterogeneous params
// value. Callers commonly pass []string, but JSON-decoded params arrive as
// []interface{} and human-typed config often uses a comma-separated string.
// Empty/whitespace entries are dropped.
func coerceSymbols(v interface{}) []string {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case []string:
		out := make([]string, 0, len(t))
		for _, s := range t {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				if s = strings.TrimSpace(s); s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	case string:
		parts := strings.Split(t, ",")
		out := make([]string, 0, len(parts))
		for _, s := range parts {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// aggregate groups input klines into buckets of `bucketMs` size, aligned to
// epoch (i.e. bucket start = floor(t / bucketMs) * bucketMs). Used for
// timeframes Yahoo doesn't return natively — currently 4h built on 1h.
// Volumes sum, highs/lows extend, open is taken from the first bar in the
// bucket, close from the last.
func aggregate(in []*banexg.Kline, bucketMs int64) []*banexg.Kline {
	if len(in) == 0 || bucketMs <= 0 {
		return in
	}
	out := make([]*banexg.Kline, 0, len(in))
	var cur *banexg.Kline
	var curBucket int64 = -1
	for _, k := range in {
		bucket := k.Time / bucketMs
		if cur == nil || bucket != curBucket {
			if cur != nil {
				out = append(out, cur)
			}
			cur = &banexg.Kline{
				Time:   bucket * bucketMs,
				Open:   k.Open,
				High:   k.High,
				Low:    k.Low,
				Close:  k.Close,
				Volume: k.Volume,
			}
			curBucket = bucket
			continue
		}
		if k.High > cur.High {
			cur.High = k.High
		}
		if k.Low < cur.Low {
			cur.Low = k.Low
		}
		cur.Close = k.Close
		cur.Volume += k.Volume
	}
	if cur != nil {
		out = append(out, cur)
	}
	return out
}

func parseQuote(body string) ([]*banexg.Ticker, *errs.Error) {
	var resp quoteResp
	if err := utils.UnmarshalString(body, &resp, utils.JsonNumDefault); err != nil {
		return nil, errs.New(errs.CodeUnmarshalFail, err)
	}
	if resp.QuoteResponse.Error != nil {
		return nil, errs.NewMsg(errs.CodeInvalidResponse, "yahoo quote error: %s %s",
			resp.QuoteResponse.Error.Code, resp.QuoteResponse.Error.Description)
	}
	out := make([]*banexg.Ticker, 0, len(resp.QuoteResponse.Result))
	for _, r := range resp.QuoteResponse.Result {
		out = append(out, &banexg.Ticker{
			Symbol:        r.Symbol,
			TimeStamp:     r.RegularMarketTime * 1000,
			Last:          r.RegularMarketPrice,
			Open:          r.RegularMarketOpen,
			High:          r.RegularMarketDayHigh,
			Low:           r.RegularMarketDayLow,
			Close:         r.RegularMarketPrice,
			Bid:           r.Bid,
			Ask:           r.Ask,
			BidVolume:     float64(r.BidSize),
			AskVolume:     float64(r.AskSize),
			BaseVolume:    float64(r.RegularMarketVolume),
			PreviousClose: r.RegularMarketPreviousClose,
			Change:        r.RegularMarketChange,
			Percentage:    r.RegularMarketChangePercent,
			Info: map[string]interface{}{
				"currency":  r.Currency,
				"exchange":  r.Exchange,
				"quoteType": r.QuoteType,
			},
		})
	}
	return out, nil
}
