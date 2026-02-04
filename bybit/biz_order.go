package bybit

import (
	"strconv"
	"strings"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

func bybitSide(side string) (string, *errs.Error) {
	switch strings.ToLower(strings.TrimSpace(side)) {
	case "buy":
		return "Buy", nil
	case "sell":
		return "Sell", nil
	default:
		return "", errs.NewMsg(errs.CodeParamInvalid, "invalid side: %s", side)
	}
}

func bybitPositionIdx(posSide string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(posSide)) {
	case "", "net", "both":
		return 0, true
	case banexg.PosSideLong:
		return 1, true
	case banexg.PosSideShort:
		return 2, true
	default:
		return 0, false
	}
}

// ensureBybitPositionIdx applies positionSide -> positionIdx mapping (if any) and
// sets Bybit default positionIdx=0 (one-way) when missing.
func ensureBybitPositionIdx(args map[string]interface{}) *errs.Error {
	if args == nil {
		return nil
	}
	// Map positionSide -> positionIdx for hedge mode; default is one-way mode (positionIdx=0).
	if err := applyBybitPositionIdx(args); err != nil {
		return err
	}
	if _, ok := args["positionIdx"]; !ok {
		args["positionIdx"] = 0
	}
	return nil
}

// applyBybitPositionIdx maps banexg.ParamPositionSide into Bybit's positionIdx when positionIdx is not provided.
// Bybit V5 uses positionIdx: 0(one-way), 1(hedge buy/long), 2(hedge sell/short).
func applyBybitPositionIdx(args map[string]interface{}) *errs.Error {
	if args == nil {
		return nil
	}
	// If user already sets positionIdx explicitly, keep it and drop positionSide to avoid ambiguity.
	if _, ok := args["positionIdx"]; ok {
		utils.PopMapVal(args, banexg.ParamPositionSide, "")
		return nil
	}
	raw, ok := args[banexg.ParamPositionSide]
	if !ok {
		return nil
	}
	delete(args, banexg.ParamPositionSide)
	posSide, ok := raw.(string)
	if !ok {
		return errs.NewMsg(errs.CodeParamInvalid, "positionSide must be a string")
	}
	idx, ok := bybitPositionIdx(posSide)
	if !ok {
		return errs.NewMsg(errs.CodeParamInvalid, "invalid positionSide: %s", posSide)
	}
	args["positionIdx"] = idx
	return nil
}

func normalizeBybitTimeInForce(tif string) string {
	if tif == "" {
		return ""
	}
	switch strings.ToUpper(strings.TrimSpace(tif)) {
	case banexg.TimeInForcePO:
		return "PostOnly"
	case "POSTONLY":
		return "PostOnly"
	case banexg.TimeInForceGTC:
		return banexg.TimeInForceGTC
	case banexg.TimeInForceIOC:
		return banexg.TimeInForceIOC
	case banexg.TimeInForceFOK:
		return banexg.TimeInForceFOK
	default:
		return tif
	}
}

func parseBybitTimeInForce(tif string) string {
	switch strings.TrimSpace(tif) {
	case "PostOnly":
		return banexg.TimeInForcePO
	case banexg.TimeInForceGTC, banexg.TimeInForceIOC, banexg.TimeInForceFOK:
		return tif
	default:
		return tif
	}
}

func bybitOrderTypeFrom(odType string, price float64) string {
	odType = strings.ToLower(strings.TrimSpace(odType))
	switch odType {
	case banexg.OdTypeMarket, banexg.OdTypeStopMarket, banexg.OdTypeTakeProfitMarket, banexg.OdTypeTrailingStopMarket:
		return "Market"
	case banexg.OdTypeLimit, banexg.OdTypeLimitMaker, banexg.OdTypeStop, banexg.OdTypeStopLossLimit, banexg.OdTypeTakeProfitLimit:
		return "Limit"
	case banexg.OdTypeStopLoss, banexg.OdTypeTakeProfit:
		if price > 0 {
			return "Limit"
		}
		return "Market"
	default:
		if price > 0 {
			return "Limit"
		}
		return "Market"
	}
}

func isBybitStopOrderType(odType string) bool {
	switch strings.ToLower(strings.TrimSpace(odType)) {
	case banexg.OdTypeStop, banexg.OdTypeStopMarket,
		banexg.OdTypeStopLoss, banexg.OdTypeStopLossLimit,
		banexg.OdTypeTakeProfit, banexg.OdTypeTakeProfitLimit, banexg.OdTypeTakeProfitMarket:
		return true
	default:
		return false
	}
}

func parseBybitOrderType(orderType, stopOrderType string, triggerPrice float64) string {
	baseType := strings.ToLower(strings.TrimSpace(orderType))
	stopType := strings.TrimSpace(stopOrderType)
	switch stopType {
	case "TakeProfit", "PartialTakeProfit":
		if baseType == "market" {
			return banexg.OdTypeTakeProfitMarket
		}
		if baseType == "limit" {
			return banexg.OdTypeTakeProfitLimit
		}
		return banexg.OdTypeTakeProfit
	case "StopLoss", "PartialStopLoss":
		if baseType == "limit" {
			return banexg.OdTypeStopLossLimit
		}
		return banexg.OdTypeStopLoss
	case "TrailingStop":
		return banexg.OdTypeTrailingStopMarket
	case "Stop":
		if baseType == "market" {
			return banexg.OdTypeStopMarket
		}
		return banexg.OdTypeStop
	}
	if triggerPrice > 0 {
		if baseType == "market" {
			return banexg.OdTypeStopMarket
		}
		if baseType == "limit" {
			return banexg.OdTypeStop
		}
	}
	return baseType
}

func parseBybitOrderStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "New", "Untriggered", "Triggered":
		return banexg.OdStatusOpen
	case "PartiallyFilled":
		return banexg.OdStatusPartFilled
	case "Filled":
		return banexg.OdStatusFilled
	case "Cancelled", "PartiallyFilledCanceled", "Deactivated":
		return banexg.OdStatusCanceled
	case "Rejected":
		return banexg.OdStatusRejected
	default:
		return status
	}
}

// bybitExecStateFromQty derives order state from order and remaining quantities.
// Bybit execution stream does not carry order status, so we infer it here.
func bybitExecStateFromQty(orderQty, leavesQty, execQty float64) string {
	if orderQty > 0 {
		filled := orderQty - leavesQty
		if filled <= 0 {
			return banexg.OdStatusOpen
		}
		if leavesQty <= 0 || filled >= orderQty {
			return banexg.OdStatusFilled
		}
		return banexg.OdStatusPartFilled
	}
	if execQty > 0 {
		return banexg.OdStatusPartFilled
	}
	return banexg.OdStatusOpen
}

func parseBybitOrderFee(e *Bybit, info map[string]interface{}) *banexg.Fee {
	if info == nil {
		return nil
	}
	if raw, ok := info["cumFeeDetail"]; ok {
		if data, ok := raw.(map[string]interface{}); ok {
			for currency, val := range data {
				cost := parseBybitNum(val)
				if cost == 0 {
					continue
				}
				currency = bybitSafeCurrency(e, currency)
				return &banexg.Fee{Currency: currency, Cost: cost}
			}
		}
	}
	if raw, ok := info["cumExecFee"]; ok {
		cost := parseBybitNum(raw)
		if cost == 0 {
			return nil
		}
		currency := ""
		if feeCcy, ok := info["feeCurrency"].(string); ok {
			currency = bybitSafeCurrency(e, feeCcy)
		}
		return &banexg.Fee{Currency: currency, Cost: cost}
	}
	return nil
}

func setBybitPriceArg(e *Bybit, market *banexg.Market, args map[string]interface{}, key string, val float64, allowZero bool) *errs.Error {
	if val == 0 {
		if allowZero {
			args[key] = "0"
		}
		return nil
	}
	if val < 0 {
		return nil
	}
	prec, err := e.PrecPrice(market, val)
	if err != nil {
		return err
	}
	args[key] = strconv.FormatFloat(prec, 'f', -1, 64)
	return nil
}

type bybitPriceArg struct {
	key string
	val float64
}

func setBybitPriceArgs(e *Bybit, market *banexg.Market, args map[string]interface{}, allowZero bool, items ...bybitPriceArg) *errs.Error {
	for _, item := range items {
		if err := setBybitPriceArg(e, market, args, item.key, item.val, allowZero); err != nil {
			return err
		}
	}
	return nil
}

func popBybitFloatArg(args map[string]interface{}, key string) (float64, bool) {
	raw, ok := args[key]
	if !ok {
		return 0, false
	}
	val, err := utils.ParseNum(raw)
	if err != nil {
		return 0, false
	}
	delete(args, key)
	return val, true
}

func setBybitFloatArg(args map[string]interface{}, key string, val float64, allowZero bool) {
	if val == 0 {
		if allowZero {
			args[key] = "0"
		}
		return
	}
	if val < 0 {
		return
	}
	args[key] = strconv.FormatFloat(val, 'f', -1, 64)
}

func popAndSetBybitFloatArgs(args map[string]interface{}, allowZero bool, keys ...string) {
	for _, key := range keys {
		if val, ok := popBybitFloatArg(args, key); ok {
			setBybitFloatArg(args, key, val, allowZero)
		}
	}
}

type bybitPriceParam struct {
	param string
	key   string
}

func popAndSetBybitPriceArgs(e *Bybit, market *banexg.Market, args map[string]interface{}, allowZero bool, items ...bybitPriceParam) *errs.Error {
	for _, item := range items {
		if val, ok := popBybitFloatArg(args, item.param); ok {
			if err := setBybitPriceArg(e, market, args, item.key, val, allowZero); err != nil {
				return err
			}
		}
	}
	return nil
}

func applyBybitSmpType(args map[string]interface{}) {
	if val, ok := args[banexg.ParamSelfTradePreventionMode]; ok {
		if _, exists := args["smpType"]; !exists {
			args["smpType"] = val
		}
		delete(args, banexg.ParamSelfTradePreventionMode)
	}
}

func hasAnyBybitArgs(args map[string]interface{}, keys ...string) bool {
	if len(args) == 0 {
		return false
	}
	for _, key := range keys {
		if _, ok := args[key]; ok {
			return true
		}
	}
	return false
}

var bybitTpslKeys = []string{
	"takeProfit",
	"stopLoss",
	"tpLimitPrice",
	"slLimitPrice",
	"tpOrderType",
	"slOrderType",
}

const bybitHistoryWindowMS = int64(7 * 24 * 60 * 60 * 1000)

func validateBybitTimeWindow(args map[string]interface{}) *errs.Error {
	start := parseBybitInt(args["startTime"])
	end := parseBybitInt(args["endTime"])
	if start > 0 && end > 0 && end-start > bybitHistoryWindowMS {
		return errs.NewMsg(errs.CodeParamInvalid, "time range must be within 7 days")
	}
	return nil
}

func normalizeBybitLoopIntv(loopIntv int64, autoClip bool) (int64, *errs.Error) {
	if loopIntv <= 0 {
		return 0, nil
	}
	if loopIntv > bybitHistoryWindowMS {
		if autoClip {
			return bybitHistoryWindowMS, nil
		}
		return 0, errs.NewMsg(errs.CodeParamInvalid, "loopIntv must be within 7 days for bybit (or enable autoClip)")
	}
	return loopIntv, nil
}

func setBybitTimeRangeArgs(args map[string]interface{}, start, end int64) {
	if start > 0 {
		args["startTime"] = start
	} else {
		delete(args, "startTime")
	}
	if end > 0 {
		args["endTime"] = end
	} else {
		delete(args, "endTime")
	}
}

func clearBybitPagingArgs(args map[string]interface{}) {
	// fetchV5List uses and may leave paging cursor/limit in args.
	delete(args, "cursor")
}

func bybitLoopTimeRange(since, until, loopIntv int64, direction string, nowMS int64, cb func(start, end int64) (bool, *errs.Error)) *errs.Error {
	if loopIntv <= 0 {
		return errs.NewMsg(errs.CodeParamInvalid, "loopIntv is required")
	}
	if until <= 0 {
		until = nowMS
	}
	endToStart := (direction == "" && since == 0) || direction == "endToStart"
	if endToStart {
		curEnd := until
		for {
			curStart := max(since, curEnd-loopIntv)
			stop, err := cb(curStart, curEnd)
			if err != nil || stop {
				return err
			}
			if curStart <= since {
				return nil
			}
			curEnd = curStart
			if curEnd <= 0 {
				return nil
			}
		}
	}
	if since <= 0 {
		return errs.NewMsg(errs.CodeParamInvalid, "since is required for startToEnd")
	}
	curStart := since
	for {
		curEnd := min(until, curStart+loopIntv)
		stop, err := cb(curStart, curEnd)
		if err != nil || stop {
			return err
		}
		if curEnd >= until {
			return nil
		}
		curStart = curEnd
	}
}

func validateBybitOrderExtraArgs(market *banexg.Market, orderType string, args map[string]interface{}) *errs.Error {
	if market == nil {
		return nil
	}
	isContract := market.Linear || market.Inverse
	if err := rejectBybitArgs(args, market.Spot, "orderFilter only supports spot market", "orderFilter"); err != nil {
		return err
	}
	if err := rejectBybitArgs(args, isContract, "triggerDirection only supports linear/inverse market", "triggerDirection"); err != nil {
		return err
	}
	if err := rejectBybitArgs(args, isContract, "triggerBy only supports linear/inverse market", "triggerBy"); err != nil {
		return err
	}
	if err := rejectBybitArgs(args, isContract, "tpTriggerBy only supports linear/inverse market", "tpTriggerBy"); err != nil {
		return err
	}
	if err := rejectBybitArgs(args, isContract, "slTriggerBy only supports linear/inverse market", "slTriggerBy"); err != nil {
		return err
	}
	if err := rejectBybitArgs(args, isContract, "tpslMode only supports linear/inverse market", "tpslMode"); err != nil {
		return err
	}
	if err := rejectBybitArgs(args, !market.Option, "tp/sl order params are not supported for option", "tpOrderType", "slOrderType", "tpLimitPrice", "slLimitPrice"); err != nil {
		return err
	}
	if err := rejectBybitArgs(args, isContract, "bboSideType only supports linear/inverse market", "bboSideType"); err != nil {
		return err
	}
	if err := rejectBybitArgs(args, isContract, "bboLevel only supports linear/inverse market", "bboLevel"); err != nil {
		return err
	}
	if err := rejectBybitArgs(args, isContract, "closeOnTrigger only supports linear/inverse market", "closeOnTrigger"); err != nil {
		return err
	}
	if err := rejectBybitArgs(args, market.Option, "mmp only supports option market", "mmp"); err != nil {
		return err
	}
	if err := rejectBybitArgs(args, market.Option, "orderIv only supports option market", "orderIv"); err != nil {
		return err
	}
	if orderType != "" && orderType != "Market" {
		if err := rejectBybitArgs(args, false, "slippageTolerance only supports market order", "slippageTolerance"); err != nil {
			return err
		}
		if err := rejectBybitArgs(args, false, "slippageToleranceType only supports market order", "slippageToleranceType"); err != nil {
			return err
		}
	}
	if err := rejectBybitArgs(args, !market.Option, "slippageTolerance only supports spot/linear/inverse market", "slippageTolerance", "slippageToleranceType"); err != nil {
		return err
	}
	return nil
}

func rejectBybitArgs(args map[string]interface{}, allow bool, msg string, keys ...string) *errs.Error {
	if allow {
		return nil
	}
	for _, key := range keys {
		if _, ok := args[key]; ok {
			return errs.NewMsg(errs.CodeParamInvalid, msg)
		}
	}
	return nil
}

func rejectBybitBefore(args map[string]interface{}) *errs.Error {
	if _, ok := args[banexg.ParamBefore]; ok {
		delete(args, banexg.ParamBefore)
		return errs.NewMsg(errs.CodeNotSupport, "bybit does not support before cursor")
	}
	return nil
}

func parseBybitOrder(e *Bybit, item *OrderInfo, info map[string]interface{}, marketType string) *banexg.Order {
	if item == nil {
		return nil
	}
	symbol := bybitSafeSymbol(e, item.Symbol, marketType)
	price := parseBybitNum(item.Price)
	amount := parseBybitNum(item.Qty)
	filled := parseBybitNum(item.CumExecQty)
	remaining := parseBybitNum(item.LeavesQty)
	if remaining == 0 && amount > 0 {
		remaining = amount - filled
		if remaining < 0 {
			remaining = 0
		}
	}
	avgPrice := parseBybitNum(item.AvgPrice)
	cost := parseBybitNum(item.CumExecValue)
	if cost == 0 && avgPrice > 0 && filled > 0 {
		cost = avgPrice * filled
	}
	if avgPrice == 0 && cost > 0 && filled > 0 {
		avgPrice = cost / filled
	}
	triggerPrice := parseBybitNum(item.TriggerPrice)
	takeProfit := parseBybitNum(item.TakeProfit)
	stopLoss := parseBybitNum(item.StopLoss)
	stopPrice := triggerPrice
	timestamp := parseBybitInt(item.CreatedTime)
	lastUpdate := parseBybitInt(item.UpdatedTime)
	if timestamp == 0 {
		timestamp = lastUpdate
	}
	if lastUpdate == 0 {
		lastUpdate = timestamp
	}
	posSide := ""
	switch item.PositionIdx {
	case 1:
		posSide = banexg.PosSideLong
	case 2:
		posSide = banexg.PosSideShort
	}
	tif := parseBybitTimeInForce(item.TimeInForce)
	orderType := parseBybitOrderType(item.OrderType, item.StopOrderType, triggerPrice)
	postOnly := strings.EqualFold(item.TimeInForce, "PostOnly") || tif == banexg.TimeInForcePO
	fee := parseBybitOrderFee(e, info)
	lastTrade := int64(0)
	if filled > 0 {
		lastTrade = lastUpdate
	}
	return &banexg.Order{
		Info:                info,
		ID:                  item.OrderId,
		ClientOrderID:       item.OrderLinkId,
		Timestamp:           timestamp,
		LastTradeTimestamp:  lastTrade,
		LastUpdateTimestamp: lastUpdate,
		Status:              parseBybitOrderStatus(item.OrderStatus),
		Symbol:              symbol,
		Type:                orderType,
		TimeInForce:         tif,
		PositionSide:        posSide,
		Side:                strings.ToLower(item.Side),
		Price:               price,
		Average:             avgPrice,
		Amount:              amount,
		Filled:              filled,
		Remaining:           remaining,
		TriggerPrice:        triggerPrice,
		StopPrice:           stopPrice,
		TakeProfitPrice:     takeProfit,
		StopLossPrice:       stopLoss,
		Cost:                cost,
		PostOnly:            postOnly,
		ReduceOnly:          item.ReduceOnly,
		Fee:                 fee,
	}
}

func parseBybitOrders(e *Bybit, items []map[string]interface{}, marketType string, symbol string) ([]*banexg.Order, *errs.Error) {
	arr, err := decodeBybitList[OrderInfo](items)
	if err != nil {
		return nil, err
	}
	result := make([]*banexg.Order, 0, len(arr))
	for i, item := range arr {
		order := parseBybitOrder(e, &item, items[i], marketType)
		if order == nil {
			continue
		}
		if symbol != "" && order.Symbol != symbol {
			continue
		}
		result = append(result, order)
	}
	return result, nil
}

func parseBybitMyTrade(e *Bybit, item *ExecutionInfo, info map[string]interface{}, marketType string) *banexg.MyTrade {
	if item == nil {
		return nil
	}
	symbol := bybitSafeSymbol(e, item.Symbol, marketType)
	price := parseBybitNum(item.ExecPrice)
	amount := parseBybitNum(item.ExecQty)
	cost := parseBybitNum(item.ExecValue)
	if cost == 0 && price > 0 && amount > 0 {
		cost = price * amount
	}
	feeCost := parseBybitNum(item.ExecFee)
	feeRate := parseBybitNum(item.FeeRate)
	feeCcy := bybitSafeCurrency(e, item.FeeCurrency)
	var fee *banexg.Fee
	if feeCost != 0 {
		fee = &banexg.Fee{Currency: feeCcy, Cost: feeCost, Rate: feeRate, IsMaker: item.IsMaker}
	}
	orderQty := parseBybitNum(item.OrderQty)
	leavesQty := parseBybitNum(item.LeavesQty)
	filled := 0.0
	if orderQty > 0 {
		filled = orderQty - leavesQty
		if filled < 0 {
			filled = 0
		}
	}
	state := bybitExecStateFromQty(orderQty, leavesQty, amount)
	trade := banexg.Trade{
		ID:        item.ExecId,
		Symbol:    symbol,
		Side:      strings.ToLower(item.Side),
		Type:      parseBybitOrderType(item.OrderType, item.StopOrderType, 0),
		Amount:    amount,
		Price:     price,
		Cost:      cost,
		Order:     item.OrderId,
		Timestamp: parseBybitInt(item.ExecTime),
		Maker:     item.IsMaker,
		Fee:       fee,
		Info:      info,
	}
	return &banexg.MyTrade{
		Trade:    trade,
		Filled:   filled,
		ClientID: item.OrderLinkId,
		Average:  price,
		State:    state,
		Info:     info,
	}
}

func parseBybitMyTrades(e *Bybit, items []map[string]interface{}, marketType string, symbol string) ([]*banexg.MyTrade, *errs.Error) {
	arr, err := decodeBybitList[ExecutionInfo](items)
	if err != nil {
		return nil, err
	}
	result := make([]*banexg.MyTrade, 0, len(arr))
	for i, item := range arr {
		trade := parseBybitMyTrade(e, &item, items[i], marketType)
		if trade == nil {
			continue
		}
		if symbol != "" && trade.Symbol != symbol {
			continue
		}
		result = append(result, trade)
	}
	return result, nil
}

func parseBybitIncome(e *Bybit, item *TransLogInfo, info map[string]interface{}, marketType string) *banexg.Income {
	if item == nil {
		return nil
	}
	symbol := bybitSafeSymbol(e, item.Symbol, marketType)
	income := parseBybitNum(item.Change)
	if income == 0 {
		cashFlow := parseBybitNum(item.CashFlow)
		funding := parseBybitNum(item.Funding)
		fee := parseBybitNum(item.Fee)
		income = cashFlow + funding - fee
	}
	asset := bybitSafeCurrency(e, item.Currency)
	return &banexg.Income{
		Symbol:     symbol,
		IncomeType: item.Type,
		Income:     income,
		Asset:      asset,
		Info:       item.Type,
		Time:       parseBybitInt(item.TransactionTime),
		TranID:     item.ID,
		TradeID:    item.TradeId,
	}
}

func (e *Bybit) loadBybitOrderArgs(symbol string, params map[string]interface{}) (map[string]interface{}, *banexg.Market, string, string, *errs.Error) {
	args := utils.SafeParams(params)
	if symbol != "" {
		var market *banexg.Market
		var err *errs.Error
		args, market, err = e.LoadArgsMarket(symbol, args)
		if err != nil {
			return nil, nil, "", "", err
		}
		category, err := bybitCategoryFromMarket(market)
		if err != nil {
			return nil, nil, "", "", err
		}
		args["category"] = category
		args["symbol"] = market.ID
		return args, market, market.Type, category, nil
	}
	marketType, _, err := e.LoadArgsMarketType(args)
	if err != nil {
		return nil, nil, "", "", err
	}
	category, err := bybitCategoryFromType(marketType)
	if err != nil {
		return nil, nil, "", "", err
	}
	args["category"] = category
	return args, nil, marketType, category, nil
}

func setBybitOrderID(args map[string]interface{}, orderId string) *errs.Error {
	clientOrderId := utils.PopMapVal(args, banexg.ParamClientOrderId, "")
	if orderId != "" {
		args["orderId"] = orderId
		return nil
	}
	if clientOrderId != "" {
		args["orderLinkId"] = clientOrderId
		return nil
	}
	return errs.NewMsg(errs.CodeParamRequired, "order id or clientOrderId required")
}

func applyBybitClientOrderID(args map[string]interface{}) {
	if val, ok := args[banexg.ParamClientOrderId]; ok {
		if _, exists := args["orderLinkId"]; !exists {
			args["orderLinkId"] = val
		}
		delete(args, banexg.ParamClientOrderId)
	}
}

func applyBybitTimeRange(args map[string]interface{}, since int64) {
	if since > 0 {
		args["startTime"] = since
	}
	if until := utils.PopMapVal(args, banexg.ParamUntil, int64(0)); until > 0 {
		args["endTime"] = until
	}
}

func (e *Bybit) createBybitTradingStop(symbol, side string, amount, price float64, market *banexg.Market, args map[string]interface{}) (*banexg.Order, *errs.Error) {
	if market == nil {
		return nil, errs.NewMsg(errs.CodeParamRequired, "symbol is required")
	}
	if !(market.Linear || market.Inverse) {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "trailing stop only supports linear/inverse market")
	}
	if err := ensureBybitPositionIdx(args); err != nil {
		return nil, err
	}
	if tpslMode := utils.GetMapVal(args, "tpslMode", ""); tpslMode == "" {
		args["tpslMode"] = "Full"
	}
	trailingDelta := utils.PopMapVal(args, banexg.ParamTrailingDelta, 0.0)
	activationPrice := utils.PopMapVal(args, banexg.ParamActivationPrice, 0.0)
	callbackRate := utils.PopMapVal(args, banexg.ParamCallbackRate, 0.0)
	if callbackRate != 0 {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "callbackRate not supported for bybit trailing stop")
	}
	if trailingDelta > 0 {
		args["trailingStop"] = trailingDelta
	}
	if activationPrice > 0 {
		args["activePrice"] = activationPrice
	}
	if err := popAndSetBybitPriceArgs(e, market, args, false,
		bybitPriceParam{param: "trailingStop", key: "trailingStop"},
		bybitPriceParam{param: "activePrice", key: "activePrice"},
	); err != nil {
		return nil, err
	}
	if _, ok := args["trailingStop"]; !ok {
		return nil, errs.NewMsg(errs.CodeParamRequired, "trailingDelta required for bybit trailing stop")
	}
	tryNum := e.GetRetryNum("SetTradingStop", 1)
	res := requestRetry[map[string]interface{}](e, MethodPrivatePostV5PositionTradingStop, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	return &banexg.Order{
		ID:        "",
		Symbol:    symbol,
		Type:      banexg.OdTypeTrailingStopMarket,
		Side:      side,
		Amount:    amount,
		Price:     price,
		Status:    banexg.OdStatusOpen,
		Timestamp: e.MilliSeconds(),
		Info:      res.Result,
	}, nil
}

func (e *Bybit) CreateOrder(symbol, odType, side string, amount, price float64, params map[string]interface{}) (*banexg.Order, *errs.Error) {
	args, market, _, _, err := e.loadBybitOrderArgs(symbol, params)
	if err != nil {
		return nil, err
	}
	if market == nil {
		return nil, errs.NewMsg(errs.CodeParamRequired, "symbol is required")
	}
	hasTrailing := hasAnyBybitArgs(args,
		banexg.ParamTrailingDelta,
		banexg.ParamActivationPrice,
		banexg.ParamCallbackRate,
		"trailingStop",
		"activePrice",
	)
	if hasTrailing {
		if odType != banexg.OdTypeTrailingStopMarket {
			return nil, errs.NewMsg(errs.CodeParamInvalid, "trailing stop params only supported for trailing stop orders")
		}
		return e.createBybitTradingStop(symbol, side, amount, price, market, args)
	}
	if odType == banexg.OdTypeTrailingStopMarket {
		return e.createBybitTradingStop(symbol, side, amount, price, market, args)
	}
	closePosition := utils.PopMapVal(args, banexg.ParamClosePosition, false)
	reduceOnly := utils.PopMapVal(args, banexg.ParamReduceOnly, false)
	if closePosition {
		if !(market.Linear || market.Inverse) {
			return nil, errs.NewMsg(errs.CodeParamInvalid, "closePosition only valid for linear/inverse markets")
		}
		if _, ok := args["closeOnTrigger"]; !ok {
			args["closeOnTrigger"] = true
		}
		reduceOnly = true
	}
	if reduceOnly {
		args["reduceOnly"] = true
	}
	forceClose := closePosition
	bySide, err := bybitSide(side)
	if err != nil {
		return nil, err
	}
	args["side"] = bySide
	applyBybitClientOrderID(args)
	applyBybitSmpType(args)
	if market.Option {
		orderLinkId := utils.GetMapVal(args, "orderLinkId", "")
		if strings.TrimSpace(orderLinkId) == "" {
			return nil, errs.NewMsg(errs.CodeParamRequired, "orderLinkId required for option orders")
		}
	}
	if market.Contract {
		if err := ensureBybitPositionIdx(args); err != nil {
			return nil, err
		}
	}
	orderType := bybitOrderTypeFrom(odType, price)
	args["orderType"] = orderType
	if orderType == "Limit" && price <= 0 {
		return nil, errs.NewMsg(errs.CodeParamRequired, "price required for limit order")
	}
	if err := validateBybitOrderExtraArgs(market, orderType, args); err != nil {
		return nil, err
	}
	tif := utils.PopMapVal(args, banexg.ParamTimeInForce, "")
	postOnly := utils.PopMapVal(args, banexg.ParamPostOnly, false)
	if postOnly && orderType == "Market" {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "postOnly not allowed for market order")
	}
	if postOnly || odType == banexg.OdTypeLimitMaker {
		tif = "PostOnly"
	}
	if tif != "" {
		args["timeInForce"] = normalizeBybitTimeInForce(tif)
	}
	triggerPrice := utils.PopMapVal(args, banexg.ParamTriggerPrice, float64(0))
	attachedStopLoss := utils.PopMapVal(args, banexg.ParamStopLossPrice, float64(0))
	attachedTakeProfit := utils.PopMapVal(args, banexg.ParamTakeProfitPrice, float64(0))
	isStopOdType := isBybitStopOrderType(odType)

	// Bybit creates conditional orders via triggerPrice (see Bybit V5 create-order docs).
	// In banexg, stop/tp order types represent conditional orders; for those, do NOT send attached TP/SL fields.
	if isStopOdType {
		if triggerPrice == 0 {
			if attachedStopLoss > 0 {
				triggerPrice = attachedStopLoss
			} else if attachedTakeProfit > 0 {
				triggerPrice = attachedTakeProfit
			}
		}
		attachedStopLoss = 0
		attachedTakeProfit = 0
	}
	// For linear/inverse markets, set triggerDirection when triggerPrice is used
	// 1: triggered when price rises, 2: triggered when price falls
	if triggerPrice > 0 && (market.Linear || market.Inverse) {
		if _, ok := args["triggerDirection"]; !ok {
			od := strings.ToLower(strings.TrimSpace(odType))
			switch od {
			case banexg.OdTypeTakeProfit, banexg.OdTypeTakeProfitLimit, banexg.OdTypeTakeProfitMarket:
				// Take profit: Sell triggers when price rises, Buy triggers when price falls.
				if side == banexg.OdSideBuy {
					args["triggerDirection"] = 2
				} else {
					args["triggerDirection"] = 1
				}
			default:
				// Default stop/breakout behavior: Buy triggers when price rises, Sell triggers when price falls.
				if side == banexg.OdSideBuy {
					args["triggerDirection"] = 1
				} else {
					args["triggerDirection"] = 2
				}
			}
		}
	}
	if err := setBybitPriceArgs(e, market, args, false,
		bybitPriceArg{key: "triggerPrice", val: triggerPrice},
		bybitPriceArg{key: "takeProfit", val: attachedTakeProfit},
		bybitPriceArg{key: "stopLoss", val: attachedStopLoss},
	); err != nil {
		return nil, err
	}
	if err := popAndSetBybitPriceArgs(e, market, args, false,
		bybitPriceParam{param: "tpLimitPrice", key: "tpLimitPrice"},
		bybitPriceParam{param: "slLimitPrice", key: "slLimitPrice"},
	); err != nil {
		return nil, err
	}
	if market.Option {
		popAndSetBybitFloatArgs(args, true, "orderIv")
	}
	popAndSetBybitFloatArgs(args, false, "slippageTolerance")
	if reduceOnly && hasAnyBybitArgs(args, bybitTpslKeys...) {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "reduceOnly cannot be used with takeProfit/stopLoss")
	}
	if market.Spot && (isBybitStopOrderType(odType) || triggerPrice > 0) {
		if _, ok := args["orderFilter"]; !ok {
			args["orderFilter"] = "StopOrder"
		}
	}
	if market.Spot || market.Type == banexg.MarketMargin {
		if market.Type == banexg.MarketMargin || utils.PopMapVal(args, banexg.ParamMarginMode, "") != "" {
			args["isLeverage"] = 1
		}
	}
	if forceClose {
		args["qty"] = "0"
	} else if orderType == "Market" && market.Spot {
		cost := utils.PopMapVal(args, banexg.ParamCost, 0.0)
		if cost <= 0 && amount <= 0 {
			return nil, errs.NewMsg(errs.CodeParamRequired, "amount or cost required for market order")
		}
		if cost > 0 {
			precCost, err := e.PrecCost(market, cost)
			if err != nil {
				return nil, err
			}
			args["qty"] = strconv.FormatFloat(precCost, 'f', -1, 64)
			if _, ok := args["marketUnit"]; !ok {
				args["marketUnit"] = "quoteCoin"
			}
		} else {
			precAmt, err := e.PrecAmount(market, amount)
			if err != nil {
				return nil, err
			}
			args["qty"] = strconv.FormatFloat(precAmt, 'f', -1, 64)
			if _, ok := args["marketUnit"]; !ok {
				args["marketUnit"] = "baseCoin"
			}
		}
	} else {
		if amount <= 0 {
			return nil, errs.NewMsg(errs.CodeParamRequired, "amount is required")
		}
		precAmt, err := e.PrecAmount(market, amount)
		if err != nil {
			return nil, err
		}
		args["qty"] = strconv.FormatFloat(precAmt, 'f', -1, 64)
	}
	if orderType == "Limit" && price > 0 {
		precPrice, err := e.PrecPrice(market, price)
		if err != nil {
			return nil, err
		}
		args["price"] = strconv.FormatFloat(precPrice, 'f', -1, 64)
	}
	tryNum := e.GetRetryNum("CreateOrder", 1)
	res := requestRetry[OrderResult](e, MethodPrivatePostV5OrderCreate, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	return &banexg.Order{
		ID:            res.Result.OrderId,
		ClientOrderID: res.Result.OrderLinkId,
		Symbol:        symbol,
		Type:          odType,
		Side:          side,
		Amount:        amount,
		Price:         price,
		Status:        banexg.OdStatusOpen,
		Timestamp:     e.MilliSeconds(),
	}, nil
}

func (e *Bybit) EditOrder(symbol, orderId, side string, amount, price float64, params map[string]interface{}) (*banexg.Order, *errs.Error) {
	args, market, _, _, err := e.loadBybitOrderArgs(symbol, params)
	if err != nil {
		return nil, err
	}
	if market == nil {
		return nil, errs.NewMsg(errs.CodeParamRequired, "symbol is required")
	}
	if err := setBybitOrderID(args, orderId); err != nil {
		return nil, err
	}
	if amount > 0 {
		precAmt, err := e.PrecAmount(market, amount)
		if err != nil {
			return nil, err
		}
		args["qty"] = strconv.FormatFloat(precAmt, 'f', -1, 64)
	}
	if price > 0 {
		precPrice, err := e.PrecPrice(market, price)
		if err != nil {
			return nil, err
		}
		args["price"] = strconv.FormatFloat(precPrice, 'f', -1, 64)
	}
	if err := popAndSetBybitPriceArgs(e, market, args, true,
		bybitPriceParam{param: banexg.ParamTriggerPrice, key: "triggerPrice"},
		bybitPriceParam{param: banexg.ParamTakeProfitPrice, key: "takeProfit"},
		bybitPriceParam{param: banexg.ParamStopLossPrice, key: "stopLoss"},
		bybitPriceParam{param: "tpLimitPrice", key: "tpLimitPrice"},
		bybitPriceParam{param: "slLimitPrice", key: "slLimitPrice"},
	); err != nil {
		return nil, err
	}
	if err := validateBybitOrderExtraArgs(market, "", args); err != nil {
		return nil, err
	}
	if market.Option {
		popAndSetBybitFloatArgs(args, true, "orderIv")
	}
	tryNum := e.GetRetryNum("EditOrder", 1)
	res := requestRetry[OrderResult](e, MethodPrivatePostV5OrderAmend, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	return &banexg.Order{
		ID:            res.Result.OrderId,
		ClientOrderID: res.Result.OrderLinkId,
		Symbol:        symbol,
		Side:          side,
		Amount:        amount,
		Price:         price,
		Status:        banexg.OdStatusOpen,
		Timestamp:     e.MilliSeconds(),
	}, nil
}

func (e *Bybit) CancelOrder(id string, symbol string, params map[string]interface{}) (*banexg.Order, *errs.Error) {
	args, market, _, _, err := e.loadBybitOrderArgs(symbol, params)
	if err != nil {
		return nil, err
	}
	if market == nil {
		return nil, errs.NewMsg(errs.CodeParamRequired, "symbol is required")
	}
	if err := setBybitOrderID(args, id); err != nil {
		return nil, err
	}
	tryNum := e.GetRetryNum("CancelOrder", 1)
	res := requestRetry[OrderResult](e, MethodPrivatePostV5OrderCancel, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	return &banexg.Order{
		ID:            res.Result.OrderId,
		ClientOrderID: res.Result.OrderLinkId,
		Symbol:        symbol,
		Status:        banexg.OdStatusCanceled,
		Timestamp:     e.MilliSeconds(),
	}, nil
}

func (e *Bybit) FetchOrder(symbol, id string, params map[string]interface{}) (*banexg.Order, *errs.Error) {
	args, market, marketType, _, err := e.loadBybitOrderArgs(symbol, params)
	if err != nil {
		return nil, err
	}
	if market == nil {
		return nil, errs.NewMsg(errs.CodeParamRequired, "symbol is required")
	}
	if err := setBybitOrderID(args, id); err != nil {
		return nil, err
	}
	tryNum := e.GetRetryNum("FetchOrder", 1)
	res := requestRetry[V5ListResult](e, MethodPrivateGetV5OrderRealtime, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	orders, err := parseBybitOrders(e, res.Result.List, marketType, symbol)
	if err != nil {
		return nil, err
	}
	if len(orders) == 0 {
		hisArgs := utils.SafeParams(args)
		delete(hisArgs, "openOnly")
		hisRes := requestRetry[V5ListResult](e, MethodPrivateGetV5OrderHistory, hisArgs, tryNum)
		if hisRes.Error != nil {
			return nil, hisRes.Error
		}
		orders, err = parseBybitOrders(e, hisRes.Result.List, marketType, symbol)
		if err != nil {
			return nil, err
		}
		if len(orders) == 0 {
			return nil, errs.NewMsg(errs.CodeDataNotFound, "empty order result")
		}
	}
	return orders[0], nil
}

func (e *Bybit) FetchOpenOrders(symbol string, since int64, limit int, params map[string]interface{}) ([]*banexg.Order, *errs.Error) {
	args, _, marketType, category, err := e.loadBybitOrderArgs(symbol, params)
	if err != nil {
		return nil, err
	}
	if err := rejectBybitBefore(args); err != nil {
		return nil, err
	}
	applyBybitClientOrderID(args)
	settleCoins := utils.PopMapVal(args, banexg.ParamSettleCoins, []string(nil))
	if len(settleCoins) > 1 {
		result := make([]*banexg.Order, 0)
		seen := make(map[string]struct{})
		for _, coin := range settleCoins {
			reqArgs := utils.SafeParams(args)
			reqArgs["settleCoin"] = coin
			orders, err := e.fetchOpenOrdersOnce(symbol, marketType, limit, reqArgs)
			if err != nil {
				return nil, err
			}
			for _, od := range orders {
				key := od.ID
				if key == "" {
					key = od.ClientOrderID
				}
				if key != "" {
					key = od.Symbol + ":" + key
				} else {
					key = od.Symbol
				}
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				result = append(result, od)
			}
		}
		return result, nil
	}
	if len(settleCoins) == 1 {
		if _, ok := args["settleCoin"]; !ok {
			args["settleCoin"] = settleCoins[0]
		}
	}
	if symbol == "" && category == banexg.MarketLinear {
		if _, ok := args["symbol"]; !ok {
			if _, ok2 := args["settleCoin"]; !ok2 {
				if _, ok3 := args["baseCoin"]; !ok3 {
					return nil, errs.NewMsg(errs.CodeParamRequired, "linear open orders require symbol, baseCoin or settleCoin")
				}
			}
		}
	}
	return e.fetchOpenOrdersOnce(symbol, marketType, limit, args)
}

func (e *Bybit) fetchOpenOrdersOnce(symbol, marketType string, limit int, args map[string]interface{}) ([]*banexg.Order, *errs.Error) {
	tryNum := e.GetRetryNum("FetchOpenOrders", 1)
	items, err := fetchV5List(e, MethodPrivateGetV5OrderRealtime, args, tryNum, limit, 50)
	if err != nil {
		return nil, err
	}
	return parseBybitOrders(e, items, marketType, symbol)
}

func (e *Bybit) FetchOrders(symbol string, since int64, limit int, params map[string]interface{}) ([]*banexg.Order, *errs.Error) {
	args, _, marketType, _, err := e.loadBybitOrderArgs(symbol, params)
	if err != nil {
		return nil, err
	}
	if err := rejectBybitBefore(args); err != nil {
		return nil, err
	}
	applyBybitClientOrderID(args)
	tryNum := e.GetRetryNum("FetchOrders", 1)
	until := utils.PopMapVal(args, banexg.ParamUntil, int64(0))
	loopIntv := utils.PopMapVal(args, banexg.ParamLoopIntv, int64(0))
	direction := utils.PopMapVal(args, banexg.ParamDirection, "")
	autoClip := utils.PopMapVal(args, banexg.ParamAutoClip, false)
	loopIntv, err2 := normalizeBybitLoopIntv(loopIntv, autoClip)
	if err2 != nil {
		return nil, err2
	}

	needLoop := loopIntv > 0
	if !needLoop {
		// Single request first; if it violates Bybit's 7-day constraint and autoClip is enabled, fall back to windowed fetch.
		setBybitTimeRangeArgs(args, since, until)
		if err := validateBybitTimeWindow(args); err != nil {
			if !(autoClip && since > 0 && until > 0) {
				return nil, err
			}
			needLoop = true
			loopIntv = bybitHistoryWindowMS
		}
	}
	if !needLoop {
		items, err := fetchV5List(e, MethodPrivateGetV5OrderHistory, args, tryNum, limit, 50)
		if err != nil {
			return nil, err
		}
		return parseBybitOrders(e, items, marketType, symbol)
	}

	nowMS := e.MilliSeconds()
	if until <= 0 {
		until = nowMS
	}
	if loopIntv > bybitHistoryWindowMS {
		// Should never happen due to normalizeBybitLoopIntv, but keep a hard guard here.
		loopIntv = bybitHistoryWindowMS
	}
	result := make([]*banexg.Order, 0)
	seen := make(map[string]struct{})
	loopErr := bybitLoopTimeRange(since, until, loopIntv, direction, nowMS, func(start, end int64) (bool, *errs.Error) {
		clearBybitPagingArgs(args)
		setBybitTimeRangeArgs(args, start, end)
		if err := validateBybitTimeWindow(args); err != nil {
			return true, err
		}
		remLimit := limit
		if limit > 0 {
			remLimit = limit - len(result)
			if remLimit <= 0 {
				return true, nil
			}
		}
		items, err := fetchV5List(e, MethodPrivateGetV5OrderHistory, args, tryNum, remLimit, 50)
		if err != nil {
			return true, err
		}
		orders, err := parseBybitOrders(e, items, marketType, symbol)
		if err != nil {
			return true, err
		}
		for _, od := range orders {
			if od == nil {
				continue
			}
			key := od.ID
			if key == "" {
				if od.ClientOrderID != "" {
					key = od.Symbol + ":" + od.ClientOrderID
				} else {
					key = od.Symbol + ":" + strconv.FormatInt(od.Timestamp, 10)
				}
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, od)
			if limit > 0 && len(result) >= limit {
				return true, nil
			}
		}
		return false, nil
	})
	if loopErr != nil {
		return result, loopErr
	}
	return result, nil
}

func (e *Bybit) FetchMyTrades(symbol string, since int64, limit int, params map[string]interface{}) ([]*banexg.MyTrade, *errs.Error) {
	args, _, marketType, _, err := e.loadBybitOrderArgs(symbol, params)
	if err != nil {
		return nil, err
	}
	if err := rejectBybitBefore(args); err != nil {
		return nil, err
	}
	applyBybitClientOrderID(args)
	tryNum := e.GetRetryNum("FetchMyTrades", 1)
	until := utils.PopMapVal(args, banexg.ParamUntil, int64(0))
	loopIntv := utils.PopMapVal(args, banexg.ParamLoopIntv, int64(0))
	direction := utils.PopMapVal(args, banexg.ParamDirection, "")
	autoClip := utils.PopMapVal(args, banexg.ParamAutoClip, false)
	loopIntv, err2 := normalizeBybitLoopIntv(loopIntv, autoClip)
	if err2 != nil {
		return nil, err2
	}

	needLoop := loopIntv > 0
	if !needLoop {
		setBybitTimeRangeArgs(args, since, until)
		if err := validateBybitTimeWindow(args); err != nil {
			if !(autoClip && since > 0 && until > 0) {
				return nil, err
			}
			needLoop = true
			loopIntv = bybitHistoryWindowMS
		}
	}
	if !needLoop {
		items, err := fetchV5List(e, MethodPrivateGetV5ExecutionList, args, tryNum, limit, 100)
		if err != nil {
			return nil, err
		}
		return parseBybitMyTrades(e, items, marketType, symbol)
	}

	nowMS := e.MilliSeconds()
	if until <= 0 {
		until = nowMS
	}
	if loopIntv > bybitHistoryWindowMS {
		loopIntv = bybitHistoryWindowMS
	}
	result := make([]*banexg.MyTrade, 0)
	seen := make(map[string]struct{})
	loopErr := bybitLoopTimeRange(since, until, loopIntv, direction, nowMS, func(start, end int64) (bool, *errs.Error) {
		clearBybitPagingArgs(args)
		setBybitTimeRangeArgs(args, start, end)
		if err := validateBybitTimeWindow(args); err != nil {
			return true, err
		}
		remLimit := limit
		if limit > 0 {
			remLimit = limit - len(result)
			if remLimit <= 0 {
				return true, nil
			}
		}
		items, err := fetchV5List(e, MethodPrivateGetV5ExecutionList, args, tryNum, remLimit, 100)
		if err != nil {
			return true, err
		}
		trades, err := parseBybitMyTrades(e, items, marketType, symbol)
		if err != nil {
			return true, err
		}
		for _, t := range trades {
			if t == nil {
				continue
			}
			key := t.ID
			if key == "" {
				key = t.Symbol + ":" + strconv.FormatInt(t.Timestamp, 10)
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, t)
			if limit > 0 && len(result) >= limit {
				return true, nil
			}
		}
		return false, nil
	})
	if loopErr != nil {
		return result, loopErr
	}
	return result, nil
}

func (e *Bybit) FetchIncomeHistory(inType string, symbol string, since int64, limit int, params map[string]interface{}) ([]*banexg.Income, *errs.Error) {
	args, market, marketType, _, err := e.loadBybitOrderArgs(symbol, params)
	if err != nil {
		return nil, err
	}
	if err := rejectBybitBefore(args); err != nil {
		return nil, err
	}
	// transaction-log does not accept symbol; use baseCoin for filtering when possible
	delete(args, "symbol")
	if market != nil {
		if _, ok := args["baseCoin"]; !ok && market.Base != "" {
			args["baseCoin"] = market.Base
		}
	}
	if inType != "" {
		args["type"] = inType
	}
	if ccy := utils.PopMapVal(args, banexg.ParamCurrency, ""); ccy != "" {
		args["currency"] = ccy
	}
	if _, ok := args["accountType"]; !ok {
		args["accountType"] = "UNIFIED"
	}
	applyBybitTimeRange(args, since)
	tryNum := e.GetRetryNum("FetchIncomeHistory", 1)
	items, err := fetchV5List(e, MethodPrivateGetV5AccountTransactionLog, args, tryNum, limit, 50)
	if err != nil {
		return nil, err
	}
	arr, err := decodeBybitList[TransLogInfo](items)
	if err != nil {
		return nil, err
	}
	result := make([]*banexg.Income, 0, len(arr))
	for i, item := range arr {
		income := parseBybitIncome(e, &item, items[i], marketType)
		if income == nil {
			continue
		}
		if symbol != "" && income.Symbol != symbol && income.Symbol != "" {
			continue
		}
		result = append(result, income)
	}
	return result, nil
}
