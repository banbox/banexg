package bybit

import (
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/sasha-s/go-deadlock"
)

type Bybit struct {
	*banexg.Exchange
	RecvWindow           int // 允许的和服务器最大毫秒时间差
	LeverageBrackets     map[string]*banexg.SymbolLvgBrackets
	LeverageBracketsLock deadlock.Mutex
	WsAuthLock           deadlock.Mutex
	WsAuthed             map[string]bool
	WsAuthDone           map[string]chan *errs.Error
	WsPendingRecons      map[string]*WsPendingRecon
}

// orderRef is shared by multiple response structs that carry both orderId/orderLinkId.
// Keep it unexported to avoid widening the public surface area of the bybit package.
type orderRef struct {
	OrderId     string `json:"orderId"`
	OrderLinkId string `json:"orderLinkId"`
}

/*
*****************************   CurrencyMap   ***********************************
 */

type Currency struct {
	Name         string   `json:"name"`
	Coin         string   `json:"coin"`
	RemainAmount string   `json:"remainAmount"`
	Chains       []*Chain `json:"chains"`
}

type Chain struct {
	Chain                 string `json:"chain"`
	ChainType             string `json:"chainType"`
	Confirmation          string `json:"confirmation"`
	WithdrawFee           string `json:"withdrawFee"`
	DepositMin            string `json:"depositMin"`
	WithdrawMin           string `json:"withdrawMin"`
	ChainDeposit          string `json:"chainDeposit"`
	ChainWithdraw         string `json:"chainWithdraw"`
	MinAccuracy           string `json:"minAccuracy"`
	WithdrawPercentageFee string `json:"withdrawPercentageFee"`
}

/*
*****************************   Markets   ***********************************
 */

type BaseMarket struct {
	Symbol    string `json:"symbol"`
	BaseCoin  string `json:"baseCoin"`
	QuoteCoin string `json:"quoteCoin"`
	Status    string `json:"status"`
}

type SpotMarket struct {
	BaseMarket
	Innovation     string      `json:"innovation"`
	MarginTrading  string      `json:"marginTrading"`
	LotSizeFilter  *LotSizeFt  `json:"lotSizeFilter"`
	PriceFilter    *PriceFt    `json:"priceFilter"`
	RiskParameters *RiskParams `json:"riskParameters"`
}

type LotSizeFt struct {
	BasePrecision         string `json:"basePrecision"`
	QuotePrecision        string `json:"quotePrecision"`
	MinOrderQty           string `json:"minOrderQty"`
	MaxOrderQty           string `json:"maxOrderQty"`
	MinOrderAmt           string `json:"minOrderAmt"`
	MaxOrderAmt           string `json:"maxOrderAmt"`
	MaxLimitOrderQty      string `json:"maxLimitOrderQty"`
	MaxMarketOrderQty     string `json:"maxMarketOrderQty"`
	PostOnlyMaxLimitOrder string `json:"postOnlyMaxLimitOrderSize"`
}

type PriceFt struct {
	MinPrice string `json:"minPrice"` // empty for spot
	MaxPrice string `json:"maxPrice"` // empty for spot
	TickSize string `json:"tickSize"`
}

type RiskParams struct {
	LimitParameter  string `json:"limitParameter"`
	MarketParameter string `json:"marketParameter"`
}

type ContractMarket struct {
	BaseMarket
	SettleCoin      string   `json:"settleCoin"`
	LaunchTime      string   `json:"launchTime"`
	DeliveryTime    string   `json:"deliveryTime"`
	DeliveryFeeRate string   `json:"deliveryFeeRate"`
	PriceFilter     *PriceFt `json:"priceFilter"`
}

type FutureMarket struct {
	ContractMarket
	ContractType       string           `json:"contractType"`
	PriceScale         string           `json:"priceScale"`
	LeverageFilter     *LeverageFt      `json:"leverageFilter"`
	LotSizeFilter      *FutureLotSizeFt `json:"lotSizeFilter"`
	UnifiedMarginTrade bool             `json:"unifiedMarginTrade"`
	FundingInterval    int              `json:"fundingInterval"`
	CopyTrading        string           `json:"copyTrading"`
	UpperFundingRate   string           `json:"upperFundingRate"`
	LowerFundingRate   string           `json:"lowerFundingRate"`
}

type LeverageFt struct {
	MinLeverage  string `json:"minLeverage"`
	MaxLeverage  string `json:"maxLeverage"`
	LeverageStep string `json:"leverageStep"`
}

type OptionLotSizeFt struct {
	MinOrderQty string `json:"minOrderQty"`
	MaxOrderQty string `json:"maxOrderQty"`
	QtyStep     string `json:"qtyStep"`
}

type FutureLotSizeFt struct {
	OptionLotSizeFt
	MaxMktOrderQty      string `json:"maxMktOrderQty"`
	PostOnlyMaxOrderQty string `json:"postOnlyMaxOrderQty"`
	MinNotionalValue    string `json:"minNotionalValue"`
}

type OptionMarket struct {
	ContractMarket
	OptionsType   string           `json:"optionsType"`
	LotSizeFilter *OptionLotSizeFt `json:"lotSizeFilter"`
}

/*
*****************************   Tickers   ***********************************
 */

type ITicker interface {
	ToStdTicker(e *Bybit, marketType string, info map[string]interface{}) *banexg.Ticker
}

type BaseTicker struct {
	Symbol       string `json:"symbol"`
	Bid1Price    string `json:"bid1Price"`
	Bid1Size     string `json:"bid1Size"`
	Ask1Price    string `json:"ask1Price"`
	Ask1Size     string `json:"ask1Size"`
	LastPrice    string `json:"lastPrice"`
	HighPrice24h string `json:"highPrice24h"`
	LowPrice24h  string `json:"lowPrice24h"`
	Turnover24h  string `json:"turnover24h"`
	Volume24h    string `json:"volume24h"`
}

type SpotTicker struct {
	BaseTicker
	PrevPrice24h  string `json:"prevPrice24h"`
	Price24hPcnt  string `json:"price24hPcnt"`
	UsdIndexPrice string `json:"usdIndexPrice"`
}

type ContractTicker struct {
	BaseTicker
	IndexPrice             string `json:"indexPrice"`
	PredictedDeliveryPrice string `json:"predictedDeliveryPrice"`
	MarkPrice              string `json:"markPrice"`
	OpenInterest           string `json:"openInterest"`
}

type OptionTicker struct {
	ContractTicker
	Bid1Iv          string `json:"bid1Iv"`
	Ask1Iv          string `json:"ask1Iv"`
	MarkIv          string `json:"markIv"`
	UnderlyingPrice string `json:"underlyingPrice"`
	TotalVolume     string `json:"totalVolume"`
	TotalTurnover   string `json:"totalTurnover"`
	Delta           string `json:"delta"`
	Gamma           string `json:"gamma"`
	Vega            string `json:"vega"`
	Theta           string `json:"theta"`
	Change24h       string `json:"change24h"`
}

type FutureTicker struct {
	ContractTicker
	PrevPrice24h      string `json:"prevPrice24h"`
	Price24hPcnt      string `json:"price24hPcnt"`
	PrevPrice1h       string `json:"prevPrice1h"`
	OpenInterestValue string `json:"openInterestValue"`
	FundingRate       string `json:"fundingRate"`
	NextFundingTime   string `json:"nextFundingTime"`
	BasisRate         string `json:"basisRate"`
	DeliveryFeeRate   string `json:"deliveryFeeRate"`
	DeliveryTime      string `json:"deliveryTime"`
	Basis             string `json:"basis"`
}

type FundRate struct {
	Symbol               string `json:"symbol"`
	FundingRate          string `json:"fundingRate"`
	FundingRateTimestamp string `json:"fundingRateTimestamp"`
}

/*
*****************************   Account / Position   ***********************************
 */

type WalletBalanceResult struct {
	List []map[string]interface{} `json:"list"`
}

type WalletBalance struct {
	AccountType            string              `json:"accountType"`
	AccountIMRate          string              `json:"accountIMRate"`
	AccountMMRate          string              `json:"accountMMRate"`
	TotalEquity            string              `json:"totalEquity"`
	TotalWalletBalance     string              `json:"totalWalletBalance"`
	TotalMarginBalance     string              `json:"totalMarginBalance"`
	TotalAvailableBalance  string              `json:"totalAvailableBalance"`
	TotalPerpUPL           string              `json:"totalPerpUPL"`
	TotalInitialMargin     string              `json:"totalInitialMargin"`
	TotalMaintenanceMargin string              `json:"totalMaintenanceMargin"`
	Coin                   []WalletBalanceCoin `json:"coin"`
}

type WalletBalanceCoin struct {
	Coin            string `json:"coin"`
	Equity          string `json:"equity"`
	WalletBalance   string `json:"walletBalance"`
	Locked          string `json:"locked"`
	BorrowAmount    string `json:"borrowAmount"`
	SpotBorrow      string `json:"spotBorrow"`
	TotalOrderIM    string `json:"totalOrderIM"`
	TotalPositionIM string `json:"totalPositionIM"`
	TotalPositionMM string `json:"totalPositionMM"`
	UnrealisedPnl   string `json:"unrealisedPnl"`
	Bonus           string `json:"bonus"`
	CumRealisedPnl  string `json:"cumRealisedPnl"`
}

type PositionInfo struct {
	PositionIdx    int    `json:"positionIdx"`
	Symbol         string `json:"symbol"`
	Side           string `json:"side"`
	Size           string `json:"size"`
	AvgPrice       string `json:"avgPrice"`
	MarkPrice      string `json:"markPrice"`
	PositionValue  string `json:"positionValue"`
	Leverage       string `json:"leverage"`
	PositionIM     string `json:"positionIM"`
	PositionMM     string `json:"positionMM"`
	UnrealisedPnl  string `json:"unrealisedPnl"`
	LiqPrice       string `json:"liqPrice"`
	TradeMode      int    `json:"tradeMode"`
	UpdatedTime    string `json:"updatedTime"`
	CreatedTime    string `json:"createdTime"`
	PositionStatus string `json:"positionStatus"`
}

type RiskLimitInfo struct {
	ID                int    `json:"id"`
	Symbol            string `json:"symbol"`
	RiskLimitValue    string `json:"riskLimitValue"`
	MaintenanceMargin string `json:"maintenanceMargin"`
	InitialMargin     string `json:"initialMargin"`
	IsLowestRisk      int    `json:"isLowestRisk"`
	MaxLeverage       string `json:"maxLeverage"`
	MmDeduction       string `json:"mmDeduction"`
}

/*
*****************************   Order / Trade   ***********************************
 */

type OrderResult struct {
	orderRef
}

type OrderInfo struct {
	orderRef
	Symbol        string `json:"symbol"`
	Side          string `json:"side"`
	OrderType     string `json:"orderType"`
	OrderStatus   string `json:"orderStatus"`
	TimeInForce   string `json:"timeInForce"`
	Price         string `json:"price"`
	Qty           string `json:"qty"`
	LeavesQty     string `json:"leavesQty"`
	CumExecQty    string `json:"cumExecQty"`
	CumExecValue  string `json:"cumExecValue"`
	CumExecFee    string `json:"cumExecFee"`
	AvgPrice      string `json:"avgPrice"`
	TriggerPrice  string `json:"triggerPrice"`
	TakeProfit    string `json:"takeProfit"`
	StopLoss      string `json:"stopLoss"`
	TpLimitPrice  string `json:"tpLimitPrice"`
	SlLimitPrice  string `json:"slLimitPrice"`
	StopOrderType string `json:"stopOrderType"`
	OrderIv       string `json:"orderIv"`
	MarketUnit    string `json:"marketUnit"`
	ReduceOnly    bool   `json:"reduceOnly"`
	PositionIdx   int    `json:"positionIdx"`
	CreatedTime   string `json:"createdTime"`
	UpdatedTime   string `json:"updatedTime"`
}

type ExecutionInfo struct {
	Symbol string `json:"symbol"`
	orderRef
	Side          string `json:"side"`
	OrderType     string `json:"orderType"`
	StopOrderType string `json:"stopOrderType"`
	OrderPrice    string `json:"orderPrice"`
	OrderQty      string `json:"orderQty"`
	LeavesQty     string `json:"leavesQty"`
	ExecId        string `json:"execId"`
	ExecPrice     string `json:"execPrice"`
	ExecQty       string `json:"execQty"`
	ExecValue     string `json:"execValue"`
	ExecFee       string `json:"execFee"`
	FeeCurrency   string `json:"feeCurrency"`
	FeeRate       string `json:"feeRate"`
	ExecType      string `json:"execType"`
	ExecTime      string `json:"execTime"`
	IsMaker       bool   `json:"isMaker"`
}

type TransLogInfo struct {
	ID              string `json:"id"`
	Symbol          string `json:"symbol"`
	Category        string `json:"category"`
	Side            string `json:"side"`
	TransactionTime string `json:"transactionTime"`
	Type            string `json:"type"`
	Qty             string `json:"qty"`
	Size            string `json:"size"`
	Currency        string `json:"currency"`
	TradePrice      string `json:"tradePrice"`
	Funding         string `json:"funding"`
	Fee             string `json:"fee"`
	CashFlow        string `json:"cashFlow"`
	Change          string `json:"change"`
	FeeRate         string `json:"feeRate"`
	TradeId         string `json:"tradeId"`
	orderRef
}
