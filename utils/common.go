package utils

import (
	"fmt"
	"time"
)

/*
YMD 将13位时间戳转为日期

separator 年月日的间隔字符
fullYear 年是否使用4个数字
*/
func YMD(timestamp int64, separator string, fullYear bool) string {
	// 将13位时间戳转换为Time对象
	t := time.Unix(0, timestamp*int64(time.Millisecond))

	yearFormat := "2006"
	if !fullYear {
		yearFormat = "06"
	}

	// 根据是否有分隔符和年份的格式来构建时间格式字符串
	layout := fmt.Sprintf("%s%s02%s01", yearFormat, separator, separator)

	// 返回格式化时间
	return t.Format(layout)
}

func ISO8601(millis int64) string {
	t := time.UnixMilli(millis)
	return t.Format(time.RFC3339)
}
