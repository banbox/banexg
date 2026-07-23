package banexg

import (
	"context"
	"testing"
	"time"

	"github.com/banbox/banexg/errs"
)

func TestWaitRequestContextStopsBackoffAtDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := waitRequestContext(ctx, time.Second); err == nil {
		t.Fatal("cancelled backoff returned no error")
	}
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("cancelled backoff took %s", elapsed)
	}
}

func TestRequestApiStopsWaitingForHostSlotAtDeadline(t *testing.T) {
	const host = "context-timeout.test"
	sem := GetHostFlowChan(host)
	for i := 0; i < cap(sem); i++ {
		sem <- struct{}{}
	}
	t.Cleanup(func() {
		for i := 0; i < cap(sem); i++ {
			<-sem
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	result := (&Exchange{}).RequestApi(ctx, "", &Entry{RawHost: host}, nil, false, false)
	if result == nil || result.Error == nil || result.Error.Code != errs.CodeTimeout {
		t.Fatalf("host-slot timeout result = %#v", result)
	}
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("host-slot timeout took %s", elapsed)
	}
}
