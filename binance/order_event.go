package binance

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"github.com/banbox/banexg"
)

var _ banexg.OrderEventCapability = (*Binance)(nil)
var _ banexg.ClientOrderCapability = (*Binance)(nil)

// ParseClientOrderID parses Binance's {botName}_{orderID}_... format.
func (*Binance) ParseClientOrderID(botName, clientID string) int64 {
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

// AlgoOrderID returns the canonical ID of the Binance algo order related to
// a triggered order event.
func (*Binance) AlgoOrderID(trade *banexg.MyTrade) string {
	if trade == nil {
		return ""
	}
	algoID := strings.TrimSpace(trade.AlgoId)
	if algoID == "" {
		for _, key := range []string{"algoId", "aid"} {
			if value, ok := trade.Info[key].(string); ok {
				algoID = strings.TrimSpace(value)
				if algoID != "" {
					break
				}
			}
		}
	}
	if algoID == "" {
		return ""
	}
	algoID = strings.TrimPrefix(algoID, "algo:")
	return "algo:" + algoID
}

// GetOrderEventOrderID returns the canonical ID used by standard orders.
func (e *Binance) GetOrderEventOrderID(trade *banexg.MyTrade) string {
	return e.AlgoOrderID(trade)
}

// NormalizeOrderTimestamp returns the latest known millisecond timestamp for
// an order event. Binance order creation time is not always the event time.
func (*Binance) NormalizeOrderTimestamp(order *banexg.Order, fallback int64) int64 {
	if order == nil {
		return fallback
	}
	timestamp := fallback
	if order.Timestamp > timestamp {
		timestamp = order.Timestamp
	}
	if order.LastTradeTimestamp > timestamp {
		timestamp = order.LastTradeTimestamp
	}
	if order.LastUpdateTimestamp > timestamp {
		timestamp = order.LastUpdateTimestamp
	}
	return timestamp
}

// BuildClientOrderID keeps Binance's underscore-separated layout at the
// adapter boundary. The client ID is an optional persisted suffix.
func (*Binance) BuildClientOrderID(namespace string, orderID int64, clientID string, randomize bool) string {
	if randomize {
		return fmt.Sprintf("%s_%d_%d_%s", namespace, orderID, rand.Intn(1000), clientID)
	}
	return fmt.Sprintf("%s_%d_%s", namespace, orderID, clientID)
}
