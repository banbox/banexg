package bybit

import (
	"strconv"
	"strings"

	"github.com/banbox/banexg"
)

var _ banexg.OrderEventCapability = (*Bybit)(nil)

// ParseClientOrderID parses Bybit's {botName}_{orderID}_... format.
func (*Bybit) ParseClientOrderID(botName, clientID string) int64 {
	if clientID == "" || botName == "" {
		return 0
	}
	prefix := botName + "_"
	if !strings.HasPrefix(clientID, prefix) {
		return 0
	}
	value := clientID[len(prefix):]
	if separator := strings.IndexByte(value, '_'); separator >= 0 {
		value = value[:separator]
	}
	orderID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || orderID <= 0 {
		return 0
	}
	return orderID
}
