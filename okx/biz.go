package okx

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

func (e *OKX) Init() *errs.Error {
	err := e.Exchange.Init()
	if err != nil {
		return err
	}
	if len(e.CareMarkets) == 0 {
		e.CareMarkets = DefCareMarkets
	}
	e.ExgInfo.NoHoliday = true
	e.ExgInfo.FullDay = true
	markRiskyApis(e)
	return nil
}

func markRiskyApis(e *OKX) {
	riskyPaths := []string{
		"order", "cancel", "amend", "leverage", "margin",
		"position", "transfer", "withdraw", "loan", "repay",
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

func makeSign(e *OKX) banexg.FuncSign {
	return func(api *banexg.Entry, args map[string]interface{}) *banexg.HttpReq {
		params := utils.SafeParams(args)
		accID := e.PopAccName(params)
		if err := e.CheckRiskyAllowed(api, accID); err != nil {
			return &banexg.HttpReq{Error: err, Private: true}
		}
		url := api.Url
		headers := http.Header{}
		body := ""
		isPrivate := api.Host == HostPrivate

		if !isPrivate && len(params) > 0 {
			url += "?" + utils.UrlEncodeMap(params, true)
		} else if isPrivate {
			var creds *banexg.Credential
			var err *errs.Error
			accID, creds, err = e.GetAccountCreds(accID)
			if err != nil {
				return &banexg.HttpReq{Error: err, Private: true}
			}
			passphrase := creds.Password
			timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
			requestPath := api.Path
			if api.Method == "GET" && len(params) > 0 {
				queryStr := utils.UrlEncodeMap(params, true)
				url += "?" + queryStr
				requestPath += "?" + queryStr
			} else if api.Method == "POST" && len(params) > 0 {
				if api.Path == "trade/cancel-algos" {
					body, _ = utils.MarshalString([]map[string]interface{}{params})
				} else {
					body, _ = utils.MarshalString(params)
				}
			}
			payload := timestamp + api.Method + "/api/v5/" + requestPath + body
			sign, _ := utils.Signature(payload, creds.Secret, "hmac", "sha256", "base64")

			headers.Set("OK-ACCESS-KEY", creds.ApiKey)
			headers.Set("OK-ACCESS-SIGN", sign)
			headers.Set("OK-ACCESS-TIMESTAMP", timestamp)
			headers.Set("OK-ACCESS-PASSPHRASE", passphrase)
			if api.Method == "POST" {
				headers.Set("Content-Type", "application/json")
			}
		}
		return &banexg.HttpReq{AccName: accID, Url: url, Method: api.Method, Headers: headers, Body: body, Private: isPrivate}
	}
}

func requestRetry[T any](e *OKX, api string, params map[string]interface{}, tryNum int) *banexg.ApiRes[T] {
	noCache := utils.PopMapVal(params, banexg.ParamNoCache, false)
	res_ := e.RequestApiRetryAdv(context.Background(), api, params, tryNum, !noCache, false)
	res := &banexg.ApiRes[T]{HttpRes: res_}
	if res.Error != nil {
		return res
	}
	var rsp = struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data T      `json:"data"`
	}{}
	err := utils.UnmarshalString(res.Content, &rsp, utils.JsonNumDefault)
	if err != nil {
		res.Error = errs.New(errs.CodeUnmarshalFail, err)
		return res
	}
	if rsp.Code != "0" {
		// Extract detailed error from data[0].sCode/sMsg if available
		errMsg := rsp.Msg
		if detail := extractDetailError(res.Content); detail != "" {
			errMsg = detail
		}
		res.Error = errs.NewMsg(errs.CodeRunTime, "[%s] %s", rsp.Code, errMsg)
	} else {
		res.Result = rsp.Data
		e.CacheApiRes(api, res_)
	}
	return res
}

// extractDetailError extracts detailed error from OKX response's data[0].sCode/sMsg
func extractDetailError(content string) string {
	var resp struct {
		Data []struct {
			SCode string `json:"sCode"`
			SMsg  string `json:"sMsg"`
		} `json:"data"`
	}
	if utils.UnmarshalString(content, &resp, utils.JsonNumDefault) == nil {
		if len(resp.Data) > 0 && resp.Data[0].SMsg != "" {
			return "[" + resp.Data[0].SCode + "] " + resp.Data[0].SMsg
		}
	}
	return ""
}

func makeFetchMarkets(e *OKX) banexg.FuncFetchMarkets {
	return func(marketTypes []string, params map[string]interface{}) (banexg.MarketMap, *errs.Error) {
		result := make(banexg.MarketMap)
		if len(marketTypes) == 0 {
			return result, nil
		}
		instTypes := collectInstTypes(marketTypes)
		tryNum := e.GetRetryNum("FetchMarkets", 1)
		for _, instType := range instTypes {
			res := requestRetry[[]Instrument](e, MethodPublicGetInstruments, map[string]interface{}{
				"instType": instType,
			}, tryNum)
			if res.Error != nil {
				return nil, res.Error
			}
			for _, inst := range res.Result {
				market := parseInstrument(e, &inst)
				applyMarketFees(e, market)
				result[market.Symbol] = market
			}
		}
		return result, nil
	}
}

func collectInstTypes(marketTypes []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(marketTypes))
	for _, mkt := range marketTypes {
		instType := marketToInstType[mkt]
		if instType == "" {
			continue
		}
		if _, ok := seen[instType]; ok {
			continue
		}
		seen[instType] = struct{}{}
		out = append(out, instType)
	}
	sort.Strings(out)
	return out
}

func applyMarketFees(e *OKX, market *banexg.Market) {
	if market == nil || e.Fees == nil {
		return
	}
	if market.Spot || market.Margin {
		if e.Fees.Main != nil {
			market.Taker = e.Fees.Main.Taker
			market.Maker = e.Fees.Main.Maker
			market.FeeSide = e.Fees.Main.FeeSide
		}
		return
	}
	if market.Contract {
		fee := e.Fees.Linear
		if market.Inverse && e.Fees.Inverse != nil {
			fee = e.Fees.Inverse
		}
		if fee != nil {
			market.Taker = fee.Taker
			market.Maker = fee.Maker
			market.FeeSide = fee.FeeSide
		}
	}
}

func parseInstrument(e *OKX, inst *Instrument) *banexg.Market {
	tickSz := parseFloat(inst.TickSz)
	lotSz := parseFloat(inst.LotSz)
	minSz := parseFloat(inst.MinSz)
	ctVal := parseFloat(inst.CtVal)
	created := parseInt(inst.ListTime)

	mktType := parseMarketType(inst.InstType, inst.CtType)
	isSwap := inst.InstType == InstTypeSwap
	isFuture := inst.InstType == InstTypeFutures
	isOption := inst.InstType == InstTypeOption
	isSpot := inst.InstType == InstTypeSpot
	isMargin := inst.InstType == InstTypeMargin
	symbol := inst.InstId
	if e != nil {
		if mktType == banexg.MarketOption {
			base := e.SafeCurrencyCode(inst.BaseCcy)
			quote := e.SafeCurrencyCode(inst.QuoteCcy)
			if base != "" {
				symbol = base
				if quote != "" {
					symbol = base + "/" + quote
				}
			}
			settle := e.SafeCurrencyCode(inst.SettleCcy)
			if settle == "" {
				settle = inst.SettleCcy
			}
			if settle != "" && symbol != "" {
				symbol = symbol + ":" + settle
			}
			parts := strings.Split(inst.InstId, "-")
			expTime := parseInt(inst.ExpTime)
			if expTime > 0 {
				symbol = symbol + "-" + utils.YMD(expTime, "", false)
			} else if len(parts) >= 3 {
				symbol = symbol + "-" + parts[2]
			}
			if len(parts) >= 4 {
				symbol = symbol + "-" + parts[3]
			}
			if len(parts) >= 5 {
				symbol = symbol + "-" + parts[4]
			}
		} else if mktType == banexg.MarketLinear || mktType == banexg.MarketInverse {
			base := e.SafeCurrencyCode(inst.BaseCcy)
			quote := e.SafeCurrencyCode(inst.QuoteCcy)
			// For derivatives, OKX may not set baseCcy/quoteCcy; parse from instFamily or uly instead
			if base == "" || quote == "" {
				familyOrUly := inst.InstFamily
				if familyOrUly == "" {
					familyOrUly = inst.Uly
				}
				if familyOrUly != "" {
					parts := strings.Split(familyOrUly, "-")
					if len(parts) >= 2 {
						if base == "" {
							base = e.SafeCurrencyCode(parts[0])
						}
						if quote == "" {
							quote = e.SafeCurrencyCode(parts[1])
						}
					}
				}
			}
			if base != "" {
				symbol = base
				if quote != "" {
					symbol = base + "/" + quote
				}
			}
			settle := e.SafeCurrencyCode(inst.SettleCcy)
			if settle == "" {
				settle = inst.SettleCcy
			}
			if settle != "" && symbol != "" {
				symbol = symbol + ":" + settle
			}
			// For futures (not perpetual swaps), append the expiry date
			if isFuture && inst.InstId != "" {
				parts := strings.Split(inst.InstId, "-")
				if len(parts) >= 3 {
					expTime := parseInt(inst.ExpTime)
					if expTime > 0 {
						symbol = symbol + "-" + utils.YMD(expTime, "", false)
					} else {
						symbol = symbol + "-" + parts[2]
					}
				}
			}
		} else if isSpot || isMargin {
			base := e.SafeCurrencyCode(inst.BaseCcy)
			quote := e.SafeCurrencyCode(inst.QuoteCcy)
			if base != "" && quote != "" {
				symbol = base + "/" + quote
			}
		}
	}

	var info map[string]interface{}
	if len(inst.TradeQuoteCcyList) > 0 {
		ccyMap := make(map[string]bool, len(inst.TradeQuoteCcyList))
		for _, ccy := range inst.TradeQuoteCcyList {
			ccyMap[ccy] = true
		}
		info = map[string]interface{}{
			"tradeQuoteCcyList": ccyMap,
		}
	}
	return &banexg.Market{
		ID:           inst.InstId,
		Symbol:       symbol,
		Base:         inst.BaseCcy,
		Quote:        inst.QuoteCcy,
		Settle:       inst.SettleCcy,
		Type:         mktType,
		Spot:         isSpot,
		Margin:       isMargin,
		Swap:         isSwap,
		Future:       isFuture,
		Option:       isOption,
		Contract:     isSwap || isFuture || isOption,
		Linear:       inst.CtType == "linear",
		Inverse:      inst.CtType == "inverse",
		ContractSize: ctVal,
		Precision: &banexg.Precision{
			Price:      tickSz,
			ModePrice:  banexg.PrecModeTickSize,
			Amount:     lotSz,
			ModeAmount: banexg.PrecModeTickSize,
		},
		Limits: &banexg.MarketLimits{
			Amount:   &banexg.LimitRange{Min: minSz},
			Leverage: &banexg.LimitRange{},
			Price:    &banexg.LimitRange{},
			Cost:     &banexg.LimitRange{},
			Market:   &banexg.LimitRange{},
		},
		Active:  inst.State == "live",
		Created: created,
		Info:    info,
	}
}
