package sketch

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math"
)

// Revision returns a fingerprint of the sketch state that [Sketch.Profiles]
// depends on: every solver variable (point coordinates, circle radii, ellipse
// axes/rotation, conic rho), plus the entity set, each entity's type, its
// defining points, its construction flag, and any structural state the entity
// stores outside the solver's variable vector (a NURBS' degree, knots and
// weights) — see entityStructuralState.
//
// It is a FINGERPRINT, not a counter: compare it for EQUALITY only, never for
// order. Equal revisions mean the sketch is geometrically unchanged; a different
// revision means it has been mutated. Returning to a previously seen state
// reproduces that state's revision, which is the honest answer — a profile built
// then is still correct now.
//
// The intended use is detecting a STALE [Profile] — one built before a later
// [Sketch.Solve], parameter edit or geometry change — via [Profile.IsStale].
// Extruding or recording a stale profile silently builds the old shape, so a
// consumer that turns a profile into a solid must check.
//
// It is derived from state rather than bumped by each mutating method on
// purpose: there is no list of bump sites to forget, so a new mutation path
// cannot silently leave the revision stale.
func (s *Sketch) Revision() uint64 {
	h := fnv.New64a()
	var buf [8]byte
	write := func(u uint64) {
		binary.LittleEndian.PutUint64(buf[:], u)
		_, _ = h.Write(buf[:])
	}

	// Every geometric unknown lives in the flat var vector, so hashing it covers
	// every coordinate and shape variable the solver can move.
	write(uint64(len(s.vars)))
	for _, v := range s.vars {
		write(math.Float64bits(v))
	}

	writeFloats := func(vs []float64) {
		// The length is hashed too, so [1, 2] and [1, 2, 0] cannot collide.
		write(uint64(len(vs)))
		for _, v := range vs {
			write(math.Float64bits(v))
		}
	}

	// Topology: which entities exist, of what type, over which points, and whether
	// they take part in profiles at all (construction geometry is excluded) — plus
	// whatever the entity stores outside the var vector.
	write(uint64(len(s.ents)))
	for _, e := range s.ents {
		_, _ = fmt.Fprintf(h, "%T", e)
		if e.IsConstruction() {
			write(1)
		} else {
			write(0)
		}
		pts := entityPoints(e)
		write(uint64(len(pts)))
		for _, p := range pts {
			write(uint64(p.id))
		}
		entityStructuralState(e, write, writeFloats)
	}
	return h.Sum64()
}

// entityStructuralState feeds an entity's STRUCTURAL state into the revision
// hash: data [Sketch.buildProfiles] reads that is neither a solver variable
// (s.vars) nor a defining point nor the construction flag — the three things
// [Sketch.Revision] already covers wholesale.
//
// A NEW ENTITY TYPE CARRYING NON-VARIABLE STRUCTURAL DATA MUST BE ADDED HERE.
// The var vector is the solver's currency, so anything the solver cannot move is
// invisible to it: change such a field and, without a case below, the revision —
// and every [Profile] built from it — would stay identical, so a stale profile
// would read fresh and a consumer would extrude the wrong shape. Every entity
// type is listed explicitly, including the ones with nothing to hash, so the
// audit is readable rather than a silent default.
func entityStructuralState(e Entity, write func(uint64), writeFloats func([]float64)) {
	switch t := e.(type) {
	case *NURBS:
		// Degree, knots and weights are stored structural data, NOT solver vars
		// (see docs/nurbs-design.md), yet buildProfiles hands all three to the
		// arrangement — they change the curve without moving a single point.
		// Degree is hashed for its own sake even though a clamped knot vector
		// already implies it (len(knots) == len(control)+degree+1, and both counts
		// are hashed), so the fingerprint never rests on that invariant holding.
		write(uint64(t.degree))
		writeFloats(t.knots)
		writeFloats(t.weights)
	case *Line, *Circle, *Arc, *Ellipse, *EllipticalArc, *Conic,
		*Spline, *ClosedSpline, *FitSpline:
		// Nothing to hash: these are defined entirely by their points and their
		// solver variables — a circle's radius, an ellipse's semi-axes/rotation
		// and a conic's rho all live in s.vars (see Sketch.entitySizeVars).
	}
}
