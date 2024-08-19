package binance

import (
	"fmt"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"go.uber.org/zap"
	"os"
	"strings"
	"testing"

	"github.com/bytedance/sonic"
)

func TestSign(t *testing.T) {
	exg := getBinance(nil)
	res, err := exg.FetchCurrencies(nil)
	if err != nil {
		panic(err)
	}
	text, err_ := sonic.MarshalString(res)
	if err_ != nil {
		panic(err_)
	}
	err_ = os.WriteFile("curr.json", []byte(text), 0644)
	if err_ != nil {
		panic(err_)
	}
	fmt.Print(len(text))
}

type CompareRes struct {
	News  []string
	Lacks []string
	Sames []string
	Diffs map[string]map[string]string
}

func compareMarkets(t *testing.T, markets, ccxtMarkets banexg.MarketMap) error {
	var news = make([]string, 0)
	var lacks = make([]string, 0)
	var sames = make([]string, 0)
	var resDiffs = make(map[string]map[string]string)
	for k, mar := range markets {
		ccxt, ok := ccxtMarkets[k]
		if !ok {
			news = append(news, k)
			continue
		}
		var diffs = make(map[string]string)
		if mar.LowercaseID != ccxt.LowercaseID {
			diffs["LowercaseID"] = fmt.Sprintf("%v-%v", mar.LowercaseID, ccxt.LowercaseID)
		}
		if mar.Symbol != ccxt.Symbol {
			diffs["Symbol"] = fmt.Sprintf("%v-%v", mar.Symbol, ccxt.Symbol)
		}
		if mar.Base != ccxt.Base {
			diffs["Base"] = fmt.Sprintf("%v-%v", mar.Base, ccxt.Base)
		}
		if mar.Quote != ccxt.Quote {
			diffs["Quote"] = fmt.Sprintf("%v-%v", mar.Quote, ccxt.Quote)
		}
		if mar.Settle != ccxt.Settle {
			diffs["Settle"] = fmt.Sprintf("%v-%v", mar.Settle, ccxt.Settle)
		}
		if mar.BaseID != ccxt.BaseID {
			diffs["BaseID"] = fmt.Sprintf("%v-%v", mar.BaseID, ccxt.BaseID)
		}
		if mar.QuoteID != ccxt.QuoteID {
			diffs["QuoteID"] = fmt.Sprintf("%v-%v", mar.QuoteID, ccxt.QuoteID)
		}
		if mar.SettleID != ccxt.SettleID {
			diffs["SettleID"] = fmt.Sprintf("%v-%v", mar.SettleID, ccxt.SettleID)
		}
		// 这里和ccxt的值存储的不同，无需对比
		//if mar.Type != ccxt.Type {
		//	diffs["Type"] = fmt.Sprintf("%v-%v", mar.Type, ccxt.Type)
		//}
		if mar.Spot != ccxt.Spot {
			diffs["Spot"] = fmt.Sprintf("%v-%v", mar.Spot, ccxt.Spot)
		}
		if mar.Margin != ccxt.Margin {
			diffs["Margin"] = fmt.Sprintf("%v-%v", mar.Margin, ccxt.Margin)
		}
		if mar.Swap != ccxt.Swap {
			diffs["Swap"] = fmt.Sprintf("%v-%v", mar.Swap, ccxt.Swap)
		}
		if mar.Future != ccxt.Future {
			diffs["Future"] = fmt.Sprintf("%v-%v", mar.Future, ccxt.Future)
		}
		if mar.Option != ccxt.Option {
			diffs["Option"] = fmt.Sprintf("%v-%v", mar.Option, ccxt.Option)
		}
		if mar.Active != ccxt.Active {
			diffs["Active"] = fmt.Sprintf("%v-%v", mar.Active, ccxt.Active)
		}
		if mar.Contract != ccxt.Contract {
			diffs["Contract"] = fmt.Sprintf("%v-%v", mar.Contract, ccxt.Contract)
		}
		if mar.Linear != ccxt.Linear {
			diffs["Linear"] = fmt.Sprintf("%v-%v", mar.Linear, ccxt.Linear)
		}
		if mar.Inverse != ccxt.Inverse {
			diffs["Inverse"] = fmt.Sprintf("%v-%v", mar.Inverse, ccxt.Inverse)
		}
		if mar.Taker != ccxt.Taker {
			diffs["Taker"] = fmt.Sprintf("%v-%v", mar.Taker, ccxt.Taker)
		}
		if mar.Maker != ccxt.Maker {
			diffs["Maker"] = fmt.Sprintf("%v-%v", mar.Maker, ccxt.Maker)
		}
		if mar.ContractSize != ccxt.ContractSize {
			diffs["ContractSize"] = fmt.Sprintf("%v-%v", mar.ContractSize, ccxt.ContractSize)
		}
		if mar.Expiry != ccxt.Expiry {
			diffs["Expiry"] = fmt.Sprintf("%v-%v", mar.Expiry, ccxt.Expiry)
		}
		if mar.Strike != ccxt.Strike {
			diffs["Strike"] = fmt.Sprintf("%v-%v", mar.Strike, ccxt.Strike)
		}
		if mar.OptionType != ccxt.OptionType {
			diffs["OptionType"] = fmt.Sprintf("%v-%v", mar.OptionType, ccxt.OptionType)
		}
		prec1 := mar.Precision.ToString()
		prec2 := ccxt.Precision.ToString()
		if prec1 != prec2 {
			diffs["Precision"] = fmt.Sprintf("%v-%v", prec1, prec2)
		}
		//limt1 := mar.Limits.ToString()
		//limt2 := ccxt.Limits.ToString()
		//if limt1 != limt2 {
		//	diffs["Limits"] = fmt.Sprintf("%v-%v", limt1, limt2)
		//}
		if len(diffs) > 0 {
			resDiffs[k] = diffs
		} else {
			sames = append(sames, k)
		}
	}
	for k := range ccxtMarkets {
		if _, ok := markets[k]; !ok {
			lacks = append(lacks, k)
		}
	}
	var dump = &CompareRes{news, lacks, sames, resDiffs}
	args := []zap.Field{zap.Int("new", len(news)), zap.Int("lack", len(lacks)),
		zap.Int("diff", len(resDiffs)), zap.Int("same", len(sames))}
	if len(resDiffs) == 0 {
		log.Info("Pass compare markets", args...)
	} else {
		log.Error("Fail compare markets", args...)
		t.Errorf("Fail compare markets")
	}
	err := utils.WriteJsonFile("D:/diff_markets.json", dump)
	if err != nil {
		return err
	}
	return nil
}

func compareCurrs(t *testing.T, currs, ccxtCurrs banexg.CurrencyMap) error {
	var news = make([]string, 0)
	var lacks = make([]string, 0)
	var sames = make([]string, 0)
	var resDiffs = make(map[string]map[string]string)
	for k, cur := range currs {
		ccxt, ok := ccxtCurrs[k]
		if !ok {
			news = append(news, k)
			continue
		}
		var diffs = make(map[string]string)
		if cur.Name != ccxt.Name {
			diffs["Name"] = fmt.Sprintf("%v-%v", cur.Name, ccxt.Name)
		}
		if cur.Code != ccxt.Code {
			diffs["Code"] = fmt.Sprintf("%v-%v", cur.Code, ccxt.Code)
		}
		if cur.Type != ccxt.Type {
			diffs["Type"] = fmt.Sprintf("%v-%v", cur.Type, ccxt.Type)
		}
		if cur.NumericID != ccxt.NumericID {
			diffs["NumericID"] = fmt.Sprintf("%v-%v", cur.NumericID, ccxt.NumericID)
		}
		if cur.Precision != ccxt.Precision {
			diffs["Precision"] = fmt.Sprintf("%v-%v", cur.Precision, ccxt.Precision)
		}
		//if cur.Active != ccxt.Active {
		//	diffs["Active"] = fmt.Sprintf("%v-%v", cur.Active, ccxt.Active)
		//}
		//if cur.Deposit != ccxt.Deposit {
		//	diffs["Deposit"] = fmt.Sprintf("%v-%v", cur.Deposit, ccxt.Deposit)
		//}
		//if cur.Withdraw != ccxt.Withdraw {
		//	diffs["Withdraw"] = fmt.Sprintf("%v-%v", cur.Withdraw, ccxt.Withdraw)
		//}
		if cur.Fee != ccxt.Fee {
			diffs["Fee"] = fmt.Sprintf("%v-%v", cur.Fee, ccxt.Fee)
		}
		limit1 := cur.Limits.ToString()
		limit2 := cur.Limits.ToString()
		if limit1 != limit2 {
			diffs["Limits"] = fmt.Sprintf("%v-%v", limit1, limit2)
		}
		if len(diffs) > 0 {
			resDiffs[k] = diffs
		} else {
			sames = append(sames, k)
		}
	}
	for k := range ccxtCurrs {
		if _, ok := currs[k]; !ok {
			lacks = append(lacks, k)
		}
	}
	var dump = &CompareRes{news, lacks, sames, resDiffs}
	args := []zap.Field{zap.Int("new", len(news)), zap.Int("lack", len(lacks)),
		zap.Int("diff", len(resDiffs)), zap.Int("same", len(sames))}
	if len(resDiffs) == 0 {
		log.Info("Pass compare currs", args...)
	} else {
		log.Error("Fail compare currs", args...)
		t.Errorf("Fail compare currs")
	}
	err := utils.WriteJsonFile("D:/diff_currs.json", dump)
	if err != nil {
		return err
	}
	return nil
}

func TestLoadMarkets(t *testing.T) {
	exg := getBinance(nil)
	// read ccxt markets
	var ccxtMarkets banexg.MarketMap
	err := utils.ReadJsonFile("testdata/ccxt_markets.json", &ccxtMarkets)
	if err != nil {
		panic(err)
	}
	markets, err := exg.LoadMarkets(false, nil)
	if err != nil {
		panic(err)
	}
	err = compareMarkets(t, markets, ccxtMarkets)
	if err != nil {
		panic(err)
	}
	var ccxtCurrs banexg.CurrencyMap
	err = utils.ReadJsonFile("testdata/ccxt_currs.json", &ccxtCurrs)
	if err != nil {
		panic(err)
	}
	err = compareCurrs(t, exg.CurrenciesByCode, ccxtCurrs)
	if err != nil {
		panic(err)
	}
}

func TestGetOHLCV(t *testing.T) {
	since := int64(1670716800000)
	btcSwapBar := banexg.Kline{
		Time: since, Open: 17120.1, High: float64(17265), Low: float64(17060),
		Close: 17077.3, Volume: 171004.111,
	}
	btcSpotBar := banexg.Kline{
		Time: since, Open: 17127.49, High: 17270.99, Low: float64(17071),
		Close: 17085.05, Volume: 155286.47871,
	}
	cases := []struct {
		TradeMode string
		Symbol    string
		TimeFrame string
		FirstBar  *banexg.Kline
	}{
		{TradeMode: banexg.MarketSwap, Symbol: "BTC/USDT:USDT", TimeFrame: "1d", FirstBar: &btcSwapBar},
		{TradeMode: banexg.MarketSwap, Symbol: "BTC/USDT", TimeFrame: "1d", FirstBar: &btcSwapBar},
		{TradeMode: banexg.MarketSpot, Symbol: "BTC/USDT", TimeFrame: "1d", FirstBar: &btcSpotBar},
	}
	exg := getBinance(nil)
	for _, c := range cases {
		exg.MarketType = c.TradeMode
		klines, err := exg.FetchOHLCV(c.Symbol, c.TimeFrame, since, 0, nil)
		if err != nil {
			panic(err)
		}
		first, out := c.FirstBar, klines[0]
		if first.Time != out.Time || first.Open != out.Open || first.High != out.High ||
			first.Low != out.Low || first.Close != out.Close || first.Volume != out.Volume {
			outText, _ := sonic.MarshalString(out)
			expText, _ := sonic.MarshalString(first)
			t.Errorf("Fail %s out: %s exp: %s", c.Symbol, outText, expText)
		}
	}
}

func TestFetchOHLCV(t *testing.T) {
	exg := getBinance(nil)
	startMs := int64(1719014400000)
	res, err := exg.FetchOHLCV("BAKE/USDT:USDT", "1h", startMs, 10, nil)
	if err != nil {
		panic(err)
	}
	for _, k := range res {
		fmt.Printf("%v, %v %v %v %v %v\n", k.Time, k.Open, k.High, k.Low, k.Close, int(k.Volume))
	}
}

func TestFetchBalances(t *testing.T) {
	exg := getBinance(nil)
	cases := []map[string]interface{}{
		{"market": banexg.MarketSpot},
		{"market": banexg.MarketLinear},
		{"market": banexg.MarketInverse},
		{banexg.ParamMarginMode: banexg.MarginCross},
		{banexg.ParamMarginMode: banexg.MarginIsolated},
		{"market": "funding"},
	}
	for _, item := range cases {
		text, _ := sonic.MarshalString(item)
		res, err := exg.FetchBalance(item)
		if err != nil {
			panic(fmt.Errorf("%s Error: %v", text, err))
		}
		res.Info = nil
		resText, _ := sonic.MarshalString(res)
		t.Logf("%s balance: %s", text, resText)
	}
}

func TestGetMarket(t *testing.T) {
	exg := getBinance(nil)
	markets, err := exg.LoadMarkets(false, nil)
	if err != nil {
		panic(err)
	}
	for symbol := range markets {
		if !strings.HasPrefix(symbol, "BTC/") {
			continue
		}
		mar, err := exg.GetMarket(symbol)
		if err != nil {
			panic(err)
		}
		if mar.Symbol != symbol {
			t.Errorf("FAIL GetMarket, get: %v, expect %v", mar.Symbol, symbol)
		} else {
			t.Logf("PASS GetMarket: %v", mar.Symbol)
		}
	}

	items := []struct {
		Symbol string
		Output string
		Args   map[string]interface{}
	}{
		{"BTC/USDT", "BTC/USDT", nil},
		{"BTC/USDT:USDT", "BTC/USDT:USDT", nil},
		{"BTC/USDT", "BTC/USDT:USDT", map[string]interface{}{
			"market": banexg.MarketLinear,
		}},
		{"BTC/USDT", "BTC/USDT:USDT", map[string]interface{}{
			"market": banexg.MarketSwap,
		}},
	}
	for _, item := range items {
		mar, err := exg.GetArgsMarket(item.Symbol, item.Args)
		if err != nil {
			panic(err)
		}
		if mar.Symbol != item.Output {
			t.Errorf("FAIL GetArgsMarket %v, exp: %v", mar.Symbol, item.Output)
		}
	}
}

func TestGetMarketType(t *testing.T) {
	exg := getBinance(nil)
	_, err := exg.LoadMarkets(false, nil)
	if err != nil {
		panic(err)
	}
	spotBtc := "BTC/USDT"
	swapBtc := "BTC/USDT:USDT"
	inverseBtc := "BTC/USD:BTC"
	mtype, _ := exg.GetArgsMarketType(nil, spotBtc)
	if mtype != banexg.MarketSpot {
		t.Errorf("FAIL GetArgsMarketType, get: %v, expect: spot", mtype)
	}

	mtype, _ = exg.GetArgsMarketType(nil, swapBtc)
	if mtype != banexg.MarketLinear {
		t.Errorf("FAIL GetArgsMarketType, get: %v, expect: swap", mtype)
	}

	mtype, _ = exg.GetArgsMarketType(nil, inverseBtc)
	if mtype != banexg.MarketInverse {
		t.Errorf("FAIL GetArgsMarketType, get: %v, expect: inverse", mtype)
	}
}

func TestGetMarketById(t *testing.T) {
	exg := getBinance(nil)
	_, err := exg.LoadMarkets(false, nil)
	if err != nil {
		panic(err)
	}
	btcSwap := "BTC/USDT:USDT"
	btcFut := "BTC/USDT:USDT-242903"
	btcFutMar := *exg.Markets[btcSwap]
	btcFutMar.Symbol = btcFut
	exg.MarketsById[btcFut] = []*banexg.Market{
		&btcFutMar,
	}
	items := []struct {
		symbol string
		market string
		output string
	}{
		{"BTCUSDT", "spot", "BTC/USDT"},
		{"BTCUSDT", "linear", "BTC/USDT:USDT"},
		{"BTCUSDT", "swap", btcSwap},
		{"BTCUSDT", "future", btcSwap},
	}
	for _, it := range items {
		mar := exg.GetMarketById(it.symbol, it.market)
		if mar.Symbol != it.output {
			t.Errorf("FAIL GetMarketById %v %v out: %v exp: %v", it.symbol, it.market, mar.Symbol, it.output)
		}
		mar = exg.SafeMarket(it.symbol, "", it.market)
		if mar.Symbol != it.output {
			t.Errorf("FAIL SafeMarket %v %v out: %v exp: %v", it.symbol, it.market, mar.Symbol, it.output)
		}
	}
}

func TestMarketCopy(t *testing.T) {
	exg := getBinance(nil)
	_, err := exg.LoadMarkets(false, nil)
	if err != nil {
		panic(err)
	}
	symbol := "BTC/USDT"
	mar, err := exg.GetMarket(symbol)
	if err != nil {
		panic(err)
	}
	mar2 := *mar
	mar2.Type = banexg.MarketMargin
	t.Logf("%v -> %v, addr: %p -> %p", mar.Type, mar2.Type, mar, &mar2)
}

func TestSetLeverage(t *testing.T) {
	exg := getBinance(nil)
	res, err := exg.SetLeverage(8, "GAS/USDT:USDT", nil)
	if err != nil {
		panic(err)
	}
	fmt.Printf("set leverage: %v", res)
}

func TestLoadLeverageBrackets(t *testing.T) {
	exg := getBinance(nil)
	exg.MarketType = banexg.MarketLinear
	err := exg.LoadLeverageBrackets(false, nil)
	if err != nil {
		panic(err)
	}
	text, err2 := sonic.MarshalString(exg.LeverageBrackets)
	if err2 != nil {
		panic(err2)
	}
	fmt.Println(text)
}

func TestParseLinearPositionRisk(t *testing.T) {
	exg := getBinance(nil)
	exg.MarketType = banexg.MarketLinear
	_, _ = exg.LoadMarkets(false, nil)
	exg.LeverageBrackets = map[string]*SymbolLvgBrackets{
		"BTC/USDT:USDT": {
			Symbol: "BTC/USDT:USDT",
			Brackets: []*LvgBracket{
				{Floor: 0, BaseLvgBracket: BaseLvgBracket{MaintMarginRatio: 0.004}},
				{Floor: 50000, BaseLvgBracket: BaseLvgBracket{MaintMarginRatio: 0.005}},
				{Floor: 500000, BaseLvgBracket: BaseLvgBracket{MaintMarginRatio: 0.01}},
				{Floor: 10000000, BaseLvgBracket: BaseLvgBracket{MaintMarginRatio: 0.025}},
				{Floor: 80000000, BaseLvgBracket: BaseLvgBracket{MaintMarginRatio: 0.05}},
				{Floor: 150000000, BaseLvgBracket: BaseLvgBracket{MaintMarginRatio: 0.1}},
				{Floor: 300000000, BaseLvgBracket: BaseLvgBracket{MaintMarginRatio: 0.125}},
				{Floor: 450000000, BaseLvgBracket: BaseLvgBracket{MaintMarginRatio: 0.15}},
				{Floor: 600000000, BaseLvgBracket: BaseLvgBracket{MaintMarginRatio: 0.25}},
				{Floor: 800000000, BaseLvgBracket: BaseLvgBracket{MaintMarginRatio: 0.5}},
			},
		},
	}
	var content = `[{"symbol":"BTCUSDT","positionAmt":"-0.003","entryPrice":"42976.6","breakEvenPrice":"42955.1117","markPrice":"42832.79591057","unRealizedProfit":"0.43141226","liquidationPrice":"48303.07832462","leverage":"20","maxNotionalValue":"80000000","marginType":"cross","isolatedMargin":"0.00000000","isAutoAddMargin":"false","positionSide":"SHORT","notional":"-128.49838773","isolatedWallet":"0","updateTime":1704362904896,"isolated":false,"adlQuantile":2}]`
	var expectStr = `{"id":"","symbol":"BTC/USDT:USDT","timestamp":1704362904896,"isolated":false,"hedged":true,"side":"short","contracts":0.003,"contractSize":1,"entryPrice":42976.6,"markPrice":42832.79591057,"notional":128.49838773,"leverage":20,"collateral":16.55907191,"initialMargin":6.42491939,"maintenanceMargin":0.51399355092,"initialMarginPercentage":0.05,"maintenanceMarginPercentage":0.004,"unrealizedPnl":0.43141226,"liquidationPrice":48303.07832462,"marginMode":"cross","marginRatio":0.031,"percentage":6.71,"info":null}`
	res := &banexg.HttpRes{Content: content}
	posList, err := parsePositionRisk[*LinearPositionRisk](exg, res)
	if err != nil {
		panic(err)
	}
	var pos = posList[0]
	pos.Info = nil
	text, _ := sonic.MarshalString(pos)
	var out = map[string]interface{}{}
	_ = utils.UnmarshalString(text, &out)
	var expect = map[string]interface{}{}
	_ = utils.UnmarshalString(expectStr, &expect)
	for k, v := range expect {
		if outv, ok := out[k]; ok {
			if outv != v {
				t.Errorf("%s fail, expect %v , out: %v", k, v, outv)
			}
		}
	}
	fmt.Println(text)
}

func TestFetchPositionsRisk(t *testing.T) {
	exg := getBinance(nil)
	exg.MarketType = banexg.MarketInverse
	posList, err := exg.FetchPositionsRisk(nil, nil)
	if err != nil {
		panic(err)
	}
	for _, p := range posList {
		p.Info = nil
	}
	text, _ := sonic.MarshalString(posList)
	fmt.Println(text)
}

func TestFetchAccountPositions(t *testing.T) {
	exg := getBinance(nil)
	exg.MarketType = banexg.MarketInverse
	posList, err := exg.FetchAccountPositions(nil, nil)
	if err != nil {
		panic(err)
	}
	for _, p := range posList {
		p.Info = nil
	}
	text, _ := sonic.MarshalString(posList)
	fmt.Println(text)
}
