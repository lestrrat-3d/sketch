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
// defining points and its construction flag.
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

	// Topology: which entities exist, of what type, over which points, and whether
	// they take part in profiles at all (construction geometry is excluded).
	write(uint64(len(s.ents)))
	for _, e := range s.ents {
		_, _ = fmt.Fprintf(h, "%T", e)
		if e.IsConstruction() {
			write(1)
		} else {
			write(0)
		}
		for _, p := range entityPoints(e) {
			write(uint64(p.id))
		}
	}
	return h.Sum64()
}
