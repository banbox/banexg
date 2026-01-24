package bybit

import (
	"math"
	"strings"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

func (e *Bybit) FetchBalance(params map[string]interface{}) (*banexg.Balances, *errs.Error) {
	args := utils.SafeParams(params)
	if ccy := utils.PopMapVal(args, banexg.ParamCurrency, ""); ccy != "" {
		args["coin"] = ccy
	}
	if _, ok := args["accountType"]; !ok {
		args["accountType"] = "UNIFIED"
	}
	tryNum := e.GetRetryNum("FetchBalance", 1)
	res := requestRetry[WalletBalanceResult](e, MethodPrivateGetV5AccountWalletBalance, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	if len(res.Result.List) == 0 {
		return nil, errs.NewMsg(errs.CodeDataNotFound, "empty balance result")
	}
	arr, err := decodeBybitList[WalletBalance](res.Result.List)
	if err != nil {
		return nil, err
	}
	if len(arr) == 0 {
		return nil, errs.NewMsg(errs.CodeDataNotFound, "empty balance result")
	}
	keepZero := false
	if _, ok := args["coin"]; ok {
		keepZero = true
	}
	balance := parseBybitBalance(e, &arr[0], res.Result.List[0], keepZero)
	if balance == nil {
		return nil, errs.NewMsg(errs.CodeDataNotFound, "empty balance result")
	}
	return balance, nil
}

func parseBybitBalance(e *Bybit, bal *WalletBalance, info map[string]interface{}, keepZero bool) *banexg.Balances {
	if bal == nil {
		return nil
	}
	res := &banexg.Balances{
		TimeStamp: e.MilliSeconds(),
		Assets:    map[string]*banexg.Asset{},
		Info:      info,
	}
	for _, item := range bal.Coin {
		code := bybitSafeCurrency(e, item.Coin)
		wallet := parseBybitNum(item.WalletBalance)
		spotBorrow := parseBybitNum(item.SpotBorrow)
		equity := parseBybitNum(item.Equity)
		baseBal := wallet - spotBorrow
		total := baseBal
		if item.Equity != "" {
			total = equity
		}
		locked := parseBybitNum(item.Locked)
		orderIM := parseBybitNum(item.TotalOrderIM)
		posIM := parseBybitNum(item.TotalPositionIM)
		used := locked + orderIM + posIM
		free := baseBal - used
		if free < 0 {
			free = 0
		}
		if total == 0 && free+used > 0 {
			total = free + used
		}
		debt := parseBybitNum(item.BorrowAmount)
		upl := parseBybitNum(item.UnrealisedPnl)
		asset := &banexg.Asset{
			Code:  code,
			Free:  free,
			Used:  used,
			Total: total,
			Debt:  debt,
			UPol:  upl,
		}
		if asset.IsEmpty() && !keepZero {
			continue
		}
		res.Assets[code] = asset
	}
	return res.Init()
}

func (e *Bybit) FetchPositions(symbols []string, params map[string]interface{}) ([]*banexg.Position, *errs.Error) {
	return e.fetchPositions(symbols, params)
}

func (e *Bybit) FetchAccountPositions(symbols []string, params map[string]interface{}) ([]*banexg.Position, *errs.Error) {
	return e.fetchPositions(symbols, params)
}

func (e *Bybit) fetchPositions(symbols []string, params map[string]interface{}) ([]*banexg.Position, *errs.Error) {
	args := utils.SafeParams(params)
	marketType, _, err := e.LoadArgsMarketType(args, symbols...)
	if err != nil {
		return nil, err
	}
	if marketType == banexg.MarketSpot || marketType == banexg.MarketMargin || marketType == "" {
		return nil, errs.NewMsg(errs.CodeUnsupportMarket, "FetchPositions supports linear/inverse/option only")
	}
	category, err := bybitCategoryFromType(marketType)
	if err != nil {
		return nil, err
	}
	args["category"] = category
	if len(symbols) > 1 {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "FetchPositions supports only one symbol per request")
	}
	if err := setBybitSymbolArg(e, args, symbols); err != nil {
		return nil, err
	}
	// Extract settleCoins for looping
	settleCoins := utils.PopMapVal(args, banexg.ParamSettleCoins, []string(nil))
	// Bybit: for linear, either symbol or settleCoin is required (symbol has higher priority).
	// banexg: prefer ParamSettleCoins, but also accept raw "settleCoin" for consistency with other bybit methods.
	if category == banexg.MarketLinear {
		if _, ok := args["symbol"]; !ok {
			if _, ok2 := args["settleCoin"]; !ok2 && len(settleCoins) == 0 {
				return nil, errs.NewMsg(errs.CodeParamRequired, "linear positions require symbol or settleCoins/settleCoin")
			}
		}
	}
	// If multiple settleCoins provided, loop and merge results
	if len(settleCoins) > 1 {
		allResults := make([]*banexg.Position, 0)
		for _, coin := range settleCoins {
			reqArgs := utils.SafeParams(args)
			reqArgs["settleCoin"] = coin
			positions, err := e.fetchPositionsOnce(symbols, reqArgs, marketType)
			if err != nil {
				return nil, err
			}
			allResults = append(allResults, positions...)
		}
		return allResults, nil
	}
	// Single or no settleCoin
	if len(settleCoins) == 1 {
		if _, ok := args["settleCoin"]; !ok {
			args["settleCoin"] = settleCoins[0]
		}
	}
	return e.fetchPositionsOnce(symbols, args, marketType)
}

func (e *Bybit) fetchPositionsOnce(symbols []string, args map[string]interface{}, marketType string) ([]*banexg.Position, *errs.Error) {
	tryNum := e.GetRetryNum("FetchPositions", 1)
	limit := utils.GetMapVal(args, banexg.ParamLimit, 0)
	items, err := fetchV5List(e, MethodPrivateGetV5PositionList, args, tryNum, limit, 200)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return []*banexg.Position{}, nil
	}
	arr, err := decodeBybitList[PositionInfo](items)
	if err != nil {
		return nil, err
	}
	symbolSet := banexg.BuildSymbolSet(symbols)
	result := make([]*banexg.Position, 0, len(arr))
	for i, item := range arr {
		pos := parseBybitPosition(e, &item, items[i], marketType)
		if pos == nil {
			continue
		}
		if symbolSet != nil {
			if _, ok := symbolSet[pos.Symbol]; !ok {
				continue
			}
		}
		result = append(result, pos)
	}
	return result, nil
}

func parseBybitPosition(e *Bybit, item *PositionInfo, info map[string]interface{}, marketType string) *banexg.Position {
	if item == nil {
		return nil
	}
	size := parseBybitNum(item.Size)
	if size == 0 && strings.TrimSpace(item.Side) == "" {
		return nil
	}
	side := strings.ToLower(strings.TrimSpace(item.Side))
	if side == "buy" {
		side = banexg.PosSideLong
	} else if side == "sell" {
		side = banexg.PosSideShort
	}
	if side == "" {
		switch item.PositionIdx {
		case 1:
			side = banexg.PosSideLong
		case 2:
			side = banexg.PosSideShort
		}
	}
	leverage := int(math.Round(parseBybitNum(item.Leverage)))
	entry := parseBybitNum(item.AvgPrice)
	mark := parseBybitNum(item.MarkPrice)
	notional := parseBybitNum(item.PositionValue)
	initMargin := parseBybitNum(item.PositionIM)
	maintMargin := parseBybitNum(item.PositionMM)
	upl := parseBybitNum(item.UnrealisedPnl)
	liq := parseBybitNum(item.LiqPrice)
	stamp := parseBybitInt(item.UpdatedTime)
	if stamp == 0 {
		stamp = parseBybitInt(item.CreatedTime)
	}
	market := e.GetMarketById(item.Symbol, marketType)
	symbol := ""
	contractSize := 0.0
	if market != nil {
		symbol = market.Symbol
		contractSize = market.ContractSize
	} else {
		symbol = bybitSafeSymbol(e, item.Symbol, marketType)
	}
	if notional == 0 && entry > 0 && size > 0 {
		if contractSize > 0 {
			notional = math.Abs(size) * entry * contractSize
		} else {
			notional = math.Abs(size) * entry
		}
	}
	marginMode := ""
	if item.TradeMode == 1 {
		marginMode = banexg.MarginIsolated
	} else if item.TradeMode == 0 {
		marginMode = banexg.MarginCross
	}
	pos := &banexg.Position{
		ID:               item.Symbol,
		Symbol:           symbol,
		TimeStamp:        stamp,
		Isolated:         marginMode == banexg.MarginIsolated,
		Hedged:           item.PositionIdx != 0,
		Side:             side,
		Contracts:        math.Abs(size),
		ContractSize:     contractSize,
		EntryPrice:       entry,
		MarkPrice:        mark,
		Notional:         math.Abs(notional),
		Leverage:         leverage,
		Collateral:       initMargin + upl,
		InitialMargin:    initMargin,
		MaintMargin:      maintMargin,
		UnrealizedPnl:    upl,
		LiquidationPrice: liq,
		MarginMode:       marginMode,
		Info:             info,
	}
	if pos.Collateral > 0 && maintMargin > 0 {
		marginRatio, err := utils.PrecFloat64(maintMargin/pos.Collateral, 4, true, 0)
		if err == nil {
			pos.MarginRatio = marginRatio
		} else {
			pos.MarginRatio = maintMargin / pos.Collateral
		}
	}
	if initMargin > 0 {
		percentage, err := utils.PrecFloat64(upl*100/initMargin, 2, true, 0)
		if err == nil {
			pos.Percentage = percentage
		} else {
			pos.Percentage = upl * 100 / initMargin
		}
	}
	if notional > 0 {
		pos.InitialMarginPct = initMargin / notional
		pos.MaintMarginPct = maintMargin / notional
	}
	return pos
}
