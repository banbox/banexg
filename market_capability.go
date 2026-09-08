package banexg

import "github.com/banbox/banexg/errs"

// SymbolScopedMarketLoader is an optional adapter capability for exchanges
// whose market metadata can only be loaded for an explicit symbol set.
// Generic callers must not infer this behavior from the market type.
type SymbolScopedMarketLoader interface {
	LoadMarketsForSymbols(reload bool, symbols []string) (MarketMap, *errs.Error)
}
