package utils

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

const thresFloat64Eq = 1e-9

/*
EqualNearly 判断两个float是否近似相等，解决浮点精读导致不等
*/
func EqualNearly(a, b float64) bool {
	return EqualIn(a, b, thresFloat64Eq)
}

/*
EqualIn 判断两个float是否在一定范围内近似相等
*/
func EqualIn(a, b, thres float64) bool {
	if math.IsNaN(a) && math.IsNaN(b) {
		return true
	}
	return math.Abs(a-b) <= thres
}

// ParseNum converts common numeric types or numeric strings to float64.
func ParseNum(val interface{}) (float64, error) {
	if parsed, ok, err := parseStringOrBytes(val, parseFloatString); ok {
		return parsed, err
	}
	switch v := val.(type) {
	case nil:
		return 0, nil
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint16:
		return float64(v), nil
	case uint8:
		return float64(v), nil
	case json.Number:
		return v.Float64()
	default:
		return 0, fmt.Errorf("unsupported num type: %T", val)
	}
}

// ParseInt64 converts common numeric types or numeric strings to int64.
func ParseInt64(val interface{}) (int64, error) {
	if parsed, ok, err := parseStringOrBytes(val, parseInt64String); ok {
		return parsed, err
	}
	switch v := val.(type) {
	case nil:
		return 0, nil
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int16:
		return int64(v), nil
	case int8:
		return int64(v), nil
	case uint:
		return int64(v), nil
	case uint64:
		return int64(v), nil
	case uint32:
		return int64(v), nil
	case uint16:
		return int64(v), nil
	case uint8:
		return int64(v), nil
	case float64:
		return int64(v), nil
	case float32:
		return int64(v), nil
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i, nil
		}
		f, err := v.Float64()
		if err != nil {
			return 0, err
		}
		return int64(f), nil
	default:
		return 0, fmt.Errorf("unsupported int type: %T", val)
	}
}

func parseStringOrBytes[T any](val interface{}, parse func(string) (T, error)) (T, bool, error) {
	var zero T
	switch v := val.(type) {
	case string:
		parsed, err := parse(v)
		return parsed, true, err
	case []byte:
		parsed, err := parse(string(v))
		return parsed, true, err
	default:
		return zero, false, nil
	}
}

func parseFloatString(val string) (float64, error) {
	if val == "" {
		return 0, nil
	}
	return strconv.ParseFloat(val, 64)
}

func parseInt64String(val string) (int64, error) {
	if val == "" {
		return 0, nil
	}
	if i, err := strconv.ParseInt(val, 10, 64); err == nil {
		return i, nil
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0, err
	}
	return int64(f), nil
}

// ParseBookSide converts order book levels to price/size pairs using parse.
func ParseBookSide(levels [][]string, parse func(string) float64) [][2]float64 {
	if len(levels) == 0 || parse == nil {
		return nil
	}
	res := make([][2]float64, 0, len(levels))
	for _, lvl := range levels {
		if len(lvl) < 2 {
			continue
		}
		res = append(res, [2]float64{parse(lvl[0]), parse(lvl[1])})
	}
	return res
}
