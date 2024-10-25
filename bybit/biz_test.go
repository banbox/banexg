package bybit

import (
	"fmt"
	"github.com/banbox/banexg/utils"
	"testing"
)

func TestLoadMarkets(t *testing.T) {
	exg := getBybit(nil)
	markets, err := exg.LoadMarkets(false, nil)
	if err != nil {
		panic(err)
	}
	outPath := "D:/bybit_markets.json"
	err_ := utils.WriteJsonFile(outPath, markets)
	if err_ != nil {
		panic(err_)
	}
	fmt.Printf("dump markets at: %v", outPath)
}
