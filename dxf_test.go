package sketch_test

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/lestrrat-3d/r3"
	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/units"
	"github.com/stretchr/testify/require"
)

// dxfGroup returns the value string of the first occurrence of group code at or
// after the byte index where marker first appears, walking the code/value pair
// stream a DXF document is built from.
func dxfGroup(t *testing.T, dxf, marker, code string) string {
	t.Helper()
	lines := strings.Split(dxf, "\n")
	start := 0
	if marker != "" {
		for i, ln := range lines {
			if ln == marker {
				start = i + 1 // marker is a value line; codes follow it
				break
			}
		}
	}
	for i := start; i+1 < len(lines); i += 2 {
		if lines[i] == code {
			return lines[i+1]
		}
	}
	t.Fatalf("group code %q not found after marker %q", code, marker)
	return ""
}

// A metric sketch emits the $INSUNITS/$MEASUREMENT/$EXT* header and leaves the
// millimetre geometry untouched — coordinates are already in base units.
func TestDXFMetricHeaderAndExtents(t *testing.T) {
	s := newSketch(t) // defaults to Metric (mm)
	o := s.CreatePoint(5, 5)
	s.CreateCircle(o, 3)

	dxf, err := s.DXF()
	require.NoError(t, err)

	require.Contains(t, dxf, "$INSUNITS\n70\n4\n", "millimetre → INSUNITS 4")
	require.Contains(t, dxf, "$MEASUREMENT\n70\n1\n", "metric measurement flag")
	require.Contains(t, dxf, "$EXTMIN", "extents written when geometry present")
	require.Contains(t, dxf, "$EXTMAX")

	// Radius stays 3 mm; extents span the circle.
	require.InDelta(t, 3, mustFloat(t, dxfGroup(t, dxf, "CIRCLE", "40")), 1e-9)
	require.InDelta(t, 2, mustFloat(t, dxfGroup(t, dxf, "$EXTMIN", "10")), 1e-9) // 5-3
	require.InDelta(t, 8, mustFloat(t, dxfGroup(t, dxf, "$EXTMAX", "10")), 1e-9) // 5+3
}

// An imperial sketch reports $INSUNITS 1 (inches) and emits every length-valued
// field converted from base millimetres into inches — angles and the unitless
// fields are unaffected.
func TestDXFImperialLengthsConverted(t *testing.T) {
	s := newSketch(t)
	s.SetUnits(units.Imperial()) // inch / degree

	// Author at base millimetres: center 5 in, radius 3 in.
	o := s.CreatePoint(127, 127) // 5 in
	s.CreateCircle(o, 76.2)      // 3 in

	dxf, err := s.DXF()
	require.NoError(t, err)

	require.Contains(t, dxf, "$INSUNITS\n70\n1\n", "inch → INSUNITS 1")
	require.Contains(t, dxf, "$MEASUREMENT\n70\n0\n", "imperial measurement flag")

	require.InDelta(t, 5, mustFloat(t, dxfGroup(t, dxf, "CIRCLE", "10")), 1e-9, "center x in inches")
	require.InDelta(t, 5, mustFloat(t, dxfGroup(t, dxf, "CIRCLE", "20")), 1e-9, "center y in inches")
	require.InDelta(t, 3, mustFloat(t, dxfGroup(t, dxf, "CIRCLE", "40")), 1e-9, "radius in inches")
	require.InDelta(t, 2, mustFloat(t, dxfGroup(t, dxf, "$EXTMIN", "10")), 1e-9, "extents converted too")
}

// An arc's center/radius convert with the display unit, but its sweep angles
// (codes 50/51) are always degrees regardless of the length unit.
func TestDXFArcAnglesStayDegrees(t *testing.T) {
	s := newSketch(t)
	s.SetUnits(units.Imperial())
	o := s.CreatePoint(0, 0)
	st := s.CreatePoint(25.4, 0) // 1 in to the right → start angle 0°
	en := s.CreatePoint(0, 25.4) // 1 in up        → end angle 90°
	s.CreateArc(o, st, en)

	dxf, err := s.DXF()
	require.NoError(t, err)

	require.InDelta(t, 1, mustFloat(t, dxfGroup(t, dxf, "ARC", "40")), 1e-9, "radius converted to inches")
	require.InDelta(t, 0, mustFloat(t, dxfGroup(t, dxf, "ARC", "50")), 1e-6, "start angle in degrees")
	require.InDelta(t, 90, mustFloat(t, dxfGroup(t, dxf, "ARC", "51")), 1e-6, "end angle in degrees")
}

func mustFloat(t *testing.T, s string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(s, 64)
	require.NoErrorf(t, err, "parse %q", s)
	return v
}

// aaa reproduces AutoCAD's arbitrary-axis algorithm independently of the
// exporter, so a world-space round-trip reconstructs the OCS frame from the
// emitted extrusion direction and validates the encoding against Point.World().
func aaa(n r3.Vec) (r3.Vec, r3.Vec) {
	const tol = 1.0 / 64.0
	var a r3.Vec
	if math.Abs(n.X) < tol && math.Abs(n.Y) < tol {
		a = r3.NewVec(0, 1, 0).Cross(n)
	} else {
		a = r3.NewVec(0, 0, 1).Cross(n)
	}
	ax, _ := a.Normalize()
	ay, _ := n.Cross(ax).Normalize()
	return ax, ay
}

// tiltedSketch returns a sketch placed on a deliberately non-axis-aligned plane
// (so the OCS arbitrary-axis path is genuinely exercised).
func tiltedSketch(t *testing.T) *sketch.Sketch {
	t.Helper()
	fr, err := r3.NewFrame(
		r3.NewVec(10, -5, 7), // origin off the world origin
		r3.NewVec(1, 2, 0),   // u (orthonormalized by NewFrame)
		r3.NewVec(0, 1, 3),   // v
	)
	require.NoError(t, err)
	w := sketch.NewWorld()
	pl, err := w.CreatePlaneFromFrame(fr)
	require.NoError(t, err)
	s, err := w.CreateSketch(pl)
	require.NoError(t, err)
	return s
}

func worldVec(t *testing.T, dxf, marker, c10 string) r3.Vec {
	t.Helper()
	gx := mustFloat(t, dxfGroup(t, dxf, marker, c10))
	// c10 is "10"/"11"; its y/z partners are +10/+20.
	switch c10 {
	case "10":
		return r3.NewVec(gx, mustFloat(t, dxfGroup(t, dxf, marker, "20")), mustFloat(t, dxfGroup(t, dxf, marker, "30")))
	case "11":
		return r3.NewVec(gx, mustFloat(t, dxfGroup(t, dxf, marker, "21")), mustFloat(t, dxfGroup(t, dxf, marker, "31")))
	}
	t.Fatalf("unexpected base code %q", c10)
	return r3.Vec{}
}

func requireVecInDelta(t *testing.T, want, got r3.Vec, msg string) {
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
	a := s.CreatePoint(3, 4)
	b := s.CreatePoint(-2, 9)
	s.CreateLine(a, b)

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
	c := s.CreatePoint(6, -1)
	s.CreateCircle(c, 3.5)

	dxf, err := s.DXF(sketch.WithWorldSpace(true))
	require.NoError(t, err)

	ext := r3.NewVec(
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
	ctr := s.CreatePoint(0, 0)
	st := s.CreatePoint(4, 0)
	en := s.CreatePoint(0, 4)
	s.CreateArc(ctr, st, en)

	dxf, err := s.DXF(sketch.WithWorldSpace(true))
	require.NoError(t, err)

	ext := r3.NewVec(
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
	c := s.CreatePoint(2, 3)
	s.CreateEllipse(c, 5, 2, 0.3) // rx > ry → major = rx, no swap

	dxf, err := s.DXF(sketch.WithWorldSpace(true))
	require.NoError(t, err)

	requireVecInDelta(t, c.World(), worldVec(t, dxf, "ELLIPSE", "10"), "ellipse center")
	maj := worldVec(t, dxf, "ELLIPSE", "11")
	require.InDelta(t, 5, maj.Len(), 1e-5, "major-axis vector length is the major semi-axis")

	ext := r3.NewVec(
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
	c := s.CreatePoint(0, 0)
	start := s.CreatePoint(4, 0) // eccentric param 0
	end := s.CreatePoint(0, 2)   // eccentric param π/2
	s.CreateEllipticalArc(c, start, end, 4, 2, 0)

	dxf, err := s.DXF(sketch.WithWorldSpace(true))
	require.NoError(t, err)

	center := worldVec(t, dxf, "ELLIPSE", "10")
	major := worldVec(t, dxf, "ELLIPSE", "11")
	ext := r3.NewVec(
		mustFloat(t, dxfGroup(t, dxf, "ELLIPSE", "210")),
		mustFloat(t, dxfGroup(t, dxf, "ELLIPSE", "220")),
		mustFloat(t, dxfGroup(t, dxf, "ELLIPSE", "230")),
	)
	ratio := mustFloat(t, dxfGroup(t, dxf, "ELLIPSE", "40"))
	p41 := mustFloat(t, dxfGroup(t, dxf, "ELLIPSE", "41"))
	p42 := mustFloat(t, dxfGroup(t, dxf, "ELLIPSE", "42"))

	majHat, _ := major.Normalize()
	minor := ext.Cross(majHat).Scale(ratio * major.Len()) // DXF derives minor = ext × major̂
	recon := func(p float64) r3.Vec {
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
	c := s.CreatePoint(5, 7)
	s.CreateCircle(c, 2)

	dxf, err := s.DXF(sketch.WithWorldSpace(true))
	require.NoError(t, err)

	center := worldVec(t, dxf, "CIRCLE", "10")
	requireVecInDelta(t, r3.NewVec(5, 7, 0), center, "XY circle center unchanged")
	ext := r3.NewVec(
		mustFloat(t, dxfGroup(t, dxf, "CIRCLE", "210")),
		mustFloat(t, dxfGroup(t, dxf, "CIRCLE", "220")),
		mustFloat(t, dxfGroup(t, dxf, "CIRCLE", "230")),
	)
	requireVecInDelta(t, r3.NewVec(0, 0, 1), ext, "XY extrusion is +Z")
}

// The default (no option) output stays plane-local: a tilted sketch still emits
// 2D coordinates with z = 0 and no extrusion — backward compatible.
func TestDXFDefaultStaysLocal(t *testing.T) {
	s := tiltedSketch(t)
	c := s.CreatePoint(6, -1)
	s.CreateCircle(c, 3.5)

	dxf, err := s.DXF()
	require.NoError(t, err)

	require.InDelta(t, 6, mustFloat(t, dxfGroup(t, dxf, "CIRCLE", "10")), 1e-9, "local x")
	require.InDelta(t, -1, mustFloat(t, dxfGroup(t, dxf, "CIRCLE", "20")), 1e-9, "local y")
	require.NotContains(t, dxf, "210\n", "no extrusion in local mode")
}
