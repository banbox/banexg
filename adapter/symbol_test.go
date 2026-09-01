package adapter_test

import (
	"testing"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/china"
)

func TestChinaPriceSymbolCapability(t *testing.T) {
	exchange, err := china.New(nil)
	if err != nil {
		t.Fatalf("create China adapter: %v", err)
	}
	capability := banexg.PriceSymbolCapability(exchange)
	parts, err := capability.PriceSymbolParts("IF2409")
	if err != nil {
		t.Fatalf("PriceSymbolParts: %v", err)
	}
	if want := [4]string{"IF", "CNY", "CNY", "2409"}; parts != want {
		t.Fatalf("PriceSymbolParts() = %#v, want %#v", parts, want)
	}
	if _, err := capability.PriceSymbolParts(""); err == nil {
		t.Fatal("empty symbol accepted")
	}
}
