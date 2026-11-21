package binance

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
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
	relPath := fmt.Sprintf("%s/%s/klines/%s/%s/%s-%s-%s.zip",
		marketPath, file.granularity, marketID, timeframe, marketID, timeframe, file.period)
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
