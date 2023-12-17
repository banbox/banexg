package utils

import (
	"fmt"
	"testing"
)

type TestCase struct {
	Input  string
	Output int
}

func TestPrecisionFromString(t *testing.T) {
	var cases = []TestCase{
		{"1e-4", 4},
		{"0.0001", 4},
		{"0.00000001", 8},
	}
	for _, it := range cases {
		out := PrecisionFromString(it.Input)
		if out == it.Output {
			t.Logf("pass %s %v", it.Input, out)
		} else {
			t.Errorf("FAIL %s %v expect: %v", it.Input, out, it.Output)
		}
	}
}

type DDD struct {
	data map[string]string
}

func TestSetFieldBy(t *testing.T) {
	holder := DDD{data: map[string]string{}}
	fmt.Printf("data: %v %v\n", holder.data, holder.data == nil)
	items := make(map[string]interface{})
	SetFieldBy(&(holder.data), items, "123", nil)
	if holder.data == nil {
		t.Errorf("Fail SetFieldBy, holder.data should not be nil")
	} else {
		fmt.Printf("Pass SetFieldBy")
	}
}

func TestParseTimeFrame(t *testing.T) {
	var cases = []TestCase{
		{"1m", 60},
		{"5s", 5},
		{"10S", 10},
		{"2H", 7200},
		{"2d", 172800},
		{"3w", 1814400},
		{"2M", 5184000},
		{"2Y", 63072000},
	}
	for _, it := range cases {
		out, err := ParseTimeFrame(it.Input)
		if err != nil {
			panic(err)
		}
		if out == it.Output {
			t.Logf("pass %s %v", it.Input, out)
		} else {
			t.Errorf("FAIL %s %v expect: %v", it.Input, out, it.Output)
		}
	}
}
