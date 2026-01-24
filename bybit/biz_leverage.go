package bybit

import (
	"math"
	"sort"
	"strconv"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

func (e *Bybit) SetLeverage(leverage float64, symbol string, params map[string]interface{}) (map[string]interface{}, *errs.Error) {
	if symbol == "" {
		return nil, errs.NewMsg(errs.CodeParamRequired, "symbol is required for SetLeverage")
	}
	if leverage <= 0 {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "invalid leverage")
	}
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	category, err := bybitCategoryFromMarket(market)
	if err != nil {
		return nil, err
	}
	if category != banexg.MarketLinear && category != banexg.MarketInverse {
		return nil, errs.NewMsg(errs.CodeUnsupportMarket, "SetLeverage supports linear/inverse only")
	}
	args["category"] = category
	args["symbol"] = market.ID
	levStr := strconv.FormatFloat(leverage, 'f', -1, 64)
	if _, ok := args["buyLeverage"]; !ok {
		args["buyLeverage"] = levStr
	}
	if _, ok := args["sellLeverage"]; !ok {
		args["sellLeverage"] = levStr
	}
	tryNum := e.GetRetryNum("SetLeverage", 1)
	res := requestRetry[map[string]interface{}](e, MethodPrivatePostV5PositionSetLeverage, args, tryNum)
	if res.Error != nil {
		// 110077: pm mode cannot set leverage
		// 110038: not allowed to change leverage due to portfolio margin mode
		// 110043: leverage not modified (already set to this value)
		// In PM mode, leverage is managed at portfolio level, treat as success
		if res.Error.BizCode == 110077 || res.Error.BizCode == 110038 || res.Error.BizCode == 110043 {
			return map[string]interface{}{}, nil
		}
		return nil, res.Error
	}
	accName := e.GetAccName(args)
	if accName != "" && market != nil {
		if acc, ok := e.Accounts[accName]; ok && acc != nil && acc.LockLeverage != nil {
			acc.LockLeverage.Lock()
			if acc.Leverages == nil {
				acc.Leverages = map[string]int{}
			}
			acc.Leverages[market.Symbol] = int(math.Round(leverage))
			acc.LockLeverage.Unlock()
		}
	}
	return res.Result, nil
}

func (e *Bybit) LoadLeverageBrackets(reload bool, params map[string]interface{}) *errs.Error {
	e.LeverageBracketsLock.Lock()
	if !reload && len(e.LeverageBrackets) > 0 {
		e.LeverageBracketsLock.Unlock()
		return nil
	}
	e.LeverageBracketsLock.Unlock()

	args := utils.SafeParams(params)
	marketType, _, err := e.LoadArgsMarketType(args)
	if err != nil {
		return err
	}
	category, err := bybitCategoryFromType(marketType)
	if err != nil {
		return err
	}
	if category != banexg.MarketLinear && category != banexg.MarketInverse {
		return errs.NewMsg(errs.CodeUnsupportMarket, "LoadLeverageBrackets supports linear/inverse only")
	}
	args["category"] = category

	items, err := e.fetchRiskLimits(args)
	if err != nil {
		return err
	}
	brackets := buildBybitLeverageBrackets(e, marketType, items)
	e.LeverageBracketsLock.Lock()
	if e.LeverageBrackets == nil {
		e.LeverageBrackets = map[string]*banexg.SymbolLvgBrackets{}
	}
	for k, v := range brackets {
		e.LeverageBrackets[k] = v
	}
	e.LeverageBracketsLock.Unlock()
	return nil
}

func (e *Bybit) CalcMaintMargin(symbol string, cost float64) (float64, *errs.Error) {
	info := findBybitLeverageBracket(e, symbol)
	if info == nil || len(info.Brackets) == 0 {
		return 0, errs.NewMsg(errs.CodeDataNotFound, "leverage bracket not found")
	}
	maintMargin := float64(-1)
	for _, row := range info.Brackets {
		if cost < row.Floor {
			break
		}
		maintMargin = row.MaintMarginRatio*cost - row.Cum
		if row.Capacity > 0 && cost <= row.Capacity {
			break
		}
	}
	if maintMargin < 0 {
		return 0, errs.NewMsg(errs.CodeParamInvalid, "cost invalid")
	}
	return maintMargin, nil
}

func (e *Bybit) GetLeverage(symbol string, notional float64, account string) (float64, float64) {
	// Ensure we have enough context (markets + leverage brackets) to return a non-zero max leverage.
	var market *banexg.Market
	if symbol != "" {
		_, _ = e.LoadMarkets(false, nil)
		if mar, err := e.GetMarket(symbol); err == nil {
			market = mar
		}
	}
	if market != nil {
		category, err := bybitCategoryFromMarket(market)
		if err == nil && (category == banexg.MarketLinear || category == banexg.MarketInverse) {
			e.LeverageBracketsLock.Lock()
			needLoad := len(e.LeverageBrackets) == 0
			e.LeverageBracketsLock.Unlock()
			if needLoad {
				_ = e.LoadLeverageBrackets(false, map[string]interface{}{
					banexg.ParamMarket: category,
				})
			}
		}
	}

	maxVal := 0.0
	if info := findBybitLeverageBracket(e, symbol); info != nil {
		for _, row := range info.Brackets {
			if notional < row.Floor {
				break
			}
			maxVal = float64(row.InitialLeverage)
			if row.Capacity > 0 && notional <= row.Capacity {
				break
			}
		}
	}
	if account == "" {
		account = e.DefAccName
	}
	curLev := 0.0
	if acc, ok := e.Accounts[account]; ok && acc != nil && acc.LockLeverage != nil {
		acc.LockLeverage.Lock()
		if acc.Leverages != nil {
			// Prefer exact key match, but also accept the market.Symbol key used by SetLeverage().
			if lev, ok := acc.Leverages[symbol]; ok {
				curLev = float64(lev)
			} else if market != nil && market.Symbol != "" {
				if lev, ok := acc.Leverages[market.Symbol]; ok {
					curLev = float64(lev)
				}
			}
		}
		acc.LockLeverage.Unlock()
	}

	// If leverage cache is empty (common when leverage is set outside of this SDK), fetch it from position/list.
	// This is best-effort: GetLeverage does not return an error.
	if curLev <= 0 && market != nil {
		category, err := bybitCategoryFromMarket(market)
		if err == nil && (category == banexg.MarketLinear || category == banexg.MarketInverse) {
			if lev, err := e.fetchCurrentLeverageFromPosition(market, account); err == nil && lev > 0 {
				curLev = lev
				if acc, ok := e.Accounts[account]; ok && acc != nil && acc.LockLeverage != nil {
					acc.LockLeverage.Lock()
					if acc.Leverages == nil {
						acc.Leverages = map[string]int{}
					}
					acc.Leverages[market.Symbol] = int(math.Round(lev))
					acc.LockLeverage.Unlock()
				}
			}
		}
	}
	return curLev, maxVal
}

func (e *Bybit) fetchCurrentLeverageFromPosition(market *banexg.Market, account string) (float64, *errs.Error) {
	if e == nil || market == nil || market.ID == "" {
		return 0, errs.NewMsg(errs.CodeParamInvalid, "market is required")
	}
	category, err := bybitCategoryFromMarket(market)
	if err != nil {
		return 0, err
	}
	if category != banexg.MarketLinear && category != banexg.MarketInverse {
		return 0, errs.NewMsg(errs.CodeUnsupportMarket, "GetLeverage supports linear/inverse only")
	}
	args := map[string]interface{}{
		"category": category,
		"symbol":   market.ID,
	}
	if account != "" {
		args[banexg.ParamAccount] = account
	}
	tryNum := e.GetRetryNum("GetLeverage", 1)
	res := requestRetry[V5ListResult](e, MethodPrivateGetV5PositionList, args, tryNum)
	if res.Error != nil {
		return 0, res.Error
	}
	arr, err2 := decodeBybitList[PositionInfo](res.Result.List)
	if err2 != nil {
		return 0, err2
	}
	maxLev := 0.0
	for _, item := range arr {
		lev := parseBybitNum(item.Leverage)
		if lev > maxLev {
			maxLev = lev
		}
	}
	return maxLev, nil
}

func findBybitLeverageBracket(e *Bybit, symbol string) *banexg.SymbolLvgBrackets {
	if e == nil {
		return nil
	}
	e.LeverageBracketsLock.Lock()
	defer e.LeverageBracketsLock.Unlock()
	if len(e.LeverageBrackets) == 0 {
		return nil
	}
	key := symbol
	if market, err := e.GetMarket(symbol); err == nil && market != nil {
		key = market.Symbol
	}
	if info, ok := e.LeverageBrackets[key]; ok {
		return info
	}
	if market, err := e.GetMarket(symbol); err == nil && market != nil && market.ID != "" {
		if info, ok := e.LeverageBrackets[market.ID]; ok {
			return info
		}
	}
	return nil
}

func (e *Bybit) fetchRiskLimits(args map[string]interface{}) ([]RiskLimitInfo, *errs.Error) {
	tryNum := e.GetRetryNum("LoadLeverageBrackets", 1)
	items, err := fetchV5List(e, MethodPublicGetV5MarketRiskLimit, args, tryNum, 0, 0)
	if err != nil {
		return nil, err
	}
	return decodeBybitList[RiskLimitInfo](items)
}

func buildBybitLeverageBrackets(e *Bybit, marketType string, items []RiskLimitInfo) map[string]*banexg.SymbolLvgBrackets {
	if len(items) == 0 {
		return map[string]*banexg.SymbolLvgBrackets{}
	}
	grouped := make(map[string][]RiskLimitInfo)
	for _, item := range items {
		symbol := bybitSafeSymbol(e, item.Symbol, marketType)
		if symbol == "" {
			continue
		}
		grouped[symbol] = append(grouped[symbol], item)
	}
	result := make(map[string]*banexg.SymbolLvgBrackets, len(grouped))
	for symbol, rows := range grouped {
		sort.Slice(rows, func(i, j int) bool {
			return parseBybitNum(rows[i].RiskLimitValue) < parseBybitNum(rows[j].RiskLimitValue)
		})
		brackets := make([]*banexg.LvgBracket, 0, len(rows))
		floor := 0.0
		cum := 0.0
		for i, row := range rows {
			capacity := parseBybitNum(row.RiskLimitValue)
			if capacity <= 0 {
				continue
			}
			mmr := parseBybitPct(row.MaintenanceMargin)
			lev := parseBybitNum(row.MaxLeverage)
			tier := row.ID
			if tier == 0 {
				tier = i + 1
			}
			brackets = append(brackets, &banexg.LvgBracket{
				BaseLvgBracket: banexg.BaseLvgBracket{
					Bracket:          tier,
					InitialLeverage:  int(math.Round(lev)),
					MaintMarginRatio: mmr,
					Cum:              cum,
				},
				Floor:    floor,
				Capacity: capacity,
			})
			if capacity > floor {
				cum += (capacity - floor) * mmr
				floor = capacity
			}
		}
		result[symbol] = &banexg.SymbolLvgBrackets{
			Symbol:   symbol,
			Brackets: brackets,
		}
	}
	return result
}
