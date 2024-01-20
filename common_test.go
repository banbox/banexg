package banexg

import (
	"fmt"
	"testing"
)

func TestSliceInsert(t *testing.T) {
	a := []int{1, 2, 3, 6, 8}
	index := 5
	val := 23
	a = append(a, 0)
	copy(a[index+1:], a[index:])
	a[index] = val
	fmt.Printf("%v", a)
}
