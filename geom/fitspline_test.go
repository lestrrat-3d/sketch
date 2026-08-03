package geom_test

import (
	"math"
	"strconv"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func TestRegionsFitSplineWithChord(t *testing.T) {
	a := geom.NewPoint(0, 0)
	m1 := geom.NewPoint(2, 3)
	m2 := geom.NewPoint(4, 3)
	b := geom.NewPoint(6, 0)
	fs, err := geom.NewFitSpline(a, m1, m2, b)
	require.NoError(t, err)
	chord := geom.NewLine(b, a)
	arr := geom.Regions([]geom.Curve{fs, chord}, nil)
	require.Len(t, arr.Regions, 1, "fit spline + chord bound one region")
	require.Greater(t, arr.Regions[0].Area, 0.0)
	require.False(t, arr.Regions[0].SelfIntersecting)
	require.Empty(t, arr.SelfIntersections)
}

func TestRegionsFitSplineSelfCrossing(t *testing.T) {
	// Fit points whose natural-cubic interpolant loops across itself, closed by a chord.
	a := geom.NewPoint(0, 0)
	m1 := geom.NewPoint(4, 1)
	m2 := geom.NewPoint(0, 2)
	m3 := geom.NewPoint(4, 3)
	fs, err := geom.NewFitSpline(a, m1, m2, m3)
	require.NoError(t, err)
	chord := geom.NewLine(m3, a)
	arr := geom.Regions([]geom.Curve{fs, chord}, nil)
	require.NotEmpty(t, arr.SelfIntersections, "the interpolant weaves across itself")
}

func TestFitSplineInterpolatesEval(t *testing.T) {
	fit := []*geom.Point{geom.NewPoint(0, 0), geom.NewPoint(1, 2), geom.NewPoint(3, -1), geom.NewPoint(5, 1), geom.NewPoint(6, 0)}
	sp, err := geom.NewFitSpline(fit...)
	require.NoError(t, err)
	var cum []float64
	var total float64
	cum = append(cum, 0)
	for i := 1; i < len(fit); i++ {
		total += math.Hypot(fit[i].X-fit[i-1].X, fit[i].Y-fit[i-1].Y)
		cum = append(cum, total)
	}
	for i, p := range fit {
		x, y := sp.Eval(cum[i] / total)
		require.InDelta(t, p.X, x, 1e-9, "interpolates fit point %d", i)
		require.InDelta(t, p.Y, y, 1e-9)
	}
}

func TestFitSplineTwoPointsIsLine(t *testing.T) {
	sp, err := geom.NewFitSpline(geom.NewPoint(0, 0), geom.NewPoint(6, 3))
	require.NoError(t, err)
	x, y := sp.Eval(0.5)
	require.InDelta(t, 3, x, 1e-9, "two fit points evaluate as a straight line")
	require.InDelta(t, 1.5, y, 1e-9)
}

func TestNewFitSplineMinTwo(t *testing.T) {
	_, err := geom.NewFitSpline(geom.NewPoint(0, 0))
	require.ErrorIs(t, err, geom.ErrTooFewFitPoints)
}

// evalFitSpan evaluates one published span at chord parameter p, in the span's own
// normalized local parameter u = (p−PStart)/(PEnd−PStart).
func evalFitSpan(sp geom.FitSpan, p float64) (float64, float64) {
	u := (p - sp.PStart) / (sp.PEnd - sp.PStart)
	x := sp.X[0] + u*(sp.X[1]+u*(sp.X[2]+u*sp.X[3]))
	y := sp.Y[0] + u*(sp.Y[1]+u*(sp.Y[2]+u*sp.Y[3]))
	return x, y
}

// requireSpanReproducesEval checks that a published span reproduces Eval across its
// own range. The tolerance is relative to the span's own COEFFICIENT scale rather
// than to the curve value: a coordinate that passes through zero inside the span has
// no relative scale of its own, while the coefficients state the size of the
// displacement the polynomial describes there.
func requireSpanReproducesEval(t *testing.T, fi *geom.FitInterpolant, s geom.FitSpan, total, rel float64) {
	t.Helper()
	var scaleX, scaleY float64
	for k := range s.X {
		scaleX = math.Max(scaleX, math.Abs(s.X[k]))
		scaleY = math.Max(scaleY, math.Abs(s.Y[k]))
	}
	for k := 0; k <= 8; k++ {
		// The fraction is applied to the width, not multiplied into it: at the top
		// of the range the intermediate product would overflow the sample itself.
		p := s.PStart + (s.PEnd-s.PStart)*(float64(k)/8)
		wantX, wantY := fi.Eval(p / total)
		gotX, gotY := evalFitSpan(s, p)
		require.InDelta(t, wantX, gotX, rel*scaleX, "span x reproduces Eval at p=%v", p)
		require.InDelta(t, wantY, gotY, rel*scaleY, "span y reproduces Eval at p=%v", p)
	}
}

func TestFitInterpolantDescribesTheEvaluatedCurve(t *testing.T) {
	fit := []*geom.Point{geom.NewPoint(0, 0), geom.NewPoint(1, 2), geom.NewPoint(3, -1), geom.NewPoint(5, 1), geom.NewPoint(6, 0)}
	sp, err := geom.NewFitSpline(fit...)
	require.NoError(t, err)
	fi, err := sp.Interpolant()
	require.NoError(t, err)

	require.Len(t, fi.Points, len(fit), "no fit point coincides, so none collapses")
	require.Len(t, fi.Params, len(fit))
	require.Len(t, fi.SecondDerivs, len(fit))
	require.Equal(t, 0.0, fi.Params[0], "the chord parameter starts at zero")
	for i, p := range fit {
		require.Equal(t, [2]float64{p.X, p.Y}, fi.Points[i], "point %d is the fit point itself", i)
		if i > 0 {
			require.Greater(t, fi.Params[i], fi.Params[i-1], "chord parameters increase")
			require.InDelta(t, math.Hypot(p.X-fit[i-1].X, p.Y-fit[i-1].Y), fi.Params[i]-fi.Params[i-1], 1e-12,
				"span %d spans its own chord length", i-1)
		}
	}
	require.Equal(t, [2]float64{0, 0}, fi.SecondDerivs[0], "natural end condition")
	require.Equal(t, [2]float64{0, 0}, fi.SecondDerivs[len(fi.SecondDerivs)-1])

	for i := 0; i <= 20; i++ {
		u := float64(i) / 20
		wantX, wantY := sp.Eval(u)
		gotX, gotY := fi.Eval(u)
		require.Equal(t, wantX, gotX, "the interpolant reproduces Eval bit for bit at t=%v", u)
		require.Equal(t, wantY, gotY)

		coords := [][2]float64{{fit[0].X, fit[0].Y}}
		for _, p := range fit[1:] {
			coords = append(coords, [2]float64{p.X, p.Y})
		}
		wantDX, wantDY, err := geom.EvalFitSplineDeriv(coords, u)
		require.NoError(t, err)
		gotDX, gotDY := fi.EvalDeriv(u)
		require.Equal(t, wantDX, gotDX, "and reproduces the tangent at t=%v", u)
		require.Equal(t, wantDY, gotDY)
	}
}

func TestFitInterpolantSpansReconstructCurve(t *testing.T) {
	fit := []*geom.Point{geom.NewPoint(0, 0), geom.NewPoint(1, 2), geom.NewPoint(3, -1), geom.NewPoint(5, 1), geom.NewPoint(6, 0)}
	sp, err := geom.NewFitSpline(fit...)
	require.NoError(t, err)
	fi, err := sp.Interpolant()
	require.NoError(t, err)
	spans := fi.Spans()
	require.Len(t, spans, len(fit)-1, "one cubic piece per interval")

	total := fi.Params[len(fi.Params)-1]
	require.Equal(t, 0.0, spans[0].TStart, "the normalized spans cover [0,1]")
	require.Equal(t, 1.0, spans[len(spans)-1].TEnd)
	for i, s := range spans {
		require.Equal(t, fi.Params[i], s.PStart)
		require.Equal(t, fi.Params[i+1], s.PEnd)
		require.InDelta(t, s.PStart/total, s.TStart, 1e-15, "span %d normalizes by the total chord length", i)
		if i > 0 {
			require.Equal(t, spans[i-1].TEnd, s.TStart, "spans meet at span %d", i)
		}
		// The polynomial reproduces the curve across its own span, endpoints included.
		for k := 0; k <= 8; k++ {
			p := s.PStart + (s.PEnd-s.PStart)*(float64(k)/8)
			wantX, wantY := sp.Eval(p / total)
			gotX, gotY := evalFitSpan(s, p)
			require.InDelta(t, wantX, gotX, 1e-9, "span %d at p=%v", i, p)
			require.InDelta(t, wantY, gotY, 1e-9)
		}
	}
}

func TestFitInterpolantCollapsesCoincidentFitPoints(t *testing.T) {
	// The exported spans must describe the curve actually evaluated, so a repeated
	// fit point (a zero-length chord) is absent from them.
	fit := []*geom.Point{geom.NewPoint(0, 0), geom.NewPoint(2, 3), geom.NewPoint(2, 3), geom.NewPoint(6, 0)}
	sp, err := geom.NewFitSpline(fit...)
	require.NoError(t, err)
	fi, err := sp.Interpolant()
	require.NoError(t, err)
	require.Len(t, fi.Points, 3, "the repeated fit point collapses away")
	require.Equal(t, [2]float64{2, 3}, fi.Points[1])
	require.Len(t, fi.Spans(), 2)
}

func TestFitInterpolantAllCoincidentIsOnePoint(t *testing.T) {
	fi, err := geom.NewFitInterpolant([][2]float64{{4, 5}, {4, 5}, {4, 5}})
	require.NoError(t, err)
	require.Equal(t, [][2]float64{{4, 5}}, fi.Points)
	require.Empty(t, fi.Spans(), "a single point spans nothing")
	x, y := fi.Eval(0.5)
	require.Equal(t, 4.0, x)
	require.Equal(t, 5.0, y)
	dx, dy := fi.EvalDeriv(0.5)
	require.Equal(t, 0.0, dx, "a degenerate interpolant has no tangent")
	require.Equal(t, 0.0, dy)
}

// requireFinite fails unless v is an ordinary number — the point of the coherent
// prefix is that no exported reader hands back NaN or an infinity.
func requireFinite(t *testing.T, what string, v float64) {
	t.Helper()
	require.False(t, math.IsNaN(v), "%s is NaN", what)
	require.False(t, math.IsInf(v, 0), "%s is infinite", what)
}

func TestFitInterpolantHandBuiltParamsReadCoherentPrefix(t *testing.T) {
	// The exported fields let a caller build a value whose Params violate the
	// strictly-increasing precondition. Every exported reader must then describe the
	// same finite curve — the leading run of Params that IS one — instead of dividing
	// by a zero, negative or NaN span width and publishing the result.
	for _, tc := range []struct {
		name   string
		params []float64
		prefix int
	}{
		{name: "repeated parameter", params: []float64{0, 0}, prefix: 1},
		{name: "decreasing parameter", params: []float64{0, -1}, prefix: 1},
		{name: "NaN parameter", params: []float64{0, math.NaN()}, prefix: 1},
		{name: "infinite parameter", params: []float64{0, math.Inf(1)}, prefix: 1},
		{name: "non-monotone after one good span", params: []float64{0, 5, 3}, prefix: 2},
		{name: "repeated parameter after one good span", params: []float64{0, 5, 5}, prefix: 2},
		{name: "does not start at zero", params: []float64{1, 2}, prefix: 0},
		{name: "starts at NaN", params: []float64{math.NaN(), 1}, prefix: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			points := make([][2]float64, len(tc.params))
			derivs := make([][2]float64, len(tc.params))
			for i := range points {
				points[i] = [2]float64{float64(i) * 3, float64(i%2) * 4}
				if i > 0 && i < len(points)-1 {
					derivs[i] = [2]float64{0.5, -0.25} // a real cubic term on the good span
				}
			}
			fi := &geom.FitInterpolant{Params: tc.params, Points: points, SecondDerivs: derivs}

			spans := fi.Spans()
			require.Len(t, spans, max(tc.prefix-1, 0), "spans cover the coherent prefix and nothing else")
			for i, s := range spans {
				requireFinite(t, "TStart", s.TStart)
				requireFinite(t, "TEnd", s.TEnd)
				requireFinite(t, "PStart", s.PStart)
				requireFinite(t, "PEnd", s.PEnd)
				for k := range s.X {
					requireFinite(t, "X coefficient", s.X[k])
					requireFinite(t, "Y coefficient", s.Y[k])
				}
				require.Less(t, s.TStart, s.TEnd, "span %d runs forward", i)
				require.GreaterOrEqual(t, s.TStart, 0.0, "span %d starts inside [0,1]", i)
				require.LessOrEqual(t, s.TEnd, 1.0, "span %d ends inside [0,1]", i)
				require.Equal(t, tc.params[i], s.PStart)
				require.Equal(t, tc.params[i+1], s.PEnd)
			}

			for i := 0; i <= 10; i++ {
				u := float64(i) / 10
				x, y := fi.Eval(u)
				requireFinite(t, "Eval x", x)
				requireFinite(t, "Eval y", y)
				dx, dy := fi.EvalDeriv(u)
				requireFinite(t, "EvalDeriv x", dx)
				requireFinite(t, "EvalDeriv y", dy)
			}

			switch {
			case tc.prefix == 0:
				x, y := fi.Eval(0.5)
				require.Equal(t, 0.0, x, "an empty prefix reads like an empty interpolant")
				require.Equal(t, 0.0, y)
				dx, dy := fi.EvalDeriv(0.5)
				require.Equal(t, 0.0, dx)
				require.Equal(t, 0.0, dy)
			case tc.prefix == 1:
				x, y := fi.Eval(0.5)
				require.Equal(t, points[0], [2]float64{x, y}, "a one-point prefix evaluates as that point")
				x, y = fi.Eval(1)
				require.Equal(t, points[0], [2]float64{x, y}, "at every parameter, including the end")
				dx, dy := fi.EvalDeriv(0.5)
				require.Equal(t, 0.0, dx, "a one-point prefix has no tangent")
				require.Equal(t, 0.0, dy)
			default:
				// Eval and the spans must describe the SAME curve: the two exported
				// views disagreed at t=1 while a bad parameter was still read.
				total := tc.params[tc.prefix-1]
				x, y := fi.Eval(1)
				require.Equal(t, points[tc.prefix-1], [2]float64{x, y}, "the prefix's last point ends the curve")
				for _, s := range spans {
					for k := 0; k <= 8; k++ {
						p := s.PStart + (s.PEnd-s.PStart)*(float64(k)/8)
						wantX, wantY := fi.Eval(p / total)
						gotX, gotY := evalFitSpan(s, p)
						require.InDelta(t, wantX, gotX, 1e-9, "span and Eval agree at p=%v", p)
						require.InDelta(t, wantY, gotY, 1e-9)
					}
				}
			}
		})
	}
}

func TestFitInterpolantEmpty(t *testing.T) {
	_, err := geom.NewFitInterpolant([][2]float64{{0, 0}})
	require.ErrorIs(t, err, geom.ErrTooFewFitPoints)

	empty := &geom.FitInterpolant{}
	x, y := empty.Eval(0.5)
	require.Equal(t, 0.0, x, "an empty interpolant evaluates without panicking")
	require.Equal(t, 0.0, y)
	require.Empty(t, empty.Spans())
	bare, err := (&geom.FitSpline{}).Interpolant()
	require.NoError(t, err)
	require.Empty(t, bare.Points)
}

func TestFitInterpolantConstructorsRefuseIndescribableCurve(t *testing.T) {
	// Every coordinate here is an ordinary finite number, but the chord length
	// between them is not representable. The prefix rule would then read the
	// interpolant as its FIRST POINT while the spline it came from still evaluates
	// the whole curve, so the constructors refuse instead of describing a different
	// curve with no error anywhere.
	for _, tc := range []struct {
		name string
		fit  [][2]float64
	}{
		{name: "chord length overflows", fit: [][2]float64{{-1e308, 0}, {1e308, 1}}},
		{name: "cumulative parameter stops increasing", fit: [][2]float64{{0, 0}, {1e300, 0}, {1e300, 1e-6}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fi, err := geom.NewFitInterpolant(tc.fit)
			require.ErrorIs(t, err, geom.ErrNonFiniteFitInterpolant)
			require.Nil(t, fi, "a refused interpolant is not handed back at all")

			pts := make([]*geom.Point, len(tc.fit))
			for i, p := range tc.fit {
				pts[i] = geom.NewPoint(p[0], p[1])
			}
			sp, err := geom.NewFitSpline(pts...)
			require.NoError(t, err)
			fi, err = sp.Interpolant()
			require.ErrorIs(t, err, geom.ErrNonFiniteFitInterpolant, "the spline method refuses through the same gate")
			require.Nil(t, fi)

			// The spline itself is unchanged: it still evaluates its own last fit
			// point, which is exactly what a truncated interpolant would have lost.
			x, y := sp.Eval(1)
			last := tc.fit[len(tc.fit)-1]
			require.Equal(t, last[0], x, "the spline still ends at its last fit point")
			require.Equal(t, last[1], y)
		})
	}
}

func TestFitInterpolantConstructorBuiltValueIsReadWhole(t *testing.T) {
	// The prefix rule exists for hand-built values; it must never shorten one the
	// evaluator produced. Across coordinate scales, everything the constructors
	// return is read over its full length by all three readers — and each published
	// span RECONSTRUCTS Eval over its own range, which finiteness alone never showed:
	// a coefficient stated in the absolute parameter underflows to zero at the wide
	// scales here, dropping a term of the cubic while staying perfectly finite.
	for _, scale := range []float64{1e-12, 1e-6, 1, 1e6, 1e100, 1e160, 1e170, 1e300} {
		t.Run(strconv.FormatFloat(scale, 'g', -1, 64), func(t *testing.T) {
			fit := [][2]float64{{0, 0}, {scale, scale / 2}, {2 * scale, 0}, {3 * scale, scale}}
			fi, err := geom.NewFitInterpolant(fit)
			require.NoError(t, err)
			require.Len(t, fi.Points, len(fit), "no point is dropped")

			total := fi.Params[len(fi.Params)-1]
			spans := fi.Spans()
			require.Len(t, spans, len(fit)-1, "the whole value is spanned, not a prefix of it")
			for i, s := range spans {
				for k := range s.X {
					requireFinite(t, "X coefficient", s.X[k])
					requireFinite(t, "Y coefficient", s.Y[k])
				}
				require.Equal(t, fi.Params[i], s.PStart)
				require.Equal(t, fi.Params[i+1], s.PEnd)
				requireSpanReproducesEval(t, fi, s, total, 1e-12)
			}

			for i, p := range fi.Points {
				x, y := fi.Eval(fi.Params[i] / total)
				require.InDelta(t, p[0], x, math.Abs(p[0])*1e-12+scale*1e-12, "Eval interpolates point %d", i)
				require.InDelta(t, p[1], y, math.Abs(p[1])*1e-12+scale*1e-12)
			}
			x, y := fi.Eval(1)
			require.Equal(t, fi.Points[len(fi.Points)-1], [2]float64{x, y}, "the curve ends at the last point, not at a truncated one")
		})
	}
}

func TestFitInterpolantSpanHoldsAtLargeChordParameters(t *testing.T) {
	// Two constructor-built curves whose chord parameter is large enough that a
	// coefficient stated in the absolute parameter p underflows to zero: it carries
	// 1/h or 1/h², the term it belongs to is silently dropped, and the published
	// polynomial then describes a different curve from Eval while every coefficient
	// stays finite. Stated in the span's own parameter both are ordinary numbers.
	for _, tc := range []struct {
		name string
		fit  [][2]float64
	}{
		{
			name: "zig-zag across the top of the range",
			fit:  [][2]float64{{0, 0}, {0, 4e307}, {0, 0}, {0, 4e307}, {0, 0}},
		},
		{
			name: "one span the whole width of the range",
			fit:  [][2]float64{{0, 0}, {1e-100, math.MaxFloat64}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fi, err := geom.NewFitInterpolant(tc.fit)
			require.NoError(t, err, "the curve is describable, so it is not refused")
			spans := fi.Spans()
			require.Len(t, spans, len(tc.fit)-1)

			total := fi.Params[len(fi.Params)-1]
			for i, s := range spans {
				// The comparison stays inside the span: its own midpoint is a
				// parameter Eval answers from this very span.
				p := s.PStart + (s.PEnd-s.PStart)/2
				wantX, wantY := fi.Eval(p / total)
				gotX, gotY := evalFitSpan(s, p)
				require.InDelta(t, wantX, gotX, math.Abs(wantX)*1e-12, "span %d reproduces Eval in x", i)
				require.InDelta(t, wantY, gotY, math.Abs(wantY)*1e-12, "span %d reproduces Eval in y", i)
				requireSpanReproducesEval(t, fi, s, total, 1e-12)
			}
		})
	}
}

func TestFitInterpolantHandBuiltOverflowingSpanIsOutsidePrefix(t *testing.T) {
	// A strictly increasing, finite Params can still give a span whose published
	// coefficients overflow: in the normalized parameter the cubic terms carry h²,
	// so a wide span with a real second derivative describes a displacement no
	// double holds. Such a span is outside the prefix, and the readers that are
	// left agree over what remains.
	for _, tc := range []struct {
		name   string
		params []float64
		points [][2]float64
		derivs [][2]float64
		prefix int
	}{
		{
			name:   "wide span with a second derivative",
			params: []float64{0, 1e200},
			points: [][2]float64{{0, 0}, {1, 0}},
			derivs: [][2]float64{{1, 0}, {0, 0}},
			prefix: 1,
		},
		{
			name:   "wide span whose second derivative is only at its far end",
			params: []float64{0, 1e200},
			points: [][2]float64{{0, 0}, {0, 1}},
			derivs: [][2]float64{{0, 0}, {0, 1}},
			prefix: 1,
		},
		{
			name:   "overflow after one describable span",
			params: []float64{0, 1, 1e200},
			points: [][2]float64{{0, 0}, {1, 1}, {2, 2}},
			derivs: [][2]float64{{0, 0}, {1, 1}, {0, 0}},
			prefix: 2,
		},
		{
			name:   "non-finite coordinate",
			params: []float64{0, 1, 2},
			points: [][2]float64{{0, 0}, {3, 4}, {math.Inf(1), 0}},
			derivs: [][2]float64{{0, 0}, {0, 0}, {0, 0}},
			prefix: 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fi := &geom.FitInterpolant{Params: tc.params, Points: tc.points, SecondDerivs: tc.derivs}

			spans := fi.Spans()
			require.Len(t, spans, tc.prefix-1, "only describable spans are part of the curve")
			for _, s := range spans {
				for k := range s.X {
					requireFinite(t, "X coefficient", s.X[k])
					requireFinite(t, "Y coefficient", s.Y[k])
				}
			}

			for i := 0; i <= 10; i++ {
				u := float64(i) / 10
				x, y := fi.Eval(u)
				requireFinite(t, "Eval x", x)
				requireFinite(t, "Eval y", y)
				dx, dy := fi.EvalDeriv(u)
				requireFinite(t, "EvalDeriv x", dx)
				requireFinite(t, "EvalDeriv y", dy)
			}

			x, y := fi.Eval(1)
			require.Equal(t, tc.points[tc.prefix-1], [2]float64{x, y}, "the curve ends at the prefix's last point")
			if tc.prefix == 1 {
				dx, dy := fi.EvalDeriv(0.5)
				require.Equal(t, 0.0, dx, "a one-point prefix has no tangent")
				require.Equal(t, 0.0, dy)
				return
			}
			total := tc.params[tc.prefix-1]
			for _, s := range spans {
				requireSpanReproducesEval(t, fi, s, total, 1e-12)
			}
		})
	}
}

func TestFitInterpolantHandBuiltNarrowSpanIsDescribable(t *testing.T) {
	// The converse of the case above, and the class the normalized parameter ends:
	// a span this narrow puts 1/h and 1/h² factors far beyond floating point, so an
	// absolute local parameter cannot describe it at all. In u the same cubic is an
	// ordinary polynomial, so the span is published rather than cut from the curve.
	for _, tc := range []struct {
		name   string
		params []float64
		points [][2]float64
		want   [4]float64
	}{
		{
			name:   "denormal span width",
			params: []float64{0, 5e-324},
			points: [][2]float64{{0, 0}, {1, 1}},
			want:   [4]float64{0, 1, 0, 0},
		},
		{
			name:   "narrow span across a large displacement",
			params: []float64{0, 1e-300},
			points: [][2]float64{{0, 0}, {1e100, 0}},
			want:   [4]float64{0, 1e100, 0, 0},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fi := &geom.FitInterpolant{
				Params:       tc.params,
				Points:       tc.points,
				SecondDerivs: make([][2]float64, len(tc.params)),
			}
			spans := fi.Spans()
			require.Len(t, spans, 1, "the span is describable, so it is part of the curve")
			require.Equal(t, tc.want, spans[0].X, "the x cubic is stated in the span's own parameter")
			x, y := fi.Eval(1)
			require.Equal(t, tc.points[1], [2]float64{x, y}, "and Eval reads the same whole curve")
			gotX, gotY := evalFitSpan(spans[0], spans[0].PEnd)
			require.Equal(t, tc.points[1][0], gotX, "the span reproduces that endpoint")
			require.Equal(t, tc.points[1][1], gotY)
		})
	}
}
