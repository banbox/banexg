package okx

import (
	"testing"

	"github.com/banbox/banexg"
)

func TestParseMarketType(t *testing.T) {
	cases := []struct {
		instType string
		ctType   string
		want     string
	}{
		{"SPOT", "", banexg.MarketSpot},
		{"MARGIN", "", banexg.MarketMargin},
		{"SWAP", "linear", banexg.MarketLinear},
		{"SWAP", "inverse", banexg.MarketInverse},
		{"FUTURES", "linear", banexg.MarketLinear},
		{"FUTURES", "inverse", banexg.MarketInverse},
		{"OPTION", "", banexg.MarketOption},
	}
	for _, c := range cases {
		if got := parseMarketType(c.instType, c.ctType); got != c.want {
			t.Fatalf("parseMarketType(%s,%s)=%s want=%s", c.instType, c.ctType, got, c.want)
		}
	}
}

func TestParseInstrument(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	inst := &Instrument{
		InstType:  "SWAP",
		InstId:    "BTC-USDT-SWAP",
		BaseCcy:   "BTC",
		QuoteCcy:  "USDT",
		SettleCcy: "USDT",
		CtVal:     "0.001",
		CtType:    "linear",
		TickSz:    "0.1",
		LotSz:     "1",
		MinSz:     "0.01",
		State:     "live",
		ListTime:  "1606468572000",
	}
	mar := parseInstrument(exg, inst)
	if mar.ID != inst.InstId || mar.Symbol != "BTC/USDT:USDT" {
		t.Fatalf("unexpected id/symbol: %s/%s", mar.ID, mar.Symbol)
	}
	if !mar.Swap || !mar.Contract || !mar.Linear || mar.Inverse {
		t.Fatalf("unexpected contract flags: swap=%v contract=%v linear=%v inverse=%v", mar.Swap, mar.Contract, mar.Linear, mar.Inverse)
	}
	if mar.Type != banexg.MarketLinear {
		t.Fatalf("unexpected market type: %s", mar.Type)
	}
	if mar.Precision == nil || mar.Precision.Price != 0.1 || mar.Precision.Amount != 1 {
		t.Fatalf("unexpected precision: %+v", mar.Precision)
	}
	if mar.Limits == nil || mar.Limits.Amount == nil || mar.Limits.Amount.Min != 0.01 {
		t.Fatalf("unexpected limits: %+v", mar.Limits)
	}
	if !mar.Active {
		t.Fatalf("expected market active")
	}
}
