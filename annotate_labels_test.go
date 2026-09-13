package sketch_test

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// namedTriangle builds a solved right triangle whose three corners and one
// hypotenuse carry names, and leaves the two legs unnamed.
func namedTriangle(t *testing.T) *sketch.Sketch {
	t.Helper()
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(40, 0)
	c := s.CreatePoint(0, 30)
	a.SetName("A")
	b.SetName("B")
	c.SetName("C")
	s.CreateLine(a, b)
	s.CreateLine(a, c)
	s.CreateLine(b, c).SetName("hypotenuse")
	s.Fix(a)
	s.Fix(b)
	s.Fix(c)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	return s
}

// textAt returns every <text> element's x, y and content, in document order.
func textAt(t *testing.T, svg string) [][3]string {
	t.Helper()
	re := regexp.MustCompile(`<text x="([^"]*)" y="([^"]*)"[^>]*>([^<]*)</text>`)
	var out [][3]string
	for _, m := range re.FindAllStringSubmatch(svg, -1) {
		out = append(out, [3]string{m[1], m[2], m[3]})
	}
	return out
}

func TestLabelsDefaultOffByteIdentical(t *testing.T) {
	s := namedTriangle(t)

	base, err := s.SVG()
	require.NoError(t, err)
	off, err := s.SVG(sketch.WithLabels(false))
	require.NoError(t, err)
	require.Equal(t, base, off, "WithLabels(false) must not change the baseline output")

	// The names are in the sketch, so the baseline carrying none of them is what
	// makes the option opt-in rather than a default that was already on.
	require.NotContains(t, base, "<text")
	require.Equal(t, "A", s.PointByName("A").Name())
}

func TestLabelsDrawn(t *testing.T) {
	s := namedTriangle(t)

	out, err := s.SVG(sketch.WithLabels(true))
	require.NoError(t, err)

	texts := textAt(t, out)
	var drawn []string
	for _, tx := range texts {
		drawn = append(drawn, tx[2])
	}
	require.Equal(t, []string{"A", "B", "C", "hypotenuse"}, drawn,
		"every named point in creation order, then every named entity")
	require.NotContains(t, out, "NaN")
	require.NotContains(t, out, "Inf")
}

func TestLabelsSkipUnnamedGeometry(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	s.CreateLine(a, b)
	s.Fix(a)
	s.Fix(b)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)

	out, err := s.SVG(sketch.WithLabels(true))
	require.NoError(t, err)
	require.NotContains(t, out, "<text", "nothing is named, so nothing is labelled")
}

// A point's label is drawn beside its marker rather than on it, and an entity's
// at the mean of the points that define it — the midpoint for a line.
func TestLabelsSitBesideTheirGeometry(t *testing.T) {
	s := namedTriangle(t)

	out, err := s.SVG(sketch.WithLabels(true))
	require.NoError(t, err)
	texts := textAt(t, out)
	require.Len(t, texts, 4)

	at := func(name string) (float64, float64) {
		t.Helper()
		for _, tx := range texts {
			if tx[2] != name {
				continue
			}
			x, err := strconv.ParseFloat(tx[0], 64)
			require.NoError(t, err)
			y, err := strconv.ParseFloat(tx[1], 64)
			require.NoError(t, err)
			return x, y
		}
		t.Fatalf("no label %q in %v", name, texts)
		return 0, 0
	}

	// B is 40 mm along +x from A, and C is 30 mm along +y. The y axis is flipped
	// on the way to screen space, so C's label is ABOVE A's and B's is to its
	// right, and neither sits on top of its own marker.
	ax, ay := at("A")
	bx, by := at("B")
	cx, cy := at("C")
	require.InDelta(t, ax+40, bx, 1e-6, "B's label is 40 mm right of A's")
	require.InDelta(t, ay, by, 1e-6, "A and B are level")
	require.InDelta(t, ax, cx, 1e-6, "A and C are in line")
	require.InDelta(t, ay-30, cy, 1e-6, "C's label is 30 mm above A's, the y axis being flipped")

	// The hypotenuse runs B->C, so its label sits at that line's midpoint, offset
	// the same way a point's is.
	hx, hy := at("hypotenuse")
	require.InDelta(t, (bx+cx)/2, hx, 1e-6)
	require.InDelta(t, (by+cy)/2, hy, 1e-6)
}

// Two names on one anchor stack downward instead of printing over each other.
func TestLabelsStackOnASharedAnchor(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(20, 0)
	s.CreateLine(a, b).SetName("first")
	s.CreateLine(a, b).SetName("second")
	s.Fix(a)
	s.Fix(b)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)

	out, err := s.SVG(sketch.WithLabels(true))
	require.NoError(t, err)
	texts := textAt(t, out)
	require.Len(t, texts, 2)
	require.Equal(t, "first", texts[0][2])
	require.Equal(t, "second", texts[1][2])
	require.Equal(t, texts[0][0], texts[1][0], "both sit at the same x")

	firstY, err := strconv.ParseFloat(texts[0][1], 64)
	require.NoError(t, err)
	secondY, err := strconv.ParseFloat(texts[1][1], 64)
	require.NoError(t, err)
	require.Greater(t, secondY, firstY, "the second name is stacked below the first")
}

// A name is caller-supplied text, so it goes through the same escaping every
// other string in the document does.
func TestLabelsEscapeTheName(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	p := s.CreatePoint(0, 0)
	p.SetName(`A & <B>`)
	s.Fix(p)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)

	out, err := s.SVG(sketch.WithLabels(true))
	require.NoError(t, err)
	require.Contains(t, out, "A &amp; &lt;B&gt;")
	require.NotContains(t, out, "<B>")
}

// The PNG rasterizer draws geometry and point markers and no annotations, so
// the option reaches it without changing a pixel. Pinned because the option's
// own type says SVG and PNG both accept it.
func TestLabelsAreSVGOnly(t *testing.T) {
	s := namedTriangle(t)

	base, err := s.PNG()
	require.NoError(t, err)
	labelled, err := s.PNG(sketch.WithLabels(true))
	require.NoError(t, err)
	require.Equal(t, base, labelled)
}

// Labels and constraint glyphs are separate passes over the same drawing, so a
// sketch that asks for both gets both.
func TestLabelsCoexistWithGlyphs(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(20, 0)
	ab := s.CreateLine(a, b)
	ab.SetName("base")
	s.AddConstraint(sketch.NewHorizontal(ab))
	s.Fix(a)
	s.Fix(b)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)

	out, err := s.SVG(sketch.WithLabels(true), sketch.WithConstraints(true))
	require.NoError(t, err)
	require.Contains(t, out, ">H<", "the horizontal glyph")
	require.Contains(t, out, ">base<", "the line's name")
	require.Greater(t, strings.Index(out, ">base<"), strings.Index(out, ">H<"),
		"names render after glyphs, so a shared anchor leaves the name on top")
}
