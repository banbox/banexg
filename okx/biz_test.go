package okx

import (
	"strings"
	"testing"

	"github.com/banbox/banexg"
)

func TestCollectInstTypes(t *testing.T) {
	got := collectInstTypes([]string{banexg.MarketSpot, banexg.MarketLinear, banexg.MarketInverse, banexg.MarketSpot})
	if len(got) != 2 {
		t.Fatalf("unexpected instTypes len: %v", got)
	}
	joined := strings.Join(got, ",")
	if !strings.Contains(joined, "SPOT") || !strings.Contains(joined, "SWAP") {
		t.Fatalf("unexpected instTypes: %v", got)
	}
}

func TestMakeSignPublicPrivate(t *testing.T) {
	pub, err := New(nil)
	if err != nil {
		t.Fatalf("new okx: %v", err)
	}
	apiPub := pub.Apis[MethodPublicGetInstruments]
	apiPub.Url = pub.GetHost(apiPub.Host) + "/" + apiPub.Path
	signPub := pub.Sign(apiPub, map[string]interface{}{"instType": "SPOT"})
	if signPub.Private {
		t.Fatalf("public api should not be private")
	}
	if !strings.Contains(signPub.Url, "instType=SPOT") {
		t.Fatalf("public url missing query: %s", signPub.Url)
	}
	if signPub.Headers.Get("OK-ACCESS-KEY") != "" {
		t.Fatalf("public api should not include auth headers")
	}

	priv, err := New(map[string]interface{}{
		banexg.OptApiKey:    "key",
		banexg.OptApiSecret: "secret",
		banexg.OptPassword:  "pass",
	})
	if err != nil {
		t.Fatalf("new okx with creds: %v", err)
	}
	apiPriv := priv.Apis[MethodAccountGetBalance]
	apiPriv.Url = priv.GetHost(apiPriv.Host) + "/" + apiPriv.Path
	signPriv := priv.Sign(apiPriv, map[string]interface{}{"ccy": "BTC"})
	if !signPriv.Private {
		t.Fatalf("private api should be private")
	}
	if signPriv.Headers.Get("OK-ACCESS-KEY") == "" || signPriv.Headers.Get("OK-ACCESS-PASSPHRASE") != "pass" {
		t.Fatalf("missing okx auth headers")
	}
	if !strings.Contains(signPriv.Url, "ccy=BTC") {
		t.Fatalf("private url missing query: %s", signPriv.Url)
	}
}

func TestDemoTradingHeaderAndHosts(t *testing.T) {
	const header = "x-simulated-trading"
	prod, _ := New(nil)
	demo, err := New(map[string]interface{}{banexg.OptEnv: "test"})
	if err != nil {
		t.Fatalf("new okx demo: %v", err)
	}
	api := demo.Apis[MethodPublicGetInstruments]
	api.Url = demo.GetHost(api.Host) + "/" + api.Path
	if got := demo.Sign(api, nil).Headers.Get(header); got != "1" {
		t.Fatalf("demo request must carry %s: 1, got %q", header, got)
	}
	if got := prod.Sign(prod.Apis[MethodPublicGetInstruments], nil).Headers.Get(header); got != "" {
		t.Fatalf("production request must not carry %s, got %q", header, got)
	}
	for _, hosts := range []map[string]string{prod.Hosts.Prod, prod.Hosts.Test} {
		for key, host := range hosts {
			if strings.Contains(host, ":8443") {
				t.Fatalf("retired websocket port in %s: %s", key, host)
			}
		}
	}
}

// ============================================================================
// API Integration Tests - require local.json with valid credentials
// Run manually with: go test -run TestAPI_LoadMarkets -v
// These tests are prefixed with TestAPI_ to distinguish them from unit tests.
// ============================================================================

func TestAPI_LoadMarkets(t *testing.T) {
	exg := getExchange(nil)
	markets, err := exg.LoadMarkets(false, nil)
	if err != nil {
		panic(err)
	}
	t.Logf("loaded %d markets", len(markets))
	// Print some sample markets
	count := 0
	for symbol, mar := range markets {
		if count >= 5 {
			break
		}
		t.Logf("market: %s, type: %s, active: %v", symbol, mar.Type, mar.Active)
		count++
	}
}

func TestRegionalSiteHosts(t *testing.T) {
	cases := []struct{ env, site, rest, ws string }{
		{"", "eea", "https://eea.okx.com/api/v5", "wss://wseea.okx.com/ws/v5/private"},
		{"test", "eea", "https://eea.okx.com/api/v5", "wss://wseeapap.okx.com/ws/v5/private"},
		{"", "us", "https://us.okx.com/api/v5", "wss://wsus.okx.com/ws/v5/private"},
		{"test", "us", "https://us.okx.com/api/v5", "wss://wsuspap.okx.com/ws/v5/private"},
	}
	for _, c := range cases {
		exg, err := New(map[string]interface{}{banexg.OptEnv: c.env, banexg.OptSite: c.site})
		if err != nil {
			t.Fatalf("new okx %s/%s: %v", c.env, c.site, err)
		}
		if exg.GetHost(HostPrivate) != c.rest || exg.GetHost(HostWsPrivate) != c.ws {
			t.Errorf("%s/%s hosts: %s %s", c.env, c.site, exg.GetHost(HostPrivate), exg.GetHost(HostWsPrivate))
		}
	}
	if _, err := New(map[string]interface{}{banexg.OptSite: "mars"}); err == nil {
		t.Fatal("unknown site should fail")
	}
}
