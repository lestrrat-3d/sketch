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

// textAt returns every drawn name, in document order.
//
// A name is written twice, once as a thickened copy in the page's own colour
// that clears the drawing under the letters and once as the letters themselves.
// The halo copy is the one carrying a stroke, and it is skipped here: it says
// nothing the name itself does not.
func textAt(t *testing.T, svg string) []drawnText {
	t.Helper()
	re := regexp.MustCompile(`<text x="([^"]*)" y="([^"]*)"([^>]*)text-anchor="([^"]*)"([^>]*)>([^<]*)</text>`)
	var out []drawnText
	for _, m := range re.FindAllStringSubmatch(svg, -1) {
		if strings.Contains(m[3], `stroke="`) {
			continue
		}
		x, err := strconv.ParseFloat(m[1], 64)
		require.NoError(t, err)
		y, err := strconv.ParseFloat(m[2], 64)
		require.NoError(t, err)
		out = append(out, drawnText{
			X: x, Y: y, Anchor: m[4],
			Italic: strings.Contains(m[5], `font-style="italic"`),
			Text:   m[6],
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

// The two kinds of label are told apart by style, so a drawing carrying both
// says which word names a point and which names an edge. Style is the channel
// that survives the placement search: where a name ends up depends on what else
// is on the page, but an entity's name is italic wherever it lands.
func TestLabelsDistinguishPointsFromEntities(t *testing.T) {
	s := namedTriangle(t)

	out, err := s.SVG(sketch.WithLabels(true))
	require.NoError(t, err)
	drawn := textAt(t, out)

	for _, name := range []string{"A", "B", "C"} {
		require.False(t, labelled(t, drawn, name).Italic, "a point's name is upright")
	}
	require.True(t, labelled(t, drawn, "hypotenuse").Italic, "an entity's name is italic")
}

// An uncrowded drawing keeps each name at its first choice: a point's up and to
// the right of its marker, an entity's centred on its own anchor.
func TestLabelsKeepTheirFirstChoiceWhenThereIsRoom(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(60, 0)
	a.SetName("A")
	b.SetName("B")
	s.Fix(a)
	s.Fix(b)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)

	out, err := s.SVG(sketch.WithLabels(true))
	require.NoError(t, err)
	drawn := textAt(t, out)
	require.Len(t, drawn, 2)

	// B is 60 mm along +x from A and nothing else is on the page, so both names
	// take the same offset from their own marker and stay level with each other.
	la, lb := labelled(t, drawn, "A"), labelled(t, drawn, "B")
	require.Equal(t, "start", la.Anchor)
	require.InDelta(t, la.X+60, lb.X, 1e-6, "each name is offset from its own marker by the same amount")
	require.InDelta(t, la.Y, lb.Y, 1e-6)
}

// Two names that want the same spot do not print over each other: the second
// one searched for is moved to a position the first left free.
func TestLabelsMoveOffEachOther(t *testing.T) {
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
	require.NotEqual(t, [2]float64{drawn[0].X, drawn[0].Y}, [2]float64{drawn[1].X, drawn[1].Y},
		"the two names are on the same anchor and must not be drawn on the same spot")
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
