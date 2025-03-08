# BanExg - 数字货币交易所SDK
一个Go版本的数字货币交易SDK  
目前支持交易所：`binance`, `china`。都实现了接口`BanExchange`  

# 如何使用
```go
var options = map[string]interface{}{}
// more option keys can be found by type `banexg.Opt...`
options[banexg.OptMarketType] = banexg.MarketLinear  // usd based future market
exchange, err := bex.New('binance', options)

// 您也可以直接实例化某个交易所对象
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

# 完整初始化选项
```go
// 初始化交易所对象时可以传入以下参数
var options = map[string]interface{}{
    // 代理服务器地址
    banexg.OptProxy: "http://127.0.0.1:7890",  
    
    // API密钥配置方式1:直接配置单个账户
    banexg.OptApiKey: "your-api-key",      // API Key
    banexg.OptApiSecret: "your-secret",    // API Secret
    
    // API密钥配置方式2:配置多个账户
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
    banexg.OptAccName: "account1",  // 设置默认账户
    
    // HTTP请求相关
    banexg.OptUserAgent: "Mozilla/5.0",  // 自定义User-Agent
    banexg.OptReqHeaders: map[string]string{  // 自定义请求头
        "X-Custom": "value",
    },
    
    // 市场类型设置
    banexg.OptMarketType: banexg.MarketLinear,     // 设置默认市场类型:现货/合约等
    banexg.OptContractType: banexg.MarketSwap,     // 设置合约类型:永续/交割
    banexg.OptTimeInForce: banexg.TimeInForceGTC,  // 订单有效期类型
    
    // WebSocket相关
    banexg.OptWsIntvs: map[string]int{  // WebSocket订阅间隔(毫秒)
        "WatchOrderBooks": 100,  // 订阅订单簿的间隔
    },
    
    // API重试设置
    banexg.OptRetries: map[string]int{  // API调用重试次数
        "FetchOrderBook": 3,     // 获取订单簿时重试3次
        "FetchPositions": 2,     // 获取持仓时重试2次
    },
    
    // API缓存设置
    banexg.OptApiCaches: map[string]int{  // API结果缓存时间(秒)
        "FetchMarkets": 3600,    // 市场信息缓存1小时
    },
    
    // 手续费设置
    banexg.OptFees: map[string]map[string]float64{
        "linear": {              // U本位合约手续费
            "maker": 0.0002,     // Maker费率
            "taker": 0.0004,     // Taker费率
        },
        "inverse": {             // 币本位合约手续费
            "maker": 0.0001,
            "taker": 0.0005,
        },
    },
    
    // 调试选项
    banexg.OptDebugWS: true,    // 打印WebSocket调试信息
    banexg.OptDebugAPI: true,   // 打印API调试信息
    
    // 数据抓取、回放
    banexg.OptDumpPath: "./ws_dump",      // WebSocket数据保存路径
    banexg.OptDumpBatchSize: 1000,        // 每批次保存的消息数量
    banexg.OptReplayPath: "./ws_replay",  // 回放数据路径
}

// 使用参数创建交易所实例
exchange, err := bex.New("binance", options)
if err != nil {
    panic(err) 
}
```

以上参数都是可选的,根据实际需要传入。一些重要说明:

1. API密钥配置支持两种方式:
   - 直接通过OptApiKey和OptApiSecret配置单个账户
   - 通过OptAccCreds配置多个账户,并用OptAccName指定默认账户

2. 市场类型(OptMarketType)可选值:
   - MarketSpot: 现货
   - MarketMargin: 保证金
   - MarketLinear: U本位合约 
   - MarketInverse: 币本位合约
   - MarketOption: 期权

3. 合约类型(OptContractType)可选值:
   - MarketSwap: 永续合约
   - MarketFuture: 交割合约

4. 订单有效期(OptTimeInForce)可选值:
   - TimeInForceGTC: 成交为止
   - TimeInForceIOC: 立即成交或取消
   - TimeInForceFOK: 全部成交或取消
   - TimeInForceGTX: 无法成为挂单方就取消
   - TimeInForceGTD: 指定时间前有效
   - TimeInForcePO: 只做挂单方

5. API缓存和重试次数可以针对不同接口单独设置

6. 手续费可以针对不同市场类型设置不同费率
```

# API列表
```go
// 加载市场信息
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

// 获取K线、订单簿、资金费率等
FetchOHLCV(symbol, timeframe string, since int64, limit int, params map[string]interface{}) ([]*Kline, *errs.Error)
FetchOrderBook(symbol string, limit int, params map[string]interface{}) (*OrderBook, *errs.Error)
FetchLastPrices(symbols []string, params map[string]interface{}) ([]*LastPrice, *errs.Error)
FetchFundingRate(symbol string, params map[string]interface{}) (*FundingRateCur, *errs.Error)
FetchFundingRates(symbols []string, params map[string]interface{}) ([]*FundingRateCur, *errs.Error)
FetchFundingRateHistory(symbol string, since int64, limit int, params map[string]interface{}) ([]*FundingRate, *errs.Error)

// 鉴权：获取订单、余额、仓位
FetchOrder(symbol, orderId string, params map[string]interface{}) (*Order, *errs.Error)
FetchOrders(symbol string, since int64, limit int, params map[string]interface{}) ([]*Order, *errs.Error)
FetchBalance(params map[string]interface{}) (*Balances, *errs.Error)
FetchAccountPositions(symbols []string, params map[string]interface{}) ([]*Position, *errs.Error)
FetchPositions(symbols []string, params map[string]interface{}) ([]*Position, *errs.Error)
FetchOpenOrders(symbol string, since int64, limit int, params map[string]interface{}) ([]*Order, *errs.Error)
FetchIncomeHistory(inType string, symbol string, since int64, limit int, params map[string]interface{}) ([]*Income, *errs.Error)
// 鉴权：创建、修改、取消订单
CreateOrder(symbol, odType, side string, amount, price float64, params map[string]interface{}) (*Order, *errs.Error)
EditOrder(symbol, orderId, side string, amount, price float64, params map[string]interface{}) (*Order, *errs.Error)
CancelOrder(id string, symbol string, params map[string]interface{}) (*Order, *errs.Error)
// 设置、计算手续费；设置杠杆，计算维持保证金
SetFees(fees map[string]map[string]float64)
CalculateFee(symbol, odType, side string, amount float64, price float64, isMaker bool, params map[string]interface{}) (*Fee, *errs.Error)
SetLeverage(leverage float64, symbol string, params map[string]interface{}) (map[string]interface{}, *errs.Error)
CalcMaintMargin(symbol string, cost float64) (float64, *errs.Error)
Call(method string, params map[string]interface{}) (*HttpRes, *errs.Error)

// websocket相关：订阅订单簿、K线、标记价格、交易流、余额、仓位、账户配置
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

// websocket数据抓取、回放（用于回测）
SetDump(path string) *errs.Error
SetReplay(path string) *errs.Error
GetReplayTo() int64
ReplayOne() *errs.Error
ReplayAll() *errs.Error
SetOnWsChan(cb FuncOnWsChan)

// 精度处理
PrecAmount(m *Market, amount float64) (float64, *errs.Error)
PrecPrice(m *Market, price float64) (float64, *errs.Error)
PrecCost(m *Market, cost float64) (float64, *errs.Error)
PrecFee(m *Market, fee float64) (float64, *errs.Error)

// 其他
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

# 注意
### 使用`Options`而不是直接字段赋值来初始化交易所对象 
交易所对象被初始化时，一些如int的简单类型字段会有类型默认值，在`Init`方法进行设置时，无法区分当前值是用户设置的值还是默认值。
所以所有需要外部传入的配置应通过`Options`传入，然后在`Init`中读取`Options`并设置到对应字段上。

### 市场类型MarketType
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
    <td>现货</td>
    <td>保证金</td>
    <td>U本位永续</td>
    <td>U本位到期</td>
    <td>币本位永续</td>
    <td>币本位到期</td>
    <td>U本位期权</td>
    <td>币本位期权</td>
</tr>
</table>

### 常见参数命名调整
**`ccxt.defaultType` -> `MarketType`**  
当前交易所的默认市场类型。可在初始化时传入`OptMarketType`设置，也可随时设置交易所的`MarketType`属性。  
有效值：`MarketSpot/MarketMargin/MarketLinear/MarketInverse/MarketOption`  
ccxt中币安的defaultType命名和其他交易所不一致，banexg中进行了统一命名。  

**`ContractType`**  
当前交易所合约类型，可选值`swap`永续合约，`future`有到期日的合约。  
可在初始化时传入`OptContractType`设置，也可初始化后设置交易所的`ContractType`属性。  

# 联系我
邮箱：`anyongjin163@163.com`  
微信：`jingyingsuixing`  
