package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestProfilesRectangle(t *testing.T) {
	s := newSketch(t)
	s.CreateRectangle(0, 0, 20, 12)
	profiles := s.Profiles()
	require.Len(t, profiles, 1, "one profile")
	require.Len(t, profiles[0].Entities, 4, "four sides")
}

func TestProfilesPolygonExcludesConstruction(t *testing.T) {
	s := newSketch(t)
	p, err := s.CreatePolygon(0, 0, 6, 5) // 6 sides + 6 construction spokes
	require.NoError(t, err)
	profiles := s.Profiles()
	require.Len(t, profiles, 1, "spokes are construction, only the hull closes")
	require.Len(t, profiles[0].Entities, 6, "hexagon sides")
	require.Len(t, p.Spokes, 6, "spokes exist but are excluded")
}

func TestProfilesSlotAndCircle(t *testing.T) {
	s := newSketch(t)
	_, err := s.CreateSlot(0, 0, 10, 0, 3) // 2 arcs + 2 flanks + 4 construction spokes
	require.NoError(t, err)
	o := s.CreatePoint(30, 0)
	s.CreateCircle(o, 2)

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
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(18, 2)
	c := s.CreatePoint(17, 11)
	d := s.CreatePoint(1, 13)
	ab := s.CreateLine(a, b)
	bc := s.CreateLine(b, c)
	dc := s.CreateLine(d, c)
	ad := s.CreateLine(a, d)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewHorizontal(dc), sketch.NewVertical(ad), sketch.NewVertical(bc))
	w := sketch.NewDistance(a, b, 20)
	s.AddConstraint(w)
	s.AddConstraint(sketch.NewDistance(a, d, 12))
	_, err := s.Solve(t.Context())
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
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	profiles = s.Profiles()
	require.Len(t, profiles, 1, "profile survives the edit")
	require.InDelta(t, 2*(35+12), perimeter(profiles[0]), 1e-6, "perimeter at width 35")
}

func TestProfilesPlateWithHole(t *testing.T) {
	s := newSketch(t)
	s.CreateRectangle(0, 0, 10, 10)
	s.CreateCircle(s.CreatePoint(5, 5), 2) // fully inside

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
	s.CreateCircle(s.CreatePoint(0, 0), 3)
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
	s.CreateRectangle(0, 0, 6, 4)
	s.CreateRectangle(3, 2, 9, 6) // overlaps in [3,6]x[2,4]

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
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(4, 4)
	c := s.CreatePoint(4, 0)
	d := s.CreatePoint(0, 4)
	s.CreateLine(a, b)
	s.CreateLine(b, c)
	s.CreateLine(c, d)
	s.CreateLine(d, a) // bowtie: a-b crosses c-d

	profiles := s.Profiles()
	require.NotEmpty(t, profiles)
	for _, p := range profiles {
		require.True(t, p.SelfIntersecting, "boundary self-crosses")
		require.False(t, p.Valid, "a self-intersecting region is not a valid profile")
	}
}

func TestProfilesOpenChainAndConstructionCircle(t *testing.T) {
	s := newSketch(t)
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(10, 0)
	c := s.CreatePoint(10, 10)
	s.CreateLine(a, b)
	s.CreateLine(b, c) // open chain

	s.CreateCircle(s.CreatePoint(30, 0), 2).SetConstruction(true)

	require.Empty(t, s.Profiles(), "no closed non-construction boundary")
}

func TestProfilesSpurConstrainedOntoCircle(t *testing.T) {
	// A dangling line whose endpoint is CONSTRAINED onto a circle — how a gear's
	// flank-to-root line meets its root circle. The spur bounds nothing, so the
	// disk stays one valid profile: the contact must not make it unverifiable.
	for _, tc := range []struct {
		name       string
		farX, farY float64
	}{
		{"pointing outward", 15, 0},
		{"pointing inward", 5, 0},
		{"outward off the sampling grid", 15 * math.Cos(0.7), 15 * math.Sin(0.7)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSketch(t)
			o := s.CreatePoint(0, 0)
			s.Fix(o)
			c := s.CreateCircle(o, 10)
			s.AddConstraint(sketch.NewDiameter(c, 20))

			on := s.CreatePoint(10, 0)
			far := s.CreatePoint(tc.farX, tc.farY)
			s.Fix(far)
			s.CreateLine(on, far)
			s.AddConstraint(sketch.NewPointOnCircle(on, c))

			_, err := s.Solve(t.Context())
			require.NoError(t, err)

			rep := s.Verify(t.Context())
			require.True(t, rep.ProfilesValid, "the spur's contact leaves the disk verifiable")
			require.Empty(t, rep.InvalidProfiles)
			require.Len(t, rep.Profiles, 1, "just the disk")
			require.True(t, rep.Profiles[0].Valid, "the disk is a valid profile")
			require.False(t, rep.Profiles[0].SelfIntersecting)
			require.InDelta(t, math.Pi*100, rep.Profiles[0].Area, 1e-6, "the whole disk")
		})
	}
}

func TestProfilesSpurLeavesOtherRegionsValid(t *testing.T) {
	// Two disjoint circles, one carrying a spur constrained onto it. Neither disk
	// is touched by anything unresolvable, so both stay valid.
	s := newSketch(t)
	o := s.CreatePoint(0, 0)
	s.Fix(o)
	c := s.CreateCircle(o, 10)
	s.AddConstraint(sketch.NewDiameter(c, 20))

	o2 := s.CreatePoint(100, 0)
	s.Fix(o2)
	c2 := s.CreateCircle(o2, 10)
	s.AddConstraint(sketch.NewDiameter(c2, 20))

	on := s.CreatePoint(10, 0)
	far := s.CreatePoint(15, 0)
	s.Fix(far)
	s.CreateLine(on, far)
	s.AddConstraint(sketch.NewPointOnCircle(on, c))

	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	rep := s.Verify(t.Context())
	require.True(t, rep.ProfilesValid)
	require.Len(t, rep.Profiles, 2, "both disks")
	for i, p := range rep.Profiles {
		require.Truef(t, p.Valid, "disk %d is valid", i)
		require.InDeltaf(t, math.Pi*100, p.Area, 1e-6, "disk %d is whole", i)
	}
}

func TestProfilesDegeneracyScopedToItsOwnRegion(t *testing.T) {
	// A rectangle carrying a duplicated stretch of its bottom edge (coincident
	// geometry the arrangement cannot resolve), and a circle far away. Only the
	// rectangle's region is invalid; the circle is built from unrelated geometry.
	s := newSketch(t)
	s.CreateRectangle(0, 0, 10, 10)
	s.CreateLine(s.CreatePoint(2, 0), s.CreatePoint(8, 0)) // lies on the bottom edge
	s.CreateCircle(s.CreatePoint(100, 0), 5)

	profiles := s.Profiles()
	require.Len(t, profiles, 2, "rectangle + circle")

	var rect, disk *sketch.Profile
	for _, p := range profiles {
		if p.Area > 99 && p.Area < 101 {
			rect = p
			continue
		}
		disk = p
	}
	require.NotNil(t, rect)
	require.NotNil(t, disk)
	require.False(t, rect.Valid, "the rectangle owns the coincident edge")
	require.True(t, disk.Valid, "an unrelated region stays valid")

	// The sketch as a whole is still reported unverifiable — the scoping refines
	// which profile is implicated, it does not bless the sketch.
	rep := s.Verify(t.Context())
	require.False(t, rep.ProfilesValid, "the arrangement was unresolvable")
	require.Equal(t, []*sketch.Profile{rect}, rep.InvalidProfiles)
}

func TestProfilesUnattributableDegeneracyInvalidatesEveryProfile(t *testing.T) {
	// The other half of the scoping rule, and the one the doc comments must not
	// omit: a zero-radius circle is unusable input, dropped before it can form an
	// edge, so the condition it raises belongs to no curve. What it would have
	// subdivided is unknown, so the unrelated rectangle is invalid too.
	s := newSketch(t)
	s.CreateRectangle(0, 0, 10, 10)
	s.CreateCircle(s.CreatePoint(100, 0), 0)

	profiles := s.Profiles()
	require.Len(t, profiles, 1, "the zero-radius circle bounds nothing")
	require.InDelta(t, 100, profiles[0].Area, 1e-6, "the rectangle is whole")
	require.False(t, profiles[0].SelfIntersecting, "its boundary is clean")
	require.False(t, profiles[0].Valid, "an unattributable condition reaches every region")

	rep := s.Verify(t.Context())
	require.False(t, rep.ProfilesValid)
	require.Equal(t, profiles[0].Area, rep.InvalidProfiles[0].Area)
	require.Len(t, rep.InvalidProfiles, 1, "every detected profile is listed")
}

// TestProfilesCoincidentCarrierGearTooth is a sketch-level equivalent of probe
// case C (.tmp/decad-2d-region-asks/probe/main.go, ask 2's motivating example): a
// root arc lying EXACTLY on a hub circle's carrier, closed by two flank lines and
// a tip line into a tooth. Before coincident-carrier resolution
// (docs/coincident-carrier-resolution-design.md) this sketch's arrangement was
// Degenerate and Verify(ctx).Trustworthy() read false — "gear-like sections
// cannot even be authored directly and verified," per the design's own framing.
// Fully constrained (DOF 0) via distances + a vertical tip, so Trustworthy()
// isolates the coincident-carrier fix rather than an unrelated under/over-
// constraint.
func TestProfilesCoincidentCarrierGearTooth(t *testing.T) {
	s := newSketch(t)
	center := s.CreatePoint(0, 0)
	s.Fix(center)
	hub := s.CreateCircle(center, 10)
	s.AddConstraint(sketch.NewRadius(hub, 10))

	ax, ay := 10*math.Cos(0.3), 10*math.Sin(0.3)
	p0 := s.CreatePoint(ax, -ay) // root arc start
	p1 := s.CreatePoint(ax, ay)  // root arc end
	s.Fix(p0)
	s.AddConstraint(sketch.NewDistance(p0, p1, math.Hypot(p1.X()-p0.X(), p1.Y()-p0.Y())))
	rootArc := s.CreateArc(center, p0, p1)

	tip0 := s.CreatePoint(13, 1)
	tip1 := s.CreatePoint(13, -1)
	s.AddConstraint(sketch.NewDistance(p1, tip0, math.Hypot(13-ax, 1-ay)))
	s.AddConstraint(sketch.NewDistance(tip0, tip1, 2))
	s.AddConstraint(sketch.NewDistance(tip1, p0, math.Hypot(13-ax, -1+ay)))
	s.AddConstraint(sketch.NewVerticalPoints(tip0, tip1))
	s.CreateLine(p1, tip0)
	tip := s.CreateLine(tip0, tip1)
	s.CreateLine(tip1, p0)

	if _, err := s.Solve(t.Context()); err != nil {
		t.Fatalf("solve: %v", err)
	}

	rep := s.Verify(t.Context())
	require.Equal(t, sketch.FullyConstrained, rep.Status)
	require.Truef(t, rep.Trustworthy(), "%v", rep.Check())
	require.True(t, rep.ProfilesValid)
	require.Len(t, rep.Profiles, 2, "hub + tooth")

	var hub2D, tooth2D *sketch.Profile
	for _, p := range rep.Profiles {
		if p.Area > 100 {
			hub2D = p
		} else {
			tooth2D = p
		}
	}
	require.NotNilf(t, hub2D, "profiles: %v", rep.Profiles)
	require.NotNilf(t, tooth2D, "profiles: %v", rep.Profiles)
	require.InDelta(t, math.Pi*100, hub2D.Area, 1e-6)
	require.InDelta(t, 11.864262, tooth2D.Area, 1e-3)

	// Both adjoining profiles must attribute the shared span to the SAME entity —
	// the root arc — never one naming it and the other naming the hub circle (the
	// pre-resolution bug the design's problem statement describes).
	require.Contains(t, hub2D.Entities, sketch.Entity(rootArc), "the hub profile reuses the root arc")
	require.Contains(t, tooth2D.Entities, sketch.Entity(rootArc), "the tooth profile uses the root arc")
	require.Contains(t, tooth2D.Entities, sketch.Entity(tip), "the tooth profile includes its own tip line")
	require.NotContains(t, tooth2D.Entities, sketch.Entity(hub), "the tooth profile must not fall back to the hub circle")

	// Every bound of the merged (root-arc) span, and every other bound in this
	// arrangement, is TExact: the resolved coincidence and every other contact
	// here (line-involved cuts) are certified exact.
	for _, e := range hub2D.Outer {
		require.Truef(t, e.TExact, "hub: %+v", e)
	}
	for _, e := range tooth2D.Outer {
		require.Truef(t, e.TExact, "tooth: %+v", e)
	}
}

// TestProfilesHiddenCrossingIsInvalid is the sketch-level face of the near-miss
// guard (geom/nearmiss.go): a curve whose bow between two consecutive samples is
// larger than the thing it crosses hides the crossing from the planar map
// entirely, and before the guard the resulting profile set was published with
// Valid, ProfilesValid and Trustworthy() all reading clean.
//
// A 1000 mm spline through a 0.05 mm circle centred on the true curve. The
// arrangement still emits the whole disk — this pass changes no topology and no
// area — but it no longer calls it sound.
func TestProfilesHiddenCrossingIsInvalid(t *testing.T) {
	s := newSketch(t)
	sp, err := s.CreateSpline(
		s.CreatePoint(-500, 300), s.CreatePoint(-250, -400), s.CreatePoint(0, -400),
		s.CreatePoint(250, -400), s.CreatePoint(500, 300))
	require.NoError(t, err)
	// The middle of one of the spline's 64 default sample spans, where its chord
	// runs furthest from it.
	cx, cy := sp.Eval(40.5 / 64)
	s.CreateCircle(s.CreatePoint(cx, cy), 0.05)

	profiles := s.Profiles()
	require.Len(t, profiles, 1, "the sampled map has the disk whole")
	require.InEpsilon(t, math.Pi*0.05*0.05, profiles[0].Area, 1e-9, "the whole disk's area")
	require.False(t, profiles[0].Valid, "a crossing this sampling cannot rule out")

	rep := s.Verify(t.Context())
	require.False(t, rep.ProfilesValid)
	require.False(t, rep.Trustworthy())
	require.ErrorIs(t, rep.Check(), sketch.ErrInvalidProfile)
}
