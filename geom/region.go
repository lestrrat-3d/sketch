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
	// Whole is true when this edge spans the entire source curve; false when it is
	// a fragment covering a strict sub-range.
	//
	// It is decided by each bound's PROVENANCE, not by comparing [TStart, TEnd]
	// against [0,1]: a bound is either the curve's own domain end (an open curve's
	// endpoint, or a closed curve's seam) or a cut/weld, and the edge is Whole iff
	// BOTH of its bounds are the curve's own ends. No float compare could make that
	// call — it cannot tell a bound that IS the curve's endpoint from a crossing
	// that landed 1e-10 away from it, and answering "whole" there is the unsafe way
	// to be wrong.
	//
	// It is read off the surviving edge after pruning and coalescing, so a curve
	// whose only contact was pruned away (or run straight through) reads Whole
	// again, and a closed curve cut once — one edge leaving the contact and coming
	// back to it, bounded by its own seam — is Whole. The single conservative corner
	// is a closed curve whose one cut lands ON the seam: both bounds are then cuts,
	// so it reads as a fragment spanning [0,1]. That errs toward reporting a
	// fragment, never toward a false Whole.
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
	// TExact reports whether TStart and TEnd are the TRUE source parameters. Its
	// precise, checkable meaning is: evaluating the source curve at the reported
	// parameter reproduces the emitted Polyline endpoint to machine precision, at BOTH
	// bounds. A false value does not mean the topology is wrong; it means a parameter
	// converges with sampling density (or was pinned off the curve) rather than being
	// exact. A consumer that must be exact (recording the region structurally, or
	// emitting CAD from it) MUST check this and reject rather than trust the range.
	//
	// A CUT bound is exact only when the closed-form kernel placed it. That kernel runs
	// only when BOTH curves of a pair are a Line, Circle or Arc — a rule about the PAIR,
	// not "a line was involved" and not the contact being a tangency. So every contact
	// involving an Ellipse, EllipticalArc, Conic, Spline, ClosedSpline, FitSpline or
	// NURBS is sampled (INCLUDING a plain line crossing one, and a plain line TANGENT to
	// one), and a FRAGMENT bounded by such a contact reports TExact = false.
	//
	// A crossing between two CURVED sources (circle/arc against circle/arc) additionally
	// has to be certified against the sampling before it is taken as exact: both
	// polylines there are chord approximations, so the arrangement checks that splicing
	// the exact crossing points into them leaves the sampled topology unchanged. When it
	// cannot — at a sampling too coarse to resolve the crossing — the pair falls back to
	// the sampled path and its fragments report TExact = false, with the topology
	// unchanged. Raising [WithSegmentsPerTurn] is what recovers exactness there.
	//
	// A WHOLE edge (Whole = true) is bounded by the curve's own domain ends, not by a
	// contact, so exactness turns on whether those ends were EVALUATED or PINNED:
	//   - Line/Arc/Circle/Ellipse/Conic/Spline/ClosedSpline/FitSpline/NURBS: the domain
	//     ends are the curve's own evaluation at t=0/t=1 (a clamped/interpolating curve
	//     passes through them exactly, a closed curve's seam is its own point), so a
	//     whole edge reports TExact = true — [0,1] reconstructs the curve exactly.
	//   - EllipticalArc: its ends are PINNED to the sketch Start/End points, which lie
	//     on the parametric ellipse only within solver tolerance. eval(t=0/t=1) misses
	//     the pinned Polyline end (by that tolerance, e.g. ~5e-3), so even a whole
	//     elliptical arc reports TExact = false. This is the one free-form curve whose
	//     whole edge is inexact, and the reason exactness is decided by reproduction,
	//     not by "is this bound the curve's own end".
	TExact bool
}

// Region is a minimal bounded area extracted from the arrangement: an outer
// boundary walked counter-clockwise, zero or more holes (inner boundaries,
// walked clockwise), the net area (outer minus holes), and whether the region
// derives from a self-intersecting input boundary.
//
// CONSTRUCT IT WITH KEYED FIELDS, or better, do not construct it at all: every
// field is an output of [Regions], and this struct GAINS fields as the engine
// reports more about a region (Degenerate below is one; [BoundaryEdge] gained
// TStart/TEnd/TExact the same way). An unkeyed composite literal breaks on each
// such addition, and that is accepted rather than designed around — an accessor
// over an unexported field would break identically, since a caller outside this
// package cannot write one either.
type Region struct {
	Outer            []BoundaryEdge
	Holes            [][]BoundaryEdge
	Area             float64 // net area (outer minus holes); >= 0 for a clean region
	SelfIntersecting bool    // an input boundary feeding this region crosses itself
	// Degenerate is true when an unresolvable condition reaches THIS region: one
	// involving a curve its own boundary is built from, or one that could not be
	// attributed to any curve (an unusable input, dropped before it formed an edge,
	// so what it would have subdivided is unknown).
	//
	// It is narrower than [Arrangement.Degenerate], which is set when the
	// arrangement holds any such condition at all — including one that produced no
	// region, or destroyed one. A region far from the trouble reads false here while
	// the arrangement still reads true, so a consumer deciding whether the whole
	// region set is trustworthy must read the arrangement flag, not this.
	Degenerate bool
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
