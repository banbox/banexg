package banexg

import (
	"fmt"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"go.uber.org/zap"
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

func (ob *OrderBook) SetSide(text string, isBuy bool) {
	var arr = make([][2]string, 0)
	err := utils.UnmarshalString(text, &arr)
	if err != nil {
		log.Error("unmarshal od book side fail", zap.Error(err))
		return
	}
	var valArr = make([][2]float64, len(arr))
	for i, row := range arr {
		val1, _ := strconv.ParseFloat(row[0], 64)
		val2, _ := strconv.ParseFloat(row[1], 64)
		valArr[i][0] = val1
		valArr[i][1] = val2
	}
	if isBuy {
		ob.Bids.Update(valArr)
	} else {
		ob.Asks.Update(valArr)
	}
}

func NewOrderBookSide(isBuy bool, depth int, deltas [][2]float64) *OrderBookSide {
	obs := &OrderBookSide{
		IsBuy: isBuy,
		Depth: depth,
		Rows:  make([][2]float64, 0, len(deltas)),
		Index: make([]float64, 0, len(deltas)),
	}
	obs.Update(deltas)
	return obs
}

func (obs *OrderBookSide) Update(deltas [][2]float64) {
	for _, delta := range deltas {
		obs.StoreArray(delta)
	}
	obs.Limit()
}

func (obs *OrderBookSide) StoreArray(delta [2]float64) {
	price := delta[0]
	size := delta[1]
	indexPrice := price
	if obs.IsBuy {
		indexPrice = -price
	}

	index := sort.SearchFloat64s(obs.Index, indexPrice)
	if size > 0 {
		if index < len(obs.Index) && obs.Index[index] == indexPrice {
			obs.Rows[index][1] = size
		} else {
			obs.Index = append(obs.Index, 0)
			copy(obs.Index[index+1:], obs.Index[index:])
			obs.Index[index] = indexPrice

			obs.Rows = append(obs.Rows, [2]float64{})
			copy(obs.Rows[index+1:], obs.Rows[index:])
			obs.Rows[index] = delta
		}
	} else if index < len(obs.Index) && obs.Index[index] == indexPrice {
		obs.Index = append(obs.Index[:index], obs.Index[index+1:]...)
		obs.Rows = append(obs.Rows[:index], obs.Rows[index+1:]...)
	}
}

func (obs *OrderBookSide) Store(price, size float64) {
	obs.StoreArray([2]float64{price, size})
}

func (obs *OrderBookSide) Limit() {
	for len(obs.Rows) > obs.Depth {
		obs.Rows = obs.Rows[:len(obs.Rows)-1]
		obs.Index = obs.Index[:len(obs.Index)-1]
	}
}

func (b *OrderBook) LimitPrice(side string, depth float64) float64 {
	book := b.Asks
	if side == OdSideBuy {
		book = b.Bids
	}
	volSum, lastPrice := float64(0), float64(0)
	for _, row := range book.Rows {
		volSum += row[1]
		lastPrice = row[0]
		if volSum >= depth {
			break
		}
	}
	if volSum < depth {
		log.Warn("depth not enough", zap.Float64("require", depth), zap.Float64("cur", volSum),
			zap.Int("len", len(book.Rows)))
	}
	return lastPrice
}

/*
SumVolTo
get sum volume between current price and target price
second return val is rate filled
*/
func (b *OrderBook) SumVolTo(side string, price float64) (float64, float64) {
	book := b.Asks
	dirt := float64(1)
	if side == OdSideBuy {
		book = b.Bids
		dirt = float64(-1)
	}
	if len(book.Rows) == 0 {
		return 0, 1
	}
	volSum := float64(0)
	lastPrice := float64(0)
	firstPrice := book.Rows[0][0]
	for _, row := range book.Rows {
		lastPrice = row[0]
		priceDiff := row[0] - price
		if priceDiff*dirt >= 0 {
			return volSum, 1
		}
		volSum += row[1]
	}
	return volSum, (lastPrice - firstPrice) / (price - firstPrice)
}

func (k *Kline) Clone() *Kline {
	return &Kline{
		Time:   k.Time,
		Open:   k.Open,
		High:   k.High,
		Low:    k.Low,
		Close:  k.Close,
		Volume: k.Volume,
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
