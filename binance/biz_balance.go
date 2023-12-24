package binance

import (
	"context"
	"fmt"
	"github.com/anyongjin/banexg"
	"github.com/anyongjin/banexg/utils"
	"github.com/bytedance/sonic"
	"strconv"
	"strings"
)

/*
query for balance and get the amount of funds available for trading or funds locked in orders
:see: https://binance-docs.github.io/apidocs/spot/en/#account-information-user_data                  # spot
:see: https://binance-docs.github.io/apidocs/spot/en/#query-cross-margin-account-details-user_data   # cross margin
:see: https://binance-docs.github.io/apidocs/spot/en/#query-isolated-margin-account-info-user_data   # isolated margin
:see: https://binance-docs.github.io/apidocs/spot/en/#lending-account-user_data                      # lending
:see: https://binance-docs.github.io/apidocs/spot/en/#funding-wallet-user_data                       # funding
:see: https://binance-docs.github.io/apidocs/futures/en/#account-information-v2-user_data            # swap
:see: https://binance-docs.github.io/apidocs/delivery/en/#account-information-user_data              # future
:see: https://binance-docs.github.io/apidocs/voptions/en/#option-account-information-trade           # option
:param dict [params]: extra parameters specific to the exchange API endpoint
:param str [params.market]: 'spot', 'future', 'swap', 'funding', or 'spot'
:param str [params.marginMode]: 'cross' or 'isolated', for margin trading, uses self.options.defaultMarginMode if not passed, defaults to None/None/None
:param str[]|None [params.symbols]: unified market symbols, only used in isolated margin mode
:returns dict: a `balance structure <https://docs.ccxt.com/#/?id=balance-structure>`
*/
func (e *Binance) FetchBalance(params *map[string]interface{}) (*banexg.Balances, error) {
	_, err := e.LoadMarkets(false, nil)
	if err != nil {
		return nil, fmt.Errorf("load markets fail: %v", err)
	}
	var args = utils.SafeParams(params)
	marketType, marketInverse := e.GetArgsMarketType(args, "")
	marginMode := utils.PopMapVal(args, "marginMode", "")
	method := "privateGetAccount"
	marketType = e.StdMarketType(marketType, marketInverse)
	if marketType == banexg.MarketLinear {
		method = "fapiPrivateV2GetAccount"
	} else if marketType == banexg.MarketInverse {
		method = "dapiPrivateGetAccount"
	} else if marginMode == "isolated" {
		method = "sapiGetMarginIsolatedAccount"
		symbols := utils.GetMapVal(args, "symbols", []string{})
		if len(symbols) > 0 {
			b := strings.Builder{}
			notFirst := false
			for _, s := range symbols {
				mid, err := e.GetMarketID(s)
				if err != nil {
					return nil, fmt.Errorf("symbol invalid %s", s)
				}
				if notFirst {
					b.WriteString(",")
					notFirst = true
				}
				b.WriteString(mid)
			}
			args["symbols"] = b.String()
		}
	} else if marketType == banexg.MarketMargin || marginMode == banexg.MarginCross {
		method = "sapiGetMarginAccount"
	} else if marketType == "funding" {
		method = "sapiPostAssetGetFundingAsset"
	}
	rsp := e.RequestApi(context.Background(), method, &args)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	switch method {
	case "privateGetAccount":
		return parseSpotBalances(e, rsp)
	case "sapiGetMarginAccount":
		return parseMarginCrossBalances(e, rsp)
	case "sapiGetMarginIsolatedAccount":
		return parseMarginIsolatedBalances(e, rsp)
	case "fapiPrivateV2GetAccount":
		return parseSwapBalances(e, rsp)
	case "dapiPrivateGetAccount":
		return parseInverseBalances(e, rsp)
	case "sapiPostAssetGetFundingAsset":
		return parseFundingBalances(e, rsp)
	default:
		return nil, fmt.Errorf("unsupport parse balance method: %s", method)
	}
}

func unmarshalBalance(content string, data interface{}) (*banexg.Balances, error) {
	err := sonic.UnmarshalString(content, data)
	if err != nil {
		return nil, fmt.Errorf("unmarshal fail: %v", err)
	}
	var result = banexg.Balances{
		Info:   data,
		Assets: map[string]*banexg.Asset{},
	}
	return &result, nil
}

func parseSpotBalances(e *Binance, rsp *banexg.HttpRes) (*banexg.Balances, error) {
	var data = SpotAccount{}
	result, err := unmarshalBalance(rsp.Content, &data)
	if err != nil {
		return nil, err
	}
	result.TimeStamp = data.UpdateTime
	for _, item := range data.Balances {
		asset := item.ToStdAsset(e.Exchange)
		if asset.IsEmpty() {
			continue
		}
		result.Assets[asset.Code] = asset
	}
	return result.Init(), nil
}

func parseMarginCrossBalances(e *Binance, rsp *banexg.HttpRes) (*banexg.Balances, error) {
	var data = MarginCrossBalances{}
	result, err := unmarshalBalance(rsp.Content, &data)
	if err != nil {
		return nil, err
	}
	for _, item := range data.UserAssets {
		asset := item.ToStdAsset(e.Exchange)
		if asset.IsEmpty() {
			continue
		}
		result.Assets[asset.Code] = asset
	}
	return result.Init(), nil
}

func parseMarginIsolatedBalances(e *Binance, rsp *banexg.HttpRes) (*banexg.Balances, error) {
	var data = IsolatedBalances{}
	result, err := unmarshalBalance(rsp.Content, &data)
	if err != nil {
		return nil, err
	}
	for _, item := range data.Assets {
		symbol := e.SafeSymbol(item.Symbol, "", banexg.MarketMargin)
		itemRes := make(map[string]*banexg.Asset)
		if item.BaseAsset != nil {
			asset := item.BaseAsset.ToStdAsset(e.Exchange)
			if asset.IsEmpty() {
				continue
			}
			itemRes[asset.Code] = asset
		}
		if item.QuoteAsset != nil {
			asset := item.QuoteAsset.ToStdAsset(e.Exchange)
			if asset.IsEmpty() {
				continue
			}
			itemRes[asset.Code] = asset
		}
		result.IsolatedAssets[symbol] = itemRes
	}
	return result.Init(), nil
}

func parseSwapBalances(e *Binance, rsp *banexg.HttpRes) (*banexg.Balances, error) {
	var data = SwapBalances{}
	result, err := unmarshalBalance(rsp.Content, &data)
	if err != nil {
		return nil, err
	}
	for _, item := range data.Assets {
		asset := item.ToStdAsset(e.Exchange)
		if asset.IsEmpty() {
			continue
		}
		result.Assets[asset.Code] = asset
	}
	return result.Init(), nil
}

func parseInverseBalances(e *Binance, rsp *banexg.HttpRes) (*banexg.Balances, error) {
	var data = InverseBalances{}
	result, err := unmarshalBalance(rsp.Content, &data)
	if err != nil {
		return nil, err
	}
	for _, item := range data.Assets {
		asset := item.ToStdAsset(e.Exchange)
		if asset.IsEmpty() {
			continue
		}
		result.Assets[asset.Code] = asset
	}
	return result.Init(), nil
}

func parseFundingBalances(e *Binance, rsp *banexg.HttpRes) (*banexg.Balances, error) {
	var data = make([]*FundingAsset, 0)
	result, err := unmarshalBalance(rsp.Content, &data)
	if err != nil {
		return nil, err
	}
	for _, item := range data {
		code := e.SafeCurrencyCode(item.Asset)
		free, _ := strconv.ParseFloat(item.Free, 64)
		freeze, _ := strconv.ParseFloat(item.Freeze, 64)
		withdraw, _ := strconv.ParseFloat(item.Withdrawing, 64)
		lock, _ := strconv.ParseFloat(item.Locked, 64)
		asset := banexg.Asset{
			Code: code,
			Free: free,
			Used: freeze + withdraw + lock,
		}
		if asset.IsEmpty() {
			continue
		}
		result.Assets[code] = &asset
	}
	return result.Init(), nil
}

func (a BnbAsset) ToStdAsset(e *banexg.Exchange) *banexg.Asset {
	free, _ := strconv.ParseFloat(a.Free, 64)
	lock, _ := strconv.ParseFloat(a.Locked, 64)
	borr, _ := strconv.ParseFloat(a.Borrowed, 64)
	inst, _ := strconv.ParseFloat(a.Interest, 64)
	code := e.SafeCurrencyCode(a.Asset)
	return &banexg.Asset{
		Code:  code,
		Free:  free,
		Used:  lock,
		Total: lock + free,
		Debt:  borr + inst,
	}
}

func (a *FutureAsset) ToStdAsset(e *banexg.Exchange) *banexg.Asset {
	code := e.SafeCurrencyCode(a.Asset)
	free, _ := strconv.ParseFloat(a.AvailableBalance, 64)
	used, _ := strconv.ParseFloat(a.InitialMargin, 64)
	total, _ := strconv.ParseFloat(a.MarginBalance, 64)
	return &banexg.Asset{
		Code:  code,
		Free:  free,
		Used:  used,
		Total: total,
	}
}
