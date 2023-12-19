package banexg

type BanExchange interface {
	LoadMarkets(reload bool, params *map[string]interface{}) (MarketMap, error)
	FetchOhlcv(symbol, timeframe string, since int64, limit int, params *map[string]interface{}) ([]*Kline, error)
	FetchBalance(params *map[string]interface{}) (*Balances, error)
	FetchOrders(symbol string, since int64, limit int, params *map[string]interface{}) ([]*Order, error)
}
