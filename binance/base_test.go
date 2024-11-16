package binance

import (
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
)

func getBinance(param map[string]interface{}) *Binance {
	log.Setup("info", "")
	args := utils.SafeParams(param)
	local := make(map[string]interface{})
	err_ := utils.ReadJsonFile("local.json", &local, utils.JsonNumAuto)
	if err_ != nil {
		panic(err_)
	}
	for k, v := range local {
		args[k] = v
	}
	exg, err := New(args)
	if err != nil {
		panic(err)
	}
	return exg
}
