package okx

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

// banexg MarketType -> OKX instType
var marketToInstType = map[string]string{
	banexg.MarketSpot:    InstTypeSpot,
	banexg.MarketMargin:  InstTypeMargin,
	banexg.MarketLinear:  InstTypeSwap,
	banexg.MarketInverse: InstTypeSwap,
	banexg.MarketOption:  InstTypeOption,
	banexg.MarketSwap:    InstTypeSwap,
	banexg.MarketFuture:  InstTypeFutures,
}

// instTypeByMarket maps banexg market/contract types to OKX instType.
// contractType matters for linear/inverse: swap vs futures.
func instTypeByMarket(marketType, contractType string) string {
	switch marketType {
	case banexg.MarketLinear, banexg.MarketInverse:
		if contractType == banexg.MarketFuture {
			return InstTypeFutures
		}
		return InstTypeSwap
	case banexg.MarketSwap:
		return InstTypeSwap
	case banexg.MarketFuture:
		return InstTypeFutures
	}
	return marketToInstType[marketType]
}

// instTypeFromMarket derives OKX instType from a resolved market.
func instTypeFromMarket(market *banexg.Market) string {
	if market == nil {
		return ""
	}
	switch {
	case market.Swap:
		return InstTypeSwap
	case market.Future:
		return InstTypeFutures
	case market.Option:
		return InstTypeOption
	case market.Margin:
		return InstTypeMargin
	case market.Spot:
		return InstTypeSpot
	}
	return instTypeByMarket(market.Type, "")
}

// OKX instType -> banexg MarketType
func parseMarketType(instType, ctType string) string {
	switch instType {
	case InstTypeSpot:
		return banexg.MarketSpot
	case InstTypeMargin:
		return banexg.MarketMargin
	case InstTypeSwap:
		if ctType == "inverse" {
			return banexg.MarketInverse
		}
		return banexg.MarketLinear
	case InstTypeFutures:
		if ctType == "inverse" {
			return banexg.MarketInverse
		}
		return banexg.MarketLinear
	case InstTypeOption:
		return banexg.MarketOption
	}
	return banexg.MarketSpot
}

func instFamilyFromID(instId string) string {
	parts := strings.Split(instId, "-")
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "-" + parts[1]
}

func parsePosition(e *OKX, item *Position, info map[string]interface{}) *banexg.Position {
	if item == nil {
		return nil
	}
	posVal := parseFloat(item.Pos)
	entry := parseFloat(item.AvgPx)
	if posVal == 0 && entry == 0 {
		return nil
	}
	side := strings.ToLower(item.PosSide)
	if side != banexg.PosSideLong && side != banexg.PosSideShort {
		if posVal > 0 {
			side = banexg.PosSideLong
		} else if posVal < 0 {
			side = banexg.PosSideShort
		}
	}
	leverage, _ := strconv.Atoi(item.Lever)
	liqPx := parseFloat(item.LiqPx)
	markPx := parseFloat(item.MarkPx)
	margin := parseFloat(item.Margin)
	mgnRatio := parseFloat(item.MgnRatio)
	upl := parseFloat(item.Upl)
	ts := parseInt(item.UTime)
	if ts == 0 {
		ts = parseInt(item.CTime)
	}
	marketType := parseMarketType(item.InstType, "")
	market := getMarketByIDAny(e, item.InstId, marketType)
	symbol := item.InstId
	contractSize := 0.0
	if market != nil {
		symbol = market.Symbol
		contractSize = market.ContractSize
	}
	notional := 0.0
	if entry > 0 && contractSize > 0 {
		notional = math.Abs(posVal) * entry * contractSize
	}
	return &banexg.Position{
		ID:               item.PosId,
		Symbol:           symbol,
		TimeStamp:        ts,
		Isolated:         strings.ToLower(item.MgnMode) == banexg.MarginIsolated,
		Side:             side,
		Contracts:        math.Abs(posVal),
		ContractSize:     contractSize,
		EntryPrice:       entry,
		MarkPrice:        markPx,
		Notional:         notional,
		Leverage:         leverage,
		Collateral:       margin,
		UnrealizedPnl:    upl,
		LiquidationPrice: liqPx,
		MarginMode:       strings.ToLower(item.MgnMode),
		MarginRatio:      mgnRatio,
		Info:             info,
	}
}

func getMarketByIDAny(e *OKX, marketId, marketType string) *banexg.Market {
	if e == nil {
		return nil
	}
	market := e.GetMarketById(marketId, marketType)
	if market != nil {
		return market
	}
	e.MarketsByIdLock.Lock()
	list := e.MarketsById[marketId]
	e.MarketsByIdLock.Unlock()
	if len(list) > 0 {
		return list[0]
	}
	return nil
}

func parseBoolStr(val string) bool {
	val = strings.ToLower(strings.TrimSpace(val))
	return val == "true" || val == "1"
}

func parseFloat(val string) float64 {
	out, _ := strconv.ParseFloat(val, 64)
	return out
}

func parseInt(val string) int64 {
	out, _ := strconv.ParseInt(val, 10, 64)
	return out
}

// decodeResult decodes raw map slice to struct slice and returns error
func decodeResult[T any](items []map[string]interface{}) ([]T, *errs.Error) {
	var arr []T
	if err := utils.DecodeStructMap(items, &arr, "json"); err != nil {
		return nil, errs.New(errs.CodeUnmarshalFail, err)
	}
	return arr, nil
}

// getTradeQuoteCcy returns market.Quote if it exists in tradeQuoteCcyList.
func getTradeQuoteCcy(market *banexg.Market) string {
	if market == nil || market.Info == nil || market.Quote == "" {
		return ""
	}
	ccyMap, ok := market.Info["tradeQuoteCcyList"].(map[string]bool)
	if !ok || !ccyMap[market.Quote] {
		return ""
	}
	return market.Quote
}

func pickArchiveMethod(args map[string]interface{}, since, until int64, recentMethod, archiveMethod string) string {
	useArchive := utils.PopMapVal(args, banexg.ParamArchive, false)
	if useArchive {
		return archiveMethod
	}
	const sevenDaysMs = int64(7 * 24 * 60 * 60 * 1000)
	now := time.Now().UnixMilli()
	if since > 0 && now-since > sevenDaysMs {
		return archiveMethod
	}
	if until > 0 && now-until > sevenDaysMs {
		return archiveMethod
	}
	return recentMethod
}
