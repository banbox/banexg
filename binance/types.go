package binance

import (
	"github.com/anyongjin/banexg"
)

type Binance struct {
	*banexg.Exchange
	RecvWindow int
}

/*
*****************************   CurrencyMap   ***********************************
 */
type BnbCurrency struct {
	Coin              string        `json:"coin"`
	DepositAllEnable  bool          `json:"depositAllEnable"`
	Free              string        `json:"free"`
	Freeze            string        `json:"freeze"`
	Ipoable           string        `json:"ipoable"`
	Ipoing            string        `json:"ipoing"`
	IsLegalMoney      bool          `json:"isLegalMoney"`
	Locked            string        `json:"locked"`
	Name              string        `json:"name"`
	Storage           string        `json:"storage"`
	Trading           bool          `json:"trading"`
	WithdrawAllEnable bool          `json:"withdrawAllEnable"`
	Withdrawing       string        `json:"withdrawing"`
	NetworkList       []*BnbNetwork `json:"networkList"`
}
type BnbNetwork struct {
	AddressRegex            string `json:"addressRegex"`
	Coin                    string `json:"coin"`
	DepositDesc             string `json:"depositDesc"`
	DepositEnable           bool   `json:"depositEnable"`
	IsDefault               bool   `json:"isDefault"`
	MemoRegex               string `json:"memoRegex"`
	MinConfirm              int    `json:"minConfirm"`
	Name                    string `json:"name"`
	Network                 string `json:"network"`
	ResetAddressStatus      bool   `json:"resetAddressStatus"`
	SpecialTips             string `json:"specialTips"`
	UnLockConfirm           int    `json:"unLockConfirm"`
	WithdrawDesc            string `json:"withdrawDesc"`
	WithdrawEnable          bool   `json:"withdrawEnable"`
	WithdrawFee             string `json:"withdrawFee"`
	WithdrawIntegerMultiple string `json:"withdrawIntegerMultiple"`
	WithdrawMax             string `json:"withdrawMax"`
	WithdrawMin             string `json:"withdrawMin"`
	SameAddress             bool   `json:"sameAddress"`
	EstimatedArrivalTime    int    `json:"estimatedArrivalTime"`
	Busy                    bool   `json:"busy"`
}

/*
*****************************   MarketMap   ***********************************
 */

type BnbMarketRsp struct {
	Timezone        string       `json:"timezone"`
	ServerTime      int64        `json:"serverTime"`
	RateLimits      []*RateLimit `json:"rateLimits"`
	ExchangeFilters []BnbFilter  `json:"exchangeFilters"`
	Symbols         []*BnbMarket `json:"symbols"`
}
type RateLimit struct {
	RateLimitType string `json:"rateLimitType"`
	Interval      string `json:"interval"`
	IntervalNum   int    `json:"intervalNum"`
	Limit         int    `json:"limit"`
}
type BnbMarket struct {
	Symbol                          string      `json:"symbol"`
	Status                          string      `json:"status"`
	BaseAsset                       string      `json:"baseAsset"`
	BaseAssetPrecision              int         `json:"baseAssetPrecision"`
	QuoteAsset                      string      `json:"quoteAsset"`
	QuotePrecision                  int         `json:"quotePrecision"`
	QuoteAssetPrecision             int         `json:"quoteAssetPrecision"`
	BaseCommissionPrecision         int         `json:"baseCommissionPrecision"`
	QuoteCommissionPrecision        int         `json:"quoteCommissionPrecision"`
	OrderTypes                      []string    `json:"orderTypes"`
	IcebergAllowed                  bool        `json:"icebergAllowed"`
	OcoAllowed                      bool        `json:"ocoAllowed"`
	QuoteOrderQtyMarketAllowed      bool        `json:"quoteOrderQtyMarketAllowed"`
	AllowTrailingStop               bool        `json:"allowTrailingStop"`
	CancelReplaceAllowed            bool        `json:"cancelReplaceAllowed"`
	IsSpotTradingAllowed            bool        `json:"isSpotTradingAllowed"`
	IsMarginTradingAllowed          bool        `json:"isMarginTradingAllowed"`
	Filters                         []BnbFilter `json:"filters"`
	Permissions                     []string    `json:"permissions"`
	DefaultSelfTradePreventionMode  string      `json:"defaultSelfTradePreventionMode"`
	AllowedSelfTradePreventionModes []string    `json:"allowedSelfTradePreventionModes"`

	// 合约
	ContractType      string `json:"contractType"`
	DeliveryDate      int64  `json:"deliveryDate"`      //期货交割时间
	MarginAsset       string `json:"marginAsset"`       // 保证金资产
	QuantityPrecision int    `json:"quantityPrecision"` // U合约数量小数点位数
	PricePrecision    int    `json:"pricePrecision"`    // U合约价格小数点位数
	OnboardDate       int64  `json:"onboardDate"`       // 合约上线时间，币u合约都有

	ContractSize   int    `json:"contractSize"`   // 币合约数量
	ContractStatus string `json:"contractStatus"` // 币合约状态

	// 期权
	ExpiryDate    int64  `json:"expiryDate"`    // 期权到期时间
	Underlying    string `json:"underlying"`    // 期权合约底层资产
	StrikePrice   string `json:"strikePrice"`   // 期权行权价
	Unit          int    `json:"unit"`          // 期权合约单位，单一合约代表的底层资产数量
	Side          string `json:"side"`          // 期权方向
	QuantityScale int    `json:"quantityScale"` // 期权数量精读
	PriceScale    int    `json:"priceScale"`    // 期权价格精度
	MinQty        string `json:"minQty"`        // 期权最小下单数量
	MaxQty        string `json:"maxQty"`        // 期权最大下单数量
}

type BnbFilter = map[string]interface{}

/*
*****************************   Kline   ***********************************
 */
type BnbOptionKline struct {
	Open        string `json:"open"`        // 开盘价
	High        string `json:"high"`        // 最高价
	Low         string `json:"low"`         // 最低价
	Close       string `json:"close"`       // 收盘价(当前K线未结束的即为最新价)
	Volume      string `json:"volume"`      // 成交额
	Amount      string `json:"amount"`      // 成交量
	Interval    string `json:"interval"`    // 时间区间
	TradeCount  int    `json:"tradeCount"`  // 成交笔数
	TakerVolume string `json:"takerVolume"` // 主动买入成交额
	TakerAmount string `json:"takerAmount"` // 主动买入成交量
	OpenTime    int64  `json:"openTime"`    // 开盘时间
	CloseTime   int64  `json:"closeTime"`   // 收盘时间
}

/*
*****************************   Account   ***********************************
 */
type SpotAccount struct {
	MakerCommission            int               `json:"makerCommission"`
	TakerCommission            int               `json:"takerCommission"`
	BuyerCommission            int               `json:"buyerCommission"`
	SellerCommission           int               `json:"sellerCommission"`
	CommissionRates            map[string]string `json:"commissionRates"`
	CanTrade                   bool              `json:"canTrade"`
	CanWithdraw                bool              `json:"canWithdraw"`
	CanDeposit                 bool              `json:"canDeposit"`
	Brokered                   bool              `json:"brokered"`
	RequireSelfTradePrevention bool              `json:"requireSelfTradePrevention"`
	PreventSor                 bool              `json:"preventSor"`
	UpdateTime                 int64             `json:"updateTime"`
	AccountType                string            `json:"accountType"`
	Balances                   []*BnbAsset       `json:"balances"`
	Permissions                []string          `json:"permissions"`
	Uid                        int               `json:"uid"`
}

type BnbAsset struct {
	Asset    string `json:"asset"`
	Free     string `json:"free"`
	Locked   string `json:"locked"`
	Borrowed string `json:"borrowed"` // margin cross only
	Interest string `json:"interest"` // margin cross only
	NetAsset string `json:"netAsset"` // margin cross only
}

/*
MarginCrossBalances

	binance margin cross balance
*/
type MarginCrossBalances struct {
	BorrowEnabled              bool        `json:"borrowEnabled"`
	MarginLevel                string      `json:"marginLevel"`
	CollateralMarginLevel      string      `json:"CollateralMarginLevel"`
	TotalAssetOfBtc            string      `json:"totalAssetOfBtc"`
	TotalLiabilityOfBtc        string      `json:"totalLiabilityOfBtc"`
	TotalNetAssetOfBtc         string      `json:"totalNetAssetOfBtc"`
	TotalCollateralValueInUSDT string      `json:"TotalCollateralValueInUSDT"`
	TradeEnabled               bool        `json:"tradeEnabled"`
	TransferEnabled            bool        `json:"transferEnabled"`
	AccountType                string      `json:"accountType"`
	UserAssets                 []*BnbAsset `json:"userAssets"`
}

/*
IsolatedBalances
Binance Margin Isolated Balance
*/
type IsolatedBalances struct {
	Assets              []IsolatedAsset `json:"assets"`
	TotalAssetOfBtc     string          `json:"totalAssetOfBtc"`
	TotalLiabilityOfBtc string          `json:"totalLiabilityOfBtc"`
	TotalNetAssetOfBtc  string          `json:"totalNetAssetOfBtc"`
}
type IsolatedAsset struct {
	BaseAsset         *IsolatedCurrAsset `json:"baseAsset"`
	QuoteAsset        *IsolatedCurrAsset `json:"quoteAsset"`
	Symbol            string             `json:"symbol"`
	IsolatedCreated   bool               `json:"isolatedCreated"`
	Enabled           bool               `json:"enabled"`
	MarginLevel       string             `json:"marginLevel"`
	MarginLevelStatus string             `json:"marginLevelStatus"`
	MarginRatio       string             `json:"marginRatio"`
	IndexPrice        string             `json:"indexPrice"`
	LiquidatePrice    string             `json:"liquidatePrice"`
	LiquidateRate     string             `json:"liquidateRate"`
	TradeEnabled      bool               `json:"tradeEnabled"`
}
type IsolatedCurrAsset struct {
	BnbAsset
	BorrowEnabled bool   `json:"borrowEnabled"`
	NetAssetOfBtc string `json:"netAssetOfBtc"`
	RepayEnabled  bool   `json:"repayEnabled"`
	TotalAsset    string `json:"totalAsset"`
}

/*
SwapBalances 永续合约账户余额
*/
type SwapBalances struct {
	FeeTier                     int64           `json:"feeTier"`     // 手续费等级
	CanTrade                    bool            `json:"canTrade"`    // 是否可以交易
	CanDeposit                  bool            `json:"canDeposit"`  // 是否可以入金
	CanWithdraw                 bool            `json:"canWithdraw"` // 是否可以出金
	UpdateTime                  int64           `json:"updateTime"`  // 保留字段，请忽略
	MultiAssetsMargin           bool            `json:"multiAssetsMargin"`
	TradeGroupId                int64           `json:"tradeGroupId"`
	TotalInitialMargin          string          `json:"totalInitialMargin"`          // 当前所需起始保证金总额(存在逐仓请忽略), 仅计算usdt资产
	TotalMaintMargin            string          `json:"totalMaintMargin"`            // 维持保证金总额, 仅计算usdt资产
	TotalWalletBalance          string          `json:"totalWalletBalance"`          // 账户总余额, 仅计算usdt资产
	TotalUnrealizedProfit       string          `json:"totalUnrealizedProfit"`       // 持仓未实现盈亏总额, 仅计算usdt资产
	TotalMarginBalance          string          `json:"totalMarginBalance"`          // 保证金总余额, 仅计算usdt资产
	TotalPositionInitialMargin  string          `json:"totalPositionInitialMargin"`  // 持仓所需起始保证金(基于最新标记价格), 仅计算usdt资产
	TotalOpenOrderInitialMargin string          `json:"totalOpenOrderInitialMargin"` // 当前挂单所需起始保证金(基于最新标记价格), 仅计算usdt资产
	TotalCrossWalletBalance     string          `json:"totalCrossWalletBalance"`     // 全仓账户余额, 仅计算usdt资产
	TotalCrossUnPnl             string          `json:"totalCrossUnPnl"`             // 全仓持仓未实现盈亏总额, 仅计算usdt资产
	AvailableBalance            string          `json:"availableBalance"`            // 可用余额, 仅计算usdt资产
	MaxWithdrawAmount           string          `json:"maxWithdrawAmount"`           // 最大可转出余额, 仅计算usdt资产
	Assets                      []*SwapAsset    `json:"assets"`
	Positions                   []*SwapPosition `json:"positions"`
}
type SwapAsset struct {
	FutureAsset
	MarginAvailable bool `json:"marginAvailable"` // 是否可用作联合保证金
}

type FuturePosition struct {
	Symbol                 string `json:"symbol"`                 // 交易对
	InitialMargin          string `json:"initialMargin"`          // 当前所需起始保证金(基于最新标记价格)
	MaintMargin            string `json:"maintMargin"`            // 维持保证金
	UnrealizedProfit       string `json:"unrealizedProfit"`       // 持仓未实现盈亏
	PositionInitialMargin  string `json:"positionInitialMargin"`  // 持仓所需起始保证金(基于最新标记价格)
	OpenOrderInitialMargin string `json:"openOrderInitialMargin"` // 当前挂单所需起始保证金(基于最新标记价格)
	Leverage               string `json:"leverage"`               // 杠杆倍率
	Isolated               bool   `json:"isolated"`               // 是否是逐仓模式
	EntryPrice             string `json:"entryPrice"`             // 持仓成本价
	PositionSide           string `json:"positionSide"`           // 持仓方向
	PositionAmt            string `json:"positionAmt"`            // 持仓数量
	UpdateTime             int64  `json:"updateTime"`             // 更新时间
}
type SwapPosition struct {
	FuturePosition
	MaxNotional string `json:"maxNotional"` // 当前杠杆下用户可用的最大名义价值
	BidNotional string `json:"bidNotional"` // 买单净值，忽略
	AskNotional string `json:"askNotional"` // 卖单净值，忽略
}

/*
InverseBalances Coin-Based Balances
*/
type InverseBalances struct {
	Assets      []*FutureAsset     `json:"assets"`
	Positions   []*InversePosition `json:"positions"`
	CanDeposit  bool               `json:"canDeposit"`
	CanTrade    bool               `json:"canTrade"`
	CanWithdraw bool               `json:"canWithdraw"`
	FeeTier     int                `json:"feeTier"`
	UpdateTime  int64              `json:"updateTime"`
}

// 资产内容
type FutureAsset struct {
	Asset                  string `json:"asset"`                  // 资产名
	WalletBalance          string `json:"walletBalance"`          // 账户余额
	UnrealizedProfit       string `json:"unrealizedProfit"`       // 全部持仓未实现盈亏
	MarginBalance          string `json:"marginBalance"`          // 保证金余额
	MaintMargin            string `json:"maintMargin"`            // 维持保证金
	InitialMargin          string `json:"initialMargin"`          // 当前所需起始保证金(按最新标标记价格)
	PositionInitialMargin  string `json:"positionInitialMargin"`  // 当前所需持仓起始保证金(按最新标标记价格)
	OpenOrderInitialMargin string `json:"openOrderInitialMargin"` // 当前所需挂单起始保证金(按最新标标记价格)
	MaxWithdrawAmount      string `json:"maxWithdrawAmount"`      // 最大可提款金额
	CrossWalletBalance     string `json:"crossWalletBalance"`     // 可用于全仓的账户余额
	CrossUnPnl             string `json:"crossUnPnl"`             // 所有全仓持仓的未实现盈亏
	AvailableBalance       string `json:"availableBalance"`       // 可用下单余额
	UpdateTime             int64  `json:"updateTime"`             // 更新时间
}

// 头寸
type InversePosition struct {
	FuturePosition
	BreakEvenPrice string `json:"breakEvenPrice"` // 盈亏平衡价
	MaxQty         string `json:"maxQty"`         // 当前杠杆下最大可开仓数(标的数量)
}

/*
FundingAsset 资金账户余额
*/
type FundingAsset struct {
	Asset        string `json:"asset"`
	Free         string `json:"free"`         // 可用余额
	Locked       string `json:"locked"`       // 锁定资金
	Freeze       string `json:"freeze"`       // 冻结资金
	Withdrawing  string `json:"withdrawing"`  // 提币
	BtcValuation string `json:"btcValuation"` // btc估值
}

/*
*****************************   Private Orders   ***********************************
 */
type OrderBase struct {
	Symbol        string `json:"symbol"`
	Side          string `json:"side"`
	ClientOrderId string `json:"clientOrderId"`
	ExecutedQty   string `json:"executedQty"`
	UpdateTime    int64  `json:"updateTime"`
	Status        string `json:"status"`
	Type          string `json:"type"`
	OrderId       int    `json:"orderId"`
	Price         string `json:"price"`
	TimeInForce   string `json:"timeInForce"`
}

type SpotBase struct {
	OrderBase
	IcebergQty              string `json:"icebergQty"`
	Time                    int64  `json:"time"`
	SelfTradePreventionMode string `json:"selfTradePreventionMode"`
	CummulativeQuoteQty     string `json:"cummulativeQuoteQty"`
	IsWorking               bool   `json:"isWorking"`
	OrigQty                 string `json:"origQty"`
	StopPrice               string `json:"stopPrice"`
}

/*
SpotOrder 现货订单
*/
type SpotOrder struct {
	SpotBase
	OrderListId       int    `json:"orderListId"` // OCO订单ID，否则为 -1
	OrigQuoteOrderQty string `json:"origQuoteOrderQty"`
	WorkingTime       int64  `json:"workingTime"`
}

/*
MarginOrder 保证金杠杆订单
*/
type MarginOrder struct {
	SpotBase
	IsIsolated bool `json:"isIsolated"` // 是否是逐仓symbol交易
}

type FutBase struct {
	OrderBase
	ReduceOnly bool   `json:"reduceOnly"` // 是否仅减仓
	AvgPrice   string `json:"avgPrice"`   // 平均成交价
}

type FutureBase struct {
	FutBase
	Time          int64  `json:"time"`          // 订单时间
	OrigType      string `json:"origType"`      // 触发前订单类型
	ActivatePrice string `json:"activatePrice"` // 跟踪止损激活价格, 仅`TRAILING_STOP_MARKET` 订单返回此字段
	WorkingType   string `json:"workingType"`   // 条件价格触发类型
	ClosePosition bool   `json:"closePosition"` // 是否条件全平仓
	PositionSide  string `json:"positionSide"`  // 持仓方向
	OrigQty       string `json:"origQty"`       // 原始委托数量
	StopPrice     string `json:"stopPrice"`     // 触发价，对`TRAILING_STOP_MARKET`无效
	PriceRate     string `json:"priceRate"`     // 跟踪止损回调比例, 仅`TRAILING_STOP_MARKET` 订单返回此字段
}

/*
FutureOrder U本位合约订单
*/
type FutureOrder struct {
	FutureBase
	PriceProtect            bool   `json:"priceProtect"`            // 是否开启条件单触发保护
	GoodTillDate            int64  `json:"goodTillDate"`            //订单TIF为GTD时的自动取消时间
	SelfTradePreventionMode string `json:"selfTradePreventionMode"` //订单自成交保护模式
	CumQuote                string `json:"cumQuote"`                // 成交金额
	PriceMatch              string `json:"priceMatch"`              //盘口价格下单模式
}

/*
InverseOrder 币本位合约订单
*/
type InverseOrder struct {
	FutureBase
	Pair    string `json:"pair"`    // 标的交易对
	CumBase string `json:"cumBase"` // 成交金额(标的数量)
}

/*
OptionOrder 期权订单
*/
type OptionOrder struct {
	FutBase
	PostOnly      bool    `json:"postOnly"`      // 仅做maker
	PriceScale    int     `json:"priceScale"`    // 价格精度
	OptionSide    string  `json:"optionSide"`    // 期权类型
	QuoteAsset    string  `json:"quoteAsset"`    // 报价资产
	Quantity      float64 `json:"quantity"`      // 订单数量
	QuantityScale int     `json:"quantityScale"` // 数量精度
	Fee           float64 `json:"fee"`           // 手续费
	CreateTime    int64   `json:"createTime"`    // 订单创建时间
	Source        string  `json:"source"`        // 订单来源
	Mmp           bool    `json:"mmp"`           // 是否为MMP订单
}

type IBnbOrder interface {
	ToStdOrder(e *Binance) *banexg.Order
}
