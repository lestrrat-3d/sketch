package sketch

import (
	"errors"
	"fmt"
	"math"
)

// Reference geometry is read-only 2D geometry whose coordinates are externally
// locked — a frozen snapshot of 3D-derived geometry (a projected edge, a pierced
// vertex) handed in by the layer above — carrying an opaque source id and a
// staleness flag. The solver never moves it; only the Refresh* methods rewrite
// it. It is distinct from construction geometry (which the solver moves) and is
// the sketch/3D separation keystone. See docs/reference-geometry-design.md.

// ErrNotReference is returned by the Refresh* methods for a point or circle that
// is not reference geometry.
var ErrNotReference = errors.New("sketch: not reference geometry")

// ErrForeignPoint is returned by the reference constructors and the Refresh*
// methods when a supplied point is not a live point of this sketch (a point of
// another sketch, or one that has been removed).
var ErrForeignPoint = errors.New("sketch: point is not a live point of this sketch")

// refState is the reference-geometry provenance embedded in a Point and in every
// Entity: externally-locked snapshot status, its opaque 3D source id, and the
// staleness bit of the atomic re-fed unit (a point's coordinates, a circle's
// radius). Line/arc entities own no coordinate of their own, so their staleness
// is derived from their points and this bit is unused for them.
type refState struct {
	reference bool
	source    string
	stale     bool
}

// IsReference reports whether this is externally-locked reference geometry.
func (r refState) IsReference() bool { return r.reference }

// Source returns reference geometry's opaque provenance id ("" otherwise).
func (r refState) Source() string { return r.source }

// owns reports whether p is a live point of this sketch — not nil, not foreign
// (belonging to another sketch), and not dead (removed, its id slot now holding
// a different point).
func (s *Sketch) owns(p *Point) bool {
	return p != nil && p.s == s && p.id >= 0 && p.id < len(s.points) && s.points[p.id] == p
}

// requireRefPoints checks that every point is a live reference point of this
// sketch, so a reference entity can never be built on foreign, dead, or free
// points.
func (s *Sketch) requireRefPoints(pts ...*Point) error {
	for _, p := range pts {
		if !s.owns(p) {
			return ErrForeignPoint
		}
		if !p.reference {
			return ErrNotReference
		}
	}
	return nil
}

// sealReference records an entity's construction-time defining points (its
// topology seal), so Verify can detect a later rewiring of an exported field.
func (s *Sketch) sealReference(e Entity, pts ...*Point) {
	if s.refSeals == nil {
		s.refSeals = make(map[Entity][]*Point)
	}
	s.refSeals[e] = append([]*Point(nil), pts...)
}

// AddReferencePoint adds a reference point at plane-local (x, y) tagged with the
// given source id. Its coordinates are locked (the solver never moves it); only
// [Sketch.RefreshReference] rewrites them. Build reference curves from the
// returned point so projected loops can close by sharing it.
func (s *Sketch) AddReferencePoint(x, y float64, source string) *Point {
	p := &Point{s: s, xi: s.newVar(x), yi: s.newVar(y), id: len(s.points)}
	p.reference = true
	p.source = source
	s.fixed[p.xi] = true
	s.fixed[p.yi] = true
	s.points = append(s.points, p)
	return p
}

// AddReferenceLine adds a reference line between two reference points of this
// sketch. It returns [ErrForeignPoint] for a foreign/dead point and
// [ErrNotReference] for a non-reference point.
func (s *Sketch) AddReferenceLine(p1, p2 *Point, source string) (*Line, error) {
	if err := s.requireRefPoints(p1, p2); err != nil {
		return nil, err
	}
	l := &Line{s: s, Start: p1, End: p2, id: len(s.ents)}
	l.reference = true
	l.source = source
	s.ents = append(s.ents, l)
	s.sealReference(l, p1, p2)
	return l, nil
}

// AddReferenceArc adds a reference arc (counter-clockwise from start to end about
// center) on reference points of this sketch. The snapshot must be a valid arc
// (start and end equidistant from the center, [ErrInvalidShape] otherwise); the
// locked points carry no radius-consistency solver constraint, and the invariant
// is re-checked by [Sketch.Verify] (so an endpoint refreshed to a different
// radius is reported broken). Same point requirements as
// [Sketch.AddReferenceLine].
func (s *Sketch) AddReferenceArc(center, start, end *Point, source string) (*Arc, error) {
	if err := s.requireRefPoints(center, start, end); err != nil {
		return nil, err
	}
	// The snapshot must be a valid arc (start and end equidistant from center).
	rs := math.Hypot(start.x()-center.x(), start.y()-center.y())
	re := math.Hypot(end.x()-center.x(), end.y()-center.y())
	if math.Abs(rs-re) > 1e-9*(1+rs) {
		return nil, fmt.Errorf("%w: reference arc start and end are not equidistant from the center", ErrInvalidShape)
	}
	a := &Arc{s: s, Center: center, Start: start, End: end, id: len(s.ents)}
	a.reference = true
	a.source = source
	s.ents = append(s.ents, a)
	// No arcRadius constraint: the points are locked, so radius consistency is a
	// property of the supplied snapshot (validated above), not something for the
	// solver to enforce — and a row touching no free variable would read as a
	// redundant constraint and wrongly fail Trustworthy().
	s.sealReference(a, center, start, end)
	return a, nil
}

// AddReferenceCircle adds a reference circle with the given reference center
// point and radius. Both the center and the radius variable are locked; refresh
// the radius with [Sketch.RefreshReferenceCircle]. Same point requirements as
// [Sketch.AddReferenceLine].
func (s *Sketch) AddReferenceCircle(center *Point, r float64, source string) (*Circle, error) {
	if err := s.requireRefPoints(center); err != nil {
		return nil, err
	}
	c := &Circle{s: s, Center: center, ri: s.newVar(r), id: len(s.ents)}
	c.reference = true
	c.source = source
	s.fixed[c.ri] = true
	s.ents = append(s.ents, c)
	s.sealReference(c, center)
	return c, nil
}

// RefreshReference rewrites a reference point's locked coordinates and clears its
// staleness — the 3D layer's re-feed path, and the only sanctioned writer of
// reference coordinates. It returns [ErrForeignPoint] for a point this sketch
// does not own and [ErrNotReference] for a non-reference point.
func (s *Sketch) RefreshReference(p *Point, x, y float64) error {
	if !s.owns(p) {
		return ErrForeignPoint
	}
	if !p.reference {
		return ErrNotReference
	}
	s.vars[p.xi] = x
	s.vars[p.yi] = y
	p.stale = false
	return nil
}

// RefreshReferenceCircle rewrites a reference circle's locked radius and clears
// its radius staleness. Refresh the center separately with
// [Sketch.RefreshReference]. Returns [ErrNotReference] for a non-reference or
// foreign circle.
func (s *Sketch) RefreshReferenceCircle(c *Circle, r float64) error {
	if c == nil || c.s != s || !c.reference {
		return ErrNotReference
	}
	s.vars[c.ri] = r
	c.stale = false
	return nil
}

// isNilEntity reports whether an Entity interface holds a nil concrete pointer
// (a typed nil, e.g. NewHorizontal(nil)'s line) — which a plain `== nil` misses,
// so calling a method on it would panic.
func isNilEntity(e Entity) bool {
	switch t := e.(type) {
	case nil:
		return true
	case *Line:
		return t == nil
	case *Circle:
		return t == nil
	case *Arc:
		return t == nil
	case *Ellipse:
		return t == nil
	case *EllipticalArc:
		return t == nil
	case *Spline:
		return t == nil
	case *ClosedSpline:
		return t == nil
	case *FitSpline:
		return t == nil
	}
	return false
}

// entityPoints returns an entity's current defining points (read from its
// exported fields), used by the reference lock-integrity and reachability checks.
func entityPoints(e Entity) []*Point {
	switch t := e.(type) {
	case *Line:
		return []*Point{t.Start, t.End}
	case *Circle:
		return []*Point{t.Center}
	case *Arc:
		return []*Point{t.Center, t.Start, t.End}
	case *Ellipse:
		return []*Point{t.Center}
	case *EllipticalArc:
		return []*Point{t.Center, t.Start, t.End}
	case *Spline:
		return t.Control
	case *ClosedSpline:
		return t.Control
	case *FitSpline:
		return t.Fit
	}
	return nil
}

// referenceBroken reports whether an entity fails the lock-integrity check: any
// entity with a foreign/dead defining point, or a reference entity that is also
// construction, has a non-reference or unlocked defining point, an unfixed owned
// var, or a topology that no longer matches its construction-time seal.
func (s *Sketch) referenceBroken(e Entity) bool {
	pts := entityPoints(e)
	for _, p := range pts {
		if !s.owns(p) {
			return true
		}
	}
	if !e.IsReference() {
		return false
	}
	if e.IsConstruction() {
		return true
	}
	for _, p := range pts {
		if !p.reference || !p.IsFixed() {
			return true
		}
	}
	if c, ok := e.(*Circle); ok && !s.fixed[c.ri] {
		return true
	}
	if a, ok := e.(*Arc); ok {
		// A reference arc carries no radius-consistency solver constraint (its
		// points are locked), so the snapshot invariant — start and end
		// equidistant from the center — is checked here, catching an endpoint
		// refreshed to a different radius.
		rs := math.Hypot(a.Start.x()-a.Center.x(), a.Start.y()-a.Center.y())
		re := math.Hypot(a.End.x()-a.Center.x(), a.End.y()-a.Center.y())
		if math.Abs(rs-re) > 1e-9*(1+rs) {
			return true
		}
	}
	seal := s.refSeals[e]
	if len(seal) != len(pts) {
		return true
	}
	for i := range pts {
		if pts[i] != seal[i] {
			return true
		}
	}
	return false
}

// scanReferenceIntegrity fills the report's broken-reference and foreign-handle
// signals and reports whether the topology is nil-corrupt (a defining point or
// constraint operand rewired to nil). It is nil-safe — it never dereferences a
// point's coordinates — so it can run before the residual/profile analysis,
// which would panic on a nil point. A nil-corrupt return tells Verify to skip
// that analysis.
func (s *Sketch) scanReferenceIntegrity(rep *VerificationReport) bool {
	nilCorrupt := false
	for _, e := range s.ents {
		if s.referenceBroken(e) {
			rep.BrokenReferences = append(rep.BrokenReferences, e)
		}
		for _, p := range entityPoints(e) {
			switch {
			case p == nil:
				nilCorrupt = true
			case !s.owns(p):
				rep.ForeignHandles = true
			}
		}
	}
	for _, c := range s.cons {
		pts, ents := constraintRefs(c)
		for _, p := range pts {
			switch {
			case p == nil:
				nilCorrupt = true
			case !s.owns(p):
				rep.ForeignHandles = true
			}
		}
		for _, ce := range ents {
			switch {
			case isNilEntity(ce):
				nilCorrupt = true // a nil operand would panic the residual analysis
			case !s.ownsEntity(ce):
				rep.ForeignHandles = true
			}
		}
	}
	return nilCorrupt
}

// scanReferenceStaleness fills the report's stale-reference signals. It reads
// IsStale (which derives an entity's staleness from its points), so it must run
// only after scanReferenceIntegrity reports the topology is not nil-corrupt.
func (s *Sketch) scanReferenceStaleness(rep *VerificationReport) {
	for _, p := range s.points {
		if p.reference && p.stale {
			rep.StaleReferencePoints = append(rep.StaleReferencePoints, p)
		}
	}
	for _, e := range s.ents {
		if e.IsReference() && e.IsStale() {
			rep.StaleReferences = append(rep.StaleReferences, e)
		}
	}
	rep.Stale = len(rep.StaleReferencePoints) > 0 || len(rep.StaleReferences) > 0
}

// MarkStale marks every reference unit derived from the given source as stale:
// the 3D layer calls it when a source changes. It resolves both a point's source
// and an entity's source down to the atomic re-fed units — every reference point
// with that source, and (via the topology seal) the defining points and radius
// of every reference entity with that source — so a projected edge whose source
// differs from its vertices' still goes stale coherently. Staleness clears only
// by re-feeding each unit (the Refresh* methods); a partially refreshed source
// is still reported stale by [Sketch.Verify].
func (s *Sketch) MarkStale(source string) {
	for _, p := range s.points {
		if p.reference && p.source == source {
			p.stale = true
		}
	}
	for _, e := range s.ents {
		if !e.IsReference() || e.Source() != source {
			continue
		}
		for _, p := range s.refSeals[e] {
			p.stale = true
		}
		if c, ok := e.(*Circle); ok {
			c.stale = true
		}
	}
}
