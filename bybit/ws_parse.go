package bybit

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"go.uber.org/zap"
)

type wsBaseMsg struct {
	Op           string          `json:"op"`
	Topic        string          `json:"topic"`
	Type         string          `json:"type"`
	Success      *bool           `json:"success"`
	RetCode      int             `json:"retCode"`
	RetCodeAlt   int             `json:"ret_code"`
	RetMsg       string          `json:"retMsg"`
	RetMsgAlt    string          `json:"ret_msg"`
	ReqID        string          `json:"req_id"`
	ConnID       string          `json:"conn_id"`
	Ts           int64           `json:"ts"`
	CreationTime int64           `json:"creationTime"`
	Data         json.RawMessage `json:"data"`
}

type wsPositionInfo struct {
	PositionInfo
	Category string `json:"category"`
}

func bybitWsOpSuccess(base *wsBaseMsg) (bool, *errs.Error) {
	if base == nil {
		return false, errs.NewMsg(errs.CodeParamInvalid, "ws msg required")
	}
	if base.Success != nil {
		if *base.Success {
			return true, nil
		}
		msg := base.RetMsg
		if msg == "" {
			msg = base.RetMsgAlt
		}
		if msg == "" {
			msg = "ws op failed"
		}
		return false, errs.NewMsg(errs.CodeRunTime, msg)
	}
	retCode := base.RetCode
	if retCode == 0 && base.RetCodeAlt != 0 {
		retCode = base.RetCodeAlt
	}
	if retCode == 0 || retCode == 20001 {
		return true, nil
	}
	msg := base.RetMsg
	if msg == "" {
		msg = base.RetMsgAlt
	}
	return false, mapBybitRetCode(retCode, msg)
}

func decodeWsList(data json.RawMessage) ([]map[string]interface{}, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var items []map[string]interface{}
	if err := utils.UnmarshalString(string(data), &items, utils.JsonNumDefault); err != nil {
		return nil, err
	}
	return items, nil
}

func decodeWsTickerData(data json.RawMessage) ([]map[string]interface{}, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if data[0] == '{' {
		var item map[string]interface{}
		if err := utils.UnmarshalString(string(data), &item, utils.JsonNumDefault); err != nil {
			return nil, err
		}
		return []map[string]interface{}{item}, nil
	}
	return decodeWsList(data)
}

func decodeBybitWsList(data json.RawMessage, failMsg string) ([]map[string]interface{}, bool) {
	items, err := decodeWsList(data)
	if err != nil {
		log.Error(failMsg, zap.Error(err))
		return nil, false
	}
	return items, true
}

func bybitWsString(val interface{}) string {
	switch v := val.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
}

func bybitWsBool(val interface{}) bool {
	switch v := val.(type) {
	case bool:
		return v
	case string:
		return strings.ToLower(v) == "true" || v == "1"
	default:
		return false
	}
}

func splitBybitTopic(topic string) (prefix, mid, symbol string) {
	parts := strings.SplitN(topic, ".", 3)
	if len(parts) > 0 {
		prefix = parts[0]
	}
	if len(parts) > 1 {
		mid = parts[1]
	}
	if len(parts) > 2 {
		symbol = parts[2]
	}
	return
}

func bybitTimeFrameFromInterval(interval string) string {
	if interval == "" {
		return interval
	}
	for tf, itv := range timeFrameMap {
		if itv == interval {
			return tf
		}
	}
	return interval
}

func bybitMarketTypeFromCategory(category string) string {
	switch strings.ToLower(category) {
	case "spot":
		return banexg.MarketSpot
	case "linear":
		return banexg.MarketLinear
	case "inverse":
		return banexg.MarketInverse
	case "option":
		return banexg.MarketOption
	default:
		return ""
	}
}

func bybitWsOrderBookDepth(category string, limit int) int {
	if limit <= 0 {
		if category == banexg.MarketOption {
			return 25
		}
		return 50
	}
	if category == banexg.MarketOption {
		if limit <= 25 {
			return 25
		}
		return 100
	}
	switch {
	case limit <= 1:
		return 1
	case limit <= 50:
		return 50
	case limit <= 200:
		return 200
	default:
		return 1000
	}
}

func bybitWsBatchSize(marketType string) int {
	switch marketType {
	case banexg.MarketSpot:
		return 10
	case banexg.MarketOption:
		return 2000
	default:
		return 100
	}
}

func bybitWsTradeTopics(e *Bybit, symbols []string) ([]string, *errs.Error) {
	keys := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		market, err := e.GetMarket(sym)
		if err != nil {
			return nil, err
		}
		topicSymbol := bybitWsTradeSymbol(market)
		if topicSymbol == "" {
			return nil, errs.NewMsg(errs.CodeParamInvalid, "invalid market for ws trades: %s", sym)
		}
		keys = append(keys, "publicTrade."+topicSymbol)
	}
	return keys, nil
}

func bybitWsTradeSymbol(market *banexg.Market) string {
	if market == nil {
		return ""
	}
	if !market.Option {
		return market.ID
	}
	if market.Base != "" {
		return market.Base
	}
	if parts := strings.Split(market.ID, "-"); len(parts) > 0 {
		return parts[0]
	}
	return market.ID
}

func bybitWsKlineTopics(e *Bybit, jobs [][2]string) ([]string, []string, *errs.Error) {
	if e == nil {
		return nil, nil, errs.NewMsg(errs.CodeParamInvalid, "exchange required for kline topics")
	}
	keys := make([]string, 0, len(jobs))
	refKeys := make([]string, 0, len(jobs))
	for _, job := range jobs {
		symbol := job[0]
		timeframe := job[1]
		if symbol == "" || timeframe == "" {
			return nil, nil, errs.NewMsg(errs.CodeParamInvalid, "invalid job for WatchOHLCVs")
		}
		market, err := e.GetMarket(symbol)
		if err != nil {
			return nil, nil, err
		}
		tf := e.GetTimeFrame(timeframe)
		if tf == "" {
			return nil, nil, errs.NewMsg(errs.CodeInvalidTimeFrame, "invalid timeframe: %s", timeframe)
		}
		keys = append(keys, fmt.Sprintf("kline.%s.%s", tf, market.ID))
		// Use the resolved Bybit interval as the ref-key, so timeframe aliases like 1d/D unwatch correctly.
		refKeys = append(refKeys, symbol+"@"+tf)
	}
	return keys, refKeys, nil
}

func bybitWsMarkPriceTopics(e *Bybit, symbols []string) ([]string, *errs.Error) {
	if e == nil {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "exchange required for mark price topics")
	}
	keys := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		market, err := e.GetMarket(sym)
		if err != nil {
			return nil, err
		}
		if market.Spot {
			return nil, errs.NewMsg(errs.CodeNotSupport, "spot market does not support mark price")
		}
		keys = append(keys, "tickers."+market.ID)
	}
	return keys, nil
}

func fillBybitWsOrderBookTs(base *wsBaseMsg, data *orderBookSnapshot) {
	if base == nil || data == nil {
		return
	}
	if data.Ts == 0 && base.Ts != 0 {
		data.Ts = base.Ts
	}
}

func applyBybitWsOrderBook(e *Bybit, market *banexg.Market, data *orderBookSnapshot, action string, depth int) *banexg.OrderBook {
	if data == nil || market == nil {
		return nil
	}
	asks := bybitParseBookSide(data.Asks)
	bids := bybitParseBookSide(data.Bids)
	symbol := market.Symbol
	e.OdBookLock.Lock()
	book, ok := e.OrderBooks[symbol]
	if !ok || action == "snapshot" {
		book = &banexg.OrderBook{
			Symbol:    symbol,
			TimeStamp: data.Ts,
			Nonce:     data.Update,
			Asks:      banexg.NewOdBookSide(false, depth, asks),
			Bids:      banexg.NewOdBookSide(true, depth, bids),
			Limit:     depth,
			Cache:     make([]map[string]string, 0),
		}
		e.OrderBooks[symbol] = book
		e.OdBookLock.Unlock()
		return book
	}
	e.OdBookLock.Unlock()
	if len(asks) > 0 {
		book.Asks.Update(asks)
	}
	if len(bids) > 0 {
		book.Bids.Update(bids)
	}
	book.TimeStamp = data.Ts
	book.Nonce = data.Update
	return book
}

func normalizeBybitWsOrderBookAction(action string, data *orderBookSnapshot) string {
	if data != nil && data.Update == 1 && action != "snapshot" {
		return "snapshot"
	}
	return action
}

func parseBybitWsTradeItem(e *Bybit, item map[string]interface{}, marketType string) *banexg.Trade {
	if item == nil {
		return nil
	}
	marketID := bybitWsString(item["s"])
	if marketID == "" {
		return nil
	}
	symbol := bybitSafeSymbol(e, marketID, marketType)
	price := parseBybitNum(item["p"])
	amount := parseBybitNum(item["v"])
	side := strings.ToLower(bybitWsString(item["S"]))
	if side == "buy" {
		side = banexg.OdSideBuy
	} else if side == "sell" {
		side = banexg.OdSideSell
	}
	trade := &banexg.Trade{
		ID:        bybitWsString(item["i"]),
		Symbol:    symbol,
		Side:      side,
		Amount:    amount,
		Price:     price,
		Cost:      price * amount,
		Timestamp: parseBybitInt(item["T"]),
		Info:      item,
	}
	return trade
}

func parseBybitWsKlineItem(item map[string]interface{}) *banexg.Kline {
	if item == nil {
		return nil
	}
	return &banexg.Kline{
		Time:   parseBybitInt(item["start"]),
		Open:   parseBybitNum(item["open"]),
		High:   parseBybitNum(item["high"]),
		Low:    parseBybitNum(item["low"]),
		Close:  parseBybitNum(item["close"]),
		Volume: parseBybitNum(item["volume"]),
		Quote:  parseBybitNum(item["turnover"]),
	}
}

func parseBybitWsMyTrade(e *Bybit, item map[string]interface{}, marketType string) *banexg.MyTrade {
	if item == nil {
		return nil
	}
	marketID := bybitWsString(item["symbol"])
	if marketID == "" {
		return nil
	}
	symbol := bybitSafeSymbol(e, marketID, marketType)
	execPrice := parseBybitNum(item["execPrice"])
	execQty := parseBybitNum(item["execQty"])
	execValue := parseBybitNum(item["execValue"])
	if execValue == 0 {
		execValue = execPrice * execQty
	}
	orderPrice := parseBybitNum(item["orderPrice"])
	if orderPrice == 0 {
		orderPrice = execPrice
	}
	orderQty := parseBybitNum(item["orderQty"])
	leavesQty := parseBybitNum(item["leavesQty"])
	orderType := parseBybitOrderType(bybitWsString(item["orderType"]), bybitWsString(item["stopOrderType"]), 0)
	side := strings.ToLower(bybitWsString(item["side"]))
	if side == "buy" {
		side = banexg.OdSideBuy
	} else if side == "sell" {
		side = banexg.OdSideSell
	}
	feeCost := parseBybitNum(item["execFee"])
	feeCurrency := bybitWsString(item["feeCurrency"])
	fee := (*banexg.Fee)(nil)
	if feeCost != 0 {
		feeCurrency = bybitSafeCurrency(e, feeCurrency)
		fee = &banexg.Fee{Currency: feeCurrency, Cost: feeCost}
	}
	filled := execQty
	if orderQty > 0 {
		filled = orderQty - leavesQty
		if filled < 0 {
			filled = 0
		}
	}
	state := bybitExecStateFromQty(orderQty, leavesQty, execQty)
	trade := &banexg.MyTrade{
		Trade: banexg.Trade{
			ID:        bybitWsString(item["execId"]),
			Symbol:    symbol,
			Side:      side,
			Type:      orderType,
			Amount:    execQty,
			Price:     execPrice,
			Cost:      execValue,
			Order:     bybitWsString(item["orderId"]),
			Timestamp: parseBybitInt(item["execTime"]),
			Maker:     bybitWsBool(item["isMaker"]),
			Fee:       fee,
			Info:      item,
		},
		Filled:   filled,
		ClientID: bybitWsString(item["orderLinkId"]),
		Average:  execPrice,
		State:    state,
		Info:     item,
	}
	return trade
}

func updateBybitAccLeveragesFromWsPositions(e *Bybit, acc *banexg.Account, items []wsPositionInfo) []*banexg.AccountConfig {
	if e == nil || acc == nil || len(items) == 0 {
		return nil
	}
	if acc.LockLeverage != nil {
		acc.LockLeverage.Lock()
		defer acc.LockLeverage.Unlock()
	}
	if acc.Leverages == nil {
		acc.Leverages = map[string]int{}
	}
	updates := make([]*banexg.AccountConfig, 0)
	for i := range items {
		item := items[i]
		if item.Symbol == "" {
			continue
		}
		marketType := bybitMarketTypeFromCategory(item.Category)
		if marketType == "" {
			marketType = banexg.MarketLinear
		}
		symbol := bybitSafeSymbol(e, item.Symbol, marketType)
		if symbol == "" {
			continue
		}
		lev := int(math.Round(parseBybitNum(item.Leverage)))
		if lev <= 0 {
			continue
		}
		cur, ok := acc.Leverages[symbol]
		if ok && cur == lev {
			continue
		}
		acc.Leverages[symbol] = lev
		updates = append(updates, &banexg.AccountConfig{Symbol: symbol, Leverage: lev})
	}
	return updates
}
