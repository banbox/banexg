package china

import (
	"github.com/banbox/banexg"
	"testing"
)

func TestChina_MapMarket(t *testing.T) {
	exg, err := NewExchange(map[string]interface{}{
		banexg.OptMarketType:   banexg.MarketLinear,
		banexg.OptContractType: banexg.MarketSwap,
	})
	if err != nil {
		panic(err)
	}
	type Item struct {
		code   string
		year   int
		result string
	}
	items := []*Item{
		{code: "AP005", year: 2019, result: "AP2005"},
		{code: "AP005", year: 2020, result: "AP2005"},
		{code: "ag2102", year: 2020, result: "AG2102"},
		{code: "ag2102", year: 2021, result: "AG2102"},
	}
	for _, it := range items {
		mar, err := exg.MapMarket(it.code, it.year)
		if err != nil {
			panic(err)
		}
		if mar.Symbol != it.result {
			t.Error(it.code, it.year, it.result, mar.Symbol)
		}
	}
}
