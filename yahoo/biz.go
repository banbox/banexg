package yahoo

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

func (e *Yahoo) Init() *errs.Error {
	if err := e.Exchange.Init(); err != nil {
		return err
	}
	if e.UserAgent == "" {
		e.UserAgent = defaultUserAgent
	}
	e.TimeFrames = map[string]string{
		"1m":  "1m",
		"2m":  "2m",
		"5m":  "5m",
		"15m": "15m",
		"30m": "30m",
		"60m": "60m",
		"1h":  "60m",
		"1H":  "60m",
		"90m": "90m",
		// 4h has no native Yahoo interval; FetchOHLCV aggregates from 1h.
		"4h":  "4h",
		"4H":  "4h",
		"1d":  "1d",
		"1D":  "1d",
		"5d":  "5d",
		"1w":  "1wk",
		"1W":  "1wk",
		"1wk": "1wk",
		"1mo": "1mo",
		"1M":  "1mo",
		"3mo": "3mo",
	}
	e.ExgInfo.Min1mHole = 1
	e.MarketsLock.Lock()
	if e.Markets == nil {
		e.Markets = banexg.MarketMap{}
	}
	e.MarketsLock.Unlock()
	e.MarketsByIdLock.Lock()
	if e.MarketsById == nil {
		e.MarketsById = banexg.MarketArrMap{}
	}
	e.MarketsByIdLock.Unlock()
	return nil
}

// LoadMarkets is lazy: Yahoo has no "list all instruments" endpoint.
// If params[ParamSymbols] is provided, build Market entries for those tickers;
// otherwise return whatever has accumulated via earlier MapMarket calls.
func (e *Yahoo) LoadMarkets(reload bool, params map[string]interface{}) (banexg.MarketMap, *errs.Error) {
	var symbols []string
	if params != nil {
		symbols = coerceSymbols(params[banexg.ParamSymbols])
	}
	for _, s := range symbols {
		e.registerMarket(buildMarket(s))
	}
	e.MarketsLock.Lock()
	out := e.Markets
	e.MarketsLock.Unlock()
	return out, nil
}

// MapMarket builds a Market on demand for a Yahoo ticker.
func (e *Yahoo) MapMarket(rawID string, year int) (*banexg.Market, *errs.Error) {
	if rawID == "" {
		return nil, errs.NewMsg(errs.CodeParamRequired, "symbol required")
	}
	if mar := e.GetMarketById(rawID, ""); mar != nil {
		return mar, nil
	}
	m := buildMarket(rawID)
	e.registerMarket(m)
	return m, nil
}

func (e *Yahoo) registerMarket(m *banexg.Market) {
	e.MarketsLock.Lock()
	e.Markets[m.Symbol] = m
	e.MarketsLock.Unlock()
	e.MarketsByIdLock.Lock()
	e.MarketsById[m.ID] = []*banexg.Market{m}
	e.MarketsByIdLock.Unlock()
}

func (e *Yahoo) FetchOHLCV(symbol, timeframe string, since int64, limit int, params map[string]interface{}) ([]*banexg.Kline, *errs.Error) {
	if symbol == "" {
		return nil, errs.NewMsg(errs.CodeParamRequired, "symbol required")
	}
	interval := e.GetTimeFrame(timeframe)
	if interval == "" {
		return nil, errs.NewMsg(errs.CodeInvalidTimeFrame, "unsupported timeframe: %s", timeframe)
	}
	// 4h has no native Yahoo interval; fetch 1h and aggregate.
	if interval == "4h" {
		baseLimit := 0
		if limit > 0 {
			baseLimit = limit * 4
		}
		base, err := e.FetchOHLCV(symbol, "1h", since, baseLimit, params)
		if err != nil {
			return nil, err
		}
		out := aggregate(base, 4*60*60*1000)
		if limit > 0 && len(out) > limit {
			out = out[len(out)-limit:]
		}
		return out, nil
	}
	args := utils.SafeParams(params)
	args["symbol"] = symbol
	args[banexg.ParamInterval] = interval
	args["events"] = "history"

	until := utils.PopMapVal(args, banexg.ParamUntil, int64(0))
	if since > 0 {
		args["period1"] = strconv.FormatInt(since/1000, 10)
		if until > 0 {
			args["period2"] = strconv.FormatInt(until/1000, 10)
		} else {
			args["period2"] = strconv.FormatInt(time.Now().Unix(), 10)
		}
	} else {
		args["range"] = chooseRange(interval, limit)
	}

	tryNum := e.GetRetryNum("FetchOHLCV", 1)
	rsp := e.RequestApiRetry(context.Background(), MidChartGet, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	klines, err := parseChart(rsp.Content)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(klines) > limit {
		// Yahoo doesn't accept a `limit` parameter, so trim from the oldest end.
		klines = klines[len(klines)-limit:]
	}
	return klines, nil
}

func (e *Yahoo) FetchTickers(symbols []string, params map[string]interface{}) ([]*banexg.Ticker, *errs.Error) {
	if len(symbols) == 0 {
		return nil, errs.NewMsg(errs.CodeParamRequired, "symbols required")
	}
	args := utils.SafeParams(params)
	args["symbols"] = strings.Join(symbols, ",")

	tryNum := e.GetRetryNum("FetchTickers", 1)
	rsp := e.RequestApiRetry(context.Background(), MidQuoteGet, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	return parseQuote(rsp.Content)
}

func (e *Yahoo) FetchTicker(symbol string, params map[string]interface{}) (*banexg.Ticker, *errs.Error) {
	list, err := e.FetchTickers([]string{symbol}, params)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, errs.NewMsg(errs.CodeDataNotFound, "no ticker for %s", symbol)
	}
	return list[0], nil
}

func (e *Yahoo) FetchTickerPrice(symbol string, params map[string]interface{}) (map[string]float64, *errs.Error) {
	var symbols []string
	if symbol == "" {
		symbols = coerceSymbols(params[banexg.ParamSymbols])
	} else {
		symbols = []string{symbol}
	}
	if len(symbols) == 0 {
		return nil, errs.NewMsg(errs.CodeParamRequired, "symbol(s) required")
	}
	list, err := e.FetchTickers(symbols, params)
	if err != nil {
		return nil, err
	}
	out := make(map[string]float64, len(list))
	for _, t := range list {
		out[t.Symbol] = t.Last
	}
	return out, nil
}

func (e *Yahoo) Close() *errs.Error {
	return nil
}
