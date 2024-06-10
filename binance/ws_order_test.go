package binance

import (
	"bufio"
	"fmt"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"github.com/bytedance/sonic"
	"github.com/h2non/gock"
	"go.uber.org/zap"
	"go.uber.org/zap/buffer"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWatchOrderBook(t *testing.T) {
	exg := getBinance(nil)
	symbol := "ETH/USDT:USDT"
	out, err := exg.WatchOrderBooks([]string{symbol}, 100, nil)
	if err != nil {
		panic(err)
	}
	for {
		select {
		case msg := <-out:
			msgText, err := sonic.MarshalString(msg)
			if err != nil {
				log.Error("marshal msg fail", zap.Error(err))
				continue
			}
			log.Info("ws", zap.String("msg", msgText))
		}
	}
}

type MockWsConn struct {
	Path    string
	file    *os.File
	scanner *bufio.Scanner
	msgChan chan []byte
	lock    sync.Mutex
}

func (c *MockWsConn) Close() error {
	if c.file != nil {
		err := c.file.Close()
		c.file = nil
		return err
	}
	return nil
}

func (c *MockWsConn) WriteClose() error {
	return nil
}

func (c *MockWsConn) NextWriter() (io.WriteCloser, error) {
	return &mockWriter{conn: c}, nil
}

func (c *MockWsConn) ReadMsg() ([]byte, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	time.Sleep(time.Millisecond * 30)

	// 检查通道是否有数据
	select {
	case data, ok := <-c.msgChan:
		if ok {
			log.Info("receive msg", zap.String("msg", string(data)))
			var msgRaw map[string]interface{}
			_ = utils.Unmarshal(data, &msgRaw)
			msg := utils.MapValStr(msgRaw)
			method, _ := utils.SafeMapVal(msg, "method", "")
			id, _ := utils.SafeMapVal(msg, "id", 0)
			var retData = fmt.Sprintf("{\"id\":%d,\"result\":null}", id)
			if method == "SUBSCRIBE" {

			} else {
				log.Error("unsupport ws method", zap.String("method", method))
			}
			log.Info("ret msg", zap.String("msg", retData))
			return []byte(retData), nil
		}
	default:
	}
	if c.scanner == nil {
		file, err := os.Open(c.Path)
		if err != nil {
			return nil, err
		}
		c.file = file
		c.scanner = bufio.NewScanner(file)
	}
	if !c.scanner.Scan() {
		err := c.scanner.Err()
		if err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	return c.scanner.Bytes(), nil
}

func (c *MockWsConn) IsOK() bool {
	return c.scanner != nil
}

type mockWriter struct {
	conn *MockWsConn
}

func (w *mockWriter) Write(p []byte) (n int, err error) {
	w.conn.lock.Lock()
	defer w.conn.lock.Unlock()
	select {
	case w.conn.msgChan <- p:
	default:
		return 0, io.ErrShortWrite
	}
	return len(p), nil
}

func (w *mockWriter) Close() error {
	// 可以在这里添加关闭逻辑，如果需要
	return nil
}

func TestWatchOrderBookOut(t *testing.T) {
	err := LoadGockItems("testdata/gock.json")
	gock.DisableNetworking()
	if err != nil {
		panic(err)
	}
	gock.New("https://fapi.binance.com").Get("/fapi/v1/depth").
		Reply(200).File("testdata/order_book_shot.json")

	var conn banexg.WsConn
	conn = &MockWsConn{
		Path:    "testdata/ws_odbook_msg.log",
		msgChan: make(chan []byte, 10),
	}
	exg := getBinance(map[string]interface{}{
		banexg.OptWsConn: conn,
	})
	// 模拟网络请求
	gock.InterceptClient(exg.HttpClient)
	symbol := "ETH/USDT:USDT" // 这里必须是ETH/USDT:USDT
	exg.WsIntvs["WatchOrderBooks"] = 100
	// 此处请求订单簿镜像时mock的testdata/order_book_shot.json
	out, err_ := exg.WatchOrderBooks([]string{symbol}, 100, nil)
	if err_ != nil {
		panic(err_)
	}
	data, err := utils.ReadFile("testdata/ccxt_book.log")
	if err != nil {
		fmt.Println("read fail:", err)
		return
	}
	expect := strings.Replace(string(data), "\r\n", "\n", -1)
	writer := buffer.Buffer{}
	var book *banexg.OrderBook
mainFor:
	for {
		select {
		case tmp, ok := <-out:
			if !ok {
				log.Info("read out chan fail, break")
				break mainFor
			}
			book = tmp
		}
	}
	if book == nil {
		panic("no book received")
	}
	_, _ = writer.WriteString(fmt.Sprintf("---------- %v ----------\n", book.TimeStamp))
	for _, row := range book.Asks.Rows {
		_, _ = writer.WriteString(fmt.Sprintf("ask: %.3f %.6f\n", row[0], row[0]*row[1]))
	}
	for _, row := range book.Bids.Rows {
		_, _ = writer.WriteString(fmt.Sprintf("bid: %.3f %.6f\n", row[0], row[0]*row[1]))
	}
	_, _ = writer.WriteString("\n")
	output := writer.String()
	if output != expect {
		outPath := "D:/banexg_odbook.log"
		t.Error("order book invalid, please check:" + outPath)
		err := os.WriteFile(outPath, []byte(output), 0644)
		if err != nil {
			panic(fmt.Sprintf("write bad order book fail:%s", outPath))
		}
	}
}

func TestWatchTrades(t *testing.T) {
	exg := getBinance(nil)
	var symbols = []string{"ETC/USDT:USDT"}
	exg.MarketType = banexg.MarketLinear
	out, err := exg.WatchTrades(symbols, nil)
	if err != nil {
		panic(err)
	}
	fmt.Println("start watching trades")
mainFor:
	for {
		select {
		case trade, ok := <-out:
			if !ok {
				log.Info("read out chan fail, break")
				break mainFor
			}
			builder := strings.Builder{}
			builder.WriteString(trade.Symbol + ", ")
			builder.WriteString(fmt.Sprintf("%v, ", trade.Amount))
			builder.WriteString(fmt.Sprintf("%v, ", trade.Price))
			builder.WriteString(fmt.Sprintf("%v, ", trade.Side))
			builder.WriteString("\n")
			fmt.Print(builder.String())
		}
	}
}

func TestWatchMyTrades(t *testing.T) {
	exg := getBinance(nil)
	exg.MarketType = banexg.MarketLinear
	out, err := exg.WatchMyTrades(nil)
	if err != nil {
		panic(err)
	}
	fmt.Println("start watching my trades")
mainFor:
	for {
		select {
		case trade, ok := <-out:
			if !ok {
				log.Info("read out chan fail, break")
				break mainFor
			}
			builder := strings.Builder{}
			builder.WriteString(trade.Symbol + ", ")
			builder.WriteString(fmt.Sprintf("%v, ", trade.Amount))
			builder.WriteString(fmt.Sprintf("%v, ", trade.Filled))
			builder.WriteString(fmt.Sprintf("%v, ", trade.Price))
			builder.WriteString(fmt.Sprintf("%v, ", trade.Average))
			builder.WriteString(fmt.Sprintf("%v, ", trade.State))
			builder.WriteString("\n")
			fmt.Print(builder.String())
		}
	}
}
