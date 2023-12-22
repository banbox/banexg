package binance

import (
	"fmt"
	"github.com/anyongjin/banexg"
	"github.com/bytedance/sonic"
	"testing"
)

func printCreateOrder(symbol string, odType string, side string, amount float64, price float64, params *map[string]interface{}) {
	exg := getBinance(nil)
	res, err := exg.CreateOrder(symbol, odType, side, amount, price, params)
	if err != nil {
		panic(err)
	}
	resStr, err := sonic.MarshalString(res)
	if err != nil {
		panic(err)
	}
	fmt.Printf(resStr)
	fmt.Printf("\n")
}

func TestBinance_CreateOrder(t *testing.T) {
	printCreateOrder("ETH/USDT", banexg.OdTypeLimit, banexg.OdSideBuy, 0.1, 1000, nil)
}
