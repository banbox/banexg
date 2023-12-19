package banexg

import (
	"bytes"
	"context"
	"fmt"
	"github.com/anyongjin/banexg/log"
	"github.com/anyongjin/banexg/utils"
	"go.uber.org/zap"
	"io"
	"math"
	"net/http"
	"net/url"
	"path/filepath"
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
	utils.SetFieldBy(&e.CareMarkets, e.Options, OptCareMarkets, nil)
	utils.SetFieldBy(&e.PrecisionMode, e.Options, OptPrecisionMode, PrecModeDecimalPlace)
	utils.SetFieldBy(&e.MarketType, e.Options, OptMarketType, MarketSpot)
	utils.SetFieldBy(&e.MarketInverse, e.Options, OptMarketInverse, false)
	e.CurrCodeMap = DefCurrCodeMap
	e.CurrenciesById = map[string]*Currency{}
	e.CurrenciesByCode = map[string]*Currency{}
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
	var err error
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

func (e *Exchange) LoadMarkets(reload bool, params *map[string]interface{}) (MarketMap, error) {
	if reload || e.Markets == nil {
		if e.MarketsWait == nil {
			e.MarketsWait = make(chan interface{})
			go doLoadMarkets(e, params)
		}
		result := <-e.MarketsWait
		e.MarketsWait = nil
		if mars := result.(MarketMap); mars != nil {
			return mars, nil
		}
		if err := result.(error); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("unknown markets type: %t", result)
	}
	return e.Markets, nil
}

func (e *Exchange) PrecisionCost(symbol string, cost float64) float64 {
	return cost
}

func (e *Exchange) GetPriceOnePip(pair string) (float64, error) {
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
	return 0, ErrNoMarketForPair
}

/*
GetMarket

	获取市场信息
	symbol ccxt的symbol、交易所的ID
	根据当前的MarketType和MarketInverse过滤匹配
*/
func (e *Exchange) GetMarket(symbol string) (*Market, error) {
	if e.Markets == nil || len(e.Markets) == 0 {
		return nil, ErrMarketNotLoad
	}
	if mar, ok := e.Markets[symbol]; ok {
		if mar.Spot && e.IsSwapOrDelivery() {
			// 当前是合约模式，返回合约的Market
			settle := mar.Quote
			if e.MarketInverse {
				settle = mar.Base
			}
			futureSymbol := symbol + ":" + settle
			if mar, ok = e.Markets[futureSymbol]; ok {
				return mar, nil
			}
			return nil, ErrNoMarketForPair
		}
		return mar, nil
	} else if mars, ok := e.MarketsById[symbol]; ok {
		for _, mar := range mars {
			if mar.Type == e.MarketType && mar.Inverse == e.MarketInverse {
				return mar, nil
			}
		}
		if len(mars) > 0 {
			return mars[0], nil
		}
	}
	return nil, ErrNoMarketForPair
}

/*
GetMarketID

	从CCXT的symbol得到交易所ID
*/
func (e *Exchange) GetMarketID(symbol string) (string, error) {
	market, err := e.GetMarket(symbol)
	if err != nil {
		return "", err
	}
	return market.ID, nil
}

/*
SafeMarket

	从交易所品种ID转为规范化市场信息
*/
func (e *Exchange) SafeMarket(marketId, delimiter, marketType string) *Market {
	if e.MarketsById != nil {
		if mars, ok := e.MarketsById[marketId]; ok {
			if len(mars) == 1 {
				return mars[0]
			}
			if marketType == "" {
				marketType = e.MarketType
			}
			isLinear := marketType == MarketLinear
			isInverse := marketType == MarketInverse
			for _, mar := range mars {
				if mar.Type == marketType {
					return mar
				} else if isLinear && mar.Linear {
					return mar
				} else if isInverse && mar.Inverse {
					return mar
				}
			}
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

func (e *Exchange) FetchOhlcv(symbol, timeframe string, since int64, limit int, params *map[string]interface{}) ([]*Kline, error) {
	return nil, ErrNotImplement
}

func (e *Exchange) FetchBalance(params *map[string]interface{}) (*Balances, error) {
	return nil, ErrNotImplement
}

func (e *Exchange) FetchOrders(symbol string, since int64, limit int, params *map[string]interface{}) ([]*Order, error) {
	return nil, ErrNotImplement
}

/*
***************************  Common Functions  ******************************
 */

func (e *Exchange) IsSwapOrDelivery() bool {
	return e.MarketType == MarketFuture || e.MarketType == MarketSwap
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

var testCacheApis = map[string]bool{
	"dapiPublicGetExchangeInfo":  true,
	"fapiPublicGetExchangeInfo":  true,
	"publicGetExchangeInfo":      true,
	"sapiGetCapitalConfigGetall": true,
}

func (e *Exchange) RequestApi(ctx context.Context, endpoint string, params *map[string]interface{}) *HttpRes {
	api, ok := e.Apis[endpoint]
	if !ok {
		log.Panic("invalid api", zap.String("endpoint", endpoint))
		return &HttpRes{Error: ErrApiNotSupport}
	}
	if IsUnitTest && testCacheApis[endpoint] {
		path := filepath.Join("testdata", endpoint+".json")
		var res = HttpRes{}
		err := utils.ReadJsonFile(path, &res)
		if err == nil {
			return &res
		} else {
			log.Error("read test data fail", zap.String("path", path), zap.Error(err))
		}
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
		return &HttpRes{Error: err}
	}
	req = req.WithContext(ctx)
	req.Header = sign.Headers
	e.setReqHeaders(&req.Header)

	log.Debug("request", zap.String("req", req.URL.String()), zap.String("method", req.Method),
		zap.Object("header", HttpHeader(req.Header)))
	rsp, err := e.HttpClient.Do(req)
	if err != nil {
		return &HttpRes{Error: err}
	}
	var result = HttpRes{Status: rsp.StatusCode, Headers: rsp.Header}
	rspData, err := io.ReadAll(rsp.Body)
	if err != nil {
		result.Error = err
		return &result
	}
	result.Content = string(rspData)
	defer func() {
		cerr := rsp.Body.Close()
		// Only overwrite the retured error if the original error was nil and an
		// error occurred while closing the body.
		if err == nil && cerr != nil {
			err = cerr
		}
	}()
	if IsUnitTest && testCacheApis[endpoint] {
		path := filepath.Join("testdata", endpoint+".json")
		err := utils.WriteJsonFile(path, result)
		if err != nil {
			log.Error("write test data fail", zap.String("path", path), zap.Error(err))
		}
	}
	return &result
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

func (e *Exchange) GetArgsMarket(args map[string]interface{}) (string, bool) {
	marketType := utils.PopMapVal(args, "market", e.MarketType)
	marketInverse := utils.PopMapVal(args, "inverse", e.MarketInverse)
	return marketType, marketInverse
}
