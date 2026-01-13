package okx

import (
	"testing"

	"github.com/banbox/banexg"
)

func TestParseIncome(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	seedMarket(exg, "BTC-USDT-SWAP", "BTC/USDT:USDT", banexg.MarketLinear)
	bill := &Bill{
		InstType: "SWAP",
		InstId:   "BTC-USDT-SWAP",
		BillId:   "623950854533513219",
		Type:     "8",
		SubType:  "173",
		Ts:       "1695033476167",
		BalChg:   "0.021933823221",
		Ccy:      "USDT",
		TradeId:  "586760148",
	}
	inc := parseIncome(exg, bill, nil)
	if inc == nil {
		t.Fatalf("unexpected nil income")
	}
	if inc.Income != 0.021933823221 {
		t.Fatalf("unexpected income: %v", inc.Income)
	}
	if inc.IncomeType != "8" || inc.Asset != "USDT" {
		t.Fatalf("unexpected income fields: %+v", inc)
	}
	if inc.TranID != bill.BillId || inc.TradeID != bill.TradeId {
		t.Fatalf("unexpected ids: %+v", inc)
	}
	if inc.Time != 1695033476167 {
		t.Fatalf("unexpected time: %v", inc.Time)
	}
	if inc.Symbol == "" {
		t.Fatalf("unexpected empty symbol")
	}
}

func TestParseIncomeFallback(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	seedMarket(exg, "ETH-USDT-SWAP", "ETH/USDT:USDT", banexg.MarketLinear)
	bill := &Bill{
		InstType: "SWAP",
		InstId:   "ETH-USDT-SWAP",
		BillId:   "1",
		Type:     "2",
		Ts:       "1700000000000",
		Pnl:      "-1.5",
		Ccy:      "USDT",
	}
	inc := parseIncome(exg, bill, nil)
	if inc == nil {
		t.Fatalf("unexpected nil income")
	}
	if inc.Income != -1.5 {
		t.Fatalf("unexpected income fallback: %v", inc.Income)
	}
}

func TestParseFundingRate(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	seedMarket(exg, "BTC-USDT-SWAP", "BTC/USDT:USDT", banexg.MarketLinear)
	item := &FundingRate{
		InstId:          "BTC-USDT-SWAP",
		FundingRate:     "0.0001",
		FundingTime:     "1743609600000",
		NextFundingRate: "0.0002",
		NextFundingTime: "1743638400000",
		InterestRate:    "0.0003",
		Ts:              "1743588686291",
	}
	res := parseFundingRate(exg, item, nil)
	if res == nil {
		t.Fatalf("unexpected nil funding rate")
	}
	if res.FundingRate != 0.0001 || res.NextFundingRate != 0.0002 {
		t.Fatalf("unexpected rates: %+v", res)
	}
	if res.FundingTimestamp != 1743609600000 || res.NextFundingTimestamp != 1743638400000 {
		t.Fatalf("unexpected timestamps: %+v", res)
	}
}

func TestParseFundingRateHistory(t *testing.T) {
	exg, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	seedMarket(exg, "BTC-USDT-SWAP", "BTC/USDT:USDT", banexg.MarketLinear)
	item := &FundingRateHistory{
		InstId:      "BTC-USDT-SWAP",
		FundingRate: "0.0000746604960499",
		FundingTime: "1703059200000",
		Method:      "next_period",
	}
	res := parseFundingRateHistory(exg, item, map[string]interface{}{"realizedRate": "0.0000746572360545"})
	if res == nil {
		t.Fatalf("unexpected nil funding rate history")
	}
	if res.FundingRate != 0.0000746604960499 {
		t.Fatalf("unexpected funding rate: %+v", res)
	}
	if res.Timestamp != 1703059200000 {
		t.Fatalf("unexpected timestamp: %+v", res)
	}
	if res.Symbol != "BTC/USDT:USDT" {
		t.Fatalf("unexpected symbol: %s", res.Symbol)
	}
}

func TestShouldContinueFundRateHistory(t *testing.T) {
	list := []*banexg.FundingRate{
		{Timestamp: 300},
		{Timestamp: 200},
		{Timestamp: 100},
	}
	next, ok := shouldContinueFundRateHistory(list, 3, 0, 0)
	if !ok || next != 100 {
		t.Fatalf("expected continue with next=100, got %v %v", next, ok)
	}
	next, ok = shouldContinueFundRateHistory(list[:2], 3, 0, 0)
	if ok {
		t.Fatalf("expected stop when page not full, got %v %v", next, ok)
	}
	next, ok = shouldContinueFundRateHistory(list, 3, 150, 0)
	if ok {
		t.Fatalf("expected stop when reaching since bound, got %v %v", next, ok)
	}
	next, ok = shouldContinueFundRateHistory(list, 3, 0, 100)
	if ok {
		t.Fatalf("expected stop when no progress, got %v %v", next, ok)
	}
}
