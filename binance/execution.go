package binance

import (
	"context"
	"fmt"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
)

// VerifyExecution performs the read-only checks required by the shared live
// execution adapter. Binance linear execution is accepted only in one-way
// mode; hedge mode cannot be represented by the net position contract.
func (e *Binance) VerifyExecution(ctx context.Context, account, currency string) (banexg.ExecutionProof, *errs.Error) {
	proof := banexg.ExecutionProof{Account: account, Currency: currency, AuthoritativeNotFound: false}
	if ctx == nil {
		ctx = context.Background()
	}
	if e == nil || e.Exchange == nil || e.MarketType != banexg.MarketLinear {
		return proof, errs.NewMsg(errs.CodeParamInvalid, "binance execution requires linear market")
	}
	if account == "" || currency == "" {
		return proof, errs.NewMsg(errs.CodeParamRequired, "execution account and currency required")
	}
	base := map[string]interface{}{banexg.ParamContext: ctx, banexg.ParamAccount: account}
	if _, err := e.LoadMarkets(false, base); err != nil {
		return proof, err
	}
	access, err := e.FetchAccountAccess(map[string]interface{}{banexg.ParamContext: ctx, banexg.ParamAccount: account})
	if err != nil {
		return proof, err
	}
	if access == nil || access.PosMode != banexg.PosModeOneWay {
		return proof, errs.NewMsg(errs.CodeParamInvalid, "binance linear account must be in one-way position mode")
	}
	if !access.TradeKnown || !access.TradeAllowed {
		return proof, errs.NewMsg(errs.CodeParamInvalid, "binance account trading permission is absent or disabled")
	}
	balances, err := e.FetchBalance(map[string]interface{}{banexg.ParamContext: ctx, banexg.ParamAccount: account, banexg.ParamFullSnapshot: true, banexg.ParamSettledCash: true})
	if err != nil {
		return proof, err
	}
	if balances == nil {
		return proof, errs.NewMsg(errs.CodeDataNotFound, "balance snapshot absent")
	}
	if allowed, known := banexg.BoolFromInfo(balances.Info, "canTrade"); !known || !allowed {
		return proof, errs.NewMsg(errs.CodeParamInvalid, "binance account trading is absent or disabled in balance snapshot")
	}
	if _, ok := balances.Total[currency]; !ok {
		return proof, errs.NewMsg(errs.CodeDataNotFound, "settled currency %s absent from balance snapshot", currency)
	}
	if _, err = e.FetchPositions(nil, map[string]interface{}{banexg.ParamContext: ctx, banexg.ParamAccount: account}); err != nil {
		return proof, err
	}
	if _, err = e.FetchOpenOrders("", 0, 0, map[string]interface{}{banexg.ParamContext: ctx, banexg.ParamAccount: account, banexg.ParamFullSnapshot: true}); err != nil {
		return proof, err
	}
	proof.ContextBound = true
	proof.StableClientID = true
	proof.QueryClientID = true
	proof.CompleteCumulativeReports = true
	proof.CompleteAccountSnapshot = true
	proof.SettledCash = true
	proof.NetLinearPositions = true
	proof.EvidenceID = fmt.Sprintf("binance-linear:%s:%s:%s", e.ID, account, currency)
	return proof, nil
}
