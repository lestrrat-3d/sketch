package geom_test

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// This file captures the CURRENT, untouched behaviour of geom.Regions/the
// curve kernels as a baseline (.tmp/perf-plan.md sections 1.3 and 4.2), so
// group C's broad-phase rewrite can prove it changed no observable output.
// Regeneration is gated on SKETCH_UPDATE_GOLDEN=1; a normal run compares
// against the committed files under testdata/golden/.

const updateGoldenEnv = "SKETCH_UPDATE_GOLDEN"

func goldenUpdate() bool { return os.Getenv(updateGoldenEnv) == "1" }

func goldenPath(name string) string { return filepath.Join("testdata", "golden", name) }

// loadOrStoreJSON writes actual as the golden (as JSON) when
// SKETCH_UPDATE_GOLDEN=1 and returns it unchanged, so the caller's assertions
// still run (trivially) on that path; otherwise it loads and returns the
// committed golden for the caller to compare actual against.
func loadOrStoreJSON[T any](t *testing.T, name string, actual T) T {
	t.Helper()
	path := goldenPath(name)
	if goldenUpdate() {
		data, err := json.MarshalIndent(actual, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, data, 0o644))
		return actual
	}
	data, err := os.ReadFile(path)
	require.NoError(t, err, "golden %s missing; run with SKETCH_UPDATE_GOLDEN=1", name)
	var want T
	require.NoError(t, json.Unmarshal(data, &want))
	return want
}

// requireRelClose asserts want and got agree to a relative tolerance rel,
// scaled by the larger magnitude (both zero always passes).
// requireRelClose compares two values to a relative tolerance, with the scale
// floored at 1. Without the floor a quantity that cancels to near zero — a
// coordinate on an axis, an area that sums away — would demand a tolerance far
// below the platform's own noise, since the residue left by a fused
// multiply-add is absolute, not relative to a result that is itself effectively
// zero.
func requireRelClose(t *testing.T, want, got, rel float64, msgAndArgs ...any) {
	t.Helper()
	if want == 0 && got == 0 {
		return
	}
	scale := math.Max(math.Max(math.Abs(want), math.Abs(got)), 1)
	require.InDelta(t, want, got, rel*scale, msgAndArgs...)
}

// --- section 4.2 fixture table ---------------------------------------------

type arrangeFixture struct {
	name  string
	build func(testing.TB) ([]geom.Curve, []geom.ClosedCurve)
}

// circles12Fixture returns the 12 disjoint circles (radius 10, 4x3 grid,
// spacing 30) shared by BenchmarkRegions/circles12 and the "disjoint curves"
// row below.
func circles12Fixture() ([]geom.Curve, []geom.ClosedCurve) {
	var closed []geom.ClosedCurve
	for row := 0; row < 3; row++ {
		for col := 0; col < 4; col++ {
			cx, cy := float64(col)*30, float64(row)*30
			closed = append(closed, geom.NewCircle(geom.NewPoint(cx, cy), 10))
		}
	}
	return nil, closed
}

// splineClosedLoopGeom builds a loop whose fourth side is an open spline
// instead of a straight line, sharing its corner points by identity.
func splineClosedLoopGeom(tb testing.TB, x0, y0, x1, y1 float64) []geom.Curve {
	tb.Helper()
	p0 := geom.NewPoint(x0, y0)
	p1 := geom.NewPoint(x1, y0)
	p2 := geom.NewPoint(x1, y1)
	p3 := geom.NewPoint(x0, y1)
	mid1 := geom.NewPoint(x0-0.25*(x1-x0), y0+0.35*(y1-y0))
	mid2 := geom.NewPoint(x0-0.1*(x1-x0), y0+0.7*(y1-y0))
	sp, err := geom.NewSpline(p3, mid2, mid1, p0)
	require.NoError(tb, err)
	return []geom.Curve{geom.NewLine(p0, p1), geom.NewLine(p1, p2), geom.NewLine(p2, p3), sp}
}

// splineLoopsFixture returns 4 disjoint closed splines (roughly circular, 6
// control points each) plus 2 loops each closed by one open spline (4
// controls) and 3 lines sharing endpoints with it — the "splines" fixture
// shared with BenchmarkRegions/splineLoops.
func splineLoopsFixture(tb testing.TB) ([]geom.Curve, []geom.ClosedCurve) {
	tb.Helper()
	var curves []geom.Curve
	var closed []geom.ClosedCurve
	for _, c := range [][2]float64{{0, 0}, {50, 0}, {0, 50}, {50, 50}} {
		sp, err := geom.NewClosedSpline(circlePoints(c[0], c[1], 10, 6)...)
		require.NoError(tb, err)
		closed = append(closed, sp)
	}
	curves = append(curves, splineClosedLoopGeom(tb, 120, 0, 150, 30)...)
	curves = append(curves, splineClosedLoopGeom(tb, 120, 60, 150, 90)...)
	return curves, closed
}

// disjointCurvesFixture pins 15 disjoint whole regions: the 12-circle grid
// plus 3 disjoint closed splines placed clear of it and of each other.
func disjointCurvesFixture(tb testing.TB) ([]geom.Curve, []geom.ClosedCurve) {
	_, closed := circles12Fixture()
	for _, c := range [][2]float64{{200, 0}, {240, 0}, {280, 0}} {
		sp, err := geom.NewClosedSpline(circlePoints(c[0], c[1], 8, 6)...)
		require.NoError(tb, err)
		closed = append(closed, sp)
	}
	return nil, closed
}

// nearMissGap is the separation used by every "near miss" fixture below; it
// also equals the WithVertexMerge(1e-3) setting every fixture is additionally
// run at (see TestGoldenArrangement), so the "inside/outside the window" and
// "non-transitive merge" fixtures are calibrated against a merge distance this
// test controls directly, not the arrangement's internally-derived default.
const nearMissGap = 1e-3

// nearMissOutsideFixture holds three near misses too far apart to weld or cut
// under any of the three settings: two circles with a 1e-3 gap, a line
// stopping 1e-3 short of a circle, and two parallel lines 1e-3 apart.
func nearMissOutsideFixture(testing.TB) ([]geom.Curve, []geom.ClosedCurve) {
	closed := []geom.ClosedCurve{
		geom.NewCircle(geom.NewPoint(0, 0), 10),
		geom.NewCircle(geom.NewPoint(20+nearMissGap, 0), 10),
		geom.NewCircle(geom.NewPoint(60, 0), 10),
	}
	curves := []geom.Curve{
		geom.NewLine(geom.NewPoint(30, 0), geom.NewPoint(50-nearMissGap, 0)),
		geom.NewLine(geom.NewPoint(0, -30), geom.NewPoint(10, -30)),
		geom.NewLine(geom.NewPoint(0, -30-nearMissGap), geom.NewPoint(10, -30-nearMissGap)),
	}
	return curves, closed
}

// nearMissInsideFixture holds two near misses close enough to weld under
// WithVertexMerge(1e-3): two lines whose unshared endpoints sit half that gap
// apart, and two circles whose nearest sample vertices sit half that gap
// apart.
func nearMissInsideFixture(testing.TB) ([]geom.Curve, []geom.ClosedCurve) {
	half := nearMissGap / 2
	curves := []geom.Curve{
		geom.NewLine(geom.NewPoint(0, 0), geom.NewPoint(10, 0)),
		geom.NewLine(geom.NewPoint(10+half, 0), geom.NewPoint(20, 0)),
	}
	closed := []geom.ClosedCurve{
		geom.NewCircle(geom.NewPoint(40, 0), 10),
		geom.NewCircle(geom.NewPoint(40+20+half, 0), 10),
	}
	return curves, closed
}

// endpointWeldsFixture pins vertex identity by shared pointer: a 4-line
// rectangle, a closed spline sharing a control point with a line endpoint,
// and a T-junction (a line endpoint landing exactly on a spline's interior
// sample point).
func endpointWeldsFixture(tb testing.TB) ([]geom.Curve, []geom.ClosedCurve) {
	a, b, c, d := geom.NewPoint(0, 0), geom.NewPoint(20, 0), geom.NewPoint(20, 20), geom.NewPoint(0, 20)
	curves := []geom.Curve{geom.NewLine(a, b), geom.NewLine(b, c), geom.NewLine(c, d), geom.NewLine(d, a)}

	shared := geom.NewPoint(60, 0)
	ctrl := append([]*geom.Point{shared}, circlePoints(60, 10, 10, 5)...)
	sp, err := geom.NewClosedSpline(ctrl...)
	require.NoError(tb, err)
	curves = append(curves, geom.NewLine(geom.NewPoint(40, 0), shared))

	tCtrl := [][2]float64{{90, 0}, {100, 10}, {110, -10}, {120, 0}}
	tSpline, err := geom.NewSpline(geom.NewPoint(90, 0), geom.NewPoint(100, 10), geom.NewPoint(110, -10), geom.NewPoint(120, 0))
	require.NoError(tb, err)
	tx, ty, err := geom.EvalCubicBSpline(tCtrl, 0.5)
	require.NoError(tb, err)
	curves = append(curves, tSpline, geom.NewLine(geom.NewPoint(tx, ty-20), geom.NewPoint(tx, ty)))

	return curves, []geom.ClosedCurve{sp}
}

// tangenciesFixture pins one disk (line tangent to a circle), two externally
// tangent disks (a merged vertex), one internal (containment) tangency, and
// one line tangent to an ellipse (a sampled path).
func tangenciesFixture(testing.TB) ([]geom.Curve, []geom.ClosedCurve) {
	closed := []geom.ClosedCurve{
		geom.NewCircle(geom.NewPoint(0, 10), 10),
		geom.NewCircle(geom.NewPoint(30, 0), 10),
		geom.NewCircle(geom.NewPoint(50, 0), 10),
		geom.NewCircle(geom.NewPoint(80, 0), 20),
		geom.NewCircle(geom.NewPoint(92, 0), 8),
		geom.NewEllipse(geom.NewPoint(150, 0), 20, 10, 0),
	}
	curves := []geom.Curve{
		geom.NewLine(geom.NewPoint(-10, 0), geom.NewPoint(10, 0)),
		geom.NewLine(geom.NewPoint(130, 10), geom.NewPoint(170, 10)),
	}
	return curves, closed
}

// collinearOverlapsFixture pins a degenerate coincident-edge condition (two
// lines sharing a stretch of one carrier, and two identical circles) plus a
// resolved case (an arc lying on its own hub circle).
func collinearOverlapsFixture(testing.TB) ([]geom.Curve, []geom.ClosedCurve) {
	curves := []geom.Curve{
		geom.NewLine(geom.NewPoint(0, -100), geom.NewPoint(20, -100)),
		geom.NewLine(geom.NewPoint(10, -100), geom.NewPoint(30, -100)),
		geom.NewArc(geom.NewPoint(60, -100), geom.NewPoint(70, -100), geom.NewPoint(60, -90)),
	}
	closed := []geom.ClosedCurve{
		geom.NewCircle(geom.NewPoint(60, -100), 10),
		geom.NewCircle(geom.NewPoint(100, -100), 10),
		geom.NewCircle(geom.NewPoint(100, -100), 10),
	}
	return curves, closed
}

// selfCrossingsFixture pins three self-crossing boundaries: a 4-line bowtie,
// an open spline with a loop, and a figure-8 closed spline.
func selfCrossingsFixture(tb testing.TB) ([]geom.Curve, []geom.ClosedCurve) {
	a, b, c, d := geom.NewPoint(0, 0), geom.NewPoint(20, 20), geom.NewPoint(20, 0), geom.NewPoint(0, 20)
	curves := []geom.Curve{geom.NewLine(a, b), geom.NewLine(b, c), geom.NewLine(c, d), geom.NewLine(d, a)}

	loopSpline, err := geom.NewSpline(
		geom.NewPoint(40, 0), geom.NewPoint(60, 20), geom.NewPoint(40, 20), geom.NewPoint(60, 0),
	)
	require.NoError(tb, err)
	curves = append(curves, loopSpline)

	fig8, err := geom.NewClosedSpline(
		geom.NewPoint(90, 0), geom.NewPoint(100, 10), geom.NewPoint(90, 20), geom.NewPoint(80, 10),
		geom.NewPoint(90, 0), geom.NewPoint(100, -10), geom.NewPoint(90, -20), geom.NewPoint(80, -10),
	)
	require.NoError(tb, err)

	return curves, []geom.ClosedCurve{fig8}
}

// nonTransitiveMergeFixture holds three lines whose free endpoints B, A, C sit
// at 0.8*nearMissGap, 0.8*nearMissGap apart in a chain (B-A, A-C), so B and C
// are 1.6*nearMissGap apart — outside a direct pairwise merge but chained
// through A. B is on the lowest-index segment, so it is inserted first.
func nonTransitiveMergeFixture(testing.TB) ([]geom.Curve, []geom.ClosedCurve) {
	step := 0.8 * nearMissGap
	bPt := geom.NewPoint(20, -180)
	aPt := geom.NewPoint(20+step, -180)
	cPt := geom.NewPoint(20+2*step, -180)
	curves := []geom.Curve{
		geom.NewLine(geom.NewPoint(0, -180), bPt),
		geom.NewLine(geom.NewPoint(40, -180), aPt),
		geom.NewLine(geom.NewPoint(60, -180), cPt),
	}
	return curves, nil
}

// existingRegressionsFixture bundles, side by side, the square,
// crossing-lines-no-region, overlapping-rectangles, nested-square-hole (also
// standing in for "plate + hole") and bowtie fixtures from arrange_test.go,
// for continuity with that suite.
func existingRegressionsFixture(testing.TB) ([]geom.Curve, []geom.ClosedCurve) {
	rect := func(x0, y0, x1, y1 float64) []geom.Curve {
		a := geom.NewPoint(x0, y0)
		b := geom.NewPoint(x1, y0)
		c := geom.NewPoint(x1, y1)
		d := geom.NewPoint(x0, y1)
		return []geom.Curve{geom.NewLine(a, b), geom.NewLine(b, c), geom.NewLine(c, d), geom.NewLine(d, a)}
	}

	var curves []geom.Curve
	curves = append(curves, square(0, 0, 10)...) // TestRegionsSquare

	l1 := geom.NewLine(geom.NewPoint(45, -5), geom.NewPoint(55, 5))
	l2 := geom.NewLine(geom.NewPoint(45, 5), geom.NewPoint(55, -5))
	curves = append(curves, l1, l2) // TestRegionsCrossingLinesNoRegion

	curves = append(curves, rect(70, 0, 76, 4)...)
	curves = append(curves, rect(73, 2, 79, 6)...) // TestRegionsOverlappingRectangles

	curves = append(curves, square(100, 0, 10)...) // TestRegionsNestedSquareHole / plate+hole
	curves = append(curves, square(103, 3, 4)...)

	a, b, c, d := geom.NewPoint(130, 0), geom.NewPoint(134, 4), geom.NewPoint(134, 0), geom.NewPoint(130, 4)
	curves = append(curves, geom.NewLine(a, b), geom.NewLine(b, c), geom.NewLine(c, d), geom.NewLine(d, a)) // bowtie

	return curves, nil
}

func arrangeFixtures() []arrangeFixture {
	return []arrangeFixture{
		{"disjointCurves", disjointCurvesFixture},
		{"nearMissOutside", nearMissOutsideFixture},
		{"nearMissInside", nearMissInsideFixture},
		{"endpointWelds", endpointWeldsFixture},
		{"tangencies", tangenciesFixture},
		{"collinearOverlaps", collinearOverlapsFixture},
		{"selfCrossings", selfCrossingsFixture},
		{"nonTransitiveMerge", nonTransitiveMergeFixture},
		{"existingRegressions", existingRegressionsFixture},
	}
}

// --- TestGoldenArrangement ---------------------------------------------

type edgeSnapshot struct {
	SourceIndex int
	Whole       bool
	Reversed    bool
	TStart      float64
	TEnd        float64
	TExact      bool
	PolylineLen int
}

type regionSnapshot struct {
	Area             float64
	Degenerate       bool
	SelfIntersecting bool
	Outer            []edgeSnapshot
	Holes            [][]edgeSnapshot
}

type arrangementSnapshot struct {
	Degenerate        bool
	Degeneracies      [][2]float64
	SelfIntersections [][2]float64
	Regions           []regionSnapshot
}

func snapshotEdges(edges []geom.BoundaryEdge) []edgeSnapshot {
	out := make([]edgeSnapshot, len(edges))
	for i, e := range edges {
		out[i] = edgeSnapshot{
			SourceIndex: e.SourceIndex,
			Whole:       e.Whole,
			Reversed:    e.Reversed,
			TStart:      e.TStart,
			TEnd:        e.TEnd,
			TExact:      e.TExact,
			PolylineLen: len(e.Polyline),
		}
	}
	return out
}

func snapshotArrangement(arr *geom.Arrangement) arrangementSnapshot {
	out := arrangementSnapshot{
		Degenerate:        arr.Degenerate,
		Degeneracies:      arr.Degeneracies,
		SelfIntersections: arr.SelfIntersections,
	}
	for _, r := range arr.Regions {
		rs := regionSnapshot{
			Area:             r.Area,
			Degenerate:       r.Degenerate,
			SelfIntersecting: r.SelfIntersecting,
			Outer:            snapshotEdges(r.Outer),
		}
		for _, h := range r.Holes {
			rs.Holes = append(rs.Holes, snapshotEdges(h))
		}
		out.Regions = append(out.Regions, rs)
	}
	return out
}

func requirePointsClose(t *testing.T, want, got [][2]float64, label string) {
	t.Helper()
	require.Equal(t, len(want), len(got), "%s count", label)
	for i := range want {
		require.InDelta(t, want[i][0], got[i][0], 1e-12, "%s %d x", label, i)
		require.InDelta(t, want[i][1], got[i][1], 1e-12, "%s %d y", label, i)
	}
}

func requireEdgesEqual(t *testing.T, want, got []edgeSnapshot, region int, label string) {
	t.Helper()
	require.Equal(t, len(want), len(got), "region %d %s edge count", region, label)
	for k := range want {
		require.Equal(t, want[k].SourceIndex, got[k].SourceIndex, "region %d %s edge %d SourceIndex", region, label, k)
		require.Equal(t, want[k].Whole, got[k].Whole, "region %d %s edge %d Whole", region, label, k)
		require.Equal(t, want[k].Reversed, got[k].Reversed, "region %d %s edge %d Reversed", region, label, k)
		require.Equal(t, want[k].TExact, got[k].TExact, "region %d %s edge %d TExact", region, label, k)
		require.Equal(t, want[k].PolylineLen, got[k].PolylineLen, "region %d %s edge %d PolylineLen", region, label, k)
		require.InDelta(t, want[k].TStart, got[k].TStart, 1e-12, "region %d %s edge %d TStart", region, label, k)
		require.InDelta(t, want[k].TEnd, got[k].TEnd, 1e-12, "region %d %s edge %d TEnd", region, label, k)
	}
}

// TestGoldenArrangement pins geom.Regions' output for every section-4.2
// fixture at three settings: default, WithSegmentsPerTurn(16) and
// WithVertexMerge(1e-3).
func TestGoldenArrangement(t *testing.T) {
	settings := []struct {
		name string
		opts []geom.Option
	}{
		{"default", nil},
		{"segsPerTurn16", []geom.Option{geom.WithSegmentsPerTurn(16)}},
		{"vertexMerge1e-3", []geom.Option{geom.WithVertexMerge(1e-3)}},
	}
	for _, f := range arrangeFixtures() {
		for _, setting := range settings {
			t.Run(f.name+"/"+setting.name, func(t *testing.T) {
				curves, closed := f.build(t)
				arr := geom.Regions(curves, closed, setting.opts...)
				got := snapshotArrangement(arr)
				name := "arrangement_" + f.name + "_" + setting.name + ".json"
				want := loadOrStoreJSON(t, name, got)

				require.Equal(t, want.Degenerate, got.Degenerate)
				requirePointsClose(t, want.Degeneracies, got.Degeneracies, "Degeneracies")
				requirePointsClose(t, want.SelfIntersections, got.SelfIntersections, "SelfIntersections")
				require.Equal(t, len(want.Regions), len(got.Regions), "region count")
				for i := range want.Regions {
					require.Equal(t, want.Regions[i].Degenerate, got.Regions[i].Degenerate, "region %d Degenerate", i)
					require.Equal(t, want.Regions[i].SelfIntersecting, got.Regions[i].SelfIntersecting, "region %d SelfIntersecting", i)
					require.InDelta(t, want.Regions[i].Area, got.Regions[i].Area, 1e-12, "region %d Area", i)
					requireEdgesEqual(t, want.Regions[i].Outer, got.Regions[i].Outer, i, "Outer")
					require.Equal(t, len(want.Regions[i].Holes), len(got.Regions[i].Holes), "region %d hole count", i)
					for j := range want.Regions[i].Holes {
						requireEdgesEqual(t, want.Regions[i].Holes[j], got.Regions[i].Holes[j], i, "Holes")
					}
				}
			})
		}
	}
}

// --- TestGoldenCurves ----------------------------------------------------

// conicCurveFixture returns a conic arc fixture used only by TestGoldenCurves.
func conicCurveFixture() *geom.Conic {
	return geom.NewConic(geom.NewPoint(0, 0), geom.NewPoint(10, 20), geom.NewPoint(20, 0), 0.5)
}

// polylineCurve is satisfied by every curve/primitive type TestGoldenCurves
// samples.
type polylineCurve interface {
	Polyline(segments int) [][2]float64
}

// TestGoldenCurves pins Polyline(n) for n in {2, 7, 128} across the spline,
// closed spline, fit spline, NURBS, ellipse, elliptical arc and conic
// fixtures.
func TestGoldenCurves(t *testing.T) {
	fixtures := []struct {
		name string
		c    polylineCurve
	}{
		{"spline", benchSplineFixture(t)},
		{"closedSpline", benchClosedSplineFixture(t)},
		{"fitSpline", benchFitSplineFixture(t)},
		{"nurbs", benchNURBSFixture()},
		{"ellipse", benchEllipseFixture()},
		{"ellipticalArc", benchEllipticalArcFixture()},
		{"conic", conicCurveFixture()},
	}
	for _, f := range fixtures {
		for _, n := range []int{2, 7, 128} {
			t.Run(f.name+"/n="+strconv.Itoa(n), func(t *testing.T) {
				got := f.c.Polyline(n)
				name := "curve_" + f.name + "_" + strconv.Itoa(n) + ".json"
				want := loadOrStoreJSON(t, name, got)
				require.Equal(t, len(want), len(got), "polyline length")
				for i := range want {
					requireRelClose(t, want[i][0], got[i][0], 1e-12, "point %d x", i)
					requireRelClose(t, want[i][1], got[i][1], 1e-12, "point %d y", i)
				}
			})
		}
	}
}
