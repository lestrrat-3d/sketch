package geom

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// This is one of the package's few internal test files (the repo otherwise tests
// only the exported API from external xxx_test packages): candidatePairs, segParams,
// collinearOverlap and forEachMergedEnd are all unexported, and the property under
// test — candidatePairs returns a SUPERSET of the pairs those three predicates would
// act on — can only be stated against them directly.
//
// See .claude/docs/profiles-geom.md, "The broad-phase reach", for the proof this test
// exercises: boxes expanded by a.merge + broadPhaseRel*(chord length) admit every pair
// segParams, collinearOverlap or forEachMergedEnd can fire on.

// scenePairArranger builds an arranger the same way Regions does, up to (but not
// including) intersect — densify has already filled a.segs and a.merge, which is
// everything candidatePairs and the three predicates need.
func scenePairArranger(curves []Curve, closed []ClosedCurve, vertexMerge float64) *arranger {
	a := newArranger(curves, closed, arrangeConfig{vertexMerge: vertexMerge})
	a.densify()
	return a
}

// bruteForcePairs recomputes candidatePairs by a naive O(n^2) box-overlap scan, with
// no sweep or active-list pruning, as a reference for the sweep implementation.
func bruteForcePairs(a *arranger) [][]int {
	n := len(a.segs)
	cand := make([][]int, n)
	boxes := make([]segBox, n)
	for i := range boxes {
		boxes[i] = a.segBoxOf(i)
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if boxesOverlap(boxes[i], boxes[j]) {
				cand[i] = append(cand[i], j)
			}
		}
	}
	return cand
}

// requireCandidateShape asserts the structural contract candidatePairs must hold
// regardless of scene content: one entry per segment, and each cand[i] strictly
// ascending with every j > i (load-bearing for splitFragments' cut-order dedup, per
// .claude/docs/profiles-geom.md).
func requireCandidateShape(t *testing.T, n int, cand [][]int) {
	t.Helper()
	require.Len(t, cand, n)
	for i, js := range cand {
		prev := i
		for _, j := range js {
			require.Greater(t, j, prev, "cand[%d] must be strictly ascending with j > i", i)
			prev = j
		}
	}
}

// requireSuperset asserts every pair the three deciding predicates would act on is a
// candidate: candidatePairs may only ever ADD pairs beyond what they accept, never
// drop one.
func requireSuperset(t *testing.T, a *arranger, cand [][]int) {
	t.Helper()
	n := len(a.segs)
	member := make([]map[int]struct{}, n)
	for i, js := range cand {
		member[i] = make(map[int]struct{}, len(js))
		for _, j := range js {
			member[i][j] = struct{}{}
		}
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			si, sj := &a.segs[i], &a.segs[j]
			fires := false
			if _, ok := segParams(si, sj); ok {
				fires = true
			}
			if !fires {
				if _, _, over := collinearOverlap(si, sj); over {
					fires = true
				}
			}
			if !fires {
				a.forEachMergedEnd(si, sj, func(mergedEnd) { fires = true })
			}
			if !fires {
				continue
			}
			_, isCand := member[i][j]
			require.True(t, isCand, "pair (%d,%d) fires a deciding predicate but is not a candidate", i, j)
		}
	}
}

// checkBroadPhase runs the full superset/shape/sweep-agreement check for one scene.
func checkBroadPhase(t *testing.T, a *arranger) {
	t.Helper()
	cand := a.candidatePairs()
	requireCandidateShape(t, len(a.segs), cand)
	requireSuperset(t, a, cand)
	require.Equal(t, bruteForcePairs(a), cand, "sweep must agree with a naive box-overlap scan")
}

// --- fixtures ---------------------------------------------------------------

func bpLine(x1, y1, x2, y2 float64) *Line {
	return NewLine(NewPoint(x1, y1), NewPoint(x2, y2))
}

func bpCircle(cx, cy, r float64) *Circle {
	return NewCircle(NewPoint(cx, cy), r)
}

func bpSpline(t *testing.T, pts [][2]float64) *Spline {
	t.Helper()
	ctrl := make([]*Point, len(pts))
	for i, p := range pts {
		ctrl[i] = NewPoint(p[0], p[1])
	}
	sp, err := NewSpline(ctrl...)
	require.NoError(t, err)
	return sp
}

// TestBroadPhaseIsSuperset is the section 4.3 proof: over representative fixtures
// from every §4.2 category and 200 seeded random scenes, candidatePairs' output is a
// superset of what segParams/collinearOverlap/forEachMergedEnd would fire on, its
// per-index lists are strictly ascending, and the sweep agrees with a naive
// box-overlap scan.
func TestBroadPhaseIsSuperset(t *testing.T) {
	t.Run("disjoint curves", func(t *testing.T) {
		var closed []ClosedCurve
		n := 4
		for i := 0; i < n; i++ {
			for j := 0; j < 3; j++ {
				closed = append(closed, bpCircle(float64(i)*30, float64(j)*30, 10))
			}
		}
		checkBroadPhase(t, scenePairArranger(nil, closed, 0))
	})

	t.Run("near miss outside the window", func(t *testing.T) {
		closed := []ClosedCurve{bpCircle(0, 0, 10), bpCircle(20.001, 0, 10)}
		curves := []Curve{bpLine(-20, 0, -0.001, 0)}
		checkBroadPhase(t, scenePairArranger(curves, closed, 0))
	})

	t.Run("near miss inside the window", func(t *testing.T) {
		merge := 1e-3
		curves := []Curve{
			bpLine(0, 0, 10, 0),
			bpLine(10+0.5*merge, 0, 20, 5),
		}
		checkBroadPhase(t, scenePairArranger(curves, nil, merge))
	})

	t.Run("endpoint welds by shared pointer", func(t *testing.T) {
		a, b, c, d := NewPoint(0, 0), NewPoint(10, 0), NewPoint(10, 10), NewPoint(0, 10)
		curves := []Curve{NewLine(a, b), NewLine(b, c), NewLine(c, d), NewLine(d, a)}
		checkBroadPhase(t, scenePairArranger(curves, nil, 0))
	})

	t.Run("tangencies", func(t *testing.T) {
		curves := []Curve{bpLine(-20, 10, 20, 10)}
		closed := []ClosedCurve{bpCircle(0, 0, 10), bpCircle(0, 20, 10)}
		checkBroadPhase(t, scenePairArranger(curves, closed, 0))
	})

	t.Run("collinear overlaps", func(t *testing.T) {
		curves := []Curve{bpLine(0, 0, 10, 0), bpLine(5, 0, 15, 0)}
		checkBroadPhase(t, scenePairArranger(curves, nil, 0))
	})

	t.Run("self-crossings", func(t *testing.T) {
		a, b, c, d := NewPoint(0, 0), NewPoint(10, 10), NewPoint(10, 0), NewPoint(0, 10)
		curves := []Curve{NewLine(a, b), NewLine(b, c), NewLine(c, d), NewLine(d, a)}
		checkBroadPhase(t, scenePairArranger(curves, nil, 0))
	})

	t.Run("open spline with a loop", func(t *testing.T) {
		sp := bpSpline(t, [][2]float64{
			{0, 0}, {10, 10}, {20, -10}, {5, -15}, {5, 15}, {20, 10}, {30, 0},
		})
		checkBroadPhase(t, scenePairArranger([]Curve{sp}, nil, 0))
	})

	t.Run("non-transitive vertex merging", func(t *testing.T) {
		merge := 1e-3
		// B sits on the lowest-index segment, so it is inserted into the vertex
		// table first; A and C both sit within 0.8*merge of B but 1.6*merge apart
		// from each other.
		bx, by := 0.0, 0.0
		ax, ay := 0.8*merge, 0.0
		cx, cy := -0.8*merge, 0.0
		curves := []Curve{
			bpLine(bx, by, 10, 5),
			bpLine(ax, ay, -10, 5),
			bpLine(cx, cy, -10, -5),
		}
		checkBroadPhase(t, scenePairArranger(curves, nil, merge))
	})

	t.Run("existing regressions: overlapping rectangles", func(t *testing.T) {
		curves := []Curve{
			bpLine(0, 0, 10, 0), bpLine(10, 0, 10, 10), bpLine(10, 10, 0, 10), bpLine(0, 10, 0, 0),
			bpLine(5, 5, 15, 5), bpLine(15, 5, 15, 15), bpLine(15, 15, 5, 15), bpLine(5, 15, 5, 5),
		}
		checkBroadPhase(t, scenePairArranger(curves, nil, 0))
	})

	t.Run("200 seeded random scenes", func(t *testing.T) {
		for seed := int64(0); seed < 200; seed++ {
			rng := rand.New(rand.NewSource(seed))
			curves, closed := randomScene(t, rng)
			vertexMerge := 0.0
			if rng.Intn(3) != 0 {
				vertexMerge = math.Pow(10, -1-rng.Float64()*6)
			}
			a := scenePairArranger(curves, closed, vertexMerge)
			if len(a.segs) == 0 {
				continue
			}
			checkBroadPhase(t, a)
		}
	})
}

// sampledCrossingsExplainedBruteForce recomputes sampledCrossingsExplained's verdict
// with NO box reject at all — an exhaustive scan of every segment pair from the two
// sources — as the reference the box-guarded production function must agree with.
func sampledCrossingsExplainedBruteForce(a *arranger, i, j int, events []xEvent) bool {
	for _, ii := range a.sourceSegs[i] {
		for _, jj := range a.sourceSegs[j] {
			p, ok := a.segsCrossInteriorAt(ii, jj)
			if !ok {
				continue
			}
			var tol float64
			if isCurvedKind(a.sources[i].kind) {
				tol = math.Max(tol, a.segLen(ii))
			}
			if isCurvedKind(a.sources[j].kind) {
				tol = math.Max(tol, a.segLen(jj))
			}
			explained := false
			for _, e := range events {
				if math.Hypot(e.x-p.x, e.y-p.y) <= tol {
					explained = true
					break
				}
			}
			if !explained {
				return false
			}
		}
	}
	return true
}

// checkSampledCrossingsExplained asserts, for every analytic-kind source pair in a,
// that the box-guarded sampledCrossingsExplained agrees with the unguarded brute-force
// reference — the box reject in geom/arrange.go must never be able to flip this
// verdict, since (unlike candidatePairs) it feeds a returned answer rather than
// guarding a side effect. It returns how many of the reference verdicts it compared
// were FALSE, so a caller can require that the refusing direction — the only one a
// wrongly-skipped pair could corrupt — was actually reached.
//
// Each pair is checked twice: once on its real analytic events, and once on an EMPTY
// event list, which leaves every sampled crossing unexplained and so refuses any pair
// whose chords cross at all. The second pass is what exercises the refusing direction:
// on the fixtures here the real events explain every sampled crossing, so the first
// pass alone would only ever compare `true` against `true` and could not observe a
// pair the box reject wrongly skipped.
//
// This reuses analyticPrepass's own pair-discovery loop (analytic-kind sources,
// i < j, analyticEvents ok) rather than analyticPrepass itself, so it exercises
// sampledCrossingsExplained on exactly the pairs production code would call it on,
// without also running the cut/degeneracy machinery those pairs would otherwise
// trigger.
func checkSampledCrossingsExplained(t *testing.T, a *arranger) int {
	t.Helper()
	a.sourceSegs = make([][]int, len(a.sources))
	for i := range a.segs {
		a.sourceSegs[a.segs[i].src] = append(a.sourceSegs[a.segs[i].src], i)
	}
	refused := 0
	compare := func(i, j int, events []xEvent, mode string) {
		want := sampledCrossingsExplainedBruteForce(a, i, j, events)
		got := a.sampledCrossingsExplained(i, j, events)
		require.Equal(t, want, got, "pair (%d,%d) with %s events disagrees with the unguarded brute force", i, j, mode)
		if !want {
			refused++
		}
	}
	for i := 0; i < len(a.sources); i++ {
		si := &a.sources[i]
		if !analyticKind(si.kind) {
			continue
		}
		for j := i + 1; j < len(a.sources); j++ {
			sj := &a.sources[j]
			if !analyticKind(sj.kind) {
				continue
			}
			events, _, ok := analyticEvents(si, sj, a.scale)
			if !ok {
				continue
			}
			compare(i, j, events, "analytic")
			compare(i, j, nil, "no")
		}
	}
	return refused
}

// TestSampledCrossingsExplainedBoxRejectAgrees is the proof for the box reject added
// to sampledCrossingsExplained: over the same representative fixtures
// TestBroadPhaseIsSuperset uses (rebuilt fresh, since analyticPrepass-adjacent state
// is not shared across checks) plus 300 further seeded random scenes, the guarded
// answer never differs from the exhaustive one.
func TestSampledCrossingsExplainedBoxRejectAgrees(t *testing.T) {
	t.Run("disjoint curves", func(t *testing.T) {
		var closed []ClosedCurve
		n := 4
		for i := 0; i < n; i++ {
			for j := 0; j < 3; j++ {
				closed = append(closed, bpCircle(float64(i)*30, float64(j)*30, 10))
			}
		}
		checkSampledCrossingsExplained(t, scenePairArranger(nil, closed, 0))
	})

	t.Run("near miss outside the window", func(t *testing.T) {
		closed := []ClosedCurve{bpCircle(0, 0, 10), bpCircle(20.001, 0, 10)}
		curves := []Curve{bpLine(-20, 0, -0.001, 0)}
		checkSampledCrossingsExplained(t, scenePairArranger(curves, closed, 0))
	})

	t.Run("near miss inside the window", func(t *testing.T) {
		merge := 1e-3
		curves := []Curve{
			bpLine(0, 0, 10, 0),
			bpLine(10+0.5*merge, 0, 20, 5),
		}
		checkSampledCrossingsExplained(t, scenePairArranger(curves, nil, merge))
	})

	t.Run("tangencies", func(t *testing.T) {
		curves := []Curve{bpLine(-20, 10, 20, 10)}
		closed := []ClosedCurve{bpCircle(0, 0, 10), bpCircle(0, 20, 10)}
		checkSampledCrossingsExplained(t, scenePairArranger(curves, closed, 0))
	})

	t.Run("collinear overlaps", func(t *testing.T) {
		curves := []Curve{bpLine(0, 0, 10, 0), bpLine(5, 0, 15, 0)}
		checkSampledCrossingsExplained(t, scenePairArranger(curves, nil, 0))
	})

	t.Run("shallow circle/circle crossing beside a sample vertex", func(t *testing.T) {
		// A near-tangent, near-sample-vertex pair — the kind of geometry the
		// consistency gate's third part exists to police — so the box reject is
		// exercised on a pair right at its own decision boundary, not just on
		// well-separated geometry.
		closed := []ClosedCurve{bpCircle(0, 0, 5), bpCircle(9.999, 0, 5)}
		checkSampledCrossingsExplained(t, scenePairArranger(nil, closed, 0))
	})

	t.Run("many overlapping arcs and lines", func(t *testing.T) {
		var curves []Curve
		const n = 20
		const r = 20.0
		for i := 0; i < n; i++ {
			cx, cy := float64(i%5)*0.5, float64(i/5)*0.5
			a0 := float64(i) * 0.11
			a1 := a0 + 5.5
			start := NewPoint(cx+r*math.Cos(a0), cy+r*math.Sin(a0))
			end := NewPoint(cx+r*math.Cos(a1), cy+r*math.Sin(a1))
			curves = append(curves, NewArc(NewPoint(cx, cy), start, end))
		}
		for i := 0; i < 6; i++ {
			y := -25.0 + float64(i)*10
			curves = append(curves, bpLine(-30, y, 30, y+2))
		}
		checkSampledCrossingsExplained(t, scenePairArranger(curves, nil, 0))
	})

	t.Run("300 seeded random scenes", func(t *testing.T) {
		refused := 0
		for seed := int64(0); seed < 300; seed++ {
			rng := rand.New(rand.NewSource(seed))
			curves, closed := randomScene(t, rng)
			vertexMerge := 0.0
			if rng.Intn(3) != 0 {
				vertexMerge = math.Pow(10, -1-rng.Float64()*6)
			}
			a := scenePairArranger(curves, closed, vertexMerge)
			if len(a.segs) == 0 {
				continue
			}
			refused += checkSampledCrossingsExplained(t, a)
		}
		// The refusing direction is the only one a wrongly-skipped pair could
		// corrupt (a hidden crossing turns a `false` into a `true`), so the suite
		// is only a proof if it reached that direction on real geometry.
		require.Greater(t, refused, 0, "no scene reached the refusing verdict, so the box reject was never tested against it")
	})
}

// randomScene builds a small scene of random lines, circles, arcs and (sometimes) a
// spline, within a bounded region so near-misses and welds occur often enough to
// exercise every deciding predicate.
func randomScene(t *testing.T, rng *rand.Rand) ([]Curve, []ClosedCurve) {
	t.Helper()
	const extent = 20.0
	rp := func() *Point { return NewPoint(rng.Float64()*extent-extent/2, rng.Float64()*extent-extent/2) }

	var curves []Curve
	var closed []ClosedCurve
	nLines := rng.Intn(5)
	for i := 0; i < nLines; i++ {
		curves = append(curves, NewLine(rp(), rp()))
	}
	nCircles := rng.Intn(4)
	for i := 0; i < nCircles; i++ {
		closed = append(closed, NewCircle(rp(), 1+rng.Float64()*8))
	}
	nArcs := rng.Intn(3)
	for i := 0; i < nArcs; i++ {
		c := rp()
		r := 1 + rng.Float64()*8
		a0 := rng.Float64() * 2 * math.Pi
		a1 := a0 + 0.2 + rng.Float64()*5
		start := NewPoint(c.X+r*math.Cos(a0), c.Y+r*math.Sin(a0))
		end := NewPoint(c.X+r*math.Cos(a1), c.Y+r*math.Sin(a1))
		curves = append(curves, NewArc(c, start, end))
	}
	if rng.Intn(2) == 0 {
		n := 4 + rng.Intn(4)
		pts := make([]*Point, n)
		for i := range pts {
			pts[i] = rp()
		}
		if sp, err := NewSpline(pts...); err == nil {
			curves = append(curves, sp)
		}
	}
	return curves, closed
}
