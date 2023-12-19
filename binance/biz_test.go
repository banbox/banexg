package binance

import (
	"fmt"
	"github.com/anyongjin/banexg"
	"github.com/anyongjin/banexg/log"
	"github.com/anyongjin/banexg/utils"
	"go.uber.org/zap"
	"os"
	"testing"

	"github.com/bytedance/sonic"
)

func TestSign(t *testing.T) {
	exg := getBinance(nil)
	res, err := exg.FetchCurrencies(nil)
	if err != nil {
		panic(err)
	}
	text, err := sonic.MarshalString(res)
	if err != nil {
		panic(err)
	}
	err = os.WriteFile("curr.json", []byte(text), 0644)
	if err != nil {
		panic(err)
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
		if mar.Type != ccxt.Type {
			diffs["Type"] = fmt.Sprintf("%v-%v", mar.Type, ccxt.Type)
		}
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
		if mar.SubType != ccxt.SubType {
			diffs["SubType"] = fmt.Sprintf("%v-%v", mar.SubType, ccxt.SubType)
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
	for k, _ := range ccxtMarkets {
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
	for k, _ := range ccxtCurrs {
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

func TestGetOhlcv(t *testing.T) {
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
		klines, err := exg.FetchOhlcv(c.Symbol, c.TimeFrame, since, 0, nil)
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

func TestFetchBalances(t *testing.T) {
	exg := getBinance(nil)
	cases := []map[string]interface{}{
		{"market": banexg.MarketSpot},
		{"market": banexg.MarketSwap},
		{"market": banexg.MarketFuture, "inverse": true},
		{"marginMode": banexg.MarginCross},
		{"marginMode": banexg.MarginIsolated},
		{"market": "funding"},
	}
	for _, item := range cases {
		text, _ := sonic.MarshalString(item)
		res, err := exg.FetchBalance(&item)
		if err != nil {
			panic(fmt.Errorf("%s Error: %v", text, err))
		}
		res.Info = nil
		resText, _ := sonic.MarshalString(res)
		t.Logf("%s balance: %s", text, resText)
	}
}

func TestFetchOrders(t *testing.T) {
	exg := getBinance(nil)
	cases := []map[string]interface{}{
		//{"market": banexg.MarketSpot},
		{"market": banexg.MarketSwap},
		//{"market": banexg.MarketFuture, "inverse": true},
		//{"market": banexg.MarketOption},
	}
	symbol := "GAS/USDT"
	since := int64(1702991965921)
	for _, item := range cases {
		text, _ := sonic.MarshalString(item)
		res, err := exg.FetchOrders(symbol, since, 0, &item)
		if err != nil {
			panic(fmt.Errorf("%s Error: %v", text, err))
		}
		resText, _ := sonic.MarshalString(res)
		t.Logf("%s result: %s", text, resText)
	}
}
