package banexg

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/banbox/banexg/errs"
)

type retrySequenceTransport struct {
	status int
	fails  int32
	calls  atomic.Int32
}

func (t *retrySequenceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	call := t.calls.Add(1)
	if call <= t.fails {
		return nil, io.ErrUnexpectedEOF
	}
	status := http.StatusOK
	header := make(http.Header)
	if call == 1 {
		status = t.status
		header.Set("Retry-After", "0")
	}
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader("{}")),
		Request:    req,
	}, nil
}

func TestRequestApiRetryRetriesTransientNetworkFailures(t *testing.T) {
	transport := &retrySequenceTransport{fails: 2}
	host := "retry-transient-network"
	api := &Entry{RawHost: host, Url: "http://" + host + "/test", Method: http.MethodGet}
	exchange := &Exchange{
		ExgInfo:         &ExgInfo{ReqHeaders: map[string]string{}},
		Apis:            map[string]*Entry{"test": api},
		HttpClient:      &http.Client{Transport: transport},
		EnableRateLimit: BoolFalse,
		Sign: func(api *Entry, _ map[string]interface{}) *HttpReq {
			return &HttpReq{Url: api.Url, Method: api.Method, Headers: make(http.Header)}
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res := exchange.RequestApiRetry(ctx, "test", nil, 2)
	if res.Error != nil {
		t.Fatalf("transient network failures were not retried: %v", res.Error)
	}
	if got := transport.calls.Load(); got != 3 {
		t.Fatalf("request calls = %d, want 3", got)
	}
}

func TestRequestApiRetryRetriesNeutralRateLimitErrors(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusTeapot} {
		transport := &retrySequenceTransport{status: status}
		host := fmt.Sprintf("retry-neutral-code-%d", status)
		api := &Entry{RawHost: host, Url: "http://" + host + "/test", Method: http.MethodGet}
		exchange := &Exchange{
			ExgInfo:         &ExgInfo{ReqHeaders: map[string]string{}},
			Apis:            map[string]*Entry{"test": api},
			HttpClient:      &http.Client{Transport: transport},
			EnableRateLimit: BoolFalse,
			Sign: func(api *Entry, _ map[string]interface{}) *HttpReq {
				return &HttpReq{Url: api.Url, Method: api.Method, Headers: make(http.Header)}
			},
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		res := exchange.RequestApiRetry(ctx, "test", nil, 1)
		cancel()
		if res.Error != nil {
			t.Fatalf("status %d: retry failed: %v", status, res.Error)
		}
		if got := transport.calls.Load(); got != 2 {
			t.Fatalf("status %d: request calls = %d, want 2", status, got)
		}
	}
}

func TestRequestApiRetryRetriesNonRiskyExecutionUnknown(t *testing.T) {
	transport := &retrySequenceTransport{status: http.StatusInternalServerError}
	host := "retry-non-risky-execution-unknown"
	api := &Entry{RawHost: host, Url: "http://" + host + "/test", Method: http.MethodGet}
	exchange := &Exchange{
		ExgInfo:         &ExgInfo{ReqHeaders: map[string]string{}},
		Apis:            map[string]*Entry{"test": api},
		HttpClient:      &http.Client{Transport: transport},
		EnableRateLimit: BoolFalse,
		MapApiError: func(_ *Entry, _ int, _ string) *errs.Error {
			return errs.NewMsg(errs.CodeExecutionUnknown, "backend timeout")
		},
		Sign: func(api *Entry, _ map[string]interface{}) *HttpReq {
			return &HttpReq{Url: api.Url, Method: api.Method, Headers: make(http.Header)}
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res := exchange.RequestApiRetry(ctx, "test", nil, 1)
	if res.Error != nil {
		t.Fatalf("retry failed: %v", res.Error)
	}
	if got := transport.calls.Load(); got != 2 {
		t.Fatalf("request calls = %d, want 2", got)
	}
}

func TestRequestApiRetryDoesNotRetryRiskyExecutionUnknown(t *testing.T) {
	transport := &retrySequenceTransport{status: http.StatusInternalServerError}
	host := "retry-risky-execution-unknown"
	api := &Entry{RawHost: host, Url: "http://" + host + "/test", Method: http.MethodGet, Risky: true}
	exchange := &Exchange{
		ExgInfo:         &ExgInfo{ReqHeaders: map[string]string{}},
		Apis:            map[string]*Entry{"test": api},
		HttpClient:      &http.Client{Transport: transport},
		EnableRateLimit: BoolFalse,
		MapApiError: func(_ *Entry, _ int, _ string) *errs.Error {
			return errs.NewMsg(errs.CodeExecutionUnknown, "backend timeout")
		},
		Sign: func(api *Entry, _ map[string]interface{}) *HttpReq {
			return &HttpReq{Url: api.Url, Method: api.Method, Headers: make(http.Header)}
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	res := exchange.RequestApiRetry(ctx, "test", nil, 1)
	if res.Error == nil || res.Error.Code != errs.CodeExecutionUnknown {
		t.Fatalf("expected execution-unknown error, got %v", res.Error)
	}
	if got := transport.calls.Load(); got != 1 {
		t.Fatalf("request calls = %d, want 1", got)
	}
}

func TestRateLimitErrorCodesRemainExchangeNeutral(t *testing.T) {
	if errs.CodeRateLimit == http.StatusTooManyRequests || errs.CodeTemporarilyBanned == http.StatusTeapot {
		t.Fatal("neutral rate-limit codes must not depend on HTTP status values")
	}
}
