package banexg

import (
	"fmt"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"go.uber.org/zap"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (p *Precision) ToString() string {
	return fmt.Sprintf("%v-%v-%v-%v", p.Amount, p.Price, p.Base, p.Quote)
}

func (r *LimitRange) ToString() string {
	return fmt.Sprintf("[%v-%v]", r.Min, r.Max)
}

func (l *MarketLimits) ToString() string {
	if l == nil {
		return ""
	}
	var b strings.Builder
	if l.Leverage != nil {
		b.WriteString("l:")
		b.WriteString(l.Leverage.ToString())
	}
	if l.Amount != nil {
		b.WriteString("a:")
		b.WriteString(l.Amount.ToString())
	}
	if l.Price != nil {
		b.WriteString("p:")
		b.WriteString(l.Price.ToString())
	}
	if l.Cost != nil {
		b.WriteString("c:")
		b.WriteString(l.Cost.ToString())
	}
	if l.Market != nil {
		b.WriteString("m:")
		b.WriteString(l.Market.ToString())
	}
	return b.String()
}

func (l *CodeLimits) ToString() string {
	if l == nil {
		return ""
	}
	var b strings.Builder
	if l.Amount != nil {
		b.WriteString("a:")
		b.WriteString(l.Amount.ToString())
	}
	if l.Withdraw != nil {
		b.WriteString("w:")
		b.WriteString(l.Withdraw.ToString())
	}
	if l.Deposit != nil {
		b.WriteString("d:")
		b.WriteString(l.Deposit.ToString())
	}
	return b.String()
}

func (b *Balances) Init() *Balances {
	if b.TimeStamp == 0 {
		b.TimeStamp = time.Now().UnixMilli()
	}
	if b.Free == nil {
		b.Free = map[string]float64{}
	}
	if b.Used == nil {
		b.Used = map[string]float64{}
	}
	if b.Total == nil {
		b.Total = map[string]float64{}
	}
	for code, ast := range b.Assets {
		if ast.Total == 0 {
			ast.Total = ast.Used + ast.Free
		}
		b.Free[code] = ast.Free
		b.Used[code] = ast.Used
		b.Total[code] = ast.Total
	}
	return b
}

func (a *Asset) IsEmpty() bool {
	return utils.EqualNearly(a.Used+a.Free, 0) && utils.EqualNearly(a.Debt, 0)
}

func (b *OrderBook) SetSide(text string, isBuy, replace bool) {
	var arr = make([][2]string, 0)
	err := utils.UnmarshalString(text, &arr)
	if err != nil {
		log.Error("unmarshal od book side fail", zap.Error(err))
		return
	}
	side := b.Asks
	if isBuy {
		side = b.Bids
	}
	if replace {
		var prices = make([]float64, len(arr))
		var sizes = make([]float64, len(arr))
		for i, row := range arr {
			prices[i], _ = strconv.ParseFloat(row[0], 64)
			sizes[i], _ = strconv.ParseFloat(row[1], 64)
		}
		side.Lock.Lock()
		side.Price = prices
		side.Size = sizes
		side.Lock.Unlock()
	} else {
		var valArr = make([][2]float64, len(arr))
		for i, row := range arr {
			val1, _ := strconv.ParseFloat(row[0], 64)
			val2, _ := strconv.ParseFloat(row[1], 64)
			valArr[i][0] = val1
			valArr[i][1] = val2
		}
		side.Update(valArr)
	}
}

func NewOdBookSide(isBuy bool, depth int, deltas [][2]float64) *OdBookSide {
	obs := &OdBookSide{
		IsBuy: isBuy,
		Depth: depth,
		Price: make([]float64, 0, len(deltas)),
		Size:  make([]float64, 0, len(deltas)),
	}
	obs.Update(deltas)
	return obs
}

func (obs *OdBookSide) Update(deltas [][2]float64) {
	obs.Lock.Lock()
	for _, delta := range deltas {
		obs.Set(delta[0], delta[1])
	}
	for len(obs.Price) > obs.Depth {
		obs.Size = obs.Size[:obs.Depth]
		obs.Price = obs.Price[:obs.Depth]
	}
	obs.Lock.Unlock()
}

func (obs *OdBookSide) Set(price, size float64) {
	oldLen := len(obs.Price)
	prices := obs.Price
	var index int
	if obs.IsBuy {
		// desc order
		index = sort.Search(oldLen, func(i int) bool {
			return prices[i] <= price
		})
	} else {
		// asc order
		index = sort.Search(oldLen, func(i int) bool {
			return prices[i] >= price
		})
	}
	if size > 0 {
		if index < oldLen && prices[index] == price {
			obs.Size[index] = size
		} else {
			obs.Price = append(prices, 0)
			copy(obs.Price[index+1:], prices[index:])
			obs.Price[index] = price

			obs.Size = append(obs.Size, 0)
			copy(obs.Size[index+1:], obs.Size[index:])
			obs.Size[index] = size
		}
	} else if index < oldLen && prices[index] == price {
		obs.Price = append(prices[:index], prices[index+1:]...)
		obs.Size = append(obs.Size[:index], obs.Size[index+1:]...)
	}
}

/*
SumVolTo return (total volume to price, filled rate)
*/
func (obs *OdBookSide) SumVolTo(price float64) (float64, float64) {
	dirt := float64(1)
	if obs.IsBuy {
		dirt = float64(-1)
	}
	if len(obs.Price) == 0 {
		return 0, 1
	}
	obs.Lock.Lock()
	volSum := float64(0)
	lastPrice := float64(0)
	firstPrice := obs.Price[0]
	for i, p := range obs.Price {
		lastPrice = p
		priceDiff := p - price
		if priceDiff*dirt >= 0 {
			obs.Lock.Unlock()
			return volSum, 1
		}
		volSum += obs.Size[i]
	}
	obs.Lock.Unlock()
	return volSum, math.Abs(lastPrice-firstPrice) / math.Abs(price-firstPrice)
}

func (obs *OdBookSide) Price0() float64 {
	var result = float64(0)
	obs.Lock.Lock()
	if len(obs.Price) > 0 {
		result = obs.Price[0]
	}
	obs.Lock.Unlock()
	return result
}

/*
AvgPrice
return average price, filled rate, change rate of first & last
*/
func (obs *OdBookSide) AvgPrice(volume float64) (float64, float64, float64) {
	obs.Lock.Lock()
	prices, sizes := obs.Price, obs.Size
	obs.Lock.Unlock()
	volSum, lastPrice, cost := float64(0), float64(0), float64(0)
	for i, price := range prices {
		size := sizes[i]
		volSum += size
		lastPrice = price
		cost += size * price
		if volSum >= volume {
			break
		}
	}
	if volSum == 0 {
		return 0, 0, 0
	}
	price0 := prices[0]
	if volSum < volume {
		lastPrice = price0 + (lastPrice-price0)*volume/volSum
		chgRate := math.Abs(lastPrice-price0) / price0
		return price0*0.3 + lastPrice*0.7, volSum / volume, chgRate
	}
	chgRate := math.Abs(lastPrice-price0) / price0
	return cost / volSum, 1, chgRate
}

func (b *OrderBook) AvgPrice(side string, depth float64) (float64, float64, float64) {
	book := b.Asks
	if side == OdSideBuy {
		book = b.Bids
	}
	return book.AvgPrice(depth)
}

/*
SumVolTo
get sum volume between current price and target price
second return val is rate filled
*/
func (b *OrderBook) SumVolTo(side string, price float64) (float64, float64) {
	book := b.Asks
	if side == OdSideBuy {
		book = b.Bids
	}
	return book.SumVolTo(price)
}

func (b *OrderBook) Reset() {
	b.Asks.Lock.Lock()
	b.Bids.Lock.Lock()
	b.Nonce = 0
	b.Bids.Size = nil
	b.Bids.Price = nil
	b.Asks.Size = nil
	b.Asks.Price = nil
	b.Cache = nil
	b.Bids.Lock.Unlock()
	b.Asks.Lock.Unlock()
}

func (b *OrderBook) Update(book *OrderBook) {
	b.TimeStamp = book.TimeStamp
	b.Nonce = book.Nonce
	b.Cache = nil
	b.Asks.Lock.Lock()
	b.Bids.Lock.Lock()
	b.Asks.Price = book.Asks.Price
	b.Asks.Size = book.Asks.Size
	b.Bids.Price = book.Bids.Price
	b.Bids.Size = book.Bids.Size
	b.Bids.Lock.Unlock()
	b.Asks.Lock.Unlock()
}

func (k *Kline) Clone() *Kline {
	return &Kline{
		Time:   k.Time,
		Open:   k.Open,
		High:   k.High,
		Low:    k.Low,
		Close:  k.Close,
		Volume: k.Volume,
		Info:   k.Info,
	}
}

func EnsureArrStr(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "[]"
	} else if strings.HasPrefix(text, "[") {
		return text
	}
	return strings.Join([]string{"[", text, "]"}, "")
}

/*
MergeMyTrades
将WatchMyTrades收到的同Symbol+Order的交易，合并为Order
*/
func MergeMyTrades(trades []*MyTrade) (*Order, *errs.Error) {
	if len(trades) == 0 {
		return nil, nil
	}
	sort.Slice(trades, func(i, j int) bool {
		return trades[i].Timestamp < trades[j].Timestamp
	})
	first := trades[0]
	od := &Order{
		ID:                  first.Order,
		ClientOrderID:       first.ClientID,
		Datetime:            utils.ISO8601(first.Timestamp),
		Timestamp:           first.Timestamp,
		LastTradeTimestamp:  first.Timestamp,
		LastUpdateTimestamp: first.Timestamp,
		Status:              "",
		Symbol:              first.Symbol,
		Type:                first.Type,
		PositionSide:        first.PosSide,
		Side:                first.Side,
		Price:               first.Price,
		Average:             first.Average,
		Amount:              first.Amount,
		Filled:              first.Filled,
		Remaining:           0,
		Cost:                first.Cost,
		ReduceOnly:          first.ReduceOnly,
		Trades:              make([]*Trade, 0, len(trades)),
		Fee:                 &Fee{},
	}
	od.Trades = append(od.Trades, &first.Trade)
	if first.Fee != nil {
		od.Fee.Cost = first.Fee.Cost
		od.Fee.Currency = first.Fee.Currency
		od.Fee.IsMaker = first.Fee.IsMaker
		od.Fee.Rate = first.Fee.Rate
	}
	var statusDone bool
	for _, trade := range trades[1:] {
		if trade.Amount == 0 {
			continue
		}
		if trade.Symbol != od.Symbol || trade.Order != od.ID || trade.Side != od.Side {
			msg := fmt.Sprintf("all trades to merge must be same pair, orderId, side, %s %s %s %s %s %s",
				trade.Symbol, od.Symbol, trade.Order, od.ID, trade.Side, od.Side)
			return nil, errs.NewMsg(errs.CodeParamInvalid, msg)
		}
		od.LastTradeTimestamp = trade.Timestamp
		od.LastUpdateTimestamp = trade.Timestamp
		od.Amount += trade.Amount
		od.Filled = trade.Filled
		od.Cost += trade.Cost
		od.Average = trade.Average
		od.Price = trade.Average
		od.ReduceOnly = od.ReduceOnly && trade.ReduceOnly
		od.Trades = append(od.Trades, &trade.Trade)
		if trade.Fee != nil {
			od.Fee.Cost += trade.Fee.Cost
			od.Fee.Currency = trade.Fee.Currency
			od.Fee.IsMaker = trade.Fee.IsMaker
			od.Fee.Rate = trade.Fee.Rate
		}
		if !statusDone {
			od.Status = trade.State
			statusDone = IsOrderDone(trade.State)
		}
	}

	if od.Average == 0 && od.Filled > 0 {
		od.Average = od.Cost / od.Filled
		od.Price = od.Average
	}
	return od, nil
}

func IsOrderDone(status string) bool {
	return status == OdStatusFilled || status == OdStatusCanceled || status == OdStatusExpired || status == OdStatusRejected
}

func (m *Market) GetTradeTimes() [][2]int64 {
	times := make([][2]int64, 0, len(m.DayTimes))
	times = append(times, m.DayTimes...)
	times = append(times, m.NightTimes...)
	sort.Slice(times, func(i, j int) bool {
		return times[i][0] < times[j][0]
	})
	return times
}

func GetHostRetryWait(host string, randAdd bool) int64 {
	var waitMS int64
	hostWaitLock.Lock()
	if until, ok := HostRetryWaits[host]; ok {
		waitMS = until - time.Now().UnixMilli()
		if waitMS < 0 {
			delete(HostRetryWaits, host)
		} else if randAdd {
			// 随机增加10s内延迟，避免全部同一时间发起
			randWait := int64(rand.Float32() * 10000)
			waitMS += randWait
		}
	}
	hostWaitLock.Unlock()
	return waitMS
}

func SetHostRetryWait(host string, waitMS int64) {
	hostWaitLock.Lock()
	HostRetryWaits[host] = time.Now().UnixMilli() + waitMS
	hostWaitLock.Unlock()
}

func GetHostFlowChan(host string) chan struct{} {
	hostFlowLock.Lock()
	out, ok := hostFlowChans[host]
	if !ok {
		out = make(chan struct{}, HostHttpConcurr)
		hostFlowChans[host] = out
	}
	hostFlowLock.Unlock()
	return out
}
