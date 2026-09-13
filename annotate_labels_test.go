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

// drawnText is one <text> element read back off the rendered document.
type drawnText struct {
	X, Y   float64
	Anchor string
	Italic bool
	Text   string
}

// textAt returns every <text> element, in document order.
func textAt(t *testing.T, svg string) []drawnText {
	t.Helper()
	re := regexp.MustCompile(`<text x="([^"]*)" y="([^"]*)"[^>]*text-anchor="([^"]*)"([^>]*)>([^<]*)</text>`)
	var out []drawnText
	for _, m := range re.FindAllStringSubmatch(svg, -1) {
		x, err := strconv.ParseFloat(m[1], 64)
		require.NoError(t, err)
		y, err := strconv.ParseFloat(m[2], 64)
		require.NoError(t, err)
		out = append(out, drawnText{
			X: x, Y: y, Anchor: m[3],
			Italic: strings.Contains(m[4], `font-style="italic"`),
			Text:   m[5],
		})
	}
	return out
}

// names returns the text of each drawn label, in document order.
func names(drawn []drawnText) []string {
	out := make([]string, 0, len(drawn))
	for _, d := range drawn {
		out = append(out, d.Text)
	}
	return out
}

// labelled returns the one label with the given text.
func labelled(t *testing.T, drawn []drawnText, name string) drawnText {
	t.Helper()
	for _, d := range drawn {
		if d.Text == name {
			return d
		}
	}
	t.Fatalf("no label %q in %v", name, names(drawn))
	return drawnText{}
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

	require.Equal(t, []string{"A", "B", "C", "hypotenuse"}, names(textAt(t, out)),
		"every named point in creation order, then every named entity")
	require.NotContains(t, out, "NaN")
	require.NotContains(t, out, "Inf")
}

// The two kinds of label are told apart by position and by style, so a drawing
// carrying both says which word names a point and which names an edge.
func TestLabelsDistinguishPointsFromEntities(t *testing.T) {
	s := namedTriangle(t)

	out, err := s.SVG(sketch.WithLabels(true))
	require.NoError(t, err)
	drawn := textAt(t, out)

	for _, name := range []string{"A", "B", "C"} {
		p := labelled(t, drawn, name)
		require.Equal(t, "start", p.Anchor, "a point's name is hung to one side of its marker")
		require.False(t, p.Italic, "a point's name is upright")
	}

	e := labelled(t, drawn, "hypotenuse")
	require.Equal(t, "middle", e.Anchor, "an entity's name is centred on its anchor")
	require.True(t, e.Italic, "an entity's name is italic")
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
	drawn := textAt(t, out)
	require.Len(t, drawn, 4)

	// B is 40 mm along +x from A, and C is 30 mm along +y. The y axis is flipped
	// on the way to screen space, so C's label is ABOVE A's and B's is to its
	// right, and each is offset off its own marker by the same amount.
	a := labelled(t, drawn, "A")
	b := labelled(t, drawn, "B")
	c := labelled(t, drawn, "C")
	require.InDelta(t, a.X+40, b.X, 1e-6, "B's label is 40 mm right of A's")
	require.InDelta(t, a.Y, b.Y, 1e-6, "A and B are level")
	require.InDelta(t, a.X, c.X, 1e-6, "A and C are in line")
	require.InDelta(t, a.Y-30, c.Y, 1e-6, "C's label is 30 mm above A's, the y axis being flipped")

	// The hypotenuse runs B->C, so its name is centred on the midpoint of those
	// two points. Its label therefore sits at the midpoint of B's and C's labels
	// LESS the offset those two carry — right by the same amount they were moved
	// left, and down by the same amount they were moved up. Asserting the two
	// differences against each other pins both the anchor and the offset without
	// naming the offset's value.
	h := labelled(t, drawn, "hypotenuse")
	dx := (b.X+c.X)/2 - h.X
	dy := h.Y - (b.Y+c.Y)/2
	require.InDelta(t, dx, dy, 1e-6, "a point's label is offset equally right and up")
	require.Greater(t, dx, 0.0, "and the offset is away from the marker, not onto it")
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
	drawn := textAt(t, out)
	require.Equal(t, []string{"first", "second"}, names(drawn))
	require.InDelta(t, drawn[0].X, drawn[1].X, 1e-9, "both sit at the same x")
	require.Greater(t, drawn[1].Y, drawn[0].Y, "the second name is stacked below the first")
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
