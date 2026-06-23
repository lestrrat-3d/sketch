package sketch_test

import (
	"encoding/json"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestClosedSplineBoundsProfile(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(4, 0)
	c := s.AddPoint(4, 4)
	d := s.AddPoint(0, 4)
	sp, err := s.AddClosedSpline(a, b, c, d)
	require.NoError(t, err)

	profiles := s.Profiles()
	require.Len(t, profiles, 1, "a closed spline bounds one region on its own")
	require.True(t, profiles[0].Valid)
	require.Greater(t, profiles[0].Area, 0.0)
	require.Contains(t, profiles[0].Entities, sketch.Entity(sp))
	require.True(t, s.Verify().ProfilesValid)
}

func TestClosedSplineFigureEightInvalid(t *testing.T) {
	// A control polygon whose periodic loop crosses itself.
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(4, 3)
	c := s.AddPoint(0, 3)
	d := s.AddPoint(4, 0)
	_, err := s.AddClosedSpline(a, b, c, d)
	require.NoError(t, err)

	rep := s.Verify()
	require.False(t, rep.ProfilesValid, "a self-crossing closed spline is not valid")
	require.NotEmpty(t, rep.InvalidProfiles)
	var sawSelfX bool
	for _, p := range rep.InvalidProfiles {
		if p.SelfIntersecting {
			sawSelfX = true
		}
	}
	require.True(t, sawSelfX)
}

func TestClosedSplineValidation(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(1, 0)
	_, err := s.AddClosedSpline(a, b)
	require.ErrorIs(t, err, sketch.ErrInvalidShape, "fewer than 3 control points is rejected")
	c := s.AddPoint(0, 1)
	_, err = s.AddClosedSpline(a, b, nil, c)
	require.ErrorIs(t, err, sketch.ErrInvalidShape, "a nil control point is rejected")
}

func TestClosedSplineFixEntityDOF(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(4, 0)
	c := s.AddPoint(4, 4)
	d := s.AddPoint(0, 4)
	sp, err := s.AddClosedSpline(a, b, c, d)
	require.NoError(t, err)
	require.Equal(t, 8, s.DOF(), "4 free control points, no size vars or internal constraints")
	s.FixEntity(sp)
	require.Equal(t, 0, s.DOF(), "FixEntity grounds every control point")
}

func TestClosedSplineConstructionExcluded(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(4, 0)
	c := s.AddPoint(4, 4)
	d := s.AddPoint(0, 4)
	sp, err := s.AddClosedSpline(a, b, c, d)
	require.NoError(t, err)
	sp.SetConstruction(true)
	require.Empty(t, s.Profiles(), "a construction closed spline bounds no reported profile")
}

func TestClosedSplineRoundTrip(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(4, 0)
	c := s.AddPoint(4, 4)
	d := s.AddPoint(0, 4)
	_, err := s.AddClosedSpline(a, b, c, d)
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	require.Contains(t, string(data), "closed_spline", "serialized as a distinct entity type")

	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Entities(), 1)
	_, isClosed := s2.Entities()[0].(*sketch.ClosedSpline)
	require.True(t, isClosed, "reloads as a ClosedSpline, not an open Spline")
	require.Len(t, s2.Profiles(), 1, "the reloaded closed spline still bounds a region")
}

func TestClosedSplineExporters(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(4, 0)
	c := s.AddPoint(4, 4)
	d := s.AddPoint(0, 4)
	_, err := s.AddClosedSpline(a, b, c, d)
	require.NoError(t, err)

	svg, err := s.SVG()
	require.NoError(t, err)
	require.Contains(t, svg, "<path", "SVG draws the closed spline as a path")

	dxf, err := s.DXF()
	require.NoError(t, err)
	require.Contains(t, dxf, "LWPOLYLINE", "DXF emits a closed polyline approximation")

	png, err := s.PNG()
	require.NoError(t, err)
	require.NotEmpty(t, png)
}

func TestClosedSplineForeignControlNotTrustworthy(t *testing.T) {
	// A closed spline built with a control point owned by another sketch must be
	// caught by the reference-integrity scan (it was previously invisible).
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(4, 0)
	s.Fix(a)
	s.Fix(b)
	other := newSketch(t)
	foreign := other.AddPoint(2, 4)
	_, err := s.AddClosedSpline(a, b, foreign)
	require.NoError(t, err)

	rep := s.Verify()
	require.True(t, rep.ForeignHandles, "a foreign control point is reachable and detected")
	require.False(t, rep.Trustworthy(), "the oracle must not bless geometry built from a foreign handle")
}

func TestClosedSplineTypedNilEntity(t *testing.T) {
	// A typed-nil *ClosedSpline must follow the foreign-entity path, not panic.
	s := newSketch(t)
	_, err := s.WorldPolyline((*sketch.ClosedSpline)(nil))
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
}
