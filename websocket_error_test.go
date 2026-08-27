package banexg

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/banbox/banexg/errs"
	"github.com/gorilla/websocket"
	"github.com/sasha-s/go-deadlock"
)

func TestCheckWsErrorDoesNotExposeNativePayload(t *testing.T) {
	err := CheckWsError(map[string]string{"error": `{"code":12345,"msg":"bad request"}`})
	if err == nil || err.Code != errs.CodeExchangeError || strings.Contains(err.Short(), "12345") {
		t.Fatalf("expected neutral websocket error, got %v", err)
	}
}

func TestWebSocketReconnectBackoff(t *testing.T) {
	attempts := 0
	waits := make([]time.Duration, 0, 5)
	ws := &WebSocket{
		lock: &deadlock.RWMutex{},
		dial: func() (*websocket.Conn, error) {
			attempts++
			if attempts < 6 {
				return nil, errors.New("offline")
			}
			return &websocket.Conn{}, nil
		},
		waitReconnect: func(wait time.Duration) bool {
			waits = append(waits, wait)
			return true
		},
	}
	if err := ws.connectWithRetry(false); err != nil {
		t.Fatal(err)
	}
	want := []time.Duration{3 * time.Second, 6 * time.Second, 12 * time.Second, 30 * time.Second, 30 * time.Second}
	if len(waits) != len(want) {
		t.Fatalf("waits = %v, want %v", waits, want)
	}
	for i := range want {
		if waits[i] != want[i] {
			t.Fatalf("waits[%d] = %s, want %s", i, waits[i], want[i])
		}
	}
}

func TestWebSocketInitialDialFailsFast(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		http.Error(w, "offline", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	if _, err := newWebSocket(1, wsURL, wsURL, nil, nil); err == nil {
		t.Fatal("initial dial unexpectedly succeeded")
	} else if !strings.Contains(err.Error(), "503 Service Unavailable") || !strings.Contains(err.Error(), wsURL) {
		t.Fatalf("initial dial error lost response context: %v", err)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("initial dial attempts = %d, want 1", got)
	}
}

func TestWebSocketCloseStopsReconnectWait(t *testing.T) {
	ws := &WebSocket{
		lock: &deadlock.RWMutex{},
		stop: make(chan struct{}),
		dial: func() (*websocket.Conn, error) {
			return nil, errors.New("offline")
		},
	}
	done := make(chan error, 1)
	go func() { done <- ws.connectWithRetry(false) }()
	time.Sleep(10 * time.Millisecond)
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("reconnect returned no error after close")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("Close did not cancel reconnect wait")
	}
}

func TestWebSocketCloseDuringDialDoesNotReviveConnection(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err == nil {
			defer conn.Close()
			<-r.Context().Done()
		}
	}))
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	started := make(chan struct{})
	release := make(chan struct{})
	ws := &WebSocket{
		lock: &deadlock.RWMutex{},
		stop: make(chan struct{}),
		dial: func() (*websocket.Conn, error) {
			close(started)
			<-release
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			return conn, err
		},
	}
	done := make(chan error, 1)
	go func() { done <- ws.connectWithRetry(false) }()
	<-started
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	close(release)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("dial succeeded after close")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("dial did not stop after close")
	}
	if ws.IsOK() {
		t.Fatal("closed websocket revived its connection")
	}
}

func TestPermanentWebSocketDialErrorsDoNotRetry(t *testing.T) {
	if !isPermanentWsDialError(errors.New("bad request"), &http.Response{StatusCode: http.StatusBadRequest}) {
		t.Fatal("HTTP 400 should not retry")
	}
	if isPermanentWsDialError(errors.New("connection refused"), nil) {
		t.Fatal("network failure should retry")
	}
}

func TestWebSocketReconnectsAfterDisconnect(t *testing.T) {
	var connections atomic.Int32
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		attempt := connections.Add(1)
		_ = conn.WriteMessage(websocket.TextMessage, []byte{byte('0' + attempt)})
		_ = conn.UnderlyingConn().Close()
	}))
	defer server.Close()

	var reconnects atomic.Int32
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, err := newWebSocket(1, wsURL, wsURL, nil, func() *errs.Error {
		reconnects.Add(1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ws := conn.WsConn.(*WebSocket)
	ws.waitReconnect = func(time.Duration) bool { return true }
	t.Cleanup(func() { _ = conn.Close() })

	first, readErr := conn.ReadMsg()
	if readErr != nil || string(first) != "1" {
		t.Fatalf("first read = %q, %v", first, readErr)
	}
	second, readErr := conn.ReadMsg()
	if readErr != nil || string(second) != "2" {
		t.Fatalf("second read = %q, %v", second, readErr)
	}
	if reconnects.Load() != 1 {
		t.Fatalf("reconnect hook calls = %d, want 1", reconnects.Load())
	}
}

func TestWsClientWriteDoesNotStopReconnect(t *testing.T) {
	waiting := make(chan struct{}, 1)
	release := make(chan struct{})
	ws := &WebSocket{
		lock: &deadlock.RWMutex{},
		stop: make(chan struct{}),
		dial: func() (*websocket.Conn, error) {
			return nil, errors.New("offline")
		},
		waitReconnect: func(time.Duration) bool {
			waiting <- struct{}{}
			<-release
			return false
		},
	}
	conn := &AsyncConn{WsConn: ws, send: make(chan *wsWrite, 2), control: make(chan int, 2)}
	e := &Exchange{WSClients: make(map[string]*WsClient)}
	client := &WsClient{Exg: e, Key: "test", LogURL: "ws://test", conns: map[int]*AsyncConn{0: conn}}
	e.WSClients[client.Key] = client

	go client.write(conn)
	reconnectDone := make(chan error, 1)
	go func() { reconnectDone <- ws.connectWithRetry(false) }()
	<-waiting

	if err := client.WriteRaw(conn, []byte("ping")); err == nil {
		t.Fatal("write during reconnect should report disconnected")
	}
	// A message accepted just before the disconnect may already be queued.
	conn.send <- &wsWrite{data: []byte("stale")}
	deadline := time.Now().Add(250 * time.Millisecond)
	for len(conn.send) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(10 * time.Millisecond)
	ws.lock.RLock()
	closed := ws.closed
	ws.lock.RUnlock()
	if closed {
		t.Fatal("transient write failure permanently closed websocket")
	}

	close(release)
	if err := <-reconnectDone; err == nil {
		t.Fatal("controlled reconnect stop returned no error")
	}
	conn.control <- ctrlClosed
	deadline = time.Now().Add(250 * time.Millisecond)
	for {
		if _, ok := e.findWSClient(client.Key); !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("websocket writer did not stop")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestDisconnectOldConnKeepsReplacement(t *testing.T) {
	oldConn := &websocket.Conn{}
	replacement := &websocket.Conn{}
	ws := &WebSocket{lock: &deadlock.RWMutex{}, conn: replacement, ready: true, generation: 2}
	if err := ws.disconnectIfCurrent(oldConn); err != nil {
		t.Fatal(err)
	}
	if err := ws.closeGeneration(1); err != nil {
		t.Fatal(err)
	}
	ws.lock.RLock()
	got, ready, closed := ws.conn, ws.ready, ws.closed
	ws.lock.RUnlock()
	if got != replacement || !ready || closed {
		t.Fatal("stale connection failure removed replacement")
	}
}

func TestWebSocketRecoveryReadiness(t *testing.T) {
	ws := &WebSocket{lock: &deadlock.RWMutex{}, conn: &websocket.Conn{}, generation: 1}
	conn := &AsyncConn{WsConn: ws, send: make(chan *wsWrite, 2)}
	client := &WsClient{}
	if err := client.WriteRaw(conn, []byte("business")); err == nil {
		t.Fatal("business write should wait for reconnect recovery")
	}
	if err := client.WriteRecovery(conn, map[string]string{"op": "login"}, nil); err != nil {
		t.Fatal(err)
	}
	queued := <-conn.send
	if !queued.recovery {
		t.Fatal("recovery write was not marked")
	}
	if queued.generation != 1 {
		t.Fatalf("recovery generation = %d, want 1", queued.generation)
	}
	generation := ws.deferReady(time.Hour)
	ws.finishReconnect()
	if ws.IsOK() {
		t.Fatal("deferred reconnect became ready before authentication")
	}
	ws.markReady(generation)
	if !ws.IsOK() {
		t.Fatal("authenticated reconnect did not become ready")
	}
}

func TestStaleRecoveryWriteDoesNotUseReplacement(t *testing.T) {
	oldConn := &websocket.Conn{}
	replacement := &websocket.Conn{}
	ws := &WebSocket{lock: &deadlock.RWMutex{}, conn: oldConn, generation: 1}
	conn := &AsyncConn{WsConn: ws, send: make(chan *wsWrite, 1)}
	client := &WsClient{}
	if err := client.WriteRecovery(conn, map[string]string{"op": "login"}, nil); err != nil {
		t.Fatal(err)
	}
	queued := <-conn.send
	ws.lock.Lock()
	ws.conn = replacement
	ws.generation = 2
	ws.lock.Unlock()

	if _, _, err := ws.nextWriter(true, queued.generation); err == nil {
		t.Fatal("stale recovery write used replacement connection")
	}
	ws.lock.RLock()
	got := ws.conn
	ws.lock.RUnlock()
	if got != replacement {
		t.Fatal("stale recovery write disconnected replacement")
	}
}

func TestWebSocketRecoveryTimeoutDisconnectsGeneration(t *testing.T) {
	upgrader := websocket.Upgrader{}
	serverConn := make(chan *websocket.Conn, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConn <- conn
		<-release
		_ = conn.Close()
	}))
	defer server.Close()
	defer close(release)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	<-serverConn
	ws := &WebSocket{lock: &deadlock.RWMutex{}, conn: clientConn, generation: 1}
	ws.deferReady(time.Millisecond)
	deadline := time.Now().Add(250 * time.Millisecond)
	for ws.isConnected() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if ws.isConnected() {
		t.Fatal("unready recovery generation remained connected after timeout")
	}
}

func TestWebSocketWriteFailureDisconnectsGeneration(t *testing.T) {
	upgrader := websocket.Upgrader{}
	serverConn := make(chan *websocket.Conn, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConn <- conn
		<-release
		_ = conn.Close()
	}))
	defer server.Close()
	defer close(release)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	<-serverConn
	ws := &WebSocket{lock: &deadlock.RWMutex{}, conn: clientConn, ready: true, generation: 1,
		stop: make(chan struct{})}
	conn := &AsyncConn{WsConn: ws, send: make(chan *wsWrite, 1), control: make(chan int, 1)}
	client := &WsClient{LogURL: wsURL, conns: map[int]*AsyncConn{0: conn}}
	go client.write(conn)
	if err := clientConn.UnderlyingConn().Close(); err != nil {
		t.Fatal(err)
	}
	if err := client.WriteRaw(conn, []byte("ping")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(250 * time.Millisecond)
	for ws.isConnected() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if ws.isConnected() {
		t.Fatal("failed write left poisoned websocket connected")
	}
	conn.control <- ctrlClosed
}

func TestPermanentWebSocketErrorClassification(t *testing.T) {
	if IsPermanentWsError(errs.NewMsg(errs.CodeRateLimit, "retry")) {
		t.Fatal("rate limit error should reconnect")
	}
	if !IsPermanentWsError(errs.NewMsg(errs.CodeUnauthorized, "bad credentials")) {
		t.Fatal("credential error should stop reconnecting")
	}
}

func TestWsCheckerStartsOnceAndStops(t *testing.T) {
	e := &Exchange{wsCheckStop: make(chan struct{})}
	var calls atomic.Int32
	done := make(chan struct{})
	e.CheckWsTimeout = func() {
		calls.Add(1)
		e.WaitWsCheck(time.Hour)
		close(done)
	}
	for range 10 {
		go e.startWsChecker()
	}
	deadline := time.Now().Add(250 * time.Millisecond)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	e.stopWsChecker()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("websocket checker did not stop")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("websocket checker calls = %d, want 1", got)
	}
}

func TestWSClientRegistryConditionalDelete(t *testing.T) {
	e := &Exchange{WSClients: make(map[string]*WsClient)}
	oldClient := &WsClient{Exg: e, Key: "same-key"}
	newClient := &WsClient{Exg: e, Key: "same-key"}
	e.WSClients[oldClient.Key] = newClient
	e.removeWSClient(oldClient)
	if got, ok := e.findWSClient(oldClient.Key); !ok || got != newClient {
		t.Fatal("old client removed its replacement")
	}
	e.removeWSClient(newClient)
	if _, ok := e.findWSClient(newClient.Key); ok {
		t.Fatal("current client remains registered")
	}
}

func TestWebSocketLogRedaction(t *testing.T) {
	secret := "issue9-super-secret-listen-key"
	if !isSecretKey("X-MBX-APIKEY") || !isSecretKey("Authorization") {
		t.Fatal("credential headers are not classified as secrets")
	}
	tests := []struct {
		name       string
		raw        string
		credential bool
		want       string
	}{
		{name: "path", raw: "wss://fstream.binance.com/private/ws/" + secret, credential: true},
		{name: "query", raw: "wss://fstream.binance.com/private/ws?listenKey=" + secret + "&events=ORDER_TRADE_UPDATE", credential: true},
		{name: "public", raw: "wss://fstream.binance.com/market/ws/btcusdt@aggTrade", want: "wss://fstream.binance.com/market/ws/btcusdt@aggTrade"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := safeWebSocketURL(test.raw, test.credential)
			if strings.Contains(got, secret) {
				t.Fatalf("secret leaked in %q", got)
			}
			if test.want != "" && got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
	msg := redactJSONSecrets([]byte(`{"listenToken":"` + secret + `","method":"userDataStream.subscribe"}`))
	if strings.Contains(string(msg), secret) {
		t.Fatalf("secret leaked in JSON: %s", msg)
	}
	form := redactRequestText("listenKey=" + secret + "&symbol=BTCUSDT")
	if strings.Contains(form, secret) {
		t.Fatalf("secret leaked in form: %s", form)
	}
	chanKey := safeWsChannelKey("acc@wss://fstream.binance.com/private/ws?listenKey=" + secret + "#mytrades")
	if strings.Contains(chanKey, secret) {
		t.Fatalf("secret leaked in channel key: %s", chanKey)
	}
}

func TestCheckWsErrorUsesExchangeMapper(t *testing.T) {
	err := CheckWsErrorWith(map[string]string{"error": `{"code":12345,"msg":"bad request"}`},
		func(_ int, _ string) *errs.Error {
			return errs.NewMsg(errs.CodeParamInvalid, "bad request")
		})
	if err == nil || err.Code != errs.CodeParamInvalid || err.BizCode != 0 {
		t.Fatalf("expected mapped websocket error, got %v", err)
	}
}
