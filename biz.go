package banexg

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"maps"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"github.com/banbox/bntp"
	"github.com/sasha-s/go-deadlock"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

func (e *Exchange) Init() *errs.Error {
	e.HttpClient = &http.Client{}
	// 代理解析：优先传入Proxy，其次环境变量HTTP(S)_PROXY，最后系统代理配置
	proxyUrl := utils.GetMapVal(e.Options, OptProxy, "")
	if proxyUrl == "" {
		proxyUrl = utils.GetSystemEnvProxy()
		if proxyUrl == "" {
			prx, err := utils.GetSystemProxy()
			if err != nil {
				log.Error("GetSystemProxy fail, skip", zap.Error(err))
			} else if prx != nil {
				proxyUrl = fmt.Sprintf("%s://%s:%s", prx.Protocol, prx.Host, prx.Port)
			}
		}
	} else if proxyUrl == "no" {
		proxyUrl = ""
	}
	if proxyUrl != "" {
		proxy, err := url.Parse(proxyUrl)
		if err != nil {
			return errs.New(errs.CodeParamInvalid, err)
		}
		e.Proxy = http.ProxyURL(proxy)
		e.HttpClient.Transport = &http.Transport{
			Proxy: e.Proxy,
		}
	}
	e.parseOptCreds()
	utils.SetFieldBy(&e.UserAgent, e.Options, OptUserAgent, "")
	if e.EnableRateLimit == BoolNull {
		e.EnableRateLimit = BoolTrue
	}
	e.CalcRateLimiterCost = makeCalcRateLimiterCost(e)
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
	// 更新api缓存时间
	apiCaches := utils.GetMapVal(e.Options, OptApiCaches, map[string]int{})
	var failCaches []string
	for k, v := range apiCaches {
		if api, ok := e.Apis[k]; ok {
			api.CacheSecs = v
		} else {
			failCaches = append(failCaches, k)
		}
	}
	if len(failCaches) > 0 {
		log.Error("invalid api keys for OptApiCaches", zap.Strings("keys", failCaches))
	}
	err := e.SetDump(utils.GetMapVal(e.Options, OptDumpPath, ""))
	if err != nil {
		return err
	}
	err = e.SetReplay(utils.GetMapVal(e.Options, OptReplayPath, ""))
	if err != nil {
		return err
	}
	apiEnv := utils.GetMapVal(e.Options, OptEnv, "")
	if apiEnv == "test" {
		e.Hosts.TestNet = true
	}
	// 更新手续费比率
	fees := utils.GetMapVal(e.Options, OptFees, map[string]map[string]float64{})
	e.SetFees(fees)
	utils.SetFieldBy(&e.CareMarkets, e.Options, OptCareMarkets, nil)
	utils.SetFieldBy(&e.MarketType, e.Options, OptMarketType, MarketSpot)
	utils.SetFieldBy(&e.ContractType, e.Options, OptContractType, "")
	utils.SetFieldBy(&e.TimeInForce, e.Options, OptTimeInForce, DefTimeInForce)
	utils.SetFieldBy(&e.DebugWS, e.Options, OptDebugWs, false)
	utils.SetFieldBy(&e.DebugAPI, e.Options, OptDebugApi, false)
	utils.SetFieldBy(&e.WsBatchSize, e.Options, OptDumpBatchSize, 1000)
	utils.SetFieldBy(&e.WsTimeout, e.Options, OptWsTimeout, 15000)
	e.CurrByCodeLock.Lock()
	e.CurrByIdLock.Lock()
	e.CurrCodeMap = DefCurrCodeMap
	e.CurrenciesById = map[string]*Currency{}
	e.CurrenciesByCode = map[string]*Currency{}
	e.WSClients = map[string]*WsClient{}
	e.WsOutChans = map[string]interface{}{}
	e.WsChanRefs = map[string]map[string]struct{}{}
	e.OrderBooks = map[string]*OrderBook{}
	e.MarkPrices = map[string]map[string]float64{}
	e.KeyTimeStamps = map[string]int64{}
	e.ExgInfo.Min1mHole = 1
	e.CurrByIdLock.Unlock()
	e.CurrByCodeLock.Unlock()
	return nil
}

/****************************  Business Functions  *******************************/

func (e *Exchange) SafeCurrency(currId string) *Currency {
	if currId == "" {
		return &Currency{}
	}
	if e.CurrenciesById != nil {
		e.CurrByIdLock.Lock()
		curr, ok := e.CurrenciesById[currId]
		e.CurrByIdLock.Unlock()
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

func (e *Exchange) SetFees(fees map[string]map[string]float64) {
	if len(fees) == 0 {
		return
	}
	if e.Fees == nil {
		e.Fees = &ExgFee{}
	}
	for market, feeMap := range fees {
		var target *TradeFee
		if market == "linear" {
			if e.Fees.Linear == nil {
				e.Fees.Linear = &TradeFee{}
			}
			target = e.Fees.Linear
		} else if market == "inverse" {
			if e.Fees.Inverse == nil {
				e.Fees.Inverse = &TradeFee{}
			}
			target = e.Fees.Inverse
		} else {
			if e.Fees.Main == nil {
				e.Fees.Main = &TradeFee{}
			}
			target = e.Fees.Main
		}
		for field, rate := range feeMap {
			field = strings.ToLower(field)
			if field == "taker" {
				target.Taker = rate
			} else if field == "maker" {
				target.Maker = rate
			} else {
				log.Warn("unknown fee Field, expect: maker/taker", zap.String("val", field))
			}
		}
	}
}

func (e *Exchange) SafeCurrencyCode(currId string) string {
	return e.SafeCurrency(currId).Code
}

func doLoadMarkets(e *Exchange, params map[string]interface{}) {
	var markets MarketMap
	var currencies CurrencyMap
	var err *errs.Error
	markets, currencies = e.getMarketsCache(true)
	if markets == nil {
		markets, currencies, err = e.fetchMarketsCurrs(params)
		if err != nil {
			e.MarketsWait <- err
			return
		}
	}
	// 更新Markets
	e.setMarkets(markets)
	// 更新currencies
	e.setCurrencies(currencies, markets)
	e.MarketsWait <- markets
}

func (e *Exchange) fetchMarketsCurrs(params map[string]interface{}) (MarketMap, CurrencyMap, *errs.Error) {
	var markets MarketMap
	var currencies CurrencyMap
	var err *errs.Error
	// 设置写入锁
	marketsLock.Lock()
	defer marketsLock.Unlock()
	ts, ok := exgMarketTS[e.Name]
	delayMins := int((e.MilliSeconds() - ts) / 60000)
	if ok && delayMins == 0 {
		// 并发写入，其他线程已写入
		markets, currencies = e.getMarketsCache(false)
		if markets != nil {
			return markets, currencies, nil
		}
	}
	if e.HasApi(ApiFetchCurrencies, "") {
		currencies, err = e.FetchCurrencies(params)
		if err != nil {
			return nil, nil, err
		}
	}
	cares := e.getAllCareMarkets()
	markets, err = e.FetchMarkets(cares, params)
	if err != nil {
		return nil, nil, err
	}
	// 写入到缓存
	exgMarketTS[e.Name] = e.MilliSeconds()
	exgCacheCurrs[e.Name] = currencies
	exgCacheMarkets[e.Name] = markets
	exgCareMarkets[e.Name] = cares
	return markets, currencies, nil
}

func (e *Exchange) setMarkets(markets MarketMap) {
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
	marketsById := make(MarketArrMap)
	var symbols = make([]string, len(markets))
	var IDs = make([]string, 0, len(markets)/2)
	for i, item := range items {
		symbols[i] = item.Symbol
		if list, ok := marketsById[item.ID]; ok {
			marketsById[item.ID] = append(list, item)
		} else {
			marketsById[item.ID] = []*Market{item}
			IDs = append(IDs, item.ID)
		}
	}
	sort.Strings(symbols)
	sort.Strings(IDs)
	// 按固定顺序获取锁，防止死锁
	e.MarketsLock.Lock()
	e.MarketsByIdLock.Lock()
	e.Markets = markets
	e.MarketsById = marketsById
	e.Symbols = symbols
	e.IDs = IDs
	e.MarketsByIdLock.Unlock()
	e.MarketsLock.Unlock()
}

func (e *Exchange) setCurrencies(currencies CurrencyMap, markets MarketMap) {
	var currByCode CurrencyMap
	var currById CurrencyMap
	if currencies == nil {
		var currs = make([]*Currency, 0)
		var defCurrPrec, defCurrPrecMode = float64(8), PrecModeDecimalPlace
		for _, market := range markets {
			if market.Base != "" {
				curr := Currency{
					ID:        market.BaseID,
					Code:      market.Base,
					Precision: market.Precision.Base,
					PrecMode:  market.Precision.ModeBase,
				}
				if curr.ID == "" {
					curr.ID = market.Base
				}
				if curr.Precision == 0 {
					if market.Precision.Amount > 0 {
						curr.Precision = market.Precision.Amount
						curr.PrecMode = market.Precision.ModeAmount
					} else {
						curr.Precision = defCurrPrec
						curr.PrecMode = defCurrPrecMode
					}
				}
				currs = append(currs, &curr)
			}
			if market.Quote == "" {
				curr := Currency{
					ID:        market.QuoteID,
					Code:      market.Quote,
					Precision: market.Precision.Quote,
					PrecMode:  market.Precision.ModeQuote,
				}
				if curr.ID == "" {
					curr.ID = market.Quote
				}
				if curr.Precision == 0 {
					if market.Precision.Price > 0 {
						curr.Precision = market.Precision.Price
						curr.PrecMode = market.Precision.ModePrice
					} else {
						curr.Precision = defCurrPrec
						curr.PrecMode = defCurrPrecMode
					}
				}
				currs = append(currs, &curr)
			}
		}
		var highPrecs = make(map[string]*Currency)
		for _, curr := range currs {
			if old, ok := highPrecs[curr.Code]; ok {
				if old.PrecMode == PrecModeTickSize {
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
		// 先复制现有的 CurrenciesByCode
		e.CurrByCodeLock.Lock()
		currByCode = make(CurrencyMap, len(e.CurrenciesByCode)+len(highPrecs))
		for k, v := range e.CurrenciesByCode {
			currByCode[k] = v
		}
		e.CurrByCodeLock.Unlock()
		for _, v := range highPrecs {
			if old, ok := currByCode[v.Code]; ok {
				old.ID = v.ID
				old.Precision = v.Precision
				old.PrecMode = v.PrecMode
			} else {
				currByCode[v.Code] = v
			}
		}
	} else {
		currByCode = currencies
	}
	currById = make(CurrencyMap, len(currByCode))
	for _, v := range currByCode {
		currById[v.ID] = v
	}
	// 按固定顺序获取锁，防止死锁
	e.CurrByCodeLock.Lock()
	e.CurrByIdLock.Lock()
	e.CurrenciesByCode = currByCode
	e.CurrenciesById = currById
	e.CurrByIdLock.Unlock()
	e.CurrByCodeLock.Unlock()
}

func (e *Exchange) getMarketsCache(lock bool) (MarketMap, CurrencyMap) {
	// 检查时间戳未过期
	if lock {
		marketsLock.RLock()
		defer marketsLock.RUnlock()
	}
	ts, ok := exgMarketTS[e.Name]
	delayMins := int((e.MilliSeconds() - ts) / 60000)
	if !ok || delayMins < exgMarketExpireMins {
		return nil, nil
	}
	// 检查是否缓存了所有所需的市场类型
	if len(e.CareMarkets) < len(e.getAllCareMarkets()) {
		return nil, nil
	}
	// 检查有缓存结果
	markets, ok := exgCacheMarkets[e.Name]
	if !ok || len(markets) == 0 {
		return nil, nil
	}
	currs, ok := exgCacheCurrs[e.Name]
	if !ok || len(currs) == 0 {
		return nil, nil
	}
	return markets, currs
}

/*
将当前交易所的CareMarkets和全局所有交易所的ExgCareMarkets合并返回。
用于刷新全部市场信息
*/
func (e *Exchange) getAllCareMarkets() []string {
	careArr, ok := exgCareMarkets[e.Name]
	if !ok || len(careArr) == 0 {
		return e.CareMarkets
	}
	cares := map[string]struct{}{}
	for _, mar := range careArr {
		cares[mar] = struct{}{}
	}
	for _, mar := range e.CareMarkets {
		cares[mar] = struct{}{}
	}
	var res = make([]string, 0, len(cares))
	for key := range cares {
		res = append(res, key)
	}
	return res
}

func (e *Exchange) LoadMarkets(reload bool, params map[string]interface{}) (MarketMap, *errs.Error) {
	if reload || e.Markets == nil {
		if e.MarketsWait == nil {
			e.MarketsWait = make(chan interface{})
			log.Debug("try load markets", zap.Int64("stamp", e.MilliSeconds()))
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
		if mar.Precision.ModePrice == PrecModeTickSize {
			return precision, nil
		} else {
			return 1 / math.Pow(10, precision), nil
		}
	}
	return 0, errs.NewMsg(errs.CodeNoMarketForPair, "no market found for pair")
}

func (e *Exchange) GetCurMarkets() MarketMap {
	result := make(MarketMap)
	fltFut := e.IsContract(e.MarketType)
	isSwap := e.ContractType == MarketSwap
	e.MarketsLock.Lock()
	for key, mar := range e.Markets {
		if !mar.Active || mar.Type != e.MarketType {
			continue
		}
		if fltFut && isSwap && !mar.Swap {
			continue
		}
		result[key] = mar
	}
	e.MarketsLock.Unlock()
	return result
}

func (e *Exchange) GetMarketBy(symbol string) (*Market, bool) {
	var mar *Market
	var ok bool
	e.MarketsLock.Lock()
	if len(e.Markets) > 0 {
		mar, ok = e.Markets[symbol]
	}
	e.MarketsLock.Unlock()
	return mar, ok
}

/*
GetMarket 获取市场信息

	symbol ccxt的symbol、交易所的ID，必须严格正确，如果可能错误，
	根据当前的MarketType和MarketInverse过滤匹配
*/
func (e *Exchange) GetMarket(symbol string) (*Market, *errs.Error) {
	if e.Markets == nil {
		return nil, errs.NewMsg(errs.CodeMarketNotLoad, "markets not loaded")
	}
	if mar, ok := e.GetMarketBy(symbol); ok {
		if mar.Spot && e.IsContract("") {
			// 当前是合约模式，返回合约的Market
			settle := mar.Quote
			if e.MarketType == MarketInverse {
				settle = mar.Base
			}
			futureSymbol := symbol + ":" + settle
			if mar, ok = e.GetMarketBy(futureSymbol); ok {
				return mar, nil
			}
			return nil, errs.NewMsg(errs.CodeNoMarketForPair, "no market found: %v - %v - %v",
				e.ExgInfo.Name, e.ExgInfo.MarketType, symbol)
		}
		return mar, nil
	} else {
		market := e.GetMarketById(symbol, "")
		if market != nil {
			return market, nil
		}
	}
	return nil, errs.NewMsg(errs.CodeNoMarketForPair, "no market found: %v - %v - %v",
		e.ExgInfo.Name, e.ExgInfo.MarketType, symbol)
}

func (e *Exchange) MapMarket(exgSID string, year int) (*Market, *errs.Error) {
	mar := e.GetMarketById(exgSID, "")
	if mar == nil {
		return nil, errs.NewMsg(errs.CodeNoMarketForPair, "no market found: %v - %v - %v",
			e.ExgInfo.Name, e.ExgInfo.MarketType, exgSID)
	}
	return mar, nil
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

/*
GetMarketById get market by exchange id (Upper Required!)
*/
func (e *Exchange) GetMarketById(marketId, marketType string) *Market {
	if e.MarketsById == nil {
		return nil
	}
	e.MarketsByIdLock.Lock()
	mars, ok := e.MarketsById[marketId]
	e.MarketsByIdLock.Unlock()
	if ok {
		// 这里不能判断有一个时直接返回，有可能市场不一致：bybit现货市场信息不含SEC，但现货tickers含SEC
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
	// GetMarketById 内部已有锁保护
	market := e.GetMarketById(marketId, marketType)
	if market != nil {
		return market
	}
	result := &Market{
		ID: marketId,
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

/*
CheckSymbols
split valid and invalid symbols
*/
func (e *Exchange) CheckSymbols(symbols ...string) ([]string, []string) {
	var items = make(map[string]struct{})
	for _, symbol := range symbols {
		items[symbol] = struct{}{}
	}
	e.MarketsLock.Lock()
	var valids = make([]string, 0, len(symbols))
	var fails = make([]string, 0, len(symbols)/5)
	for symbol := range items {
		if _, ok := e.Markets[symbol]; ok {
			valids = append(valids, symbol)
		} else {
			fails = append(fails, symbol)
		}
	}
	e.MarketsLock.Unlock()
	return valids, fails
}

func (e *Exchange) Info() *ExgInfo {
	return e.ExgInfo
}

func (e *Exchange) FetchOHLCV(symbol, timeframe string, since int64, limit int, params map[string]interface{}) ([]*Kline, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) FetchBalance(params map[string]interface{}) (*Balances, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) FetchPositions(symbols []string, params map[string]interface{}) ([]*Position, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) FetchAccountPositions(symbols []string, params map[string]interface{}) ([]*Position, *errs.Error) {
	return nil, nil
}

func (e *Exchange) FetchTicker(symbol string, params map[string]interface{}) (*Ticker, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) FetchTickers(symbols []string, params map[string]interface{}) ([]*Ticker, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) FetchTickerPrice(symbol string, params map[string]interface{}) (map[string]float64, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) FetchOrder(symbol, orderId string, params map[string]interface{}) (*Order, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) FetchOrders(symbol string, since int64, limit int, params map[string]interface{}) ([]*Order, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) FetchOpenOrders(symbol string, since int64, limit int, params map[string]interface{}) ([]*Order, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) FetchIncomeHistory(inType string, symbol string, since int64, limit int, params map[string]interface{}) ([]*Income, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) FetchFundingRate(symbol string, params map[string]interface{}) (*FundingRateCur, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) FetchFundingRates(symbols []string, params map[string]interface{}) ([]*FundingRateCur, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) FetchFundingRateHistory(symbol string, since int64, limit int, params map[string]interface{}) ([]*FundingRate, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) FetchLastPrices(symbols []string, params map[string]interface{}) ([]*LastPrice, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) FetchOrderBook(symbol string, limit int, params map[string]interface{}) (*OrderBook, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) CreateOrder(symbol, odType, side string, amount float64, price float64, params map[string]interface{}) (*Order, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) EditOrder(symbol, orderId, side string, amount, price float64, params map[string]interface{}) (*Order, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) CancelOrder(id string, symbol string, params map[string]interface{}) (*Order, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) SetLeverage(leverage float64, symbol string, params map[string]interface{}) (map[string]interface{}, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) LoadLeverageBrackets(reload bool, params map[string]interface{}) *errs.Error {
	return errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) GetLeverage(symbol string, notional float64, account string) (float64, float64) {
	return 0, 0
}

func (e *Exchange) CalcMaintMargin(symbol string, cost float64) (float64, *errs.Error) {
	return 0, nil
}

func (e *Exchange) InitLeverageBrackets() *errs.Error {
	return nil
}

func (e *Exchange) Call(method string, params map[string]interface{}) (*HttpRes, *errs.Error) {
	params = utils.SafeParams(params)
	tryNum := utils.PopMapVal(params, ParamRetry, -1)
	if tryNum < 0 {
		tryNum = e.GetRetryNum(method, 1)
	}
	rsp := e.RequestApiRetry(context.Background(), method, params, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	return rsp, nil
}

func (e *Exchange) WatchOrderBooks(symbols []string, limit int, params map[string]interface{}) (chan *OrderBook, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) UnWatchOrderBooks(symbols []string, params map[string]interface{}) *errs.Error {
	return errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) WatchOHLCVs(jobs [][2]string, params map[string]interface{}) (chan *PairTFKline, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) UnWatchOHLCVs(jobs [][2]string, params map[string]interface{}) *errs.Error {
	return errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) WatchMarkPrices(symbols []string, params map[string]interface{}) (chan map[string]float64, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) UnWatchMarkPrices(symbols []string, params map[string]interface{}) *errs.Error {
	return errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) WatchTrades(symbols []string, params map[string]interface{}) (chan *Trade, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) UnWatchTrades(symbols []string, params map[string]interface{}) *errs.Error {
	return errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) WatchMyTrades(params map[string]interface{}) (chan *MyTrade, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) WatchBalance(params map[string]interface{}) (chan *Balances, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) WatchPositions(params map[string]interface{}) (chan []*Position, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) WatchAccountConfig(params map[string]interface{}) (chan *AccountConfig, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *Exchange) CloseWsFile() {
	if e.WsFile != nil {
		err_ := e.WsFile.Close()
		if err_ != nil {
			log.Error("close ws file fail", zap.Error(err_))
		}
		e.WsFile = nil
	}
}

func (e *Exchange) SetDump(path string) *errs.Error {
	if path == "" {
		if e.WsEncoder != nil {
			if len(e.WsCache) > 0 {
				err_ := e.WsEncoder.Encode(e.WsCache)
				if err_ != nil {
					log.Error("dump ws cache fail", zap.Error(err_))
				}
			}
			e.WsEncoder = nil
			err_ := e.WsWriter.Close()
			if err_ != nil {
				log.Error("write ws cache fail", zap.Error(err_))
			}
			e.WsWriter = nil
			e.CloseWsFile()
		}
		return nil
	}
	if e.WsDecoder != nil {
		return errs.NewMsg(errs.CodeRunTime, "cannot dump in replay mode")
	}
	// dump websocket messages for replay later
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return errs.New(errs.CodeIOWriteFail, err)
	}
	e.WsFile = file
	e.WsCache = nil
	e.WsWriter = gzip.NewWriter(file)
	e.WsEncoder = gob.NewEncoder(e.WsWriter)
	return nil
}

func (e *Exchange) SetReplay(path string) *errs.Error {
	if path == "" {
		if e.WsDecoder != nil {
			e.WsDecoder = nil
			err := e.WsReader.Close()
			if err != nil {
				return errs.New(errs.CodeIOReadFail, err)
			}
			e.WsReader = nil
			e.CloseWsFile()
		}
		return nil
	}
	// replay ws message with dumped file
	if e.WsEncoder != nil {
		return errs.NewMsg(errs.CodeRunTime, "cannot set replay in dump mode !")
	}
	file, err := os.Open(path)
	if err != nil {
		return errs.New(errs.CodeIOReadFail, err)
	}
	e.WsFile = file
	e.WsCache = nil
	e.WsReader, err = gzip.NewReader(file)
	if err != nil {
		return errs.New(errs.CodeIOReadFail, err)
	}
	e.WsDecoder = gob.NewDecoder(e.WsReader)
	return nil
}

func (e *Exchange) DumpWS(name string, data interface{}) {
	if e.WsEncoder == nil || data == nil {
		return
	}
	dataStr, err_ := utils.MarshalString(data)
	if err_ != nil {
		log.Warn("marshal data fail for DumpWs", zap.String("name", name), zap.Error(err_))
		return
	}
	item := &WsLog{
		Name:    name,
		TimeMS:  bntp.UTCStamp(),
		Content: dataStr,
	}
	e.WsCache = append(e.WsCache, item)
	if len(e.WsCache) > e.WsBatchSize {
		rows := e.WsCache
		e.WsCache = nil
		go func() {
			e.wsCacheLock.Lock()
			defer e.wsCacheLock.Unlock()
			err := e.WsEncoder.Encode(rows)
			if err != nil {
				log.Error("dump ws cache fail", zap.Error(err))
			}
		}()
	}
}

func (e *Exchange) GetReplayTo() int64 {
	if e.WsNextMS == 0 {
		if len(e.WsCache) == 0 {
			e.WsCache = make([]*WsLog, 0, e.WsBatchSize)
			if err := e.WsDecoder.Decode(&e.WsCache); err != nil {
				e.WsNextMS = math.MaxInt64
				return e.WsNextMS
			}
		}
		if len(e.WsCache) > 0 {
			e.WsNextMS = e.WsCache[0].TimeMS
		} else {
			e.WsNextMS = math.MaxInt64
		}
	}
	return e.WsNextMS
}

func (e *Exchange) ReplayOne() *errs.Error {
	if e.WsNextMS == math.MaxInt64 {
		return nil
	}
	item := e.WsCache[0]
	e.WsCache = e.WsCache[1:]
	e.WsNextMS = 0
	e.WsReplayTo = item.TimeMS
	handle, ok := e.WsReplayFn[item.Name]
	if !ok {
		log.Warn("no ws replay handle found", zap.String("for", item.Name), zap.String("exg", e.Name))
		return nil
	}
	return handle(item)
}

func (e *Exchange) ReplayAll() *errs.Error {
	if e.WsFile == nil {
		return errs.NewMsg(errs.CodeRunTime, "Replay not initialized")
	}
	var counts = make(map[string]int)
	for {
		var bads = make(map[string]bool)
		for _, item := range e.WsCache {
			oldNum, _ := counts[item.Name]
			counts[item.Name] = oldNum + 1
			e.WsReplayTo = item.TimeMS
			handle, ok := e.WsReplayFn[item.Name]
			if !ok {
				bads[item.Name] = true
				continue
			}
			err := handle(item)
			if err != nil {
				return err
			}
		}
		if len(bads) > 0 {
			fails := utils.KeysOfMap(bads)
			log.Warn("no ws replay handle found", zap.Strings("for", fails), zap.String("exg", e.Name))
		}
		e.WsCache = make([]*WsLog, 0, e.WsBatchSize)
		if err_ := e.WsDecoder.Decode(&e.WsCache); err_ != nil {
			// read done
			break
		}
	}
	log.Debug("replay counts", zap.Any("r", counts))
	return nil
}

func (e *Exchange) SetOnWsChan(cb FuncOnWsChan) {
	e.OnWsChan = cb
}

func (e *Exchange) CalculateFee(symbol, odType, side string, amount float64, price float64, isMaker bool,
	params map[string]interface{}) (*Fee, *errs.Error) {
	if odType == OdTypeMarket && isMaker {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "maker only is invalid for market order")
	}
	market, err := e.GetMarket(symbol)
	if err != nil {
		return nil, errs.NewFull(errs.CodeParamInvalid, err, "get market fail")
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
	amountDc := decimal.NewFromFloat(amount)
	cost := amountDc
	priceDc := decimal.NewFromFloat(price)
	costQuote := amountDc.Mul(priceDc)
	currency := ""
	if useQuote {
		cost = costQuote
		currency = market.Quote
	} else {
		currency = market.Base
	}
	if !market.Spot {
		currency = market.Settle
	}
	if e.CalcFee != nil {
		return e.CalcFee(market, currency, isMaker, amountDc, priceDc, params)
	}
	feeRate := 0.0
	if isMaker {
		feeRate = market.Maker
	} else {
		feeRate = market.Taker
	}
	cost = cost.Mul(decimal.NewFromFloat(feeRate))
	costQuote = costQuote.Mul(decimal.NewFromFloat(feeRate))
	costVal, _ := cost.Float64()
	costQuoteVal, _ := costQuote.Float64()
	return &Fee{
		isMaker, currency, costVal, costQuoteVal, feeRate,
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
	prec := market.Precision.Price
	if market.Precision.ModePrice == PrecModeTickSize {
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
	if e.WsDecoder != nil {
		return e.WsReplayTo
	}
	return bntp.UTCStamp()
}

func (e *Exchange) Nonce() int64 {
	return e.MilliSeconds() - e.TimeDelay
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

/*
RequestApi
Request exchange API without checking cache
Concurrency control: Same host, default concurrent 3 times at the same time

请求交易所API，不检查缓存
并发控制：同一个host，默认同时并发3
*/
func (e *Exchange) RequestApi(ctx context.Context, cacheKey string, api *Entry, params map[string]interface{}, cache, debug bool) *HttpRes {
	if e.NetDisable {
		err := errs.NewMsg(errs.CodeNetDisable, fmt.Sprintf("net disabled for %v, fail: %v", e.Name, api.Url))
		return &HttpRes{Error: err}
	}
	// Traffic control, block if concurrency is full
	// 流量控制，如果并发已满则阻塞
	sem := GetHostFlowChan(api.RawHost)
	sem <- struct{}{}
	defer func() {
		<-sem
	}()
	// Check if 429 or 418 appears and wait
	// 检查是否出现429或418需要等待
	waitMS := GetHostRetryWait(api.RawHost, true)
	if waitMS > 0 {
		time.Sleep(time.Millisecond * time.Duration(waitMS))
	}
	if e.EnableRateLimit == BoolTrue {
		e.rateM.Lock()
		elapsed := e.MilliSeconds() - e.lastRequestMS
		cost := e.CalcRateLimiterCost(api, params)
		sleepMS := int64(math.Round(float64(e.RateLimit) * cost))
		if elapsed < sleepMS {
			time.Sleep(time.Duration(sleepMS-elapsed) * time.Millisecond)
		}
		e.lastRequestMS = e.MilliSeconds()
		e.rateM.Unlock()
	}
	sign := e.Sign(api, params)
	if sign.Error != nil {
		return &HttpRes{AccName: sign.AccName, Error: sign.Error}
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
		return &HttpRes{Url: sign.Url, AccName: sign.AccName, Error: errs.New(errs.CodeInvalidRequest, err)}
	}
	req = req.WithContext(ctx)
	req.Header = sign.Headers
	e.setReqHeaders(&req.Header)

	if debug || e.DebugAPI {
		log.Debug("request", zap.String(sign.Method, sign.Url),
			zap.Object("header", HttpHeader(req.Header)), zap.String("body", sign.Body))
	}
	rsp, err := e.HttpClient.Do(req)
	if err != nil {
		return &HttpRes{Url: sign.Url, AccName: sign.AccName, Error: errs.New(errs.CodeNetFail, err)}
	}
	defer rsp.Body.Close()
	var result = HttpRes{Url: sign.Url, AccName: sign.AccName, Status: rsp.StatusCode, Headers: rsp.Header,
		CacheKey: cacheKey}
	rspData, err := io.ReadAll(rsp.Body)
	if err != nil {
		result.Error = errs.New(errs.CodeNetFail, err)
		return &result
	}
	result.Content = string(rspData)
	cutLen := min(len(result.Content), 3000)
	bodyShort := zap.String("body", result.Content[:cutLen])
	if debug || e.DebugAPI {
		log.Debug("rsp", zap.Int("status", result.Status), zap.String("url", sign.Url),
			zap.Object("head", HttpHeader(result.Headers)),
			zap.Int("len", len(result.Content)), bodyShort)
	}
	if result.Status >= 400 {
		msg := fmt.Sprintf("%s: %s  %v", sign.AccName, req.URL, result.Content)
		result.Error = errs.NewMsg(result.Status, msg)
		var resData = make(map[string]interface{})
		err = utils.UnmarshalString(result.Content, &resData, utils.JsonNumAuto)
		if err == nil {
			// Handle both string (OKX) and int64 (Binance) code values
			if codeVal, ok := resData["code"]; ok && codeVal != nil {
				switch v := codeVal.(type) {
				case int64:
					result.Error.BizCode = int(v)
				case string:
					if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
						result.Error.BizCode = int(parsed)
					}
				}
			}
		}
		if result.Status == 429 || result.Status == 418 {
			waitStr := rsp.Header.Get("Retry-After")
			var waitSecs int64 = 30
			if waitStr != "" {
				if parsed, err := strconv.ParseInt(waitStr, 10, 64); err != nil {
					log.Error("parse Retry-After fail", zap.String("val", waitStr), zap.Error(err))
				} else {
					waitSecs = parsed
				}
			}
			result.Error.Data = waitSecs
			SetHostRetryWait(api.RawHost, waitSecs*1000)
		}
	} else if cache && api.CacheSecs > 0 {
		if sign.Private {
			log.Warn("cache private api result is not recommend:" + sign.Url)
		}
		e.cacheApiRes(api, &result)
	}
	return &result
}

func (e *Exchange) RequestApiRetry(ctx context.Context, endpoint string, params map[string]interface{}, retryNum int) *HttpRes {
	noCache := utils.PopMapVal(params, ParamNoCache, false)
	return e.RequestApiRetryAdv(ctx, endpoint, params, retryNum, !noCache, true)
}

func (e *Exchange) GetCacheKey(endpoint string, params map[string]interface{}) string {
	paramStr, _ := utils.MarshalString(params)
	return fmt.Sprintf("%s_%s_%s.json", e.ID, endpoint, utils.MD5([]byte(paramStr))[:10])
}

func (e *Exchange) CacheApiRes(endpoint string, res *HttpRes) {
	if !res.IsCache {
		api, _ := e.Apis[endpoint]
		if api != nil && api.CacheSecs > 0 {
			e.cacheApiRes(api, res)
		}
	}
}

func (e *Exchange) cacheApiRes(api *Entry, res *HttpRes) {
	cacheText, err_ := utils.MarshalString(&res)
	if err_ != nil {
		log.Error("cache api rsp fail", zap.String("url", res.Url), zap.Error(err_))
	} else {
		err2 := utils.WriteCacheFile(res.CacheKey, cacheText, api.CacheSecs)
		if err2 != nil {
			log.Error("write api rsp cache fail", zap.String("url", res.Url), zap.Error(err2))
		}
	}
}

func (e *Exchange) RequestApiRetryAdv(ctx context.Context, endpoint string, params map[string]interface{}, retryNum int, readCache, writeCache bool) *HttpRes {
	api, ok := e.Apis[endpoint]
	if !ok {
		log.Panic("invalid api", zap.String("endpoint", endpoint))
		return &HttpRes{Error: errs.NewMsg(errs.CodeApiNotSupport, "api not support")}
	}
	debug := utils.PopMapVal(params, ParamDebug, false)
	// 检查是否有缓存
	var cacheKey string
	if readCache && api.CacheSecs > 0 {
		cacheKey = e.GetCacheKey(endpoint, params)
		cacheText, err := utils.ReadCacheFile(cacheKey)
		if err != nil && (!(err.Code == errs.CodeExpired && e.NetDisable)) {
			if debug || e.DebugAPI {
				log.Debug("read api cache fail", zap.String("url", api.Path), zap.String("err", err.Short()))
			}
		} else {
			var res = &HttpRes{}
			err_ := utils.UnmarshalString(cacheText, res, utils.JsonNumDefault)
			if err_ != nil {
				log.Warn("unmarshal api cache fail", zap.String("url", api.Path), zap.Error(err_))
			} else {
				res.IsCache = true
				res.CacheKey = cacheKey
				return res
			}
		}
	}
	if api.RawHost == "" || e.onHost != nil {
		// we should recalculate on each time if onHost is provided as it may return a random host
		// 提供onHost时，可能每次请求host不同，需要重新计算
		api.Url = e.GetHost(api.Host) + "/" + api.Path
		parsed, err_ := url.Parse(api.Url)
		if err_ != nil {
			return &HttpRes{Error: errs.New(errs.CodeRunTime, err_)}
		}
		api.RawHost = parsed.Host
	}
	if e.NetDisable {
		err := errs.NewMsg(errs.CodeNetDisable, fmt.Sprintf("net disabled for %v, fail: %v", e.Name, api.Url))
		return &HttpRes{Error: err}
	}
	tryNum := retryNum + 1
	var rsp *HttpRes
	var sleep = 0
	for i := 0; i < tryNum; i++ {
		if sleep > 0 {
			time.Sleep(time.Second * time.Duration(sleep))
			sleep = 0
		}
		rsp = e.RequestApi(ctx, cacheKey, api, params, writeCache, debug)
		if rsp.Error != nil {
			if rsp.Error.Code == errs.CodeNetFail {
				// 网络错误等待3s重试
				sleep = 3
				log.Warn(fmt.Sprintf("net fail, retry after: %v", sleep))
				continue
			} else if rsp.Error.Code == 503 {
				// 交易所服务器过载
				sleep = 3
				log.Warn(fmt.Sprintf("exchange overload, retry after: %v", sleep))
				continue
			} else if rsp.Error.Code == 429 || rsp.Error.Code == 418 {
				// 请求过于频繁，随机休息
				retryAfter, _ := rsp.Error.Data.(int64)
				randWait := int(rand.Float32() * 10)
				if retryAfter > 0 {
					sleep = int(retryAfter) + randWait
				} else {
					sleep = 30 + randWait
				}
				log.Warn(fmt.Sprintf("%v occur, retry after: %v, %v", rsp.Error.Code, sleep, rsp.Url))
				continue
			} else if e.GetRetryWait != nil {
				// 子交易所根据错误信息返回睡眠时间
				sleep = e.GetRetryWait(rsp.Error)
				if sleep >= 0 {
					log.Info(fmt.Sprintf("server err, retry after: %v", sleep))
					continue
				}
			}
		}
		break
	}
	rsp.CacheKey = cacheKey
	return rsp
}

func (e *Exchange) HasApi(key, market string) bool {
	items, hasMar := e.Has[market]
	if hasMar && items != nil {
		val, ok := items[key]
		if ok {
			return val != HasFail
		}
	}
	if market != "" {
		// 检测默认市场是否有api配置
		return e.HasApi(key, "")
	}
	return false
}

func (e *Exchange) SetOnHost(cb func(n string) string) {
	e.onHost = cb
}

func (e *Exchange) GetHost(name string) string {
	var result string
	if e.onHost != nil {
		result = e.onHost(name)
	}
	if result == "" {
		result = e.Hosts.GetHost(name)
	}
	return result
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
	marketType := utils.PopMapVal(args, ParamMarket, "")
	contractType := utils.GetMapVal(args, ParamContract, "")
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
	marketType := utils.PopMapVal(args, ParamMarket, "")
	contractType := utils.PopMapVal(args, ParamContract, "")
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
func (e *Exchange) LoadArgsMarket(symbol string, params map[string]interface{}) (map[string]interface{}, *Market, *errs.Error) {
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

func (e *Exchange) PrecAmount(m *Market, amount float64) (float64, *errs.Error) {
	prec := m.Precision
	text, err := utils.PrecFloat64Str(amount, prec.Amount, false, prec.ModeAmount)
	if err != nil {
		return 0, errs.New(errs.CodePrecDecFail, err)
	}
	res, err2 := strconv.ParseFloat(text, 64)
	if err2 != nil {
		return 0, errs.New(errs.CodePrecDecFail, err2)
	}
	return res, nil
}

func (e *Exchange) precPriceCost(m *Market, value float64, round bool) (float64, *errs.Error) {
	prec := m.Precision
	text, err := utils.PrecFloat64Str(value, prec.Price, round, prec.ModePrice)
	if err != nil {
		return 0, errs.New(errs.CodePrecDecFail, err)
	}
	res, err2 := strconv.ParseFloat(text, 64)
	if err2 != nil {
		return 0, errs.New(errs.CodePrecDecFail, err2)
	}
	return res, nil
}

func (e *Exchange) PrecPrice(m *Market, price float64) (float64, *errs.Error) {
	return e.precPriceCost(m, price, true)
}

func (e *Exchange) PrecCost(m *Market, cost float64) (float64, *errs.Error) {
	return e.precPriceCost(m, cost, false)
}

func (e *Exchange) PrecFee(m *Market, fee float64) (float64, *errs.Error) {
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

func (e *Exchange) PopAccName(params map[string]interface{}) string {
	if params == nil {
		return e.DefAccName
	}
	return utils.PopMapVal(params, ParamAccount, e.DefAccName)
}

func (e *Exchange) GetAccName(params map[string]interface{}) string {
	if params == nil {
		return e.DefAccName
	}
	return utils.GetMapVal(params, ParamAccount, e.DefAccName)
}

func (e *Exchange) GetAccount(id string) (*Account, *errs.Error) {
	isCmd := strings.HasPrefix(id, ":")
	if id == "" || isCmd {
		if e.DefAccName != "" {
			id = e.DefAccName
		} else if len(e.Accounts) == 1 || id == ":first" {
			for key := range e.Accounts {
				id = key
				break
			}
			if len(e.Accounts) == 1 {
				e.DefAccName = id
			}
		} else {
			return nil, errs.NewMsg(errs.CodeAccKeyError, "ParamAccount or DefAccName must be specified")
		}
	}
	acc, ok := e.Accounts[id]
	if !ok {
		return nil, errs.NewMsg(errs.CodeAccKeyError, "Account key invalid: %s", id)
	}
	return acc, nil
}

func (e *Exchange) GetAccountCreds(id string) (string, *Credential, *errs.Error) {
	acc, err := e.GetAccount(id)
	if err != nil {
		return id, nil, err
	}
	if acc.Creds != nil {
		err = acc.Creds.CheckFilled(e.CredKeys)
		if err != nil {
			return acc.Name, nil, err
		}
		return acc.Name, acc.Creds, nil
	}
	return acc.Name, nil, errs.NewMsg(errs.CodeCredsRequired, "Creds not exits")
}

// CheckRiskyAllowed 检查当前账户是否允许执行危险操作
// 如果api.Risky为true且账户NoTrade为true，返回错误
func (e *Exchange) CheckRiskyAllowed(api *Entry, accID string) *errs.Error {
	if !api.Risky {
		return nil
	}
	acc, err := e.GetAccount(accID)
	if err != nil {
		return err
	}
	if acc.NoTrade {
		return errs.NewMsg(errs.CodeNoTrade, "risky operation forbidden: %s", api.Path)
	}
	return nil
}

func (e *Exchange) parseOptCreds() {
	var defCreds map[string]map[string]interface{}
	creds := utils.GetMapVal(e.Options, OptAccCreds, defCreds)
	e.Accounts = make(map[string]*Account)
	if creds != nil {
		for k, cred := range creds {
			e.Accounts[k] = newAccount(k, cred)
		}
		e.DefAccName = utils.GetMapVal(e.Options, OptAccName, "")
	} else {
		apiKey := utils.GetMapVal(e.Options, OptApiKey, "")
		apiSecret := utils.GetMapVal(e.Options, OptApiSecret, "")
		apiPass := utils.GetMapVal(e.Options, OptPassword, "")
		if apiKey != "" || apiSecret != "" || apiPass != "" {
			e.DefAccName = "default"
			e.Accounts[e.DefAccName] = &Account{
				Name:         e.DefAccName,
				NoTrade:      utils.PopMapVal(e.Options, OptNoTrade, false),
				Creds:        &Credential{ApiKey: apiKey, Secret: apiSecret, Password: apiPass},
				MarBalances:  map[string]*Balances{},
				MarPositions: map[string][]*Position{},
				Leverages:    map[string]int{},
				Data:         map[string]interface{}{},
				LockBalance:  &deadlock.Mutex{},
				LockPos:      &deadlock.Mutex{},
				LockLeverage: &deadlock.Mutex{},
				LockData:     &deadlock.Mutex{},
			}
		}
	}
}

func newAccount(name string, cred map[string]interface{}) *Account {
	var current = map[string]interface{}{}
	maps.Copy(current, cred)
	return &Account{
		Name:    name,
		NoTrade: utils.PopMapVal(current, OptNoTrade, false),
		Creds: &Credential{
			ApiKey:   utils.PopMapVal(current, OptApiKey, ""),
			Secret:   utils.PopMapVal(current, OptApiSecret, ""),
			Password: utils.PopMapVal(current, OptPassword, ""),
		},
		MarPositions: map[string][]*Position{},
		MarBalances:  map[string]*Balances{},
		Leverages:    map[string]int{},
		Data:         current,
		LockBalance:  &deadlock.Mutex{},
		LockPos:      &deadlock.Mutex{},
		LockLeverage: &deadlock.Mutex{},
		LockData:     &deadlock.Mutex{},
	}
}

func (e *Exchange) SetMarketType(marketType, contractType string) *errs.Error {
	if marketType != "" {
		if _, ok := AllMarketTypes[marketType]; !ok {
			return errs.NewMsg(errs.CodeParamInvalid, "invalid market type: %s", marketType)
		}
		e.MarketType = marketType
	}
	if contractType != "" {
		if _, ok := AllContractTypes[contractType]; !ok {
			return errs.NewMsg(errs.CodeParamInvalid, "invalid contract type: %s", contractType)
		}
		e.ContractType = contractType
	}
	return nil
}

func (e *Exchange) GetID() string {
	return e.ID
}

func (e *Exchange) GetNetDisable() bool {
	return e.NetDisable
}

func (e *Exchange) SetNetDisable(v bool) {
	e.NetDisable = v
}

func (e *Exchange) GetExg() *Exchange {
	return e
}

func (e *Exchange) Close() *errs.Error {
	if e.MarketsWait != nil {
		close(e.MarketsWait)
		e.MarketsWait = nil
	}
	e.lockOutChan.Lock()
	for key, chanQ := range e.WsOutChans {
		chVal := reflect.ValueOf(chanQ)
		if chVal.Kind() == reflect.Chan {
			chVal.Close()
		}
		delete(e.WsOutChans, key)
	}
	e.lockOutChan.Unlock()
	for _, client := range e.WSClients {
		client.Close()
	}
	e.WSClients = map[string]*WsClient{}
	err := e.SetDump("")
	if err != nil {
		return err
	}
	e.WsCache = nil
	err = e.SetReplay("")
	if err != nil {
		return err
	}
	return nil
}

func makeCalcRateLimiterCost(e *Exchange) FuncCalcRateLimiterCost {
	return func(api *Entry, params map[string]interface{}) float64 {
		return api.Cost
	}
}
