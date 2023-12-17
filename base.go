package banexg

import (
	"fmt"
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

func (c *Credential) CheckFilled() error {
	var requires []string
	if c.ApiKey == "" && c.Keys["ApiKey"] {
		requires = append(requires, "ApiKey")
	}
	if c.Secret == "" && c.Keys["Secret"] {
		requires = append(requires, "Secret")
	}
	if c.UID == "" && c.Keys["UID"] {
		requires = append(requires, "UID")
	}
	if c.Password == "" && c.Keys["Password"] {
		requires = append(requires, "Password")
	}
	if len(requires) > 0 {
		return fmt.Errorf("credential required %v", requires)
	}
	return nil
}
