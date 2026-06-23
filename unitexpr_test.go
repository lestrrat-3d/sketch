package sketch_test

import (
	"encoding/json"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/param"
	"github.com/lestrrat-3d/sketch/units"
	"github.com/stretchr/testify/require"
)

// boundWidth builds a fixed point and a free point with a Distance dimension
// between them, bound to expr against a table carrying width (length) and theta
// (angle) parameters.
func boundWidth(t *testing.T, expr string) (*sketch.Sketch, *sketch.Distance) {
	t.Helper()
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(80, 0)
	s.Fix(a)
	d := sketch.NewDistance(a, b, 80)
	s.AddConstraint(d)
	tbl := s.Params() // the world's shared table
	require.NoError(t, tbl.SetValue("width", units.Millimeters(80)))
	require.NoError(t, tbl.SetValue("pad", units.Millimeters(20)))
	require.NoError(t, tbl.SetValue("theta", units.Degrees(30)))
	require.NoError(t, s.Bind(d, tbl, expr)) // Bind is syntax-only
	return s, d
}

func TestUnitExprMixedKindRejected(t *testing.T) {
	// A length dimension driven by a length + angle expression: the oracle must
	// reject it, not solve against a meaningless retagged magnitude.
	s, _ := boundWidth(t, "width + theta")
	_, err := s.Solve()
	require.ErrorIs(t, err, param.ErrIncompatibleKind, "the bad expression fails the solve")

	rep := s.Verify()
	require.False(t, rep.ParametersValid)
	require.NotEmpty(t, rep.ParameterErrors)
	require.ErrorIs(t, rep.ParameterErrors[0], param.ErrIncompatibleKind)
	require.False(t, rep.Trustworthy(), "a unit-kind bug is not blessed")
}

func TestUnitExprWrongKindRejected(t *testing.T) {
	// A well-formed ANGLE expression driving a LENGTH dimension is rejected on the
	// kind mismatch, even though the expression itself is internally consistent.
	s, _ := boundWidth(t, "theta * 2")
	_, err := s.Solve()
	require.Error(t, err)
	require.False(t, s.Verify().ParametersValid)
}

func TestUnitExprValidLengthSolves(t *testing.T) {
	// A length + length expression drives the length dimension cleanly.
	s, d := boundWidth(t, "width + pad")
	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 100, d.Target().Base(), 1e-9, "80mm + 20mm")
	rep := s.Verify()
	require.True(t, rep.ParametersValid)
	require.Empty(t, rep.ParameterErrors)
}

func TestUnitExprScalarStillDrives(t *testing.T) {
	// A purely dimensionless expression still drives a length dimension (tagged
	// with the dimension's base unit), matching the prior behavior.
	s, d := boundWidth(t, "40 * 2")
	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 80, d.Target().Base(), 1e-9)
	require.True(t, s.Verify().ParametersValid)
}

func TestUnitExprSmuggledAngleRejected(t *testing.T) {
	// A length dimension bound to a parameter that is DECLARED length but DEFINED
	// by an angle expression must not pass: the angle would otherwise drive the
	// length dimension undetected.
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(80, 0)
	s.Fix(a)
	d := sketch.NewDistance(a, b, 80)
	s.AddConstraint(d)
	tbl := s.Params() // the world's shared table
	require.NoError(t, tbl.SetValue("theta", units.Degrees(30)))
	require.NoError(t, tbl.SetExpr("w", "theta", units.Millimeter)) // length-declared, angle-defined
	require.NoError(t, s.Bind(d, tbl, "w"))

	_, err := s.Solve()
	require.ErrorIs(t, err, param.ErrIncompatibleKind)
	rep := s.Verify()
	require.False(t, rep.ParametersValid)
	require.False(t, rep.Trustworthy())
}

func TestUnitExprRoundTripPreservesKindError(t *testing.T) {
	// The bad expression survives serialization; the reloaded sketch still reports
	// the kind error (the validation is on the interpretation, not stored state).
	s, _ := boundWidth(t, "width + theta")
	data, err := json.Marshal(s)
	require.NoError(t, err)
	require.Contains(t, string(data), "width + theta")

	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	_, err = s2.Solve()
	require.ErrorIs(t, err, param.ErrIncompatibleKind)
	require.False(t, s2.Verify().ParametersValid)
}
