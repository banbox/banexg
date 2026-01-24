package binance

import (
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

func (e *Binance) FetchAccountAccess(params map[string]interface{}) (*banexg.AccountAccess, *errs.Error) {
	args := utils.SafeParams(params)
	res := &banexg.AccountAccess{}
	if bal, ok := args[banexg.ParamBalance].(*banexg.Balances); ok && bal != nil {
		banexg.FillAccountAccessFromInfo(res, bal.Info)
	}
	// Remove internal params that should not be sent to API
	delete(args, banexg.ParamBalance)
	rsp, err := e.Call(MethodSapiGetAccountApiRestrictions, args)
	if err != nil {
		if res.HasAny() {
			return res, nil
		}
		return res, err
	}
	var raw map[string]interface{}
	info, err2 := utils.UnmarshalStringMap(rsp.Content, &raw)
	if err2 != nil {
		if res.HasAny() {
			return res, nil
		}
		return res, errs.New(errs.CodeUnmarshalFail, err2)
	}
	if res.Info == nil {
		res.Info = info
	}
	if val, ok := banexg.BoolFromInfo(info, "ipRestrict", "ipRestriction"); ok {
		res.IPKnown = true
		res.IPAny = !val
	}
	if banexg.IsContract(e.MarketType) {
		if val, ok := banexg.BoolFromInfo(info, "enableFutures"); ok {
			res.TradeKnown = true
			res.TradeAllowed = val
		}
	}
	if !res.TradeKnown {
		if val, ok := banexg.BoolFromInfo(info, "enableSpotAndMarginTrading", "enableTrading", "canTrade", "tradeEnabled"); ok {
			res.TradeKnown = true
			res.TradeAllowed = val
		}
	}
	if val, ok := banexg.BoolFromInfo(info, "enableWithdrawals", "canWithdraw", "withdrawEnable", "withdrawEnabled"); ok {
		res.WithdrawKnown = true
		res.WithdrawAllowed = val
	}
	if banexg.IsContract(e.MarketType) && res.PosMode == "" {
		method := ""
		switch e.MarketType {
		case banexg.MarketLinear, banexg.MarketSwap, banexg.MarketFuture:
			method = MethodFapiPrivateGetPositionSideDual
		case banexg.MarketInverse:
			method = MethodDapiPrivateGetPositionSideDual
		}
		if method != "" {
			rsp, err := e.Call(method, args)
			if err == nil {
				var raw map[string]interface{}
				info, err := utils.UnmarshalStringMap(rsp.Content, &raw)
				if err == nil {
					if val, ok := banexg.BoolFromInfo(info, "dualSidePosition"); ok {
						res.PosMode = banexg.PosModeFromBool(val)
					}
				}
			}
		}
	}
	return res, nil
}
