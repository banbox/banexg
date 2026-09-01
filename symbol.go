package banexg

import "github.com/banbox/banexg/errs"

// PriceSymbolCapability exposes adapter-owned quote and identifier semantics
// for symbols that cannot be represented by generic slash-delimited markets.
type PriceSymbolCapability interface {
	PriceSymbolParts(symbol string) ([4]string, *errs.Error)
}
