package sketch

import (
	"errors"
	"fmt"
	"math"

	"github.com/lestrrat-3d/sketch/geom"
)

// ErrInvalidShape is returned by the compound shape builders and creation
// variants in this file (for example [Sketch.CreatePolygon], [Sketch.CreateSlot]
// and [Sketch.CreateArcThreePoints]) and by the pattern tools
// ([Sketch.CreatePatternRect], [Sketch.CreatePatternCircular]) when given counts
// or coordinates that cannot form the requested shape.
var ErrInvalidShape = errors.New("sketch: invalid shape parameters")

// Compound constructors build several primitives plus the constraints that
// hold them in shape, in one call. They are pure sugar over the primitive
// Add… methods: the created points, entities and constraints are ordinary
// sketch citizens and serialize as such. Only the returned grouping handle is
// not persisted — reloading a sketch yields the same geometry and constraints,
// without the handle.

// Rectangle groups the geometry created by [Sketch.CreateRectangle]. Corners run
// counter-clockwise: A=(x1,y1), B=(x2,y1), C=(x2,y2), D=(x1,y2), with sides
// AB, BC, CD, DA connecting them in order.
type Rectangle struct {
	A, B, C, D     *Point
	AB, BC, CD, DA *Line
}

// CreateRectangle builds a rectangle aligned to the sketch's plane-local axes,
// between two opposite corners:
// four lines sharing corner points, held rectangular by horizontal constraints
// on AB/CD and vertical constraints on BC/DA. Position, width and height stay
// free to ground and dimension.
func (s *Sketch) CreateRectangle(x1, y1, x2, y2 float64) *Rectangle {
	a, b := s.CreatePoint(x1, y1), s.CreatePoint(x2, y1)
	c, d := s.CreatePoint(x2, y2), s.CreatePoint(x1, y2)
	r := &Rectangle{
		A: a, B: b, C: c, D: d,
		AB: s.CreateLine(a, b),
		BC: s.CreateLine(b, c),
		CD: s.CreateLine(c, d),
		DA: s.CreateLine(d, a),
	}
	s.AddConstraint(NewHorizontal(r.AB), NewHorizontal(r.CD), NewVertical(r.BC), NewVertical(r.DA))
	return r
}

// Polygon groups the geometry created by [Sketch.CreatePolygon].
type Polygon struct {
	Center   *Point
	Vertices []*Point
	Sides    []*Line
	Spokes   []*Line // construction lines center→vertex that hold regularity
}

// CreatePolygon builds a regular n-sided polygon centered at (cx, cy) with
// circumradius r and its first vertex at angle 0. Regularity is held by
// construction "spoke" lines from the center to every vertex constrained equal,
// plus all sides constrained equal — the standard sketcher formulation. The
// polygon keeps 4 degrees of freedom (position, rotation, size) to ground and
// dimension. It returns [ErrInvalidShape] when n < 3.
func (s *Sketch) CreatePolygon(cx, cy float64, n int, r float64) (*Polygon, error) {
	if n < 3 {
		return nil, fmt.Errorf("%w: CreatePolygon requires n >= 3, got %d", ErrInvalidShape, n)
	}
	p := &Polygon{Center: s.CreatePoint(cx, cy)}
	for i := 0; i < n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		p.Vertices = append(p.Vertices, s.CreatePoint(cx+r*math.Cos(a), cy+r*math.Sin(a)))
	}
	for i := 0; i < n; i++ {
		p.Sides = append(p.Sides, s.CreateLine(p.Vertices[i], p.Vertices[(i+1)%n]))
		spoke := s.CreateLine(p.Center, p.Vertices[i])
		spoke.SetConstruction(true)
		p.Spokes = append(p.Spokes, spoke)
	}
	for i := 1; i < n; i++ {
		s.AddConstraint(NewEqual(p.Sides[0], p.Sides[i]), NewEqual(p.Spokes[0], p.Spokes[i]))
	}
	return p, nil
}

// Slot groups the geometry created by [Sketch.CreateSlot].
type Slot struct {
	C1, C2 *Point  // cap centers
	A1, A2 *Arc    // semicircular end caps
	L1, L2 *Line   // straight flanks (L1 on the right of the c1→c2 axis, L2 on the left)
	Spokes []*Line // construction center→contact-point lines holding tangency
}

// CreateSlot builds a straight slot: two semicircular end caps of radius r around
// (x1, y1) and (x2, y2), joined by two straight flanks that share the cap
// endpoints. The shape is held by an equal-radius constraint between the caps
// plus a construction "spoke" from each cap center to each of its contact
// points, constrained perpendicular to the flank through that point.
// Perpendicularity at a point on the line implies tangency and — unlike a
// plain tangent constraint — also pins the contact point, so the caps stay
// semicircular instead of sliding along the flanks. The slot keeps 5 degrees
// of freedom (both centers and the radius) to ground and dimension; pin the
// width by dimensioning a cap contact point to its center. It returns
// [ErrInvalidShape] if the two centers coincide.
func (s *Sketch) CreateSlot(x1, y1, x2, y2, r float64) (*Slot, error) {
	dx, dy := x2-x1, y2-y1
	n := math.Hypot(dx, dy)
	if n == 0 {
		return nil, fmt.Errorf("%w: CreateSlot requires distinct cap centers", ErrInvalidShape)
	}
	nx, ny := -dy/n, dx/n // left normal of the c1→c2 axis
	c1, c2 := s.CreatePoint(x1, y1), s.CreatePoint(x2, y2)
	p1l := s.CreatePoint(x1+r*nx, y1+r*ny)
	p1r := s.CreatePoint(x1-r*nx, y1-r*ny)
	p2l := s.CreatePoint(x2+r*nx, y2+r*ny)
	p2r := s.CreatePoint(x2-r*nx, y2-r*ny)
	st := &Slot{
		C1: c1, C2: c2,
		// Both caps sweep counter-clockwise across the far side of the slot.
		A1: s.CreateArc(c1, p1l, p1r),
		A2: s.CreateArc(c2, p2r, p2l),
		L1: s.CreateLine(p1r, p2r),
		L2: s.CreateLine(p2l, p1l),
	}
	spoke := func(center, contact *Point, flank *Line) {
		sp := s.CreateLine(center, contact)
		sp.SetConstruction(true)
		st.Spokes = append(st.Spokes, sp)
		s.AddConstraint(NewPerpendicular(sp, flank))
	}
	spoke(c1, p1r, st.L1)
	spoke(c2, p2r, st.L1)
	spoke(c1, p1l, st.L2)
	spoke(c2, p2l, st.L2)
	s.AddConstraint(NewEqualRadius(st.A1, st.A2))
	return st, nil
}

// allFinite reports whether every value is neither NaN nor ±Inf.
func allFinite(vs ...float64) bool {
	for _, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// CreateArcThreePoints adds an arc from start to end that passes through
// (mx, my), the counterpart of Fusion's three-point arc. start and end are
// existing points, so the arc can share them with other geometry; (mx, my) only
// places the arc and is not kept as a point. A new center point is created at
// the circumcenter. Arcs sweep counter-clockwise, so when (mx, my) lies
// clockwise of the path from start to end, the returned arc's Start is end and
// its End is start. To keep a point on the arc, create it and add
// [NewPointOnArc]. Beyond its two endpoints the arc adds one degree of freedom
// (its bulge). It returns [ErrForeignEntity] if start or end is not a live point
// of this sketch, and [ErrInvalidShape] if start and end are the same point, mx
// or my is not finite, or the three positions coincide or are collinear — in
// each case without modifying the sketch.
func (s *Sketch) CreateArcThreePoints(start *Point, mx, my float64, end *Point) (*Arc, error) {
	if !s.owns(start) || !s.owns(end) {
		return nil, ErrForeignEntity
	}
	if start == end {
		return nil, fmt.Errorf("%w: CreateArcThreePoints requires distinct start and end points", ErrInvalidShape)
	}
	if !allFinite(mx, my) {
		return nil, fmt.Errorf("%w: CreateArcThreePoints requires a finite middle position", ErrInvalidShape)
	}
	sg, eg := start.Geometry(), end.Geometry()
	g, ok := geom.ArcThroughPoints(sg, geom.NewPoint(mx, my), eg)
	if !ok {
		return nil, fmt.Errorf("%w: CreateArcThreePoints requires three distinct, finite, non-collinear positions", ErrInvalidShape)
	}
	center := s.CreatePoint(g.Center.X, g.Center.Y)
	if g.Start == sg {
		return s.CreateArc(center, start, end), nil
	}
	return s.CreateArc(center, end, start), nil
}

// CreateArcTangent adds an arc that starts at at, an endpoint of l, leaves it
// tangent to l in the direction l runs toward at, and ends at a new point
// (x, y). It is the counterpart of Fusion's tangent arc. A new center point is
// created, and [NewTangent] holds the arc tangent to l at the shared point, so
// the arc stays tangent when either is re-dimensioned. Arcs sweep
// counter-clockwise: an arc that turns left has at as its Start, and one that
// turns right has at as its End. Beyond the line, the arc adds two degrees of
// freedom (its end point). It returns [ErrForeignEntity] if l is not a live line
// of this sketch or has an endpoint that is not a live point of it, and
// [ErrInvalidShape] if at is not an endpoint of l, l has zero length, x or y is
// not finite, or (x, y) lies on the infinite line through l (no arc tangent at
// at reaches it) — in each case without modifying the sketch.
func (s *Sketch) CreateArcTangent(l *Line, at *Point, x, y float64) (*Arc, error) {
	if s.foreignInput(l) {
		return nil, ErrForeignEntity
	}
	var other *Point
	switch at {
	case l.End:
		other = l.Start
	case l.Start:
		other = l.End
	default:
		return nil, fmt.Errorf("%w: CreateArcTangent requires at to be an endpoint of l", ErrInvalidShape)
	}
	if !allFinite(x, y) {
		return nil, fmt.Errorf("%w: CreateArcTangent requires a finite end point", ErrInvalidShape)
	}
	dx, dy := at.x()-other.x(), at.y()-other.y()
	dl := math.Hypot(dx, dy)
	if dl <= epsTool {
		return nil, fmt.Errorf("%w: CreateArcTangent requires a line of nonzero length", ErrInvalidShape)
	}
	vx, vy := x-at.x(), y-at.y()
	cross := dx*vy - dy*vx
	if math.Abs(cross) <= epsTool*dl*math.Hypot(vx, vy) {
		return nil, fmt.Errorf("%w: CreateArcTangent requires an end point off the line", ErrInvalidShape)
	}
	nx, ny := -dy/dl, dx/dl // left normal of the travel direction
	k := (vx*vx + vy*vy) / (2 * (nx*vx + ny*vy))
	end := s.CreatePoint(x, y)
	center := s.CreatePoint(at.x()+k*nx, at.y()+k*ny)
	var a *Arc
	if cross > 0 {
		a = s.CreateArc(center, at, end)
	} else {
		a = s.CreateArc(center, end, at)
	}
	s.AddConstraint(NewTangent(l, a))
	return a, nil
}

// CreateCircleThreeTangents adds a circle tangent to the infinite lines through
// l1, l2 and l3, the counterpart of Fusion's three-tangent circle. Three lines
// that form a triangle have four such circles (the incircle and three
// excircles); the hint (hx, hy) picks the one whose region contains it: inside
// the triangle for the incircle, across one side for that side's excircle. A new
// center point and the circle are created, held by one [NewTangent] per line.
// The circle adds no degree of freedom: the lines determine it. It returns
// [ErrForeignEntity] if a line is not a live line of this sketch or has an
// endpoint that is not a live point of it, and [ErrInvalidShape] if two
// arguments are the same line, a line has zero length, two lines are parallel,
// hx or hy is not finite, the hint lies on one of the lines, or no tangent
// circle lies in the hint's region (a hint across a vertex, or three lines
// through one point) — in each case without modifying the sketch.
func (s *Sketch) CreateCircleThreeTangents(l1, l2, l3 *Line, hx, hy float64) (*Circle, error) {
	if s.foreignInput(l1, l2, l3) {
		return nil, ErrForeignEntity
	}
	if l1 == l2 || l1 == l3 || l2 == l3 {
		return nil, fmt.Errorf("%w: CreateCircleThreeTangents requires three distinct lines", ErrInvalidShape)
	}
	if !allFinite(hx, hy) {
		return nil, fmt.Errorf("%w: CreateCircleThreeTangents requires a finite hint", ErrInvalidShape)
	}
	lines := [3]*Line{l1, l2, l3}
	var m [3][3]float64 // rows: nx·cx + ny·cy − σ·ρ = d
	var rhs [3]float64
	for i, l := range lines {
		dx, dy := l.End.x()-l.Start.x(), l.End.y()-l.Start.y()
		dl := math.Hypot(dx, dy)
		if dl <= epsTool {
			return nil, fmt.Errorf("%w: CreateCircleThreeTangents requires lines of nonzero length", ErrInvalidShape)
		}
		nx, ny := -dy/dl, dx/dl
		d := nx*l.Start.x() + ny*l.Start.y()
		side := nx*hx + ny*hy - d
		if math.Abs(side) <= epsTool {
			return nil, fmt.Errorf("%w: CreateCircleThreeTangents requires a hint off every line", ErrInvalidShape)
		}
		sigma := 1.0
		if side < 0 {
			sigma = -1
		}
		m[i] = [3]float64{nx, ny, -sigma}
		rhs[i] = d
	}
	for i := 0; i < 3; i++ {
		for j := i + 1; j < 3; j++ {
			if math.Abs(m[i][0]*m[j][1]-m[i][1]*m[j][0]) <= epsTool {
				return nil, fmt.Errorf("%w: CreateCircleThreeTangents requires non-parallel lines", ErrInvalidShape)
			}
		}
	}
	det := det3(m)
	if math.Abs(det) <= epsTool {
		return nil, fmt.Errorf("%w: CreateCircleThreeTangents found no tangent circle", ErrInvalidShape)
	}
	var sol [3]float64
	for k := 0; k < 3; k++ {
		mk := m
		for i := 0; i < 3; i++ {
			mk[i][k] = rhs[i]
		}
		sol[k] = det3(mk) / det
	}
	if sol[2] <= epsTool {
		return nil, fmt.Errorf("%w: CreateCircleThreeTangents found no tangent circle in the hint's region", ErrInvalidShape)
	}
	c := s.CreateCircle(s.CreatePoint(sol[0], sol[1]), sol[2])
	s.AddConstraint(NewTangent(l1, c), NewTangent(l2, c), NewTangent(l3, c))
	return c, nil
}

// det3 returns the determinant of a 3×3 matrix.
func det3(m [3][3]float64) float64 {
	return m[0][0]*(m[1][1]*m[2][2]-m[1][2]*m[2][1]) -
		m[0][1]*(m[1][0]*m[2][2]-m[1][2]*m[2][0]) +
		m[0][2]*(m[1][0]*m[2][1]-m[1][1]*m[2][0])
}

// ArcSlot groups the geometry created by [Sketch.CreateArcSlot].
type ArcSlot struct {
	Center       *Point  // center of the slot's centerline arc
	C1, C2       *Point  // cap centers: the centerline's start and end
	Outer, Inner *Arc    // concentric flanks about Center
	A1, A2       *Arc    // semicircular end caps about C1 and C2
	Spokes       []*Line // construction lines Center→C1 and Center→C2, for dimensioning
}

// CreateArcSlot builds an arc slot, the counterpart of Fusion's center-point
// arc slot: a slot of cap radius r (half the slot width) whose centerline is the
// arc about (cx, cy) running counter-clockwise from (x1, y1) toward the
// direction of (x2, y2). As with [Sketch.CreateArc], (x2, y2) sets only the end
// direction; the centerline radius is the distance from the center to
// (x1, y1). Swap the two points for the clockwise slot. Two concentric flank
// arcs about Center are joined by two semicircular caps about C1 and C2; each
// cap is held tangent to both flanks at the endpoints it shares with them
// ([NewTangentCircles]: internally to Outer, externally to Inner), which also
// keeps both caps the same radius and both cap centers on the centerline. The
// construction Spokes carry no constraint; they exist so the sweep can be
// dimensioned with [NewAngle] and the centerline radius with [NewDistance] on
// Center and C1. The slot keeps 6 degrees of freedom (center, centerline
// radius, start angle, sweep, cap radius). It returns [ErrInvalidShape] if an
// argument is not finite, (x1, y1) or (x2, y2) coincides with the center,
// r <= 0 or r is not less than the centerline radius, the sweep is a full turn,
// or the caps would overlap across the gap of a sweep above 180°.
func (s *Sketch) CreateArcSlot(cx, cy, x1, y1, x2, y2, r float64) (*ArcSlot, error) {
	if !allFinite(cx, cy, x1, y1, x2, y2, r) {
		return nil, fmt.Errorf("%w: CreateArcSlot requires finite arguments", ErrInvalidShape)
	}
	rc := math.Hypot(x1-cx, y1-cy)
	if rc <= epsTool || math.Hypot(x2-cx, y2-cy) <= epsTool {
		return nil, fmt.Errorf("%w: CreateArcSlot requires both points away from the center", ErrInvalidShape)
	}
	if r <= 0 || r >= rc {
		return nil, fmt.Errorf("%w: CreateArcSlot requires 0 < r < centerline radius", ErrInvalidShape)
	}
	th1 := math.Atan2(y1-cy, x1-cx)
	sweep := math.Mod(math.Atan2(y2-cy, x2-cx)-th1, 2*math.Pi)
	if sweep < 0 {
		sweep += 2 * math.Pi
	}
	if sweep <= epsTool || sweep >= 2*math.Pi-epsTool {
		return nil, fmt.Errorf("%w: CreateArcSlot requires a sweep below a full turn", ErrInvalidShape)
	}
	if sweep > math.Pi && rc*math.Sin(sweep/2) <= r {
		return nil, fmt.Errorf("%w: CreateArcSlot caps would overlap across the gap", ErrInvalidShape)
	}
	th2 := th1 + sweep
	at := func(th, rad float64) *Point { return s.CreatePoint(cx+rad*math.Cos(th), cy+rad*math.Sin(th)) }
	center := s.CreatePoint(cx, cy)
	c1, c2 := at(th1, rc), at(th2, rc)
	p1o, p1i := at(th1, rc+r), at(th1, rc-r)
	p2o, p2i := at(th2, rc+r), at(th2, rc-r)
	sl := &ArcSlot{
		Center: center, C1: c1, C2: c2,
		Outer: s.CreateArc(center, p1o, p2o),
		Inner: s.CreateArc(center, p1i, p2i),
		// Each cap's half-turn bulges away from the slot body.
		A1: s.CreateArc(c1, p1i, p1o),
		A2: s.CreateArc(c2, p2o, p2i),
	}
	for _, c := range []*Point{c1, c2} {
		sp := s.CreateLine(center, c)
		sp.SetConstruction(true)
		sl.Spokes = append(sl.Spokes, sp)
	}
	s.AddConstraint(
		NewTangentCircles(sl.A1, sl.Outer, true),
		NewTangentCircles(sl.A1, sl.Inner, false),
		NewTangentCircles(sl.A2, sl.Outer, true),
		NewTangentCircles(sl.A2, sl.Inner, false),
	)
	return sl, nil
}
