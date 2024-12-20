package binance

import (
	"fmt"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/utils"
	"testing"
)

func TestFetchTicker(t *testing.T) {
	exg := getBinance(nil)
	ticker, err := exg.FetchTicker("BTC/USDT", nil)
	if err != nil {
		panic(err)
	}
	ticker.Info = nil
	fmt.Println(utils.MarshalString(ticker))
}

func TestFetchTicker2(t *testing.T) {
	exg := getBinance(nil)
	ticker, err := exg.FetchTicker("BTC/USDT:USDT", nil)
	if err != nil {
		panic(err)
	}
	ticker.Info = nil
	fmt.Println(utils.MarshalString(ticker))
}

func TestFetchTicker3(t *testing.T) {
	exg := getBinance(nil)
	ticker, err := exg.FetchTicker("BTC/USD:BTC", nil)
	if err != nil {
		panic(err)
	}
	ticker.Info = nil
	fmt.Println(utils.MarshalString(ticker))
}

func TestFetchTickers(t *testing.T) {
	exg := getBinance(nil)
	tickers, err := exg.FetchTickers(nil, map[string]interface{}{
		"market": banexg.MarketLinear,
	})
	if err != nil {
		panic(err)
	}
	for _, ticker := range tickers {
		ticker.Info = nil
	}
	fmt.Println(utils.MarshalString(tickers))
}

func TestFetchTickerPrice(t *testing.T) {
	exg := getBinance(nil)
	prices, err := exg.FetchTickerPrice("", map[string]interface{}{
		"market": banexg.MarketLinear,
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(utils.MarshalString(prices))
}
