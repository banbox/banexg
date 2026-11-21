package binance

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

const (
	maxDerivativesHistBatch = 500
	derivativesRESTMaxAgeMS = 29 * 24 * 3600 * 1000
	derivativesProbeOldMS   = 31 * 24 * 3600 * 1000
)

func isSupportedDerivativesPeriod(period string) bool {
	switch period {
	case "5m", "15m", "30m", "1h", "2h", "4h", "6h", "12h", "1d":
		return true
	default:
		return false
	}
}

func derivativesPeriodMS(period string) (int64, int64, bool) {
	if !isSupportedDerivativesPeriod(period) {
		return 0, 0, false
	}
	secs, err := utils.TFToSecSafe(period)
	if err != nil || secs <= 0 {
		return 0, 0, false
	}
	_, alignOffSecs := utils.GetTfAlignOrigin(secs)
	return int64(secs) * 1000, int64(alignOffSecs) * 1000, true
}

func utcDayStartMS(ms int64) int64 {
	t := time.UnixMilli(ms).UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).UnixMilli()
}

// FetchDerivativesMetrics combines immutable Binance Vision 5m daily metrics
// archives (resampled to timeframe with closing "last" semantics) for completed
// UTC days with Binance Futures REST endpoints for recent unarchived tail data.
func (e *Binance) FetchDerivativesMetrics(ctx context.Context, symbol, timeframe string, startMS, endMS int64, limit int, params map[string]interface{}) ([]*banexg.DerivativesMetric, bool, *errs.Error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, errs.New(errs.CodeTimeout, err)
	}
	args, market, err := e.LoadArgsMarket(symbol, params)
	if err != nil {
		return nil, false, err
	}
	if market == nil || (!market.Linear && !market.Inverse) || market.Spot || market.Option || market.Future {
		return nil, false, nil
	}
	tfMS, alignOffMS, ok := derivativesPeriodMS(timeframe)
	if !ok {
		return nil, false, errs.NewMsg(errs.CodeInvalidTimeFrame, "unsupported derivatives metrics timeframe: %s", timeframe)
	}
	delete(args, banexg.ParamContext)
	if until := utils.PopMapVal(args, banexg.ParamUntil, int64(0)); until > 0 && endMS <= 0 {
		endMS = until
	}
	wantFields := popRequestedMetricFields(args)

	if startMS <= 0 && endMS <= 0 {
		items, restErr := e.fetchDerivativesMetricsRESTLatest(ctx, market, timeframe, limit, wantFields, args)
		return items, true, restErr
	}
	nowMS := e.MilliSeconds()
	if nowMS <= 0 {
		nowMS = time.Now().UTC().UnixMilli()
	}
	if endMS <= 0 {
		endMS = nowMS
	}
	if startMS > endMS {
		return []*banexg.DerivativesMetric{}, true, nil
	}

	var collected []*banexg.DerivativesMetric
	restFrom := startMS
	todayStart := utcDayStartMS(nowMS)
	if restFrom > 0 && restFrom <= todayStart {
		archEnd := min(endMS, todayStart)
		archItems, nextStartMS, archAvail, archErr := e.fetchDerivativesMetricsArchivePartial(ctx, market, timeframe, tfMS, alignOffMS, restFrom, archEnd)
		if archErr != nil {
			return nil, true, archErr
		}
		if archAvail {
			collected = append(collected, archItems...)
			restFrom = nextStartMS
		}
	}

	if restFrom <= endMS && (limit <= 0 || len(collected) < limit) {
		remLimit := 0
		if limit > 0 {
			remLimit = limit - len(collected)
		}
		restItems, restErr := e.fetchDerivativesMetricsRESTRange(ctx, market, timeframe, tfMS, restFrom, endMS, remLimit, nowMS, wantFields, args)
		if restErr != nil {
			return nil, true, restErr
		}
		collected = append(collected, restItems...)
	}

	merged := mergeAndDedupeDerivativesMetrics(collected)
	filtered := make([]*banexg.DerivativesMetric, 0, len(merged))
	for _, item := range merged {
		if startMS > 0 && item.Timestamp < startMS {
			continue
		}
		if endMS > 0 && item.Timestamp > endMS {
			continue
		}
		filtered = append(filtered, item)
	}
	if limit > 0 && len(filtered) > limit {
		if startMS <= 0 && endMS > 0 {
			filtered = filtered[len(filtered)-limit:]
		} else {
			filtered = filtered[:limit]
		}
	}
	return filtered, true, nil
}

// FetchOpenInterestHistory fetches recent open interest history via Binance Futures REST.
func (e *Binance) FetchOpenInterestHistory(symbol, timeframe string, since int64, limit int, params map[string]interface{}) ([]*banexg.DerivativesMetric, *errs.Error) {
	args := utils.SafeParams(params)
	ctx := context.Background()
	if rawCtx, ok := args[banexg.ParamContext].(context.Context); ok && rawCtx != nil {
		ctx = rawCtx
		delete(args, banexg.ParamContext)
	}
	until := utils.PopMapVal(args, banexg.ParamUntil, int64(0))
	args["fields"] = []string{"oi", "oi_value"}
	args[banexg.ParamArchive] = false
	args, market, err := e.LoadArgsMarket(symbol, args)
	if err != nil {
		return nil, err
	}
	if market == nil || (!market.Linear && !market.Inverse) || market.Spot || market.Option || market.Future {
		return nil, errs.NewMsg(errs.CodeNotSupport, "open interest history requires linear or inverse swap")
	}
	tfMS, _, ok := derivativesPeriodMS(timeframe)
	if !ok {
		return nil, errs.NewMsg(errs.CodeInvalidTimeFrame, "unsupported open interest timeframe: %s", timeframe)
	}
	delete(args, banexg.ParamArchive)
	wantFields := popRequestedMetricFields(args)
	if since <= 0 && until <= 0 {
		return e.fetchDerivativesMetricsRESTLatest(ctx, market, timeframe, limit, wantFields, args)
	}
	nowMS := e.MilliSeconds()
	if nowMS <= 0 {
		nowMS = time.Now().UTC().UnixMilli()
	}
	if until <= 0 {
		until = nowMS
	}
	return e.fetchDerivativesMetricsRESTRange(ctx, market, timeframe, tfMS, since, until, limit, nowMS, wantFields, args)
}

func popRequestedMetricFields(args map[string]interface{}) map[string]bool {
	raw, ok := args["fields"]
	if !ok {
		return nil
	}
	delete(args, "fields")
	var names []string
	switch v := raw.(type) {
	case []string:
		names = v
	case []interface{}:
		for _, elem := range v {
			if s, ok := elem.(string); ok {
				names = append(names, s)
			}
		}
	case string:
		if v != "" {
			names = strings.Split(v, ",")
		}
	}
	if len(names) == 0 {
		return nil
	}
	out := make(map[string]bool, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" {
			out[name] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func metricFieldWanted(want map[string]bool, keys ...string) bool {
	if len(want) == 0 {
		return true
	}
	for _, k := range keys {
		if want[k] {
			return true
		}
	}
	return false
}

type derivativesEndpointSpec struct {
	method string
	apply  func(dst *banexg.DerivativesMetric, row map[string]interface{})
}

func (e *Binance) derivativesRESTEndpoints(market *banexg.Market, want map[string]bool) []derivativesEndpointSpec {
	var specs []derivativesEndpointSpec
	if market.Linear {
		if metricFieldWanted(want, "oi", "oi_value", "openInterest", "openInterestValue") {
			specs = append(specs, derivativesEndpointSpec{
				method: MethodFapiDataGetOpenInterestHist,
				apply: func(dst *banexg.DerivativesMetric, row map[string]interface{}) {
					if v, ok := parseFlexFloat(row["sumOpenInterest"]); ok {
						dst.OpenInterest = v
					}
					if v, ok := parseFlexFloat(row["sumOpenInterestValue"]); ok {
						dst.OpenInterestValue = v
					}
				},
			})
		}
		if metricFieldWanted(want, "top_ls_pos_ratio", "topTraderLongShortPosRatio") {
			specs = append(specs, derivativesEndpointSpec{
				method: MethodFapiDataGetTopLongShortPositionRatio,
				apply: func(dst *banexg.DerivativesMetric, row map[string]interface{}) {
					if v, ok := parseFlexFloat(row["longShortRatio"]); ok {
						dst.TopTraderLongShortPosRatio = v
					}
				},
			})
		}
		if metricFieldWanted(want, "top_ls_acc_ratio", "topTraderLongShortAccountRatio") {
			specs = append(specs, derivativesEndpointSpec{
				method: MethodFapiDataGetTopLongShortAccountRatio,
				apply: func(dst *banexg.DerivativesMetric, row map[string]interface{}) {
					if v, ok := parseFlexFloat(row["longShortRatio"]); ok {
						dst.TopTraderLongShortAccountRatio = v
					}
				},
			})
		}
		if metricFieldWanted(want, "global_ls_ratio", "globalLongShortAccountRatio") {
			specs = append(specs, derivativesEndpointSpec{
				method: MethodFapiDataGetGlobalLongShortAccountRatio,
				apply: func(dst *banexg.DerivativesMetric, row map[string]interface{}) {
					if v, ok := parseFlexFloat(row["longShortRatio"]); ok {
						dst.GlobalLongShortAccountRatio = v
					}
				},
			})
		}
		if metricFieldWanted(want, "taker_ls_vol_ratio", "takerLongShortVolRatio") {
			specs = append(specs, derivativesEndpointSpec{
				method: MethodFapiDataGetTakerlongshortRatio,
				apply: func(dst *banexg.DerivativesMetric, row map[string]interface{}) {
					if v, ok := parseFlexFloat(row["buySellRatio"]); ok {
						dst.TakerLongShortVolRatio = v
					}
				},
			})
		}
		return specs
	}

	if metricFieldWanted(want, "oi", "oi_value", "openInterest", "openInterestValue") {
		specs = append(specs, derivativesEndpointSpec{
			method: MethodDapiDataGetOpenInterestHist,
			apply: func(dst *banexg.DerivativesMetric, row map[string]interface{}) {
				if v, ok := parseFlexFloat(row["sumOpenInterest"]); ok {
					dst.OpenInterest = v
				}
				if v, ok := parseFlexFloat(row["sumOpenInterestValue"]); ok {
					dst.OpenInterestValue = v
				}
			},
		})
	}
	if metricFieldWanted(want, "top_ls_pos_ratio", "topTraderLongShortPosRatio") {
		specs = append(specs, derivativesEndpointSpec{
			method: MethodDapiDataGetTopLongShortPositionRatio,
			apply: func(dst *banexg.DerivativesMetric, row map[string]interface{}) {
				if v, ok := parseFlexFloat(row["longShortRatio"]); ok {
					dst.TopTraderLongShortPosRatio = v
				}
			},
		})
	}
	if metricFieldWanted(want, "top_ls_acc_ratio", "topTraderLongShortAccountRatio") {
		specs = append(specs, derivativesEndpointSpec{
			method: MethodDapiDataGetTopLongShortAccountRatio,
			apply: func(dst *banexg.DerivativesMetric, row map[string]interface{}) {
				if v, ok := parseFlexFloat(row["longShortRatio"]); ok {
					dst.TopTraderLongShortAccountRatio = v
				}
			},
		})
	}
	if metricFieldWanted(want, "global_ls_ratio", "globalLongShortAccountRatio") {
		specs = append(specs, derivativesEndpointSpec{
			method: MethodDapiDataGetGlobalLongShortAccountRatio,
			apply: func(dst *banexg.DerivativesMetric, row map[string]interface{}) {
				if v, ok := parseFlexFloat(row["longShortRatio"]); ok {
					dst.GlobalLongShortAccountRatio = v
				}
			},
		})
	}
	if metricFieldWanted(want, "taker_ls_vol_ratio", "takerLongShortVolRatio") {
		specs = append(specs, derivativesEndpointSpec{
			method: MethodDapiDataGetTakerBuySellVol,
			apply: func(dst *banexg.DerivativesMetric, row map[string]interface{}) {
				if v, ok := parseFlexFloat(row["buySellRatio"]); ok {
					dst.TakerLongShortVolRatio = v
					return
				}
				buyVol, okBuy := parseFlexFloat(row["takerBuyVol"])
				sellVol, okSell := parseFlexFloat(row["takerSellVol"])
				if okBuy && okSell && sellVol > 0 {
					dst.TakerLongShortVolRatio = buyVol / sellVol
				}
			},
		})
	}
	return specs
}

func (e *Binance) buildDerivativesRESTArgs(market *banexg.Market, period string, limit int, startMS, endMS int64, baseArgs map[string]interface{}) map[string]interface{} {
	reqArgs := utils.SafeParams(baseArgs)
	reqArgs["period"] = period
	reqArgs["limit"] = limit
	reqArgs[banexg.ParamNoCache] = true
	if market.Linear {
		reqArgs["symbol"] = market.ID
	} else {
		pair := market.BaseID + market.QuoteID
		if pair == "" {
			pair = strings.Split(market.ID, "_")[0]
		}
		reqArgs["pair"] = pair
		if _, has := reqArgs["contractType"]; !has {
			reqArgs["contractType"] = "PERPETUAL"
		}
	}
	if startMS > 0 {
		reqArgs["startTime"] = startMS
	} else {
		delete(reqArgs, "startTime")
	}
	if endMS > 0 {
		reqArgs["endTime"] = endMS
	} else {
		delete(reqArgs, "endTime")
	}
	return reqArgs
}

func (e *Binance) fetchDerivativesMetricsRESTLatest(ctx context.Context, market *banexg.Market, period string, limit int, want map[string]bool, baseArgs map[string]interface{}) ([]*banexg.DerivativesMetric, *errs.Error) {
	reqLimit := limit
	if reqLimit <= 0 || reqLimit > maxDerivativesHistBatch {
		reqLimit = maxDerivativesHistBatch
	}
	specs := e.derivativesRESTEndpoints(market, want)
	if len(specs) == 0 {
		return nil, nil
	}
	byTS := make(map[int64]*banexg.DerivativesMetric)
	for i, spec := range specs {
		reqArgs := e.buildDerivativesRESTArgs(market, period, reqLimit, 0, 0, baseArgs)
		rows, err := e.callDerivativesDataEndpoint(ctx, spec.method, reqArgs)
		if err != nil {
			if i == 0 {
				return nil, err
			}
			continue
		}
		applyDerivativesEndpointRows(byTS, market.Symbol, rows, spec.apply)
	}
	items := sortedDerivativesMap(byTS)
	if limit > 0 && len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items, nil
}

func (e *Binance) fetchDerivativesMetricsRESTRange(ctx context.Context, market *banexg.Market, period string, tfMS, startMS, endMS int64, limit int, nowMS int64, want map[string]bool, baseArgs map[string]interface{}) ([]*banexg.DerivativesMetric, *errs.Error) {
	specs := e.derivativesRESTEndpoints(market, want)
	if len(specs) == 0 {
		return nil, nil
	}
	curSince := startMS
	if curSince <= 0 {
		curSince = max(1, endMS-int64(maxDerivativesHistBatch)*tfMS)
	}
	byTS := make(map[int64]*banexg.DerivativesMetric)
	probedOld := false
	for curSince <= endMS {
		if err := ctx.Err(); err != nil {
			return nil, errs.New(errs.CodeTimeout, err)
		}
		batchLimit := maxDerivativesHistBatch
		if limit > 0 {
			remaining := limit - len(byTS)
			if remaining <= 0 {
				break
			}
			batchLimit = min(remaining, maxDerivativesHistBatch)
		}
		pageEnd := endMS
		if spanEnd := curSince + int64(batchLimit)*tfMS - 1; spanEnd > curSince && spanEnd < pageEnd {
			pageEnd = spanEnd
		}

		pageCount := 0
		var maxPageTS int64
		jumpedRecent := false
		for i, spec := range specs {
			reqArgs := e.buildDerivativesRESTArgs(market, period, batchLimit, curSince, pageEnd, baseArgs)
			rows, err := e.callDerivativesDataEndpoint(ctx, spec.method, reqArgs)
			if err != nil {
				if !probedOld && nowMS-curSince > derivativesRESTMaxAgeMS && isDerivativesOldRangeErr(err) {
					probedOld = true
					recentStart := nowMS - derivativesRESTMaxAgeMS
					if recentStart > endMS {
						return sortedDerivativesMap(byTS), nil
					}
					curSince = recentStart
					jumpedRecent = true
					break
				}
				if i == 0 {
					return nil, err
				}
				continue
			}
			n, maxTS := applyDerivativesEndpointRows(byTS, market.Symbol, rows, spec.apply)
			if n > pageCount {
				pageCount = n
			}
			if maxTS > maxPageTS {
				maxPageTS = maxTS
			}
		}
		if jumpedRecent {
			continue
		}
		if pageCount == 0 && !probedOld && nowMS-curSince > derivativesProbeOldMS {
			probedOld = true
			recentStart := nowMS - derivativesRESTMaxAgeMS
			if recentStart > curSince {
				curSince = recentStart
				continue
			}
		}
		if pageEnd >= endMS {
			break
		}
		nextSince := pageEnd + 1
		if maxPageTS > pageEnd {
			nextSince = maxPageTS + 1
		}
		if nextSince <= curSince {
			break
		}
		curSince = nextSince
	}

	items := sortedDerivativesMap(byTS)
	out := items[:0]
	for _, item := range items {
		if startMS > 0 && item.Timestamp < startMS {
			continue
		}
		if endMS > 0 && item.Timestamp > endMS {
			continue
		}
		out = append(out, item)
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func isDerivativesOldRangeErr(err *errs.Error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return err.Code == -1130 || err.Code == errs.CodeParamInvalid ||
		strings.Contains(msg, "starttime") || strings.Contains(msg, "-1130")
}

func (e *Binance) callDerivativesDataEndpoint(ctx context.Context, method string, args map[string]interface{}) ([]map[string]interface{}, *errs.Error) {
	tryNum := e.GetRetryNum("FetchDerivativesMetrics", 2)
	rsp := e.RequestApiRetry(ctx, method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	content := strings.TrimSpace(rsp.Content)
	if content == "" || content == "[]" {
		return nil, nil
	}
	var rows []map[string]interface{}
	if err := utils.UnmarshalString(content, &rows, utils.JsonNumAuto); err != nil {
		return nil, errs.NewFull(errs.CodeUnmarshalFail, err, "decode %s response", method)
	}
	return rows, nil
}

func applyDerivativesEndpointRows(byTS map[int64]*banexg.DerivativesMetric, symbol string, rows []map[string]interface{}, apply func(dst *banexg.DerivativesMetric, row map[string]interface{})) (int, int64) {
	var maxTS int64
	count := 0
	for _, row := range rows {
		ts, ok := parseFlexInt64(row["timestamp"])
		if !ok || ts <= 0 {
			continue
		}
		if ts > maxTS {
			maxTS = ts
		}
		count++
		dst := byTS[ts]
		if dst == nil {
			dst = banexg.NewDerivativesMetric(symbol, ts)
			dst.Info = make(map[string]interface{})
			byTS[ts] = dst
		}
		apply(dst, row)
		if dst.Info != nil {
			for k, v := range row {
				dst.Info[k] = v
			}
		}
	}
	return count, maxTS
}

func sortedDerivativesMap(byTS map[int64]*banexg.DerivativesMetric) []*banexg.DerivativesMetric {
	if len(byTS) == 0 {
		return nil
	}
	items := make([]*banexg.DerivativesMetric, 0, len(byTS))
	for _, item := range byTS {
		if hasAnyValidDerivativesMetric(item) {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Timestamp < items[j].Timestamp })
	return items
}

func hasAnyValidDerivativesMetric(m *banexg.DerivativesMetric) bool {
	if m == nil || m.Timestamp <= 0 {
		return false
	}
	return isFiniteFloat(m.OpenInterest) ||
		isFiniteFloat(m.OpenInterestValue) ||
		isFiniteFloat(m.TopTraderLongShortAccountRatio) ||
		isFiniteFloat(m.TopTraderLongShortPosRatio) ||
		isFiniteFloat(m.GlobalLongShortAccountRatio) ||
		isFiniteFloat(m.TakerLongShortVolRatio)
}

func isFiniteFloat(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func mergeDerivativesMetricInto(dst, src *banexg.DerivativesMetric) {
	if dst == nil || src == nil {
		return
	}
	if isFiniteFloat(src.OpenInterest) {
		dst.OpenInterest = src.OpenInterest
	}
	if isFiniteFloat(src.OpenInterestValue) {
		dst.OpenInterestValue = src.OpenInterestValue
	}
	if isFiniteFloat(src.TopTraderLongShortAccountRatio) {
		dst.TopTraderLongShortAccountRatio = src.TopTraderLongShortAccountRatio
	}
	if isFiniteFloat(src.TopTraderLongShortPosRatio) {
		dst.TopTraderLongShortPosRatio = src.TopTraderLongShortPosRatio
	}
	if isFiniteFloat(src.GlobalLongShortAccountRatio) {
		dst.GlobalLongShortAccountRatio = src.GlobalLongShortAccountRatio
	}
	if isFiniteFloat(src.TakerLongShortVolRatio) {
		dst.TakerLongShortVolRatio = src.TakerLongShortVolRatio
	}
	if len(src.Info) > 0 {
		if dst.Info == nil {
			dst.Info = make(map[string]interface{}, len(src.Info))
		}
		for k, v := range src.Info {
			dst.Info[k] = v
		}
	}
}

func mergeAndDedupeDerivativesMetrics(items []*banexg.DerivativesMetric) []*banexg.DerivativesMetric {
	if len(items) == 0 {
		return nil
	}
	valid := make([]*banexg.DerivativesMetric, 0, len(items))
	for _, item := range items {
		if hasAnyValidDerivativesMetric(item) {
			valid = append(valid, item)
		}
	}
	sort.SliceStable(valid, func(i, j int) bool { return valid[i].Timestamp < valid[j].Timestamp })
	out := make([]*banexg.DerivativesMetric, 0, len(valid))
	for _, item := range valid {
		if len(out) > 0 && out[len(out)-1].Timestamp == item.Timestamp {
			mergeDerivativesMetricInto(out[len(out)-1], item)
			continue
		}
		cp := *item
		if item.Info != nil {
			cp.Info = make(map[string]interface{}, len(item.Info))
			for k, v := range item.Info {
				cp.Info[k] = v
			}
		}
		out = append(out, &cp)
	}
	return out
}

func parseFlexInt64(raw interface{}) (int64, bool) {
	switch v := raw.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, false
		}
		return int64(v), true
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return n, true
		}
		if f, err := v.Float64(); err == nil && isFiniteFloat(f) {
			return int64(f), true
		}
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return 0, false
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n, true
		}
	}
	return 0, false
}

func parseFlexFloat(raw interface{}) (float64, bool) {
	switch v := raw.(type) {
	case float64:
		return v, isFiniteFloat(v)
	case float32:
		f := float64(v)
		return f, isFiniteFloat(f)
	case int64:
		return float64(v), true
	case int:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil && isFiniteFloat(f)
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return math.NaN(), false
		}
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil && isFiniteFloat(f)
	}
	return math.NaN(), false
}
