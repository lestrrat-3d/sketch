package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// newSketch returns an empty sketch on a fresh world's XY datum — the standard
// way to obtain a sketch now that every sketch belongs to a World.
func newSketch(t *testing.T) *sketch.Sketch {
	t.Helper()
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	return s
}
