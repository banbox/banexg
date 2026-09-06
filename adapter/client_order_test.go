package adapter_test

import (
	"strings"
	"testing"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/binance"
	"github.com/banbox/banexg/bybit"
	"github.com/banbox/banexg/china"
	"github.com/banbox/banexg/okx"
)

func TestClientOrderCapabilityPreservesAdapterLayouts(t *testing.T) {
	cases := []struct {
		name       string
		capability banexg.ClientOrderCapability
		want       string
	}{
		{name: "binance", capability: &binance.Binance{}, want: "banbot_42_client"},
		{name: "bybit", capability: &bybit.Bybit{}, want: "banbot_42_client"},
		{name: "okx", capability: &okx.OKX{}, want: "bxrc600000000000420000"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got, ok := banexg.BuildClientOrderID(item.capability, "banbot", 42, "client", false)
			if !ok {
				t.Fatal("BuildClientOrderID did not report capability")
			}
			if got != item.want {
				t.Fatalf("BuildClientOrderID() = %q; want %q", got, item.want)
			}
			if random, ok := banexg.BuildClientOrderID(item.capability, "banbot", 42, "client", true); !ok || random == "" {
				t.Fatalf("randomized BuildClientOrderID() = %q, capability=%v", random, ok)
			}
		})
	}
}

func TestClientOrderCapabilityIsNotGuessedForUnsupportedAdapter(t *testing.T) {
	if got, ok := banexg.BuildClientOrderID(&china.China{}, "banbot", 42, "client", false); ok || got != "" {
		t.Fatalf("unsupported adapter returned (%q, %v)", got, ok)
	}
	if capability := banexg.GetClientOrderCapability(struct{}{}); capability != nil {
		t.Fatalf("unexpected capability for unrelated value: %T", capability)
	}
	if !strings.HasPrefix("banbot_42_client", "banbot_") {
		t.Fatal("test fixture is invalid")
	}
}
