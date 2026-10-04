package binance

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/sasha-s/go-deadlock"
)

func TestExecutionWatchMyTradesCanceledParameters(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	params := map[string]interface{}{banexg.ParamContext: ctx, banexg.ParamAccount: "acct"}
	e := &Binance{}
	if _, err := e.WatchMyTrades(params); err == nil {
		t.Fatal("expected canceled context error")
	}
	if _, ok := params[banexg.ParamContext]; !ok {
		t.Fatal("WatchMyTrades modified caller parameters")
	}
}

func TestExecutionWatchMyTradesCancelsHandshake(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	e := fundingTestExchange(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"listenKey":"unit-listen-key"}`)
	})
	e.Accounts["shared"].LockData = &deadlock.Mutex{}
	e.Accounts["shared"].Data = map[string]interface{}{}
	e.Hosts.Prod[banexg.MarketLinear] = "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		select {
		case <-started:
			cancel()
		case <-ctx.Done():
		}
	}()
	start := time.Now()
	_, err := e.WatchMyTrades(map[string]interface{}{banexg.ParamContext: ctx, banexg.ParamAccount: "shared"})
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("handshake cancellation ignored: %v elapsed=%s", err, time.Since(start))
	}
	if len(e.WSClients) != 0 {
		t.Fatal("failed initialization retained a websocket client")
	}
}

func TestExecutionListenKeyRefreshOutlivesRequestContext(t *testing.T) {
	var refreshed atomic.Bool
	e := fundingTestExchange(t, func(w http.ResponseWriter, r *http.Request) {
		refreshed.Store(true)
		fmt.Fprint(w, `{"listenKey":"unit-listen-key"}`)
	})
	acc := e.Accounts["shared"]
	acc.LockData = &deadlock.Mutex{}
	acc.Data = map[string]interface{}{banexg.MarketLinear + banexg.MidListenKey: "unit-listen-key"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	params := map[string]interface{}{banexg.ParamContext: ctx, banexg.ParamAccount: "shared"}
	e.keepAliveListenKeyRetry(acc, params, 0)
	if !refreshed.Load() {
		t.Fatal("listen key refresh retained canceled initialization context")
	}
	if _, ok := params[banexg.ParamContext]; !ok {
		t.Fatal("refresh modified caller parameters")
	}
}

func TestExecutionWatchMyTradesColdMarketsDeadline(t *testing.T) {
	e := fundingTestExchange(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	e.Name = t.Name()
	e.Markets = nil
	e.FetchCurrencies = func(map[string]interface{}) (banexg.CurrencyMap, *errs.Error) { return nil, nil }
	e.FetchMarkets = func(_ []string, params map[string]interface{}) (banexg.MarketMap, *errs.Error) {
		rsp := e.RequestApiRetryAdv(context.Background(), MethodFapiPrivateGetIncome, params, 0, false, false)
		return nil, rsp.Error
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := e.WatchMyTrades(map[string]interface{}{banexg.ParamContext: ctx, banexg.ParamAccount: "shared"})
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("cold market request lost deadline: %v elapsed=%s", err, time.Since(start))
	}
}
