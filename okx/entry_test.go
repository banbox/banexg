package okx

import (
	"testing"

	"github.com/banbox/banexg"
)

func TestHasCapabilities(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	if exg.Has == nil {
		t.Fatalf("expected has map")
	}
	has := exg.Has[""]
	if has == nil {
		t.Fatalf("expected default has map")
	}
	expected := map[string]int{
		banexg.ApiFetchTicker:           banexg.HasOk,
		banexg.ApiFetchTickers:          banexg.HasOk,
		banexg.ApiFetchTickerPrice:      banexg.HasOk,
		banexg.ApiLoadLeverageBrackets:  banexg.HasOk,
		banexg.ApiFetchCurrencies:       banexg.HasFail,
		banexg.ApiGetLeverage:           banexg.HasOk,
		banexg.ApiFetchOHLCV:            banexg.HasOk,
		banexg.ApiFetchOrderBook:        banexg.HasOk,
		banexg.ApiFetchOrder:            banexg.HasOk,
		banexg.ApiFetchOrders:           banexg.HasOk,
		banexg.ApiFetchBalance:          banexg.HasOk,
		banexg.ApiFetchAccountPositions: banexg.HasOk,
		banexg.ApiFetchPositions:        banexg.HasOk,
		banexg.ApiFetchOpenOrders:       banexg.HasOk,
		banexg.ApiCreateOrder:           banexg.HasOk,
		banexg.ApiEditOrder:             banexg.HasOk,
		banexg.ApiCancelOrder:           banexg.HasOk,
		banexg.ApiSetLeverage:           banexg.HasOk,
		banexg.ApiCalcMaintMargin:       banexg.HasOk,
		banexg.ApiWatchOrderBooks:       banexg.HasOk,
		banexg.ApiUnWatchOrderBooks:     banexg.HasOk,
		banexg.ApiWatchOHLCVs:           banexg.HasOk,
		banexg.ApiUnWatchOHLCVs:         banexg.HasOk,
		banexg.ApiWatchMarkPrices:       banexg.HasOk,
		banexg.ApiUnWatchMarkPrices:     banexg.HasOk,
		banexg.ApiWatchTrades:           banexg.HasOk,
		banexg.ApiUnWatchTrades:         banexg.HasOk,
		banexg.ApiWatchMyTrades:         banexg.HasOk,
		banexg.ApiWatchBalance:          banexg.HasOk,
		banexg.ApiWatchPositions:        banexg.HasOk,
		banexg.ApiWatchAccountConfig:    banexg.HasOk,
	}
	for api, want := range expected {
		if got := has[api]; got != want {
			t.Fatalf("has[%s] = %d, want %d", api, got, want)
		}
	}
}
