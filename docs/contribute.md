## 项目概述

BanExg是多交易所统一SDK，通过标准接口抽象不同交易所API。已完整支持Binance，部分支持Bybit。

## 一、架构设计

### 1.1 文件组织
```
banexg/
├── intf.go           # BanExchange核心接口
├── types.go          # Exchange/Market/Order等核心结构
├── base.go/biz.go    # 基础功能和通用业务逻辑
├── common.go/data.go # 工具函数和常量定义
├── websocket.go      # WebSocket客户端
├── {exchange}/       # 各交易所实现
│   ├── entry.go      # 工厂函数、Apis映射表
│   ├── biz*.go       # REST API业务实现
│   ├── ws*.go        # WebSocket实现
│   ├── data.go       # 方法名常量
│   └── types.go      # 交易所特定结构
├── bex/              # 交易所注册工厂
└── utils/log/errs/   # 基础设施
```

### 1.2 四层架构
1. **接口层**(`intf.go`) - `BanExchange`定义100+方法
2. **业务层**(`biz.go`) - 通用逻辑：市场加载、费率计算、数据转换
3. **适配层**(`{exchange}/`) - 各交易所具体实现
4. **基础层**(`utils/`) - 工具、日志、错误处理

## 二、核心类型定义

### 2.1 BanExchange接口 (`intf.go`)
统一交易所操作标准，100+方法分为5类：
- **市场数据**: `LoadMarkets`, `FetchTicker`, `FetchOHLCV`, `FetchOrderBook`
- **交易**: `CreateOrder`, `CancelOrder`, `EditOrder`
- **账户**: `FetchBalance`, `FetchPositions`, `SetLeverage`
- **WebSocket**: `WatchOrderBooks`, `WatchTrades`, `WatchBalance`
- **工具**: `CalculateFee`, `CalcMaintMargin`, `PrecAmount`

### 2.2 Exchange结构体 (`types.go:34`)
所有交易所嵌入的基础实现，关键字段：
- `ExgInfo`: ID/Name/Markets缓存/OrderBooks缓存
- `Apis`: API端点映射表(Path/Host/Method/Cost)
- `Accounts`: 多账户管理(Creds/Positions/Balances)
- `WSClients`: WebSocket客户端池(`accName@url`索引)
- `Options`: 用户配置(ApiKey/MarketType/Proxy等)
- `Sign/FetchMarkets/CalcFee`: 交易所特定实现函数

### 2.3 核心数据结构 (`types.go`)
- **Market**(156行): Symbol/Type/Precision/Limits/Maker/Taker费率
- **Order**(443行): ID/Status/Price/Amount/Filled/Remaining
- **Position**(417行): Symbol/Side/EntryPrice/UnrealizedPnl/Leverage
- **OrderBook**(507行): Asks/Bids(Price/Size数组)/Nonce更新ID
- **Kline**(382行): Time/OHLCV
- **Balances**(398行): Free/Used/Total资产映射

#### Market.Symbol 标准品种命名
```text
BTC/USDT    # 现货
BTC/USDT:BTC    # 币本位永续合约
BTC/USDT:USDT   # U本位永续合约
BTC/USDT:BTC-211225    # 币本位合约
BTC/USDT:BTC-211225-60000-P    # 币本位期权
ETH/USDT:USDT-211225-40000-C    # U本位期权
```
#### Market.ID 交易所品种命名
按交易所不同，详细查看交易所下README.md

## 三、设计模式

1. **接口导向**: `BanExchange`统一标准，交易所可替换
2. **插件架构**: `bex.New()`工厂动态创建交易所实例
3. **策略模式**: `Sign/FetchMarkets/CalcFee`函数指针注入特定实现
4. **模板方法**: `Exchange`基类定义流程，子类覆盖数据获取

## 四、开发规范

### 4.1 交易所目录结构
```
{exchange}/
├── entry.go    # 工厂函数，Apis映射表(Path/Host/Method/Cost)
├── data.go     # MethodXXX常量，HostXXX常量
├── types.go    # 交易所原始响应结构(通常Bnb/Byb前缀)
├── biz*.go     # REST API实现：LoadArgsMarket→请求→解析→转标准格式
├── ws*.go      # WS: makeHandleWsMsg路由，handleXXX处理器
└── common.go   # GetPrecision/GetMarketLimits等转换工具
```

### 4.2 核心约定
1. **参数传递**: `map[string]interface{}`动态参数，`utils.PopMapVal`提取
2. **错误处理**: `*errs.Error`统一类型，含Code/Msg/Stack
3. **重试机制**: `RequestApiRetry`自动重试网络错误，配置`Retries`
4. **配置注入**: Options支持`OptApiKey/OptEnv/OptMarketType`等20+选项
5. **环境切换**: `OptEnv="test"`切换TestNet，`Hosts`自动选择地址

## 五、关键技术实现

### 5.1 市场管理
- **缓存机制**: `exgCacheMarkets`全局缓存，360分钟TTL
- **多类型支持**: `CareMarkets`指定加载spot/linear/inverse/option
- **并发加载**: `MarketsWait` chan阻塞并发请求，单次加载
- **Symbol映射**: `Markets[symbol]`和`MarketsById[id]`双向索引

### 5.2 精度处理(3种模式)
- **DecimalPlace**: 小数位数，如`0.01`=2位
- **SignifDigits**: 有效数字，如`0.00123`=3位
- **TickSize**: 最小变动单位，如`price % 0.01 == 0`

### 5.3 WebSocket架构
- **连接池**: `WSClients[accName@url]`复用连接
- **订阅管理**: `SubscribeKeys`存储订阅，断线自动恢复
- **消息路由**: `OnMessage(msg)` → `makeHandleWsMsg` → 具体handler
- **并发控制**: 每连接1个读goroutine，`send` chan写队列
- **录制回放**: `SetDump/SetReplay`支持调试

## 六、新交易所接入流程

### 6.1 标准8步骤
1. **创建目录**: `mkdir {exchange} && cd {exchange}`
2. **entry.go**: `New()`工厂 + `Apis`映射表(参考binance/entry.go结构)
3. **data.go**: 定义`MethodXXX`和`HostXXX`常量
4. **types.go**: 定义原始响应结构体
5. **biz*.go**: 实现核心方法(见下)
6. **ws*.go**: 实现WebSocket订阅
7. **bex/entrys.go**: 注册`newExgs["name"] = NewExchange`
8. **测试**: 使用testdata/mock验证

### 6.2 必需实现方法(优先级排序)
**P0(市场基础)**:
- `FetchMarkets`: 从API加载市场列表
- `LoadMarkets`: 调用FetchMarkets并缓存
- `Sign`: 实现签名算法(HMAC-SHA256/RSA等)

**P1(行情)**:
- `FetchTicker/FetchTickers`: 行情数据
- `FetchOrderBook`: 订单簿快照
- `FetchOHLCV`: K线数据

**P2(交易)**:
- `CreateOrder/CancelOrder/FetchOrder`: 订单操作
- `FetchBalance`: 余额查询
- `FetchPositions`: 持仓查询(合约)

**P3(WebSocket)**:
- `WatchOrderBooks/WatchTrades`: 实时行情
- `WatchBalance/WatchPositions`: 账户更新


## 七、核心模块说明

### 7.1 根目录文件
| 文件 | 核心功能 | 关键方法 |
|------|---------|----------|
| `base.go` | 基础工具 | `GetHost`主机选择, `CheckFilled`凭证校验 |
| `biz.go` | 通用业务 | `Init`初始化, `LoadMarkets`市场加载(带缓存+并发控制), `CalculateFee`费率计算 |
| `common.go` | 数据转换 | `OrderBook.Update`订单簿更新, `MergeMyTrades`交易合并 |
| `data.go` | 常量定义 | 市场类型/订单状态/API名称/配置键 |
| `types.go` | 类型系统 | Exchange/Market/Order/Position/Balances等20+结构 |
| `websocket.go` | WS客户端 | `WsClient`管理连接池/订阅/重连/消息路由 |

### 7.2 Binance实现参考
| 文件 | 行数 | 说明 |
|------|-----|------|
| `entry.go` | 871 | 715个方法的Apis映射表 + 费率配置 |
| `data.go` | - | 方法名常量(MethodSapiGetXXX) + Host常量 |
| `types.go` | - | Bnb前缀的原始响应结构 |
| `biz*.go` | - | 按功能拆分：order/ticker/balance/position等 |
| `ws*.go` | - | makeHandleWsMsg路由 + 各类型handler |
| `common.go` | - | GetPrecision/GetMarketLimits转换 |

### 7.3 基础模块
- **utils**: 时间处理(`ISO8601`), 数值精度(`DecimalPrec`), 加密(`HMAC/RSA`)
- **errs**: 33个错误码 + `Error`结构(Code/Msg/Stack)
- **log**: zap日志系统，支持轮转
- **bex**: `New(name, opts)`工厂，已注册binance/bybit/china


## 八、开发最佳实践

### 8.1 添加新API接口(标准4步)
1. **data.go**: 添加`MethodXxxYyy`常量
2. **entry.go**: 在`Apis`中注册`Path/Host/Method/Cost`
3. **types.go**: 定义原始响应结构(如`XxxRsp`)
4. **biz_xxx.go**: 实现方法 - `LoadArgsMarket` → 请求 → 解析 → 转标准格式

### 8.2 调试技巧
- **API调试**: `OptDebugApi: true`打印请求/响应
- **WS调试**: `OptDebugWs: true` + `SetDump(path)`录制消息
- **回放测试**: `SetReplay(path)`从录制文件重放
- **市场缓存**: 查看`exgCacheMarkets`全局变量

### 8.3 常见问题
1. **Markets为空**: 检查`CareMarkets`配置，确保包含目标市场类型
2. **签名失败**: 验证`Sign`函数参数顺序和编码方式
3. **精度错误**: 确认使用正确的`PrecMode`(Decimal/Signif/Tick)
4. **WS断连**: 检查`SubscribeKeys`是否正确存储，`OnReConn`是否恢复订阅

### 8.4 单元测试
- 手动运行单个测试：go test -run TestAPI_FetchTicker -v
- 运行所有单元测试（不含API测试）：go test -run "^Test[^A]" -v


## 九、 核心原则
- OptDebugApi模式下应输出所有api请求，OptDebugWs模式下应输出所有ws消息
- 密切相关的函数应该在一起维护，不要分散
- 用户调用接口传入的币种代码应该始终是banexg的标准代码Market.Symbol，内部转为交易所品种代码Markey.ID请求接口
- BanExg公开方法的`map[string]interface{}`参数是用户传入，其中key必须是`banexg.ParamXXX`，尽量提取为多交易所通用命名，然后内部转为交易所参数
- 交易所go包下的结构体和方法名中，不应该包含交易所名。比如okx包下mapOkxOrderType应改为mapOrderType；这样可减少信息冗余。
- 返回数据必须是规范的，固定几个值应添加常量枚举，动态值返回英文，永远不要返回中文。
- 对交易所接口的单元测试应使用local.json创建一个有效的交易所对象，然后真实发出请求和交易所进行交互。统一使用`TestApi_`前缀，在自动批量测试时应被排除，只应由用户手动单个执行这些测试。