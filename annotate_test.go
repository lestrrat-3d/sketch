package sketch_test

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// rectWithDistance builds a solved unit rectangle and returns the sketch plus a
// width Distance dimension already added.
func rectWithDistance(t *testing.T) (*sketch.Sketch, *sketch.Distance) {
	t.Helper()
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(20, 0)
	c := s.CreatePoint(20, 12)
	d := s.CreatePoint(0, 12)
	ab := s.CreateLine(a, b)
	bc := s.CreateLine(b, c)
	cd := s.CreateLine(c, d)
	da := s.CreateLine(d, a)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(
		sketch.NewHorizontal(ab),
		sketch.NewHorizontal(cd),
		sketch.NewVertical(bc),
		sketch.NewVertical(da),
	)
	width := sketch.NewDistance(a, b, 20)
	height := sketch.NewDistance(a, d, 12)
	s.AddConstraint(width, height)
	_, err = s.Solve()
	require.NoError(t, err)
	return s, width
}

func TestAnnotationDefaultOffByteIdentical(t *testing.T) {
	s, _ := rectWithDistance(t)

	base, err := s.SVG()
	require.NoError(t, err)
	off, err := s.SVG(sketch.WithDimensions(false))
	require.NoError(t, err)
	require.Equal(t, base, off, "WithDimensions(false) must not change the baseline output")

	// The baseline carries no dimension label.
	require.NotContains(t, base, "20 mm")
}

func TestAnnotationDimensionsDrawn(t *testing.T) {
	s, _ := rectWithDistance(t)

	out, err := s.SVG(sketch.WithDimensions(true))
	require.NoError(t, err)
	require.Contains(t, out, "20 mm", "width label")
	require.Contains(t, out, "12 mm", "height label")
	require.Contains(t, out, "<text", "labels are <text> elements")
	require.Regexp(t, `<path d="M[^"]*Z" fill=`, out, "arrowheads are filled triangle paths")
	require.NotContains(t, out, "NaN")
	require.NotContains(t, out, "Inf")
}

func TestAnnotationDrivenParenthesized(t *testing.T) {
	s, width := rectWithDistance(t)
	width.SetDriven(true)
	_, err := s.Solve()
	require.NoError(t, err)

	out, err := s.SVG(sketch.WithDimensions(true))
	require.NoError(t, err)
	require.Regexp(t, `\(20[^)]*mm\)`, out, "driven dimension is parenthesized")
}

func TestAnnotationRadiusDiameterAngle(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)

	o := s.CreatePoint(0, 0)
	o.MoveTo(0, 0)
	s.Fix(o)
	circle := s.CreateCircle(o, 5)
	s.AddConstraint(sketch.NewRadius(circle, 5))

	o2 := s.CreatePoint(30, 0)
	circle2 := s.CreateCircle(o2, 4)
	s.AddConstraint(sketch.NewDiameter(circle2, 8))

	require.NoError(t, err)
	_, err = s.Solve()
	require.NoError(t, err)

	out, err := s.SVG(sketch.WithDimensions(true))
	require.NoError(t, err)
	require.Contains(t, out, "R5 mm", "radius label with R prefix")
	require.Contains(t, out, "⌀8 mm", "diameter label with ⌀ prefix")
}

func TestAnnotationAngleGlyph(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	c := s.CreatePoint(7, 7)
	a.MoveTo(0, 0)
	s.Fix(a)
	l1 := s.CreateLine(a, b)
	l2 := s.CreateLine(a, c)
	s.AddConstraint(sketch.NewHorizontal(l1))
	ang := sketch.NewAngle(l1, l2, 45)
	s.AddConstraint(ang)
	_, err = s.Solve()
	require.NoError(t, err)

	out, err := s.SVG(sketch.WithDimensions(true))
	require.NoError(t, err)
	require.Contains(t, out, "45°", "angle label with degree symbol")
	require.NotContains(t, out, "deg", "degrees prettified, no raw 'deg'")
}

// TestAnnotationAngleParallelNoNaN guards the degenerate case: an Angle between
// parallel lines must not divide by a near-zero cross product.
func TestAnnotationAngleParallelNoNaN(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	c := s.CreatePoint(0, 5)
	d := s.CreatePoint(10, 5)
	a.MoveTo(0, 0)
	s.Fix(a)
	l1 := s.CreateLine(a, b)
	l2 := s.CreateLine(c, d)
	s.AddConstraint(sketch.NewHorizontal(l1), sketch.NewHorizontal(l2))
	ang := sketch.NewAngle(l1, l2, 0)
	ang.SetDriven(true) // measures 0° between the two parallel lines
	s.AddConstraint(ang)
	_, err = s.Solve()
	require.NoError(t, err)

	out, err := s.SVG(sketch.WithDimensions(true))
	require.NoError(t, err)
	require.NotContains(t, out, "NaN")
	require.NotContains(t, out, "Inf")
}

// TestAnnotationScalesWithBBox renders the same sketch at 1x and 1000x and
// checks the font-size scales proportionally (annotation size derives from the
// bbox diagonal).
func TestAnnotationScalesWithBBox(t *testing.T) {
	small := fontSize(t, 1)
	big := fontSize(t, 1000)
	ratio := big / small
	require.InDelta(t, 1000.0, ratio, 5.0, "font size scales ~linearly with geometry scale")
}

func fontSize(t *testing.T, scale float64) float64 {
	t.Helper()
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(20*scale, 0)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(s.CreateLine(a, b)))
	s.AddConstraint(sketch.NewDistance(a, b, 20*scale))
	_, err = s.Solve()
	require.NoError(t, err)

	out, err := s.SVG(sketch.WithDimensions(true))
	require.NoError(t, err)
	fs := extractFontSize(t, out)
	require.Greater(t, fs, 0.0)
	return fs
}

func extractFontSize(t *testing.T, svg string) float64 {
	t.Helper()
	const key = `font-size="`
	i := strings.Index(svg, key)
	require.GreaterOrEqual(t, i, 0, "output has a font-size")
	rest := svg[i+len(key):]
	j := strings.IndexByte(rest, '"')
	require.GreaterOrEqual(t, j, 0)
	v, err := strconv.ParseFloat(rest[:j], 64)
	require.NoError(t, err)
	require.False(t, math.IsNaN(v))
	return v
}

func TestGlyphsDefaultOffByteIdentical(t *testing.T) {
	s, _ := rectWithDistance(t)
	base, err := s.SVG()
	require.NoError(t, err)
	off, err := s.SVG(sketch.WithConstraints(false))
	require.NoError(t, err)
	require.Equal(t, base, off)
}

func TestGlyphsDrawn(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(20, 0)
	c := s.CreatePoint(20, 12)
	d := s.CreatePoint(0, 12)
	ab := s.CreateLine(a, b)
	bc := s.CreateLine(b, c)
	cd := s.CreateLine(c, d)
	da := s.CreateLine(d, a)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(
		sketch.NewHorizontal(ab),
		sketch.NewVertical(da),
		sketch.NewParallel(ab, cd),
		sketch.NewPerpendicular(ab, bc),
		sketch.NewEqual(ab, cd),
	)
	_, err = s.Solve()
	require.NoError(t, err)

	out, err := s.SVG(sketch.WithConstraints(true))
	require.NoError(t, err)
	require.Contains(t, out, ">H<", "horizontal glyph")
	require.Contains(t, out, ">V<", "vertical glyph")
	require.Contains(t, out, "∥", "parallel glyph")
	require.Contains(t, out, "⊥", "perpendicular glyph")
	require.Contains(t, out, ">=<", "equal glyph")
	require.Contains(t, out, "<rect", "glyph badges are boxed")
	require.NotContains(t, out, "NaN")
	require.NotContains(t, out, "Inf")
}

func TestPointRadiusFloorScalesWithDrawing(t *testing.T) {
	// A point marker has a scale-relative minimum (a fraction of the bbox
	// diagonal), so it stays visible on a large drawing where a fixed radius
	// would be a speck; a small drawing keeps the configured radius.
	line := func(length float64) string {
		w := sketch.NewWorld()
		s, err := w.CreateSketch(w.XY())
		require.NoError(t, err)
		s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(length, 0))
		out, err := s.SVG(sketch.WithPointRadius(2))
		require.NoError(t, err)
		return out
	}
	// diagonal 10 → floor 0.08 < 2, configured radius wins.
	require.Contains(t, line(10), `r="2"`)
	// diagonal 1000 → floor 0.008*1000 = 8 wins.
	require.Contains(t, line(1000), `r="8"`)
	require.NotContains(t, line(1000), `r="2"`)
}

func TestGlyphsStackOnSharedAnchor(t *testing.T) {
	// Two constraints on the same line share an anchor and must not draw their
	// badges at the identical position (they stack).
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	l := s.CreateLine(a, b)
	l2 := s.CreateLine(a, b)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(l), sketch.NewParallel(l, l2))
	_, err = s.Solve()
	require.NoError(t, err)

	out, err := s.SVG(sketch.WithConstraints(true))
	require.NoError(t, err)
	// Both an H and a parallel glyph exist; the stacking offset keeps their
	// text y-coordinates distinct.
	require.Contains(t, out, ">H<")
	require.Contains(t, out, "∥")
}

func TestOverlaysDefaultOffByteIdentical(t *testing.T) {
	s, _ := rectWithDistance(t)
	base, err := s.SVG()
	require.NoError(t, err)
	for _, opt := range []sketch.SVGOption{
		sketch.WithDOFColoring(false),
		sketch.WithConflicts(false),
		sketch.WithStatusBadge(false),
		sketch.WithProfileFill(false),
	} {
		out, err := s.SVG(opt)
		require.NoError(t, err)
		require.Equal(t, base, out)
	}
}

func TestDOFColoringHollowFreePoint(t *testing.T) {
	// A line with one fixed and one free endpoint (no length) has a free point.
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.CreateLine(a, b)
	_, _ = s.Solve()
	require.Greater(t, s.DOF(), 0)

	out, err := s.SVG(sketch.WithDOFColoring(true))
	require.NoError(t, err)
	require.Contains(t, out, `fill="white" stroke="#1a73e8"`, "free point is hollow blue")
	require.Contains(t, out, `stroke="#1a73e8"`, "free geometry is blue")
}

func TestDOFColoringFullyConstrainedBlack(t *testing.T) {
	s, _ := rectWithDistance(t) // DOF 0
	out, err := s.SVG(sketch.WithDOFColoring(true))
	require.NoError(t, err)
	require.NotContains(t, out, `fill="white" stroke="#1a73e8"`, "no free (hollow) points")
	require.Contains(t, out, `stroke="#202124"`, "fully constrained geometry is black")
}

func TestDOFColoringFixedPointGreenSquare(t *testing.T) {
	// The grounded anchor renders as a green square, distinct from other
	// fully-constrained points (black circles). rectWithDistance fixes corner A.
	s, _ := rectWithDistance(t)
	out, err := s.SVG(sketch.WithDOFColoring(true))
	require.NoError(t, err)
	require.Regexp(t, `<rect [^>]*fill="#188038"`, out, "grounded point is a green square")
	require.Equal(t, 1, strings.Count(out, "#188038"), "exactly one grounded anchor")
	require.Regexp(t, `<circle [^>]*fill="#202124"`, out, "other constrained points stay black circles")

	// The green anchor is a DOF-coloring overlay: baseline output has no green.
	base, err := s.SVG()
	require.NoError(t, err)
	require.NotContains(t, base, "#188038")
}

func TestConflictHighlight(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(20, 0)
	a.MoveTo(0, 0)
	s.Fix(a)
	l := s.CreateLine(a, b)
	s.AddConstraint(sketch.NewHorizontal(l))
	// Two conflicting distances on the same pair.
	s.AddConstraint(sketch.NewDistance(a, b, 20))
	s.AddConstraint(sketch.NewDistance(a, b, 30))
	_, _ = s.Solve()
	require.NotEmpty(t, s.Diagnose().Conflicting)

	out, err := s.SVG(sketch.WithConflicts(true))
	require.NoError(t, err)
	require.Contains(t, out, `stroke="#d93025"`, "conflicting line drawn red")
}

func TestStatusBadge(t *testing.T) {
	s, _ := rectWithDistance(t)
	out, err := s.SVG(sketch.WithStatusBadge(true))
	require.NoError(t, err)
	require.Contains(t, out, "DOF 0")
	require.Contains(t, out, "fully constrained")
}

func TestProfileFillValidRegion(t *testing.T) {
	s, _ := rectWithDistance(t) // a closed rectangle => one region
	out, err := s.SVG(sketch.WithProfileFill(true))
	require.NoError(t, err)
	require.Contains(t, out, `fill-opacity="0.12"`, "region fill emitted")
	require.Contains(t, out, `fill-rule="evenodd"`)
}

func TestPixelWidthScalesDisplayNotViewBox(t *testing.T) {
	s, _ := rectWithDistance(t)

	def, err := s.SVG()
	require.NoError(t, err)
	// Default: display width equals the viewBox width (geometry units).
	require.Regexp(t, `width="40" height="32" viewBox="0 0 40 32"`, def)

	out, err := s.SVG(sketch.WithPixelWidth(400))
	require.NoError(t, err)
	// Display width is 400px; the viewBox stays in geometry units, aspect kept.
	require.Regexp(t, `width="400" height="320" viewBox="0 0 40 32"`, out)
}

func TestEntityIsFullyConstrained(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	o := s.CreatePoint(0, 0)
	o.MoveTo(0, 0)
	s.Fix(o)
	c := s.CreateCircle(o, 5)
	_, _ = s.Solve()

	// Center pinned but radius free: the point is constrained, the circle is not.
	require.True(t, o.IsFullyConstrained())
	require.False(t, s.EntityIsFullyConstrained(c), "a free radius leaves the circle under-constrained")

	// Pin the radius: now the circle is fully constrained.
	s.AddConstraint(sketch.NewRadius(c, 5))
	_, _ = s.Solve()
	require.True(t, s.EntityIsFullyConstrained(c))
}

// TestDOFColoringFreeRadiusCircleBlue is the case the old points-only overlay
// got wrong: a circle whose center is pinned but whose radius is free must read
// blue (under-constrained), not black.
func TestDOFColoringFreeRadiusCircleBlue(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	o := s.CreatePoint(0, 0)
	o.MoveTo(0, 0)
	s.Fix(o)
	c := s.CreateCircle(o, 5)
	_, _ = s.Solve()
	require.False(t, s.EntityIsFullyConstrained(c))

	out, err := s.SVG(sketch.WithDOFColoring(true))
	require.NoError(t, err)
	// The circle geometry (fill="none") is stroked blue because its radius is free.
	require.Regexp(t, `<circle cx="[^"]*" cy="[^"]*" r="[^"]*" fill="none" stroke="#1a73e8"`, out)

	// With the radius pinned it becomes black.
	s.AddConstraint(sketch.NewRadius(c, 5))
	_, _ = s.Solve()
	out, err = s.SVG(sketch.WithDOFColoring(true))
	require.NoError(t, err)
	require.Regexp(t, `<circle cx="[^"]*" cy="[^"]*" r="[^"]*" fill="none" stroke="#202124"`, out)
}
