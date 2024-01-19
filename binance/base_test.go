package binance

import (
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
)

func getBinance(param *map[string]interface{}) *Binance {
	log.Setup(true, "")
	args := utils.SafeParams(param)
	local := make(map[string]interface{})
	_ = utils.ReadJsonFile("local.json", &local)
	for k, v := range local {
		args[k] = v
	}
	exg, err := New(args)
	if err != nil {
		panic(err)
	}
	return exg
}
