package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/units"
	"github.com/stretchr/testify/require"
)

// The bounding-box guard (bbox.finite, and its non-finite-knot regression in
// nonfinite_export_test.go) is a PRECONDITION over the geometry that produced
// the box, not a postcondition over what an exporter actually writes. A
// bounding box can be finite by that check while the value the exporter
// writes still is not: the box's own SPAN can overflow after the check ran
// (WithMargin/WithPixelWidth/WithScale multiplying a finite box), or a later
// unit conversion can overflow a finite coordinate with no span arithmetic
// involved at all (DXF's display-unit conversion). These tests reach the
// guard now moved into the formatters themselves (svgWriter.f, DXF's pairf,
// and PNG's pre-conversion pixel-dimension check).

// maxFloatLineSketch builds the sharpest, simplest reproduction: two ordinary
// points at +/-math.MaxFloat64 joined by a line. CreatePoint validates
// nothing and has no error return, so no NURBS or foreign-sketch machinery is
// needed to reach it — two CreatePoint calls and one CreateLine are enough.
// The two corners are individually finite, so bbox.finite() passes, but their
// span (maxX-minX = 2*MaxFloat64) overflows to +Inf inside renderBounds.
func maxFloatLineSketch(t *testing.T) *sketch.Sketch {
	t.Helper()
	s := newSketch(t)
	a := s.CreatePoint(math.MaxFloat64, math.MaxFloat64)
	b := s.CreatePoint(-math.MaxFloat64, -math.MaxFloat64)
	s.CreateLine(a, b)
	return s
}

// TestSVGRefusesMaxFloat64LineSpanOverflow pins the reported case: the SVG
// exporter must return the sentinel rather than a document whose width/height
// attributes are the literal token "+Inf".
func TestSVGRefusesMaxFloat64LineSpanOverflow(t *testing.T) {
	s := maxFloatLineSketch(t)
	out, err := s.SVG()
	require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)
	require.Empty(t, out)
}

// TestPNGRefusesMaxFloat64LineSpanOverflow doubles as a no-panic regression:
// on the pre-fix code the same +Inf span drove scale to 0 via
// pngFitLongSide/+Inf, and Inf*0 is NaN, so int(math.Max(1, math.Round(NaN)))
// fed an undefined pixel dimension straight into image.NewNRGBA — the same
// crash the PR's own headline (NURBS-knot) case exists to fix, reached
// through the simplest possible input instead.
func TestPNGRefusesMaxFloat64LineSpanOverflow(t *testing.T) {
	s := maxFloatLineSketch(t)
	var (
		data []byte
		err  error
	)
	require.NotPanics(t, func() {
		data, err = s.PNG()
	}, "a non-finite pixel dimension must be refused before it reaches image.Rect, never panic")
	require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)
	require.Nil(t, data)
}

// TestSVGRefusesMarginOverflowOnOrdinaryGeometry is the case a span-only fix
// (checking only b.maxX-b.minX etc.) would still miss: ordinary, entirely
// finite 10x10 geometry whose bounding box AND span are both perfectly finite.
// The overflow happens later still, inside renderBounds's own
// w := (b.maxX-b.minX) + 2*margin, when an enormous WithMargin blows the
// already-finite span past float64's range.
func TestSVGRefusesMarginOverflowOnOrdinaryGeometry(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 10)
	s.CreateLine(a, b)

	out, err := s.SVG(sketch.WithMargin(1e308))
	require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)
	require.Empty(t, out)
}

// TestSVGRefusesPixelWidthOverflowOnTallSketch is the case any box-DERIVED
// fix (one that inspects the bounding box or its span, however it computes
// them) would still miss: every box-derived value here — corners, span,
// margin-padded w/h — is perfectly finite. The overflow happens two steps
// later, in outH := cfg.pixelWidth * canvasH / canvasW: a sketch far taller
// than it is wide multiplies a huge (but finite) height ratio by the
// requested pixel width and only THEN leaves float64's range.
func TestSVGRefusesPixelWidthOverflowOnTallSketch(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(1, 9e307)
	s.CreateLine(a, b)

	out, err := s.SVG(sketch.WithPixelWidth(1000))
	require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)
	require.Empty(t, out)
}

// TestPNGRefusesScaleOverflowOnOrdinaryGeometry pins the int-conversion guard
// specifically: WithScale(1e300) on ordinary, entirely finite 10x10 geometry
// never produces a NaN or an infinity anywhere — w*scale is a huge but
// perfectly finite float64 — so this is caught only by finitePixelDim's
// int-range check, not by any NaN/Inf test.
func TestPNGRefusesScaleOverflowOnOrdinaryGeometry(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 10)
	s.CreateLine(a, b)

	var (
		data []byte
		err  error
	)
	require.NotPanics(t, func() {
		data, err = s.PNG(sketch.WithScale(1e300))
	}, "a pixel dimension outside int range must be refused before it reaches image.Rect, never panic")
	require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)
	require.Nil(t, data)
}

// TestDXFRefusesThouUnitConversionOverflow is the gap bounds()-based checks
// cannot close at all: DXF emits every length in the sketch's DISPLAY length
// unit via units.FromBase, and that conversion runs after bounds() has
// already been sampled. A MaxFloat64 (millimetre) coordinate is an ordinary
// finite bbox corner and an ordinary finite span, but dividing it by a thou's
// 0.0254 mm factor overflows float64 on its own, with no margin/scale/pixel
// arithmetic anywhere in the picture.
func TestDXFRefusesThouUnitConversionOverflow(t *testing.T) {
	s := newSketch(t)
	s.SetUnits(units.System{Length: units.Thou, Angle: units.Degree})
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(math.MaxFloat64, math.MaxFloat64)
	s.CreateLine(a, b)

	out, err := s.DXF()
	require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)
	require.Empty(t, out)
}

// TestDXFWorldSpaceRefusesOCSProjectionOverflow is another gap a bounds()-based
// precondition cannot close: on a tilted plane (tiltedSketch, shared with
// dxf_worldspace_test.go), a circle centered at (MaxFloat64, MaxFloat64) has a
// perfectly finite bounding box in LOCAL coordinates — bbox.finite() passes —
// but WithWorldSpace(true) routes CIRCLE/ARC through putOCS's arbitrary-axis
// projection (w.Dot(ax)/w.Dot(ay)), and that dot product overflows float64 on
// its own, after the precondition already ran clean. A LINE-only sketch does
// not reach this: putWCS writes World() coordinates directly and stays finite
// at the same values, so the case needs a circle (or arc) to reach putOCS.
func TestDXFWorldSpaceRefusesOCSProjectionOverflow(t *testing.T) {
	s := tiltedSketch(t)
	c := s.CreatePoint(math.MaxFloat64, math.MaxFloat64)
	s.CreateCircle(c, 3.5)

	local, err := s.DXF()
	require.NoError(t, err, "local mode never reaches putOCS's projection")
	require.NotContains(t, local, "NaN")
	require.NotContains(t, local, "Inf")
	require.NotContains(t, local, "+Inf")
	require.NotContains(t, local, "-Inf")

	world, err := s.DXF(sketch.WithWorldSpace(true))
	require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)
	require.Empty(t, world)
}

// TestPNGRefusesPixelBufferOverflowOnOrdinaryGeometry pins the pixel-BUFFER
// guard specifically, distinct from finitePixelDim's per-dimension check:
// WithScale(7e7) on ordinary, entirely finite 10x10 geometry (bounds 0..10
// plus the default margin 10 per side = 30 units) gives pw = ph = 30*7e7 =
// 2.1e9, individually inside int32, so finitePixelDim alone passes both — it
// does not start refusing this geometry until scale 7.1583e7 (MaxInt32/30).
// Their product, at 4 bytes/pixel, is ~1.76e19 bytes: past pngMaxPixelBytes,
// and past MaxInt64, which is where image.NewNRGBA's own overflow check
// (mul3NonNeg) panics instead of allocating — a recoverable panic, so a test
// may sit here. The scale is deliberately ABOVE the regime where the byte
// count fits int64 but not memory, which instead reaches make() and dies with
// the runtime's unrecoverable out-of-memory fatal error that no recover()
// catches — that regime must never be exercised by a test. The byte count
// here is 3600*scale^2, so it stops fitting int64 at scale 5.0617e7: a case
// like this one must sit in the interval (5.0617e7, 7.1583e7).
func TestPNGRefusesPixelBufferOverflowOnOrdinaryGeometry(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 10)
	s.CreateLine(a, b)

	var (
		data []byte
		err  error
	)
	require.NotPanics(t, func() {
		data, err = s.PNG(sketch.WithScale(7e7))
	}, "a pixel buffer past the byte budget must be refused before image.NewNRGBA allocates, never panic")
	require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)
	require.Nil(t, data)
}

// TestPNGControlSaneScaleStillSucceeds is the control for the pixel-buffer
// guard: an ordinary scale, orders of magnitude under pngMaxPixelBytes, on
// the same geometry must still render — the new guard is not over-broad.
func TestPNGControlSaneScaleStillSucceeds(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 10)
	s.CreateLine(a, b)

	data, err := s.PNG(sketch.WithScale(10))
	require.NoError(t, err)
	require.NotEmpty(t, data)
}

// TestExportersControlOrdinaryGeometryUnaffected is the control every case
// above needs: with none of the extreme options, all three exporters still
// produce ordinary non-empty output for ordinary geometry, so the refusal
// added by this fix is not over-broad.
func TestExportersControlOrdinaryGeometryUnaffected(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 10)
	s.CreateLine(a, b)

	svg, err := s.SVG()
	require.NoError(t, err)
	require.NotEmpty(t, svg)

	png, err := s.PNG()
	require.NoError(t, err)
	require.NotEmpty(t, png)

	dxf, err := s.DXF()
	require.NoError(t, err)
	require.NotEmpty(t, dxf)
}

// nonFiniteDimensionSketch builds a sketch with one dimensional constraint
// (a plain point-to-point Distance) whose target is then driven non-finite
// through the public Set API — the only reachable route to a non-finite
// dimension label: a bound expression poisons the geometry so the bbox check
// refuses first, and a driven dimension is refreshed from measurement.
func nonFiniteDimensionSketch(t *testing.T, mag float64) *sketch.Sketch {
	t.Helper()
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	d := sketch.NewDistance(a, b, 10)
	s.AddConstraint(d)
	d.Set(mag)
	return s
}

// TestSVGRefusesNonFiniteDimensionLabel pins the reported bypass: a
// dimension's value label is built through units.Value.String, not
// svgWriter.f, so a NaN/Inf target rendered without the fix as the literal
// text "NaN mm"/"+Inf mm" inside a <text> element while SVG returned a nil
// error. The label's magnitude must now be screened before it is formatted,
// so WithDimensions refuses exactly like every other non-finite-geometry
// case.
func TestSVGRefusesNonFiniteDimensionLabel(t *testing.T) {
	s := nonFiniteDimensionSketch(t, math.NaN())
	out, err := s.SVG(sketch.WithDimensions(true))
	require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)
	require.Empty(t, out)
}

// TestSVGRefusesInfiniteDimensionLabel is the +Inf half of the same bypass.
func TestSVGRefusesInfiniteDimensionLabel(t *testing.T) {
	s := nonFiniteDimensionSketch(t, math.Inf(1))
	out, err := s.SVG(sketch.WithDimensions(true))
	require.ErrorIs(t, err, sketch.ErrNonFiniteGeometry)
	require.Empty(t, out)
}

// TestSVGDimensionLabelOrdinaryUnaffected is the control the two cases above
// need: an ordinary finite dimension still renders successfully with
// WithDimensions, and the fix (screening the magnitude before formatting it)
// does not alter that output.
func TestSVGDimensionLabelOrdinaryUnaffected(t *testing.T) {
	s := nonFiniteDimensionSketch(t, 15)
	out, err := s.SVG(sketch.WithDimensions(true))
	require.NoError(t, err)
	require.NotEmpty(t, out)
	require.Contains(t, out, "15 mm")
}
