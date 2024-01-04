package banexg

import (
	"github.com/anyongjin/banexg/errs"
	"io"
)

type BanExchange interface {
	LoadMarkets(reload bool, params *map[string]interface{}) (MarketMap, *errs.Error)
	FetchOhlcv(symbol, timeframe string, since int64, limit int, params *map[string]interface{}) ([]*Kline, *errs.Error)
	FetchBalance(params *map[string]interface{}) (*Balances, *errs.Error)
	FetchOrders(symbol string, since int64, limit int, params *map[string]interface{}) ([]*Order, *errs.Error)
	FetchOpenOrders(symbol string, since int64, limit int, params *map[string]interface{}) ([]*Order, *errs.Error)
	FetchTicker(symbol string, params *map[string]interface{}) (*Ticker, *errs.Error)
	FetchTickers(symbols []string, params *map[string]interface{}) ([]*Ticker, *errs.Error)
	FetchOrderBook(symbol string, limit int, params *map[string]interface{}) (*OrderBook, *errs.Error)
	CreateOrder(symbol, odType, side string, amount float64, price float64, params *map[string]interface{}) (*Order, *errs.Error)
	CancelOrder(id string, symbol string, params *map[string]interface{}) (*Order, *errs.Error)
	CalculateFee(symbol, odType, side string, amount float64, price float64, isMaker bool, params *map[string]interface{}) (*Fee, *errs.Error)
	SetLeverage(leverage int, symbol string, params *map[string]interface{}) (map[string]interface{}, *errs.Error)
	LoadLeverageBrackets(reload bool, params *map[string]interface{}) *errs.Error
	PriceOnePip(symbol string) (float64, *errs.Error)
}

type WsConn interface {
	Close() error
	WriteClose() error
	NextWriter() (io.WriteCloser, error)
	ReadMsg() ([]byte, error)
}
