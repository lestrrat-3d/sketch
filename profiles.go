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
}

// BoundaryEdge is one directed edge of a region boundary: a whole sketch entity,
// or a fragment of one produced by splitting at a bare crossing.
type BoundaryEdge struct {
	// Entity is the source sketch entity this edge lies on (*Line/*Arc/*Circle/
	// *Ellipse).
	Entity Entity
	// Partial is true when this edge covers only a sub-range of Entity (the
	// entity was split at a crossing); false when it spans the whole entity.
	Partial bool
	// Reversed is true when the boundary walks Entity against its natural
	// Start→End (or counter-clockwise, for a closed entity) direction.
	Reversed bool
	// Polyline is the densified sample of this edge in walk order — the first
	// point its start, the last its end. A whole line is two points; an arc or
	// fragment is more.
	Polyline [][2]float64
}

// Profiles detects the closed planar regions formed by the sketch's
// non-construction geometry. Lines, arcs, circles and ellipses are arranged
// together and split at their bare crossings (curves that intersect without
// sharing a point), so overlapping shapes subdivide into regions and a shape
// inside another becomes a hole. Open chains and dangling spurs contribute
// nothing. Reference geometry participates like ordinary geometry; construction
// geometry is excluded. Splines are not yet considered.
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
		case *Circle:
			closed = append(closed, t.Geometry())
			closedEnts = append(closedEnts, t)
		case *Ellipse:
			closed = append(closed, t.Geometry())
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
	profiles := make([]*Profile, 0, len(arr.Regions))
	for _, r := range arr.Regions {
		p := &Profile{
			Area:             r.Area,
			SelfIntersecting: r.SelfIntersecting,
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
	}
}
