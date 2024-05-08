package china

import (
	"github.com/banbox/banexg/log"
	"go.uber.org/zap"
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
