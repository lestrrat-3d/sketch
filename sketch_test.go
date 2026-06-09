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

// A fully constrained rectangle: grounded origin, horizontal/vertical sides,
// width and height dimensions.
func newRectangle(t *testing.T) (s *Sketch, w *Distance, b, c, d *Point) {
	s = New()
	a := s.AddPoint(0, 0)
	b = s.AddPoint(18, 2) // deliberately rough guesses
	c = s.AddPoint(17, 11)
	d = s.AddPoint(1, 13)

	ab := s.AddLine(a, b)
	bc := s.AddLine(b, c)
	dc := s.AddLine(d, c)
	ad := s.AddLine(a, d)

	s.Lock(a, 0, 0)
	s.Horizontal(ab)
	s.Horizontal(dc)
	s.Vertical(ad)
	s.Vertical(bc)
	w = s.Distance(a, b, 20)
	s.Distance(a, d, 12)
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

func TestTangentLineCircle(t *testing.T) {
	s := New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	s.Lock(a, 0, 0)
	s.Lock(b, 10, 0)
	line := s.AddLine(a, b)

	center := s.AddPoint(5, 5)
	s.Fix(center)
	circ := s.AddCircle(center, 2) // bad initial radius
	s.Tangent(line, circ)

	mustSolve(t, s)
	approx(t, "tangent radius", circ.R(), 5)
}

func TestPerpendicular(t *testing.T) {
	s := New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 1)
	c := s.AddPoint(1, 5)
	s.Lock(a, 0, 0)
	l1 := s.AddLine(a, b)
	l2 := s.AddLine(a, c)
	s.Horizontal(l1)
	s.Distance(a, b, 10)
	s.Perpendicular(l1, l2)
	s.Distance(a, c, 5)

	mustSolve(t, s)
	d1x, d1y := dir(l1)
	d2x, d2y := dir(l2)
	approx(t, "perp dot", d1x*d2x+d1y*d2y, 0)
	approx(t, "ac length", dist(a, c), 5)
}

func TestAngleConstraint(t *testing.T) {
	s := New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	c := s.AddPoint(5, 5)
	s.Lock(a, 0, 0)
	l1 := s.AddLine(a, b)
	l2 := s.AddLine(a, c)
	s.Horizontal(l1)
	s.Distance(a, b, 10)
	s.Distance(a, c, 8)
	s.Angle(l1, l2, 45) // degrees (the sketch's default angle unit)

	mustSolve(t, s)
	d1x, d1y := dir(l1)
	d2x, d2y := dir(l2)
	ang := math.Atan2(d1x*d2y-d1y*d2x, d1x*d2x+d1y*d2y)
	approx(t, "angle", ang, math.Pi/4)
}

func TestArcRadiusConsistency(t *testing.T) {
	s := New()
	center := s.AddPoint(0, 0)
	start := s.AddPoint(5, 0)
	end := s.AddPoint(1, 9) // off the radius-5 circle
	s.Lock(center, 0, 0)
	s.Fix(start)
	arc := s.AddArc(center, start, end)

	mustSolve(t, s)
	approx(t, "arc radius via end", math.Hypot(end.X(), end.Y()), 5)
	approx(t, "arc R()", arc.R(), 5)
}

func TestConcentricEqualRadius(t *testing.T) {
	s := New()
	o1 := s.AddPoint(0, 0)
	o2 := s.AddPoint(3, 2)
	s.Lock(o1, 0, 0)
	c1 := s.AddCircle(o1, 5)
	c2 := s.AddCircle(o2, 9)
	s.Concentric(c1, c2)
	s.EqualRadius(c1, c2)
	s.Radius(c1, 7)

	mustSolve(t, s)
	approx(t, "c2 center x", o2.X(), 0)
	approx(t, "c2 center y", o2.Y(), 0)
	approx(t, "c1 radius", c1.R(), 7)
	approx(t, "c2 radius", c2.R(), 7)
}

func TestSymmetric(t *testing.T) {
	s := New()
	// vertical axis along x = 0
	axA := s.AddPoint(0, 0)
	axB := s.AddPoint(0, 10)
	s.Lock(axA, 0, 0)
	s.Lock(axB, 0, 10)
	axis := s.AddLine(axA, axB)

	p1 := s.AddPoint(-3, 4)
	p2 := s.AddPoint(5, 1)
	s.Fix(p1)
	s.Symmetric(p1, p2, axis)

	mustSolve(t, s)
	approx(t, "mirror x", p2.X(), 3)
	approx(t, "mirror y", p2.Y(), 4)
}

func TestUnderConstrainedDOF(t *testing.T) {
	s := New()
	s.AddPoint(0, 0) // single free point, nothing else
	if got := s.DOF(); got != 2 {
		t.Errorf("DOF = %d, want 2", got)
	}
}

func TestRedundantConstraint(t *testing.T) {
	s, _, _, _, _ := newRectangle(t)
	// Add a redundant duplicate width dimension.
	a := s.points[0]
	b := s.points[1]
	s.Distance(a, b, 20)
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
	o := s.AddPoint(10, 6)
	s.AddCircle(o, 3)

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
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	s.AddLine(a, b)
	o := s.AddPoint(5, 5)
	s.AddCircle(o, 3)
	st := s.AddPoint(8, 5)
	en := s.AddPoint(5, 8)
	s.AddArc(o, st, en)

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
