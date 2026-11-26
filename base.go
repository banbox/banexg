package banexg

import (
	"fmt"
	"maps"

	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/utils"
)

func (h *ExgHosts) GetHost(key string) string {
	var dict map[string]string
	if h.TestNet {
		dict = h.Test
	} else {
		dict = h.Prod
	}
	host, ok := dict[key]
	if !ok {
		panic(fmt.Sprintf("Entry not exist: %s", key))
	}
	return host
}

// Select picks the active hosts: env chooses production (""/"prod"), "test" or a named variant
// such as "demo"; site layers a regional variant on top; custom (map of host key to url) wins last.
func (h *ExgHosts) Select(env, site string, custom interface{}) *errs.Error {
	if h == nil {
		return nil
	}
	if env == "prod" {
		env = ""
	}
	h.TestNet = env == "test"
	hosts := h.Prod
	if h.TestNet {
		hosts = h.Test
	}
	var names []string
	if env != "" && !h.TestNet {
		names = append(names, env)
	}
	if site != "" && env != "" {
		names = append(names, site+"-"+env)
	} else if site != "" {
		names = append(names, site)
	}
	for _, name := range names {
		variant, ok := h.Variants[name]
		if !ok {
			return errs.NewMsg(errs.CodeParamInvalid, "unsupported hosts %q, available: %v", name, utils.KeysOfMap(h.Variants))
		}
		maps.Copy(hosts, variant)
	}
	switch items := custom.(type) {
	case map[string]string:
		maps.Copy(hosts, items)
	case map[string]interface{}:
		for key, val := range items {
			hosts[key] = fmt.Sprint(val)
		}
	}
	return nil
}

func (c *Credential) CheckFilled(keys map[string]bool) *errs.Error {
	var requires []string
	if c.ApiKey == "" && keys["ApiKey"] {
		requires = append(requires, "ApiKey")
	}
	if c.Secret == "" && keys["Secret"] {
		requires = append(requires, "Secret")
	}
	if c.UID == "" && keys["UID"] {
		requires = append(requires, "UID")
	}
	if c.Password == "" && keys["Password"] {
		requires = append(requires, "Password")
	}
	if len(requires) > 0 {
		return errs.NewMsg(errs.CodeCredsRequired, "credential required %v", requires)
	}
	return nil
}

func IsContract(marketType string) bool {
	return marketType == MarketFuture || marketType == MarketSwap ||
		marketType == MarketLinear || marketType == MarketInverse
}
