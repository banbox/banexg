# BanExg - CryptoCurrency Exchange Trading Library
A Go library for cryptocurrency trading, whose most interfaces are consistent with [CCXT](https://github.com/ccxt/ccxt).  
Note: This project is currently in the pre-release state, the main interface test has passed, but it has not yet been tested in the long-term production environment.  
Currently supported exchanges: `binance`, `china`. Both implement the interface `BanExchange`  

# How to Use
```go
var options = map[string]interface{}{}
// more option keys can be found by type `banexg.Opt...`
options[banexg.OptMarketType] = banexg.MarketLinear  // usd based future market
exchange, err := bex.New('binance', options)

// You can also instantiate an exchange object directly
// exchange, err := binance.NewExchange(options)

// exchange is a BanExchange interface object
res, err := exg.FetchOHLCV("ETH/USDT:USDT", "1d", 0, 10, nil)
if err != nil {
    panic(err)
}
for _, k := range res {
    fmt.Printf("%v, %v %v %v %v %v\n", k.Time, k.Open, k.High, k.Low, k.Close, int(k.Volume))
}
```

# Complete Initialization Options
```go
// The following parameters can be passed when initializing the exchange object
var options = map[string]interface{}{
    // Proxy server address
    banexg.OptProxy: "http://127.0.0.1:7890",  
    
    // API key configuration method 1: directly configure a single account
    banexg.OptApiKey: "your-api-key",      // API Key
    banexg.OptApiSecret: "your-secret",    // API Secret
    
    // API key configuration method 2: configure multiple accounts
    banexg.OptAccCreds: map[string]map[string]interface{}{
        "account1": {
            "ApiKey": "key1",
            "ApiSecret": "secret1", 
        },
        "account2": {
            "ApiKey": "key2", 
            "ApiSecret": "secret2",
        },
    },
    banexg.OptAccName: "account1",  // Set default account
    
    // HTTP request related
    banexg.OptUserAgent: "Mozilla/5.0",  // Custom User-Agent
    banexg.OptReqHeaders: map[string]string{  // Custom request headers
        "X-Custom": "value",
    },
    
    // Market type settings
    banexg.OptMarketType: banexg.MarketLinear,     // Set default market type: spot/contract etc
    banexg.OptContractType: banexg.MarketSwap,     // Set contract type: perpetual/delivery
    banexg.OptTimeInForce: banexg.TimeInForceGTC,  // Order validity type
    
    // WebSocket related
    banexg.OptWsIntvs: map[string]int{  // WebSocket subscription intervals (milliseconds)
        "WatchOrderBooks": 100,  // Orderbook subscription interval
    },
    
    // API retry settings
    banexg.OptRetries: map[string]int{  // API call retry count
        "FetchOrderBook": 3,     // Retry 3 times when fetching orderbook
        "FetchPositions": 2,     // Retry 2 times when fetching positions
    },
    
    // API cache settings
    banexg.OptApiCaches: map[string]int{  // API result cache time (seconds)
        "FetchMarkets": 3600,    // Cache market info for 1 hour
    },
    
    // Fee settings
    banexg.OptFees: map[string]map[string]float64{
        "linear": {              // USDT-M contract fees
            "maker": 0.0002,     // Maker fee rate
            "taker": 0.0004,     // Taker fee rate
        },
        "inverse": {             // Coin-M contract fees
            "maker": 0.0001,
            "taker": 0.0005,
        },
    },
    
    // Debug options
    banexg.OptDebugWS: true,    // Print WebSocket debug info
    banexg.OptDebugAPI: true,   // Print API debug info
    
    // Data capture and replay
    banexg.OptDumpPath: "./ws_dump",      // WebSocket data save path
    banexg.OptDumpBatchSize: 1000,        // Number of messages per batch save
    banexg.OptReplayPath: "./ws_replay",  // Replay data path
}

// Create exchange instance with parameters
exchange, err := bex.New("binance", options)
if err != nil {
    panic(err) 
}
```

All above parameters are optional, pass according to actual needs. Some important notes:

1. API key configuration supports two methods:
   - Directly configure single account via OptApiKey and OptApiSecret
   - Configure multiple accounts via OptAccCreds and specify default account with OptAccName

2. Market type (OptMarketType) options:
   - MarketSpot: Spot
   - MarketMargin: Margin
   - MarketLinear: USDT-M Futures
   - MarketInverse: Coin-M Futures
   - MarketOption: Options

3. Contract type (OptContractType) options:
   - MarketSwap: Perpetual contracts
   - MarketFuture: Delivery contracts

4. Time in force (OptTimeInForce) options:
   - TimeInForceGTC: Good Till Cancel
   - TimeInForceIOC: Immediate or Cancel
   - TimeInForceFOK: Fill or Kill
   - TimeInForceGTX: Good Till Crossing
   - TimeInForceGTD: Good Till Date
   - TimeInForcePO: Post Only

5. API cache and retry counts can be set individually for different interfaces

6. Fees can be set with different rates for different market types

# API List
```go
// Load market information
LoadMarkets(reload bool, params map[string]interface{}) (MarketMap, *errs.Error)
GetCurMarkets() MarketMap
GetMarket(symbol string) (*Market, *errs.Error)
MapMarket(rawID string, year int) (*Market, *errs.Error)
FetchTicker(symbol string, params map[string]interface{}) (*Ticker, *errs.Error)
FetchTickers(symbols []string, params map[string]interface{}) ([]*Ticker, *errs.Error)
FetchTickerPrice(symbol string, params map[string]interface{}) (map[string]float64, *errs.Error)
LoadLeverageBrackets(reload bool, params map[string]interface{}) *errs.Error
GetLeverage(symbol string, notional float64, account string) (float64, float64)
CheckSymbols(symbols ...string) ([]string, []string)
Info() *ExgInfo

// Fetch OHLCV, orderbook, funding rate etc
FetchOHLCV(symbol, timeframe string, since int64, limit int, params map[string]interface{}) ([]*Kline, *errs.Error)
FetchOrderBook(symbol string, limit int, params map[string]interface{}) (*OrderBook, *errs.Error)
FetchLastPrices(symbols []string, params map[string]interface{}) ([]*LastPrice, *errs.Error)
FetchFundingRate(symbol string, params map[string]interface{}) (*FundingRateCur, *errs.Error)
FetchFundingRates(symbols []string, params map[string]interface{}) ([]*FundingRateCur, *errs.Error)
FetchFundingRateHistory(symbol string, since int64, limit int, params map[string]interface{}) ([]*FundingRate, *errs.Error)

// Authentication: fetch orders, balance, positions
FetchOrder(symbol, orderId string, params map[string]interface{}) (*Order, *errs.Error)
FetchOrders(symbol string, since int64, limit int, params map[string]interface{}) ([]*Order, *errs.Error)
FetchBalance(params map[string]interface{}) (*Balances, *errs.Error)
FetchAccountPositions(symbols []string, params map[string]interface{}) ([]*Position, *errs.Error)
FetchPositions(symbols []string, params map[string]interface{}) ([]*Position, *errs.Error)
FetchOpenOrders(symbol string, since int64, limit int, params map[string]interface{}) ([]*Order, *errs.Error)
FetchIncomeHistory(inType string, symbol string, since int64, limit int, params map[string]interface{}) ([]*Income, *errs.Error)

// Authentication: create, modify, cancel orders
CreateOrder(symbol, odType, side string, amount, price float64, params map[string]interface{}) (*Order, *errs.Error)
EditOrder(symbol, orderId, side string, amount, price float64, params map[string]interface{}) (*Order, *errs.Error)
CancelOrder(id string, symbol string, params map[string]interface{}) (*Order, *errs.Error)

// Set/calculate fees; set leverage, calculate maintenance margin
SetFees(fees map[string]map[string]float64)
CalculateFee(symbol, odType, side string, amount float64, price float64, isMaker bool, params map[string]interface{}) (*Fee, *errs.Error)
SetLeverage(leverage float64, symbol string, params map[string]interface{}) (map[string]interface{}, *errs.Error)
CalcMaintMargin(symbol string, cost float64) (float64, *errs.Error)

// WebSocket related: watch orderbook, klines, mark price, trades, balance, positions, account config
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

// WebSocket data capture and replay (for backtesting)
SetDump(path string) *errs.Error
SetReplay(path string) *errs.Error
GetReplayTo() int64
ReplayOne() *errs.Error
ReplayAll() *errs.Error
SetOnWsChan(cb FuncOnWsChan)

// Precision handling
PrecAmount(m *Market, amount float64) (float64, *errs.Error)
PrecPrice(m *Market, price float64) (float64, *errs.Error)
PrecCost(m *Market, cost float64) (float64, *errs.Error)
PrecFee(m *Market, fee float64) (float64, *errs.Error)

// Others
HasApi(key, market string) bool
SetOnHost(cb func(n string) string)
PriceOnePip(symbol string) (float64, *errs.Error)
IsContract(marketType string) bool
MilliSeconds() int64

GetAccount(id string) (*Account, *errs.Error)
SetMarketType(marketType, contractType string) *errs.Error
GetExg() *Exchange
Close() *errs.Error
```

# Note
### Use `Options` instead of `direct fields assign` to initialized a Exchange 
When an exchange object is initialized, some fields of simple types like int will have default type values. When setting these in the `Init` method, it's impossible to distinguish whether the current value is one set by the user or the default value. 
Therefore, any configuration needed from outside should be passed in through `Options`, and then these `Options` should be read and set onto the corresponding fields in the `Init` method.

### Market Type
<table>
<tr>
    <th rowspan="2"></th>
    <th rowspan="2">Spot</th>
    <th rowspan="2">Margin</th>
    <th colspan="2">Contract Linear</th>
    <th colspan="2">Contract Inverse</th>
    <th colspan="2">Option</th>
</tr>
<tr>
    <th>Swap Linear</th>
    <th>future Linear</th>
    <th>swap Inverse</th>
    <th>future Inverse</th>
    <th>option linear</th>
    <th>option inverse</th>
</tr>
<tr>
    <td>Desc</td>
    <td>spot</td>
    <td>margin</td>
    <td>USDⓈ-M Perpetual</td>
    <td>USDⓈ-M Futures</td>
    <td>COIN-M Perpetual</td>
    <td>COIN-M Futures</td>
    <td>USDⓈ-M Option</td>
    <td>COIN-M Option</td>
</tr>
</table>

### Common parameter naming adjustments
**`ccxt.defaultType` -> `MarketType`**  
The default market type of the current exchange. It can be set during initialization using `OptMarketType` or by modifying the `MarketType` property of the exchange at any time.  
Valid Values: `MarketSpot/MarketMargin/MarketLinear/MarketInverse/MarketOption`  
In ccxt, the naming of `defaultType` for Binance is inconsistent with other exchanges. In banexg, a unified naming convention has been applied.  

**`ContractType`**  
The contract type for the current exchange, with options of `swap` for perpetual contracts and `future` for contracts with an expiration date.   
It can be set during initialization using `OptContractType` or by modifying the `ContractType` property of the exchange after initialization.

# Contact Me
Email: `anyongjin163@163.com`  
WeChat: `jingyingsuixing`  
