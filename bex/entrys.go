package bex

import (
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/binance"
	"github.com/banbox/banexg/bybit"
	"github.com/banbox/banexg/china"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/okx"
	"github.com/banbox/banexg/utils"
)

func init() {
	newExgs = map[string]FuncNewExchange{
		"binance": binance.NewExchange,
		"bybit":   bybit.NewExchange,
		"china":   china.NewExchange,
		"okx":     okx.NewExchange,
	}
}

func New(name string, options map[string]interface{}) (banexg.BanExchange, *errs.Error) {
	fn, ok := newExgs[name]
	if !ok {
		return nil, errs.NewMsg(errs.CodeBadExgName, "invalid exg name: %s", name)
	}
	return fn(utils.SafeParams(options))
}
