package binance

import (
	"github.com/banbox/banexg/base"
	"github.com/banbox/banexg/utils"
	"strconv"
)

func (mar *BnbMarket) GetPrecision() *base.Precision {
	var pre = base.Precision{}
	if mar.QuantityPrecision > 0 {
		pre.Amount = float64(mar.QuantityPrecision)
	} else if mar.QuantityScale > 0 {
		pre.Amount = float64(mar.QuantityScale)
	}
	if mar.PricePrecision > 0 {
		pre.Price = float64(mar.PricePrecision)
	} else if mar.PriceScale > 0 {
		pre.Price = float64(mar.PriceScale)
	}
	pre.Base = float64(mar.BaseAssetPrecision)
	pre.Quote = float64(mar.QuotePrecision)
	return &pre
}

func (mar *BnbMarket) GetMarketLimits() (*base.MarketLimits, float64, float64) {
	minQty, _ := strconv.ParseFloat(mar.MinQty, 64)
	maxQty, _ := strconv.ParseFloat(mar.MaxQty, 64)
	var filters = make(map[string]BnbFilter)
	for _, flt := range mar.Filters {
		filters[utils.GetMapVal(flt, "filterType", "")] = flt
	}
	var res = base.MarketLimits{
		Amount: &base.LimitRange{
			Min: minQty,
			Max: maxQty,
		},
		Leverage: &base.LimitRange{},
		Price:    &base.LimitRange{},
		Cost:     &base.LimitRange{},
		Market:   &base.LimitRange{},
	}
	var pricePrec, amountPrec float64
	if flt, ok := filters["PRICE_FILTER"]; ok {
		// PRICE_FILTER reports zero values for maxPrice
		// since they updated filter types in November 2018
		// https://github.com/ccxt/ccxt/issues/4286
		// therefore limits['price']['max'] doesn't have any meaningful value except None
		res.Price.Min = utils.GetMapFloat(flt, "minPrice")
		res.Price.Max = utils.GetMapFloat(flt, "maxPrice")
		precText := utils.GetMapVal(flt, "tickSize", "")
		pricePrec = utils.PrecisionFromString(precText)
	}
	if flt, ok := filters["LOT_SIZE"]; ok {
		res.Amount.Min = utils.GetMapFloat(flt, "minQty")
		res.Amount.Max = utils.GetMapFloat(flt, "maxQty")
		amtText := utils.GetMapVal(flt, "stepSize", "")
		amountPrec = utils.PrecisionFromString(amtText)
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
	return &res, pricePrec, amountPrec
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
