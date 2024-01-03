package binance

import (
	"context"
	"fmt"
	"github.com/anyongjin/banexg"
	"github.com/anyongjin/banexg/errs"
	"github.com/anyongjin/banexg/log"
	"github.com/anyongjin/banexg/utils"
	"github.com/bytedance/sonic"
	"go.uber.org/zap"
	"strconv"
	"strings"
	"time"
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

type AuthRes struct {
	ListenKey string `json:"listenKey"`
}

func makeAuthenticate(e *Binance) banexg.FuncAuth {
	return func(params *map[string]interface{}) *errs.Error {
		zeroVal := int64(0)
		args := utils.SafeParams(params)
		marketType, _ := e.GetArgsMarketType(args, "")
		lastTimeKey := marketType + "lastAuthTime"
		authField := marketType + banexg.MidListenKey
		lastAuthTime := utils.GetMapVal(e.Data, lastTimeKey, zeroVal)
		authRefreshSecs := utils.GetMapVal(e.Data, banexg.OptAuthRefreshSecs, 1200)
		refreshDuration := int64(authRefreshSecs * 1000)
		curTime := e.MilliSeconds()
		if curTime-lastAuthTime <= refreshDuration {
			return nil
		}
		marginMode := utils.PopMapVal(args, banexg.ParamMarginMode, "")
		method := "publicPostUserDataStream"
		if marketType == banexg.MarketLinear {
			method = "fapiPrivatePostListenKey"
		} else if marketType == banexg.MarketInverse {
			method = "dapiPrivatePostListenKey"
		} else if marginMode == banexg.MarginIsolated {
			method = "sapiPostUserDataStreamIsolated"
			marketId, err := e.GetMarketIDByArgs(args, true)
			if err != nil {
				return err
			}
			args["symbol"] = marketId
		} else if marketType == banexg.MarketMargin {
			method = "sapiPostUserDataStream"
		}
		rsp := e.RequestApiRetry(context.Background(), method, &args, 1)
		if rsp.Error != nil {
			return rsp.Error
		}
		var res = AuthRes{}
		err2 := sonic.UnmarshalString(rsp.Content, &res)
		if err2 != nil {
			return errs.New(errs.CodeUnmarshalFail, err2)
		}
		e.Data[lastTimeKey] = curTime
		e.Data[authField] = res.ListenKey
		refreshAfter := time.Duration(authRefreshSecs) * time.Second
		time.AfterFunc(refreshAfter, func() {
			e.keepAliveListenKey(params)
		})
		return nil
	}
}

func (e *Binance) keepAliveListenKey(params *map[string]interface{}) {
	args := utils.SafeParams(params)
	marketType, _ := e.GetArgsMarketType(args, "")
	lastTimeKey := marketType + "lastAuthTime"
	authField := marketType + banexg.MidListenKey
	listenKey := utils.GetMapVal(e.Data, authField, "")
	if listenKey == "" {
		return
	}
	var success = false
	defer func() {
		if success {
			return
		}
		delete(e.Data, authField)
		delete(e.Data, lastTimeKey)
		wsUrl := e.Hosts.GetHost(marketType) + "/" + listenKey
		if client, ok := e.WSClients[wsUrl]; ok {
			_ = client.Conn.WriteClose()
			log.Warn("renew listenKey fail, close ws client", zap.String("url", wsUrl))
		}
	}()
	method := "publicPutUserDataStream"
	if marketType == banexg.MarketLinear {
		method = "fapiPrivatePutListenKey"
	} else if marketType == banexg.MarketInverse {
		method = "dapiPrivatePutListenKey"
	} else {
		args[banexg.MidListenKey] = listenKey
		if marketType == banexg.MarketMargin {
			method = "sapiPutUserDataStream"
			marketId, err := e.GetMarketIDByArgs(args, true)
			if err != nil {
				log.Error("keepAliveListenKey fail", zap.Error(err))
				return
			}
			args["symbol"] = marketId
		}
	}
	rsp := e.RequestApiRetry(context.Background(), method, &args, 1)
	if rsp.Error != nil {
		log.Error("refresh listenKey fail", zap.Error(rsp.Error))
		return
	}
	success = true
	e.Data[lastTimeKey] = e.MilliSeconds()
	authRefreshSecs := utils.GetMapVal(e.Data, banexg.OptAuthRefreshSecs, 1200)
	refreshDuration := time.Duration(authRefreshSecs) * time.Second
	time.AfterFunc(refreshDuration, func() {
		e.keepAliveListenKey(params)
	})
}

func (e *Binance) getAuthClient(params *map[string]interface{}) (string, *banexg.WsClient, *errs.Error) {
	err := e.Authenticate(params)
	if err != nil {
		return "", nil, err
	}
	args := utils.SafeParams(params)
	marketType, _ := e.GetArgsMarketType(args, "")
	listenKey := utils.GetMapVal(e.Data, marketType+banexg.MidListenKey, "")
	wsUrl := e.Hosts.GetHost(marketType) + "/" + listenKey
	client, err := e.GetClient(wsUrl, marketType)
	return listenKey, client, err
}

func (e *Binance) WatchBalance(params *map[string]interface{}) (chan banexg.Balances, *errs.Error) {
	_, client, err := e.getAuthClient(params)
	if err != nil {
		return nil, err
	}
	balances, err := e.FetchBalance(params)
	if err != nil {
		return nil, err
	}
	e.MarBalances[client.MarketType] = balances
	args := utils.SafeParams(params)
	chanKey := client.URL + "#balance"
	create := func(cap int) chan banexg.Balances { return make(chan banexg.Balances, cap) }
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, args)
	e.AddWsChanRefs(chanKey, "account")
	out <- *balances
	return out, nil
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
	banexg.WriteOutChan(e.Exchange, chanKey, *kline, true)
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

/*
handleBalance
处理现货余额变动更新消息
*/
func (e *Binance) handleBalance(client *banexg.WsClient, msg map[string]string) {
	event, _ := utils.SafeMapVal(msg, "e", "")
	balances, ok := e.MarBalances[client.MarketType]
	if !ok {
		balances = &banexg.Balances{
			Assets: map[string]*banexg.Asset{},
		}
		e.MarBalances[client.MarketType] = balances
	}
	msgText, _ := sonic.MarshalString(msg)
	log.Info("balance update", zap.String("msg", msgText))
	evtTime, _ := utils.SafeMapVal(msg, "E", int64(0))
	balances.TimeStamp = evtTime
	if event == "balanceUpdate" {
		// 现货：余额更新，提现充值划转触发
		currencyId, _ := utils.SafeMapVal(msg, "a", "")
		code := e.SafeCurrencyCode(currencyId)
		delta, _ := utils.SafeMapVal(msg, "d", float64(0))
		if asset, ok := balances.Assets[code]; ok {
			asset.Free += delta
		}
	} else if event == "outboundAccountPosition" {
		// 现货：余额变动
		text, _ := utils.SafeMapVal(msg, "B", "")
		items := make([]struct {
			Asset string `json:"a"`
			Free  string `json:"f"`
			Lock  string `json:"l"`
		}, 0)
		err := sonic.UnmarshalString(text, &items)
		if err != nil {
			log.Error("unmarshal balance fail", zap.String("text", text), zap.Error(err))
			return
		}
		for _, item := range items {
			code := e.SafeCurrencyCode(item.Asset)
			asset, ok := balances.Assets[code]
			free, _ := strconv.ParseFloat(item.Free, 64)
			lock, _ := strconv.ParseFloat(item.Lock, 64)
			total := free + lock
			if ok {
				asset.Free = free
				asset.Used = lock
				asset.Total = total
			} else {
				asset = &banexg.Asset{Code: code, Free: free, Used: lock, Total: total}
				balances.Assets[code] = asset
			}
		}
	} else {
		log.Error("invalid balance update", zap.String("event", event))
	}
	chanKey := client.URL + "#balance"
	if outRaw, ok := e.WsOutChans[chanKey]; ok {
		out := outRaw.(chan banexg.Balances)
		out <- *balances
	}
}

type ContractAsset struct {
	Asset         string `json:"a"`
	WalletBalance string `json:"wb"`
	CrossWallet   string `json:"cw"`
	BalanceChange string `json:"bc"`
}

type ContractPosition struct {
	Symbol         string `json:"s"`
	PosAmount      string `json:"pa"`
	EntryPrice     string `json:"ep"`
	BreakEvenPrice string `json:"bep"`
	AccuRealized   string `json:"cr"`
	UnrealizedPnl  string `json:"up"`
	MarginType     string `json:"mt"`
	IsolatedWallet string `json:"iw"`
	PositionSide   string `json:"ps"`
}

/*
处理U本位合约，币本位合约，期权账户更新消息
*/
func (e *Binance) handleAccountUpdate(client *banexg.WsClient, msg map[string]string) {
	balances, ok := e.MarBalances[client.MarketType]
	if !ok {
		balances = &banexg.Balances{
			Assets: map[string]*banexg.Asset{},
		}
		e.MarBalances[client.MarketType] = balances
	}
	evtTime, _ := utils.SafeMapVal(msg, "E", int64(0))
	balances.TimeStamp = evtTime
	if client.MarketType != banexg.MarketOption {
		// linear/inverse
		text, _ := msg["a"]
		log.Info("account update", zap.String("msg", text))
		var Data = struct {
			Reason    string             `json:"m"`
			Balances  []ContractAsset    `json:"B"`
			Positions []ContractPosition `json:"P"`
		}{}
		err := sonic.UnmarshalString(text, &Data)
		if err != nil {
			log.Error("unmarshal account update fail", zap.Error(err), zap.String("text", text))
			return
		}
		for _, item := range Data.Balances {
			code := e.SafeCurrencyCode(item.Asset)
			asset, ok := balances.Assets[code]
			// 收到币安wb/cw值完全相同，bc始终是0，只能检测到总资产数量，无法获知可用余额变化
			total, _ := strconv.ParseFloat(item.WalletBalance, 64)
			change, _ := strconv.ParseFloat(item.BalanceChange, 64)
			if ok {
				asset.Free += change
				asset.Total = total
				asset.Used = total - asset.Free
			} else {
				asset = &banexg.Asset{Code: code, Free: total, Used: 0, Total: total}
				balances.Assets[code] = asset
			}
		}
		// TODO: update positions
	}
	chanKey := client.URL + "#balance"
	if outRaw, ok := e.WsOutChans[chanKey]; ok {
		out := outRaw.(chan banexg.Balances)
		out <- *balances
	}
}
func (e *Binance) handleOrderUpdate(client *banexg.WsClient, msg map[string]string) {

}
