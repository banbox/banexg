# BanExg - CryptoCurrency Exchange Trading Library
A Go library for cryptocurrency trading, whose most interfaces are consistent with [CCXT](https://github.com/ccxt/ccxt).  
Note: This project is currently in the pre-release state, the main interface test has passed, but it has not yet been tested in the long-term production environment.  
Currently supported exchanges: `binance`, `china`. Both implement the interface `BanExchange`  
Since I don't have the energy to update the document at present, I suggest that you check the `inf.go` interface for specific use, as well as the specific exchange method; welcome to help improve the document and code.

# How to Use
```go
var options = map[string]interface{}{}
// more option keys can be found by type `banexg.Opt...`
options[banexg.OptMarketType] = banexg.MarketLinear  // usd based future market
exchange, err := bex.New('binance', options)

// You can also instantiate an exchange object directly
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

# Note
### Use `Options` instead of `direct fields assign` to initialized a Exchange 
When an exchange object is initialized, some fields of simple types like int will have default type values. When setting these in the `Init` method, it's impossible to distinguish whether the current value is one set by the user or the default value. 
Therefore, any configuration needed from outside should be passed in through `Options`, and then these `Options` should be read and set onto the corresponding fields in the `Init` method.

### Market Type
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
    <td>spot</td>
    <td>margin</td>
    <td>USDⓈ-M Perpetual</td>
    <td>USDⓈ-M Futures</td>
    <td>COIN-M Perpetual</td>
    <td>COIN-M Futures</td>
    <td>USDⓈ-M Option</td>
    <td>COIN-M Option</td>
</tr>
</table>

### Common parameter naming adjustments
**`ccxt.defaultType` -> `MarketType`**  
The default market type of the current exchange. It can be set during initialization using `OptMarketType` or by modifying the `MarketType` property of the exchange at any time.  
Valid Values: `MarketSpot/MarketMargin/MarketLinear/MarketInverse/MarketOption`  
In ccxt, the naming of `defaultType` for Binance is inconsistent with other exchanges. In banexg, a unified naming convention has been applied.  

**`ContractType`**  
The contract type for the current exchange, with options of `swap` for perpetual contracts and `future` for contracts with an expiration date.   
It can be set during initialization using `OptContractType` or by modifying the `ContractType` property of the exchange after initialization.
