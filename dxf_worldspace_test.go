package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/space"
	"github.com/stretchr/testify/require"
)

// aaa reproduces AutoCAD's arbitrary-axis algorithm independently of the
// exporter, so a world-space round-trip reconstructs the OCS frame from the
// emitted extrusion direction and validates the encoding against Point.World().
func aaa(n space.Vec3) (space.Vec3, space.Vec3) {
	const tol = 1.0 / 64.0
	var a space.Vec3
	if math.Abs(n.X) < tol && math.Abs(n.Y) < tol {
		a = space.NewVec3(0, 1, 0).Cross(n)
	} else {
		a = space.NewVec3(0, 0, 1).Cross(n)
	}
	ax, _ := a.Normalize()
	ay, _ := n.Cross(ax).Normalize()
	return ax, ay
}

// tiltedSketch returns a sketch placed on a deliberately non-axis-aligned plane
// (so the OCS arbitrary-axis path is genuinely exercised).
func tiltedSketch(t *testing.T) *sketch.Sketch {
	t.Helper()
	fr, err := space.NewFrame(
		space.NewVec3(10, -5, 7), // origin off the world origin
		space.NewVec3(1, 2, 0),   // u (orthonormalized by NewFrame)
		space.NewVec3(0, 1, 3),   // v
	)
	require.NoError(t, err)
	w := sketch.NewWorld()
	pl, err := w.CreatePlaneFromFrame(fr)
	require.NoError(t, err)
	s, err := w.CreateSketch(pl)
	require.NoError(t, err)
	return s
}

func worldVec(t *testing.T, dxf, marker, c10 string) space.Vec3 {
	t.Helper()
	gx := mustFloat(t, dxfGroup(t, dxf, marker, c10))
	// c10 is "10"/"11"; its y/z partners are +10/+20.
	switch c10 {
	case "10":
		return space.NewVec3(gx, mustFloat(t, dxfGroup(t, dxf, marker, "20")), mustFloat(t, dxfGroup(t, dxf, marker, "30")))
	case "11":
		return space.NewVec3(gx, mustFloat(t, dxfGroup(t, dxf, marker, "21")), mustFloat(t, dxfGroup(t, dxf, marker, "31")))
	}
	t.Fatalf("unexpected base code %q", c10)
	return space.Vec3{}
}

func requireVecInDelta(t *testing.T, want, got space.Vec3, msg string) {
	t.Helper()
	// 1e-5 absorbs the exporter's 6-decimal ASCII rounding (trimFloat(v, 6)),
	// which accumulates a few ×1e-6 across an OCS basis reconstruction — far
	// below any real placement error.
	require.InDeltaf(t, want.X, got.X, 1e-5, "%s x", msg)
	require.InDeltaf(t, want.Y, got.Y, 1e-5, "%s y", msg)
	require.InDeltaf(t, want.Z, got.Z, 1e-5, "%s z", msg)
}

// A LINE on a tilted plane carries true WCS endpoints — exactly Point.World().
func TestDXFWorldSpaceLine(t *testing.T) {
	s := tiltedSketch(t)
	a := s.AddPoint(3, 4)
	b := s.AddPoint(-2, 9)
	s.AddLine(a, b)

	dxf, err := s.DXF(sketch.WithWorldSpace(true))
	require.NoError(t, err)

	requireVecInDelta(t, a.World(), worldVec(t, dxf, "LINE", "10"), "line start")
	requireVecInDelta(t, b.World(), worldVec(t, dxf, "LINE", "11"), "line end")
}

// A CIRCLE on a tilted plane is emitted in OCS + extrusion; reconstructing the
// world center from (OCS center, extrusion) must reproduce the center's
// Point.World(), the extrusion must equal the plane normal, and radius is rigid.
func TestDXFWorldSpaceCircleOCS(t *testing.T) {
	s := tiltedSketch(t)
	c := s.AddPoint(6, -1)
	s.AddCircle(c, 3.5)

	dxf, err := s.DXF(sketch.WithWorldSpace(true))
	require.NoError(t, err)

	ext := space.NewVec3(
		mustFloat(t, dxfGroup(t, dxf, "CIRCLE", "210")),
		mustFloat(t, dxfGroup(t, dxf, "CIRCLE", "220")),
		mustFloat(t, dxfGroup(t, dxf, "CIRCLE", "230")),
	)
	ocs := worldVec(t, dxf, "CIRCLE", "10") // OCS coords live in the 10/20/30 slots
	ax, ay := aaa(ext)
	recon := ax.Scale(ocs.X).Add(ay.Scale(ocs.Y)).Add(ext.Scale(ocs.Z))
	requireVecInDelta(t, c.World(), recon, "circle center")

	fr, err := s.Plane().Frame()
	require.NoError(t, err)
	requireVecInDelta(t, fr.N(), ext, "extrusion is plane normal")
	require.InDelta(t, 3.5, mustFloat(t, dxfGroup(t, dxf, "CIRCLE", "40")), 1e-9, "radius is rigid")
}

// An ARC on a tilted plane: reconstructing each endpoint from (OCS center,
// radius, OCS angle) must reproduce the endpoints' Point.World(), validating the
// OCS center AND the recomputed OCS angles together.
func TestDXFWorldSpaceArcAngles(t *testing.T) {
	s := tiltedSketch(t)
	ctr := s.AddPoint(0, 0)
	st := s.AddPoint(4, 0)
	en := s.AddPoint(0, 4)
	s.AddArc(ctr, st, en)

	dxf, err := s.DXF(sketch.WithWorldSpace(true))
	require.NoError(t, err)

	ext := space.NewVec3(
		mustFloat(t, dxfGroup(t, dxf, "ARC", "210")),
		mustFloat(t, dxfGroup(t, dxf, "ARC", "220")),
		mustFloat(t, dxfGroup(t, dxf, "ARC", "230")),
	)
	ax, ay := aaa(ext)
	ocs := worldVec(t, dxf, "ARC", "10")
	wc := ax.Scale(ocs.X).Add(ay.Scale(ocs.Y)).Add(ext.Scale(ocs.Z))
	r := mustFloat(t, dxfGroup(t, dxf, "ARC", "40"))
	sa := mustFloat(t, dxfGroup(t, dxf, "ARC", "50")) * math.Pi / 180
	ea := mustFloat(t, dxfGroup(t, dxf, "ARC", "51")) * math.Pi / 180

	wStart := wc.Add(ax.Scale(r * math.Cos(sa))).Add(ay.Scale(r * math.Sin(sa)))
	wEnd := wc.Add(ax.Scale(r * math.Cos(ea))).Add(ay.Scale(r * math.Sin(ea)))
	requireVecInDelta(t, st.World(), wStart, "arc start from OCS angle")
	requireVecInDelta(t, en.World(), wEnd, "arc end from OCS angle")
}

// An ELLIPSE on a tilted plane uses the WCS form: center is Point.World(), the
// major-axis vector (11/21/31) is the world direction of the longer semi-axis
// (so its length is that semi-axis and it lies in the plane), and the extrusion
// is the plane normal so the reader derives the minor axis in-plane.
func TestDXFWorldSpaceEllipse(t *testing.T) {
	s := tiltedSketch(t)
	c := s.AddPoint(2, 3)
	s.AddEllipse(c, 5, 2, 0.3) // rx > ry → major = rx, no swap

	dxf, err := s.DXF(sketch.WithWorldSpace(true))
	require.NoError(t, err)

	requireVecInDelta(t, c.World(), worldVec(t, dxf, "ELLIPSE", "10"), "ellipse center")
	maj := worldVec(t, dxf, "ELLIPSE", "11")
	require.InDelta(t, 5, maj.Len(), 1e-5, "major-axis vector length is the major semi-axis")

	ext := space.NewVec3(
		mustFloat(t, dxfGroup(t, dxf, "ELLIPSE", "210")),
		mustFloat(t, dxfGroup(t, dxf, "ELLIPSE", "220")),
		mustFloat(t, dxfGroup(t, dxf, "ELLIPSE", "230")),
	)
	fr, err := s.Plane().Frame()
	require.NoError(t, err)
	requireVecInDelta(t, fr.N(), ext, "extrusion is plane normal")
	require.InDelta(t, 0, maj.Dot(ext), 1e-5, "major axis lies in the plane")
	require.InDelta(t, 0.4, mustFloat(t, dxfGroup(t, dxf, "ELLIPSE", "40")), 1e-9, "ratio ry/rx = 2/5")
}

// An elliptical ARC on a tilted plane: reconstructing its endpoints from
// (WCS center, major-axis vector, extrusion-derived minor axis, params 41/42)
// must reproduce the start/end Point.World() — validating that the eccentric
// params and the DXF-derived minor axis survive the world rotation.
func TestDXFWorldSpaceEllipticalArc(t *testing.T) {
	s := tiltedSketch(t)
	c := s.AddPoint(0, 0)
	start := s.AddPoint(4, 0) // eccentric param 0
	end := s.AddPoint(0, 2)   // eccentric param π/2
	s.AddEllipticalArc(c, start, end, 4, 2, 0)

	dxf, err := s.DXF(sketch.WithWorldSpace(true))
	require.NoError(t, err)

	center := worldVec(t, dxf, "ELLIPSE", "10")
	major := worldVec(t, dxf, "ELLIPSE", "11")
	ext := space.NewVec3(
		mustFloat(t, dxfGroup(t, dxf, "ELLIPSE", "210")),
		mustFloat(t, dxfGroup(t, dxf, "ELLIPSE", "220")),
		mustFloat(t, dxfGroup(t, dxf, "ELLIPSE", "230")),
	)
	ratio := mustFloat(t, dxfGroup(t, dxf, "ELLIPSE", "40"))
	p41 := mustFloat(t, dxfGroup(t, dxf, "ELLIPSE", "41"))
	p42 := mustFloat(t, dxfGroup(t, dxf, "ELLIPSE", "42"))

	majHat, _ := major.Normalize()
	minor := ext.Cross(majHat).Scale(ratio * major.Len()) // DXF derives minor = ext × major̂
	recon := func(p float64) space.Vec3 {
		return center.Add(major.Scale(math.Cos(p))).Add(minor.Scale(math.Sin(p)))
	}
	requireVecInDelta(t, start.World(), recon(p41), "elliptical-arc start param")
	requireVecInDelta(t, end.World(), recon(p42), "elliptical-arc end param")
}

// On the world-XY plane the OCS reduces to the WCS: the circle center keeps its
// local coordinates and the extrusion is the implied +Z. So world-space output
// matches local geometry plus an explicit extrusion — no surprise displacement.
func TestDXFWorldSpaceXYReducesToLocal(t *testing.T) {
	s := newSketch(t) // world-XY
	c := s.AddPoint(5, 7)
	s.AddCircle(c, 2)

	dxf, err := s.DXF(sketch.WithWorldSpace(true))
	require.NoError(t, err)

	center := worldVec(t, dxf, "CIRCLE", "10")
	requireVecInDelta(t, space.NewVec3(5, 7, 0), center, "XY circle center unchanged")
	ext := space.NewVec3(
		mustFloat(t, dxfGroup(t, dxf, "CIRCLE", "210")),
		mustFloat(t, dxfGroup(t, dxf, "CIRCLE", "220")),
		mustFloat(t, dxfGroup(t, dxf, "CIRCLE", "230")),
	)
	requireVecInDelta(t, space.NewVec3(0, 0, 1), ext, "XY extrusion is +Z")
}

// The default (no option) output stays plane-local: a tilted sketch still emits
// 2D coordinates with z = 0 and no extrusion — backward compatible.
func TestDXFDefaultStaysLocal(t *testing.T) {
	s := tiltedSketch(t)
	c := s.AddPoint(6, -1)
	s.AddCircle(c, 3.5)

	dxf, err := s.DXF()
	require.NoError(t, err)

	require.InDelta(t, 6, mustFloat(t, dxfGroup(t, dxf, "CIRCLE", "10")), 1e-9, "local x")
	require.InDelta(t, -1, mustFloat(t, dxfGroup(t, dxf, "CIRCLE", "20")), 1e-9, "local y")
	require.NotContains(t, dxf, "210\n", "no extrusion in local mode")
}
