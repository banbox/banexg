package bybit

import (
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
	"strconv"
)

func (e *Bybit) FetchTickers(symbols []string, params map[string]interface{}) ([]*banexg.Ticker, *errs.Error) {
	args := utils.SafeParams(params)
	marketType, _, err := e.LoadArgsMarketType(args, symbols...)
	if err != nil {
		return nil, err
	}
	return e.fetchTickers(marketType, args)
}

func (e *Bybit) FetchTicker(symbol string, params map[string]interface{}) (*banexg.Ticker, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	args["symbol"] = market.ID
	var items []*banexg.Ticker
	items, err = e.fetchTickers(market.Type, args)
	if len(items) > 0 {
		return items[0], nil
	}
	return nil, err
}

func (e *Bybit) FetchTickerPrice(symbol string, params map[string]interface{}) (map[string]float64, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "not support FetchTickerPrice")
}

func (e *Bybit) fetchTickers(marketType string, args map[string]interface{}) ([]*banexg.Ticker, *errs.Error) {
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
	tryNum := e.GetRetryNum("FetchTicker", 1)
	if marketType == banexg.MarketOption {
		return parseTickers[*OptionTicker](e, marketType, method, args, tryNum)
	} else if marketType == banexg.MarketLinear || marketType == banexg.MarketInverse {
		return parseTickers[*FutureTicker](e, marketType, method, args, tryNum)
	} else {
		return parseTickers[*SpotTicker](e, marketType, method, args, tryNum)
	}
}

func parseTickers[T ITicker](e *Bybit, marketType, method string, args map[string]interface{}, tryNum int) ([]*banexg.Ticker, *errs.Error) {
	rsp := requestRetry[struct {
		Category string `json:"category"`
		List     []T    `json:"list"`
	}](e, method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	items := rsp.Result.List
	var result = make([]*banexg.Ticker, 0, len(items))
	timeStamp := e.MilliSeconds()
	for _, item := range items {
		ticker := item.ToStdTicker(e, marketType)
		if ticker.Symbol == "" {
			continue
		}
		ticker.TimeStamp = timeStamp
		result = append(result, ticker)
	}
	return result, nil
}

func (t *BaseTicker) ToStdTicker(e *Bybit, marketType string) *banexg.Ticker {
	symbol := e.SafeSymbol(t.Symbol, "", marketType)
	bid1, _ := strconv.ParseFloat(t.Bid1Price, 64)
	bid1Vol, _ := strconv.ParseFloat(t.Bid1Size, 64)
	ask1, _ := strconv.ParseFloat(t.Ask1Price, 64)
	ask1Vol, _ := strconv.ParseFloat(t.Ask1Size, 64)
	high, _ := strconv.ParseFloat(t.HighPrice24h, 64)
	low, _ := strconv.ParseFloat(t.LowPrice24h, 64)
	vol, _ := strconv.ParseFloat(t.Volume24h, 64)
	quoteVol, _ := strconv.ParseFloat(t.Turnover24h, 64)
	lastPrice, _ := strconv.ParseFloat(t.LastPrice, 64)
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
	}
}

func (t *SpotTicker) ToStdTicker(e *Bybit, marketType string) *banexg.Ticker {
	res := t.BaseTicker.ToStdTicker(e, marketType)
	open, _ := strconv.ParseFloat(t.PrevPrice24h, 64)
	pcnt, _ := strconv.ParseFloat(t.Price24hPcnt, 64)
	// indexPrice, _ := strconv.ParseFloat(t.UsdIndexPrice, 64)
	res.Open = open
	res.Percentage = pcnt * 100
	res.Info = t
	return res
}

func (t *ContractTicker) ToStdTicker(e *Bybit, marketType string) *banexg.Ticker {
	res := t.BaseTicker.ToStdTicker(e, marketType)
	indexPrice, _ := strconv.ParseFloat(t.IndexPrice, 64)
	//delvPrice, _ := strconv.ParseFloat(t.PredictedDeliveryPrice, 64)
	markPrice, _ := strconv.ParseFloat(t.MarkPrice, 64)
	//openInterest, _ := strconv.ParseFloat(t.OpenInterest, 64)
	res.IndexPrice = indexPrice
	res.MarkPrice = markPrice
	return res
}

func (t *FutureTicker) ToStdTicker(e *Bybit, marketType string) *banexg.Ticker {
	res := t.ContractTicker.ToStdTicker(e, marketType)
	open, _ := strconv.ParseFloat(t.PrevPrice24h, 64)
	pcnt, _ := strconv.ParseFloat(t.Price24hPcnt, 64)
	// PrevPrice1h, OpenInterestValue, FundingRate, NextFundingTime, BasisRate, DeliveryFeeRate
	// DeliveryTime, Basis
	res.Open = open
	res.Percentage = pcnt * 100
	res.Info = t
	return res
}

func (t *OptionTicker) ToStdTicker(e *Bybit, marketType string) *banexg.Ticker {
	res := t.ContractTicker.ToStdTicker(e, marketType)
	// Bid1Iv, Ask1Iv, MarkIv, UnderlyingPrice, TotalVolume, TotalTurnover, Delta
	// Gamma, Vega, Theta, Change24h
	res.Info = t
	return res
}
