package bex

import (
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
)

type FuncNewExchange = func(map[string]interface{}) (banexg.BanExchange, *errs.Error)

var newExgs map[string]FuncNewExchange

func WrapNew[T banexg.BanExchange](fn func(map[string]interface{}) (T, *errs.Error)) FuncNewExchange {
	return func(options map[string]interface{}) (banexg.BanExchange, *errs.Error) {
		return fn(options)
	}
}
