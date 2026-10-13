package sketch_test

import (
	"encoding/json"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestAttachmentBindsSketchPointerAndRevision(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	key := sketch.NewAttachmentKey[int]()
	var nilKey *sketch.AttachmentKey[int]
	require.False(t, nilKey.Attach(s, 1))
	require.False(t, key.Attach(nil, 1))
	_, ok := nilKey.Load(s)
	require.False(t, ok)
	_, ok = key.Load(nil)
	require.False(t, ok)
	require.True(t, key.Attach(s, 42))
	value, ok := key.Load(s)
	require.True(t, ok)
	require.Equal(t, 42, value)

	copySketch := *s
	_, ok = key.Load(&copySketch)
	require.False(t, ok)
	_, ok = sketch.NewAttachmentKey[int]().Load(s)
	require.False(t, ok)

	s.CreatePoint(1, 2)
	_, ok = key.Load(s)
	require.False(t, ok)
}

func TestAttachmentDoesNotCertifyReferenceStaleness(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	s.CreateReferencePoint(1, 2, "source")
	key := sketch.NewAttachmentKey[int]()
	require.True(t, key.Attach(s, 42))
	revision := s.Revision()
	s.MarkStale("source")
	require.Equal(t, revision, s.Revision())
	_, ok := key.Load(s)
	require.True(t, ok, "a consumer must inspect reference staleness separately")
}

func TestAttachmentClearsOnInPlaceJSONLoad(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	key := sketch.NewAttachmentKey[string]()
	require.True(t, key.Attach(s, "source"))
	data, err := json.Marshal(s)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, s))
	_, ok := key.Load(s)
	require.False(t, ok)
}
