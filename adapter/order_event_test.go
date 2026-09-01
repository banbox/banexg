package adapter_test

import (
	"testing"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/binance"
	"github.com/banbox/banexg/bybit"
	"github.com/banbox/banexg/china"
	"github.com/banbox/banexg/okx"
)

func TestOrderEventCapabilityClientOrderID(t *testing.T) {
	cases := []struct {
		name       string
		capability banexg.OrderEventCapability
		clientID   string
		want       int64
	}{
		{
			name:       "binance",
			capability: &binance.Binance{},
			clientID:   "banbot_42_7_",
			want:       42,
		},
		{
			name:       "bybit",
			capability: &bybit.Bybit{},
			clientID:   "banbot_42_7_",
			want:       42,
		},
		{
			name:       "okx",
			capability: &okx.OKX{},
			clientID:   "bxrc600000000000420007",
			want:       42,
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if got := item.capability.ParseClientOrderID("banbot", item.clientID); got != item.want {
				t.Fatalf("ParseClientOrderID() = %d; want %d", got, item.want)
			}
			if got := item.capability.ParseClientOrderID("other", item.clientID); got != 0 {
				t.Fatalf("mismatched bot name parsed as %d", got)
			}
		})
	}
}

func TestOrderEventCapabilityRelationAndTimestamp(t *testing.T) {
	capabilities := []banexg.OrderEventCapability{
		&binance.Binance{},
		&okx.OKX{},
	}
	for _, capability := range capabilities {
		if got := capability.AlgoOrderID(&banexg.MyTrade{AlgoId: "123"}); got != "algo:123" {
			t.Fatalf("AlgoOrderID() = %q; want algo:123", got)
		}
		if got := capability.GetOrderEventOrderID(&banexg.MyTrade{AlgoId: "123"}); got != "algo:123" {
			t.Fatalf("GetOrderEventOrderID() = %q; want algo:123", got)
		}
		if got := capability.AlgoOrderID(&banexg.MyTrade{}); got != "" {
			t.Fatalf("empty algo order ID = %q", got)
		}
		if got := capability.GetOrderEventOrderID(&banexg.MyTrade{}); got != "" {
			t.Fatalf("empty event order ID = %q", got)
		}
		if got := capability.NormalizeOrderTimestamp(&banexg.Order{
			Timestamp:           100,
			LastTradeTimestamp:  130,
			LastUpdateTimestamp: 120,
		}, 140); got != 140 {
			t.Fatalf("NormalizeOrderTimestamp() = %d, want 140", got)
		}
	}
}

func TestOrderEventCapabilityDefaultIsSafe(t *testing.T) {
	capability := banexg.GetOrderEventCapability(struct{}{})
	if got := capability.ParseClientOrderID("banbot", "banbot_42_7_"); got != 0 {
		t.Fatalf("default capability guessed a client order ID: %d", got)
	}
	if got := capability.AlgoOrderID(&banexg.MyTrade{AlgoId: "123"}); got != "123" {
		t.Fatalf("default capability changed algo order ID to %q", got)
	}
	if got := capability.GetOrderEventOrderID(&banexg.MyTrade{AlgoId: "123"}); got != "" {
		t.Fatalf("default capability guessed event order ID %q", got)
	}
	if got := capability.NormalizeOrderTimestamp(&banexg.Order{Timestamp: 100, LastUpdateTimestamp: 200}, 150); got != 200 {
		t.Fatalf("default capability changed timestamp to %d", got)
	}
	if got := capability.NormalizeOrderTimestamp(nil, 200); got != 200 {
		t.Fatalf("default capability ignored fallback: %d", got)
	}
}

func TestOrderEventRootHelpersAndDefaultAdapters(t *testing.T) {
	if got := banexg.ParseClientOrderID(&binance.Binance{}, "banbot", "banbot_42_7_"); got != 42 {
		t.Fatalf("root Binance ParseClientOrderID() = %d; want 42", got)
	}
	if got := banexg.ParseClientOrderID(&bybit.Bybit{}, "banbot", "banbot_42_7_"); got != 42 {
		t.Fatalf("root Bybit ParseClientOrderID() = %d; want 42", got)
	}
	if got := banexg.AlgoOrderID(&okx.OKX{}, &banexg.MyTrade{AlgoId: "42"}); got != "algo:42" {
		t.Fatalf("root OKX AlgoOrderID() = %q; want algo:42", got)
	}
	if got := banexg.GetOrderEventOrderID(&okx.OKX{}, &banexg.MyTrade{AlgoId: "42"}); got != "algo:42" {
		t.Fatalf("root OKX GetOrderEventOrderID() = %q; want algo:42", got)
	}
	if got := banexg.NormalizeOrderTimestamp(&binance.Binance{}, &banexg.Order{Timestamp: 100}, 200); got != 200 {
		t.Fatalf("root NormalizeOrderTimestamp() = %d; want 200", got)
	}
	for _, exg := range []interface{}{&china.China{}} {
		if _, ok := exg.(banexg.OrderEventCapability); !ok {
			t.Fatalf("%T does not expose explicit default order event semantics", exg)
		}
		if got := banexg.ParseClientOrderID(exg, "banbot", "banbot_42_7_"); got != 0 {
			t.Fatalf("default adapter parsed client ID as %d", got)
		}
	}
}
