package banexg

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWebSocketEstablishedStreamOutlivesInitializationContext(t *testing.T) {
	release := make(chan struct{})
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		<-release
		_ = conn.WriteMessage(websocket.TextMessage, []byte("still-live"))
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, err := newWebSocket(1, wsURL, wsURL, map[string]interface{}{ParamContext: ctx}, nil)
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	defer conn.Close()
	cancel()
	close(release)
	ws := conn.WsConn.(*WebSocket)
	raw, lock := ws.readConn()
	lock.RUnlock()
	_ = raw.SetReadDeadline(time.Now().Add(time.Second))
	_, message, readErr := raw.ReadMessage()
	if readErr != nil || string(message) != "still-live" {
		t.Fatalf("initialization context closed established stream: %q, %v", message, readErr)
	}
}
