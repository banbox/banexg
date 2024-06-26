package binance

import (
	"errors"
	"fmt"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"github.com/bytedance/sonic"
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

func (e *Binance) GetWsClient(marType, msgHash string) (*banexg.WsClient, *errs.Error) {
	host := e.Hosts.GetHost(marType)
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
	getJobFn := func(client *banexg.WsClient) (*banexg.WsJobInfo, *errs.Error) {
		if limit != 0 {
			if e.IsContract(client.MarketType) {
				if !utils.ArrContains(contOdBookLimits, limit) {
					return nil, errs.NewMsg(errs.CodeParamInvalid, "WatchOrderBooks.limit must be 0,5,10,20,50,100,500,1000")
				}
			} else if limit > 5000 {
				return nil, errs.NewMsg(errs.CodeParamInvalid, "WatchOrderBooks.limit must be <= 5000")
			}
		}
		jobInfo := &banexg.WsJobInfo{
			Symbols: symbols,
			Method:  e.HandleOrderBookSub,
			Limit:   limit,
		}
		return jobInfo, nil
	}
	chanKey, args, err := e.prepareBookArgs(true, getJobFn, symbols, params)
	if err != nil {
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
	return out, nil
}

func (e *Binance) UnWatchOrderBooks(symbols []string, params map[string]interface{}) *errs.Error {
	chanKey, _, err := e.prepareBookArgs(false, nil, symbols, params)
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

func (e *Binance) prepareBookArgs(isSub bool, getJobInfo banexg.FuncGetWsJob, symbols []string, params map[string]interface{}) (string, map[string]interface{}, *errs.Error) {
	if len(symbols) == 0 {
		return "", nil, errs.NewMsg(errs.CodeParamRequired, "symbols required for UnWatchOrderBooks")
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
	err = e.WriteWSMsg(client, isSub, symbols, func(m *banexg.Market, _ int) string {
		return fmt.Sprintf("%s@depth@%dms", m.LowercaseID, watchRate)
	}, func(client *banexg.WsClient) (*banexg.WsJobInfo, *errs.Error) {
		var jobInfo *banexg.WsJobInfo
		if getJobInfo != nil {
			jobInfo, err = getJobInfo(client)
			if err != nil {
				return nil, err
			}
			if jobInfo != nil {
				jobInfo.MsgHash = msgHash
				jobInfo.Name = "depth"
			}
		}
		return jobInfo, nil
	})
	if err != nil {
		return "", nil, err
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

	err = e.WriteWSMsg(client, isSub, symbols, func(m *banexg.Market, _ int) string {
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
	var chanKey = client.Prefix("depth")
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
					banexg.WriteOutChan(e.Exchange, chanKey, book, true)
				}
			} else {
				err = errors.New("out of date")
			}
		}
	} else {
		// contract
		// 4. Drop any event where u is < lastUpdateId in the snapshot
		if e.DebugWS {
			log.Debug("depth msg", zap.Int64("nonce", nonce), zap.Int64("u", u), zap.Int64("U", U),
				zap.Int64("pu", pu))
		}
		if u >= nonce {
			// 5. The first processed event should have U <= lastUpdateId AND u >= lastUpdateId
			// 6. While listening to the stream, each new event's pu should be equal to the previous event's u, otherwise initialize the process from step 3
			if U <= nonce || pu == nonce {
				e.handleOrderBookMsg(msg, book)
				if nonce < book.Nonce {
					banexg.WriteOutChan(e.Exchange, chanKey, book, true)
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

func (e *Binance) HandleOrderBookSub(client *banexg.WsClient, msg map[string]string, info *banexg.WsJobInfo) {
	err := banexg.CheckWsError(msg)
	urlZap := zap.String("url", client.URL)
	chanKey := client.Prefix(info.MsgHash)
	if err != nil {
		e.DelWsChanRefs(chanKey, info.Symbols...)
		log.Error("sub order error", urlZap, zap.Error(err))
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
	book, err := e.FetchOrderBook(symbol, info.Limit, info.Params)
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
	banexg.WriteOutChan(e.Exchange, info.MsgHash, book, true)
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
