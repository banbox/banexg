package yahoo

import (
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
)

func New(Options map[string]interface{}) (*Yahoo, *errs.Error) {
	if Options == nil {
		Options = make(map[string]interface{})
	}
	if _, ok := Options[banexg.OptUserAgent]; !ok {
		Options[banexg.OptUserAgent] = defaultUserAgent
	}
	exg := &Yahoo{
		Exchange: &banexg.Exchange{
			ExgInfo: &banexg.ExgInfo{
				ID:        "yahoo",
				Name:      "Yahoo Finance",
				Countries: []string{"US"},
				FixedLvg:  true,
			},
			RateLimit: 200,
			Options:   Options,
			Hosts: &banexg.ExgHosts{
				Prod: map[string]string{
					HostQuery1: "https://query1.finance.yahoo.com",
					HostQuery2: "https://query2.finance.yahoo.com",
				},
			},
			Apis: map[string]*banexg.Entry{
				MidChartGet: {Path: "v8/finance/chart/{symbol}", Host: HostQuery1, Method: "GET", CacheSecs: 60},
				MidQuoteGet: {Path: "v7/finance/quote", Host: HostQuery1, Method: "GET", CacheSecs: 5},
			},
			Has: map[string]map[string]int{
				"": {
					banexg.ApiFetchTicker:           banexg.HasOk,
					banexg.ApiFetchTickers:          banexg.HasOk,
					banexg.ApiFetchTickerPrice:      banexg.HasOk,
					banexg.ApiFetchOHLCV:            banexg.HasOk,
					banexg.ApiLoadLeverageBrackets:  banexg.HasFail,
					banexg.ApiGetLeverage:           banexg.HasFail,
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
	if err := exg.Init(); err != nil {
		return nil, err
	}
	exg.Sign = makeSign(exg)
	return exg, nil
}

func NewExchange(Options map[string]interface{}) (banexg.BanExchange, *errs.Error) {
	return New(Options)
}
