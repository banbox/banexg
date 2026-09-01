package banexg

// OrderEventOrderIDCapability is the narrow optional capability needed to map
// a triggered trade to its canonical algo order ID.
type OrderEventOrderIDCapability interface {
	GetOrderEventOrderID(trade *MyTrade) string
}

// OrderEventCapability contains exchange-owned order event semantics. The
// base Exchange supplies explicit no-correlation defaults for adapters that
// do not need exchange-specific handling.
type OrderEventCapability interface {
	OrderEventOrderIDCapability
	ParseClientOrderID(botName, clientID string) int64
	AlgoOrderID(trade *MyTrade) string
	NormalizeOrderTimestamp(order *Order, fallback int64) int64
}

type defaultOrderEventCapability struct{}

var _ OrderEventCapability = (*Exchange)(nil)

func (defaultOrderEventCapability) ParseClientOrderID(string, string) int64 {
	return 0
}

func (defaultOrderEventCapability) AlgoOrderID(trade *MyTrade) string {
	if trade == nil {
		return ""
	}
	return trade.AlgoId
}

func (defaultOrderEventCapability) GetOrderEventOrderID(*MyTrade) string {
	return ""
}

func (defaultOrderEventCapability) NormalizeOrderTimestamp(order *Order, fallback int64) int64 {
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

// The base exchange provides explicit no-correlation semantics for adapters
// that do not have exchange-specific order event identifiers.
func (*Exchange) ParseClientOrderID(botName, clientID string) int64 {
	return defaultOrderEventCapability{}.ParseClientOrderID(botName, clientID)
}

func (*Exchange) AlgoOrderID(trade *MyTrade) string {
	return defaultOrderEventCapability{}.AlgoOrderID(trade)
}

func (*Exchange) GetOrderEventOrderID(trade *MyTrade) string {
	return defaultOrderEventCapability{}.GetOrderEventOrderID(trade)
}

func (*Exchange) NormalizeOrderTimestamp(order *Order, fallback int64) int64 {
	return defaultOrderEventCapability{}.NormalizeOrderTimestamp(order, fallback)
}

// GetOrderEventCapability returns an exchange capability when it is exposed,
// otherwise a safe implementation that never guesses an order association.
func GetOrderEventCapability(exg interface{}) OrderEventCapability {
	if capability, ok := exg.(OrderEventCapability); ok && capability != nil {
		return capability
	}
	return defaultOrderEventCapability{}
}

// ParseClientOrderID delegates to an optional exchange capability.
func ParseClientOrderID(exg interface{}, botName, clientID string) int64 {
	return GetOrderEventCapability(exg).ParseClientOrderID(botName, clientID)
}

// AlgoOrderID delegates to an optional exchange capability.
func AlgoOrderID(exg interface{}, trade *MyTrade) string {
	return GetOrderEventCapability(exg).AlgoOrderID(trade)
}

// GetOrderEventOrderID delegates to the narrow optional capability.
func GetOrderEventOrderID(exg interface{}, trade *MyTrade) string {
	capability, ok := exg.(OrderEventOrderIDCapability)
	if !ok || capability == nil {
		return ""
	}
	return capability.GetOrderEventOrderID(trade)
}

// NormalizeOrderTimestamp delegates to an optional exchange capability.
func NormalizeOrderTimestamp(exg interface{}, order *Order, fallback int64) int64 {
	return GetOrderEventCapability(exg).NormalizeOrderTimestamp(order, fallback)
}
