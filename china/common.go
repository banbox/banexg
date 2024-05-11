package china

import (
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/log"
	utils2 "github.com/banbox/banexg/utils"
	"go.uber.org/zap"
	"strings"
)

func (m *ItemMarket) Resolve(bases map[string]*ItemMarket) {
	if m.Extend == "" {
		return
	}
	base, _ := bases[m.Extend]
	if base == nil {
		log.Warn("china market extend invalid", zap.String("val", m.Extend), zap.String("from", m.Code))
		return
	}
	if m.Market == "" && base.Market != "" {
		m.Market = base.Market
	}
	if m.Exchange == "" && base.Exchange != "" {
		m.Exchange = base.Exchange
	}
	if m.DayRanges == nil && len(base.DayRanges) > 0 {
		m.DayRanges = base.DayRanges
	}
	if m.NightRanges == nil && len(base.NightRanges) > 0 {
		m.NightRanges = base.NightRanges
	}
	if m.Fee == nil && base.Fee != nil {
		m.Fee = base.Fee
	}
	if m.FeeCT == nil && base.FeeCT != nil {
		m.FeeCT = base.FeeCT
	}
	if m.PriceTick == 0 && base.PriceTick != 0 {
		m.PriceTick = base.PriceTick
	}
	if m.LimitChgPct == 0 && base.LimitChgPct != 0 {
		m.LimitChgPct = base.LimitChgPct
	}
	if m.MarginPct == 0 && base.MarginPct != 0 {
		m.MarginPct = base.MarginPct
	}
}

func (m *ItemMarket) toSymbol(parts []*utils2.StrType, toStd bool) (string, *errs.Error) {
	if len(parts) == 0 {
		return "", errs.NewMsg(errs.CodeParamRequired, "parts is empty")
	}
	exchange := ctExgs[m.Exchange]
	var b strings.Builder
	if m.Market != banexg.MarketSpot {
		// 期货、期权
		p0, p1 := parts[0], parts[1]
		if p0.Type != utils2.StrStr {
			return "", errs.NewMsg(errs.CodeParamInvalid, "part0 should be str")
		}
		if toStd {
			b.WriteString(strings.ToUpper(p0.Val))
		} else if exchange.CaseLower {
			b.WriteString(strings.ToLower(p0.Val))
		} else {
			b.WriteString(p0.Val)
		}
		if p1.Type != utils2.StrInt {
			return "", errs.NewMsg(errs.CodeParamInvalid, "part1 should be int")
		}
		// 写入年月
		p1val := p1.Val
		if toStd || (p1val == "000" || p1val == "888" || p1val == "999") {
			b.WriteString(p1val)
		} else {
			b.WriteString(p1val[len(p1val)-exchange.DateNum:])
		}
		// 判断是否期权
		if len(parts) == 4 && parts[2].Type == utils2.StrStr && len(parts[2].Val) == 1 && parts[3].Type == utils2.StrInt {
			// 第三个是C/P，第四个是价格
			if toStd {
				b.WriteString(strings.ReplaceAll(parts[2].Val, "-", ""))
			} else if exchange.OptionDash {
				b.WriteString("-")
				b.WriteString(parts[2].Val)
				b.WriteString("-")
			} else {
				b.WriteString(parts[2].Val)
			}
			b.WriteString(parts[3].Val)
		} else {
			for _, p := range parts[2:] {
				b.WriteString(p.Val)
			}
			if m.Market == banexg.MarketOption {
				return "", errs.NewMsg(errs.CodeParamInvalid, "invalid option symbol: %s", b.String())
			}
		}
		return b.String(), nil
	}
	return "", errs.NotImplement
}

/*
ToStdSymbol
转为标准Symbol，注意期货的年月需要提前归一化为4位数字
*/
func (m *ItemMarket) ToStdSymbol(parts []*utils2.StrType) (string, *errs.Error) {
	return m.toSymbol(parts, true)
}

/*
ToRawSymbol
转为交易所Symbol
*/
func (m *ItemMarket) ToRawSymbol(parts []*utils2.StrType) (string, *errs.Error) {
	return m.toSymbol(parts, false)
}
