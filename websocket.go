package banexg

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math/rand"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"github.com/banbox/bntp"
	"github.com/gorilla/websocket"
	"github.com/sasha-s/go-deadlock"
	"go.uber.org/zap"
)

var (
	maxClientConn = 20
	connMinSubs   = 50
)

type WsClient struct {
	Exg           *Exchange
	conns         map[int]*AsyncConn
	URL           string
	LogURL        string
	AccName       string
	MarketType    string
	Key           string
	Debug         bool
	JobInfos      map[string]*WsJobInfo // request id: Sub Data
	ChanCaps      map[string]int        // msgHash: cap size of cache msg
	SubscribeKeys map[string]int        // Subscription key, used to restore subscription after reconnection 订阅的key，用于重连后恢复订阅
	SubsKeyStamps map[string]int64      // 记录订阅key上次收到消息的时间戳，用于检测超时自动重新订阅
	subsKeyMap    map[string]string     // 通用key到SubsKeyStamps中key的转换
	odBookLimits  map[string]int        // Record the depth of each target subscription order book for easy cancellation 记录每个标的订阅订单簿的深度，方便取消
	OnMessage     func(client *WsClient, msg *WsMsg)
	OnError       func(client *WsClient, err *errs.Error)
	OnClose       func(client *WsClient, err *errs.Error)
	OnReConn      func(client *WsClient, connID int) *errs.Error
	NextConnId    int
	connArgs      map[string]interface{}
	connSubs      map[int]int
	connLock      deadlock.Mutex
	limitsLock    deadlock.Mutex // for odBookLimits
	subsLock      deadlock.Mutex // for SubsKeyStamps
}

type AsyncConn struct {
	WsConn
	send    chan *wsWrite
	control chan int // Used for internal synchronization control commands 用于内部同步控制命令
}

type wsWrite struct {
	data       []byte
	recovery   bool
	generation uint64
}

func (c *AsyncConn) isConnected() bool {
	if ws, ok := c.WsConn.(*WebSocket); ok {
		return ws.isConnected()
	}
	return c.IsOK()
}

type WebSocket struct {
	conn          *websocket.Conn // nil表示断开
	lock          *deadlock.RWMutex
	url           string
	logURL        string
	dialer        *websocket.Dialer
	onReConnect   func() *errs.Error
	id            int
	closed        bool
	ready         bool
	readyDeferred bool
	generation    uint64
	stop          chan struct{}
	reconnectLock deadlock.Mutex
	dial          func() (*websocket.Conn, error)
	waitReconnect func(time.Duration) bool
}

type permanentWsDialError struct{ error }

func (ws *WebSocket) Close() error {
	return ws.closeGeneration(0)
}

func (ws *WebSocket) closeGeneration(generation uint64) error {
	ws.lock.Lock()
	if generation != 0 && ws.generation != generation {
		ws.lock.Unlock()
		return nil
	}
	if !ws.closed {
		ws.closed = true
		if ws.stop != nil {
			close(ws.stop)
		}
	}
	conn := ws.conn
	ws.conn = nil
	ws.ready = false
	ws.readyDeferred = false
	ws.lock.Unlock()
	if conn != nil {
		return conn.Close()
	}
	return nil
}

func (ws *WebSocket) WriteClose() error {
	ws.lock.Lock()
	if !ws.closed {
		ws.closed = true
		if ws.stop != nil {
			close(ws.stop)
		}
	}
	conn := ws.conn
	ws.conn = nil
	ws.ready = false
	ws.readyDeferred = false
	ws.lock.Unlock()
	if conn != nil {
		exitData := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
		return conn.WriteMessage(websocket.CloseMessage, exitData)
	}
	return nil
}

func (ws *WebSocket) readConn() (*websocket.Conn, *deadlock.RWMutex) {
	ws.lock.RLock()
	return ws.conn, ws.lock
}

func (ws *WebSocket) reConnect() error {
	return ws.connectWithRetry(true)
}

func (ws *WebSocket) ReConnect() error {
	err := ws.disconnect()
	if err != nil {
		log.Warn("close ws conn fail", zap.String("url", ws.logURL), zap.Int("id", ws.id), zap.Error(err))
	}
	return ws.reConnect()
}

func (ws *WebSocket) disconnect() error {
	ws.lock.Lock()
	conn := ws.conn
	ws.conn = nil
	ws.ready = false
	ws.readyDeferred = false
	ws.lock.Unlock()
	if conn != nil {
		return conn.Close()
	}
	return nil
}

func (ws *WebSocket) disconnectIfCurrent(expected *websocket.Conn) error {
	return ws.disconnectGeneration(expected, 0)
}

func (ws *WebSocket) disconnectGeneration(expected *websocket.Conn, generation uint64) error {
	ws.lock.Lock()
	if ws.conn != expected || generation != 0 && ws.generation != generation {
		ws.lock.Unlock()
		return nil
	}
	ws.conn = nil
	ws.ready = false
	ws.readyDeferred = false
	ws.lock.Unlock()
	if expected != nil {
		return expected.Close()
	}
	return nil
}

func (ws *WebSocket) NextWriter() (io.WriteCloser, error) {
	writer, _, err := ws.nextWriter(false, 0)
	return writer, err
}

func (ws *WebSocket) nextWriter(recovery bool, generation uint64) (io.WriteCloser, *websocket.Conn, error) {
	conn, lock := ws.readConn()
	ready := ws.ready
	var writer io.WriteCloser
	var err error
	attempted := conn != nil && (ready || recovery) && (generation == 0 || ws.generation == generation)
	if attempted {
		writer, err = conn.NextWriter(websocket.TextMessage)
	} else {
		err = fmt.Errorf("ws conn [%d] %s closed, NextWriter fail", ws.id, ws.logURL)
	}
	lock.RUnlock()
	if err != nil && attempted {
		_ = ws.disconnectGeneration(conn, generation)
	}
	return writer, conn, err
}

func (ws *WebSocket) ReadMsg() ([]byte, error) {
	for {
		conn, lock := ws.readConn()
		closed := ws.closed
		lock.RUnlock()
		if closed {
			return nil, errors.New("ws conn closed, read fail")
		}
		if conn == nil {
			if err := ws.connectWithRetry(true); err != nil {
				return nil, err
			}
			continue
		}
		msgType, msgRaw, err := conn.ReadMessage()
		if err != nil {
			ws.lock.Lock()
			if ws.conn == conn {
				ws.conn = nil
				ws.ready = false
				ws.readyDeferred = false
			}
			closed = ws.closed
			ws.lock.Unlock()
			_ = conn.Close()
			if closed {
				return nil, err
			}
			log.Info("websocket disconnected, reconnecting", zap.String("url", ws.logURL),
				zap.Int("id", ws.id), zap.Error(err))
			if err = ws.connectWithRetry(true); err != nil {
				return nil, err
			}
			continue
		}
		if msgType == websocket.TextMessage {
			return msgRaw, nil
		}
	}
}

func (ws *WebSocket) IsOK() bool {
	conn, lock := ws.readConn()
	ok := conn != nil && ws.ready
	lock.RUnlock()
	return ok
}

func (ws *WebSocket) isConnected() bool {
	conn, lock := ws.readConn()
	ok := conn != nil
	lock.RUnlock()
	return ok
}

func (ws *WebSocket) initConn() error {
	conn, err := ws.dial()
	if err != nil {
		return err
	}
	if conn == nil {
		return errors.New("websocket dial returned no connection")
	}
	ws.lock.Lock()
	if ws.closed {
		ws.lock.Unlock()
		_ = conn.Close()
		return errors.New("websocket closed")
	}
	ws.conn = conn
	ws.ready = false
	ws.readyDeferred = false
	ws.generation++
	ws.lock.Unlock()
	return nil
}

const wsRecoveryReadyTimeout = 10 * time.Second

func (ws *WebSocket) deferReady(timeout time.Duration) uint64 {
	ws.lock.Lock()
	if ws.conn == nil || ws.closed {
		ws.lock.Unlock()
		return 0
	}
	ws.readyDeferred = true
	generation := ws.generation
	ws.lock.Unlock()
	time.AfterFunc(timeout, func() {
		if ws.disconnectIfUnready(generation) {
			log.Warn("websocket recovery timed out", zap.String("url", ws.logURL), zap.Int("id", ws.id))
		}
	})
	return generation
}

func (ws *WebSocket) disconnectIfUnready(generation uint64) bool {
	ws.lock.Lock()
	if ws.conn == nil || ws.closed || ws.ready || !ws.readyDeferred || ws.generation != generation {
		ws.lock.Unlock()
		return false
	}
	conn := ws.conn
	ws.conn = nil
	ws.readyDeferred = false
	ws.lock.Unlock()
	_ = conn.Close()
	return true
}

func (ws *WebSocket) markReady(generation uint64) bool {
	ws.lock.Lock()
	if ws.conn != nil && !ws.closed && (generation == 0 || ws.generation == generation) {
		ws.ready = true
		ws.readyDeferred = false
		ws.lock.Unlock()
		return true
	}
	ws.lock.Unlock()
	return false
}

func (ws *WebSocket) finishReconnect() {
	ws.lock.Lock()
	if ws.conn != nil && !ws.closed && !ws.readyDeferred {
		ws.ready = true
	}
	ws.lock.Unlock()
}

var reconnectDelays = [...]time.Duration{3 * time.Second, 6 * time.Second, 12 * time.Second, 30 * time.Second}

func (ws *WebSocket) connectWithRetry(callHook bool) error {
	ws.reconnectLock.Lock()
	defer ws.reconnectLock.Unlock()
	if callHook {
		conn, lock := ws.readConn()
		lock.RUnlock()
		if conn != nil {
			return nil
		}
	}
	for attempt := 0; ; attempt++ {
		ws.lock.RLock()
		closed := ws.closed
		ws.lock.RUnlock()
		if closed {
			return errors.New("websocket closed")
		}
		err := ws.initConn()
		if err == nil {
			conn, lock := ws.readConn()
			lock.RUnlock()
			if callHook && ws.onReConnect != nil {
				if hookErr := ws.onReConnect(); hookErr != nil {
					permanent := isPermanentWsError(hookErr)
					_ = ws.disconnectIfCurrent(conn)
					if permanent {
						return hookErr
					}
				} else {
					ws.finishReconnect()
					log.Info("reconnect success", zap.String("url", ws.logURL), zap.Int("id", ws.id))
					return nil
				}
			} else {
				ws.finishReconnect()
				return nil
			}
		} else {
			var permanent *permanentWsDialError
			if errors.As(err, &permanent) {
				return err
			}
		}
		wait := reconnectDelays[min(attempt, len(reconnectDelays)-1)]
		if ws.waitReconnect != nil {
			if !ws.waitReconnect(wait) {
				return errors.New("websocket reconnect stopped")
			}
			continue
		}
		// Spread reconnects from clients that were disconnected at the same time.
		jitter := time.Duration(rand.Int63n(int64(wait/5)+1)) - wait/10
		wait += jitter
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ws.stop:
			timer.Stop()
			return errors.New("websocket closed")
		}
	}
}

func (ws *WebSocket) GetID() int {
	return ws.id
}

func (ws *WebSocket) SetID(v int) {
	ws.id = v
}

func newWebSocket(id int, reqUrl, logURL string, args map[string]interface{}, onReConnect func() *errs.Error) (*AsyncConn, *errs.Error) {
	var dialer = &websocket.Dialer{}
	dialer.HandshakeTimeout = utils.GetMapVal(args, ParamHandshakeTimeout, time.Second*15)
	var defProxy func(*http.Request) (*url.URL, error)
	var proxy = utils.GetMapVal(args, ParamProxy, defProxy)
	if proxy != nil {
		dialer.Proxy = proxy
	}
	res := &WebSocket{id: id, dialer: dialer, url: reqUrl, logURL: logURL, onReConnect: onReConnect,
		stop: make(chan struct{})}
	res.lock = &deadlock.RWMutex{}
	res.dial = func() (*websocket.Conn, error) {
		conn, rsp, err := dialer.Dial(reqUrl, http.Header{})
		if err != nil {
			err = wrapWsDialError(logURL, err, rsp)
			if isPermanentWsDialError(err, rsp) {
				return nil, &permanentWsDialError{error: err}
			}
		}
		return conn, err
	}
	err := res.initConn()
	if err != nil {
		return nil, errs.New(errs.CodeConnectFail, err)
	}
	res.markReady(0)
	return &AsyncConn{
		WsConn:  res,
		send:    make(chan *wsWrite, 10),
		control: make(chan int, 2),
	}, nil
}

func wrapWsDialError(logURL string, err error, rsp *http.Response) error {
	if rsp == nil {
		return fmt.Errorf("websocket dial %s: %w", logURL, err)
	}
	var body string
	if rsp.Body != nil {
		content, readErr := io.ReadAll(io.LimitReader(rsp.Body, 512))
		_ = rsp.Body.Close()
		if readErr == nil {
			body = strings.TrimSpace(redactRequestText(string(content)))
		}
	}
	if body != "" {
		return fmt.Errorf("websocket dial %s failed with HTTP %s body=%q: %w", logURL, rsp.Status, body, err)
	}
	return fmt.Errorf("websocket dial %s failed with HTTP %s: %w", logURL, rsp.Status, err)
}

func isPermanentWsDialError(err error, rsp *http.Response) bool {
	if rsp != nil && rsp.StatusCode >= 400 && rsp.StatusCode < 500 && rsp.StatusCode != http.StatusTooManyRequests {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "malformed ws or wss url") || strings.Contains(text, "missing protocol scheme") ||
		strings.Contains(text, "unsupported protocol scheme")
}

func isPermanentWsError(err *errs.Error) bool {
	if err == nil {
		return false
	}
	switch err.Code {
	case errs.CodeUnauthorized, errs.CodeForbidden, errs.CodeAccKeyError, errs.CodeMissingApiKey,
		errs.CodeCredsRequired, errs.CodeSignFail, errs.CodeParamInvalid:
		return true
	default:
		return false
	}
}

var (
	ParamHandshakeTimeout = "HandshakeTimeout"
	ParamChanCaps         = "ChanCaps"
	ParamChanCap          = "ChanCap"
)

const (
	ctrlDoClose = iota
	ctrlClosed
)

var (
	DefChanCaps = map[string]int{
		"@depth": 1000,
	}
)

func newWsClient(exg *Exchange, reqUrl, marketType, acc string, onMsg FuncOnWsMsg, onErr FuncOnWsErr,
	onClose FuncOnWsClose, onReCon FuncOnWsReCon, params map[string]interface{}, debug bool) (*WsClient, *errs.Error) {
	args := utils.SafeParams(params)
	var result = &WsClient{
		Exg:           exg,
		AccName:       acc,
		URL:           reqUrl,
		LogURL:        safeWebSocketURL(reqUrl, acc != ""),
		MarketType:    marketType,
		Key:           acc + "@" + reqUrl,
		Debug:         debug,
		conns:         make(map[int]*AsyncConn),
		JobInfos:      make(map[string]*WsJobInfo),
		SubscribeKeys: make(map[string]int),
		SubsKeyStamps: make(map[string]int64),
		subsKeyMap:    make(map[string]string),
		odBookLimits:  make(map[string]int),
		OnMessage:     onMsg,
		OnError:       onErr,
		OnClose:       onClose,
		OnReConn:      onReCon,
		NextConnId:    1,
		connArgs:      args,
		connSubs:      make(map[int]int),
	}
	result.ChanCaps = DefChanCaps
	chanCaps := utils.GetMapVal(args, ParamChanCaps, map[string]int{})
	for k, v := range chanCaps {
		result.ChanCaps[k] = v
	}
	var conn *AsyncConn
	var err *errs.Error
	conn = utils.GetMapVal(args, OptWsConn, conn)
	if conn == nil {
		conn, err = result.newConn(false)
		if err != nil {
			return nil, err
		}
	}
	result.addConn(conn)
	return result, nil
}

func (e *Exchange) GetClient(wsUrl string, marketType, accName string) (*WsClient, *errs.Error) {
	clientKey := accName + "@" + wsUrl
	client, ok := e.findWSClient(clientKey)
	if ok {
		conns, lock := client.LockConns()
		connNum := len(conns)
		lock.Unlock()
		if connNum > 0 {
			return client, nil
		}
	}
	params := map[string]interface{}{}
	if e.Proxy != nil {
		params[ParamProxy] = e.Proxy
	}
	if conn, ok := e.Options[OptWsConn]; ok {
		params[OptWsConn] = conn
	}
	if e.OnWsMsg == nil {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "OnWsMsg is required for ws client")
	}
	onClosed := func(client *WsClient, err *errs.Error) {
		if e.OnWsClose != nil {
			e.OnWsClose(client, err)
		}
		num := e.handleWsClientClosed(client)
		log.Info("closed out chan for ws client", zap.Int("num", num))
	}
	client, err := newWsClient(e, wsUrl, marketType, accName, e.OnWsMsg, e.OnWsErr, onClosed,
		e.OnWsReCon, params, e.DebugWS)
	if err != nil {
		return nil, err
	}
	e.lockWSClient.Lock()
	if current := e.WSClients[clientKey]; current != nil {
		conns, lock := current.LockConns()
		usable := len(conns) > 0
		lock.Unlock()
		if usable {
			e.lockWSClient.Unlock()
			client.Close()
			return current, nil
		}
	}
	e.WSClients[clientKey] = client
	e.lockWSClient.Unlock()
	e.startWsChecker()
	return client, nil
}

func (e *Exchange) startWsChecker() {
	if e.CheckWsTimeout == nil {
		return
	}
	e.wsCheckOnce.Do(func() {
		e.WsChecking = true
		go func() {
			defer func() { e.WsChecking = false }()
			e.CheckWsTimeout()
		}()
	})
}

func (e *Exchange) stopWsChecker() {
	if e.wsCheckStop != nil {
		e.wsStopOnce.Do(func() { close(e.wsCheckStop) })
	}
}

func (e *Exchange) WaitWsCheck(delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-e.wsCheckStop:
		return false
	}
}

func (e *Exchange) findWSClient(key string) (*WsClient, bool) {
	e.lockWSClient.RLock()
	client, ok := e.WSClients[key]
	e.lockWSClient.RUnlock()
	return client, ok
}

func (e *Exchange) FindWSClient(key string) (*WsClient, bool) {
	return e.findWSClient(key)
}

func (e *Exchange) removeWSClient(client *WsClient) {
	if client == nil {
		return
	}
	e.lockWSClient.Lock()
	if e.WSClients[client.Key] == client {
		delete(e.WSClients, client.Key)
	}
	e.lockWSClient.Unlock()
}

func (e *Exchange) WSClientSnapshot() []*WsClient {
	e.lockWSClient.RLock()
	clients := utils.ValsOfMap(e.WSClients)
	e.lockWSClient.RUnlock()
	return clients
}

func (e *Exchange) drainWSClients() []*WsClient {
	e.lockWSClient.Lock()
	clients := utils.ValsOfMap(e.WSClients)
	e.WSClients = make(map[string]*WsClient)
	e.lockWSClient.Unlock()
	return clients
}

/*
GetWsOutChan
获取指定msgHash的输出通道
如果不存在则创建新的并存储
*/
func GetWsOutChan[T any](e *Exchange, chanKey string, create func(int) T, args map[string]interface{}) T {
	e.lockOutChan.Lock()
	outRaw, oldChan := e.WsOutChans[chanKey]
	e.lockOutChan.Unlock()
	if oldChan {
		res := outRaw.(T)
		return res
	} else {
		chanCap := utils.PopMapVal(args, ParamChanCap, 100)
		res := create(chanCap)
		e.lockOutChan.Lock()
		e.WsOutChans[chanKey] = res
		e.lockOutChan.Unlock()
		if e.OnWsChan != nil {
			e.OnWsChan(chanKey, res)
		}
		return res
	}
}

func WriteOutChan[T any](e *Exchange, chanKey string, msg T, popIfNeed bool) bool {
	// Keep lock held during send so DelWsChanRefs can't close the channel concurrently.
	// Otherwise, we can panic with "send on closed channel" under unsubscribe races.
	e.lockOutChan.Lock()
	outRaw, outOk := e.WsOutChans[chanKey]
	if !outOk {
		e.lockOutChan.Unlock()
		return false
	}
	out, ok := outRaw.(chan T)
	if !ok {
		e.lockOutChan.Unlock()
		log.Error("out chan type error", zap.String("k", safeWsChannelKey(chanKey)))
		return false
	}
	select {
	case out <- msg:
		e.lockOutChan.Unlock()
		return true
	default:
		if !popIfNeed {
			e.lockOutChan.Unlock()
			log.Error("out chan full", zap.String("k", safeWsChannelKey(chanKey)))
			return false
		}
		// chan通道满了，弹出最早的消息，重新发送
		<-out
		out <- msg
		e.lockOutChan.Unlock()
		return true
	}
}

func (e *Exchange) AddWsChanRefs(chanKey string, keys ...string) {
	e.lockWsRef.Lock()
	data, ok := e.WsChanRefs[chanKey]
	if !ok {
		data = make(map[string]struct{})
		e.WsChanRefs[chanKey] = data
	}
	e.lockWsRef.Unlock()
	for _, k := range keys {
		data[k] = struct{}{}
	}
}

func (e *Exchange) DelWsChanRefs(chanKey string, keys ...string) int {
	e.lockWsRef.Lock()
	data, ok := e.WsChanRefs[chanKey]
	e.lockWsRef.Unlock()
	if !ok {
		return -1
	}
	for _, k := range keys {
		delete(data, k)
	}
	hasNum := len(data)
	if hasNum == 0 {
		e.lockOutChan.Lock()
		if out, ok := e.WsOutChans[chanKey]; ok {
			val := reflect.ValueOf(out)
			if val.Kind() == reflect.Chan {
				val.Close()
			}
			delete(e.WsOutChans, chanKey)
			log.Info("remove chan", zap.String("key", safeWsChannelKey(chanKey)))
		}
		e.lockOutChan.Unlock()
	}
	return hasNum
}

func (e *Exchange) handleWsClientClosed(client *WsClient) int {
	e.removeWSClient(client)
	prefix := client.Prefix("")
	removeNum := 0
	e.lockWsRef.Lock()
	refKeys := utils.KeysOfMap(e.WsChanRefs)
	e.lockWsRef.Unlock()
	for _, key := range refKeys {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		e.lockWsRef.Lock()
		delete(e.WsChanRefs, key)
		e.lockWsRef.Unlock()
		e.lockOutChan.Lock()
		if out, ok := e.WsOutChans[key]; ok {
			val := reflect.ValueOf(out)
			if val.Kind() == reflect.Chan {
				val.Close()
			}
			delete(e.WsOutChans, key)
			removeNum += 1
		}
		e.lockOutChan.Unlock()
	}
	return removeNum
}

/*
CheckWsError
从websocket返回的消息结果中，检查是否有错误信息
*/
func CheckWsError(msg map[string]string) *errs.Error {
	return CheckWsErrorWith(msg, nil)
}

func CheckWsErrorWith(msg map[string]string, mapper func(status int, content string) *errs.Error) *errs.Error {
	errRaw, ok := msg["error"]
	if ok {
		if mapper != nil {
			if err := mapper(http.StatusBadRequest, errRaw); err != nil {
				return err
			}
		}
		return errs.NewMsg(errs.CodeExchangeError, "websocket request failed")
	}
	status, ok := msg["status"]
	if ok && status != "200" {
		statusVal, e := strconv.Atoi(status)
		if e != nil {
			return nil
		}
		if mapper != nil {
			msgStr, _ := utils.MarshalString(msg)
			if err := mapper(statusVal, msgStr); err != nil {
				return err
			}
		}
		return mapHTTPError(nil, statusVal)
	}
	return nil
}

type SubStat struct {
	ConnId   int
	Timeouts map[string]int64 // 超时的key，及其毫秒数
	Stamps   map[string]int64 // 所有key上次收到消息的时间戳
}

func (c *WsClient) GetConnSubStats(timeout int64) map[int]*SubStat {
	curMS := bntp.UTCStamp()
	c.subsLock.Lock()
	var result = make(map[int]*SubStat)
	for k, cid := range c.SubscribeKeys {
		stamp, _ := c.SubsKeyStamps[k]
		stat, ok := result[cid]
		if !ok {
			stat = &SubStat{
				ConnId:   cid,
				Timeouts: make(map[string]int64),
				Stamps:   make(map[string]int64),
			}
			result[cid] = stat
		}
		stat.Stamps[k] = stamp
		if stamp > 0 && curMS-stamp > timeout {
			stat.Timeouts[k] = curMS - stamp
		}
	}
	c.subsLock.Unlock()
	return result
}

func (c *WsClient) SetSubsKeyStamp(key string, stamp int64) {
	c.subsLock.Lock()
	if target, ok := c.subsKeyMap[key]; ok {
		c.SubsKeyStamps[target] = stamp
	} else if _, ok := c.SubscribeKeys[key]; ok {
		c.SubsKeyStamps[key] = stamp
		c.subsKeyMap[key] = key
	} else {
		match := false
		for k := range c.SubscribeKeys {
			if strings.HasPrefix(key, k) {
				c.SubsKeyStamps[k] = stamp
				c.subsKeyMap[key] = k
				match = true
				break
			}
		}
		if !match {
			// 未匹配，使用@切分，分别匹配头部和中间特征；针对期权markPrice
			arr := strings.Split(key, "@")
			prefix := arr[0]
			fea := "@" + strings.Join(arr[1:], "@")
			for k := range c.SubscribeKeys {
				if strings.HasPrefix(k, prefix) && strings.Contains(k, fea) {
					c.SubsKeyStamps[k] = stamp
					c.subsKeyMap[key] = k
					match = true
					break
				}
			}
			if !match {
				log.Warn("SetSubsKeyStamp not match", zap.String("k", key),
					zap.Strings("has", utils.KeysOfMap(c.SubsKeyStamps)))
			}
		}
	}
	c.subsLock.Unlock()
}

/*
Write
send a message to the WS server to set the information required for processing task results
发送消息到ws服务器，可设置处理任务结果需要的信息
jobID: The task ID of this message uniquely identifies this request 此次消息的任务ID，唯一标识此次请求
jobInfo: The main information of this task will be used when receiving the task results 此次任务的主要信息，在收到任务结果时使用
*/
func (c *WsClient) Write(conn *AsyncConn, msg interface{}, info *WsJobInfo) *errs.Error {
	return c.queueWrite(conn, msg, info, false, 0)
}

func (c *WsClient) WriteRecovery(conn *AsyncConn, msg interface{}, info *WsJobInfo) *errs.Error {
	return c.queueWrite(conn, msg, info, true, 0)
}

func (c *WsClient) WriteRecoveryFor(conn *AsyncConn, generation uint64, msg interface{}, info *WsJobInfo) *errs.Error {
	return c.queueWrite(conn, msg, info, true, generation)
}

func (c *WsClient) queueWrite(conn *AsyncConn, msg interface{}, info *WsJobInfo, recovery bool,
	generation uint64) *errs.Error {
	if conn == nil || c.Exg != nil && c.Exg.WsDecoder != nil {
		// skip write ws msg in replay mode
		return nil
	}
	if ws, ok := conn.WsConn.(*WebSocket); ok {
		ws.lock.RLock()
		if generation == 0 {
			generation = ws.generation
		}
		available := ws.conn != nil && ws.generation == generation && (ws.ready || recovery)
		ws.lock.RUnlock()
		if !available {
			return errs.NewMsg(errs.CodeConnectFail, "websocket disconnected")
		}
	} else if !conn.IsOK() {
		return errs.NewMsg(errs.CodeConnectFail, "websocket disconnected")
	}
	data, err2 := utils.Marshal(msg)
	if err2 != nil {
		return errs.New(errs.CodeUnmarshalFail, err2)
	}
	if info != nil {
		if info.ID == "" {
			return errs.NewMsg(errs.CodeParamRequired, "WsJobInfo.ID is required")
		}
		if _, ok := c.JobInfos[info.ID]; !ok {
			c.JobInfos[info.ID] = info
		}
	}
	if c.Debug {
		log.Debug("write ws msg", zap.String("url", c.LogURL), zap.Int("id", conn.GetID()),
			zap.ByteString("msg", redactJSONSecrets(data)))
	}
	conn.send <- &wsWrite{data: data, recovery: recovery, generation: generation}
	return nil
}

// WriteRaw sends raw bytes without JSON marshaling (e.g., for OKX ping/pong)
func (c *WsClient) WriteRaw(conn *AsyncConn, data []byte) *errs.Error {
	if conn == nil || c.Exg != nil && c.Exg.WsDecoder != nil {
		return nil
	}
	generation := uint64(0)
	if ws, ok := conn.WsConn.(*WebSocket); ok {
		ws.lock.RLock()
		generation = ws.generation
		available := ws.conn != nil && ws.ready
		ws.lock.RUnlock()
		if !available {
			return errs.NewMsg(errs.CodeConnectFail, "websocket disconnected")
		}
	} else if !conn.IsOK() {
		return errs.NewMsg(errs.CodeConnectFail, "websocket disconnected")
	}
	if c.Debug {
		log.Debug("write ws raw", zap.String("url", c.LogURL), zap.Int("id", conn.GetID()),
			zap.ByteString("msg", redactJSONSecrets(data)))
	}
	conn.send <- &wsWrite{data: data, generation: generation}
	return nil
}

func (c *WsClient) connWebSocket(connID int) *WebSocket {
	c.connLock.Lock()
	conn := c.conns[connID]
	c.connLock.Unlock()
	if conn == nil {
		return nil
	}
	ws, _ := conn.WsConn.(*WebSocket)
	return ws
}

func (c *WsClient) DeferConnReady(connID int) uint64 {
	if ws := c.connWebSocket(connID); ws != nil {
		return ws.deferReady(wsRecoveryReadyTimeout)
	}
	return 0
}

func (c *WsClient) MarkConnReady(connID int, generation uint64) bool {
	if ws := c.connWebSocket(connID); ws != nil {
		return ws.markReady(generation)
	}
	return false
}

func (c *WsClient) FailConn(connID int, generation uint64, permanent bool) {
	if ws := c.connWebSocket(connID); ws != nil {
		if permanent {
			_ = ws.closeGeneration(generation)
		} else {
			conn, lock := ws.readConn()
			lock.RUnlock()
			_ = ws.disconnectGeneration(conn, generation)
		}
	}
}

func IsPermanentWsError(err *errs.Error) bool {
	return isPermanentWsError(err)
}

func (c *WsClient) Close() {
	c.connLock.Lock()
	conns := utils.ValsOfMap(c.conns)
	c.connLock.Unlock()
	for _, conn := range conns {
		if conn.control != nil {
			conn.control <- ctrlDoClose
		}
	}
}

func (c *WsClient) write(conn *AsyncConn) {
	zapFields := []zap.Field{zap.String("url", c.LogURL), zap.Int("id", conn.GetID())}
	defer func() {
		log.Debug("stop write ws", zapFields...)
		err := conn.Close()
		if err != nil {
			log.Error("close ws error", append(zapFields, zap.Error(err))...)
		}
		close(conn.control)
		conn.control = nil
		c.connLock.Lock()
		delete(c.conns, conn.GetID())
		lastConn := len(c.conns) == 0
		c.connLock.Unlock()
		if lastConn {
			if c.Exg != nil {
				c.Exg.removeWSClient(c)
			}
		}
	}()
	for {
		select {
		case ctrlType, ok := <-conn.control:
			if !ok {
				log.Error("read control fail", zap.Int("flag", ctrlType))
				continue
			}
			if ctrlType == ctrlClosed {
				return
			} else if ctrlType == ctrlDoClose {
				// Cleanly close the connection by sending a close message and then
				// waiting (with timeout) for the server to close the connection.
				err := conn.WriteClose()
				if err != nil {
					log.Error("write ws close error", append(zapFields, zap.Error(err))...)
					return
				}
			} else {
				log.Error("invalid ws control type", append(zapFields, zap.Int("val", ctrlType))...)
			}
		case msg, ok := <-conn.send:
			if !ok {
				err := conn.WriteClose()
				if err != nil {
					log.Error("write ws close error", append(zapFields, zap.Error(err))...)
					return
				}
				log.Info("WsClient.Send closed", zapFields...)
				return
			}
			var w io.WriteCloser
			var err error
			var target *websocket.Conn
			var ws *WebSocket
			if typed, ok := conn.WsConn.(*WebSocket); ok {
				ws = typed
				w, target, err = ws.nextWriter(msg.recovery, msg.generation)
			} else {
				w, err = conn.NextWriter()
			}
			if err != nil {
				log.Error("failed to create Ws.Writer", append(zapFields, zap.Error(err))...)
				continue
			}
			// 一次只能写入一条消息
			writeErr := error(nil)
			if _, writeErr = w.Write(msg.data); writeErr != nil {
				log.Error("write ws fail", append(zapFields, zap.Error(writeErr))...)
			}
			closeErr := w.Close()
			if closeErr != nil {
				log.Error("close WriteCloser fail", append(zapFields, zap.Error(closeErr))...)
			}
			if ws != nil && (writeErr != nil || closeErr != nil) {
				_ = ws.disconnectGeneration(target, msg.generation)
			}
		}
	}
}

func (c *WsClient) read(conn *AsyncConn) {
	defer func() {
		if conn.control != nil {
			conn.control <- ctrlClosed
		}
	}()
	for {
		msgRaw, err := conn.ReadMsg()
		if err != nil {
			if !conn.IsOK() {
				log.Error("read fail, ws closed", zap.String("url", c.LogURL), zap.Int("id", conn.GetID()), zap.Error(err))
				if c.OnClose != nil {
					c.OnClose(c, errs.New(errs.CodeWsReadFail, err))
				}
				return
			} else {
				log.Error("read error", zap.String("url", c.LogURL), zap.Int("id", conn.GetID()), zap.Error(err))
				if c.OnError != nil {
					c.OnError(c, errs.New(errs.CodeWsReadFail, err))
				}
				continue
			}
		}
		// skip ws msg in replay mode
		if c.Exg.WsDecoder == nil {
			// We cannot start a goroutine for each message here, otherwise it will result in incorrect message processing order
			// 这里不能对每个消息启动一个goroutine，否则会导致消息处理顺序错误
			c.Exg.DumpWS("wsMsg", []string{c.LogURL, c.MarketType, c.AccName, string(redactJSONSecrets(msgRaw))})
			c.HandleRawMsg(msgRaw)
		}
	}
}

func (c *WsClient) HandleRawMsg(msgRaw []byte) {
	msgText := string(msgRaw)
	if c.Debug {
		log.Debug("receive ws msg", zap.String("url", c.LogURL), zap.ByteString("msg", redactJSONSecrets(msgRaw)))
	}
	if msgText == "pong" {
		return
	}
	// fmt.Printf("receive %s\n", msgText)
	msg, err := NewWsMsg(msgText)
	if err != nil {
		if c.OnError != nil {
			c.OnError(c, err)
		}
		log.Error("invalid ws msg", zap.ByteString("msg", redactJSONSecrets(msgRaw)), zap.Error(err))
		return
	}
	if !msg.IsArray && msg.ID != "" {
		if sub, ok := c.JobInfos[msg.ID]; ok && sub.Method != nil {
			// 订阅信息中提供了处理函数，则调用处理函数
			sub.Method(c, msg.Object, sub)
			delete(c.JobInfos, msg.ID)
			return
		}
	}
	// 未匹配则调用通用消息处理
	c.OnMessage(c, msg)
}

func (c *WsClient) Prefix(key string) string {
	var arr = []string{c.AccName, "@", c.URL, "#", key}
	return strings.Join(arr, "")
}

func (c *WsClient) UpdateSubs(connID int, isSub bool, keys []string) (string, *AsyncConn) {
	method := "SUBSCRIBE"
	var conn *AsyncConn
	if !isSub {
		method = "UNSUBSCRIBE"
		c.subsLock.Lock()
		for _, key := range keys {
			if cid, ok := c.SubscribeKeys[key]; ok {
				c.decrementConnSub(cid)
				delete(c.SubscribeKeys, key)
				delete(c.SubsKeyStamps, key)
			}
		}
		c.subsLock.Unlock()
	} else {
		connMap, lock := c.LockConns()
		conn, _ = connMap[connID]
		connNum := len(connMap)
		// Check if there are any existing connections that have not reached the minimum number of subscriptions
		// 检查已有连接，是否有未达到最低订阅数的
		var connDups map[int]*AsyncConn
		if conn == nil {
			connDups = maps.Clone(connMap)
		}
		lock.Unlock()
		reconnecting := false
		if len(connDups) > 0 {
			// IsOk would require new lock, we should release LockConns first
			for cid, con := range connDups {
				num, _ := c.connSubs[cid]
				if num < connMinSubs && con.IsOK() {
					conn = con
					break
				}
				if con.isConnected() {
					reconnecting = true
				}
			}
		}
		if conn == nil && reconnecting {
			return method, nil
		}
		// Attempt to create a new connection
		// 尝试创建新连接
		if conn == nil && connNum < maxClientConn {
			var err *errs.Error
			conn, err = c.newConn(true)
			if err != nil {
				log.Warn("make new websocket fail", zap.String("url", c.LogURL), zap.String("err", err.Short()))
			}
		}
		// Randomly select one from existing connections
		// 从已有连接随机挑一个
		if conn == nil {
			if connNum == 0 {
				return method, nil
			}
			lock.Lock()
			cids := utils.KeysOfMap(connMap)
			conn = connMap[cids[rand.Intn(len(cids))]]
			lock.Unlock()
		}
		connID = conn.GetID()
		curMS := bntp.UTCStamp()
		c.subsLock.Lock()
		for _, key := range keys {
			oldId, ok := c.SubscribeKeys[key]
			if oldId != connID {
				if ok {
					c.decrementConnSub(oldId)
				}
				num, _ := c.connSubs[connID]
				c.connSubs[connID] = num + 1
			}
			c.SubscribeKeys[key] = connID
			c.SubsKeyStamps[key] = curMS
		}
		c.subsLock.Unlock()
	}
	return method, conn
}

func (c *WsClient) decrementConnSub(cid int) {
	num, _ := c.connSubs[cid]
	if num <= 1 {
		delete(c.connSubs, cid)
	} else {
		c.connSubs[cid] = num - 1
	}
}

func (c *WsClient) GetSubKeys(connID int) []string {
	var keys = make([]string, 0, 16)
	c.subsLock.Lock()
	for key, id := range c.SubscribeKeys {
		if id == connID {
			keys = append(keys, key)
		}
	}
	c.subsLock.Unlock()
	return keys
}

// HasSubKeyPrefix checks if any subscription key starts with the given prefix.
func (c *WsClient) HasSubKeyPrefix(prefix string) bool {
	c.subsLock.Lock()
	defer c.subsLock.Unlock()
	for key := range c.SubscribeKeys {
		if key == prefix || strings.HasPrefix(key, prefix+":") {
			return true
		}
	}
	return false
}

func (c *WsClient) newConn(add bool) (*AsyncConn, *errs.Error) {
	connID := c.NextConnId
	conn, err := newWebSocket(connID, c.URL, c.LogURL, c.connArgs, func() *errs.Error {
		return c.OnReConn(c, connID)
	})
	if err != nil {
		return nil, err
	}
	log.Debug("new websocket conn", zap.String("url", c.LogURL), zap.Int("id", conn.GetID()))
	c.NextConnId += 1
	if add {
		c.addConn(conn)
	}
	return conn, nil
}

func (c *WsClient) addConn(conn *AsyncConn) {
	connID := conn.GetID()
	c.connLock.Lock()
	if _, has := c.conns[connID]; has {
		conn.SetID(c.NextConnId)
		c.NextConnId += 1
		connID = conn.GetID()
	}
	c.conns[connID] = conn
	c.connLock.Unlock()
	go c.read(conn)
	go c.write(conn)
}

func (c *WsClient) LockConns() (map[int]*AsyncConn, *deadlock.Mutex) {
	c.connLock.Lock()
	return c.conns, &c.connLock
}

func (c *WsClient) LockOdBookLimits() (map[string]int, *deadlock.Mutex) {
	c.limitsLock.Lock()
	return c.odBookLimits, &c.limitsLock
}

func safeWebSocketURL(raw string, credential bool) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "[redacted websocket URL]"
	}
	query := parsed.Query()
	for key := range query {
		if isSecretKey(key) {
			query.Set(key, "[redacted]")
		}
	}
	parsed.RawQuery = query.Encode()
	if credential {
		const marker = "/ws/"
		if idx := strings.LastIndex(parsed.Path, marker); idx >= 0 && idx+len(marker) < len(parsed.Path) {
			parsed.Path = parsed.Path[:idx+len(marker)] + "[redacted]"
			parsed.RawPath = ""
		}
	}
	return parsed.String()
}

func safeWsChannelKey(key string) string {
	at := strings.IndexByte(key, '@')
	hash := strings.LastIndexByte(key, '#')
	if at < 0 || hash <= at {
		return key
	}
	return key[:at+1] + safeWebSocketURL(key[at+1:hash], at > 0) + key[hash:]
}

func isSecretKey(key string) bool {
	normalized := strings.NewReplacer("-", "", "_", "").Replace(strings.ToLower(key))
	switch normalized {
	case "listenkey", "listentoken", "apikey", "secret", "signature", "sign", "authorization":
		return true
	default:
		return strings.Contains(normalized, "apikey") || strings.Contains(normalized, "accesskey")
	}
}

func redactJSONSecrets(raw []byte) []byte {
	var value interface{}
	if json.Unmarshal(raw, &value) != nil {
		return raw
	}
	redactSecretValue(value)
	redacted, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return redacted
}

func redactRequestText(raw string) string {
	if raw == "" {
		return raw
	}
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return string(redactJSONSecrets([]byte(raw)))
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return raw
	}
	changed := false
	for key := range values {
		if isSecretKey(key) {
			values.Set(key, "[redacted]")
			changed = true
		}
	}
	if !changed {
		return raw
	}
	return values.Encode()
}

func RedactLogText(raw string) string {
	return redactRequestText(raw)
}

func redactSecretValue(value interface{}) {
	switch item := value.(type) {
	case map[string]interface{}:
		for key, child := range item {
			if isSecretKey(key) {
				item[key] = "[redacted]"
			} else {
				redactSecretValue(child)
			}
		}
	case []interface{}:
		for _, child := range item {
			redactSecretValue(child)
		}
	}
}

func NewWsMsg(msgText string) (*WsMsg, *errs.Error) {
	var err_ error
	if strings.HasPrefix(msgText, "{") {
		var msg = make(map[string]interface{})
		err_ = utils.UnmarshalString(msgText, &msg, utils.JsonNumStr)
		if err_ == nil {
			var obj = utils.MapValStr(msg)
			event, _ := utils.SafeMapVal(obj, "e", "")
			id, _ := utils.SafeMapVal(obj, "id", "")
			return &WsMsg{Event: event, ID: id, Object: obj, Text: msgText}, nil
		}
	} else if strings.HasPrefix(msgText, "[") {
		var msgs = make([]map[string]interface{}, 0)
		err_ = utils.UnmarshalString(msgText, &msgs, utils.JsonNumStr)
		if err_ == nil && len(msgs) > 0 {
			var event string
			var itemList = make([]map[string]string, len(msgs))
			for i, it := range msgs {
				var obj = utils.MapValStr(it)
				if i == 0 {
					event, _ = utils.SafeMapVal(obj, "e", "")
				}
				itemList[i] = obj
			}
			return &WsMsg{Event: event, IsArray: true, List: itemList, Text: msgText}, nil
		}
	} else {
		return nil, errs.NewMsg(errs.CodeWsInvalidMsg, "invalid ws msg, not dict or list")
	}
	return nil, errs.New(errs.CodeUnmarshalFail, err_)
}
