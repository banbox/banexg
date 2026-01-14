package okx

import (
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/sasha-s/go-deadlock"
)

// WsPendingRecon stores info needed to restore subscriptions after reconnection login.
type WsPendingRecon struct {
	Client *banexg.WsClient
	ConnID int
	Keys   []string
}

type OKX struct {
	*banexg.Exchange
	RecvWindow int
	// LeverageBrackets caches leverage tiers by symbol or instFamily.
	LeverageBrackets     map[string]*banexg.SymbolLvgBrackets
	LeverageBracketsLock deadlock.Mutex
	// WsAuthDone tracks login completion channels per client key.
	// Each channel receives nil on success or an error on failure.
	WsAuthDone map[string]chan *errs.Error
	// WsAuthed tracks whether each client has successfully logged in.
	WsAuthed map[string]bool
	// WsPendingRecons stores pending reconnection info to restore subs after login.
	WsPendingRecons map[string]*WsPendingRecon
	WsAuthLock      deadlock.Mutex
}

// Instrument describes /public/instruments response item.
type Instrument struct {
	InstType          string   `json:"instType"`
	InstId            string   `json:"instId"`
	InstFamily        string   `json:"instFamily"`
	Uly               string   `json:"uly"`
	BaseCcy           string   `json:"baseCcy"`
	QuoteCcy          string   `json:"quoteCcy"`
	SettleCcy         string   `json:"settleCcy"`
	CtVal             string   `json:"ctVal"`
	CtMult            string   `json:"ctMult"`
	CtType            string   `json:"ctType"`
	Lever             string   `json:"lever"`
	TickSz            string   `json:"tickSz"`
	LotSz             string   `json:"lotSz"`
	MinSz             string   `json:"minSz"`
	State             string   `json:"state"`
	ListTime          string   `json:"listTime"`
	ExpTime           string   `json:"expTime"`
	TradeQuoteCcyList []string `json:"tradeQuoteCcyList"`
}

// Ticker describes /market/ticker(s) response item.
type Ticker struct {
	InstType  string `json:"instType"`
	InstId    string `json:"instId"`
	Last      string `json:"last"`
	LastSz    string `json:"lastSz"`
	AskPx     string `json:"askPx"`
	AskSz     string `json:"askSz"`
	BidPx     string `json:"bidPx"`
	BidSz     string `json:"bidSz"`
	Open24h   string `json:"open24h"`
	High24h   string `json:"high24h"`
	Low24h    string `json:"low24h"`
	VolCcy24h string `json:"volCcy24h"`
	Vol24h    string `json:"vol24h"`
	SodUtc0   string `json:"sodUtc0"`
	SodUtc8   string `json:"sodUtc8"`
	Ts        string `json:"ts"`
}

// OrderBook describes /market/books response item.
type OrderBook struct {
	Asks [][]string `json:"asks"`
	Bids [][]string `json:"bids"`
	Ts   string     `json:"ts"`
}

// Balance describes /account/balance response item.
type Balance struct {
	UTime   string          `json:"uTime"`
	TotalEq string          `json:"totalEq"`
	Details []BalanceDetail `json:"details"`
}

type BalanceDetail struct {
	Ccy       string `json:"ccy"`
	Eq        string `json:"eq"`
	CashBal   string `json:"cashBal"`
	AvailBal  string `json:"availBal"`
	FrozenBal string `json:"frozenBal"`
	AvailEq   string `json:"availEq"`
}

// Bill describes /account/bills response item.
type Bill struct {
	InstType string `json:"instType"`
	InstId   string `json:"instId"`
	BillId   string `json:"billId"`
	Type     string `json:"type"`
	SubType  string `json:"subType"`
	Ts       string `json:"ts"`
	BalChg   string `json:"balChg"`
	Pnl      string `json:"pnl"`
	Fee      string `json:"fee"`
	Ccy      string `json:"ccy"`
	TradeId  string `json:"tradeId"`
	Notes    string `json:"notes"`
}

// Position describes /account/positions response item.
type Position struct {
	InstType string `json:"instType"`
	InstId   string `json:"instId"`
	MgnMode  string `json:"mgnMode"`
	PosId    string `json:"posId"`
	PosSide  string `json:"posSide"`
	Pos      string `json:"pos"`
	AvgPx    string `json:"avgPx"`
	Upl      string `json:"upl"`
	Lever    string `json:"lever"`
	LiqPx    string `json:"liqPx"`
	MarkPx   string `json:"markPx"`
	Margin   string `json:"margin"`
	MgnRatio string `json:"mgnRatio"`
	Ccy      string `json:"ccy"`
	CTime    string `json:"cTime"`
	UTime    string `json:"uTime"`
}

// LeverageInfo describes /account/leverage-info response item.
type LeverageInfo struct {
	Ccy     string `json:"ccy"`
	InstId  string `json:"instId"`
	MgnMode string `json:"mgnMode"`
	PosSide string `json:"posSide"`
	Lever   string `json:"lever"`
}

// PositionTier describes /public/position-tiers response item.
type PositionTier struct {
	InstType     string `json:"instType"`
	InstId       string `json:"instId"`
	InstFamily   string `json:"instFamily"`
	Uly          string `json:"uly"`
	Tier         string `json:"tier"`
	MinSz        string `json:"minSz"`
	MaxSz        string `json:"maxSz"`
	Mmr          string `json:"mmr"`
	Imr          string `json:"imr"`
	MaxLever     string `json:"maxLever"`
	OptMgnFactor string `json:"optMgnFactor"`
	BaseMaxLoan  string `json:"baseMaxLoan"`
	QuoteMaxLoan string `json:"quoteMaxLoan"`
}

// Order describes /trade/order response item.
type Order struct {
	InstType  string `json:"instType"`
	InstId    string `json:"instId"`
	OrdId     string `json:"ordId"`
	ClOrdId   string `json:"clOrdId"`
	Px        string `json:"px"`
	Sz        string `json:"sz"`
	Side      string `json:"side"`
	PosSide   string `json:"posSide"`
	OrdType   string `json:"ordType"`
	TdMode    string `json:"tdMode"`
	State     string `json:"state"`
	AvgPx     string `json:"avgPx"`
	AccFillSz string `json:"accFillSz"`
	Fee       string `json:"fee"`
	FeeCcy    string `json:"feeCcy"`
	CTime     string `json:"cTime"`
	UTime     string `json:"uTime"`
}

// WsOrder describes websocket "orders" channel item.
type WsOrder struct {
	Order
	FillPx      string `json:"fillPx"`
	FillSz      string `json:"fillSz"`
	FillTime    string `json:"fillTime"`
	TradeId     string `json:"tradeId"`
	ReduceOnly  string `json:"reduceOnly"`
	FillFee     string `json:"fillFee"`
	FillFeeCcy  string `json:"fillFeeCcy"`
	ExecType    string `json:"execType"`
	AlgoClOrdId string `json:"algoClOrdId"` // Client-defined algo order ID when algo order triggers
	AlgoId      string `json:"algoId"`      // Algo order ID when algo order triggers
}

// OrderResult describes /trade/order or /trade/cancel-order result item.
type OrderResult struct {
	OrdId   string `json:"ordId"`
	ClOrdId string `json:"clOrdId"`
	ReqId   string `json:"reqId"`
	Ts      string `json:"ts"`
	SCode   string `json:"sCode"`
	SMsg    string `json:"sMsg"`
}

// WsAlgoOrder describes websocket "orders-algo" channel item.
type WsAlgoOrder struct {
	InstType    string `json:"instType"`
	InstId      string `json:"instId"`
	AlgoId      string `json:"algoId"`
	AlgoClOrdId string `json:"algoClOrdId"`
	ClOrdId     string `json:"clOrdId"`
	OrdId       string `json:"ordId"`
	Sz          string `json:"sz"`
	OrdType     string `json:"ordType"`
	Side        string `json:"side"`
	PosSide     string `json:"posSide"`
	TdMode      string `json:"tdMode"`
	State       string `json:"state"`
	Lever       string `json:"lever"`
	ActualSz    string `json:"actualSz"`
	ActualPx    string `json:"actualPx"`
	ActualSide  string `json:"actualSide"`
	TriggerPx   string `json:"triggerPx"`
	TriggerTime string `json:"triggerTime"`
	OrdPx       string `json:"ordPx"`
	TpTriggerPx string `json:"tpTriggerPx"`
	TpOrdPx     string `json:"tpOrdPx"`
	SlTriggerPx string `json:"slTriggerPx"`
	SlOrdPx     string `json:"slOrdPx"`
	ReduceOnly  string `json:"reduceOnly"`
	CTime       string `json:"cTime"`
	UTime       string `json:"uTime"`
}

// FundingRate describes /public/funding-rate response item.
type FundingRate struct {
	InstType        string `json:"instType"`
	InstId          string `json:"instId"`
	FundingRate     string `json:"fundingRate"`
	FundingTime     string `json:"fundingTime"`
	NextFundingRate string `json:"nextFundingRate"`
	NextFundingTime string `json:"nextFundingTime"`
	InterestRate    string `json:"interestRate"`
	Ts              string `json:"ts"`
}

// FundingRateHistory describes /public/funding-rate-history response item.
type FundingRateHistory struct {
	InstType     string `json:"instType"`
	InstId       string `json:"instId"`
	FundingRate  string `json:"fundingRate"`
	RealizedRate string `json:"realizedRate"`
	FundingTime  string `json:"fundingTime"`
	FormulaType  string `json:"formulaType"`
	Method       string `json:"method"`
}
