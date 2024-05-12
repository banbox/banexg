package china

import (
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
)

func New(Options map[string]interface{}) (*China, *errs.Error) {
	exg := &China{
		Exchange: &banexg.Exchange{
			ExgInfo: &banexg.ExgInfo{
				ID:        "china",
				Name:      "China",
				Countries: []string{"CN"},
				FixedLvg:  true,
			},
			RateLimit: 50,
			Options:   Options,
			Hosts:     &banexg.ExgHosts{},
			Fees: &banexg.ExgFee{
				Linear: &banexg.TradeFee{
					FeeSide:    "quote",
					TierBased:  false,
					Percentage: true,
					Taker:      0.0002,
					Maker:      0.0002,
				},
			},
			Apis: map[string]banexg.Entry{
				"test": {Path: "", Host: "", Method: "GET"},
			},
			Has: map[string]map[string]int{
				"": {
					banexg.ApiFetchTicker:           banexg.HasFail,
					banexg.ApiFetchTickers:          banexg.HasFail,
					banexg.ApiFetchTickerPrice:      banexg.HasFail,
					banexg.ApiLoadLeverageBrackets:  banexg.HasOk,
					banexg.ApiGetLeverage:           banexg.HasOk,
					banexg.ApiFetchOHLCV:            banexg.HasFail,
					banexg.ApiFetchOrderBook:        banexg.HasFail,
					banexg.ApiFetchOrder:            banexg.HasFail,
					banexg.ApiFetchOrders:           banexg.HasFail,
					banexg.ApiFetchBalance:          banexg.HasFail,
					banexg.ApiFetchAccountPositions: banexg.HasFail,
					banexg.ApiFetchPositions:        banexg.HasFail,
					banexg.ApiFetchOpenOrders:       banexg.HasFail,
					banexg.ApiCreateOrder:           banexg.HasFail,
					banexg.ApiEditOrder:             banexg.HasFail,
					banexg.ApiCancelOrder:           banexg.HasFail,
					banexg.ApiSetLeverage:           banexg.HasFail,
					banexg.ApiCalcMaintMargin:       banexg.HasFail,
					banexg.ApiWatchOrderBooks:       banexg.HasFail,
					banexg.ApiUnWatchOrderBooks:     banexg.HasFail,
					banexg.ApiWatchOHLCVs:           banexg.HasFail,
					banexg.ApiUnWatchOHLCVs:         banexg.HasFail,
					banexg.ApiWatchMarkPrices:       banexg.HasFail,
					banexg.ApiUnWatchMarkPrices:     banexg.HasFail,
					banexg.ApiWatchTrades:           banexg.HasFail,
					banexg.ApiUnWatchTrades:         banexg.HasFail,
					banexg.ApiWatchMyTrades:         banexg.HasFail,
					banexg.ApiWatchBalance:          banexg.HasFail,
					banexg.ApiWatchPositions:        banexg.HasFail,
					banexg.ApiWatchAccountConfig:    banexg.HasFail,
				},
			},
		},
	}
	err := exg.Init()
	return exg, err
}

func NewExchange(Options map[string]interface{}) (banexg.BanExchange, *errs.Error) {
	return New(Options)
}
