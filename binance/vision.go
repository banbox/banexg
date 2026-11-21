package binance

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

var binanceVisionBaseURL = "https://data.binance.vision/data"
var binanceVisionS3BaseURL = "https://s3-ap-northeast-1.amazonaws.com/data.binance.vision/data"

var binanceVisionHTTPFlow = make(chan struct{}, 4)
var binanceVisionRequestTimeout = 2 * time.Minute

const maxVisionDownloadAttempts = 6

type visionArchiveFile struct {
	granularity string
	period      string
}

// FetchOHLCVArchive reads immutable Binance Vision kline files. The optional
// capability is kept separate from FetchOHLCV so API callers retain their
// existing request semantics and the caller can choose the age cutover.
func (e *Binance) FetchOHLCVArchive(ctx context.Context, symbol, timeframe string, startMS, endMS int64) ([]*banexg.Kline, bool, *errs.Error) {
	if startMS >= endMS {
		return nil, true, nil
	}
	market, err := e.GetMarket(symbol)
	if err != nil {
		return nil, false, err
	}
	marketPath, ok := visionMarketPath(market)
	if !ok || market.ID == "" {
		return nil, false, nil
	}
	visionTF, ok := visionTimeframe(timeframe)
	if !ok {
		return nil, false, nil
	}
	files := visionArchiveFiles(startMS, endMS)
	if len(files) == 0 {
		return nil, true, nil
	}

	var result []*banexg.Kline
	archiveFound := false
	for _, file := range files {
		body, found, fetchErr := e.fetchVisionFile(ctx, marketPath, strings.ToUpper(market.ID), visionTF, file)
		if fetchErr != nil {
			return nil, false, fetchErr
		}
		if !found {
			if !archiveFound {
				// Markets can have a stale listing date. Skip files before the first
				// published archive, but do not hide a hole inside the archive.
				continue
			}
			// A missing archive after data starts is a real coverage gap. Let the
			// caller use the exchange API for the complete requested segment.
			return nil, false, nil
		}
		archiveFound = true
		klines, parseErr := parseVisionKlines(body, startMS, endMS, market.Inverse)
		if parseErr != nil {
			return nil, false, errs.NewFull(parseErr.Code, parseErr,
				"parse Binance Vision file %s-%s", file.granularity, file.period)
		}
		result = append(result, klines...)
	}
	if !archiveFound {
		return nil, false, nil
	}
	return dedupeVisionKlines(result), true, nil
}

// FetchPriceOHLCVArchive reads immutable monthly and daily Binance Vision
// markPriceKlines, indexPriceKlines, or premiumIndexKlines archives over
// [startMS, endMS) for USD-M and COIN-M futures. Callers can fall back to
// FetchOHLCV with params["price"] when available is false.
func (e *Binance) FetchPriceOHLCVArchive(ctx context.Context, priceType, symbol, timeframe string, startMS, endMS int64) ([]*banexg.Kline, bool, *errs.Error) {
	if ctx == nil {
		ctx = context.Background()
	}
	normalizedPrice, err := normalizeBinancePriceType(priceType)
	if err != nil {
		return nil, false, err
	}
	if normalizedPrice == "" {
		return nil, false, errs.NewMsg(errs.CodeParamInvalid, "priceType is required for FetchPriceOHLCVArchive")
	}
	if startMS >= endMS {
		return nil, true, nil
	}
	market, err := e.GetMarket(symbol)
	if err != nil {
		return nil, false, err
	}
	marketPath, ok := visionPriceKlineMarketPath(market, normalizedPrice)
	if !ok {
		return nil, false, nil
	}
	category, targetID, ok := visionPriceKlineTarget(market, normalizedPrice)
	if !ok {
		return nil, false, nil
	}
	visionTF, ok := visionPriceKlineTimeframe(timeframe)
	if !ok {
		return nil, false, nil
	}
	files := visionArchiveFiles(startMS, endMS)
	if len(files) == 0 {
		return nil, true, nil
	}

	var result []*banexg.Kline
	archiveFound := false
	for _, file := range files {
		body, found, fetchErr := e.fetchVisionKlineCategoryFile(ctx, marketPath, category, targetID, visionTF, file)
		if fetchErr != nil {
			return nil, false, fetchErr
		}
		if !found {
			if !archiveFound {
				continue
			}
			return nil, false, nil
		}
		archiveFound = true
		klines, parseErr := parseVisionPriceKlines(body, startMS, endMS)
		if parseErr != nil {
			return nil, false, errs.NewFull(parseErr.Code, parseErr,
				"parse Binance Vision %s file %s-%s", category, file.granularity, file.period)
		}
		result = append(result, klines...)
	}
	if !archiveFound {
		return nil, false, nil
	}
	return dedupeVisionKlines(result), true, nil
}

// FetchFundingRateArchive reads immutable monthly Binance Vision fundingRate
// files for perpetual contracts over inclusive [startMS, endMS]. Callers can
// fall back to FetchFundingRateHistory when available is false.
func (e *Binance) FetchFundingRateArchive(ctx context.Context, symbol string, startMS, endMS int64) ([]*banexg.FundingRate, bool, *errs.Error) {
	if startMS > endMS {
		return nil, true, nil
	}
	market, err := e.GetMarket(symbol)
	if err != nil {
		return nil, false, err
	}
	marketPath, ok := visionFundingMarketPath(market)
	if !ok || market.ID == "" {
		return nil, false, nil
	}
	periods := visionMonthlyPeriods(startMS, endMS)
	if len(periods) == 0 {
		return nil, true, nil
	}
	marketID := strings.ToUpper(market.ID)
	var result []*banexg.FundingRate
	archiveFound := false
	for _, period := range periods {
		relPath := fmt.Sprintf("%s/monthly/fundingRate/%s/%s-fundingRate-%s.zip", marketPath, marketID, marketID, period)
		body, found, fetchErr := e.fetchVisionRelPath(ctx, relPath)
		if fetchErr != nil {
			return nil, false, fetchErr
		}
		if !found {
			if !archiveFound {
				continue
			}
			return nil, false, nil
		}
		archiveFound = true
		rates, parseErr := parseVisionFundingRates(body, market.Symbol, startMS, endMS)
		if parseErr != nil {
			return nil, false, errs.NewFull(parseErr.Code, parseErr,
				"parse Binance Vision fundingRate file %s", period)
		}
		result = append(result, rates...)
	}
	if !archiveFound {
		return nil, false, nil
	}
	return dedupeVisionFundingRates(result), true, nil
}

type visionDailyPeriod struct {
	date    string
	startMS int64
	endMS   int64
}

type visionDayFetchResult struct {
	body  []byte
	found bool
	err   *errs.Error
}

// FetchDerivativesMetricsArchive reads immutable daily Binance Vision 5m metrics
// files for perpetual contracts over inclusive [startMS, endMS] and resamples
// them to timeframe using closing ("last") snapshot semantics.
func (e *Binance) FetchDerivativesMetricsArchive(ctx context.Context, symbol, timeframe string, startMS, endMS int64) ([]*banexg.DerivativesMetric, bool, *errs.Error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if startMS > endMS {
		return nil, true, nil
	}
	market, err := e.GetMarket(symbol)
	if err != nil {
		return nil, false, err
	}
	tfMS, alignOffMS, ok := derivativesPeriodMS(timeframe)
	if !ok {
		return nil, false, nil
	}
	items, nextStartMS, avail, fetchErr := e.fetchDerivativesMetricsArchivePartial(ctx, market, timeframe, tfMS, alignOffMS, startMS, endMS)
	if fetchErr != nil {
		return nil, false, fetchErr
	}
	if !avail || nextStartMS <= endMS {
		return nil, false, nil
	}
	return items, true, nil
}

func (e *Binance) fetchDerivativesMetricsArchivePartial(ctx context.Context, market *banexg.Market, timeframe string, tfMS, alignOffMS, startMS, endMS int64) ([]*banexg.DerivativesMetric, int64, bool, *errs.Error) {
	if startMS > endMS {
		return nil, endMS + 1, true, nil
	}
	marketPath, ok := visionFundingMarketPath(market)
	if !ok || market.ID == "" {
		return nil, startMS, false, nil
	}
	firstBucketStart := utils.AlignTfMSecsOffset(max(int64(0), startMS-1), tfMS, alignOffMS)
	rawStartMS := firstBucketStart + 1
	lastSampleMS := max(rawStartMS, endMS-1)
	days := visionDailyPeriods(rawStartMS, lastSampleMS)
	if len(days) == 0 {
		return nil, endMS + 1, true, nil
	}

	marketID := strings.ToUpper(market.ID)
	results, fetchErr := e.fetchVisionMetricsDays(ctx, marketPath, marketID, days)
	if fetchErr != nil {
		return nil, startMS, false, fetchErr
	}

	var raw []*banexg.DerivativesMetric
	archiveFound := false
	trailingMissingStart := int64(0)
	for i, day := range days {
		res := results[i]
		if !res.found {
			if !archiveFound {
				continue
			}
			// Check whether any later day exists; an interior hole invalidates
			// contiguous archive coverage, whereas all-missing trailing days are
			// simply not yet published and can be stitched from REST.
			for j := i + 1; j < len(results); j++ {
				if results[j].found {
					return nil, startMS, false, nil
				}
			}
			trailingMissingStart = day.startMS
			break
		}
		archiveFound = true
		parsed, parseErr := parseVisionDerivativesMetrics(res.body, market.Symbol, rawStartMS, endMS)
		if parseErr != nil {
			return nil, startMS, false, errs.NewFull(parseErr.Code, parseErr,
				"parse Binance Vision metrics file %s", day.date)
		}
		raw = append(raw, parsed...)
	}
	if !archiveFound {
		return nil, startMS, false, nil
	}

	raw = mergeAndDedupeDerivativesMetrics(raw)
	// When endMS is an exact UTC day boundary and the previous day's CSV ended at
	// 23:55:00 rather than 24:00:00, probe the boundary day's CSV for 00:00:00.
	if trailingMissingStart == 0 && endMS == utcDayStartMS(endMS) && (len(raw) == 0 || raw[len(raw)-1].Timestamp < endMS) {
		endDate := time.UnixMilli(endMS).UTC().Format("2006-01-02")
		relPath := fmt.Sprintf("%s/daily/metrics/%s/%s-metrics-%s.zip", marketPath, marketID, marketID, endDate)
		body, found, probeErr := e.fetchVisionRelPath(ctx, relPath)
		if probeErr != nil {
			return nil, startMS, false, probeErr
		}
		if found {
			parsed, parseErr := parseVisionDerivativesMetrics(body, market.Symbol, rawStartMS, endMS)
			if parseErr != nil {
				return nil, startMS, false, errs.NewFull(parseErr.Code, parseErr,
					"parse Binance Vision metrics file %s", endDate)
			}
			raw = mergeAndDedupeDerivativesMetrics(append(raw, parsed...))
		}
	}

	effectiveEndMS := endMS
	if trailingMissingStart > 0 && trailingMissingStart < effectiveEndMS {
		// Allow 24:00:00 row already present in the last found day file.
		if len(raw) > 0 && raw[len(raw)-1].Timestamp == trailingMissingStart {
			effectiveEndMS = trailingMissingStart
		} else {
			effectiveEndMS = trailingMissingStart - 1
		}
	}

	resampled, coveredThroughMS := resampleVisionDerivativesMetrics(raw, market.Symbol, tfMS, alignOffMS, startMS, effectiveEndMS)
	nextStartMS := endMS + 1
	if trailingMissingStart > 0 {
		if coveredThroughMS > 0 {
			nextStartMS = coveredThroughMS + 1
		} else {
			nextStartMS = max(startMS, trailingMissingStart)
		}
	} else if coveredThroughMS < endMS {
		// If the final bucket at endMS could not be closed from archived 5m rows,
		// allow REST to supply the unclosed/missing tail from coveredThroughMS+1.
		if coveredThroughMS > 0 {
			nextStartMS = coveredThroughMS + 1
		} else {
			nextStartMS = startMS
		}
	}
	return resampled, nextStartMS, true, nil
}

func (e *Binance) fetchVisionMetricsDays(ctx context.Context, marketPath, marketID string, days []visionDailyPeriod) ([]visionDayFetchResult, *errs.Error) {
	results := make([]visionDayFetchResult, len(days))
	if len(days) == 1 {
		relPath := fmt.Sprintf("%s/daily/metrics/%s/%s-metrics-%s.zip", marketPath, marketID, marketID, days[0].date)
		body, found, err := e.fetchVisionRelPath(ctx, relPath)
		if err != nil {
			return nil, err
		}
		results[0] = visionDayFetchResult{body: body, found: found}
		return results, nil
	}

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	workers := min(cap(binanceVisionHTTPFlow), len(days))
	if workers < 1 {
		workers = 1
	}
	var idx atomic.Int64
	idx.Store(-1)
	var firstErr *errs.Error
	var errOnce sync.Once
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(idx.Add(1))
				if i >= len(days) || workerCtx.Err() != nil {
					return
				}
				relPath := fmt.Sprintf("%s/daily/metrics/%s/%s-metrics-%s.zip", marketPath, marketID, marketID, days[i].date)
				body, found, err := e.fetchVisionRelPath(workerCtx, relPath)
				if err != nil {
					if ctx.Err() == nil && workerCtx.Err() != nil {
						return
					}
					errOnce.Do(func() {
						firstErr = err
						cancel()
					})
					return
				}
				results[i] = visionDayFetchResult{body: body, found: found}
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, errs.New(errs.CodeTimeout, err)
	}
	return results, nil
}

func visionDailyPeriods(startMS, endMS int64) []visionDailyPeriod {
	if startMS > endMS {
		return nil
	}
	startTime := time.UnixMilli(startMS).UTC()
	cur := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, time.UTC)
	endTime := time.UnixMilli(endMS).UTC()
	last := time.Date(endTime.Year(), endTime.Month(), endTime.Day(), 0, 0, 0, 0, time.UTC)
	out := make([]visionDailyPeriod, 0)
	for !cur.After(last) {
		next := cur.AddDate(0, 0, 1)
		out = append(out, visionDailyPeriod{
			date:    cur.Format("2006-01-02"),
			startMS: cur.UnixMilli(),
			endMS:   next.UnixMilli(),
		})
		cur = next
	}
	return out
}

func resampleVisionDerivativesMetrics(raw []*banexg.DerivativesMetric, symbol string, tfMS, alignOffMS, startMS, endMS int64) ([]*banexg.DerivativesMetric, int64) {
	if len(raw) == 0 {
		return nil, 0
	}
	const fiveMinMS = int64(5 * 60 * 1000)
	if tfMS <= fiveMinMS {
		var out []*banexg.DerivativesMetric
		var maxTS int64
		for _, item := range raw {
			if item.Timestamp > endMS {
				continue
			}
			if item.Timestamp > maxTS {
				maxTS = item.Timestamp
			}
			if item.Timestamp >= startMS {
				cp := *item
				out = append(out, &cp)
			}
		}
		return out, maxTS
	}

	var out []*banexg.DerivativesMetric
	var curBucketEnd int64
	var curLast *banexg.DerivativesMetric
	var coveredThrough int64
	flush := func() {
		if curLast == nil || curBucketEnd > endMS {
			curLast = nil
			return
		}
		// Require the final 5m slot of the coarse bucket to be present so a
		// truncated archive day never fabricates a premature coarse snapshot.
		if curLast.Timestamp <= curBucketEnd-fiveMinMS {
			curLast = nil
			return
		}
		if curBucketEnd > coveredThrough {
			coveredThrough = curBucketEnd
		}
		if curBucketEnd >= startMS {
			snap := *curLast
			snap.Symbol = symbol
			snap.Timestamp = curBucketEnd
			out = append(out, &snap)
		}
		curLast = nil
	}

	for _, item := range raw {
		if item == nil || item.Timestamp <= 0 || item.Timestamp > endMS {
			continue
		}
		bucketEnd := utils.AlignTfMSecsOffset(max(int64(0), item.Timestamp-1), tfMS, alignOffMS) + tfMS
		if curLast != nil && bucketEnd != curBucketEnd {
			flush()
		}
		curBucketEnd = bucketEnd
		curLast = item
	}
	flush()
	return out, coveredThrough
}

func parseVisionDerivativesMetrics(data []byte, symbol string, startMS, endMS int64) ([]*banexg.DerivativesMetric, *errs.Error) {
	var result []*banexg.DerivativesMetric
	err := readVisionZipCSV(data, isVisionMetricsHeader, func(row []string) error {
		item, rowErr := parseVisionDerivativesMetricRow(row, symbol)
		if rowErr != nil {
			return rowErr
		}
		if item == nil {
			return nil
		}
		if item.Timestamp >= startMS && item.Timestamp <= endMS {
			result = append(result, item)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return mergeAndDedupeDerivativesMetrics(result), nil
}

func isVisionMetricsHeader(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "create_time" || value == "create time"
}

func parseVisionDerivativesMetricRow(row []string, symbol string) (*banexg.DerivativesMetric, error) {
	if len(row) < 4 {
		return nil, fmt.Errorf("expected at least 4 columns, got %d", len(row))
	}
	ts, err := parseVisionCreateTime(row[0])
	if err != nil {
		return nil, fmt.Errorf("create_time: %w", err)
	}
	item := banexg.NewDerivativesMetric(symbol, ts)
	// Standard Binance Vision metrics CSV schema:
	// 0: create_time
	// 1: symbol
	// 2: sum_open_interest
	// 3: sum_open_interest_value
	// 4: count_toptrader_long_short_ratio (account count ratio)
	// 5: sum_toptrader_long_short_ratio   (position ratio)
	// 6: count_long_short_ratio           (global account ratio)
	// 7: sum_taker_long_short_vol_ratio   (taker buy/sell volume ratio)
	if v, ok, err := parseOptionalVisionFloat(row, 2); err != nil {
		return nil, err
	} else if ok {
		item.OpenInterest = v
	}
	if v, ok, err := parseOptionalVisionFloat(row, 3); err != nil {
		return nil, err
	} else if ok {
		item.OpenInterestValue = v
	}
	if v, ok, err := parseOptionalVisionFloat(row, 4); err != nil {
		return nil, err
	} else if ok {
		item.TopTraderLongShortAccountRatio = v
	}
	if v, ok, err := parseOptionalVisionFloat(row, 5); err != nil {
		return nil, err
	} else if ok {
		item.TopTraderLongShortPosRatio = v
	}
	if v, ok, err := parseOptionalVisionFloat(row, 6); err != nil {
		return nil, err
	} else if ok {
		item.GlobalLongShortAccountRatio = v
	}
	if v, ok, err := parseOptionalVisionFloat(row, 7); err != nil {
		return nil, err
	} else if ok {
		item.TakerLongShortVolRatio = v
	}
	if !hasAnyValidDerivativesMetric(item) {
		return nil, nil
	}
	return item, nil
}

func parseVisionCreateTime(raw string) (int64, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, fmt.Errorf("empty timestamp")
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n >= 100_000_000_000_000 {
			n /= 1000
		}
		return n, nil
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.999999999",
		time.RFC3339Nano,
		"2006-01-02T15:04:05",
	} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t.UnixMilli(), nil
		}
	}
	return 0, fmt.Errorf("unsupported timestamp %q", s)
}

func parseOptionalVisionFloat(row []string, index int) (float64, bool, error) {
	if index >= len(row) {
		return math.NaN(), false, nil
	}
	s := strings.TrimSpace(row[index])
	if s == "" || strings.EqualFold(s, "nan") || strings.EqualFold(s, "null") {
		return math.NaN(), false, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return math.NaN(), false, fmt.Errorf("column %d: %w", index, err)
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return math.NaN(), false, nil
	}
	return v, true, nil
}

func visionFundingMarketPath(market *banexg.Market) (string, bool) {
	if market == nil || market.Future || market.Option {
		return "", false
	}
	switch {
	case market.Linear:
		return "futures/um", true
	case market.Inverse:
		return "futures/cm", true
	default:
		return "", false
	}
}

func visionMonthlyPeriods(startMS, endMS int64) []string {
	if startMS > endMS {
		return nil
	}
	startTime := time.UnixMilli(startMS).UTC()
	cur := time.Date(startTime.Year(), startTime.Month(), 1, 0, 0, 0, 0, time.UTC)
	endTime := time.UnixMilli(endMS).UTC()
	last := time.Date(endTime.Year(), endTime.Month(), 1, 0, 0, 0, 0, time.UTC)
	periods := make([]string, 0)
	for !cur.After(last) {
		periods = append(periods, cur.Format("2006-01"))
		cur = cur.AddDate(0, 1, 0)
	}
	return periods
}

func visionMarketPath(market *banexg.Market) (string, bool) {
	if market == nil {
		return "", false
	}
	switch {
	case market.Linear:
		return "futures/um", true
	case market.Inverse:
		return "futures/cm", true
	case market.Spot || market.Margin || market.Type == banexg.MarketMargin:
		return "spot", true
	default:
		return "", false
	}
}

func visionTimeframe(timeframe string) (string, bool) {
	switch timeframe {
	case "1s", "1m", "3m", "5m", "15m", "30m", "1h", "2h":
		return timeframe, true
	default:
		return "", false
	}
}

func visionPriceKlineMarketPath(market *banexg.Market, priceType string) (string, bool) {
	if market == nil || market.Spot || market.Option {
		return "", false
	}
	if priceType == "premiumIndex" && market.Future {
		return "", false
	}
	switch {
	case market.Linear:
		return "futures/um", true
	case market.Inverse:
		return "futures/cm", true
	default:
		return "", false
	}
}

func visionPriceKlineTarget(market *banexg.Market, priceType string) (string, string, bool) {
	if market == nil {
		return "", "", false
	}
	switch priceType {
	case "mark":
		id := strings.ToUpper(strings.TrimSpace(market.ID))
		return "markPriceKlines", id, id != ""
	case "index":
		pair := strings.ToUpper(strings.TrimSpace(binanceIndexPairID(market)))
		return "indexPriceKlines", pair, pair != ""
	case "premiumIndex":
		id := strings.ToUpper(strings.TrimSpace(market.ID))
		return "premiumIndexKlines", id, id != ""
	default:
		return "", "", false
	}
}

func visionPriceKlineTimeframe(timeframe string) (string, bool) {
	switch strings.TrimSpace(timeframe) {
	case "1m", "3m", "5m", "15m", "30m", "1h", "2h", "4h", "6h", "8h", "12h", "1d", "3d", "1w":
		return strings.TrimSpace(timeframe), true
	case "1M", "1mo":
		return "1mo", true
	default:
		return "", false
	}
}

func visionArchiveFiles(startMS, endMS int64) []visionArchiveFile {
	if startMS >= endMS {
		return nil
	}
	files := make([]visionArchiveFile, 0)
	for cur := startMS; cur < endMS; {
		curTime := time.UnixMilli(cur).UTC()
		dayStart := time.Date(curTime.Year(), curTime.Month(), curTime.Day(), 0, 0, 0, 0, time.UTC)
		monthStart := time.Date(curTime.Year(), curTime.Month(), 1, 0, 0, 0, 0, time.UTC)
		monthEnd := monthStart.AddDate(0, 1, 0)
		if cur == monthStart.UnixMilli() && monthEnd.UnixMilli() <= endMS {
			files = append(files, visionArchiveFile{
				granularity: "monthly",
				period:      monthStart.Format("2006-01"),
			})
			cur = monthEnd.UnixMilli()
			continue
		}
		files = append(files, visionArchiveFile{
			granularity: "daily",
			period:      dayStart.Format("2006-01-02"),
		})
		cur = dayStart.AddDate(0, 0, 1).UnixMilli()
	}
	return files
}

func (e *Binance) fetchVisionFile(ctx context.Context, marketPath, marketID, timeframe string,
	file visionArchiveFile) ([]byte, bool, *errs.Error) {
	return e.fetchVisionKlineCategoryFile(ctx, marketPath, "klines", marketID, timeframe, file)
}

func (e *Binance) fetchVisionKlineCategoryFile(ctx context.Context, marketPath, category, targetID, timeframe string,
	file visionArchiveFile) ([]byte, bool, *errs.Error) {
	relPath := fmt.Sprintf("%s/%s/%s/%s/%s/%s-%s-%s.zip",
		marketPath, file.granularity, category, targetID, timeframe, targetID, timeframe, file.period)
	return e.fetchVisionRelPath(ctx, relPath)
}

func (e *Binance) fetchVisionRelPath(ctx context.Context, relPath string) ([]byte, bool, *errs.Error) {
	requestURL := fmt.Sprintf("%s/%s", strings.TrimRight(binanceVisionBaseURL, "/"), relPath)
	requestURLs := []string{requestURL}
	if strings.TrimRight(binanceVisionBaseURL, "/") == "https://data.binance.vision/data" {
		requestURLs = append(requestURLs, fmt.Sprintf("%s/%s",
			strings.TrimRight(binanceVisionS3BaseURL, "/"), relPath))
	}
	select {
	case binanceVisionHTTPFlow <- struct{}{}:
	case <-ctx.Done():
		return nil, false, errs.New(errs.CodeTimeout, ctx.Err())
	}
	defer func() { <-binanceVisionHTTPFlow }()

	client := e.HttpClient
	if client == nil {
		client = http.DefaultClient
	}
	for attempt := 0; attempt < maxVisionDownloadAttempts; attempt++ {
		requestURL = requestURLs[attempt%len(requestURLs)]
		requestCtx, cancel := context.WithTimeout(ctx, binanceVisionRequestTimeout)
		request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, requestURL, nil)
		if err != nil {
			cancel()
			return nil, false, errs.New(errs.CodeInvalidRequest, err)
		}
		if e.UserAgent != "" {
			request.Header.Set("User-Agent", e.UserAgent)
		}
		for key, value := range e.ReqHeaders {
			request.Header.Set(key, value)
		}
		response, err := client.Do(request)
		requestTimedOut := requestCtx.Err() == context.DeadlineExceeded
		if err != nil {
			cancel()
			if ctx.Err() != nil {
				return nil, false, errs.New(errs.CodeTimeout, ctx.Err())
			}
			if attempt+1 < maxVisionDownloadAttempts {
				if waitErr := waitVisionRetry(ctx, attempt); waitErr != nil {
					return nil, false, waitErr
				}
				continue
			}
			if requestTimedOut {
				return nil, false, errs.NewFull(errs.CodeTimeout, err, "download Binance Vision file %s timed out", requestURL)
			}
			return nil, false, errs.NewFull(errs.CodeNetFail, err, "download Binance Vision file %s", requestURL)
		}
		if response.StatusCode == http.StatusNotFound {
			_ = response.Body.Close()
			cancel()
			return nil, false, nil
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			status := response.StatusCode
			code := errs.CodeServerError
			switch status {
			case http.StatusUnauthorized:
				code = errs.CodeUnauthorized
			case http.StatusForbidden:
				code = errs.CodeForbidden
			case http.StatusTooManyRequests:
				code = errs.CodeRateLimit
			}
			_ = response.Body.Close()
			cancel()
			if isVisionTransientStatus(status) && attempt+1 < maxVisionDownloadAttempts {
				if waitErr := waitVisionRetry(ctx, attempt); waitErr != nil {
					return nil, false, waitErr
				}
				continue
			}
			return nil, false, errs.NewMsg(code, "Binance Vision request failed status=%d url=%s", status, requestURL)
		}
		body, err := io.ReadAll(response.Body)
		closeErr := response.Body.Close()
		requestTimedOut = requestCtx.Err() == context.DeadlineExceeded
		cancel()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			return body, true, nil
		}
		if attempt+1 < maxVisionDownloadAttempts {
			if waitErr := waitVisionRetry(ctx, attempt); waitErr != nil {
				return nil, false, waitErr
			}
			continue
		}
		if requestTimedOut {
			return nil, false, errs.NewFull(errs.CodeTimeout, err, "read Binance Vision file %s timed out", requestURL)
		}
		return nil, false, errs.NewFull(errs.CodeNetFail, err, "read Binance Vision file %s", requestURL)
	}
	return nil, false, errs.NewMsg(errs.CodeNetFail, "download Binance Vision file %s failed", requestURL)
}

func isVisionTransientStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func waitVisionRetry(ctx context.Context, attempt int) *errs.Error {
	timer := time.NewTimer(time.Second << attempt)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return errs.New(errs.CodeTimeout, ctx.Err())
	}
}

func readVisionZipCSV(data []byte, isHeader func(string) bool, onRow func(row []string) error) *errs.Error {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return errs.NewFull(errs.CodeInvalidData, err, "decode Binance Vision zip")
	}
	foundCSV := false
	for _, file := range archive.File {
		if file.FileInfo().IsDir() || !strings.HasSuffix(strings.ToLower(file.Name), ".csv") {
			continue
		}
		foundCSV = true
		reader, err := file.Open()
		if err != nil {
			return errs.NewFull(errs.CodeIOReadFail, err, "open Binance Vision CSV %s", file.Name)
		}
		csvReader := csv.NewReader(reader)
		csvReader.FieldsPerRecord = -1
		lineNum := 0
		for {
			row, readErr := csvReader.Read()
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				_ = reader.Close()
				return errs.NewFull(errs.CodeInvalidData, readErr,
					"read Binance Vision CSV %s line %d", file.Name, lineNum+1)
			}
			lineNum++
			if len(row) == 0 || (len(row) > 0 && isHeader != nil && isHeader(row[0])) {
				continue
			}
			if rowErr := onRow(row); rowErr != nil {
				_ = reader.Close()
				return errs.NewFull(errs.CodeInvalidData, rowErr,
					"parse Binance Vision CSV %s line %d", file.Name, lineNum)
			}
		}
		if err := reader.Close(); err != nil {
			return errs.NewFull(errs.CodeIOReadFail, err, "close Binance Vision CSV %s", file.Name)
		}
	}
	if !foundCSV {
		return errs.NewMsg(errs.CodeInvalidData, "Binance Vision zip contains no CSV")
	}
	return nil
}

func parseVisionKlines(data []byte, startMS, endMS int64, inverse bool) ([]*banexg.Kline, *errs.Error) {
	var result []*banexg.Kline
	err := readVisionZipCSV(data, isVisionHeader, func(row []string) error {
		kline, rowErr := parseVisionKlineRow(row, inverse)
		if rowErr != nil {
			return rowErr
		}
		if kline.Time >= startMS && kline.Time < endMS {
			result = append(result, kline)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return dedupeVisionKlines(result), nil
}

func parseVisionPriceKlines(data []byte, startMS, endMS int64) ([]*banexg.Kline, *errs.Error) {
	var result []*banexg.Kline
	err := readVisionZipCSV(data, isVisionHeader, func(row []string) error {
		kline, rowErr := parseVisionPriceKlineRow(row)
		if rowErr != nil {
			return rowErr
		}
		if kline.Time >= startMS && kline.Time < endMS {
			result = append(result, kline)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return dedupeVisionKlines(result), nil
}

func parseVisionPriceKlineRow(row []string) (*banexg.Kline, error) {
	if len(row) < 5 {
		return nil, fmt.Errorf("expected at least 5 columns, got %d", len(row))
	}
	openTime, err := strconv.ParseInt(strings.TrimSpace(row[0]), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("open time: %w", err)
	}
	if openTime >= 100_000_000_000_000 {
		openTime /= 1000
	}
	open, err := parseVisionFloat(row, 1)
	if err != nil {
		return nil, err
	}
	high, err := parseVisionFloat(row, 2)
	if err != nil {
		return nil, err
	}
	low, err := parseVisionFloat(row, 3)
	if err != nil {
		return nil, err
	}
	closePrice, err := parseVisionFloat(row, 4)
	if err != nil {
		return nil, err
	}
	return &banexg.Kline{
		Time:  openTime,
		Open:  open,
		High:  high,
		Low:   low,
		Close: closePrice,
	}, nil
}

func parseVisionFundingRates(data []byte, symbol string, startMS, endMS int64) ([]*banexg.FundingRate, *errs.Error) {
	var result []*banexg.FundingRate
	err := readVisionZipCSV(data, isVisionFundingHeader, func(row []string) error {
		item, rowErr := parseVisionFundingRateRow(row, symbol)
		if rowErr != nil {
			return rowErr
		}
		if item.Timestamp >= startMS && item.Timestamp <= endMS {
			result = append(result, item)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return dedupeVisionFundingRates(result), nil
}

func isVisionFundingHeader(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "calc_time" || value == "calc time"
}

func parseVisionFundingRateRow(row []string, symbol string) (*banexg.FundingRate, error) {
	if len(row) < 2 {
		return nil, fmt.Errorf("expected at least 2 columns, got %d", len(row))
	}
	calcTime, err := strconv.ParseInt(strings.TrimSpace(row[0]), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("calc time: %w", err)
	}
	if calcTime >= 100_000_000_000_000 {
		calcTime /= 1000
	}
	rateIndex := 1
	var intervalHours int64
	if len(row) >= 3 {
		rateIndex = 2
		if rawInterval := strings.TrimSpace(row[1]); rawInterval != "" {
			parsedInterval, intErr := strconv.ParseInt(rawInterval, 10, 64)
			if intErr != nil {
				return nil, fmt.Errorf("funding interval hours: %w", intErr)
			}
			intervalHours = parsedInterval
		}
	}
	rate, err := parseVisionFloat(row, rateIndex)
	if err != nil {
		return nil, err
	}
	info := map[string]interface{}{
		"calcTime":        calcTime,
		"lastFundingRate": strings.TrimSpace(row[rateIndex]),
	}
	if intervalHours > 0 {
		info["fundingIntervalHours"] = intervalHours
	}
	return &banexg.FundingRate{
		Symbol:      symbol,
		FundingRate: rate,
		Timestamp:   calcTime,
		Info:        info,
	}, nil
}

func dedupeVisionFundingRates(rates []*banexg.FundingRate) []*banexg.FundingRate {
	sort.SliceStable(rates, func(i, j int) bool { return rates[i].Timestamp < rates[j].Timestamp })
	result := rates[:0]
	for _, rate := range rates {
		if len(result) > 0 && result[len(result)-1].Timestamp == rate.Timestamp {
			result[len(result)-1] = rate
			continue
		}
		result = append(result, rate)
	}
	return result
}

func isVisionHeader(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "open time" || value == "open_time"
}

func parseVisionKlineRow(row []string, inverse bool) (*banexg.Kline, error) {
	if len(row) < 11 {
		return nil, fmt.Errorf("expected at least 11 columns, got %d", len(row))
	}
	openTime, err := strconv.ParseInt(strings.TrimSpace(row[0]), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("open time: %w", err)
	}
	if openTime >= 100_000_000_000_000 {
		openTime /= 1000
	}
	open, err := parseVisionFloat(row, 1)
	if err != nil {
		return nil, err
	}
	high, err := parseVisionFloat(row, 2)
	if err != nil {
		return nil, err
	}
	low, err := parseVisionFloat(row, 3)
	if err != nil {
		return nil, err
	}
	closePrice, err := parseVisionFloat(row, 4)
	if err != nil {
		return nil, err
	}
	volumeIndex, buyVolumeIndex := 5, 9
	if inverse {
		volumeIndex, buyVolumeIndex = 7, 10
	}
	volume, err := parseVisionFloat(row, volumeIndex)
	if err != nil {
		return nil, err
	}
	quote, err := parseVisionFloat(row, 7)
	if err != nil {
		return nil, err
	}
	buyVolume, err := parseVisionFloat(row, buyVolumeIndex)
	if err != nil {
		return nil, err
	}
	tradeNum, err := strconv.ParseInt(strings.TrimSpace(row[8]), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("trade number: %w", err)
	}
	return &banexg.Kline{
		Time:      openTime,
		Open:      open,
		High:      high,
		Low:       low,
		Close:     closePrice,
		Volume:    volume,
		Quote:     quote,
		BuyVolume: buyVolume,
		TradeNum:  tradeNum,
	}, nil
}

func parseVisionFloat(row []string, index int) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(row[index]), 64)
	if err != nil {
		return 0, fmt.Errorf("column %d: %w", index, err)
	}
	return value, nil
}

func dedupeVisionKlines(klines []*banexg.Kline) []*banexg.Kline {
	sort.SliceStable(klines, func(i, j int) bool { return klines[i].Time < klines[j].Time })
	result := klines[:0]
	for _, kline := range klines {
		if len(result) > 0 && result[len(result)-1].Time == kline.Time {
			result[len(result)-1] = kline
			continue
		}
		result = append(result, kline)
	}
	return result
}
