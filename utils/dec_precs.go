package utils

import (
	"errors"
	"fmt"
	"github.com/shopspring/decimal"
	"regexp"
	"strconv"
	"strings"
)

const (
	PrecModeDecimalPlace = 2
	PrecModeSignifDigits = 3
	PrecModeTickSize     = 4
)

var (
	regTrimEndZero = regexp.MustCompile(`0+$`)
)

var (
	ErrInvalidPrecision   = errors.New("invalid precision")
	ErrNegPrecForTickSize = errors.New("negative precision for tick size")
)

/*
DecToPrec
将数字转换为指定精度返回

	num: 需要处理的数字字符串
	countMode: PrecModeDecimalPlace 保留小数点后指定位；
	PrecModeSignifDigits 保留全部有效数字个数
	PrecModeTickSize 返回给定的数字的整数倍
	precision: 结合countMode使用，正整数，负整数，正浮点数
	isRound: 是否四舍五入，默认截断
	padZero: 是否尾部填充0到指定长度
*/
func DecToPrec(num string, countMode int, precision string, isRound, padZero bool) (string, error) {
	if precision == "" {
		return "", ErrInvalidPrecision
	}
	if countMode < PrecModeDecimalPlace || countMode > PrecModeTickSize {
		return "", fmt.Errorf("invalid count mode %d", countMode)
	}
	precVal, err := decimal.NewFromString(precision)
	if err != nil {
		return "", fmt.Errorf("invalid precision %s %v", precision, err)
	}
	numVal, err := decimal.NewFromString(num)
	if err != nil {
		return "", fmt.Errorf("invalid num %s %v", num, err)
	}
	zeroVal := decimal.NewFromInt(0)
	tenVal := decimal.NewFromInt(10)
	negVal := decimal.NewFromInt(-1)

	powerOfTen := func(d *decimal.Decimal) decimal.Decimal {
		return tenVal.Pow(d.Mul(negVal))
	}

	if precVal.LessThan(zeroVal) {
		if countMode == PrecModeTickSize {
			return "", ErrNegPrecForTickSize
		}
		nearest := powerOfTen(&precVal)
		if isRound {
			numDivVal := numVal.Div(nearest).String()
			midRes, err := DecToPrec(numDivVal, PrecModeDecimalPlace, "0", isRound, padZero)
			if err != nil {
				return "", err
			}
			midResVal, err := decimal.NewFromString(midRes)
			if err != nil {
				return "", err
			}
			return midResVal.Mul(nearest).String(), nil
		} else {
			numTruc := numVal.Sub(numVal.Mod(nearest)).String()
			return DecToPrec(numTruc, PrecModeDecimalPlace, "0", isRound, padZero)
		}
	}
	if countMode == PrecModeTickSize {
		missing := numVal.Abs().Mod(precVal)
		if !missing.Equal(zeroVal) {
			delta := missing
			if isRound {
				twoVal := decimal.NewFromInt(2)
				if missing.GreaterThanOrEqual(precVal.Div(twoVal)) {
					delta = delta.Sub(precVal)
				}
				if numVal.GreaterThan(zeroVal) {
					delta = delta.Mul(negVal)
				}
			} else {
				if numVal.GreaterThanOrEqual(zeroVal) {
					delta = delta.Mul(negVal)
				}
			}
			numVal = numVal.Add(delta)
		}
		parts := strings.Split(regTrimEndZero.ReplaceAllString(precVal.String(), ""), ".")
		newPrec := "0"
		if len(parts) > 1 {
			newPrec = strconv.Itoa(len(parts[1]))
		} else {
			match := regTrimEndZero.FindString(parts[0])
			if match != "" {
				newPrec = strconv.Itoa(-len(match))
			}
		}
		return DecToPrec(numVal.String(), PrecModeDecimalPlace, newPrec, true, padZero)
	}
	precise := zeroVal
	numExp := adjusted(numVal)
	if isRound {
		if countMode == PrecModeDecimalPlace {
			precise = numVal.Round(int32(precVal.IntPart()))
		} else if countMode == PrecModeSignifDigits {
			q := precVal.Sub(decimal.NewFromInt32(numExp + 1))
			sigFig := powerOfTen(&q)
			if q.LessThan(zeroVal) {
				numPrecText := numVal.String()[:precVal.IntPart()]
				if numPrecText == "" {
					numPrecText = "0"
				}
				numPrecVal, err := decimal.NewFromString(numPrecText)
				if err != nil {
					return "", fmt.Errorf("numPrecText fail %v %v", numPrecText, err)
				}
				below := sigFig.Mul(numPrecVal)
				above := below.Add(sigFig)
				if below.Sub(numVal).Abs().LessThan(above.Sub(numVal).Abs()) {
					precise = below
				} else {
					precise = above
				}
			} else {
				precise = numVal.Round(int32(q.IntPart()))
			}
		} else {
			return "", fmt.Errorf("invalid cound mode: %v", countMode)
		}
		numExp = adjusted(precise)
	} else {
		if countMode == PrecModeDecimalPlace {
			precise = numVal.Truncate(int32(precVal.IntPart()))
		} else if countMode == PrecModeSignifDigits {
			if !precVal.Equal(zeroVal) {
				margin := tenVal.Pow(decimal.NewFromInt32(numExp))
				dotVal := numVal.Div(margin).Truncate(int32(precVal.IntPart()) - 1)
				precise = dotVal.Mul(margin)
			}
		} else {
			return "", fmt.Errorf("invalid cound mode: %v", countMode)
		}
	}
	if !padZero {
		return precise.String(), nil
	}
	if countMode == PrecModeDecimalPlace {
		return precise.StringFixed(int32(precVal.IntPart())), nil
	}
	// PrecModeSignifDigits
	dotNum := int32(precVal.IntPart()) - numExp - 1
	if dotNum > 0 {
		return precise.StringFixed(dotNum), nil
	}
	return precise.String(), nil
}

// 获取与Python中Decimal.adjusted()类似的值
func adjusted(dec decimal.Decimal) int32 {
	// 计算有效数字（coefficient）
	coefficient := dec.Coefficient()
	coefficient.Abs(coefficient)
	// 获取指数值
	exponent := dec.Exponent()

	// 计算Decimal的字符串表示中小数点前的数字个数
	adjustedExponent := int32(len(coefficient.String())) - 1

	// 返回指数+调整后（小数点前的数字个数-1）
	return adjustedExponent + exponent
}

/*
PrecFloat64
对给定浮点数取近似值，精确到指定位数
*/
func PrecFloat64(num float64, prec float64, isRound bool) (float64, error) {
	resStr, err := PrecFloat64Str(num, prec, isRound)
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(resStr, 64)
}

func PrecFloat64Str(num float64, prec float64, isRound bool) (string, error) {
	numStr := strconv.FormatFloat(num, 'f', -1, 64)
	precStr := strconv.FormatFloat(prec, 'f', -1, 64)
	return DecToPrec(numStr, PrecModeDecimalPlace, precStr, isRound, false)
}
