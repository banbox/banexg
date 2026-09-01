package china

import (
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

var _ banexg.PriceSymbolCapability = (*China)(nil)

// PriceSymbolParts returns base, quote, settlement currency, and contract ID.
func (e *China) PriceSymbolParts(symbol string) ([4]string, *errs.Error) {
	if symbol == "" {
		return [4]string{}, errs.NewMsg(errs.CodeParamRequired, "symbol is required")
	}
	market, err := e.MapMarket(symbol, 0)
	if err != nil {
		return [4]string{}, err
	}
	parts := utils.SplitParts(market.Symbol)
	identifier := ""
	if len(parts) > 1 {
		identifier = parts[1].Val
	}
	return [4]string{market.Base, "CNY", "CNY", identifier}, nil
}
