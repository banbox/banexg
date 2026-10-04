package binance

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/banbox/banexg"
)

func verifyTestExchange(t *testing.T, target string, hedge, disabled bool) *Binance {
	accountCalls := 0
	e := fundingTestExchange(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Query().Has(banexg.ParamContext) || r.URL.Query().Has(banexg.ParamFullSnapshot) || r.URL.Query().Has(banexg.ParamSettledCash) || r.URL.Query().Has(banexg.ParamAccount) {
			t.Errorf("internal controls leaked: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("X-MBX-APIKEY") != "shared-key" {
			t.Errorf("requested wrong account on %s", r.URL.Path)
		}
		if strings.HasPrefix(target, "null:") && strings.HasSuffix(r.URL.Path, strings.TrimPrefix(target, "null:")) {
			fmt.Fprint(w, `null`)
			return
		}
		if target != "" && strings.HasSuffix(r.URL.Path, target) {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/apiRestrictions"):
			if strings.HasPrefix(target, "permissions:") {
				fmt.Fprint(w, strings.TrimPrefix(target, "permissions:"))
				return
			}
			fmt.Fprintf(w, `{"enableFutures":%t,"enableWithdrawals":false,"ipRestrict":true}`, !disabled)
		case strings.HasSuffix(r.URL.Path, "/positionSide/dual"):
			fmt.Fprintf(w, `{"dualSidePosition":%t}`, hedge)
		case strings.HasSuffix(r.URL.Path, "/account"):
			accountCalls++
			if strings.HasPrefix(target, "balance:") {
				fmt.Fprint(w, strings.TrimPrefix(target, "balance:"))
				return
			}
			if accountCalls > 1 && strings.HasPrefix(target, "positions:") {
				fmt.Fprint(w, strings.TrimPrefix(target, "positions:"))
				return
			}
			fmt.Fprint(w, `{"canTrade":true,"assets":[{"asset":"USDT","walletBalance":"10","marginBalance":"11","unrealizedProfit":"1","maintMargin":"0"}],"positions":[]}`)
		case strings.HasSuffix(r.URL.Path, "/positionRisk"), strings.HasSuffix(r.URL.Path, "/openOrders"), strings.HasSuffix(r.URL.Path, "/openAlgoOrders"), strings.HasSuffix(r.URL.Path, "/leverageBracket"):
			fmt.Fprint(w, `[]`)
		default:
			t.Errorf("unexpected verification request: %s", r.URL.Path)
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	})
	for _, host := range []string{HostSApi, HostFApiPrivateV2} {
		e.Hosts.Prod[host] = e.Hosts.Prod[HostFApiPrivate]
	}
	return e
}

func TestExecutionRequestContextPreservesSharedCallerParams(t *testing.T) {
	e := fundingTestExchange(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has(banexg.ParamContext) || r.URL.Query().Has(banexg.ParamAccount) {
			t.Errorf("context or account leaked into signature: %s", r.URL)
		}
		<-r.Context().Done()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	params := map[string]interface{}{banexg.ParamContext: ctx, banexg.ParamAccount: "shared", "incomeType": "FUNDING_FEE"}
	var wg sync.WaitGroup
	results := make(chan *banexg.HttpRes, 8)
	start := time.Now()
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- e.RequestApiRetryAdv(context.Background(), MethodFapiPrivateGetIncome, params, 0, false, false)
		}()
	}
	wg.Wait()
	close(results)
	for rsp := range results {
		if rsp == nil || rsp.Error == nil {
			t.Fatal("shared caller context did not cancel request")
		}
	}
	if time.Since(start) > time.Second {
		t.Fatal("parallel requests exceeded caller deadline")
	}
	if params[banexg.ParamContext] != ctx || params[banexg.ParamAccount] != "shared" || params["incomeType"] != "FUNDING_FEE" {
		t.Fatalf("caller params mutated: %+v", params)
	}
	rsp := e.RequestApiRetryAdv(context.Background(), MethodFapiPrivateGetIncome, params, 0, false, false)
	if rsp == nil || rsp.Error == nil {
		t.Fatal("reused canceled context was lost")
	}
}

func TestExecutionVerifySettledCashSnapshot(t *testing.T) {
	for _, wallet := range []string{"0", "10", "bad"} {
		t.Run(wallet, func(t *testing.T) {
			e := fundingTestExchange(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Query().Has(banexg.ParamSettledCash) || r.URL.Query().Has(banexg.ParamFullSnapshot) || r.URL.Query().Has(banexg.ParamContext) {
					t.Errorf("invalid settled cash request: %s %s", r.Method, r.URL)
				}
				fmt.Fprintf(w, `{"assets":[{"asset":"USDT","walletBalance":%q,"marginBalance":"11","unrealizedProfit":"1","maintMargin":"0"}]}`, wallet)
			})
			e.Hosts.Prod[HostFApiPrivateV2] = e.Hosts.Prod[HostFApiPrivate]
			balance, err := e.FetchBalance(map[string]interface{}{banexg.ParamAccount: "shared", banexg.ParamFullSnapshot: true, banexg.ParamSettledCash: true})
			if wallet == "bad" {
				if err == nil || balance != nil {
					t.Fatalf("invalid wallet accepted: %+v %v", balance, err)
				}
				return
			}
			if err != nil || balance == nil {
				t.Fatalf("missing balance: %v", err)
			}
			cash, ok := balance.Total["USDT"]
			want := float64(0)
			if wallet == "10" {
				want = 10
			}
			if !ok || cash != want {
				t.Fatalf("settled wallet cash lost or includes unrealized pnl: %+v", balance.Total)
			}
		})
	}
}

func TestExecutionVerifyRejectsNullSnapshot(t *testing.T) {
	for _, endpoint := range []string{"/positionRisk", "/openOrders", "/openAlgoOrders"} {
		t.Run(endpoint, func(t *testing.T) {
			e := verifyTestExchange(t, "null:"+endpoint, false, false)
			proof, err := e.VerifyExecution(context.Background(), "shared", "USDT")
			if err == nil || proof.CompleteAccountSnapshot {
				t.Fatalf("null snapshot accepted: %+v %v", proof, err)
			}
		})
	}
}

func TestExecutionVerifyReadOnlyAccount(t *testing.T) {
	e := verifyTestExchange(t, "", false, false)
	proof, err := e.VerifyExecution(context.Background(), "shared", "USDT")
	if err != nil || proof.Account != "shared" || proof.Currency != "USDT" || proof.EvidenceID == "" || !proof.ContextBound || !proof.CompleteAccountSnapshot || !proof.CompleteCumulativeReports || !proof.SettledCash || !proof.NetLinearPositions || proof.AuthoritativeNotFound {
		t.Fatalf("verification proof: %+v %v", proof, err)
	}
}

func TestExecutionVerifyAccountPositionSnapshot(t *testing.T) {
	for _, raw := range []string{`null`, `{}`, `{"positions":null}`, `{"positions":{}}`, `{"positions":[{"symbol":"UNKNOWN","positionAmt":"1"}]}`, `{"positions":[{"symbol":"BTCUSDT","positionAmt":"1"}]}`, `{"positions":[]}`} {
		t.Run(raw, func(t *testing.T) {
			e := verifyTestExchange(t, "positions:"+raw, false, false)
			e.Options[banexg.OptPositionMethod] = "account"
			proof, err := e.VerifyExecution(context.Background(), "shared", "USDT")
			if raw == `{"positions":[]}` {
				if err != nil || !proof.CompleteAccountSnapshot {
					t.Fatalf("explicit empty positions rejected: %+v %v", proof, err)
				}
			} else if err == nil || proof.CompleteAccountSnapshot {
				t.Fatalf("incomplete account positions accepted: %+v %v", proof, err)
			}
		})
	}
}

func TestExecutionVerifyRequiresTradingPermission(t *testing.T) {
	for _, target := range []string{
		`permissions:{}`, `permissions:{"enableFutures":null}`, `permissions:{"enableFutures":false}`,
		`balance:{"assets":[{"asset":"USDT","walletBalance":"10"}]}`,
		`balance:{"canTrade":false,"assets":[{"asset":"USDT","walletBalance":"10"}]}`,
		`balance:{"canTrade":null,"assets":[{"asset":"USDT","walletBalance":"10"}]}`,
	} {
		t.Run(target, func(t *testing.T) {
			e := verifyTestExchange(t, target, false, false)
			proof, err := e.VerifyExecution(context.Background(), "shared", "USDT")
			if err == nil || proof.CompleteAccountSnapshot || proof.ContextBound {
				t.Fatalf("unverified trading permission accepted: %+v %v", proof, err)
			}
		})
	}
}

func TestExecutionVerifyRejectsUnsafeModeAndPermissions(t *testing.T) {
	for _, tc := range []struct {
		name            string
		hedge, disabled bool
	}{{"hedge", true, false}, {"trading-disabled", false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			e := verifyTestExchange(t, "", tc.hedge, tc.disabled)
			proof, err := e.VerifyExecution(context.Background(), "shared", "USDT")
			if err == nil || proof.CompleteAccountSnapshot || proof.ContextBound {
				t.Fatalf("unsafe execution accepted: %+v %v", proof, err)
			}
		})
	}
}

func TestExecutionVerifyEveryRequestHonorsDeadline(t *testing.T) {
	for _, endpoint := range []string{"/positionSide/dual", "/account", "/leverageBracket", "/positionRisk", "/openOrders", "/openAlgoOrders"} {
		t.Run(endpoint, func(t *testing.T) {
			e := verifyTestExchange(t, endpoint, false, false)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			start := time.Now()
			proof, err := e.VerifyExecution(ctx, "shared", "USDT")
			if err == nil || proof.ContextBound || time.Since(start) > 300*time.Millisecond {
				t.Fatalf("deadline ignored on %s: %+v %v elapsed=%s", endpoint, proof, err, time.Since(start))
			}
		})
	}
}
