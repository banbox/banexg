package banexg

// BuildSymbolSet converts symbols to a lookup set. Returns nil for empty input.
func BuildSymbolSet(symbols []string) map[string]struct{} {
	if len(symbols) == 0 {
		return nil
	}
	result := make(map[string]struct{}, len(symbols))
	for _, sym := range symbols {
		result[sym] = struct{}{}
	}
	return result
}

// FilterTickers filters tickers by symbolSet when provided.
func FilterTickers(tickers []*Ticker, symbolSet map[string]struct{}) []*Ticker {
	if len(tickers) == 0 {
		return nil
	}
	result := make([]*Ticker, 0, len(tickers))
	for _, ticker := range tickers {
		if ticker == nil || ticker.Symbol == "" {
			continue
		}
		if symbolSet != nil {
			if _, ok := symbolSet[ticker.Symbol]; !ok {
				continue
			}
		}
		result = append(result, ticker)
	}
	return result
}

// TickersToLastPrices converts tickers to last price list; filters by symbolSet when provided.
func TickersToLastPrices(tickers []*Ticker, symbolSet map[string]struct{}) []*LastPrice {
	if len(tickers) == 0 {
		return nil
	}
	filtered := FilterTickers(tickers, symbolSet)
	result := make([]*LastPrice, 0, len(filtered))
	for _, ticker := range filtered {
		result = append(result, &LastPrice{
			Symbol:    ticker.Symbol,
			Timestamp: ticker.TimeStamp,
			Price:     ticker.Last,
			Info:      ticker.Info,
		})
	}
	return result
}

// TickersToPriceMap converts tickers to symbol->last price map; filters by symbolSet when provided.
func TickersToPriceMap(tickers []*Ticker, symbolSet map[string]struct{}) map[string]float64 {
	result := make(map[string]float64)
	for _, ticker := range FilterTickers(tickers, symbolSet) {
		result[ticker.Symbol] = ticker.Last
	}
	return result
}
