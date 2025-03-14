package bntp

import (
	"github.com/banbox/banexg/log"
	"go.uber.org/zap"
	"testing"
	"time"
)

// 使用示例
func TestTimeSync(t *testing.T) {
	// 初始化时间同步器，使用中国区域设置
	SetTimeSync(
		WithCountryCode("zh-CN"),
		WithRandomRate(0.1),
	)
	// 获取校正后的时间戳
	offset := GetTimeOffset()
	trueStr := Now().Format(time.RFC3339)
	curStr := time.Now().Format(time.RFC3339)
	log.Info("ntp sync res", zap.String("true", trueStr), zap.String("cur", curStr),
		zap.Int64("offset", offset))
}
