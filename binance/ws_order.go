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
	client, msgHash, requestId, args, err := e.prepareBookArgs(symbols, params)
	if err != nil {
		return nil, err
	}
	if limit != 0 {
		if e.IsContract(client.MarketType) {
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
	args["method"] = "SUBSCRIBE"
	chanKey := client.URL + "#" + msgHash
	create := func(cap int) chan banexg.OrderBook { return make(chan banexg.OrderBook, cap) }
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, args)
	e.AddWsChanRefs(chanKey, symbols...)
	jobInfo := &banexg.WsJobInfo{
		ID:      strconv.Itoa(requestId),
		MsgHash: msgHash,
		Name:    "depth",
		Symbols: symbols,
		Method:  e.HandleOrderBookSub,
		Limit:   limit,
		Params:  args,
	}
	err = client.Write(args, jobInfo)
	if err != nil {
		e.DelWsChanRefs(chanKey, symbols...)
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
func (e *Binance) prepareBookArgs(symbols []string, params *map[string]interface{}) (*banexg.WsClient, string, int, map[string]interface{}, *errs.Error) {
	if len(symbols) == 0 {
		return nil, "", 0, nil, errs.NewMsg(errs.CodeParamRequired, "symbols required for UnWatchOrderBooks")
	}
	args, market, err := e.LoadArgsMarket(symbols[0], params)
	if err != nil {
		return nil, "", 0, nil, err
	}
	var msgHash = market.Type + "@depth"
	wsUrl, requestId, err := e.GetWsInfo(market.Type, msgHash)
	if err != nil {
		return nil, "", 0, nil, err
	}
	client, err := e.GetClient(wsUrl, market.Type)
	if err != nil {
		return nil, "", 0, nil, err
	}
	watchRate, ok := e.WsIntvs["WatchOrderBooks"]
	if !ok {
		watchRate = 100
	}
	exgParams, err := e.getExgWsParams(symbols, fmt.Sprintf("depth@%dms", watchRate))
	if err != nil {
		return nil, "", 0, nil, err
	}
	args["params"] = exgParams
	args["id"] = requestId
	return client, msgHash, requestId, args, nil
}

func (e *Binance) UnWatchOrderBooks(symbols []string, params *map[string]interface{}) *errs.Error {
	client, _, _, args, err := e.prepareBookArgs(symbols, params)
	if err != nil {
		return err
	}
	args["method"] = "UNSUBSCRIBE"
	return client.Write(args, nil)
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
	var chanKey = fmt.Sprintf("%s#%s@depth", client.URL, client.MarketType)
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
				if nonce < book.Nonce {
					banexg.WriteOutChan(e.Exchange, chanKey, *book, true)
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
				if nonce < book.Nonce {
					banexg.WriteOutChan(e.Exchange, chanKey, *book, true)
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

func (e *Binance) HandleOrderBookSub(wsUrl string, msg map[string]string, info *banexg.WsJobInfo) {
	err := banexg.CheckWsError(msg)
	urlZap := zap.String("url", wsUrl)
	chanKey := wsUrl + "#" + info.MsgHash
	if err != nil {
		e.DelWsChanRefs(chanKey, info.Symbols...)
		log.Error("sub order error", urlZap, zap.Error(err))
		return
	}
	client, ok := e.WSClients[wsUrl]
	if !ok {
		e.DelWsChanRefs(chanKey, info.Symbols...)
		log.Error("no ws client for", urlZap)
		return
	}
	symbols := info.Symbols
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
		e.DelWsChanRefs(chanKey, failSymbols...)
		log.Error("sub ws od books fail", zap.Strings("symbols", failSymbols))
		err = e.UnWatchOrderBooks(failSymbols, nil)
		if err != nil {
			log.Error("unwatch ws order book fail", zap.Strings("symbols", failSymbols), zap.Error(err))
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
			if e.IsContract(client.MarketType) {
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
