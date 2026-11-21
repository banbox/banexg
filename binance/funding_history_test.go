package binance

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/banbox/banexg"
)

func TestFetchFundingRateHistoryPaginatesDynamicIntervalsWithoutGap(t *testing.T) {
	// Generate 1001 settlements: 999 at 8h intervals, then transition to 4h and 1h,
	// so page 1 has 1000 rows and page 2 has a single row (previously panicked on list[1]).
	const startMS = int64(1700006400000)
	const hourMS = int64(3600 * 1000)
	all := make([]map[string]interface{}, 0, 1001)
	cur := startMS
	for i := 0; i < 1001; i++ {
		rate := 0.0001
		if i%2 == 1 {
			rate = -0.0002
		}
		all = append(all, map[string]interface{}{
			"symbol":      "BTCUSDT",
			"fundingTime": cur,
			"fundingRate": strconv.FormatFloat(rate, 'f', 8, 64),
			"markPrice":   "40000",
		})
		switch {
		case i < 998:
			cur += 8 * hourMS
		case i == 998:
			cur += 4 * hourMS
		default:
			cur += 1 * hourMS
		}
	}
	untilMS := all[len(all)-1]["fundingTime"].(int64)

	calls := 0
	e := fundingTestExchange(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		q := r.URL.Query()
		reqStart, _ := strconv.ParseInt(q.Get("startTime"), 10, 64)
		reqEnd, _ := strconv.ParseInt(q.Get("endTime"), 10, 64)
		reqLimit, _ := strconv.Atoi(q.Get("limit"))
		if reqLimit <= 0 {
			reqLimit = 1000
		}
		page := make([]map[string]interface{}, 0)
		for _, row := range all {
			ts := row["fundingTime"].(int64)
			if ts >= reqStart && (reqEnd <= 0 || ts <= reqEnd) {
				page = append(page, row)
				if len(page) >= reqLimit {
					break
				}
			}
		}
		_ = json.NewEncoder(w).Encode(page)
	})

	items, err := e.FetchFundingRateHistory("BTC/USDT:USDT", startMS, 0, map[string]interface{}{
		banexg.ParamUntil: untilMS,
	})
	if err != nil {
		t.Fatalf("FetchFundingRateHistory failed: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 paginated API calls, got %d", calls)
	}
	if len(items) != len(all) {
		t.Fatalf("expected all %d settlements without gap, got %d", len(all), len(items))
	}
	for i := 1; i < len(items); i++ {
		if items[i-1].Timestamp >= items[i].Timestamp {
			t.Fatalf("items not strictly ascending at %d: %d >= %d", i, items[i-1].Timestamp, items[i].Timestamp)
		}
	}
	if items[0].Timestamp != startMS || items[len(items)-1].Timestamp != untilMS {
		t.Fatalf("unexpected range [%d, %d], want [%d, %d]", items[0].Timestamp, items[len(items)-1].Timestamp, startMS, untilMS)
	}
}
