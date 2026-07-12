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
