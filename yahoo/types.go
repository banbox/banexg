package yahoo

import "github.com/banbox/banexg"

// Yahoo wraps the base Exchange to provide read-only Yahoo Finance market data.
type Yahoo struct {
	*banexg.Exchange
}

// chartResp is the v8/finance/chart response shape.
type chartResp struct {
	Chart struct {
		Result []chartResult `json:"result"`
		Error  *yahooErr     `json:"error"`
	} `json:"chart"`
}

type chartResult struct {
	Meta struct {
		Symbol             string  `json:"symbol"`
		Currency           string  `json:"currency"`
		ExchangeName       string  `json:"exchangeName"`
		InstrumentType     string  `json:"instrumentType"`
		RegularMarketPrice float64 `json:"regularMarketPrice"`
		Gmtoffset          int64   `json:"gmtoffset"`
		Timezone           string  `json:"timezone"`
	} `json:"meta"`
	Timestamp  []int64 `json:"timestamp"`
	Indicators struct {
		Quote []struct {
			Open   []*float64 `json:"open"`
			High   []*float64 `json:"high"`
			Low    []*float64 `json:"low"`
			Close  []*float64 `json:"close"`
			Volume []*float64 `json:"volume"`
		} `json:"quote"`
	} `json:"indicators"`
}

// quoteResp is the v7/finance/quote response shape.
type quoteResp struct {
	QuoteResponse struct {
		Result []quoteRes `json:"result"`
		Error  *yahooErr  `json:"error"`
	} `json:"quoteResponse"`
}

type quoteRes struct {
	Symbol                     string  `json:"symbol"`
	Currency                   string  `json:"currency"`
	Exchange                   string  `json:"exchange"`
	QuoteType                  string  `json:"quoteType"`
	RegularMarketPrice         float64 `json:"regularMarketPrice"`
	RegularMarketDayHigh       float64 `json:"regularMarketDayHigh"`
	RegularMarketDayLow        float64 `json:"regularMarketDayLow"`
	RegularMarketOpen          float64 `json:"regularMarketOpen"`
	RegularMarketPreviousClose float64 `json:"regularMarketPreviousClose"`
	RegularMarketChange        float64 `json:"regularMarketChange"`
	RegularMarketChangePercent float64 `json:"regularMarketChangePercent"`
	RegularMarketVolume        int64   `json:"regularMarketVolume"`
	Bid                        float64 `json:"bid"`
	Ask                        float64 `json:"ask"`
	BidSize                    int64   `json:"bidSize"`
	AskSize                    int64   `json:"askSize"`
	RegularMarketTime          int64   `json:"regularMarketTime"`
}

type yahooErr struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}
