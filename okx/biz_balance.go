package okx

import (
	"strings"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

func (e *OKX) FetchBalance(params map[string]interface{}) (*banexg.Balances, *errs.Error) {
	args := utils.SafeParams(params)
	if ccy := utils.PopMapVal(args, banexg.ParamCurrency, ""); ccy != "" {
		args[FldCcy] = ccy
	}
	tryNum := e.GetRetryNum("FetchBalance", 1)
	res := requestRetry[[]map[string]interface{}](e, MethodAccountGetBalance, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	if len(res.Result) == 0 {
		return nil, errs.NewMsg(errs.CodeDataNotFound, "empty balance result")
	}
	arr, err := decodeResult[Balance](res.Result)
	if err != nil {
		return nil, err
	}
	return parseBalance(e, &arr[0], res.Result[0]), nil
}

func (e *OKX) FetchPositions(symbols []string, params map[string]interface{}) ([]*banexg.Position, *errs.Error) {
	args := utils.SafeParams(params)
	marketType, contractType, err := e.LoadArgsMarketType(args, symbols...)
	if err != nil {
		return nil, err
	}
	if len(symbols) > 0 {
		ids := make([]string, 0, len(symbols))
		for _, sym := range symbols {
			id, err := e.GetMarketID(sym)
			if err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		args[FldInstId] = strings.Join(ids, ",")
	} else {
		instType := instTypeByMarket(marketType, contractType)
		if instType == "" || instType == InstTypeSpot {
			return nil, errs.NewMsg(errs.CodeNotSupport, "FetchPositions only supports margin/derivatives")
		}
		args[FldInstType] = instType
	}
	tryNum := e.GetRetryNum("FetchPositions", 1)
	res := requestRetry[[]map[string]interface{}](e, MethodAccountGetPositions, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	return parsePositions(e, res.Result, nil)
}

func (e *OKX) FetchAccountPositions(symbols []string, params map[string]interface{}) ([]*banexg.Position, *errs.Error) {
	args := utils.SafeParams(params)
	if len(symbols) > 0 {
		_, err := e.LoadMarkets(false, nil)
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(symbols))
		for _, sym := range symbols {
			id, err := e.GetMarketID(sym)
			if err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		args[FldInstId] = strings.Join(ids, ",")
	} else {
		marketType, contractType, err := e.LoadArgsMarketType(args)
		if err != nil {
			return nil, err
		}
		if instType := instTypeByMarket(marketType, contractType); instType != "" && instType != InstTypeSpot {
			args[FldInstType] = instType
		}
	}
	tryNum := e.GetRetryNum("FetchAccountPositions", 1)
	res := requestRetry[[]map[string]interface{}](e, MethodAccountGetPositions, args, tryNum)
	if res.Error != nil {
		return nil, res.Error
	}
	var symbolSet map[string]struct{}
	if len(symbols) > 0 {
		symbolSet = make(map[string]struct{}, len(symbols))
		for _, sym := range symbols {
			symbolSet[sym] = struct{}{}
		}
	}
	return parsePositions(e, res.Result, symbolSet)
}

func parsePositions(e *OKX, items []map[string]interface{}, symbols map[string]struct{}) ([]*banexg.Position, *errs.Error) {
	arr, err := decodeResult[Position](items)
	if err != nil {
		return nil, err
	}
	result := make([]*banexg.Position, 0, len(arr))
	for i, item := range arr {
		pos := parsePosition(e, &item, items[i])
		if pos == nil {
			continue
		}
		if symbols != nil {
			if _, ok := symbols[pos.Symbol]; !ok {
				continue
			}
		}
		result = append(result, pos)
	}
	return result, nil
}

func parseBalance(e *OKX, bal *Balance, info map[string]interface{}) *banexg.Balances {
	if bal == nil {
		return nil
	}
	res := &banexg.Balances{
		TimeStamp: parseInt(bal.UTime),
		Assets:    map[string]*banexg.Asset{},
		Info:      info,
	}
	for _, d := range bal.Details {
		code := e.SafeCurrencyCode(d.Ccy)
		free := parseFloat(d.AvailBal)
		used := parseFloat(d.FrozenBal)
		total := parseFloat(d.Eq)
		if total == 0 {
			total = parseFloat(d.CashBal)
		}
		res.Assets[code] = &banexg.Asset{
			Code:  code,
			Free:  free,
			Used:  used,
			Total: total,
		}
	}
	return res.Init()
}
