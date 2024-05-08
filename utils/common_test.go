package utils

import (
	"testing"
)

func TestSplitParts(t *testing.T) {
	tests := []struct {
		args string
		want []*StrType
	}{
		{args: "CI1211C432.54", want: []*StrType{
			{Val: "CI", Type: StrStr},
			{Val: "1211", Type: StrInt},
			{Val: "C", Type: StrStr},
			{Val: "432.54", Type: StrFloat},
		}},
		{args: "125DP", want: []*StrType{
			{Val: "125", Type: StrInt},
			{Val: "DP", Type: StrStr},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.args, func(t *testing.T) {
			got := SplitParts(tt.args)
			if len(got) != len(tt.want) {
				t.Errorf("len wrong %v, want %v", len(got), len(tt.want))
				return
			}
			for i, v := range tt.want {
				g := got[i]
				if v.Val != g.Val || v.Type != g.Type {
					t.Errorf("%v val wrong %v %v, want %v %v", i, g.Val, g.Type, v.Val, v.Type)
					break
				}
			}
		})
	}
}
