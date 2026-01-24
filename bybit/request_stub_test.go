package bybit

import (
	"context"
	"sync"
	"testing"

	"github.com/banbox/banexg"
)

var (
	bybitTestReqMu sync.Mutex
	bybitTestReqFn func(ctx context.Context, endpoint string, params map[string]interface{}, retryNum int, readCache, writeCache bool) *banexg.HttpRes
)

func setBybitTestRequest(t *testing.T, fn func(ctx context.Context, endpoint string, params map[string]interface{}, retryNum int, readCache, writeCache bool) *banexg.HttpRes) {
	t.Helper()
	bybitTestReqMu.Lock()
	bybitTestReqFn = fn
	bybitTestReqMu.Unlock()
	t.Cleanup(func() {
		bybitTestReqMu.Lock()
		bybitTestReqFn = nil
		bybitTestReqMu.Unlock()
	})
}

func (e *Bybit) RequestApiRetryAdv(ctx context.Context, endpoint string, params map[string]interface{}, retryNum int, readCache, writeCache bool) *banexg.HttpRes {
	bybitTestReqMu.Lock()
	fn := bybitTestReqFn
	bybitTestReqMu.Unlock()
	if fn != nil {
		return fn(ctx, endpoint, params, retryNum, readCache, writeCache)
	}
	return e.Exchange.RequestApiRetryAdv(ctx, endpoint, params, retryNum, readCache, writeCache)
}
