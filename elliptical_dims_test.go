package sketch_test

import (
	"encoding/json"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestEllipticalArcShapeDimensions(t *testing.T) {
	s := sketch.New()
	c := s.AddPoint(0, 0)
	s.Fix(c)
	start := s.AddPoint(5, 0)
	end := s.AddPoint(0, 3)
	ea := s.AddEllipticalArc(c, start, end, 5, 3, 0)

	// Drive the arc's underlying-ellipse shape via the widened dimensions.
	s.AddConstraint(
		sketch.NewSemiMajor(ea, 8),
		sketch.NewSemiMinor(ea, 4),
		sketch.NewEllipseRotation(ea, 0),
	)
	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 8, ea.Rx(), 1e-6, "semi-major driven")
	require.InDelta(t, 4, ea.Ry(), 1e-6, "semi-minor driven")
	require.InDelta(t, 0, ea.Rotation(), 1e-6, "rotation driven")
}

func TestEllipticalArcShapeDimensionsRoundTrip(t *testing.T) {
	s := sketch.New()
	c := s.AddPoint(0, 0)
	s.Fix(c)
	start := s.AddPoint(6, 0)
	end := s.AddPoint(0, 2)
	ea := s.AddEllipticalArc(c, start, end, 6, 2, 0)
	s.AddConstraint(sketch.NewSemiMajor(ea, 6), sketch.NewEllipseRotation(ea, 0))
	_, err := s.Solve()
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Constraints(), len(s.Constraints()), "dims on the elliptical arc survive reload")
	_, err = s2.Solve()
	require.NoError(t, err)
}

// The widening is non-breaking: a plain ellipse still works with the same dims.
func TestEllipseShapeDimensionsStillWork(t *testing.T) {
	s := sketch.New()
	o := s.AddPoint(0, 0)
	s.Fix(o)
	e := s.AddEllipse(o, 3, 2, 0)
	s.AddConstraint(sketch.NewSemiMajor(e, 10), sketch.NewSemiMinor(e, 5))
	_, err := s.Solve()
	require.NoError(t, err)
	require.InDelta(t, 10, e.Rx(), 1e-6)
	require.InDelta(t, 5, e.Ry(), 1e-6)
}
