package binance

import (
	"github.com/banbox/banexg/log"
	"go.uber.org/zap"
	"testing"
)

func TestFetchOrderBook(t *testing.T) {
	exg := getBinance(nil)
	symbol := "ETH/USDT"
	res, err := exg.FetchOrderBook(symbol, 100, nil)
	if err != nil {
		panic(err)
	}
	log.Info("fetch order book", zap.Any("v", res))
}
