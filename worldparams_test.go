package sketch_test

import (
	"encoding/json"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/param"
	"github.com/lestrrat-3d/sketch/units"
	"github.com/stretchr/testify/require"
)

func TestWorldSharedParameterTable(t *testing.T) {
	w := sketch.NewWorld()
	s1, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	s2, err := w.CreateSketch(w.XZ())
	require.NoError(t, err)
	require.Same(t, w.Params(), s1.Params(), "a world sketch shares the world's table")
	require.Same(t, s1.Params(), s2.Params(), "all world sketches share one table")

	// Binding a world sketch against a DIFFERENT table is the existing mismatch.
	a := s1.CreatePoint(0, 0)
	b := s1.CreatePoint(10, 0)
	d := sketch.NewDistance(a, b, 10)
	s1.AddConstraint(d)
	require.ErrorIs(t, s1.Bind(d, param.New(), "x"), sketch.ErrTableMismatch)
}

func TestWorldGlobalParameterDrivesTwoSketches(t *testing.T) {
	w := sketch.NewWorld()
	require.NoError(t, w.Params().SetValue("thickness", units.Millimeters(5)))

	bind := func(s *sketch.Sketch) *sketch.HorizontalDistance {
		a := s.CreatePoint(0, 0)
		b := s.CreatePoint(3, 1)
		s.Fix(a)
		s.AddConstraint(sketch.NewVerticalDistance(a, b, 0)) // b.y = 0 → fully constrained
		d := sketch.NewHorizontalDistance(a, b, 0)
		s.AddConstraint(d)
		require.NoError(t, s.Bind(d, s.Params(), "thickness")) // s.Params() == w.Params()
		return d
	}
	s1, _ := w.CreateSketch(w.XY())
	s2, _ := w.CreateSketch(w.XZ())
	d1, d2 := bind(s1), bind(s2)

	_, err := s1.Solve()
	require.NoError(t, err)
	_, err = s2.Solve()
	require.NoError(t, err)
	require.InDelta(t, 5, d1.Target().Base(), 1e-9)
	require.InDelta(t, 5, d2.Target().Base(), 1e-9)

	// Editing the one global parameter reflows both sketches.
	require.NoError(t, w.Params().SetValue("thickness", units.Millimeters(8)))
	_, _ = s1.Solve()
	_, _ = s2.Solve()
	require.InDelta(t, 8, d1.Target().Base(), 1e-9)
	require.InDelta(t, 8, d2.Target().Base(), 1e-9)
	require.True(t, w.Verify().Trustworthy())
}

func TestWorldSketchSharesWorldParams(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	require.Same(t, w.Params(), s.Params(), "a world sketch shares the world's global table from creation")
}

func TestParameterDrivenOffsetPlane(t *testing.T) {
	w := sketch.NewWorld()
	require.NoError(t, w.Params().SetValue("gap", units.Millimeters(10)))
	op, err := w.CreateOffsetPlane(w.XY(), 0)
	require.NoError(t, err)
	require.NoError(t, w.BindOffsetPlane(op, "gap"))

	f, err := op.Frame()
	require.NoError(t, err)
	require.InDelta(t, 10, f.Origin().Z, 1e-9, "offset along XY normal (+Z) by the parameter")

	// The frame reflects a parameter edit immediately — no ApplyParameters, no cache.
	require.NoError(t, w.Params().SetValue("gap", units.Millimeters(25)))
	f2, err := op.Frame()
	require.NoError(t, err)
	require.InDelta(t, 25, f2.Origin().Z, 1e-9)

	// Unbinding freezes the currently resolved distance (25) as the literal, rather
	// than reverting to the original literal (0).
	require.NoError(t, w.UnbindOffsetPlane(op))
	f3, err := op.Frame()
	require.NoError(t, err)
	require.InDelta(t, 25, f3.Origin().Z, 1e-9, "unbind keeps the plane where it was")
	require.NoError(t, w.Params().SetValue("gap", units.Millimeters(99)))
	f4, _ := op.Frame()
	require.InDelta(t, 25, f4.Origin().Z, 1e-9, "now a literal, no longer tracks the parameter")
}

func TestOffsetPlaneWrongKindRejected(t *testing.T) {
	w := sketch.NewWorld()
	require.NoError(t, w.Params().SetValue("theta", units.Degrees(30)))
	op, err := w.CreateOffsetPlane(w.XY(), 0)
	require.NoError(t, err)
	require.NoError(t, w.BindOffsetPlane(op, "theta")) // an angle, not a length

	_, err = op.Frame()
	require.ErrorIs(t, err, param.ErrIncompatibleKind, "offset distance must be a length")
	require.ErrorIs(t, w.ApplyParameters(), param.ErrIncompatibleKind)

	rep := w.Verify()
	require.NotEmpty(t, rep.PlaneErrors)
	require.False(t, rep.Trustworthy())

	// A non-offset plane cannot be bound.
	require.ErrorIs(t, w.BindOffsetPlane(w.XY(), "gap"), sketch.ErrNotOffsetPlane)
}

func TestWorldParamsRoundTrip(t *testing.T) {
	w := sketch.NewWorld()
	require.NoError(t, w.Params().SetValue("thickness", units.Millimeters(5)))
	op, _ := w.CreateOffsetPlane(w.XY(), 0)
	require.NoError(t, w.BindOffsetPlane(op, "thickness"))
	s1, _ := w.CreateSketch(w.XY())
	a := s1.CreatePoint(0, 0)
	b := s1.CreatePoint(3, 1)
	s1.Fix(a)
	s1.AddConstraint(sketch.NewVerticalDistance(a, b, 0))
	d := sketch.NewHorizontalDistance(a, b, 0)
	s1.AddConstraint(d)
	require.NoError(t, s1.Bind(d, s1.Params(), "thickness"))

	data, err := json.Marshal(w)
	require.NoError(t, err)
	require.Contains(t, string(data), "thickness")
	require.Contains(t, string(data), "dist_expr")
	require.Contains(t, string(data), `"version":3`)

	var w2 sketch.World
	require.NoError(t, json.Unmarshal(data, &w2))
	require.Len(t, w2.Sketches(), 1)
	require.Same(t, w2.Params(), w2.Sketches()[0].Params(), "loaded sketch shares the world table")
	// the offset plane (id 3, after XY/XZ/YZ) still resolves to thickness=5
	f, err := w2.Planes()[3].Frame()
	require.NoError(t, err)
	require.InDelta(t, 5, f.Origin().Z, 1e-9)
	// the bound dimension still solves to thickness
	_, err = w2.Sketches()[0].Solve()
	require.NoError(t, err)
	require.True(t, w2.Verify().Trustworthy())
}

func TestWorldVersionAndLegacyMigration(t *testing.T) {
	datums := []any{
		map[string]any{"kind": "worldXY"},
		map[string]any{"kind": "worldXZ"},
		map[string]any{"kind": "worldYZ"},
	}
	// A future version is rejected.
	future, _ := json.Marshal(map[string]any{
		"kind": "world", "version": 99, "planes": datums, "sketches": []any{},
	})
	var wf sketch.World
	require.Error(t, json.Unmarshal(future, &wf))

	// A legacy v2 world with a per-sketch parameter table promotes it to the shared
	// table. Build the table's JSON from a real table so the on-disk format matches.
	pt := param.New()
	require.NoError(t, pt.SetValue("thickness", units.Millimeters(5)))
	ptJSON, err := json.Marshal(pt)
	require.NoError(t, err)
	legacy, _ := json.Marshal(map[string]any{
		"kind": "world", "version": 2, "planes": datums,
		"sketches": []any{
			map[string]any{
				"plane": 0, "points": []any{}, "entities": []any{}, "constraints": []any{},
				"parameters": json.RawMessage(ptJSON),
			},
		},
	})
	var wl sketch.World
	require.NoError(t, json.Unmarshal(legacy, &wl))
	require.Same(t, wl.Params(), wl.Sketches()[0].Params(), "promoted to the shared table")
	got, err := wl.Params().Get("thickness")
	require.NoError(t, err)
	require.InDelta(t, 5, got, 1e-9)

	// Two legacy sketches with CONFLICTING per-sketch tables cannot collapse to one
	// shared table and are rejected rather than silently merged.
	pt2 := param.New()
	require.NoError(t, pt2.SetValue("thickness", units.Millimeters(9))) // differs from pt (5)
	pt2JSON, err := json.Marshal(pt2)
	require.NoError(t, err)
	conflict, _ := json.Marshal(map[string]any{
		"kind": "world", "version": 2, "planes": datums,
		"sketches": []any{
			map[string]any{"plane": 0, "points": []any{}, "entities": []any{}, "constraints": []any{}, "parameters": json.RawMessage(ptJSON)},
			map[string]any{"plane": 1, "points": []any{}, "entities": []any{}, "constraints": []any{}, "parameters": json.RawMessage(pt2JSON)},
		},
	})
	var wc sketch.World
	require.Error(t, json.Unmarshal(conflict, &wc), "conflicting legacy tables cannot migrate")
}
