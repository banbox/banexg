package utils

import (
	"fmt"
	"github.com/banbox/banexg/errs"
	"sort"
	"strings"
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

const (
	StrStr   = 0
	StrInt   = 1
	StrFloat = 2
)

type StrType struct {
	Val  string
	Type int
}

func SplitParts(text string) []*StrType {
	res := make([]*StrType, 0)
	var b strings.Builder
	var state = StrStr // 初始化状态为字符串类型

	addToResult := func() {
		res = append(res, &StrType{Val: b.String(), Type: state})
		b.Reset() // 重置Builder以便累积下一个部分
	}

	for _, c := range text {
		switch {
		case c >= '0' && c <= '9': // 数字
			if state == StrStr { // 类型转换
				if b.Len() > 0 {
					addToResult()
				}
				state = StrInt
			}
			b.WriteRune(c)
		case c == '.': // 小数点，仅在字符串或整数后有效，转换为浮点数类型
			if state == StrInt {
				state = StrFloat
			} else if state == StrFloat {
				if b.Len() > 0 {
					addToResult()
				}
				state = StrStr
			}
			b.WriteRune(c)
		default: // 非数字字符，累积的数字（如有）加入结果，然后处理当前字符
			if state != StrStr {
				if b.Len() > 0 {
					addToResult()
				}
				state = StrStr
			}
			b.WriteRune(c)
		}
	}

	if b.Len() > 0 {
		addToResult()
	}

	return res
}

func ParseTimeRanges(items []string, loc *time.Location) ([][2]int64, *errs.Error) {
	result := make([][2]int64, 0, len(items))
	for _, dt := range items {
		arr := strings.Split(dt, "-")
		t1, err_ := time.ParseInLocation("15:04", arr[0], loc)
		if err_ != nil {
			return nil, errs.NewMsg(errs.CodeParamInvalid, "time format must be 15:04, current:%s", arr[0])
		}
		t2, err_ := time.ParseInLocation("15:04", arr[1], loc)
		if err_ != nil {
			return nil, errs.NewMsg(errs.CodeParamInvalid, "time format must be 15:04, current:%s", arr[1])
		}
		msecs1 := int64(t1.Hour()*60+t1.Minute()) * 60000
		msecs2 := int64(t2.Hour()*60+t2.Minute()) * 60000
		if msecs1 > msecs2 {
			// 结束时间是次日
			msecs2 += 24 * 60 * 60000
		}
		result = append(result, [2]int64{msecs1, msecs2})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i][0] < result[j][0]
	})
	return result, nil
}
