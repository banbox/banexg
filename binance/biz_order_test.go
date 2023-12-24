package binance

import (
	"fmt"
	"github.com/anyongjin/banexg"
	"github.com/anyongjin/banexg/log"
	"github.com/bytedance/sonic"
	"go.uber.org/zap"
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
	args := &map[string]interface{}{
		banexg.ParamPositionSide: "LONG",
	}
	symbol := "ETH/USDT:USDT"
	printCreateOrder(symbol, banexg.OdTypeLimit, banexg.OdSideBuy, 0.02, 1000, args)
}

func TestCalcelOrder(t *testing.T) {
	exg := getBinance(nil)
	symbol := "ETH/USDT:USDT"

	res, err := exg.CancelOrder("8389765637620768314", symbol, nil)
	if err != nil {
		panic(err)
	}
	resStr, _ := sonic.MarshalString(res)
	log.Info("cancel order", zap.String("res", resStr))
}
