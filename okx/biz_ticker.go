package okx

import (
	"strconv"
	"time"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

func (e *OKX) FetchTickers(symbols []string, params map[string]interface{}) ([]*banexg.Ticker, *errs.Error) {
	return e.fetchTickersWithArgs(symbols, params)
}

func (e *OKX) FetchTicker(symbol string, params map[string]interface{}) (*banexg.Ticker, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	args[FldInstId] = market.ID
	tryNum := e.GetRetryNum("FetchTicker", 1)
	res := requestRetry[[]map[string]interface{}](e, MethodMarketGetTicker, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	items := res.Result
	if len(items) == 0 {
		return nil, errs.NewMsg(errs.CodeDataNotFound, "empty ticker result")
	}
	arr, err := decodeResult[Ticker](items)
	if err != nil {
		return nil, err
	}
	ticker := parseTicker(e, &arr[0], items[0], market.Type)
	if ticker == nil {
		return nil, errs.NewMsg(errs.CodeDataNotFound, "empty ticker")
	}
	return ticker, nil
}

func (e *OKX) FetchOHLCV(symbol, timeframe string, since int64, limit int, params map[string]interface{}) ([]*banexg.Kline, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	args[FldInstId] = market.ID
	args[FldBar] = e.GetTimeFrame(timeframe)
	args[FldLimit] = strconv.Itoa(limit)
	until := utils.PopMapVal(args, banexg.ParamUntil, int64(0))
	// OKX API: after=请求此时间戳之前的数据(ts < after), before=请求此时间戳之后的数据(ts > before)
	// since表示起始时间，应使用before；until表示结束时间，应使用after
	// 注意：OKX文档说明 before 单独使用时会返回最新数据，必须配合 after 使用
	// 减1ms以包含since时间点的K线（before是严格大于）
	if since > 0 {
		args[FldBefore] = strconv.FormatInt(since-1, 10)
		// 必须设置after来限制上界，否则OKX会忽略before返回最新数据
		if until <= 0 {
			tfSecs := utils.TFToSecs(timeframe)
			until = since + int64(limit*tfSecs*1000)
		}
	}
	if until > 0 {
		args[FldAfter] = strconv.FormatInt(until, 10)
	}
	method := MethodMarketGetCandles
	// history-candles 用于获取历史K线，当指定since且数据较老时使用
	// 对于最近的数据（1天内），使用regular candles以获取最新数据
	if since > 0 {
		nowMs := time.Now().UnixMilli()
		// 如果since在1天以前，使用history-candles
		if nowMs-since > 86400000 {
			method = MethodMarketGetHistoryCandles
		}
	}
	tryNum := e.GetRetryNum("FetchOHLCV", 1)
	res := requestRetry[[][]string](e, method, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	return parseOHLCV(res.Result), nil
}

func (e *OKX) FetchTickerPrice(symbol string, params map[string]interface{}) (map[string]float64, *errs.Error) {
	var symbols []string
	if symbol != "" {
		symbols = []string{symbol}
	}
	tickers, err := e.fetchTickersWithArgs(symbols, params)
	if err != nil {
		return nil, err
	}
	return banexg.TickersToPriceMap(tickers, nil), nil
}

func (e *OKX) FetchLastPrices(symbols []string, params map[string]interface{}) ([]*banexg.LastPrice, *errs.Error) {
	tickers, err := e.fetchTickersWithArgs(symbols, params)
	if err != nil {
		return nil, err
	}
	return banexg.TickersToLastPrices(tickers, nil), nil
}

func (e *OKX) loadTickersArgs(symbols []string, params map[string]interface{}) (string, string, map[string]interface{}, *errs.Error) {
	args := utils.SafeParams(params)
	marketType, contractType, err := e.LoadArgsMarketType(args, symbols...)
	if err != nil {
		return "", "", nil, err
	}
	return marketType, contractType, args, nil
}

func (e *OKX) fetchTickersWithArgs(symbols []string, params map[string]interface{}) ([]*banexg.Ticker, *errs.Error) {
	marketType, contractType, args, err := e.loadTickersArgs(symbols, params)
	if err != nil {
		return nil, err
	}
	return e.fetchTickersByType(marketType, contractType, symbols, args)
}

func (e *OKX) fetchTickersByType(marketType, contractType string, symbols []string, args map[string]interface{}) ([]*banexg.Ticker, *errs.Error) {
	instType := mapTickerInstType(marketType, contractType)
	if instType == "" {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "unsupported market: %v", marketType)
	}
	args[FldInstType] = instType
	tryNum := e.GetRetryNum("FetchTickers", 1)
	res := requestRetry[[]map[string]interface{}](e, MethodMarketGetTickers, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	items := res.Result
	arr, err := decodeResult[Ticker](items)
	if err != nil {
		return nil, err
	}
	result := make([]*banexg.Ticker, 0, len(arr))
	for i, item := range arr {
		ticker := parseTicker(e, &item, items[i], marketType)
		result = append(result, ticker)
	}
	symbolSet := banexg.BuildSymbolSet(symbols)
	return banexg.FilterTickers(result, symbolSet), nil
}

func (e *OKX) FetchOrderBook(symbol string, limit int, params map[string]interface{}) (*banexg.OrderBook, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	args[FldInstId] = market.ID
	method := MethodMarketGetBooks
	if limit > 400 {
		method = MethodMarketGetBooksFull
		if limit > 5000 {
			limit = 5000
		}
	}
	if limit > 0 {
		args[FldSz] = strconv.Itoa(limit)
	}
	tryNum := e.GetRetryNum("FetchOrderBook", 1)
	res := requestRetry[[]OrderBook](e, method, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	if len(res.Result) == 0 {
		return nil, errs.NewMsg(errs.CodeDataNotFound, "empty orderbook result")
	}
	return parseOrderBook(market, &res.Result[0], limit), nil
}

func mapTickerInstType(marketType, contractType string) string {
	if marketType == banexg.MarketMargin {
		return InstTypeSpot
	}
	return instTypeByMarket(marketType, contractType)
}

func parseTicker(e *OKX, item *Ticker, info map[string]interface{}, marketType string) *banexg.Ticker {
	if item == nil {
		return nil
	}
	symbol := e.SafeSymbol(item.InstId, "", marketType)
	if symbol == "" {
		symbol = item.InstId
	}
	bid := parseFloat(item.BidPx)
	bidVol := parseFloat(item.BidSz)
	ask := parseFloat(item.AskPx)
	askVol := parseFloat(item.AskSz)
	last := parseFloat(item.Last)
	open := parseFloat(item.Open24h)
	high := parseFloat(item.High24h)
	low := parseFloat(item.Low24h)
	vol := parseFloat(item.Vol24h)
	quoteVol := parseFloat(item.VolCcy24h)
	ts := parseInt(item.Ts)
	change := last - open
	pct := 0.0
	if open > 0 {
		pct = change / open * 100
	}
	return &banexg.Ticker{
		Symbol:      symbol,
		TimeStamp:   ts,
		Bid:         bid,
		BidVolume:   bidVol,
		Ask:         ask,
		AskVolume:   askVol,
		High:        high,
		Low:         low,
		Open:        open,
		Close:       last,
		Last:        last,
		Change:      change,
		Percentage:  pct,
		BaseVolume:  vol,
		QuoteVolume: quoteVol,
		Info:        info,
	}
}

func parseOrderBook(market *banexg.Market, ob *OrderBook, limit int) *banexg.OrderBook {
	if ob == nil || market == nil {
		return nil
	}
	asks := utils.ParseBookSide(ob.Asks, parseFloat)
	bids := utils.ParseBookSide(ob.Bids, parseFloat)
	return &banexg.OrderBook{
		Symbol:    market.Symbol,
		TimeStamp: parseInt(ob.Ts),
		Asks:      banexg.NewOdBookSide(false, len(asks), asks),
		Bids:      banexg.NewOdBookSide(true, len(bids), bids),
		Limit:     limit,
		Cache:     make([]map[string]string, 0),
	}
}

func parseOHLCV(rows [][]string) []*banexg.Kline {
	if len(rows) == 0 {
		return nil
	}
	res := make([]*banexg.Kline, 0, len(rows))
	for _, row := range rows {
		if len(row) < 6 {
			continue
		}
		// row[8] is confirm: 0=未完结, 1=已完结
		// 不再根据confirm过滤，因为OKX可能在K线周期结束后延迟更新confirm状态
		// 由调用方（如spider）根据时间戳判断K线是否完成
		stamp := parseInt(row[0])
		open := parseFloat(row[1])
		high := parseFloat(row[2])
		low := parseFloat(row[3])
		closeP := parseFloat(row[4])
		vol := parseFloat(row[5])
		info := 0.0
		if len(row) > 7 {
			info = parseFloat(row[7])
		} else if len(row) > 6 {
			info = parseFloat(row[6])
		}
		res = append(res, &banexg.Kline{
			Time:   stamp,
			Open:   open,
			High:   high,
			Low:    low,
			Close:  closeP,
			Volume: vol,
			Info:   info,
		})
	}
	// OKX返回数据是降序的（最新在前），需要反转为升序（最旧在前）以与其他交易所保持一致
	for i, j := 0, len(res)-1; i < j; i, j = i+1, j-1 {
		res[i], res[j] = res[j], res[i]
	}
	return res
}
