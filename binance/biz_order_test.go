package binance

import (
	"fmt"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"github.com/banbox/bntp"
	"go.uber.org/zap"
	"testing"
	"time"
)

func TestFetchOrder(t *testing.T) {
	exg := getBinance(nil)
	cases := map[string]map[string]interface{}{
		"25578760824": {"market": banexg.MarketLinear},
	}

	symbol := "ETC/USDT:USDT"
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
	now := time.Now().UnixMilli()
	loopIntv := int64(86400000 * 7)
	cases := []map[string]interface{}{
		//{"market": banexg.MarketSpot},
		{"market": banexg.MarketLinear, banexg.ParamUntil: now, banexg.ParamLoopIntv: loopIntv},
		//{"market": banexg.MarketInverse},
		//{"market": banexg.MarketOption},
	}

	symbol := "XRP/USDT:USDT"
	since := now - loopIntv*4 // 4周前作为开始时间
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
	symbol := "ETC/USDT:USDT"
	since := time.Date(2025, 4, 6, 0, 0, 0, 0, time.UTC).UnixMilli()
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

func TestTriggerLongStop(t *testing.T) {
	bntp.LangCode = bntp.LangZhCN
	exg := getBinance(nil)
	symbol := "DOGE/USDT:USDT"
	priceMap, err := exg.FetchTickerPrice(symbol, nil)
	if err != nil {
		panic(err)
	}
	price := priceMap[symbol]
	if price == 0 {
		fmt.Printf("get ticker price fail: %v", priceMap)
		return
	}
	args := map[string]interface{}{
		banexg.ParamPositionSide:    "LONG",
		banexg.ParamTakeProfitPrice: price * 1.05,
	}
	amt := 10 / price
	printCreateOrder(symbol, banexg.OdTypeMarket, banexg.OdSideBuy, amt, 0, args)
}

func TestSellOrder(t *testing.T) {
	symbol := "USDT/BRL"
	printCreateOrder(symbol, banexg.OdTypeMarket, banexg.OdSideSell, 10, 0, nil)
}

func TestCalcelOrder(t *testing.T) {
	exg := getBinance(nil)
	symbol := "ETH/USDT:USDT"

	res, err := exg.CancelOrder("8389765870487818124", symbol, nil)
	if err != nil {
		panic(err)
	}
	resStr, _ := utils.MarshalString(res)
	log.Info("cancel order", zap.String("res", resStr))
}
