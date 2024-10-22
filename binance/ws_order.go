package binance

import (
	"errors"
	"fmt"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"go.uber.org/zap"
	"strconv"
	"strings"
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

// GetWsClient get WsClient for public data
func (e *Binance) GetWsClient(marType, msgHash string) (*banexg.WsClient, *errs.Error) {
	host := e.GetHost(marType)
	if host == "" {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "unsupport wss host for %s: %s", e.Name, marType)
	}
	wsUrl := host + "/" + e.Stream(marType, msgHash)
	client, err := e.GetClient(wsUrl, marType, "")
	if err != nil {
		return nil, err
	}
	return client, nil
}

/*
WatchOrderBooks
watches information on open orders with bid(buy) and ask(sell) prices, volumes and other data
When depth limit <= 20, and not spot market, subscribe to limited depth instead of incremental depth
当深度<=20时，且非现货时，订阅有限档深度而非增量深度（币安现货有限档推送缺少event和symbol）

	:param str symbol: unified symbol of the market to fetch the order book for
	:param int [limit]: the maximum amount of order book entries to return
	:param dict [params]: extra parameters specific to the exchange API endpoint
	:returns dict: A dictionary of `order book structures <https://docs.ccxt.com/#/?id=order-book-structure>` indexed by market symbols
*/
func (e *Binance) WatchOrderBooks(symbols []string, limit int, params map[string]interface{}) (chan *banexg.OrderBook, *errs.Error) {
	/*
		# todo add support for <levels>-snapshots(depth)
		# https://github.com/binance-exchange/binance-official-api-docs/blob/master/web-socket-streams.md#partial-book-depth-streams        # <symbol>@depth<levels>@100ms or <symbol>@depth<levels>(1000ms)
		# valid <levels> are 5, 10, or 20
		#
		# default 100, max 1000, valid limits 5, 10, 20, 50, 100, 500, 1000
	*/
	if len(symbols) == 0 {
		return nil, errs.NewMsg(errs.CodeParamRequired, "symbols required for WatchOrderBooks")
	}
	_, market, err := e.LoadArgsMarket(symbols[0], params)
	if err != nil {
		return nil, err
	}
	if limit != 0 {
		if e.IsContract(market.Type) {
			if !utils.ArrContains(contOdBookLimits, limit) {
				return nil, errs.NewMsg(errs.CodeParamInvalid, "WatchOrderBooks.limit must be 0,5,10,20,50,100,500,1000")
			}
		} else if limit > 5000 {
			return nil, errs.NewMsg(errs.CodeParamInvalid, "WatchOrderBooks.limit must be <= 5000")
		}
	} else {
		e.OdBookLock.Lock()
		for _, code := range symbols {
			if book, ok := e.OrderBooks[code]; ok {
				limit = book.Limit
				if limit > 0 {
					break
				}
			}
		}
		e.OdBookLock.Unlock()
		if limit == 0 {
			limit = 100
		}
	}
	chanKey, args, err := e.prepareBookArgs(true, limit, symbols, params)
	if err != nil || chanKey == "" {
		return nil, err
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
	create := func(cap int) chan *banexg.OrderBook { return make(chan *banexg.OrderBook, cap) }
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, args)
	e.AddWsChanRefs(chanKey, symbols...)
	e.DumpWS("WatchOrderBooks", symbols)
	return out, nil
}

func (e *Binance) UnWatchOrderBooks(symbols []string, params map[string]interface{}) *errs.Error {
	chanKey, _, err := e.prepareBookArgs(false, 0, symbols, params)
	if err != nil {
		return err
	}
	e.DelWsChanRefs(chanKey, symbols...)
	return nil
}

func (e *Binance) getExgWsParams(offset int, symbols []string, cvt func(m *banexg.Market, i int) string) ([]string, *errs.Error) {
	exgParams := make([]string, 0, len(symbols))
	for i, sym := range symbols {
		mar, err := e.GetMarket(sym)
		if err != nil {
			if strings.Contains(sym, "@") {
				exgParams = append(exgParams, sym)
				continue
			}
			return nil, err
		}
		subText := cvt(mar, offset+i)
		if subText == "" {
			continue
		}
		exgParams = append(exgParams, subText)
	}
	return exgParams, nil
}

func (e *Binance) prepareBookArgs(isSub bool, limit int, symbols []string, params map[string]interface{}) (string, map[string]interface{}, *errs.Error) {
	if len(symbols) == 0 {
		return "", nil, errs.NewMsg(errs.CodeParamRequired, "symbols required for WatchOrderBooks")
	}
	args, market, err := e.LoadArgsMarket(symbols[0], params)
	if err != nil {
		return "", nil, err
	}
	var msgHash = market.Type + "@depth"
	client, err := e.GetWsClient(market.Type, msgHash)
	if err != nil {
		return "", nil, err
	}
	watchRate, ok := e.WsIntvs["WatchOrderBooks"]
	if !ok {
		watchRate = 100
	}
	var task = "depth"
	// save the depth limit of subscription
	// 记录订阅的深度信息
	client.LimitsLock.Lock()
	defer client.LimitsLock.Unlock()
	if isSub {
		for _, code := range symbols {
			client.OdBookLimits[code] = limit
		}
	} else {
		for _, code := range symbols {
			if val, ok := client.OdBookLimits[code]; ok {
				limit = val
				break
			}
		}
		if limit <= 0 {
			// no sub symbols, return
			return "", nil, nil
		}
	}
	if limit <= 20 && !market.Spot {
		// ignoring binance's spot, as no symbols and events to distinguish on Binance's spot limited depth msg
		// 币安的现货有限档深度推送，缺少symbol和event，无法区分，忽略现货
		// limited depth order book
		// 有限档深度信息: 5/10/20
		task += strconv.Itoa(limit)
	}
	err = e.WriteWSMsg(client, 0, isSub, symbols, func(m *banexg.Market, _ int) string {
		return fmt.Sprintf("%s@%s@%dms", m.LowercaseID, task, watchRate)
	}, nil)
	if err != nil {
		return "", nil, err
	}
	if !isSub {
		for _, code := range symbols {
			delete(client.OdBookLimits, code)
		}
	}
	chanKey := client.Prefix(msgHash)
	return chanKey, args, err
}

func (e *Binance) WatchTrades(symbols []string, params map[string]interface{}) (chan *banexg.Trade, *errs.Error) {
	chanKey, args, err := e.prepareWatchTrades(true, symbols, params)
	if err != nil {
		return nil, err
	}

	create := func(cap int) chan *banexg.Trade { return make(chan *banexg.Trade, cap) }
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, args)
	e.AddWsChanRefs(chanKey, symbols...)
	e.DumpWS("WatchTrades", symbols)
	return out, nil
}

func (e *Binance) UnWatchTrades(symbols []string, params map[string]interface{}) *errs.Error {
	chanKey, _, err := e.prepareWatchTrades(false, symbols, params)

	if err != nil {
		return err
	}
	e.DelWsChanRefs(chanKey, symbols...)
	return nil
}

func (e *Binance) prepareWatchTrades(isSub bool, symbols []string, params map[string]interface{}) (string, map[string]interface{}, *errs.Error) {
	if len(symbols) == 0 {
		return "", nil, errs.NewMsg(errs.CodeParamRequired, "symbols is required")
	}
	args, market, err := e.LoadArgsMarket(symbols[0], params)
	if err != nil {
		return "", nil, err
	}
	name := utils.PopMapVal(args, banexg.ParamName, "trade")
	msgHash := market.Type + "@" + name
	client, err := e.GetWsClient(market.Type, msgHash)
	if err != nil {
		return "", nil, err
	}

	err = e.WriteWSMsg(client, 0, isSub, symbols, func(m *banexg.Market, _ int) string {
		return fmt.Sprintf("%s@%s", m.LowercaseID, name)
	}, nil)
	if err != nil {
		return "", nil, err
	}
	chanKey := client.Prefix(msgHash)
	return chanKey, args, nil
}

/*
WatchMyTrades

	watches information on multiple trades made by the user

:param str symbol: unified market symbol of the market orders were made in
:param int [since]: the earliest time in ms to fetch orders for
:param int [limit]: the maximum number of  orde structures to retrieve
:param dict [params]: extra parameters specific to the exchange API endpoint
:returns dict[]: a list of [trade structures]{@link https://docs.ccxt.com/#/?id=trade-structure
*/
func (e *Binance) WatchMyTrades(params map[string]interface{}) (chan *banexg.MyTrade, *errs.Error) {
	_, client, err := e.getAuthClient(params)
	if err != nil {
		return nil, err
	}
	args := utils.SafeParams(params)
	chanKey := client.Prefix("mytrades")
	create := func(cap int) chan *banexg.MyTrade { return make(chan *banexg.MyTrade, cap) }
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, args)
	e.AddWsChanRefs(chanKey, "account")
	return out, nil
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
	symbol := market.Symbol
	e.OdBookLock.Lock()
	book, ok := e.OrderBooks[symbol]
	if !ok {
		client.LimitsLock.Lock()
		limit, _ := client.OdBookLimits[symbol]
		if limit <= 0 {
			limit = 500
			client.OdBookLimits[symbol] = limit
		}
		client.LimitsLock.Unlock()
		book = &banexg.OrderBook{
			Symbol: symbol,
			Cache:  make([]map[string]string, 0),
			Limit:  limit,
			Asks:   banexg.NewOdBookSide(false, limit, nil),
			Bids:   banexg.NewOdBookSide(true, limit, nil),
		}
		e.OrderBooks[symbol] = book
	}
	e.OdBookLock.Unlock()

	var chanKey = client.Prefix(market.Type + "@depth")
	if !market.Spot && book.Limit <= 20 {
		// ignoring binance's spot, as no symbols and events to distinguish on Binance's spot limited depth msg
		// 币安的现货有限档深度推送，缺少symbol和event，无法区分，忽略现货
		// Limited depth, not incremental updates
		// 有限档深度，非增量更新
		if _, ok = msg["bids"]; ok {
			// spot: lastUpdateId, bids, asks
			e.applyDepthMsgBy(msg, book, true, "asks", "bids", "lastUpdateId", "")
		} else {
			// usd-m, coin-m
			e.applyDepthMsgBy(msg, book, true, "a", "b", "u", "E")
		}
		banexg.WriteOutChan(e.Exchange, chanKey, book, true)
		return
	}
	// Incremental update 增量更新
	refresh := func() {
		err := e.fetchOrderBookSnapshot(client, symbol, chanKey, book.Limit)
		if err != nil {
			e.DelWsChanRefs(chanKey, symbol)
			log.Error("fetch od book from rest fail", zap.String("code", symbol), zap.Error(err))
			err = e.UnWatchOrderBooks([]string{symbol}, nil)
			if err != nil {
				log.Error("unwatch ws order book fail", zap.String("code", symbol), zap.Error(err))
			}
		}
	}
	nonce := book.Nonce // 上一次的u
	if nonce == 0 {
		book.Cache = append(book.Cache, msg)
		if len(book.Cache) == 1 {
			// new order book, refresh from rest
			go refresh()
		}
		return
	}
	var zero = int64(0)
	U, _ := utils.SafeMapVal(msg, "U", zero) // 上次推送至今新增的第一个id，应恰好等于上一次u+1
	u, _ := utils.SafeMapVal(msg, "u", zero) // 上次推送至今新增的最后一个id
	pu, _ := utils.SafeMapVal(msg, "pu", zero)
	var err_ error
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
				e.applyDepthMsg(msg, book)
				if nonce < book.Nonce {
					banexg.WriteOutChan(e.Exchange, chanKey, book, true)
				}
			} else {
				err_ = errors.New("out of date")
			}
		}
	} else {
		// contract
		// 4. Drop any event where u is < lastUpdateId in the snapshot
		if u >= nonce {
			// 5. The first processed event should have U <= lastUpdateId AND u >= lastUpdateId
			// 6. While listening to the stream, each new event's pu should be equal to the previous event's u, otherwise initialize the process from step 3
			if U <= nonce || pu == nonce {
				e.applyDepthMsg(msg, book)
				if nonce < book.Nonce {
					banexg.WriteOutChan(e.Exchange, chanKey, book, true)
				}
			} else {
				err_ = errors.New("out of date")
			}
		}
	}
	if err_ != nil {
		// order book is out of date, refresh from rest-api
		log.Warn("ws order book out-of-date, refresh", urlZap, zap.String("code", symbol),
			zap.Int64("cur", nonce), zap.Int64("latest", u))
		book.Reset()
		book.Cache = append(book.Cache, msg)
		go refresh()
	}
}

func (e *Binance) applyDepthMsg(msg map[string]string, book *banexg.OrderBook) {
	e.applyDepthMsgBy(msg, book, false, "a", "b", "u", "E")
}

func (e *Binance) applyDepthMsgBy(msg map[string]string, book *banexg.OrderBook, replace bool, a, b, u, t string) {
	var zero = int64(0)
	uv, _ := utils.SafeMapVal(msg, u, zero)
	at, ok := msg[a]
	if !ok {
		log.Error("asks not found in ws depth")
		at = "[]"
	}
	book.SetSide(at, false, replace)
	bt, ok := msg[b]
	if !ok {
		log.Error("bids not found in ws depth")
		bt = "[]"
	}
	book.SetSide(bt, true, replace)
	book.Nonce = uv
	timestamp, err := utils.SafeMapVal(msg, t, zero)
	if err == nil {
		if timestamp == 0 {
			timestamp = e.MilliSeconds()
		}
		book.TimeStamp = timestamp
	}
}

func (e *Binance) fetchOrderBookSnapshot(client *banexg.WsClient, symbol, chanKey string, limit int) *errs.Error {
	// 3. Get a depth snapshot from https://www.binance.com/api/v1/depth?symbol=BNBBTC&limit=1000 .
	// default 100, max 1000, valid limits 5, 10, 20, 50, 100, 500, 1000
	if e.WsDecoder != nil {
		// skip request odBook shot in replay mode
		return nil
	}
	book, err := e.FetchOrderBook(symbol, limit, nil)
	if err != nil {
		return err
	}
	e.DumpWS("OdBookShot", &banexg.OdBookShotLog{
		MarketType: client.MarketType,
		Symbol:     book.Symbol,
		ChanKey:    chanKey,
		Book:       book,
	})
	return e.applyOdBookSnapshot(client.MarketType, book.Symbol, chanKey, book)
}

func (e *Binance) applyOdBookSnapshot(marketType, symbol, chanKey string, book *banexg.OrderBook) *errs.Error {
	e.OdBookLock.Lock()
	oldBook, ok := e.OrderBooks[symbol]
	var cache []map[string]string
	if ok && len(oldBook.Cache) > 0 {
		cache = oldBook.Cache
	}
	if oldBook != nil {
		oldBook.Update(book)
		book = oldBook
	} else {
		e.OrderBooks[symbol] = book
	}
	if len(cache) > 0 {
		var zero = int64(0)
		for _, msg := range cache {
			U, _ := utils.SafeMapVal(msg, "U", zero)
			u, _ := utils.SafeMapVal(msg, "u", zero)
			pu, _ := utils.SafeMapVal(msg, "pu", zero)
			nonce := book.Nonce
			if e.IsContract(marketType) {
				//4. Drop any event where u is < lastUpdateId in the snapshot
				if u < nonce {
					continue
				}
				// 5. The first processed event should have U <= lastUpdateId AND u >= lastUpdateId
				if U <= nonce && u >= nonce || pu == nonce {
					e.applyDepthMsg(msg, book)
				}
			} else {
				// 4. Drop any event where u is <= lastUpdateId in the snapshot
				if u <= nonce {
					continue
				}
				// 5. The first processed event should have U <= lastUpdateId+1 AND u >= lastUpdateId+1
				if U-1 <= nonce && u-1 >= nonce {
					e.applyDepthMsg(msg, book)
				}
			}
		}
	}
	e.OdBookLock.Unlock()
	banexg.WriteOutChan(e.Exchange, chanKey, book, true)
	return nil
}

/*
parseMyTrade
将websocket收到的交易转为Trade，注意Symbol和fee.Currency未进行标准化

	public trade
	public agg trade
	private spot trade
	private contract trade
*/
func parseMyTrade(msg map[string]string) banexg.MyTrade {
	var res = banexg.MyTrade{}
	zeroFlt := float64(0)
	// execType, _ := utils.SafeMapVal(msg, "x", "")
	res.ID, _ = utils.SafeMapVal(msg, "t", "")
	res.Price, _ = utils.SafeMapVal(msg, "L", zeroFlt)
	res.Amount, _ = utils.SafeMapVal(msg, "l", zeroFlt)
	res.Cost, _ = utils.SafeMapVal(msg, "Y", zeroFlt)
	res.Order, _ = utils.SafeMapVal(msg, "i", "")
	res.Maker, _ = utils.SafeMapVal(msg, "m", false)
	feeCost, _ := utils.SafeMapVal(msg, "n", zeroFlt)
	feeCurr, _ := utils.SafeMapVal(msg, "N", "")
	res.Fee = &banexg.Fee{
		IsMaker:  res.Maker,
		Currency: feeCurr,
		Cost:     feeCost,
	}
	odType, _ := utils.SafeMapVal(msg, "o", "")
	res.Type = strings.ToLower(odType)
	odState, _ := utils.SafeMapVal(msg, "X", "")
	res.State = mapOrderStatus(odState)
	if res.Cost == 0 && (res.State == banexg.OdStatusPartFilled || res.State == banexg.OdStatusFilled) {
		res.Cost = res.Price * res.Amount
	}
	res.Filled, _ = utils.SafeMapVal(msg, "z", zeroFlt)
	res.ClientID, _ = utils.SafeMapVal(msg, "c", "")
	res.Average, _ = utils.SafeMapVal(msg, "ap", zeroFlt)
	posSide, _ := utils.SafeMapVal(msg, "ps", "")
	res.PosSide = strings.ToLower(posSide)
	res.ReduceOnly, _ = utils.SafeMapVal(msg, "R", false)

	res.Info = msg
	res.Timestamp, _ = utils.SafeMapVal(msg, "T", int64(0))
	res.Symbol, _ = utils.SafeMapVal(msg, "s", "")
	side, _ := utils.SafeMapVal(msg, "S", "")
	res.Side = strings.ToLower(side)
	return res
}

func parsePubTrade(msg map[string]string) banexg.Trade {
	var res = banexg.Trade{}
	zeroFlt := float64(0)
	res.ID, _ = utils.SafeMapVal(msg, "a", "")
	res.Price, _ = utils.SafeMapVal(msg, "p", zeroFlt)
	res.Amount, _ = utils.SafeMapVal(msg, "q", zeroFlt)
	res.Cost = res.Price * res.Amount

	res.Info = msg
	res.Timestamp, _ = utils.SafeMapVal(msg, "T", int64(0))
	res.Symbol, _ = utils.SafeMapVal(msg, "s", "")
	side, _ := utils.SafeMapVal(msg, "S", "")
	res.Side = strings.ToLower(side)
	return res
}
