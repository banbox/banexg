package binance

import "testing"

func TestMapMarketPerpetualDeliveryDate(t *testing.T) {
	e, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, contract string
		delivery, want int64
		swap           bool
	}{
		{"live-perpetual", "PERPETUAL", 4133404800000, 0, true},
		{"delisted-perpetual", "PERPETUAL", 1770000000000, 1770000000000, true},
		{"undated-perpetual", "PERPETUAL", 0, 0, true},
		{"legacy-swap", "", 4133404800000, 0, true},
		{"dated-future", "CURRENT_QUARTER", 1770000000000, 1770000000000, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			market := e.mapMarket(&BnbMarket{Symbol: "BTCUSDT", BaseAsset: "BTC", QuoteAsset: "USDT", MarginAsset: "USDT", ContractType: tt.contract, DeliveryDate: tt.delivery}, nil)
			if market.Expiry != tt.want || market.Swap != tt.swap {
				t.Fatalf("expiry=%d swap=%t; want expiry=%d swap=%t", market.Expiry, market.Swap, tt.want, tt.swap)
			}
			if tt.swap && tt.contract != "" && market.Symbol != "BTC/USDT:USDT" {
				t.Fatalf("swap symbol changed: %s", market.Symbol)
			}
		})
	}
}
