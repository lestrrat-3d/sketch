package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func TestProfilesRectangle(t *testing.T) {
	s := sketch.New()
	s.AddRectangle(0, 0, 20, 12)
	profiles := s.Profiles()
	require.Len(t, profiles, 1, "one profile")
	require.Len(t, profiles[0].Entities, 4, "four sides")
}

func TestProfilesPolygonExcludesConstruction(t *testing.T) {
	s := sketch.New()
	p := s.AddPolygon(0, 0, 6, 5) // 6 sides + 6 construction spokes
	profiles := s.Profiles()
	require.Len(t, profiles, 1, "spokes are construction, only the hull closes")
	require.Len(t, profiles[0].Entities, 6, "hexagon sides")
	require.Len(t, p.Spokes, 6, "spokes exist but are excluded")
}

func TestProfilesSlotAndCircle(t *testing.T) {
	s := sketch.New()
	s.AddSlot(0, 0, 10, 0, 3) // 2 arcs + 2 flanks + 4 construction spokes
	o := addPt(s, 30, 0)
	addCir(s, o, 2)

	profiles := s.Profiles()
	require.Len(t, profiles, 2, "slot loop + circle")
	// The circle is registered first (closed primitives), then the chain.
	require.Len(t, profiles[0].Entities, 1, "circle stands alone")
	require.Len(t, profiles[1].Entities, 4, "slot boundary: two arcs, two flanks")
}

// TestProfilesReflectSolvedGeometry pins that profiles are views over live
// solver-bound geometry: a dimension edit followed by a solve is reflected in
// a fresh detection pass, which is what downstream consumers (extrude, export)
// rely on for parametric behavior.
func TestProfilesReflectSolvedGeometry(t *testing.T) {
	s, w, _, _, _ := newRectangle(t)
	mustSolve(t, s)

	perimeter := func(p *sketch.Profile) float64 {
		var sum float64
		for _, e := range p.Entities {
			l, ok := e.(*sketch.Line)
			require.True(t, ok, "rectangle profile is all lines")
			sum += l.Length()
		}
		return sum
	}

	profiles := s.Profiles()
	require.Len(t, profiles, 1, "one closed profile")
	require.Len(t, profiles[0].Entities, 4, "four sides")
	require.InDelta(t, 2*(20+12), perimeter(profiles[0]), 1e-6, "perimeter at width 20")

	w.Set(35)
	mustSolve(t, s)
	profiles = s.Profiles()
	require.Len(t, profiles, 1, "profile survives the edit")
	require.InDelta(t, 2*(35+12), perimeter(profiles[0]), 1e-6, "perimeter at width 35")
}

func TestProfilesOpenChainAndConstructionCircle(t *testing.T) {
	s := sketch.New()
	a := addPt(s, 0, 0)
	b := addPt(s, 10, 0)
	c := addPt(s, 10, 10)
	addLn(s, a, b)
	addLn(s, b, c) // open chain

	g := geom.NewCircle(geom.NewPoint(30, 0), 2)
	g.Construction = true
	s.AddCircle(g)

	require.Empty(t, s.Profiles(), "no closed non-construction boundary")
}
