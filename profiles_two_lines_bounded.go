package sketch

import (
	"context"
	"errors"
	"math"

	"github.com/lestrrat-3d/sketch/geom"
)

// ProfilesTwoLineCapsBounded publishes two complete, hole-free line-profile
// snapshots under one work charge. Both sketches must be distinct, finite
// line scenes and must not be mutated during the call. A refused charge,
// unsupported scene, or cancellation publishes neither cap.
func ProfilesTwoLineCapsBounded(ctx context.Context, scenes [2]*Sketch,
	charge func(uint64) error) ([2][]*Profile, error) {
	var none [2][]*Profile
	if ctx == nil || charge == nil || scenes[0] == nil || scenes[1] == nil ||
		scenes[0] == scenes[1] {
		return none, geom.ErrUnsupportedLineScene
	}
	if err := ctx.Err(); err != nil {
		return none, err
	}
	var used uint64
	take := func(n uint64) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if n > 256 || n > 1<<16-used {
			return geom.ErrUnsupportedLineScene
		}
		if n == 0 {
			return nil
		}
		if err := charge(n); err != nil {
			return err
		}
		used += n
		return nil
	}
	var curves [2][]geom.Curve
	fastScene := true
	for i, scene := range scenes {
		converted, err := twoLineCapSceneCurves(scene, take)
		if errors.Is(err, geom.ErrUnsupportedLineScene) {
			fastScene = false
			break
		}
		if err != nil {
			return none, err
		}
		curves[i] = converted
	}
	if fastScene {
		arrangements, err := geom.RegionsTwoConvexLineCapsBounded(ctx, curves, take)
		if err == nil {
			return twoLineCapProfiles(ctx, scenes, arrangements, take)
		}
		if !errors.Is(err, geom.ErrUnsupportedLineScene) {
			return none, err
		}
	}
	var profiles [2][]*Profile
	for i, scene := range scenes {
		cap, err := scene.ProfilesLinesBounded(ctx, take)
		if err != nil {
			return none, err
		}
		if len(cap) == 0 {
			return none, geom.ErrUnsupportedLineScene
		}
		for _, profile := range cap {
			if err := take(1); err != nil {
				return none, err
			}
			if profile == nil || !profile.Valid || profile.SelfIntersecting ||
				len(profile.Outer) < 3 || len(profile.Holes) != 0 ||
				math.IsNaN(profile.Area) || math.IsInf(profile.Area, 0) || profile.Area <= 0 {
				return none, geom.ErrUnsupportedLineScene
			}
		}
		profiles[i] = cap
	}
	if err := ctx.Err(); err != nil {
		return none, err
	}
	return profiles, nil
}

func twoLineCapSceneCurves(scene *Sketch, take func(uint64) error) ([]geom.Curve, error) {
	if len(scene.ents) != 8 || len(scene.points) != 8 || len(scene.vars) > 34 ||
		len(scene.fixed) != len(scene.vars) || len(scene.cons) != 0 ||
		len(scene.refSeals) != 0 || scene.origin == nil {
		return nil, geom.ErrUnsupportedLineScene
	}
	if err := take(17); err != nil { // nine point inspections and eight slots
		return nil, err
	}
	checkPoint := func(p *Point) bool {
		return scene.owns(p) && p.xi >= 0 && p.yi >= 0 &&
			p.xi < len(scene.vars) && p.yi < len(scene.vars) &&
			!math.IsNaN(p.x()) && !math.IsInf(p.x(), 0) &&
			!math.IsNaN(p.y()) && !math.IsInf(p.y(), 0)
	}
	if !checkPoint(scene.origin) {
		return nil, geom.ErrUnsupportedLineScene
	}
	for _, point := range scene.points {
		if !checkPoint(point) {
			return nil, geom.ErrUnsupportedLineScene
		}
	}
	if err := take(16); err != nil { // point map and curve slice
		return nil, err
	}
	points := make(map[*Point]*geom.Point, 8)
	point := func(p *Point) *geom.Point {
		if g, ok := points[p]; ok {
			return g
		}
		g := p.Geometry()
		points[p] = g
		return g
	}
	curves := make([]geom.Curve, 0, 8)
	for _, entity := range scene.ents {
		if err := take(4); err != nil {
			return nil, err
		}
		line, ok := entity.(*Line)
		if !ok || line == nil || !scene.ownsEntity(line) ||
			line.IsConstruction() || line.source != "" ||
			!scene.owns(line.Start) || !scene.owns(line.End) {
			return nil, geom.ErrUnsupportedLineScene
		}
		curves = append(curves, geom.NewLine(point(line.Start), point(line.End)))
	}
	return curves, nil
}

func twoLineCapProfiles(ctx context.Context, scenes [2]*Sketch,
	arrangements [2]*geom.Arrangement, take func(uint64) error) ([2][]*Profile, error) {
	var none, profiles [2][]*Profile
	for i, scene := range scenes {
		rev, err := scene.revisionWithCharge(take)
		if err != nil {
			return none, err
		}
		arr := arrangements[i]
		if arr == nil || arr.Degenerate || len(arr.Regions) != 3 || len(arr.Chains) != 0 {
			return none, geom.ErrUnsupportedLineScene
		}
		if err := take(3); err != nil {
			return none, err
		}
		out := make([]*Profile, 0, 3)
		for _, region := range arr.Regions {
			if err := take(1); err != nil {
				return none, err
			}
			if region == nil || region.Degenerate || region.SelfIntersecting ||
				len(region.Holes) != 0 || len(region.Outer) < 3 ||
				math.IsNaN(region.Area) || math.IsInf(region.Area, 0) || region.Area <= 0 {
				return none, geom.ErrUnsupportedLineScene
			}
			if err := take(uint64(len(region.Outer) * 4)); err != nil {
				return none, err
			}
			p := &Profile{Area: region.Area, Valid: true, sketch: scene, revision: rev}
			seen := make(map[Entity]struct{}, len(region.Outer))
			for _, edge := range region.Outer {
				if edge.SourceIndex < 0 || edge.SourceIndex >= len(scene.ents) ||
					len(edge.Polyline) != 2 {
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
