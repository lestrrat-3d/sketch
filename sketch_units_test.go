package sketch

import (
	"math"
	"strings"
	"testing"

	"github.com/lestrrat-3d/sketch/param"
	"github.com/lestrrat-3d/sketch/units"
)

// A distance set with a typed value keeps that unit but solves in base mm.
func TestDimensionTypedValue(t *testing.T) {
	s := New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(1, 0)
	s.Lock(a, 0, 0)
	s.Horizontal(s.AddLine(a, b))
	d := s.Distance(a, b, 0)
	if err := d.SetValue(units.Inches(4)); err != nil {
		t.Fatal(err)
	}
	if d.Target().Unit() != units.Inch {
		t.Errorf("unit = %v, want in", d.Target().Unit())
	}
	mustSolve(t, s)
	approx(t, "4in in mm", b.X(), 101.6) // 4 * 25.4
}

// SetValue must reject a value of the wrong kind.
func TestDimensionWrongKind(t *testing.T) {
	s := New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(1, 0)
	d := s.Distance(a, b, 0)
	if err := d.SetValue(units.Degrees(30)); err == nil {
		t.Fatal("expected error setting a length dimension from an angle")
	}
}

// The bare-float Angle constructor uses the sketch's default angle unit.
func TestAngleDefaultUnitDegrees(t *testing.T) {
	s := New() // metric default: degrees
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	c := s.AddPoint(5, 5)
	s.Lock(a, 0, 0)
	l1 := s.AddLine(a, b)
	l2 := s.AddLine(a, c)
	s.Horizontal(l1)
	s.Distance(a, b, 10)
	s.Distance(a, c, 8)
	s.Angle(l1, l2, 90) // 90 degrees

	mustSolve(t, s)
	d1x, d1y := dir(l1)
	d2x, d2y := dir(l2)
	approx(t, "perp via 90deg", d1x*d2x+d1y*d2y, 0)
}

// SetUnits changes how bare-float dimension values are interpreted.
func TestSetUnitsImperial(t *testing.T) {
	s := New()
	s.SetUnits(units.Imperial()) // inches, degrees
	a := s.AddPoint(0, 0)
	b := s.AddPoint(1, 0)
	s.Lock(a, 0, 0)
	s.Horizontal(s.AddLine(a, b))
	s.Distance(a, b, 2) // 2 inches
	mustSolve(t, s)
	approx(t, "2in in mm", b.X(), 50.8)
}

// A length dimension bound to a typed length parameter converts correctly.
func TestBindTypedLengthParam(t *testing.T) {
	s := New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(1, 0)
	s.Lock(a, 0, 0)
	s.Horizontal(s.AddLine(a, b))

	p := param.New()
	p.SetValue("len", units.Meters(1)) // 1 m == 1000 mm
	if err := s.Bind(s.Distance(a, b, 0), p, "len"); err != nil {
		t.Fatal(err)
	}
	mustSolve(t, s)
	approx(t, "1m in mm", b.X(), 1000)
}

// Binding a length dimension to an angle parameter is a kind error at solve.
func TestBindKindMismatch(t *testing.T) {
	s := New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(1, 0)
	s.Lock(a, 0, 0)
	s.Horizontal(s.AddLine(a, b))

	p := param.New()
	p.SetValue("turn", units.Degrees(45))
	if err := s.Bind(s.Distance(a, b, 0), p, "turn"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Solve(); err == nil {
		t.Fatal("expected kind-mismatch error when a length is driven by an angle")
	}
}

// An angle dimension bound to a typed angle parameter (degrees) solves correctly.
func TestBindAngleParam(t *testing.T) {
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

	p := param.New()
	p.SetValue("theta", units.Degrees(30))
	if err := s.Bind(s.Angle(l1, l2, 0), p, "theta"); err != nil {
		t.Fatal(err)
	}
	mustSolve(t, s)
	d1x, d1y := dir(l1)
	d2x, d2y := dir(l2)
	ang := math.Atan2(d1x*d2y-d1y*d2x, d1x*d2x+d1y*d2y)
	approx(t, "30 degrees in rad", ang, math.Pi/6)
}

// The unit system and dimension units survive a JSON round-trip.
func TestJSONRoundTripUnits(t *testing.T) {
	s := New()
	s.SetUnits(units.Imperial())
	a := s.AddPoint(0, 0)
	b := s.AddPoint(1, 0)
	s.Lock(a, 0, 0)
	s.Horizontal(s.AddLine(a, b))
	d := s.Distance(a, b, 0)
	d.SetValue(units.Inches(3))
	mustSolve(t, s)

	data, err := s.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\"units\"") || !strings.Contains(string(data), "\"in\"") {
		t.Errorf("expected units/in in JSON: %s", data)
	}

	var s2 Sketch
	if err := s2.UnmarshalJSON(data); err != nil {
		t.Fatal(err)
	}
	if s2.Units().Length != units.Inch {
		t.Errorf("reloaded length unit = %v, want in", s2.Units().Length)
	}
	mustSolve(t, &s2)
	approx(t, "reloaded 3in in mm", s2.Points()[1].X(), 76.2) // 3 * 25.4
}
