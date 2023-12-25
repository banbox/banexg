package binance

import (
	"fmt"
	"testing"
)

func TestFetchOrderBook(t *testing.T) {
	exg := getBinance(nil)
	symbol := "ETH/USDT"
	res, err := exg.FetchOrderBook(symbol, 100, nil)
	if err != nil {
		panic(err)
	}
	res.Info = nil
	fmt.Printf("fetch order book %v", res)
}
