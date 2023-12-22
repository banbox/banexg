package utils

import (
	"github.com/shopspring/decimal"
	"testing"
)

type DecPrecCase struct {
	text      string
	precision string
	countMode int
	isRound   bool
	padZero   bool
	output    string
}

func TestNumToPrecs(t *testing.T) {
	items := []DecPrecCase{
		{"12.3456000", "100", PrecModeDecimalPlace, false, false, "12.3456"},
		{"12.3456", "100", PrecModeDecimalPlace, false, false, "12.3456"},
		{"12.3456", "4", PrecModeDecimalPlace, false, false, "12.3456"},
		{"12.3456", "3", PrecModeDecimalPlace, false, false, "12.345"},
		{"12.3456", "2", PrecModeDecimalPlace, false, false, "12.34"},
		{"12.3456", "1", PrecModeDecimalPlace, false, false, "12.3"},
		{"12.3456", "0", PrecModeDecimalPlace, false, false, "12"},
		{"0.0000001", "8", PrecModeDecimalPlace, false, false, "0.0000001"},
		{"0.00000001", "8", PrecModeDecimalPlace, false, false, "0.00000001"},
		{"0.000000000", "9", PrecModeDecimalPlace, false, true, "0.000000000"},
		{"0.000000001", "9", PrecModeDecimalPlace, false, true, "0.000000001"},
		{"12.3456", "-1", PrecModeDecimalPlace, false, false, "10"},
		{"123.456", "-1", PrecModeDecimalPlace, false, false, "120"},
		{"123.456", "-2", PrecModeDecimalPlace, false, false, "100"},
		{"9.99999", "-1", PrecModeDecimalPlace, false, false, "0"},
		{"99.9999", "-1", PrecModeDecimalPlace, false, false, "90"},
		{"99.9999", "-2", PrecModeDecimalPlace, false, false, "0"},
		{"0", "0", PrecModeDecimalPlace, false, false, "0"},
		{"-0.9", "0", PrecModeDecimalPlace, false, false, "0"},
		{"0.000123456700", "100", PrecModeSignifDigits, false, false, "0.0001234567"},
		{"0.0001234567", "100", PrecModeSignifDigits, false, false, "0.0001234567"},
		{"0.0001234567", "7", PrecModeSignifDigits, false, false, "0.0001234567"},
		{"0.000123456", "6", PrecModeSignifDigits, false, false, "0.000123456"},
		{"0.000123456", "5", PrecModeSignifDigits, false, false, "0.00012345"},
		{"0.000123456", "2", PrecModeSignifDigits, false, false, "0.00012"},
		{"0.000123456", "1", PrecModeSignifDigits, false, false, "0.0001"},
		{"123.0000987654", "10", PrecModeSignifDigits, false, true, "123.0000987"},
		{"123.0000987654", "8", PrecModeSignifDigits, false, false, "123.00009"},
		{"123.0000987654", "7", PrecModeSignifDigits, false, true, "123.0000"},
		{"123.0000987654", "6", PrecModeSignifDigits, false, false, "123"},
		{"123.0000987654", "5", PrecModeSignifDigits, false, true, "123.00"},
		{"123.0000987654", "4", PrecModeSignifDigits, false, false, "123"},
		{"123.0000987654", "4", PrecModeSignifDigits, false, true, "123.0"},
		{"123.0000987654", "3", PrecModeSignifDigits, false, true, "123"},
		{"123.0000987654", "2", PrecModeSignifDigits, false, false, "120"},
		{"123.0000987654", "1", PrecModeSignifDigits, false, false, "100"},
		{"123.0000987654", "1", PrecModeSignifDigits, false, true, "100"},
		{"1234", "5", PrecModeSignifDigits, false, false, "1234"},
		{"1234", "5", PrecModeSignifDigits, false, true, "1234.0"},
		{"1234", "4", PrecModeSignifDigits, false, false, "1234"},
		{"1234", "4", PrecModeSignifDigits, false, true, "1234"},
		{"1234.69", "0", PrecModeSignifDigits, false, false, "0"},
		{"1234.69", "0", PrecModeSignifDigits, false, true, "0"},
		{"12.3456000", "100", PrecModeDecimalPlace, true, false, "12.3456"},
		{"12.3456", "100", PrecModeDecimalPlace, true, false, "12.3456"},
		{"12.3456", "4", PrecModeDecimalPlace, true, false, "12.3456"},
		{"12.3456", "3", PrecModeDecimalPlace, true, false, "12.346"},
		{"12.3456", "2", PrecModeDecimalPlace, true, false, "12.35"},
		{"12.3456", "1", PrecModeDecimalPlace, true, false, "12.3"},
		{"12.3456", "0", PrecModeDecimalPlace, true, false, "12"},
		{"10000", "6", PrecModeDecimalPlace, true, false, "10000"},
		{"0.00003186", "8", PrecModeDecimalPlace, true, false, "0.00003186"},
		{"12.3456", "-1", PrecModeDecimalPlace, true, false, "10"},
		{"123.456", "-1", PrecModeDecimalPlace, true, false, "120"},
		{"123.456", "-2", PrecModeDecimalPlace, true, false, "100"},
		{"9.99999", "-1", PrecModeDecimalPlace, true, false, "10"},
		{"99.9999", "-1", PrecModeDecimalPlace, true, false, "100"},
		{"99.9999", "-2", PrecModeDecimalPlace, true, false, "100"},
		{"9.999", "3", PrecModeDecimalPlace, true, false, "9.999"},
		{"9.999", "2", PrecModeDecimalPlace, true, false, "10"},
		{"9.999", "2", PrecModeDecimalPlace, true, true, "10.00"},
		{"99.999", "2", PrecModeDecimalPlace, true, true, "100.00"},
		{"-99.999", "2", PrecModeDecimalPlace, true, true, "-100.00"},
		{"0.000123456700", "100", PrecModeSignifDigits, true, false, "0.0001234567"},
		{"0.0001234567", "100", PrecModeSignifDigits, true, false, "0.0001234567"},
		{"0.0001234567", "7", PrecModeSignifDigits, true, false, "0.0001234567"},
		{"0.000123456", "6", PrecModeSignifDigits, true, false, "0.000123456"},
		{"0.000123456", "5", PrecModeSignifDigits, true, false, "0.00012346"},
		{"0.000123456", "4", PrecModeSignifDigits, true, false, "0.0001235"},
		{"0.00012", "2", PrecModeSignifDigits, true, false, "0.00012"},
		{"0.0001", "1", PrecModeSignifDigits, true, false, "0.0001"},
		{"123.0000987654", "7", PrecModeSignifDigits, true, false, "123.0001"},
		{"123.0000987654", "6", PrecModeSignifDigits, true, false, "123"},
		{"0.00098765", "2", PrecModeSignifDigits, true, false, "0.00099"},
		{"0.00098765", "2", PrecModeSignifDigits, true, true, "0.00099"},
		{"0.00098765", "1", PrecModeSignifDigits, true, false, "0.001"},
		{"0.00098765", "10", PrecModeSignifDigits, true, true, "0.0009876500000"},
		{"0.098765", "1", PrecModeSignifDigits, true, true, "0.1"},
		{"0", "0", PrecModeSignifDigits, true, false, "0"},
		{"-0.123", "0", PrecModeSignifDigits, true, false, "0"},
		{"0.00000044", "5", PrecModeSignifDigits, true, false, "0.00000044"},
		{"0.000123456700", "0.00012", PrecModeTickSize, true, false, "0.00012"},
		{"0.0001234567", "0.00013", PrecModeTickSize, true, false, "0.00013"},
		{"0.0001234567", "0.00013", PrecModeTickSize, false, false, "0"},
		{"101.000123456700", "100", PrecModeTickSize, true, false, "100"},
		{"0.000123456700", "100", PrecModeTickSize, true, false, "0"},
		{"165", "110", PrecModeTickSize, false, false, "110"},
		{"3210", "1110", PrecModeTickSize, false, false, "2220"},
		{"165", "110", PrecModeTickSize, true, false, "220"},
		{"0.000123456789", "0.00000012", PrecModeTickSize, true, false, "0.00012348"},
		{"0.000123456789", "0.00000012", PrecModeTickSize, false, false, "0.00012336"},
		{"0.000273398", "1e-7", PrecModeTickSize, true, false, "0.0002734"},
		{"0.00005714", "0.00000001", PrecModeTickSize, false, false, "0.00005714"},
		{"0.01", "0.0001", PrecModeTickSize, true, true, "0.0100"},
		{"0.01", "0.0001", PrecModeTickSize, false, true, "0.0100"},
		{"-0.000123456789", "0.00000012", PrecModeTickSize, true, false, "-0.00012348"},
		{"-0.000123456789", "0.00000012", PrecModeTickSize, false, false, "-0.00012336"},
		{"-165", "110", PrecModeTickSize, false, false, "-110"},
		{"-165", "110", PrecModeTickSize, true, false, "-220"},
		{"-1650", "1100", PrecModeTickSize, false, false, "-1100"},
		{"-1650", "1100", PrecModeTickSize, true, false, "-2200"},
		{"0.0006", "0.0001", PrecModeTickSize, false, false, "0.0006"},
		{"-0.0006", "0.0001", PrecModeTickSize, false, false, "-0.0006"},
		{"0.6", "0.2", PrecModeTickSize, false, false, "0.6"},
		{"-0.6", "0.2", PrecModeTickSize, false, false, "-0.6"},
		{"1.2", "0.4", PrecModeTickSize, true, false, "1.2"},
		{"-1.2", "0.4", PrecModeTickSize, true, false, "-1.2"},
		{"1.2", "0.02", PrecModeTickSize, true, false, "1.2"},
		{"-1.2", "0.02", PrecModeTickSize, true, false, "-1.2"},
		{"44", "4.4", PrecModeTickSize, true, false, "44"},
		{"-44", "4.4", PrecModeTickSize, true, false, "-44"},
		{"44.00000001", "4.4", PrecModeTickSize, true, false, "44"},
		{"-44.00000001", "4.4", PrecModeTickSize, true, false, "-44"},
		{"20", "0.00000001", PrecModeTickSize, false, false, "20"},
		{"-0.123456", "5", PrecModeDecimalPlace, false, false, "-0.12345"},
		{"-0.123456", "5", PrecModeDecimalPlace, true, false, "-0.12346"},
		{"123", "0", PrecModeDecimalPlace, false, false, "123"},
		{"123", "5", PrecModeDecimalPlace, false, false, "123"},
		{"123", "5", PrecModeDecimalPlace, false, true, "123.00000"},
		{"123.", "0", PrecModeDecimalPlace, false, false, "123"},
		{"123.", "5", PrecModeDecimalPlace, false, true, "123.00000"},
		{"0.", "0", PrecModeDecimalPlace, false, false, "0"},
		{"0.", "5", PrecModeDecimalPlace, false, true, "0.00000"},
		{"1.44", "1", PrecModeDecimalPlace, true, false, "1.4"},
		{"1.45", "1", PrecModeDecimalPlace, true, false, "1.5"},
		{"1.45", "0", PrecModeDecimalPlace, true, false, "1"},
		{"5", "-1", PrecModeDecimalPlace, true, false, "10"},
		{"4.999", "-1", PrecModeDecimalPlace, true, false, "0"},
		{"0.0431531423", "-1", PrecModeDecimalPlace, true, false, "0"},
		{"-69.3", "-1", PrecModeDecimalPlace, true, false, "-70"},
		{"5001", "-4", PrecModeDecimalPlace, true, false, "10000"},
		{"4999.999", "-4", PrecModeDecimalPlace, true, false, "0"},
		{"69.3", "-2", PrecModeDecimalPlace, false, false, "0"},
		{"-69.3", "-2", PrecModeDecimalPlace, false, false, "0"},
		{"69.3", "-1", PrecModeSignifDigits, false, false, "60"},
		{"-69.3", "-1", PrecModeSignifDigits, false, false, "-60"},
		{"69.3", "-2", PrecModeSignifDigits, false, false, "0"},
		{"1602000000000000000000", "3", PrecModeSignifDigits, false, false, "1600000000000000000000"},
		{"-0.000123456789", "0.00000012", PrecModeTickSize, true, false, "-0.00012348"},
		{"-0.000123456789", "0.00000012", PrecModeTickSize, false, false, "-0.00012336"},
		{"-165", "110", PrecModeTickSize, false, false, "-110"},
		{"-165", "110", PrecModeTickSize, true, false, "-220"},
	}
	for _, it := range items {
		outText, err := DecToPrec(it.text, it.countMode, it.precision, it.isRound, it.padZero)
		if err != nil {
			panic(err)
		}
		if outText != it.output {
			t.Errorf("Fail %s %v %v %v %v out: %s exp: %s", it.text, it.precision, it.countMode, it.isRound, it.padZero, outText, it.output)
		} else {
			//t.Logf("Pass %s %v %v %v %v out: %s exp: %s", it.text, it.precision, it.countMode, it.isRound, it.padZero, outText, it.output)
		}
	}
}

func TestAdjust(t *testing.T) {
	cases := []struct {
		input  string
		output int32
	}{
		{"0.01", -2},
		{"0.0123", -2},
		{"0.1", -1},
		{"0.003", -3},
		{"3.125", 0},
		{"30.125", 1},
		{"39.125", 1},
		{"3912348.125", 6},
		{"-3912348.125", 6},
		{"-0.0123", -2},
		{"-3.125", 0},
		{"-30.125", 1},
	}
	for _, c := range cases {
		dec, _ := decimal.NewFromString(c.input)
		output := adjusted(dec)
		if output != c.output {
			t.Errorf("FAIL %s, out: %d exp: %d", c.input, output, c.output)
		} else {
			t.Logf("Pass %s, out: %d exp: %d", c.input, output, c.output)
		}
	}
}
