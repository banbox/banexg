package bybit

import (
	"testing"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
)

func newBybitWsTest(t *testing.T, marketID, symbol, marketType string) (*Bybit, *banexg.WsClient) {
	return newBybitWsTestWithClient(t, marketID, symbol, marketType, marketType)
}

func newBybitWsTestWithClient(t *testing.T, marketID, symbol, marketType, clientType string) (*Bybit, *banexg.WsClient) {
	t.Helper()
	conn := newTestAsyncConn()
	exg, err := New(map[string]interface{}{
		banexg.OptWsConn:    conn,
		banexg.OptApiKey:    "test-api-key",
		banexg.OptApiSecret: "test-api-secret",
	})
	if err != nil {
		t.Fatalf("new bybit failed: %v", err)
	}
	seedMarketIfNeeded(exg, marketID, symbol, marketType)
	if clientType == "" {
		clientType = marketType
	}
	client, err2 := exg.GetClient("wss://test", clientType, "")
	if err2 != nil {
		t.Fatalf("get ws client failed: %v", err2)
	}
	if exg.DefAccName != "" {
		client.AccName = exg.DefAccName
	}
	return exg, client
}

func TestHandleWsOrderBookSnapshotDelta(t *testing.T) {
	exg, client := newBybitWsTest(t, "BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	topic := "orderbook.50.BTCUSDT"
	client.SubscribeKeys[topic] = 0
	out := wsOutChan[*banexg.OrderBook](exg, client, "orderbook")

	snapshot := orderBookSnapshot{
		Symbol: "BTCUSDT",
		Bids:   [][]string{{"100", "1"}},
		Asks:   [][]string{{"101", "2"}},
		Ts:     1700000000000,
		Update: 1,
	}
	exg.handleWsOrderBook(client, &wsBaseMsg{Topic: topic, Type: "snapshot", Data: mustJSON(t, snapshot)})
	book := readChan(t, out)
	if book.Symbol != "BTC/USDT" || len(book.Bids.Price) != 1 || len(book.Asks.Price) != 1 {
		t.Fatalf("unexpected snapshot book: %+v", book)
	}
	if book.Bids.Size[0] != 1 || book.Asks.Size[0] != 2 {
		t.Fatalf("unexpected snapshot sizes: bids=%v asks=%v", book.Bids.Size, book.Asks.Size)
	}

	delta := orderBookSnapshot{
		Symbol: "BTCUSDT",
		Bids:   [][]string{{"100", "3"}},
		Asks:   [][]string{{"101", "4"}},
		Ts:     1700000001000,
		Update: 2,
	}
	exg.handleWsOrderBook(client, &wsBaseMsg{Topic: topic, Type: "delta", Data: mustJSON(t, delta)})
	book2 := readChan(t, out)
	if book2.Nonce != 2 || book2.TimeStamp != 1700000001000 {
		t.Fatalf("unexpected delta stamps: nonce=%d ts=%d", book2.Nonce, book2.TimeStamp)
	}
	if book2.Bids.Size[0] != 3 || book2.Asks.Size[0] != 4 {
		t.Fatalf("unexpected delta sizes: bids=%v asks=%v", book2.Bids.Size, book2.Asks.Size)
	}

	reset := orderBookSnapshot{
		Symbol: "BTCUSDT",
		Bids:   [][]string{{"102", "5"}},
		Asks:   [][]string{},
		Ts:     1700000002000,
		Update: 1,
	}
	exg.handleWsOrderBook(client, &wsBaseMsg{Topic: topic, Type: "delta", Data: mustJSON(t, reset)})
	book3 := readChan(t, out)
	if len(book3.Asks.Price) != 0 {
		t.Fatalf("expected snapshot reset to clear asks, got: %+v", book3.Asks)
	}
	if len(book3.Bids.Price) != 1 || book3.Bids.Price[0] != 102 {
		t.Fatalf("unexpected reset bids: %+v", book3.Bids)
	}
}

func TestHandleWsTrades(t *testing.T) {
	exg, client := newBybitWsTest(t, "BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	topic := "publicTrade.BTCUSDT"
	client.SubscribeKeys[topic] = 0
	out := wsOutChan[*banexg.Trade](exg, client, "trades")
	items := []map[string]interface{}{
		{"T": int64(1700000000000), "s": "BTCUSDT", "S": "Buy", "v": "0.1", "p": "100", "i": "t1"},
		{"T": int64(1700000001000), "s": "BTCUSDT", "S": "Sell", "v": "0.2", "p": "101", "i": "t2"},
	}
	exg.handleWsTrades(client, &wsBaseMsg{Topic: topic, Data: mustJSON(t, items)})
	trade := readChan(t, out)
	if trade.Symbol != "BTC/USDT" || trade.Price != 100 || trade.Amount != 0.1 {
		t.Fatalf("unexpected trade: %+v", trade)
	}
}

func TestHandleWsOHLCV(t *testing.T) {
	exg, client := newBybitWsTest(t, "BTCUSDT", "BTC/USDT", banexg.MarketSpot)
	topic := "kline.1.BTCUSDT"
	client.SubscribeKeys[topic] = 0
	out := wsOutChan[*banexg.PairTFKline](exg, client, "kline")
	items := []map[string]interface{}{{
		"start":  int64(1700000000000),
		"open":   "100",
		"high":   "110",
		"low":    "90",
		"close":  "105",
		"volume": "12.5",
	}}
	exg.handleWsOHLCV(client, &wsBaseMsg{Topic: topic, Data: mustJSON(t, items)})
	res := readChan(t, out)
	if res.Symbol != "BTC/USDT" || res.TimeFrame != "1m" {
		t.Fatalf("unexpected pair kline: %+v", res)
	}
	if res.Kline.Open != 100 || res.Kline.Close != 105 {
		t.Fatalf("unexpected kline data: %+v", res.Kline)
	}
}

func TestHandleWsTickers(t *testing.T) {
	exg, client := newBybitWsTest(t, "BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear)
	topic := "tickers.BTCUSDT"
	client.SubscribeKeys[topic] = 0
	out := wsOutChan[map[string]float64](exg, client, "markPrice")
	item := map[string]interface{}{
		"symbol":    "BTCUSDT",
		"markPrice": "123.45",
	}
	exg.handleWsTickers(client, &wsBaseMsg{Topic: topic, Data: mustJSON(t, item)})
	res := readChan(t, out)
	if res["BTC/USDT:USDT"] != 123.45 {
		t.Fatalf("unexpected markPrice output: %+v", res)
	}
	if exg.MarkPrices[client.MarketType]["BTC/USDT:USDT"] != 123.45 {
		t.Fatalf("markPrice cache not updated: %+v", exg.MarkPrices)
	}
}

func TestHandleWsWallet(t *testing.T) {
	exg, client := newBybitWsTestWithClient(t, "BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear, wsPrivate)
	client.SubscribeKeys["wallet"] = 0
	out := wsOutChan[*banexg.Balances](exg, client, "balance")
	items := []map[string]interface{}{
		{
			"accountType": "UNIFIED",
			"coin": []map[string]interface{}{
				{
					"coin":            "USDT",
					"walletBalance":   "10",
					"spotBorrow":      "1",
					"locked":          "2",
					"totalOrderIM":    "1",
					"totalPositionIM": "0",
					"equity":          "9",
				},
			},
		},
	}
	exg.handleWsWallet(client, &wsBaseMsg{Topic: "wallet", Data: mustJSON(t, items)})
	bal := readChan(t, out)
	asset := bal.Assets["USDT"]
	if asset == nil || asset.Total != 9 {
		t.Fatalf("unexpected balance: %+v", bal)
	}
	acc, err := exg.GetAccount(client.AccName)
	if err != nil || acc.MarBalances[client.MarketType] == nil {
		t.Fatalf("account balances not updated: %v", err)
	}
}

func TestHandleWsPositions(t *testing.T) {
	exg, client := newBybitWsTestWithClient(t, "BTCUSDT", "BTC/USDT:USDT", banexg.MarketLinear, wsPrivate)
	client.SubscribeKeys["position"] = 0
	out := wsOutChan[[]*banexg.Position](exg, client, "positions")
	items := []map[string]interface{}{
		{
			"category":      "linear",
			"symbol":        "BTCUSDT",
			"side":          "Buy",
			"size":          "1",
			"avgPrice":      "20000",
			"markPrice":     "19900",
			"positionValue": "20000",
			"leverage":      "5",
			"positionIM":    "100",
			"positionMM":    "10",
			"unrealisedPnl": "-5",
			"liqPrice":      "15000",
			"updatedTime":   "1700000000000",
			"positionIdx":   0,
			"tradeMode":     1,
		},
	}
	exg.handleWsPositions(client, &wsBaseMsg{Topic: "position", Data: mustJSON(t, items)})
	positions := readChan(t, out)
	if len(positions) != 1 || positions[0].Symbol != "BTC/USDT:USDT" {
		t.Fatalf("unexpected positions: %+v", positions)
	}
	acc, err := exg.GetAccount(client.AccName)
	if err != nil || len(acc.MarPositions[client.MarketType]) == 0 {
		t.Fatalf("account positions not updated: %v", err)
	}
}

func TestHandleWsExecutions(t *testing.T) {
	exg, client := newBybitWsTestWithClient(t, "BTCUSDT", "BTC/USDT", banexg.MarketSpot, wsPrivate)
	client.SubscribeKeys["execution"] = 0
	out := wsOutChan[*banexg.MyTrade](exg, client, "mytrades")
	items := []map[string]interface{}{
		{
			"category":    "spot",
			"symbol":      "BTCUSDT",
			"orderId":     "o1",
			"orderLinkId": "c1",
			"side":        "Buy",
			"orderType":   "Limit",
			"orderPrice":  "100",
			"orderQty":    "0.1",
			"leavesQty":   "0",
			"execId":      "e1",
			"execPrice":   "100",
			"execQty":     "0.1",
			"execValue":   "10",
			"execFee":     "0.01",
			"feeCurrency": "USDT",
			"execTime":    "1700000000000",
			"isMaker":     true,
		},
	}
	exg.handleWsExecutions(client, &wsBaseMsg{Topic: "execution", Data: mustJSON(t, items)})
	trade := readChan(t, out)
	if trade.Symbol != "BTC/USDT" || trade.Price != 100 || trade.Amount != 0.1 {
		t.Fatalf("unexpected execution trade: %+v", trade)
	}
}

func TestHandleWsOpAuthSuccess(t *testing.T) {
	exg, client := newBybitWsTestWithClient(t, "", "", banexg.MarketSpot, wsPrivate)
	done := make(chan *errs.Error, 1)
	exg.WsAuthDone[client.Key] = done
	ok := true
	exg.handleWsOp(client, &wsBaseMsg{Op: "auth", Success: &ok})
	err := readChan(t, done, "auth success")
	if err != nil {
		t.Fatalf("expected nil auth error, got %v", err)
	}
	if !exg.WsAuthed[client.Key] {
		t.Fatal("expected ws authed state to be true")
	}
}

func TestHandleWsOpAuthFail(t *testing.T) {
	exg, client := newBybitWsTestWithClient(t, "", "", banexg.MarketSpot, wsPrivate)
	done := make(chan *errs.Error, 1)
	exg.WsAuthDone[client.Key] = done
	ok := false
	exg.handleWsOp(client, &wsBaseMsg{Op: "auth", Success: &ok, RetMsg: "bad-auth"})
	err := readChan(t, done, "auth failure")
	if err == nil || err.Message() != "bad-auth" {
		t.Fatalf("expected bad-auth error, got %v", err)
	}
	if exg.WsAuthed[client.Key] {
		t.Fatal("expected ws authed state to be false")
	}
}
