package banexg

import (
	"fmt"
	"testing"
)

func TestSliceInsert(t *testing.T) {
	a := []int{1, 2, 3, 6, 8}
	index := 5
	val := 23
	a = append(a, 0)
	copy(a[index+1:], a[index:])
	a[index] = val
	fmt.Printf("%v", a)
}

func TestOdBookSide(t *testing.T) {
	bids := NewOdBookSide(true, 100, [][2]float64{
		{120, 10},
		{119, 15},
		{117, 20},
		{115, 40},
		{110, 80},
	})
	asks := NewOdBookSide(false, 100, [][2]float64{
		{122, 10},
		{123, 15},
		{125, 20},
		{127, 40},
		{130, 80},
	})
	bids.Set(119, 16)
	bids.Set(118, 17)
	asks.Set(127, 43)
	asks.Set(135, 100)
	asks.Set(121.8, 5)
	avgBidPrice, lastBidPrice := bids.AvgPrice(43)
	volSum, fillRate := asks.SumVolTo(127)
	volSum2, fillRate2 := bids.SumVolTo(116)
	if avgBidPrice != 118.83720930232558 || lastBidPrice != 118 {
		t.Errorf("AvgPrice fail")
	}
	if volSum != 50 || fillRate != 1 || volSum2 != 63 || fillRate2 != 1 {
		t.Errorf("SumVolTo fail")
	}
}
