package okx

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

func (e *OKX) SetLeverage(leverage float64, symbol string, params map[string]interface{}) (map[string]interface{}, *errs.Error) {
	if leverage <= 0 {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "invalid leverage")
	}
	args := utils.SafeParams(params)
	accName := e.GetAccName(args)
	var market *banexg.Market
	var err *errs.Error
	if symbol != "" {
		args, market, err = e.LoadArgsMarket(symbol, args)
		if err != nil {
			return nil, err
		}
		args[FldInstId] = market.ID
		posSide := utils.PopMapVal(args, banexg.ParamPositionSide, "")
		if posSide != "" {
			args[FldPosSide] = strings.ToLower(posSide)
		}
	} else {
		ccy := utils.PopMapVal(args, banexg.ParamCurrency, "")
		if ccy == "" {
			return nil, errs.NewMsg(errs.CodeParamRequired, "symbol or ccy required")
		}
		args[FldCcy] = ccy
	}
	mgnMode := utils.PopMapVal(args, banexg.ParamMarginMode, banexg.MarginCross)
	args[FldMgnMode] = mgnMode
	args[FldLever] = strconv.FormatFloat(leverage, 'f', -1, 64)

	tryNum := e.GetRetryNum("SetLeverage", 1)
	res := requestRetry[[]map[string]interface{}](e, MethodAccountSetLeverage, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	if len(res.Result) == 0 {
		return nil, errs.NewMsg(errs.CodeDataNotFound, "empty set leverage result")
	}
	if market != nil && accName != "" {
		if acc, ok := e.Accounts[accName]; ok && acc != nil && acc.LockLeverage != nil {
			acc.LockLeverage.Lock()
			acc.Leverages[market.Symbol] = int(math.Round(leverage))
			acc.LockLeverage.Unlock()
		}
	}
	return res.Result[0], nil
}

func (e *OKX) LoadLeverageBrackets(reload bool, params map[string]interface{}) *errs.Error {
	return nil
}

func (e *OKX) GetLeverage(symbol string, notional float64, account string) (float64, float64) {
	info := e.findOrLoadLvgBracket(symbol)
	maxVal := 0.0
	if info != nil && len(info.Brackets) > 0 {
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
		if lev, ok := acc.Leverages[symbol]; ok {
			curLev = float64(lev)
		}
		acc.LockLeverage.Unlock()
	}
	return curLev, maxVal
}

// findOrLoadLvgBracket finds the leverage bracket for a symbol in cache,
// or fetches it from the API and caches it if not found.
func (e *OKX) findOrLoadLvgBracket(symbol string) *banexg.SymbolLvgBrackets {
	// First try to find in cache
	info := findLeverageBracket(e, symbol)
	if info != nil {
		return info
	}
	// Not found in cache, fetch from API for this symbol
	market, err := e.GetMarket(symbol)
	if err != nil || market == nil {
		return nil
	}
	instType, err := marketToInstTypeForLeverage(market)
	if err != nil {
		return nil
	}
	args := map[string]interface{}{
		FldInstType: instType,
		FldTdMode:   banexg.MarginCross,
	}
	// For MARGIN, use instId; for derivatives, use instFamily
	if instType == InstTypeMargin {
		args[FldInstId] = market.ID
	} else if family := instFamilyFromID(market.ID); family != "" {
		args[FldInstFamily] = family
	} else {
		args[FldInstId] = market.ID
	}
	tiers, err := e.fetchPositionTiers(args)
	if err != nil || len(tiers) == 0 {
		return nil
	}
	brackets := buildLeverageBrackets(tiers)
	// Merge into existing cache
	e.LeverageBracketsLock.Lock()
	if e.LeverageBrackets == nil {
		e.LeverageBrackets = make(map[string]*banexg.SymbolLvgBrackets)
	}
	for k, v := range brackets {
		e.LeverageBrackets[k] = v
	}
	e.LeverageBracketsLock.Unlock()
	// Return the bracket for this symbol
	return findLeverageBracket(e, symbol)
}

// marketToInstTypeForLeverage determines the instType for leverage query based on market.
func marketToInstTypeForLeverage(market *banexg.Market) (string, *errs.Error) {
	if market.Margin && !market.Contract {
		return InstTypeMargin, nil
	}
	if market.Option {
		return InstTypeOption, nil
	}
	if market.Swap {
		return InstTypeSwap, nil
	}
	if market.Future {
		return InstTypeFutures, nil
	}
	return "", errs.NewMsg(errs.CodeUnsupportMarket, "leverage brackets only support margin/derivatives")
}

func (e *OKX) CalcMaintMargin(symbol string, cost float64) (float64, *errs.Error) {
	info := e.findOrLoadLvgBracket(symbol)
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

func (e *OKX) fetchPositionTiers(args map[string]interface{}) ([]PositionTier, *errs.Error) {
	tryNum := e.GetRetryNum("LoadLeverageBrackets", 1)
	res := requestRetry[[]map[string]interface{}](e, MethodPublicGetPositionTiers, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	if len(res.Result) == 0 {
		return nil, nil
	}
	tiers, err := decodeResult[PositionTier](res.Result)
	if err != nil {
		return nil, err
	}
	return tiers, nil
}

func buildLeverageBrackets(tiers []PositionTier) map[string]*banexg.SymbolLvgBrackets {
	if len(tiers) == 0 {
		return map[string]*banexg.SymbolLvgBrackets{}
	}
	grouped := make(map[string][]PositionTier)
	for _, tier := range tiers {
		key := strings.TrimSpace(tier.InstId)
		if key == "" {
			key = strings.TrimSpace(tier.InstFamily)
		}
		if key == "" {
			key = strings.TrimSpace(tier.Uly)
		}
		if key == "" {
			continue
		}
		grouped[key] = append(grouped[key], tier)
	}
	result := make(map[string]*banexg.SymbolLvgBrackets, len(grouped))
	for key, items := range grouped {
		sort.Slice(items, func(i, j int) bool {
			return parseFloat(items[i].MinSz) < parseFloat(items[j].MinSz)
		})
		brackets := make([]*banexg.LvgBracket, 0, len(items))
		cum := 0.0
		for i, item := range items {
			floor := parseFloat(item.MinSz)
			capacity := parseFloat(item.MaxSz)
			mmr := parseFloat(item.Mmr)
			lev := parseFloat(item.MaxLever)
			tierNum := int(parseFloat(item.Tier))
			if tierNum == 0 {
				tierNum = i + 1
			}
			brackets = append(brackets, &banexg.LvgBracket{
				BaseLvgBracket: banexg.BaseLvgBracket{
					Bracket:          tierNum,
					InitialLeverage:  int(math.Round(lev)),
					MaintMarginRatio: mmr,
					Cum:              cum,
				},
				Floor:    floor,
				Capacity: capacity,
			})
			if capacity > floor {
				cum += (capacity - floor) * mmr
			}
		}
		result[key] = &banexg.SymbolLvgBrackets{
			Symbol:   key,
			Brackets: brackets,
		}
	}
	return result
}

func findLeverageBracket(e *OKX, symbol string) *banexg.SymbolLvgBrackets {
	if e == nil {
		return nil
	}
	e.LeverageBracketsLock.Lock()
	defer e.LeverageBracketsLock.Unlock()
	if len(e.LeverageBrackets) == 0 {
		return nil
	}
	key := symbol
	if market, err := e.GetMarket(symbol); err == nil && market != nil && market.ID != "" {
		key = market.ID
	}
	if info, ok := e.LeverageBrackets[key]; ok {
		return info
	}
	if family := instFamilyFromID(key); family != "" {
		if info, ok := e.LeverageBrackets[family]; ok {
			return info
		}
	}
	return nil
}
