package binance

import (
	"context"
	"fmt"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
	"github.com/shopspring/decimal"
	"strings"
	"time"
)

func (e *Binance) FetchFundingCash(ctx context.Context, account, currency string, since, until int64) ([]banexg.FundingCash, *errs.Error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if since < 0 || until < since || until-since > int64(7*24*time.Hour/time.Millisecond) {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "invalid funding cash window")
	}
	if e == nil || e.Exchange == nil || e.MarketType != banexg.MarketLinear || account == "" || currency == "" {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "linear account and currency required")
	}
	out := make([]banexg.FundingCash, 0)
	seen := map[string]bool{}
	for _, incomeType := range []string{"FUNDING_FEE", "SPECIAL_FUNDING_FEE"} {
		rows, err := e.fundingIncomeWindow(ctx, account, incomeType, since, until)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.IncomeType != incomeType || row.TranID <= 0 {
				return nil, errs.NewMsg(errs.CodeUnmarshalFail, "incomplete funding income")
			}
			id := fmt.Sprintf("funding/%s/%d", row.Asset, row.TranID)
			// Transaction IDs are unique within income types, not across them.
			if incomeType == "SPECIAL_FUNDING_FEE" {
				id = fmt.Sprintf("funding/%s/%s/%d", incomeType, row.Asset, row.TranID)
			}
			if seen[id] {
				return nil, errs.NewMsg(errs.CodeInvalidData, "duplicate funding income")
			}
			seen[id] = true
			if row.Asset != currency {
				continue
			}
			market := e.GetMarketById(row.Symbol, banexg.MarketLinear)
			if market == nil {
				return nil, errs.NewMsg(errs.CodeDataNotFound, "unknown funding symbol %s", row.Symbol)
			}
			rateType := "Regular"
			if incomeType == "SPECIAL_FUNDING_FEE" {
				rateType = "Special"
			}
			mark, rate, er := e.fundingCashRate(ctx, account, row.Symbol, row.Time, rateType)
			if er != nil {
				return nil, er
			}
			if _, er := decimal.NewFromString(row.Income); er != nil {
				return nil, errs.NewMsg(errs.CodeUnmarshalFail, "invalid funding income")
			}
			out = append(out, banexg.FundingCash{ID: id, Symbol: market.Symbol, Currency: row.Asset, Amount: row.Income, Mark: mark, Rate: rate, AtMS: row.Time})
		}
	}
	return out, nil
}

// Income no longer supports page. Bisect saturated inclusive windows rather
// than advancing past a timestamp that may contain unreturned settlements.
func (e *Binance) fundingIncomeWindow(ctx context.Context, account, incomeType string, since, until int64) ([]Income, *errs.Error) {
	args := map[string]interface{}{banexg.ParamAccount: account, "startTime": since, "endTime": until, "incomeType": incomeType, "limit": 1000}
	rsp := e.RequestApiRetry(ctx, MethodFapiPrivateGetIncome, args, 0)
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	if strings.TrimSpace(rsp.Content) == "null" {
		return nil, errs.NewMsg(errs.CodeInvalidData, "null funding inventory is not a complete snapshot")
	}
	var rows []Income
	if err := utils.UnmarshalString(rsp.Content, &rows, utils.JsonNumDefault); err != nil {
		return nil, errs.New(errs.CodeUnmarshalFail, err)
	}
	for _, row := range rows {
		if row.Time < since || row.Time > until {
			return nil, errs.NewMsg(errs.CodeInvalidData, "funding income outside requested window")
		}
	}
	if len(rows) < 1000 {
		return rows, nil
	}
	if since == until {
		return nil, errs.NewMsg(errs.CodeInvalidData, "funding income timestamp exceeds snapshot limit")
	}
	mid := since + (until-since)/2
	left, err := e.fundingIncomeWindow(ctx, account, incomeType, since, mid)
	if err != nil {
		return nil, err
	}
	right, err := e.fundingIncomeWindow(ctx, account, incomeType, mid+1, until)
	if err != nil {
		return nil, err
	}
	return append(left, right...), nil
}

func (e *Binance) fundingCashRate(ctx context.Context, account, symbol string, at int64, rateType string) (string, string, *errs.Error) {
	rsp := e.RequestApiRetry(ctx, MethodFapiPublicGetFundingRate, map[string]interface{}{banexg.ParamAccount: account, "symbol": symbol, "startTime": at, "endTime": at, "limit": 1000}, 0)
	if rsp.Error != nil {
		return "", "", rsp.Error
	}
	var rows []struct {
		Symbol      string `json:"symbol"`
		FundingRate string `json:"fundingRate"`
		MarkPrice   string `json:"markPrice"`
		FundingTime int64  `json:"fundingTime"`
		RateType    string `json:"rateType"`
	}
	if er := utils.UnmarshalString(rsp.Content, &rows, utils.JsonNumDefault); er != nil {
		return "", "", errs.New(errs.CodeUnmarshalFail, er)
	}
	for _, row := range rows {
		if row.FundingTime == at && row.Symbol == symbol && (row.RateType == rateType || row.RateType == "" && rateType == "Regular") {
			m, er := decimal.NewFromString(row.MarkPrice)
			if er != nil || !m.IsPositive() {
				return "", "", errs.NewMsg(errs.CodeDataNotFound, "funding mark missing")
			}
			if _, er = decimal.NewFromString(row.FundingRate); er != nil {
				return "", "", errs.NewMsg(errs.CodeDataNotFound, "funding rate missing")
			}
			return row.MarkPrice, row.FundingRate, nil
		}
	}
	return "", "", errs.NewMsg(errs.CodeDataNotFound, "funding rate missing")
}
