package bybit

import (
	"testing"

	"github.com/banbox/banexg/errs"
)

func TestNormalizeBybitLoopIntv(t *testing.T) {
	t.Parallel()
	got, err := normalizeBybitLoopIntv(bybitHistoryWindowMS+1, false)
	if err == nil {
		t.Fatalf("expected error, got nil (got=%d)", got)
	}
	got, err = normalizeBybitLoopIntv(bybitHistoryWindowMS+1, true)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if got != bybitHistoryWindowMS {
		t.Fatalf("expected %d, got %d", bybitHistoryWindowMS, got)
	}
}

func TestBybitLoopTimeRange_EndToStart(t *testing.T) {
	t.Parallel()
	now := int64(10_000_000_000)
	since := now - 30*24*60*60*1000
	until := now
	loopIntv := bybitHistoryWindowMS

	type win struct{ s, e int64 }
	var wins []win
	err := bybitLoopTimeRange(since, until, loopIntv, "endToStart", now, func(s, e int64) (bool, *errs.Error) {
		wins = append(wins, win{s: s, e: e})
		return false, nil
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(wins) < 2 {
		t.Fatalf("expected multiple windows, got %d", len(wins))
	}
	for i, w := range wins {
		if w.e <= w.s {
			t.Fatalf("invalid window %d: start=%d end=%d", i, w.s, w.e)
		}
		if w.e-w.s > bybitHistoryWindowMS {
			t.Fatalf("window too large %d: %d", i, w.e-w.s)
		}
		if i > 0 && wins[i-1].s != w.e {
			t.Fatalf("windows not contiguous at %d: prevStart=%d currEnd=%d", i, wins[i-1].s, w.e)
		}
	}
	if wins[len(wins)-1].s != since {
		t.Fatalf("expected last window start == since (%d), got %d", since, wins[len(wins)-1].s)
	}
	if wins[0].e != until {
		t.Fatalf("expected first window end == until (%d), got %d", until, wins[0].e)
	}
}

func TestBybitLoopTimeRange_StartToEndRequiresSince(t *testing.T) {
	t.Parallel()
	now := int64(10_000_000_000)
	err := bybitLoopTimeRange(0, now, bybitHistoryWindowMS, "startToEnd", now, func(s, e int64) (bool, *errs.Error) {
		return false, nil
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}
