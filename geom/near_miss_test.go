package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// bowChordLine lays a line across the deepest bow of a curve's OWN default sample
// chord, just inside it, so the true curve crosses the line twice between two
// consecutive samples while the sampled polyline never reaches it. That is the
// hidden-crossing shape: the arrangement's planar map has no crossing at all,
// while the geometry has two.
//
// nDefault must be the segment count the arrangement itself samples that curve at,
// or the bow found here is not the bow the map is built from.
func bowChordLine(t *testing.T, poly func(int) [][2]float64, nDefault int) (*geom.Line, float64) {
	t.Helper()
	coarse := poly(nDefault)
	const sub = 400
	fine := poly(nDefault * sub)
	worst, at := -1.0, 0
	var bulge [2]float64
	for i := 1; i < len(coarse); i++ {
		pa, pb := coarse[i-1], coarse[i]
		ex, ey := pb[0]-pa[0], pb[1]-pa[1]
		l := math.Hypot(ex, ey)
		if l == 0 {
			continue
		}
		for k := (i-1)*sub + 1; k < i*sub && k < len(fine); k++ {
			q := fine[k]
			if d := math.Abs((q[0]-pa[0])*ey-(q[1]-pa[1])*ex) / l; d > worst {
				worst, at, bulge = d, i, q
			}
		}
	}
	require.Positive(t, worst, "the curve must bow away from its own chords")
	pa, pb := coarse[at-1], coarse[at]
	ex, ey := pb[0]-pa[0], pb[1]-pa[1]
	l := math.Hypot(ex, ey)
	ux, uy := ex/l, ey/l
	nx, ny := -uy, ux
	if (bulge[0]-pa[0])*nx+(bulge[1]-pa[1])*ny < 0 {
		nx, ny = -nx, -ny
	}
	mx := (pa[0]+pb[0])/2 + nx*0.1*worst
	my := (pa[1]+pb[1])/2 + ny*0.1*worst
	return geom.NewLine(geom.NewPoint(mx-ux*l, my-uy*l), geom.NewPoint(mx+ux*l, my+uy*l)), worst
}

func requireAreas(t *testing.T, arr *geom.Arrangement, want []float64) {
	t.Helper()
	require.Len(t, arr.Regions, len(want), "region count")
	for i, w := range want {
		require.InEpsilonf(t, w, arr.Regions[i].Area, 1e-9, "region %d area", i)
	}
}

// TestNearMissHiddenCrossingIsDegenerate pins the defect this guard exists for: a
// curve can bow far enough away from its own sample chords to hide a genuine
// crossing entirely, and before the guard every trust signal read clean over it.
//
// Each case asserts three things at once — the flag is now raised, the region
// count is the SAME one the pre-fix engine published, and the areas are the SAME
// numbers. The guard sets a flag and changes nothing else.
func TestNearMissHiddenCrossingIsDegenerate(t *testing.T) {
	t.Run("spline hides a tiny circle", func(t *testing.T) {
		// A 1000 mm cubic spline against a 0.05 mm circle centred ON the true curve,
		// in the middle of one of the spline's 64 default sample spans. The spline's
		// own chord runs far outside the circle, so the sampled map has the disk whole
		// and the spline nowhere on its boundary — one profile of pi*r^2 where the
		// truth is two half-disks.
		sp, err := geom.NewSpline(geom.NewPoint(-500, 300), geom.NewPoint(-250, -400),
			geom.NewPoint(0, -400), geom.NewPoint(250, -400), geom.NewPoint(500, 300))
		require.NoError(t, err)
		cx, cy := sp.Eval(40.5 / 64)
		closed := []geom.ClosedCurve{geom.NewCircle(geom.NewPoint(cx, cy), 0.05)}

		arr := geom.Regions([]geom.Curve{sp}, closed)
		require.True(t, arr.Degenerate, "a crossing hidden inside one sample span")
		requireAreas(t, arr, []float64{math.Pi * 0.05 * 0.05})
		for _, rg := range arr.Regions {
			require.True(t, rg.Degenerate, "the condition reaches this region's own curves")
			for _, b := range rg.Outer {
				require.Equal(t, 1, b.SourceIndex, "the spline is absent from the boundary")
			}
		}

		truth := geom.Regions([]geom.Curve{sp}, closed, geom.WithSegmentsPerTurn(256))
		require.Len(t, truth.Regions, 2, "the spline really does cut the disk in two")
		require.False(t, truth.Degenerate, "resolved sampling rules the crossing in")
	})

	t.Run("spline grazing a circle rim", func(t *testing.T) {
		// The other shape the band catches: not a crossing hidden inside one span but
		// a lens NARROWER than one span. A smile spline dipping 4.2e-05 mm through a
		// 50 mm circle's rim cuts a real sliver off the disk, and at 512 segments per
		// turn the map still has the disk whole — the sliver only emerges at 1024.
		// The two rims run inside the combined band with nothing recorded between
		// them, which is what the guard reports.
		//
		// Read at the DEFAULT density this pair is silent instead: the chords cross
		// there, close enough to the approach that the crossing explains it, so the
		// wrong region count goes unflagged. That is the guard's known limit — it
		// answers "was a crossing recorded near this approach", not "is the recorded
		// crossing count right".
		ctrl := [][2]float64{{-40, 40}, {-24, -34}, {0, -34}, {24, -34}, {40, 40}}
		var base []*geom.Point
		for _, c := range ctrl {
			base = append(base, geom.NewPoint(c[0], c[1]))
		}
		flat, err := geom.NewSpline(base...)
		require.NoError(t, err)
		low := math.Inf(1)
		for _, q := range flat.Polyline(50000) {
			low = math.Min(low, q[1])
		}
		var dipped []*geom.Point
		for _, c := range ctrl {
			dipped = append(dipped, geom.NewPoint(c[0], c[1]-4.217e-5-low))
		}
		sp, err := geom.NewSpline(dipped...)
		require.NoError(t, err)
		closed := []geom.ClosedCurve{geom.NewCircle(geom.NewPoint(0, -50), 50)}

		arr := geom.Regions([]geom.Curve{sp}, closed, geom.WithSegmentsPerTurn(512))
		require.True(t, arr.Degenerate, "a lens narrower than one sample span")
		// The disk whole, to within the sliver the map is missing.
		require.Len(t, arr.Regions, 1)
		require.InEpsilon(t, math.Pi*50*50, arr.Regions[0].Area, 1e-8)

		truth := geom.Regions([]geom.Curve{sp}, closed, geom.WithSegmentsPerTurn(1024))
		require.Len(t, truth.Regions, 2, "the sliver is real")
	})

	for _, tc := range []struct {
		name   string
		weight float64
	}{
		{"nurbs uniform knots unit weights", 1},
		{"nurbs middle weight 5", 5},
		{"nurbs middle weight 20", 20},
		{"nurbs middle weight 100", 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A degree-3 NURBS arch in an 80 mm scene, with a line laid inside the
			// deepest bow of its own default sampling. Raising one middle weight
			// scales the bow, and with it the crossing the sampled map loses.
			ctrl := []*geom.Point{geom.NewPoint(-40, 30), geom.NewPoint(-20, -20),
				geom.NewPoint(0, -34), geom.NewPoint(20, -20), geom.NewPoint(40, 30)}
			w := []float64{1, 1, tc.weight, 1, 1}
			nb := geom.NewNURBS(3, ctrl, geom.ClampedUniformKnots(len(ctrl), 3), w)
			ln, bow := bowChordLine(t, nb.Polyline, 16*len(ctrl))
			require.Positive(t, bow)
			open := []geom.Curve{nb, ln}

			arr := geom.Regions(open, nil)
			require.True(t, arr.Degenerate, "a crossing hidden inside one sample span")
			require.Empty(t, arr.Regions, "the lens is absent from the sampled map")

			truth := geom.Regions(open, nil, geom.WithSegmentsPerTurn(256))
			require.Len(t, truth.Regions, 1, "the line really does cut a lens off the arch")
			require.False(t, truth.Degenerate, "resolved sampling rules the crossing in")
		})
	}
}

// TestNearMissGuardLeavesCorrectAnswersAlone is the other half of the verdict: the
// guard must not flag geometry the sampled map already resolves. It is scoped to
// pairs with a free-form source, so an all-analytic scene never reaches it at all,
// and a free-form pair whose separation exceeds the two deviation bands is left
// alone whatever else is in the drawing.
func TestNearMissGuardLeavesCorrectAnswersAlone(t *testing.T) {
	t.Run("all-analytic scene untouched", func(t *testing.T) {
		// Two overlapping circles and a secant line: every crossing has a closed form,
		// so no deviation band enters the verdict.
		arr := geom.Regions(
			[]geom.Curve{geom.NewLine(geom.NewPoint(-12, 1), geom.NewPoint(12, 1))},
			[]geom.ClosedCurve{
				geom.NewCircle(geom.NewPoint(0, 0), 5),
				geom.NewCircle(geom.NewPoint(6, 0), 5),
			})
		require.False(t, arr.Degenerate)
		require.NotEmpty(t, arr.Regions)
	})

	t.Run("gear-shaped section untouched", func(t *testing.T) {
		// Alternating tooth flanks and root/tip arcs: the everyday analytic profile.
		const teeth = 9
		var open []geom.Curve
		c := geom.NewPoint(0, 0)
		at := func(r, a float64) *geom.Point {
			return geom.NewPoint(r*math.Cos(a), r*math.Sin(a))
		}
		step := 2 * math.Pi / teeth
		for k := 0; k < teeth; k++ {
			a0 := float64(k) * step
			open = append(open,
				geom.NewArc(c, at(20, a0), at(20, a0+step*0.4)),
				geom.NewLine(at(20, a0+step*0.4), at(16, a0+step*0.5)),
				geom.NewArc(c, at(16, a0+step*0.5), at(16, a0+step*0.9)),
				geom.NewLine(at(16, a0+step*0.9), at(20, a0+step)))
		}
		arr := geom.Regions(open, nil)
		require.False(t, arr.Degenerate)
		require.Len(t, arr.Regions, 1)
	})

	t.Run("spline clear of a circle", func(t *testing.T) {
		sp, err := geom.NewSpline(geom.NewPoint(-60, 40), geom.NewPoint(-30, -20),
			geom.NewPoint(0, -20), geom.NewPoint(30, -20), geom.NewPoint(60, 40))
		require.NoError(t, err)
		low := math.Inf(1)
		for _, p := range sp.Polyline(20000) {
			low = math.Min(low, p[1])
		}
		for _, clear := range []float64{0.2, 0.5, 1, 5} {
			arr := geom.Regions([]geom.Curve{sp},
				[]geom.ClosedCurve{geom.NewCircle(geom.NewPoint(0, low-clear-10), 10)})
			require.Falsef(t, arr.Degenerate, "clearance %g is far outside the band", clear)
		}
	})

	t.Run("spline meeting a line at shared endpoints", func(t *testing.T) {
		// A closed profile whose two curves touch at exactly the places the sampled map
		// already carries a vertex for. Zero separation is not a near miss when a
		// contact explains it.
		a, b := geom.NewPoint(-40, 0), geom.NewPoint(40, 0)
		sp, err := geom.NewSpline(a, geom.NewPoint(-20, 30), geom.NewPoint(20, 30), b)
		require.NoError(t, err)
		arr := geom.Regions([]geom.Curve{sp, geom.NewLine(a, b)}, nil)
		require.False(t, arr.Degenerate)
		require.Len(t, arr.Regions, 1)
	})

	t.Run("resolved crossing not flagged", func(t *testing.T) {
		// The same NURBS bow as the regression, at a density that resolves it: the
		// bands shrink quadratically, the crossing appears, and the flag clears.
		ctrl := []*geom.Point{geom.NewPoint(-40, 30), geom.NewPoint(-20, -20),
			geom.NewPoint(0, -34), geom.NewPoint(20, -20), geom.NewPoint(40, 30)}
		nb := geom.NewNURBS(3, ctrl, geom.ClampedUniformKnots(len(ctrl), 3), []float64{1, 1, 1, 1, 1})
		ln, _ := bowChordLine(t, nb.Polyline, 16*len(ctrl))
		arr := geom.Regions([]geom.Curve{nb, ln}, nil, geom.WithSegmentsPerTurn(256))
		require.False(t, arr.Degenerate)
		require.Len(t, arr.Regions, 1)
	})
}
