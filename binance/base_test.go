package binance

import (
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
)

func getBinance(param *map[string]interface{}) *Binance {
	log.SetupByArgs(true, "")
	args := utils.SafeParams(param)
	local := make(map[string]interface{})
	_ = utils.ReadJsonFile("local.json", &local)
	for k, v := range local {
		args[k] = v
	}
	return NewExchange(args)
}
