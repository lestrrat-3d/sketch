package geom

// Orientation is the winding sense of a closed boundary walk.
type Orientation int

const (
	// CW is a clockwise walk (negative signed area).
	CW Orientation = -1
	// CCW is a counter-clockwise walk (positive signed area).
	CCW Orientation = 1
)

// BoundaryEdge is one edge of a region boundary: a maximal run that came from a
// single input curve, walked between two arrangement vertices. It back-
// references the originating curve by its index in the Regions input slice, so
// a caller can map the edge (or fragment) to its own entity.
type BoundaryEdge struct {
	// SourceIndex is the position of the originating curve in the Regions
	// input — its index in curves, or len(curves)+k for the k-th closed curve.
	SourceIndex int
	// Whole is true when this edge spans the entire source curve — i.e. its
	// [TStart, TEnd] covers the curve's full domain; false when it is a fragment
	// covering a strict sub-range. It is derived from the edge's own surviving
	// range, so a curve whose only contact was pruned away (or run straight
	// through) reads Whole again, and a closed curve cut once — one edge leaving
	// the contact and returning to it — is Whole, as it should be.
	Whole bool
	// Reversed is true when the boundary walks the source curve against its
	// natural Start→End (or CCW, for a closed curve) direction.
	Reversed bool
	// Polyline is the densified sample of this edge in walk order, the first
	// point the edge's start vertex and the last its end vertex. A line edge is
	// two points; an arc/closed-curve fragment is more.
	Polyline [][2]float64
	// TStart and TEnd are the fragment's parameter range on the source curve, in
	// the curve's NATURAL parameter direction — so TStart < TEnd always, and
	// Reversed (not the order of these two) is what says the walk runs backwards.
	// A whole edge spans the curve's full domain. The range never wraps: a
	// fragment of a closed curve straddling the seam is emitted as two edges.
	//
	// The parameter is the arrangement's normalized t in [0,1], which is NOT the
	// curve's own angle/knot parameter:
	//
	//	Line              lerp Start→End
	//	Arc               angle = StartAngle + t·Sweep
	//	Circle            angle = 2π·t, from the absolute +x axis (a circle has no start)
	//	Ellipse           eccentric angle 2π·t in the rotated local frame
	//	EllipticalArc     eccentric angle = StartParam + t·Sweep
	//	Conic             the rational quadratic Bézier parameter
	//	Spline/FitSpline  the curve's own t (interior knots at j/(n-3))
	//	ClosedSpline      the periodic parameter, span boundaries at i/n
	//	NURBS             normalized: knot u = lo + (hi-lo)·t over Domain()
	TStart, TEnd float64
	// TExact reports whether TStart and TEnd are the TRUE source parameters.
	//
	// It is true when both bounds come from the closed-form kernel (an analytic
	// crossing), a sample vertex, or the curve's own endpoint — and false when
	// either bound comes from a SAMPLED crossing, whose parameter is interpolated
	// between two sample params and whose point lies off the true curve by the
	// chord sagitta. A false value does not mean the topology is wrong; it means
	// the parameter converges with sampling density rather than being exact.
	//
	// Only line-vs-{line,circle,arc} crossings are analytic today. Every crossing
	// involving an ellipse, elliptical arc, conic, spline, closed spline, fit
	// spline or NURBS — even against a plain line — is sampled, as is every
	// curve/curve crossing. So a partial fragment of a free-form curve reports
	// TExact = false. A consumer that must be exact (e.g. one recording the region
	// structurally, or emitting CAD code from it) MUST check this flag and reject
	// rather than trust the range.
	TExact bool
}

// Region is a minimal bounded area extracted from the arrangement: an outer
// boundary walked counter-clockwise, zero or more holes (inner boundaries,
// walked clockwise), the net area (outer minus holes), and whether the region
// derives from a self-intersecting input boundary.
type Region struct {
	Outer            []BoundaryEdge
	Holes            [][]BoundaryEdge
	Area             float64 // net area (outer minus holes); >= 0 for a clean region
	SelfIntersecting bool    // an input boundary feeding this region crosses itself
}

// Arrangement is the result of Regions: the bounded regions plus arrangement-
// wide soundness signals.
type Arrangement struct {
	Regions []*Region
	// SelfIntersections lists the points where a single closed input boundary
	// (a simple loop whose curves meet only at shared endpoints) crosses or
	// touches itself — distinct from a legitimate crossing between two separate
	// boundaries, which subdivides rather than invalidates.
	SelfIntersections [][2]float64
	// Degenerate is set when the arrangement could not be resolved soundly:
	// collinear overlapping curves (duplicated/coincident edges) or a crossing
	// too close to a vertex or another crossing to place reliably given the
	// sampling. The region set is then not trustworthy and a caller (the oracle)
	// must treat the profiles as unverifiable rather than valid.
	Degenerate bool
	// Degeneracies lists representative points of the degenerate conditions.
	Degeneracies [][2]float64
}

// ClosedCurve is a closed primitive (circle, ellipse, or closed spline) admitted
// to the arrangement: it is sampled to a closed polyline. *Circle, *Ellipse and
// *ClosedSpline satisfy it via their Polyline samplers. The unexported marker
// seals the interface so an open *Spline (which also has a Polyline method) does
// not accidentally satisfy it.
type ClosedCurve interface {
	Polyline(segments int) [][2]float64
	closedCurve()
}

func (c *Circle) closedCurve()  {}
func (e *Ellipse) closedCurve() {}
