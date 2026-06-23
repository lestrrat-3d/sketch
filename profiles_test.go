package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestProfilesRectangle(t *testing.T) {
	s := newSketch(t)
	s.AddRectangle(0, 0, 20, 12)
	profiles := s.Profiles()
	require.Len(t, profiles, 1, "one profile")
	require.Len(t, profiles[0].Entities, 4, "four sides")
}

func TestProfilesPolygonExcludesConstruction(t *testing.T) {
	s := newSketch(t)
	p, err := s.AddPolygon(0, 0, 6, 5) // 6 sides + 6 construction spokes
	require.NoError(t, err)
	profiles := s.Profiles()
	require.Len(t, profiles, 1, "spokes are construction, only the hull closes")
	require.Len(t, profiles[0].Entities, 6, "hexagon sides")
	require.Len(t, p.Spokes, 6, "spokes exist but are excluded")
}

func TestProfilesSlotAndCircle(t *testing.T) {
	s := newSketch(t)
	_, err := s.AddSlot(0, 0, 10, 0, 3) // 2 arcs + 2 flanks + 4 construction spokes
	require.NoError(t, err)
	o := s.AddPoint(30, 0)
	s.AddCircle(o, 2)

	profiles := s.Profiles()
	require.Len(t, profiles, 2, "slot loop + circle")
	// Two disjoint regions: the slot boundary (two arcs + two flanks) and the
	// standalone circle (one entity). Both are valid.
	var slot, circle *sketch.Profile
	for _, p := range profiles {
		switch len(p.Entities) {
		case 1:
			circle = p
		case 4:
			slot = p
		}
		require.True(t, p.Valid, "both regions are clean")
	}
	require.NotNil(t, circle, "the circle is its own region")
	require.NotNil(t, slot, "the slot boundary closes")
	_, ok := circle.Entities[0].(*sketch.Circle)
	require.True(t, ok, "the lone region is the circle")
}

// TestProfilesReflectSolvedGeometry pins that profiles are views over live
// solver-bound geometry: a dimension edit followed by a solve is reflected in
// a fresh detection pass, which is what downstream consumers (extrude, export)
// rely on for parametric behavior.
func TestProfilesReflectSolvedGeometry(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(18, 2)
	c := s.AddPoint(17, 11)
	d := s.AddPoint(1, 13)
	ab := s.AddLine(a, b)
	bc := s.AddLine(b, c)
	dc := s.AddLine(d, c)
	ad := s.AddLine(a, d)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))
	w := sketch.NewDistance(a, b, 20)
	s.AddConstraint(w)
	s.AddConstraint(sketch.NewDistance(a, d, 12))
	_, err := s.Solve()
	require.NoError(t, err)

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
	_, err = s.Solve()
	require.NoError(t, err)
	profiles = s.Profiles()
	require.Len(t, profiles, 1, "profile survives the edit")
	require.InDelta(t, 2*(35+12), perimeter(profiles[0]), 1e-6, "perimeter at width 35")
}

func TestProfilesPlateWithHole(t *testing.T) {
	s := newSketch(t)
	s.AddRectangle(0, 0, 10, 10)
	s.AddCircle(s.AddPoint(5, 5), 2) // fully inside

	profiles := s.Profiles()
	require.Len(t, profiles, 2, "the plate (with a hole) and the inner disk")
	var plate, disk *sketch.Profile
	for _, p := range profiles {
		if len(p.Holes) == 1 {
			plate = p
		} else {
			disk = p
		}
	}
	require.NotNil(t, plate, "plate carries the circular hole")
	require.NotNil(t, disk, "the disk is a separate region")
	require.Len(t, plate.Entities, 4, "plate outer is four sides")
	require.InDelta(t, 100-math.Pi*4, plate.Area, 1e-2, "plate net area = square minus disk")
	require.InDelta(t, math.Pi*4, disk.Area, 1e-2, "disk area")
	require.True(t, plate.Valid)
	_, ok := plate.Holes[0][0].Entity.(*sketch.Circle)
	require.True(t, ok, "the hole boundary is the circle")
	require.False(t, plate.Holes[0][0].Partial, "an uncut circle hole is a whole edge, not a fragment")
	for _, e := range disk.Outer {
		require.False(t, e.Partial, "the uncut disk boundary is whole")
	}
}

func TestProfilesLoneCircleWhole(t *testing.T) {
	s := newSketch(t)
	s.AddCircle(s.AddPoint(0, 0), 3)
	profiles := s.Profiles()
	require.Len(t, profiles, 1, "one disk region")
	require.Len(t, profiles[0].Entities, 1, "the circle")
	require.InDelta(t, math.Pi*9, profiles[0].Area, 1e-2)
	require.True(t, profiles[0].Valid)
	for _, e := range profiles[0].Outer {
		require.False(t, e.Partial, "an uncut circle is a whole boundary")
	}
}

func TestProfilesBareCrossingSubdivision(t *testing.T) {
	s := newSketch(t)
	s.AddRectangle(0, 0, 6, 4)
	s.AddRectangle(3, 2, 9, 6) // overlaps in [3,6]x[2,4]

	profiles := s.Profiles()
	require.Len(t, profiles, 3, "two L-shapes and the overlap")
	var total float64
	var sawPartial bool
	for _, p := range profiles {
		require.True(t, p.Valid)
		total += p.Area
		for _, e := range p.Outer {
			if e.Partial {
				sawPartial = true
			}
		}
	}
	require.InDelta(t, 24+24-6, total, 1e-9, "areas partition the union")
	require.True(t, sawPartial, "split edges are reported as fragments")
}

func TestProfilesSelfIntersectingInvalid(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(4, 4)
	c := s.AddPoint(4, 0)
	d := s.AddPoint(0, 4)
	s.AddLine(a, b)
	s.AddLine(b, c)
	s.AddLine(c, d)
	s.AddLine(d, a) // bowtie: a-b crosses c-d

	profiles := s.Profiles()
	require.NotEmpty(t, profiles)
	for _, p := range profiles {
		require.True(t, p.SelfIntersecting, "boundary self-crosses")
		require.False(t, p.Valid, "a self-intersecting region is not a valid profile")
	}
}

func TestProfilesOpenChainAndConstructionCircle(t *testing.T) {
	s := newSketch(t)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(10, 0)
	c := s.AddPoint(10, 10)
	s.AddLine(a, b)
	s.AddLine(b, c) // open chain

	s.AddCircle(s.AddPoint(30, 0), 2).SetConstruction(true)

	require.Empty(t, s.Profiles(), "no closed non-construction boundary")
}
