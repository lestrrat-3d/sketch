package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/param"
	"github.com/lestrrat-3d/sketch/units"
	"github.com/stretchr/testify/require"
)

// A distance set with a typed value keeps that unit but solves in base mm.
func TestDimensionTypedValue(t *testing.T) {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(1, 0)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(s.AddLine(a, b)))
	d := sketch.NewDistance(a, b, 0)
	s.AddConstraint(d)
	require.NoError(t, d.SetValue(units.Inches(4)))
	require.Equal(t, units.Inch, d.Target().Unit(), "unit")
	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 101.6, b.X(), 1e-6, "4in in mm") // 4 * 25.4
}

// SetValue must reject a value of the wrong kind.
func TestDimensionWrongKind(t *testing.T) {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(1, 0)
	d := sketch.NewDistance(a, b, 0)
	require.Error(t, d.SetValue(units.Degrees(30)), "expected error setting a length dimension from an angle")
}

// The bare-float Angle constructor uses the sketch's default angle unit.
func TestAngleDefaultUnitDegrees(t *testing.T) {
	s := sketch.New() // metric default: degrees
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	c := s.AddPoint(5, 5)
	a.MoveTo(0, 0)
	s.Fix(a)
	l1 := s.AddLine(a, b)
	l2 := s.AddLine(a, c)
	s.AddConstraint(sketch.NewHorizontal(l1), sketch.NewAngle(l1, l2, 90)) // 90 degrees
	s.AddConstraint(sketch.NewDistance(a, b, 10))
	s.AddConstraint(sketch.NewDistance(a, c, 8))

	_, err := s.Solve()
	require.NoError(t, err)
	d1x, d1y := l1.End.X()-l1.Start.X(), l1.End.Y()-l1.Start.Y()
	d2x, d2y := l2.End.X()-l2.Start.X(), l2.End.Y()-l2.Start.Y()
	require.InDelta(t, 0, d1x*d2x+d1y*d2y, 1e-6, "perp via 90deg")
}

// SetUnits changes how bare-float dimension values are interpreted.
func TestSetUnitsImperial(t *testing.T) {
	s := sketch.New()
	s.SetUnits(units.Imperial()) // inches, degrees
	a := s.AddPoint(0, 0)
	b := s.AddPoint(1, 0)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(s.AddLine(a, b)))
	s.AddConstraint(sketch.NewDistance(a, b, 2)) // 2 inches
	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 50.8, b.X(), 1e-6, "2in in mm")
}

// A length dimension bound to a typed length parameter converts correctly.
func TestBindTypedLengthParam(t *testing.T) {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(1, 0)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(s.AddLine(a, b)))

	p := param.New()
	require.NoError(t, p.SetValue("len", units.Meters(1))) // 1 m == 1000 mm
	lenDim := sketch.NewDistance(a, b, 0)
	s.AddConstraint(lenDim)
	require.NoError(t, s.Bind(lenDim, p, "len"))
	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 1000, b.X(), 1e-6, "1m in mm")
}

// Binding a length dimension to an angle parameter is a kind error at solve.
func TestBindKindMismatch(t *testing.T) {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(1, 0)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(s.AddLine(a, b)))

	p := param.New()
	require.NoError(t, p.SetValue("turn", units.Degrees(45)))
	turnDim := sketch.NewDistance(a, b, 0)
	s.AddConstraint(turnDim)
	require.NoError(t, s.Bind(turnDim, p, "turn"))
	_, err := s.Solve()
	require.Error(t, err, "expected kind-mismatch error when a length is driven by an angle")
}

// An angle dimension bound to a typed angle parameter (degrees) solves correctly.
func TestBindAngleParam(t *testing.T) {
	s := sketch.New()
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	c := s.AddPoint(5, 5)
	a.MoveTo(0, 0)
	s.Fix(a)
	l1 := s.AddLine(a, b)
	l2 := s.AddLine(a, c)
	s.AddConstraint(sketch.NewHorizontal(l1))
	s.AddConstraint(sketch.NewDistance(a, b, 10))
	s.AddConstraint(sketch.NewDistance(a, c, 8))

	p := param.New()
	require.NoError(t, p.SetValue("theta", units.Degrees(30)))
	theta := sketch.NewAngle(l1, l2, 0)
	s.AddConstraint(theta)
	require.NoError(t, s.Bind(theta, p, "theta"))
	_, err := s.Solve()
	require.NoError(t, err)
	d1x, d1y := l1.End.X()-l1.Start.X(), l1.End.Y()-l1.Start.Y()
	d2x, d2y := l2.End.X()-l2.Start.X(), l2.End.Y()-l2.Start.Y()
	ang := math.Atan2(d1x*d2y-d1y*d2x, d1x*d2x+d1y*d2y)
	require.InDelta(t, math.Pi/6, ang, 1e-6, "30 degrees in rad")
}

// The unit system and dimension units survive a JSON round-trip.
func TestJSONRoundTripUnits(t *testing.T) {
	s := sketch.New()
	s.SetUnits(units.Imperial())
	a := s.AddPoint(0, 0)
	b := s.AddPoint(1, 0)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(s.AddLine(a, b)))
	d := sketch.NewDistance(a, b, 0)
	s.AddConstraint(d)
	require.NoError(t, d.SetValue(units.Inches(3)))
	_, err := s.Solve()
	require.NoError(t, err)

	data, err := s.MarshalJSON()
	require.NoError(t, err)
	require.Contains(t, string(data), `"units"`, "expected units in JSON")
	require.Contains(t, string(data), `"in"`, "expected in (inch) in JSON")

	var s2 sketch.Sketch
	require.NoError(t, s2.UnmarshalJSON(data))
	require.Equal(t, units.Inch, s2.Units().Length, "reloaded length unit")
	_, err = s2.Solve()
	require.NoError(t, err)
	require.InDelta(t, 76.2, s2.Points()[1].X(), 1e-6, "reloaded 3in in mm") // 3 * 25.4
}
