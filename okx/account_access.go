package okx

import (
	"strings"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

type accConfig struct {
	AcctLv  string `json:"acctLv"`
	PosMode string `json:"posMode"`
	Perm    string `json:"perm"`
	IP      string `json:"ip"`
}

type accConfigResp struct {
	Code string      `json:"code"`
	Data []accConfig `json:"data"`
	Msg  string      `json:"msg"`
}

func (e *OKX) FetchAccountAccess(params map[string]interface{}) (*banexg.AccountAccess, *errs.Error) {
	args := utils.SafeParams(params)
	res := &banexg.AccountAccess{}
	if bal, ok := args[banexg.ParamBalance].(*banexg.Balances); ok && bal != nil {
		banexg.FillAccountAccessFromInfo(res, bal.Info)
	}
	rsp, err := e.Call(MethodAccountGetConfig, args)
	if err != nil {
		if res.HasAny() {
			return res, nil
		}
		return res, err
	}
	var out accConfigResp
	if err := utils.UnmarshalString(rsp.Content, &out, utils.JsonNumDefault); err != nil {
		if res.HasAny() {
			return res, nil
		}
		return res, errs.New(errs.CodeUnmarshalFail, err)
	}
	if out.Code != "0" || len(out.Data) == 0 {
		if res.HasAny() {
			return res, nil
		}
		return res, errs.NewMsg(errs.CodeUnmarshalFail, "account config returned code=%s", out.Code)
	}
	cfg := out.Data[0]
	res.AcctLv = cfg.AcctLv
	res.AcctMode = mapAcctMode(cfg.AcctLv)
	res.PosMode = banexg.NormalizePosMode(cfg.PosMode)
	if res.PosMode == "" {
		res.PosMode = cfg.PosMode
	}
	permSet := splitPerm(cfg.Perm)
	if len(permSet) > 0 {
		res.TradeKnown = true
		res.TradeAllowed = permSet["trade"]
		res.WithdrawKnown = true
		res.WithdrawAllowed = permSet["withdraw"]
	}
	res.IPKnown = true
	res.IPAny = strings.TrimSpace(cfg.IP) == ""
	return res, nil
}

func splitPerm(perm string) map[string]bool {
	res := make(map[string]bool)
	for _, item := range strings.Split(perm, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			res[item] = true
		}
	}
	return res
}

func mapAcctMode(acctLv string) string {
	switch acctLv {
	case "1":
		return banexg.AcctModeSpot
	case "2":
		return banexg.AcctModeSingleCurrencyMargin
	case "3":
		return banexg.AcctModeMultiCurrencyMargin
	case "4":
		return banexg.AcctModePortfolioMargin
	default:
		return ""
	}
}
