package banexg

import (
	"fmt"
	"github.com/anyongjin/banexg/log"
	"github.com/anyongjin/banexg/utils"
	"github.com/bytedance/sonic"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"net/http"
	"net/url"
	"time"
)

type WsClient struct {
	Conn      *websocket.Conn
	URL       string
	Proxy     *url.URL
	Send      chan []byte
	control   chan int              // 用于内部同步控制命令
	SubInfos  map[string]*WsSubInfo // request id: Sub Data
	ChanCaps  map[string]int        // msgHash: cap size of cache msg
	OnMessage FuncOnWsMsg
	OnError   FuncOnWsErr
	OnClose   FuncOnWsClose
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
	params *map[string]interface{}) (*WsClient, error) {
	var result = &WsClient{
		URL:       reqUrl,
		Send:      make(chan []byte, 1024),
		SubInfos:  make(map[string]*WsSubInfo),
		OnMessage: onMsg,
		OnError:   onErr,
		OnClose:   onClose,
	}
	var dialer websocket.Dialer
	args := utils.SafeParams(params)
	dialer.HandshakeTimeout = utils.GetMapVal(args, ParamHandshakeTimeout, time.Second*15)
	var defProxy *url.URL
	var proxy = utils.GetMapVal(args, ParamProxy, defProxy)
	if proxy != nil {
		dialer.Proxy = http.ProxyURL(proxy)
		result.Proxy = proxy
	}
	result.ChanCaps = DefChanCaps
	chanCaps := utils.GetMapVal(args, ParamChanCaps, map[string]int{})
	for k, v := range chanCaps {
		result.ChanCaps[k] = v
	}
	conn, _, err := dialer.Dial(reqUrl, http.Header{})
	if err != nil {
		return nil, err
	}
	result.Conn = conn
	go result.read()
	go result.write()
	return result, nil
}

func (e *Exchange) GetClient(wsUrl string) (*WsClient, error) {
	if client, ok := e.WSClients[wsUrl]; ok && client.Conn != nil {
		return client, nil
	}
	params := map[string]interface{}{}
	if e.Proxy != nil {
		params[ParamProxy] = e.Proxy
	}
	if e.OnWsMsg == nil {
		return nil, fmt.Errorf("OnWsMsg is required for ws client")
	}
	client, err := newWsClient(wsUrl, e.OnWsMsg, e.OnWsErr, e.OnWsClose, &params)
	if err != nil {
		return nil, err
	}
	e.WSClients[wsUrl] = client
	return client, nil
}

/*
Watch
发送消息到ws服务器，并设置订阅
subID: ws服务器返回的任务ID
subInfo: 处理服务器返回时需要的订阅信息
*/
func (e *Exchange) Watch(wsUrl string, msg []byte, subID string, subInfo *WsSubInfo) error {
	client, err := e.GetClient(wsUrl)
	if err != nil {
		return err
	}
	if msg != nil {
		client.Send <- msg
	}
	if subID != "" {
		if _, ok := client.SubInfos[subID]; !ok {
			if subInfo == nil {
				subInfo = &WsSubInfo{}
			}
			client.SubInfos[subID] = subInfo
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
	exitData := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
	for {
		select {
		case ctrlType, ok := <-c.control:
			if !ok {
				continue
			}
			if ctrlType == ctrlClosed {
				log.Debug("conn closed")
				return
			} else if ctrlType == ctrlDoClose {
				// Cleanly close the connection by sending a close message and then
				// waiting (with timeout) for the server to close the connection.
				err := c.Conn.WriteMessage(websocket.CloseMessage, exitData)
				if err != nil {
					log.Error("write ws close error", zapUrl, zap.Error(err))
					return
				}
			} else {
				log.Error("invalid ws control type", zapUrl, zap.Int("val", ctrlType))
			}
		case msg, ok := <-c.Send:
			if !ok {
				err := c.Conn.WriteMessage(websocket.CloseMessage, exitData)
				if err != nil {
					log.Error("write ws close error", zapUrl, zap.Error(err))
					return
				}
				log.Info("WsClient.Send closed", zapUrl)
				return
			}
			w, err := c.Conn.NextWriter(websocket.TextMessage)
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
		msgType, msgRaw, err := c.Conn.ReadMessage()
		if err != nil {
			if c.OnClose != nil {
				c.OnClose(c.URL, err)
			}
			log.Error("ws closed", zap.String("url", c.URL), zap.Error(err))
			return
		}
		if msgType != websocket.TextMessage {
			continue
		}
		var msg = make(map[string]interface{})
		err = sonic.Unmarshal(msgRaw, &msg)
		if err != nil {
			fmt.Printf("unmarshal msg err\n")
			if c.OnError != nil {
				c.OnError(c.URL, err)
			}
			msgStr := string(msgRaw)
			log.Error("invalid ws msg", zap.String("msg", msgStr), zap.Error(err))
			continue
		}
		msgId, ok := msg["id"]
		if ok {
			id := fmt.Sprintf("%v", msgId)
			if sub, ok := c.SubInfos[id]; ok && sub.Method != nil {
				// 订阅信息中提供了处理函数，则调用处理函数
				sub.Method(c.URL, msg)
				delete(c.SubInfos, id)
				continue
			}
		}
		// 未匹配则调用通用消息处理
		c.OnMessage(c.URL, msg)
	}
}
