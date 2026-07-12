# Exchange-Neutral Error Codes

`errs.Error.Code` is the public error contract. `BizCode` is deprecated and is
always zero for exchange responses; exchange-native numeric/string codes are
not included in public error text.

The shared taxonomy keeps transport/runtime errors and adds trading semantics:

| Code | Meaning |
| --- | --- |
| `CodeRateLimit` | Request quota exceeded |
| `CodeTemporarilyBanned` | Temporary client/IP ban |
| `CodeExecutionUnknown` | The exchange did not confirm whether a trading request executed |
| `CodeExchangeError` | Unclassified exchange rejection |
| `CodeSymbolInvalid` | Unknown or invalid instrument |
| `CodeMarketUnavailable` | Instrument is closed, delisted, or unavailable |
| `CodeOrderNotFound` | Order does not exist |
| `CodeOrderRejected` | Order rejected without a more specific category |
| `CodeInsufficientFunds` | Wallet/available balance is insufficient |
| `CodeInsufficientMargin` | Position/order margin is insufficient |
| `CodeRiskLimit` | Position, order, leverage, or risk-tier limit exceeded |
| `CodePositionModeConflict` | Position/margin mode conflicts with the operation |
| `CodeReduceOnlyRejected` | Reduce-only constraints reject the order |
| `CodeDuplicateRequest` | Duplicate request or client order ID |
| `CodeNoChange` | Requested state is already active |
| `CodeAccountRestricted` | Trading/account/region/compliance restriction |
| `CodeStreamExpired` | User stream token is absent or expired |
| `CodeOrderWouldTrigger` | Trigger order would execute immediately |
| `CodeOrderNotCancelable` | Order is final or cannot be canceled |
| `CodeLeverageInvalid` | Leverage cannot be changed in the current account mode |
| `CodePrecisionViolation` | Amount/price precision or step is invalid |

## Mapping rules

Mappings are maintained in `binance/errors.go`, `okx/errors.go`, and
`bybit/common_util.go`. They apply exact native codes before ranges or message
fallbacks. Ambiguous native codes use the official message semantics; unknown
codes map to `CodeExchangeError`.

HTTP 418/429 map to temporary ban/rate limit. A failed HTTP 5xx response for a
risky trading endpoint maps to `CodeExecutionUnknown`, preventing callers from
blindly retrying a potentially successful order. Non-trading 5xx responses map
to `CodeServerError`.

Callers should compare only `err.Code`, for example:

```go
if err.Code == errs.CodeOrderNotFound {
	// Reconcile local order state.
}
```
