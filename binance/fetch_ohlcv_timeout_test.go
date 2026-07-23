package binance

import (
	"testing"
	"time"
)

func TestFetchOHLCVContextHasDeadline(t *testing.T) {
	old := fetchOHLCVTimeout
	fetchOHLCVTimeout = 20 * time.Millisecond
	t.Cleanup(func() { fetchOHLCVTimeout = old })
	ctx, cancel := fetchOHLCVContext()
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > fetchOHLCVTimeout {
		t.Fatalf("deadline=%v ok=%v", deadline, ok)
	}
}
