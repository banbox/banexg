package bybit

import "github.com/banbox/banexg"

const (
	HostPublic  = "public"
	HostPrivate = "private"
)

const (
	OptRecvWindow = "RecvWindow"
)

var (
	DefCareMarkets = []string{
		banexg.MarketSpot, banexg.MarketLinear, banexg.MarketInverse,
	}
)
