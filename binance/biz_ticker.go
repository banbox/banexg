package binance

import (
	"context"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
	"strconv"
)

/*
FetchTickers
fetches price tickers for multiple markets, statistical information calculated over the past 24 hours for each market

	:see: https://binance-docs.github.io/apidocs/spot/en/#24hr-ticker-price-change-statistics         # spot
	:see: https://binance-docs.github.io/apidocs/futures/en/#24hr-ticker-price-change-statistics      # swap
	:see: https://binance-docs.github.io/apidocs/delivery/en/#24hr-ticker-price-change-statistics     # future
	:see: https://binance-docs.github.io/apidocs/voptions/en/#24hr-ticker-price-change-statistics     # option
	:param str[]|None symbols: unified symbols of the markets to fetch the ticker for, all market tickers are returned if not assigned
	:param dict [params]: extra parameters specific to the exchange API endpoint
	:returns dict: a dictionary of `ticker structures <https://docs.ccxt.com/#/?id=ticker-structure>`
*/
func (e *Binance) FetchTickers(symbols []string, params map[string]interface{}) ([]*banexg.Ticker, *errs.Error) {
	args := utils.SafeParams(params)
	marketType, _, err := e.LoadArgsMarketType(args, symbols...)
	if err != nil {
		return nil, err
	}
	var method string
	inMethod := utils.PopMapVal(args, banexg.ParamMethod, "")
	if inMethod == "" {
		switch marketType {
		case banexg.MarketOption:
			method = MethodEapiPublicGetTicker
		case banexg.MarketLinear:
			method = MethodFapiPublicGetTicker24hr
		case banexg.MarketInverse:
			method = MethodDapiPublicGetTicker24hr
		default:
			method = MethodPublicGetTicker24hr
		}
	} else if inMethod == "bookTicker" {
		switch marketType {
		case banexg.MarketOption:
			return nil, errs.NewMsg(errs.CodeParamInvalid, "option market dont support bookTicker")
		case banexg.MarketLinear:
			method = MethodFapiPublicGetTickerBookTicker
		case banexg.MarketInverse:
			method = MethodDapiPublicGetTickerBookTicker
		default:
			method = MethodPublicGetTickerBookTicker
		}
	} else {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "unsupported method: %v", inMethod)
	}
	tryNum := e.GetRetryNum("FetchTickers", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	if inMethod == "bookTicker" {
		switch marketType {
		case banexg.MarketLinear:
			return parseTickers[*LinearBookTicker](rsp, e, marketType)
		case banexg.MarketInverse:
			return parseTickers[*InverseBookTicker](rsp, e, marketType)
		default:
			return parseTickers[*SpotBookTicker](rsp, e, marketType)
		}
	} else {
		switch marketType {
		case banexg.MarketOption:
			return parseTickers[*OptionTicker](rsp, e, marketType)
		case banexg.MarketLinear:
			return parseTickers[*LinearTicker](rsp, e, marketType)
		case banexg.MarketInverse:
			return parseTickers[*InverseTicker24hr](rsp, e, marketType)
		default:
			return parseTickers[*SpotTicker24hr](rsp, e, marketType)
		}
	}
}

func (e *Binance) FetchTicker(symbol string, params map[string]interface{}) (*banexg.Ticker, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	args["symbol"] = market.ID
	var method string
	if market.Option {
		method = MethodEapiPublicGetTicker
	} else if market.Linear {
		method = MethodFapiPublicGetTicker24hr
	} else if market.Inverse {
		method = MethodDapiPublicGetTicker24hr
	} else {
		rolling := utils.PopMapVal(args, banexg.ParamRolling, false)
		if rolling {
			method = MethodPublicGetTicker
		} else {
			method = MethodPublicGetTicker24hr
		}
	}
	tryNum := e.GetRetryNum("FetchTicker", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	if method == MethodEapiPublicGetTicker {
		tickers, err := parseTickers[*OptionTicker](rsp, e, market.Type)
		if len(tickers) > 0 {
			return tickers[0], err
		}
		return nil, err
	} else if method == MethodFapiPublicGetTicker24hr {
		return parseTicker[*LinearTicker](rsp, e, market.Type)
	} else if method == MethodDapiPublicGetTicker24hr {
		tickers, err := parseTickers[*InverseTicker24hr](rsp, e, market.Type)
		if len(tickers) > 0 {
			return tickers[0], err
		}
		return nil, err
	} else if method == MethodPublicGetTicker {
		return parseTicker[*SpotTicker](rsp, e, market.Type)
	} else if method == MethodPublicGetTicker24hr {
		return parseTicker[*SpotTicker24hr](rsp, e, market.Type)
	} else {
		return nil, errs.NewMsg(errs.CodeNotSupport, "unsupport method: %v", method)
	}
}

/*
FetchTickerPrice
symbol为空表示获取所有，不为空获取单个
*/
func (e *Binance) FetchTickerPrice(symbol string, params map[string]interface{}) (map[string]float64, *errs.Error) {
	args := utils.SafeParams(params)
	marketType, _, err := e.LoadArgsMarketType(args)
	if err != nil {
		return nil, err
	}
	var method string
	switch marketType {
	case banexg.MarketOption:
		method = MethodEapiPublicGetTicker
	case banexg.MarketLinear:
		method = MethodFapiPublicV2GetTickerPrice
	case banexg.MarketInverse:
		method = MethodDapiPublicGetTickerPrice
	default:
		method = MethodPublicGetTickerPrice
	}
	if symbol != "" {
		market, err := e.GetMarket(symbol)
		if err != nil {
			return nil, err
		}
		args["symbol"] = market.ID
	}
	tryNum := e.GetRetryNum("GetTickerPrice", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	switch marketType {
	case banexg.MarketOption:
		return parsePrices[*OptionTicker](rsp, e, marketType)
	case banexg.MarketLinear:
		return parsePrices[*LinearTickerPrice](rsp, e, marketType)
	case banexg.MarketInverse:
		return parsePrices[*InverseTickerPrice](rsp, e, marketType)
	default:
		return parsePrices[*SymbolPrice](rsp, e, marketType)
	}
}

func parseTickers[T IBnbTicker](rsp *banexg.HttpRes, e *Binance, marketType string) ([]*banexg.Ticker, *errs.Error) {
	var data = make([]T, 0)
	rspText := banexg.EnsureArrStr(rsp.Content)
	items, err := utils.UnmarshalStringMapArr(rspText, &data)
	if err != nil {
		return nil, errs.New(errs.CodeUnmarshalFail, err)
	}
	var result = make([]*banexg.Ticker, 0, len(data))
	for i, item := range data {
		ticker := item.ToStdTicker(e, marketType, items[i])
		if ticker.Symbol == "" {
			continue
		}
		result = append(result, ticker)
	}
	return result, nil
}

func parseTicker[T IBnbTicker](rsp *banexg.HttpRes, e *Binance, marketType string) (*banexg.Ticker, *errs.Error) {
	var data = new(T)
	info, err := utils.UnmarshalStringMap(rsp.Content, &data)
	if err != nil {
		return nil, errs.New(errs.CodeUnmarshalFail, err)
	}
	result := (*data).ToStdTicker(e, marketType, info)
	return result, nil
}

func (t *SpotTicker) ToStdTicker(e *Binance, marketType string, info map[string]interface{}) *banexg.Ticker {
	highPrice, _ := strconv.ParseFloat(t.HighPrice, 64)
	lowPrice, _ := strconv.ParseFloat(t.LowPrice, 64)
	openPrice, _ := strconv.ParseFloat(t.OpenPrice, 64)
	lastPrice, _ := strconv.ParseFloat(t.LastPrice, 64)
	change, _ := strconv.ParseFloat(t.PriceChange, 64)
	percent, _ := strconv.ParseFloat(t.PriceChangePercent, 64)
	wAvgPrice, _ := strconv.ParseFloat(t.WeightedAvgPrice, 64)
	volume, _ := strconv.ParseFloat(t.Volume, 64)
	quoteVolume, _ := strconv.ParseFloat(t.QuoteVolume, 64)
	symbol := e.SafeSymbol(t.Symbol, "", marketType)
	ticker := &banexg.Ticker{
		Symbol:      symbol,
		TimeStamp:   t.CloseTime,
		High:        highPrice,
		Low:         lowPrice,
		Open:        openPrice,
		Close:       lastPrice,
		Last:        lastPrice,
		Change:      change,
		Percentage:  percent,
		Vwap:        wAvgPrice,
		BaseVolume:  volume,
		QuoteVolume: quoteVolume,
	}
	return ticker
}

func (t *BookTicker) SetStdTicker(ticker *banexg.Ticker) {
	bidPrice, _ := strconv.ParseFloat(t.BidPrice, 64)
	bidQty, _ := strconv.ParseFloat(t.BidQty, 64)
	askPrice, _ := strconv.ParseFloat(t.AskPrice, 64)
	askQty, _ := strconv.ParseFloat(t.AskQty, 64)
	ticker.Bid = bidPrice
	ticker.BidVolume = bidQty
	ticker.Ask = askPrice
	ticker.AskVolume = askQty
}

func (t *SpotBookTicker) ToStdTicker(e *Binance, marketType string, info map[string]interface{}) *banexg.Ticker {
	symbol := e.SafeSymbol(t.Symbol, "", marketType)
	ticker := &banexg.Ticker{
		Symbol: symbol,
	}
	t.BookTicker.SetStdTicker(ticker)
	ticker.Info = info
	ticker.TimeStamp = e.MilliSeconds()
	return ticker
}

func (t *LinearBookTicker) ToStdTicker(e *Binance, marketType string, info map[string]interface{}) *banexg.Ticker {
	ticker := t.SpotBookTicker.ToStdTicker(e, marketType, info)
	ticker.TimeStamp = t.Time
	ticker.Info = info
	return ticker
}

func (t *InverseBookTicker) ToStdTicker(e *Binance, marketType string, info map[string]interface{}) *banexg.Ticker {
	ticker := t.LinearBookTicker.ToStdTicker(e, marketType, info)
	ticker.Info = info
	return ticker
}

func (t *LinearTicker) ToStdTicker(e *Binance, marketType string, info map[string]interface{}) *banexg.Ticker {
	ticker := t.SpotTicker.ToStdTicker(e, marketType, info)
	ticker.Info = info
	return ticker
}

func (t *SpotTicker24hr) ToStdTicker(e *Binance, marketType string, info map[string]interface{}) *banexg.Ticker {
	ticker := t.LinearTicker.ToStdTicker(e, marketType, info)
	ticker.Symbol = e.SafeSymbol(t.Symbol, "", marketType)
	ticker.Info = info
	t.BookTicker.SetStdTicker(ticker)
	pClosePrice, _ := strconv.ParseFloat(t.PrevClosePrice, 64)
	ticker.PreviousClose = pClosePrice
	return ticker
}

func (t *InverseTicker24hr) ToStdTicker(e *Binance, marketType string, info map[string]interface{}) *banexg.Ticker {
	ticker := t.SpotTicker.ToStdTicker(e, marketType, info)
	ticker.Info = info
	baseVolume, _ := strconv.ParseFloat(t.BaseVolume, 64)
	ticker.BaseVolume = baseVolume
	return ticker
}

func (t *OptionTicker) ToStdTicker(e *Binance, marketType string, info map[string]interface{}) *banexg.Ticker {
	ticker := &banexg.Ticker{
		Symbol:      e.SafeSymbol(t.Symbol, "", marketType),
		TimeStamp:   t.CloseTime,
		Change:      t.PriceChange,
		Percentage:  t.PriceChangePercent,
		Last:        t.LastPrice,
		Close:       t.LastPrice,
		Open:        t.Open,
		High:        t.High,
		Low:         t.Low,
		BaseVolume:  t.Volume,
		QuoteVolume: t.Amount,
		Bid:         t.BidPrice,
		Ask:         t.AskPrice,
	}
	return ticker
}

type ITickerPrice interface {
	ToStdPrice(e *Binance, marketType string) (string, float64)
}

type SymbolPrice struct {
	Symbol string  `json:"symbol"`       // 交易对，比如 "LTCBTC"
	Price  float64 `json:"price,string"` // 交易价格，保留为字符串以防止精度损失
}

type LinearTickerPrice struct {
	SymbolPrice
	Time int64 `json:"time"` // 撮合引擎的时间戳，单位为毫秒
}

type InverseTickerPrice struct {
	LinearTickerPrice
	PS string `json:"ps"` // 标的交易对
}

func (t *OptionTicker) ToStdPrice(e *Binance, marketType string) (string, float64) {
	symbol := e.SafeSymbol(t.Symbol, "", marketType)
	return symbol, t.LastPrice
}

func (t *SymbolPrice) ToStdPrice(e *Binance, marketType string) (string, float64) {
	symbol := e.SafeSymbol(t.Symbol, "", marketType)
	return symbol, t.Price
}

func parsePrices[T ITickerPrice](rsp *banexg.HttpRes, e *Binance, marketType string) (map[string]float64, *errs.Error) {
	var data = make([]T, 0)
	rspText := banexg.EnsureArrStr(rsp.Content)
	err := utils.UnmarshalString(rspText, &data, utils.JsonNumDefault)
	if err != nil {
		return nil, errs.New(errs.CodeUnmarshalFail, err)
	}
	var result = make(map[string]float64)
	for _, item := range data {
		pair, price := item.ToStdPrice(e, marketType)
		if pair == "" {
			continue
		}
		result[pair] = price
	}
	return result, nil
}
