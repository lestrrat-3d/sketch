package sketch

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
// direction (both stated in coordinates, not in entity order), so the same
// drawing publishes the same chains however it was authored — which is what lets
// a consumer compare a held chain against a freshly resolved one.
func (s *Sketch) Chains() []*Chain {
	return s.buildProfiles().chains
}
