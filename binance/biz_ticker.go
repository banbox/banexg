package binance

import (
	"context"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
	"github.com/bytedance/sonic"
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
func (e *Binance) FetchTickers(symbols []string, params *map[string]interface{}) ([]*banexg.Ticker, *errs.Error) {
	args := utils.SafeParams(params)
	marketType, _, err := e.LoadArgsMarketType(args, symbols...)
	if err != nil {
		return nil, err
	}
	var method string
	switch marketType {
	case banexg.MarketOption:
		method = "eapiPublicGetTicker"
	case banexg.MarketLinear:
		method = "fapiPublicGetTicker24hr"
	case banexg.MarketInverse:
		method = "dapiPublicGetTicker24hr"
	default:
		method = "publicGetTicker24hr"
	}
	tryNum := e.GetRetryNum("FetchTickers", 1)
	rsp := e.RequestApiRetry(context.Background(), method, &args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
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

func (e *Binance) FetchTicker(symbol string, params *map[string]interface{}) (*banexg.Ticker, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	args["symbol"] = market.ID
	var method string
	if market.Option {
		method = "eapiPublicGetTicker"
	} else if market.Linear {
		method = "fapiPublicGetTicker24hr"
	} else if market.Inverse {
		method = "dapiPublicGetTicker24hr"
	} else {
		rolling := utils.PopMapVal(args, banexg.ParamRolling, false)
		if rolling {
			method = "publicGetTicker"
		} else {
			method = "publicGetTicker24hr"
		}
	}
	tryNum := e.GetRetryNum("FetchTicker", 1)
	rsp := e.RequestApiRetry(context.Background(), method, &args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	if method == "eapiPublicGetTicker" {
		tickers, err := parseTickers[*OptionTicker](rsp, e, market.Type)
		if len(tickers) > 0 {
			return tickers[0], err
		}
		return nil, err
	} else if method == "fapiPublicGetTicker24hr" {
		return parseTicker[*LinearTicker](rsp, e, market.Type)
	} else if method == "dapiPublicGetTicker24hr" {
		tickers, err := parseTickers[*InverseTicker24hr](rsp, e, market.Type)
		if len(tickers) > 0 {
			return tickers[0], err
		}
		return nil, err
	} else if method == "publicGetTicker" {
		return parseTicker[*SpotTicker](rsp, e, market.Type)
	} else if method == "publicGetTicker24hr" {
		return parseTicker[*SpotTicker24hr](rsp, e, market.Type)
	} else {
		return nil, errs.NewMsg(errs.CodeNotSupport, "unsupport method: %v", method)
	}
}

func parseTickers[T IBnbTicker](rsp *banexg.HttpRes, e *Binance, marketType string) ([]*banexg.Ticker, *errs.Error) {
	var data = make([]T, 0)
	err := sonic.UnmarshalString(rsp.Content, &data)
	if err != nil {
		return nil, errs.New(errs.CodeUnmarshalFail, err)
	}
	var result = make([]*banexg.Ticker, len(data))
	for i, item := range data {
		result[i] = item.ToStdTicker(e, marketType)
	}
	return result, nil
}

func parseTicker[T IBnbTicker](rsp *banexg.HttpRes, e *Binance, marketType string) (*banexg.Ticker, *errs.Error) {
	var data = new(T)
	err := sonic.UnmarshalString(rsp.Content, &data)
	if err != nil {
		return nil, errs.New(errs.CodeUnmarshalFail, err)
	}
	result := (*data).ToStdTicker(e, marketType)
	return result, nil
}

func (t *SpotTicker) ToStdTicker(e *Binance, marketType string) *banexg.Ticker {
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

func (t *LinearTicker) ToStdTicker(e *Binance, marketType string) *banexg.Ticker {
	ticker := t.SpotTicker.ToStdTicker(e, marketType)
	ticker.Info = t
	return ticker
}

func (t *SpotTicker24hr) ToStdTicker(e *Binance, marketType string) *banexg.Ticker {
	ticker := t.LinearTicker.ToStdTicker(e, marketType)
	ticker.Symbol = e.SafeSymbol(t.Symbol, "", marketType)
	ticker.Info = t
	t.BookTicker.SetStdTicker(ticker)
	pClosePrice, _ := strconv.ParseFloat(t.PrevClosePrice, 64)
	ticker.PreviousClose = pClosePrice
	return ticker
}

func (t *InverseTicker24hr) ToStdTicker(e *Binance, marketType string) *banexg.Ticker {
	ticker := t.SpotTicker.ToStdTicker(e, marketType)
	ticker.Info = t
	baseVolume, _ := strconv.ParseFloat(t.BaseVolume, 64)
	ticker.BaseVolume = baseVolume
	return ticker
}

func (t *OptionTicker) ToStdTicker(e *Binance, marketType string) *banexg.Ticker {
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
