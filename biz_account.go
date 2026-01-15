package banexg

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/banbox/banexg/errs"
)

type AccountAccess struct {
	TradeAllowed    bool
	TradeKnown      bool
	WithdrawAllowed bool
	WithdrawKnown   bool
	IPAny           bool
	IPKnown         bool
	PosMode         string
	AcctMode        string
	AcctLv          string
	MarginMode      string
	Info            map[string]interface{}
}

func (a *AccountAccess) HasAny() bool {
	if a == nil {
		return false
	}
	return a.TradeKnown || a.WithdrawKnown || a.IPKnown || a.PosMode != "" || a.AcctMode != "" || a.AcctLv != "" || a.MarginMode != ""
}

func (e *Exchange) FetchAccountAccess(params map[string]interface{}) (*AccountAccess, *errs.Error) {
	res := &AccountAccess{}
	if params == nil {
		return res, nil
	}
	if bal, ok := params[ParamBalance].(*Balances); ok && bal != nil {
		res.Info = bal.Info
		FillAccountAccessFromInfo(res, bal.Info)
	}
	return res, nil
}

func FillAccountAccessFromInfo(acc *AccountAccess, info map[string]interface{}) {
	if acc == nil || info == nil {
		return
	}
	if acc.Info == nil {
		acc.Info = info
	}
	if !acc.TradeKnown {
		if val, ok := BoolFromInfo(info, "canTrade", "tradeEnabled", "enableTrading"); ok {
			acc.TradeKnown = true
			acc.TradeAllowed = val
		}
	}
	if !acc.WithdrawKnown {
		if val, ok := BoolFromInfo(info, "canWithdraw", "withdrawEnable", "withdrawEnabled", "enableWithdrawals"); ok {
			acc.WithdrawKnown = true
			acc.WithdrawAllowed = val
		}
	}
	if acc.PosMode == "" {
		if val, ok := info["posMode"]; ok {
			acc.PosMode = NormalizePosMode(fmt.Sprintf("%v", val))
		}
		if acc.PosMode == "" {
			if val, ok := info["dualSidePosition"]; ok {
				if parsed, ok := ParseBool(val); ok {
					acc.PosMode = PosModeFromBool(parsed)
				}
			}
		}
	}
	if !acc.IPKnown {
		if val, ok := BoolFromInfo(info, "ipRestrict", "ipRestriction"); ok {
			acc.IPKnown = true
			acc.IPAny = !val
		} else if val, ok := info["ip"]; ok {
			ipStr := strings.TrimSpace(fmt.Sprintf("%v", val))
			acc.IPKnown = true
			acc.IPAny = ipStr == ""
		}
	}
}

func MergeAccountAccess(dst *AccountAccess, src *AccountAccess) {
	if dst == nil || src == nil {
		return
	}
	if !dst.TradeKnown && src.TradeKnown {
		dst.TradeKnown = true
		dst.TradeAllowed = src.TradeAllowed
	}
	if !dst.WithdrawKnown && src.WithdrawKnown {
		dst.WithdrawKnown = true
		dst.WithdrawAllowed = src.WithdrawAllowed
	}
	if !dst.IPKnown && src.IPKnown {
		dst.IPKnown = true
		dst.IPAny = src.IPAny
	}
	if dst.PosMode == "" && src.PosMode != "" {
		dst.PosMode = src.PosMode
	}
	if dst.AcctMode == "" && src.AcctMode != "" {
		dst.AcctMode = src.AcctMode
	}
	if dst.AcctLv == "" && src.AcctLv != "" {
		dst.AcctLv = src.AcctLv
	}
	if dst.MarginMode == "" && src.MarginMode != "" {
		dst.MarginMode = src.MarginMode
	}
	if dst.Info == nil && src.Info != nil {
		dst.Info = src.Info
	}
}

func NormalizePosMode(mode string) string {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return ""
	}
	switch strings.ToLower(mode) {
	case PosModeHedge, "long_short_mode", "hedged", "dual", "true", "1", "yes", "双向":
		return PosModeHedge
	case PosModeOneWay, "net_mode", "single", "false", "0", "no", "单向":
		return PosModeOneWay
	default:
		return ""
	}
}

func PosModeFromBool(dual bool) string {
	if dual {
		return PosModeHedge
	}
	return PosModeOneWay
}

func BoolFromInfo(info map[string]interface{}, keys ...string) (bool, bool) {
	for _, key := range keys {
		if val, ok := info[key]; ok {
			if parsed, ok := ParseBool(val); ok {
				return parsed, true
			}
		}
	}
	return false, false
}

func ParseBool(val interface{}) (bool, bool) {
	switch v := val.(type) {
	case bool:
		return v, true
	case string:
		v = strings.TrimSpace(strings.ToLower(v))
		if v == "true" || v == "1" || v == "yes" {
			return true, true
		}
		if v == "false" || v == "0" || v == "no" {
			return false, true
		}
	case float64:
		return v != 0, true
	case float32:
		return v != 0, true
	case int:
		return v != 0, true
	case int64:
		return v != 0, true
	case int32:
		return v != 0, true
	case uint64:
		return v != 0, true
	case uint32:
		return v != 0, true
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f != 0, true
		}
	}
	return false, false
}
