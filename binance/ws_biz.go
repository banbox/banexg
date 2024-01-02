package binance

import (
	"fmt"
	"github.com/anyongjin/banexg"
	"github.com/anyongjin/banexg/errs"
	"github.com/anyongjin/banexg/log"
	"github.com/anyongjin/banexg/utils"
	"github.com/bytedance/sonic"
	"go.uber.org/zap"
	"strconv"
	"strings"
)

func makeHandleWsMsg(e *Binance) banexg.FuncOnWsMsg {
	return func(wsUrl string, msg map[string]string) {
		event, ok := msg["e"]
		if !ok {
			if jobId, ok := msg["id"]; ok {
				// 任务结果返回
				err := banexg.CheckWsError(msg)
				if err != nil {
					log.Error("ws job fail", zap.String("job", jobId), zap.Error(err))
				} else {
					log.Info("ws job ok", zap.String("job", jobId))
				}
			} else {
				msgText, _ := sonic.MarshalString(msg)
				log.Error("no event ws msg", zap.String("msg", msgText))
			}
			return
		}
		client, ok := e.WSClients[wsUrl]
		if !ok {
			log.Error("no ws client found for ", zap.String("url", wsUrl))
		}
		switch event {
		case "depthUpdate":
			e.handleOrderBook(client, msg)
		case "trade":
			e.handleTrade(client, msg)
		case "aggTrade":
			e.handleTrade(client, msg)
		case "kline":
			e.handleOhlcv(client, msg)
		case "markPrice_kline":
			e.handleOhlcv(client, msg)
		case "indexPrice_kline":
			e.handleOhlcv(client, msg)
		case "24hrTicker":
			e.handleTicker(client, msg)
		case "24hrMiniTicker":
			e.handleTicker(client, msg)
		case "bookTicker":
			e.handleTicker(client, msg)
		case "outboundAccountPosition":
			e.handleBalance(client, msg)
		case "balanceUpdate":
			e.handleBalance(client, msg)
		case "ACCOUNT_UPDATE":
			e.handleAccountUpdate(client, msg)
		case "executionReport":
			e.handleOrderUpdate(client, msg)
		case "ORDER_TRADE_UPDATE":
			e.handleOrderUpdate(client, msg)
		default:
			msgText, _ := sonic.MarshalString(msg)
			log.Warn("unhandle ws msg", zap.String("msg", msgText))
		}
	}
}

func (e *Binance) handleTrade(client *banexg.WsClient, msg map[string]string) {

}

/*
WatchOhlcvs
watches historical candlestick data containing the open, high, low, and close price, and the volume of a market
:param [][2]string jobs: array of arrays containing unified symbols and timeframes to fetch OHLCV data for, example [['BTC/USDT', '1m'], ['LTC/USDT', '5m']]
:param dict [params]: extra parameters specific to the exchange API endpoint
:returns int[][]: A list of candles ordered, open, high, low, close, volume
*/
func (e *Binance) WatchOhlcvs(jobs [][2]string, params *map[string]interface{}) (chan banexg.SymbolKline, *errs.Error) {
	client, msgHash, requestId, subParams, symbols, args, err := e.prepareOhlcvSub(jobs, params)
	if err != nil {
		return nil, err
	}

	chanKey := client.URL + "#" + msgHash
	create := func(cap int) chan banexg.SymbolKline { return make(chan banexg.SymbolKline, cap) }
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, args)
	e.AddWsChanRefs(chanKey, symbols...)
	var request = map[string]interface{}{
		"method": "SUBSCRIBE",
		"params": subParams,
		"id":     requestId,
	}
	err = client.Write(request, nil)
	if err != nil {
		e.DelWsChanRefs(chanKey, symbols...)
		return nil, err
	}
	return out, nil
}

type WsKline struct {
	OpenTime   int64  `json:"t"`
	CloseTime  int64  `json:"T"`
	Symbol     string `json:"s"`
	PairSymbol string `json:"ps"`
	TimeFrame  string `json:"i"`
	Open       string `json:"o"`
	Close      string `json:"c"`
	High       string `json:"h"`
	Low        string `json:"l"`
	Volume     string `json:"v"`
	LastId     int64  `json:"L"`
}

func (e *Binance) handleOhlcv(client *banexg.WsClient, msg map[string]string) {
	/*
		https://binance-docs.github.io/apidocs/futures/cn/#k-7
	*/
	event, _ := utils.SafeMapVal(msg, "e", "")
	switch event {
	case "indexPrice_kline":
		event = "indexPriceKline"
	case "markPrice_kline":
		event = "markPriceKline"
	}
	kText, err := utils.SafeMapVal(msg, "k", "")
	if err != nil {
		log.Error("invalid kline ws", zap.Error(err))
		return
	}
	var k = WsKline{}
	err = sonic.UnmarshalString(kText, &k)
	if err != nil {
		log.Error("unmarshal ws kline fail", zap.String("k", kText), zap.Error(err))
		return
	}
	var chanKey = client.URL + "#" + client.MarketType + "@" + event
	var marketId string
	if event == "indexPriceKline" {
		marketId, _ = utils.SafeMapVal(msg, "ps", "")
	} else if k.Symbol != "" {
		marketId = k.Symbol
	} else if k.PairSymbol != "" {
		marketId = k.PairSymbol
	}
	o, _ := strconv.ParseFloat(k.Open, 64)
	c, _ := strconv.ParseFloat(k.Close, 64)
	h, _ := strconv.ParseFloat(k.High, 64)
	l, _ := strconv.ParseFloat(k.Low, 64)
	v, _ := strconv.ParseFloat(k.Volume, 64)
	var kline = &banexg.SymbolKline{
		Symbol: e.SafeSymbol(marketId, "", client.MarketType),
		Kline: banexg.Kline{
			Time:   k.OpenTime,
			Open:   o,
			Close:  c,
			High:   h,
			Low:    l,
			Volume: v,
		},
	}
	banexg.WriteOutChan(e.Exchange, chanKey, *kline)
}

func (e *Binance) prepareOhlcvSub(jobs [][2]string, params *map[string]interface{}) (*banexg.WsClient, string, int, []string, []string, map[string]interface{}, *errs.Error) {
	if len(jobs) == 0 {
		return nil, "", 0, nil, nil, nil, errs.NewMsg(errs.CodeParamRequired, "symbols is required")
	}
	args, market, err := e.LoadArgsMarket(jobs[0][0], params)
	if err != nil {
		return nil, "", 0, nil, nil, nil, err
	}
	name := utils.PopMapVal(args, banexg.ParamName, "kline")
	msgHash := market.Type + "@" + name
	wsUrl, requestId, err := e.GetWsInfo(market.Type, msgHash)
	if err != nil {
		return nil, "", 0, nil, nil, nil, err
	}

	subParams := make([]string, 0, len(jobs))
	symbols := make([]string, 0, len(jobs))
	for _, row := range jobs {
		mar, err := e.GetMarket(row[0])
		if err != nil {
			return nil, "", 0, nil, nil, nil, err
		}
		marketId := mar.LowercaseID
		if name == "indexPriceKline" {
			marketId = strings.Replace(marketId, "_perp", "", -1)
		}
		subParams = append(subParams, fmt.Sprintf("%s@%s_%s", marketId, name, row[1]))
		symbols = append(symbols, row[0])
	}
	client, err := e.GetClient(wsUrl, market.Type)
	if err != nil {
		return nil, "", 0, nil, nil, nil, err
	}

	return client, msgHash, requestId, subParams, symbols, args, nil
}

func (e *Binance) UnWatchOhlcvs(jobs [][2]string, params *map[string]interface{}) *errs.Error {
	client, msgHash, requestId, subParams, symbols, _, err := e.prepareOhlcvSub(jobs, params)
	if err != nil {
		log.Error("prepareOhlcvSub fail")
		return err
	}

	chanKey := client.URL + "#" + msgHash
	var request = map[string]interface{}{
		"method": "UNSUBSCRIBE",
		"params": subParams,
		"id":     requestId,
	}
	err = client.Write(request, &banexg.WsJobInfo{
		ID:      strconv.Itoa(requestId),
		Symbols: symbols,
		Method: func(wsUrl string, msg map[string]string, info *banexg.WsJobInfo) {
			e.DelWsChanRefs(chanKey, symbols...)
		},
	})
	if err != nil {
		log.Error("unwatch pairs fail")
		e.DelWsChanRefs(chanKey, symbols...)
		return err
	}
	return nil
}

func (e *Binance) handleTicker(client *banexg.WsClient, msg map[string]string) {

}
func (e *Binance) handleBalance(client *banexg.WsClient, msg map[string]string) {

}
func (e *Binance) handleAccountUpdate(client *banexg.WsClient, msg map[string]string) {

}
func (e *Binance) handleOrderUpdate(client *banexg.WsClient, msg map[string]string) {

}
