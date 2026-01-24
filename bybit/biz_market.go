package bybit

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
	"github.com/sasha-s/go-deadlock"
)

func (e *Bybit) Init() *errs.Error {
	err := e.Exchange.Init()
	if err != nil {
		return err
	}
	utils.SetFieldBy(&e.RecvWindow, e.Options, banexg.OptRecvWindow, 30000)
	if len(e.CareMarkets) == 0 {
		e.CareMarkets = banexg.DefaultCareMarkets()
	}
	e.ExgInfo.NoHoliday = true
	e.ExgInfo.FullDay = true
	e.OnWsMsg = makeHandleWsMsg(e)
	e.OnWsReCon = makeHandleWsReCon(e)
	e.WsAuthed = map[string]bool{}
	e.WsAuthDone = map[string]chan *errs.Error{}
	e.WsPendingRecons = map[string]*WsPendingRecon{}
	e.regReplayHandles()
	markRiskyApis(e)
	return nil
}

func markRiskyApis(e *Bybit) {
	riskyPaths := []string{
		"order", "cancel", "batch", "leverage", "margin",
		"position/set", "position/switch", "position/trading",
		"transfer", "withdraw", "loan", "repay",
	}
	for _, api := range e.Apis {
		if api.Method == "GET" {
			continue
		}
		for _, rp := range riskyPaths {
			if strings.Contains(api.Path, rp) {
				api.Risky = true
				break
			}
		}
	}
}

func makeSign(e *Bybit) banexg.FuncSign {
	return func(api *banexg.Entry, args map[string]interface{}) *banexg.HttpReq {
		var params = utils.SafeParams(args)
		accID := e.PopAccName(params)
		if err := e.CheckRiskyAllowed(api, accID); err != nil {
			return &banexg.HttpReq{Error: err, Private: true}
		}
		url := api.Url
		headers := http.Header{}
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
	noCache := utils.PopMapVal(params, banexg.ParamNoCache, false)
	res_ := e.RequestApiRetryAdv(context.Background(), api, params, tryNum, !noCache, false)
	res := &banexg.ApiRes[T]{HttpRes: res_}
	if res.Error != nil {
		return res
	}
	var rsp V5Resp[T]
	err := utils.UnmarshalString(res.Content, &rsp, utils.JsonNumDefault)
	if err != nil {
		res.Error = errs.New(errs.CodeUnmarshalFail, err)
		return res
	}
	if rsp.RetCode != 0 {
		res.Error = mapBybitRetCode(rsp.RetCode, rsp.RetMsg)
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
		if ccy := utils.PopMapVal(params, banexg.ParamCurrency, ""); ccy != "" {
			if _, ok := params["coin"]; !ok {
				params["coin"] = ccy
			}
		}
		res := requestRetry[struct {
			Rows []map[string]interface{} `json:"rows"`
		}](e, MethodPrivateGetV5AssetCoinQueryInfo, params, tryNum)
		if res.Error != nil {
			return nil, res.Error
		}
		var currList = res.Result
		currArr, err := decodeBybitList[*Currency](currList.Rows)
		if err != nil {
			return nil, err
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
		var lock deadlock.Mutex
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
	delete(params, banexg.ParamLimit)
	delete(params, banexg.ParamAfter)
	delete(params, "cursor")
	list, arr, _, err := getMarketsLoop[*SpotMarket](e, MethodPublicGetV5MarketInstrumentsInfo, params, 0)
	if err != nil {
		return nil, err
	}
	var result = make(banexg.MarketMap)
	for i, it := range arr {
		amtPrec, minOrderQty, maxOrderQty, minOrderAmt, maxOrderAmt := it.LotSizeFilter.parse()
		var quotePrec float64
		var maxMarketOrderQty float64
		if it.LotSizeFilter != nil {
			quotePrec = parseBybitNum(it.LotSizeFilter.QuotePrecision)
			maxMarketOrderQty = parseBybitNum(it.LotSizeFilter.MaxMarketOrderQty)
		}
		var pricePrec float64
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
			Base:       amtPrec,
			ModeBase:   banexg.PrecModeTickSize,
			Quote:      quotePrec,
			ModeQuote:  banexg.PrecModeTickSize,
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
		if maxMarketOrderQty > 0 {
			mar.Limits.Market = &banexg.LimitRange{
				Min: minOrderQty,
				Max: maxMarketOrderQty,
			}
		}
		mar.Info = list[i]
		result[mar.Symbol] = mar
	}
	return result, nil
}

func getMarkets[T any](e *Bybit, method string, params map[string]interface{}) ([]map[string]interface{}, []T, string, string, *errs.Error) {
	tryNum := e.GetRetryNum("FetchMarkets", 1)
	rsp := requestRetry[V5ListResult](e, method, params, tryNum)
	if rsp.Error != nil {
		return nil, nil, "", "", rsp.Error
	}
	var res = rsp.Result
	arr, err := decodeBybitList[T](res.List)
	if err != nil {
		return nil, nil, "", "", err
	}
	return res.List, arr, res.Category, res.NextPageCursor, nil
}

func getMarketsLoop[T any](e *Bybit, method string, params map[string]interface{}, defaultLimit int) ([]map[string]interface{}, []T, string, *errs.Error) {
	if defaultLimit > 0 {
		if _, ok := params[banexg.ParamLimit]; !ok {
			params[banexg.ParamLimit] = defaultLimit
		}
	}
	var category string
	var items []map[string]interface{}
	var arrList []T
	cursor := popV5Cursor(params)
	for {
		setV5Cursor(params, cursor)
		list, arr, cate, nextCursor, err := getMarkets[T](e, method, params)
		if err != nil {
			return nil, nil, "", err
		}
		items = append(items, list...)
		arrList = append(arrList, arr...)
		category = cate
		cursor = nextCursor
		if cursor == "" {
			break
		}
	}
	return items, arrList, category, nil
}

func (e *Bybit) fetchFutureMarkets(params map[string]interface{}) (banexg.MarketMap, *errs.Error) {
	method := MethodPublicGetV5MarketInstrumentsInfo
	items, arr, category, err := getMarketsLoop[*FutureMarket](e, method, params, 1000)
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
		minOrderQty, maxOrderQty, lotQtyStep, maxMarketOrderQty := it.LotSizeFilter.parse()
		if maxOrderQty == 0 {
			maxOrderQty = maxMarketOrderQty
		}
		var minNotional float64
		if it.LotSizeFilter != nil {
			minNotional = parseBybitNum(it.LotSizeFilter.MinNotionalValue)
		}
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
		if maxMarketOrderQty > 0 {
			mar.Limits.Market = &banexg.LimitRange{
				Min: minOrderQty,
				Max: maxMarketOrderQty,
			}
		}
		if minNotional > 0 {
			mar.Limits.Cost.Min = minNotional
		}
		mar.Info = items[i]
		result[mar.Symbol] = mar
	}
	return result, nil
}

func (e *Bybit) fetchOptionMarkets(params map[string]interface{}) (banexg.MarketMap, *errs.Error) {
	params["category"] = "option"
	method := MethodPublicGetV5MarketInstrumentsInfo
	items, arr, _, err := getMarketsLoop[*OptionMarket](e, method, params, 1000)
	if err != nil {
		return nil, err
	}
	var result = make(banexg.MarketMap)
	for i, it := range arr {
		mar := it.ContractMarket.ToStdMarket(e)
		codeArr := strings.Split(it.Symbol, "-")
		symbol := mar.Symbol
		expiry := parseBybitInt(it.DeliveryTime)
		if expiry <= 0 && len(codeArr) > 1 {
			symbol = symbol + "-" + codeArr[1]
		}
		if len(codeArr) >= 4 {
			symbol = fmt.Sprintf("%s-%s-%s", symbol, codeArr[2], codeArr[3])
		} else if len(codeArr) >= 3 {
			symbol = fmt.Sprintf("%s-%s", symbol, codeArr[2])
		}
		mar.Symbol = symbol
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

func minPositive(vals ...float64) float64 {
	var res float64
	for _, val := range vals {
		if val <= 0 {
			continue
		}
		if res == 0 || val < res {
			res = val
		}
	}
	return res
}

func (f *LotSizeFt) parse() (float64, float64, float64, float64, float64) {
	if f == nil {
		return 0, 0, 0, 0, 0
	}
	amtPrec := parseBybitNum(f.BasePrecision)
	minOrderQty := parseBybitNum(f.MinOrderQty)
	maxOrderQty := parseBybitNum(f.MaxOrderQty)
	minOrderAmt := parseBybitNum(f.MinOrderAmt)
	maxOrderAmt := parseBybitNum(f.MaxOrderAmt)
	maxLimitQty := parseBybitNum(f.MaxLimitOrderQty)
	maxMarketQty := parseBybitNum(f.MaxMarketOrderQty)
	maxQty := minPositive(maxLimitQty, maxMarketQty)
	if maxQty > 0 {
		maxOrderQty = maxQty
	}
	return amtPrec, minOrderQty, maxOrderQty, minOrderAmt, maxOrderAmt
}

func (f *OptionLotSizeFt) parse() (float64, float64, float64) {
	if f == nil {
		return 0, 0, 0
	}
	minOrderQty := parseBybitNum(f.MinOrderQty)
	maxOrderQty := parseBybitNum(f.MaxOrderQty)
	lotQtyStep := parseBybitNum(f.QtyStep)
	return minOrderQty, maxOrderQty, lotQtyStep
}

func (f *FutureLotSizeFt) parse() (float64, float64, float64, float64) {
	if f == nil {
		return 0, 0, 0, 0
	}
	minOrderQty := parseBybitNum(f.MinOrderQty)
	maxOrderQty := parseBybitNum(f.MaxOrderQty)
	lotQtyStep := parseBybitNum(f.QtyStep)
	maxMarketOrderQty := parseBybitNum(f.MaxMktOrderQty)
	return minOrderQty, maxOrderQty, lotQtyStep, maxMarketOrderQty
}
