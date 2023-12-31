package banexg

import (
	"github.com/anyongjin/banexg/errs"
	"github.com/anyongjin/banexg/log"
	"github.com/anyongjin/banexg/utils"
	"github.com/bytedance/sonic"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type WsClient struct {
	Conn       WsConn
	URL        string
	MarketType string
	Send       chan []byte
	control    chan int              // 用于内部同步控制命令
	JobInfos   map[string]*WsJobInfo // request id: Sub Data
	ChanCaps   map[string]int        // msgHash: cap size of cache msg
	OnMessage  FuncOnWsMsg
	OnError    FuncOnWsErr
	OnClose    FuncOnWsClose
}

type WebSocket struct {
	Conn *websocket.Conn
}

func (ws *WebSocket) Close() error {
	return ws.Conn.Close()
}
func (ws *WebSocket) WriteClose() error {
	exitData := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
	return ws.Conn.WriteMessage(websocket.CloseMessage, exitData)
}
func (ws *WebSocket) NextWriter() (io.WriteCloser, error) {
	return ws.Conn.NextWriter(websocket.TextMessage)
}
func (ws *WebSocket) ReadMsg() ([]byte, error) {
	for {
		msgType, msgRaw, err := ws.Conn.ReadMessage()
		if err != nil || msgType == websocket.TextMessage {
			return msgRaw, err
		}
	}
}

func newWebSocket(reqUrl string, args map[string]interface{}) (*WebSocket, error) {
	var dialer websocket.Dialer
	dialer.HandshakeTimeout = utils.GetMapVal(args, ParamHandshakeTimeout, time.Second*15)
	var defProxy *url.URL
	var proxy = utils.GetMapVal(args, ParamProxy, defProxy)
	if proxy != nil {
		dialer.Proxy = http.ProxyURL(proxy)
	}
	conn, _, err := dialer.Dial(reqUrl, http.Header{})
	if err != nil {
		return nil, errs.New(errs.CodeConnectFail, err)
	}
	return &WebSocket{Conn: conn}, nil
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

func newWsClient(reqUrl string, onMsg FuncOnWsMsg, onErr FuncOnWsErr, onClose FuncOnWsClose,
	params *map[string]interface{}) (*WsClient, *errs.Error) {
	var result = &WsClient{
		URL:       reqUrl,
		Send:      make(chan []byte, 1024),
		JobInfos:  make(map[string]*WsJobInfo),
		OnMessage: onMsg,
		OnError:   onErr,
		OnClose:   onClose,
		control:   make(chan int, 1),
	}
	args := utils.SafeParams(params)
	result.ChanCaps = DefChanCaps
	chanCaps := utils.GetMapVal(args, ParamChanCaps, map[string]int{})
	for k, v := range chanCaps {
		result.ChanCaps[k] = v
	}
	var conn WsConn
	conn = utils.GetMapVal(args, OptWsConn, conn)
	if conn == nil {
		var err error
		conn, err = newWebSocket(reqUrl, args)
		if err != nil {
			return nil, errs.New(errs.CodeConnectFail, err)
		}
	}
	result.Conn = conn
	go result.read()
	go result.write()
	return result, nil
}

func (e *Exchange) GetClient(wsUrl string, marketType string) (*WsClient, *errs.Error) {
	if client, ok := e.WSClients[wsUrl]; ok && client.Conn != nil {
		return client, nil
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
	client, err := newWsClient(wsUrl, e.OnWsMsg, e.OnWsErr, e.OnWsClose, &params)
	if err != nil {
		return nil, err
	}
	client.MarketType = marketType
	e.WSClients[wsUrl] = client
	return client, nil
}

/*
Write
发送消息到ws服务器，可设置处理任务结果需要的信息
jobID: 此次消息的任务ID，唯一标识此次请求
jobInfo: 此次任务的主要信息，在收到任务结果时使用
*/
func (c *WsClient) Write(msg interface{}, jobID string, jobInfo *WsJobInfo) *errs.Error {
	data, err2 := sonic.Marshal(msg)
	if err2 != nil {
		return errs.New(errs.CodeUnmarshalFail, err2)
	}
	c.Send <- data
	if jobID != "" && jobInfo != nil {
		if _, ok := c.JobInfos[jobID]; !ok {
			c.JobInfos[jobID] = jobInfo
		}
	}
	return nil
}

func (c *WsClient) Close() {
	c.control <- ctrlDoClose
}

func (c *WsClient) write() {
	zapUrl := zap.String("url", c.URL)
	defer func() {
		log.Debug("stop write ws", zapUrl)
		err := c.Conn.Close()
		if err != nil {
			log.Error("close ws error", zapUrl, zap.Error(err))
		}
		close(c.control)
		c.Conn = nil // 置为nil表示连接已关闭
	}()
	for {
		select {
		case ctrlType, ok := <-c.control:
			if !ok {
				log.Error("read control fail", zap.Int("flag", ctrlType))
				continue
			}
			if ctrlType == ctrlClosed {
				log.Debug("conn closed")
				return
			} else if ctrlType == ctrlDoClose {
				// Cleanly close the connection by sending a close message and then
				// waiting (with timeout) for the server to close the connection.
				err := c.Conn.WriteClose()
				if err != nil {
					log.Error("write ws close error", zapUrl, zap.Error(err))
					return
				}
			} else {
				log.Error("invalid ws control type", zapUrl, zap.Int("val", ctrlType))
			}
		case msg, ok := <-c.Send:
			if !ok {
				err := c.Conn.WriteClose()
				if err != nil {
					log.Error("write ws close error", zapUrl, zap.Error(err))
					return
				}
				log.Info("WsClient.Send closed", zapUrl)
				return
			}
			w, err := c.Conn.NextWriter()
			if err != nil {
				log.Error("failed to create Ws.Writer", zapUrl, zap.Error(err))
				return
			}
			_, err = w.Write(msg)
			if err != nil {
				log.Error("write ws fail", zapUrl, zap.Error(err))
			}
			n := len(c.Send)
			for i := 0; i < n; i++ {
				_, err = w.Write(<-c.Send)
				if err != nil {
					log.Error("write ws fail", zapUrl, zap.Error(err))
				}
			}
			if err := w.Close(); err != nil {
				log.Error("close WriteCloser fail", zapUrl, zap.Error(err))
				return
			}
		}
	}
}

func (c *WsClient) read() {
	defer func() {
		c.control <- ctrlClosed
	}()
	for {
		msgRaw, err := c.Conn.ReadMsg()
		if err != nil {
			if c.OnClose != nil {
				c.OnClose(c.URL, errs.New(errs.CodeWsReadFail, err))
			}
			log.Error("ws closed", zap.String("url", c.URL), zap.Error(err))
			return
		}
		// 这里不能对每个消息启动一个goroutine，否则会导致消息处理顺序错误
		c.handleRawMsg(msgRaw)
	}
}

func (c *WsClient) handleRawMsg(msgRaw []byte) {
	msgText := string(msgRaw)
	var err *errs.Error
	var err_ error
	var id string
	if strings.HasPrefix(msgText, "{") {
		var msg = make(map[string]interface{})
		err_ = sonic.UnmarshalString(msgText, &msg)
		if err_ == nil {
			id = c.handleMsg(utils.MapValStr(msg))
		}
	} else if strings.HasPrefix(msgText, "[") {
		var msgs = make([]map[string]interface{}, 0)
		err_ = sonic.UnmarshalString(msgText, &msgs)
		if err_ == nil && len(msgs) > 0 {
			for _, it := range msgs {
				id = c.handleMsg(utils.MapValStr(it))
			}
		}
	} else {
		err = errs.NewMsg(errs.CodeWsInvalidMsg, "invalid ws msg, not dict or list")
	}
	if err_ != nil {
		err = errs.New(errs.CodeUnmarshalFail, err_)
	}
	if err != nil {
		if c.OnError != nil {
			c.OnError(c.URL, err)
		}
		log.Error("invalid ws msg", zap.String("msg", msgText), zap.Error(err))
	} else if id != "" {
		delete(c.JobInfos, id)
	}
}

func (c *WsClient) handleMsg(msg map[string]string) string {
	id, ok := msg["id"]
	if ok {
		if sub, ok := c.JobInfos[id]; ok && sub.Method != nil {
			// 订阅信息中提供了处理函数，则调用处理函数
			sub.Method(c.URL, msg, sub)
			delete(c.JobInfos, id)
			return id
		}
	}
	// 未匹配则调用通用消息处理
	c.OnMessage(c.URL, msg)
	return ""
}
