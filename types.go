package banexg

import (
	"net/http"
)

type FuncSign = func(api Entry, params *map[string]interface{}) *HttpReq
type FuncFetchCurr = func(params *map[string]interface{}) (CurrencyMap, error)
type FuncFetchMarkets = func(params *map[string]interface{}) (MarketMap, error)

type Exchange struct {
	ID        string   // 交易所ID
	Name      string   // 显示名称
	Countries []string // 可用国家
	Hosts     *ExgHosts
	Fees      *ExgFee
	Apis      map[string]Entry // 所有API的路径
	Has       map[string]int   // 是否定义了某个API
	Creds     *Credential
	Options   map[string]interface{} // 用户传入的配置

	EnableRateLimit int   // 是否启用请求速率控制:BoolNull/BoolTrue/BoolFalse
	RateLimit       int64 // 请求速率控制毫秒数，最小间隔单位
	lastRequestMS   int64 // 上次请求的13位时间戳

	UserAgent  string            // UserAgent of http request
	ReqHeaders map[string]string // http headers for request exchange

	MarketsWait chan interface{} // whether is loading markets
	Markets     MarketMap        //cache for all markets
	MarketsById MarketArrMap     // markets index by id
	CareMarkets []string         // markets to be fetch: spot/linear/inverse/option

	Symbols    []string
	IDs        []string
	TimeFrames map[string]string // map timeframe from common to specific

	CurrenciesById   CurrencyMap       // CurrencyMap index by id
	CurrenciesByCode CurrencyMap       // CurrencyMap index by code
	CurrCodeMap      map[string]string // common code maps

	TimeDelay  int64 // 系统时钟延迟的毫秒数
	HttpClient *http.Client

	PrecisionMode int
	PrecPadZero   bool   // padding zero for precision
	MarketType    string // MarketSpot/MarketMargin/MarketSwap/MarketFuture/MarketOption
	MarketInverse bool   // true: coin-based contract
	MarginMode    string // MarginCross/MarginIsolated
	TimeInForce   string // GTC/IOC/FOK

	// for calling sub struct func in parent struct
	Sign            FuncSign
	FetchCurrencies FuncFetchCurr
	FetchMarkets    FuncFetchMarkets
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
	Path   string
	Host   string
	Method string
	Cost   float64
	More   map[string]interface{}
}

type Credential struct {
	Keys     map[string]bool
	ApiKey   string
	Secret   string
	UID      string
	Password string
}

type HttpReq struct {
	Url     string
	Method  string
	Headers http.Header
	Body    string
	Error   error
}

type HttpRes struct {
	Status  int
	Headers http.Header
	Content string
	Error   error
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
	Type           string        `json:"type"`
	Spot           bool          `json:"spot"`
	Margin         bool          `json:"margin"`
	Swap           bool          `json:"swap"`
	Future         bool          `json:"future"`
	Option         bool          `json:"option"`
	Active         bool          `json:"active"`
	Contract       bool          `json:"contract"`
	Linear         bool          `json:"linear"`  // usd-based contract
	Inverse        bool          `json:"inverse"` // coin-based contract
	Taker          float64       `json:"taker"`
	Maker          float64       `json:"maker"`
	ContractSize   float64       `json:"contractSize"`
	Expiry         int64         `json:"expiry"`
	ExpiryDatetime string        `json:"expiryDatetime"`
	Strike         float64       `json:"strike"`
	OptionType     string        `json:"optionType"`
	Precision      *Precision    `json:"precision"`
	Limits         *MarketLimits `json:"limits"`
	Created        int64         `json:"created"`
	SubType        string        `json:"subType"`
	FeeSide        string        `json:"feeSide"` // get/give/base/quote/other
	Info           interface{}   `json:"info"`
}

type Precision struct {
	Amount int `json:"amount"`
	Price  int `json:"price"`
	Base   int `json:"base"`
	Quote  int `json:"quote"`
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

/*
**************************   Business Types   **************************
 */

type OhlcvArr = [6]float64

type Kline struct {
	Time   int64
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
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
	Info         interface{} `json:"info"`
	Amount       float64     `json:"amount"`
	Datetime     string      `json:"datetime"`
	ID           string      `json:"id"`
	Order        string      `json:"order"`
	Price        float64     `json:"price"`
	Timestamp    int64       `json:"timestamp"`
	Type         string      `json:"type"`
	Side         string      `json:"side"`
	Symbol       string      `json:"symbol"`
	TakerOrMaker string      `json:"takerOrMaker"`
	Cost         float64     `json:"cost"`
	Fee          *Fee        `json:"fee"`
}

type Fee struct {
	IsMaker  bool    `json:"isMaker"` // for calculate fee
	Currency string  `json:"currency"`
	Cost     float64 `json:"cost"`
	Rate     float64 `json:"rate,omitempty"`
}
