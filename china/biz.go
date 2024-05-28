package china

import (
	_ "embed"
	"fmt"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
	"github.com/shopspring/decimal"
	"gopkg.in/yaml.v3"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

func (e *China) Init() *errs.Error {
	err := e.Exchange.Init()
	if err != nil {
		return err
	}
	e.ExgInfo.Min1mHole = 5
	return nil
}

var (
	bases        = make(map[string]*ItemMarket)
	ctMarkets    = make(map[string]*ItemMarket) // 期货品种代码对应的品种描述
	stockMarkets = make(map[string]*ItemMarket)
	ctExgs       = make(map[string]*Exchange)
	lockMars     = sync.Mutex{}
)

//go:embed markets.yml
var marketsData []byte

func loadRawMarkets() *errs.Error {
	if len(ctMarkets) > 0 {
		return nil
	}
	lockMars.Lock()
	defer lockMars.Unlock()
	if len(ctMarkets) > 0 {
		return nil
	}
	cfg := CnMarkets{}
	err := yaml.Unmarshal(marketsData, &cfg)
	if err != nil {
		return errs.New(errs.CodeUnmarshalFail, err)
	}
	for _, item := range cfg.Contracts {
		item.Resolve(bases)
		if strings.HasPrefix(item.Code, "base") {
			bases[item.Code] = item
		} else {
			key := fmt.Sprintf("%s_%s", item.Market, strings.ToUpper(item.Code))
			if item.Multiplier == 0 {
				return errs.NewMsg(errs.CodeInvalidData, "`multiplier` required: %s", key)
			}
			if item.Market == banexg.MarketLinear {
				if item.PriceTick == 0 {
					return errs.NewMsg(errs.CodeInvalidData, "`price_tick` required: %s", key)
				}
			}
			item.Fee.ParseStd()
			ctMarkets[key] = item
			if len(item.Alias) > 0 {
				for _, alias := range item.Alias {
					key = fmt.Sprintf("%s_%s", item.Market, strings.ToUpper(alias))
					ctMarkets[key] = item
				}
			}
		}
	}
	for exgName, exg := range cfg.Exchanges {
		exg.Code = exgName
	}
	ctExgs = cfg.Exchanges
	return nil
}

func (e *China) LoadMarkets(reload bool, params map[string]interface{}) (banexg.MarketMap, *errs.Error) {
	if len(e.Markets) > 0 && !reload {
		return e.Markets, nil
	}
	err := loadRawMarkets()
	if err != nil {
		return nil, err
	}
	if e.Markets == nil {
		e.Markets = make(banexg.MarketMap)
	}
	if e.MarketsById == nil {
		e.MarketsById = make(banexg.MarketArrMap)
	}
	// 加载股票列表
	for _, it := range stockMarkets {
		if it.Market == banexg.MarketSpot {
			market, err := parseMarket(it.Code, 0, false)
			if err != nil {
				return nil, err
			}
			e.Markets[it.Code] = market
			e.MarketsById[market.ID] = []*banexg.Market{market}
		}
	}
	// 期货期权代码需要传入
	var symbols []string
	if params != nil {
		val, _ := params[banexg.ParamSymbols]
		if val != nil {
			symbols, _ = val.([]string)
		}
	}
	if len(symbols) > 0 {
		for _, symbol := range symbols {
			if symbol == "" {
				continue
			}
			market, err := parseMarket(symbol, 0, false)
			if err != nil {
				return nil, err
			}
			e.Markets[symbol] = market
			e.MarketsById[market.ID] = []*banexg.Market{market}
		}
	}
	return e.Markets, nil
}

func (e *China) MapMarket(exgSID string, year int) (*banexg.Market, *errs.Error) {
	_, err := e.LoadMarkets(false, nil)
	if err != nil {
		return nil, err
	}
	mar := e.GetMarketById(exgSID, "")
	if mar != nil {
		return mar, nil
	}
	if strings.Contains(exgSID, "&") {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "combine contract is invalid")
	}
	if exgSID[0] == 'S' || exgSID[0] == 'I' {
		parts := strings.Split(exgSID, " ")
		prefix := strings.ToUpper(parts[0])
		if prefix == "SPD" || prefix == "SPC" || prefix == "SP" || prefix == "IPS" {
			return nil, errs.NewMsg(errs.CodeParamInvalid, "combine contract is invalid")
		}
	}
	mar, err = parseMarket(exgSID, year, true)
	if err != nil {
		return nil, err
	}
	e.Markets[mar.Symbol] = mar
	e.MarketsById[mar.ID] = []*banexg.Market{mar}
	return mar, nil
}

func parseMarket(symbol string, year int, isRaw bool) (*banexg.Market, *errs.Error) {
	parts := utils.SplitParts(symbol)
	if len(parts) == 0 || parts[0].Type != utils.StrStr {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "exchange symbol id must startsWith letters")
	}
	isActive, isFuture, isSwap := true, false, false
	expiry := int64(0) // 过期时间，13位毫秒
	if len(parts) > 1 && parts[1].Type == utils.StrInt {
		// 第二部分是数字，表示期货
		var curTime = time.Now()
		p1val := parts[1].Val
		if len(p1val) == 3 {
			// 至少两部分，第二部分是3个数字，改为4个数字
			p1num, _ := strconv.Atoi(p1val[1:])
			if p1num >= 1 && p1num <= 12 {
				if year == 0 {
					year = curTime.Year()
				}
				// 取到期年月的年个位
				yLast, _ := strconv.Atoi(p1val[:1])
				// 计算实际到期年
				dstYear := year/10*10 + yLast
				for dstYear < year {
					dstYear += 10
				}
				// 计算实际到期年的倒数第二个数字
				prefix := strconv.Itoa(dstYear % 100 / 10)
				parts[1].Val = prefix + p1val
				p1val = parts[1].Val
			}
		}
		if len(p1val) == 4 {
			isFuture = true
			inYearMon, _ := strconv.Atoi(p1val)
			curYear, curMon, _ := curTime.Date()
			curYearMon := curYear%100*100 + int(curMon)
			maxYearMon := curYearMon + 200 // 合约编号最长是2年，大部分1年
			monDiff := 0
			expYear := curTime.Year()/100*100 + inYearMon/100
			if inYearMon > maxYearMon {
				// 超过未来2年的期货合约ID，认为是100年前的
				monDiff = curYearMon + 10000 - inYearMon
				expYear -= 100
			} else {
				monDiff = curYearMon - inYearMon
			}
			if monDiff >= 0 {
				// 当前年月超过合约到期年月，已交割，不可交易
				isActive = false
			}
			// 计算过期时间
			expMon := time.Month(inYearMon%100 + 1)
			expDt := time.Date(expYear, expMon, 1, 0, 0, 0, 0, defTimeLoc)
			expiry = expDt.UnixMilli()
		} else if len(p1val) == 3 && (p1val == "000" || p1val == "888" || p1val == "999") {
			// 期货指数、主连
			isFuture = true
			isSwap = true
		}
	}
	market := banexg.MarketSpot
	if isFuture {
		market = banexg.MarketLinear
		if len(parts) >= 3 {
			last1, last2 := parts[len(parts)-1], parts[len(parts)-2]
			if last1.Type != utils.StrStr && (last2.Val == "P" || last2.Val == "C") {
				market = banexg.MarketOption
			}
		}
	}
	code := strings.ToUpper(parts[0].Val)
	key := fmt.Sprintf("%s_%s", market, code)
	rawMar, ok := ctMarkets[key]
	if !ok {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "symbol invalid: %s", symbol)
	}
	var exgSID, stdSymbol string
	var err *errs.Error
	if parts[0].Type == utils.StrStr {
		parts[0].Val = rawMar.Code
	}
	if isRaw {
		stdSymbol, err = rawMar.ToStdSymbol(parts)
		exgSID = symbol
	} else {
		stdSymbol = symbol
		exgSID, err = rawMar.ToRawSymbol(parts)
	}
	if err != nil {
		return nil, err
	}
	isOption := market == banexg.MarketOption
	leverage := 100 / rawMar.MarginPct
	mar := &banexg.Market{
		ID:          exgSID,
		LowercaseID: strings.ToLower(exgSID),
		Symbol:      stdSymbol,
		Base:        rawMar.Code,
		ExgReal:     rawMar.Exchange,
		Type:        market,
		Spot:        market == banexg.MarketSpot,
		Future:      isFuture,
		Swap:        isSwap,
		Combined:    isSwap,
		Option:      isOption,
		Contract:    isFuture,
		Active:      isActive,
		Linear:      isFuture && !isOption,
		Expiry:      expiry,
		FeeSide:     "quote",
		Precision: &banexg.Precision{
			Amount:     rawMar.Multiplier,
			Price:      rawMar.PriceTick,
			Base:       rawMar.Multiplier,
			Quote:      rawMar.PriceTick,
			ModeAmount: banexg.PrecModeTickSize,
			ModeBase:   banexg.PrecModeTickSize,
			ModePrice:  banexg.PrecModeTickSize,
			ModeQuote:  banexg.PrecModeTickSize,
		},
		Limits: &banexg.MarketLimits{
			Leverage: &banexg.LimitRange{
				Min: leverage,
				Max: leverage,
			},
			Amount: &banexg.LimitRange{
				Min: rawMar.Multiplier,
			},
		},
		Info: rawMar,
	}
	if len(rawMar.DayRanges) > 0 {
		mar.DayTimes, err = utils.ParseTimeRanges(rawMar.DayRanges, banexg.LocUTC)
		if err != nil {
			return nil, err
		}
	}
	if len(rawMar.NightRanges) > 0 {
		mar.NightTimes, err = utils.ParseTimeRanges(rawMar.NightRanges, banexg.LocUTC)
		if err != nil {
			return nil, err
		}
	}
	return mar, nil
}

func (e *China) FetchTicker(symbol string, params map[string]interface{}) (*banexg.Ticker, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *China) FetchTickers(symbols []string, params map[string]interface{}) ([]*banexg.Ticker, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *China) FetchTickerPrice(symbol string, params map[string]interface{}) (map[string]float64, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *China) LoadLeverageBrackets(reload bool, params map[string]interface{}) *errs.Error {
	return nil
}

func (e *China) GetLeverage(symbol string, notional float64, account string) (float64, float64) {
	mar, exist := e.Markets[symbol]
	if !exist {
		return 0, 0
	}
	if mar.Type == banexg.MarketSpot {
		return 1, 1
	}
	raw, _ := mar.Info.(*ItemMarket)
	if raw != nil {
		leverage := 100 / raw.MarginPct
		return leverage, leverage
	}
	return 0, 0
}

func (e *China) FetchOHLCV(symbol, timeframe string, since int64, limit int, params map[string]interface{}) ([]*banexg.Kline, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *China) FetchOrderBook(symbol string, limit int, params map[string]interface{}) (*banexg.OrderBook, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *China) FetchOrder(symbol, orderId string, params map[string]interface{}) (*banexg.Order, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *China) FetchOrders(symbol string, since int64, limit int, params map[string]interface{}) ([]*banexg.Order, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *China) FetchBalance(params map[string]interface{}) (*banexg.Balances, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *China) FetchAccountPositions(symbols []string, params map[string]interface{}) ([]*banexg.Position, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *China) FetchPositions(symbols []string, params map[string]interface{}) ([]*banexg.Position, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *China) FetchOpenOrders(symbol string, since int64, limit int, params map[string]interface{}) ([]*banexg.Order, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *China) CreateOrder(symbol, odType, side string, amount, price float64, params map[string]interface{}) (*banexg.Order, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *China) EditOrder(symbol, orderId, side string, amount, price float64, params map[string]interface{}) (*banexg.Order, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *China) CancelOrder(id string, symbol string, params map[string]interface{}) (*banexg.Order, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeNotImplement, "method not implement")
}

func (e *China) SetLeverage(leverage float64, symbol string, params map[string]interface{}) (map[string]interface{}, *errs.Error) {
	return nil, errs.NewMsg(errs.CodeApiNotSupport, "api not support")
}

func (e *China) CalcMaintMargin(symbol string, cost float64) (float64, *errs.Error) {
	leverage, _ := e.GetLeverage(symbol, cost, "")
	return cost / leverage, nil
}

func makeCalcFee(e *China) banexg.FuncCalcFee {
	return func(market *banexg.Market, curr string, maker bool, amount, price decimal.Decimal, params map[string]interface{}) (*banexg.Fee, *errs.Error) {
		raw, _ := market.Info.(*ItemMarket)
		if raw == nil {
			return nil, errs.NewMsg(errs.CodeParamInvalid, "raw market invalid")
		}
		closeToday, _ := params["closeToday"]
		unit := raw.Fee.Unit
		feeVal := raw.Fee.Val
		if closeToday != nil {
			// 平今手续费
			feeVal = raw.Fee.ValCT
		}
		feeValDc := decimal.NewFromFloat(feeVal)
		var costVal float64
		if unit == "wan" {
			wanDc := decimal.NewFromInt(10000)
			costVal, _ = amount.Mul(price).Mul(feeValDc).Div(wanDc).Float64()
		} else if unit == "lot" {
			mulDc := decimal.NewFromFloat(raw.Multiplier)
			costVal, _ = amount.Div(mulDc).Mul(feeValDc).Float64()
		} else {
			return nil, errs.NewMsg(errs.CodeRunTime, "invalid fee unit: %s", unit)
		}
		costVal = math.Round(costVal*100) / 100
		odCost, _ := amount.Mul(price).Float64()
		return &banexg.Fee{
			Cost:     costVal,
			Currency: curr,
			IsMaker:  maker,
			Rate:     costVal / odCost,
		}, nil
	}
}

func (e *China) Close() *errs.Error {
	return errs.NewMsg(errs.CodeNotImplement, "method not implement")
}
