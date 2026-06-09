package sketch

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func approx(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("%s = %.9f, want %.9f", name, got, want)
	}
}

func mustSolve(t *testing.T, s *Sketch) *Result {
	t.Helper()
	res, err := s.Solve()
	if err != nil {
		t.Fatalf("solve failed: %v (residual=%.3e)", err, res.Residual)
	}
	return res
}

// Test helpers that construct geometry and immediately commit it, keeping the
// test bodies terse. They exercise the construct-then-add API underneath.
func addPt(s *Sketch, x, y float64) *Point          { return s.AddPoint(NewPoint(x, y)) }
func addLn(s *Sketch, a, b *Point) *Line            { return s.AddLine(NewLine(a, b)) }
func addCir(s *Sketch, c *Point, r float64) *Circle { return s.AddCircle(NewCircle(c, r)) }
func addArc(s *Sketch, c, a, b *Point) *Arc         { return s.AddArc(NewArc(c, a, b)) }
func addDist(s *Sketch, a, b *Point, d float64) *Distance {
	c := NewDistance(a, b, d)
	s.AddConstraint(c)
	return c
}

// A fully constrained rectangle: grounded origin, horizontal/vertical sides,
// width and height dimensions.
func newRectangle(t *testing.T) (s *Sketch, w *Distance, b, c, d *Point) {
	s = New()
	a := addPt(s, 0, 0)
	b = addPt(s, 18, 2) // deliberately rough guesses
	c = addPt(s, 17, 11)
	d = addPt(s, 1, 13)

	ab := addLn(s, a, b)
	bc := addLn(s, b, c)
	dc := addLn(s, d, c)
	ad := addLn(s, a, d)

	s.Lock(a, 0, 0)
	s.AddConstraint(NewHorizontal(ab), NewHorizontal(dc), NewVertical(ad), NewVertical(bc))
	w = addDist(s, a, b, 20)
	addDist(s, a, d, 12)
	return s, w, b, c, d
}

func TestRectangleSolves(t *testing.T) {
	s, _, b, c, d := newRectangle(t)
	res := mustSolve(t, s)

	if res.DOF != 0 {
		t.Errorf("DOF = %d, want 0 (fully constrained)", res.DOF)
	}
	approx(t, "b.X", b.X(), 20)
	approx(t, "b.Y", b.Y(), 0)
	approx(t, "c.X", c.X(), 20)
	approx(t, "c.Y", c.Y(), 12)
	approx(t, "d.X", d.X(), 0)
	approx(t, "d.Y", d.Y(), 12)
}

func TestParametricUpdate(t *testing.T) {
	s, w, b, c, _ := newRectangle(t)
	mustSolve(t, s)

	w.Set(35) // change the driving width dimension
	mustSolve(t, s)
	approx(t, "b.X after edit", b.X(), 35)
	approx(t, "c.X after edit", c.X(), 35)
	approx(t, "c.Y after edit", c.Y(), 12) // height unchanged
}

func TestDetachedThenCommitted(t *testing.T) {
	// Construct geometry fully detached, inspect it, then commit.
	a := NewPoint(0, 0)
	b := NewPoint(40, 0)
	approx(t, "detached b.X", b.X(), 40) // readable before being added
	l := NewLine(a, b)
	approx(t, "detached length", l.Length(), 40)

	s := New()
	s.AddLine(l) // cascades: commits a and b too
	if !a.IsFixed() && len(s.points) != 2 {
		t.Fatalf("expected 2 points committed, got %d", len(s.points))
	}
	s.Lock(a, 0, 0)
	s.AddConstraint(NewHorizontal(l))
	addDist(s, a, b, 25)
	mustSolve(t, s)
	approx(t, "committed b.X", b.X(), 25)
}

func TestAddIdempotent(t *testing.T) {
	s := New()
	p := NewPoint(1, 2)
	s.AddPoint(p)
	s.AddPoint(p) // no-op
	if len(s.points) != 1 {
		t.Errorf("idempotent add: got %d points, want 1", len(s.points))
	}
}

func TestTangentLineCircle(t *testing.T) {
	s := New()
	a := addPt(s, 0, 0)
	b := addPt(s, 10, 0)
	s.Lock(a, 0, 0)
	s.Lock(b, 10, 0)
	line := addLn(s, a, b)

	center := addPt(s, 5, 5)
	s.Fix(center)
	circ := addCir(s, center, 2) // bad initial radius
	s.AddConstraint(NewTangent(line, circ))

	mustSolve(t, s)
	approx(t, "tangent radius", circ.R(), 5)
}

func TestPerpendicular(t *testing.T) {
	s := New()
	a := addPt(s, 0, 0)
	b := addPt(s, 10, 1)
	c := addPt(s, 1, 5)
	s.Lock(a, 0, 0)
	l1 := addLn(s, a, b)
	l2 := addLn(s, a, c)
	s.AddConstraint(NewHorizontal(l1), NewPerpendicular(l1, l2))
	addDist(s, a, b, 10)
	addDist(s, a, c, 5)

	mustSolve(t, s)
	d1x, d1y := dir(l1)
	d2x, d2y := dir(l2)
	approx(t, "perp dot", d1x*d2x+d1y*d2y, 0)
	approx(t, "ac length", dist(a, c), 5)
}

func TestAngleConstraint(t *testing.T) {
	s := New()
	a := addPt(s, 0, 0)
	b := addPt(s, 10, 0)
	c := addPt(s, 5, 5)
	s.Lock(a, 0, 0)
	l1 := addLn(s, a, b)
	l2 := addLn(s, a, c)
	s.AddConstraint(NewHorizontal(l1))
	addDist(s, a, b, 10)
	addDist(s, a, c, 8)
	s.AddConstraint(NewAngle(l1, l2, 45)) // degrees (the sketch's default angle unit)

	mustSolve(t, s)
	d1x, d1y := dir(l1)
	d2x, d2y := dir(l2)
	ang := math.Atan2(d1x*d2y-d1y*d2x, d1x*d2x+d1y*d2y)
	approx(t, "angle", ang, math.Pi/4)
}

func TestArcRadiusConsistency(t *testing.T) {
	s := New()
	center := addPt(s, 0, 0)
	start := addPt(s, 5, 0)
	end := addPt(s, 1, 9) // off the radius-5 circle
	s.Lock(center, 0, 0)
	s.Fix(start)
	arc := addArc(s, center, start, end)

	mustSolve(t, s)
	approx(t, "arc radius via end", math.Hypot(end.X(), end.Y()), 5)
	approx(t, "arc R()", arc.R(), 5)
}

func TestConcentricEqualRadius(t *testing.T) {
	s := New()
	o1 := addPt(s, 0, 0)
	o2 := addPt(s, 3, 2)
	s.Lock(o1, 0, 0)
	c1 := addCir(s, o1, 5)
	c2 := addCir(s, o2, 9)
	s.AddConstraint(NewConcentric(c1, c2), NewEqualRadius(c1, c2), NewRadius(c1, 7))

	mustSolve(t, s)
	approx(t, "c2 center x", o2.X(), 0)
	approx(t, "c2 center y", o2.Y(), 0)
	approx(t, "c1 radius", c1.R(), 7)
	approx(t, "c2 radius", c2.R(), 7)
}

func TestSymmetric(t *testing.T) {
	s := New()
	// vertical axis along x = 0
	axA := addPt(s, 0, 0)
	axB := addPt(s, 0, 10)
	s.Lock(axA, 0, 0)
	s.Lock(axB, 0, 10)
	axis := addLn(s, axA, axB)

	p1 := addPt(s, -3, 4)
	p2 := addPt(s, 5, 1)
	s.Fix(p1)
	s.AddConstraint(NewSymmetric(p1, p2, axis))

	mustSolve(t, s)
	approx(t, "mirror x", p2.X(), 3)
	approx(t, "mirror y", p2.Y(), 4)
}

func TestUnderConstrainedDOF(t *testing.T) {
	s := New()
	addPt(s, 0, 0) // single free point, nothing else
	if got := s.DOF(); got != 2 {
		t.Errorf("DOF = %d, want 2", got)
	}
}

func TestRedundantConstraint(t *testing.T) {
	s, _, b, _, _ := newRectangle(t)
	// Add a redundant duplicate width dimension.
	a := s.points[0]
	addDist(s, a, b, 20)
	res := mustSolve(t, s)
	if res.Redundant == 0 {
		t.Errorf("expected at least one redundant equation, got %d", res.Redundant)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	s, _, b, c, d := newRectangle(t)
	mustSolve(t, s)

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var s2 Sketch
	if err := json.Unmarshal(data, &s2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(s2.points) != len(s.points) {
		t.Fatalf("points: got %d want %d", len(s2.points), len(s.points))
	}
	res := mustSolve(t, &s2)
	if res.DOF != 0 {
		t.Errorf("reloaded DOF = %d, want 0", res.DOF)
	}
	approx(t, "reloaded b.X", s2.points[b.id].X(), 20)
	approx(t, "reloaded c.Y", s2.points[c.id].Y(), 12)
	approx(t, "reloaded d.X", s2.points[d.id].X(), 0)
}

func TestSVGOutput(t *testing.T) {
	s, _, _, _, _ := newRectangle(t)
	mustSolve(t, s)
	o := addPt(s, 10, 6)
	addCir(s, o, 3)

	svg, err := s.SVG(DefaultSVGOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<svg", "<line", "<circle", "</svg>"} {
		if !strings.Contains(svg, want) {
			t.Errorf("SVG missing %q", want)
		}
	}
}

func TestDXFOutput(t *testing.T) {
	s := New()
	a := addPt(s, 0, 0)
	b := addPt(s, 10, 0)
	addLn(s, a, b)
	o := addPt(s, 5, 5)
	addCir(s, o, 3)
	st := addPt(s, 8, 5)
	en := addPt(s, 5, 8)
	addArc(s, o, st, en)

	dxf, err := s.DXF()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"SECTION", "ENTITIES", "LINE", "CIRCLE", "ARC", "EOF"} {
		if !strings.Contains(dxf, want) {
			t.Errorf("DXF missing %q", want)
		}
	}
}
