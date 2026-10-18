package sketch_test

import (
	"math"
	"math/big"
	"testing"

	"github.com/lestrrat-3d/sketch"
)

func fmaCapRectangle(x0, y0, x1, y1 float64) [][2]float64 {
	return [][2]float64{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
}

func fmaCapSeparate(a, b, c float64) float64 {
	// Round the exact product before adding, even when the compiler can fuse
	// a source expression into a single multiply-add instruction.
	product := new(big.Rat).Mul(new(big.Rat).SetFloat64(a),
		new(big.Rat).SetFloat64(b))
	roundedProduct, _ := product.Float64()
	sum := new(big.Rat).Add(new(big.Rat).SetFloat64(roundedProduct),
		new(big.Rat).SetFloat64(c))
	roundedSum, _ := sum.Float64()
	return roundedSum
}

func TestProfilesTwoLineCapsBoundedFMAProducers(t *testing.T) {
	eps := math.Ldexp(1, -52)
	nearEnd := eps / (1 + 2*eps)
	if math.FMA(3.0/5.0, 5, -3) == 0 ||
		math.FMA(nearEnd, 1+2*eps, -eps) == 0 ||
		fmaCapSeparate(nearEnd, 1+2*eps, -eps) != 0 {
		t.Fatal("fixtures do not distinguish fused and separate endpoint evaluation")
	}
	cases := []struct {
		name  string
		prism [4]float64
		lofts [2][4]float64
		cuts  [2][2][2]float64
	}{
		{
			name:  "left_x",
			prism: [4]float64{0, 0, 10, 10},
			lofts: [2][4]float64{{-3, 3, 2, 7}, {-3, 2, 2, 8}},
			cuts:  [2][2][2]float64{{{0, 3}, {0, 7}}, {{0, 2}, {0, 8}}},
		},
		{
			name:  "translated_524",
			prism: [4]float64{0, -eps, 120, 1 + eps},
			lofts: [2][4]float64{
				{119, 0, 163 - 896*eps, 1},
				{119, 0, 165 - 1024*eps, 1},
			},
			cuts: [2][2][2]float64{{{120, 0}, {120, 1}}, {{120, 0}, {120, 1}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var scenes [2]*sketch.Sketch
			for cap := range scenes {
				l := tc.lofts[cap]
				scenes[cap] = lineScene(t, [][][2]float64{
					fmaCapRectangle(tc.prism[0], tc.prism[1], tc.prism[2], tc.prism[3]),
					fmaCapRectangle(l[0], l[1], l[2], l[3]),
				}, nil)
			}
			var charged uint64
			profiles, err := sketch.ProfilesTwoLineCapsBounded(t.Context(), scenes,
				func(n uint64) error {
					if charged+n > 4096 {
						t.Fatalf("shared source work exceeds 4096")
					}
					charged += n
					return nil
				})
			if err != nil {
				t.Fatalf("real paired producer: %v, charged=%d", err, charged)
			}
			t.Logf("paired producer charged %d of 4096 steps", charged)
			for cap, found := range profiles {
				if len(found) != 3 {
					t.Fatalf("cap %d: profiles=%d", cap, len(found))
				}
				seen := map[[2]float64]bool{}
				for _, p := range found {
					if !p.Valid || len(p.Holes) != 0 {
						t.Fatalf("cap %d: invalid or holed profile", cap)
					}
					for edge := range p.Outer {
						current := p.Outer[edge]
						if !current.TExact {
							t.Fatalf("cap %d: inexact source edge", cap)
						}
						next := p.Outer[(edge+1)%len(p.Outer)]
						point, ok := scenes[cap].CertifyLineLineJunction(current, next)
						if ok {
							seen[point] = true
						}
					}
				}
				for _, point := range tc.cuts[cap] {
					if !seen[point] {
						t.Fatalf("cap %d: missing certified cut (%g,%g)",
							cap, point[0], point[1])
					}
				}
			}
		})
	}
}
