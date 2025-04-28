package binance

import (
	"context"
	"fmt"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"go.uber.org/zap"
	"maps"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var secretApis = map[string]bool{
	"private":         true,
	"eapiPrivate":     true,
	"sapiV2":          true,
	"sapiV3":          true,
	"sapiV4":          true,
	HostDApiPrivate:   true,
	HostDApiPrivateV2: true,
	"fapiPrivate":     true,
	"fapiPrivateV2":   true,
	"papi":            true,
}

func (e *Binance) Init() *errs.Error {
	err := e.Exchange.Init()
	if err != nil {
		return err
	}
	utils.SetFieldBy(&e.RecvWindow, e.Options, OptRecvWindow, 30000)
	if e.CareMarkets == nil || len(e.CareMarkets) == 0 {
		e.CareMarkets = DefCareMarkets
	}
	e.streamIndex = -1
	e.streamLimits = map[string]int{
		banexg.MarketSpot:    50,
		banexg.MarketMargin:  50,
		banexg.MarketLinear:  50,
		banexg.MarketInverse: 50,
		banexg.MarketOption:  50,
	}
	e.streamBySubHash = map[string]string{}
	e.wsRequestId = map[string]int{}
	e.ExgInfo.NoHoliday = true
	e.ExgInfo.FullDay = true
	e.regReplayHandles()
	e.CalcRateLimiterCost = makeCalcRateLimiterCost(e)
	return nil
}

func makeSign(e *Binance) banexg.FuncSign {
	return func(api *banexg.Entry, args map[string]interface{}) *banexg.HttpReq {
		var params = utils.SafeParams(args)
		accID := e.PopAccName(params)
		path := api.Path
		hostKey := api.Host
		url := api.Url
		headers := http.Header{}
		query := make([]string, 0)
		body := ""
		isPrivate := false
		var creds *banexg.Credential
		var err *errs.Error
		if path == "historicalTrades" {
			accID, creds, err = e.GetAccountCreds(accID)
			if err != nil {
				log.Panic("historicalTrades requires `apiKey`", zap.String("id", e.ID))
				return &banexg.HttpReq{Error: err, Private: true}
			}
			headers.Add("X-MBX-APIKEY", creds.ApiKey)
			isPrivate = true
		} else if path == "userDataStream" || path == "listenKey" {
			//v1 special case for userDataStream
			accID, creds, err = e.GetAccountCreds(accID)
			if err != nil {
				log.Panic("userDataStream requires `apiKey`", zap.String("id", e.ID))
				return &banexg.HttpReq{Error: err, Private: true}
			}
			headers.Add("X-MBX-APIKEY", creds.ApiKey)
			headers.Add("Content-Type", "application/x-www-form-urlencoded")
			isPrivate = true
			if api.Method != "GET" {
				body = utils.UrlEncodeMap(params, true)
			}
		} else if _, ok := secretApis[hostKey]; ok || (hostKey == "sapi" && path != "system/status") {
			isPrivate = true
			accID, creds, err = e.GetAccountCreds(accID)
			if err != nil {
				return &banexg.HttpReq{Error: err, Private: true}
			}
			extendParams := map[string]interface{}{
				"timestamp": e.Nonce(),
			}
			maps.Copy(extendParams, params)
			if e.RecvWindow > 0 {
				extendParams["recvWindow"] = e.RecvWindow
			}
			if path == "batchOrders" || strings.Contains(path, "sub-account") || path == "capital/withdraw/apply" || strings.Contains(path, "staking") {
				query = append(query, utils.UrlEncodeMap(extendParams, true))
				if api.Method == "DELETE" && path == "batchOrders" {
					if orderIds, ok := extendParams[banexg.ParamOrderIds]; ok {
						if ids, ok := orderIds.([]string); ok {
							idText := strings.Join(ids, ",")
							query = append(query, "orderidlist=["+idText+"]")
						}
					}
					if orderIds, ok := extendParams[banexg.ParamOrigClientOrderIDs]; ok {
						if ids, ok := orderIds.([]string); ok {
							idText := strings.Join(ids, ",")
							query = append(query, "origclientorderidlist=["+idText+"]")
						}
					}
				}
			} else {
				query = append(query, utils.UrlEncodeMap(extendParams, false))
			}
			var sign, method, hash string
			var digest = "hex"
			var secret = creds.Secret
			if strings.Contains(secret, "PRIVATE KEY") {
				if len(secret) > 120 {
					method, hash = "rsa", "sha256"
				} else {
					method, hash = "eddsa", "ed25519"
				}
			} else {
				method, hash = "hmac", "sha256"
			}
			queryText := strings.Join(query, "&")
			sign, err = utils.Signature(queryText, secret, method, hash, digest)
			if err != nil {
				return &banexg.HttpReq{Error: err, Private: true}
			}
			query = append(query, "signature="+sign)
			headers.Add("X-MBX-APIKEY", creds.ApiKey)
			if api.Method == "GET" || api.Method == "DELETE" {
				url += "?" + strings.Join(query, "&")
			} else {
				body = strings.Join(query, "&")
				headers.Add("Content-Type", "application/x-www-form-urlencoded")
			}
		} else if len(params) > 0 {
			url += "?" + utils.UrlEncodeMap(params, true)
		}
		return &banexg.HttpReq{AccName: accID, Url: url, Method: api.Method, Headers: headers, Body: body,
			Private: isPrivate}
	}
}

/*
fetches all available currencies on an exchange
:see: https://binance-docs.github.io/apidocs/spot/en/#all-coins-39-information-user_data
:param dict [params]: extra parameters specific to the exchange API endpoint
:returns dict: an associative dictionary of currencies
*/
func makeFetchCurr(e *Binance) banexg.FuncFetchCurr {
	return func(params map[string]interface{}) (banexg.CurrencyMap, *errs.Error) {
		if e.Hosts.TestNet {
			//sandbox/testnet does not support sapi endpoints
			return nil, errs.NewMsg(errs.CodeSandboxApiNotSupport, "sandbox api not support")
		}
		tryNum := e.GetRetryNum("FetchCurr", 1)
		if params == nil {
			params = map[string]interface{}{banexg.ParamAccount: ":first"}
		} else if utils.GetMapVal(params, banexg.ParamAccount, "") == "" {
			params[banexg.ParamAccount] = ":first"
		}
		res := e.RequestApiRetry(context.Background(), MethodSapiGetCapitalConfigGetall, params, tryNum)
		if res.Error != nil {
			return nil, res.Error
		}
		if !strings.HasPrefix(res.Content, "[") {
			return nil, errs.NewMsg(errs.CodeInvalidResponse, "FetchCurrencies api fail: %s", res.Content)
		}
		var currList []*BnbCurrency
		currArr, err := utils.UnmarshalStringMapArr(res.Content, &currList)
		if err != nil {
			return nil, errs.New(errs.CodeUnmarshalFail, err)
		}
		var result = make(banexg.CurrencyMap)
		for ic, item := range currList {
			isWithDraw, isDeposit := false, false
			raw := currArr[ic]
			var curr = banexg.Currency{
				ID:       item.Coin,
				Name:     item.Name,
				Code:     item.Coin,
				Networks: make([]*banexg.ChainNetwork, len(item.NetworkList)),
				Fee:      -1,
				Fees:     make(map[string]float64),
				Info:     raw,
			}
			var nets []map[string]interface{}
			nets = utils.GetMapVal(raw, "networkList", nets)
			for i, net := range item.NetworkList {
				if !isWithDraw && net.WithdrawEnable {
					isWithDraw = true
				}
				if !isDeposit && net.DepositEnable {
					isDeposit = true
				}
				withDrawFee, err := strconv.ParseFloat(net.WithdrawFee, 64)
				if err == nil {
					curr.Fees[net.Network] = withDrawFee
					if net.IsDefault || curr.Fee == -1 {
						curr.Fee = withDrawFee
					}
				}
				precisionTick := utils.PrecisionFromString(net.WithdrawIntegerMultiple)
				if precisionTick != 0 {
					curr.Precision = precisionTick
					curr.PrecMode = banexg.PrecModeTickSize
				}
				curr.Networks[i] = &banexg.ChainNetwork{
					ID:        net.Network,
					Network:   net.Network,
					Name:      net.Name,
					Active:    net.DepositEnable || net.WithdrawEnable,
					Fee:       withDrawFee,
					Precision: precisionTick,
					Deposit:   net.DepositEnable,
					Withdraw:  net.WithdrawEnable,
					Info:      nets[i],
				}
			}
			curr.Active = isDeposit && isWithDraw && item.Trading
			curr.Deposit = isDeposit
			curr.Withdraw = isWithDraw
			if curr.Fee == -1 {
				curr.Fee = 0
			}
			result[item.Coin] = &curr
		}
		return result, nil
	}
}

func makeGetRetryWait(e *Binance) func(e *errs.Error) int {
	return func(err *errs.Error) int {
		//https://binance-docs.github.io/apidocs/futures/cn/#rest
		if err == nil || err.Code <= 500 {
			// 无需重试
			return -1
		}
		msg := err.Message()
		if err.Code/100 == 5 && strings.Contains(msg, "Request occur unknown error") {
			return 2
		}
		if err.Code == 503 {
			if strings.Contains(msg, "Service Unavailable") {
				return 3
			} else if strings.Contains(msg, "Please try again") {
				// 立即重试
				return 1
			}
		}
		return -1
	}
}

var marketApiMap = map[string]string{
	banexg.MarketSpot:    MethodPublicGetExchangeInfo,
	banexg.MarketLinear:  MethodFapiPublicGetExchangeInfo,
	banexg.MarketInverse: MethodDapiPublicGetExchangeInfo,
	banexg.MarketOption:  MethodEapiPublicGetExchangeInfo,
}

func (e *Binance) mapMarket(mar *BnbMarket, info map[string]interface{}) *banexg.Market {
	isSwap, isFuture, isOption := false, false, false
	var symParts = strings.Split(mar.Symbol, "-")
	var baseId = mar.BaseAsset
	var quoteId = mar.QuoteAsset
	var baseCode = e.SafeCurrency(baseId).Code
	var quote = e.SafeCurrency(quoteId).Code
	var symbol = fmt.Sprintf("%s/%s", baseCode, quote)
	var isContract = mar.ContractType != ""
	var expiry = max(mar.DeliveryDate, mar.ExpiryDate)
	var settleId = mar.MarginAsset
	if mar.ContractType == "PERPETUAL" || expiry == 4133404800000 {
		//some swap markets do not have contract type, eg: BTCST
		expiry = 0
		isSwap = true
	} else if mar.Underlying != "" {
		isContract = true
		isOption = true
		if settleId == "" {
			settleId = "USDT"
		}
	} else if expiry > 0 {
		isFuture = true
	}
	var settle = e.SafeCurrency(settleId).Code
	isSpot := !isContract
	contractSize := 0.0
	isLinear, isInverse := false, false
	fees := e.Fees.Main
	status := mar.Status
	if status == "" && mar.ContractStatus != "" {
		status = mar.ContractStatus
	}

	if isContract {
		if isSwap {
			symbol += ":" + settle
		} else if isFuture {
			symbol += ":" + settle + "-" + utils.YMD(expiry, "", false)
		} else if isOption {
			ymd := utils.YMD(expiry, "", false)
			last := "nil"
			if len(symParts) > 3 {
				last = symParts[3]
			}
			symbol = fmt.Sprintf("%s:%s-%s-%s-%s", symbol, settle, ymd, mar.StrikePrice, last)
		}
		if mar.ContractSize != 0 {
			contractSize = float64(mar.ContractSize)
		} else if mar.Unit != 0 {
			contractSize = float64(mar.Unit)
		} else {
			contractSize = 1.0
		}
		isLinear = settle == quote
		isInverse = settle == baseCode
		if isLinear && e.Fees.Linear != nil {
			fees = e.Fees.Linear
		} else if !isLinear && e.Fees.Inverse != nil {
			fees = e.Fees.Inverse
		} else {
			fees = &banexg.TradeFee{}
		}
	}
	isActive := status == "TRADING"
	if isSpot {
		for _, pms := range mar.Permissions {
			if pms == "TRD_GRP_003" {
				isActive = false
				break
			}
		}
	}
	marketType := ""
	if isOption {
		marketType = banexg.MarketOption
		isActive = false
	} else if isInverse {
		marketType = banexg.MarketInverse
	} else if isLinear {
		marketType = banexg.MarketLinear
	} else if isSpot {
		marketType = banexg.MarketSpot
	}
	strikePrice, _ := strconv.ParseFloat(mar.StrikePrice, 64)
	prec := mar.GetPrecision()
	limits := mar.GetMarketLimits(prec)
	var market = banexg.Market{
		ID:             mar.Symbol,
		LowercaseID:    strings.ToLower(mar.Symbol),
		Symbol:         symbol,
		Base:           baseCode,
		Quote:          quote,
		Settle:         settle,
		BaseID:         baseId,
		QuoteID:        quoteId,
		SettleID:       settleId,
		Type:           marketType,
		Spot:           isSpot,
		Margin:         isSpot && mar.IsMarginTradingAllowed,
		Swap:           isSwap,
		Future:         isFuture,
		Option:         isOption,
		Active:         isActive,
		Contract:       isContract,
		Linear:         isLinear,
		Inverse:        isInverse,
		Taker:          fees.Taker,
		Maker:          fees.Maker,
		ContractSize:   contractSize,
		Expiry:         expiry,
		ExpiryDatetime: utils.ISO8601(expiry),
		Strike:         strikePrice,
		OptionType:     strings.ToLower(mar.Side),
		Precision:      prec,
		Limits:         limits,
		Created:        mar.OnboardDate,
		Info:           info,
	}
	return &market
}

/*
retrieves data on all markets for binance
:see: https://binance-docs.github.io/apidocs/spot/en/#exchange-information         # spot
:see: https://binance-docs.github.io/apidocs/futures/en/#exchange-information      # swap
:see: https://binance-docs.github.io/apidocs/delivery/en/#exchange-information     # future
:see: https://binance-docs.github.io/apidocs/voptions/en/#exchange-information     # option
:param dict [params]: extra parameters specific to the exchange API endpoint
:returns dict[]: an array of objects representing market data
*/
func makeFetchMarkets(e *Binance) banexg.FuncFetchMarkets {
	return func(marketTypes []string, params map[string]interface{}) (banexg.MarketMap, *errs.Error) {
		var ctx = context.Background()
		var ch = make(chan *banexg.HttpRes)
		doReq := func(key string) {
			apiKey, ok := marketApiMap[key]
			if !ok {
				log.Error("unsupported market type", zap.String("key", key))
				ch <- &banexg.HttpRes{
					Error: errs.NewMsg(errs.CodeUnsupportMarket, "unsupported market type"),
				}
				return
			}
			tryNum := e.GetRetryNum("FetchMarkets", 1)
			ch <- e.RequestApiRetry(ctx, apiKey, params, tryNum)
		}
		watNum := 0
		for _, marketType := range marketTypes {
			if e.Hosts.TestNet && marketType == banexg.MarketOption {
				// option market not support in sandbox env
				continue
			}
			go doReq(marketType)
			watNum += 1
		}
		var err2 *errs.Error
		var result = make(banexg.MarketMap)
		for i := 0; i < watNum; i++ {
			rsp, ok := <-ch
			if !ok {
				break
			}
			if rsp.Error != nil {
				err2 = rsp.Error
				continue
			}
			var res BnbMarketRsp
			rawRsp, err := utils.UnmarshalStringMap(rsp.Content, &res)
			if err != nil {
				log.Error("Unmarshal bnb market fail", zap.String("text", rsp.Content))
				continue
			}
			var rawList []map[string]interface{}
			rawList = utils.GetMapVal(rawRsp, "symbols", rawList)
			if res.Symbols != nil {
				for j, item := range res.Symbols {
					market := e.mapMarket(item, rawList[j])
					result[market.Symbol] = market
				}
			}
		}
		return result, err2
	}
}

func parseOptionOHLCV(rsp *banexg.HttpRes) ([]*banexg.Kline, *errs.Error) {
	var klines = make([]*BnbOptionKline, 0)
	err := utils.UnmarshalString(rsp.Content, &klines, utils.JsonNumDefault)
	if err != nil {
		return nil, errs.NewFull(errs.CodeUnmarshalFail, err, "decode option kline fail")
	}
	var res = make([]*banexg.Kline, len(klines))
	for i, bar := range klines {
		open, _ := strconv.ParseFloat(bar.Open, 64)
		high, _ := strconv.ParseFloat(bar.High, 64)
		low, _ := strconv.ParseFloat(bar.Low, 64)
		closeP, _ := strconv.ParseFloat(bar.Close, 64)
		volume, _ := strconv.ParseFloat(bar.Amount, 64)
		takerVolume, _ := strconv.ParseFloat(bar.TakerAmount, 64)
		res[i] = &banexg.Kline{
			Time:   bar.OpenTime,
			Open:   open,
			High:   high,
			Low:    low,
			Close:  closeP,
			Volume: volume,
			Info:   takerVolume,
		}
	}
	return res, nil
}

func parseBnbOHLCV(rsp *banexg.HttpRes, volIndex, buyVolIndex int) ([]*banexg.Kline, *errs.Error) {
	var klines = make([][]interface{}, 0)
	err := utils.UnmarshalString(rsp.Content, &klines, utils.JsonNumAuto)
	if err != nil {
		return nil, errs.NewFull(errs.CodeUnmarshalFail, err, "parse bnb ohlcv fail")
	}
	if len(klines) == 0 {
		return nil, nil
	}
	var res = make([]*banexg.Kline, len(klines))
	for i, bar := range klines {
		barTime, _ := bar[0].(int64)
		openStr, _ := bar[1].(string)
		highStr, _ := bar[2].(string)
		lowStr, _ := bar[3].(string)
		closeStr, _ := bar[4].(string)
		volStr, _ := bar[volIndex].(string)
		buyVolStr, _ := bar[buyVolIndex].(string)
		// barTime, _ := strconv.ParseInt(timeText, 10, 64)
		open, _ := strconv.ParseFloat(openStr, 64)
		high, _ := strconv.ParseFloat(highStr, 64)
		low, _ := strconv.ParseFloat(lowStr, 64)
		closeP, _ := strconv.ParseFloat(closeStr, 64)
		volume, _ := strconv.ParseFloat(volStr, 64)
		buyVolume, _ := strconv.ParseFloat(buyVolStr, 64)
		res[i] = &banexg.Kline{
			Time:   barTime,
			Open:   open,
			High:   high,
			Low:    low,
			Close:  closeP,
			Volume: volume,
			Info:   buyVolume,
		}
	}
	return res, nil
}

/*
fetches historical candlestick data containing the open, high, low, and close price, and the volume of a market
:see: https://binance-docs.github.io/apidocs/spot/en/#kline-candlestick-data
:see: https://binance-docs.github.io/apidocs/voptions/en/#kline-candlestick-data
:see: https://binance-docs.github.io/apidocs/futures/en/#index-price-kline-candlestick-data
:see: https://binance-docs.github.io/apidocs/futures/en/#mark-price-kline-candlestick-data
:see: https://binance-docs.github.io/apidocs/futures/en/#kline-candlestick-data
:see: https://binance-docs.github.io/apidocs/delivery/en/#index-price-kline-candlestick-data
:see: https://binance-docs.github.io/apidocs/delivery/en/#mark-price-kline-candlestick-data
:see: https://binance-docs.github.io/apidocs/delivery/en/#kline-candlestick-data
:param str symbol: unified symbol of the market to fetch OHLCV data for
:param str timeframe: the length of time each candle represents
:param int [since]: timestamp in ms of the earliest candle to fetch
:param int [limit]: the maximum amount of candles to fetch
:param dict [params]: extra parameters specific to the exchange API endpoint
:param str [params.price]: "mark" or "index" for mark price and index price candles
:param int [params.until]: timestamp in ms of the latest candle to fetch
:param boolean [params.paginate]: default False, when True will automatically paginate by calling self endpoint multiple times. See in the docs all the [availble parameters](https://github.com/ccxt/ccxt/wiki/Manual#pagination-params)
:returns int[][]: A list of candles ordered, open, high, low, close, volume
*/
func (e *Binance) FetchOHLCV(symbol, timeframe string, since int64, limit int, params map[string]interface{}) ([]*banexg.Kline, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	priceType := utils.PopMapVal(args, "price", "")
	until := utils.PopMapVal(args, banexg.ParamUntil, int64(0))
	//binance docs say that the default limit 500, max 1500 for futures, max 1000 for spot markets
	//the reality is that the time range wider than 500 candles won't work right
	if limit == 0 {
		limit = 500
	} else {
		limit = min(limit, 1500)
	}
	args["interval"] = e.GetTimeFrame(timeframe)
	args["limit"] = limit
	if priceType == "index" {
		args["pair"] = market.ID
	} else {
		args["symbol"] = market.ID
	}
	if since > 0 {
		args["startTime"] = since
		//It didn't work before without the endTime
		//https://github.com/ccxt/ccxt/issues/8454
		if market.Inverse {
			secs, err := utils.ParseTimeFrame(timeframe)
			if err != nil {
				return nil, err
			}
			endTime := since + int64(limit*secs*1000) - 1
			args["endTime"] = min(e.MilliSeconds(), endTime)
		}
	}
	if until > 0 {
		args["endTime"] = until
	}
	method := MethodPublicGetKlines
	if market.Option {
		method = MethodEapiPublicGetKlines
	} else if priceType == "mark" {
		if market.Inverse {
			method = MethodDapiPublicGetMarkPriceKlines
		} else {
			method = MethodFapiPublicGetMarkPriceKlines
		}
	} else if priceType == "index" {
		if market.Inverse {
			method = MethodDapiPublicGetIndexPriceKlines
		} else {
			method = MethodFapiPublicGetIndexPriceKlines
		}
	} else if market.Linear {
		method = MethodFapiPublicGetKlines
	} else if market.Inverse {
		method = MethodDapiPublicGetKlines
	}
	tryNum := e.GetRetryNum("FetchOHLCV", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	if market.Option {
		return parseOptionOHLCV(rsp)
	} else {
		volIndex, buyVolIdx := 5, 9
		if market.Inverse {
			volIndex, buyVolIdx = 7, 10
		}
		return parseBnbOHLCV(rsp, volIndex, buyVolIdx)
	}
}

/*
SetLeverage
set the level of leverage for a market

	:see: https://binance-docs.github.io/apidocs/futures/en/#change-initial-leverage-trade
	:see: https://binance-docs.github.io/apidocs/delivery/en/#change-initial-leverage-trade
	:param float leverage: the rate of leverage
	:param str symbol: unified market symbol
	:param dict [params]: extra parameters specific to the exchange API endpoint
	:returns dict: response from the exchange
*/
func (e *Binance) SetLeverage(leverage float64, symbol string, params map[string]interface{}) (map[string]interface{}, *errs.Error) {
	if symbol == "" {
		return nil, errs.NewMsg(errs.CodeParamRequired, "symbol is required for %v.SetLeverage", e.Name)
	}
	if leverage < 1 || leverage > 125 {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "%v leverage should be between 1 and 125", e.Name)
	}
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	var method string
	if market.Linear {
		method = MethodFapiPrivatePostLeverage
	} else if market.Inverse {
		method = MethodDapiPrivatePostLeverage
	} else {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "%v SetLeverage supports linear and inverse contracts only", e.Name)
	}
	args["symbol"] = market.ID
	args["leverage"] = int(math.Round(leverage))
	tryNum := e.GetRetryNum("SetLeverage", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	var res = make(map[string]interface{})
	err2 := utils.UnmarshalString(rsp.Content, &res, utils.JsonNumAuto)
	if err2 != nil {
		return nil, errs.NewFull(errs.CodeUnmarshalFail, err, "%s decode rsp fail", e.Name)
	}
	return res, nil
}

func (e *Binance) LoadLeverageBrackets(reload bool, params map[string]interface{}) *errs.Error {
	if len(e.LeverageBrackets) > 0 && !reload {
		return nil
	}
	args := utils.SafeParams(params)
	marketType, _, err := e.LoadArgsMarketType(args)
	if err != nil {
		return err
	}
	var method string
	if marketType == banexg.MarketLinear {
		method = MethodFapiPrivateGetLeverageBracket
	} else if marketType == banexg.MarketInverse {
		method = MethodDapiPrivateV2GetLeverageBracket
	} else {
		return errs.NewMsg(errs.CodeUnsupportMarket, "LoadLeverageBrackets support linear/inverse contracts only")
	}
	retryNum := e.GetRetryNum("LoadLeverageBrackets", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, retryNum)
	if rsp.Error != nil {
		return rsp.Error
	}
	var res = make([]LinearSymbolLvgBrackets, 0)
	err2 := utils.UnmarshalString(rsp.Content, &res, utils.JsonNumDefault)
	if err2 != nil {
		return errs.New(errs.CodeUnmarshalFail, err2)
	}
	mapSymbol := func(id string) string {
		return e.SafeSymbol(id, "", marketType)
	}
	var brackets map[string]*SymbolLvgBrackets
	if marketType == banexg.MarketLinear {
		brackets, err = parseLvgBrackets[*LinearSymbolLvgBrackets](mapSymbol, rsp)
	} else {
		brackets, err = parseLvgBrackets[*InversePairLvgBrackets](mapSymbol, rsp)
	}
	if err != nil {
		return err
	}
	e.LeverageBrackets = brackets
	return nil
}

func (e *Binance) GetLeverage(symbol string, notional float64, account string) (float64, float64) {
	info, ok := e.LeverageBrackets[symbol]
	maxVal := 0
	if ok && len(info.Brackets) > 0 {
		for _, row := range info.Brackets {
			if notional < row.Floor {
				break
			}
			maxVal = row.InitialLeverage
			break
		}
	}
	if account == "" {
		account = e.DefAccName
	}
	var leverage int
	if acc, ok := e.Accounts[account]; ok {
		acc.LockLeverage.Lock()
		leverage, _ = acc.Leverages[symbol]
		acc.LockLeverage.Unlock()
	}
	return float64(leverage), float64(maxVal)
}

/*
GetMaintMarginPct
获取指定名义价值的维持保证金比率
*/
func (e *Binance) GetMaintMarginPct(symbol string, notional float64) float64 {
	info, ok := e.LeverageBrackets[symbol]
	maintMarginPct := float64(0)
	if ok && len(info.Brackets) > 0 {
		for _, row := range info.Brackets {
			if notional < row.Floor {
				break
			}
			maintMarginPct = row.MaintMarginRatio
		}
	}
	return maintMarginPct
}

func (e *Binance) CalcMaintMargin(symbol string, cost float64) (float64, *errs.Error) {
	if len(e.LeverageBrackets) == 0 {
		return 0, errs.NewMsg(errs.CodeRunTime, "LeverageBrackets not load")
	}
	info, ok := e.LeverageBrackets[symbol]
	maintMargin := float64(-1)
	if ok && len(info.Brackets) > 0 {
		for _, row := range info.Brackets {
			if cost < row.Floor {
				break
			} else if cost >= row.Floor {
				maintMargin = row.MaintMarginRatio*cost - row.Cum
			}
		}
	}
	if maintMargin < 0 {
		return 0, errs.NewMsg(errs.CodeParamInvalid, "cost invalid")
	}
	return maintMargin, nil
}

func parseLvgBrackets[T ISymbolLvgBracket](mapSymbol func(string) string, rsp *banexg.HttpRes) (map[string]*SymbolLvgBrackets, *errs.Error) {
	var data = make([]T, 0)
	err := utils.UnmarshalString(rsp.Content, &data, utils.JsonNumDefault)
	if err != nil {
		return nil, errs.New(errs.CodeUnmarshalFail, err)
	}
	var res = make(map[string]*SymbolLvgBrackets)
	for _, item := range data {
		symbol := mapSymbol(item.GetSymbol())
		if symbol == "" {
			continue
		}
		bracket := item.ToStdBracket()
		bracket.Symbol = symbol
		res[symbol] = bracket
	}
	return res, nil
}

func (e *Binance) Close() *errs.Error {
	err := e.Exchange.Close()
	if err != nil {
		return err
	}
	e.streamBySubHash = make(map[string]string)
	e.streamIndex = -1
	e.wsRequestId = map[string]int{}
	return nil
}

func (e *Binance) nextId(client *banexg.WsClient) int {
	requestId := e.wsRequestId[client.URL] + 1
	e.wsRequestId[client.URL] = requestId
	return requestId
}

/*
WriteWSMsg 向交易所写入ws消息。
isSub true订阅、false取消订阅
symbols 标准标的ID、或订阅字符串
cvt 不为空时，尝试对symbols进行标准化
getJobInfo 添加对返回结果的回调。会更新ID、symbols
*/
func (e *Binance) WriteWSMsg(client *banexg.WsClient, connID int, isSub bool, symbols []string, cvt func(m *banexg.Market, i int) string, getJobInfo banexg.FuncGetWsJob) *errs.Error {
	leftSymbols := symbols
	batchNum := 100
	var err *errs.Error
	var offset int
	for len(leftSymbols) > 0 {
		// Subscription in batches, with a maximum of 100 per batch
		// 分批次订阅，每批最大100个
		curOff := offset
		if len(leftSymbols) > batchNum {
			symbols = leftSymbols[:batchNum]
			leftSymbols = leftSymbols[batchNum:]
			offset += batchNum
		} else {
			symbols = leftSymbols
			leftSymbols = nil
		}
		var exgParams []string
		if cvt != nil {
			exgParams, err = e.getExgWsParams(curOff, symbols, cvt)
			if err != nil {
				return err
			}
		} else {
			exgParams = symbols
		}
		method, conn := client.UpdateSubs(connID, isSub, exgParams)
		if conn == nil {
			return errs.NewMsg(errs.CodeRunTime, "get ws conn fail")
		}
		id := e.nextId(client)
		var request = map[string]interface{}{
			"method": method,
			"params": exgParams,
			"id":     id,
		}
		var info *banexg.WsJobInfo
		if getJobInfo != nil {
			info, err = getJobInfo(client)
			if err != nil {
				return err
			}
			if info != nil {
				info.ID = strconv.Itoa(id)
				if len(info.Symbols) == 0 {
					info.Symbols = symbols
				}
			}
		}
		err = client.Write(conn, request, info)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *Binance) regReplayHandles() {
	e.WsReplayFn = map[string]func(item *banexg.WsLog) *errs.Error{
		"WatchOrderBooks": func(item *banexg.WsLog) *errs.Error {
			var symbols = make([]string, 0)
			err_ := utils.UnmarshalString(item.Content, &symbols, utils.JsonNumDefault)
			if err_ != nil {
				return errs.New(errs.CodeUnmarshalFail, err_)
			}
			log.Debug("replay WatchOrderBooks", zap.Strings("codes", symbols))
			_, err := e.WatchOrderBooks(symbols, 100, nil)
			return err
		},
		"WatchTrades": func(item *banexg.WsLog) *errs.Error {
			var symbols = make([]string, 0)
			err_ := utils.UnmarshalString(item.Content, &symbols, utils.JsonNumDefault)
			if err_ != nil {
				return errs.New(errs.CodeUnmarshalFail, err_)
			}
			log.Debug("replay WatchTrades", zap.Strings("codes", symbols))
			_, err := e.WatchTrades(symbols, nil)
			return err
		},
		"WatchOHLCVs": func(item *banexg.WsLog) *errs.Error {
			var jobs = make([][2]string, 0)
			err_ := utils.UnmarshalString(item.Content, &jobs, utils.JsonNumDefault)
			if err_ != nil {
				return errs.New(errs.CodeUnmarshalFail, err_)
			}
			if log.GetLevel() > zap.DebugLevel {
				var symbols = make([]string, 0, len(jobs)*2)
				for _, j := range jobs {
					symbols = append(symbols, j[0], j[1])
				}
				log.Debug("replay WatchOHLCVs", zap.Strings("codes", symbols))
			}
			_, err := e.WatchOHLCVs(jobs, nil)
			return err
		},
		"WatchMarkPrices": func(item *banexg.WsLog) *errs.Error {
			var symbols = make([]string, 0)
			err_ := utils.UnmarshalString(item.Content, &symbols, utils.JsonNumDefault)
			if err_ != nil {
				return errs.New(errs.CodeUnmarshalFail, err_)
			}
			log.Debug("replay WatchMarkPrices", zap.Strings("codes", symbols))
			_, err := e.WatchMarkPrices(symbols, nil)
			return err
		},
		"OdBookShot": func(item *banexg.WsLog) *errs.Error {
			var pak = &banexg.OdBookShotLog{}
			err_ := utils.UnmarshalString(item.Content, pak, utils.JsonNumDefault)
			if err_ != nil {
				return errs.New(errs.CodeUnmarshalFail, err_)
			}
			log.Debug("replay OdBookShot", zap.String("code", pak.Symbol))
			return e.applyOdBookSnapshot(pak.MarketType, pak.Symbol, pak.ChanKey, pak.Book)
		},
		"wsMsg": func(item *banexg.WsLog) *errs.Error {
			var arr = make([]string, 0)
			err_ := utils.UnmarshalString(item.Content, &arr, utils.JsonNumDefault)
			if err_ != nil {
				return errs.New(errs.CodeUnmarshalFail, err_)
			}
			client, err := e.GetClient(arr[0], arr[1], arr[2])
			if err != nil {
				return err
			}
			log.Debug("replay wsMsg", zap.String("msg", arr[3]))
			client.HandleRawMsg([]byte(arr[3]))
			return nil
		},
	}
}

var rateCostMap = map[string]string{
	"noCoin":   "coin",
	"noSymbol": "symbol",
	"noPoolId": "poolId",
}

func makeCalcRateLimiterCost(e *Binance) banexg.FuncCalcRateLimiterCost {
	return func(api *banexg.Entry, params map[string]interface{}) float64 {
		if api.More != nil {
			for key, val := range rateCostMap {
				if noVal, ok1 := api.More[key]; ok1 {
					if _, ok1_ := params[val]; !ok1_ {
						noValI, ok1_ := noVal.(int)
						if ok1_ {
							return float64(noValI)
						} else {
							noValF, ok1_ := noVal.(float64)
							if ok1_ {
								return noValF
							} else {
								log.Warn(fmt.Sprintf("bad cost type: binance.%v.%s", api.Path, key))
							}
						}
					}
				}
			}
			if byLimitV, ok := api.More["byLimit"]; ok {
				if limitV, ok2 := params["limit"]; ok2 {
					byLimitF, ok_ := byLimitV.([]int)
					limitF, ok2_ := limitV.(int)
					if ok_ && ok2_ {
						for i := 0; i+1 < len(byLimitF); i += 2 {
							level, cost := byLimitF[i], byLimitF[i+1]
							if limitF <= level {
								return float64(cost)
							}
						}
					} else {
						log.Warn(fmt.Sprintf("bad cost type: binance.%v byLimit: %v limit: %v",
							api.Path, ok_, ok2_))
					}
				}
			}
		}
		return api.Cost
	}
}

func (e *Binance) FetchFundingRate(symbol string, params map[string]interface{}) (*banexg.FundingRateCur, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	args["symbol"] = market.Symbol
	var method string
	if market.Linear {
		method = MethodFapiPublicGetPremiumIndex
	} else if market.Inverse {
		method = MethodDapiPublicGetPremiumIndex
	} else {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "unsupport market: %v", market.Type)
	}
	tryNum := e.GetRetryNum("FetchFundingRate", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	var ft = FundingRateCur{}
	raw, err_ := utils.UnmarshalStringMap(rsp.Content, &ft)
	if err_ != nil {
		return nil, errs.NewFull(errs.CodeUnmarshalFail, err_, "decode fail")
	}
	return ft.ToStd(e, market.Type, raw), nil
}

func (e *Binance) FetchFundingRates(symbols []string, params map[string]interface{}) ([]*banexg.FundingRateCur, *errs.Error) {
	args := utils.SafeParams(params)
	marketType, _, err := e.LoadArgsMarketType(args, symbols...)
	if err != nil {
		return nil, err
	}
	var method string
	if marketType == banexg.MarketLinear {
		method = MethodFapiPublicGetPremiumIndex
	} else if marketType == banexg.MarketInverse {
		method = MethodDapiPublicGetPremiumIndex
	} else {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "unsupport market: %v", marketType)
	}
	tryNum := e.GetRetryNum("FetchFundingRates", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	var items = make([]*FundingRateCur, 0)
	raws, err_ := utils.UnmarshalStringMapArr(rsp.Content, &items)
	if err_ != nil {
		return nil, errs.NewFull(errs.CodeUnmarshalFail, err_, "decode fail")
	}
	var list = make([]*banexg.FundingRateCur, 0, len(items))
	for i, it := range items {
		item := it.ToStd(e, marketType, raws[i])
		if item.Symbol != "" {
			list = append(list, item)
		}
	}
	return list, nil
}

const maxFundRateBatch = 1000 // 一次最多返回1000个

func (e *Binance) FetchFundingRateHistory(symbol string, since int64, limit int, params map[string]interface{}) ([]*banexg.FundingRate, *errs.Error) {
	args := utils.SafeParams(params)
	var marketType string
	var err *errs.Error
	if symbol != "" {
		mar, ok := e.Markets[symbol]
		if !ok {
			return nil, errs.NewMsg(errs.CodeParamInvalid, "symbol invalid: %v", symbol)
		}
		args["symbol"] = mar.ID
		marketType = mar.Type
	} else {
		marketType, _, err = e.LoadArgsMarketType(args)
		if err != nil {
			return nil, err
		}
	}
	if limit <= 0 {
		limit = maxFundRateBatch
	}
	args["limit"] = min(limit, maxFundRateBatch)
	var method string
	if marketType == banexg.MarketLinear {
		method = MethodFapiPublicGetFundingRate
	} else if marketType == banexg.MarketInverse {
		method = MethodDapiPublicGetFundingRate
	} else {
		return nil, errs.NewMsg(errs.CodeNotSupport, "market not support: %s", marketType)
	}
	until := utils.PopMapVal(args, banexg.ParamUntil, int64(0))
	if since > 0 {
		args["startTime"] = since
		if until <= 0 {
			interval := int64(60 * 60 * 8 * 1000)
			until = since + int64(limit)*interval
		}
	}
	if until > 0 {
		args["endTime"] = until
	}
	items := make([]*banexg.FundingRate, 0)
	for {
		list, hasMore, err := e.getFundRateHis(marketType, method, until, args)
		if err != nil {
			return nil, err
		}
		items = append(items, list...)
		if !hasMore {
			break
		}
		// 有未完成的数据，需要继续请求
		intv := list[1].Timestamp - list[0].Timestamp
		since = list[len(list)-1].Timestamp + intv
		args["startTime"] = since
	}
	return items, nil
}

func (e *Binance) getFundRateHis(marketType, method string, until int64, args map[string]interface{}) ([]*banexg.FundingRate, bool, *errs.Error) {
	tryNum := e.GetRetryNum("FetchFundingRateHistory", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, tryNum)
	if rsp.Error != nil {
		return nil, false, rsp.Error
	}
	var items = make([]*FundingRate, 0)
	rawList, err := utils.UnmarshalStringMapArr(rsp.Content, &items)
	if err != nil {
		return nil, false, errs.NewFull(errs.CodeUnmarshalFail, err, "decode option kline fail")
	}
	var lastMS int64
	var list = make([]*banexg.FundingRate, 0, len(items))
	for i, it := range items {
		code := e.SafeSymbol(it.Symbol, "", marketType)
		stamp := it.FundingTime
		if stamp > lastMS {
			lastMS = stamp
		}
		if code == "" {
			continue
		}
		rate, _ := strconv.ParseFloat(it.FundingRate, 64)
		list = append(list, &banexg.FundingRate{
			Symbol:      code,
			FundingRate: rate,
			Timestamp:   stamp,
			Info:        rawList[i],
		})
	}
	interval := int64(60 * 60 * 8 * 1000)
	hasMore := until > 0 && len(items) == maxFundRateBatch && lastMS+interval < until
	return list, hasMore, nil
}

func (f *FundingRateCur) ToStd(e *Binance, marketType string, info map[string]interface{}) *banexg.FundingRateCur {
	code := e.SafeSymbol(f.Symbol, "", marketType)
	markPrice, _ := strconv.ParseFloat(f.MarkPrice, 64)
	indexPrice, _ := strconv.ParseFloat(f.IndexPrice, 64)
	estSettlePrice, _ := strconv.ParseFloat(f.EstimatedSettlePrice, 64)
	lastRate, _ := strconv.ParseFloat(f.LastFundingRate, 64)
	interestRate, _ := strconv.ParseFloat(f.InterestRate, 64)
	return &banexg.FundingRateCur{
		Symbol:               code,
		FundingRate:          lastRate,
		Timestamp:            f.Time,
		Info:                 info,
		MarkPrice:            markPrice,
		IndexPrice:           indexPrice,
		EstimatedSettlePrice: estSettlePrice,
		InterestRate:         interestRate,
		NextFundingTimestamp: f.NextFundingTime,
	}
}

func (e *Binance) FetchLastPrices(symbols []string, params map[string]interface{}) ([]*banexg.LastPrice, *errs.Error) {
	args := utils.SafeParams(params)
	marketType, _, err := e.LoadArgsMarketType(args, symbols...)
	if err != nil {
		return nil, err
	}
	var method string
	if marketType == banexg.MarketLinear {
		method = MethodFapiPublicV2GetTickerPrice
	} else if marketType == banexg.MarketInverse {
		method = MethodDapiPublicGetTickerPrice
	} else if marketType == banexg.MarketSpot {
		method = MethodPublicGetTickerPrice
	} else {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "unsupported market: %v", marketType)
	}
	tryNum := e.GetRetryNum("FetchLastPrices", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	var items = make([]*LastPrice, 0)
	raws, err_ := utils.UnmarshalStringMapArr(rsp.Content, &items)
	if err_ != nil {
		return nil, errs.NewFull(errs.CodeUnmarshalFail, err_, "decode fail")
	}
	var list = make([]*banexg.LastPrice, 0, len(items))
	for i, it := range items {
		code := e.SafeSymbol(it.Symbol, "", marketType)
		price, _ := strconv.ParseFloat(it.Price, 64)
		list = append(list, &banexg.LastPrice{
			Symbol:    code,
			Timestamp: it.Time,
			Price:     price,
			Info:      raws[i],
		})
	}
	return list, nil
}

/*
定期检查公开ws数据消息是否超时（每个订阅key都应该定期收到数据推送，最长不超过3s），超时则自动重新连接
Regularly check whether the public ws data messages are timed out (each subscription key should receive data push regularly, with a maximum interval of 3 seconds). If a timeout occurs, automatically reconnect.
*/
func makeCheckWsTimeout(e *Binance) func() {
	return func() {
		e.WsChecking = true
		defer func() {
			e.WsChecking = false
		}()
		if e.WsTimeout < 3100 {
			log.Warn("WsTimeout for binance must >= 3100")
			e.WsTimeout = 3100
		}
		// 以超时时间的1/3作为轮询间隔
		loopIntv := time.Duration(e.WsTimeout) * time.Millisecond / 3
		for {
			time.Sleep(loopIntv)
			for _, client := range e.WSClients {
				if client.AccName != "" {
					// 跳过订阅账户数据推送（因不是定期稳定推送）
					// Skip the data push for subscription account data (as it is not regularly and stably pushed).
					continue
				}
				stats := client.GetConnSubStats(e.WsTimeout)
				connKeys := make([]string, 0, 4)
				for _, stat := range stats {
					allNum := len(stat.Stamps)
					if allNum == 0 || len(stat.Timeouts) == 0 {
						continue
					}
					failRate := float64(len(stat.Timeouts)) / float64(allNum)
					if failRate >= 0.5 && allNum >= 5 {
						// 失败过多，重新连接并订阅
						err_ := stat.Conn.ReConnect()
						if err_ != nil {
							log.Error("reconnect ws fail", zap.String("url", client.URL), zap.Error(err_))
						} else {
							log.Info("reconnect ws success", zap.String("url", client.URL), zap.Int("num", allNum))
						}
					} else {
						keys := utils.KeysOfMap(stat.Timeouts)
						connKeys = append(connKeys, keys...)
						err := e.WriteWSMsg(client, stat.ConnId, true, keys, nil, nil)
						if err != nil {
							log.Error("re-subscribe timeout keys fail", zap.Int("conn", stat.ConnId),
								zap.String("url", client.URL), zap.Error(err))
						}
					}
				}
				if len(connKeys) > 0 {
					log.Info("Found websocket timeout keys", zap.String("url", client.URL),
						zap.Any("keys", connKeys))
				}
			}
		}
	}
}
