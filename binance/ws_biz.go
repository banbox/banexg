package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"github.com/banbox/bntp"
	"go.uber.org/zap"
)

func makeHandleWsMsg(e *Binance) banexg.FuncOnWsMsg {
	return func(client *banexg.WsClient, item *banexg.WsMsg) {
		if item.Event == "" && client.MarketType == banexg.MarketMargin {
			nested, err := unwrapUserDataEvent(item.Text)
			if err != nil {
				log.Warn("decode margin user data event fail", zap.Error(err))
				return
			}
			if nested != nil {
				item = nested
			}
		}
		if item.Event == "" {
			if item.ID != "" {
				// 任务结果返回
				err := banexg.CheckWsErrorWith(item.Object, func(status int, content string) *errs.Error {
					return mapBinanceError(nil, status, content)
				})
				if err != nil {
					log.Error("ws job fail", zap.String("job", item.ID), zap.Error(err))
				} else {
					log.Debug("ws job ok", zap.String("job", item.ID))
				}
			} else {
				log.Warn("no event ws msg", zap.String("msg", item.Text))
			}
			return
		}
		var msgList = item.List
		if !item.IsArray {
			msgList = []map[string]string{item.Object}
		}
		var msg = item.Object
		switch item.Event {
		case "depthUpdate":
			e.handleOrderBook(client, msg)
		case "trade":
			e.handleTrade(client, msg)
		case "aggTrade":
			e.handleTrade(client, msg)
		case "kline":
			e.handleOHLCV(client, msg)
		case "markPrice_kline":
			e.handleOHLCV(client, msg)
		case "indexPrice_kline":
			e.handleOHLCV(client, msg)
		case "markPriceUpdate":
			// linear/inverse
			e.handleMarkPrices(client, msgList, item.IsArray)
		case "markPrice":
			// option
			e.handleMarkPrices(client, msgList, item.IsArray)
		case "24hrTicker":
			//spot/linear/inverse/option
			e.handleTickers(client, msgList)
		case "24hrMiniTicker":
			//spot/linear/inverse
			e.handleTickers(client, msgList)
		case "bookTicker":
			e.handleTickers(client, msgList)
		case "openInterest":
			// option 合约持仓量
			break
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
		case "ACCOUNT_CONFIG_UPDATE":
			e.handleAccountConfigUpdate(client, msg)
		case "TRADE_LITE":
		default:
			log.Warn("unhandle ws msg", zap.String("msg", item.Text))
		}
	}
}

func makeHandleWsReCon(e *Binance) banexg.FuncOnWsReCon {
	return func(client *banexg.WsClient, connID int) *errs.Error {
		if client.MarketType == banexg.MarketMargin {
			acc, err := e.GetAccount(client.AccName)
			if err != nil {
				return err
			}
			acc.LockData.Lock()
			listenToken := utils.GetMapVal(acc.Data, banexg.MarketMargin+banexg.MidListenKey, "")
			acc.LockData.Unlock()
			return e.subscribeMarginUserData(client, connID, listenToken)
		}
		subParams := client.GetSubKeys(connID)
		if len(subParams) == 0 {
			return nil
		}
		zapFields := []zap.Field{zap.String("url", client.URL), zap.Int("id", connID),
			zap.Int("job", len(subParams))}
		log.Info("re-subscribe ws", zapFields...)
		err := e.WriteWSMsg(client, connID, true, subParams, nil, nil)
		if err != nil {
			return err
		}
		log.Info("re-subscribe ok", zapFields...)
		return nil
	}
}

type AuthRes struct {
	ListenKey   string `json:"listenKey"`
	ListenToken string `json:"listenToken"`
}

const marginUserDataSubKey = "userDataStream.subscribe.listenToken"

type userDataEventEnvelope struct {
	Event json.RawMessage `json:"event"`
}

func unwrapUserDataEvent(content string) (*banexg.WsMsg, error) {
	var envelope userDataEventEnvelope
	if err := utils.UnmarshalString(content, &envelope, utils.JsonNumDefault); err != nil {
		return nil, err
	}
	if len(envelope.Event) == 0 || string(envelope.Event) == "null" {
		return nil, nil
	}
	msg, wsErr := banexg.NewWsMsg(string(envelope.Event))
	if wsErr != nil {
		return nil, wsErr
	}
	return msg, nil
}

func marginUserDataRequest(id int, listenToken string) map[string]interface{} {
	return map[string]interface{}{
		"id":     id,
		"method": marginUserDataSubKey,
		"params": map[string]interface{}{"listenToken": listenToken},
	}
}

func (e *Binance) postListenKey(acc *banexg.Account, params map[string]interface{}) *errs.Error {
	zeroVal := int64(0)
	args := utils.SafeParams(params)
	marketType, _ := e.GetArgsMarketType(args, "")
	lastTimeKey := marketType + "lastAuthTime"
	authField := marketType + banexg.MidListenKey
	lastAuthTime := utils.GetMapVal(acc.Data, lastTimeKey, zeroVal)
	authRefreshSecs := utils.GetMapVal(e.Options, banexg.OptAuthRefreshSecs, 1200)
	refreshDuration := int64(authRefreshSecs * 1000)
	curTime := e.MilliSeconds()
	if curTime-lastAuthTime <= refreshDuration {
		return nil
	}
	method := MethodPublicPostUserDataStream
	if marketType == banexg.MarketLinear {
		method = MethodFapiPrivatePostListenKey
	} else if marketType == banexg.MarketInverse {
		method = MethodDapiPrivatePostListenKey
	} else if marketType == banexg.MarketMargin {
		method = MethodSapiPostUserListenToken
	}
	rsp := e.RequestApiRetry(context.Background(), method, args, 1)
	if rsp.Error != nil {
		return rsp.Error
	}
	var res = AuthRes{}
	err2 := utils.UnmarshalString(rsp.Content, &res, utils.JsonNumDefault)
	if err2 != nil {
		return errs.New(errs.CodeUnmarshalFail, err2)
	}
	listenKey := res.ListenKey
	if marketType == banexg.MarketMargin {
		listenKey = res.ListenToken
	}
	if listenKey == "" {
		return errs.NewMsg(errs.CodeUnmarshalFail, "listen token missing in response")
	}
	acc.LockData.Lock()
	acc.Data[lastTimeKey] = curTime
	acc.Data[authField] = listenKey
	acc.LockData.Unlock()
	if marketType != banexg.MarketMargin {
		refreshAfter := time.Duration(authRefreshSecs) * time.Second
		time.AfterFunc(refreshAfter, func() {
			e.keepAliveListenKey(acc, params)
		})
	}
	return nil
}

func (e *Binance) keepAliveListenKey(acc *banexg.Account, params map[string]interface{}) {
	args := utils.SafeParams(params)
	marketType, _ := e.GetArgsMarketType(args, "")
	if marketType == banexg.MarketMargin {
		return
	}
	lastTimeKey := marketType + "lastAuthTime"
	authField := marketType + banexg.MidListenKey
	acc.LockData.Lock()
	listenKey := utils.GetMapVal(acc.Data, authField, "")
	acc.LockData.Unlock()
	if listenKey == "" {
		return
	}
	var success = false
	defer func() {
		if success {
			return
		}
		acc.LockData.Lock()
		delete(acc.Data, authField)
		delete(acc.Data, lastTimeKey)
		acc.LockData.Unlock()
		clientKey := acc.Name + "@" + e.GetHost(marketType) + "/" + listenKey
		if client, ok := e.WSClients[clientKey]; ok {
			conns, lock := client.LockConns()
			connList := utils.ValsOfMap(conns)
			lock.Unlock()
			for _, conn := range connList {
				_ = conn.WriteClose()
			}
			log.Warn("renew listenKey fail, close ws client", zap.String("key", clientKey))
		}
	}()
	method := MethodPublicPutUserDataStream
	if marketType == banexg.MarketLinear {
		method = MethodFapiPrivatePutListenKey
	} else if marketType == banexg.MarketInverse {
		method = MethodDapiPrivatePutListenKey
	} else {
		args[banexg.MidListenKey] = listenKey
	}
	rsp := e.RequestApiRetry(context.Background(), method, args, 1)
	if rsp.Error != nil {
		if rsp.Error.Code == errs.CodeStreamExpired || rsp.Error.Code == errs.CodeExchangeError {
			err := e.postListenKey(acc, params)
			if err != nil {
				log.Error("post new listenKey fail", zap.Error(err))
				return
			}
			success = true
			return
		}
		log.Error("refresh listenKey fail", zap.Error(rsp.Error))
		return
	}
	success = true
	acc.LockData.Lock()
	acc.Data[lastTimeKey] = e.MilliSeconds()
	authRefreshSecs := utils.GetMapVal(acc.Data, banexg.OptAuthRefreshSecs, 1200)
	acc.LockData.Unlock()
	refreshDuration := time.Duration(authRefreshSecs) * time.Second
	time.AfterFunc(refreshDuration, func() {
		e.keepAliveListenKey(acc, params)
	})
}

func (e *Binance) getAuthClient(params map[string]interface{}) (string, *banexg.WsClient, *errs.Error) {
	_, err := e.LoadMarkets(false, nil)
	if err != nil {
		return "", nil, err
	}
	acc, err := e.GetAccount(e.GetAccName(params))
	if err != nil {
		return "", nil, err
	}
	err = e.AuthWS(acc, params)
	if err != nil {
		return "", nil, err
	}
	args := utils.SafeParams(params)
	marketType, _ := e.GetArgsMarketType(args, "")
	acc.LockData.Lock()
	listenKey := utils.GetMapVal(acc.Data, marketType+banexg.MidListenKey, "")
	acc.LockData.Unlock()
	wsUrl := e.GetHost(marketType) + "/" + listenKey
	if marketType == banexg.MarketLinear {
		wsUrl = linearPrivateWsHost(e.GetHost(marketType)) + "/" + listenKey
	} else if marketType == banexg.MarketMargin {
		wsUrl = e.GetHost(WssApi)
	}
	client, err := e.GetClient(wsUrl, marketType, acc.Name)
	if err == nil && marketType == banexg.MarketMargin && !client.HasSubKeyPrefix(marginUserDataSubKey) {
		err = e.subscribeMarginUserData(client, 0, listenKey)
	}
	return listenKey, client, err
}

func (e *Binance) subscribeMarginUserData(client *banexg.WsClient, connID int, listenToken string) *errs.Error {
	if listenToken == "" {
		return errs.NewMsg(errs.CodeParamRequired, "margin listen token is required")
	}
	_, conn := client.UpdateSubs(connID, true, []string{marginUserDataSubKey})
	if conn == nil {
		return errs.NewMsg(errs.CodeRunTime, "get margin user data ws conn fail")
	}
	id := e.nextId(client)
	return client.Write(conn, marginUserDataRequest(id, listenToken), nil)
}

func (e *Binance) WatchBalance(params map[string]interface{}) (chan *banexg.Balances, *errs.Error) {
	_, client, err := e.getAuthClient(params)
	if err != nil {
		return nil, err
	}
	balances, err := e.FetchBalance(params)
	if err != nil {
		return nil, err
	}
	acc, err := e.GetAccount(client.AccName)
	if err != nil {
		return nil, err
	}
	acc.LockBalance.Lock()
	acc.MarBalances[client.MarketType] = balances
	acc.LockBalance.Unlock()
	args := utils.SafeParams(params)
	chanKey := client.Prefix("balance")
	create := func(cap int) chan *banexg.Balances { return make(chan *banexg.Balances, cap) }
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, args)
	e.AddWsChanRefs(chanKey, "account")
	out <- balances
	return out, nil
}

func (e *Binance) WatchPositions(params map[string]interface{}) (chan []*banexg.Position, *errs.Error) {
	_, client, err := e.getAuthClient(params)
	if err != nil {
		return nil, err
	}
	acc, err := e.GetAccount(client.AccName)
	if err != nil {
		return nil, err
	}
	positions, err := e.FetchPositions(nil, params)
	if err != nil {
		return nil, err
	}
	acc.LockPos.Lock()
	acc.MarPositions[client.MarketType] = positions
	acc.LockPos.Unlock()
	args := utils.SafeParams(params)
	chanKey := client.Prefix("positions")
	create := func(cap int) chan []*banexg.Position { return make(chan []*banexg.Position, cap) }
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, args)
	e.AddWsChanRefs(chanKey, "account")
	out <- positions
	return out, nil
}

/*
WatchOHLCVs
watches historical candlestick data containing the open, high, low, and close price, and the volume of a market
:param map[string]string jobs: array of arrays containing unified symbols and timeframes to fetch OHLCV data for, example {{'BTC/USDT': '1m'}, {'LTC/USDT': '5m'}}
:param dict [params]: extra parameters specific to the exchange API endpoint
:returns int[][]: A list of candles ordered, open, high, low, close, volume
*/
func (e *Binance) WatchOHLCVs(jobs [][2]string, params map[string]interface{}) (chan *banexg.PairTFKline, *errs.Error) {
	chanKey, symbols, args, err := e.prepareOHLCVSub(true, jobs, params)
	if err != nil {
		return nil, err
	}

	create := func(cap int) chan *banexg.PairTFKline { return make(chan *banexg.PairTFKline, cap) }
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, args)
	e.AddWsChanRefs(chanKey, symbols...)
	e.DumpWS("WatchOHLCVs", jobs)
	return out, nil
}

func (e *Binance) UnWatchOHLCVs(jobs [][2]string, params map[string]interface{}) *errs.Error {
	chanKey, symbols, _, err := e.prepareOHLCVSub(false, jobs, params)
	if err != nil {
		return err
	}
	e.DelWsChanRefs(chanKey, symbols...)
	return nil
}

func (e *Binance) WatchMarkPrices(symbols []string, params map[string]interface{}) (chan map[string]float64, *errs.Error) {
	chanKey, args, err := e.prepareMarkPrices(true, symbols, params)
	if err != nil {
		return nil, err
	}
	create := func(cap int) chan map[string]float64 { return make(chan map[string]float64, cap) }
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, args)
	e.AddWsChanRefs(chanKey, "markPrice")
	e.DumpWS("WatchMarkPrices", symbols)
	return out, nil
}

func (e *Binance) UnWatchMarkPrices(symbols []string, params map[string]interface{}) *errs.Error {
	chanKey, _, err := e.prepareMarkPrices(false, symbols, params)
	if err != nil {
		return err
	}
	e.DelWsChanRefs(chanKey, "markPrice")
	return nil
}

func (e *Binance) prepareMarkPrices(isSub bool, symbols []string, params map[string]interface{}) (string, map[string]interface{}, *errs.Error) {
	args := utils.SafeParams(params)
	marketType, _, err := e.LoadArgsMarketType(args, symbols...)
	if err != nil {
		return "", nil, err
	}
	if !e.IsContract(marketType) && marketType != banexg.MarketOption {
		return "", nil, errs.NewMsg(errs.CodeUnsupportMarket, "WatchMarkPrices support linear/inverse/option, current: %s", marketType)
	}
	msgHash := marketType + "@markPrice"
	client, err := e.GetWsClient(marketType, msgHash)
	if err != nil {
		return "", nil, err
	}
	intv := utils.PopMapVal(args, banexg.ParamInterval, "")
	if intv != "" {
		if intv != "1s" {
			return "", nil, errs.NewMsg(errs.CodeParamInvalid, "ParamInterval must be 1s or empty")
		}
		intv = "@" + intv
	}
	if len(symbols) == 0 {
		symbols = []string{"!markPrice@arr" + intv}
	}
	err = e.WriteWSMsg(client, 0, isSub, symbols, func(m *banexg.Market, _ int) string {
		return m.LowercaseID + "@markPrice" + intv
	}, nil)
	if err != nil {
		return "", nil, err
	}
	chanKey := client.Prefix(msgHash)
	return chanKey, args, nil
}

func (e *Binance) handleMarkPrices(client *banexg.WsClient, msgList []map[string]string, isArray bool) {
	//evtTime, _ := utils.SafeMapVal(msgList[0], "E", int64(0))
	//e.KeyTimeStamps["markPrices"] = evtTime
	e.MarkPriceLock.Lock()
	data, ok := e.MarkPrices[client.MarketType]
	if !ok {
		data = map[string]float64{}
		e.MarkPrices[client.MarketType] = data
	}
	var res = map[string]float64{}
	var marketId string
	for _, msg := range msgList {
		marketId, _ = utils.SafeMapVal(msg, "s", "")
		markPrice, _ := utils.SafeMapVal(msg, "p", float64(0))
		symbol := e.SafeSymbol(marketId, "", client.MarketType)
		if symbol == "" {
			continue
		}
		res[symbol] = markPrice
	}
	maps.Copy(data, res)
	e.MarkPriceLock.Unlock()
	marketId = strings.ToLower(marketId)
	var subsKey string
	if client.MarketType == banexg.MarketLinear {
		if isArray {
			// 仅U本位合约有全市场标记价格推送
			subsKey = "!markPrice@arr"
		} else {
			subsKey = marketId + "@markPrice"
		}
	} else if client.MarketType == banexg.MarketInverse {
		// 币本位合约，只取_隔开的第一个
		marketId = strings.Split(marketId, "_")[0]
		subsKey = marketId + "@markPrice"
	} else {
		// 期权，使用-隔开的第一个
		marketId = strings.Split(marketId, "-")[0]
		subsKey = marketId + "@markPrice"
	}
	client.SetSubsKeyStamp(subsKey, bntp.UTCStamp())
	chanKey := client.Prefix(client.MarketType + "@markPrice")
	banexg.WriteOutChan(e.Exchange, chanKey, res, true)
}

func (e *Binance) handleTrade(client *banexg.WsClient, msg map[string]string) {
	// event: aggTrade/trade
	event, _ := utils.SafeMapVal(msg, "e", "trade")
	var isAggTrade = event == "aggTrade"
	var chanKey = client.Prefix(client.MarketType + "@" + event)
	marketId, _ := utils.SafeMapVal(msg, "s", "")
	var symbol = e.SafeSymbol(marketId, "", client.MarketType)
	client.SetSubsKeyStamp(strings.ToLower(marketId)+"@"+event, bntp.UTCStamp())
	var tradeId string
	if isAggTrade {
		tradeId, _ = utils.SafeMapVal(msg, "a", "")
	} else {
		tradeId, _ = utils.SafeMapVal(msg, "t", "")
	}
	maker, _ := utils.SafeMapVal(msg, "m", false)
	quality, _ := utils.SafeMapVal(msg, "q", float64(0))
	price, _ := utils.SafeMapVal(msg, "p", float64(0))
	tradeTime, _ := utils.SafeMapVal(msg, "T", int64(0))
	var trade = &banexg.Trade{
		ID:        tradeId,
		Symbol:    symbol,
		Amount:    quality,
		Price:     price,
		Cost:      price * quality,
		Timestamp: tradeTime,
		Maker:     maker,
		Info:      utils.ToStdMap(msg),
	}
	if maker {
		trade.Side = banexg.OdSideSell
	} else {
		trade.Side = banexg.OdSideBuy
	}
	banexg.WriteOutChan(e.Exchange, chanKey, trade, true)
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

func (e *Binance) handleOHLCV(client *banexg.WsClient, msg map[string]string) {
	/*
		https://binance-docs.github.io/apidocs/futures/cn/#k-7
	*/
	event, _ := utils.SafeMapVal(msg, "e", "kline")
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
	err = utils.UnmarshalString(kText, &k, utils.JsonNumDefault)
	if err != nil {
		log.Error("unmarshal ws kline fail", zap.String("k", kText), zap.Error(err))
		return
	}
	var chanKey = client.Prefix(client.MarketType + "@" + event)
	var marketId string
	if event == "indexPriceKline" {
		marketId, _ = utils.SafeMapVal(msg, "ps", "")
	} else if k.Symbol != "" {
		marketId = k.Symbol
	} else if k.PairSymbol != "" {
		marketId = k.PairSymbol
	}
	client.SetSubsKeyStamp(strings.ToLower(marketId)+"@"+event, bntp.UTCStamp())
	o, _ := strconv.ParseFloat(k.Open, 64)
	c, _ := strconv.ParseFloat(k.Close, 64)
	h, _ := strconv.ParseFloat(k.High, 64)
	l, _ := strconv.ParseFloat(k.Low, 64)
	v, _ := strconv.ParseFloat(k.Volume, 64)
	var kline = &banexg.PairTFKline{
		Symbol:    e.SafeSymbol(marketId, "", client.MarketType),
		TimeFrame: k.TimeFrame,
		Kline: banexg.Kline{
			Time:   k.OpenTime,
			Open:   o,
			Close:  c,
			High:   h,
			Low:    l,
			Volume: v,
		},
	}
	banexg.WriteOutChan(e.Exchange, chanKey, kline, true)
}

func (e *Binance) prepareOHLCVSub(isSub bool, jobs [][2]string, params map[string]interface{}) (string, []string, map[string]interface{}, *errs.Error) {
	if len(jobs) == 0 {
		return "", nil, nil, errs.NewMsg(errs.CodeParamRequired, "symbols is required")
	}
	var first string
	for _, it := range jobs {
		first = it[0]
		break
	}
	args, market, err := e.LoadArgsMarket(first, params)
	if err != nil {
		return "", nil, nil, err
	}
	name := utils.PopMapVal(args, banexg.ParamName, "kline")
	msgHash := market.Type + "@" + name
	client, err := e.GetWsClient(market.Type, msgHash)
	if err != nil {
		return "", nil, nil, err
	}

	symbols := make([]string, 0, len(jobs))
	for _, k := range jobs {
		symbols = append(symbols, k[0])
	}
	err = e.WriteWSMsg(client, 0, isSub, symbols, func(m *banexg.Market, i int) string {
		marketId := m.LowercaseID
		if name == "indexPriceKline" {
			marketId = strings.Replace(marketId, "_perp", "", -1)
		}
		tf := jobs[i][1]
		return fmt.Sprintf("%s@%s_%s", marketId, name, tf)
	}, nil)
	if err != nil {
		return "", nil, nil, err
	}
	chanKey := client.Prefix(msgHash)
	return chanKey, symbols, args, nil
}

func (e *Binance) handleTickers(client *banexg.WsClient, msgList []map[string]string) {

}

/*
handleBalance
处理现货余额变动更新消息
*/
func (e *Binance) handleBalance(client *banexg.WsClient, msg map[string]string) {
	event, _ := utils.SafeMapVal(msg, "e", "")
	acc, err := e.GetAccount(client.AccName)
	if err != nil {
		log.Error("account for ws not found", zap.String("name", client.AccName))
		return
	}
	acc.LockBalance.Lock()
	balances, ok := acc.MarBalances[client.MarketType]
	if !ok {
		balances = &banexg.Balances{
			Assets: map[string]*banexg.Asset{},
		}
		acc.MarBalances[client.MarketType] = balances
	}
	acc.LockBalance.Unlock()
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
		err := utils.UnmarshalString(text, &items, utils.JsonNumDefault)
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
	chanKey := client.Prefix("balance")
	banexg.WriteOutChan(e.Exchange, chanKey, balances, true)
}

type ContractAsset struct {
	Asset         string `json:"a"`
	WalletBalance string `json:"wb"`
	CrossWallet   string `json:"cw"`
	BalanceChange string `json:"bc"`
}

type WSContractPosition struct {
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
	updBalance := false
	updPosition := false
	acc, err := e.GetAccount(client.AccName)
	if err != nil {
		log.Error("account for ws client not found", zap.String("name", client.AccName))
		return
	}
	acc.LockBalance.Lock()
	balances, ok := acc.MarBalances[client.MarketType]
	if !ok {
		balances = &banexg.Balances{
			Assets: map[string]*banexg.Asset{},
		}
		acc.MarBalances[client.MarketType] = balances
	}
	acc.LockBalance.Unlock()
	acc.LockPos.Lock()
	positions, ok := acc.MarPositions[client.MarketType]
	if !ok {
		positions = make([]*banexg.Position, 0)
		acc.MarPositions[client.MarketType] = positions
	}
	acc.LockPos.Unlock()
	posMap := make(map[string]*banexg.Position)
	for _, p := range positions {
		posMap[p.Symbol+"#"+p.Side] = p
	}
	evtTime, _ := utils.SafeMapVal(msg, "E", int64(0))
	balances.TimeStamp = evtTime
	if client.MarketType != banexg.MarketOption {
		// linear/inverse
		text, _ := msg["a"]
		var Data = struct {
			Reason    string               `json:"m"`
			Balances  []ContractAsset      `json:"B"`
			Positions []WSContractPosition `json:"P"`
		}{}
		raw, err_ := utils.UnmarshalStringMap(text, &Data)
		if err_ != nil {
			log.Error("unmarshal account update fail", zap.Error(err_), zap.String("text", text))
			return
		}
		for _, item := range Data.Balances {
			code := e.SafeCurrencyCode(item.Asset)
			asset, ok := balances.Assets[code]
			// 收到币安wb/cw值完全相同，bc始终是0，只能检测到总资产数量，无法获知可用余额变化
			walletBalance, _ := strconv.ParseFloat(item.WalletBalance, 64)
			if ok {
				asset.Free = walletBalance
			} else {
				asset = &banexg.Asset{Code: code, Free: walletBalance}
				balances.Assets[code] = asset
			}
		}
		var posList []map[string]interface{}
		posList = utils.GetMapVal(raw, "P", posList)
		for i, pos := range Data.Positions {
			symbol := e.SafeSymbol(pos.Symbol, "", client.MarketType)
			if symbol == "" {
				continue
			}
			side := strings.ToLower(pos.PositionSide)
			key := symbol + "#" + side
			p, ok := posMap[key]
			if !ok {
				p = &banexg.Position{
					Info:       posList[i],
					Symbol:     symbol,
					Side:       side,
					Hedged:     side != banexg.PosSideBoth,
					MarginMode: pos.MarginType,
				}
				posMap[key] = p
			}
			p.TimeStamp = evtTime
			p.EntryPrice, _ = strconv.ParseFloat(pos.EntryPrice, 64)
			p.UnrealizedPnl, _ = strconv.ParseFloat(pos.UnrealizedPnl, 64)
			p.Contracts, _ = strconv.ParseFloat(pos.PosAmount, 64)
		}
		positions = make([]*banexg.Position, 0, len(posMap))
		for _, p := range posMap {
			if p.Contracts == 0 {
				continue
			}
			positions = append(positions, p)
		}
		acc.LockPos.Lock()
		acc.MarPositions[client.MarketType] = positions
		acc.LockPos.Unlock()
		updBalance = len(Data.Balances) > 0
		updPosition = len(Data.Positions) > 0
	} else {
		// TODO: support account update for option market
		return
	}
	if updBalance {
		banexg.WriteOutChan(e.Exchange, client.Prefix("balance"), balances, true)
	}
	if updPosition {
		acc.LockPos.Lock()
		positions = acc.MarPositions[client.MarketType]
		acc.LockPos.Unlock()
		banexg.WriteOutChan(e.Exchange, client.Prefix("positions"), positions, true)
	}
}
func (e *Binance) handleOrderUpdate(client *banexg.WsClient, msg map[string]string) {
	event, _ := utils.SafeMapVal(msg, "e", "")
	if event == "ORDER_TRADE_UPDATE" {
		objText, _ := utils.SafeMapVal(msg, "o", "")
		var obj = map[string]interface{}{}
		err := utils.UnmarshalString(objText, &obj, utils.JsonNumStr)
		if err != nil {
			log.Error("unmarshal ORDER_TRADE_UPDATE fail", zap.String("o", objText), zap.Error(err))
			return
		}
		msg = utils.MapValStr(obj)
	}
	trade := parseMyTrade(msg)
	market := e.GetMarketById(trade.Symbol, client.MarketType)
	if market == nil {
		log.Error("no market found for my trade", zap.String("symbol", trade.Symbol))
		return
	}
	trade.Symbol = market.Symbol
	if trade.Fee != nil {
		trade.Fee.Currency = e.SafeCurrencyCode(trade.Fee.Currency)
	}

	banexg.WriteOutChan(e.Exchange, client.Prefix("mytrades"), &trade, false)
}

func (e *Binance) handleAccountConfigUpdate(client *banexg.WsClient, msg map[string]string) {
	acText, ok := msg["ac"]
	if !ok || acText == "" {
		return
	}
	var data = make(map[string]interface{})
	err_ := utils.UnmarshalString(acText, &data, utils.JsonNumAuto)
	if err_ != nil {
		log.Error("unmarshal AccountConfigUpdate fail", zap.String("ac", acText), zap.Error(err_))
		return
	}
	marketId := utils.GetMapVal(data, "s", "")
	leverage := int(utils.GetMapVal(data, "l", int64(0)))
	market := e.GetMarketById(marketId, client.MarketType)
	if market == nil {
		log.Error("no market found for AccountConfigUpdate", zap.String("symbol", marketId))
		return
	}
	if acc, ok := e.Accounts[client.AccName]; ok {
		acc.LockLeverage.Lock()
		acc.Leverages[market.Symbol] = leverage
		acc.LockLeverage.Unlock()
	}
	item := &banexg.AccountConfig{Symbol: market.Symbol, Leverage: leverage}
	banexg.WriteOutChan(e.Exchange, client.Prefix("accConfig"), item, false)
}
