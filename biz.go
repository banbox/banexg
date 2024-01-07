package banexg

import (
	"bytes"
	"context"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

func (e *Exchange) Init() {
	e.HttpClient = &http.Client{}
	proxyUrl := utils.GetMapVal(e.Options, OptProxy, "")
	if proxyUrl != "" {
		proxy, err := url.Parse(proxyUrl)
		if err != nil {
			panic(err)
		}
		e.Proxy = proxy
		e.HttpClient.Transport = &http.Transport{
			Proxy: http.ProxyURL(proxy),
		}
	}
	if e.Creds == nil {
		e.Creds = &Credential{}
	}
	utils.SetFieldBy(&e.Creds.ApiKey, e.Options, OptApiKey, "")
	utils.SetFieldBy(&e.Creds.Secret, e.Options, OptApiSecret, "")
	utils.SetFieldBy(&e.UserAgent, e.Options, OptUserAgent, "")
	if e.EnableRateLimit == BoolNull {
		e.EnableRateLimit = BoolTrue
	}
	e.ReqHeaders = DefReqHeaders
	reqHeaders := utils.GetMapVal(e.Options, OptReqHeaders, map[string]string{})
	for k, v := range reqHeaders {
		e.ReqHeaders[k] = v
	}
	e.WsIntvs = DefWsIntvs
	wsIntvs := utils.GetMapVal(e.Options, OptWsIntvs, map[string]int{})
	for k, v := range wsIntvs {
		e.WsIntvs[k] = v
	}
	e.Retries = DefRetries
	retries := utils.GetMapVal(e.Options, OptRetries, map[string]int{})
	for k, v := range retries {
		e.Retries[k] = v
	}
	utils.SetFieldBy(&e.CareMarkets, e.Options, OptCareMarkets, nil)
	utils.SetFieldBy(&e.PrecisionMode, e.Options, OptPrecisionMode, PrecModeDecimalPlace)
	utils.SetFieldBy(&e.MarketType, e.Options, OptMarketType, MarketSpot)
	utils.SetFieldBy(&e.ContractType, e.Options, OptContractType, "")
	utils.SetFieldBy(&e.TimeInForce, e.Options, OptTimeInForce, DefTimeInForce)
	e.CurrCodeMap = DefCurrCodeMap
	e.CurrenciesById = map[string]*Currency{}
	e.CurrenciesByCode = map[string]*Currency{}
	e.WSClients = map[string]*WsClient{}
	e.WsOutChans = map[string]interface{}{}
	e.WsChanRefs = map[string]map[string]struct{}{}
	e.OrderBooks = map[string]*OrderBook{}
	e.MarBalances = map[string]*Balances{}
	e.MarPositions = map[string][]*Position{}
	e.MarkPrices = map[string]map[string]float64{}
	e.KeyTimeStamps = map[string]int64{}
}

/*
***************************  Business Functions  ******************************
 */

func (e *Exchange) SafeCurrency(currId string) *Currency {
	if e.CurrenciesById != nil {
		curr, ok := e.CurrenciesById[currId]
		if ok {
			return curr
		}
	}
	code := currId
	if mapped, ok := e.CurrCodeMap[strings.ToUpper(currId)]; ok {
		code = mapped
	}
	return &Currency{
		ID:   currId,
		Code: code,
	}
}

func (e *Exchange) SafeCurrencyCode(currId string) string {
	return e.SafeCurrency(currId).Code
}

func doLoadMarkets(e *Exchange, params *map[string]interface{}) {
	var currencies CurrencyMap
	var err *errs.Error
	if e.HasApi("fetchCurrencies") {
		currencies, err = e.FetchCurrencies(params)
		if err != nil {
			e.MarketsWait <- err
			return
		}
	}
	markets, err := e.FetchMarkets(params)
	if err != nil {
		e.MarketsWait <- err
		return
	}
	// 现货的放在前面
	items := make([]*Market, 0, len(markets))
	for _, v := range markets {
		items = append(items, v)
	}
	sort.Slice(items, func(i, j int) bool {
		var iv, ij = 0, 0
		if items[i].Spot {
			iv = 1
		}
		if items[j].Spot {
			ij = 1
		}
		return iv > ij
	})
	// 更新Markets
	e.MarketsById = make(MarketArrMap)
	var symbols = make([]string, len(markets))
	var IDs = make([]string, 0, len(markets)/2)
	for i, item := range items {
		symbols[i] = item.Symbol
		if list, ok := e.MarketsById[item.ID]; ok {
			e.MarketsById[item.ID] = append(list, item)
		} else {
			e.MarketsById[item.ID] = []*Market{item}
			IDs = append(IDs, item.ID)
		}
	}
	e.Markets = markets
	sort.Strings(symbols)
	sort.Strings(IDs)
	e.Symbols = symbols
	e.IDs = IDs
	// 处理currencies
	if currencies == nil {
		var currs = make([]*Currency, 0)
		var defCurrPrecision = 1e-8
		if e.PrecisionMode == PrecModeDecimalPlace {
			defCurrPrecision = 8
		}
		for _, market := range markets {
			if market.Base != "" {
				curr := Currency{
					ID:        market.BaseID,
					Code:      market.Base,
					Precision: float64(market.Precision.Base),
				}
				if curr.ID == "" {
					curr.ID = market.Base
				}
				if curr.Precision == 0 {
					if market.Precision.Amount > 0 {
						curr.Precision = float64(market.Precision.Amount)
					} else {
						curr.Precision = defCurrPrecision
					}
				}
				currs = append(currs, &curr)
			}
			if market.Quote == "" {
				curr := Currency{
					ID:        market.QuoteID,
					Code:      market.Quote,
					Precision: float64(market.Precision.Quote),
				}
				if curr.ID == "" {
					curr.ID = market.Quote
				}
				if curr.Precision == 0 {
					if market.Precision.Price > 0 {
						curr.Precision = float64(market.Precision.Price)
					} else {
						curr.Precision = defCurrPrecision
					}
				}
				currs = append(currs, &curr)
			}
		}
		var highPrecs = make(map[string]*Currency)
		for _, curr := range currs {
			if old, ok := highPrecs[curr.Code]; ok {
				if e.PrecisionMode == PrecModeTickSize {
					if curr.Precision < old.Precision {
						highPrecs[curr.Code] = curr
					}
				} else if curr.Precision > old.Precision {
					highPrecs[curr.Code] = curr
				}
			} else {
				highPrecs[curr.Code] = curr
			}
		}
		for _, v := range highPrecs {
			if old, ok := e.CurrenciesByCode[v.Code]; ok {
				old.ID = v.ID
				old.Precision = v.Precision
			} else {
				e.CurrenciesByCode[v.Code] = v
			}
		}
	} else {
		e.CurrenciesByCode = currencies
	}
	for _, v := range e.CurrenciesByCode {
		e.CurrenciesById[v.ID] = v
	}
	e.MarketsWait <- markets
}

func (e *Exchange) LoadMarkets(reload bool, params *map[string]interface{}) (MarketMap, *errs.Error) {
	if reload || e.Markets == nil {
		if e.MarketsWait == nil {
			e.MarketsWait = make(chan interface{})
			go doLoadMarkets(e, params)
		}
		result := <-e.MarketsWait
		e.MarketsWait = nil
		if mars, ok := result.(MarketMap); ok && mars != nil {
			return mars, nil
		}
		if err, ok := result.(*errs.Error); ok && err != nil {
			return nil, err
		}
		return nil, errs.NewMsg(errs.CodeUnsupportMarket, "unknown markets type: %t", result)
	}
	return e.Markets, nil
}

func (e *Exchange) GetPriceOnePip(pair string) (float64, *errs.Error) {
	markets, err := e.LoadMarkets(false, nil)
	if err != nil {
		return 0, err
	}
	if mar, ok := markets[pair]; ok {
		precision := mar.Precision.Price
		if e.PrecisionMode == PrecModeTickSize {
			return float64(precision), nil
		} else {
			return 1 / math.Pow(10, float64(precision)), nil
		}
	}
	return 0, errs.NoMarketForPair
}

/*
GetMarket 获取市场信息

	symbol ccxt的symbol、交易所的ID，必须严格正确，如果可能错误，
	根据当前的MarketType和MarketInverse过滤匹配
*/
func (e *Exchange) GetMarket(symbol string) (*Market, *errs.Error) {
	if e.Markets == nil || len(e.Markets) == 0 {
		return nil, errs.MarketNotLoad
	}
	if mar, ok := e.Markets[symbol]; ok {
		if mar.Spot && e.IsContract("") {
			// 当前是合约模式，返回合约的Market
			settle := mar.Quote
			if e.MarketType == MarketInverse {
				settle = mar.Base
			}
			futureSymbol := symbol + ":" + settle
			if mar, ok = e.Markets[futureSymbol]; ok {
				return mar, nil
			}
			return nil, errs.NoMarketForPair
		}
		return mar, nil
	} else {
		market := e.GetMarketById(symbol, "")
		if market != nil {
			return market, nil
		}
	}
	return nil, errs.NoMarketForPair
}

/*
GetMarketID

	从CCXT的symbol得到交易所ID
*/
func (e *Exchange) GetMarketID(symbol string) (string, *errs.Error) {
	market, err := e.GetMarket(symbol)
	if err != nil {
		return "", err
	}
	return market.ID, nil
}

func (e *Exchange) GetMarketIDByArgs(args map[string]interface{}, required bool) (string, *errs.Error) {
	symbol := utils.PopMapVal(args, ParamSymbol, "")
	if symbol == "" {
		if required {
			return "", errs.NewMsg(errs.CodeParamRequired, "symbol required")
		}
		return "", nil
	}
	return e.GetMarketID(symbol)
}

func (e *Exchange) GetMarketById(marketId, marketType string) *Market {
	if e.MarketsById == nil {
		return nil
	}
	if mars, ok := e.MarketsById[marketId]; ok {
		if len(mars) == 1 {
			return mars[0]
		}
		if marketType == "" {
			marketType = e.MarketType
		}
		for _, mar := range mars {
			if mar.Type == marketType {
				return mar
			} else if mar.Margin && marketType == MarketMargin {
				mar2 := *mar
				mar2.Type = MarketMargin
				return &mar2
			}
		}
	}
	return nil
}

/*
SafeMarket

	从交易所品种ID转为规范化市场信息
*/
func (e *Exchange) SafeMarket(marketId, delimiter, marketType string) *Market {
	if e.MarketsById != nil {
		market := e.GetMarketById(marketId, marketType)
		if market != nil {
			return market
		}
	}
	result := &Market{
		Symbol: marketId,
	}
	if delimiter != "" {
		parts := strings.Split(marketId, delimiter)
		if len(parts) == 2 {
			result.BaseID = parts[0]
			result.QuoteID = parts[1]
			result.Base = e.SafeCurrencyCode(result.BaseID)
			result.Quote = e.SafeCurrencyCode(result.QuoteID)
			result.Symbol = result.Base + "/" + result.Quote
		}
	}
	return result
}

/*
SafeSymbol 将交易所品种ID转为规范化品种ID

marketType TradeSpot/TradeMargin/TradeSwap/TradeFuture/TradeOption

	linear/inverse
*/
func (e *Exchange) SafeSymbol(marketId, delimiter, marketType string) string {
	return e.SafeMarket(marketId, delimiter, marketType).Symbol
}

func (e *Exchange) FetchOhlcv(symbol, timeframe string, since int64, limit int, params *map[string]interface{}) ([]*Kline, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *Exchange) FetchBalance(params *map[string]interface{}) (*Balances, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *Exchange) FetchPositions(symbols []string, params *map[string]interface{}) ([]*Position, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *Exchange) FetchTicker(symbol string, params *map[string]interface{}) (*Ticker, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *Exchange) FetchTickers(symbols []string, params *map[string]interface{}) ([]*Ticker, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *Exchange) FetchOrders(symbol string, since int64, limit int, params *map[string]interface{}) ([]*Order, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *Exchange) FetchOpenOrders(symbol string, since int64, limit int, params *map[string]interface{}) ([]*Order, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *Exchange) FetchOrderBook(symbol string, limit int, params *map[string]interface{}) (*OrderBook, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *Exchange) CreateOrder(symbol, odType, side string, amount float64, price float64, params *map[string]interface{}) (*Order, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *Exchange) CancelOrder(id string, symbol string, params *map[string]interface{}) (*Order, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *Exchange) SetLeverage(leverage int, symbol string, params *map[string]interface{}) (map[string]interface{}, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *Exchange) LoadLeverageBrackets(reload bool, params *map[string]interface{}) *errs.Error {
	return errs.NotImplement
}

func (e *Exchange) CalculateFee(symbol, odType, side string, amount float64, price float64, isMaker bool,
	params *map[string]interface{}) (*Fee, *errs.Error) {
	if odType == OdTypeMarket && isMaker {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "maker only is invalid for market order")
	}
	market, err := e.GetMarket(symbol)
	if err != nil {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "get market fail: %v", err)
	}
	feeSide := market.FeeSide
	if feeSide == "" {
		if e.Fees != nil {
			if market.Spot || market.Margin {
				feeSide = e.Fees.Main.FeeSide
			} else if market.Linear {
				feeSide = e.Fees.Linear.FeeSide
			} else if market.Inverse {
				feeSide = e.Fees.Inverse.FeeSide
			}
		}
		if feeSide == "" {
			feeSide = "quote"
		}
	}
	useQuote := false
	if feeSide == "get" {
		useQuote = side == OdSideSell
	} else if feeSide == "give" {
		useQuote = side == OdSideBuy
	} else {
		useQuote = feeSide == "quote"
	}
	cost := decimal.NewFromFloat(amount)
	currency := ""
	if useQuote {
		cost = cost.Mul(decimal.NewFromFloat(price))
		currency = market.Quote
	} else {
		currency = market.Base
	}
	if !market.Spot {
		currency = market.Settle
	}
	feeRate := 0.0
	if isMaker {
		feeRate = market.Maker
	} else {
		feeRate = market.Taker
	}
	cost = cost.Mul(decimal.NewFromFloat(feeRate))
	costVal, _ := cost.Float64()
	return &Fee{
		isMaker, currency, costVal, feeRate,
	}, nil
}

/*
PriceOnePip
Get's the "1 pip" value for this pair.

	Used in PriceFilter to calculate the 1pip movements.
*/
func (e *Exchange) PriceOnePip(symbol string) (float64, *errs.Error) {
	market, err := e.GetMarket(symbol)
	if err != nil {
		return 0, err
	}
	prec := float64(market.Precision.Price)
	if e.PrecisionMode == PrecModeTickSize {
		return prec, nil
	}
	return 1 / math.Pow(10.0, prec), nil
}

/*
***************************  Common Functions  ******************************
 */

func (e *Exchange) IsContract(marketType string) bool {
	if marketType == "" {
		marketType = e.MarketType
	}
	return marketType == MarketFuture || marketType == MarketSwap ||
		marketType == MarketLinear || marketType == MarketInverse
}

func (e *Exchange) MilliSeconds() int64 {
	return time.Now().UnixMilli()
}

func (e *Exchange) Nonce() int64 {
	return time.Now().UnixMilli() - e.TimeDelay
}

func (e *Exchange) setReqHeaders(head *http.Header) {
	for k, v := range e.ReqHeaders {
		val := head.Get(k)
		if val == "" {
			head.Set(k, v)
		}
	}
	curUserAgent := head.Get("User-Agent")
	if curUserAgent == "" && e.UserAgent != "" {
		head.Set("User-Agent", e.UserAgent)
	}
}

func (e *Exchange) RequestApi(ctx context.Context, endpoint string, params *map[string]interface{}) *HttpRes {
	api, ok := e.Apis[endpoint]
	if !ok {
		log.Panic("invalid api", zap.String("endpoint", endpoint))
		return &HttpRes{Error: errs.ApiNotSupport}
	}
	if e.EnableRateLimit == BoolTrue {
		elapsed := e.MilliSeconds() - e.lastRequestMS
		sleepMS := int64(float64(e.RateLimit) * api.Cost)
		if elapsed < sleepMS {
			time.Sleep(time.Duration(sleepMS-elapsed) * time.Millisecond)
		}
		e.lastRequestMS = e.MilliSeconds()
	}
	sign := e.Sign(api, params)
	if sign.Error != nil {
		return &HttpRes{Error: sign.Error}
	}
	var req *http.Request
	var err error
	if sign.Body != "" {
		var body *bytes.Buffer
		body = bytes.NewBufferString(sign.Body)
		req, err = http.NewRequest(sign.Method, sign.Url, body)
	} else {
		req, err = http.NewRequest(sign.Method, sign.Url, nil)
	}
	if err != nil {
		return &HttpRes{Error: errs.New(errs.CodeInvalidRequest, err)}
	}
	req = req.WithContext(ctx)
	req.Header = sign.Headers
	e.setReqHeaders(&req.Header)

	log.Debug("request", zap.String(sign.Method, req.URL.String()),
		zap.Object("header", HttpHeader(req.Header)), zap.String("body", sign.Body))
	rsp, err := e.HttpClient.Do(req)
	if err != nil {
		return &HttpRes{Error: errs.New(errs.CodeNetFail, err)}
	}
	var result = HttpRes{Status: rsp.StatusCode, Headers: rsp.Header}
	rspData, err := io.ReadAll(rsp.Body)
	if err != nil {
		result.Error = errs.New(errs.CodeNetFail, err)
		return &result
	}
	result.Content = string(rspData)
	cutLen := min(len(result.Content), 3000)
	bodyShort := zap.String("body", result.Content[:cutLen])
	log.Debug("rsp", zap.Int("status", result.Status), zap.Object("method", HttpHeader(result.Headers)),
		zap.Int("len", len(result.Content)), bodyShort)
	if result.Status >= 400 {
		result.Error = errs.NewMsg(result.Status, result.Content)
	}
	defer func() {
		cerr := rsp.Body.Close()
		// Only overwrite the retured error if the original error was nil and an
		// error occurred while closing the body.
		if err == nil && cerr != nil {
			err = cerr
		}
	}()
	return &result
}

func (e *Exchange) RequestApiRetry(ctx context.Context, endpoint string, params *map[string]interface{}, retryNum int) *HttpRes {
	tryNum := retryNum + 1
	var rsp *HttpRes
	var sleep = 0
	for i := 0; i < tryNum; i++ {
		if sleep > 0 {
			time.Sleep(time.Second * time.Duration(sleep))
			sleep = 0
		}
		rsp = e.RequestApi(ctx, endpoint, params)
		if rsp.Error != nil {
			if rsp.Error.Code == errs.CodeNetFail {
				// 网络错误等待3s重试
				sleep = 3
				continue
			} else if e.GetRetryWait != nil {
				// 子交易所根据错误信息返回睡眠时间
				sleep = e.GetRetryWait(rsp.Error)
				if sleep >= 0 {
					continue
				}
			}
		}
		break
	}
	return rsp
}

func (e *Exchange) HasApi(key string) bool {
	val, ok := e.Has[key]
	if ok && val != HasFail {
		return true
	}
	return false
}

func (e *Exchange) GetTimeFrame(timeframe string) string {
	if e.TimeFrames != nil {
		if val, ok := e.TimeFrames[timeframe]; ok {
			return val
		}
	}
	return timeframe
}

func (e *Exchange) GetArgsMarketType(args map[string]interface{}, symbol string) (string, string) {
	marketType := utils.PopMapVal(args, "market", "")
	contractType := utils.GetMapVal(args, "contract", "")
	if marketType == "" {
		marketType = e.MarketType
		contractType = e.ContractType
		if symbol != "" {
			market, err := e.GetMarket(symbol)
			if err == nil {
				marketType = market.Type
			}
		}
	}
	return marketType, contractType
}

/*
GetArgsMarket
从symbol和args中的market+inverse得到对应的Market对象
*/
func (e *Exchange) GetArgsMarket(symbol string, args map[string]interface{}) (*Market, *errs.Error) {
	marketType := utils.PopMapVal(args, "market", "")
	contractType := utils.PopMapVal(args, "contract", "")
	backType, backContrType := "", ""
	if marketType != "" {
		backType, backContrType = e.MarketType, e.ContractType
		e.MarketType, e.ContractType = marketType, contractType
	}
	market, err := e.GetMarket(symbol)
	if marketType != "" {
		e.MarketType, e.ContractType = backType, backContrType
		if marketType == MarketMargin {
			//market.Type无法区分margin，这里复制并设置为margin
			market2 := *market
			market2.Type = MarketMargin
			return &market2, err
		}
	}
	return market, err
}

/*
LoadArgsMarket
LoadMarkets && GetArgsMarket
*/
func (e *Exchange) LoadArgsMarket(symbol string, params *map[string]interface{}) (map[string]interface{}, *Market, *errs.Error) {
	var args = utils.SafeParams(params)
	_, err := e.LoadMarkets(false, nil)
	if err != nil {
		return args, nil, err
	}
	market, err := e.GetArgsMarket(symbol, args)
	if err != nil {
		return args, nil, err
	}
	return args, market, err
}

func (e *Exchange) LoadArgsMarketType(args map[string]interface{}, symbols ...string) (string, string, *errs.Error) {
	firstSymbol := ""
	if len(symbols) > 0 {
		firstSymbol = symbols[0]
	}
	_, err := e.LoadMarkets(false, nil)
	if err != nil {
		return "", "", err
	}
	marketType, contractType := e.GetArgsMarketType(args, firstSymbol)
	return marketType, contractType, nil
}

func (e *Exchange) PrecAmount(m *Market, amount float64) (string, *errs.Error) {
	res, err := utils.PrecFloat64Str(amount, m.Precision.Amount, false)
	if err != nil {
		return "", errs.New(errs.CodePrecDecFail, err)
	}
	return res, nil
}

func (e *Exchange) precPriceCost(m *Market, value float64, round bool) (string, *errs.Error) {
	res, err := utils.PrecFloat64Str(value, m.Precision.Price, round)
	if err != nil {
		return "", errs.New(errs.CodePrecDecFail, err)
	}
	return res, nil
}

func (e *Exchange) PrecPrice(m *Market, price float64) (string, *errs.Error) {
	return e.precPriceCost(m, price, true)
}

func (e *Exchange) PrecCost(m *Market, cost float64) (string, *errs.Error) {
	return e.precPriceCost(m, cost, false)
}

func (e *Exchange) PrecFee(m *Market, fee float64) (string, *errs.Error) {
	return e.precPriceCost(m, fee, true)
}

/*
GetRetryNum
返回失败时重试次数，未设置时默认0
*/
func (e *Exchange) GetRetryNum(key string, defVal int) int {
	if retryNum, ok := e.Retries[key]; ok {
		return retryNum
	}
	return defVal
}
