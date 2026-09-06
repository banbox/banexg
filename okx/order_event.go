package okx

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"github.com/banbox/banexg"
)

var _ banexg.OrderEventCapability = (*OKX)(nil)
var _ banexg.ClientOrderCapability = (*OKX)(nil)

// ParseClientOrderID parses OKX's fixed layout:
// {hash(botName)6}{orderID12}{suffix}.
func (*OKX) ParseClientOrderID(botName, clientID string) int64 {
	if clientID == "" || botName == "" || len(clientID) < 18 {
		return 0
	}
	if !strings.HasPrefix(clientID, okxClientOrderNamespace(botName)) {
		return 0
	}
	orderID, err := strconv.ParseInt(clientID[6:18], 10, 64)
	if err != nil || orderID <= 0 {
		return 0
	}
	return orderID
}

// AlgoOrderID returns the canonical ID of the OKX algo order related to a
// triggered order event.
func (*OKX) AlgoOrderID(trade *banexg.MyTrade) string {
	if trade == nil {
		return ""
	}
	algoID := strings.TrimSpace(trade.AlgoId)
	if algoID == "" {
		for _, key := range []string{"algoId", "algoID"} {
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
func (e *OKX) GetOrderEventOrderID(trade *banexg.MyTrade) string {
	return e.AlgoOrderID(trade)
}

// NormalizeOrderTimestamp returns the latest known millisecond timestamp for
// an order event while preserving the standard order fields.
func (*OKX) NormalizeOrderTimestamp(order *banexg.Order, fallback int64) int64 {
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

// BuildClientOrderID keeps OKX's alphanumeric fixed-width clOrdId layout at
// the adapter boundary. OKX does not accept the persisted client suffix.
func (*OKX) BuildClientOrderID(namespace string, orderID int64, _ string, randomize bool) string {
	randNum := 0
	if randomize {
		randNum = rand.Intn(10000)
	}
	return fmt.Sprintf("%s%012d%04d", okxClientOrderNamespace(namespace), orderID, randNum)
}

func okxClientOrderNamespace(namespace string) string {
	const alphaNum = "0123456789abcdefghijklmnopqrstuvwxyz"
	const size = 6
	var hash uint64
	for _, char := range namespace {
		hash = hash*31 + uint64(char)
	}
	result := make([]byte, size)
	for i := size - 1; i >= 0; i-- {
		result[i] = alphaNum[hash%36]
		hash /= 36
	}
	return string(result)
}
