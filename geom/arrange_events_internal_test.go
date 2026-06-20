package geom

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// White-box tests for the analytic crossing-event kernel. It is an internal
// arrangement helper (no exported surface yet), and its correctness is the
// foundation the analytic arrangement is built on, so it is verified directly
// here before being wired into Regions.

func lineSrc(ax, ay, bx, by float64) *source {
	return &source{kind: srcLine, ax: ax, ay: ay, bx: bx, by: by}
}
func circleSrc(cx, cy, r float64) *source {
	return &source{kind: srcCircle, cx: cx, cy: cy, r: r, sweep: 2 * math.Pi}
}
func arcSrc(cx, cy, r, phi0, sweep float64) *source {
	return &source{kind: srcArc, cx: cx, cy: cy, r: r, phi0: phi0, sweep: sweep}
}

func TestAnalyticLineLine(t *testing.T) {
	// An X: two diagonals crossing at the origin, transverse.
	ev, amb, ok := analyticEvents(lineSrc(-1, -1, 1, 1), lineSrc(-1, 1, 1, -1), 4)
	require.True(t, ok)
	require.False(t, amb)
	require.Len(t, ev, 1)
	require.Equal(t, evCross, ev[0].kind)
	require.InDelta(t, 0, ev[0].x, 1e-12)
	require.InDelta(t, 0, ev[0].y, 1e-12)
	require.InDelta(t, 0.5, ev[0].ti, 1e-12)
	require.InDelta(t, 0.5, ev[0].tj, 1e-12)
}

func TestAnalyticLineLineParallelDisjoint(t *testing.T) {
	ev, _, ok := analyticEvents(lineSrc(0, 0, 10, 0), lineSrc(0, 5, 10, 5), 10)
	require.True(t, ok)
	require.Empty(t, ev, "parallel disjoint lines never cross")
}

func TestAnalyticLineLineCollinearOverlap(t *testing.T) {
	ev, _, ok := analyticEvents(lineSrc(0, 0, 10, 0), lineSrc(5, 0, 15, 0), 10)
	require.True(t, ok)
	require.Len(t, ev, 1)
	require.Equal(t, evOverlap, ev[0].kind)
}

func TestAnalyticLineCircleSecant(t *testing.T) {
	// Horizontal line y=0 through a unit circle at origin: crosses at (±1, 0).
	ev, amb, ok := analyticEvents(lineSrc(-3, 0, 3, 0), circleSrc(0, 0, 1), 6)
	require.True(t, ok)
	require.False(t, amb)
	require.Len(t, ev, 2)
	xs := []float64{ev[0].x, ev[1].x}
	require.Contains(t, []bool{true}, math.Abs(xs[0])-1 < 1e-9 || math.Abs(xs[1])-1 < 1e-9)
	for _, e := range ev {
		require.Equal(t, evCross, e.kind)
		require.InDelta(t, 0, e.y, 1e-9)
		require.InDelta(t, 1, math.Abs(e.x), 1e-9)
	}
}

func TestAnalyticLineCircleTangent(t *testing.T) {
	// Line y=1 tangent to the unit circle at (0,1).
	ev, amb, ok := analyticEvents(lineSrc(-3, 1, 3, 1), circleSrc(0, 0, 1), 6)
	require.True(t, ok)
	require.False(t, amb)
	require.Len(t, ev, 1)
	require.Equal(t, evTangent, ev[0].kind, "a tangent line is a contact, not a crossing")
	require.InDelta(t, 0, ev[0].x, 1e-7)
	require.InDelta(t, 1, ev[0].y, 1e-7)
}

func TestAnalyticLineCircleMiss(t *testing.T) {
	ev, _, ok := analyticEvents(lineSrc(-3, 2, 3, 2), circleSrc(0, 0, 1), 6)
	require.True(t, ok)
	require.Empty(t, ev, "a line clear of the circle has no contact")
}

func TestAnalyticCircleCircleExternalTangent(t *testing.T) {
	// Two unit circles touching externally at (1,0).
	ev, amb, ok := analyticEvents(circleSrc(0, 0, 1), circleSrc(2, 0, 1), 4)
	require.True(t, ok)
	require.False(t, amb)
	require.Len(t, ev, 1)
	require.Equal(t, evTangent, ev[0].kind, "externally tangent circles touch, not cross")
	require.InDelta(t, 1, ev[0].x, 1e-7)
	require.InDelta(t, 0, ev[0].y, 1e-7)
}

func TestAnalyticCircleCircleSecant(t *testing.T) {
	// Two unit circles whose centers are 1 apart: they cross at two points.
	ev, amb, ok := analyticEvents(circleSrc(0, 0, 1), circleSrc(1, 0, 1), 4)
	require.True(t, ok)
	require.False(t, amb)
	require.Len(t, ev, 2)
	for _, e := range ev {
		require.Equal(t, evCross, e.kind)
		require.InDelta(t, 0.5, e.x, 1e-9, "crossings lie on the radical line x=0.5")
	}
}

func TestAnalyticCircleCircleSeparate(t *testing.T) {
	ev, _, ok := analyticEvents(circleSrc(0, 0, 1), circleSrc(5, 0, 1), 6)
	require.True(t, ok)
	require.Empty(t, ev)
}

func TestAnalyticCircleCircleCoincident(t *testing.T) {
	ev, _, ok := analyticEvents(circleSrc(0, 0, 2), circleSrc(0, 0, 2), 4)
	require.True(t, ok)
	require.Len(t, ev, 1)
	require.Equal(t, evOverlap, ev[0].kind)
}

func TestAnalyticArcSweepClips(t *testing.T) {
	// The line y=0 meets the full circle at (±1,0), but a top semicircle arc
	// (phi0=0, sweep=π, from (1,0) CCW to (-1,0)) includes both endpoints — clip
	// to a quarter arc (0..π/2) and only (1,0) survives.
	quarter := arcSrc(0, 0, 1, 0, math.Pi/2) // (1,0) to (0,1)
	ev, _, ok := analyticEvents(lineSrc(-3, 0, 3, 0), quarter, 6)
	require.True(t, ok)
	require.Len(t, ev, 1, "only the (1,0) crossing is on the quarter arc")
	require.InDelta(t, 1, ev[0].x, 1e-9)
	require.InDelta(t, 0, ev[0].y, 1e-9)
}

func TestAnalyticUnsupportedFallsBack(t *testing.T) {
	ell := &source{kind: srcEllipse, cx: 0, cy: 0, rx: 3, ry: 2}
	_, _, ok := analyticEvents(lineSrc(-3, 0, 3, 0), ell, 6)
	require.False(t, ok, "ellipse pairs have no closed form here → caller keeps the sampled fallback")
}

func TestAnalyticCWArcStartContact(t *testing.T) {
	// A CW quarter arc from (1,0) to (0,-1) (phi0=0, sweep=-π/2): its START endpoint
	// (1,0) must read t=0 and survive the sweep clip — the regression Codex caught.
	cw := arcSrc(0, 0, 1, 0, -math.Pi/2)
	ev, _, ok := analyticEvents(lineSrc(-3, 0, 3, 0), cw, 6)
	require.True(t, ok)
	require.Len(t, ev, 1, "only the (1,0) arc-start contact is on the CW arc")
	require.InDelta(t, 1, ev[0].x, 1e-9)
	require.InDelta(t, 0, ev[0].y, 1e-9)
}

func TestAnalyticLineSegmentClipped(t *testing.T) {
	// Carrier lines cross at (2,0), which is OFF segment a (0,0)-(1,0): no contact.
	ev, _, ok := analyticEvents(lineSrc(0, 0, 1, 0), lineSrc(2, -1, 2, 1), 4)
	require.True(t, ok)
	require.Empty(t, ev, "a carrier crossing off the segment is not a contact")
}

func TestAnalyticCollinearDisjointNoEvent(t *testing.T) {
	// Same carrier, disjoint extents: no overlap event.
	ev, _, ok := analyticEvents(lineSrc(0, 0, 1, 0), lineSrc(5, 0, 6, 0), 6)
	require.True(t, ok)
	require.Empty(t, ev, "collinear but disjoint segments do not overlap")
}

func TestAnalyticLineCircleNearMissNotTangent(t *testing.T) {
	// A line just outside the tangent band is NOT certified tangent — it is ambiguous
	// (the contact is unresolved), never a fake tangent contact off the circle.
	ev, amb, ok := analyticEvents(lineSrc(-3, 1+3e-6, 3, 1+3e-6), circleSrc(0, 0, 1), 6)
	require.True(t, ok)
	require.True(t, amb, "a near-tangent gap inside the band is ambiguous")
	require.Empty(t, ev)
	// A clear miss is clean, not ambiguous.
	ev2, amb2, _ := analyticEvents(lineSrc(-3, 2, 3, 2), circleSrc(0, 0, 1), 6)
	require.False(t, amb2)
	require.Empty(t, ev2)
}

func TestAnalyticNearCoincidentCirclesAmbiguous(t *testing.T) {
	// Same center, radii within the band but not certify-equal: ambiguous, not a
	// confident overlap.
	ev, amb, ok := analyticEvents(circleSrc(0, 0, 2), circleSrc(0, 0, 2+3e-6), 4)
	require.True(t, ok)
	require.True(t, amb)
	require.Empty(t, ev)
	// Clearly different concentric radii are a clean miss (an annulus), not ambiguous.
	ev2, amb2, _ := analyticEvents(circleSrc(0, 0, 2), circleSrc(0, 0, 3), 6)
	require.False(t, amb2)
	require.Empty(t, ev2)
}

func TestAnalyticInternalTangent(t *testing.T) {
	// A unit circle internally tangent inside a radius-3 circle: centers 2 apart,
	// contact at (3,0) on the far side.
	ev, amb, ok := analyticEvents(circleSrc(0, 0, 3), circleSrc(2, 0, 1), 6)
	require.True(t, ok)
	require.False(t, amb)
	require.Len(t, ev, 1)
	require.Equal(t, evTangent, ev[0].kind)
	require.InDelta(t, 3, ev[0].x, 1e-7)
	require.InDelta(t, 0, ev[0].y, 1e-7)
}

func TestAnalyticCollinearEndpointTouchNoOverlap(t *testing.T) {
	// Two collinear segments sharing only the endpoint (1,0) form a join (a corner),
	// not a degenerate overlap.
	ev, amb, ok := analyticEvents(lineSrc(0, 0, 1, 0), lineSrc(1, 0, 2, 0), 4)
	require.True(t, ok)
	require.False(t, amb)
	require.Empty(t, ev, "an endpoint-only touch is a join, not an overlap")

	// A genuine positive-length overlap is still flagged.
	ov, _, _ := analyticEvents(lineSrc(0, 0, 2, 0), lineSrc(1, 0, 3, 0), 4)
	require.Len(t, ov, 1)
	require.Equal(t, evOverlap, ov[0].kind)
}
