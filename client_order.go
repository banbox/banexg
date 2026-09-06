package banexg

import (
	"fmt"
	"math/rand"
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
