package utils

import "math"

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
