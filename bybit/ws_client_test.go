package bybit

import (
	"testing"

	"github.com/banbox/banexg"
)

func TestBybitWsBatchSize(t *testing.T) {
	cases := []struct {
		marketType string
		want       int
	}{
		{banexg.MarketSpot, 10},
		{banexg.MarketOption, 2000},
		{banexg.MarketLinear, 100},
	}
	for _, c := range cases {
		if got := bybitWsBatchSize(c.marketType); got != c.want {
			t.Fatalf("batch size mismatch market=%s: got %d want %d", c.marketType, got, c.want)
		}
	}
}
