package binance

import (
	"github.com/anyongjin/banexg/log"
	"github.com/bytedance/sonic"
	"go.uber.org/zap"
	"testing"
)

func TestWatchOrderBook(t *testing.T) {
	exg := getBinance(nil)
	symbol := "ETC/USDT:USDT"
	out, err := exg.WatchOrderBook(symbol, 0, nil)
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
