package bex

import (
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
)

type FuncNewExchange = func(map[string]interface{}) (banexg.BanExchange, *errs.Error)

var newExgs map[string]FuncNewExchange
