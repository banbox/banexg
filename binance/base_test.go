package binance

import (
	"github.com/anyongjin/banexg"
	"github.com/anyongjin/banexg/log"
	"github.com/anyongjin/banexg/utils"
)

func getBinance(param *map[string]interface{}) *Binance {
	banexg.IsUnitTest = true
	log.SetupByArgs(true, "")
	args := utils.SafeParams(param)
	local := make(map[string]interface{})
	_ = utils.ReadJsonFile("local.json", &local)
	for k, v := range local {
		args[k] = v
	}
	return NewExchange(args)
}
