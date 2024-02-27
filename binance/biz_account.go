package binance

import (
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

func (e *Binance) WatchAccountConfig(params *map[string]interface{}) (chan banexg.AccountConfig, *errs.Error) {
	_, client, err := e.getAuthClient(params)
	if err != nil {
		return nil, err
	}
	args := utils.SafeParams(params)
	chanKey := client.Prefix("accConfig")
	create := func(cap int) chan banexg.AccountConfig { return make(chan banexg.AccountConfig, cap) }
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, args)
	e.AddWsChanRefs(chanKey, "account")
	return out, nil
}
