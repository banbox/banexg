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
	for page := 1; ; page++ {
		args := map[string]interface{}{banexg.ParamAccount: account, "startTime": since, "endTime": until, "incomeType": "FUNDING_FEE", "limit": 1000, "page": page}
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
			pageID := fmt.Sprintf("%s/%s/%d", row.IncomeType, row.Asset, row.TranID)
			if row.TranID <= 0 || seen[pageID] {
				return nil, errs.NewMsg(errs.CodeInvalidData, "funding pagination did not advance")
			}
			seen[pageID] = true
			if row.IncomeType != "FUNDING_FEE" || row.TranID <= 0 {
				return nil, errs.NewMsg(errs.CodeUnmarshalFail, "incomplete funding income")
			}
			if row.Asset != currency || row.Time < since || row.Time > until {
				continue
			}
			market := e.GetMarketById(row.Symbol, banexg.MarketLinear)
			if market == nil {
				return nil, errs.NewMsg(errs.CodeDataNotFound, "unknown funding symbol %s", row.Symbol)
			}
			mark, rate, er := e.fundingCashRate(ctx, account, row.Symbol, row.Time)
			if er != nil {
				return nil, er
			}
			if _, er := decimal.NewFromString(row.Income); er != nil {
				return nil, errs.NewMsg(errs.CodeUnmarshalFail, "invalid funding income")
			}
			id := fmt.Sprintf("funding/%s/%d", row.Asset, row.TranID)
			if seen[id] {
				return nil, errs.NewMsg(errs.CodeUnmarshalFail, "duplicate funding income")
			}
			seen[id] = true
			out = append(out, banexg.FundingCash{ID: id, Symbol: market.Symbol, Currency: row.Asset, Amount: row.Income, Mark: mark, Rate: rate, AtMS: row.Time})
		}
		if len(rows) < 1000 {
			return out, nil
		}
	}
}
func (e *Binance) fundingCashRate(ctx context.Context, account, symbol string, at int64) (string, string, *errs.Error) {
	rsp := e.RequestApiRetry(ctx, MethodFapiPublicGetFundingRate, map[string]interface{}{banexg.ParamAccount: account, "symbol": symbol, "startTime": at, "endTime": at, "limit": 1}, 0)
	if rsp.Error != nil {
		return "", "", rsp.Error
	}
	var rows []struct {
		Symbol      string `json:"symbol"`
		FundingRate string `json:"fundingRate"`
		MarkPrice   string `json:"markPrice"`
		FundingTime int64  `json:"fundingTime"`
	}
	if er := utils.UnmarshalString(rsp.Content, &rows, utils.JsonNumDefault); er != nil {
		return "", "", errs.New(errs.CodeUnmarshalFail, er)
	}
	for _, row := range rows {
		if row.FundingTime == at && row.Symbol == symbol {
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
