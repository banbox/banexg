package binance

import (
	"errors"
	"fmt"
	"github.com/anyongjin/banexg"
	"github.com/anyongjin/banexg/errs"
	"github.com/anyongjin/banexg/log"
	"github.com/anyongjin/banexg/utils"
	"github.com/bytedance/sonic"
	"go.uber.org/zap"
	"strconv"
)

var (
	contOdBookLimits = []int{5, 10, 20, 50, 100, 500, 1000}
)

func (e *Binance) Stream(marType, subHash string) string {
	if stream, ok := e.streamBySubHash[subHash]; ok {
		return stream
	}
	limit, ok := e.streamLimits[marType]
	if !ok {
		limit = 50
		log.Warn("ws streamLimits not config, use default", zap.String("name", e.Name), zap.String("type", marType))
	}
	e.streamIndex += 1
	stream := strconv.Itoa(e.streamIndex % limit)
	e.streamBySubHash[subHash] = stream
	return stream
}

func (e *Binance) GetWsInfo(marType, msgHash string) (string, int, *errs.Error) {
	host := e.Hosts.GetHost(marType)
	if host == "" {
		return "", 0, errs.NewMsg(errs.CodeParamInvalid, "unsupport wss host for %s: %s", e.Name, marType)
	}
	wsUrl := host + "/" + e.Stream(marType, msgHash)
	requestId := e.wsRequestId[wsUrl] + 1
	e.wsRequestId[wsUrl] = requestId
	return wsUrl, requestId, nil
}

/*
WatchOrderBooks
watches information on open orders with bid(buy) and ask(sell) prices, volumes and other data

	:param str symbol: unified symbol of the market to fetch the order book for
	:param int [limit]: the maximum amount of order book entries to return
	:param dict [params]: extra parameters specific to the exchange API endpoint
	:returns dict: A dictionary of `order book structures <https://docs.ccxt.com/#/?id=order-book-structure>` indexed by market symbols
*/
func (e *Binance) WatchOrderBooks(symbols []string, limit int, params *map[string]interface{}) (chan banexg.OrderBook, *errs.Error) {
	/*
		# todo add support for <levels>-snapshots(depth)
		# https://github.com/binance-exchange/binance-official-api-docs/blob/master/web-socket-streams.md#partial-book-depth-streams        # <symbol>@depth<levels>@100ms or <symbol>@depth<levels>(1000ms)
		# valid <levels> are 5, 10, or 20
		#
		# default 100, max 1000, valid limits 5, 10, 20, 50, 100, 500, 1000
	*/
	if len(symbols) == 0 {
		return nil, errs.NewMsg(errs.CodeParamRequired, "symbols is required for WatchOrderBooks")
	}
	args, market, err := e.LoadArgsMarket(symbols[0], params)
	if err != nil {
		return nil, err
	}
	if limit != 0 {
		if market.Contract {
			if !utils.ArrContains(contOdBookLimits, limit) {
				return nil, errs.NewMsg(errs.CodeParamInvalid, "WatchOrderBooks.limit must be 0,5,10,20,50,100,500,1000")
			}
		} else if limit > 5000 {
			return nil, errs.NewMsg(errs.CodeParamInvalid, "WatchOrderBooks.limit must be <= 5000")
		}
	}
	/*
		# notice the differences between trading futures and spot trading
		# the algorithms use different urls in step 1
		# delta caching and merging also differs in steps 4, 5, 6
		#
		# spot/margin
		# https://binance-docs.github.io/apidocs/spot/en/#how-to-manage-a-local-order-book-correctly
		#
		# 1. Open a stream to wss://stream.binance.com:9443/ws/bnbbtc@depth.
		# 2. Buffer the events you receive from the stream.
		# 3. Get a depth snapshot from https://www.binance.com/api/v1/depth?symbol=BNBBTC&limit=1000 .
		# 4. Drop any event where u is <= lastUpdateId in the snapshot.
		# 5. The first processed event should have U <= lastUpdateId+1 AND u >= lastUpdateId+1.
		# 6. While listening to the stream, each new event's U should be equal to the previous event's u+1.
		# 7. The data in each event is the absolute quantity for a price level.
		# 8. If the quantity is 0, remove the price level.
		# 9. Receiving an event that removes a price level that is not in your local order book can happen and is normal.
		#
		# futures
		# https://binance-docs.github.io/apidocs/futures/en/#how-to-manage-a-local-order-book-correctly
		#
		# 1. Open a stream to wss://fstream.binance.com/stream?streams=btcusdt@depth.
		# 2. Buffer the events you receive from the stream. For same price, latest received update covers the previous one.
		# 3. Get a depth snapshot from https://fapi.binance.com/fapi/v1/depth?symbol=BTCUSDT&limit=1000 .
		# 4. Drop any event where u is < lastUpdateId in the snapshot.
		# 5. The first processed event should have U <= lastUpdateId AND u >= lastUpdateId
		# 6. While listening to the stream, each new event's pu should be equal to the previous event's u, otherwise initialize the process from step 3.
		# 7. The data in each event is the absolute quantity for a price level.
		# 8. If the quantity is 0, remove the price level.
		# 9. Receiving an event that removes a price level that is not in your local order book can happen and is normal.
	*/
	name := "depth"
	// 所有symbol使用相同的msgHash，确保输出到同一个chan
	var msgHash = market.Type + "@" + name
	wsUrl, requestId, err := e.GetWsInfo(market.Type, msgHash)
	if err != nil {
		return nil, err
	}

	watchRate, ok := e.WsIntvs["WatchOrderBooks"]
	if !ok {
		watchRate = 100
	}
	args["method"] = "SUBSCRIBE"
	args["id"] = requestId
	exgParams, err := e.getExgWsParams(symbols, fmt.Sprintf("%s@%dms", name, watchRate))
	if err != nil {
		return nil, err
	}
	args["params"] = exgParams
	outRaw, oldChan := e.getWsOutChan(wsUrl, msgHash, args)
	out := outRaw.(chan banexg.OrderBook)
	reqIdStr := strconv.Itoa(requestId)
	jobInfo := &banexg.WsJobInfo{
		ID:         reqIdStr,
		MsgHash:    msgHash,
		Name:       name,
		Symbols:    symbols,
		Method:     e.HandleOrderBookSub(out),
		Limit:      limit,
		MarketType: market.Type,
		Params:     args,
	}
	client, err := e.GetClient(wsUrl, market.Type)
	if err == nil {
		err = client.Write(args, reqIdStr, jobInfo)
	}
	if err != nil {
		if !oldChan {
			close(out)
			e.delWsChan(wsUrl, msgHash)
		}
		return nil, err
	}
	return out, nil
}

func (e *Binance) getExgWsParams(symbols []string, suffix string) ([]string, *errs.Error) {
	exgParams := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		mar, err := e.GetMarket(sym)
		if err != nil {
			return nil, err
		}
		subText := fmt.Sprintf("%s@%s", mar.LowercaseID, suffix)
		exgParams = append(exgParams, subText)
	}
	return exgParams, nil
}

func (e *Binance) UnWatchOrderBooks(symbols []string, params *map[string]interface{}) *errs.Error {
	if len(symbols) == 0 {
		return errs.NewMsg(errs.CodeParamRequired, "symbols required for UnWatchOrderBooks")
	}
	args, market, err := e.LoadArgsMarket(symbols[0], params)
	if err != nil {
		return err
	}
	name := "depth"
	var msgHash = market.Type + "@" + name
	wsUrl, requestId, err := e.GetWsInfo(market.Type, msgHash)
	if err != nil {
		return err
	}
	watchRate, ok := e.WsIntvs["WatchOrderBooks"]
	if !ok {
		watchRate = 100
	}
	args["method"] = "UNSUBSCRIBE"
	args["id"] = requestId
	exgParams, err := e.getExgWsParams(symbols, fmt.Sprintf("%s@%dms", name, watchRate))
	if err != nil {
		return err
	}
	args["params"] = exgParams
	client, err := e.GetClient(wsUrl, market.Type)
	if err != nil {
		return err
	}
	return client.Write(args, "", nil)
}

/*
delWsChan

	将ws的输出chan移除。
	注意：通道仍未关闭，需要手动close
*/
func (e *Binance) delWsChan(wsUrl, msgHash string) {
	chanKey := wsUrl + "#" + msgHash
	delete(e.WsOutChans, chanKey)
}

/*
getWsOutChan
获取指定msgHash的输出通道
如果不存在则创建新的并存储
*/
func (e *Binance) getWsOutChan(wsUrl, msgHash string, args map[string]interface{}) (interface{}, bool) {
	chanCap := utils.PopMapVal(args, banexg.ParamChanCap, 0)
	chanKey := wsUrl + "#" + msgHash
	if out, ok := e.WsOutChans[chanKey]; ok {
		return out, ok
	}
	out := make(chan banexg.OrderBook, chanCap)
	e.WsOutChans[chanKey] = out
	return out, false
}

func makeHandleWsMsg(e *Binance) banexg.FuncOnWsMsg {
	return func(wsUrl string, msg map[string]string) {
		event, ok := msg["e"]
		if !ok {
			msgText, _ := sonic.MarshalString(msg)
			log.Error("no event ws msg", zap.String("msg", msgText))
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

func (e *Binance) handleOrderBook(client *banexg.WsClient, msg map[string]string) {
	/*
		# initial snapshot is fetched with ccxt's fetchOrderBook
		# the feed does not include a snapshot, just the deltas
		#
		#     {
		#         "e": "depthUpdate",  # Event type
		#         "E": 1577554482280,  # Event time
		#         "s": "BNBBTC",  # Symbol
		#         "U": 157,  # First update ID in event
		#         "u": 160,  # Final update ID in event
		#         "b": [ # bids
		#             ["0.0024", "10"],  # price, size
		#         ],
		#         "a": [ # asks
		#             ["0.0026", "100"],  # price, size
		#         ]
		#     }
	*/
	marketId, _ := msg["s"]
	market := e.GetMarketById(marketId, client.MarketType)
	urlZap := zap.String("url", client.URL)
	if market == nil {
		log.Error("no market for ws depth update", urlZap, zap.String("symbol", marketId))
		return
	}
	book, ok := e.OrderBooks[market.Symbol]
	if !ok {
		return
	}
	nonce := book.Nonce
	if nonce == 0 {
		book.Cache = append(book.Cache, msg)
		log.Info("book nonce empty, cache")
		return
	}
	var msgHash = fmt.Sprintf("%s#%s@depth", client.URL, client.MarketType)
	outRaw, outOk := e.WsOutChans[msgHash]
	var out chan banexg.OrderBook
	if outOk {
		out = outRaw.(chan banexg.OrderBook)
	} else {
		log.Error("ws od book chan closed", zap.String("k", msgHash))
	}
	var zero = int64(0)
	U, _ := utils.SafeMapVal(msg, "U", zero)
	u, _ := utils.SafeMapVal(msg, "u", zero)
	pu, _ := utils.SafeMapVal(msg, "pu", zero)
	var err error
	if pu == 0 {
		// spot
		// 4. Drop any event where u is <= lastUpdateId in the snapshot
		if u > nonce {
			timestamp, _ := utils.SafeMapVal(msg, "timestamp", zero)
			valid := false
			if timestamp == 0 {
				// 5. The first processed event should have U <= lastUpdateId+1 AND u >= lastUpdateId+1
				valid = U-1 <= nonce && u-1 >= nonce
			} else {
				// 6. While listening to the stream, each new event's U should be equal to the previous event's u+1.
				valid = U-1 == nonce
			}
			if valid {
				e.handleOrderBookMsg(msg, book)
				if nonce < book.Nonce && outOk {
					out <- *book
				}
			} else {
				err = errors.New("out of date")
			}
		}
	} else {
		// contract
		// 4. Drop any event where u is < lastUpdateId in the snapshot
		log.Debug("depth msg", zap.Int64("nonce", nonce), zap.Int64("u", u), zap.Int64("U", U),
			zap.Int64("pu", pu))
		if u >= nonce {
			// 5. The first processed event should have U <= lastUpdateId AND u >= lastUpdateId
			// 6. While listening to the stream, each new event's pu should be equal to the previous event's u, otherwise initialize the process from step 3
			if U <= nonce || pu == nonce {
				e.handleOrderBookMsg(msg, book)
				if nonce < book.Nonce && outOk {
					out <- *book
				}
			} else {
				err = errors.New("out of date")
			}
		}
	}
	if err != nil {
		msgText, _ := sonic.MarshalString(msg)
		log.Error("ws order book received an out-of-order nonce", urlZap, zap.String("msg", msgText),
			zap.Int64("nonce", nonce))
		delete(e.OrderBooks, market.Symbol)
	}
}

func (e *Binance) handleOrderBookMsg(msg map[string]string, book *banexg.OrderBook) {
	var zero = int64(0)
	u, _ := utils.SafeMapVal(msg, "u", zero)
	at, ok := msg["a"]
	if !ok {
		log.Error("asks not found in ws depth")
		at = "[]"
	}
	book.SetSide(at, false)
	bt, ok := msg["b"]
	if !ok {
		log.Error("bids not found in ws depth")
		bt = "[]"
	}
	book.SetSide(bt, true)
	book.Nonce = u
	timestamp, err := utils.SafeMapVal(msg, "E", zero)
	if err == nil {
		book.TimeStamp = timestamp
	}
}

func (e *Binance) handleTrade(client *banexg.WsClient, msg map[string]string) {

}

func (e *Binance) handleOhlcv(client *banexg.WsClient, msg map[string]string) {

}

func (e *Binance) handleTicker(client *banexg.WsClient, msg map[string]string) {

}
func (e *Binance) handleBalance(client *banexg.WsClient, msg map[string]string) {

}
func (e *Binance) handleAccountUpdate(client *banexg.WsClient, msg map[string]string) {

}
func (e *Binance) handleOrderUpdate(client *banexg.WsClient, msg map[string]string) {

}
func (e *Binance) HandleOrderBookSub(out chan banexg.OrderBook) banexg.FuncOnWsMethod {
	return func(wsUrl string, msg map[string]string, info *banexg.WsJobInfo) {
		err := e.checkWsError(msg)
		urlZap := zap.String("url", wsUrl)
		if err != nil {
			close(out)
			log.Error("sub order error", urlZap, zap.Error(err))
			return
		}
		client, ok := e.WSClients[wsUrl]
		if !ok {
			close(out)
			log.Error("no ws client for", urlZap)
			return
		}
		symbols := info.Symbols
		if len(symbols) == 0 {
			symbols = append(symbols, info.Symbol)
		}
		var failSymbols []string
		for _, symbol := range symbols {
			delete(e.OrderBooks, symbol)
			e.OrderBooks[symbol] = &banexg.OrderBook{
				Symbol: symbol,
				Cache:  make([]map[string]string, 0),
			}
			err = e.fetchOrderBookSnapshot(client, symbol, info)
			if err != nil {
				failSymbols = append(failSymbols, symbol)
			}
		}
		if len(failSymbols) > 0 {
			log.Error("sub ws od books fail", zap.Strings("symbols", failSymbols))
			err = e.UnWatchOrderBooks(failSymbols, nil)
			if err != nil {
				log.Error("unwatch ws order book fail", zap.Strings("symbols", failSymbols), zap.Error(err))
			}
		}
	}
}

func (e *Binance) fetchOrderBookSnapshot(client *banexg.WsClient, symbol string, info *banexg.WsJobInfo) *errs.Error {
	// 3. Get a depth snapshot from https://www.binance.com/api/v1/depth?symbol=BNBBTC&limit=1000 .
	// default 100, max 1000, valid limits 5, 10, 20, 50, 100, 500, 1000
	book, err := e.FetchOrderBook(symbol, info.Limit, &info.Params)
	if err != nil {
		return err
	}
	oldBook, ok := e.OrderBooks[symbol]
	var cache []map[string]string
	if ok && len(oldBook.Cache) > 0 {
		cache = oldBook.Cache
	}
	e.OrderBooks[symbol] = book
	if len(cache) > 0 {
		var zero = int64(0)
		for _, msg := range cache {
			U, _ := utils.SafeMapVal(msg, "U", zero)
			u, _ := utils.SafeMapVal(msg, "u", zero)
			pu, _ := utils.SafeMapVal(msg, "pu", zero)
			nonce := book.Nonce
			if e.IsContract(info.MarketType) {
				//4. Drop any event where u is < lastUpdateId in the snapshot
				if u < nonce {
					continue
				}
				// 5. The first processed event should have U <= lastUpdateId AND u >= lastUpdateId
				if U <= nonce && u >= nonce || pu == nonce {
					e.handleOrderBookMsg(msg, book)
				}
			} else {
				// 4. Drop any event where u is <= lastUpdateId in the snapshot
				if u <= nonce {
					continue
				}
				// 5. The first processed event should have U <= lastUpdateId+1 AND u >= lastUpdateId+1
				if U-1 <= nonce && u-1 >= nonce {
					e.handleOrderBookMsg(msg, book)
				}
			}
		}
	}
	outRaw, ok := e.WsOutChans[info.MsgHash]
	if ok {
		out := outRaw.(chan banexg.OrderBook)
		out <- *book
	}
	return nil
}

/*
checkWsError
从websocket返回的消息结果中，检查是否有错误信息
*/
func (e *Binance) checkWsError(msg map[string]string) *errs.Error {
	errRaw, ok := msg["error"]
	if ok {
		var err = &errs.Error{}
		errData, _ := sonic.Marshal(errRaw)
		_ = sonic.Unmarshal(errData, err)
		return err
	}
	status, ok := msg["status"]
	if ok && status != "200" {
		statusVal, e := strconv.Atoi(status)
		if e != nil {
			return nil
		}
		msgStr, _ := sonic.MarshalString(msg)
		return errs.NewMsg(statusVal, msgStr)
	}
	return nil
}
