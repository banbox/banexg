package china

import (
	_ "embed"
	"fmt"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
	"gopkg.in/yaml.v3"
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
	bases    = make(map[string]*ItemMarket)
	items    = make(map[string]*ItemMarket)
	lockMars = sync.Mutex{}
)

//go:embed markets.yml
var marketsData []byte

func getItemMarkets() (map[string]*ItemMarket, *errs.Error) {
	lockMars.Lock()
	defer lockMars.Unlock()
	if len(items) > 0 {
		return items, nil
	}
	cfg := CnMarkets{}
	err := yaml.Unmarshal(marketsData, &cfg)
	if err != nil {
		return nil, errs.New(errs.CodeUnmarshalFail, err)
	}
	for _, item := range cfg.Goods {
		item.Resolve(bases)
		if strings.HasPrefix(item.Code, "base") {
			bases[item.Code] = item
		} else {
			key := fmt.Sprintf("%s_%s", item.Market, strings.ToUpper(item.Code))
			items[key] = item
			if len(item.Alias) > 0 {
				for _, alias := range item.Alias {
					key = fmt.Sprintf("%s_%s", item.Market, strings.ToUpper(alias))
					items[key] = item
				}
			}
		}
	}
	return items, nil
}

func (e *China) MapMarket(exgSID string, year int) (*banexg.Market, *errs.Error) {
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
	rawMars, err := getItemMarkets()
	if err != nil {
		return nil, err
	}
	parts := utils.SplitParts(exgSID)
	if len(parts) == 0 || parts[0].Type != utils.StrStr {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "exchange symbol id must startsWith letters")
	}
	isActive, isFuture := true, false
	if len(parts) > 1 && parts[1].Type == utils.StrInt {
		// 第二部分是数字，表示期货
		var curTime = time.Now()
		if len(parts[1].Val) == 3 {
			// 至少两部分，第二部分是3个数字，改为4个数字
			var yearStr string
			if year == 0 {
				yearStr = curTime.Format("2006")
			} else {
				yearStr = strconv.Itoa(year)
			}
			parts[1].Val = yearStr[len(yearStr)-2:len(yearStr)-1] + parts[1].Val
		}
		if len(parts[1].Val) == 4 {
			isFuture = true
			inYearMon, _ := strconv.Atoi(parts[1].Val)
			curYear, curMon, _ := curTime.Date()
			curYearMon := curYear%100*100 + int(curMon)
			maxYearMon := curYearMon + 200 // 合约编号最长是2年，大部分1年
			monDiff := 0
			if inYearMon > maxYearMon {
				// 超过未来2年的期货合约ID，认为是100年前的
				monDiff = curYearMon + 10000 - inYearMon
			} else {
				monDiff = curYearMon - inYearMon
			}
			if monDiff >= 0 {
				// 当前年月超过合约到期年月，已交割，不可交易
				isActive = false
			}
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
	rawMar, ok := rawMars[key]
	if !ok {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "symbol invalid: %s", exgSID)
	}
	var b strings.Builder
	b.Grow(len(exgSID) + 1)
	b.WriteString(strings.ToUpper(rawMar.Code))
	for _, p := range parts[1:] {
		b.WriteString(strings.ToUpper(p.Val))
	}
	stdSymbol := b.String()
	isOption := market == banexg.MarketOption
	mar = &banexg.Market{
		ID:          exgSID,
		LowercaseID: strings.ToLower(exgSID),
		Symbol:      stdSymbol,
		Base:        rawMar.Code,
		ExgReal:     rawMar.Exchange,
		Type:        market,
		Spot:        market == banexg.MarketSpot,
		Future:      isFuture,
		Option:      isOption,
		Contract:    isFuture,
		Active:      isActive,
		Linear:      isFuture && !isOption,
		FeeSide:     "quote",
		Info:        rawMar,
	}
	timeLoc, _ := time.LoadLocation("UTC")
	if len(rawMar.DayRanges) > 0 {
		mar.DayTimes, err = utils.ParseTimeRanges(rawMar.DayRanges, timeLoc)
		if err != nil {
			return nil, err
		}
	}
	if len(rawMar.NightRanges) > 0 {
		mar.NightTimes, err = utils.ParseTimeRanges(rawMar.NightRanges, timeLoc)
		if err != nil {
			return nil, err
		}
	}
	if e.MarketsById == nil {
		e.MarketsById = make(banexg.MarketArrMap)
	}
	e.MarketsById[exgSID] = []*banexg.Market{mar}
	if e.Markets == nil {
		e.Markets = make(banexg.MarketMap)
	}
	e.Markets[mar.Symbol] = mar
	return mar, nil
}

func (e *China) FetchTicker(symbol string, params map[string]interface{}) (*banexg.Ticker, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *China) FetchTickers(symbols []string, params map[string]interface{}) ([]*banexg.Ticker, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *China) FetchTickerPrice(symbol string, params map[string]interface{}) (map[string]float64, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *China) LoadLeverageBrackets(reload bool, params map[string]interface{}) *errs.Error {
	return errs.NotImplement
}

func (e *China) GetLeverage(symbol string, notional float64, account string) (int, int) {
	return 0, 0
}

func (e *China) FetchOHLCV(symbol, timeframe string, since int64, limit int, params map[string]interface{}) ([]*banexg.Kline, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *China) FetchOrderBook(symbol string, limit int, params map[string]interface{}) (*banexg.OrderBook, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *China) FetchOrder(symbol, orderId string, params map[string]interface{}) (*banexg.Order, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *China) FetchOrders(symbol string, since int64, limit int, params map[string]interface{}) ([]*banexg.Order, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *China) FetchBalance(params map[string]interface{}) (*banexg.Balances, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *China) FetchAccountPositions(symbols []string, params map[string]interface{}) ([]*banexg.Position, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *China) FetchPositions(symbols []string, params map[string]interface{}) ([]*banexg.Position, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *China) FetchOpenOrders(symbol string, since int64, limit int, params map[string]interface{}) ([]*banexg.Order, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *China) CreateOrder(symbol, odType, side string, amount, price float64, params map[string]interface{}) (*banexg.Order, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *China) EditOrder(symbol, orderId, side string, amount, price float64, params map[string]interface{}) (*banexg.Order, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *China) CancelOrder(id string, symbol string, params map[string]interface{}) (*banexg.Order, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *China) SetLeverage(leverage int, symbol string, params map[string]interface{}) (map[string]interface{}, *errs.Error) {
	return nil, errs.NotImplement
}

func (e *China) CalcMaintMargin(symbol string, cost float64) (float64, *errs.Error) {
	return 0, errs.NotImplement
}

func (e *China) Close() *errs.Error {
	return errs.NotImplement
}
