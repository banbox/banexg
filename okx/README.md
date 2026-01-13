
## OKX instFamily / uly 参数说明
以下说明以 BTC 合约为例，其他币种的合约同理。

- uly 是指数，如："BTC-USD"，与盈亏结算和保证金币种 (settleCcy) 会存在一对多的关系。
- instFamily 是交易品种，如：BTC-USD_UM，与盈亏结算和保证金币种 (settleCcy) 一一对应。
- 以下表格详细展示了 uly, instFamily，settleCcy 和 instId 的对应关系。

| 合约类型 | uly | instFamily | settleCcy | 交割合约 instId | 永续合约 instId |
| --- | --- | --- | --- | --- | --- |
| USDT 本位合约 | BTC-USDT | BTC-USDT | USDT | BTC-USDT-250808 | BTC-USDT-SWAP |
| USDC 本位合约 | BTC-USDC | BTC-USDC | USDC | BTC-USDC-250808 | BTC-USDC-SWAP |
| USD 本位合约 | BTC-USD | BTC-USD_UM | USDⓈ | BTC-USD_UM-250808 | BTC-USD_UM-SWAP |
| 币本位合约 | BTC-USD | BTC-USD | BTC | BTC-USD-250808 | BTC-USD-SWAP |

注意：
1. USDⓈ 代表 USD 以及多种 USD 稳定币，如：USD, USDC, USDG。
2. 盈亏结算和保证金币种指的获取交易产品基础信息（私有）接口返回的 settleCcy 字段。
