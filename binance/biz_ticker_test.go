package binance

import (
	"fmt"
	"github.com/banbox/banexg/base"
	"github.com/bytedance/sonic"
	"testing"
)

func TestFetchTicker(t *testing.T) {
	exg := getBinance(nil)
	ticker, err := exg.FetchTicker("BTC/USDT", nil)
	if err != nil {
		panic(err)
	}
	ticker.Info = nil
	fmt.Println(sonic.MarshalString(ticker))
}

func TestFetchTicker2(t *testing.T) {
	exg := getBinance(nil)
	ticker, err := exg.FetchTicker("BTC/USDT:USDT", nil)
	if err != nil {
		panic(err)
	}
	ticker.Info = nil
	fmt.Println(sonic.MarshalString(ticker))
}

func TestFetchTicker3(t *testing.T) {
	exg := getBinance(nil)
	ticker, err := exg.FetchTicker("BTC/USD:BTC", nil)
	if err != nil {
		panic(err)
	}
	ticker.Info = nil
	fmt.Println(sonic.MarshalString(ticker))
}

func TestFetchTickers(t *testing.T) {
	exg := getBinance(nil)
	tickers, err := exg.FetchTickers(nil, &map[string]interface{}{
		"market": base.MarketLinear,
	})
	if err != nil {
		panic(err)
	}
	for _, ticker := range tickers {
		ticker.Info = nil
	}
	fmt.Println(sonic.MarshalString(tickers))
}
