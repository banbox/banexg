package utils

import (
	"github.com/banbox/banexg/errs"
	"regexp"
	"strconv"
)

func PrecisionFromString(input string) float64 {
	//support string formats like '1e-4'
	matched, _ := regexp.MatchString("e", input)
	if matched {
		re := regexp.MustCompile(`\de`)
		numStr := re.ReplaceAllString(input, "")
		num, _ := strconv.ParseFloat(numStr, 64)
		return num * -1
	}
	//support integer formats (without dot) like '1', '10' etc [Note: bug in decimalToPrecision, so this should not be used atm]
	// if not ('.' in str):
	//     return len(str) * -1
	//default strings like '0.0001'
	parts := regexp.MustCompile(`0+$`).Split(input, -1)
	if matched, _ := regexp.MatchString(`\.`, input); matched {
		innerParts := regexp.MustCompile(`\.`).Split(parts[0], -1)
		if len(innerParts) > 1 {
			return float64(len(innerParts[1]))
		}
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
