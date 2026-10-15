package banexg

import (
	"fmt"
	"math/rand"

	"github.com/banbox/banexg/errs"
)

// ClientOrderCapability exposes the adapter-owned client order ID contract.
// Different exchanges impose different character sets and layouts, so the
// caller must not construct these identifiers from an exchange name.
type ClientOrderCapability interface {
	BuildClientOrderID(namespace string, orderID int64, clientID string, randomize bool) string
}

// ClientOrderIDCapability is an alias for callers that use the identifier
// terminology already present in normalized order data.
type ClientOrderIDCapability = ClientOrderCapability

// ClientOrderPurpose distinguishes the submissions that share one local order
// id. Entry and exit are the two, and both must stay resolvable on their own.
type ClientOrderPurpose string

const (
	ClientOrderPurposeEnter ClientOrderPurpose = "enter"
	ClientOrderPurposeExit  ClientOrderPurpose = "exit"
)

// ClientOrderPurposeCapability builds a stable client order id for one purpose
// of a local order. Adapters that implement it promise that an entry and its
// exit can never collide, and that rebuilding the id for the same order and
// purpose returns the same value, which makes a resubmit idempotent.
type ClientOrderPurposeCapability interface {
	BuildPurposeClientOrderID(namespace string, orderID int64, clientID string, purpose ClientOrderPurpose) string
}

// GetClientOrderPurposeCapability returns the optional adapter capability.
func GetClientOrderPurposeCapability(exchange interface{}) ClientOrderPurposeCapability {
	capability, ok := exchange.(ClientOrderPurposeCapability)
	if !ok || capability == nil {
		return nil
	}
	return capability
}

// SubmitReconcileCapability resolves a submission whose response never arrived.
// Only the adapter knows which transport failures leave the outcome unknown and
// how to ask the exchange for the order behind a client order id.
type SubmitReconcileCapability interface {
	// IndeterminateSubmit reports whether a failed submission may still have
	// been accepted by the exchange.
	IndeterminateSubmit(err *errs.Error) bool
	// FindSubmittedOrder looks an order up by its client order id. A nil order
	// with a nil error means the exchange confirmed the order does not exist.
	FindSubmittedOrder(symbol, clientOrderID string, params map[string]interface{}) (*Order, *errs.Error)
}

// GetSubmitReconcileCapability returns the optional adapter capability.
func GetSubmitReconcileCapability(exchange interface{}) SubmitReconcileCapability {
	capability, ok := exchange.(SubmitReconcileCapability)
	if !ok || capability == nil {
		return nil
	}
	return capability
}

// GetClientOrderCapability returns the optional adapter capability.
func GetClientOrderCapability(exchange interface{}) ClientOrderCapability {
	capability, ok := exchange.(ClientOrderCapability)
	if !ok || capability == nil {
		return nil
	}
	return capability
}

// BuildClientOrderID delegates formatting to an adapter. The boolean reports
// whether the adapter exposes the capability; an empty identifier is valid
// only as the result of an adapter that deliberately returned one.
func BuildClientOrderID(exchange interface{}, namespace string, orderID int64, clientID string, randomize bool) (string, bool) {
	capability := GetClientOrderCapability(exchange)
	if capability == nil {
		return "", false
	}
	return capability.BuildClientOrderID(namespace, orderID, clientID, randomize), true
}

// BuildLegacyClientOrderID preserves the pre-capability format for callers
// that do not have an adapter instance yet. Exchange-specific layouts remain
// owned by this boundary rather than being reconstructed by consumers.
func BuildLegacyClientOrderID(exchangeName, namespace string, orderID int64, clientID string, randomize bool) string {
	if exchangeName == "okx" {
		randNum := 0
		if randomize {
			randNum = rand.Intn(10000)
		}
		return fmt.Sprintf("%s%012d%04d", hashClientOrderNamespace(namespace), orderID, randNum)
	}
	if randomize {
		return fmt.Sprintf("%s_%d_%d_%s", namespace, orderID, rand.Intn(1000), clientID)
	}
	return fmt.Sprintf("%s_%d_%s", namespace, orderID, clientID)
}

func hashClientOrderNamespace(value string) string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	var hash uint64
	for _, char := range value {
		hash = hash*31 + uint64(char)
	}
	const size = 6
	result := make([]byte, size)
	for i := size - 1; i >= 0; i-- {
		result[i] = alphabet[hash%uint64(len(alphabet))]
		hash /= uint64(len(alphabet))
	}
	return string(result)
}
