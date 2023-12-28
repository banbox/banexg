package binance

import (
	"fmt"
	"github.com/anyongjin/banexg"
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

func (e *Binance) GetWsInfo(marType, msgHash string) (string, int, error) {
	host := e.Hosts.GetHost(marType)
	if host == "" {
		return "", 0, fmt.Errorf("unsupport wss host for %s: %s", e.Name, marType)
	}
	wsUrl := host + "/" + e.Stream(marType, msgHash)
	requestId := e.wsRequestId[wsUrl] + 1
	e.wsRequestId[wsUrl] = requestId
	return wsUrl, requestId, nil
}

/*
WatchOrderBook
watches information on open orders with bid(buy) and ask(sell) prices, volumes and other data

	:param str symbol: unified symbol of the market to fetch the order book for
	:param int [limit]: the maximum amount of order book entries to return
	:param dict [params]: extra parameters specific to the exchange API endpoint
	:returns dict: A dictionary of `order book structures <https://docs.ccxt.com/#/?id=order-book-structure>` indexed by market symbols
*/
func (e *Binance) WatchOrderBook(symbol string, limit int, params *map[string]interface{}) (chan banexg.OrderBook, error) {
	/*
		# todo add support for <levels>-snapshots(depth)
		# https://github.com/binance-exchange/binance-official-api-docs/blob/master/web-socket-streams.md#partial-book-depth-streams        # <symbol>@depth<levels>@100ms or <symbol>@depth<levels>(1000ms)
		# valid <levels> are 5, 10, or 20
		#
		# default 100, max 1000, valid limits 5, 10, 20, 50, 100, 500, 1000
	*/
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	if limit != 0 {
		if market.Contract {
			if !utils.ArrContains(contOdBookLimits, limit) {
				return nil, fmt.Errorf("WatchOrderBook.limit must be 0,5,10,20,50,100,500,1000")
			}
		} else if limit > 5000 {
			return nil, fmt.Errorf("WatchOrderBook.limit must be <= 5000")
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
	msgHash := market.LowercaseID + "@" + name
	wsUrl, requestId, err := e.GetWsInfo(market.Type, msgHash)
	if err != nil {
		return nil, err
	}

	watchRate, ok := e.WsIntvs["WatchOrderBook"]
	if !ok {
		watchRate = 100
	}
	args["method"] = "SUBSCRIBE"
	args["id"] = requestId
	args["params"] = []string{fmt.Sprintf("%s@%dms", msgHash, watchRate)}
	msg, err := sonic.Marshal(args)
	if err != nil {
		return nil, err
	}
	outRaw, ok := e.getWsOutChan(wsUrl, msgHash, args)
	out := outRaw.(chan banexg.OrderBook)
	reqIdStr := strconv.Itoa(requestId)
	subInfo := &banexg.WsSubInfo{
		ID:         reqIdStr,
		MsgHash:    msgHash,
		Name:       name,
		Symbol:     market.Symbol,
		Method:     e.HandleOrderBookSub(out),
		Limit:      limit,
		MarketType: market.Type,
		Params:     args,
	}
	err = e.Watch(wsUrl, msg, reqIdStr, subInfo)
	if err != nil {
		close(out)
		e.delWsChan(wsUrl, msgHash)
		return nil, err
	}
	return out, nil
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
	return func(wsUrl string, msg map[string]interface{}) {
		msgText, _ := sonic.MarshalString(msg)
		log.Info("ws msg", zap.String("msg", msgText))
	}
}

func (e *Binance) HandleOrderBookSub(out chan banexg.OrderBook) banexg.FuncOnWsMsg {
	return func(wsUrl string, msg map[string]interface{}) {
		msgText, _ := sonic.MarshalString(msg)
		log.Info("obk", zap.String("msg", msgText))
		//client := e.WSClients[wsUrl]

	}
}
