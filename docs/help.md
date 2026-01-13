# BanExg 后端项目概述

## 项目整体概述
BanExg 是一个用Go语言开发的多交易所统一SDK类库，旨在为加密货币交易提供统一的API接口。目前已完整支持Binance和OKX交易所，部分支持Bybit交易所，并支持中国期货市场本地模拟。支持现货、U本位合约、币本位合约、期权等多种市场类型，提供REST API和WebSocket双协议支持。

## 技术架构与实现方案
- **接口抽象**: BanExchange统一接口定义，100+方法覆盖市场数据、交易操作、账户管理、实时数据
- **四层架构**: 接口抽象层、业务逻辑层、适配器层、基础设施层清晰分离
- **多交易所适配**: 工厂模式动态注册，各交易所独立包实现统一接口
- **并发安全**: 使用deadlock进行死锁检测，全局状态管理使用RWMutex保护
- **错误处理**: 自定义Error类型，包含错误码、堆栈跟踪、业务码，支持错误链传递
- **重试机制**: 可配置的重试策略，支持按方法名和错误类型定制重试次数和等待时间
- **速率控制**: 支持请求速率限制，自动计算API权重消耗，域名级并发控制
- **WebSocket管理**: 自动重连、消息队列、心跳保活、订阅恢复、录制与回放功能
- **精度处理**: 支持DecimalPlace和TickSize两种精度模式，decimal库保证计算精度
- **日志系统**: 基于zap的高性能日志，支持文件轮转、分级输出、上下文注入
- **代理支持**: 自动解析系统代理或环境变量代理配置

## 核心文件索引

### 根目录 - 核心接口与基础实现

#### 接口定义
- **intf.go**: BanExchange核心接口，定义100+统一方法（LoadMarkets/FetchTicker/FetchOHLCV/CreateOrder/CancelOrder/WatchOrderBooks等），所有交易所必须实现
- **types.go**: 核心数据结构，Exchange基础实现（ExgInfo/Accounts/Markets/WSClients/Options配置），Account多账户管理，Market/Order/Position/Balances/Kline/OrderBook等交易数据类型，WebSocket相关结构（WsClient/AsyncConn/WebSocket）

#### 业务逻辑实现
- **biz.go**: Exchange通用业务逻辑，Init初始化（代理配置/凭证解析/速率限制/WS录制回放），LoadMarkets市场加载，FetchTicker/FetchOHLCV行情获取，CreateOrder/CancelOrder订单操作，FetchBalance/FetchPositions账户查询，WS录制与回放（SetDump/SetReplay/ReplayOne/ReplayAll）
- **common.go**: 通用工具函数，Precision/LimitRange/MarketLimits/Balances等结构体辅助方法，精度格式化ToString，Balances初始化Init，OrderBook/OdBookSide订单簿操作（SetSide/Update/Set/SumVolTo/AvgPrice/Level/Reset），Kline克隆，MergeMyTrades合并交易为订单，IsOrderDone订单状态判断，Host重试等待管理（GetHostRetryWait/SetHostRetryWait），HTTP流量控制GetHostFlowChan，SetBoolArg布尔参数格式化
- **base.go**: 基础功能，ExgHosts主机选择（TestNet/Prod切换），Credential凭证校验CheckFilled，IsContract市场类型判断

#### 常量与配置
- **data.go**: 参数常量定义（ParamClientOrderId/ParamTimeInForce/ParamTriggerPrice等50+），默认配置（DefReqHeaders/DefCurrCodeMap/DefWsIntvs/DefRetries），状态常量（BoolNull/BoolTrue/BoolFalse），Options配置键（OptProxy/OptApiKey/OptEnv等25+），精度模式常量（PrecModeDecimalPlace/PrecModeSignifDigits/PrecModeTickSize），市场/合约/保证金类型常量，订单状态/类型/方向/持仓方向常量，TimeInForce常量，Api方法名常量（ApiFetchTicker/ApiCreateOrder等30+），市场缓存管理（exgCacheMarkets/exgMarketTS/exgMarketExpireMins），HTTP并发控制HostHttpConcurr

#### WebSocket实现
- **websocket.go**: WebSocket客户端实现，WsClient连接管理（多连接池/订阅恢复/超时检测），AsyncConn异步消息发送，WebSocket底层封装（自动重连/读写分离），订阅管理（SubscribeKeys/SubsKeyStamps），深度订阅odBookLimits

#### 扩展工具
- **exts.go**: 日志扩展，HttpHeader实现zapcore.ObjectMarshaler接口用于日志输出
- **leverage.go**: 杠杆档位结构定义，BaseLvgBracket基础档位（Bracket/InitialLeverage/MaintMarginRatio/Cum），LvgBracket标准化档位（含Capacity/Floor），SymbolLvgBrackets品种杠杆档位集合，ISymbolLvgBracket接口用于交易所特定格式转换

### 基础设施层

#### bex/ - 交易所工厂
- **common.go**: 工厂函数类型定义FuncNewExchange
- **entrys.go**: 交易所注册表，init注册binance/bybit/china/okx四个交易所，New工厂方法动态创建交易所实例

#### errs/ - 错误处理
- **types.go**: Error结构体（Code/msg/Stack/err/BizCode/Data）
- **main.go**: 错误创建（NewFull/NewMsg/New），错误格式化（Short/Error/Message），堆栈跟踪CallStack，错误码名称管理UpdateErrNames
- **data.go**: 错误码常量定义（CodeNetFail/CodeNotSupport/CodeParamInvalid/CodeUnmarshalFail/CodeTimeout/CodeNoTrade等40+个错误码），错误码名称映射表errCodeNames，PrintErr自定义错误打印函数

#### log/ - 日志系统
- **log.go**: 日志核心，InitLogger初始化配置（文件/控制台输出），ReplaceGlobals全局日志替换，分级日志获取（debugL/infoL/warnL/errorL），Sync日志刷新，LogFilePath获取日志文件路径
- **config.go**: 日志配置，FileLogConfig文件日志配置（LogPath/MaxSize/MaxDays/MaxBackups），Config日志配置（Level/Format/Stdout/Development等），ZapProperties日志属性封装
- **global.go**: 全局日志函数，Debug/Info/Warn/Error/Panic/Fatal日志输出，With字段附加，SetLevel动态调整日志级别，WithTraceID/WithReqID/WithModule上下文日志，Ctx获取上下文日志器

#### utils/ - 工具函数库
- **common.go**: 时间工具（YMD日期格式化/ISO8601转换），字符串分析SplitParts（按数字/字符串/浮点分段），时间范围解析ParseTimeRanges
- **crypto.go**: 加密签名，Signature统一签名接口（支持rsa/eddsa/hmac三种方法，sha256/sha384/sha512哈希，base64/hex摘要），HMAC/Eddsa/rsaSign具体实现，Latin1编码转换
- **dec_precs.go**: 精度处理，DecimalToPrecision精度格式化（DECIMAL_PLACES/SIGNIFICANT_DIGITS/TICK_SIZE三种模式），TruncateToString/RoundToString精度截断与四舍五入
- **exg.go**: 交易所工具，ParseTimeFrame解析时间周期字符串，timeFrame标准化映射
- **file.go**: 文件操作，ReadFile/WriteFile/AppendFile文件读写，PathExists路径检查，MkdirAll目录创建
- **misc.go**: 杂项工具，UUID生成随机ID，ArrSum/ArrContains数组操作，UrlEncodeMap/EncodeURIComponent URL编码，GetMapVal/PopMapVal/SafeMapVal泛型字典取值，SetFieldBy字段设置，OmitMapKeys删除键，MapValStr值转字符串，SafeParams安全复制参数，EqualNearly浮点数近似相等，GetSystemProxy/GetSystemEnvProxy代理配置，Marshal/Unmarshal序列化（基于sonic），KeysOfMap/ValsOfMap字典键值提取，DecodeStructMap结构体转字典，IsNil空值判断
- **num_utils.go**: 数值工具，ParseNum字符串转数值
- **text.go**: 文本工具，UcFirst首字母大写
- **tf_utils.go**: 时间周期工具，ParseTimeDelta解析时间增量，FmtTimeFrame格式化时间周期，TFSecsMap时间周期秒数映射

### 交易所实现层

#### binance/ - Binance交易所完整实现
- **entry.go**: 交易所入口，New构造函数（ExgInfo/Hosts配置/Fees费率/Apis路由表），TestNet和Prod双环境配置，支持现货/杠杆/U本位/币本位/期权五大市场
- **data.go**: 常量定义，Host类型常量（HostDApiPublic/HostFApiPrivate/HostSApi/HostPApi等），OptRecvWindow配置项，订单状态常量（OdStatusNew/OdStatusFilled/OdStatusCanceled等），400+ MethodXXX方法名常量（涵盖现货/杠杆/合约/期权所有API）
- **types.go**: 数据结构，Binance主结构体（RecvWindow/streamBySubHash/streamLimits/wsRequestId/LeverageBrackets），Bnb前缀的原始响应结构体（BnbCurrency/BnbNetwork/BnbMarket/BnbFilter/BnbOptionKline等），账户结构（SpotAccount/SpotAsset/MarginCrossBalances），LinearSymbolLvgBrackets/InversePairLvgBrackets杠杆档位
- **biz.go**: 业务逻辑入口，Init初始化（streamLimits/wsRequestId/重放处理注册/CalcRateLimiterCost），makeSign签名函数（HMAC-SHA256签名/recvWindow时间窗口/账户权限检查/NoTrade限制），markRiskyApis危险API标记（order/leverage/transfer等路径）
- **biz_account.go**: 账户相关，账户信息查询
- **biz_balance.go**: 资产查询，FetchBalance资产余额（现货/杠杆/合约统一处理），parseBalance余额解析（Spot/Margin/Linear/Inverse四种市场），FetchPositions持仓查询（Linear/Inverse合约），parsePosition持仓解析
- **biz_order.go**: 订单查询，FetchOrder单个订单查询，FetchOrders历史订单列表，FetchOpenOrders未完成订单，parseOrder泛型订单解析器（SpotOrder/MarginOrder/LinearOrder/InverseOrder），订单状态映射parseOrderStatus
- **biz_order_create.go**: 订单创建，CreateOrder下单（Spot/Margin/Linear/Inverse/Option），EditOrder改单，参数校验与转换（amount/price/stopPrice/leverage）
- **biz_order_algo.go**: 算法订单，条件单/止盈止损单创建，算法订单查询与取消
- **biz_order_book.go**: 订单簿，FetchOrderBook深度数据查询
- **biz_ticker.go**: 行情数据，FetchTicker单个行情，FetchTickers批量行情，parseTickers泛型行情解析器（SpotTicker/LinearTicker/InverseTicker/OptionTicker），FetchOHLCV K线数据（Spot/Linear/Inverse/Option），FetchLastPrices最新价，FetchFundingRate/FetchFundingRates资金费率
- **common.go**: 通用转换，BnbMarket.GetPrecision精度提取（QuantityPrecision/PricePrecision/QuantityScale/PriceScale），BnbMarket.GetMarketLimits限额转换（PRICE_FILTER/LOT_SIZE/MARKET_LOT_SIZE/MIN_NOTIONAL/NOTIONAL过滤器），LinearSymbolLvgBrackets/InversePairLvgBrackets.ToStdBracket杠杆档位标准化转换
- **ws_biz.go**: WebSocket业务，makeHandleWsMsg消息路由函数（depthUpdate/trade/kline/markPriceUpdate/24hrTicker/executionReport等20+事件），handleOrderBook/handleTrade/handleOHLCV/handleBalance等具体处理器
- **ws_order.go**: WebSocket订单，WatchMyTrades我的成交监听，WatchBalance资产变动监听，WatchPositions持仓变动监听，WatchAccountConfig账户配置监听

#### bybit/ - Bybit交易所部分实现
- **entry.go**: 交易所入口，New构造函数（支持Spot/Linear/Inverse/Option），Apis路由表（涵盖v3/v5两个版本API），TestNet和Prod环境配置，费率配置（Main/Linear/Inverse/Option）
- **data.go**: 常量定义，Host类型（HostPublic/HostPrivate），300+ MethodXXX方法名常量（Spot/Derivatives/V5多版本），OptRecvWindow配置项，订单状态/时间周期映射表
- **types.go**: 数据结构，Bybit主结构体（RecvWindow），原始响应结构体
- **biz.go**: 业务逻辑，Init初始化，makeSign签名函数（HMAC-SHA256/X-BAPI-*头部/openapi与v5双模式签名），markRiskyApis危险API标记，LoadMarkets市场加载，FetchBalance资产查询
- **biz_ticker.go**: 行情实现，FetchTicker/FetchTickers数据解析与转换

#### okx/ - OKX交易所完整实现（新增）
- **entry.go**: 交易所入口，New构造函数（支持Spot/Linear/Inverse/Option），Apis路由表，REST和WebSocket三端点配置（Public/Private/Business），Has能力声明，CredKeys需Password
- **data.go**: 常量定义，Host类型（HostPublic/HostPrivate/HostWsPublic/HostWsPrivate/HostWsBusiness），OKX字段常量（FldInstType/FldInstId/FldMgnMode等），WebSocket通道名（WsChanTrades/WsChanBooks/WsChanOrders等），instType常量（SPOT/MARGIN/SWAP/FUTURES/OPTION），20+ MethodXXX方法名常量，orderStatusMap/orderTypeMap映射
- **types.go**: 数据结构，OKX主结构体（LeverageBrackets），Okx前缀原始响应结构体（OkxInstrument/OkxTicker/OkxOrderBook/OkxBalance/OkxPosition/OkxOrder/OkxWsOrder/OkxFundingRate等）
- **biz.go**: 业务逻辑入口，Init初始化，makeSign签名函数（OK-ACCESS-*头部/HMAC-SHA256/base64摘要/Passphrase），markRiskyApis危险API标记，requestRetry泛型请求函数，makeFetchMarkets市场加载
- **biz_balance.go**: 资产查询，FetchBalance余额（统一账户模式）
- **biz_order.go**: 订单操作，CreateOrder/EditOrder/CancelOrder/FetchOrder/FetchOrders/FetchOpenOrders
- **biz_ticker.go**: 行情数据，FetchTicker/FetchTickers/FetchOHLCV/FetchOrderBook/FetchFundingRate
- **biz_leverage.go**: 杠杆管理，LoadLeverageBrackets加载杠杆档位，GetLeverage获取当前杠杆，SetLeverage设置杠杆，CalcMaintMargin计算维持保证金
- **biz_account_history.go**: 账户历史，FetchIncomeHistory账单流水查询（支持archive归档）
- **common.go**: 通用工具，marketToInstType市场类型映射，parseMarketType/parseInstrument品种解析，parseOrder/parseTicker数据转换，getInstType/parseWsKey辅助函数
- **ws_biz.go**: WebSocket业务，makeHandleWsMsg消息路由（trades/books/balance_and_position/orders/mark-price/candle），WatchOrderBooks/WatchTrades/WatchOHLCVs/WatchMarkPrices订阅，WatchMyTrades/WatchBalance/WatchPositions私有订阅，wsLogin认证

#### china/ - 中国期货交易所（本地模拟）
- **entry.go**: 交易所入口，New构造函数，无网络请求的本地模拟交易所
- **data.go**: 常量定义，defTimeLoc默认时区
- **types.go**: 数据结构，China主结构体，Exchange交易所信息（Code/Title/IndexUrl/Suffix/CaseLower/DateNum/OptionDash），ItemMarket品种描述（Code/Title/Market/Exchange/Multiplier/PriceTick/LimitChgPct/MarginPct/DayRanges/NightRanges/Fee/Alias），Fee手续费结构（Unit/Val/ValCT/ValTD），CnMarkets配置结构
- **biz.go**: 业务逻辑，Init初始化，loadRawMarkets从markets.yml加载品种配置（embed嵌入），LoadMarkets市场加载，MapMarket品种ID映射（支持年度合约/主连/指数自动识别），parseMarket解析品种符号（处理期货/期权/现货），GetLeverage获取杠杆，CalcMaintMargin计算维持保证金，makeCalcFee手续费计算（wan万分之/lot每手）
- **common.go**: 通用工具，ItemMarket.Resolve继承解析，ItemMarket.ToStdSymbol/ToRawSymbol品种符号转换，Fee.ParseStd手续费标准化
- **markets.yml**: 品种配置文件（embed嵌入），包含期货交易所/合约品种基础信息（合约乘数/最小变动价位/涨跌停板/保证金比率/交易时间/手续费等）

## 项目特点

1. **统一接口**: BanExchange接口定义统一标准，一套代码支持多交易所
2. **完整覆盖**: 支持REST和WebSocket双协议，涵盖市场数据、交易、账户全链路
3. **生产就绪**: 完善的错误处理、重试机制、速率控制、并发安全
4. **精度保证**: decimal库保证数值计算精度，支持多种精度模式
5. **高性能**: WebSocket自动重连、消息批处理、连接池复用
6. **可测试**: 支持WebSocket录制与回放，便于调试和单元测试
7. **可扩展**: 工厂模式+接口抽象，新增交易所只需实现BanExchange接口

## 开发指南

### 添加新交易所
1. 在新包中定义交易所结构体，嵌入`*banexg.Exchange`
2. 实现`BanExchange`接口的所有方法
3. 在`bex/entrys.go`中注册工厂函数
4. 参考`binance`包的实现模式

### Binance开发规范
1. **新增REST API**: data.go定义Method常量 → entry.go注册Apis路由 → types.go定义响应结构 → biz_*.go实现业务方法
2. **市场与参数**: 所有业务方法首行调用`LoadArgsMarket`预处理参数和市场
3. **Host选择**: Spot用HostPublic/HostPrivate，U本位用HostFApi*，币本位用HostDApi*，杠杆用HostSApi*，期权用HostEApi*
4. **错误处理**: 统一使用`*errs.Error`，参数错误用`errs.CodeParamInvalid`
5. **WebSocket**: 在`ws_biz.go`的`makeHandleWsMsg`中根据`item.Event`路由到具体处理器

### 通用开发原则
- 严格遵守DRY原则，避免重复代码，提取公共函数
- 保持代码简洁，只实现当前需要的功能，不过度设计
- 所有导出函数添加注释说明参数和返回值
- 使用`utils`包提供的工具函数，避免重复实现
- 并发访问共享状态必须加锁保护
