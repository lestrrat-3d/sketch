package sketchtest_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
)

func newSketch(tb testing.TB) *sketch.Sketch {
	tb.Helper()

	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	if err != nil {
		tb.Fatalf("create test sketch: %v", err)
		return nil
	}
	return s
}

func constrainedRectangle(tb testing.TB) (*sketch.Sketch, *sketch.Rectangle, sketch.Constraint) {
	tb.Helper()

	s := newSketch(tb)
	rect := s.CreateRectangle(0, 0, 20, 12)
	width := sketch.NewDistance(rect.A, rect.B, 20)
	s.AddConstraint(
		sketch.NewCoincident(rect.A, s.Origin()),
		width,
		sketch.NewDistance(rect.A, rect.D, 12),
	)
	return s, rect, width
}
