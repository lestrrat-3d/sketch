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
	// Whole is true when this edge spans the entire source curve (the curve was
	// not split by any crossing); false when it is a fragment.
	Whole bool
	// Reversed is true when the boundary walks the source curve against its
	// natural Start→End (or CCW, for a closed curve) direction.
	Reversed bool
	// Polyline is the densified sample of this edge in walk order, the first
	// point the edge's start vertex and the last its end vertex. A line edge is
	// two points; an arc/closed-curve fragment is more.
	Polyline [][2]float64
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

// ClosedCurve is a closed primitive (circle or ellipse) admitted to the
// arrangement: it is sampled to a closed polyline. *Circle and *Ellipse satisfy
// it via their Polyline samplers.
type ClosedCurve interface {
	Polyline(segments int) [][2]float64
}
