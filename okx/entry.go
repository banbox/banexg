package okx

import (
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
)

func New(options map[string]interface{}) (*OKX, *errs.Error) {
	exg := &OKX{
		WsPendingRecons: make(map[string]*WsPendingRecon),
		Exchange: &banexg.Exchange{
			ExgInfo: &banexg.ExgInfo{
				ID:        "okx",
				Name:      "OKX",
				Countries: []string{"SC"},
			},
			RateLimit:  20,
			Options:    options,
			TimeFrames: timeFrameMap,
			Hosts: &banexg.ExgHosts{
				Test: map[string]string{
					HostPublic:     "https://www.okx.com/api/v5",
					HostPrivate:    "https://www.okx.com/api/v5",
					HostWsPublic:   "wss://wspap.okx.com:8443/ws/v5/public",
					HostWsPrivate:  "wss://wspap.okx.com:8443/ws/v5/private",
					HostWsBusiness: "wss://wspap.okx.com:8443/ws/v5/business",
				},
				Prod: map[string]string{
					HostPublic:     "https://www.okx.com/api/v5",
					HostPrivate:    "https://www.okx.com/api/v5",
					HostWsPublic:   "wss://ws.okx.com:8443/ws/v5/public",
					HostWsPrivate:  "wss://ws.okx.com:8443/ws/v5/private",
					HostWsBusiness: "wss://ws.okx.com:8443/ws/v5/business",
				},
				Www: "https://www.okx.com",
				Doc: []string{
					"https://www.okx.com/docs-v5/",
				},
				Fees: "https://www.okx.com/fees",
			},
			Fees: &banexg.ExgFee{
				Main:   &banexg.TradeFee{FeeSide: "get", Taker: 0.001, Maker: 0.0008, Percentage: true},
				Linear: &banexg.TradeFee{FeeSide: "quote", Taker: 0.0005, Maker: 0.0002, Percentage: true},
			},
			Apis: map[string]*banexg.Entry{
				MethodPublicGetInstruments:         {Path: "public/instruments", Host: HostPublic, Method: "GET", Cost: 5},
				MethodMarketGetTicker:              {Path: "market/ticker", Host: HostPublic, Method: "GET", Cost: 5},
				MethodMarketGetTickers:             {Path: "market/tickers", Host: HostPublic, Method: "GET", Cost: 5},
				MethodMarketGetBooks:               {Path: "market/books", Host: HostPublic, Method: "GET", Cost: 5},
				MethodMarketGetBooksFull:           {Path: "market/books-full", Host: HostPublic, Method: "GET", Cost: 5},
				MethodMarketGetCandles:             {Path: "market/candles", Host: HostPublic, Method: "GET", Cost: 5},
				MethodMarketGetHistoryCandles:      {Path: "market/history-candles", Host: HostPublic, Method: "GET", Cost: 5},
				MethodPublicGetFundingRate:         {Path: "public/funding-rate", Host: HostPublic, Method: "GET", Cost: 5},
				MethodPublicGetFundingRateHistory:  {Path: "public/funding-rate-history", Host: HostPublic, Method: "GET", Cost: 5},
				MethodPublicGetPositionTiers:       {Path: "public/position-tiers", Host: HostPublic, Method: "GET", Cost: 5},
				MethodAccountGetBalance:            {Path: "account/balance", Host: HostPrivate, Method: "GET", Cost: 5},
				MethodAccountGetConfig:             {Path: "account/config", Host: HostPrivate, Method: "GET", Cost: 5},
				MethodAccountGetBills:              {Path: "account/bills", Host: HostPrivate, Method: "GET", Cost: 5},
				MethodAccountGetBillsArchive:       {Path: "account/bills-archive", Host: HostPrivate, Method: "GET", Cost: 5},
				MethodAccountGetPositions:          {Path: "account/positions", Host: HostPrivate, Method: "GET", Cost: 5},
				MethodAccountGetLeverageInfo:       {Path: "account/leverage-info", Host: HostPrivate, Method: "GET", Cost: 5},
				MethodAccountGetPositionTiers:      {Path: "account/position-tiers", Host: HostPrivate, Method: "GET", Cost: 5},
				MethodAccountSetLeverage:           {Path: "account/set-leverage", Host: HostPrivate, Method: "POST", Cost: 5},
				MethodTradePostOrder:               {Path: "trade/order", Host: HostPrivate, Method: "POST", Cost: 1},
				MethodTradePostOrderAlgo:           {Path: "trade/order-algo", Host: HostPrivate, Method: "POST", Cost: 1},
				MethodTradePostCancelOrder:         {Path: "trade/cancel-order", Host: HostPrivate, Method: "POST", Cost: 1},
				MethodTradePostCancelAlgos:         {Path: "trade/cancel-algos", Host: HostPrivate, Method: "POST", Cost: 1},
				MethodTradePostAmendOrder:          {Path: "trade/amend-order", Host: HostPrivate, Method: "POST", Cost: 1},
				MethodTradePostAmendAlgos:          {Path: "trade/amend-algos", Host: HostPrivate, Method: "POST", Cost: 1},
				MethodTradeGetOrder:                {Path: "trade/order", Host: HostPrivate, Method: "GET", Cost: 1},
				MethodTradeGetOrderAlgo:            {Path: "trade/order-algo", Host: HostPrivate, Method: "GET", Cost: 1},
				MethodTradeGetOrdersPending:        {Path: "trade/orders-pending", Host: HostPrivate, Method: "GET", Cost: 1},
				MethodTradeGetOrdersAlgoPending:    {Path: "trade/orders-algo-pending", Host: HostPrivate, Method: "GET", Cost: 1},
				MethodTradeGetOrdersHistory:        {Path: "trade/orders-history", Host: HostPrivate, Method: "GET", Cost: 1},
				MethodTradeGetOrdersHistoryArchive: {Path: "trade/orders-history-archive", Host: HostPrivate, Method: "GET", Cost: 1},
				MethodTradeGetOrdersAlgoHistory:    {Path: "trade/orders-algo-history", Host: HostPrivate, Method: "GET", Cost: 1},
			},
			Has: map[string]map[string]int{
				"": {
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
				},
			},
			CredKeys: map[string]bool{"ApiKey": true, "Secret": true, "Password": true},
		},
		WsAuthDone: make(map[string]chan *errs.Error),
		WsAuthed:   make(map[string]bool),
	}
	exg.Sign = makeSign(exg)
	exg.FetchMarkets = makeFetchMarkets(exg)
	exg.OnWsMsg = makeHandleWsMsg(exg)
	exg.OnWsReCon = makeHandleWsReCon(exg)
	exg.CheckWsTimeout = makeCheckWsTimeout(exg)
	err := exg.Init()
	return exg, err
}

func NewExchange(options map[string]interface{}) (banexg.BanExchange, *errs.Error) {
	return New(options)
}
