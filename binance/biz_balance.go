package binance

import (
	"context"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"go.uber.org/zap"
	"math"
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
func (e *Binance) FetchBalance(params map[string]interface{}) (*banexg.Balances, *errs.Error) {
	args := utils.SafeParams(params)
	marketType, _, err := e.LoadArgsMarketType(args)
	if err != nil {
		return nil, err
	}
	marginMode := utils.PopMapVal(args, banexg.ParamMarginMode, "")
	method := "privateGetAccount"
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
					return nil, err
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
	tryNum := e.GetRetryNum("FetchBalance", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	getCurrCode := func(currId string) string {
		return e.SafeCurrencyCode(currId)
	}
	switch method {
	case "privateGetAccount":
		return parseSpotBalances(getCurrCode, rsp)
	case "sapiGetMarginAccount":
		return parseMarginCrossBalances(getCurrCode, rsp)
	case "sapiGetMarginIsolatedAccount":
		return parseMarginIsolatedBalances(e, rsp)
	case "fapiPrivateV2GetAccount":
		return parseLinearBalances(getCurrCode, rsp)
	case "dapiPrivateGetAccount":
		return parseInverseBalances(getCurrCode, rsp)
	case "sapiPostAssetGetFundingAsset":
		return parseFundingBalances(e, rsp)
	default:
		return nil, errs.NewMsg(errs.CodeNotSupport, "unsupport parse balance method: %s", method)
	}
}

/*
FetchPositions get 'positionRisk' or 'account' positions (by banexg.OptPositionMethod)
*/
func (e *Binance) FetchPositions(symbols []string, params map[string]interface{}) ([]*banexg.Position, *errs.Error) {
	args := utils.SafeParams(params)
	method := utils.GetMapVal(e.Options, banexg.OptPositionMethod, "positionRisk")
	method = utils.PopMapVal(args, banexg.ParamMethod, method)
	if method == "positionRisk" {
		return e.FetchPositionsRisk(symbols, params)
	} else if method == "account" {
		return e.FetchAccountPositions(symbols, params)
	} else {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "%s for FetchPositions is invalid, choose: positionRisk/account", method)
	}
}

func (e *Binance) FetchPositionsRisk(symbols []string, params map[string]interface{}) ([]*banexg.Position, *errs.Error) {
	args := utils.SafeParams(params)
	marketType, _, err := e.LoadArgsMarketType(args, symbols...)
	if err != nil {
		return nil, err
	}
	var method string
	if marketType == banexg.MarketLinear {
		method = "fapiPrivateV2GetPositionRisk"
	} else if marketType == banexg.MarketInverse {
		method = "dapiPrivateGetPositionRisk"
	} else {
		return nil, errs.NewMsg(errs.CodeInvalidRequest, "FetchPositionsRisk support linear/inverse contracts only")
	}
	err = e.LoadLeverageBrackets(false, params)
	if err != nil {
		return nil, err
	}
	retryNum := e.GetRetryNum("FetchPositionsRisk", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, retryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	if marketType == banexg.MarketLinear {
		return parsePositionRisk[*LinearPositionRisk](e, rsp)
	} else {
		return parsePositionRisk[*InversePositionRisk](e, rsp)
	}
}

/*
FetchAccountPositions

	:see: https://binance-docs.github.io/apidocs/futures/en/#account-information-v2-user_data
	:see: https://binance-docs.github.io/apidocs/delivery/en/#account-information-user_data
	:param str[]|None symbols: list of unified market symbols
	:param dict [params]: extra parameters specific to the exchange API endpoint
	:returns dict: data on account positions
*/
func (e *Binance) FetchAccountPositions(symbols []string, params map[string]interface{}) ([]*banexg.Position, *errs.Error) {
	args := utils.SafeParams(params)
	marketType, _, err := e.LoadArgsMarketType(args, symbols...)
	if err != nil {
		return nil, err
	}
	var method string
	if marketType == banexg.MarketLinear {
		method = "fapiPrivateV2GetAccount"
	} else if marketType == banexg.MarketInverse {
		method = "dapiPrivateGetAccount"
	} else {
		return nil, errs.NewMsg(errs.CodeInvalidRequest, "FetchAccountPositions support linear/inverse contracts only")
	}
	err = e.LoadLeverageBrackets(false, params)
	if err != nil {
		return nil, err
	}
	retryNum := e.GetRetryNum("FetchAccountPositions", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, retryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	return parseAccPosition(e, rsp, marketType)
}

func (e *Binance) FetchIncomeHistory(inType string, symbol string, since int64, limit int, params map[string]interface{}) ([]*banexg.Income, *errs.Error) {
	args := utils.SafeParams(params)
	var marketType string
	var err *errs.Error
	if symbol != "" {
		market, err := e.GetMarket(symbol)
		if err != nil {
			return nil, err
		}
		if !market.Swap {
			return nil, errs.NewMsg(errs.CodeUnsupportMarket, "FetchIncomeHistory support swap market only")
		}
		args["symbol"] = market.ID
		marketType = market.Type
	} else {
		marketType, _, err = e.LoadArgsMarketType(args)
		if err != nil {
			return nil, err
		}
	}
	if !banexg.IsContract(marketType) {
		return nil, errs.NewMsg(errs.CodeUnsupportMarket, "FetchIncomeHistory support future market only")
	}
	args["startTime"] = since
	args["limit"] = limit
	args["incomeType"] = inType
	var method string
	if marketType == banexg.MarketLinear {
		method = "fapiPrivateGetIncome"
	} else if marketType == banexg.MarketInverse {
		method = "dapiPrivateGetIncome"
	} else {
		return nil, errs.NewMsg(errs.CodeUnsupportMarket, "FetchIncomeHistory not support: "+marketType)
	}
	tryNum := e.GetRetryNum("FetchIncomeHistory", 1)
	rsp := e.RequestApiRetry(context.Background(), method, args, tryNum)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	var data = make([]*Income, 0)
	err_ := utils.UnmarshalString(rsp.Content, &data)
	if err_ != nil {
		return nil, errs.New(errs.CodeUnmarshalFail, err_)
	}
	var res = make([]*banexg.Income, 0, len(data))
	for _, it := range data {
		market := e.GetMarketById(it.Symbol, marketType)
		if market == nil {
			log.Warn("no symbol for", zap.String("code", it.Symbol))
			continue
		}
		income, _ := strconv.ParseFloat(it.Income, 64)
		res = append(res, &banexg.Income{
			Symbol:     market.Symbol,
			IncomeType: it.IncomeType,
			Income:     income,
			Asset:      e.SafeCurrencyCode(it.Asset),
			Info:       it.Info,
			Time:       it.Time,
			TranID:     strconv.FormatInt(it.TranID, 10),
			TradeID:    it.TradeID,
		})
	}
	return res, nil
}

func parseAccPosition(e *Binance, rsp *banexg.HttpRes, marketType string) ([]*banexg.Position, *errs.Error) {
	assets := make(map[string]*FutureAsset)
	var posList = make([]IAccPosition, 0)
	if marketType == banexg.MarketLinear {
		var res = LinearAccPositions{}
		err := utils.UnmarshalString(rsp.Content, &res)
		if err != nil {
			return nil, errs.New(errs.CodeUnmarshalFail, err)
		}
		for _, a := range res.Assets {
			assets[e.SafeCurrencyCode(a.Asset)] = &a.FutureAsset
		}
		for _, p := range res.Positions {
			posList = append(posList, p)
		}
	} else {
		var res = InverseAccPositions{}
		err := utils.UnmarshalString(rsp.Content, &res)
		if err != nil {
			return nil, errs.New(errs.CodeUnmarshalFail, err)
		}
		for _, a := range res.Assets {
			assets[e.SafeCurrencyCode(a.Asset)] = a
		}
		for _, p := range res.Positions {
			posList = append(posList, p)
		}
	}
	isLinear := marketType == banexg.MarketLinear
	var result = make([]*banexg.Position, 0)
	for _, p := range posList {
		futPos := p.GetFutPosition()
		market := e.GetMarketById(futPos.Symbol, marketType)
		if market == nil {
			continue
		}
		code := market.Quote
		if !isLinear {
			code = market.Base
		}
		asset, ok := assets[code]
		if !ok {
			continue
		}
		pos, err2 := parseAccountPosition(e, futPos, asset, market, p.GetNotional())
		if err2 != nil {
			return nil, err2
		}
		if pos == nil {
			continue
		}
		pos.Info = p
		result = append(result, pos)
	}
	return result, nil
}

func parseAccountPosition(e *Binance, p *FuturePosition, a *FutureAsset, market *banexg.Market, notional string) (*banexg.Position, *errs.Error) {
	res := p.BaseContPosition.ToStdPos()
	if res == nil {
		return nil, nil
	}
	res.Symbol = market.Symbol
	res.ContractSize = market.ContractSize
	initMargin, _ := strconv.ParseFloat(p.InitialMargin, 64)
	res.InitialMargin = initMargin
	res.InitialMarginPct = 1 / float64(res.Leverage)
	maintMargin, _ := strconv.ParseFloat(p.MaintMargin, 64)
	res.MaintMargin = maintMargin
	// 名义价值
	notionalVal, _ := strconv.ParseFloat(notional, 64)
	res.Notional = math.Abs(notionalVal)
	// 维持保证金比率
	maintMarginPct := e.GetMaintMarginPct(res.Symbol, res.Notional)
	res.MaintMarginPct = maintMarginPct
	// 计算当前保证金collateral
	var marginMode string
	var walletBalance float64
	var collateral float64
	if p.Isolated {
		marginMode = banexg.MarginIsolated
		walletBalance, _ = strconv.ParseFloat(p.IsolatedWallet, 64)
		collateral = walletBalance + res.UnrealizedPnl
	} else {
		marginMode = banexg.MarginCross
		walletBalance, _ = strconv.ParseFloat(a.CrossWalletBalance, 64)
		unPnl, _ := strconv.ParseFloat(a.CrossUnPnl, 64)
		collateral = walletBalance + unPnl
	}
	res.MarginMode = marginMode
	res.Collateral = collateral
	// 计算marginRatio
	marginRatio, _ := utils.PrecFloat64(maintMargin/collateral, 4, true, 0)
	res.MarginRatio = marginRatio
	// percentage
	percentage, _ := utils.PrecFloat64(res.UnrealizedPnl*100/initMargin, 2, true, 0)
	res.Percentage = percentage
	// 计算强平价格
	isShort := res.Side == banexg.PosSideShort
	entryPriceSign := res.EntryPrice
	revtMaintMarginPct := float64(0)
	prec := market.Precision
	if market.Type == banexg.MarketLinear {
		// liquidationPrice = (walletBalance / (contracts * (±1 + mmp))) + (±entryPrice / (±1 + mmp))
		// mmp = maintenanceMarginPercentage
		// where ± is negative for long and positive for short
		if isShort {
			revtMaintMarginPct = 1 + maintMarginPct
		} else {
			revtMaintMarginPct = -1 + maintMarginPct
			entryPriceSign *= -1
		}
		leftSide := walletBalance / (res.Contracts * revtMaintMarginPct)
		rightSide := entryPriceSign / revtMaintMarginPct
		liquidationPrice, _ := utils.PrecFloat64(leftSide+rightSide, prec.Price, true, prec.ModePrice)
		res.LiquidationPrice = liquidationPrice
	} else {
		// liquidationPrice = (contracts * contractSize(±1 - mmp)) / (±1/entryPrice * contracts * contractSize - walletBalance)
		if isShort {
			revtMaintMarginPct = 1 - maintMarginPct
		} else {
			revtMaintMarginPct = -1 - maintMarginPct
			entryPriceSign *= -1
		}
		size := res.Contracts * res.ContractSize
		leftSide := size * revtMaintMarginPct
		rightSide := size/entryPriceSign - walletBalance
		liquidationPrice, _ := utils.PrecFloat64(leftSide/rightSide, prec.Price, true, prec.ModePrice)
		res.LiquidationPrice = liquidationPrice
	}
	res.Hedged = res.Side != banexg.PosSideBoth
	return res, nil
}

func (p *LinearAccountPosition) GetFutPosition() *FuturePosition {
	return &p.FuturePosition
}

func (p *LinearAccountPosition) GetNotional() string {
	return p.Notional
}

func (p *InversePosition) GetFutPosition() *FuturePosition {
	return &p.FuturePosition
}

func (p *InversePosition) GetNotional() string {
	return p.NotionalValue
}

func parsePositionRisk[T IBnbPosRisk](e *Binance, rsp *banexg.HttpRes) ([]*banexg.Position, *errs.Error) {
	var data = make([]T, 0)
	// fmt.Println(rsp.Content)
	err := utils.UnmarshalString(rsp.Content, &data)
	if err != nil {
		return nil, errs.New(errs.CodeUnmarshalFail, err)
	}
	var result = make([]*banexg.Position, 0)
	for _, item := range data {
		pos, err2 := item.ToStdPos(e)
		if err2 != nil {
			return nil, err2
		}
		if pos == nil {
			continue
		}
		result = append(result, pos)
	}
	return result, nil
}

func (p *BaseContPosition) ToStdPos() *banexg.Position {
	leverage, _ := strconv.Atoi(p.Leverage)
	entryPrice, _ := strconv.ParseFloat(p.EntryPrice, 64)
	posAmt, _ := strconv.ParseFloat(p.PositionAmt, 64)
	if posAmt == 0 && entryPrice == 0 {
		// 数量价格都为0，认为是无效的
		return nil
	}
	unp, _ := strconv.ParseFloat(p.UnRealizedProfit, 64)
	side := strings.ToLower(p.PositionSide)
	if side != banexg.PosSideLong && side != banexg.PosSideShort {
		if posAmt > 0 {
			side = banexg.PosSideLong
		} else if posAmt < 0 {
			side = banexg.PosSideShort
		}
	}
	var pos = banexg.Position{
		TimeStamp:     p.UpdateTime,
		Leverage:      leverage,
		EntryPrice:    entryPrice,
		Contracts:     math.Abs(posAmt),
		UnrealizedPnl: unp,
		Side:          side,
	}
	return &pos
}

func (p *ContPositionRisk) ToStdPos() *banexg.Position {
	var res = p.BaseContPosition.ToStdPos()
	if res == nil {
		return nil
	}
	res.MarginMode = strings.ToLower(p.MarginType)
	liqdPrice, _ := strconv.ParseFloat(p.LiquidationPrice, 64)
	markPrice, _ := strconv.ParseFloat(p.MarkPrice, 64)
	res.LiquidationPrice = liqdPrice
	res.MarkPrice = markPrice
	return res
}

func (p *LinearPositionRisk) ToStdPos(e *Binance) (*banexg.Position, *errs.Error) {
	var res = p.ContPositionRisk.ToStdPos()
	if res == nil {
		return nil, nil
	}
	res.TimeStamp = p.UpdateTime
	// 名义价值
	notional, _ := strconv.ParseFloat(p.Notional, 64)
	res.Notional = notional
	res.Info = p
	market := e.GetMarketById(p.Symbol, banexg.MarketLinear)
	if market == nil {
		return nil, errs.NewMsg(errs.CodeNoMarketForPair, "no market for %s, total %d", p.Symbol, len(e.Markets))
	}
	return calcPositionRisk(res, e, market, p.IsolatedMargin)
}

func (p *InversePositionRisk) ToStdPos(e *Binance) (*banexg.Position, *errs.Error) {
	var res = p.ContPositionRisk.ToStdPos()
	if res == nil {
		return nil, nil
	}
	res.TimeStamp = p.UpdateTime
	// 名义价值
	notional, _ := strconv.ParseFloat(p.NotionalValue, 64)
	res.Notional = notional
	res.Info = p
	market := e.GetMarketById(p.Symbol, banexg.MarketInverse)
	if market == nil {
		return nil, errs.NewMsg(errs.CodeNoMarketForPair, "no market for %s, total %d", p.Symbol, len(e.Markets))
	}
	return calcPositionRisk(res, e, market, p.IsolatedMargin)
}

func calcPositionRisk(res *banexg.Position, e *Binance, market *banexg.Market, isolatedMarginStr string) (*banexg.Position, *errs.Error) {
	res.Symbol = market.Symbol
	res.ContractSize = market.ContractSize
	// 当前保证金
	var collateral float64
	// 名义价值
	notional := math.Abs(res.Notional)
	// 维持保证金比率
	maintMarginPct := e.GetMaintMarginPct(res.Symbol, notional)
	if res.MarginMode == banexg.MarginCross {
		// 全仓模式，计算保证金
		revMaintPct := float64(0)
		entryPriceSign := res.EntryPrice
		mmpSign := -1.0
		if market.Type == banexg.MarketLinear {
			mmpSign = 1.0
		}
		if res.Side == banexg.PosSideShort {
			revMaintPct = 1.0 + maintMarginPct*mmpSign
			entryPriceSign = -1 * entryPriceSign
		} else {
			revMaintPct = -1.0 + maintMarginPct*mmpSign
		}
		var precVal float64
		var mode int
		var prec = market.Precision
		if market.Type == banexg.MarketLinear {
			// walletBalance = (liquidationPrice * (±1 + mmp) ± entryPrice) * contracts
			leftSide := res.LiquidationPrice*revMaintPct + entryPriceSign
			if prec.Quote != 0 {
				precVal = prec.Quote
				mode = prec.ModeQuote
			} else {
				precVal = prec.Price
				mode = prec.ModePrice
			}
			collateral = leftSide * res.Contracts
		} else {
			entryPriceSign *= -1
			// walletBalance = (contracts * contractSize) * (±1/entryPrice - (±1 - mmp) / liquidationPrice)
			leftSide := res.Contracts * res.ContractSize
			rightSide := (1.0 / entryPriceSign) - (revMaintPct / res.LiquidationPrice)
			precVal = prec.Base
			mode = prec.ModeBase
			collateral = leftSide * rightSide
		}
		if precVal != 0 {
			var err error
			collateral, err = utils.PrecFloat64(collateral, precVal, false, mode)
			if err != nil {
				return nil, errs.New(errs.CodePrecDecFail, err)
			}
		}
	} else {
		collateral, _ = strconv.ParseFloat(isolatedMarginStr, 64)
	}
	// 计算initMargin
	initMargin, _ := utils.PrecFloat64(notional/float64(res.Leverage), 8, true, 0)
	maintMargin, _ := utils.PrecFloat64(maintMarginPct*notional, 11, true, 0)
	marginRatio, _ := utils.PrecFloat64(maintMargin/collateral, 4, true, 0)
	percentage, _ := utils.PrecFloat64(res.UnrealizedPnl*100/initMargin, 2, true, 0)
	res.Collateral = collateral
	res.Hedged = res.Side != banexg.PosSideBoth
	res.Notional = notional
	res.InitialMargin = initMargin
	res.InitialMarginPct = 1 / float64(res.Leverage)
	res.MaintMarginPct = maintMarginPct
	res.MaintMargin = maintMargin
	res.MarginRatio = marginRatio
	res.Percentage = percentage
	return res, nil
}

func unmarshalBalance(content string, data interface{}) (*banexg.Balances, *errs.Error) {
	err := utils.UnmarshalString(content, data)
	if err != nil {
		return nil, errs.New(errs.CodeUnmarshalFail, err)
	}
	var result = banexg.Balances{
		Info:   data,
		Assets: map[string]*banexg.Asset{},
	}
	return &result, nil
}

func parseSpotBalances(getCurrCode func(string) string, rsp *banexg.HttpRes) (*banexg.Balances, *errs.Error) {
	var data = SpotAccount{}
	result, err := unmarshalBalance(rsp.Content, &data)
	if err != nil {
		return nil, err
	}
	result.TimeStamp = data.UpdateTime
	for _, item := range data.Balances {
		asset := item.ToStdAsset(getCurrCode)
		if asset.IsEmpty() {
			continue
		}
		result.Assets[asset.Code] = asset
	}
	return result.Init(), nil
}

func parseMarginCrossBalances(getCurrCode func(string) string, rsp *banexg.HttpRes) (*banexg.Balances, *errs.Error) {
	var data = MarginCrossBalances{}
	result, err := unmarshalBalance(rsp.Content, &data)
	if err != nil {
		return nil, err
	}
	for _, item := range data.UserAssets {
		asset := item.ToStdAsset(getCurrCode)
		if asset.IsEmpty() {
			continue
		}
		result.Assets[asset.Code] = asset
	}
	return result.Init(), nil
}

func parseMarginIsolatedBalances(e *Binance, rsp *banexg.HttpRes) (*banexg.Balances, *errs.Error) {
	var data = IsolatedBalances{}
	result, err := unmarshalBalance(rsp.Content, &data)
	if err != nil {
		return nil, err
	}
	getCurrCode := func(currId string) string {
		return e.SafeCurrencyCode(currId)
	}
	for _, item := range data.Assets {
		symbol := e.SafeSymbol(item.Symbol, "", banexg.MarketMargin)
		if symbol == "" {
			continue
		}
		itemRes := make(map[string]*banexg.Asset)
		if item.BaseAsset != nil {
			asset := item.BaseAsset.ToStdAsset(getCurrCode)
			if asset.IsEmpty() {
				continue
			}
			itemRes[asset.Code] = asset
		}
		if item.QuoteAsset != nil {
			asset := item.QuoteAsset.ToStdAsset(getCurrCode)
			if asset.IsEmpty() {
				continue
			}
			itemRes[asset.Code] = asset
		}
		result.IsolatedAssets[symbol] = itemRes
	}
	return result.Init(), nil
}

func parseLinearBalances(getCurrCode func(string) string, rsp *banexg.HttpRes) (*banexg.Balances, *errs.Error) {
	var data = LinearBalances{}
	result, err := unmarshalBalance(rsp.Content, &data)
	if err != nil {
		return nil, err
	}
	for _, item := range data.Assets {
		asset := item.ToStdAsset(getCurrCode)
		if asset.IsEmpty() {
			continue
		}
		result.Assets[asset.Code] = asset
	}
	return result.Init(), nil
}

func parseInverseBalances(getCurrCode func(string) string, rsp *banexg.HttpRes) (*banexg.Balances, *errs.Error) {
	var data = InverseBalances{}
	result, err := unmarshalBalance(rsp.Content, &data)
	if err != nil {
		return nil, err
	}
	for _, item := range data.Assets {
		asset := item.ToStdAsset(getCurrCode)
		if asset.IsEmpty() {
			continue
		}
		result.Assets[asset.Code] = asset
	}
	return result.Init(), nil
}

func parseFundingBalances(e *Binance, rsp *banexg.HttpRes) (*banexg.Balances, *errs.Error) {
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

func (a SpotAsset) ToStdAsset(getCurrCode func(string) string) *banexg.Asset {
	free, _ := strconv.ParseFloat(a.Free, 64)
	lock, _ := strconv.ParseFloat(a.Locked, 64)
	borr, _ := strconv.ParseFloat(a.Borrowed, 64)
	inst, _ := strconv.ParseFloat(a.Interest, 64)
	code := getCurrCode(a.Asset)
	return &banexg.Asset{
		Code:  code,
		Free:  free,
		Used:  lock,
		Total: lock + free,
		Debt:  borr + inst,
	}
}

func (a *FutureAsset) ToStdAsset(getCurrCode func(string) string) *banexg.Asset {
	code := getCurrCode(a.Asset)
	used, _ := strconv.ParseFloat(a.InitialMargin, 64)
	total, _ := strconv.ParseFloat(a.WalletBalance, 64)
	uPol, _ := strconv.ParseFloat(a.UnrealizedProfit, 64)
	free := total - used + uPol
	return &banexg.Asset{
		Code:  code,
		Free:  free,
		Used:  used,
		Total: total,
		UPol:  uPol,
	}
}
