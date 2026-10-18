package sketch

import (
	"context"
	"math"

	"github.com/lestrrat-3d/sketch/geom"
)

// ProfilesTwoRectangleCapsBounded publishes two separate, live profile
// snapshots for the strict rectangular right-wall crossing accepted by
// geom.RegionsTwoRectangleCapsBounded. It never publishes just one cap.
// The sketches must not be mutated concurrently with this call.
func ProfilesTwoRectangleCapsBounded(ctx context.Context, scenes [2]*Sketch,
	charge func(uint64) error) ([2][]*Profile, error) {
	var none [2][]*Profile
	var used uint64
	if ctx == nil || charge == nil || scenes[0] == nil || scenes[1] == nil ||
		scenes[0] == scenes[1] {
		return none, geom.ErrUnsupportedLineScene
	}
	take := func(n uint64) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if n > 256 || n > 1<<16-used {
			return geom.ErrUnsupportedLineScene
		}
		if err := charge(n); err != nil {
			return err
		}
		used += n
		return nil
	}
	var curves [2][]geom.Curve
	for i, scene := range scenes {
		converted, err := twoRectangleSceneCurves(scene, take)
		if err != nil {
			return none, err
		}
		curves[i] = converted
	}
	arrangements, err := geom.RegionsTwoRectangleCapsBounded(ctx, curves, take)
	if err != nil {
		return none, err
	}
	var profiles [2][]*Profile
	for i, scene := range scenes {
		rev, err := scene.revisionWithCharge(take)
		if err != nil {
			return none, err
		}
		arr := arrangements[i]
		if arr == nil || arr.Degenerate || len(arr.Regions) != 3 ||
			len(arr.Chains) != 0 {
			return none, geom.ErrUnsupportedLineScene
		}
		if err := take(uint64(len(arr.Regions))); err != nil {
			return none, err
		}
		out := make([]*Profile, 0, len(arr.Regions))
		for _, region := range arr.Regions {
			if err := take(1); err != nil {
				return none, err
			}
			if region == nil || region.Degenerate || region.SelfIntersecting ||
				len(region.Holes) != 0 || len(region.Outer) == 0 ||
				math.IsNaN(region.Area) || math.IsInf(region.Area, 0) || region.Area <= 0 {
				return none, geom.ErrUnsupportedLineScene
			}
			if err := take(uint64(len(region.Outer))); err != nil {
				return none, err
			}
			p := &Profile{Area: region.Area, Valid: true, sketch: scene, revision: rev}
			seen := make(map[Entity]struct{}, len(region.Outer))
			for _, edge := range region.Outer {
				if err := take(1 + uint64(len(edge.Polyline))); err != nil {
					return none, err
				}
				if edge.SourceIndex < 0 || edge.SourceIndex >= len(scene.ents) {
					return none, geom.ErrUnsupportedLineScene
				}
				mapped := mapBoundaryEdge(edge, func(j int) Entity { return scene.ents[j] }, scene, rev)
				p.Outer = append(p.Outer, mapped)
				if _, ok := seen[mapped.Entity]; !ok {
					seen[mapped.Entity] = struct{}{}
					p.Entities = append(p.Entities, mapped.Entity)
				}
			}
			if err := take(uint64(len(p.Outer))); err != nil {
				return none, err
			}
			stampJunctionWalk(p.Outer, true)
			out = append(out, p)
		}
		profiles[i] = out
	}
	if err := ctx.Err(); err != nil {
		return none, err
	}
	return profiles, nil
}

func twoRectangleSceneCurves(scene *Sketch,
	take func(uint64) error) ([]geom.Curve, error) {
	if len(scene.ents) != 8 || len(scene.points) != 8 ||
		len(scene.vars) > 34 || len(scene.fixed) != len(scene.vars) ||
		len(scene.cons) != 0 || len(scene.refSeals) != 0 || scene.origin == nil {
		return nil, geom.ErrUnsupportedLineScene
	}
	checkPoint := func(p *Point) error {
		if err := take(1); err != nil {
			return err
		}
		if !scene.owns(p) || p.xi < 0 || p.yi < 0 ||
			p.xi >= len(scene.vars) || p.yi >= len(scene.vars) ||
			math.IsNaN(p.x()) || math.IsInf(p.x(), 0) ||
			math.IsNaN(p.y()) || math.IsInf(p.y(), 0) {
			return geom.ErrUnsupportedLineScene
		}
		return nil
	}
	if err := checkPoint(scene.origin); err != nil {
		return nil, err
	}
	for _, p := range scene.points {
		if err := checkPoint(p); err != nil {
			return nil, err
		}
	}
	// Reserve the eight point-map and eight curve slots before either allocation.
	if err := take(16); err != nil {
		return nil, err
	}
	points := make(map[*Point]*geom.Point, len(scene.points))
	point := func(p *Point) *geom.Point {
		if g, ok := points[p]; ok {
			return g
		}
		g := p.Geometry()
		points[p] = g
		return g
	}
	curves := make([]geom.Curve, 0, 8)
	for _, ent := range scene.ents {
		if err := take(4); err != nil {
			return nil, err
		}
		line, ok := ent.(*Line)
		if !ok || line == nil || !scene.ownsEntity(line) ||
			line.IsConstruction() || line.source != "" ||
			!scene.owns(line.Start) || !scene.owns(line.End) {
			return nil, geom.ErrUnsupportedLineScene
		}
		curves = append(curves, geom.NewLine(point(line.Start), point(line.End)))
	}
	for _, fixed := range scene.fixed {
		if err := take(1); err != nil {
			return nil, err
		}
		if !fixed {
			return nil, geom.ErrUnsupportedLineScene
		}
	}
	return curves, nil
}
