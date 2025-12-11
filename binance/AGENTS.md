# Binance 模块开发指南

## 1. 核心文件职责

| 文件名 | 核心职责 | 关键内容 |
| :--- | :--- | :--- |
| **`entry.go`** | **入口与路由** | `New`构造函数；**`Apis` 映射表** (定义所有REST API路由、Host、权重)。 |
| **`data.go`** | **常量定义** | `MethodXXX` 方法名常量；Host类型常量；API枚举值映射 (如订单状态)。 |
| **`types.go`** | **数据结构** | `Binance` 主结构体；**交易所原始JSON响应结构体** (通常以 `Bnb` 开头)。 |
| **`biz_*.go`** | **业务逻辑** | REST API 具体实现。如 `biz_order.go` (订单), `biz_ticker.go` (行情)。 |
| **`ws_*.go`** | **WebSocket** | WS 连接管理、订阅逻辑、消息路由 (`makeHandleWsMsg`)。 |
| **`common.go`** | **通用工具** | 精度计算 `GetPrecision`；限额转换 `GetMarketLimits`。 |

## 2. 新增 REST API 开发流程 (Standard Flow)

添加新接口必须严格遵循以下 **4步走** 流程：

### Step 1: 定义方法常量 (`data.go`)
在 `data.go` 中添加唯一的 Method 常量。
```go
// 命名规范: Method + ApiType(Sapi/Fapi/Public) + Action
const MethodSapiGetNewFeature = "sapiGetNewFeature"
```

### Step 2: 注册 API 路由 (`entry.go`)
在 `entry.go` 的 `Apis` 变量中注册接口配置。
```go
MethodSapiGetNewFeature: {
    Path: "path/to/endpoint", // URL 相对路径
    Host: HostSApi,           // 引用 data.go 中的 Host 常量
    Method: "GET",            // HTTP 方法
    Cost: 1,                  // 权重消耗
},
```

### Step 3: 定义原始数据结构 (`types.go`)
在 `types.go` 中定义接口返回的 **原始 JSON 结构**。
```go
type BnbNewFeatureRsp struct {
    Id    string `json:"id"`
    Value string `json:"value"`
}
```

### Step 4: 实现业务方法 (`biz_*.go`)
在合适的 `biz_` 文件中实现方法，需遵循统一模式：

```go
func (e *Binance) FetchNewFeature(symbol string, params map[string]interface{}) (*banexg.StandardType, *errs.Error) {
    // 1. 预处理参数与市场 (必须)
    args, market, err := e.LoadArgsMarket(symbol, params)
    if err != nil {
        return nil, err
    }
    
    // 2. 准备请求参数 (使用 utils 提取)
    args["symbol"] = market.ID // Binance 通常需要转为内部 ID
    someVal := utils.PopMapVal(args, "someParam", "default")
    
    // 3. 发起请求 (自动处理签名与重试)
    // 根据市场类型选择 Method 常量 (如现货、U本位合约可能不同)
    method := MethodSapiGetNewFeature
    rsp := e.RequestApiRetry(context.Background(), method, args, 1)
    if rsp.Error != nil {
        return nil, rsp.Error
    }
    
    // 4. 解析与转换 (使用泛型解析器或手动转换)
    // 需将 Bnb 原始结构转换为 banexg 标准结构
    var raw BnbNewFeatureRsp
    if err := utils.Unmarshal(rsp.Data, &raw); err != nil {
        return nil, errs.NewMsg(errs.CodeDataLost, "parse error: %v", err)
    }
    
    return &banexg.StandardType{
        ID: raw.Id,
        // ... 字段映射
    }, nil
}
```

## 3. 重要开发约定

### 3.1 市场与参数处理
- **`LoadArgsMarket`**: 所有业务方法首行必须调用。它负责：
  - 复制 `params` 防止副作用。
  - 解析 `symbol` 字符串为 `*banexg.Market` 对象。
  - 校验市场是否存在。
- **Generic Parsing**: 许多模块（如 Order, Ticker）使用了泛型解析函数（如 `parseOrder[*SpotOrder]`, `parseTickers[*LinearTicker]`），添加新类型时应优先复用这些模式。

### 3.2 Host 常量选择指南
- **`HostPublic` / `HostPrivate`**: 现货 (Spot)
- **`HostFApi...`**: U本位合约 (USDT-M Futures)
- **`HostDApi...`**: 币本位合约 (COIN-M Futures)
- **`HostSApi...`**: 杠杆/理财/通用 (Margin/Savings)
- **`HostEApi...`**: 期权 (Options)

### 3.3 错误处理
- 统一使用 `*errs.Error`。
- 参数错误使用 `errs.NewMsg(errs.CodeParamInvalid, "msg")`。
- 网络/API错误由 `RequestApiRetry` 自动封装。

### 3.4 WebSocket 开发
- 消息路由位于 `ws_biz.go` 的 `makeHandleWsMsg` 函数。
- 需根据 `item.Event` 字符串分发到具体的 `handleXXX` 方法。
