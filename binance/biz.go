package binance

import (
	"context"
	"fmt"
	"github.com/anyongjin/banexg"
	"github.com/anyongjin/banexg/log"
	"github.com/anyongjin/banexg/utils"
	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/decoder"
	"go.uber.org/zap"
	"net/http"
	"reflect"
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

func (e *Binance) Init() {
	e.Exchange.Init()
	utils.SetFieldBy(&e.RecvWindow, e.Options, OptRecvWindow, 10000)
	if e.CareMarkets == nil || len(e.CareMarkets) == 0 {
		e.CareMarkets = DefCareMarkets
	}
}

func makeSign(e *Binance) banexg.FuncSign {
	return func(api banexg.Entry, args *map[string]interface{}) *banexg.HttpReq {
		var params map[string]interface{}
		if args == nil {
			params = make(map[string]interface{})
		} else {
			params = *args
		}
		path := api.Path
		hostKey := api.Host
		url := e.Hosts.GetHost(hostKey) + "/" + path
		headers := http.Header{}
		query := make([]string, 0)
		body := ""
		if path == "historicalTrades" {
			if e.Creds.ApiKey == "" {
				log.Panic("historicalTrades requires `apiKey`", zap.String("id", e.ID))
				return &banexg.HttpReq{Error: banexg.ErrMissingApiKey}
			}
			headers.Add("X-MBX-APIKEY", e.Creds.ApiKey)
		} else if path == "userDataStream" || path == "listenKey" {
			//v1 special case for userDataStream
			if e.Creds.ApiKey == "" {
				log.Panic("userDataStream requires `apiKey`", zap.String("id", e.ID))
				return &banexg.HttpReq{Error: banexg.ErrMissingApiKey}
			}
			headers.Add("X-MBX-APIKEY", e.Creds.ApiKey)
			headers.Add("Content-Type", "application/x-www-form-urlencoded")
		} else if _, ok := secretApis[hostKey]; ok || (hostKey == "sapi" && path != "system/status") {
			err := e.Creds.CheckFilled()
			if err != nil {
				return &banexg.HttpReq{Error: err}
			}
			extendParams := map[string]interface{}{
				"timestamp": e.Nonce(),
			}
			utils.DeepCopy(params, extendParams)
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
			var secret = e.Creds.Secret
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
				return &banexg.HttpReq{Error: err}
			}
			query = append(query, "signature="+sign)
			headers.Add("X-MBX-APIKEY", e.Creds.ApiKey)
			if api.Method == "GET" || api.Method == "DELETE" {
				url += "?" + strings.Join(query, "&")
			} else {
				body = strings.Join(query, "&")
				headers.Add("Content-Type", "application/x-www-form-urlencoded")
			}
		} else if len(params) > 0 {
			url += "?" + utils.UrlEncodeMap(params, true)
		}
		return &banexg.HttpReq{Url: url, Method: api.Method, Headers: headers, Body: body}
	}
}

/*
fetches all available currencies on an exchange
:see: https://binance-docs.github.io/apidocs/spot/en/#all-coins-39-information-user_data
:param dict [params]: extra parameters specific to the exchange API endpoint
:returns dict: an associative dictionary of currencies
*/
func makeFetchCurr(e *Binance) banexg.FuncFetchCurr {
	return func(params *map[string]interface{}) (banexg.CurrencyMap, error) {
		if !e.HasApi("fetchCurrencies") {
			return nil, banexg.ErrApiNotSupport
		}
		if err := e.Creds.CheckFilled(); err != nil {
			return nil, banexg.ErrCredsRequired
		}
		if e.Hosts.TestNet {
			//sandbox/testnet does not support sapi endpoints
			return nil, banexg.ErrSandboxApiNotSupport
		}
		res := e.RequestApi(context.Background(), "sapiGetCapitalConfigGetall", params)
		if res.Error != nil {
			return nil, res.Error
		}
		if !strings.HasPrefix(res.Content, "[") {
			return nil, fmt.Errorf("FetchCurrencies api fail: %s", res.Content)
		}
		var currList []*BnbCurrency
		err := sonic.UnmarshalString(res.Content, &currList)
		if err != nil {
			return nil, err
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
					if curr.Precision == 0 || float64(precisionTick) > curr.Precision {
						curr.Precision = float64(precisionTick)
					}
				}
				curr.Networks[i] = &banexg.ChainNetwork{
					ID:        net.Network,
					Network:   net.Network,
					Name:      net.Name,
					Active:    net.DepositEnable || net.WithdrawEnable,
					Fee:       withDrawFee,
					Precision: float64(precisionTick),
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

var marketApiMap = map[string]string{
	"spot":    "publicGetExchangeInfo",
	"linear":  "fapiPublicGetExchangeInfo",
	"inverse": "dapiPublicGetExchangeInfo",
	"option":  "eapiPublicGetExchangeInfo",
}

func (e *Binance) mapMarket(mar *BnbMarket) *banexg.Market {
	isSwap, isFuture, isOption := false, false, false
	var symParts = strings.Split(mar.Symbol, "-")
	var baseId = mar.BaseAsset
	var quoteId = mar.QuoteAsset
	var base = e.SafeCurrency(baseId).Code
	var quote = e.SafeCurrency(quoteId).Code
	var symbol = fmt.Sprintf("%s/%s", base, quote)
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
		isInverse = settle == base
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
	unifiedType := ""
	if isSpot {
		unifiedType = banexg.MarketSpot
	} else if isSwap {
		unifiedType = banexg.MarketSwap
	} else if isFuture {
		unifiedType = banexg.MarketFuture
	} else if isOption {
		unifiedType = banexg.MarketOption
		isActive = false
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
	var subType = ""
	if isLinear {
		subType = "linear"
	} else if isInverse {
		subType = "inverse"
	}
	var market = banexg.Market{
		ID:             mar.Symbol,
		LowercaseID:    strings.ToLower(mar.Symbol),
		Symbol:         symbol,
		Base:           base,
		Quote:          quote,
		Settle:         settle,
		BaseID:         baseId,
		QuoteID:        quoteId,
		SettleID:       settleId,
		Type:           unifiedType,
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
		SubType:        subType,
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
	return func(params *map[string]interface{}) (banexg.MarketMap, error) {
		var ctx = context.Background()
		var ch = make(chan *banexg.HttpRes)
		doReq := func(key string) {
			apiKey, ok := marketApiMap[key]
			if !ok {
				log.Error("unsupported market type", zap.String("key", key))
				ch <- &banexg.HttpRes{Error: banexg.ErrUnsupportMarket}
				return
			}
			ch <- e.RequestApi(ctx, apiKey, params)
		}
		watNum := 0
		for _, marketType := range e.CareMarkets {
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
			err := sonic.UnmarshalString(rsp.Content, &res)
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

func parseOptionOhlcv(rsp *banexg.HttpRes) ([]*banexg.Kline, error) {
	var klines = make([]*BnbOptionKline, 0)
	err := sonic.UnmarshalString(rsp.Content, &klines)
	if err != nil {
		return nil, fmt.Errorf("decode option kline fail %v", err)
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

func parseBnbOhlcv(rsp *banexg.HttpRes, volIndex int) ([]*banexg.Kline, error) {
	var klines = make([][]interface{}, 0)
	dc := decoder.NewDecoder(rsp.Content)
	dc.UseInt64()
	err := dc.Decode(&klines)
	//err := sonic.UnmarshalString(rsp.Content, &klines)
	if err != nil {
		return nil, fmt.Errorf("parse bnb ohlcv fail: %v", err)
	}
	var res = make([]*banexg.Kline, len(klines))
	v := reflect.TypeOf(klines[0][0])
	log.Info("time format", zap.String("type", v.Name()))
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
func makeFetchOhlcv(e *Binance) banexg.FuncFetchOhlcv {
	return func(symbol, timeframe string, since int64, limit int, params *map[string]interface{}) ([]*banexg.Kline, error) {
		_, err := e.LoadMarkets(false, nil)
		if err != nil {
			return nil, fmt.Errorf("load markets fail: %v", err)
		}
		market, err := e.GetMarket(symbol)
		if err != nil {
			return nil, fmt.Errorf("get market fail: %v", err)
		}
		var args = utils.SafeParams(params)
		priceType := utils.GetMapVal(args, "price", "")
		until := utils.GetMapVal(args, "until", int64(0))
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
					return nil, fmt.Errorf("parse timeframe fail: %v", err)
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
		rsp := e.RequestApi(context.Background(), method, &args)
		if rsp.Error != nil {
			return nil, fmt.Errorf("api fail %v", rsp.Error)
		}
		if market.Option {
			return parseOptionOhlcv(rsp)
		} else {
			volIndex := 5
			if market.Inverse {
				volIndex = 7
			}
			return parseBnbOhlcv(rsp, volIndex)
		}
	}
}

func unmarshalBalance(content string, data interface{}) (*banexg.Balances, error) {
	err := sonic.UnmarshalString(content, data)
	if err != nil {
		return nil, fmt.Errorf("unmarshal fail: %v", err)
	}
	var result = banexg.Balances{
		Info:   data,
		Assets: map[string]*banexg.Asset{},
	}
	return &result, nil
}

func parseSpotBalances(e *Binance, rsp *banexg.HttpRes) (*banexg.Balances, error) {
	var data = SpotAccount{}
	result, err := unmarshalBalance(rsp.Content, &data)
	if err != nil {
		return nil, err
	}
	result.TimeStamp = data.UpdateTime
	for _, item := range data.Balances {
		asset := item.ToStdAsset(e.Exchange)
		if asset.IsEmpty() {
			continue
		}
		result.Assets[asset.Code] = asset
	}
	return result.Init(), nil
}

func parseMarginCrossBalances(e *Binance, rsp *banexg.HttpRes) (*banexg.Balances, error) {
	var data = MarginCrossBalances{}
	result, err := unmarshalBalance(rsp.Content, &data)
	if err != nil {
		return nil, err
	}
	for _, item := range data.UserAssets {
		asset := item.ToStdAsset(e.Exchange)
		if asset.IsEmpty() {
			continue
		}
		result.Assets[asset.Code] = asset
	}
	return result.Init(), nil
}

func parseMarginIsolatedBalances(e *Binance, rsp *banexg.HttpRes) (*banexg.Balances, error) {
	var data = IsolatedBalances{}
	result, err := unmarshalBalance(rsp.Content, &data)
	if err != nil {
		return nil, err
	}
	for _, item := range data.Assets {
		symbol := e.SafeSymbol(item.Symbol, "", banexg.MarketSpot)
		itemRes := make(map[string]*banexg.Asset)
		if item.BaseAsset != nil {
			asset := item.BaseAsset.ToStdAsset(e.Exchange)
			if asset.IsEmpty() {
				continue
			}
			itemRes[asset.Code] = asset
		}
		if item.QuoteAsset != nil {
			asset := item.QuoteAsset.ToStdAsset(e.Exchange)
			if asset.IsEmpty() {
				continue
			}
			itemRes[asset.Code] = asset
		}
		result.IsolatedAssets[symbol] = itemRes
	}
	return result.Init(), nil
}

func parseSwapBalances(e *Binance, rsp *banexg.HttpRes) (*banexg.Balances, error) {
	var data = SwapBalances{}
	result, err := unmarshalBalance(rsp.Content, &data)
	if err != nil {
		return nil, err
	}
	for _, item := range data.Assets {
		asset := item.ToStdAsset(e.Exchange)
		if asset.IsEmpty() {
			continue
		}
		result.Assets[asset.Code] = asset
	}
	return result.Init(), nil
}

func parseInverseBalances(e *Binance, rsp *banexg.HttpRes) (*banexg.Balances, error) {
	var data = InverseBalances{}
	result, err := unmarshalBalance(rsp.Content, &data)
	if err != nil {
		return nil, err
	}
	for _, item := range data.Assets {
		asset := item.ToStdAsset(e.Exchange)
		if asset.IsEmpty() {
			continue
		}
		result.Assets[asset.Code] = asset
	}
	return result.Init(), nil
}

func parseFundingBalances(e *Binance, rsp *banexg.HttpRes) (*banexg.Balances, error) {
	var data = make([]*FundingAsset, 0)
	result, err := unmarshalBalance(rsp.Content, &data)
	if err != nil {
		return nil, err
	}
	for _, item := range data {
		code := e.SafeCurrencyCode(item.Asset)
		free, _ := strconv.ParseFloat(item.Free, 64)
		freeze, _ := strconv.ParseFloat(item.Freeze, 64)
		withdraw, _ := strconv.ParseFloat(item.Withdrawing, 64)
		lock, _ := strconv.ParseFloat(item.Locked, 64)
		asset := banexg.Asset{
			Code: code,
			Free: free,
			Used: freeze + withdraw + lock,
		}
		if asset.IsEmpty() {
			continue
		}
		result.Assets[code] = &asset
	}
	return result.Init(), nil
}

/*
query for balance and get the amount of funds available for trading or funds locked in orders
:see: https://binance-docs.github.io/apidocs/spot/en/#account-information-user_data                  # spot
:see: https://binance-docs.github.io/apidocs/spot/en/#query-cross-margin-account-details-user_data   # cross margin
:see: https://binance-docs.github.io/apidocs/spot/en/#query-isolated-margin-account-info-user_data   # isolated margin
:see: https://binance-docs.github.io/apidocs/spot/en/#lending-account-user_data                      # lending
:see: https://binance-docs.github.io/apidocs/spot/en/#funding-wallet-user_data                       # funding
:see: https://binance-docs.github.io/apidocs/futures/en/#account-information-v2-user_data            # swap
:see: https://binance-docs.github.io/apidocs/delivery/en/#account-information-user_data              # future
:see: https://binance-docs.github.io/apidocs/voptions/en/#option-account-information-trade           # option
:param dict [params]: extra parameters specific to the exchange API endpoint
:param str [params.market]: 'spot', 'future', 'swap', 'funding', or 'spot'
:param str [params.marginMode]: 'cross' or 'isolated', for margin trading, uses self.options.defaultMarginMode if not passed, defaults to None/None/None
:param str[]|None [params.symbols]: unified market symbols, only used in isolated margin mode
:returns dict: a `balance structure <https://docs.ccxt.com/#/?id=balance-structure>`
*/
func makeFetchBalances(e *Binance) banexg.FuncFetchBalance {
	return func(params *map[string]interface{}) (*banexg.Balances, error) {
		_, err := e.LoadMarkets(false, nil)
		if err != nil {
			return nil, fmt.Errorf("load markets fail: %v", err)
		}
		var args = utils.SafeParams(params)
		marketType, marketInverse := e.GetArgsMarket(args)
		marginMode := utils.GetMapVal(args, "marginMode", "")
		method := "privateGetAccount"
		isContract := marketType == banexg.MarketSwap || marketType == banexg.MarketFuture
		if isContract {
			if !marketInverse {
				method = "fapiPrivateV2GetAccount"
			} else {
				method = "dapiPrivateGetAccount"
			}
		} else if marginMode == "isolated" {
			method = "sapiGetMarginIsolatedAccount"
			symbols := utils.GetMapVal(args, "symbols", []string{})
			if len(symbols) > 0 {
				b := strings.Builder{}
				notFirst := false
				for _, s := range symbols {
					mid, err := e.GetMarketID(s)
					if err != nil {
						return nil, fmt.Errorf("symbol invalid %s", s)
					}
					if notFirst {
						b.WriteString(",")
						notFirst = true
					}
					b.WriteString(mid)
				}
				args["symbols"] = b.String()
			}
		} else if marketType == banexg.MarketMargin || marginMode == banexg.MarginCross {
			method = "sapiGetMarginAccount"
		} else if marketType == "funding" {
			method = "sapiPostAssetGetFundingAsset"
		}
		rsp := e.RequestApi(context.Background(), method, &args)
		if rsp.Error != nil {
			return nil, rsp.Error
		}
		switch method {
		case "privateGetAccount":
			return parseSpotBalances(e, rsp)
		case "sapiGetMarginAccount":
			return parseMarginCrossBalances(e, rsp)
		case "sapiGetMarginIsolatedAccount":
			return parseMarginIsolatedBalances(e, rsp)
		case "fapiPrivateV2GetAccount":
			return parseSwapBalances(e, rsp)
		case "dapiPrivateGetAccount":
			return parseInverseBalances(e, rsp)
		case "sapiPostAssetGetFundingAsset":
			return parseFundingBalances(e, rsp)
		default:
			return nil, fmt.Errorf("unsupport parse balance method: %s", method)
		}
	}
}
