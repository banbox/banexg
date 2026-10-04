package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/banbox/banexg"
)

func completeOrderExchange(t *testing.T, handler http.HandlerFunc) *Binance {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	e, err := New(map[string]interface{}{banexg.OptApiKey: "unit-key", banexg.OptApiSecret: "unit-secret", banexg.OptMarketType: banexg.MarketLinear})
	if err != nil {
		t.Fatal(err)
	}
	e.HttpClient = server.Client()
	e.EnableRateLimit = banexg.BoolFalse
	e.Hosts.Prod[HostFApiPrivate] = server.URL
	e.Markets = banexg.MarketMap{"BTC/USDT:USDT": {ID: "BTCUSDT", Symbol: "BTC/USDT:USDT", Type: banexg.MarketLinear, Linear: true, Swap: true, Settle: "USDT"}}
	return e
}

func completeOrderJSON(quantity, quote string) string {
	amount := quantity
	status := "FILLED"
	if quantity == "0" {
		amount, status = "1", "NEW"
	}
	return fmt.Sprintf(`{"orderId":42,"symbol":"BTCUSDT","clientOrderId":"stable-id","status":%q,"type":"LIMIT","side":"BUY","executedQty":%q,"origQty":%q,"price":"100","avgPrice":"100","cumQuote":%q,"time":100,"updateTime":200}`, status, quantity, amount, quote)
}

func completeTrade(id int, quantity, quote, commission, currency string) map[string]interface{} {
	return map[string]interface{}{"id": id, "orderId": 42, "symbol": "BTCUSDT", "qty": quantity, "price": "100", "quoteQty": quote, "commission": commission, "commissionAsset": currency, "time": 150, "side": "BUY", "maker": true}
}

func TestCompleteOrderCumulativeExecution(t *testing.T) {
	for _, tc := range []struct {
		name, quantity, quote string
		trades                []map[string]interface{}
		wantFee               float64
		wantError             bool
	}{
		{"filled", "0.3", "30", []map[string]interface{}{completeTrade(1, "0.1", "10", "0.01", "USDT"), completeTrade(2, "0.2", "20", "0.02", "USDT")}, 0.03, false},
		{"unfilled", "0", "0", nil, 0, false},
		{"zero-cost-invalid", "0", "1", nil, 0, true},
		{"missing-execution", "0.3", "30", []map[string]interface{}{completeTrade(1, "0.1", "10", "0.01", "USDT")}, 0, true},
		{"newer-execution", "0.1", "10", []map[string]interface{}{completeTrade(1, "0.2", "20", "0.02", "USDT")}, 0, true},
		{"wrong-fee-currency", "0.1", "10", []map[string]interface{}{completeTrade(1, "0.1", "10", "0.01", "BNB")}, 0, true},
		{"duplicate-execution", "0.2", "20", []map[string]interface{}{completeTrade(1, "0.1", "10", "0.01", "USDT"), completeTrade(1, "0.1", "10", "0.01", "USDT")}, 0, true},
		{"malformed-fee", "0.1", "10", []map[string]interface{}{completeTrade(1, "0.1", "10", "bad", "USDT")}, 0, true},
		{"rebate", "0.1", "10", []map[string]interface{}{completeTrade(1, "0.1", "10", "-0.01", "USDT")}, -0.01, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tradeCalls := 0
			e := completeOrderExchange(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Query().Has(banexg.ParamCompleteOrder) || r.URL.Query().Has(banexg.ParamContext) {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}
				if strings.HasSuffix(r.URL.Path, "/order") {
					fmt.Fprint(w, completeOrderJSON(tc.quantity, tc.quote))
					return
				}
				tradeCalls++
				if r.URL.Query().Get("orderId") != "42" || r.URL.Query().Get("symbol") != "BTCUSDT" {
					t.Error("missing execution order filter")
				}
				json.NewEncoder(w).Encode(tc.trades)
			})
			order, err := e.FetchOrder("BTC/USDT:USDT", "42", map[string]interface{}{banexg.ParamCompleteOrder: true})
			if tc.wantError {
				if err == nil || order != nil {
					t.Fatalf("expected incomplete snapshot, got %+v, %v", order, err)
				}
				return
			}
			if err != nil || order == nil {
				t.Fatalf("FetchOrder: %v", err)
			}
			qty, _ := strconv.ParseFloat(tc.quantity, 64)
			cost, _ := strconv.ParseFloat(tc.quote, 64)
			if order.Fee == nil || order.Fee.Currency != "USDT" || order.Fee.Cost != tc.wantFee || order.Filled != qty || order.Cost != cost || order.LastUpdateTimestamp != 200 || order.Timestamp != 100 {
				t.Fatalf("bad cumulative snapshot: %+v fee=%+v", order, order.Fee)
			}
			if qty == 0 && tradeCalls != 0 {
				t.Fatal("zero-fill queried trades")
			}
			if qty > 0 && (order.LastTradeTimestamp != 150 || len(order.Trades) != len(tc.trades)) {
				t.Fatal("missing execution timestamp or trades")
			}
		})
	}
}

func TestCompleteOrderPagination(t *testing.T) {
	calls := 0
	e := completeOrderExchange(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/order") {
			fmt.Fprint(w, completeOrderJSON("1001", "100100"))
			return
		}
		calls++
		var trades []map[string]interface{}
		if calls == 1 {
			if r.URL.Query().Has("fromId") {
				t.Error("first page has cursor")
			}
			for i := 1; i <= 1000; i++ {
				trades = append(trades, completeTrade(i, "1", "100", "0.01", "USDT"))
			}
		} else {
			if r.URL.Query().Get("fromId") != "1001" {
				t.Error("wrong pagination cursor")
			}
			trades = append(trades, completeTrade(1001, "1", "100", "0.01", "USDT"))
		}
		json.NewEncoder(w).Encode(trades)
	})
	order, err := e.FetchOrder("BTC/USDT:USDT", "42", map[string]interface{}{banexg.ParamCompleteOrder: true})
	if err != nil || calls != 2 || order == nil || order.Fee.Cost != 10.01 || order.Filled != 1001 || len(order.Trades) != 1001 {
		t.Fatalf("pagination: calls=%d order=%+v err=%v", calls, order, err)
	}
}

func TestCompleteOrderRequiresSupportedStatus(t *testing.T) {
	for _, status := range []string{"missing", "", "UNKNOWN", "NEW", "CANCELED"} {
		t.Run(status, func(t *testing.T) {
			e := completeOrderExchange(t, func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/order") {
					t.Error("zero-fill snapshot queried executions")
				}
				var raw map[string]interface{}
				if err := json.Unmarshal([]byte(completeOrderJSON("0", "0")), &raw); err != nil {
					t.Fatal(err)
				}
				if status == "missing" {
					delete(raw, "status")
				} else {
					raw["status"] = status
				}
				json.NewEncoder(w).Encode(raw)
			})
			order, err := e.FetchOrder("BTC/USDT:USDT", "42", map[string]interface{}{banexg.ParamCompleteOrder: true})
			if status == "NEW" || status == "CANCELED" {
				if err != nil || order == nil {
					t.Fatalf("supported status rejected: %+v %v", order, err)
				}
			} else if err == nil || order != nil {
				t.Fatalf("invalid status accepted: %+v %v", order, err)
			}
		})
	}
}

func TestCompleteOrderStatusMustMatchFill(t *testing.T) {
	for _, tc := range []struct {
		status, quantity, amount string
	}{
		{"FILLED", "0.5", "1"}, {"FILLED", "0", "1"},
		{"NEW", "0.5", "1"},
		{"PARTIALLY_FILLED", "0", "1"}, {"PARTIALLY_FILLED", "1", "1"},
	} {
		t.Run(tc.status+tc.quantity, func(t *testing.T) {
			e := completeOrderExchange(t, func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/order") {
					t.Error("inconsistent snapshot queried executions")
				}
				var raw map[string]interface{}
				if err := json.Unmarshal([]byte(completeOrderJSON(tc.quantity, "0")), &raw); err != nil {
					t.Fatal(err)
				}
				raw["status"], raw["origQty"] = tc.status, tc.amount
				json.NewEncoder(w).Encode(raw)
			})
			order, err := e.FetchOrder("BTC/USDT:USDT", "42", map[string]interface{}{banexg.ParamCompleteOrder: true})
			if err == nil || order != nil {
				t.Fatalf("inconsistent status accepted: %+v %v", order, err)
			}
		})
	}
}

func TestCompleteOrderContextDeadline(t *testing.T) {
	e := completeOrderExchange(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/order") {
			fmt.Fprint(w, completeOrderJSON("1", "100"))
			return
		}
		<-r.Context().Done()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	order, err := e.FetchOrder("BTC/USDT:USDT", "42", map[string]interface{}{banexg.ParamCompleteOrder: true, banexg.ParamContext: ctx})
	if err == nil || order != nil || time.Since(start) > time.Second {
		t.Fatalf("deadline not honored: %v", err)
	}
}
