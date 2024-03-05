package banexg

import (
	"github.com/banbox/banexg/errs"
	"io"
)

type BanExchange interface {
	LoadMarkets(reload bool, params *map[string]interface{}) (MarketMap, *errs.Error)
	GetCurMarkets() MarketMap
	GetMarket(symbol string) (*Market, *errs.Error)
	FetchTicker(symbol string, params *map[string]interface{}) (*Ticker, *errs.Error)
	FetchTickers(symbols []string, params *map[string]interface{}) ([]*Ticker, *errs.Error)
	FetchTickerPrice(symbol string, params *map[string]interface{}) (map[string]float64, *errs.Error)
	LoadLeverageBrackets(reload bool, params *map[string]interface{}) *errs.Error
	GetLeverage(symbol string, notional float64, account string) (int, int)
	CheckSymbols(symbols ...string) ([]string, []string)

	FetchOHLCV(symbol, timeframe string, since int64, limit int, params *map[string]interface{}) ([]*Kline, *errs.Error)
	FetchOrderBook(symbol string, limit int, params *map[string]interface{}) (*OrderBook, *errs.Error)

	FetchOrders(symbol string, since int64, limit int, params *map[string]interface{}) ([]*Order, *errs.Error)
	FetchBalance(params *map[string]interface{}) (*Balances, *errs.Error)
	FetchAccountPositions(symbols []string, params *map[string]interface{}) ([]*Position, *errs.Error)
	FetchPositions(symbols []string, params *map[string]interface{}) ([]*Position, *errs.Error)
	FetchOpenOrders(symbol string, since int64, limit int, params *map[string]interface{}) ([]*Order, *errs.Error)

	CreateOrder(symbol, odType, side string, amount float64, price float64, params *map[string]interface{}) (*Order, *errs.Error)
	CancelOrder(id string, symbol string, params *map[string]interface{}) (*Order, *errs.Error)

	CalculateFee(symbol, odType, side string, amount float64, price float64, isMaker bool, params *map[string]interface{}) (*Fee, *errs.Error)
	SetLeverage(leverage int, symbol string, params *map[string]interface{}) (map[string]interface{}, *errs.Error)
	CalcMaintMargin(symbol string, cost float64) (float64, *errs.Error)

	WatchOrderBooks(symbols []string, limit int, params *map[string]interface{}) (chan *OrderBook, *errs.Error)
	UnWatchOrderBooks(symbols []string, params *map[string]interface{}) *errs.Error
	WatchOHLCVs(jobs [][2]string, params *map[string]interface{}) (chan *PairTFKline, *errs.Error)
	UnWatchOHLCVs(jobs [][2]string, params *map[string]interface{}) *errs.Error
	WatchMarkPrices(symbols []string, params *map[string]interface{}) (chan map[string]float64, *errs.Error)
	UnWatchMarkPrices(symbols []string, params *map[string]interface{}) *errs.Error
	WatchTrades(symbols []string, params *map[string]interface{}) (chan *Trade, *errs.Error)
	UnWatchTrades(symbols []string, params *map[string]interface{}) *errs.Error
	WatchMyTrades(params *map[string]interface{}) (chan *MyTrade, *errs.Error)
	WatchBalance(params *map[string]interface{}) (chan *Balances, *errs.Error)
	WatchPositions(params *map[string]interface{}) (chan []*Position, *errs.Error)
	WatchAccountConfig(params *map[string]interface{}) (chan *AccountConfig, *errs.Error)

	PrecAmount(m *Market, amount float64) (float64, *errs.Error)
	PrecPrice(m *Market, price float64) (float64, *errs.Error)
	PrecCost(m *Market, cost float64) (float64, *errs.Error)
	PrecFee(m *Market, fee float64) (float64, *errs.Error)
	PrecMode() int

	HasApi(key string) bool
	PriceOnePip(symbol string) (float64, *errs.Error)
	IsContract(marketType string) bool
	MilliSeconds() int64

	GetAccount(id string) (*Account, *errs.Error)
	SetMarketType(marketType, contractType string) *errs.Error
	GetID() string
	Close() *errs.Error
}

type WsConn interface {
	Close() error
	WriteClose() error
	NextWriter() (io.WriteCloser, error)
	ReadMsg() ([]byte, error)
}
