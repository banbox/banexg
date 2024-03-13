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
	"net/http"
	"strconv"
	"strings"
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
	return nil
}

func makeSign(e *Binance) banexg.FuncSign {
	return func(api banexg.Entry, args map[string]interface{}) *banexg.HttpReq {
		var params = utils.SafeParams(args)
		accID := e.GetAccName(params)
		path := api.Path
		hostKey := api.Host
		url := e.Hosts.GetHost(hostKey) + "/" + path
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
		if !e.HasApi("fetchCurrencies") {
			return nil, errs.ApiNotSupport
		}
		if e.Hosts.TestNet {
			//sandbox/testnet does not support sapi endpoints
			return nil, errs.SandboxApiNotSupport
		}
		tryNum := e.GetRetryNum("FetchCurr", 1)
		if params == nil {
			params = map[string]interface{}{banexg.ParamAccount: ":first"}
		} else if utils.GetMapVal(params, banexg.ParamAccount, "") == "" {
			params[banexg.ParamAccount] = ":first"
		}
		res := e.RequestApiRetry(context.Background(), "sapiGetCapitalConfigGetall", params, tryNum)
		if res.Error != nil {
			return nil, res.Error
		}
		if !strings.HasPrefix(res.Content, "[") {
			return nil, errs.NewMsg(errs.CodeInvalidResponse, "FetchCurrencies api fail: %s", res.Content)
		}
		var currList []*BnbCurrency
		err := utils.UnmarshalString(res.Content, &currList)
		if err != nil {
			return nil, errs.New(errs.CodeUnmarshalFail, err)
		}
		var result = make(banexg.CurrencyMap)
		for _, item := range currList {
			isWithDraw, isDeposit := false, false
			var curr = banexg.Currency{
				ID:       item.Coin,
				Name:     item.Name,
				Code:     item.Coin,
				Networks: make([]*banexg.ChainNetwork, len(item.NetworkList)),
				Fee:      -1,
				Fees:     make(map[string]float64),
				Info:     item,
			}
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
					if curr.Precision == 0 || precisionTick > curr.Precision {
						curr.Precision = precisionTick
					}
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
					Info:      net,
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
	banexg.MarketSpot:    "publicGetExchangeInfo",
	banexg.MarketLinear:  "fapiPublicGetExchangeInfo",
	banexg.MarketInverse: "dapiPublicGetExchangeInfo",
	banexg.MarketOption:  "eapiPublicGetExchangeInfo",
}

func (e *Binance) mapMarket(mar *BnbMarket) *banexg.Market {
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
	limits, pricePrec, amountPrec := mar.GetMarketLimits()
	if pricePrec > 0 {
		prec.Price = pricePrec
	}
	if amountPrec > 0 {
		prec.Amount = amountPrec
	}
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
		Info:           mar,
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
				ch <- &banexg.HttpRes{Error: errs.UnsupportMarket}
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
		var result = make(banexg.MarketMap)
		for i := 0; i < watNum; i++ {
			rsp, ok := <-ch
			if !ok {
				break
			}
			if rsp.Error != nil {
				continue
			}
			var res BnbMarketRsp
			err := utils.UnmarshalString(rsp.Content, &res)
			if err != nil {
				log.Error("Unmarshal bnb market fail", zap.String("text", rsp.Content))
				continue
			}
			if res.Symbols != nil {
				for _, item := range res.Symbols {
					market := e.mapMarket(item)
					result[market.Symbol] = market
				}
			}
		}
		return result, nil
	}
}

func parseOptionOHLCV(rsp *banexg.HttpRes) ([]*banexg.Kline, *errs.Error) {
	var klines = make([]*BnbOptionKline, 0)
	err := utils.UnmarshalString(rsp.Content, &klines)
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
		res[i] = &banexg.Kline{
			Time:   bar.OpenTime,
			Open:   open,
			High:   high,
			Low:    low,
			Close:  closeP,
			Volume: volume,
		}
	}
	return res, nil
}

func parseBnbOHLCV(rsp *banexg.HttpRes, volIndex int) ([]*banexg.Kline, *errs.Error) {
	var klines = make([][]interface{}, 0)
	err := utils.UnmarshalString(rsp.Content, &klines)
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
		//barTime, _ := strconv.ParseInt(timeText, 10, 64)
		open, _ := strconv.ParseFloat(openStr, 64)
		high, _ := strconv.ParseFloat(highStr, 64)
		low, _ := strconv.ParseFloat(lowStr, 64)
		closeP, _ := strconv.ParseFloat(closeStr, 64)
		volume, _ := strconv.ParseFloat(volStr, 64)
		res[i] = &banexg.Kline{
			Time:   int64(barTime),
			Open:   open,
			High:   high,
			Low:    low,
			Close:  closeP,
			Volume: volume,
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
	until := utils.PopMapVal(args, "until", int64(0))
	utils.OmitMapKeys(args, "price", "until")
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
	method := "publicGetKlines"
	if market.Option {
		method = "eapiPublicGetKlines"
	} else if priceType == "mark" {
		if market.Inverse {
			method = "dapiPublicGetMarkPriceKlines"
		} else {
			method = "fapiPublicGetMarkPriceKlines"
		}
	} else if priceType == "index" {
		if market.Inverse {
			method = "dapiPublicGetIndexPriceKlines"
		} else {
			method = "fapiPublicGetIndexPriceKlines"
		}
	} else if market.Linear {
		method = "fapiPublicGetKlines"
	} else if market.Inverse {
		method = "dapiPublicGetKlines"
	}
	tryNum := e.GetRetryNum("FetchOHLCV", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	if market.Option {
		return parseOptionOHLCV(rsp)
	} else {
		volIndex := 5
		if market.Inverse {
			volIndex = 7
		}
		return parseBnbOHLCV(rsp, volIndex)
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
func (e *Binance) SetLeverage(leverage int, symbol string, params map[string]interface{}) (map[string]interface{}, *errs.Error) {
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
		method = "fapiPrivatePostLeverage"
	} else if market.Inverse {
		method = "dapiPrivatePostLeverage"
	} else {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "%v SetLeverage supports linear and inverse contracts only", e.Name)
	}
	args["symbol"] = market.ID
	args["leverage"] = leverage
	tryNum := e.GetRetryNum("SetLeverage", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	var res = make(map[string]interface{})
	err2 := utils.UnmarshalString(rsp.Content, &res)
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
		method = "fapiPrivateGetLeverageBracket"
	} else if marketType == banexg.MarketInverse {
		method = "dapiPrivateV2GetLeverageBracket"
	} else {
		return errs.NewMsg(errs.CodeUnsupportMarket, "LoadLeverageBrackets support linear/inverse contracts only")
	}
	retryNum := e.GetRetryNum("LoadLeverageBrackets", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, retryNum)
	if rsp.Error != nil {
		return rsp.Error
	}
	var res = make([]LinearSymbolLvgBrackets, 0)
	err2 := utils.UnmarshalString(rsp.Content, &res)
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

func (e *Binance) GetLeverage(symbol string, notional float64, account string) (int, int) {
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
	return leverage, maxVal
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
	err := utils.UnmarshalString(rsp.Content, &data)
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
