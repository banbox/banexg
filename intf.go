package banexg

type BanExchange interface {
	LoadMarkets(reload bool, params *map[string]interface{}) (MarketMap, error)
	FetchOhlcv(symbol, timeframe string, since int64, limit int, params *map[string]interface{}) ([]*Kline, error)
	FetchBalance(params *map[string]interface{}) (*Balances, error)
	FetchOrders(symbol string, since int64, limit int, params *map[string]interface{}) ([]*Order, error)
	FetchTicker(symbol string, params *map[string]interface{}) (*Ticker, error)
	FetchTickers(symbols []string, params *map[string]interface{}) ([]*Ticker, error)
	CreateOrder(symbol, odType, side string, amount float64, price float64, params *map[string]interface{}) (*Order, error)
	CalculateFee(symbol, odType, side string, amount float64, price float64, isMaker bool, params *map[string]interface{}) (*Fee, error)
	PriceOnePip(symbol string) (float64, error)
}
