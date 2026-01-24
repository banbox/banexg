package bybit

import (
	"fmt"
	"maps"
	"strconv"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"github.com/banbox/bntp"
	"go.uber.org/zap"
)

func (e *Bybit) WatchOrderBooks(symbols []string, limit int, params map[string]interface{}) (chan *banexg.OrderBook, *errs.Error) {
	if len(symbols) == 0 {
		return nil, errs.NewMsg(errs.CodeParamRequired, "symbols required for WatchOrderBooks")
	}
	args := utils.SafeParams(params)
	category, client, err := e.getWsPublicCategoryClient(args, symbols...)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(symbols))
	// If the same symbol is re-subscribed with a different depth, proactively unsubscribe the old topic.
	// Otherwise, users can't fully cancel the subscription later because UnWatchOrderBooks only accepts symbols.
	unsubKeys := make([]string, 0, len(symbols))
	limits, lock := client.LockOdBookLimits()
	for _, sym := range symbols {
		market, err := e.GetMarket(sym)
		if err != nil {
			lock.Unlock()
			return nil, err
		}
		depth := bybitWsOrderBookDepth(category, limit)
		if oldDepth := limits[sym]; oldDepth != 0 && oldDepth != depth {
			unsubKeys = append(unsubKeys, fmt.Sprintf("orderbook.%d.%s", oldDepth, market.ID))
		}
		keys = append(keys, fmt.Sprintf("orderbook.%d.%s", depth, market.ID))
		limits[sym] = depth
	}
	lock.Unlock()
	if err := e.writeWsTopics(client, 0, false, unsubKeys); err != nil {
		return nil, err
	}
	if err := e.writeWsTopics(client, 0, true, keys); err != nil {
		return nil, err
	}
	chanKey := client.Prefix("orderbook")
	create := func(cap int) chan *banexg.OrderBook { return make(chan *banexg.OrderBook, cap) }
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, args)
	e.AddWsChanRefs(chanKey, symbols...)
	e.DumpWS("WatchOrderBooks", symbols)
	return out, nil
}

func (e *Bybit) UnWatchOrderBooks(symbols []string, params map[string]interface{}) *errs.Error {
	if len(symbols) == 0 {
		return errs.NewMsg(errs.CodeParamRequired, "symbols required for UnWatchOrderBooks")
	}
	args := utils.SafeParams(params)
	category, client, err := e.getWsPublicCategoryClient(args, symbols...)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(symbols))
	limits, lock := client.LockOdBookLimits()
	for _, sym := range symbols {
		market, err := e.GetMarket(sym)
		if err != nil {
			lock.Unlock()
			return err
		}
		depth := limits[sym]
		if depth == 0 {
			depth = bybitWsOrderBookDepth(category, 0)
		}
		delete(limits, sym)
		keys = append(keys, fmt.Sprintf("orderbook.%d.%s", depth, market.ID))
	}
	lock.Unlock()
	if err := e.writeWsTopics(client, 0, false, keys); err != nil {
		return err
	}
	chanKey := client.Prefix("orderbook")
	e.DelWsChanRefs(chanKey, symbols...)
	return nil
}

func (e *Bybit) WatchTrades(symbols []string, params map[string]interface{}) (chan *banexg.Trade, *errs.Error) {
	if len(symbols) == 0 {
		return nil, errs.NewMsg(errs.CodeParamRequired, "symbols required for WatchTrades")
	}
	args := utils.SafeParams(params)
	create := func(cap int) chan *banexg.Trade { return make(chan *banexg.Trade, cap) }
	return watchBybitWsPublicSymbols(e, args, symbols, bybitWsTradeTopics, "trades", "WatchTrades", symbols, create)
}

func (e *Bybit) UnWatchTrades(symbols []string, params map[string]interface{}) *errs.Error {
	if len(symbols) == 0 {
		return errs.NewMsg(errs.CodeParamRequired, "symbols required for UnWatchTrades")
	}
	args := utils.SafeParams(params)
	return e.unwatchWsPublicSymbols(args, symbols, bybitWsTradeTopics, "trades", symbols)
}

func (e *Bybit) WatchOHLCVs(jobs [][2]string, params map[string]interface{}) (chan *banexg.PairTFKline, *errs.Error) {
	if len(jobs) == 0 {
		return nil, errs.NewMsg(errs.CodeParamRequired, "jobs required for WatchOHLCVs")
	}
	args := utils.SafeParams(params)
	create := func(cap int) chan *banexg.PairTFKline { return make(chan *banexg.PairTFKline, cap) }
	return watchBybitWsPublicJobs(e, args, jobs, bybitWsKlineTopics, "kline", "WatchOHLCVs", create)
}

func (e *Bybit) UnWatchOHLCVs(jobs [][2]string, params map[string]interface{}) *errs.Error {
	if len(jobs) == 0 {
		return errs.NewMsg(errs.CodeParamRequired, "jobs required for UnWatchOHLCVs")
	}
	args := utils.SafeParams(params)
	return e.unwatchWsPublicJobs(args, jobs, bybitWsKlineTopics, "kline")
}

func (e *Bybit) WatchMarkPrices(symbols []string, params map[string]interface{}) (chan map[string]float64, *errs.Error) {
	if len(symbols) == 0 {
		return nil, errs.NewMsg(errs.CodeParamRequired, "symbols required for WatchMarkPrices")
	}
	args := utils.SafeParams(params)
	create := func(cap int) chan map[string]float64 { return make(chan map[string]float64, cap) }
	return watchBybitWsPublicSymbols(e, args, symbols, bybitWsMarkPriceTopics, "markPrice", "WatchMarkPrices", symbols, create)
}

func (e *Bybit) UnWatchMarkPrices(symbols []string, params map[string]interface{}) *errs.Error {
	if len(symbols) == 0 {
		return errs.NewMsg(errs.CodeParamRequired, "symbols required for UnWatchMarkPrices")
	}
	args := utils.SafeParams(params)
	return e.unwatchWsPublicSymbols(args, symbols, bybitWsMarkPriceTopics, "markPrice", symbols)
}

func (e *Bybit) WatchMyTrades(params map[string]interface{}) (chan *banexg.MyTrade, *errs.Error) {
	args := utils.SafeParams(params)
	category := ""
	if marketType := utils.GetMapVal(args, banexg.ParamMarket, ""); marketType != "" {
		if cat, err := bybitCategoryFromType(marketType); err == nil {
			category = cat
		}
	}
	topic := "execution"
	if category != "" {
		topic = "execution." + category
	}
	create := func(cap int) chan *banexg.MyTrade { return make(chan *banexg.MyTrade, cap) }
	_, out, err := watchBybitWsPrivateTopic(e, args, topic, "mytrades", "WatchMyTrades", []string{"account"}, nil, create)
	return out, err
}

func (e *Bybit) WatchAccountConfig(params map[string]interface{}) (chan *banexg.AccountConfig, *errs.Error) {
	args := utils.SafeParams(params)
	topic, err := bybitWsPrivatePositionTopic(args, "WatchAccountConfig")
	if err != nil {
		return nil, err
	}
	create := func(cap int) chan *banexg.AccountConfig { return make(chan *banexg.AccountConfig, cap) }
	_, out, err := watchBybitWsPrivateTopic(e, args, topic, "accConfig", "WatchAccountConfig", []string{"account"}, nil, create)
	return out, err
}

func (e *Bybit) WatchBalance(params map[string]interface{}) (chan *banexg.Balances, *errs.Error) {
	args := utils.SafeParams(params)
	create := func(cap int) chan *banexg.Balances { return make(chan *banexg.Balances, cap) }
	client, out, err := watchBybitWsPrivateTopic(e, args, "wallet", "balance", "WatchBalance", []string{"account"}, nil, create)
	if err != nil {
		return nil, err
	}
	if balances, err := e.FetchBalance(args); err == nil && balances != nil {
		if acc, err := e.GetAccount(client.AccName); err == nil {
			acc.LockBalance.Lock()
			acc.MarBalances[client.MarketType] = balances
			acc.LockBalance.Unlock()
		}
		out <- balances
	}
	return out, nil
}

func (e *Bybit) WatchPositions(params map[string]interface{}) (chan []*banexg.Position, *errs.Error) {
	args := utils.SafeParams(params)
	topic, err := bybitWsPrivatePositionTopic(args, "WatchPositions")
	if err != nil {
		return nil, err
	}
	create := func(cap int) chan []*banexg.Position { return make(chan []*banexg.Position, cap) }
	client, out, err := watchBybitWsPrivateTopic(e, args, topic, "positions", "WatchPositions", []string{"account"}, nil, create)
	if err != nil {
		return nil, err
	}
	if positions, err := e.FetchPositions(nil, args); err == nil && len(positions) > 0 {
		if acc, err := e.GetAccount(client.AccName); err == nil {
			acc.LockPos.Lock()
			acc.MarPositions[client.MarketType] = positions
			acc.LockPos.Unlock()
		}
		out <- positions
	}
	return out, nil
}

func (e *Bybit) handleWsOrderBook(client *banexg.WsClient, base *wsBaseMsg) {
	var data orderBookSnapshot
	if err := utils.UnmarshalString(string(base.Data), &data, utils.JsonNumDefault); err != nil {
		log.Error("bybit ws orderbook decode fail", zap.Error(err))
		return
	}
	fillBybitWsOrderBookTs(base, &data)
	_, mid, _ := splitBybitTopic(base.Topic)
	depth, _ := strconv.Atoi(mid)
	if depth <= 0 {
		depth = bybitWsOrderBookDepth(client.MarketType, 0)
	}
	market := e.GetMarketById(data.Symbol, client.MarketType)
	if market == nil {
		market = e.SafeMarket(data.Symbol, "", client.MarketType)
	}
	action := normalizeBybitWsOrderBookAction(base.Type, &data)
	book := applyBybitWsOrderBook(e, market, &data, action, depth)
	if book == nil {
		return
	}
	client.SetSubsKeyStamp(base.Topic, bntp.UTCStamp())
	chanKey := client.Prefix("orderbook")
	banexg.WriteOutChan(e.Exchange, chanKey, book, true)
}

func (e *Bybit) handleWsTrades(client *banexg.WsClient, base *wsBaseMsg) {
	items, ok := decodeBybitWsList(base.Data, "bybit ws trade decode fail")
	if !ok {
		return
	}
	client.SetSubsKeyStamp(base.Topic, bntp.UTCStamp())
	chanKey := client.Prefix("trades")
	for _, item := range items {
		trade := parseBybitWsTradeItem(e, item, client.MarketType)
		if trade == nil {
			continue
		}
		banexg.WriteOutChan(e.Exchange, chanKey, trade, true)
	}
}

func (e *Bybit) handleWsOHLCV(client *banexg.WsClient, base *wsBaseMsg) {
	items, ok := decodeBybitWsList(base.Data, "bybit ws kline decode fail")
	if !ok {
		return
	}
	_, interval, symbolID := splitBybitTopic(base.Topic)
	if symbolID == "" {
		return
	}
	tf := bybitTimeFrameFromInterval(interval)
	symbol := bybitSafeSymbol(e, symbolID, client.MarketType)
	client.SetSubsKeyStamp(base.Topic, bntp.UTCStamp())
	chanKey := client.Prefix("kline")
	for _, item := range items {
		kline := parseBybitWsKlineItem(item)
		if kline == nil {
			continue
		}
		out := &banexg.PairTFKline{
			Symbol:    symbol,
			TimeFrame: tf,
			Kline:     *kline,
		}
		banexg.WriteOutChan(e.Exchange, chanKey, out, true)
	}
}

func (e *Bybit) handleWsTickers(client *banexg.WsClient, base *wsBaseMsg) {
	items, err := decodeWsTickerData(base.Data)
	if err != nil {
		log.Error("bybit ws ticker decode fail", zap.Error(err))
		return
	}
	res := map[string]float64{}
	e.MarkPriceLock.Lock()
	data, ok := e.MarkPrices[client.MarketType]
	if !ok {
		data = map[string]float64{}
		e.MarkPrices[client.MarketType] = data
	}
	for _, item := range items {
		symbolID := bybitWsString(item["symbol"])
		if symbolID == "" {
			_, symbolID, _ = splitBybitTopic(base.Topic)
		}
		if symbolID == "" {
			continue
		}
		markPrice := parseBybitNum(item["markPrice"])
		if markPrice == 0 {
			continue
		}
		symbol := bybitSafeSymbol(e, symbolID, client.MarketType)
		if symbol == "" {
			continue
		}
		res[symbol] = markPrice
	}
	maps.Copy(data, res)
	e.MarkPriceLock.Unlock()
	if len(res) > 0 {
		client.SetSubsKeyStamp(base.Topic, bntp.UTCStamp())
		chanKey := client.Prefix("markPrice")
		banexg.WriteOutChan(e.Exchange, chanKey, res, true)
	}
}

func (e *Bybit) handleWsWallet(client *banexg.WsClient, base *wsBaseMsg) {
	items, ok := decodeBybitWsList(base.Data, "bybit ws wallet decode fail")
	if !ok {
		return
	}
	if len(items) == 0 {
		return
	}
	arr, err2 := decodeBybitList[WalletBalance](items)
	if err2 != nil || len(arr) == 0 {
		if err2 != nil {
			log.Error("bybit ws wallet map decode fail", zap.Error(err2))
		}
		return
	}
	balances := parseBybitBalance(e, &arr[0], items[0], true)
	if balances == nil {
		return
	}
	if acc, err := e.GetAccount(client.AccName); err == nil {
		acc.LockBalance.Lock()
		acc.MarBalances[client.MarketType] = balances
		acc.LockBalance.Unlock()
	}
	client.SetSubsKeyStamp(base.Topic, bntp.UTCStamp())
	chanKey := client.Prefix("balance")
	banexg.WriteOutChan(e.Exchange, chanKey, balances, true)
}

func (e *Bybit) handleWsPositions(client *banexg.WsClient, base *wsBaseMsg) {
	items, ok := decodeBybitWsList(base.Data, "bybit ws position decode fail")
	if !ok {
		return
	}
	arr, err2 := decodeBybitList[wsPositionInfo](items)
	if err2 != nil {
		log.Error("bybit ws position map decode fail", zap.Error(err2))
		return
	}
	var acc *banexg.Account
	if a, err := e.GetAccount(client.AccName); err == nil {
		acc = a
	}
	accConfigs := updateBybitAccLeveragesFromWsPositions(e, acc, arr)
	positions := make([]*banexg.Position, 0, len(arr))
	for i := range arr {
		item := arr[i]
		marketType := bybitMarketTypeFromCategory(item.Category)
		if marketType == "" {
			marketType = banexg.MarketLinear
		}
		pos := parseBybitPosition(e, &item.PositionInfo, items[i], marketType)
		if pos != nil {
			positions = append(positions, pos)
		}
	}
	if len(positions) == 0 && len(accConfigs) == 0 {
		return
	}
	if acc != nil && len(positions) > 0 {
		acc.LockPos.Lock()
		acc.MarPositions[client.MarketType] = positions
		acc.LockPos.Unlock()
	}
	client.SetSubsKeyStamp(base.Topic, bntp.UTCStamp())
	if len(positions) > 0 {
		chanKey := client.Prefix("positions")
		banexg.WriteOutChan(e.Exchange, chanKey, positions, true)
	}
	if len(accConfigs) > 0 {
		accChanKey := client.Prefix("accConfig")
		for _, cfg := range accConfigs {
			banexg.WriteOutChan(e.Exchange, accChanKey, cfg, true)
		}
	}
}

func (e *Bybit) handleWsExecutions(client *banexg.WsClient, base *wsBaseMsg) {
	items, ok := decodeBybitWsList(base.Data, "bybit ws execution decode fail")
	if !ok {
		return
	}
	trades := make([]*banexg.MyTrade, 0, len(items))
	for _, item := range items {
		category := bybitWsString(item["category"])
		marketType := bybitMarketTypeFromCategory(category)
		if marketType == "" {
			marketType = banexg.MarketLinear
		}
		trade := parseBybitWsMyTrade(e, item, marketType)
		if trade != nil {
			trades = append(trades, trade)
		}
	}
	if len(trades) == 0 {
		return
	}
	client.SetSubsKeyStamp(base.Topic, bntp.UTCStamp())
	chanKey := client.Prefix("mytrades")
	for _, trade := range trades {
		banexg.WriteOutChan(e.Exchange, chanKey, trade, true)
	}
}
