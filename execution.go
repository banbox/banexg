package banexg

import (
	"context"

	"github.com/banbox/banexg/errs"
)

// ParamContext binds a synchronous SDK request to the caller's cancellation
// and deadline. Exchange adapters consume and remove this value before
// encoding request parameters.
const ParamContext = "context"

// ParamCompleteOrder asks order queries to return a complete cumulative
// quantity, cost, fee and terminal-state snapshot. Implementations must fail
// closed when the venue cannot prove completeness.
const ParamCompleteOrder = "completeOrder"

// ExecutionProof is the venue capability evidence consumed by higher-level
// execution engines. Flags describe observed, verified behavior; they are not
// configuration declarations.
type ExecutionProof struct {
	Account                   string
	Currency                  string
	EvidenceID                string
	ContextBound              bool
	StableClientID            bool
	QueryClientID             bool
	AuthoritativeNotFound     bool
	CompleteCumulativeReports bool
	CompleteAccountSnapshot   bool
	SettledCash               bool
	NetLinearPositions        bool
	PostOnly                  bool
}

// ExecutionCapability is an optional adapter-owned live execution contract.
// VerifyExecution must perform only read-only checks; all subsequent SDK
// calls receive ParamContext and are expected to honor its cancellation.
type ExecutionCapability interface {
	VerifyExecution(context.Context, string, string) (ExecutionProof, *errs.Error)
}

type FundingCash struct {
	ID       string
	Symbol   string
	Currency string
	Amount   string
	Mark     string
	Rate     string
	AtMS     int64
}

type FundingCashCapability interface {
	FetchFundingCash(context.Context, string, string, int64, int64) ([]FundingCash, *errs.Error)
}
