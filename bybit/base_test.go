package bybit

import (
	"testing"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
)

func bybitTestOptions(param map[string]interface{}) (map[string]interface{}, error) {
	args := utils.SafeParams(param)
	local := make(map[string]interface{})
	if err := utils.ReadJsonFile("local.json", &local, utils.JsonNumAuto); err != nil {
		return nil, err
	}
	for k, v := range local {
		args[k] = v
	}
	return args, nil
}

func getBybitOrSkip(t *testing.T, param map[string]interface{}) *Bybit {
	t.Helper()
	log.Setup("info", "")
	args, err := bybitTestOptions(param)
	if err != nil {
		t.Skipf("missing local.json: %v", err)
		return nil
	}
	exg, newErr := New(args)
	if newErr != nil {
		t.Fatalf("new bybit exchange failed: %s", newErr.Short())
	}
	return exg
}

func requireBybitCreds(t *testing.T, exg *Bybit) {
	t.Helper()
	if exg == nil {
		t.Skip("bybit exchange not initialized")
		return
	}
	apiKey := utils.GetMapVal(exg.Options, banexg.OptApiKey, "")
	apiSecret := utils.GetMapVal(exg.Options, banexg.OptApiSecret, "")
	if apiKey == "" || apiSecret == "" {
		t.Skip("ApiKey/ApiSecret required in local.json")
	}
	if apiKey == "test-api-key" || apiSecret == "test-api-secret" {
		t.Skip("placeholder ApiKey/ApiSecret in local.json")
	}
}

func getBybitAuthed(t *testing.T, param map[string]interface{}) *Bybit {
	t.Helper()
	exg := getBybitOrSkip(t, param)
	requireBybitCreds(t, exg)
	return exg
}

func getBybitAuthedNoCurr(t *testing.T, param map[string]interface{}) *Bybit {
	t.Helper()
	exg := getBybitAuthed(t, param)
	if exg == nil {
		return nil
	}
	if exg.Has == nil {
		exg.Has = map[string]map[string]int{}
	}
	if exg.Has[""] == nil {
		exg.Has[""] = map[string]int{}
	}
	exg.Has[""][banexg.ApiFetchCurrencies] = banexg.HasFail
	return exg
}

func getBybitOrSkipNoCurr(t *testing.T, param map[string]interface{}) *Bybit {
	t.Helper()
	exg := getBybitOrSkip(t, param)
	if exg == nil {
		return nil
	}
	// LoadMarkets() triggers FetchCurrencies() when enabled; this helper disables it so that
	// public-only tests can run without real API credentials.
	if exg.Has == nil {
		exg.Has = map[string]map[string]int{}
	}
	if exg.Has[""] == nil {
		exg.Has[""] = map[string]int{}
	}
	exg.Has[""][banexg.ApiFetchCurrencies] = banexg.HasFail
	return exg
}

func seedMarket(exg *Bybit, marketID, symbol, marketType string) {
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
		ID:     marketID,
		Symbol: symbol,
		Type:   marketType,
	}
	switch marketType {
	case banexg.MarketSpot:
		market.Spot = true
	case banexg.MarketLinear:
		market.Linear = true
		market.Contract = true
	case banexg.MarketInverse:
		market.Inverse = true
		market.Contract = true
	case banexg.MarketOption:
		market.Option = true
		market.Contract = true
	}
	exg.Markets[symbol] = market
	exg.MarketsById[marketID] = []*banexg.Market{market}
}
