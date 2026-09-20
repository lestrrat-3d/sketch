package sketch

import "sort"

// Chain is an ordered OPEN run of boundary edges detected in a sketch: a
// connected walk over the sketch's own geometry whose two ends are free. It is
// the open counterpart of [Profile], over the same [BoundaryEdge] vocabulary —
// a polyline, a lone line, an arc joined to a line, or whatever geometry is left
// once the closed regions have taken their boundaries.
//
// A chain is what an open-curve operation consumes: sweeping one into a surface
// (a ribbon from a line, an uncapped shell from a revolved open profile) is the
// commonest thing to do with sketch geometry that encloses nothing. Like a
// [Profile] it is a SNAPSHOT — check [Chain.IsStale] before acting on one.
//
// An entity's edge is published in exactly ONE of the two places: a region
// boundary, or a chain. Close the four sides of a rectangle and [Sketch.Chains]
// reports nothing while [Sketch.Profiles] reports the region; leave one side off
// and the reverse happens.
//
// The walk is CUT at every vertex where it cannot continue unambiguously — a
// point where three or more edges meet, and the crossing point of two curves
// (four edges) — so three lines meeting at a point publish three chains, each
// usable on its own. A chain is open by definition: a closed run is a [Profile]
// if it bounds anything, and is published nowhere if it does not.
type Chain struct {
	// Entities is the de-duplicated set of distinct sketch entities on the
	// chain, in first-seen walk order. A curve split at a crossing appears once.
	Entities []Entity
	// Edges is the ordered walk, first edge first. Each edge is a whole entity or
	// a fragment of one, carrying the same TStart/TEnd/TExact trim contract a
	// profile's boundary edges carry.
	Edges []BoundaryEdge
	// Length is the chain's total arc length in base units (mm). It is exact for
	// a *Line, *Arc or *Circle fragment and a sampling-convergent underestimate
	// (the chord sum of the emitted polyline) for any other entity. It is
	// published whatever TExact reports, exactly as [Profile.Area] is: the range
	// still describes the emitted geometry, and a consumer that needs an exact
	// trim reads TExact rather than inferring it from this.
	Length float64
	// Valid is false when the chain cannot be trusted as a sweepable curve: a
	// walk that crosses or touches itself, or an unresolvable (degenerate)
	// arrangement condition that REACHES this chain — one involving a curve its
	// own edges are built from, or one no curve could be blamed for at all.
	//
	// It is scoped the way [Profile.Valid] is scoped. An ATTRIBUTABLE condition,
	// on curves this chain does not use, leaves this chain valid; an
	// unattributable one (an unusable input dropped before it reached the
	// arrangement, such as a zero-radius circle) invalidates every chain and
	// every profile, since what it would have subdivided is unknown.
	Valid bool
	// SelfIntersecting marks a chain whose own walk crosses or touches itself.
	// A self-crossing the arrangement RESOLVED is not reported here: it is a
	// vertex, so the walk is cut there and the pieces are published as separate
	// chains. This flags the crossing the arrangement could not place — the same
	// condition that makes the chain invalid from the degeneracy side.
	SelfIntersecting bool

	// sketch is the sketch this chain was built from, and revision that sketch's
	// [Sketch.Revision] at build time. Together they let a consumer ask whether
	// the chain still describes its sketch — see [Chain.IsStale].
	sketch   *Sketch
	revision uint64
}

// Sketch returns the sketch this chain was built from.
//
// A [Chain] is a snapshot, freshly allocated by every [Sketch.Chains] call, so
// pointer identity can never establish provenance; this can.
func (c *Chain) Sketch() *Sketch { return c.sketch }

// Revision is the value of [Sketch.Revision] at the moment this chain was built.
// Compare it against the sketch's current revision to detect staleness — or just
// call [Chain.IsStale].
func (c *Chain) Revision() uint64 { return c.revision }

// IsStale reports whether the sketch has changed since this chain was built, so
// the chain no longer describes it.
//
// A chain is a snapshot of geometry at one instant. Solving the sketch, editing
// a driving parameter, or adding/removing geometry moves that geometry — but the
// chain still holds the OLD walk, and its entities still belong to the sketch,
// so nothing about the handle looks wrong:
//
//	ch := s.Chains()[0]
//	s.Params().SetValue("height", units.Millimeters(60))
//	s.Solve(ctx)          // geometry moves; ch now describes the old shape
//	ch.IsStale()          // true — rebuild with s.Chains() before using it
//
// A consumer that sweeps a chain into a surface, or records it, must check this
// first: sweeping a stale chain silently builds the wrong shape, with no error
// anywhere to catch it.
func (c *Chain) IsStale() bool {
	if c.sketch == nil {
		return false // a zero-value Chain was never built from a sketch
	}
	return c.sketch.Revision() != c.revision
}

// Chains detects the OPEN connected runs of the sketch's non-construction
// geometry — everything the closed regions [Sketch.Profiles] reports do not use.
// The two publications partition the same arrangement, so an edge is never
// reported by both and geometry is never lost between them: a sketch's entities
// are split at their bare crossings once, and each resulting edge is either part
// of a region boundary or part of a chain.
//
// Each chain reports its ordered walk, the entities on it, its arc length, and
// whether it is a valid (non-self-intersecting, non-degenerate) curve to sweep.
// Reference geometry participates like ordinary geometry; construction geometry
// is excluded, exactly as it is from [Sketch.Profiles] — a construction curve is
// a drafting aid, and clearing its construction flag is what admits it to both
// publications.
//
// The chains come back in a deterministic order, each walked in a deterministic
// direction, so the same drawing publishes the same chains however it was
// authored — which is what lets a consumer compare a held chain against a
// freshly resolved one. Both are decided by the walk's COORDINATES first, never
// by entity order. Coordinates alone cannot rank two chains whose walks are
// point-for-point identical (coincident duplicate geometry, which is a
// degenerate arrangement — such chains report Valid false), so those are ranked
// by the [Entity.Name] labels along the walk, an authored property that survives
// reordering the authoring. Chains identical in BOTH coordinates and names stay
// interchangeable: nothing published about them differs, and which of their
// entities lands at which index is not defined.
func (s *Sketch) Chains() []*Chain {
	return s.buildProfiles().chains
}

// orderChains settles the published order the geometry alone leaves open.
//
// The [geom] arrangement ranks chains by their walk coordinates and nothing
// else, so two chains whose walks are point-for-point identical tie, and its
// stable sort then leaves them in the order the arranger saw their sources —
// which is the order the entities were authored in. That is the one way the
// published list could still inherit authoring order, so the tie is settled
// here, by the names on the entities the tied chains walk.
//
// The coordinate order is never disturbed. Equal walks are adjacent after the
// arrangement's own sort, so only such a run is reordered, and a chain the
// coordinates already rank keeps that rank whatever its name says.
func orderChains(chains []*Chain) {
	if len(chains) < 2 {
		return
	}
	walks := make([][][2]float64, len(chains))
	for i, c := range chains {
		walks[i] = chainWalk(c)
	}
	for i := 0; i < len(chains); {
		j := i + 1
		for j < len(chains) && samePolyline(walks[i], walks[j]) {
			j++
		}
		if j-i > 1 {
			tied := chains[i:j]
			sort.SliceStable(tied, func(x, y int) bool { return chainNamesLess(tied[x], tied[y]) })
		}
		i = j
	}
}

// chainWalk is the chain's own walk as one polyline — every edge's samples in
// walk order, less the joint each shares with the edge before it. It is the key
// the arrangement ranks chains by, so two chains with equal walks are exactly
// the two it left tied.
func chainWalk(c *Chain) [][2]float64 {
	var walk [][2]float64
	for i, e := range c.Edges {
		if i > 0 && len(e.Polyline) > 0 {
			walk = append(walk, e.Polyline[1:]...)
			continue
		}
		walk = append(walk, e.Polyline...)
	}
	return walk
}

func samePolyline(a, b [][2]float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// chainNamesLess ranks two chains with identical walks by the names of the
// entities they walk, edge by edge: the first differing name decides, and the
// shorter walk decides if one runs out first. An unnamed entity carries the
// empty name, which sorts first — so naming none of a coincident group leaves
// the group tied, and the order within it undefined, exactly as the group is.
func chainNamesLess(x, y *Chain) bool {
	for i := 0; i < len(x.Edges) && i < len(y.Edges); i++ {
		xn, yn := edgeEntityName(x.Edges[i]), edgeEntityName(y.Edges[i])
		if xn != yn {
			return xn < yn
		}
	}
	return len(x.Edges) < len(y.Edges)
}

func edgeEntityName(e BoundaryEdge) string {
	if e.Entity == nil {
		return ""
	}
	return e.Entity.Name()
}
