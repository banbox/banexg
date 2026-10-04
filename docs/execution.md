# Optional live execution capabilities

`ExecutionCapability` and `FundingCashCapability` are optional interfaces. They do not add methods to `BanExchange`; existing adapters continue to implement that interface unchanged. Consumers must check capability support before starting shared live execution.

`VerifyExecution(ctx, account, currency)` performs read-only account checks and returns an `ExecutionProof`. The Binance implementation supports linear contracts in one-way position mode. It checks account access, settled currency cash, positions and open orders. Unsupported modes, absent snapshots or unproven completeness fail explicitly. It does not switch account modes or submit orders.

Pass `ParamContext` in request parameters to bind supported execution requests to cancellation and deadlines. The adapter removes this transport parameter before encoding venue requests and preserves the caller's parameter map.

Private-stream initialization carries the request context through market loading, authentication and the first WebSocket handshake. Established streams, reconnects and listen-key refreshes use their own lifecycle after initialization. The legacy shared market loader still serializes loads behind a global lock; cancellation does not interrupt waiting for that lock.

`ParamCompleteOrder` requests cumulative executed quantity, cost, fee and terminal state. Binance combines the order snapshot with paginated executions and returns an error when it cannot prove completeness. `ParamSettledCash` requests wallet cash for linear balances rather than margin equity.

`FetchFundingCash` returns settled funding cash for an account, currency and inclusive millisecond range, with stable identifiers and resolved symbols. Binance accepts windows of at most seven days. Pagination and identity checks reject malformed or repeated pages; consumers must supply actual settlement records or explicitly choose a zero-funding policy.

`ExecutionProof` reports the evidence supported by the adapter. The Binance implementation leaves `AuthoritativeNotFound` and `PostOnly` false; callers must not infer these guarantees from a successful verification. A local test pass is not a real-account acceptance result.
