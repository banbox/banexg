package utils

import (
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

func DeepCopy(src, dst map[string]interface{}) {
	for k, v := range src {
		if v, ok := v.(map[string]interface{}); ok {
			if bv, ok := dst[k]; ok {
				if bv, ok := bv.(map[string]interface{}); ok {
					DeepCopy(v, bv)
					continue
				}
			}
		}
		dst[k] = v
	}
}

/*
UrlEncodeMap
将map编码为url查询字符串
escape: 是否对键和值进行编码
*/
func UrlEncodeMap(params map[string]interface{}, escape bool) string {
	var parts []string
	for k, v := range params {
		// 将值转换为字符串
		// 注意：这里的实现可能需要根据具体的情况调整，例如，如何处理非字符串的值等
		valueStr := fmt.Sprintf("%v", v)
		// 对键和值进行URL编码
		if escape {
			parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(valueStr))
		} else {
			parts = append(parts, k+"="+valueStr)
		}
	}
	// 使用'&'拼接所有的键值对
	return strings.Join(parts, "&")
}

func EncodeURIComponent(str string, safe string) string {
	escapeStr := url.QueryEscape(str)
	for _, char := range safe {
		escapeStr = strings.ReplaceAll(escapeStr, url.QueryEscape(string(char)), string(char))
	}
	return escapeStr
}

func GetMapFloat(data map[string]interface{}, key string) float64 {
	if rawVal, ok := data[key]; ok {
		if rawVal == nil {
			return 0.0
		}
		val, err := strconv.ParseFloat(rawVal.(string), 64)
		if err != nil {
			return 0.0
		}
		return val
	}
	return 0.0
}

func GetMapVal[T any](items map[string]interface{}, key string, defVal T) T {
	if val, ok := items[key]; ok {
		if tVal, ok := val.(T); ok {
			return tVal
		} else {
			var zero T
			typ := reflect.TypeOf(zero)
			panic(fmt.Sprintf("option %s should be %s", key, typ.Name()))
		}
	}
	return defVal
}

func SetFieldBy[T any](field *T, items map[string]interface{}, key string, defVal T) {
	if field == nil {
		panic(fmt.Sprintf("field can not be nil for key: %s", key))
	}
	val := GetMapVal(items, key, defVal)
	if !IsNil(val) {
		*field = val
	}
}

func OmitMapKeys(items map[string]interface{}, keys ...string) {
	for _, k := range keys {
		if _, ok := items[k]; ok {
			delete(items, k)
		}
	}
}

/*
IsNil 判断是否为nil

	golang中类型和值是分开存储的，如果一个指针有类型，值为nil，直接判断==nil会返回false
*/
func IsNil(i interface{}) bool {
	if i == nil {
		return true
	}
	switch reflect.TypeOf(i).Kind() {
	case reflect.Ptr, reflect.Map, reflect.Array, reflect.Chan, reflect.Slice:
		return reflect.ValueOf(i).IsNil()
	default:
		return false
	}
}
