package geom

import "testing"

func BenchmarkAnalyticLinePairFilter(b *testing.B) {
	for _, tc := range []struct {
		name string
		n    int
		span float64
	}{
		{name: "disjointLines192", n: 192, span: 10},
		{name: "overlappingLines64", n: 64, span: 0},
	} {
		curves := analyticLineBenchmarkScene(tc.n, tc.span)
		for _, filter := range []bool{false, true} {
			name := "unfiltered"
			if filter {
				name = "filtered"
			}
			b.Run(tc.name+"/"+name, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					regionsWithBudgetPairFilter(curves, nil, arrangeConfig{}, nil, filter)
				}
			})
		}
	}
}

func analyticLineBenchmarkScene(n int, spacing float64) []Curve {
	curves := make([]Curve, n)
	for i := range curves {
		x, y := 0.0, 0.0
		dx, dy := 1.0, 1.0
		if spacing > 0 {
			x = float64(i%16) * spacing
			y = float64(i/16) * spacing
			dy = 0.25 + float64(i%5)*0.05
		}
		curves[i] = NewLine(NewPoint(x, y), NewPoint(x+dx, y+dy))
	}
	return curves
}
