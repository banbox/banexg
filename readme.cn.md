# BanExg - 数字货币交易所SDK
一个Go版本的数字货币交易SDK，主要接口和[CCXT](https://github.com/ccxt/ccxt)保持一致。  
注意：此项目目前处于预发行状态，主要接口测试通过，但尚未进行长期生产环境测试。  
目前支持交易所：`binance`, `china`。都实现了接口`BanExchange`  
由于我目前没有精力更新文档，所以具体如何使用建议查看`inf.go`接口，以及具体交易所的方法；欢迎帮助改善文档和代码。

# 如何使用
```go
var options = map[string]interface{}{}
// more option keys can be found by type `banexg.Opt...`
options[banexg.OptMarketType] = banexg.MarketLinear  // usd based future market
exchange, err := bex.New('binance', options)

// 您也可以直接实例化某个交易所对象
// exchange, err := binance.NewExchange(options)

// exchange is a BanExchange interface object
res, err := exg.FetchOHLCV("ETH/USDT:USDT", "1d", 0, 10, nil)
if err != nil {
    panic(err)
}
for _, k := range res {
    fmt.Printf("%v, %v %v %v %v %v\n", k.Time, k.Open, k.High, k.Low, k.Close, int(k.Volume))
}
```

# 注意
### sonic的大整数反序列化问题
本项目使用sonic作为json序列化库，已知在反序列化int64的长整数时会使用float64导致精度损失。目前使用`UseInt64`强制使用int64来解析长整数，但此项仅在`amd64`芯片下生效，其他如`arm64`或者`mac m1`等依然会使用sonic的默认行为。

### 使用`Options`而不是直接字段赋值来初始化交易所对象 
交易所对象被初始化时，一些如int的简单类型字段会有类型默认值，在`Init`方法进行设置时，无法区分当前值是用户设置的值还是默认值。
所以所有需要外部传入的配置应通过`Options`传入，然后在`Init`中读取`Options`并设置到对应字段上。

### 市场类型MarketType
<table>
<tr>
    <th rowspan="2"></th>
    <th rowspan="2">Spot</th>
    <th rowspan="2">Margin</th>
    <th colspan="2">Contract Linear</th>
    <th colspan="2">Contract Inverse</th>
    <th colspan="2">Option</th>
</tr>
<tr>
    <th>Swap Linear</th>
    <th>future Linear</th>
    <th>swap Inverse</th>
    <th>future Inverse</th>
    <th>option linear</th>
    <th>option inverse</th>
</tr>
<tr>
    <td>Desc</td>
    <td>现货</td>
    <td>保证金</td>
    <td>U本位永续</td>
    <td>U本位到期</td>
    <td>币本位永续</td>
    <td>币本位到期</td>
    <td>U本位期权</td>
    <td>币本位期权</td>
</tr>
</table>

### 常见参数命名调整
**`ccxt.defaultType` -> `MarketType`**  
当前交易所的默认市场类型。可在初始化时传入`OptMarketType`设置，也可随时设置交易所的`MarketType`属性。  
有效值：`MarketSpot/MarketMargin/MarketLinear/MarketInverse/MarketOption`  
ccxt中币安的defaultType命名和其他交易所不一致，banexg中进行了统一命名。  

**`ContractType`**  
当前交易所合约类型，可选值`swap`永续合约，`future`有到期日的合约。  
可在初始化时传入`OptContractType`设置，也可初始化后设置交易所的`ContractType`属性。  
