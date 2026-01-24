package bybit

import (
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

func (e *Bybit) FetchTickers(symbols []string, params map[string]interface{}) ([]*banexg.Ticker, *errs.Error) {
	tickers, symbolSet, err := e.fetchTickersWithArgs("FetchTickers", symbols, params)
	if err != nil {
		return nil, err
	}
	if symbolSet == nil {
		return tickers, nil
	}
	return banexg.FilterTickers(tickers, symbolSet), nil
}

func (e *Bybit) FetchTicker(symbol string, params map[string]interface{}) (*banexg.Ticker, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	args["symbol"] = market.ID
	var items []*banexg.Ticker
	items, err = e.fetchTickers("FetchTicker", market.Type, args)
	if len(items) > 0 {
		return items[0], nil
	}
	return nil, err
}

func (e *Bybit) FetchTickerPrice(symbol string, params map[string]interface{}) (map[string]float64, *errs.Error) {
	var symbols []string
	if symbol != "" {
		symbols = []string{symbol}
	}
	tickers, symbolSet, err := e.fetchTickersWithArgs("FetchTickerPrice", symbols, params)
	if err != nil {
		return nil, err
	}
	return banexg.TickersToPriceMap(tickers, symbolSet), nil
}

func (e *Bybit) loadTickersArgs(symbols []string, params map[string]interface{}) (string, map[string]interface{}, map[string]struct{}, *errs.Error) {
	args := utils.SafeParams(params)
	marketType, _, err := e.LoadArgsMarketType(args, symbols...)
	if err != nil {
		return "", nil, nil, err
	}
	if err := setBybitSymbolArg(e, args, symbols); err != nil {
		return "", nil, nil, err
	}
	symbolSet := banexg.BuildSymbolSet(symbols)
	return marketType, args, symbolSet, nil
}

func (e *Bybit) fetchTickersWithArgs(opName string, symbols []string, params map[string]interface{}) ([]*banexg.Ticker, map[string]struct{}, *errs.Error) {
	marketType, args, symbolSet, err := e.loadTickersArgs(symbols, params)
	if err != nil {
		return nil, nil, err
	}
	if marketType == banexg.MarketOption && len(symbols) > 1 {
		if _, ok := args["symbol"]; !ok {
			if _, ok := args["baseCoin"]; !ok {
				tickers, err := e.fetchOptionTickersBySymbols(opName, symbols, args)
				if err != nil {
					return nil, nil, err
				}
				return tickers, symbolSet, nil
			}
		}
	}
	tickers, err := e.fetchTickers(opName, marketType, args)
	if err != nil {
		return nil, nil, err
	}
	return tickers, symbolSet, nil
}

func (e *Bybit) fetchOptionTickersBySymbols(opName string, symbols []string, args map[string]interface{}) ([]*banexg.Ticker, *errs.Error) {
	baseGroups := make(map[string][]string)
	for _, sym := range symbols {
		market, err := e.GetMarket(sym)
		if err != nil {
			return nil, err
		}
		base := market.Base
		if base == "" {
			return nil, errs.NewMsg(errs.CodeParamInvalid, "option symbol missing base coin: %v", sym)
		}
		baseGroups[base] = append(baseGroups[base], sym)
	}
	result := make([]*banexg.Ticker, 0, len(symbols))
	for base, group := range baseGroups {
		groupArgs := utils.SafeParams(args)
		groupArgs["baseCoin"] = base
		tickers, err := e.fetchTickers(opName, banexg.MarketOption, groupArgs)
		if err != nil {
			return nil, err
		}
		result = append(result, banexg.FilterTickers(tickers, banexg.BuildSymbolSet(group))...)
	}
	return result, nil
}

func (e *Bybit) fetchTickers(opName, marketType string, args map[string]interface{}) ([]*banexg.Ticker, *errs.Error) {
	if marketType == banexg.MarketOption {
		if _, ok := args["symbol"]; !ok {
			if _, ok := args["baseCoin"]; !ok {
				return nil, errs.NewMsg(errs.CodeParamRequired, "option tickers require symbol or baseCoin")
			}
		}
	}
	switch marketType {
	case banexg.MarketOption:
		args["category"] = "option"
	case banexg.MarketLinear:
		args["category"] = "linear"
	case banexg.MarketInverse:
		args["category"] = "inverse"
	default:
		args["category"] = "spot"
	}
	method := MethodPublicGetV5MarketTickers
	tryNum := e.GetRetryNum(opName, 1)
	if marketType == banexg.MarketOption {
		return parseTickers[*OptionTicker](e, marketType, method, args, tryNum)
	} else if marketType == banexg.MarketLinear || marketType == banexg.MarketInverse {
		return parseTickers[*FutureTicker](e, marketType, method, args, tryNum)
	} else {
		return parseTickers[*SpotTicker](e, marketType, method, args, tryNum)
	}
}

func parseTickers[T ITicker](e *Bybit, marketType, method string, args map[string]interface{}, tryNum int) ([]*banexg.Ticker, *errs.Error) {
	rsp := requestRetry[V5ListResult](e, method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	items := rsp.Result.List
	arr, err := decodeBybitList[T](items)
	if err != nil {
		return nil, err
	}
	var result = make([]*banexg.Ticker, 0, len(items))
	timeStamp := e.MilliSeconds()
	for i, item := range arr {
		ticker := item.ToStdTicker(e, marketType, items[i])
		if ticker.Symbol == "" {
			continue
		}
		ticker.TimeStamp = timeStamp
		result = append(result, ticker)
	}
	return result, nil
}

func (t *BaseTicker) ToStdTicker(e *Bybit, marketType string, info map[string]interface{}) *banexg.Ticker {
	symbol := bybitSafeSymbol(e, t.Symbol, marketType)
	bid1 := parseBybitNum(t.Bid1Price)
	bid1Vol := parseBybitNum(t.Bid1Size)
	ask1 := parseBybitNum(t.Ask1Price)
	ask1Vol := parseBybitNum(t.Ask1Size)
	high := parseBybitNum(t.HighPrice24h)
	low := parseBybitNum(t.LowPrice24h)
	vol := parseBybitNum(t.Volume24h)
	quoteVol := parseBybitNum(t.Turnover24h)
	lastPrice := parseBybitNum(t.LastPrice)
	return &banexg.Ticker{
		Symbol:      symbol,
		Bid:         bid1,
		BidVolume:   bid1Vol,
		Ask:         ask1,
		AskVolume:   ask1Vol,
		High:        high,
		Low:         low,
		Close:       lastPrice,
		Last:        lastPrice,
		BaseVolume:  vol,
		QuoteVolume: quoteVol,
		Info:        info,
	}
}

func applyBybitTickerOpenPct(ticker *banexg.Ticker, open, pcnt float64, info map[string]interface{}) {
	if ticker == nil {
		return
	}
	ticker.Open = open
	if open > 0 {
		ticker.PreviousClose = open
	}
	last := ticker.Last
	if last == 0 {
		last = ticker.Close
	}
	if last != 0 && open > 0 {
		ticker.Change = last - open
	}
	if pcnt != 0 {
		ticker.Percentage = pcnt * 100
	} else if ticker.Change != 0 && open > 0 {
		ticker.Percentage = ticker.Change / open * 100
	}
	ticker.Info = info
}

func (t *SpotTicker) ToStdTicker(e *Bybit, marketType string, info map[string]interface{}) *banexg.Ticker {
	res := t.BaseTicker.ToStdTicker(e, marketType, info)
	open := parseBybitNum(t.PrevPrice24h)
	pcnt := parseBybitNum(t.Price24hPcnt)
	// indexPrice, _ := strconv.ParseFloat(t.UsdIndexPrice, 64)
	applyBybitTickerOpenPct(res, open, pcnt, info)
	return res
}

func (t *ContractTicker) ToStdTicker(e *Bybit, marketType string, info map[string]interface{}) *banexg.Ticker {
	res := t.BaseTicker.ToStdTicker(e, marketType, info)
	indexPrice := parseBybitNum(t.IndexPrice)
	//delvPrice, _ := strconv.ParseFloat(t.PredictedDeliveryPrice, 64)
	markPrice := parseBybitNum(t.MarkPrice)
	//openInterest, _ := strconv.ParseFloat(t.OpenInterest, 64)
	res.IndexPrice = indexPrice
	res.MarkPrice = markPrice
	return res
}

func (t *FutureTicker) ToStdTicker(e *Bybit, marketType string, info map[string]interface{}) *banexg.Ticker {
	res := t.ContractTicker.ToStdTicker(e, marketType, info)
	open := parseBybitNum(t.PrevPrice24h)
	pcnt := parseBybitNum(t.Price24hPcnt)
	// PrevPrice1h, OpenInterestValue, FundingRate, NextFundingTime, BasisRate, DeliveryFeeRate
	// DeliveryTime, Basis
	applyBybitTickerOpenPct(res, open, pcnt, info)
	return res
}

func (t *OptionTicker) ToStdTicker(e *Bybit, marketType string, info map[string]interface{}) *banexg.Ticker {
	res := t.ContractTicker.ToStdTicker(e, marketType, info)
	// Bid1Iv, Ask1Iv, MarkIv, UnderlyingPrice, TotalVolume, TotalTurnover, Delta
	// Gamma, Vega, Theta, Change24h
	res.Info = info
	return res
}
