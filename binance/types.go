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
