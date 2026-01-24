package bybit

import "github.com/banbox/banexg"

func bybitPreferredSymbolsForMarketType(marketType string) []string {
	switch marketType {
	case banexg.MarketSpot:
		return []string{"XRP/USDT", "DOGE/USDT", "ETH/USDT", "BTC/USDT"}
	case banexg.MarketLinear:
		return []string{"XRP/USDT:USDT", "DOGE/USDT:USDT", "ETH/USDT:USDT", "BTC/USDT:USDT"}
	case banexg.MarketInverse:
		return []string{"BTC/USD:BTC", "ETH/USD:ETH"}
	case banexg.MarketOption:
		// Option symbols vary a lot; prefer none and let the picker choose any available option market.
		return nil
	default:
		return nil
	}
}
