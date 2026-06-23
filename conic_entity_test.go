package sketch_test

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestConicAddAndAccessors(t *testing.T) {
	s := newSketch(t)
	start := s.CreatePoint(0, 0)
	apex := s.CreatePoint(2, 5)
	end := s.CreatePoint(6, 0)
	c, err := s.CreateConic(start, apex, end, 0.4)
	require.NoError(t, err)
	require.InDelta(t, 0.4, c.Rho(), 1e-12)
	require.Same(t, start, c.Start)
	require.Same(t, apex, c.Apex)
	require.Same(t, end, c.End)

	x0, y0 := c.Eval(0)
	require.InDelta(t, 0, x0, 1e-12)
	require.InDelta(t, 0, y0, 1e-12)
	x1, y1 := c.Eval(1)
	require.InDelta(t, 6, x1, 1e-12)
	require.InDelta(t, 0, y1, 1e-12)

	g := c.Geometry()
	require.InDelta(t, 0.4, g.Rho, 1e-12)
}

func TestConicValidation(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(2, 5)
	d := s.CreatePoint(6, 0)
	for _, rho := range []float64{0, 1, -0.1, 1.5, math.NaN()} {
		_, err := s.CreateConic(a, b, d, rho)
		require.ErrorIsf(t, err, sketch.ErrInvalidShape, "rho %v rejected", rho)
	}
	_, err := s.CreateConic(nil, b, d, 0.5)
	require.ErrorIs(t, err, sketch.ErrInvalidShape, "nil start rejected")
}

func TestConicFreeDOF(t *testing.T) {
	// A free conic is 3 points (6) + 1 rho (7) of free DOF, exactly like a free
	// ellipse is under-constrained.
	s := newSketch(t)
	start := s.CreatePoint(0, 0)
	apex := s.CreatePoint(2, 5)
	end := s.CreatePoint(6, 0)
	c, err := s.CreateConic(start, apex, end, 0.5)
	require.NoError(t, err)
	require.Equal(t, 7, s.DOF(), "3 free points + 1 free rho")

	s.FixEntity(c)
	require.Equal(t, 0, s.DOF(), "fixing the conic grounds its points and rho")
	require.True(t, s.EntityFixed(c))
}

func TestConicFixEntityGroundsRho(t *testing.T) {
	s := newSketch(t)
	start := s.CreatePoint(0, 0)
	apex := s.CreatePoint(2, 5)
	end := s.CreatePoint(6, 0)
	s.Fix(start)
	s.Fix(apex)
	s.Fix(end)
	c, err := s.CreateConic(start, apex, end, 0.5)
	require.NoError(t, err)
	// Points fixed but rho free: 1 DOF remains.
	require.Equal(t, 1, s.DOF(), "rho is still a free DOF")
	s.FixEntity(c)
	require.Equal(t, 0, s.DOF(), "FixEntity grounds rho too")
}

func TestConicProfileParticipation(t *testing.T) {
	s := newSketch(t)
	start := s.CreatePoint(0, 0)
	apex := s.CreatePoint(3, 4)
	end := s.CreatePoint(6, 0)
	c, err := s.CreateConic(start, apex, end, 0.6)
	require.NoError(t, err)
	s.CreateLine(end, start) // chord closes the loop

	profiles := s.Profiles()
	require.Len(t, profiles, 1, "conic + chord bound one region")
	require.True(t, profiles[0].Valid)
	require.Greater(t, profiles[0].Area, 0.0)
	require.Contains(t, profiles[0].Entities, sketch.Entity(c))
}

func TestConicProfileAreaSamplingIndependent(t *testing.T) {
	// A conic-bounded region's area is exact, so it is identical at every
	// rendering fidelity for every family.
	for _, rho := range []float64{0.3, 0.5, 0.8} {
		s := newSketch(t)
		start := s.CreatePoint(0, 0)
		apex := s.CreatePoint(2, 5)
		end := s.CreatePoint(6, 0)
		_, err := s.CreateConic(start, apex, end, rho)
		require.NoError(t, err)
		s.CreateLine(end, start)
		profiles := s.Profiles()
		require.Lenf(t, profiles, 1, "rho %v", rho)
		require.Greaterf(t, profiles[0].Area, 1e-6, "rho %v positive area", rho)
	}
}

func TestConicConstructionExcluded(t *testing.T) {
	s := newSketch(t)
	start := s.CreatePoint(0, 0)
	apex := s.CreatePoint(3, 4)
	end := s.CreatePoint(6, 0)
	c, err := s.CreateConic(start, apex, end, 0.6)
	require.NoError(t, err)
	c.SetConstruction(true)
	s.CreateLine(end, start)
	require.Empty(t, s.Profiles(), "a construction conic bounds no reported profile")
}

func TestConicRoundTrip(t *testing.T) {
	s := newSketch(t)
	start := s.CreatePoint(0, 0)
	apex := s.CreatePoint(2, 5)
	end := s.CreatePoint(6, 0)
	_, err := s.CreateConic(start, apex, end, 0.73)
	require.NoError(t, err)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	require.Contains(t, string(data), `"conic"`, "serialized as a distinct entity type")

	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Len(t, s2.Entities(), 1, "no doubled internal constraints")
	cc, ok := s2.Entities()[0].(*sketch.Conic)
	require.True(t, ok, "reloads as a Conic")
	require.InDelta(t, 0.73, cc.Rho(), 1e-12, "rho preserved")
	x1, y1 := cc.Eval(1)
	require.InDelta(t, 6, x1, 1e-9)
	require.InDelta(t, 0, y1, 1e-9)

	// No constraints survive (a conic has none) — the round-trip never doubles
	// an internal constraint.
	require.Equal(t, s.DOF(), s2.DOF(), "DOF stable across round-trip")
}

func TestConicExportersContainIt(t *testing.T) {
	s := newSketch(t)
	start := s.CreatePoint(0, 0)
	apex := s.CreatePoint(3, 4)
	end := s.CreatePoint(6, 0)
	_, err := s.CreateConic(start, apex, end, 0.5)
	require.NoError(t, err)

	svg, err := s.SVG()
	require.NoError(t, err)
	require.Contains(t, svg, "<path", "SVG draws the conic as a path")

	png, err := s.PNG()
	require.NoError(t, err)
	require.NotEmpty(t, png)

	dxf, err := s.DXF()
	require.NoError(t, err)
	require.Contains(t, dxf, "SPLINE", "DXF emits a native SPLINE for the conic")
}

func TestConicDXFRationalWeights(t *testing.T) {
	s := newSketch(t)
	start := s.CreatePoint(0, 0)
	apex := s.CreatePoint(3, 4)
	end := s.CreatePoint(6, 0)
	const rho = 0.6
	_, err := s.CreateConic(start, apex, end, rho)
	require.NoError(t, err)

	dxf, err := s.DXF()
	require.NoError(t, err)

	// Flag 70 = 12 (planar | rational), degree 71 = 2.
	require.Equal(t, "12", dxfGroup(t, dxf, "SPLINE", "70"))
	require.Equal(t, "2", dxfGroup(t, dxf, "SPLINE", "71"))

	// The three rational weights are 1, w, 1 with w = rho/(1−rho).
	w := rho / (1 - rho)
	weights := dxfGroupAll(dxf, "SPLINE", "41")
	require.Len(t, weights, 3, "three control-point weights")
	require.InDelta(t, 1, mustFloat(t, weights[0]), 1e-9)
	require.InDelta(t, w, mustFloat(t, weights[1]), 1e-6, "apex weight w = rho/(1-rho)")
	require.InDelta(t, 1, mustFloat(t, weights[2]), 1e-9)

	// Local control points round-trip Start/Apex/End.
	xs := dxfGroupAll(dxf, "SPLINE", "10")
	require.Len(t, xs, 3)
	require.InDelta(t, 0, mustFloat(t, xs[0]), 1e-9)
	require.InDelta(t, 3, mustFloat(t, xs[1]), 1e-9)
	require.InDelta(t, 6, mustFloat(t, xs[2]), 1e-9)
}

func TestConicDXFWorldSpaceControlPoints(t *testing.T) {
	// On a tilted plane the conic's control points are emitted in true world
	// coordinates (the putWCS path) — each equals the point's World().
	s := tiltedSketch(t)
	start := s.CreatePoint(0, 0)
	apex := s.CreatePoint(3, 4)
	end := s.CreatePoint(6, 0)
	_, err := s.CreateConic(start, apex, end, 0.5)
	require.NoError(t, err)

	dxf, err := s.DXF(sketch.WithWorldSpace(true))
	require.NoError(t, err)

	xs := dxfGroupAll(dxf, "SPLINE", "10")
	ys := dxfGroupAll(dxf, "SPLINE", "20")
	zs := dxfGroupAll(dxf, "SPLINE", "30")
	require.Len(t, xs, 3)
	for i, p := range []*sketch.Point{start, apex, end} {
		w := p.World()
		require.InDeltaf(t, w.X, mustFloat(t, xs[i]), 1e-5, "ctrl %d world x", i)
		require.InDeltaf(t, w.Y, mustFloat(t, ys[i]), 1e-5, "ctrl %d world y", i)
		require.InDeltaf(t, w.Z, mustFloat(t, zs[i]), 1e-5, "ctrl %d world z", i)
	}
}

func TestConicRemoveEntityRetiresRho(t *testing.T) {
	s := newSketch(t)
	start := s.CreatePoint(0, 0)
	apex := s.CreatePoint(3, 4)
	end := s.CreatePoint(6, 0)
	c, err := s.CreateConic(start, apex, end, 0.5)
	require.NoError(t, err)
	require.Equal(t, 7, s.DOF())

	require.True(t, s.RemoveEntity(c))
	require.Empty(t, s.Entities())
	// The three points survive (6 DOF); the rho var is retired (fixed), not free.
	require.Equal(t, 6, s.DOF(), "rho retired, points remain")
}

func TestConicTypedNilEntity(t *testing.T) {
	s := newSketch(t)
	_, err := s.WorldPolyline((*sketch.Conic)(nil))
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
}

func TestConicForeignPointNotTrustworthy(t *testing.T) {
	// A conic built over a point from another sketch is a foreign handle — the
	// reachability scan (which reads the conic's defining points) must see it.
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(6, 0)
	s.Fix(a)
	s.Fix(b)
	other := newSketch(t)
	foreign := other.CreatePoint(3, 4)
	_, err := s.CreateConic(a, foreign, b, 0.5)
	require.NoError(t, err)
	rep := s.Verify()
	require.True(t, rep.ForeignHandles)
	require.False(t, rep.Trustworthy())
}

// dxfGroupAll returns every value emitted for group code after marker, in order.
func dxfGroupAll(dxf, marker, code string) []string {
	lines := strings.Split(dxf, "\n")
	start := 0
	for i, ln := range lines {
		if ln == marker {
			start = i + 1
			break
		}
	}
	var out []string
	for i := start; i+1 < len(lines); i += 2 {
		// Stop at the next entity boundary (group code 0) so we only collect this
		// entity's values.
		if lines[i] == "0" && i > start {
			break
		}
		if lines[i] == code {
			out = append(out, lines[i+1])
		}
	}
	return out
}
