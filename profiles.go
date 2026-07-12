package sketch

import "github.com/lestrrat-3d/sketch/geom"

// Profile is a closed planar region detected in a sketch. Its outer boundary is
// an ordered loop of edges; each edge is a whole sketch entity or a fragment of
// one (a curve split where it crosses another). A region may enclose holes. A
// single closed primitive (circle or ellipse) is a region on its own.
//
// Profiles are what downstream operations (eventually extrude/revolve) consume,
// so an [Entity]'s shape, closure, area and validity are observable here.
type Profile struct {
	// Entities is the de-duplicated set of distinct sketch entities on the
	// OUTER boundary, in first-seen walk order. For a boundary with no bare
	// crossings this is the historical contract unchanged (a rectangle is four
	// lines, a circle is one circle); a curve split at a crossing appears once.
	Entities []Entity
	// Outer is the ordered outer-boundary edge loop, counter-clockwise. Each
	// edge is a whole entity or a fragment of one.
	Outer []BoundaryEdge
	// Holes are inner boundary loops (each clockwise), nil when the region is
	// simply connected. A hole is a void in this region — a separate region may
	// also occupy it.
	Holes [][]BoundaryEdge
	// Area is the net region area (outer minus holes) in base units (mm²),
	// >= 0 for a clean region, 0 for a degenerate one.
	Area float64
	// Valid is false when the region cannot be trusted as an extrudable profile:
	// a self-intersecting or degenerate boundary, or a degenerate arrangement.
	Valid bool
	// SelfIntersecting marks the specific invalidity that the boundary the
	// region derives from crosses or touches itself.
	SelfIntersecting bool

	// sketch is the sketch this profile was built from, and revision that
	// sketch's [Sketch.Revision] at build time. Together they let a consumer ask
	// whether the profile still describes its sketch — see [Profile.IsStale].
	sketch   *Sketch
	revision uint64
}

// Sketch returns the sketch this profile was built from.
//
// A [Profile] is a snapshot, freshly allocated by every [Sketch.Profiles] call,
// so pointer identity can never establish provenance; this can.
func (p *Profile) Sketch() *Sketch { return p.sketch }

// Revision is the value of [Sketch.Revision] at the moment this profile was
// built. Compare it against the sketch's current revision to detect staleness —
// or just call [Profile.IsStale].
func (p *Profile) Revision() uint64 { return p.revision }

// IsStale reports whether the sketch has changed since this profile was built,
// so the profile no longer describes it.
//
// A profile is a snapshot of geometry at one instant. Solving the sketch, editing
// a driving parameter, or adding/removing geometry moves that geometry — but the
// profile still holds the OLD boundary, and its entities still belong to the
// sketch, so nothing about the handle looks wrong:
//
//	prof := s.Profiles()[0]
//	s.Params().SetValue("height", units.Millimeters(60))
//	s.Solve(ctx)          // geometry moves; prof now describes the old shape
//	prof.IsStale()        // true — rebuild with s.Profiles() before using it
//
// A consumer that turns a profile into a solid (extrude, revolve) or records it
// must check this first: extruding a stale profile silently builds the wrong
// part, with no error anywhere to catch it.
func (p *Profile) IsStale() bool {
	if p.sketch == nil {
		return false // a zero-value Profile was never built from a sketch
	}
	return p.sketch.Revision() != p.revision
}

// BoundaryEdge is one directed edge of a region boundary: a whole sketch entity,
// or a fragment of one produced by splitting at a bare crossing.
type BoundaryEdge struct {
	// Entity is the source sketch entity this edge lies on (*Line/*Arc/*Circle/
	// *Ellipse).
	Entity Entity
	// Partial is true when this edge covers only a sub-range of Entity; false when
	// it spans the whole entity.
	//
	// It is decided by each bound's provenance — the edge is whole exactly when BOTH
	// of its bounds are the entity's own domain ends (an open curve's endpoint, or a
	// closed curve's seam) rather than a crossing — and never by asking whether
	// [TStart, TEnd] comes numerically close to [0,1], which cannot distinguish a
	// bound that IS the entity's end from a crossing that landed 1e-10 from it. So an
	// entity whose only crossing bounds nothing (the partner dangles and is pruned) is
	// covered whole and reads Partial = false, while an entity grazed by a crossing a
	// hair from its seam reads Partial = true — it is a fragment, however little of it
	// is missing. See [geom.BoundaryEdge.Whole].
	Partial bool
	// Reversed is true when the boundary walks Entity against its natural
	// Start→End (or counter-clockwise, for a closed entity) direction.
	Reversed bool
	// Polyline is the densified sample of this edge in walk order — the first
	// point its start, the last its end. A whole line is two points; an arc or
	// fragment is more.
	Polyline [][2]float64
	// TStart and TEnd are this edge's parameter range on Entity, in the entity's
	// NATURAL parameter direction — so TStart < TEnd always, and Reversed (not
	// their order) says the walk runs backwards. A whole edge spans the entity's
	// full domain; the range never wraps. Partial alone cannot tell you WHICH
	// sub-range an edge covers — this is what can.
	//
	// The parameter is normalized t in [0,1], not the entity's own angle/knot
	// parameter; see [geom.BoundaryEdge] for the per-type mapping (an *Arc's t
	// is the fraction of its sweep, a *Circle's is the angle from +x over 2π, a
	// *NURBS's maps linearly onto its knot domain, and so on).
	TStart, TEnd float64
	// TExact reports whether TStart/TEnd are the TRUE parameters on Entity rather
	// than sampling-accurate approximations.
	//
	// It is false whenever either bound came from a sampled crossing. The closed-form
	// kernel runs only when BOTH entities of a pair are a *Line, *Circle or *Arc — a
	// rule about the PAIR, not about "a *Line was involved", and not about the contact
	// being a tangency. So every contact involving an *Ellipse, *EllipticalArc,
	// *Conic, *Spline, *ClosedSpline, *FitSpline or *NURBS is sampled, INCLUDING a
	// plain *Line crossing one and a plain *Line TANGENT to one; so is every
	// curve/curve crossing. A fragment of a free-form curve reports TExact = false.
	//
	// The topology is still correct when this is false; only the parameter (and
	// the Polyline, equally) converges with sampling rather than being exact. A
	// consumer that records the profile structurally or emits CAD code from it
	// must check this and reject, not round-trip an approximate range as if it
	// were the real one.
	TExact bool
}

// Profiles detects the closed planar regions formed by the sketch's
// non-construction geometry. Lines, arcs, circles and ellipses are arranged
// together and split at their bare crossings (curves that intersect without
// sharing a point), so overlapping shapes subdivide into regions and a shape
// inside another becomes a hole. Open chains and dangling spurs contribute
// nothing. Reference geometry participates like ordinary geometry; construction
// geometry is excluded. Splines participate too — sampled to a polyline like
// arcs/ellipses, with a sampled fragment area and self-crossing detection.
//
// Each region reports its outer boundary, holes, net area, and whether it is a
// valid (non-self-intersecting, non-degenerate) extrudable profile. A region
// touched by an unresolvable (degenerate) arrangement — coincident edges or an
// ill-conditioned near-tangent crossing — is reported invalid.
func (s *Sketch) Profiles() []*Profile {
	profiles, _, _ := s.buildProfiles()
	return profiles
}

// buildProfiles runs the arrangement and returns the profiles together with the
// arrangement-level degeneracy signal — collinear-overlap / near-tangent
// conditions that make the region set unverifiable even when (or especially
// when) no region is produced. Verify consumes the extra signals; Profiles
// exposes only the regions.
func (s *Sketch) buildProfiles() ([]*Profile, bool, [][2]float64) {
	var curves []geom.Curve
	var closed []geom.ClosedCurve
	var openEnts, closedEnts []Entity

	// Share one geom.Point per sketch point so entities meeting at a shared
	// point connect in the arrangement (the natural state of shared-point
	// geometry); coincident-but-unshared points stay distinct.
	gpt := map[*Point]*geom.Point{}
	pt := func(p *Point) *geom.Point {
		if g, ok := gpt[p]; ok {
			return g
		}
		g := p.Geometry()
		gpt[p] = g
		return g
	}

	for _, e := range s.ents {
		if e.IsConstruction() {
			continue
		}
		switch t := e.(type) {
		case *Line:
			curves = append(curves, geom.NewLine(pt(t.Start), pt(t.End)))
			openEnts = append(openEnts, t)
		case *Arc:
			curves = append(curves, geom.NewArc(pt(t.Center), pt(t.Start), pt(t.End)))
			openEnts = append(openEnts, t)
		case *EllipticalArc:
			curves = append(curves, geom.NewEllipticalArc(pt(t.Center), pt(t.Start), pt(t.End), t.rx(), t.ry(), t.rot()))
			openEnts = append(openEnts, t)
		case *Conic:
			// A conic is an open curve (like an arc), built over the shared
			// geom.Point map so its endpoints join adjacent geometry by identity.
			curves = append(curves, geom.NewConic(pt(t.Start), pt(t.Apex), pt(t.End), t.rho()))
			openEnts = append(openEnts, t)
		case *Spline:
			// Build over the shared geom.Point map so the spline's first/last
			// control points (the curve's endpoints) join adjacent lines/arcs by
			// pointer identity, exactly like a line's endpoints.
			ctrl := make([]*geom.Point, len(t.Control))
			for i, cp := range t.Control {
				if cp != nil { // CreateSpline rejects nil; stay panic-safe regardless
					ctrl[i] = pt(cp)
				}
			}
			curves = append(curves, &geom.Spline{Control: ctrl})
			openEnts = append(openEnts, t)
		case *FitSpline:
			// An interpolating spline passes through its fit points (its endpoints
			// are the first/last fit point), so it is an open Curve; build over the
			// shared geom.Point map so those join adjacent geometry by identity.
			fit := make([]*geom.Point, len(t.Fit))
			for i, fp := range t.Fit {
				if fp != nil {
					fit[i] = pt(fp)
				}
			}
			curves = append(curves, &geom.FitSpline{Fit: fit})
			openEnts = append(openEnts, t)
		case *NURBS:
			// An open curve like the spline; build over the shared geom.Point map so
			// its first/last control points (the endpoints) join adjacent geometry by
			// identity. Degree/knots/weights are the entity's stored structural data.
			ctrl := make([]*geom.Point, len(t.Control))
			for i, cp := range t.Control {
				if cp != nil { // CreateNURBS rejects nil; stay panic-safe regardless
					ctrl[i] = pt(cp)
				}
			}
			curves = append(curves, &geom.NURBS{
				Degree:  t.degree,
				Control: ctrl,
				Knots:   append([]float64(nil), t.knots...),
				Weights: append([]float64(nil), t.weights...),
			})
			openEnts = append(openEnts, t)
		case *Circle:
			closed = append(closed, t.Geometry())
			closedEnts = append(closedEnts, t)
		case *Ellipse:
			closed = append(closed, t.Geometry())
			closedEnts = append(closedEnts, t)
		case *ClosedSpline:
			// A closed spline bounds a region on its own, so it is a ClosedCurve.
			// Build over the shared geom.Point map for consistency (its control
			// points may be shared with other geometry).
			ctrl := make([]*geom.Point, len(t.Control))
			for i, cp := range t.Control {
				if cp != nil {
					ctrl[i] = pt(cp)
				}
			}
			closed = append(closed, &geom.ClosedSpline{Control: ctrl})
			closedEnts = append(closedEnts, t)
		}
	}

	entityFor := func(srcIndex int) Entity {
		if srcIndex < len(openEnts) {
			return openEnts[srcIndex]
		}
		return closedEnts[srcIndex-len(openEnts)]
	}

	arr := geom.Regions(curves, closed)
	// Stamp every profile with the state it was built from, so a later mutation
	// makes it detectably stale (Profile.IsStale) instead of silently wrong.
	rev := s.Revision()
	profiles := make([]*Profile, 0, len(arr.Regions))
	for _, r := range arr.Regions {
		p := &Profile{
			Area:             r.Area,
			SelfIntersecting: r.SelfIntersecting,
			sketch:           s,
			revision:         rev,
		}
		seen := map[Entity]struct{}{}
		for _, ge := range r.Outer {
			be := mapBoundaryEdge(ge, entityFor)
			p.Outer = append(p.Outer, be)
			if _, ok := seen[be.Entity]; !ok {
				seen[be.Entity] = struct{}{}
				p.Entities = append(p.Entities, be.Entity)
			}
		}
		for _, hole := range r.Holes {
			var he []BoundaryEdge
			for _, ge := range hole {
				he = append(he, mapBoundaryEdge(ge, entityFor))
			}
			p.Holes = append(p.Holes, he)
		}
		p.Valid = !r.SelfIntersecting && r.Area > areaEps && !arr.Degenerate
		profiles = append(profiles, p)
	}
	return profiles, arr.Degenerate, arr.Degeneracies
}

// areaEps is the smallest area a region must enclose to count as non-degenerate.
const areaEps = 1e-9

func mapBoundaryEdge(ge geom.BoundaryEdge, entityFor func(int) Entity) BoundaryEdge {
	return BoundaryEdge{
		Entity:   entityFor(ge.SourceIndex),
		Partial:  !ge.Whole,
		Reversed: ge.Reversed,
		Polyline: ge.Polyline,
		TStart:   ge.TStart,
		TEnd:     ge.TEnd,
		TExact:   ge.TExact,
	}
}
