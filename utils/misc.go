package utils

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strconv"
	"strings"

	"github.com/banbox/banexg/log"
	"go.uber.org/zap"
)

func UUID(length int) string {
	randomBits := rand.Uint64()
	text := fmt.Sprintf("%x", randomBits) // 将randomBits转化为十六进制
	if len(text) > length {
		text = text[:length]
	}
	return text
}

func ArrSum(s []float64) float64 {
	var res float64
	for _, a := range s {
		res += a
	}
	return res
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

/*
GetMapVal
从map中获取指定类型的值。只支持简单类型，不支持slice,map,array,struct等
*/
func GetMapVal[T any](items map[string]interface{}, key string, defVal T) T {
	if val, ok := items[key]; ok {
		if tVal, ok := val.(T); ok {
			return tVal
		} else {
			var zero T
			reqType := reflect.TypeOf(zero).String()
			curType := reflect.TypeOf(val).String()
			panic(fmt.Sprintf("option %s should be %s, but is %s", key, reqType, curType))
		}
	}
	return defVal
}

/*
PopMapVal
从map中获取指定类型的值并删除。只支持简单类型，不支持slice,map,array,struct等
*/
func PopMapVal[T any](items map[string]interface{}, key string, defVal T) T {
	if val, ok := items[key]; ok {
		delete(items, key)
		if tVal, ok := val.(T); ok {
			return tVal
		} else {
			var zero T
			typ := reflect.TypeOf(zero)
			panic(fmt.Sprintf("option %s should be %s", key, typ.String()))
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
			err = UnmarshalString(text, &result, JsonNumDefault)
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
			data, _ := MarshalString(v)
			result[key] = data
		}
	}

	return result
}

func KeysOfMap[M ~map[K]V, K comparable, V any](m M) []K {
	r := make([]K, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	return r
}

func ValsOfMap[M ~map[K]V, K comparable, V any](m M) []V {
	r := make([]V, 0, len(m))
	for _, v := range m {
		r = append(r, v)
	}
	return r
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
func ByteToStruct[T any](byteChan <-chan []byte, outChan chan<- T, numType int) {
	defer close(outChan)

	for b := range byteChan {
		// 初始化目标类型的值
		var val T
		// 解析数据
		err := Unmarshal(b, &val, numType)
		if err != nil {
			log.Error("Error unmarshalling chan", zap.Error(err))
			continue // or handle the error as necessary
		}
		outChan <- val
	}
}

const (
	JsonNumDefault = 0 // equal to JsonNumFloat
	JsonNumFloat   = 0 // parse number in json to float64
	JsonNumStr     = 1 // keep number in json as json.Number type
	JsonNumAuto    = 2 // auto parse json.Number to int64/float64 in []interface{} map[string]interface{}
)

/*
UnmarshalString decode json (big int as float64)

numType: JsonNumDefault(JsonNumFloat), JsonNumStr, JsonNumAuto
*/
func UnmarshalString(text string, out interface{}, numType int) error {
	return Unmarshal([]byte(text), out, numType)
}

/*
Unmarshal decode json

numType: JsonNumDefault(JsonNumFloat), JsonNumStr, JsonNumAuto
*/
func Unmarshal(data []byte, out interface{}, numType int) error {
	if numType == JsonNumDefault {
		return json.Unmarshal(data, out)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	err := dec.Decode(out)
	if err != nil {
		return err
	}
	if numType != JsonNumAuto {
		return nil
	}
	_, err = parseJsonNumber(out)
	return err
}

func Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func MarshalString(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

/*
ParseJsonNumber

parse json.Number to int64/float64 in *[]interface{} *map[string]interface{}
*/
func ParseJsonNumber(data interface{}) error {
	_, err := parseJsonNumber(data)
	return err
}

func parseJsonNumber(data interface{}) (interface{}, error) {
	switch value := data.(type) {
	case *map[string]interface{}:
		m := *value
		for k, v := range m {
			val, err := parseJsonNumber(v)
			if err != nil {
				return nil, err
			}
			m[k] = val
		}
		return value, nil
	case *[]interface{}:
		arr := *value
		for i, v := range arr {
			val, err := parseJsonNumber(v)
			if err != nil {
				return nil, err
			}
			arr[i] = val
		}
		return value, nil
	case json.Number:
		if !strings.ContainsRune(string(value), '.') {
			if intValue, err := value.Int64(); err == nil {
				return intValue, nil
			}
		}
		if floatValue, err := value.Float64(); err == nil {
			return floatValue, nil
		} else {
			return nil, errors.New("invalid json.Number value")
		}
	default:
		return value, nil
	}
}

func MD5(data []byte) string {
	hash := md5.New()
	hash.Write(data)
	hashInBytes := hash.Sum(nil)

	return hex.EncodeToString(hashInBytes)
}

func GetSystemEnvProxy() string {
	proxyEnvVars := []string{
		"HTTPS_PROXY",
		"HTTP_PROXY",
	}

	for _, envVar := range proxyEnvVars {
		proxyStr := os.Getenv(envVar)
		if proxyStr != "" {
			return proxyStr
		}
	}

	return ""
}

// ProxyInfo 包含代理服务器的详细信息
type ProxyInfo struct {
	Host     string
	Port     string
	Protocol string // http, https, socks等
}

// GetSystemProxy 获取系统代理设置
func GetSystemProxy() (*ProxyInfo, error) {
	switch runtime.GOOS {
	case "windows":
		return getWindowsProxy()
	case "darwin":
		return getMacOSProxy()
	case "linux":
		return getLinuxProxy()
	default:
		log.Warn("GetSystemProxy unsupported", zap.String("os", runtime.GOOS))
		return nil, nil
	}
}

// getWindowsProxy 从 Windows 注册表获取代理设置
func getWindowsProxy() (*ProxyInfo, error) {
	cmd := exec.Command("reg", "query", "HKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	proxyEnable := false
	var proxyServer string

	for _, line := range strings.Split(string(output), "\n") {
		if strings.Contains(line, "ProxyEnable") {
			proxyEnable = strings.Contains(line, "0x1")
		}
		if strings.Contains(line, "ProxyServer") {
			parts := strings.Split(line, "REG_SZ")
			if len(parts) > 1 {
				proxyServer = strings.TrimSpace(parts[1])
			}
		}
	}

	if !proxyEnable || proxyServer == "" {
		return nil, nil
	}

	// 解析代理服务器字符串
	parts := strings.Split(proxyServer, ":")
	if len(parts) != 2 {
		return nil, errors.New("invalid proxy server format")
	}

	return &ProxyInfo{
		Host:     parts[0],
		Port:     parts[1],
		Protocol: "http", // Windows 注册表通常只返回 HTTP 代理
	}, nil
}

// getMacOSProxy 从 macOS 系统偏好设置获取代理
func getMacOSProxy() (*ProxyInfo, error) {
	cmd := exec.Command("osascript", "-e",
		`tell application "System Events"
			tell network preferences
				set httpProxy to HTTP proxy
				set httpProxyPort to HTTP proxy port
				if httpProxy is not "" then
					return httpProxy & ":" & httpProxyPort
				end if
			end tell
		end tell`)

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	proxyStr := strings.TrimSpace(string(output))
	if proxyStr == "" {
		return nil, nil
	}

	parts := strings.Split(proxyStr, ":")
	if len(parts) != 2 {
		return nil, errors.New("invalid proxy server format")
	}

	return &ProxyInfo{
		Host:     parts[0],
		Port:     parts[1],
		Protocol: "http",
	}, nil
}

// getLinuxProxy 从 Linux 系统获取代理设置
func getLinuxProxy() (*ProxyInfo, error) {
	// 尝试使用 gsettings
	cmd := exec.Command("gsettings", "get", "org.gnome.system.proxy", "mode")
	output, err := cmd.Output()
	if err == nil && strings.Contains(string(output), "manual") {
		// 如果是手动模式，获取 HTTP 代理
		cmd = exec.Command("gsettings", "get", "org.gnome.system.proxy.http", "host")
		host, err := cmd.Output()
		if err != nil {
			return nil, err
		}

		cmd = exec.Command("gsettings", "get", "org.gnome.system.proxy.http", "port")
		port, err := cmd.Output()
		if err != nil {
			return nil, err
		}

		hostStr := strings.Trim(strings.TrimSpace(string(host)), "'")
		portStr := strings.Trim(strings.TrimSpace(string(port)), "'")

		if hostStr != "" && portStr != "" {
			return &ProxyInfo{
				Host:     hostStr,
				Port:     portStr,
				Protocol: "http",
			}, nil
		}
	}

	// 如果 gsettings 失败，尝试使用 NetworkManager
	cmd = exec.Command("nmcli", "-t", "-f", "proxy.http.method,proxy.http.url", "connection", "show", "--active")
	output, err = cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			parts := strings.Split(line, ":")
			if len(parts) == 2 && parts[0] == "manual" {
				// 解析 URL
				proxyURL := strings.TrimSpace(parts[1])
				if proxyURL != "" {
					// 假设 URL 格式为 http://host:port
					urlParts := strings.Split(proxyURL, ":")
					if len(urlParts) == 3 {
						return &ProxyInfo{
							Host:     strings.TrimPrefix(urlParts[1], "//"),
							Port:     urlParts[2],
							Protocol: "http",
						}, nil
					}
				}
			}
		}
	}

	return nil, nil
}
