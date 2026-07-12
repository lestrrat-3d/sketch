package sketch

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math"
)

// Revision returns a fingerprint of the sketch state that [Sketch.Profiles]
// depends on: every solver variable, plus the entity set, each entity's INSTANCE
// IDENTITY — a never-reused uid, so a removed-and-recreated entity is a new
// instance even at the same id with the same shape, because a [Profile] holds
// LIVE entity handles — its type,
// its construction flag, its defining points — their sharing structure, ids AND
// the coordinates those point pointers actually resolve to, which is what
// buildProfiles reads and need not live in this sketch's var vector — and each
// entity's own shape state, resolved through the same accessors buildProfiles
// uses: a circle's radius, an ellipse's / elliptical arc's semi-axes and
// rotation, a conic's rho, a NURBS' degree, knots and weights — see
// entityStructuralState.
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

	// The flat var vector: every unknown the solver owns, including the aux vars
	// constraints allocate. It is hashed for what it is — a cheap catch-all over
	// solver state — NOT as a stand-in for what any entity reads out of it: an
	// entity resolves its shape through its own selector (c.ri, e.rxi, …) and its
	// own sketch pointer, so a rebound selector changes the shape while this
	// vector stays identical. Each entity's resolved shape value is hashed
	// separately below, in entityStructuralState.
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
	//
	// A defining point is hashed as buildProfiles CONSUMES it, not by a proxy for
	// it. buildProfiles follows the entity's *Point pointer and reads the
	// coordinates that pointer resolves to (Point.Geometry -> p.s.vars[p.xi]), and
	// it shares one geom.Point per DISTINCT *Point so that entities meeting at the
	// same point connect in the arrangement. So three things are hashed per point:
	//
	//   - its identity as sharing sees it (a first-seen sequence number over the
	//     entity walk — pointer addresses are not hashed, they are not deterministic
	//     across runs, but this ordinal captures exactly which entities share a
	//     point);
	//   - its id (the document-level handle);
	//   - its actual coordinates, via math.Float64bits.
	//
	// The coordinates are NOT covered by hashing s.vars above: the entity's fields
	// are exported, so a caller can rewire a defining point to a *Point belonging to
	// ANOTHER sketch (or to a removed handle), whose coordinates live outside this
	// sketch's var vector or under a renumbered id. Hashing p.id alone made such a
	// rewire invisible — the profiles changed while the revision did not, so a stale
	// Profile read fresh.
	seq := make(map[*Point]uint64)
	write(uint64(len(s.ents)))
	for _, e := range s.ents {
		// The entity's INSTANCE IDENTITY, not its positional id: a Profile hands
		// out live entity handles (Profile.Entities, BoundaryEdge.Entity), so the
		// revision must cover WHICH INSTANCES those handles are, not merely what
		// they look like. Removal renumbers ids, and a Line owns no scalar var to
		// retire, so removing a line and creating an identical one leaves type,
		// points, shape, id and the var vector all identical — only the uid (never
		// reused; see Sketch.addEntity) reveals that the old handle is now dead and
		// the sketch owns a different instance.
		write(s.entityUID(e))
		_, _ = fmt.Fprintf(h, "%T", e)
		if e.IsConstruction() {
			write(1)
		} else {
			write(0)
		}
		pts := entityPoints(e)
		write(uint64(len(pts)))
		for _, p := range pts {
			if p == nil || p.s == nil {
				// buildProfiles is nil-safe here (the builders reject nil), so the
				// fingerprint must be too; a distinct sentinel keeps a nil slot from
				// colliding with a real point.
				write(math.MaxUint64)
				continue
			}
			n, ok := seq[p]
			if !ok {
				n = uint64(len(seq))
				seq[p] = n
			}
			write(n)
			write(uint64(p.id))
			write(math.Float64bits(p.x()))
			write(math.Float64bits(p.y()))
		}
		entityStructuralState(e, write, writeFloats)
	}
	return h.Sum64()
}

// entityStructuralState feeds an entity's own SHAPE state into the revision
// hash: everything [Sketch.buildProfiles] reads off the entity itself, as
// opposed to the defining points and the construction flag that [Sketch.Revision]
// already covers per entity.
//
// It hashes the RESOLVED SHAPE VALUE for every entity that has one — a circle's
// radius, an ellipse's / elliptical arc's semi-axes and rotation, a conic's rho,
// a NURBS' degree/knots/weights — reading each through the very accessor
// buildProfiles reads it through (t.r(), t.rx(), t.rho(), …). It does NOT hash
// the selector INDEX (c.ri, e.rxi, …) as a stand-in for the value: an index is a
// proxy, and hashing the var VECTOR wholesale is a proxy too — the vector covers
// the var VALUES but not the entity→var BINDING, so swapping two circles'
// radius selectors leaves both the vector and the index multiset identical while
// the profiles swap shape. Hashing the value the consumer resolves closes that
// whole class: whatever the binding, the fingerprint sees what buildProfiles saw.
//
// A NEW ENTITY TYPE MUST BE ADDED HERE if buildProfiles reads anything off it
// besides its defining points — a solver-var shape value or stored data the
// solver cannot move alike. Without a case below such a change leaves the
// revision identical, so a stale [Profile] reads fresh and a consumer extrudes
// the wrong shape. Every entity type is listed explicitly, including the ones
// with nothing to hash, so the audit is readable rather than a silent default.
func entityStructuralState(e Entity, write func(uint64), writeFloats func([]float64)) {
	switch t := e.(type) {
	case *Circle:
		// geom.Circle{Center, Radius}: buildProfiles reads the radius via t.r().
		writeFloats([]float64{t.r()})
	case *Ellipse:
		// geom.Ellipse{Center, Rx, Ry, Rotation}.
		writeFloats([]float64{t.rx(), t.ry(), t.rot()})
	case *EllipticalArc:
		// geom.NewEllipticalArc(center, start, end, rx, ry, rot).
		writeFloats([]float64{t.rx(), t.ry(), t.rot()})
	case *Conic:
		// geom.NewConic(start, apex, end, rho).
		writeFloats([]float64{t.rho()})
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
	case *Line, *Arc, *Spline, *ClosedSpline, *FitSpline:
		// Nothing to hash: buildProfiles reads NOTHING off these entities but their
		// defining points — a line is its two endpoints, an arc its center/start/end
		// (its radius is the derived dist(Center, Start), not a stored value), and
		// the spline family is its control/fit points over a fixed uniform basis.
	}
}
