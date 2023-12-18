package banexg

import (
	"fmt"
	"github.com/anyongjin/banexg/utils"
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
