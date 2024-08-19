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
### sonic's large integer deserialization problem
This project uses sonic as the json serialization library. It is known that float64 will be used when deserializing int64 long integers, resulting in precision loss. Currently, `UseInt64` is used to force the use of int64 to parse long integers, but this item is only effective under the `amd64` chip. Others such as `arm64` or `mac m1` will still use sonic's default behavior.

### Use `Options` instead of `direct fields assign` to initialized a Exchange 
When an exchange object is initialized, some fields of simple types like int will have default type values. When setting these in the `Init` method, it's impossible to distinguish whether the current value is one set by the user or the default value. 
Therefore, any configuration needed from outside should be passed in through `Options`, and then these `Options` should be read and set onto the corresponding fields in the `Init` method.
