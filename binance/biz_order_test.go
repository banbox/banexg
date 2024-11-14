package binance

import (
	"fmt"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"go.uber.org/zap"
	"testing"
)

func TestFetchOrder(t *testing.T) {
	exg := getBinance(nil)
	cases := map[string]map[string]interface{}{
		"281077600438": {"market": banexg.MarketLinear},
	}

	symbol := "BTC/USDT:USDT"
	for orderId, item := range cases {
		text, _ := utils.MarshalString(item)
		res, err := exg.FetchOrder(symbol, orderId, item)
		if err != nil {
			panic(fmt.Errorf("%s Error: %v", text, err))
		}
		resText, _ := utils.MarshalString(res)
		t.Logf("%s result: %s", text, resText)
	}
}

func TestFetchOrders(t *testing.T) {
	exg := getBinance(nil)
	cases := []map[string]interface{}{
		//{"market": banexg.MarketSpot},
		{"market": banexg.MarketLinear},
		//{"market": banexg.MarketInverse},
		//{"market": banexg.MarketOption},
	}

	symbol := "BTC/USDT:USDT"
	since := int64(1708511530259)
	for _, item := range cases {
		text, _ := utils.MarshalString(item)
		res, err := exg.FetchOrders(symbol, since, 0, item)
		if err != nil {
			panic(fmt.Errorf("%s Error: %v", text, err))
		}
		resText, _ := utils.MarshalString(res)
		t.Logf("%s result: %s", text, resText)
	}
}

func TestFetchOpenOrders(t *testing.T) {
	exg := getBinance(nil)
	cases := []map[string]interface{}{
		//{"market": banexg.MarketSpot},
		{"market": banexg.MarketLinear},
		//{"market": banexg.MarketInverse},
		//{"market": banexg.MarketOption},
	}
	symbol := "ETH/USDT:USDT"
	since := int64(1702991965921)
	for _, item := range cases {
		text, _ := utils.MarshalString(item)
		res, err := exg.FetchOpenOrders(symbol, since, 0, item)
		if err != nil {
			panic(fmt.Errorf("%s Error: %v", text, err))
		}
		resText, _ := utils.MarshalString(res)
		t.Logf("%s result: %s", text, resText)
	}
}

func printCreateOrder(symbol string, odType string, side string, amount float64, price float64, params map[string]interface{}) {
	exg := getBinance(nil)
	res, err := exg.CreateOrder(symbol, odType, side, amount, price, params)
	if err != nil {
		panic(err)
	}
	resStr, err2 := utils.MarshalString(res)
	if err2 != nil {
		panic(err2)
	}
	fmt.Printf(resStr)
	fmt.Printf("\n")
}

func TestBinance_CreateOrder(t *testing.T) {
	args := map[string]interface{}{
		banexg.ParamPositionSide: "LONG",
	}
	symbol := "ETH/USDT:USDT"
	printCreateOrder(symbol, banexg.OdTypeLimit, banexg.OdSideBuy, 0.02, 1000, args)
}

func TestSellOrder(t *testing.T) {
	symbol := "USDT/BRL"
	printCreateOrder(symbol, banexg.OdTypeMarket, banexg.OdSideSell, 10, 0, nil)
}

func TestCalcelOrder(t *testing.T) {
	exg := getBinance(nil)
	symbol := "ETH/USDT:USDT"

	res, err := exg.CancelOrder("8389765637843621129", symbol, nil)
	if err != nil {
		panic(err)
	}
	resStr, _ := utils.MarshalString(res)
	log.Info("cancel order", zap.String("res", resStr))
}
