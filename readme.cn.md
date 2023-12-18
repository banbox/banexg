# BanExg - 数字货币交易所SDK
一个Go版本的数字货币交易SDK，主要接口和[CCXT](https://github.com/ccxt/ccxt)保持一致。  
注意：此项目正在积极开发，尚不能用于生产环境。  

# 注意
### 使用`Options`而不是直接字段赋值来初始化交易所对象 
交易所对象被初始化时，一些如int的简单类型字段会有类型默认值，在`Init`方法进行设置时，无法区分当前值是用户设置的值还是默认值。
所以所有需要外部传入的配置应通过`Options`传入，然后在`Init`中读取`Options`并设置到对应字段上。

### 市场类型MarketType
<table>
<tr>
    <th rowspan="2"></th>
    <th rowspan="2">Spot</th>
    <th rowspan="2">Margin</th>
    <th colspan="3">linear</th>
    <th colspan="3">inverse</th>
</tr>
<tr>
    <th>swap</th>
    <th>future</th>
    <th>option</th>
    <th>swap</th>
    <th>future</th>
    <th>option</th>
</tr>
<tr>
    <td>Desc</td>
    <td>现货</td>
    <td>保证金</td>
    <td>U本位永续</td>
    <td>U本位到期</td>
    <td>U本位期权</td>
    <td>币本位永续</td>
    <td>币本位到期</td>
    <td>币本位期权</td>
</tr>
</table>

### 常见参数命名调整
**`ccxt.defaultType` -> `MarketType`**  
当前交易所的默认市场类型。可在初始化时传入`OptMarketType`设置，也可随时设置交易所的`MarketType`属性。  
有效值：`MarketSpot/MarketMargin/MarketSwap/MarketFuture/MarketOption`  
ccxt中币安的defaultType命名和其他交易所不一致，banexg中进行了统一命名。  

**`ccxt.defaultSubType` -> `MarketInverse`**  
当前交易所市场结算类型，ccxt中可选值为`linear`和`inverse`。banexg中改为`bool`类型的`MarketInverse`，默认false。  
可在初始化时传入`OptMarketInverse`设置，也可初始化后设置交易所的`MarketInverse`属性。  
