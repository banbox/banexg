package binance

import (
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/utils"
	"strconv"
)

func (mar *BnbMarket) GetPrecision() *banexg.Precision {
	var pre = banexg.Precision{}
	if mar.QuantityPrecision > 0 {
		pre.Amount = float64(mar.QuantityPrecision)
		pre.ModeAmount = banexg.PrecModeDecimalPlace
	} else if mar.QuantityScale > 0 {
		pre.Amount = float64(mar.QuantityScale)
		pre.ModeAmount = banexg.PrecModeDecimalPlace
	}
	if mar.PricePrecision > 0 {
		pre.Price = float64(mar.PricePrecision)
		pre.ModePrice = banexg.PrecModeDecimalPlace
	} else if mar.PriceScale > 0 {
		pre.Price = float64(mar.PriceScale)
		pre.ModePrice = banexg.PrecModeDecimalPlace
	}
	pre.Base = float64(mar.BaseAssetPrecision)
	pre.ModeBase = banexg.PrecModeDecimalPlace
	pre.Quote = float64(mar.QuotePrecision)
	pre.ModeQuote = banexg.PrecModeDecimalPlace
	return &pre
}

func (mar *BnbMarket) GetMarketLimits(p *banexg.Precision) *banexg.MarketLimits {
	minQty, _ := strconv.ParseFloat(mar.MinQty, 64)
	maxQty, _ := strconv.ParseFloat(mar.MaxQty, 64)
	var filters = make(map[string]BnbFilter)
	for _, flt := range mar.Filters {
		filters[utils.GetMapVal(flt, "filterType", "")] = flt
	}
	var res = banexg.MarketLimits{
		Amount: &banexg.LimitRange{
			Min: minQty,
			Max: maxQty,
		},
		Leverage: &banexg.LimitRange{},
		Price:    &banexg.LimitRange{},
		Cost:     &banexg.LimitRange{},
		Market:   &banexg.LimitRange{},
	}
	if flt, ok := filters["PRICE_FILTER"]; ok {
		// PRICE_FILTER reports zero values for maxPrice
		// since they updated filter types in November 2018
		// https://github.com/ccxt/ccxt/issues/4286
		// therefore limits['price']['max'] doesn't have any meaningful value except None
		res.Price.Min = utils.GetMapFloat(flt, "minPrice")
		res.Price.Max = utils.GetMapFloat(flt, "maxPrice")
		priceTick := utils.GetMapFloat(flt, "tickSize")
		if priceTick > 0 {
			p.Price = priceTick
			p.ModePrice = banexg.PrecModeTickSize
		}
	}
	if flt, ok := filters["LOT_SIZE"]; ok {
		res.Amount.Min = utils.GetMapFloat(flt, "minQty")
		res.Amount.Max = utils.GetMapFloat(flt, "maxQty")
		amountTick := utils.GetMapFloat(flt, "stepSize")
		if amountTick > 0 {
			p.Amount = amountTick
			p.ModeAmount = banexg.PrecModeTickSize
		}
	}
	if flt, ok := filters["MARKET_LOT_SIZE"]; ok {
		res.Market.Min = utils.GetMapFloat(flt, "minQty")
		res.Market.Max = utils.GetMapFloat(flt, "maxQty")
	}
	if flt, ok := filters["MIN_NOTIONAL"]; ok {
		res.Cost.Min = utils.GetMapFloat(flt, "notional")
	} else if flt, ok := filters["NOTIONAL"]; ok {
		res.Cost.Min = utils.GetMapFloat(flt, "minNotional")
		res.Cost.Max = utils.GetMapFloat(flt, "maxNotional")
	}
	return &res
}

func (b *LinearSymbolLvgBrackets) ToStdBracket() *SymbolLvgBrackets {
	var res = SymbolLvgBrackets{
		NotionalCoef: b.NotionalCoef,
		Brackets:     make([]*LvgBracket, len(b.Brackets)),
	}
	for i, item := range b.Brackets {
		res.Brackets[i] = &LvgBracket{
			BaseLvgBracket: item.BaseLvgBracket,
			Capacity:       item.NotionalCap,
			Floor:          item.NotionalFloor,
		}
	}
	return &res
}
func (b *LinearSymbolLvgBrackets) GetSymbol() string {
	return b.Symbol
}

func (b *InversePairLvgBrackets) ToStdBracket() *SymbolLvgBrackets {
	var res = SymbolLvgBrackets{
		NotionalCoef: b.NotionalCoef,
		Brackets:     make([]*LvgBracket, len(b.Brackets)),
	}
	for i, item := range b.Brackets {
		res.Brackets[i] = &LvgBracket{
			BaseLvgBracket: item.BaseLvgBracket,
			Capacity:       item.QtyCap,
			Floor:          item.QtylFloor,
		}
	}
	return &res
}
func (b *InversePairLvgBrackets) GetSymbol() string {
	return b.Symbol
}
