package banexg

import (
	"fmt"
	"github.com/banbox/banexg/errs"
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
