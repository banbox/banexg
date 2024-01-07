package binance

import (
	"context"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/bytedance/sonic"
	"strconv"
)

func (e *Binance) FetchOrderBook(symbol string, limit int, params *map[string]interface{}) (*banexg.OrderBook, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	args["symbol"] = market.ID
	if limit > 0 {
		args["limit"] = limit
	}
	var method string
	if market.Option {
		method = "eapiPublicGetDepth"
	} else if market.Linear {
		method = "fapiPublicGetDepth"
	} else if market.Inverse {
		method = "dapiPublicGetDepth"
	} else {
		method = "publicGetDepth"
	}
	tryNum := e.GetRetryNum("FetchOrderBook", 1)
	rsp := e.RequestApiRetry(context.Background(), method, &args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	if market.Option {
		return parseOrderBook[OptionOrderBook](market, rsp)
	} else if market.Linear {
		return parseOrderBook[LinearOrderBook](market, rsp)
	} else if market.Inverse {
		return parseOrderBook[InverseOrderBook](market, rsp)
	} else {
		return parseOrderBook[SpotOrderBook](market, rsp)
	}
}

func parseOrderBook[T IBnbOrderBook](m *banexg.Market, rsp *banexg.HttpRes) (*banexg.OrderBook, *errs.Error) {
	var data = new(T)
	err := sonic.UnmarshalString(rsp.Content, &data)
	if err != nil {
		return nil, errs.New(errs.CodeUnmarshalFail, err)
	}
	result := (*data).ToStdOrderBook(m)
	return result, nil
}

func (o BaseOrderBook) ToStdOrderBook(market *banexg.Market) *banexg.OrderBook {
	var asks = make([][2]float64, len(o.Asks))
	var bids = make([][2]float64, len(o.Bids))
	for i, it := range o.Asks {
		item := [2]float64{}
		item[0], _ = strconv.ParseFloat(it[0], 64)
		item[1], _ = strconv.ParseFloat(it[1], 64)
		asks[i] = item
	}
	for i, it := range o.Bids {
		item := [2]float64{}
		item[0], _ = strconv.ParseFloat(it[0], 64)
		item[1], _ = strconv.ParseFloat(it[1], 64)
		bids[i] = item
	}
	var res = banexg.OrderBook{
		Symbol: market.Symbol,
		Asks:   banexg.NewOrderBookSide(false, len(asks), asks),
		Bids:   banexg.NewOrderBookSide(true, len(bids), bids),
		Cache:  make([]map[string]string, 0),
	}
	return &res
}

func (o OptionOrderBook) ToStdOrderBook(market *banexg.Market) *banexg.OrderBook {
	var res = o.BaseOrderBook.ToStdOrderBook(market)
	res.TimeStamp = o.Time
	res.Nonce = int64(o.UpdateID)
	return res
}

func (o LinearOrderBook) ToStdOrderBook(market *banexg.Market) *banexg.OrderBook {
	var res = o.BaseOrderBook.ToStdOrderBook(market)
	res.TimeStamp = o.Time
	res.Nonce = int64(o.UpdateID)
	return res
}

func (o InverseOrderBook) ToStdOrderBook(market *banexg.Market) *banexg.OrderBook {
	var res = o.LinearOrderBook.ToStdOrderBook(market)
	return res
}

func (o SpotOrderBook) ToStdOrderBook(market *banexg.Market) *banexg.OrderBook {
	var res = o.BaseOrderBook.ToStdOrderBook(market)
	res.Nonce = int64(o.UpdateID)
	return res
}
