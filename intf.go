package banexg

import (
	"github.com/banbox/banexg/errs"
	"io"
)

/*
Exchange interface
交易所接口
*/

type BanExchange interface {
	LoadMarkets(reload bool, params map[string]interface{}) (MarketMap, *errs.Error)
	GetCurMarkets() MarketMap
	GetMarket(symbol string) (*Market, *errs.Error)
	/*
		Map the original variety ID of the exchange to a standard symbol, where year is the year where the K-line data is located
		将交易所原始品种ID映射为标准symbol，year是K线数据所在年
	*/
	MapMarket(rawID string, year int) (*Market, *errs.Error)
	FetchTicker(symbol string, params map[string]interface{}) (*Ticker, *errs.Error)
	FetchTickers(symbols []string, params map[string]interface{}) ([]*Ticker, *errs.Error)
	FetchTickerPrice(symbol string, params map[string]interface{}) (map[string]float64, *errs.Error)
	LoadLeverageBrackets(reload bool, params map[string]interface{}) *errs.Error
	GetLeverage(symbol string, notional float64, account string) (float64, float64)
	CheckSymbols(symbols ...string) ([]string, []string)
	Info() *ExgInfo

	FetchOHLCV(symbol, timeframe string, since int64, limit int, params map[string]interface{}) ([]*Kline, *errs.Error)
	FetchOrderBook(symbol string, limit int, params map[string]interface{}) (*OrderBook, *errs.Error)

	// FetchOrder query given order
	FetchOrder(symbol, orderId string, params map[string]interface{}) (*Order, *errs.Error)
	// FetchOrders Get all account orders; active, canceled, or filled. (symbol required)
	FetchOrders(symbol string, since int64, limit int, params map[string]interface{}) ([]*Order, *errs.Error)
	FetchBalance(params map[string]interface{}) (*Balances, *errs.Error)
	// FetchAccountPositions Get account positions on all symbols
	FetchAccountPositions(symbols []string, params map[string]interface{}) ([]*Position, *errs.Error)
	// FetchPositions Get position risks (default) or account positions on all symbols
	FetchPositions(symbols []string, params map[string]interface{}) ([]*Position, *errs.Error)
	// FetchOpenOrders Get all open orders on a symbol or all symbol.
	FetchOpenOrders(symbol string, since int64, limit int, params map[string]interface{}) ([]*Order, *errs.Error)
	FetchIncomeHistory(inType string, symbol string, since int64, limit int, params map[string]interface{}) ([]*Income, *errs.Error)
	FetchLastPrices(symbols []string, params map[string]interface{}) ([]*LastPrice, *errs.Error)
	FetchFundingRate(symbol string, params map[string]interface{}) (*FundingRateCur, *errs.Error)
	FetchFundingRates(symbols []string, params map[string]interface{}) ([]*FundingRateCur, *errs.Error)
	FetchFundingRateHistory(symbol string, since int64, limit int, params map[string]interface{}) ([]*FundingRate, *errs.Error)

	CreateOrder(symbol, odType, side string, amount, price float64, params map[string]interface{}) (*Order, *errs.Error)
	EditOrder(symbol, orderId, side string, amount, price float64, params map[string]interface{}) (*Order, *errs.Error)
	CancelOrder(id string, symbol string, params map[string]interface{}) (*Order, *errs.Error)

	SetFees(fees map[string]map[string]float64)
	CalculateFee(symbol, odType, side string, amount float64, price float64, isMaker bool, params map[string]interface{}) (*Fee, *errs.Error)
	SetLeverage(leverage float64, symbol string, params map[string]interface{}) (map[string]interface{}, *errs.Error)
	CalcMaintMargin(symbol string, cost float64) (float64, *errs.Error)
	Call(method string, params map[string]interface{}) (*HttpRes, *errs.Error)

	WatchOrderBooks(symbols []string, limit int, params map[string]interface{}) (chan *OrderBook, *errs.Error)
	UnWatchOrderBooks(symbols []string, params map[string]interface{}) *errs.Error
	WatchOHLCVs(jobs [][2]string, params map[string]interface{}) (chan *PairTFKline, *errs.Error)
	UnWatchOHLCVs(jobs [][2]string, params map[string]interface{}) *errs.Error
	WatchMarkPrices(symbols []string, params map[string]interface{}) (chan map[string]float64, *errs.Error)
	UnWatchMarkPrices(symbols []string, params map[string]interface{}) *errs.Error
	WatchTrades(symbols []string, params map[string]interface{}) (chan *Trade, *errs.Error)
	UnWatchTrades(symbols []string, params map[string]interface{}) *errs.Error
	WatchMyTrades(params map[string]interface{}) (chan *MyTrade, *errs.Error)
	WatchBalance(params map[string]interface{}) (chan *Balances, *errs.Error)
	WatchPositions(params map[string]interface{}) (chan []*Position, *errs.Error)
	WatchAccountConfig(params map[string]interface{}) (chan *AccountConfig, *errs.Error)

	// SetDump Record all websocket messages to the specified file 将websocket所有消息记录到指定文件
	SetDump(path string) *errs.Error
	// SetReplay Replay all websocket messages from the specified file 从指定文件重放所有websocket消息
	SetReplay(path string) *errs.Error
	// GetReplayTo Retrieve the 13 bit timestamp of the next message to be replayed, with sys. MaxInt64 indicating no next message 获取下一个要重放的消息13位时间戳，sys.MaxInt64表示无下一个消息
	GetReplayTo() int64
	// ReplayOne Replay the next websocket message 重放下一个websocket消息
	ReplayOne() *errs.Error
	// ReplayAll Replay all recorded websocket messages 重放所有记录的websocket消息
	ReplayAll() *errs.Error
	// SetOnWsChan Trigger callback when creating a new websocket message chan 创建新websocket消息chan时触发回调
	SetOnWsChan(cb FuncOnWsChan)

	PrecAmount(m *Market, amount float64) (float64, *errs.Error)
	PrecPrice(m *Market, price float64) (float64, *errs.Error)
	PrecCost(m *Market, cost float64) (float64, *errs.Error)
	PrecFee(m *Market, fee float64) (float64, *errs.Error)

	HasApi(key, market string) bool
	SetOnHost(cb func(n string) string)
	PriceOnePip(symbol string) (float64, *errs.Error)
	IsContract(marketType string) bool
	MilliSeconds() int64

	GetAccount(id string) (*Account, *errs.Error)
	SetMarketType(marketType, contractType string) *errs.Error
	GetExg() *Exchange
	Close() *errs.Error
}

type WsConn interface {
	Close() error
	WriteClose() error
	NextWriter() (io.WriteCloser, error)
	ReadMsg() ([]byte, error)
	IsOK() bool
	GetID() int
	SetID(id int)
}
