package utils

import (
	"regexp"
	"strconv"
)

func PrecisionFromString(input string) int {
	//support string formats like '1e-4'
	matched, _ := regexp.MatchString("e", input)
	if matched {
		re := regexp.MustCompile(`\de`)
		numStr := re.ReplaceAllString(input, "")
		num, _ := strconv.Atoi(numStr)
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
			return len(innerParts[1])
		}
	}
	return 0
}

func SafeParams(params *map[string]interface{}) map[string]interface{} {
	if params == nil {
		return map[string]interface{}{}
	}
	return *params
}

func ParseTimeFrame(timeframe string) (int, error) {
	if val, ok := tfSecsCache[timeframe]; ok {
		return val, nil
	}
	length := len(timeframe)
	if length <= 1 {
		return 0, ErrInvalidTimeFrame
	}

	amount, err := strconv.Atoi(timeframe[:length-1])
	if err != nil {
		return 0, ErrInvalidTimeFrame
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
		return 0, ErrInvalidTimeFrame
	}

	res := amount * scale
	tfSecsCache[timeframe] = res

	return res, nil
}
