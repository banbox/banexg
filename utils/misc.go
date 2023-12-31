package utils

import (
	"fmt"
	"github.com/anyongjin/banexg/log"
	"github.com/bytedance/sonic"
	"go.uber.org/zap"
	"math/rand"
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

func UUID(length int) string {
	randomBits := rand.Uint64()
	text := fmt.Sprintf("%x", randomBits) // 将randomBits转化为十六进制
	if len(text) > length {
		text = text[:length]
	}
	return text
}

func ArrContains[T comparable](s []T, e T) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
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

func PopMapVal[T any](items map[string]interface{}, key string, defVal T) T {
	if val, ok := items[key]; ok {
		delete(items, key)
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

/*
SafeMapVal
从字典中读取给定键的值，并自动转换为需要的类型，如果出错则返回默认值
*/
func SafeMapVal[T any](items map[string]string, key string, defVal T) (result T, err error) {
	if text, ok := items[key]; ok {
		var err error
		valType := reflect.TypeOf(defVal)
		switch valType.Kind() {
		case reflect.Int:
			var val int
			val, err = strconv.Atoi(text)
			result = any(val).(T)
		case reflect.Int64:
			var val int64
			val, err = strconv.ParseInt(text, 10, 64)
			result = any(val).(T)
		case reflect.Float64:
			var val float64
			val, err = strconv.ParseFloat(text, 64)
			result = any(val).(T)
		case reflect.Bool:
			var val bool
			val, err = strconv.ParseBool(text)
			result = any(val).(T)
		case reflect.String:
			result = any(text).(T)
		default:
			err = sonic.UnmarshalString(text, &result)
		}
		if err != nil {
			return defVal, err
		}
		return result, nil
	}
	return defVal, nil
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

func MapValStr(input map[string]interface{}) map[string]string {
	result := make(map[string]string)

	for key, value := range input {
		switch v := value.(type) {
		case nil:
			result[key] = ""
		case bool:
			result[key] = fmt.Sprintf("%v", v)
		case int:
			result[key] = strconv.Itoa(v)
		case int64:
			result[key] = strconv.FormatInt(v, 10)
		case float32:
			result[key] = fmt.Sprintf("%f", v)
		case float64:
			result[key] = strconv.FormatFloat(v, 'f', -1, 64)
		case string:
			result[key] = v
		default:
			data, _ := sonic.MarshalString(v)
			result[key] = data
		}
	}

	return result
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

/*
ByteToStruct
将[]byte类型的chan通道，转为指定类型通道
*/
func ByteToStruct[T any](byteChan <-chan []byte, outChan chan<- T) {
	defer close(outChan)

	for b := range byteChan {
		// 初始化目标类型的值
		var val T
		// 解析数据
		err := sonic.Unmarshal(b, &val)
		if err != nil {
			log.Error("Error unmarshalling chan", zap.Error(err))
			continue // or handle the error as necessary
		}
		outChan <- val
	}
}
