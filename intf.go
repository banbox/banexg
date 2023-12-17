package banexg

type BanExchange interface {
	FetchCurrencies(params map[string]interface{}) CurrencyMap
}
