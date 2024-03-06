package utils

import (
	"github.com/banbox/banexg/errs"
	"math"
	"strconv"
	"strings"
)

/*
PrecisionFromString
1e-4 -> 4
0.000001 -> 6
100 -> 0
*/
func PrecisionFromString(input string) float64 {
	input = strings.TrimSpace(input)
	if input == "" {
		return 0
	}
	val, err := strconv.ParseFloat(input, 64)
	if err != nil {
		return 0
	}
	prec := math.Round(math.Log10(val))
	if prec < 0 {
		return prec * -1
	}
	return 0
}

func SafeParams(params map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	if params != nil {
		for k, v := range params {
			result[k] = v
		}
	}
	return result
}

func ParseTimeFrame(timeframe string) (int, *errs.Error) {
	if val, ok := tfSecsCache[timeframe]; ok {
		return val, nil
	}
	length := len(timeframe)
	if length <= 1 {
		return 0, errs.InvalidTimeFrame
	}

	amount, err := strconv.Atoi(timeframe[:length-1])
	if err != nil {
		return 0, errs.InvalidTimeFrame
	}

	unit := timeframe[length-1]
	var scale int

	switch unit {
	case 'y', 'Y':
		scale = 60 * 60 * 24 * 365
	case 'M':
		scale = 60 * 60 * 24 * 30
	case 'w', 'W':
		scale = 60 * 60 * 24 * 7
	case 'd', 'D':
		scale = 60 * 60 * 24
	case 'h', 'H':
		scale = 60 * 60
	case 'm':
		scale = 60
	case 's', 'S':
		scale = 1
	default:
		return 0, errs.InvalidTimeFrame
	}

	res := amount * scale
	tfSecsCache[timeframe] = res

	return res, nil
}
