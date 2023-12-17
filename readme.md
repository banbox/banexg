# BanExg - CryptoCurrency Exchange Trading Library
A Go library for cryptocurrency trading, whose most interfaces are consistent with [CCXT](https://github.com/ccxt/ccxt).  
**Please note that this library is under development and is not ready for production!**

# Notes
### Use `Options` instead of `direct fields assign` to initialized a Exchange 
When an exchange object is initialized, some fields of simple types like int will have default type values. When setting these in the `Init` method, it's impossible to distinguish whether the current value is one set by the user or the default value. 
Therefore, any configuration needed from outside should be passed in through `Options`, and then these `Options` should be read and set onto the corresponding fields in the `Init` method.
