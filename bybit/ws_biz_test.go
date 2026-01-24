package bybit

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
)

// ============================================================================
// Helper functions for WebSocket tests
// ============================================================================

// WS client helpers
func bybitWsConnIDs(client *banexg.WsClient) []int {
	if client == nil {
		return nil
	}
	conns, lock := client.LockConns()
	ids := make([]int, 0, len(conns))
	for id := range conns {
		ids = append(ids, id)
	}
	lock.Unlock()
	sort.Ints(ids)
	return ids
}

func bybitWsHasSubKey(client *banexg.WsClient, want string) bool {
	if client == nil || want == "" {
		return false
	}
	for _, cid := range bybitWsConnIDs(client) {
		for _, key := range client.GetSubKeys(cid) {
			if key == want {
				return true
			}
		}
	}
	return false
}

func bybitWsPrivateClientMust(t *testing.T, exg *Bybit, params map[string]interface{}) *banexg.WsClient {
	t.Helper()
	if exg == nil {
		t.Fatal("bybit exchange not initialized")
	}
	wsURL := exg.GetHost(HostWsPrivate)
	if wsURL == "" {
		t.Fatal("bybit private ws host missing")
	}
	accKey := exg.GetAccName(params)
	acc, err := exg.GetAccount(accKey)
	if err != nil || acc == nil || acc.Name == "" {
		t.Fatal("bybit account name empty")
	}
	clientKey := acc.Name + "@" + wsURL
	client, ok := exg.WSClients[clientKey]
	if !ok || client == nil {
		t.Fatalf("bybit ws client not found: %s", clientKey)
	}
	return client
}

// Trade helpers
func bybitTradeIsValid(tr *banexg.Trade) bool {
	if tr == nil {
		return false
	}
	if tr.Symbol == "" {
		return false
	}
	if tr.Price <= 0 || tr.Amount <= 0 || tr.Timestamp <= 0 {
		return false
	}
	return true
}

func bybitOptionBaseFromSymbol(exg *Bybit, symbol string) (string, bool) {
	if exg == nil || symbol == "" {
		return "", false
	}
	m, err := exg.GetMarket(symbol)
	if err != nil || m == nil || !m.Option || m.Base == "" {
		return "", false
	}
	return m.Base, true
}

func bybitPickOptionTradeSymbolsDistinctBases(t *testing.T, exg *Bybit, wantBases int) []string {
	t.Helper()
	if exg == nil {
		return nil
	}
	if wantBases <= 0 {
		wantBases = 1
	}
	candidates := bybitPickSymbolsForWs(t, exg, banexg.MarketOption, 20)
	if len(candidates) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, wantBases)
	out := make([]string, 0, wantBases)
	for _, sym := range candidates {
		base, ok := bybitOptionBaseFromSymbol(exg, sym)
		if !ok {
			continue
		}
		if _, exists := seen[base]; exists {
			continue
		}
		seen[base] = struct{}{}
		out = append(out, sym)
		if len(out) >= wantBases {
			break
		}
	}
	return out
}

func bybitOptionBasesSet(t *testing.T, exg *Bybit, symbols []string) map[string]struct{} {
	t.Helper()
	if exg == nil {
		return nil
	}
	out := make(map[string]struct{}, len(symbols))
	for _, sym := range symbols {
		base, ok := bybitOptionBaseFromSymbol(exg, sym)
		if !ok {
			continue
		}
		out[base] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func bybitTryWaitWsOptionTradeAnyBase(exg *Bybit, ch <-chan *banexg.Trade, wantBases map[string]struct{}, timeout time.Duration) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		select {
		case tr, ok := <-ch:
			if !ok {
				return false
			}
			if !bybitTradeIsValid(tr) {
				continue
			}
			base, ok := bybitOptionBaseFromSymbol(exg, tr.Symbol)
			if !ok {
				continue
			}
			if _, ok := wantBases[base]; !ok {
				continue
			}
			return true
		case <-deadline.C:
			return false
		}
	}
}

func bybitWaitWsTradeMatch(t *testing.T, ch <-chan *banexg.Trade, timeout time.Duration, accept func(*banexg.Trade) bool, onTimeout string) *banexg.Trade {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		select {
		case tr, ok := <-ch:
			if !ok {
				t.Fatalf("trades channel closed early")
			}
			if !bybitTradeIsValid(tr) {
				continue
			}
			if accept != nil && !accept(tr) {
				continue
			}
			return tr
		case <-deadline.C:
			t.Fatalf("%s (%s)", onTimeout, timeout)
		}
	}
}

func bybitWaitWsTrade(t *testing.T, ch <-chan *banexg.Trade, wantSymbol string, timeout time.Duration) *banexg.Trade {
	t.Helper()
	return bybitWaitWsTradeMatch(
		t,
		ch,
		timeout,
		func(tr *banexg.Trade) bool { return tr.Symbol == wantSymbol },
		"timeout waiting ws trade for "+wantSymbol,
	)
}

func bybitWaitWsTradesSeenAll(t *testing.T, ch <-chan *banexg.Trade, wantSymbols []string, timeout time.Duration) map[string]*banexg.Trade {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	need := make(map[string]struct{}, len(wantSymbols))
	seen := make(map[string]*banexg.Trade, len(wantSymbols))
	for _, s := range wantSymbols {
		if s != "" {
			need[s] = struct{}{}
		}
	}

	for len(seen) < len(need) {
		select {
		case tr, ok := <-ch:
			if !ok {
				t.Fatalf("trades channel closed early; seen=%v need=%v", seen, wantSymbols)
			}
			if !bybitTradeIsValid(tr) {
				continue
			}
			if _, want := need[tr.Symbol]; !want {
				continue
			}
			if _, done := seen[tr.Symbol]; done {
				continue
			}
			seen[tr.Symbol] = tr
		case <-deadline.C:
			t.Fatalf("timeout waiting ws trades multi-symbol; seen=%v need=%v (%s)", seen, wantSymbols, timeout)
		}
	}
	return seen
}

// ============================================================================
// WatchTrades tests
// ============================================================================

func bybitWatchTradesOne(t *testing.T, marketType string, params map[string]interface{}) {
	t.Helper()
	exg := getBybitOrSkipNoCurr(t, nil)
	if exg == nil {
		return
	}
	defer func() { _ = exg.Close() }()

	symbols := bybitPickSymbolsForWs(t, exg, marketType, 1)
	if len(symbols) == 0 {
		return
	}
	symbol := symbols[0]

	ch, err := exg.WatchTrades([]string{symbol}, params)
	if err != nil {
		t.Fatalf("WatchTrades(%s) failed: %v", marketType, err)
	}
	defer func() { _ = exg.UnWatchTrades([]string{symbol}, params) }()

	_ = bybitWaitWsTrade(t, ch, symbol, 20*time.Second)
}

func TestApi_WatchTrades_Spot(t *testing.T) {
	bybitWatchTradesOne(t, banexg.MarketSpot, nil)
}

func TestApi_WatchTrades_Linear(t *testing.T) {
	bybitWatchTradesOne(t, banexg.MarketLinear, nil)
}

func TestApi_WatchTrades_Inverse(t *testing.T) {
	bybitWatchTradesOne(t, banexg.MarketInverse, nil)
}

func TestApi_WatchTrades_Option(t *testing.T) {
	exg := getBybitOrSkipNoCurr(t, nil)
	if exg == nil {
		return
	}
	defer func() { _ = exg.Close() }()

	subSymbols := bybitPickOptionTradeSymbolsDistinctBases(t, exg, 2)
	if len(subSymbols) == 0 {
		t.Skip("no option markets with baseCoin found")
		return
	}

	ch, err := exg.WatchTrades(subSymbols, nil)
	if err != nil {
		t.Fatalf("WatchTrades option failed: %v", err)
	}
	defer func() { _ = exg.UnWatchTrades(subSymbols, nil) }()

	wantBases := bybitOptionBasesSet(t, exg, subSymbols)
	if wantBases == nil {
		t.Skip("no option markets with baseCoin found")
		return
	}
	if !bybitTryWaitWsOptionTradeAnyBase(exg, ch, wantBases, 40*time.Second) {
		t.Skip("no option trade updates received (options might be inactive on the current environment)")
		return
	}
}

func TestApi_WatchTrades_Spot_MultiSymbol(t *testing.T) {
	exg := getBybitOrSkipNoCurr(t, nil)
	if exg == nil {
		return
	}
	defer func() { _ = exg.Close() }()

	symbols := bybitPickSymbolsForWs(t, exg, banexg.MarketSpot, 2)
	if len(symbols) < 2 {
		return
	}

	ch, err := exg.WatchTrades(symbols, nil)
	if err != nil {
		t.Fatalf("WatchTrades spot multi-symbol failed: %v", err)
	}
	defer func() { _ = exg.UnWatchTrades(symbols, nil) }()

	_ = bybitWaitWsTradesSeenAll(t, ch, symbols, 30*time.Second)
}

func TestApi_WatchTrades_Linear_ExplicitMarketType(t *testing.T) {
	bybitWatchTradesOne(t, banexg.MarketLinear, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
}

func TestApi_WatchTrades_ParamMarketMismatch_Error(t *testing.T) {
	exg := getBybitOrSkipNoCurr(t, nil)
	if exg == nil {
		return
	}
	defer func() { _ = exg.Close() }()

	symbols := bybitPickSymbolsForWs(t, exg, banexg.MarketSpot, 1)
	if len(symbols) == 0 {
		return
	}
	symbol := symbols[0]

	_, err := exg.WatchTrades([]string{symbol}, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
	if err == nil {
		t.Fatalf("expected WatchTrades to fail for ParamMarket mismatch (symbol=%s)", symbol)
	}
}

// ============================================================================
// WatchOHLCVs tests
// ============================================================================

func bybitWatchOHLCVsOne(t *testing.T, marketType string, timeframe string, params map[string]interface{}) {
	t.Helper()
	exg := getBybitOrSkipNoCurr(t, nil)
	if exg == nil {
		return
	}
	defer func() { _ = exg.Close() }()

	symbols := bybitPickSymbolsForWsKline(t, exg, marketType, 1)
	if len(symbols) == 0 {
		return
	}
	symbol := symbols[0]

	jobs := [][2]string{{symbol, timeframe}}
	ch, err := exg.WatchOHLCVs(jobs, params)
	if err != nil {
		t.Fatalf("WatchOHLCVs(%s %s) failed: %v", marketType, timeframe, err)
	}
	defer func() { _ = exg.UnWatchOHLCVs(jobs, params) }()

	got := bybitWaitWsKline(t, ch, symbol, timeframe, 25*time.Second)
	if got.Symbol != symbol {
		t.Fatalf("unexpected kline symbol: got %s want %s", got.Symbol, symbol)
	}
	if got.TimeFrame == "" {
		t.Fatalf("unexpected empty kline timeframe for %s", symbol)
	}
}

func TestApi_WatchOHLCVs_Spot_1m(t *testing.T) {
	bybitWatchOHLCVsOne(t, banexg.MarketSpot, "1m", nil)
}

func TestApi_WatchOHLCVs_Spot_1d(t *testing.T) {
	bybitWatchOHLCVsOne(t, banexg.MarketSpot, "1d", nil)
}

func TestApi_WatchOHLCVs_Linear_1m(t *testing.T) {
	bybitWatchOHLCVsOne(t, banexg.MarketLinear, "1m", nil)
}

func TestApi_WatchOHLCVs_Inverse_1m(t *testing.T) {
	bybitWatchOHLCVsOne(t, banexg.MarketInverse, "1m", nil)
}

func TestApi_WatchOHLCVs_Option_1m(t *testing.T) {
	exg := getBybitOrSkipNoCurr(t, nil)
	if exg == nil {
		return
	}
	defer func() { _ = exg.Close() }()

	symbols := bybitPickSymbolsForWsKline(t, exg, banexg.MarketOption, 6)
	if len(symbols) == 0 {
		return
	}
	jobs := make([][2]string, 0, len(symbols))
	want := make(map[string]struct{}, len(symbols))
	for _, sym := range symbols {
		jobs = append(jobs, [2]string{sym, "1m"})
		want[sym] = struct{}{}
	}

	ch, err := exg.WatchOHLCVs(jobs, nil)
	if err != nil {
		t.Fatalf("WatchOHLCVs option failed: %v", err)
	}
	defer func() { _ = exg.UnWatchOHLCVs(jobs, nil) }()

	if _, ok := bybitTryWaitWsKlineAnySymbol(ch, want, "1m", 35*time.Second); !ok {
		t.Skip("no option kline updates received (options might be inactive on the current environment)")
	}
}

func TestApi_WatchOHLCVs_Spot_MultiJob(t *testing.T) {
	exg := getBybitOrSkipNoCurr(t, nil)
	if exg == nil {
		return
	}
	defer func() { _ = exg.Close() }()

	symbols := bybitPickSymbolsForWsKline(t, exg, banexg.MarketSpot, 2)
	if len(symbols) == 0 {
		return
	}
	jobs := [][2]string{
		{symbols[0], "1m"},
		{symbols[1], "1h"},
	}

	ch, err := exg.WatchOHLCVs(jobs, nil)
	if err != nil {
		t.Fatalf("WatchOHLCVs multi-job failed: %v", err)
	}
	defer func() { _ = exg.UnWatchOHLCVs(jobs, nil) }()

	want := map[string]bool{
		symbols[0] + "@1m": false,
		symbols[1] + "@1h": false,
	}
	deadline := time.NewTimer(35 * time.Second)
	defer deadline.Stop()
	for {
		all := true
		for _, ok := range want {
			all = all && ok
		}
		if all {
			return
		}
		select {
		case item, ok := <-ch:
			if !ok {
				t.Fatalf("kline channel closed early; got=%v", want)
			}
			if item == nil || item.Symbol == "" || item.TimeFrame == "" || item.Time <= 0 {
				continue
			}
			if item.Symbol == symbols[0] && bybitTfMatch(item.TimeFrame, "1m") {
				want[symbols[0]+"@1m"] = true
			} else if item.Symbol == symbols[1] && bybitTfMatch(item.TimeFrame, "1h") {
				want[symbols[1]+"@1h"] = true
			}
		case <-deadline.C:
			missing := make([]string, 0, len(want))
			for k, ok := range want {
				if !ok {
					missing = append(missing, k)
				}
			}
			t.Fatalf("timeout waiting ws kline multi-job, missing=%v", missing)
		}
	}
}

func TestApi_WatchOHLCVs_Spot_ExplicitMarketType(t *testing.T) {
	bybitWatchOHLCVsOne(t, banexg.MarketSpot, "1m", map[string]interface{}{
		banexg.ParamMarket: banexg.MarketSpot,
	})
}

// Kline helpers
func bybitTfMatch(got, want string) bool {
	if got == want {
		return true
	}
	switch want {
	case "1d", "D":
		return got == "1d" || got == "D"
	case "1w", "W":
		return got == "1w" || got == "W"
	case "1M", "M":
		return got == "1M" || got == "M"
	default:
		return false
	}
}

func bybitWaitWsKline(t *testing.T, ch <-chan *banexg.PairTFKline, wantSymbol, wantTF string, timeout time.Duration) *banexg.PairTFKline {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		select {
		case item, ok := <-ch:
			if !ok {
				t.Fatalf("kline channel closed before receiving %s@%s", wantSymbol, wantTF)
			}
			if item == nil || item.Symbol != wantSymbol || !bybitTfMatch(item.TimeFrame, wantTF) {
				continue
			}
			if item.Time <= 0 {
				continue
			}
			return item
		case <-deadline.C:
			t.Fatalf("timeout waiting ws kline for %s@%s (%s)", wantSymbol, wantTF, timeout)
		}
	}
}

func bybitTryWaitWsKlineAnySymbol(ch <-chan *banexg.PairTFKline, wantSymbols map[string]struct{}, wantTF string, timeout time.Duration) (*banexg.PairTFKline, bool) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		select {
		case item, ok := <-ch:
			if !ok {
				return nil, false
			}
			if item == nil || item.Symbol == "" || item.Time <= 0 || item.TimeFrame == "" || !bybitTfMatch(item.TimeFrame, wantTF) {
				continue
			}
			if _, exists := wantSymbols[item.Symbol]; !exists {
				continue
			}
			return item, true
		case <-deadline.C:
			return nil, false
		}
	}
}

func bybitPickSymbolsForWsKline(t *testing.T, exg *Bybit, marketType string, n int) []string {
	return bybitPickSymbolsForWs(t, exg, marketType, n)
}

// OrderBook helpers
func bybitWaitWsOrderBook(t *testing.T, ch <-chan *banexg.OrderBook, wantSymbol string, timeout time.Duration) *banexg.OrderBook {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		select {
		case book, ok := <-ch:
			if !ok {
				t.Fatalf("orderbook channel closed before receiving %s", wantSymbol)
			}
			if book == nil || book.Symbol != wantSymbol {
				continue
			}
			if book.Asks == nil || book.Bids == nil || len(book.Asks.Price) == 0 || len(book.Bids.Price) == 0 {
				continue
			}
			return book
		case <-deadline.C:
			t.Fatalf("timeout waiting ws orderbook for %s (%s)", wantSymbol, timeout)
		}
	}
}

func bybitPickSymbolsForWsOrderBook(t *testing.T, exg *Bybit, marketType string, n int) []string {
	return bybitPickSymbolsForWs(t, exg, marketType, n)
}

// MarkPrice helpers
func bybitWaitWsMarkPrice(t *testing.T, ch <-chan map[string]float64, wantSymbol string, timeout time.Duration) float64 {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		select {
		case mp, ok := <-ch:
			if !ok {
				t.Fatalf("markPrice channel closed before receiving %s", wantSymbol)
			}
			if mp == nil {
				continue
			}
			val, ok := mp[wantSymbol]
			if !ok || val <= 0 {
				continue
			}
			return val
		case <-deadline.C:
			t.Fatalf("timeout waiting ws markPrice for %s (%s)", wantSymbol, timeout)
		}
	}
}

func bybitTryWaitWsMarkPriceAnySymbol(ch <-chan map[string]float64, wantSymbols map[string]struct{}, timeout time.Duration) (string, float64, bool) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		select {
		case mp, ok := <-ch:
			if !ok {
				return "", 0, false
			}
			if mp == nil {
				continue
			}
			for sym, val := range mp {
				if val <= 0 {
					continue
				}
				if _, exists := wantSymbols[sym]; !exists {
					continue
				}
				return sym, val, true
			}
		case <-deadline.C:
			return "", 0, false
		}
	}
}

func bybitWaitWsMarkPricesSeenAll(t *testing.T, ch <-chan map[string]float64, wantSymbols []string, timeout time.Duration) map[string]float64 {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	need := make(map[string]struct{}, len(wantSymbols))
	seen := make(map[string]float64, len(wantSymbols))
	for _, s := range wantSymbols {
		if s != "" {
			need[s] = struct{}{}
		}
	}

	for len(seen) < len(need) {
		select {
		case mp, ok := <-ch:
			if !ok {
				t.Fatalf("markPrice channel closed early; seen=%v need=%v", seen, wantSymbols)
			}
			if mp == nil {
				continue
			}
			for sym := range need {
				if _, done := seen[sym]; done {
					continue
				}
				if val, ok := mp[sym]; ok && val > 0 {
					seen[sym] = val
				}
			}
		case <-deadline.C:
			t.Fatalf("timeout waiting ws markPrice multi-symbol, seen=%v need=%v (%s)", seen, wantSymbols, timeout)
		}
	}
	return seen
}

// Balance helpers
func bybitWaitWsBalance(t *testing.T, ch <-chan *banexg.Balances, timeout time.Duration) *banexg.Balances {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		select {
		case bal, ok := <-ch:
			if !ok {
				t.Fatalf("balance channel closed early")
			}
			if bal == nil || bal.Assets == nil || bal.Info == nil || bal.TimeStamp <= 0 {
				continue
			}
			return bal
		case <-deadline.C:
			t.Fatalf("timeout waiting ws balance (%s)", timeout)
		}
	}
}

// ============================================================================
// WatchOrderBooks tests
// ============================================================================

func bybitWatchOrderBooksOne(t *testing.T, marketType string, inLimit int, wantDepth int, params map[string]interface{}) {
	t.Helper()
	exg := getBybitOrSkipNoCurr(t, nil)
	if exg == nil {
		return
	}
	defer func() { _ = exg.Close() }()

	origin := exg.CareMarkets
	exg.CareMarkets = []string{marketType}
	defer func() { exg.CareMarkets = origin }()

	markets, err := exg.LoadMarkets(false, nil)
	if err != nil {
		t.Fatalf("LoadMarkets failed: %v", err)
	}
	candidates := pickBybitOrderBookMarkets(markets, marketType, 8)
	if len(candidates) == 0 {
		t.Skipf("no %s markets found", marketType)
		return
	}

	symbol := candidates[0].Symbol
	ch, err2 := exg.WatchOrderBooks([]string{symbol}, inLimit, params)
	if err2 != nil {
		t.Fatalf("WatchOrderBooks(%s limit=%d) failed: %v", marketType, inLimit, err2)
	}
	defer func() { _ = exg.UnWatchOrderBooks([]string{symbol}, params) }()

	book := bybitWaitWsOrderBook(t, ch, symbol, 12*time.Second)
	if book.Limit != wantDepth {
		t.Fatalf("unexpected ws orderbook depth: got %d want %d (market=%s limit=%d symbol=%s)", book.Limit, wantDepth, marketType, inLimit, symbol)
	}
}

func TestApi_WatchOrderBooks_Spot_DefaultDepth(t *testing.T) {
	bybitWatchOrderBooksOne(t, banexg.MarketSpot, 0, 50, nil)
}

func TestApi_WatchOrderBooks_Spot_Depth1(t *testing.T) {
	bybitWatchOrderBooksOne(t, banexg.MarketSpot, 1, 1, nil)
}

func TestApi_WatchOrderBooks_Spot_Depth200(t *testing.T) {
	bybitWatchOrderBooksOne(t, banexg.MarketSpot, 200, 200, nil)
}

func TestApi_WatchOrderBooks_Spot_Depth1000(t *testing.T) {
	bybitWatchOrderBooksOne(t, banexg.MarketSpot, 1000, 1000, nil)
}

func TestApi_WatchOrderBooks_Linear_DefaultDepth(t *testing.T) {
	bybitWatchOrderBooksOne(t, banexg.MarketLinear, 0, 50, nil)
}

func TestApi_WatchOrderBooks_Linear_Depth1(t *testing.T) {
	bybitWatchOrderBooksOne(t, banexg.MarketLinear, 1, 1, nil)
}

func TestApi_WatchOrderBooks_Linear_Depth200(t *testing.T) {
	bybitWatchOrderBooksOne(t, banexg.MarketLinear, 200, 200, nil)
}

func TestApi_WatchOrderBooks_Linear_Depth1000(t *testing.T) {
	bybitWatchOrderBooksOne(t, banexg.MarketLinear, 1000, 1000, nil)
}

func TestApi_WatchOrderBooks_Inverse_DefaultDepth(t *testing.T) {
	bybitWatchOrderBooksOne(t, banexg.MarketInverse, 0, 50, nil)
}

func TestApi_WatchOrderBooks_Inverse_Depth1(t *testing.T) {
	bybitWatchOrderBooksOne(t, banexg.MarketInverse, 1, 1, nil)
}

func TestApi_WatchOrderBooks_Inverse_Depth200(t *testing.T) {
	bybitWatchOrderBooksOne(t, banexg.MarketInverse, 200, 200, nil)
}

func TestApi_WatchOrderBooks_Inverse_Depth1000(t *testing.T) {
	bybitWatchOrderBooksOne(t, banexg.MarketInverse, 1000, 1000, nil)
}

func TestApi_WatchOrderBooks_Option_DefaultDepth(t *testing.T) {
	bybitWatchOrderBooksOne(t, banexg.MarketOption, 0, 25, nil)
}

func TestApi_WatchOrderBooks_Option_Depth100(t *testing.T) {
	bybitWatchOrderBooksOne(t, banexg.MarketOption, 100, 100, nil)
}

func TestApi_WatchOrderBooks_Spot_MultiSymbol(t *testing.T) {
	exg := getBybitOrSkipNoCurr(t, nil)
	if exg == nil {
		return
	}
	defer func() { _ = exg.Close() }()

	symbols := bybitPickSymbolsForWsOrderBook(t, exg, banexg.MarketSpot, 2)
	if len(symbols) == 0 {
		return
	}

	ch, err := exg.WatchOrderBooks(symbols, 50, nil)
	if err != nil {
		t.Fatalf("WatchOrderBooks multi-symbol failed: %v", err)
	}
	defer func() { _ = exg.UnWatchOrderBooks(symbols, nil) }()

	want := map[string]bool{symbols[0]: false, symbols[1]: false}
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	for {
		all := true
		for _, ok := range want {
			all = all && ok
		}
		if all {
			return
		}
		select {
		case book, ok := <-ch:
			if !ok {
				t.Fatalf("orderbook channel closed early; got=%v", want)
			}
			if book == nil || book.Symbol == "" {
				continue
			}
			if _, exists := want[book.Symbol]; !exists {
				continue
			}
			if book.Asks == nil || book.Bids == nil || len(book.Asks.Price) == 0 || len(book.Bids.Price) == 0 {
				continue
			}
			if book.Limit != 50 {
				t.Fatalf("unexpected ws orderbook depth for %s: got %d want %d", book.Symbol, book.Limit, 50)
			}
			want[book.Symbol] = true
		case <-deadline.C:
			missing := make([]string, 0, len(want))
			for sym, ok := range want {
				if !ok {
					missing = append(missing, sym)
				}
			}
			t.Fatalf("timeout waiting ws orderbook multi-symbol, missing=%v", missing)
		}
	}
}

func TestApi_WatchOrderBooks_Spot_ExplicitMarketType(t *testing.T) {
	bybitWatchOrderBooksOne(t, banexg.MarketSpot, 50, 50, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketSpot,
	})
}

// ============================================================================
// WatchMarkPrices tests
// ============================================================================

func bybitWatchMarkPricesOne(t *testing.T, marketType string, params map[string]interface{}) {
	t.Helper()
	exg := getBybitOrSkipNoCurr(t, nil)
	if exg == nil {
		return
	}
	defer func() { _ = exg.Close() }()

	symbols := bybitPickSymbolsForWsKline(t, exg, marketType, 1)
	if len(symbols) == 0 {
		return
	}
	symbol := symbols[0]

	ch, err := exg.WatchMarkPrices([]string{symbol}, params)
	if err != nil {
		t.Fatalf("WatchMarkPrices(%s) failed: %v", marketType, err)
	}
	defer func() { _ = exg.UnWatchMarkPrices([]string{symbol}, params) }()

	if got := bybitWaitWsMarkPrice(t, ch, symbol, 20*time.Second); got <= 0 {
		t.Fatalf("unexpected non-positive markPrice for %s: %v", symbol, got)
	}
}

func TestApi_WatchMarkPrices_Linear(t *testing.T) {
	bybitWatchMarkPricesOne(t, banexg.MarketLinear, nil)
}

func TestApi_WatchMarkPrices_Inverse(t *testing.T) {
	bybitWatchMarkPricesOne(t, banexg.MarketInverse, nil)
}

func TestApi_WatchMarkPrices_Option(t *testing.T) {
	exg := getBybitOrSkipNoCurr(t, nil)
	if exg == nil {
		return
	}
	defer func() { _ = exg.Close() }()

	symbols := bybitPickSymbolsForWsKline(t, exg, banexg.MarketOption, 6)
	if len(symbols) == 0 {
		return
	}
	want := make(map[string]struct{}, len(symbols))
	for _, sym := range symbols {
		want[sym] = struct{}{}
	}

	ch, err := exg.WatchMarkPrices(symbols, nil)
	if err != nil {
		t.Fatalf("WatchMarkPrices option failed: %v", err)
	}
	defer func() { _ = exg.UnWatchMarkPrices(symbols, nil) }()

	if _, _, ok := bybitTryWaitWsMarkPriceAnySymbol(ch, want, 35*time.Second); !ok {
		t.Skip("no option markPrice updates received (options might be inactive on the current environment)")
	}
}

func TestApi_WatchMarkPrices_Linear_MultiSymbol(t *testing.T) {
	exg := getBybitOrSkipNoCurr(t, nil)
	if exg == nil {
		return
	}
	defer func() { _ = exg.Close() }()

	symbols := bybitPickSymbolsForWsKline(t, exg, banexg.MarketLinear, 2)
	if len(symbols) < 2 {
		return
	}

	ch, err := exg.WatchMarkPrices(symbols, nil)
	if err != nil {
		t.Fatalf("WatchMarkPrices linear multi-symbol failed: %v", err)
	}
	defer func() { _ = exg.UnWatchMarkPrices(symbols, nil) }()

	_ = bybitWaitWsMarkPricesSeenAll(t, ch, symbols, 25*time.Second)
}

func TestApi_WatchMarkPrices_Linear_ExplicitMarketType(t *testing.T) {
	bybitWatchMarkPricesOne(t, banexg.MarketLinear, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	})
}

func TestApi_WatchMarkPrices_Spot_NotSupported(t *testing.T) {
	exg := getBybitOrSkipNoCurr(t, nil)
	if exg == nil {
		return
	}
	defer func() { _ = exg.Close() }()

	symbols := bybitPickSymbolsForWsKline(t, exg, banexg.MarketSpot, 1)
	if len(symbols) == 0 {
		return
	}
	symbol := symbols[0]

	_, err := exg.WatchMarkPrices([]string{symbol}, nil)
	if err == nil {
		t.Fatalf("expected WatchMarkPrices spot to fail, got nil error (symbol=%s)", symbol)
	}
}

// ============================================================================
// WatchBalance tests
// ============================================================================

func bybitWatchBalanceOnce(t *testing.T, params map[string]interface{}, wantAssets ...string) {
	t.Helper()
	exg := getBybitAuthed(t, nil)
	defer func() { _ = exg.Close() }()

	ch, err := exg.WatchBalance(params)
	if err != nil {
		t.Fatalf("WatchBalance failed: %v", err)
	}
	if ch == nil {
		t.Fatal("expected balance channel")
	}

	client := bybitWsPrivateClientMust(t, exg, params)
	if !bybitWsHasSubKey(client, "wallet") {
		t.Fatalf("expected ws to subscribe key %q, got connIDs=%v", "wallet", bybitWsConnIDs(client))
	}

	bal := bybitWaitWsBalance(t, ch, 20*time.Second)
	if len(wantAssets) > 0 {
		requireBalanceHasAssets(t, bal, wantAssets...)
	}
}

func TestApi_WatchBalance_DefaultParams(t *testing.T) {
	bybitWatchBalanceOnce(t, nil)
}

func TestApi_WatchBalance_ExplicitAccountType(t *testing.T) {
	bybitWatchBalanceOnce(t, map[string]interface{}{
		"accountType": "UNIFIED",
	})
}

func TestApi_WatchBalance_CoinOnly(t *testing.T) {
	bybitWatchBalanceOnce(t, map[string]interface{}{
		banexg.ParamCurrency: "USDT,BTC",
	}, "USDT", "BTC")
}

func TestApi_WatchBalance_CoinAndAccountType(t *testing.T) {
	bybitWatchBalanceOnce(t, map[string]interface{}{
		"accountType":        "UNIFIED",
		banexg.ParamCurrency: "USDT,BTC",
	}, "USDT", "BTC")
}

func TestApi_WatchBalance_ParamAccountOnly(t *testing.T) {
	bybitWatchBalanceOnce(t, map[string]interface{}{
		banexg.ParamAccount: ":first",
	})
}

func TestApi_WatchBalance_ParamAccountAndCoin(t *testing.T) {
	bybitWatchBalanceOnce(t, map[string]interface{}{
		banexg.ParamAccount:  ":first",
		banexg.ParamCurrency: "USDT,BTC",
	}, "USDT", "BTC")
}

func TestApi_WatchBalance_ParamAccountAndAccountType(t *testing.T) {
	bybitWatchBalanceOnce(t, map[string]interface{}{
		banexg.ParamAccount: ":first",
		"accountType":       "UNIFIED",
	})
}

func TestApi_WatchBalance_ParamAccountCoinAccountType(t *testing.T) {
	bybitWatchBalanceOnce(t, map[string]interface{}{
		banexg.ParamAccount:  ":first",
		"accountType":        "UNIFIED",
		banexg.ParamCurrency: "USDT,BTC",
	}, "USDT", "BTC")
}

// ============================================================================
// WatchMyTrades tests
// ============================================================================

func bybitWatchMyTradesRequireWsSubKey(t *testing.T, params map[string]interface{}, wantKey string) {
	t.Helper()
	exg := getBybitAuthed(t, nil)
	defer func() { _ = exg.Close() }()

	out, err := exg.WatchMyTrades(params)
	if err != nil {
		t.Fatalf("WatchMyTrades failed: %v", err)
	}
	if out == nil {
		t.Fatal("expected mytrades channel")
	}
	client := bybitWsPrivateClientMust(t, exg, params)
	if !bybitWsHasSubKey(client, wantKey) {
		t.Fatalf("expected ws to subscribe key %q, got connIDs=%v", wantKey, bybitWsConnIDs(client))
	}
}

func TestApi_WatchMyTrades_AllInOne(t *testing.T) {
	bybitWatchMyTradesRequireWsSubKey(t, nil, "execution")
}

func TestApi_WatchMyTrades_Categorised_Spot(t *testing.T) {
	bybitWatchMyTradesRequireWsSubKey(t, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketSpot,
	}, "execution.spot")
}

func TestApi_WatchMyTrades_Categorised_Linear(t *testing.T) {
	bybitWatchMyTradesRequireWsSubKey(t, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	}, "execution.linear")
}

func TestApi_WatchMyTrades_Categorised_Inverse(t *testing.T) {
	bybitWatchMyTradesRequireWsSubKey(t, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketInverse,
	}, "execution.inverse")
}

func TestApi_WatchMyTrades_Categorised_Option(t *testing.T) {
	bybitWatchMyTradesRequireWsSubKey(t, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketOption,
	}, "execution.option")
}

// ============================================================================
// WatchAccountConfig tests
// ============================================================================

func bybitWatchAccountConfigRequireWsSubKey(t *testing.T, params map[string]interface{}, wantKey string) {
	t.Helper()
	exg := getBybitAuthed(t, nil)
	defer func() { _ = exg.Close() }()

	out, err := exg.WatchAccountConfig(params)
	if err != nil {
		t.Fatalf("WatchAccountConfig failed: %v", err)
	}
	if out == nil {
		t.Fatal("expected accConfig channel")
	}
	client := bybitWsPrivateClientMust(t, exg, params)
	if !bybitWsHasSubKey(client, wantKey) {
		t.Fatalf("expected ws to subscribe key %q, got connIDs=%v", wantKey, bybitWsConnIDs(client))
	}
}

func TestApi_WatchAccountConfig_AllInOne(t *testing.T) {
	bybitWatchAccountConfigRequireWsSubKey(t, nil, "position")
}

func TestApi_WatchAccountConfig_Categorised_Linear(t *testing.T) {
	bybitWatchAccountConfigRequireWsSubKey(t, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketLinear,
	}, "position.linear")
}

func TestApi_WatchAccountConfig_Categorised_Inverse(t *testing.T) {
	bybitWatchAccountConfigRequireWsSubKey(t, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketInverse,
	}, "position.inverse")
}

func TestApi_WatchAccountConfig_Categorised_Option(t *testing.T) {
	bybitWatchAccountConfigRequireWsSubKey(t, map[string]interface{}{
		banexg.ParamMarket: banexg.MarketOption,
	}, "position.option")
}

func TestApi_WatchAccountConfig_SpotReject(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	defer func() { _ = exg.Close() }()

	out, err := exg.WatchAccountConfig(map[string]interface{}{
		banexg.ParamMarket: banexg.MarketSpot,
	})
	if err == nil {
		t.Fatalf("expected error for spot WatchAccountConfig, got out=%v", out != nil)
	}
	if err.Code != errs.CodeUnsupportMarket {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApi_WatchAccountConfig_MarginReject(t *testing.T) {
	exg := getBybitAuthed(t, nil)
	defer func() { _ = exg.Close() }()

	out, err := exg.WatchAccountConfig(map[string]interface{}{
		banexg.ParamMarket: banexg.MarketMargin,
	})
	if err == nil {
		t.Fatalf("expected error for margin WatchAccountConfig, got out=%v", out != nil)
	}
	if err.Code != errs.CodeUnsupportMarket {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApi_WatchAccountConfig_ParamAccountOnly(t *testing.T) {
	bybitWatchAccountConfigRequireWsSubKey(t, map[string]interface{}{
		banexg.ParamAccount: ":first",
	}, "position")
}

func TestApi_WatchAccountConfig_ParamAccountAndLinear(t *testing.T) {
	bybitWatchAccountConfigRequireWsSubKey(t, map[string]interface{}{
		banexg.ParamAccount: ":first",
		banexg.ParamMarket:  banexg.MarketLinear,
	}, "position.linear")
}

func TestApi_WatchAccountConfig_ParamAccountAndInverse(t *testing.T) {
	bybitWatchAccountConfigRequireWsSubKey(t, map[string]interface{}{
		banexg.ParamAccount: ":first",
		banexg.ParamMarket:  banexg.MarketInverse,
	}, "position.inverse")
}

func TestApi_WatchAccountConfig_ParamAccountAndOption(t *testing.T) {
	bybitWatchAccountConfigRequireWsSubKey(t, map[string]interface{}{
		banexg.ParamAccount: ":first",
		banexg.ParamMarket:  banexg.MarketOption,
	}, "position.option")
}

// ============================================================================
// UnWatch helper functions
// ============================================================================

func bybitWaitChanClosed[T any](t *testing.T, ch <-chan T, timeout time.Duration) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline.C:
			t.Fatalf("timeout waiting channel closed (%s)", timeout)
		}
	}
}

func bybitWsSubKeyExists(client *banexg.WsClient, key string) bool {
	if client == nil || key == "" {
		return false
	}
	_, ok := client.SubscribeKeys[key]
	return ok
}

func bybitAssertWsSubKey(t *testing.T, client *banexg.WsClient, key string, want bool) {
	t.Helper()
	got := bybitWsSubKeyExists(client, key)
	if got != want {
		t.Fatalf("unexpected ws subscribe key %s: got=%v want=%v", key, got, want)
	}
}

func bybitTradeTopicKey(t *testing.T, exg *Bybit, symbol string) string {
	t.Helper()
	keys, err := bybitWsTradeTopics(exg, []string{symbol})
	if err != nil {
		t.Fatalf("bybitWsTradeTopics(%s) failed: %v", symbol, err)
	}
	if len(keys) != 1 {
		t.Fatalf("unexpected trade topic keys for %s: %v", symbol, keys)
	}
	return keys[0]
}

func bybitWaitWsTradeAnySymbol(t *testing.T, ch <-chan *banexg.Trade, wantSymbols []string, timeout time.Duration) *banexg.Trade {
	t.Helper()
	want := make(map[string]struct{}, len(wantSymbols))
	for _, s := range wantSymbols {
		if s != "" {
			want[s] = struct{}{}
		}
	}
	return bybitWaitWsTradeMatch(
		t,
		ch,
		timeout,
		func(tr *banexg.Trade) bool {
			if tr == nil {
				return false
			}
			_, ok := want[tr.Symbol]
			return ok
		},
		"timeout waiting ws trade for any subscribed symbol",
	)
}

func bybitKlineTopicKey(t *testing.T, exg *Bybit, symbol, timeframe string) string {
	t.Helper()
	if exg == nil {
		t.Fatal("exg is nil")
	}
	market, err := exg.GetMarket(symbol)
	if err != nil {
		t.Fatalf("GetMarket(%s) failed: %v", symbol, err)
	}
	interval := exg.GetTimeFrame(timeframe)
	if interval == "" {
		t.Fatalf("invalid timeframe: %s", timeframe)
	}
	return fmt.Sprintf("kline.%s.%s", interval, market.ID)
}

func bybitOrderBookTopicKey(t *testing.T, exg *Bybit, symbol string, depth int) string {
	t.Helper()
	if exg == nil {
		t.Fatal("exg is nil")
	}
	market, err := exg.GetMarket(symbol)
	if err != nil {
		t.Fatalf("GetMarket(%s) failed: %v", symbol, err)
	}
	return fmt.Sprintf("orderbook.%d.%s", depth, market.ID)
}

func bybitMarkPriceTopicKey(t *testing.T, exg *Bybit, symbol string) string {
	t.Helper()
	if exg == nil {
		t.Fatal("exg is nil")
	}
	market, err := exg.GetMarket(symbol)
	if err != nil {
		t.Fatalf("GetMarket(%s) failed: %v", symbol, err)
	}
	return fmt.Sprintf("tickers.%s", market.ID)
}

func bybitAssertOdBookDepthCleared(t *testing.T, client *banexg.WsClient, symbol string) {
	t.Helper()
	limits, lock := client.LockOdBookLimits()
	_, ok := limits[symbol]
	lock.Unlock()
	if ok {
		t.Fatalf("expected orderbook depth to be cleared for %s", symbol)
	}
}
