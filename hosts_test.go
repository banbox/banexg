package banexg

import "testing"

func TestExgHostsSelect(t *testing.T) {
	newHosts := func() *ExgHosts {
		return &ExgHosts{
			Prod: map[string]string{"rest": "prod", "ws": "prod-ws"},
			Test: map[string]string{"rest": "test", "ws": "test-ws"},
			Variants: map[string]map[string]string{
				"demo":     {"rest": "demo"},
				"eea":      {"rest": "eea"},
				"eea-test": {"ws": "eea-test-ws"},
			},
		}
	}
	cases := []struct {
		env, site string
		custom    interface{}
		rest, ws  string
	}{
		{"", "", nil, "prod", "prod-ws"},
		{"prod", "eea", nil, "eea", "prod-ws"},
		{"test", "", nil, "test", "test-ws"},
		{"test", "eea", nil, "test", "eea-test-ws"},
		{"demo", "", nil, "demo", "prod-ws"},
		{"", "eea", map[string]string{"ws": "mine"}, "eea", "mine"},
		{"test", "", map[string]interface{}{"rest": "mine"}, "mine", "test-ws"},
	}
	for _, c := range cases {
		h := newHosts()
		if err := h.Select(c.env, c.site, c.custom); err != nil {
			t.Fatalf("select %q/%q: %v", c.env, c.site, err)
		}
		if h.TestNet != (c.env == "test") || h.GetHost("rest") != c.rest || h.GetHost("ws") != c.ws {
			t.Errorf("select %q/%q: testnet=%v rest=%s ws=%s", c.env, c.site, h.TestNet, h.GetHost("rest"), h.GetHost("ws"))
		}
	}
	for _, c := range [][2]string{{"paper", ""}, {"", "mars"}, {"demo", "eea"}} {
		if err := newHosts().Select(c[0], c[1], nil); err == nil {
			t.Errorf("select %q/%q should fail", c[0], c[1])
		}
	}
}
