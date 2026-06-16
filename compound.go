package sketch

import (
	"errors"
	"fmt"
	"math"
)

// ErrInvalidShape is returned by the compound shape builders ([Sketch.AddPolygon],
// [Sketch.AddSlot]) and the pattern tools ([Sketch.AddPatternRect],
// [Sketch.AddPatternCircular]) when given counts or coordinates that cannot form
// the requested shape.
var ErrInvalidShape = errors.New("sketch: invalid shape parameters")

// Compound constructors build several primitives plus the constraints that
// hold them in shape, in one call. They are pure sugar over the primitive
// Add… methods: the created points, entities and constraints are ordinary
// sketch citizens and serialize as such. Only the returned grouping handle is
// not persisted — reloading a sketch yields the same geometry and constraints,
// without the handle.

// Rectangle groups the geometry created by [Sketch.AddRectangle]. Corners run
// counter-clockwise: A=(x1,y1), B=(x2,y1), C=(x2,y2), D=(x1,y2), with sides
// AB, BC, CD, DA connecting them in order.
type Rectangle struct {
	A, B, C, D     *Point
	AB, BC, CD, DA *Line
}

// AddRectangle builds a rectangle aligned to the sketch's plane-local axes,
// between two opposite corners:
// four lines sharing corner points, held rectangular by horizontal constraints
// on AB/CD and vertical constraints on BC/DA. Position, width and height stay
// free to ground and dimension.
func (s *Sketch) AddRectangle(x1, y1, x2, y2 float64) *Rectangle {
	a, b := s.AddPoint(x1, y1), s.AddPoint(x2, y1)
	c, d := s.AddPoint(x2, y2), s.AddPoint(x1, y2)
	r := &Rectangle{
		A: a, B: b, C: c, D: d,
		AB: s.AddLine(a, b),
		BC: s.AddLine(b, c),
		CD: s.AddLine(c, d),
		DA: s.AddLine(d, a),
	}
	s.AddConstraint(NewHorizontal(r.AB), NewHorizontal(r.CD), NewVertical(r.BC), NewVertical(r.DA))
	return r
}

// Polygon groups the geometry created by [Sketch.AddPolygon].
type Polygon struct {
	Center   *Point
	Vertices []*Point
	Sides    []*Line
	Spokes   []*Line // construction lines center→vertex that hold regularity
}

// AddPolygon builds a regular n-sided polygon centered at (cx, cy) with
// circumradius r and its first vertex at angle 0. Regularity is held by
// construction "spoke" lines from the center to every vertex constrained equal,
// plus all sides constrained equal — the standard sketcher formulation. The
// polygon keeps 4 degrees of freedom (position, rotation, size) to ground and
// dimension. It returns [ErrInvalidShape] when n < 3.
func (s *Sketch) AddPolygon(cx, cy float64, n int, r float64) (*Polygon, error) {
	if n < 3 {
		return nil, fmt.Errorf("%w: AddPolygon requires n >= 3, got %d", ErrInvalidShape, n)
	}
	p := &Polygon{Center: s.AddPoint(cx, cy)}
	for i := 0; i < n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		p.Vertices = append(p.Vertices, s.AddPoint(cx+r*math.Cos(a), cy+r*math.Sin(a)))
	}
	for i := 0; i < n; i++ {
		p.Sides = append(p.Sides, s.AddLine(p.Vertices[i], p.Vertices[(i+1)%n]))
		spoke := s.AddLine(p.Center, p.Vertices[i])
		spoke.SetConstruction(true)
		p.Spokes = append(p.Spokes, spoke)
	}
	for i := 1; i < n; i++ {
		s.AddConstraint(NewEqual(p.Sides[0], p.Sides[i]), NewEqual(p.Spokes[0], p.Spokes[i]))
	}
	return p, nil
}

// Slot groups the geometry created by [Sketch.AddSlot].
type Slot struct {
	C1, C2 *Point  // cap centers
	A1, A2 *Arc    // semicircular end caps
	L1, L2 *Line   // straight flanks (L1 on the right of the c1→c2 axis, L2 on the left)
	Spokes []*Line // construction center→contact-point lines holding tangency
}

// AddSlot builds a straight slot: two semicircular end caps of radius r around
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
func (s *Sketch) AddSlot(x1, y1, x2, y2, r float64) (*Slot, error) {
	dx, dy := x2-x1, y2-y1
	n := math.Hypot(dx, dy)
	if n == 0 {
		return nil, fmt.Errorf("%w: AddSlot requires distinct cap centers", ErrInvalidShape)
	}
	nx, ny := -dy/n, dx/n // left normal of the c1→c2 axis
	c1, c2 := s.AddPoint(x1, y1), s.AddPoint(x2, y2)
	p1l := s.AddPoint(x1+r*nx, y1+r*ny)
	p1r := s.AddPoint(x1-r*nx, y1-r*ny)
	p2l := s.AddPoint(x2+r*nx, y2+r*ny)
	p2r := s.AddPoint(x2-r*nx, y2-r*ny)
	st := &Slot{
		C1: c1, C2: c2,
		// Both caps sweep counter-clockwise across the far side of the slot.
		A1: s.AddArc(c1, p1l, p1r),
		A2: s.AddArc(c2, p2r, p2l),
		L1: s.AddLine(p1r, p2r),
		L2: s.AddLine(p2l, p1l),
	}
	spoke := func(center, contact *Point, flank *Line) {
		sp := s.AddLine(center, contact)
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
