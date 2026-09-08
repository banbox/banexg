package china

import (
	"testing"

	"github.com/banbox/banexg"
)

func TestLoadMarketsForSymbols(t *testing.T) {
	exchange, err := NewExchange(map[string]interface{}{
		banexg.OptMarketType: banexg.MarketLinear,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	loader, ok := exchange.(banexg.SymbolScopedMarketLoader)
	if !ok {
		t.Fatal("exchange does not provide symbol-scoped market loading")
	}
	markets, err := loader.LoadMarketsForSymbols(true, []string{"IF2409", "AG2409"})
	if err != nil {
		t.Fatal(err)
	}
	for _, symbol := range []string{"IF2409", "AG2409"} {
		if market := markets[symbol]; market == nil || market.Symbol != symbol {
			t.Fatalf("missing requested market %s", symbol)
		}
	}
}
