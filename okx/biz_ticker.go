package okx

import (
	"strconv"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

func (e *OKX) FetchTickers(symbols []string, params map[string]interface{}) ([]*banexg.Ticker, *errs.Error) {
	args := utils.SafeParams(params)
	marketType, contractType, err := e.LoadArgsMarketType(args, symbols...)
	if err != nil {
		return nil, err
	}
	return e.fetchTickersByType(marketType, contractType, symbols, args)
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
	// 减1ms以包含since时间点的K线（before是严格大于）
	if since > 0 {
		args[FldBefore] = strconv.FormatInt(since-1, 10)
	}
	if until > 0 {
		args[FldAfter] = strconv.FormatInt(until, 10)
	}
	tryNum := e.GetRetryNum("FetchOHLCV", 1)
	res := requestRetry[[][]string](e, MethodMarketGetCandles, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	return parseOHLCV(res.Result), nil
}

func (e *OKX) FetchTickerPrice(symbol string, params map[string]interface{}) (map[string]float64, *errs.Error) {
	args := utils.SafeParams(params)
	marketType, contractType, err := e.LoadArgsMarketType(args, symbol)
	if err != nil {
		return nil, err
	}
	var symbols []string
	if symbol != "" {
		symbols = []string{symbol}
	}
	tickers, err := e.fetchTickersByType(marketType, contractType, symbols, args)
	if err != nil {
		return nil, err
	}
	return tickersToPriceMap(tickers), nil
}

func (e *OKX) FetchLastPrices(symbols []string, params map[string]interface{}) ([]*banexg.LastPrice, *errs.Error) {
	args := utils.SafeParams(params)
	marketType, contractType, err := e.LoadArgsMarketType(args, symbols...)
	if err != nil {
		return nil, err
	}
	tickers, err := e.fetchTickersByType(marketType, contractType, symbols, args)
	if err != nil {
		return nil, err
	}
	return tickersToLastPrices(tickers), nil
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
	var symbolSet map[string]struct{}
	if len(symbols) > 0 {
		symbolSet = make(map[string]struct{}, len(symbols))
		for _, s := range symbols {
			symbolSet[s] = struct{}{}
		}
	}
	for i, item := range arr {
		ticker := parseTicker(e, &item, items[i], marketType)
		if ticker == nil {
			continue
		}
		if symbolSet != nil {
			if _, ok := symbolSet[ticker.Symbol]; !ok {
				continue
			}
		}
		result = append(result, ticker)
	}
	return result, nil
}

func tickersToLastPrices(tickers []*banexg.Ticker) []*banexg.LastPrice {
	if len(tickers) == 0 {
		return nil
	}
	result := make([]*banexg.LastPrice, 0, len(tickers))
	for _, ticker := range tickers {
		if ticker == nil || ticker.Symbol == "" {
			continue
		}
		result = append(result, &banexg.LastPrice{
			Symbol:    ticker.Symbol,
			Timestamp: ticker.TimeStamp,
			Price:     ticker.Last,
			Info:      ticker.Info,
		})
	}
	return result
}

func tickersToPriceMap(tickers []*banexg.Ticker) map[string]float64 {
	result := make(map[string]float64)
	for _, ticker := range tickers {
		if ticker == nil || ticker.Symbol == "" {
			continue
		}
		result[ticker.Symbol] = ticker.Last
	}
	return result
}

func (e *OKX) FetchOrderBook(symbol string, limit int, params map[string]interface{}) (*banexg.OrderBook, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	args[FldInstId] = market.ID
	if limit > 0 {
		if limit > 400 {
			limit = 400
		}
		args[FldSz] = strconv.Itoa(limit)
	}
	tryNum := e.GetRetryNum("FetchOrderBook", 1)
	res := requestRetry[[]OrderBook](e, MethodMarketGetBooks, args, tryNum)
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
	asks := parseBookSide(ob.Asks)
	bids := parseBookSide(ob.Bids)
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

func parseBookSide(levels [][]string) [][2]float64 {
	if len(levels) == 0 {
		return nil
	}
	res := make([][2]float64, 0, len(levels))
	for _, lvl := range levels {
		if len(lvl) < 2 {
			continue
		}
		res = append(res, [2]float64{parseFloat(lvl[0]), parseFloat(lvl[1])})
	}
	return res
}
