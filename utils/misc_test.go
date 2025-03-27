package utils

import (
	"fmt"
	"github.com/banbox/banexg/log"
	"go.uber.org/zap"
	"math"
	"testing"
)

func TestSonicUnmarshal(t *testing.T) {
	// sonic默认反序列化json中的number时，使用float64，对于一些大的int64的值，会导致精度损失，这里是测试哪些类型会有精度损失
	// 使用utils.UnmarshalString替换后，大整数都能正常解析了
	runSonicItem("MaxInt64", math.MaxInt64)
	runSonicItem("MinInt64", math.MinInt64)
	runSonicItem("MaxInt32", math.MaxInt32)
	runSonicItem("MinInt32", math.MinInt32)
	runSonicItem("MaxFloat64", math.MaxFloat64)
	runSonicItem("MaxFloat32", math.MaxFloat32)
}

func runSonicItem[T comparable](name string, val T) {
	text, err := MarshalString(val)
	if err != nil {
		panic(fmt.Sprintf("marshal %v fail: %v", name, err))
	}
	textWrap := fmt.Sprintf("{\"val\":%v}", text)
	var res = make(map[string]interface{})
	// err2 := UnmarshalString(textWrap, &res)
	err2 := UnmarshalString(textWrap, &res, JsonNumAuto)
	if err2 != nil {
		log.Error("unmarshal fail", zap.String("name", name), zap.Error(err2))
	}
	input := fmt.Sprintf("%v", val)
	output := fmt.Sprintf("%v", res["val"])
	if input != output {
		log.Error("unmarshal wrong", zap.String("name", name), zap.String("input", input),
			zap.String("text", text), zap.String("output", output))
	}
}

func TestGetSystemProxy(t *testing.T) {
	prx, err := GetSystemProxy()
	if err != nil {
		panic(err)
	}
	systemProxy := fmt.Sprintf("%s://%s:%s", prx.Protocol, prx.Host, prx.Port)

	envProxy := GetSystemEnvProxy()
	fmt.Printf("envProxy: %s, systemProxy: %s\n", envProxy, systemProxy)
}
