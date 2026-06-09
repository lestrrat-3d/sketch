package sketch

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/lestrrat-3d/sketch/param"
)

// parametricRect builds a rectangle whose width and height are driven by
// parameters "w" and "h".
func parametricRect(t *testing.T) (s *Sketch, b, c, d *Point) {
	t.Helper()
	s = New()
	a := addPt(s, 0, 0)
	b = addPt(s, 5, 1)
	c = addPt(s, 4, 6)
	d = addPt(s, 1, 5)
	ab := addLn(s, a, b)
	bc := addLn(s, b, c)
	dc := addLn(s, d, c)
	ad := addLn(s, a, d)
	s.Lock(a, 0, 0)
	s.AddConstraint(NewHorizontal(ab), NewHorizontal(dc), NewVertical(ad), NewVertical(bc))

	p := param.New()
	if err := p.Set("w", "20"); err != nil {
		t.Fatal(err)
	}
	if err := p.Set("h", "w / 2"); err != nil { // height depends on width
		t.Fatal(err)
	}
	wDim := addDist(s, a, b, 0)
	hDim := addDist(s, a, d, 0)
	if err := s.Bind(wDim, p, "w"); err != nil {
		t.Fatal(err)
	}
	if err := s.Bind(hDim, p, "h"); err != nil {
		t.Fatal(err)
	}
	return s, b, c, d
}

func TestBoundDimensionsSolve(t *testing.T) {
	s, b, c, d := parametricRect(t)
	res := mustSolve(t, s)
	if res.DOF != 0 {
		t.Errorf("DOF = %d, want 0", res.DOF)
	}
	approx(t, "b.X", b.X(), 20)
	approx(t, "d.Y", d.Y(), 10)
	approx(t, "c", c.X(), 20)
	approx(t, "c.Y", c.Y(), 10)
}

func TestParameterEditPropagates(t *testing.T) {
	s, b, _, d := parametricRect(t)
	mustSolve(t, s)

	// Change the width parameter; height ("w/2") follows automatically.
	if err := s.Params().Set("w", "30"); err != nil {
		t.Fatal(err)
	}
	mustSolve(t, s)
	approx(t, "b.X after edit", b.X(), 30)
	approx(t, "d.Y after edit", d.Y(), 15)
}

func TestManualSetUnbinds(t *testing.T) {
	s, b, _, _ := parametricRect(t)
	mustSolve(t, s)

	// Find the width dimension and override it literally.
	var wDim *Distance
	for _, c := range s.Constraints() {
		if dd, ok := c.(*Distance); ok && dd.driverExpr() == "w" {
			wDim = dd
		}
	}
	if wDim == nil {
		t.Fatal("could not find bound width dimension")
	}
	wDim.Set(42)
	if wDim.driverExpr() != "" {
		t.Error("Set should clear the binding")
	}
	mustSolve(t, s)
	approx(t, "b.X after manual set", b.X(), 42)

	// Changing the parameter no longer affects the now-literal dimension.
	if err := s.Params().Set("w", "99"); err != nil {
		t.Fatal(err)
	}
	mustSolve(t, s)
	approx(t, "b.X still literal", b.X(), 42)
}

func TestBindExpressionInline(t *testing.T) {
	s := New()
	a := addPt(s, 0, 0)
	b := addPt(s, 3, 0)
	s.Lock(a, 0, 0)
	s.AddConstraint(NewHorizontal(addLn(s, a, b)))
	p := param.New()
	p.Set("gap", "8")
	dim := addDist(s, a, b, 0)
	// Expression combining a parameter, a function and a constant.
	if err := s.Bind(dim, p, "gap * 2 + sqrt(16)"); err != nil {
		t.Fatal(err)
	}
	mustSolve(t, s)
	approx(t, "b.X", b.X(), 20) // 8*2 + 4
}

func TestBindSyntaxError(t *testing.T) {
	s := New()
	a := addPt(s, 0, 0)
	b := addPt(s, 1, 0)
	dim := NewDistance(a, b, 1)
	if err := s.Bind(dim, param.New(), "1 +"); err == nil {
		t.Fatal("expected syntax error from Bind")
	}
}

func TestBindNilTable(t *testing.T) {
	s := New()
	a := addPt(s, 0, 0)
	b := addPt(s, 1, 0)
	dim := NewDistance(a, b, 1)
	if err := s.Bind(dim, nil, "1"); err == nil {
		t.Fatal("expected error binding with a nil table")
	}
}

func TestBindTableMismatch(t *testing.T) {
	s := New()
	a := addPt(s, 0, 0)
	b := addPt(s, 1, 0)
	c := addPt(s, 0, 1)
	p1 := param.New()
	p1.Set("x", "1")
	p2 := param.New()
	p2.Set("x", "1")
	if err := s.Bind(NewDistance(a, b, 0), p1, "x"); err != nil {
		t.Fatal(err)
	}
	err := s.Bind(NewDistance(a, c, 0), p2, "x") // different table
	if !errors.Is(err, ErrTableMismatch) {
		t.Fatalf("expected ErrTableMismatch, got %v", err)
	}
}

func TestUndefinedParameterFailsSolve(t *testing.T) {
	s := New()
	a := addPt(s, 0, 0)
	b := addPt(s, 1, 0)
	s.Lock(a, 0, 0)
	s.AddConstraint(NewHorizontal(addLn(s, a, b)))
	dim := addDist(s, a, b, 1)
	if err := s.Bind(dim, param.New(), "nope * 2"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Solve(); err == nil {
		t.Fatal("expected solve to fail on undefined parameter")
	}
}

func TestJSONRoundTripWithParameters(t *testing.T) {
	s, _, _, _ := parametricRect(t)
	mustSolve(t, s)

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\"parameters\"") {
		t.Error("serialized sketch should include parameters")
	}
	if !strings.Contains(string(data), "\"expr\"") {
		t.Error("serialized dimension should include its bound expression")
	}

	var s2 Sketch
	if err := json.Unmarshal(data, &s2); err != nil {
		t.Fatal(err)
	}
	// The reloaded sketch must still be parametric: edit and re-solve.
	if err := s2.Params().Set("w", "50"); err != nil {
		t.Fatal(err)
	}
	mustSolve(t, &s2)
	approx(t, "reloaded b.X", s2.Points()[1].X(), 50)
	approx(t, "reloaded d.Y", s2.Points()[3].Y(), 25)
}
