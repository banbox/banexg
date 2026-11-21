package bybit

import (
	"context"
	"math"
	"sort"
	"strconv"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

func (e *Bybit) FetchOHLCV(symbol, timeframe string, since int64, limit int, params map[string]interface{}) ([]*banexg.Kline, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	price := utils.PopMapVal(args, "price", "")
	switch price {
	case "premium", "premium_index":
		price = "premiumIndex"
	case "", "mark", "index", "premiumIndex":
	default:
		return nil, errs.NewMsg(errs.CodeParamInvalid, "unsupported price kline type: %s", price)
	}
	if market.Option && price != "mark" {
		return nil, errs.NewMsg(errs.CodeNotSupport, "option market only supports mark price kline")
	}
	if market.Spot && price != "" {
		return nil, errs.NewMsg(errs.CodeNotSupport, "spot market does not support %s price kline", price)
	}
	args["symbol"] = market.ID
	maxLimit := 1000
	if market.Option {
		maxLimit = 500
	}
	if limit <= 0 {
		limit = 200
	} else if limit > maxLimit {
		limit = maxLimit
	}
	args["limit"] = limit
	if since > 0 {
		args["start"] = since
	}
	until := utils.PopMapVal(args, banexg.ParamUntil, int64(0))
	if until > 0 {
		args["end"] = until
	}
	interval := e.GetTimeFrame(timeframe)
	if interval == "" {
		return nil, errs.NewMsg(errs.CodeInvalidTimeFrame, "invalid timeframe")
	}
	args["interval"] = interval
	category, err := bybitCategoryFromMarket(market)
	if err != nil {
		return nil, err
	}
	args["category"] = category
	var method string
	if market.Spot {
		method = MethodPublicGetV5MarketKline
	} else {
		if price == "mark" {
			method = MethodPublicGetV5MarketMarkPriceKline
		} else if price == "index" {
			method = MethodPublicGetV5MarketIndexPriceKline
		} else if price == "premiumIndex" {
			method = MethodPublicGetV5MarketPremiumIndexPriceKline
		} else {
			method = MethodPublicGetV5MarketKline
		}
	}
	tryNum := e.GetRetryNum("FetchOHLCV", 1)
	rsp := requestRetry[struct {
		Symbol   string     `json:"symbol"`
		Category string     `json:"category"`
		List     [][]string `json:"list"`
	}](e, method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	return parseBybitOHLCV(rsp.Result.List), nil
}

const maxFundRateBatch = 200

func bybitFundingIntervalMS(market *banexg.Market) int64 {
	if market == nil || market.Info == nil {
		return 0
	}
	if raw, ok := market.Info["fundingInterval"]; ok {
		if mins := parseBybitInt(raw); mins > 0 {
			return mins * 60 * 1000
		}
	}
	return 0
}

func (e *Bybit) FetchFundingRateHistory(symbol string, since int64, limit int, params map[string]interface{}) ([]*banexg.FundingRate, *errs.Error) {
	if symbol == "" {
		return nil, errs.NewMsg(errs.CodeParamRequired, "symbol is required for bybit FetchFundingRateHistory")
	}
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	if !market.Swap || (market.Type != banexg.MarketLinear && market.Type != banexg.MarketInverse) {
		return nil, errs.NewMsg(errs.CodeNotSupport, "only linear/inverse swap support")
	}
	args["symbol"] = market.ID
	args["category"] = market.Type
	until := utils.PopMapVal(args, banexg.ParamUntil, int64(0))
	if since > 0 && until > 0 && since > until {
		return []*banexg.FundingRate{}, nil
	}
	unlimited := limit <= 0 && since > 0 && until > 0
	if !unlimited && limit <= 0 {
		limit = maxFundRateBatch
	}
	interval := bybitFundingIntervalMS(market)
	if since > 0 {
		args["startTime"] = since
		if until <= 0 {
			if interval <= 0 {
				return nil, errs.NewMsg(errs.CodeParamRequired, "endTime required when fundingInterval is unavailable")
			}
			until = since + int64(limit)*interval
		}
	}
	if until > 0 {
		args["endTime"] = until
	}
	itemsByTimestamp := make(map[int64]*banexg.FundingRate, max(limit, 0))
	for {
		batchLimit := maxFundRateBatch
		if !unlimited {
			remaining := limit - len(itemsByTimestamp)
			if remaining <= 0 {
				break
			}
			batchLimit = min(remaining, maxFundRateBatch)
		}
		args["limit"] = batchLimit
		list, hasMore, _, err := e.getFundRateHis(market.Type, until, interval, args)
		if err != nil {
			return nil, err
		}
		oldest := int64(0)
		for _, item := range list {
			if item.Timestamp < since || (until > 0 && item.Timestamp > until) {
				continue
			}
			itemsByTimestamp[item.Timestamp] = item
			if oldest == 0 || item.Timestamp < oldest {
				oldest = item.Timestamp
			}
		}
		if !hasMore || oldest == 0 || (since > 0 && oldest <= since) {
			break
		}
		nextEnd := oldest - 1
		if currentEnd, ok := args["endTime"].(int64); ok && nextEnd >= currentEnd {
			break
		}
		args["endTime"] = nextEnd
	}
	items := make([]*banexg.FundingRate, 0, len(itemsByTimestamp))
	for _, item := range itemsByTimestamp {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Timestamp < items[j].Timestamp })
	if !unlimited && len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items, nil
}

func (e *Bybit) getFundRateHis(marketType string, until int64, interval int64, args map[string]interface{}) ([]*banexg.FundingRate, bool, int64, *errs.Error) {
	method := MethodPublicGetV5MarketFundingHistory
	tryNum := e.GetRetryNum("FetchFundingRateHistory", 1)
	rsp := requestRetry[struct {
		Category string                   `json:"category"`
		List     []map[string]interface{} `json:"list"`
	}](e, method, args, tryNum)
	if rsp.Error != nil {
		return nil, false, 0, rsp.Error
	}
	arr := rsp.Result.List
	items, err := decodeBybitList[*FundRate](arr)
	if err != nil {
		return nil, false, 0, err
	}
	var list = make([]*banexg.FundingRate, 0, len(rsp.Result.List))
	for i, it := range items {
		code := bybitSafeSymbol(e, it.Symbol, marketType)
		stamp, _ := strconv.ParseInt(it.FundingRateTimestamp, 10, 64)
		if code == "" {
			continue
		}
		rate, _ := strconv.ParseFloat(it.FundingRate, 64)
		list = append(list, &banexg.FundingRate{
			Symbol:      code,
			FundingRate: rate,
			Timestamp:   stamp,
			Info:        arr[i],
		})
	}
	usedInterval := interval
	if len(list) >= 2 {
		usedInterval = list[1].Timestamp - list[0].Timestamp
		if usedInterval <= 0 {
			usedInterval = interval
		}
	}
	if usedInterval <= 0 {
		usedInterval = int64(60 * 60 * 8 * 1000)
	}
	requestLimit := utils.GetMapVal(args, "limit", maxFundRateBatch)
	hasMore := len(rsp.Result.List) == requestLimit
	return list, hasMore, usedInterval, nil
}

type orderBookSnapshot struct {
	Symbol string     `json:"s"`
	Bids   [][]string `json:"b"`
	Asks   [][]string `json:"a"`
	Ts     int64      `json:"ts"`
	Update int64      `json:"u"`
	Seq    int64      `json:"seq"`
	Cts    int64      `json:"cts"`
}

func bybitCategoryFromMarket(market *banexg.Market) (string, *errs.Error) {
	if market == nil {
		return "", errs.NewMsg(errs.CodeParamInvalid, "market is required")
	}
	if market.Option {
		return banexg.MarketOption, nil
	}
	if market.Linear {
		return banexg.MarketLinear, nil
	}
	if market.Inverse {
		return banexg.MarketInverse, nil
	}
	if market.Spot || market.Type == banexg.MarketMargin || market.Type == "" {
		return banexg.MarketSpot, nil
	}
	return "", errs.NewMsg(errs.CodeParamInvalid, "unsupported market: %v", market.Type)
}

func bybitCategoryFromType(marketType string) (string, *errs.Error) {
	switch marketType {
	case banexg.MarketLinear, banexg.MarketInverse, banexg.MarketOption, banexg.MarketSpot, banexg.MarketMargin:
		if marketType == banexg.MarketMargin {
			return banexg.MarketSpot, nil
		}
		return marketType, nil
	default:
		if marketType == "" {
			return "", errs.NewMsg(errs.CodeParamInvalid, "market type is required")
		}
		return "", errs.NewMsg(errs.CodeParamInvalid, "unsupported market: %v", marketType)
	}
}

func bybitOrderBookLimit(category string, limit int) int {
	if limit <= 0 {
		return 0
	}
	maxLimit := 1000
	switch category {
	case banexg.MarketOption:
		maxLimit = 25
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if limit < 1 {
		limit = 1
	}
	return limit
}

func bybitParseBookSide(levels [][]string) [][2]float64 {
	if len(levels) == 0 {
		return nil
	}
	return utils.ParseBookSide(levels, func(val string) float64 {
		return parseBybitNum(val)
	})
}

func parseBybitOrderBook(market *banexg.Market, ob *orderBookSnapshot, limit int) *banexg.OrderBook {
	if market == nil || ob == nil {
		return nil
	}
	asks := bybitParseBookSide(ob.Asks)
	bids := bybitParseBookSide(ob.Bids)
	return &banexg.OrderBook{
		Symbol:    market.Symbol,
		TimeStamp: ob.Ts,
		Nonce:     ob.Update,
		Asks:      banexg.NewOdBookSide(false, len(asks), asks),
		Bids:      banexg.NewOdBookSide(true, len(bids), bids),
		Limit:     limit,
		Cache:     make([]map[string]string, 0),
	}
}

func parseBybitOHLCV(rows [][]string) []*banexg.Kline {
	if len(rows) == 0 {
		return nil
	}
	res := make([]*banexg.Kline, 0, len(rows))
	for _, row := range rows {
		if len(row) < 5 {
			continue
		}
		stamp := parseBybitInt(row[0])
		open := parseBybitNum(row[1])
		high := parseBybitNum(row[2])
		low := parseBybitNum(row[3])
		closeP := parseBybitNum(row[4])
		vol := 0.0
		info := 0.0
		if len(row) > 5 {
			vol = parseBybitNum(row[5])
		}
		if len(row) > 6 {
			info = parseBybitNum(row[6])
		}
		res = append(res, &banexg.Kline{
			Time:   stamp,
			Open:   open,
			High:   high,
			Low:    low,
			Close:  closeP,
			Volume: vol,
			Quote:  info,
		})
	}
	for i, j := 0, len(res)-1; i < j; i, j = i+1, j-1 {
		res[i], res[j] = res[j], res[i]
	}
	return res
}

func setBybitSymbolArg(e *Bybit, args map[string]interface{}, symbols []string) *errs.Error {
	if len(symbols) != 1 {
		return nil
	}
	if _, ok := args["symbol"]; ok {
		return nil
	}
	market, err := e.GetMarket(symbols[0])
	if err != nil {
		return err
	}
	args["symbol"] = market.ID
	return nil
}

func (e *Bybit) FetchLastPrices(symbols []string, params map[string]interface{}) ([]*banexg.LastPrice, *errs.Error) {
	tickers, symbolSet, err := e.fetchTickersWithArgs("FetchLastPrices", symbols, params)
	if err != nil {
		return nil, err
	}
	return banexg.TickersToLastPrices(tickers, symbolSet), nil
}

func (e *Bybit) FetchOrderBook(symbol string, limit int, params map[string]interface{}) (*banexg.OrderBook, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	args["symbol"] = market.ID
	category, err := bybitCategoryFromMarket(market)
	if err != nil {
		return nil, err
	}
	args["category"] = category
	limit = bybitOrderBookLimit(category, limit)
	if limit > 0 {
		args["limit"] = limit
	}
	tryNum := e.GetRetryNum("FetchOrderBook", 1)
	rsp := requestRetry[orderBookSnapshot](e, MethodPublicGetV5MarketOrderbook, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	book := parseBybitOrderBook(market, &rsp.Result, limit)
	if book != nil && book.TimeStamp == 0 {
		book.TimeStamp = e.MilliSeconds()
	}
	return book, nil
}

func (e *Bybit) FetchFundingRate(symbol string, params map[string]interface{}) (*banexg.FundingRateCur, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	if !market.Swap || (market.Type != banexg.MarketLinear && market.Type != banexg.MarketInverse) {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "funding rate only supports swap")
	}
	args["symbol"] = market.ID
	category, err := bybitCategoryFromMarket(market)
	if err != nil {
		return nil, err
	}
	args["category"] = category
	tryNum := e.GetRetryNum("FetchFundingRate", 1)
	list, err := parseFundingRates(e, market.Type, args, tryNum, nil)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, errs.NewMsg(errs.CodeDataNotFound, "empty funding rate result")
	}
	return list[0], nil
}

func (e *Bybit) FetchFundingRates(symbols []string, params map[string]interface{}) ([]*banexg.FundingRateCur, *errs.Error) {
	if len(symbols) == 1 {
		item, err := e.FetchFundingRate(symbols[0], params)
		if err != nil {
			return nil, err
		}
		if item == nil {
			return nil, nil
		}
		return []*banexg.FundingRateCur{item}, nil
	}
	args := utils.SafeParams(params)
	marketType, _, err := e.LoadArgsMarketType(args, symbols...)
	if err != nil {
		return nil, err
	}
	if marketType != banexg.MarketLinear && marketType != banexg.MarketInverse {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "funding rate only supports linear/inverse")
	}
	category, err := bybitCategoryFromType(marketType)
	if err != nil {
		return nil, err
	}
	// Normalize requested symbols under marketType, so callers can pass spot symbols with ParamMarket=linear.
	// Also reject mixed market types early to avoid silent empty results.
	wantSymbols := symbols
	if len(symbols) > 0 {
		wantSymbols = make([]string, 0, len(symbols))
		for _, sym := range symbols {
			market, err := e.GetArgsMarket(sym, map[string]interface{}{banexg.ParamMarket: marketType})
			if err != nil {
				return nil, err
			}
			if market == nil || !market.Swap {
				return nil, errs.NewMsg(errs.CodeParamInvalid, "funding rate only supports swap")
			}
			if market.Type != marketType {
				return nil, errs.NewMsg(errs.CodeParamInvalid, "mixed market types in symbols: want=%s got=%s sym=%s", marketType, market.Type, sym)
			}
			wantSymbols = append(wantSymbols, market.Symbol)
		}
	}
	args["category"] = category
	tryNum := e.GetRetryNum("FetchFundingRates", 1)
	symbolSet := banexg.BuildSymbolSet(wantSymbols)
	return parseFundingRates(e, marketType, args, tryNum, symbolSet)
}

func parseFundingRates(e *Bybit, marketType string, args map[string]interface{}, tryNum int, symbolSet map[string]struct{}) ([]*banexg.FundingRateCur, *errs.Error) {
	rsp := requestRetry[V5ListResult](e, MethodPublicGetV5MarketTickers, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	items := rsp.Result.List
	arr, err := decodeBybitList[*FutureTicker](items)
	if err != nil {
		return nil, err
	}
	now := e.MilliSeconds()
	result := make([]*banexg.FundingRateCur, 0, len(arr))
	for i, item := range arr {
		if item == nil {
			continue
		}
		symbol := bybitSafeSymbol(e, item.Symbol, marketType)
		if symbol == "" {
			continue
		}
		if symbolSet != nil {
			if _, ok := symbolSet[symbol]; !ok {
				continue
			}
		}
		if market, err := e.GetMarket(symbol); err == nil && !market.Swap {
			continue
		}
		result = append(result, &banexg.FundingRateCur{
			Symbol:               symbol,
			FundingRate:          parseBybitNum(item.FundingRate),
			Timestamp:            now,
			MarkPrice:            parseBybitNum(item.MarkPrice),
			IndexPrice:           parseBybitNum(item.IndexPrice),
			NextFundingTimestamp: parseBybitInt(item.NextFundingTime),
			Info:                 items[i],
		})
	}
	return result, nil
}

func bybitOpenInterestInterval(timeframe string) (string, bool) {
	switch timeframe {
	case "5m":
		return "5min", true
	case "15m":
		return "15min", true
	case "30m":
		return "30min", true
	case "1h":
		return "1h", true
	case "4h":
		return "4h", true
	case "1d":
		return "1d", true
	default:
		return "", false
	}
}

// FetchDerivativesMetrics fetches historical open interest and global account
// long/short ratio snapshots on Bybit V5 linear/inverse contracts. Metrics not
// exposed by Bybit V5 remain math.NaN() so downstream stores persist NULL.
func (e *Bybit) FetchDerivativesMetrics(ctx context.Context, symbol, timeframe string, startMS, endMS int64, limit int, params map[string]interface{}) ([]*banexg.DerivativesMetric, bool, *errs.Error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, false, errs.New(errs.CodeTimeout, ctx.Err())
	}
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, false, err
	}
	if market == nil || (!market.Linear && !market.Inverse) || market.Spot || market.Option || market.Future {
		return nil, false, nil
	}
	intervalStr, ok := bybitOpenInterestInterval(timeframe)
	if !ok {
		return nil, false, errs.NewMsg(errs.CodeInvalidTimeFrame, "unsupported bybit open interest timeframe: %s", timeframe)
	}
	delete(args, banexg.ParamContext)
	delete(args, "fields")
	if until := utils.PopMapVal(args, banexg.ParamUntil, int64(0)); until > 0 && endMS <= 0 {
		endMS = until
	}
	if startMS > 0 && endMS > 0 && startMS > endMS {
		return []*banexg.DerivativesMetric{}, true, nil
	}
	category, catErr := bybitCategoryFromMarket(market)
	if catErr != nil {
		return nil, false, catErr
	}

	byTS := make(map[int64]*banexg.DerivativesMetric)
	if err := e.fetchBybitOpenInterestPages(ctx, market, category, intervalStr, startMS, endMS, limit, args, byTS); err != nil {
		return nil, true, err
	}
	_ = e.fetchBybitAccountRatioPages(ctx, market, category, intervalStr, startMS, endMS, limit, args, byTS)

	items := make([]*banexg.DerivativesMetric, 0, len(byTS))
	for _, item := range byTS {
		if startMS > 0 && item.Timestamp < startMS {
			continue
		}
		if endMS > 0 && item.Timestamp > endMS {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Timestamp < items[j].Timestamp })
	if limit > 0 && len(items) > limit {
		if startMS <= 0 && endMS > 0 {
			items = items[len(items)-limit:]
		} else {
			items = items[:limit]
		}
	}
	return items, true, nil
}

func (e *Bybit) fetchBybitOpenInterestPages(ctx context.Context, market *banexg.Market, category, intervalStr string, startMS, endMS int64, limit int, baseArgs map[string]interface{}, byTS map[int64]*banexg.DerivativesMetric) *errs.Error {
	const maxBatch = 200
	cursor := ""
	seen := make(map[string]bool)
	for {
		if ctx != nil && ctx.Err() != nil {
			return errs.New(errs.CodeTimeout, ctx.Err())
		}
		batchLimit := maxBatch
		if limit > 0 {
			rem := limit - len(byTS)
			if rem <= 0 {
				break
			}
			batchLimit = min(rem, maxBatch)
		}
		reqArgs := utils.SafeParams(baseArgs)
		reqArgs["category"] = category
		reqArgs["symbol"] = market.ID
		reqArgs["intervalTime"] = intervalStr
		reqArgs["limit"] = batchLimit
		reqArgs[banexg.ParamNoCache] = true
		if startMS > 0 {
			reqArgs["startTime"] = startMS
		}
		if endMS > 0 {
			reqArgs["endTime"] = endMS
		}
		setV5Cursor(reqArgs, cursor)
		tryNum := e.GetRetryNum("FetchDerivativesMetrics", 1)
		rsp := requestRetry[V5ListResult](e, MethodPublicGetV5MarketOpenInterest, reqArgs, tryNum)
		if rsp.Error != nil {
			return rsp.Error
		}
		if len(rsp.Result.List) == 0 {
			break
		}
		var oldest int64
		for _, raw := range rsp.Result.List {
			ts := parseBybitInt(raw["timestamp"])
			if ts <= 0 {
				continue
			}
			if oldest == 0 || ts < oldest {
				oldest = ts
			}
			oi, oiErr := utils.ParseNum(raw["openInterest"])
			if oiErr != nil || math.IsNaN(oi) || math.IsInf(oi, 0) {
				continue
			}
			dst := byTS[ts]
			if dst == nil {
				dst = banexg.NewDerivativesMetric(market.Symbol, ts)
				dst.Info = make(map[string]interface{})
				byTS[ts] = dst
			}
			dst.OpenInterest = oi
			for k, v := range raw {
				dst.Info[k] = v
			}
		}
		next := rsp.Result.NextPageCursor
		if next == "" || seen[next] || len(rsp.Result.List) < batchLimit || (startMS > 0 && oldest > 0 && oldest <= startMS) {
			break
		}
		seen[next] = true
		cursor = next
	}
	return nil
}

func (e *Bybit) fetchBybitAccountRatioPages(ctx context.Context, market *banexg.Market, category, periodStr string, startMS, endMS int64, limit int, baseArgs map[string]interface{}, byTS map[int64]*banexg.DerivativesMetric) *errs.Error {
	const maxBatch = 500
	cursor := ""
	seen := make(map[string]bool)
	fetched := 0
	for {
		if ctx != nil && ctx.Err() != nil {
			return errs.New(errs.CodeTimeout, ctx.Err())
		}
		batchLimit := maxBatch
		if limit > 0 {
			rem := limit - fetched
			if rem <= 0 {
				break
			}
			batchLimit = min(rem, maxBatch)
		}
		reqArgs := utils.SafeParams(baseArgs)
		reqArgs["category"] = category
		reqArgs["symbol"] = market.ID
		reqArgs["period"] = periodStr
		reqArgs["limit"] = batchLimit
		reqArgs[banexg.ParamNoCache] = true
		if startMS > 0 {
			reqArgs["startTime"] = startMS
		}
		if endMS > 0 {
			reqArgs["endTime"] = endMS
		}
		setV5Cursor(reqArgs, cursor)
		tryNum := e.GetRetryNum("FetchDerivativesMetrics", 1)
		rsp := requestRetry[V5ListResult](e, MethodPublicGetV5MarketAccountRatio, reqArgs, tryNum)
		if rsp.Error != nil {
			return rsp.Error
		}
		if len(rsp.Result.List) == 0 {
			break
		}
		var oldest int64
		for _, raw := range rsp.Result.List {
			ts := parseBybitInt(raw["timestamp"])
			if ts <= 0 {
				continue
			}
			if oldest == 0 || ts < oldest {
				oldest = ts
			}
			buyRatio, bErr := utils.ParseNum(raw["buyRatio"])
			sellRatio, sErr := utils.ParseNum(raw["sellRatio"])
			if bErr != nil || sErr != nil || sellRatio <= 0 {
				continue
			}
			ratio := buyRatio / sellRatio
			if math.IsNaN(ratio) || math.IsInf(ratio, 0) {
				continue
			}
			dst := byTS[ts]
			if dst == nil {
				dst = banexg.NewDerivativesMetric(market.Symbol, ts)
				dst.Info = make(map[string]interface{})
				byTS[ts] = dst
			}
			dst.GlobalLongShortAccountRatio = ratio
			fetched++
		}
		next := rsp.Result.NextPageCursor
		if next == "" || seen[next] || len(rsp.Result.List) < batchLimit || (startMS > 0 && oldest > 0 && oldest <= startMS) {
			break
		}
		seen[next] = true
		cursor = next
	}
	return nil
}
