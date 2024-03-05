package bex

import (
	"fmt"
	"github.com/banbox/banexg/utils"
	"testing"
)

func getExchange(name string, param map[string]interface{}) map[string]interface{} {
	args := utils.SafeParams(param)
	local := make(map[string]interface{})
	localpath := name + "/local.json"
	err := utils.ReadJsonFile(localpath, &local)
	if err != nil {
		panic(fmt.Sprintf("read %s fail: %v", localpath, err))
	}
	for k, v := range local {
		args[k] = v
	}
	return args
}

func TestNewExg(t *testing.T) {
	exgName := "binance"
	args := getExchange(exgName, nil)
	exg, err := New(exgName, args)
	if err != nil {
		panic(err)
	}
	res, err := exg.FetchOHLCV("ETH/USDT:USDT", "1d", 0, 10, nil)
	if err != nil {
		panic(err)
	}
	for _, k := range res {
		fmt.Printf("%v, %v %v %v %v %v\n", k.Time, k.Open, k.High, k.Low, k.Close, k.Volume)
	}
}
