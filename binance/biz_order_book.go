package binance

import (
	"context"
	"github.com/anyongjin/banexg"
	"github.com/bytedance/sonic"
	"strconv"
)

func (e *Binance) FetchOrderBook(symbol string, limit int, params *map[string]interface{}) (*banexg.OrderBook, error) {
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
	rsp := e.RequestApi(context.Background(), method, &args)
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

func parseOrderBook[T IBnbOrderBook](m *banexg.Market, rsp *banexg.HttpRes) (*banexg.OrderBook, error) {
	var data = new(T)
	err := sonic.UnmarshalString(rsp.Content, &data)
	if err != nil {
		return nil, err
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
		Asks:   asks,
		Bids:   bids,
	}
	return &res
}

func (o OptionOrderBook) ToStdOrderBook(market *banexg.Market) *banexg.OrderBook {
	var res = o.BaseOrderBook.ToStdOrderBook(market)
	res.TimeStamp = o.Time
	res.Info = o
	return res
}

func (o LinearOrderBook) ToStdOrderBook(market *banexg.Market) *banexg.OrderBook {
	var res = o.BaseOrderBook.ToStdOrderBook(market)
	res.TimeStamp = o.Time
	res.Info = o
	return res
}

func (o InverseOrderBook) ToStdOrderBook(market *banexg.Market) *banexg.OrderBook {
	var res = o.LinearOrderBook.ToStdOrderBook(market)
	res.Info = o
	return res
}

func (o SpotOrderBook) ToStdOrderBook(market *banexg.Market) *banexg.OrderBook {
	var res = o.BaseOrderBook.ToStdOrderBook(market)
	res.Info = o
	return res
}
