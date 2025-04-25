package banexg

import (
	"errors"
	"fmt"
	"github.com/banbox/banexg/bntp"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"github.com/gorilla/websocket"
	"github.com/sasha-s/go-deadlock"
	"go.uber.org/zap"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
)

var (
	maxClientConn = 20
	connMinSubs   = 50
)

type WsClient struct {
	Exg           *Exchange
	conns         map[int]*AsyncConn
	URL           string
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
	Send    chan []byte
	control chan int // Used for internal synchronization control commands 用于内部同步控制命令
}

type WebSocket struct {
	conn        *websocket.Conn // nil表示禁用
	lock        *deadlock.RWMutex
	url         string
	dialer      *websocket.Dialer
	onReConnect func() *errs.Error
	id          int
}

func (ws *WebSocket) Close() error {
	if ws.conn != nil {
		ws.lock.Lock()
		var err error
		if ws.conn != nil {
			err = ws.conn.Close()
			ws.conn = nil
		}
		ws.lock.Unlock()
		return err
	}
	return nil
}

func (ws *WebSocket) WriteClose() error {
	conn, lock := ws.readConn()
	var err error
	if conn != nil {
		exitData := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
		err = conn.WriteMessage(websocket.CloseMessage, exitData)
	}
	lock.RUnlock()
	return err
}

func (ws *WebSocket) readConn() (*websocket.Conn, *deadlock.RWMutex) {
	ws.lock.RLock()
	return ws.conn, ws.lock
}

func (ws *WebSocket) reConnect() error {
	err_ := ws.initConn()
	if err_ != nil {
		return err_
	}
	log.Info("reconnect success", zap.String("url", ws.url), zap.Int("id", ws.id))
	if ws.onReConnect != nil {
		err2 := ws.onReConnect()
		if err2 != nil {
			return err2
		}
	}
	return nil
}

func (ws *WebSocket) ReConnect() error {
	var err = ws.Close()
	if err != nil {
		log.Warn("close ws conn fail", zap.String("url", ws.url), zap.Int("id", ws.id), zap.Error(err))
	}
	return ws.reConnect()
}

func (ws *WebSocket) NextWriter() (io.WriteCloser, error) {
	conn, lock := ws.readConn()
	var writer io.WriteCloser
	var err error
	if conn != nil {
		writer, err = conn.NextWriter(websocket.TextMessage)
	} else {
		err = errors.New(fmt.Sprintf("ws conn [%d] %s closed, NextWriter fail", ws.id, ws.url))
	}
	lock.RUnlock()
	return writer, err
}

func (ws *WebSocket) ReadMsg() ([]byte, error) {
	var msgType int
	var msgRaw []byte
	var err error
	for {
		conn, lock := ws.readConn()
		if conn != nil {
			msgType, msgRaw, err = conn.ReadMessage()
		} else {
			msgType = -1
		}
		lock.RUnlock()
		if msgType < 0 {
			return nil, errors.New(fmt.Sprintf("ws conn [%d] %s closed, read fail", ws.id, ws.url))
		}
		if err != nil {
			var closeErr *websocket.CloseError
			var wait time.Duration
			var tryReConn = false
			var code = -1
			var errText = err.Error()
			if errors.As(err, &closeErr) {
				// Closed, no further use allowed
				// 已关闭，禁止继续使用
				code = closeErr.Code
				tryReConn = true
				if code == 1006 || code == 1011 || code == 1012 || code == 1013 {
					if code == 1013 {
						// 等10s重试
						wait = time.Millisecond * 10000
					} else {
						wait = time.Millisecond * 500
					}
				} else if code == 1008 && strings.Contains(errText, "Pong timeout") {
					wait = time.Millisecond * 500
				} else {
					wait = time.Millisecond * 1000
				}
			} else if strings.Contains(errText, "EOF") || strings.Contains(errText, "connection timed out") ||
				strings.Contains(errText, "connection reset") {
				tryReConn = true
				wait = time.Millisecond * 500
			}
			if tryReConn {
				// 连接不可用，提前锁定
				ws.lock.Lock()
			}
			if wait > 0 {
				time.Sleep(wait)
			}
			if tryReConn {
				log.Info(fmt.Sprintf("[%v] ws %v closed, reconnecting: %s, err: %T %v", code, ws.id, ws.url, err, errText))
				err_ := ws.reConnect()
				if err_ != nil {
					// 重连失败，禁用
					ws.conn = nil
					ws.lock.Unlock()
					return nil, err_
				}
				ws.lock.Unlock()
				return ws.ReadMsg()
			}
			return nil, err
		} else if msgType == websocket.TextMessage {
			return msgRaw, nil
		}
	}
}

func (ws *WebSocket) IsOK() bool {
	return ws.conn != nil
}

func (ws *WebSocket) initConn() error {
	conn, _, err := ws.dialer.Dial(ws.url, http.Header{})
	if err != nil {
		return err
	}
	ws.conn = conn
	return nil
}

func (ws *WebSocket) GetID() int {
	return ws.id
}

func (ws *WebSocket) SetID(v int) {
	ws.id = v
}

func newWebSocket(id int, reqUrl string, args map[string]interface{}, onReConnect func() *errs.Error) (*AsyncConn, error) {
	var dialer = &websocket.Dialer{}
	dialer.HandshakeTimeout = utils.GetMapVal(args, ParamHandshakeTimeout, time.Second*15)
	var defProxy func(*http.Request) (*url.URL, error)
	var proxy = utils.GetMapVal(args, ParamProxy, defProxy)
	if proxy != nil {
		dialer.Proxy = proxy
	}
	res := &WebSocket{id: id, dialer: dialer, url: reqUrl, onReConnect: onReConnect}
	res.lock = &deadlock.RWMutex{}
	err := res.initConn()
	if err != nil {
		return nil, errs.New(errs.CodeConnectFail, err)
	}
	return &AsyncConn{
		WsConn:  res,
		Send:    make(chan []byte),
		control: make(chan int),
	}, nil
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

func newWsClient(reqUrl, acc string, onMsg FuncOnWsMsg, onErr FuncOnWsErr, onClose FuncOnWsClose, onReCon FuncOnWsReCon,
	params map[string]interface{}, debug bool) (*WsClient, *errs.Error) {
	args := utils.SafeParams(params)
	var result = &WsClient{
		AccName:       acc,
		URL:           reqUrl,
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
	client, ok := e.WSClients[clientKey]
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
	client, err := newWsClient(wsUrl, accName, e.OnWsMsg, e.OnWsErr, onClosed, e.OnWsReCon, params, e.DebugWS)
	if err != nil {
		return nil, err
	}
	client.Exg = e
	client.MarketType = marketType
	client.Key = clientKey
	e.WSClients[clientKey] = client
	if e.CheckWsTimeout != nil && !e.WsChecking {
		go e.CheckWsTimeout()
	}
	return client, nil
}

/*
GetWsOutChan
获取指定msgHash的输出通道
如果不存在则创建新的并存储
*/
func GetWsOutChan[T any](e *Exchange, chanKey string, create func(int) T, args map[string]interface{}) T {
	outRaw, oldChan := e.WsOutChans[chanKey]
	if oldChan {
		res := outRaw.(T)
		return res
	} else {
		chanCap := utils.PopMapVal(args, ParamChanCap, 100)
		res := create(chanCap)
		e.WsOutChans[chanKey] = res
		if e.OnWsChan != nil {
			e.OnWsChan(chanKey, res)
		}
		return res
	}
}

func WriteOutChan[T any](e *Exchange, chanKey string, msg T, popIfNeed bool) bool {
	outRaw, outOk := e.WsOutChans[chanKey]
	if outOk {
		out, ok := outRaw.(chan T)
		if !ok {
			log.Error("out chan type error", zap.String("k", chanKey))
			return false
		}
		select {
		case out <- msg:
		default:
			if !popIfNeed {
				log.Error("out chan full", zap.String("k", chanKey))
				return false
			}
			// chan通道满了，弹出最早的消息，重新发送
			<-out
			out <- msg
		}
	}
	return outOk
}

func (e *Exchange) AddWsChanRefs(chanKey string, keys ...string) {
	data, ok := e.WsChanRefs[chanKey]
	if !ok {
		data = make(map[string]struct{})
		e.WsChanRefs[chanKey] = data
	}
	for _, k := range keys {
		data[k] = struct{}{}
	}
}

func (e *Exchange) DelWsChanRefs(chanKey string, keys ...string) int {
	data, ok := e.WsChanRefs[chanKey]
	if !ok {
		return -1
	}
	for _, k := range keys {
		delete(data, k)
	}
	hasNum := len(data)
	if hasNum == 0 {
		if out, ok := e.WsOutChans[chanKey]; ok {
			val := reflect.ValueOf(out)
			if val.Kind() == reflect.Chan {
				val.Close()
			}
			delete(e.WsOutChans, chanKey)
			log.Info("remove chan", zap.String("key", chanKey))
		}
	}
	return hasNum
}

func (e *Exchange) handleWsClientClosed(client *WsClient) int {
	prefix := client.Prefix("")
	removeNum := 0
	for key, _ := range e.WsChanRefs {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		delete(e.WsChanRefs, key)
		if out, ok := e.WsOutChans[key]; ok {
			val := reflect.ValueOf(out)
			if val.Kind() == reflect.Chan {
				val.Close()
			}
			delete(e.WsOutChans, key)
			removeNum += 1
		}
	}
	return removeNum
}

/*
CheckWsError
从websocket返回的消息结果中，检查是否有错误信息
*/
func CheckWsError(msg map[string]string) *errs.Error {
	errRaw, ok := msg["error"]
	if ok {
		var err = &errs.Error{}
		_ = utils.UnmarshalString(errRaw, err, utils.JsonNumDefault)
		return err
	}
	status, ok := msg["status"]
	if ok && status != "200" {
		statusVal, e := strconv.Atoi(status)
		if e != nil {
			return nil
		}
		msgStr, _ := utils.MarshalString(msg)
		return errs.NewMsg(statusVal, msgStr)
	}
	return nil
}

type SubStat struct {
	Conn     *AsyncConn
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
			c.connLock.Lock()
			conn := c.conns[cid]
			c.connLock.Unlock()
			stat = &SubStat{
				Conn:     conn,
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
			if strings.HasPrefix(k, key) {
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
	if conn == nil || c.Exg.WsDecoder != nil {
		// skip write ws msg in replay mode
		return nil
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
		log.Debug("write ws msg", zap.String("url", c.URL), zap.Int("id", conn.GetID()),
			zap.String("msg", string(data)))
	}
	conn.Send <- data
	return nil
}

func (c *WsClient) Close() {
	c.connLock.Lock()
	conns := utils.ValsOfMap(c.conns)
	c.connLock.Unlock()
	for _, conn := range conns {
		conn.control <- ctrlDoClose
	}
}

func (c *WsClient) write(conn *AsyncConn) {
	zapFields := []zap.Field{zap.String("url", c.URL), zap.Int("id", conn.GetID())}
	defer func() {
		log.Debug("stop write ws", zapFields...)
		err := conn.Close()
		if err != nil {
			log.Error("close ws error", append(zapFields, zap.Error(err))...)
		}
		close(conn.control)
		c.connLock.Lock()
		delete(c.conns, conn.GetID())
		c.connLock.Unlock()
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
		case msg, ok := <-conn.Send:
			if !ok {
				err := conn.WriteClose()
				if err != nil {
					log.Error("write ws close error", append(zapFields, zap.Error(err))...)
					return
				}
				log.Info("WsClient.Send closed", zapFields...)
				return
			}
			w, err := conn.NextWriter()
			if err != nil {
				log.Error("failed to create Ws.Writer", append(zapFields, zap.Error(err))...)
				return
			}
			// 一次只能写入一条消息
			_, err = w.Write(msg)
			if err != nil {
				log.Error("write ws fail", append(zapFields, zap.Error(err))...)
			}
			if err = w.Close(); err != nil {
				log.Error("close WriteCloser fail", append(zapFields, zap.Error(err))...)
				return
			}
		}
	}
}

func (c *WsClient) read(conn *AsyncConn) {
	defer func() {
		conn.control <- ctrlClosed
	}()
	for {
		msgRaw, err := conn.ReadMsg()
		if err != nil {
			if !conn.IsOK() {
				if c.OnClose != nil {
					c.OnClose(c, errs.New(errs.CodeWsReadFail, err))
				}
				log.Error("read fail, ws closed", zap.String("url", c.URL), zap.Int("id", conn.GetID()), zap.Error(err))
				return
			} else {
				if c.OnError != nil {
					c.OnError(c, errs.New(errs.CodeWsReadFail, err))
				}
				log.Error("read error", zap.String("url", c.URL), zap.Int("id", conn.GetID()), zap.Error(err))
				continue
			}
		}
		// skip ws msg in replay mode
		if c.Exg.WsDecoder == nil {
			// We cannot start a goroutine for each message here, otherwise it will result in incorrect message processing order
			// 这里不能对每个消息启动一个goroutine，否则会导致消息处理顺序错误
			c.Exg.DumpWS("wsMsg", []string{c.URL, c.MarketType, c.AccName, string(msgRaw)})
			c.HandleRawMsg(msgRaw)
		}
	}
}

func (c *WsClient) HandleRawMsg(msgRaw []byte) {
	msgText := string(msgRaw)
	if c.Debug {
		log.Debug("receive ws msg", zap.String("url", c.URL), zap.String("msg", msgText))
	}
	// fmt.Printf("receive %s\n", msgText)
	msg, err := NewWsMsg(msgText)
	if err != nil {
		if c.OnError != nil {
			c.OnError(c, err)
		}
		log.Error("invalid ws msg", zap.String("msg", msgText), zap.Error(err))
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
				num, _ := c.connSubs[cid]
				if num <= 1 {
					delete(c.connSubs, cid)
				} else {
					c.connSubs[cid] = num - 1
				}
				delete(c.SubscribeKeys, key)
				delete(c.SubsKeyStamps, key)
			}
		}
		c.subsLock.Unlock()
	} else {
		connMap, lock := c.LockConns()
		conn, _ = connMap[connID]
		// Check if there are any existing connections that have not reached the minimum number of subscriptions
		// 检查已有连接，是否有未达到最低订阅数的
		if conn == nil {
			for cid, con := range connMap {
				num, _ := c.connSubs[cid]
				if num < connMinSubs {
					conn = con
					break
				}
			}
		}
		// Attempt to create a new connection
		// 尝试创建新连接
		connNum := len(connMap)
		lock.Unlock()
		if conn == nil && connNum < maxClientConn {
			var err *errs.Error
			conn, err = c.newConn(true)
			if err != nil {
				log.Warn("make new websocket fail", zap.String("url", c.URL))
			}
		}
		// Randomly select one from existing connections
		// 从已有连接随机挑一个
		if conn == nil {
			lock.Lock()
			cids := utils.KeysOfMap(connMap)
			conn = connMap[cids[rand.Intn(len(cids))]]
			lock.Unlock()
		}
		connID = conn.GetID()
		curMS := bntp.UTCStamp()
		c.subsLock.Lock()
		for _, key := range keys {
			c.SubscribeKeys[key] = connID
			c.SubsKeyStamps[key] = curMS
		}
		c.subsLock.Unlock()
		num, _ := c.connSubs[connID]
		c.connSubs[connID] = num + len(keys)
	}
	return method, conn
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

func (c *WsClient) newConn(add bool) (*AsyncConn, *errs.Error) {
	connID := c.NextConnId
	conn, err := newWebSocket(connID, c.URL, c.connArgs, func() *errs.Error {
		return c.OnReConn(c, connID)
	})
	if err != nil {
		return nil, errs.New(errs.CodeConnectFail, err)
	}
	log.Debug("new websocket conn", zap.String("url", c.URL), zap.Int("id", conn.GetID()))
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
