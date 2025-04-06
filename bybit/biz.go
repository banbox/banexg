package bybit

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

func (e *Bybit) Init() *errs.Error {
	err := e.Exchange.Init()
	if err != nil {
		return err
	}
	utils.SetFieldBy(&e.RecvWindow, e.Options, OptRecvWindow, 30000)
	if e.CareMarkets == nil || len(e.CareMarkets) == 0 {
		e.CareMarkets = DefCareMarkets
	}
	e.ExgInfo.NoHoliday = true
	e.ExgInfo.FullDay = true
	return nil
}

func makeSign(e *Bybit) banexg.FuncSign {
	return func(api *banexg.Entry, args map[string]interface{}) *banexg.HttpReq {
		var params = utils.SafeParams(args)
		url := api.Url
		headers := http.Header{}
		accID := e.PopAccName(params)
		body := ""
		isPrivate := false
		if api.Host == HostPublic && len(params) > 0 {
			url += "?" + utils.UrlEncodeMap(params, true)
		} else if api.Host == HostPrivate {
			isPrivate = true
			var creds *banexg.Credential
			var err *errs.Error
			var err_ error
			accID, creds, err = e.GetAccountCreds(accID)
			if err != nil {
				return &banexg.HttpReq{Error: err, Private: true}
			}
			var sign string
			var method, hash = "hmac", "sha256"
			var digest = "hex"
			var secret = creds.Secret
			var timeStamp = strconv.FormatInt(e.Nonce(), 10)
			if strings.Contains(url, "openapi") {
				body = "{}"
				if len(params) > 0 {
					body, err_ = utils.MarshalString(params)
					if err_ != nil {
						return &banexg.HttpReq{Error: errs.New(errs.CodeMarshalFail, err_), Private: true}
					}
				}
				payload := timeStamp + creds.ApiKey + body
				sign, err = utils.Signature(payload, secret, method, hash, digest)
				if err != nil {
					return &banexg.HttpReq{Error: err, Private: true}
				}
				headers.Add("Content-Type", "application/json")
				headers.Add("X-BAPI-API-KEY", creds.ApiKey)
				headers.Add("X-BAPI-TIMESTAMP", timeStamp)
				headers.Add("X-BAPI-SIGN", sign)
			} else if strings.Contains(url, "v5") {
				headers.Add("X-BAPI-API-KEY", creds.ApiKey)
				headers.Add("X-BAPI-TIMESTAMP", timeStamp)
				headers.Add("X-BAPI-RECV-WINDOW", strconv.Itoa(e.RecvWindow))
				payload := timeStamp + creds.ApiKey + strconv.Itoa(e.RecvWindow)
				if api.Method == "POST" {
					body = "{}"
					if len(params) > 0 {
						body, err_ = utils.MarshalString(params)
						if err_ != nil {
							return &banexg.HttpReq{Error: errs.New(errs.CodeMarshalFail, err_), Private: true}
						}
					}
					payload += body
				} else if len(params) > 0 {
					encoded := utils.UrlEncodeMap(params, true)
					payload += encoded
					url += "?" + encoded
				}
				if strings.Contains(secret, "PRIVATE KEY") {
					method, hash = "rsa", "sha256"
				}
				sign, err = utils.Signature(payload, secret, method, hash, digest)
				if err != nil {
					return &banexg.HttpReq{Error: err, Private: true}
				}
				if api.Method != "GET" {
					headers.Add("Content-Type", "application/json")
				}
				headers.Add("X-BAPI-SIGN", sign)
			} else {
				return &banexg.HttpReq{Error: errs.NewMsg(errs.CodeRunTime, "unsupported api"), Private: true}
			}
		}
		if api.Method == "POST" {
			brokerId := utils.GetMapVal(params, banexg.ParamBrokerId, "")
			if brokerId != "" {
				headers.Add("Referer", brokerId)
			}
		}
		return &banexg.HttpReq{AccName: accID, Url: url, Method: api.Method, Headers: headers, Body: body,
			Private: isPrivate}
	}
}

func requestRetry[T any](e *Bybit, api string, params map[string]interface{}, tryNum int) *banexg.ApiRes[T] {
	res_ := e.RequestApiRetryAdv(context.Background(), api, params, tryNum, true, false)
	res := &banexg.ApiRes[T]{HttpRes: res_}
	if res.Error != nil {
		return res
	}
	var rsp = struct {
		RetCode    int             `json:"retCode"`
		RetMsg     string          `json:"retMsg"`
		Result     T               `json:"result"`
		RetExtInfo json.RawMessage `json:"retExtInfo"`
		Time       int64           `json:"time"`
	}{}
	err := utils.UnmarshalString(res.Content, &rsp, utils.JsonNumDefault)
	if err != nil {
		res.Error = errs.New(errs.CodeUnmarshalFail, err)
		return res
	}
	if rsp.RetCode != 0 {
		res.Error = errs.NewMsg(errs.CodeRunTime, "[%v] %s", rsp.RetCode, rsp.RetMsg)
	} else {
		res.Result = rsp.Result
		e.CacheApiRes(api, res_)
	}
	return res
}

func makeFetchCurr(e *Bybit) banexg.FuncFetchCurr {
	return func(params map[string]interface{}) (banexg.CurrencyMap, *errs.Error) {
		tryNum := e.GetRetryNum("FetchCurr", 1)
		if params == nil {
			params = map[string]interface{}{banexg.ParamAccount: ":first"}
		} else if utils.GetMapVal(params, banexg.ParamAccount, "") == "" {
			params[banexg.ParamAccount] = ":first"
		}
		res := requestRetry[struct {
			Rows []map[string]interface{} `json:"rows"`
		}](e, MethodPrivateGetV5AssetCoinQueryInfo, params, tryNum)
		if res.Error != nil {
			return nil, res.Error
		}
		var currList = res.Result
		var currArr []*Currency
		err := utils.DecodeStructMap(currList.Rows, &currArr, "json")
		if err != nil {
			return nil, errs.New(errs.CodeUnmarshalFail, err)
		}
		var result = make(banexg.CurrencyMap)
		for i, row := range currArr {
			raw := currList.Rows[i]
			nets := make([]*banexg.ChainNetwork, 0, len(row.Chains))
			curr := &banexg.Currency{
				ID:       row.Coin,
				Name:     row.Name,
				Code:     row.Coin,
				Networks: nets,
				Fee:      -1,
				Fees:     make(map[string]float64),
				Limits: &banexg.CodeLimits{
					Amount:   &banexg.LimitRange{},
					Withdraw: &banexg.LimitRange{},
					Deposit:  &banexg.LimitRange{},
				},
				Info: raw,
			}
			var chains []map[string]interface{}
			chains = utils.GetMapVal(raw, "chains", chains)
			deposit, withDraw := false, false
			for j, ch := range row.Chains {
				depositAllow := ch.ChainDeposit == "1"
				withdrawAllow := ch.ChainWithdraw == "1"
				if depositAllow {
					deposit = true
				}
				if withdrawAllow {
					withDraw = true
				}
				withDrawFee, err := strconv.ParseFloat(ch.WithdrawFee, 64)
				if err == nil {
					curr.Fees[ch.Chain] = withDrawFee
					if curr.Fee == -1 || curr.Fee > withDrawFee {
						curr.Fee = withDrawFee
					}
				}
				precisionTick := utils.PrecisionFromString(ch.MinAccuracy)
				if precisionTick != 0 && (curr.Precision == 0 || curr.Precision > precisionTick) {
					curr.Precision = precisionTick
					curr.PrecMode = banexg.PrecModeTickSize
				}
				minWithDraw, err1 := strconv.ParseFloat(ch.WithdrawMin, 64)
				minDeposit, err2 := strconv.ParseFloat(ch.DepositMin, 64)
				if err1 == nil && (curr.Limits.Withdraw.Min == 0 || curr.Limits.Withdraw.Min > minWithDraw) {
					curr.Limits.Withdraw.Min = minWithDraw
				}
				if err2 == nil && (curr.Limits.Deposit.Min == 0 || curr.Limits.Deposit.Min > minDeposit) {
					curr.Limits.Deposit.Min = minDeposit
				}
				nets = append(nets, &banexg.ChainNetwork{
					ID:        ch.Chain,
					Network:   ch.Chain,
					Name:      ch.Chain,
					Active:    depositAllow && withdrawAllow,
					Deposit:   depositAllow,
					Withdraw:  withdrawAllow,
					Fee:       withDrawFee,
					Precision: precisionTick,
					Limits: &banexg.CodeLimits{
						Withdraw: &banexg.LimitRange{
							Min: minWithDraw,
						},
						Deposit: &banexg.LimitRange{
							Min: minDeposit,
						},
					},
					Info: chains[j],
				})
			}
			curr.Active = deposit && withDraw
			curr.Deposit = deposit
			curr.Withdraw = withDraw
			result[row.Coin] = curr
		}
		return result, nil
	}
}

func makeFetchMarkets(e *Bybit) banexg.FuncFetchMarkets {
	return func(marketTypes []string, params map[string]interface{}) (banexg.MarketMap, *errs.Error) {
		var result = make(banexg.MarketMap)
		var lock sync.Mutex
		var outErr *errs.Error
		var wg sync.WaitGroup
		wg.Add(len(marketTypes))
		for _, mkt := range marketTypes {
			go func(market string) {
				defer wg.Done()
				var markets banexg.MarketMap
				var err *errs.Error
				args := utils.SafeParams(params)
				if market == banexg.MarketSpot {
					markets, err = e.fetchSpotMarkets(args)
				} else if market == banexg.MarketLinear {
					args["category"] = "linear"
					markets, err = e.fetchFutureMarkets(args)
				} else if market == banexg.MarketInverse {
					args["category"] = "inverse"
					markets, err = e.fetchFutureMarkets(args)
				} else if market == banexg.MarketOption {
					markets, err = e.fetchOptionMarkets(args)
				} else {
					err = errs.NewMsg(errs.CodeParamInvalid, "unsupported market: %v", market)
				}
				lock.Lock()
				if err != nil {
					outErr = err
				} else {
					for key, m := range markets {
						result[key] = m
					}
				}
				lock.Unlock()
			}(mkt)
		}
		wg.Wait()
		return result, outErr
	}
}

func (e *Bybit) fetchSpotMarkets(params map[string]interface{}) (banexg.MarketMap, *errs.Error) {
	params["category"] = "spot"
	list, arr, _, _, err := getMarkets[*SpotMarket](e, MethodPublicGetV5MarketInstrumentsInfo, params)
	if err != nil {
		return nil, err
	}
	var result = make(banexg.MarketMap)
	for i, it := range arr {
		var amtPrec, pricePrec float64
		var minOrderQty, maxOrderQty float64
		var minOrderAmt, maxOrderAmt float64
		if it.LotSizeFilter != nil {
			amtPrec, _ = strconv.ParseFloat(it.LotSizeFilter.BasePrecision, 64)
			minOrderQty, _ = strconv.ParseFloat(it.LotSizeFilter.MinOrderQty, 64)
			maxOrderQty, _ = strconv.ParseFloat(it.LotSizeFilter.MaxOrderQty, 64)
			minOrderAmt, _ = strconv.ParseFloat(it.LotSizeFilter.MinOrderAmt, 64)
			maxOrderAmt, _ = strconv.ParseFloat(it.LotSizeFilter.MaxOrderAmt, 64)
		}
		if it.PriceFilter != nil {
			pricePrec, _ = strconv.ParseFloat(it.PriceFilter.TickSize, 64)
		}
		mar := it.BaseMarket.ToStdMarket(e)
		mar.Spot = true
		mar.Type = banexg.MarketSpot
		mar.Margin = it.MarginTrading != "none"
		mar.Taker = e.Fees.Main.Taker
		mar.Maker = e.Fees.Main.Maker
		mar.FeeSide = "get"
		mar.Precision = &banexg.Precision{
			Amount:     amtPrec,
			ModeAmount: banexg.PrecModeTickSize,
			Price:      pricePrec,
			ModePrice:  banexg.PrecModeTickSize,
		}
		mar.Limits = &banexg.MarketLimits{
			Leverage: &banexg.LimitRange{
				Min: 1,
			},
			Amount: &banexg.LimitRange{
				Min: minOrderQty,
				Max: maxOrderQty,
			},
			Price: &banexg.LimitRange{},
			Cost: &banexg.LimitRange{
				Min: minOrderAmt,
				Max: maxOrderAmt,
			},
		}
		mar.Info = list[i]
		result[mar.Symbol] = mar
	}
	return result, nil
}

func getMarkets[T any](e *Bybit, method string, params map[string]interface{}) ([]map[string]interface{}, []T, string, string, *errs.Error) {
	tryNum := e.GetRetryNum("FetchMarkets", 1)
	rsp := requestRetry[struct {
		Category       string                   `json:"category"`
		List           []map[string]interface{} `json:"list"`
		NextPageCursor string                   `json:"nextPageCursor"`
	}](e, method, params, tryNum)
	if rsp.Error != nil {
		return nil, nil, "", "", rsp.Error
	}
	var res = rsp.Result
	var arr []T
	if len(res.List) > 0 {
		err_ := utils.DecodeStructMap(res.List, &arr, "json")
		if err_ != nil {
			return nil, nil, "", "", errs.New(errs.CodeUnmarshalFail, err_)
		}
	}
	return res.List, arr, res.Category, res.NextPageCursor, nil
}

func getMarketsLoop[T any](e *Bybit, method string, params map[string]interface{}) ([]map[string]interface{}, []T, string, *errs.Error) {
	if _, ok := params[banexg.ParamLimit]; !ok {
		params[banexg.ParamLimit] = 1000
	}
	var category string
	var items []map[string]interface{}
	var arrList []T
	for {
		list, arr, cate, cursor, err := getMarkets[T](e, method, params)
		if err != nil {
			return nil, nil, "", err
		}
		items = append(items, list...)
		arrList = append(arrList, arr...)
		category = cate
		if cursor == "" {
			break
		}
		params["cursor"] = cursor
	}
	return items, arrList, category, nil
}

/*
https://bybit-exchange.github.io/docs/v5/market/instrument
*/
func (e *Bybit) fetchFutureMarkets(params map[string]interface{}) (banexg.MarketMap, *errs.Error) {
	method := MethodPublicGetV5MarketInstrumentsInfo
	items, arr, category, err := getMarketsLoop[*FutureMarket](e, method, params)
	if err != nil {
		return nil, err
	}
	var result = make(banexg.MarketMap)
	isLinear := category == "linear"
	isInverse := category == "inverse"
	for i, it := range arr {
		mar := it.ContractMarket.ToStdMarket(e)
		linearPerpetual := it.ContractType == "LinearPerpetual"
		if mar.SettleID == "" {
			if isLinear {
				mar.SettleID = mar.QuoteID
			} else {
				mar.SettleID = mar.BaseID
			}
			var settle string
			if linearPerpetual && mar.SettleID == "USD" {
				settle = "USDC"
			} else {
				settle = e.SafeCurrency(mar.SettleID).Code
			}
			symbol := mar.Symbol + ":" + settle
			deliveryTime, _ := strconv.ParseInt(it.DeliveryTime, 10, 64)
			if deliveryTime > 0 {
				symbol += "-" + utils.YMD(deliveryTime, "", false)
			}
			mar.Symbol = symbol
			mar.Settle = settle
		}
		if isLinear {
			mar.Type = banexg.MarketLinear
		} else {
			mar.Type = banexg.MarketInverse
		}
		mar.Swap = linearPerpetual || it.ContractType == "InversePerpetual"
		mar.Future = it.ContractType == "LinearFutures" || it.ContractType == "InverseFutures"
		mar.Linear = isLinear
		mar.Inverse = isInverse
		fee := e.Fees.Linear
		if isInverse {
			fee = e.Fees.Inverse
			mar.FeeSide = "base"
		} else {
			mar.FeeSide = "quote"
		}
		mar.Taker = fee.Taker
		mar.Maker = fee.Maker
		minOrderQty, maxOrderQty, lotQtyStep := it.LotSizeFilter.parse()
		if isInverse {
			mar.ContractSize = minOrderQty
		}
		mar.Precision.Amount = lotQtyStep
		mar.Precision.ModeAmount = banexg.PrecModeTickSize
		var lvgMin, lvgMax float64
		if it.LeverageFilter != nil {
			lvgMin, _ = strconv.ParseFloat(it.LeverageFilter.MinLeverage, 64)
			lvgMax, _ = strconv.ParseFloat(it.LeverageFilter.MaxLeverage, 64)
		}
		mar.Limits.Leverage = &banexg.LimitRange{
			Min: lvgMin,
			Max: lvgMax,
		}
		mar.Limits.Amount = &banexg.LimitRange{
			Min: minOrderQty,
			Max: maxOrderQty,
		}
		mar.Info = items[i]
		result[mar.Symbol] = mar
	}
	return result, nil
}

func (e *Bybit) fetchOptionMarkets(params map[string]interface{}) (banexg.MarketMap, *errs.Error) {
	params["category"] = "option"
	method := MethodPublicGetV5MarketInstrumentsInfo
	items, arr, _, err := getMarketsLoop[*OptionMarket](e, method, params)
	if err != nil {
		return nil, err
	}
	var result = make(banexg.MarketMap)
	for i, it := range arr {
		mar := it.ContractMarket.ToStdMarket(e)
		codeArr := strings.Split(it.Symbol, "-")
		mar.Symbol = fmt.Sprintf("%s-%s-%s", mar.Symbol, codeArr[2], codeArr[3])
		mar.Type = banexg.MarketOption
		mar.Option = true
		mar.Taker = e.Fees.Option.Taker
		mar.Maker = e.Fees.Option.Maker
		mar.FeeSide = "quote"
		mar.Strike, _ = strconv.ParseFloat(codeArr[2], 64)
		mar.OptionType = it.OptionsType
		minOrderQty, maxOrderQty, lotQtyStep := it.LotSizeFilter.parse()
		mar.Precision.Amount = lotQtyStep
		mar.Precision.ModeAmount = banexg.PrecModeTickSize
		mar.Limits.Amount = &banexg.LimitRange{
			Min: minOrderQty,
			Max: maxOrderQty,
		}
		mar.Limits.Leverage = &banexg.LimitRange{}
		mar.Info = items[i]
		result[mar.Symbol] = mar
	}
	return result, nil
}

func (m *BaseMarket) ToStdMarket(e *Bybit) *banexg.Market {
	var baseCode = e.SafeCurrency(m.BaseCoin).Code
	var quote = e.SafeCurrency(m.QuoteCoin).Code
	var symbol = fmt.Sprintf("%s/%s", baseCode, quote)
	return &banexg.Market{
		ID:          m.Symbol,
		LowercaseID: strings.ToLower(m.Symbol),
		Symbol:      symbol,
		Base:        baseCode,
		Quote:       quote,
		BaseID:      m.BaseCoin,
		QuoteID:     m.QuoteCoin,
		Active:      m.Status == "Trading",
	}
}

func (m *ContractMarket) ToStdMarket(e *Bybit) *banexg.Market {
	mar := m.BaseMarket.ToStdMarket(e)
	settleId := m.SettleCoin
	var settle = e.SafeCurrency(settleId).Code
	symbol := mar.Symbol + ":" + settle
	deliveryTime, _ := strconv.ParseInt(m.DeliveryTime, 10, 64)
	if deliveryTime > 0 {
		symbol += "-" + utils.YMD(deliveryTime, "", false)
	}
	mar.Symbol = symbol
	mar.Settle = settle
	mar.SettleID = settleId
	mar.Contract = true
	mar.Future = deliveryTime > 0
	var priceTickSize, priceMin, priceMax float64
	if m.PriceFilter != nil {
		priceTickSize, _ = strconv.ParseFloat(m.PriceFilter.TickSize, 64)
		priceMin, _ = strconv.ParseFloat(m.PriceFilter.MinPrice, 64)
		priceMax, _ = strconv.ParseFloat(m.PriceFilter.MaxPrice, 64)
	}
	mar.ContractSize = 1
	mar.Expiry = deliveryTime
	mar.ExpiryDatetime = utils.ISO8601(deliveryTime)
	mar.Precision = &banexg.Precision{
		Price:     priceTickSize,
		ModePrice: banexg.PrecModeTickSize,
	}
	mar.Limits = &banexg.MarketLimits{
		Price: &banexg.LimitRange{
			Min: priceMin,
			Max: priceMax,
		},
		Cost: &banexg.LimitRange{},
	}
	mar.Created, _ = strconv.ParseInt(m.LaunchTime, 10, 64)
	return mar
}

/*
return minOrderQty, maxOrderQty, lotQtyStep
*/
func (f *OptionLotSizeFt) parse() (float64, float64, float64) {
	var minOrderQty, maxOrderQty, lotQtyStep float64
	if f != nil {
		minOrderQty, _ = strconv.ParseFloat(f.MinOrderQty, 64)
		maxOrderQty, _ = strconv.ParseFloat(f.MaxOrderQty, 64)
		lotQtyStep, _ = strconv.ParseFloat(f.QtyStep, 64)
	}
	return minOrderQty, maxOrderQty, lotQtyStep
}

func (e *Bybit) FetchOHLCV(symbol, timeframe string, since int64, limit int, params map[string]interface{}) ([]*banexg.Kline, *errs.Error) {
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	args["symbol"] = market.ID
	if limit <= 0 {
		limit = 200
	}
	args["limit"] = limit
	if since > 0 {
		args["start"] = since
	}
	until := utils.PopMapVal(args, banexg.ParamUntil, int64(0))
	if until > 0 {
		args["end"] = until
	}
	args["interval"] = utils.TFToSecs(timeframe) / 60
	var method string
	if market.Spot {
		args["categoty"] = "spot"
		method = MethodPublicGetV5MarketKline
	} else {
		price := utils.PopMapVal(args, "price", "")
		if market.Linear {
			args["categoty"] = "linear"
		} else if market.Inverse {
			args["categoty"] = "inverse"
		} else {
			return nil, errs.NewMsg(errs.CodeParamInvalid, "not support market: %v", market.Type)
		}
		if price == "mark" {
			method = MethodPublicGetV5MarketMarkPriceKline
		} else if price == "index" {
			method = MethodPublicGetV5MarketIndexPriceKline
		} else if price == "premiumIndex" {
			method = MethodPublicGetV5MarketPremiumIndexPriceKline
		} else {
			method = MethodPublicGetV5MarketKline
		}
	}
	tryNum := e.GetRetryNum("FetchOHLCV", 1)
	rsp := requestRetry[struct {
		Symbol   string     `json:"symbol"`
		Category string     `json:"category"`
		List     [][]string `json:"list"`
	}](e, method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	var res = make([]*banexg.Kline, 0, len(rsp.Result.List))
	for _, row := range rsp.Result.List {
		var arr = make([]float64, 0, len(row)-1)
		stamp, _ := strconv.ParseInt(row[0], 10, 64)
		for _, str := range row[1:] {
			val, _ := strconv.ParseFloat(str, 64)
			arr = append(arr, val)
		}
		kline := &banexg.Kline{
			Time:  stamp,
			Open:  arr[0],
			High:  arr[1],
			Low:   arr[2],
			Close: arr[3],
		}
		if len(arr) >= 6 {
			kline.Volume = arr[4]
			kline.Info = arr[5]
		}
	}
	return res, nil
}

const maxFundRateBatch = 200 // 一次最多返回200个

func (e *Bybit) FetchFundingRateHistory(symbol string, since int64, limit int, params map[string]interface{}) ([]*banexg.FundingRate, *errs.Error) {
	if symbol == "" {
		return nil, errs.NewMsg(errs.CodeParamRequired, "symbol is required for bybit FetchFundingRateHistory")
	}
	if limit <= 0 {
		limit = maxFundRateBatch
	}
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, err
	}
	if market.Spot || market.Option {
		return nil, errs.NewMsg(errs.CodeNotSupport, "only linear/inverse market support")
	}
	args["limit"] = min(limit, maxFundRateBatch)
	args["symbol"] = market.ID
	args["category"] = market.Type
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
		list, hasMore, err := e.getFundRateHis(market.Type, until, args)
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

func (e *Bybit) getFundRateHis(marketType string, until int64, args map[string]interface{}) ([]*banexg.FundingRate, bool, *errs.Error) {
	method := MethodPublicGetV5MarketFundingHistory
	tryNum := e.GetRetryNum("FetchFundingRateHistory", 1)
	rsp := requestRetry[struct {
		Category string                   `json:"category"`
		List     []map[string]interface{} `json:"list"`
	}](e, method, args, tryNum)
	if rsp.Error != nil {
		return nil, false, rsp.Error
	}
	var arr = rsp.Result.List
	var items = make([]*FundRate, 0, len(arr))
	err := utils.DecodeStructMap(arr, &items, "json")
	if err != nil {
		return nil, false, errs.New(errs.CodeUnmarshalFail, err)
	}
	var lastMS int64
	var list = make([]*banexg.FundingRate, 0, len(rsp.Result.List))
	for i, it := range items {
		code := e.SafeSymbol(it.Symbol, "", marketType)
		stamp, _ := strconv.ParseInt(it.FundingRateTimestamp, 10, 64)
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
			Info:        arr[i],
		})
	}
	interval := int64(60 * 60 * 8 * 1000)
	hasMore := until > 0 && len(rsp.Result.List) == maxFundRateBatch && lastMS+interval < until
	return list, hasMore, nil
}
