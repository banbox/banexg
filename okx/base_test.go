package okx

import (
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
)

// getExchange creates an OKX exchange instance with credentials from local.json.
// This is used for API integration tests that interact with the production environment.
// To use these tests:
// 1. Copy local.json.example to local.json
// 2. Fill in your OKX API credentials (apiKey, apiSecret, password)
// 3. Run individual tests with: go test -run TestFetchTicker -v
func getExchange(param map[string]interface{}) *OKX {
	args := utils.SafeParams(param)
	// Set log level to debug if debugApi is enabled
	if debugApi, ok := args[banexg.OptDebugApi].(bool); ok && debugApi {
		log.Setup("debug", "")
	} else {
		log.Setup("info", "")
	}
	local := make(map[string]interface{})
	err_ := utils.ReadJsonFile("local.json", &local, utils.JsonNumAuto)
	if err_ != nil {
		panic(err_)
	}
	for k, v := range local {
		args[k] = v
	}
	exg, err := New(args)
	if err != nil {
		panic(err)
	}
	// Apply debugApi to exchange instance
	if debugApi, ok := args[banexg.OptDebugApi].(bool); ok && debugApi {
		exg.DebugAPI = true
	}
	return exg
}

func seedMarket(exg *OKX, instId, symbol, marketType string) {
	if exg == nil {
		return
	}
	if exg.Markets == nil {
		exg.Markets = banexg.MarketMap{}
	}
	if exg.MarketsById == nil {
		exg.MarketsById = banexg.MarketArrMap{}
	}
	market := &banexg.Market{
		ID:     instId,
		Symbol: symbol,
		Type:   marketType,
	}
	exg.Markets[symbol] = market
	exg.MarketsById[instId] = []*banexg.Market{market}
}
