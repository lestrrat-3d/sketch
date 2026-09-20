package sketch

import (
	"fmt"
	"math"
	"sort"
)

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
// freshly resolved one. Both are decided by the walk itself first — its
// COORDINATES, then how many edges it is cut into — and never by entity order.
// The walk alone cannot rank two chains whose walks are point-for-point
// identical (coincident duplicate geometry, which is a degenerate arrangement —
// such chains report Valid false), so those are ranked by everything else the
// chain publishes about itself, in this order: the
// [Entity.Name] labels along the walk, the names of the points those entities
// are defined from, each entity's kind, its reference provenance ([Entity.IsReference]
// and [Entity.Source]), each edge's trim (Partial, TStart, TEnd, TExact), each
// edge's walk direction (Reversed), and last the chain's Length. Every one of
// those is a property of the drawing, not of the order it was authored in.
//
// Two chains stay tied only when all of that is equal, and such chains are
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
// published list could still inherit authoring order, and compareChains closes
// it: it consults the coordinates first, by the same rule [geom] uses, and then
// every other source-independent property the chain publishes.
//
// The coordinate order is therefore never disturbed — a chain the coordinates
// already rank keeps that rank whatever the rest of it says — and the sort is
// stable, so the one pair nothing published can separate keeps the order the
// arrangement handed over.
func orderChains(chains []*Chain) {
	if len(chains) < 2 {
		return
	}
	ranked := make([]rankedChain, len(chains))
	for i, c := range chains {
		ranked[i] = rankedChain{chain: c, walk: chainWalk(c)}
	}
	sort.SliceStable(ranked, func(i, j int) bool { return compareChains(ranked[i], ranked[j]) < 0 })
	for i, r := range ranked {
		chains[i] = r.chain
	}
}

// rankedChain is a chain paired with its walk, so the sort computes each walk
// once rather than once per comparison.
type rankedChain struct {
	chain *Chain
	walk  [][2]float64
}

// compareChains is THE ordered comparison behind the published chain list, and
// the only place a chain is ranked. It returns a negative number when x sorts
// before y, a positive one when it sorts after, and zero when the two are tied.
//
// It ranks on what the chain PUBLISHES about itself and on nothing else, so the
// same drawing publishes the same order however it was authored. The precedence
// is fixed:
//
//  1. the walk itself — its coordinates by [geom]'s own rule (start point, end
//     point, then the whole walk), then how many edges it is cut into. This
//     comparison therefore REFINES the arrangement's order and never disturbs it;
//  2. the [Entity.Name] labels along the walk;
//  3. the names of the points each entity is defined from;
//  4. the kind of each entity (a *Line ranks apart from an *Arc);
//  5. reference provenance — [Entity.IsReference], then [Entity.Source];
//  6. each edge's trim — Partial, TStart, TEnd, TExact;
//  7. each edge's walk direction, Reversed;
//  8. the chain's Length.
//
// Rungs 2 through 7 are per-edge and are asked of the WHOLE walk before the next
// rung is asked at all, so one property decides the order everywhere it differs.
//
// A property added to [Chain] or [BoundaryEdge] later belongs here, on a rung of
// its own — this is the single place to put it, and [Sketch.Chains] states the
// same precedence for consumers, so the two move together. The two flags [Chain]
// publishes that are absent above, Valid and SelfIntersecting, need no rung:
// both are functions of the walk and of the arrangement's per-curve degeneracy
// attribution, which agree for any pair this comparison leaves tied.
//
// What is deliberately NOT consulted is entity identity: the [Entity] pointer,
// and the entity id behind it, ARE the authoring order. Two chains equal on
// every rung above stay tied, and the caller's stable sort keeps them adjacent
// in whatever order the arrangement produced. Nothing published separates them.
func compareChains(x, y rankedChain) int {
	if c := compareWalks(x.walk, y.walk); c != 0 {
		return c
	}
	if c := compareInt(len(x.chain.Edges), len(y.chain.Edges)); c != 0 {
		return c
	}
	for _, rank := range chainEdgeRanks {
		for i := range x.chain.Edges {
			if c := rank(x.chain.Edges[i], y.chain.Edges[i]); c != 0 {
				return c
			}
		}
	}
	return compareFloat(x.chain.Length, y.chain.Length)
}

// chainEdgeRanks is the per-edge half of compareChains's precedence, in order.
// A new per-edge property is ranked by adding its comparison here, at the rung
// it belongs on.
var chainEdgeRanks = []func(x, y BoundaryEdge) int{
	compareEdgeEntityName,
	compareEdgePointNames,
	compareEdgeKind,
	compareEdgeProvenance,
	compareEdgeTrim,
	compareEdgeReversed,
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

// compareWalks states the arrangement's own coordinate order — start point,
// then end point, then the whole walk — over the sketch layer's walks, so this
// rung reproduces the rank [geom] already gave a pair its coordinates separate.
func compareWalks(a, b [][2]float64) int {
	if len(a) == 0 || len(b) == 0 {
		return compareInt(len(a), len(b))
	}
	if c := comparePoint(a[0], b[0]); c != 0 {
		return c
	}
	if c := comparePoint(a[len(a)-1], b[len(b)-1]); c != 0 {
		return c
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		if c := comparePoint(a[i], b[i]); c != 0 {
			return c
		}
	}
	return compareInt(len(a), len(b))
}

// compareEdgeEntityName ranks by the label on the entity the edge lies on. An
// unnamed entity carries the empty name, which sorts first — so a coincident
// group nobody named is left to the rungs below rather than to authoring order.
func compareEdgeEntityName(x, y BoundaryEdge) int {
	return compareString(edgeEntityName(x), edgeEntityName(y))
}

// compareEdgePointNames ranks by the labels on the points the entity is DEFINED
// from, in the entity's own point order. A caller who names the points of
// otherwise indistinguishable coincident curves can read those names back off
// the published chain, so they have to rank it.
func compareEdgePointNames(x, y BoundaryEdge) int {
	xp, yp := edgeDefiningPoints(x), edgeDefiningPoints(y)
	for i := 0; i < len(xp) && i < len(yp); i++ {
		if c := compareString(pointName(xp[i]), pointName(yp[i])); c != 0 {
			return c
		}
	}
	return compareInt(len(xp), len(yp))
}

// compareEdgeKind ranks by the entity's Go type, which is the published kind: an
// *Arc and an *EllipticalArc drawn over the same points are separable by a type
// switch, so they must not tie. Reading the type rather than a hand-kept table
// keeps a new entity type ranked without a second place to update.
func compareEdgeKind(x, y BoundaryEdge) int {
	return compareString(edgeEntityKind(x), edgeEntityKind(y))
}

// compareEdgeProvenance ranks ordinary geometry before reference geometry, then
// by source id. A reference curve is identifiable through [Entity.IsReference]
// and [Entity.Source] whatever else it shares with the curve beside it.
func compareEdgeProvenance(x, y BoundaryEdge) int {
	xr, yr := edgeIsReference(x), edgeIsReference(y)
	if c := compareBool(xr, yr); c != 0 {
		return c
	}
	return compareString(edgeSource(x), edgeSource(y))
}

// compareEdgeTrim ranks by the sub-range the edge covers: whole edges before
// fragments, then the range itself, then whether its bounds are exact. Two
// fragments can walk identical coordinates and still cover different parameter
// ranges of different entities.
func compareEdgeTrim(x, y BoundaryEdge) int {
	if c := compareBool(x.Partial, y.Partial); c != 0 {
		return c
	}
	if c := compareFloat(x.TStart, y.TStart); c != 0 {
		return c
	}
	if c := compareFloat(x.TEnd, y.TEnd); c != 0 {
		return c
	}
	return compareBool(x.TExact, y.TExact)
}

// compareEdgeReversed ranks a walk that runs with its entity before one that
// runs against it. The canonical walk direction is a coordinate property, but
// which way the ENTITY under it points is not, and Reversed publishes that.
func compareEdgeReversed(x, y BoundaryEdge) int {
	return compareBool(x.Reversed, y.Reversed)
}

func edgeEntityName(e BoundaryEdge) string {
	if isNilEntity(e.Entity) {
		return ""
	}
	return e.Entity.Name()
}

func edgeEntityKind(e BoundaryEdge) string {
	if isNilEntity(e.Entity) {
		return ""
	}
	return fmt.Sprintf("%T", e.Entity)
}

func edgeIsReference(e BoundaryEdge) bool {
	if isNilEntity(e.Entity) {
		return false
	}
	return e.Entity.IsReference()
}

func edgeSource(e BoundaryEdge) string {
	if isNilEntity(e.Entity) {
		return ""
	}
	return e.Entity.Source()
}

func edgeDefiningPoints(e BoundaryEdge) []*Point {
	if isNilEntity(e.Entity) {
		return nil
	}
	return entityPoints(e.Entity)
}

func pointName(p *Point) string {
	if p == nil {
		return ""
	}
	return p.Name()
}

func comparePoint(a, b [2]float64) int {
	if c := compareFloat(a[0], b[0]); c != 0 {
		return c
	}
	return compareFloat(a[1], b[1])
}

// compareFloat orders two coordinates or parameters. A NaN sorts after every
// ordinary value and ties with another NaN, so a non-finite value left anywhere
// in the walk still yields a consistent order rather than a comparison that
// reports every pair unordered.
func compareFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	case a == b:
		return 0
	case math.IsNaN(a) && math.IsNaN(b):
		return 0
	case math.IsNaN(a):
		return 1
	}
	return -1
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func compareString(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// compareBool orders false before true.
func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a:
		return -1
	}
	return 1
}
