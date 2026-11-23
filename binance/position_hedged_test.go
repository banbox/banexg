package binance

import (
	"testing"

	"github.com/banbox/banexg"
)

// One-way (net) mode reports positionSide BOTH. The position keeps a long/short
// direction from the amount sign but must not be reported as hedged; only
// LONG/SHORT position sides (hedge mode) are hedged.
func TestBaseContPositionHedgedFollowsVenuePositionSide(t *testing.T) {
	cases := []struct {
		side, amt string
		wantSide  string
		hedged    bool
	}{
		{"BOTH", "6.3", banexg.PosSideLong, false},
		{"BOTH", "-6.3", banexg.PosSideShort, false},
		{"LONG", "6.3", banexg.PosSideLong, true},
		{"SHORT", "-6.3", banexg.PosSideShort, true},
	}
	for _, c := range cases {
		p := &BaseContPosition{Symbol: "XRPUSDT", PositionSide: c.side, PositionAmt: c.amt, Leverage: "2", EntryPrice: "0.9405"}
		got := p.ToStdPos()
		if got == nil || got.Side != c.wantSide || got.Hedged != c.hedged {
			t.Fatalf("%s %s: got %+v, want side %s hedged %v", c.side, c.amt, got, c.wantSide, c.hedged)
		}
	}
}
