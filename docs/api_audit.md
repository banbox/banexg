# Exchange API Audit

Audit date: 2026-07-11

The repository and upstream history contain Binance, OKX, and Bybit adapters.
There is no Bitget package, branch, tag, or historical path; the requested
third exchange was therefore audited as Bybit.

## Reproducing the documentation snapshot

```bash
python3 docs/crawler.py --output /tmp/banexg-api-docs
python3 docs/crawler.py --inventory-only --output /tmp/banexg-api-inventory
```

The crawler derives its inventory from `Method*` constants referenced by
production Go files and intersects them with each `entry.go`. It downloads:

- Binance: official `llms-full.txt`
- OKX: official API V5 HTML
- Bybit: official `bybit-exchange/docs` source archive

Each output contains the filtered official sections, the REST inventory, the
source URL, fetch time, source/output SHA-256, and any unmatched routes. The
pre-fix audit found 91 Binance, 29 OKX, and 23 Bybit production REST routes.
After removing obsolete margin stream routes and adding `userListenToken`, the
current generated inventory contains 89 Binance routes.

## Confirmed drift and resolution

### Binance

- USD-M WebSocket traffic now uses `/public`, `/market`, and `/private` routed
  endpoints. Client allocation and URL construction now include the route.
- Contract trades now subscribe to `aggTrade`; options use `optionTrade` and
  spot keeps `trade`.
- Funding-rate requests now send the exchange symbol ID.
- Removed margin listen-key REST calls were replaced with `userListenToken`
  and WebSocket API `userDataStream.subscribe.listenToken`.
- Options order responses now retain `selfTradePreventionMode` in the typed
  response.

Spot listen-key REST is deprecated but has no confirmed production removal
date, so it remains for compatibility. Migrating spot private streams to the
WebSocket API is a separate lifecycle change.

### OKX

- Algo-order history now supplies required `ordType` and `state`/`algoId`,
  removes unsupported time parameters, fans out supported types/states, and
  filters/deduplicates locally.
- Pending algo orders include `twap` and `chase`, which this SDK can create.
- X-Perps FUTURES funding rates are accepted and filtered from `instId=ANY`.
- OHLCV limits are clamped to the documented maximum of 300.
- Deprecated `nextFundingRate` is no longer treated as predictive data.

WebSocket methods/channels match the current docs. Sequence/checksum validation
for OKX order books remains a robustness improvement, not an endpoint drift.

### Bybit

- Option mark-price OHLCV is supported with the documented maximum of 500;
  unsupported option kline variants remain rejected.
- REST order-book limits are 1000 for spot/linear/inverse and 25 for options.
- Funding history paginates backward by the oldest settlement timestamp and
  now supports windows larger than 200 rows with deduplication and filtering.
- Position parsing no longer derives margin mode from deprecated `tradeMode`;
  it uses explicit account `marginMode` or leaves the value unknown.

Coin-level available balance in unified cross/portfolio accounts remains an
exchange-defined limitation. The current `Free` calculation was not changed
without a reliable replacement field.

## Verification boundary

Mocked request/response tests cover the changed routes, parameters, pagination,
WebSocket URL selection, and error mapping. Public smoke requests covered
Binance spot/USD-M/COIN-M/options, OKX instruments/ticker/books, and Bybit
instruments/ticker/orderbook. Authenticated order lifecycle and private stream
tests require local credentials and must use demo/test accounts.

# Exchange API Audit — 2026-11-26

Scope: official changelogs from 2025-11 to 2026-11-26, checked against the
adapters. Run tests with `GOTOOLCHAIN=go1.25.8` (sonic v1.14.1 does not build
on Go 1.26+).

## Fixed

### Binance

- Spot user data stream: `POST/PUT/DELETE /api/v3/userDataStream` were retired
  on 2026-02-20 (the endpoint answers `410 Gone`). Spot now subscribes on the
  WebSocket API with `userDataStream.subscribe.signature`; no listenKey and no
  keep-alive are needed.
  `eventStreamTerminated` renews the subscription; `serverShutdown` is logged.
- Signed REST requests now sign the percent-encoded payload (enforced since
  2026-01-15, otherwise `-1022`). This matters for non-ASCII symbols and the
  `orderidlist` of batch cancels.
- Options listenKey used the spot route; it now uses `eapi` listenKey routes.
- COIN-M stop orders: `POST /dapi/v1/order` will reject stop types with `-4120`
  at a date Binance has not announced. The plain route is still tried first and
  the order is re-sent through `POST /dapi/v1/algoOrder` on that rejection.
  Cancel and open-order queries for `algo:` ids use the dapi algo routes.
  Single/history algo queries are not documented for COIN-M and return
  `CodeNotSupport`.
- Spot testnet WebSocket hosts are `stream.testnet.binance.vision` and
  `ws-api.testnet.binance.vision` (the old host answers 404).

### OKX

- WebSocket port 8443 was retired (public 2026-10-31, colo 2026-11-16); URLs no
  longer carry a port.
- Demo trading requests carry `x-simulated-trading: 1`. Without it `Env=test`
  REST calls went to the production environment.
- A caller-supplied `tradeQuoteCcy` is kept (needed to trade in USD on the
  merged Crypto-USDC books, 2026-09-30).
- Instrument state `post_only` counts as active.
- Order books validate `seqId`/`prevSeqId` (checksum is fixed to 0 since
  2026-06-23) and resubscribe on a gap.
- Repeated fill pushes on the orders channel are de-duplicated.
- Error codes 54094, 54109 and 59113 map to `CodeForbidden`.
- `notice` events are logged (64008 at warn).

### Bybit

- `/v5/account/transaction-log` cost matches the 25 req/s limit.
- Option markets are loaded with `baseCoin=All`; the default returned BTC only.
- `slippageTolerance`/`slippageToleranceType` are accepted for options.
- Spot `Limits.Amount.Min` no longer uses the deprecated `minOrderQty`;
  `Limits.Cost.Min` (`minOrderAmt`) is the minimum.

### Hosts, environments and regional sites

- Core: `ExgHosts.Variants` holds named host override sets. `OptEnv` picks
  production, `test` or a named environment; `OptSite` layers a regional site
  (`<site>-<env>` inside a non-production environment); `OptHosts` overrides
  single host keys. Unknown names fail at construction.
- OKX: sites `eea` (`eea.okx.com`, `wseea`/`wseeapap`) and `us` (`us.okx.com`,
  `wsus`/`wsuspap`).
- Bybit: environment `demo` (`api-demo.bybit.com`, private stream on
  `stream-demo.bybit.com`, market data from mainnet) and sites `tr`, `kz`,
  `ge`, `id`, `jp`, `hk`, plus REST-only `ae` and `eu` (the docs list no stream
  host for them) and REST-only `jp-test`, `hk-test`.
- Binance margin user data uses `POST/PUT /sapi/v2/margin/listen-key` and
  `wss://fstream.binance.com/private/ws/<listenKey>` for every account; the
  listenToken subscription is gone.
- Binance option streams moved to `wss://fstream.binance.com/{public,market}`
  (depth and bookTicker on `public`, the rest on `market`) and option user data
  to `/private/ws/<listenKey>`; `nbstream.binance.com/eoptions` answers 404.
- Binance futures testnet uses `demo-fapi`, `demo-dapi`, `demo-fstream` and
  `demo-dstream`. Options stay excluded in the sandbox: `demo-fapi` answers
  403 for `/eapi`.

## Open items

- Binance COIN-M: whether `ALGO_UPDATE` is pushed on the COIN-M user stream is
  not documented.
- OKX 64008 notice does not trigger a proactive reconnect.
- Bybit portfolio-margin accounts: `totalOrderIM`/`totalPositionIM` are empty,
  so per-coin `Free` is overstated; the docs give only an account-level USD
  figure.
- Bybit split executions (`ForwardSplitSettle`, `ReverseSplitSettle`,
  `ForwardSplitOpen`, `ReverseSplitOpen`) are filtered out of my-trades.
- Bybit `entry.go` still lists about 150 retired v2/v3 routes.
