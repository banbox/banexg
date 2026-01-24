package bybit

import (
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/banbox/banexg"
)

type stubWsConn struct {
	id    int
	block chan struct{}
}

func (c *stubWsConn) Close() error {
	return nil
}

func (c *stubWsConn) WriteClose() error {
	return nil
}

func (c *stubWsConn) ReConnect() error {
	return nil
}

func (c *stubWsConn) NextWriter() (io.WriteCloser, error) {
	return nopWriteCloser{Writer: io.Discard}, nil
}

func (c *stubWsConn) ReadMsg() ([]byte, error) {
	<-c.block
	return nil, io.EOF
}

func (c *stubWsConn) IsOK() bool {
	return true
}

func (c *stubWsConn) GetID() int {
	return c.id
}

func (c *stubWsConn) SetID(v int) {
	c.id = v
}

type nopWriteCloser struct {
	io.Writer
}

func (n nopWriteCloser) Close() error {
	return nil
}

func newTestAsyncConn() *banexg.AsyncConn {
	c := &stubWsConn{block: make(chan struct{})}
	return &banexg.AsyncConn{WsConn: c}
}

func seedMarketIfNeeded(exg *Bybit, marketID, symbol, marketType string) {
	if marketID == "" && symbol == "" {
		return
	}
	seedMarket(exg, marketID, symbol, marketType)
}

func wsOutChan[T any](exg *Bybit, client *banexg.WsClient, key string) chan T {
	chanKey := client.Prefix(key)
	create := func(cap int) chan T {
		return make(chan T, cap)
	}
	args := map[string]interface{}{banexg.ParamChanCap: 4}
	return banexg.GetWsOutChan(exg.Exchange, chanKey, create, args)
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json marshal failed: %v", err)
	}
	return data
}

func readChan[T any](t *testing.T, ch <-chan T, waitFor ...string) T {
	t.Helper()
	what := "ws output"
	if len(waitFor) > 0 && waitFor[0] != "" {
		what = waitFor[0]
	}
	select {
	case msg := <-ch:
		return msg
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for %s", what)
	}
	var zero T
	return zero
}
