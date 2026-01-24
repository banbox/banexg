package bybit

import (
	"testing"
)

func bybitPickSymbolsForWs(t *testing.T, exg *Bybit, marketType string, n int) []string {
	t.Helper()
	if exg == nil {
		return nil
	}
	origin := exg.CareMarkets
	exg.CareMarkets = []string{marketType}
	defer func() { exg.CareMarkets = origin }()

	markets, err := exg.LoadMarkets(false, nil)
	if err != nil {
		t.Fatalf("LoadMarkets failed: %v", err)
	}
	candidates := pickBybitOrderBookMarkets(markets, marketType, n)
	if len(candidates) < n {
		t.Skipf("need %d %s markets, got %d", n, marketType, len(candidates))
		return nil
	}
	out := make([]string, 0, n)
	for _, m := range candidates[:n] {
		out = append(out, m.Symbol)
	}
	return out
}
