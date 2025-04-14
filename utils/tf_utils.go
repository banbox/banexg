package utils

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
)

type TFOrigin struct {
	TFSecs     int
	OffsetSecs int
	Origin     string
}

var (
	tfSecsMap = map[string]int{}
	secsTfMap = map[int]string{}
	tfLock    sync.Mutex
	tfOrigins = []*TFOrigin{{604800, 345600, "1970-01-05"}}
)

const (
	SecsMin  = 60
	SecsHour = SecsMin * 60
	SecsDay  = SecsHour * 24
	SecsWeek = SecsDay * 7
	SecsMon  = SecsDay * 30
	SecsQtr  = SecsMon * 3
	SecsYear = SecsDay * 365
)

func RegTfSecs(items map[string]int) {
	tfLock.Lock()
	for key, val := range items {
		tfSecsMap[key] = val
		secsTfMap[val] = key
	}
	tfLock.Unlock()
}

func parseTimeFrame(timeframe string) (int, error) {
	if len(timeframe) < 2 {
		return 0, errors.New("timeframe string too short")
	}

	amountStr := timeframe[:len(timeframe)-1]
	unit := timeframe[len(timeframe)-1]

	amount, err := strconv.Atoi(amountStr)
	if err != nil {
		return 0, err
	}

	var scale int
	switch unit {
	case 'y', 'Y':
		scale = SecsYear
	case 'q', 'Q':
		scale = SecsQtr
	case 'M':
		scale = SecsMon
	case 'w', 'W':
		scale = SecsWeek
	case 'd', 'D':
		scale = SecsDay
	case 'h', 'H':
		scale = SecsHour
	case 'm':
		scale = SecsMin
	case 's', 'S':
		scale = 1
	default:
		return 0, errors.New("timeframe unit " + string(unit) + " is not supported")
	}

	return amount * scale, nil
}

/*
TFToSecs
Convert the time cycle to seconds
Supporting units: s, m, h, d, M, Q, Y
将时间周期转为秒
支持单位：s, m, h, d, M, Q, Y
*/
func TFToSecs(timeFrame string) int {
	tfLock.Lock()
	secs, ok := tfSecsMap[timeFrame]
	var err error
	if !ok {
		secs, err = parseTimeFrame(timeFrame)
		if err == nil {
			tfSecsMap[timeFrame] = secs
			secsTfMap[secs] = timeFrame
		}
	}
	tfLock.Unlock()
	if err != nil {
		panic(err)
	}
	return secs
}

func GetTfAlignOrigin(secs int) (string, int) {
	for _, item := range tfOrigins {
		if secs < item.TFSecs {
			break
		}
		if secs%item.TFSecs == 0 {
			return item.Origin, item.OffsetSecs
		}
	}
	return "1970-01-01", 0
}

/*
AlignTfSecsOffset
Convert the given 10 second timestamp to the header start timestamp for the specified time period, using the specified offset
将给定的10位秒级时间戳，转为指定时间周期下，的头部开始时间戳，使用指定偏移
*/
func AlignTfSecsOffset(timeSecs int64, tfSecs int, offset int) int64 {
	if timeSecs > 1000000000000 {
		panic("10 digit timestamp is require for AlignTfSecs")
	}
	tfSecs64 := int64(tfSecs)
	if offset == 0 {
		return timeSecs / tfSecs64 * tfSecs64
	}
	offset64 := int64(offset)
	return (timeSecs-offset64)/tfSecs64*tfSecs64 + offset64
}

/*
AlignTfSecs
Convert the given 10 second timestamp to the header start timestamp for the specified time period
将给定的10位秒级时间戳，转为指定时间周期下，的头部开始时间戳
*/
func AlignTfSecs(timeSecs int64, tfSecs int) int64 {
	_, offset := GetTfAlignOrigin(tfSecs)
	return AlignTfSecsOffset(timeSecs, tfSecs, offset)
}

/*
AlignTfMSecs
Convert the given 13 millisecond timestamp to the header start timestamp for the specified time period
将给定的13位毫秒级时间戳，转为指定时间周期下，的头部开始时间戳
*/
func AlignTfMSecs(timeMSecs int64, tfMSecs int64) int64 {
	if timeMSecs < 100000000000 {
		panic(fmt.Sprintf("12 digit is required for AlignTfMSecs, : %v", timeMSecs))
	}
	if tfMSecs < 1000 {
		panic("milliseconds tfMSecs is require for AlignTfMSecs")
	}
	return AlignTfSecs(timeMSecs/1000, int(tfMSecs/1000)) * 1000
}

func AlignTfMSecsOffset(timeMSecs, tfMSecs, offset int64) int64 {
	if timeMSecs < 100000000000 {
		panic(fmt.Sprintf("12 digit is required for AlignTfMSecsOffset, : %v", timeMSecs))
	}
	if tfMSecs < 1000 {
		panic("milliseconds tfMSecs is require for AlignTfMSecs")
	}
	return AlignTfSecsOffset(timeMSecs/1000, int(tfMSecs/1000), int(offset/1000)) * 1000
}

/*
SecsToTF
Convert the seconds of a time period into a time period
将时间周期的秒数，转为时间周期
*/
func SecsToTF(tfSecs int) string {
	tfLock.Lock()
	timeFrame, ok := secsTfMap[tfSecs]
	invalid := false
	if !ok {
		switch {
		case tfSecs >= SecsYear:
			timeFrame = strconv.Itoa(tfSecs/SecsYear) + "y"
		case tfSecs >= SecsQtr:
			timeFrame = strconv.Itoa(tfSecs/SecsQtr) + "q"
		case tfSecs >= SecsMon:
			timeFrame = strconv.Itoa(tfSecs/SecsMon) + "M"
		case tfSecs >= SecsWeek:
			timeFrame = strconv.Itoa(tfSecs/SecsWeek) + "w"
		case tfSecs >= SecsDay:
			timeFrame = strconv.Itoa(tfSecs/SecsDay) + "d"
		case tfSecs >= SecsHour:
			timeFrame = strconv.Itoa(tfSecs/SecsHour) + "h"
		case tfSecs >= SecsMin:
			timeFrame = strconv.Itoa(tfSecs/SecsMin) + "m"
		case tfSecs >= 1:
			timeFrame = strconv.Itoa(tfSecs) + "s"
		default:
			invalid = true
		}
		secsTfMap[tfSecs] = timeFrame
	}
	tfLock.Unlock()
	if invalid {
		panic("unsupport tfSecs:" + strconv.Itoa(tfSecs))
	}
	return timeFrame
}
