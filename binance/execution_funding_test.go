package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/banbox/banexg"
)

func fundingTestExchange(t *testing.T, handler http.HandlerFunc) *Binance {
	e := completeOrderExchange(t, handler)
	for _, api := range e.Apis {
		api.CacheSecs = 0
	}
	e.Hosts.Prod[HostFApiPublic] = e.Hosts.Prod[HostFApiPrivate]
	e.MarketsById = banexg.MarketArrMap{"BTCUSDT": {e.Markets["BTC/USDT:USDT"]}}
	e.Accounts["shared"] = &banexg.Account{Name: "shared", Creds: &banexg.Credential{ApiKey: "shared-key", Secret: "shared-secret"}}
	return e
}

func fundingIncome(id int) map[string]interface{} {
	return map[string]interface{}{"symbol": "BTCUSDT", "incomeType": "FUNDING_FEE", "income": "-0.1234567890123456789", "asset": "USDT", "time": 1000, "tranId": id}
}

func TestExecutionFundingCashExactAndFailClosed(t *testing.T) {
	for _, name := range []string{"exact", "null-page", "wrong-symbol", "wrong-time", "missing-mark", "bad-rate", "bad-income", "wrong-income-type", "duplicate-income"} {
		t.Run(name, func(t *testing.T) {
			e := fundingTestExchange(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Query().Has(banexg.ParamContext) || r.URL.Query().Has(banexg.ParamAccount) {
					t.Errorf("internal controls leaked: %s %s", r.Method, r.URL)
				}
				if strings.HasSuffix(r.URL.Path, "/income") {
					if name == "null-page" {
						fmt.Fprint(w, `null`)
						return
					}
					if r.Header.Get("X-MBX-APIKEY") != "shared-key" || r.URL.Query().Get("incomeType") != "FUNDING_FEE" {
						t.Error("wrong account or income filter")
					}
					row := fundingIncome(42)
					if name == "bad-income" {
						row["income"] = "bad"
					}
					if name == "wrong-income-type" {
						row["incomeType"] = "REALIZED_PNL"
					}
					rows := []map[string]interface{}{row}
					if name == "duplicate-income" {
						rows = append(rows, row)
					}
					json.NewEncoder(w).Encode(rows)
					return
				}
				if r.URL.Query().Get("startTime") != "1000" || r.URL.Query().Get("endTime") != "1000" || r.URL.Query().Get("symbol") != "BTCUSDT" {
					t.Error("funding rate was not queried at exact funding time")
				}
				row := map[string]interface{}{"symbol": "BTCUSDT", "fundingTime": 1000, "markPrice": "100.1234567890123456789", "fundingRate": "-0.0001234567890123456789"}
				if name == "wrong-symbol" {
					row["symbol"] = "ETHUSDT"
				}
				if name == "wrong-time" {
					row["fundingTime"] = 1001
				}
				if name == "missing-mark" {
					delete(row, "markPrice")
				}
				if name == "bad-rate" {
					row["fundingRate"] = "bad"
				}
				json.NewEncoder(w).Encode([]map[string]interface{}{row})
			})
			rows, err := e.FetchFundingCash(context.Background(), "shared", "USDT", 900, 1100)
			if name != "exact" {
				if err == nil || rows != nil {
					t.Fatalf("expected incomplete funding rejection; rows=%+v err=%v", rows, err)
				}
				return
			}
			if err != nil || len(rows) != 1 {
				t.Fatalf("funding cash: %+v %v", rows, err)
			}
			row := rows[0]
			if row.ID != "funding/USDT/42" || row.Symbol != "BTC/USDT:USDT" || row.Amount != "-0.1234567890123456789" || row.Mark != "100.1234567890123456789" || row.Rate != "-0.0001234567890123456789" || row.AtMS != 1000 {
				t.Fatalf("precision or identity lost: %+v", row)
			}
		})
	}
}

func TestExecutionFundingCashPagination(t *testing.T) {
	pages := 0
	e := fundingTestExchange(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/fundingRate") {
			fmt.Fprint(w, `[{"symbol":"BTCUSDT","fundingTime":1000,"markPrice":"100","fundingRate":"0.001"}]`)
			return
		}
		pages++
		if r.URL.Query().Get("page") != fmt.Sprint(pages) {
			t.Error("nonadvancing income page")
		}
		var rows []map[string]interface{}
		if pages == 1 {
			for i := 1; i <= 1000; i++ {
				row := fundingIncome(i)
				row["asset"] = "USDC"
				rows = append(rows, row)
			}
		} else {
			rows = append(rows, fundingIncome(1001))
		}
		json.NewEncoder(w).Encode(rows)
	})
	rows, err := e.FetchFundingCash(context.Background(), "shared", "USDT", 900, 1100)
	if err != nil || pages != 2 || len(rows) != 1 || rows[0].ID != "funding/USDT/1001" {
		t.Fatalf("pagination: pages=%d rows=%+v err=%v", pages, rows, err)
	}
}

func TestExecutionFundingCashDeadline(t *testing.T) {
	e := fundingTestExchange(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	rows, err := e.FetchFundingCash(ctx, "shared", "USDT", 900, 1100)
	if err == nil || rows != nil || time.Since(start) > time.Second {
		t.Fatalf("unbounded funding request: %v", err)
	}
}

func TestExecutionFundingCashRejectsRepeatedFilteredPage(t *testing.T) {
	pages := 0
	e := fundingTestExchange(t, func(w http.ResponseWriter, r *http.Request) {
		pages++
		rows := make([]map[string]interface{}, 1000)
		for i := range rows {
			rows[i] = fundingIncome(i + 1)
			rows[i]["asset"] = "USDC"
		}
		json.NewEncoder(w).Encode(rows)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	rows, err := e.FetchFundingCash(ctx, "shared", "USDT", 900, 1100)
	if err == nil || rows != nil || pages != 2 {
		t.Fatalf("repeated filtered page was not rejected: pages=%d rows=%+v err=%v", pages, rows, err)
	}
}
