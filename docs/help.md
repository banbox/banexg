# BanExg 后端项目概述

## 项目整体概述
BanExg 是一个用Go语言开发的多交易所统一SDK类库，旨在为加密货币交易提供统一的API接口。目前已完整支持Binance和OKX交易所，部分支持Bybit交易所，并支持中国期货市场本地模拟。支持现货、杠杆、U本位合约、币本位合约、期权等多种市场类型，提供REST API和WebSocket双协议支持。

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
- **intf.go**: BanExchange核心接口，定义100+统一方法（LoadMarkets/FetchTicker/FetchOHLCV/CreateOrder/CancelOrder/FetchBalance/WatchOrderBooks/WatchBalance等），支持市场数据/交易操作/账户管理/实时订阅/精度处理/录制回放
- **types.go**: 核心数据结构，Exchange基础实现（ExgInfo/Apis/Accounts/Markets/WSClients/Sign/OnWsMsg等），ExgInfo交易所元信息（ID/Name/Markets/DebugAPI等），Account多账户管理（Name/Creds/MarBalances等），函数类型定义（FuncSign/FuncOnWsMsg/FuncCalcFee等）

#### 业务逻辑实现
- **biz.go**: Exchange通用业务逻辑，Init初始化（HttpClient/代理解析/速率控制/重试策略/录制回放/环境切换/市场筛选/调试开关等配置项），SafeCurrency币种安全获取
- **biz_account.go**: 账户访问权限，AccountAccess结构（TradeAllowed/WithdrawAllowed/PosMode/AcctMode等），FetchAccountAccess权限提取，FillAccountAccessFromInfo权限解析，NormalizePosMode持仓模式标准化
- **common.go**: 通用工具函数，Balances.Init余额初始化，OrderBook订单簿操作（Update/SetSide/AvgPrice等），IsOrderDone订单状态判断，GetHostRetryWait重试等待，SetBoolArg参数格式化
- **base.go**: 基础功能，ExgHosts.GetHost主机获取（TestNet/Prod/Test自动切换），Credential.CheckFilled凭证校验（ApiKey/Secret/UID/Password必填检查），IsContract市场类型判断（future/swap/linear/inverse）

#### 常量与配置
- **data.go**: 参数常量60+个（ParamClientOrderId/ParamTriggerPrice/ParamMarginMode/ParamPositionSide等），默认配置（DefReqHeaders/DefRetries/HostHttpConcurr等），精度模式（PrecModeDecimalPlace/PrecModeTickSize等），Has常量（HasFail/HasOk/HasEmulated）

#### WebSocket实现
- **websocket.go**: WsClient连接管理（conns连接池/SubscribeKeys/JobInfos/OnMessage/OnError等回调），AsyncConn异步消息处理，支持自动重连/订阅恢复/心跳保活/多连接池

#### 扩展工具
- **exts.go**: HttpHeader实现zapcore.ObjectMarshaler接口用于日志输出
- **tickers.go**: Ticker工具函数，BuildSymbolSet符号集合构建，FilterTickers按符号集过滤，TickersToLastPrices转最新价列表，TickersToPriceMap转价格映射表

### 基础设施层

#### bex/ - 交易所工厂
- **common.go**: FuncNewExchange工厂函数类型定义，WrapNew泛型包装器将具体交易所构造函数转换为统一接口
- **entrys.go**: 交易所注册表，init注册binance/bybit/china/okx四个交易所到newExgs映射，New工厂方法根据name动态创建交易所实例

#### errs/ - 错误处理
- **types.go**: Error结构体（Code/msg/Stack/err/BizCode/Data）
- **main.go**: 错误创建（NewFull全参数/NewMsg仅消息/New仅错误），错误格式化（Short简短/Error详细/Message消息/CodeName码名），堆栈跟踪CallStack（skip跳过层级/maxNum最大数量），错误码名称管理UpdateErrNames，支持错误链Unwrap
- **data.go**: 错误码常量48个（CodeNetFail/CodeInvalidRequest/CodeSignFail/CodeParamInvalid/CodeConnectFail/CodeTimeout/CodeUnauthorized/CodeServerError等），errCodeNames映射表，PrintErr错误打印

#### log/ - 日志系统
- **log.go**: 日志核心，InitLogger初始化（File/Stdout/Level/Handlers），InitLoggerWithWriteSyncer自定义输出，newStdLogger标准日志器，全局日志原子存储，LogFilePath获取日志文件路径
- **config.go**: FileLogConfig文件配置（LogPath/MaxSize/MaxDays/MaxBackups），Config日志配置（Level/Format/Stdout/File/Sampling等），ZapProperties日志属性，newZapTextEncoder编码器
- **global.go**: 全局日志函数（Debug/Info/Warn/Error等），With字段附加，SetLevel/GetLevel级别管理，WithTraceID/WithModule上下文日志

#### utils/ - 工具函数库
- **common.go**: 时间工具（YMD日期格式化/ISO8601转换），SplitParts字符串分段，ParseTimeRanges时间范围解析
- **crypto.go**: Signature统一签名接口（method支持rsa/eddsa/hmac，hashName支持sha256等，digest支持base64/hex），rsaSign/Eddsa/HMAC具体实现，loadPrivateKey私钥加载
- **dec_precs.go**: DecToPrec精度格式化（支持DecimalPlace/SignifDigits/TickSize三种模式），isRound四舍五入，基于decimal高精度计算
- **exg.go**: PrecisionFromString精度提取，SafeParams参数安全复制，ParseTimeFrame解析时间周期（支持s/m/h/d/w/M/y单位）
- **file.go**: 文件操作（WriteFile/ReadFile/WriteJsonFile/ReadJsonFile），缓存文件（WriteCacheFile/ReadCacheFile支持过期），GetCacheDir跨平台缓存目录
- **misc.go**: UUID随机ID生成，ArrContains泛型包含检查，Marshal/Unmarshal JSON序列化，GetSystemProxy系统代理获取，GetMapVal泛型字典取值等辅助工具
- **tf_utils.go**: TFOrigin时间周期对齐原点，RegTfSecs注册自定义周期，parseTimeFrame解析周期（支持s/m/h/d/w/M/q/Y），AlignTfSecs/AlignTfMSecs时间戳对齐

### 交易所实现层

#### binance/ - Binance交易所完整实现
- **entry.go**: 交易所入口，New构造函数（ExgInfo基础信息，RateLimit=50ms，Hosts双环境配置，Fees四种市场费率，Apis路由表600+条），支持Spot/Margin/Linear/Inverse/Option五大市场，HTTP和WebSocket端点按市场类型分离
- **data.go**: Host类型常量24个（HostPublic/HostPrivate/HostFApi/HostDApi/HostSApi等），订单状态常量（OdStatusNew/OdStatusFilled/OdStatusCanceled等），Method方法名常量600+个（按MethodSapi/MethodPublic/MethodFapi/MethodDapi等分类），命名规范Method+ApiType+Action
- **types.go**: Binance主结构体（RecvWindow/streamBySubHash/LeverageBrackets等），Bnb前缀原始响应结构体（BnbMarket/BnbTicker/BnbOrder/BnbPosition等），SpotAccount/LinearAccount账户结构
- **biz.go**: 业务逻辑入口，Init初始化（Exchange.Init/RecvWindow/streamLimits/CalcRateLimiterCost/markRiskyApis等），markRiskyApis标记危险API，makeSign签名（HMAC-SHA256/X-MBX-APIKEY/账户权限检查）
- **account_access.go**: FetchAccountAccess提取账户权限（canTrade/canWithdraw/dualSidePosition/permissions字段）
- **biz_account.go**: FetchAccounts查询账户列表
- **biz_balance.go**: FetchBalance资产余额（Spot/Margin/Linear/Inverse统一处理），parseBalance余额解析，FetchPositions持仓查询
- **biz_order.go**: FetchOrder单个订单查询，FetchOrders历史订单，FetchOpenOrders未完成订单，parseOrder泛型订单解析器
- **biz_order_create.go**: CreateOrder下单（Spot/Margin/Linear/Inverse/Option），EditOrder改单，参数校验，市场类型路由
- **biz_order_algo.go**: 算法订单创建（条件单/止盈止损单），算法订单查询与取消
- **biz_order_book.go**: FetchOrderBook深度数据查询
- **biz_ticker.go**: FetchTicker单个行情，FetchTickers批量行情，parseTickers泛型行情解析器，FetchOHLCV K线，FetchLastPrices最新价，FetchFundingRate资金费率
- **common.go**: BnbMarket.GetPrecision精度提取，BnbMarket.GetMarketLimits限额转换（filters过滤器解析），SymbolLvgBrackets.ToStdBracket杠杆档位标准化
- **ws_biz.go**: makeHandleWsMsg消息路由（depthUpdate/trade/kline/markPriceUpdate/24hrTicker/ACCOUNT_UPDATE/executionReport等20+事件），handleOrderBook/handleTrade/handleBalance/handleOrderUpdate等具体处理器
- **ws_order.go**: WatchMyTrades我的成交监听，WatchBalance资产变动，WatchPositions持仓变动，WatchAccountConfig账户配置监听，listenKey管理

#### bybit/ - Bybit交易所部分实现
- **entry.go**: 交易所入口，New构造函数（支持Spot/Linear/Inverse/Option四种市场），Apis路由表（涵盖V5 API版本），TestNet和Prod双环境配置，费率Main/Linear/Inverse/Option，HTTP端点（HostPublic/HostPrivate），WebSocket端点（HostWsPublicSpot/HostWsPublicLinear/HostWsPublicInverse/HostWsPublicOption/HostWsPrivate按市场分离），Has能力声明
- **data.go**: Host类型常量（HostPublic/HostPrivate/HostWsPublicSpot/HostWsPublicLinear/HostWsPrivate等），Method方法名常量300+个（MethodV5开头），订单状态/类型/方向映射（orderStatusMap/orderTypeMap/sideMap）
- **types.go**: Bybit主结构体（RecvWindow接收窗口），V5Resp通用响应结构，V5ListResult列表结构，BybitTime时间类型，原始响应结构体
- **common_util.go**: V5Resp.ToStdCode/ToErr错误转换，V5ListResult分页处理，BybitTime.UnmarshalJSON时间解析，ParseNum数值解析，FetchAccountAccess账户权限提取
- **biz_market.go**: LoadMarkets市场数据加载（V5接口），解析instruments为标准市场结构
- **biz_balance.go**: FetchBalance资产余额（统一账户），FetchPositions持仓查询
- **biz_order.go**: CreateOrder/EditOrder/CancelOrder订单操作，FetchOrder/FetchOrders/FetchOpenOrders订单查询，按市场类型路由到V5接口
- **biz_ticker.go**: FetchTicker/FetchTickers行情查询，FetchOHLCV K线，FetchOrderBook订单簿，FetchFundingRate资金费率
- **biz_leverage.go**: LoadLeverageBrackets加载杠杆档位，GetLeverage获取杠杆，SetLeverage设置杠杆，CalcMaintMargin维持保证金计算
- **biz_data.go**: FetchLastPrices最新价，数据查询相关接口
- **ws_biz.go**: makeHandleWsMsg消息路由，handleTicker/handleTrade/handleOrderBook/handleKline处理器
- **ws_client.go**: WebSocket连接管理，订阅管理，重连逻辑
- **ws_parse.go**: WebSocket消息解析，事件分发

#### okx/ - OKX交易所完整实现
- **entry.go**: 交易所入口，New构造函数（支持Spot/Linear/Inverse/Option），RateLimit=20ms，Hosts双环境三端点，Fees费率Main/Linear，Apis路由表，Has能力声明30+接口，CredKeys需ApiKey/Secret/Password
- **data.go**: Host常量（HostPublic/HostPrivate/HostWsPublic等），字段常量（FldInstType/FldOrdType等），WebSocket通道名（WsChanTrades/WsChanBooks/WsChanOrders等），Method方法名常量40+个，订单状态/类型映射
- **types.go**: OKX主结构体（LeverageBrackets/WsPendingRecons），Okx前缀原始响应（OkxInstrument/OkxTicker/OkxOrder/OkxPosition等），WsPendingRecon重连待处理
- **biz.go**: Init初始化，makeSign签名（OK-ACCESS-*头部，HMAC-SHA256，base64摘要），markRiskyApis危险API标记，requestRetry泛型请求
- **account_access.go**: FetchAccountAccess提取账户权限（acctLv/posMode/mgnMode等字段）
- **biz_balance.go**: FetchBalance余额查询（统一账户模式），FetchPositions持仓查询
- **biz_order.go**: CreateOrder/EditOrder/CancelOrder订单操作，FetchOrder/FetchOrders/FetchOpenOrders订单查询
- **biz_order_algo.go**: 算法订单创建（条件单/止盈止损/跟踪单），算法订单查询取消
- **biz_ticker.go**: FetchTicker/FetchTickers行情，FetchOHLCV K线，FetchOrderBook订单簿，FetchFundingRate资金费率
- **biz_leverage.go**: LoadLeverageBrackets杠杆档位，GetLeverage获取杠杆，SetLeverage设置杠杆，CalcMaintMargin维持保证金
- **biz_account_history.go**: FetchIncomeHistory账单流水（支持archive归档查询）
- **common.go**: marketToInstType市场类型映射，parseInstrument品种解析，parseOrder/parseTicker转换
- **ws_biz.go**: makeHandleWsMsg消息路由（trades/books/balance_and_position/orders/mark-price/candle等），WatchOrderBooks/WatchTrades/WatchOHLCVs/WatchMarkPrices公有订阅，WatchMyTrades/WatchBalance/WatchPositions私有订阅，wsLogin认证

#### china/ - 中国期货交易所本地模拟
- **entry.go**: New构造函数（ExgInfo基本信息ID/Name/Countries，FixedLvg=true固定杠杆，RateLimit=50ms），无网络请求的本地模拟，Fees仅Linear手续费0.0002，Has声明仅支持LoadLeverageBrackets/GetLeverage，所有其他接口HasFail，makeCalcFee手续费计算
- **data.go**: defTimeLoc默认时区
- **types.go**: China主结构体，Exchange交易所信息（Code/Title/Suffix），ItemMarket品种描述（Code/Market/Multiplier/PriceTick/MarginPct/DayRanges/Fee），Fee手续费结构
- **biz.go**: Init初始化，loadRawMarkets从markets.yml加载品种配置（embed），LoadMarkets市场加载，MapMarket品种ID映射，parseMarket解析品种符号，GetLeverage获取杠杆，makeCalcFee手续费计算
- **common.go**: ItemMarket.Resolve继承解析，ToStdSymbol/ToRawSymbol品种符号转换，Fee.ParseStd手续费标准化
- **markets.yml**: 品种配置文件（embed嵌入），期货交易所和合约品种基础信息

## 项目特点
本节原内容与上文“技术架构与实现方案”高度重合，已合并到该节以减少重复。

## 开发指南

### 添加新交易所
1. 在新包中定义交易所结构体，嵌入`*banexg.Exchange`
2. 在entry.go中配置ExgInfo/Hosts/Fees/Apis，实现New构造函数
3. 实现`BanExchange`接口的核心方法（LoadMarkets/FetchTicker/CreateOrder/FetchBalance等）
4. 在`bex/entrys.go`中注册工厂函数
5. 参考binance/okx的完整实现模式

### 新增REST API标准流程
1. **data.go**: 定义Method常量（命名规范Method+ApiType+Action）
2. **entry.go**: 在Apis注册路由（Path/Host/Method/Cost）
3. **types.go**: 定义原始响应结构体（通常以交易所前缀开头）
4. **biz_*.go**: 实现业务方法（LoadArgsMarket预处理→请求参数准备→RequestApiRetry请求→解析转换为标准结构）

### 通用开发约定
- **DRY原则**: 避免重复代码，提取公共函数
- **参数处理**: 业务方法首行调用`LoadArgsMarket`预处理参数和市场
- **错误处理**: 统一使用`*errs.Error`，参数错误用`errs.CodeParamInvalid`
- **签名实现**: 在biz.go中实现makeSign函数，赋值给Exchange.Sign
- **WebSocket**: 在ws_biz.go实现makeHandleWsMsg消息路由，根据event分发到具体处理器
- **并发安全**: 访问共享状态必须加锁保护（使用deadlock.Mutex）
- **精度处理**: 使用decimal库保证高精度计算，实现GetPrecision/GetMarketLimits方法
- **代码风格**: 简洁实用，不过度设计，导出函数添加注释
