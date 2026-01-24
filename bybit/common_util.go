package bybit

import (
	"encoding/json"
	"strings"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"go.uber.org/zap"
)

type bybitAccountInfo struct {
	MarginMode          string `json:"marginMode"`
	UnifiedMarginStatus int    `json:"unifiedMarginStatus"`
	SpotHedgingStatus   string `json:"spotHedgingStatus"`
}

type bybitApiKeyInfo struct {
	ReadOnly    int                 `json:"readOnly"`
	Permissions map[string][]string `json:"permissions"`
	Ips         []string            `json:"ips"`
}

// V5Resp is the common V5 response wrapper.
type V5Resp[T any] struct {
	RetCode    int             `json:"retCode"`
	RetMsg     string          `json:"retMsg"`
	Result     T               `json:"result"`
	RetExtInfo json.RawMessage `json:"retExtInfo"`
	Time       BybitTime       `json:"time"`
}

// V5ListResult is the common list container in V5 results.
type V5ListResult struct {
	Category       string                   `json:"category"`
	List           []map[string]interface{} `json:"list"`
	NextPageCursor string                   `json:"nextPageCursor"`
}

// BybitTime handles millisecond timestamps as string or number.
type BybitTime int64

func (t *BybitTime) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	var val interface{}
	if err := json.Unmarshal(b, &val); err != nil {
		return err
	}
	parsed, err := utils.ParseInt64(val)
	if err != nil {
		return err
	}
	*t = BybitTime(parsed)
	return nil
}

func parseBybitNum(val interface{}) float64 {
	num, err := utils.ParseNum(val)
	if err != nil {
		return 0
	}
	return num
}

func parseBybitPct(val interface{}) float64 {
	num := parseBybitNum(val)
	if num == 0 {
		return 0
	}
	return num / 100
}

func parseBybitInt(val interface{}) int64 {
	num, err := utils.ParseInt64(val)
	if err != nil {
		return 0
	}
	return num
}

func decodeBybitList[T any](items []map[string]interface{}) ([]T, *errs.Error) {
	if len(items) == 0 {
		return []T{}, nil
	}
	var arr []T
	if err := utils.DecodeStructMap(items, &arr, "json"); err != nil {
		return nil, errs.New(errs.CodeUnmarshalFail, err)
	}
	return arr, nil
}

func mapBybitRetCode(retCode int, retMsg string) *errs.Error {
	if retCode == 0 {
		return nil
	}
	code := errs.CodeRunTime
	switch retCode {
	case -1, 10002:
		code = errs.CodeExpired
	case 10000:
		code = errs.CodeTimeout
	case 10001, 10029, 110003, 110017, 110018, 110019, 110032, 110049, 110072, 110092, 110093, 110094, 110108, 110109, 110120, 110121:
		code = errs.CodeParamInvalid
	case 10003, 33004, -2015:
		code = errs.CodeAccKeyError
	case 10004:
		code = errs.CodeSignFail
	case 10005, 10007:
		code = errs.CodeUnauthorized
	case 10006, 10429, 20003, 429:
		code = errs.CodeSystemBusy
	case 10008, 10009, 10010, 10024, 10028, 100028:
		code = errs.CodeForbidden
	case 10014, 10017, 10404, 20006, 110005, 110008, 110009, 110010, 110015, 110024, 110028, 110029, 110033, 110036, 110038, 110041, 110043:
		code = errs.CodeInvalidRequest
	case 10016:
		code = errs.CodeServerError
	case 10027:
		code = errs.CodeNoTrade
	case 110001, 110031, 110034:
		code = errs.CodeDataNotFound
	case 110004, 110006, 110007, 110011, 110012, 110013, 110014, 110016, 110020, 110021, 110022, 110023, 110039, 110040, 110044, 110045, 110046, 110047, 110048, 110051, 110052, 110053, 110066, 110070, 110074, 170346, 170360:
		code = errs.CodeNoTrade
	case 3100181, 3100326:
		code = errs.CodeParamRequired
	}
	return newBybitErr(code, retCode, retMsg)
}

func newBybitErr(code int, retCode int, retMsg string) *errs.Error {
	err := errs.NewMsg(code, "[%d] %s", retCode, retMsg)
	err.BizCode = retCode
	return err
}

// popV5Cursor reads paging cursor from args["cursor"] or pops banexg.ParamAfter.
// Note: it intentionally consumes ParamAfter to match existing paging behavior.
func popV5Cursor(args map[string]interface{}) string {
	if args == nil {
		return ""
	}
	cursor, _ := args["cursor"].(string)
	if cursor == "" {
		cursor = utils.PopMapVal(args, banexg.ParamAfter, "")
	}
	return cursor
}

func setV5Cursor(args map[string]interface{}, cursor string) {
	if args == nil {
		return
	}
	if cursor != "" {
		args["cursor"] = cursor
	} else {
		delete(args, "cursor")
	}
}

func fetchV5List(e *Bybit, method string, args map[string]interface{}, tryNum int, limit int, maxLimit int) ([]map[string]interface{}, *errs.Error) {
	pageLimit := limit
	if pageLimit <= 0 {
		pageLimit = maxLimit
	} else if maxLimit > 0 && pageLimit > maxLimit {
		pageLimit = maxLimit
	}
	if pageLimit > 0 {
		args["limit"] = pageLimit
	}
	cursor := popV5Cursor(args)
	items := make([]map[string]interface{}, 0)
	for {
		setV5Cursor(args, cursor)
		res := requestRetry[V5ListResult](e, method, args, tryNum)
		if res.Error != nil {
			return nil, res.Error
		}
		if len(res.Result.List) > 0 {
			items = append(items, res.Result.List...)
		}
		if limit > 0 && len(items) >= limit {
			return items[:limit], nil
		}
		if res.Result.NextPageCursor == "" || (pageLimit > 0 && len(res.Result.List) < pageLimit) {
			break
		}
		cursor = res.Result.NextPageCursor
	}
	return items, nil
}

func bybitSafeSymbol(e *Bybit, symbol, marketType string) string {
	if e == nil || symbol == "" {
		return symbol
	}
	if safe := e.SafeSymbol(symbol, "", marketType); safe != "" {
		return safe
	}
	return symbol
}

func bybitSafeCurrency(e *Bybit, currency string) string {
	if e == nil || currency == "" {
		return currency
	}
	return e.SafeCurrencyCode(currency)
}

// FetchAccountAccess queries Bybit account access info and API key permissions.
func (e *Bybit) FetchAccountAccess(params map[string]interface{}) (*banexg.AccountAccess, *errs.Error) {
	args := utils.SafeParams(params)
	res := &banexg.AccountAccess{}
	if bal, ok := args[banexg.ParamBalance].(*banexg.Balances); ok && bal != nil {
		banexg.FillAccountAccessFromInfo(res, bal.Info)
	}
	// Remove internal params that should not be sent to API
	delete(args, banexg.ParamBalance)
	tryNum := e.GetRetryNum("FetchAccountAccess", 1)

	accRes := requestRetry[bybitAccountInfo](e, MethodPrivateGetV5AccountInfo, args, tryNum)
	accOk := accRes.Error == nil
	if accOk {
		applyBybitAccountInfo(res, &accRes.Result)
		if res.Info == nil {
			res.Info = bybitAccountInfoToMap(&accRes.Result)
		}
	}

	apiRes := requestRetry[bybitApiKeyInfo](e, MethodPrivateGetV5UserQueryApi, args, tryNum)
	apiOk := apiRes.Error == nil
	if apiOk {
		applyBybitApiKeyInfo(res, &apiRes.Result)
		if res.Info == nil {
			res.Info = bybitApiKeyInfoToMap(&apiRes.Result)
		}
	}

	if accOk || apiOk || res.HasAny() {
		return res, nil
	}
	if apiRes.Error != nil {
		return res, apiRes.Error
	}
	if accRes.Error != nil {
		return res, accRes.Error
	}
	return res, nil
}

func applyBybitAccountInfo(acc *banexg.AccountAccess, info *bybitAccountInfo) {
	if acc == nil || info == nil {
		return
	}
	switch strings.ToUpper(strings.TrimSpace(info.MarginMode)) {
	case "ISOLATED_MARGIN":
		acc.MarginMode = banexg.MarginIsolated
	case "REGULAR_MARGIN":
		acc.MarginMode = banexg.MarginCross
	case "PORTFOLIO_MARGIN":
		if acc.AcctMode == "" {
			acc.AcctMode = banexg.AcctModePortfolioMargin
		}
	}
}

func applyBybitApiKeyInfo(acc *banexg.AccountAccess, info *bybitApiKeyInfo) {
	if acc == nil || info == nil {
		return
	}
	acc.IPKnown = true
	acc.IPAny = bybitIPAny(info.Ips)

	acc.TradeKnown = true
	if info.ReadOnly == 0 {
		acc.TradeAllowed = bybitHasPerm(info.Permissions, "ContractTrade", "Order", "Position") ||
			bybitHasPerm(info.Permissions, "Spot", "SpotTrade") ||
			bybitHasPerm(info.Permissions, "Options", "OptionsTrade") ||
			bybitHasPerm(info.Permissions, "Derivatives", "DerivativesTrade")
	}

	if info.Permissions != nil {
		acc.WithdrawKnown = true
		acc.WithdrawAllowed = bybitHasPerm(info.Permissions, "Wallet", "Withdraw")
	}
}

func bybitHasPerm(perms map[string][]string, key string, names ...string) bool {
	if perms == nil {
		return false
	}
	items, ok := perms[key]
	if !ok {
		return false
	}
	if len(names) == 0 {
		return len(items) > 0
	}
	for _, item := range items {
		for _, name := range names {
			if strings.EqualFold(item, name) {
				return true
			}
		}
	}
	return false
}

func bybitIPAny(ips []string) bool {
	if len(ips) == 0 {
		return true
	}
	for _, ip := range ips {
		if strings.TrimSpace(ip) == "" || ip == "*" {
			return true
		}
	}
	return false
}

func bybitAccountInfoToMap(info *bybitAccountInfo) map[string]interface{} {
	if info == nil {
		return nil
	}
	return map[string]interface{}{
		"marginMode":          info.MarginMode,
		"unifiedMarginStatus": info.UnifiedMarginStatus,
		"spotHedgingStatus":   info.SpotHedgingStatus,
	}
}

func bybitApiKeyInfoToMap(info *bybitApiKeyInfo) map[string]interface{} {
	if info == nil {
		return nil
	}
	return map[string]interface{}{
		"readOnly":    info.ReadOnly,
		"permissions": info.Permissions,
		"ips":         info.Ips,
	}
}

func (e *Bybit) regReplayHandles() {
	e.WsReplayFn = map[string]func(item *banexg.WsLog) *errs.Error{
		"WatchOrderBooks": func(item *banexg.WsLog) *errs.Error {
			symbols, err := decodeWsLog[[]string](item)
			if err != nil || len(symbols) == 0 {
				return err
			}
			log.Debug("replay WatchOrderBooks", zap.Strings("symbols", symbols))
			_, err = e.WatchOrderBooks(symbols, 0, nil)
			return err
		},
		"WatchTrades": func(item *banexg.WsLog) *errs.Error {
			symbols, err := decodeWsLog[[]string](item)
			if err != nil || len(symbols) == 0 {
				return err
			}
			log.Debug("replay WatchTrades", zap.Strings("symbols", symbols))
			_, err = e.WatchTrades(symbols, nil)
			return err
		},
		"WatchOHLCVs": func(item *banexg.WsLog) *errs.Error {
			jobs, err := decodeWsLog[[][2]string](item)
			if err != nil || len(jobs) == 0 {
				return err
			}
			log.Debug("replay WatchOHLCVs", zap.Int("num", len(jobs)))
			_, err = e.WatchOHLCVs(jobs, nil)
			return err
		},
		"WatchMarkPrices": func(item *banexg.WsLog) *errs.Error {
			symbols, err := decodeWsLog[[]string](item)
			if err != nil || len(symbols) == 0 {
				return err
			}
			log.Debug("replay WatchMarkPrices", zap.Strings("symbols", symbols))
			_, err = e.WatchMarkPrices(symbols, nil)
			return err
		},
		"WatchMyTrades": func(item *banexg.WsLog) *errs.Error {
			log.Debug("replay WatchMyTrades")
			_, err := e.WatchMyTrades(nil)
			return err
		},
		"WatchBalance": func(item *banexg.WsLog) *errs.Error {
			log.Debug("replay WatchBalance")
			_, err := e.WatchBalance(nil)
			return err
		},
		"WatchPositions": func(item *banexg.WsLog) *errs.Error {
			log.Debug("replay WatchPositions")
			_, err := e.WatchPositions(nil)
			return err
		},
		"wsMsg": func(item *banexg.WsLog) *errs.Error {
			arr, err := decodeWsLog[[]string](item)
			if err != nil {
				return err
			}
			if len(arr) < 4 {
				return errs.NewMsg(errs.CodeParamInvalid, "wsMsg content invalid")
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

func decodeWsLog[T any](item *banexg.WsLog) (T, *errs.Error) {
	var zero T
	if item == nil || item.Content == "" {
		return zero, errs.NewMsg(errs.CodeParamInvalid, "ws log content required")
	}
	if err := utils.UnmarshalString(item.Content, &zero, utils.JsonNumDefault); err != nil {
		return zero, errs.New(errs.CodeUnmarshalFail, err)
	}
	return zero, nil
}
