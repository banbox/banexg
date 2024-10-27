package bybit

import (
	"github.com/banbox/banexg"
)

type Bybit struct {
	*banexg.Exchange
	RecvWindow int // 允许的和服务器最大毫秒时间差
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
	BasePrecision  string `json:"basePrecision"`
	QuotePrecision string `json:"quotePrecision"`
	MinOrderQty    string `json:"minOrderQty"`
	MaxOrderQty    string `json:"maxOrderQty"`
	MinOrderAmt    string `json:"minOrderAmt"`
	MaxOrderAmt    string `json:"maxOrderAmt"`
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
	ToStdTicker(e *Bybit, marketType string) *banexg.Ticker
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
