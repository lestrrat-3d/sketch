package sketch_test

import (
	"encoding/json"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/space"
	"github.com/stretchr/testify/require"
)

func TestWorldJSONRoundTrip(t *testing.T) {
	w := sketch.NewWorld()
	off, err := w.CreateOffsetPlane(w.XY(), 5)
	require.NoError(t, err)
	s, err := w.CreateSketch(off)
	require.NoError(t, err)
	s.CreatePoint(3, 4)
	// A second sketch on the XZ datum, to exercise plane id references.
	s2, err := w.CreateSketch(w.XZ())
	require.NoError(t, err)
	s2.CreatePoint(1, 1)

	data, err := json.Marshal(w)
	require.NoError(t, err)
	require.Contains(t, string(data), `"kind":"world"`)

	var w2 sketch.World
	require.NoError(t, json.Unmarshal(data, &w2))
	require.Len(t, w2.Planes(), 4) // 3 datums + offset
	require.Len(t, w2.Sketches(), 2)

	worldVecEqual(t, space.NewVec3(3, 4, 5), w2.Sketches()[0].Points()[0].World())
	worldVecEqual(t, space.NewVec3(1, 0, 1), w2.Sketches()[1].Points()[0].World())
}

func TestWorldJSONFixedPoint(t *testing.T) {
	w := sketch.NewWorld()
	off, err := w.CreateOffsetPlane(w.XY(), 2.5)
	require.NoError(t, err)
	s, err := w.CreateSketch(off)
	require.NoError(t, err)
	s.CreatePoint(1, 2)

	data1, err := json.Marshal(w)
	require.NoError(t, err)
	var w2 sketch.World
	require.NoError(t, json.Unmarshal(data1, &w2))
	data2, err := json.Marshal(&w2)
	require.NoError(t, err)
	require.Equal(t, string(data1), string(data2), "world marshal∘unmarshal is a fixed point")
}

func TestStandaloneSketchPlaneRoundTrip(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XZ())
	require.NoError(t, err)
	s.CreatePoint(1, 1)
	data, err := json.Marshal(s)
	require.NoError(t, err)
	require.Contains(t, string(data), `"kind":"sketch"`)
	require.Contains(t, string(data), `"kind":"worldXZ"`)

	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	worldVecEqual(t, space.NewVec3(1, 0, 1), s2.Points()[0].World())
}

func TestSketchUnmarshalRejectsWorldDocument(t *testing.T) {
	w := sketch.NewWorld()
	_, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	data, err := json.Marshal(w)
	require.NoError(t, err)

	var s sketch.Sketch
	require.ErrorIs(t, json.Unmarshal(data, &s), sketch.ErrWrongDocumentKind)
}

func TestWorldUnmarshalRejectsSketchDocument(t *testing.T) {
	s := newSketch(t)
	s.CreatePoint(0, 0)
	data, err := json.Marshal(s)
	require.NoError(t, err)

	var w sketch.World
	require.ErrorIs(t, json.Unmarshal(data, &w), sketch.ErrWrongDocumentKind)
}

func TestMissingKindWithV2KeyRejected(t *testing.T) {
	base := map[string]any{
		"version":     1,
		"points":      []any{},
		"entities":    []any{},
		"constraints": []any{},
	}
	for _, key := range []string{"plane", "planes", "sketches"} {
		doc := map[string]any{}
		for k, v := range base {
			doc[k] = v
		}
		doc[key] = map[string]any{"kind": "worldXY"} // shape irrelevant; presence is the trigger
		data, err := json.Marshal(doc)
		require.NoError(t, err)
		var s sketch.Sketch
		require.ErrorIsf(t, json.Unmarshal(data, &s), sketch.ErrWrongDocumentKind,
			"legacy doc carrying %q must be rejected", key)
	}
}

func TestV2SketchRequiresPlane(t *testing.T) {
	doc := map[string]any{
		"kind":        "sketch",
		"version":     2,
		"points":      []any{},
		"entities":    []any{},
		"constraints": []any{},
		// no "plane"
	}
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	var s sketch.Sketch
	require.ErrorIs(t, json.Unmarshal(data, &s), sketch.ErrMissingPlane)
}

func TestLegacyDocumentLoadsAsWorldXY(t *testing.T) {
	doc := map[string]any{
		"version":     1,
		"points":      []any{map[string]any{"x": 3.0, "y": 4.0}},
		"entities":    []any{},
		"constraints": []any{},
	}
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	var s sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s))
	worldVecEqual(t, space.NewVec3(3, 4, 0), s.Points()[0].World())
}

func TestStandaloneDerivedPlaneRejected(t *testing.T) {
	doc := map[string]any{
		"kind":        "sketch",
		"version":     2,
		"points":      []any{},
		"entities":    []any{},
		"constraints": []any{},
		"plane":       map[string]any{"kind": "offset", "base_id": 0, "dist": 5.0},
	}
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	var s sketch.Sketch
	require.Error(t, json.Unmarshal(data, &s))
}

func TestWorldSketchMissingPlaneRejected(t *testing.T) {
	datums := []any{
		map[string]any{"kind": "worldXY"},
		map[string]any{"kind": "worldXZ"},
		map[string]any{"kind": "worldYZ"},
	}
	doc := map[string]any{
		"kind":    "world",
		"version": 2,
		"planes":  datums,
		"sketches": []any{
			map[string]any{ // no "plane" reference
				"points": []any{}, "entities": []any{}, "constraints": []any{},
			},
		},
	}
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	var w sketch.World
	require.ErrorIs(t, json.Unmarshal(data, &w), sketch.ErrMissingPlane,
		"a world sketch with no plane must be rejected, not silently placed on XY")
}

func TestMalformedReferencesRejected(t *testing.T) {
	base := func(entities, constraints []any) []byte {
		doc := map[string]any{
			"kind": "sketch", "version": 2,
			"points":      []any{map[string]any{"x": 0.0, "y": 0.0}},
			"entities":    entities,
			"constraints": constraints,
			"plane":       map[string]any{"kind": "worldXY"},
		}
		data, err := json.Marshal(doc)
		require.NoError(t, err)
		return data
	}

	// Entity referencing an out-of-range point id must error, not panic.
	badEntity := base([]any{map[string]any{"type": "line", "points": []any{0, 99}}}, []any{})
	var s1 sketch.Sketch
	require.Error(t, json.Unmarshal(badEntity, &s1))

	// Constraint referencing an out-of-range point id must error, not panic.
	badConstraint := base([]any{}, []any{map[string]any{"type": "coincident", "points": []any{0, 42}}})
	var s2 sketch.Sketch
	require.Error(t, json.Unmarshal(badConstraint, &s2))

	// Constraint with too few arguments must error, not panic.
	tooFew := base([]any{}, []any{map[string]any{"type": "coincident", "points": []any{0}}})
	var s3 sketch.Sketch
	require.Error(t, json.Unmarshal(tooFew, &s3))

	// Constraint with too many arguments must error (arity is exact, so loads
	// stay a marshal fixed point).
	tooMany := base([]any{}, []any{map[string]any{"type": "coincident", "points": []any{0, 0, 0}}})
	var s4 sketch.Sketch
	require.Error(t, json.Unmarshal(tooMany, &s4))
}

func TestWorldForwardBaseIDRejected(t *testing.T) {
	w := sketch.NewWorld()
	off, err := w.CreateOffsetPlane(w.XY(), 1)
	require.NoError(t, err)
	_, err = w.CreateSketch(off)
	require.NoError(t, err)
	data, err := json.Marshal(w)
	require.NoError(t, err)

	// Tamper: point the offset plane's base at a forward (not-yet-built) plane.
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))
	planes := doc["planes"].([]any)
	planes[3].(map[string]any)["base_id"] = 99
	tampered, err := json.Marshal(doc)
	require.NoError(t, err)

	var w2 sketch.World
	require.ErrorContains(t, json.Unmarshal(tampered, &w2), "earlier plane")
}
