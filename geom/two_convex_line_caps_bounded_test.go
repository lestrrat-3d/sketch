package geom

import (
	"context"
	"errors"
	"math"
	"math/big"
	"testing"
)

func convexCapRectangle(prism, loft [4]float64) []Curve {
	curves := make([]Curve, 0, 8)
	for _, box := range [2][4]float64{prism, loft} {
		points := [4]*Point{
			NewPoint(box[0], box[1]), NewPoint(box[2], box[1]),
			NewPoint(box[2], box[3]), NewPoint(box[0], box[3]),
		}
		for i := range points {
			curves = append(curves, NewLine(points[i], points[(i+1)%4]))
		}
	}
	return curves
}

func convexCapRealScenes() [2][]Curve {
	prism := [4]float64{0, 0, 10, 10}
	return [2][]Curve{
		convexCapRectangle(prism, [4]float64{-3, 3, 2, 7}),
		convexCapRectangle(prism, [4]float64{-3, 2, 2, 8}),
	}
}

func TestTwoConvexLineCapsBoundedFMAEndpoint(t *testing.T) {
	if math.FMA(3.0/5.0, 5, -3) == 0 {
		t.Fatal("fixture does not exercise the fused endpoint mismatch")
	}
	scenes := convexCapRealScenes()
	var charged uint64
	arrs, err := RegionsTwoConvexLineCapsBounded(t.Context(), scenes,
		func(n uint64) error {
			if charged+n > 4096 {
				return ErrUnsupportedLineScene
			}
			charged += n
			return nil
		})
	if err != nil {
		t.Fatalf("real two-cap producer: %v, charged=%d", err, charged)
	}
	if charged == 0 {
		t.Fatal("real producer did not charge work")
	}
	t.Logf("real two-cap producer charged %d of 4096 steps", charged)
	for cap, arr := range arrs {
		if arr == nil {
			t.Fatalf("cap %d: nil arrangement", cap)
		}
		if len(arr.Regions) != 3 || arr.Degenerate || len(arr.Chains) != 0 {
			t.Fatalf("cap %d: regions=%d degenerate=%t chains=%d",
				cap, len(arr.Regions), arr.Degenerate, len(arr.Chains))
		}
		tokens := map[*LineLineCrossing]struct{}{}
		for _, region := range arr.Regions {
			for _, edge := range region.Outer {
				if !edge.TExact || edge.TStart >= edge.TEnd ||
					len(edge.Polyline) != 2 {
					t.Fatalf("cap %d: bad edge source=%d exact=%t",
						cap, edge.SourceIndex, edge.TExact)
				}
				for _, token := range [2]*LineLineCrossing{
					edge.LineLineCrossingAtStart(), edge.LineLineCrossingAtEnd(),
				} {
					if token != nil {
						tokens[token] = struct{}{}
					}
				}
			}
		}
		if len(tokens) != 2 {
			t.Fatalf("cap %d: crossing tokens=%d, want 2", cap, len(tokens))
		}
		seen := map[float64]bool{}
		for token := range tokens {
			point := token.Point()
			if point[0] != 0 {
				t.Fatalf("cap %d: crossing x=%g", cap, point[0])
			}
			seen[point[1]] = true
		}
		wantLower, wantUpper := 3.0, 7.0
		if cap == 1 {
			wantLower, wantUpper = 2, 8
		}
		if !seen[wantLower] || !seen[wantUpper] {
			t.Fatalf("cap %d: crossing heights missing: lower=%t upper=%t",
				cap, seen[wantLower], seen[wantUpper])
		}
	}
}

func TestConvexCapEndpointRefusesOutsideSourceRoundoff(t *testing.T) {
	a := convexCapPoint{xy: [2]float64{-3, 4},
		rat: [2]*big.Rat{capRat(-3), capRat(4)}}
	b := convexCapPoint{xy: [2]float64{2, 4},
		rat: [2]*big.Rat{capRat(2), capRat(4)}}
	crossing := convexCapPoint{xy: [2]float64{0, 4},
		rat: [2]*big.Rat{capRat(0), capRat(4)}}
	var charged uint64
	take := func(n uint64) error { charged += n; return nil }
	ok, err := convexCapEndpointReproduces(a, b, crossing, 3.0/5.0, take)
	if err != nil || !ok {
		t.Fatalf("exact crossing refused: ok=%t err=%v", ok, err)
	}
	ok, err = convexCapEndpointReproduces(a, b, crossing, 3.0/5.0+1e-9, take)
	if err != nil || ok {
		t.Fatalf("out-of-bound endpoint admitted: ok=%t err=%v", ok, err)
	}
	if charged == 0 || charged > 128 {
		t.Fatalf("endpoint checks charged=%d, want bounded work", charged)
	}
	endpointErr := errors.New("endpoint work refused")
	ok, err = convexCapEndpointReproduces(a, b, crossing, 3.0/5.0,
		func(uint64) error { return endpointErr })
	if ok || !errors.Is(err, endpointErr) {
		t.Fatalf("endpoint budget: ok=%t err=%v", ok, err)
	}
}

func TestTwoConvexLineCapsBoundedStopsOnBudgetAndCancellation(t *testing.T) {
	scenes := convexCapRealScenes()
	budgetErr := errors.New("focused caller budget")
	calls := 0
	got, err := RegionsTwoConvexLineCapsBounded(t.Context(), scenes,
		func(uint64) error { calls++; return budgetErr })
	if !errors.Is(err, budgetErr) || got[0] != nil || got[1] != nil || calls != 1 {
		t.Fatalf("budget refusal: err=%v first_nil=%t second_nil=%t calls=%d",
			err, got[0] == nil, got[1] == nil, calls)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	got, err = RegionsTwoConvexLineCapsBounded(canceled, scenes,
		func(uint64) error { t.Fatal("charged after cancellation"); return nil })
	if !errors.Is(err, context.Canceled) || got[0] != nil || got[1] != nil {
		t.Fatalf("cancellation: err=%v first_nil=%t second_nil=%t",
			err, got[0] == nil, got[1] == nil)
	}
}

func TestConvexCapWholeEndpointsUseExactSourceIdentity(t *testing.T) {
	a := convexCapPoint{xy: [2]float64{-3, 4},
		rat: [2]*big.Rat{capRat(-3), capRat(4)}}
	b := convexCapPoint{xy: [2]float64{2, 4},
		rat: [2]*big.Rat{capRat(2), capRat(4)}}
	verified := map[convexCapEndpointKey]struct{}{}
	var charged uint64
	take := func(n uint64) error { charged += n; return nil }
	ok, err := convexCapCachedEndpoint(a, b, a, big.NewRat(0, 1), 0,
		4, verified, take)
	if err != nil || !ok {
		t.Fatalf("exact source start: ok=%t err=%v", ok, err)
	}
	first := charged
	if first == 0 || first >= 40 {
		t.Fatalf("whole-endpoint charge=%d, want below full cut proof", first)
	}
	ok, err = convexCapCachedEndpoint(a, b, a, big.NewRat(0, 1), 0,
		4, verified, take)
	if err != nil || !ok || charged != first+1 {
		t.Fatalf("cached source start: ok=%t err=%v charged=%d", ok, err, charged)
	}
	ok, err = convexCapCachedEndpoint(a, b, b, big.NewRat(1, 1), 1,
		4, verified, take)
	if err != nil || !ok {
		t.Fatalf("exact source end: ok=%t err=%v", ok, err)
	}
	if charged-first-1 >= 40 {
		t.Fatalf("whole-endpoint end charge=%d, want below full cut proof",
			charged-first-1)
	}
	ok, err = convexCapCachedEndpoint(a, b, a, big.NewRat(1, 1), 0,
		4, verified, take)
	if err != nil || ok {
		t.Fatalf("source end reused source start: ok=%t err=%v", ok, err)
	}
	wrong := convexCapPoint{xy: [2]float64{0, 4},
		rat: [2]*big.Rat{capRat(0), capRat(4)}}
	ok, err = convexCapCachedEndpoint(a, b, wrong, big.NewRat(0, 1), 0,
		4, verified, take)
	if err != nil || ok {
		t.Fatalf("wrong source start: ok=%t err=%v", ok, err)
	}

	// Rounded binary64 zero is not a whole source endpoint when the exact
	// parameter remains strictly inside the line.
	masquerade := convexCapPoint{xy: a.xy,
		rat: [2]*big.Rat{
			new(big.Rat).Add(a.rat[0], big.NewRat(1, 1<<60)),
			capRat(4),
		}}
	before := charged
	ok, err = convexCapCachedEndpoint(a, b, masquerade,
		big.NewRat(1, 1<<60), 0, 4, verified, take)
	if err != nil || ok || charged-before <= first {
		t.Fatalf("rounded cut masqueraded as whole: ok=%t err=%v charge=%d",
			ok, err, charged-before)
	}
	budgetErr := errors.New("whole-endpoint budget")
	ok, err = convexCapCachedEndpoint(a, b, a, big.NewRat(0, 1), 0,
		7, map[convexCapEndpointKey]struct{}{},
		func(n uint64) error {
			if n == 8 {
				return budgetErr
			}
			return nil
		})
	if ok || !errors.Is(err, budgetErr) {
		t.Fatalf("whole-endpoint budget: ok=%t err=%v", ok, err)
	}
}
