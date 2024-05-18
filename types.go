package banexg

import (
	"github.com/banbox/banexg/errs"
	"github.com/shopspring/decimal"
	"net/http"
	"net/url"
	"sync"
)

type FuncSign = func(api Entry, params map[string]interface{}) *HttpReq
type FuncFetchCurr = func(params map[string]interface{}) (CurrencyMap, *errs.Error)
type FuncFetchMarkets = func(marketTypes []string, params map[string]interface{}) (MarketMap, *errs.Error)
type FuncAuthWS = func(acc *Account, params map[string]interface{}) *errs.Error
type FuncCalcFee = func(market *Market, curr string, maker bool, amount, price decimal.Decimal, params map[string]interface{}) (*Fee, *errs.Error)

type FuncOnWsMsg = func(client *WsClient, msg *WsMsg)
type FuncOnWsMethod = func(client *WsClient, msg map[string]string, info *WsJobInfo)
type FuncOnWsErr = func(client *WsClient, err *errs.Error)
type FuncOnWsClose = func(client *WsClient, err *errs.Error)

type FuncGetWsJob = func(client *WsClient) (*WsJobInfo, *errs.Error)

type Exchange struct {
	*ExgInfo
	Hosts   *ExgHosts
	Fees    *ExgFee
	Apis    map[string]Entry          // 所有API的路径
	Has     map[string]map[string]int // 是否定义了某个API
	Options map[string]interface{}    // 用户传入的配置
	Proxy   *url.URL

	CredKeys   map[string]bool     // cred keys required for exchange
	Accounts   map[string]*Account // name: account
	DefAccName string              // default account name

	EnableRateLimit int        // 是否启用请求速率控制:BoolNull/BoolTrue/BoolFalse
	RateLimit       int64      // 请求速率控制毫秒数，最小间隔单位
	lastRequestMS   int64      // 上次请求的13位时间戳
	rateM           sync.Mutex // 同步锁

	MarketsWait chan interface{} // whether is loading markets
	CareMarkets []string         // markets to be fetch: spot/linear/inverse/option

	Symbols     []string
	IDs         []string
	TimeFrames  map[string]string // map timeframe from common to specific
	CurrCodeMap map[string]string // common code maps

	Retries map[string]int // retry nums for methods

	TimeDelay  int64 // 系统时钟延迟的毫秒数
	HttpClient *http.Client

	WSClients  map[string]*WsClient           // accName@url: websocket clients
	WsIntvs    map[string]int                 // milli secs interval for ws endpoints
	WsOutChans map[string]interface{}         // accName@url+msgHash: chan Type
	WsChanRefs map[string]map[string]struct{} // accName@url+msgHash: symbols use this chan

	KeyTimeStamps map[string]int64 // key: int64 更新的时间戳

	// for calling sub struct func in parent struct
	Sign            FuncSign
	FetchCurrencies FuncFetchCurr
	FetchMarkets    FuncFetchMarkets
	AuthWS          FuncAuthWS
	CalcFee         FuncCalcFee
	GetRetryWait    func(e *errs.Error) int // 根据错误信息计算重试间隔秒数，<0表示无需重试

	OnWsMsg   FuncOnWsMsg
	OnWsErr   FuncOnWsErr
	OnWsClose FuncOnWsClose

	Flags map[string]string
}

type ExgInfo struct {
	ID        string   // 交易所ID
	Name      string   // 显示名称
	Countries []string // 可用国家
	NoHoliday bool     // true表示365天全年开放
	FullDay   bool     // true表示一天24小时可交易
	Min1mHole int      // 1分钟K线空洞的最小间隔，少于此认为正常无交易而非空洞
	FixedLvg  bool     // 杠杆倍率是否固定不可修改

	DebugWS  bool // 是否输出WS调试信息
	DebugAPI bool // 是否输出API请求测试信息

	UserAgent  string            // UserAgent of http request
	ReqHeaders map[string]string // http headers for request exchange

	CurrenciesById   CurrencyMap                   // CurrencyMap index by id
	CurrenciesByCode CurrencyMap                   // CurrencyMap index by code
	Markets          MarketMap                     // cache for all markets
	MarketsById      MarketArrMap                  // markets index by id
	OrderBooks       map[string]*OrderBook         // symbol: OrderBook update by wss
	MarkPrices       map[string]map[string]float64 // marketType: symbol: mark price

	PrecPadZero  bool   // padding zero for precision
	MarketType   string // MarketSpot/MarketMargin/MarketLinear/MarketInverse/MarketOption
	ContractType string // MarketSwap/MarketFuture
	MarginMode   string // MarginCross/MarginIsolated
	TimeInForce  string // GTC/IOC/FOK
}

type Account struct {
	Name         string
	Creds        *Credential
	MarPositions map[string][]*Position // marketType: Position List
	MarBalances  map[string]*Balances   // marketType: Balances
	Leverages    map[string]int         // 币种当前的杠杆倍数
	Data         map[string]interface{}
	LockPos      *sync.Mutex
	LockBalance  *sync.Mutex
	LockLeverage *sync.Mutex
	LockData     *sync.Mutex
}

type ExgHosts struct {
	TestNet bool
	Logo    string
	Test    map[string]string
	Prod    map[string]string
	Www     string
	Doc     []string
	Fees    string
}

type ExgFee struct {
	Main    *TradeFee //默认
	Linear  *TradeFee //U本位合约
	Inverse *TradeFee // 币本位合约
}

type TradeFee struct {
	FeeSide    string
	TierBased  bool
	Percentage bool
	Taker      float64
	Maker      float64
	Tiers      *FeeTiers
}

type FeeTiers struct {
	Taker []*FeeTierItem
	Maker []*FeeTierItem
}

type FeeTierItem struct {
	Amount float64
	Rate   float64
}

type Entry struct {
	Path      string
	Host      string
	Method    string
	Cost      float64
	More      map[string]interface{}
	CacheSecs int
}

type Credential struct {
	ApiKey   string
	Secret   string
	UID      string
	Password string
}

type HttpReq struct {
	AccName string
	Url     string
	Method  string
	Headers http.Header
	Body    string
	Private bool // 此请求需要认证信息
	Error   *errs.Error
}

type HttpRes struct {
	AccName string      `json:"acc_name"`
	Status  int         `json:"status"`
	Headers http.Header `json:"headers"`
	Content string      `json:"content"`
	Error   *errs.Error
}

/*
**************************   Currency   **************************
 */
type CurrencyMap = map[string]*Currency

type Currency struct {
	ID        string
	Name      string
	Code      string
	Type      string
	NumericID int
	Precision float64
	PrecMode  int // 保留精度的模式：PrecModeDecimalPlace/PrecModeSignifDigits/PrecModeTickSize
	Active    bool
	Deposit   bool
	Withdraw  bool
	Networks  []*ChainNetwork
	Fee       float64
	Fees      map[string]float64
	Limits    *CodeLimits
	Info      interface{}
}

type ChainNetwork struct {
	ID        string
	Network   string
	Name      string
	Active    bool
	Fee       float64
	Precision float64
	Deposit   bool
	Withdraw  bool
	Limits    *CodeLimits
	Info      interface{}
}

type CodeLimits struct {
	Amount   *LimitRange
	Withdraw *LimitRange
	Deposit  *LimitRange
}

type LimitRange struct {
	Min float64
	Max float64
}

/*
**************************   Market   **************************
 */

type Market struct {
	ID             string        `json:"id"`
	LowercaseID    string        `json:"lowercaseId"`
	Symbol         string        `json:"symbol"`
	Base           string        `json:"base"`
	Quote          string        `json:"quote"`
	Settle         string        `json:"settle"`
	BaseID         string        `json:"baseId"`
	QuoteID        string        `json:"quoteId"`
	SettleID       string        `json:"settleId"`
	ExgReal        string        `json:"exgReal"`
	Type           string        `json:"type"`     // spot/linear/inverse/option 无法区分margin 和ccxt的值不同
	Combined       bool          `json:"combined"` // 是否是二次组合的数据
	Spot           bool          `json:"spot"`     // 现货市场
	Margin         bool          `json:"margin"`   // 保证金杠杆市场
	Swap           bool          `json:"swap"`     // 期货永续合约市场
	Future         bool          `json:"future"`   // 期货市场
	Option         bool          `json:"option"`   // 期权市场
	Active         bool          `json:"active"`   // 是否可交易
	Contract       bool          `json:"contract"` // 是否是合约
	Linear         bool          `json:"linear"`   // usd-based contract
	Inverse        bool          `json:"inverse"`  // coin-based contract
	Taker          float64       `json:"taker"`    // 吃单方费率
	Maker          float64       `json:"maker"`    // 挂单方费率
	ContractSize   float64       `json:"contractSize"`
	Expiry         int64         `json:"expiry"` // 过期的13毫秒数
	ExpiryDatetime string        `json:"expiryDatetime"`
	Strike         float64       `json:"strike"`
	OptionType     string        `json:"optionType"`
	DayTimes       [][2]int64    `json:"dayTimes"`   // 日盘交易时间
	NightTimes     [][2]int64    `json:"nightTimes"` // 夜盘交易时间
	Precision      *Precision    `json:"precision"`
	Limits         *MarketLimits `json:"limits"`
	Created        int64         `json:"created"`
	FeeSide        string        `json:"feeSide"` // get/give/base/quote/other
	Info           interface{}   `json:"info"`
}

type Precision struct {
	Amount     float64 `json:"amount"`
	Price      float64 `json:"price"`
	Base       float64 `json:"base"`
	Quote      float64 `json:"quote"`
	ModeAmount int     `json:"modeAmount"` // PrecModeTickSize/PrecModeSignifDigits/PrecModeDecimalPlace
	ModePrice  int     `json:"modePrice"`
	ModeBase   int     `json:"modeBase"`
	ModeQuote  int     `json:"modeQuote"`
}

type MarketLimits struct {
	Leverage *LimitRange `json:"leverage"`
	Amount   *LimitRange `json:"amount"`
	Price    *LimitRange `json:"price"`
	Cost     *LimitRange `json:"cost"`
	Market   *LimitRange `json:"market"`
}

type MarketMap = map[string]*Market

type MarketArrMap = map[string][]*Market

type Ticker struct {
	Symbol        string      `json:"symbol"`
	TimeStamp     int64       `json:"timestamp"`
	Bid           float64     `json:"bid"`
	BidVolume     float64     `json:"bidVolume"`
	Ask           float64     `json:"ask"`
	AskVolume     float64     `json:"askVolume"`
	High          float64     `json:"high"`
	Low           float64     `json:"low"`
	Open          float64     `json:"open"`
	Close         float64     `json:"close"`
	Last          float64     `json:"last"`
	Change        float64     `json:"change"`
	Percentage    float64     `json:"percentage"`
	Average       float64     `json:"average"`
	Vwap          float64     `json:"vwap"`
	BaseVolume    float64     `json:"baseVolume"`
	QuoteVolume   float64     `json:"quoteVolume"`
	PreviousClose float64     `json:"previousClose"`
	Info          interface{} `json:"info"`
}

/*
**************************   Business Types   **************************
 */

type OHLCVArr = [6]float64

type Kline struct {
	Time   int64
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
	Info   float64
}

type PairTFKline struct {
	Kline
	Symbol    string
	TimeFrame string
}

type Balances struct {
	TimeStamp      int64
	Free           map[string]float64
	Used           map[string]float64
	Total          map[string]float64
	Assets         map[string]*Asset
	IsolatedAssets map[string]map[string]*Asset // 逐仓账户资产，键是symbol
	Info           interface{}
}

type Asset struct {
	Code  string
	Free  float64
	Used  float64
	Total float64
	Debt  float64
	UPol  float64
}

type Position struct {
	ID               string      `json:"id"`
	Symbol           string      `json:"symbol"`
	TimeStamp        int64       `json:"timestamp"`
	Isolated         bool        `json:"isolated"`                    // 隔离
	Hedged           bool        `json:"hedged"`                      // 对冲
	Side             string      `json:"side"`                        // long or short
	Contracts        float64     `json:"contracts"`                   // 合约数量
	ContractSize     float64     `json:"contractSize"`                // 单份合约价值
	EntryPrice       float64     `json:"entryPrice"`                  // 入场价格
	MarkPrice        float64     `json:"markPrice"`                   // 标记价格
	Notional         float64     `json:"notional"`                    // 名义价值
	Leverage         int         `json:"leverage"`                    // 杠杆倍数
	Collateral       float64     `json:"collateral"`                  // 当前保证金：初始保证金+未实现盈亏
	InitialMargin    float64     `json:"initialMargin"`               // 初始保证金额
	MaintMargin      float64     `json:"maintenanceMargin"`           // 维持保证金额
	InitialMarginPct float64     `json:"initialMarginPercentage"`     // 初始保证金率
	MaintMarginPct   float64     `json:"maintenanceMarginPercentage"` // 维持保证金率
	UnrealizedPnl    float64     `json:"unrealizedPnl"`               // 未实现盈亏
	LiquidationPrice float64     `json:"liquidationPrice"`            // 清算价格
	MarginMode       string      `json:"marginMode"`                  // cross/isolated
	MarginRatio      float64     `json:"marginRatio"`
	Percentage       float64     `json:"percentage"` // 未实现盈亏百分比
	Info             interface{} `json:"info"`
}

type Order struct {
	Info                interface{} `json:"info"`
	ID                  string      `json:"id"`
	ClientOrderID       string      `json:"clientOrderId"`
	Datetime            string      `json:"datetime"`
	Timestamp           int64       `json:"timestamp"`
	LastTradeTimestamp  int64       `json:"lastTradeTimestamp"`
	LastUpdateTimestamp int64       `json:"lastUpdateTimestamp"`
	Status              string      `json:"status"`
	Symbol              string      `json:"symbol"`
	Type                string      `json:"type"`
	TimeInForce         string      `json:"timeInForce"`
	PositionSide        string      `json:"positionSide"`
	Side                string      `json:"side"`
	Price               float64     `json:"price"`
	Average             float64     `json:"average"`
	Amount              float64     `json:"amount"`
	Filled              float64     `json:"filled"`
	Remaining           float64     `json:"remaining"`
	TriggerPrice        float64     `json:"triggerPrice"`
	StopPrice           float64     `json:"stopPrice"`
	TakeProfitPrice     float64     `json:"takeProfitPrice"`
	StopLossPrice       float64     `json:"stopLossPrice"`
	Cost                float64     `json:"cost"`
	PostOnly            bool        `json:"postOnly"`
	ReduceOnly          bool        `json:"reduceOnly"`
	Trades              []*Trade    `json:"trades"`
	Fee                 *Fee        `json:"fee"`
}

type Trade struct {
	ID        string      `json:"id"`        // 交易ID
	Symbol    string      `json:"symbol"`    // 币种ID
	Side      string      `json:"side"`      // buy/sell
	Type      string      `json:"type"`      // market/limit
	Amount    float64     `json:"amount"`    // 当前交易的数量
	Price     float64     `json:"price"`     // 价格
	Cost      float64     `json:"cost"`      // 当前交易花费
	Order     string      `json:"order"`     // 当前交易所属订单号
	Timestamp int64       `json:"timestamp"` // 时间戳
	Maker     bool        `json:"maker"`     // 是否maker
	Fee       *Fee        `json:"fee"`       // 手续费
	Info      interface{} `json:"info"`
}

type MyTrade struct {
	Trade
	Filled     float64     `json:"filled"`     // 订单累计成交量（不止当前交易）
	ClientID   string      `json:"clientID"`   // 客户端订单ID
	Average    float64     `json:"average"`    // 平均成交价格
	State      string      `json:"state"`      // 状态
	PosSide    string      `json:"posSide"`    // 持仓方向 long/short
	ReduceOnly bool        `json:"reduceOnly"` // 是否是只减仓单
	Info       interface{} `json:"info"`
}

type Fee struct {
	IsMaker  bool    `json:"isMaker"` // for calculate fee
	Currency string  `json:"currency"`
	Cost     float64 `json:"cost"`
	Rate     float64 `json:"rate,omitempty"`
}

type OrderBook struct {
	Symbol    string         `json:"symbol"`
	TimeStamp int64          `json:"timestamp"`
	Asks      *OrderBookSide `json:"asks"`
	Bids      *OrderBookSide `json:"bids"`
	Nonce     int64          // latest update id
	Cache     []map[string]string
}

/*
OrderBookSide
订单簿一侧。不需要加锁，因为只有一个goroutine可以修改
*/
type OrderBookSide struct {
	IsBuy bool
	Rows  [][2]float64 // [][price, size]
	Index []float64
	Depth int
}

/*
**************************   WebSockets   **************************
 */

/*
WsJobInfo
调用websocket api时暂存的任务信息。用于返回结果时处理。
*/
type WsJobInfo struct {
	ID      string
	MsgHash string
	Name    string
	Symbols []string
	Method  func(client *WsClient, msg map[string]string, info *WsJobInfo)
	Limit   int
	Params  map[string]interface{}
}

/*
WsMsg
表示websocket收到的消息
*/
type WsMsg struct {
	Event   string
	ID      string
	IsArray bool
	Text    string
	Object  map[string]string
	List    []map[string]string
}

type AccountConfig struct {
	Symbol   string
	Leverage int
}
