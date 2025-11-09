package search

import "testing"

func TestAscii32ChooseBest(t *testing.T) {
	t.Parallel()
	negInf := float32(-1e9)
	threshold := negInf / 2
	tests := []struct {
		p       float32
		pi      int32
		v       float32
		vi      int32
		want    float32
		wantIdx int32
	}{
		{10, 5, 9, 4, 10, 5},
		{3, 2, 4, 1, 4, 1},
		{threshold - 1, 3, threshold - 2, 1, negInf, -1},
		{5, -1, 2, 9, negInf, -1},
		{1, 7, 2, -1, negInf, -1},
	}
	for i, tc := range tests {
		gotScore, gotIdx := ascii32ChooseBest(tc.p, tc.pi, tc.v, tc.vi, threshold, negInf)
		if gotScore != tc.want || gotIdx != tc.wantIdx {
			t.Fatalf("case %d got (%f,%d) want (%f,%d)", i, gotScore, gotIdx, tc.want, tc.wantIdx)
		}
	}
}
