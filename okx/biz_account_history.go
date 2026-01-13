package okx

import (
	"strconv"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

func (e *OKX) FetchIncomeHistory(inType string, symbol string, since int64, limit int, params map[string]interface{}) ([]*banexg.Income, *errs.Error) {
	args := utils.SafeParams(params)
	if ccy := utils.PopMapVal(args, banexg.ParamCurrency, ""); ccy != "" {
		args[FldCcy] = ccy
	}
	if mgnMode := utils.PopMapVal(args, banexg.ParamMarginMode, ""); mgnMode != "" {
		args[FldMgnMode] = mgnMode
	}
	var marketType string
	var contractType string
	var market *banexg.Market
	if symbol != "" {
		var err *errs.Error
		args, market, err = e.LoadArgsMarket(symbol, args)
		if err != nil {
			return nil, err
		}
		args[FldInstId] = market.ID
		marketType = market.Type
	} else {
		var err *errs.Error
		marketType, contractType, err = e.LoadArgsMarketType(args)
		if err != nil {
			return nil, err
		}
	}
	if market != nil {
		if instType := instTypeFromMarket(market); instType != "" {
			args[FldInstType] = instType
		}
	} else if marketType != "" {
		if instType := instTypeByMarket(marketType, contractType); instType != "" {
			args[FldInstType] = instType
		}
	}
	if inType != "" {
		args[FldType] = inType
	}
	until := utils.PopMapVal(args, banexg.ParamUntil, int64(0))
	if since > 0 {
		args[FldBegin] = strconv.FormatInt(since, 10)
	}
	if until > 0 {
		args[FldEnd] = strconv.FormatInt(until, 10)
	}
	pageLimit := limit
	if pageLimit <= 0 || pageLimit > 100 {
		pageLimit = 100
	}
	args[FldLimit] = strconv.Itoa(pageLimit)
	method := pickArchiveMethod(args, since, until, MethodAccountGetBills, MethodAccountGetBillsArchive)
	after := utils.PopMapVal(args, banexg.ParamAfter, "")
	before := utils.PopMapVal(args, banexg.ParamBefore, "")
	result := make([]*banexg.Income, 0)
	for {
		if after != "" {
			args[FldAfter] = after
		} else {
			delete(args, FldAfter)
		}
		if before != "" {
			args[FldBefore] = before
		} else {
			delete(args, FldBefore)
		}
		tryNum := e.GetRetryNum("FetchIncomeHistory", 1)
		res := requestRetry[[]map[string]interface{}](e, method, args, tryNum)
		if res.Error != nil {
			return nil, res.Error
		}
		if len(res.Result) == 0 {
			break
		}
		arr, err := decodeResult[Bill](res.Result)
		if err != nil {
			return nil, err
		}
		page := make([]*banexg.Income, 0, len(arr))
		for i, item := range arr {
			inc := parseIncome(e, &item, res.Result[i])
			if inc == nil {
				continue
			}
			if symbol != "" && inc.Symbol != symbol && inc.Symbol != "" {
				continue
			}
			page = append(page, inc)
		}
		result = append(result, page...)
		if limit > 0 && len(result) >= limit {
			return result[:limit], nil
		}
		if len(arr) < pageLimit {
			break
		}
		nextAfter := arr[len(arr)-1].BillId
		if nextAfter == "" || nextAfter == after {
			break
		}
		after = nextAfter
	}
	return result, nil
}

// isValidFundingRateMarket checks if the market supports funding rate
func isValidFundingRateMarket(market *banexg.Market) bool {
	return market.Swap || market.Type == banexg.MarketLinear || market.Type == banexg.MarketInverse
}

func (e *OKX) FetchFundingRate(symbol string, params map[string]interface{}) (*banexg.FundingRateCur, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	if !isValidFundingRateMarket(market) {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "funding rate only supports swap")
	}
	args[FldInstId] = market.ID
	tryNum := e.GetRetryNum("FetchFundingRate", 1)
	res := requestRetry[[]map[string]interface{}](e, MethodPublicGetFundingRate, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	if len(res.Result) == 0 {
		return nil, errs.NewMsg(errs.CodeDataNotFound, "empty funding rate result")
	}
	arr, err := decodeResult[FundingRate](res.Result)
	if err != nil {
		return nil, err
	}
	return parseFundingRate(e, &arr[0], res.Result[0]), nil
}

func (e *OKX) FetchFundingRates(symbols []string, params map[string]interface{}) ([]*banexg.FundingRateCur, *errs.Error) {
	// Fetch all funding rates when no symbols specified
	if len(symbols) == 0 {
		args := utils.SafeParams(params)
		marketType, contractType, err := e.LoadArgsMarketType(args)
		if err != nil {
			return nil, err
		}
		if contractType == banexg.MarketFuture || marketType == banexg.MarketFuture {
			return nil, errs.NewMsg(errs.CodeParamInvalid, "funding rates only supports swap")
		}
		if marketType != "" && marketType != banexg.MarketLinear && marketType != banexg.MarketInverse {
			return nil, errs.NewMsg(errs.CodeParamInvalid, "funding rates only supports swap")
		}
		args[FldInstId] = InstIdAny
		tryNum := e.GetRetryNum("FetchFundingRates", 1)
		res := requestRetry[[]map[string]interface{}](e, MethodPublicGetFundingRate, args, tryNum)
		if res.Error != nil {
			return nil, res.Error
		}
		arr, err := decodeResult[FundingRate](res.Result)
		if err != nil {
			return nil, err
		}
		result := make([]*banexg.FundingRateCur, 0, len(arr))
		for i, item := range arr {
			cur := parseFundingRate(e, &item, res.Result[i])
			if cur != nil {
				result = append(result, cur)
			}
		}
		return result, nil
	}
	// Fetch funding rates for each symbol by reusing FetchFundingRate
	result := make([]*banexg.FundingRateCur, 0, len(symbols))
	for _, symbol := range symbols {
		cur, err := e.FetchFundingRate(symbol, params)
		if err != nil {
			return nil, err
		}
		if cur != nil {
			result = append(result, cur)
		}
	}
	return result, nil
}

func (e *OKX) FetchFundingRateHistory(symbol string, since int64, limit int, params map[string]interface{}) ([]*banexg.FundingRate, *errs.Error) {
	if symbol == "" {
		return nil, errs.NewMsg(errs.CodeParamRequired, "symbol is required for okx FetchFundingRateHistory")
	}
	pageLimit := limit
	if pageLimit <= 0 {
		pageLimit = 400
	}
	if pageLimit > 400 {
		pageLimit = 400
	}
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	if !isValidFundingRateMarket(market) {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "funding rate only supports swap")
	}
	args[FldInstId] = market.ID
	args[FldLimit] = strconv.Itoa(pageLimit)
	until := utils.PopMapVal(args, banexg.ParamUntil, int64(0))
	after := utils.PopMapVal(args, banexg.ParamAfter, int64(0))
	before := utils.PopMapVal(args, banexg.ParamBefore, int64(0))
	if after == 0 {
		after = until
	}
	if before == 0 {
		before = since
	}
	if since > 0 {
		args[FldBefore] = strconv.FormatInt(before, 10)
	}
	if until > 0 {
		args[FldAfter] = strconv.FormatInt(after, 10)
	}
	result := make([]*banexg.FundingRate, 0)
	for {
		curAfter := after
		if after > 0 {
			args[FldAfter] = strconv.FormatInt(after, 10)
		} else {
			delete(args, FldAfter)
		}
		if before > 0 {
			args[FldBefore] = strconv.FormatInt(before, 10)
		} else {
			delete(args, FldBefore)
		}
		tryNum := e.GetRetryNum("FetchFundingRateHistory", 1)
		res := requestRetry[[]map[string]interface{}](e, MethodPublicGetFundingRateHistory, args, tryNum)
		if res.Error != nil {
			return nil, res.Error
		}
		arr, err := decodeResult[FundingRateHistory](res.Result)
		if err != nil {
			return nil, err
		}
		page := make([]*banexg.FundingRate, 0, len(arr))
		for i, item := range arr {
			his := parseFundingRateHistory(e, &item, res.Result[i])
			if his != nil {
				page = append(page, his)
			}
		}
		result = append(result, page...)
		nextAfter, ok := shouldContinueFundRateHistory(page, pageLimit, since, curAfter)
		if !ok {
			break
		}
		after = nextAfter
	}
	return result, nil
}

func shouldContinueFundRateHistory(list []*banexg.FundingRate, pageLimit int, since int64, lastAfter int64) (int64, bool) {
	if len(list) == 0 {
		return lastAfter, false
	}
	if len(list) < pageLimit {
		return lastAfter, false
	}
	nextAfter := list[len(list)-1].Timestamp
	if nextAfter <= 0 || nextAfter == lastAfter {
		return nextAfter, false
	}
	if since > 0 && nextAfter <= since {
		return nextAfter, false
	}
	return nextAfter, true
}

func parseIncome(e *OKX, bill *Bill, info map[string]interface{}) *banexg.Income {
	if bill == nil {
		return nil
	}
	income := parseFloat(bill.BalChg)
	if bill.BalChg == "" {
		if bill.Pnl != "" {
			income = parseFloat(bill.Pnl)
		} else if bill.Fee != "" {
			income = parseFloat(bill.Fee)
		}
	}
	marketType := parseMarketType(bill.InstType, "")
	symbol := bill.InstId
	if e != nil && bill.InstId != "" {
		if safe := e.SafeSymbol(bill.InstId, "", marketType); safe != "" {
			symbol = safe
		}
	}
	infoStr := bill.SubType
	if infoStr == "" {
		infoStr = bill.Notes
	}
	asset := bill.Ccy
	if e != nil {
		asset = e.SafeCurrencyCode(bill.Ccy)
	}
	return &banexg.Income{
		Symbol:     symbol,
		IncomeType: bill.Type,
		Income:     income,
		Asset:      asset,
		Info:       infoStr,
		Time:       parseInt(bill.Ts),
		TranID:     bill.BillId,
		TradeID:    bill.TradeId,
	}
}

func parseFundingRate(e *OKX, item *FundingRate, info map[string]interface{}) *banexg.FundingRateCur {
	if item == nil {
		return nil
	}
	marketType := ""
	if market := getMarketByIDAny(e, item.InstId, marketType); market != nil {
		marketType = market.Type
	}
	symbol := item.InstId
	if marketType != "" {
		symbol = e.SafeSymbol(item.InstId, "", marketType)
	}
	return &banexg.FundingRateCur{
		Symbol:               symbol,
		FundingRate:          parseFloat(item.FundingRate),
		Timestamp:            parseInt(item.Ts),
		Info:                 info,
		FundingTimestamp:     parseInt(item.FundingTime),
		NextFundingRate:      parseFloat(item.NextFundingRate),
		NextFundingTimestamp: parseInt(item.NextFundingTime),
		InterestRate:         parseFloat(item.InterestRate),
	}
}

func parseFundingRateHistory(e *OKX, item *FundingRateHistory, info map[string]interface{}) *banexg.FundingRate {
	if item == nil {
		return nil
	}
	marketType := ""
	if market := getMarketByIDAny(e, item.InstId, marketType); market != nil {
		marketType = market.Type
	}
	symbol := item.InstId
	if marketType != "" {
		symbol = e.SafeSymbol(item.InstId, "", marketType)
	}
	return &banexg.FundingRate{
		Symbol:      symbol,
		FundingRate: parseFloat(item.FundingRate),
		Timestamp:   parseInt(item.FundingTime),
		Info:        info,
	}
}
