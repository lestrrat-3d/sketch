package sketch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/lestrrat-3d/sketch/param"
	"github.com/lestrrat-3d/sketch/space"
)

// jsonWorldVersion is the world document schema version. It is ahead of the
// standalone-sketch jsonVersion (2): a world document carries top-level shared
// parameters and parameter-driven plane offsets, which an older (v2) reader would
// silently drop — so world documents are v3, and a v3 reader still migrates a v2
// world (promoting identical per-sketch parameter tables to the shared table).
const jsonWorldVersion = 3

// Serialization errors.
var (
	// ErrWrongDocumentKind is returned when a document's "kind" does not match
	// the decoder (e.g. a world document handed to [Sketch.UnmarshalJSON]), when
	// a version-2 document omits "kind", or when a legacy document carries a
	// version-2-only key.
	ErrWrongDocumentKind = errors.New("sketch: wrong document kind")
	// ErrMissingPlane is returned when a version-2 "sketch" document omits its
	// required plane.
	ErrMissingPlane = errors.New("sketch: version 2 sketch document is missing its plane")
)

// preflightDoc is the result of inspecting a document's top-level shape before
// the typed unmarshal: today's decoders ignore unknown fields, so the
// discriminator and v2-only keys must be checked on the raw object.
type preflightDoc struct {
	kind    string
	version int
	keys    map[string]struct{}
}

func (pf preflightDoc) has(key string) bool {
	_, ok := pf.keys[key]
	return ok
}

// preflight reads the top-level "kind"/"version" and records which top-level
// keys are present.
func preflight(data []byte) (preflightDoc, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return preflightDoc{}, err
	}
	pf := preflightDoc{keys: make(map[string]struct{}, len(raw))}
	for k := range raw {
		pf.keys[k] = struct{}{}
	}
	if r, ok := raw["kind"]; ok {
		if err := json.Unmarshal(r, &pf.kind); err != nil {
			return preflightDoc{}, err
		}
	}
	if r, ok := raw["version"]; ok {
		if err := json.Unmarshal(r, &pf.version); err != nil {
			return preflightDoc{}, err
		}
	}
	return pf, nil
}

// jsonPlane is a plane's on-disk definition, discriminated by Kind. Only the
// fields relevant to Kind are populated. A derived ("offset") plane uses BaseID,
// a plane-id reference that exists only inside a world document.
type jsonPlane struct {
	Kind     string      `json:"kind"`
	Name     string      `json:"name,omitempty"`
	Origin   *[3]float64 `json:"origin,omitempty"`    // frame
	U        *[3]float64 `json:"u,omitempty"`         // frame
	V        *[3]float64 `json:"v,omitempty"`         // frame
	A        *[3]float64 `json:"a,omitempty"`         // points
	B        *[3]float64 `json:"b,omitempty"`         // points
	C        *[3]float64 `json:"c,omitempty"`         // points
	BaseID   *int        `json:"base_id,omitempty"`   // offset (world documents only)
	Dist     float64     `json:"dist,omitempty"`      // offset literal distance
	DistExpr string      `json:"dist_expr,omitempty"` // offset: a parameter expression driving the distance
}

// plane definition kind strings.
const (
	defXY     = "worldXY"
	defXZ     = "worldXZ"
	defYZ     = "worldYZ"
	defFrame  = "frame"
	defPoints = "points"
	defOffset = "offset"
)

func vec3Arr(v space.Vec3) [3]float64 { return [3]float64{v.X, v.Y, v.Z} }
func arrVec3(a [3]float64) space.Vec3 { return space.NewVec3(a[0], a[1], a[2]) }

// planeToJSON serializes a plane's definition.
func planeToJSON(p *Plane) (jsonPlane, error) {
	jp := jsonPlane{Name: p.name}
	switch p.def.kind {
	case planeXY:
		jp.Kind = defXY
	case planeXZ:
		jp.Kind = defXZ
	case planeYZ:
		jp.Kind = defYZ
	case planeFrame:
		jp.Kind = defFrame
		o, u, v := vec3Arr(p.def.frame.Origin()), vec3Arr(p.def.frame.U()), vec3Arr(p.def.frame.V())
		jp.Origin, jp.U, jp.V = &o, &u, &v
	case planePoints:
		jp.Kind = defPoints
		a, b, c := vec3Arr(p.def.a), vec3Arr(p.def.b), vec3Arr(p.def.c)
		jp.A, jp.B, jp.C = &a, &b, &c
	case planeOffset:
		jp.Kind = defOffset
		if p.def.base == nil {
			return jsonPlane{}, fmt.Errorf("sketch: offset plane has no base")
		}
		bid := p.def.base.id
		jp.BaseID = &bid
		jp.Dist = p.def.dist
		jp.DistExpr = p.def.distExpr
	default:
		return jsonPlane{}, fmt.Errorf("sketch: unknown plane definition kind %d", p.def.kind)
	}
	return jp, nil
}

// inlinePlaneJSON serializes a standalone sketch's plane, which must be a
// world-frame datum (no derived planes outside a world).
func inlinePlaneJSON(p *Plane) (*jsonPlane, error) {
	jp, err := planeToJSON(p)
	if err != nil {
		return nil, err
	}
	if jp.Kind == defOffset {
		return nil, fmt.Errorf("sketch: cannot serialize a standalone sketch on a derived plane; marshal the World instead")
	}
	return &jp, nil
}

// planeDefFromJSON rebuilds a plane definition. base resolves an offset plane's
// base by id (a world document); standalone callers pass a resolver that errors.
func planeDefFromJSON(jp jsonPlane, base func(int) (*Plane, error)) (planeDef, error) {
	switch jp.Kind {
	case defXY:
		return planeDef{kind: planeXY}, nil
	case defXZ:
		return planeDef{kind: planeXZ}, nil
	case defYZ:
		return planeDef{kind: planeYZ}, nil
	case defFrame:
		if jp.Origin == nil || jp.U == nil || jp.V == nil {
			return planeDef{}, fmt.Errorf("sketch: frame plane needs origin, u and v")
		}
		f, err := space.NewFrame(arrVec3(*jp.Origin), arrVec3(*jp.U), arrVec3(*jp.V))
		if err != nil {
			return planeDef{}, err
		}
		return planeDef{kind: planeFrame, frame: f}, nil
	case defPoints:
		if jp.A == nil || jp.B == nil || jp.C == nil {
			return planeDef{}, fmt.Errorf("sketch: points plane needs a, b and c")
		}
		a, b, c := arrVec3(*jp.A), arrVec3(*jp.B), arrVec3(*jp.C)
		if _, err := frameFromPoints(a, b, c); err != nil {
			return planeDef{}, err
		}
		return planeDef{kind: planePoints, a: a, b: b, c: c}, nil
	case defOffset:
		if jp.BaseID == nil {
			return planeDef{}, fmt.Errorf("sketch: offset plane needs base_id")
		}
		b, err := base(*jp.BaseID)
		if err != nil {
			return planeDef{}, err
		}
		return planeDef{kind: planeOffset, base: b, dist: jp.Dist, distExpr: jp.DistExpr}, nil
	}
	return planeDef{}, fmt.Errorf("sketch: unknown plane kind %q", jp.Kind)
}

// datumPlaneFromJSON builds the inline plane of a single-sketch document as a
// world-owned datum on w: the standard datums map to the world's seeded XY/XZ/YZ
// planes, and a frame/points plane becomes a created world-owned plane. A derived
// (offset) plane is rejected — a single-sketch document cannot reference a base
// plane (only a world document carries the base it would point at).
func datumPlaneFromJSON(w *World, jp jsonPlane) (*Plane, error) {
	def, err := planeDefFromJSON(jp, func(int) (*Plane, error) {
		return nil, fmt.Errorf("sketch: a standalone sketch cannot contain a derived plane")
	})
	if err != nil {
		return nil, err
	}
	switch def.kind {
	case planeXY:
		return w.XY(), nil
	case planeXZ:
		return w.XZ(), nil
	case planeYZ:
		return w.YZ(), nil
	case planeFrame:
		p, err := w.CreatePlaneFromFrame(def.frame)
		if err != nil {
			return nil, err
		}
		p.name = jp.Name
		return p, nil
	case planePoints:
		p, err := w.CreatePlaneFromPoints(def.a, def.b, def.c)
		if err != nil {
			return nil, err
		}
		p.name = jp.Name
		return p, nil
	}
	return nil, fmt.Errorf("sketch: a standalone sketch cannot contain a derived plane")
}

// jsonWorldSketch is a sketch inside a world document: the shared body plus a
// plane-id reference into the world's planes. Plane is a pointer so a missing
// reference is rejected (as [ErrMissingPlane]) rather than silently decoding to
// id 0 (the XY datum).
type jsonWorldSketch struct {
	jsonSketchBody
	Plane *int `json:"plane"`
}

// jsonWorldDoc is the world document root.
type jsonWorldDoc struct {
	Kind       string            `json:"kind"`
	Version    int               `json:"version"`
	Parameters *param.Table      `json:"parameters,omitempty"`
	Planes     []jsonPlane       `json:"planes"`
	Sketches   []jsonWorldSketch `json:"sketches"`
}

// MarshalJSON implements [json.Marshaler], producing a world document (kind
// "world") with all planes and the sketches placed on them.
func (w *World) MarshalJSON() ([]byte, error) {
	doc := jsonWorldDoc{Kind: kindWorld, Version: jsonWorldVersion}
	if w.params != nil && len(w.params.Names()) > 0 {
		doc.Parameters = w.params
	}
	for _, p := range w.planes {
		jp, err := planeToJSON(p)
		if err != nil {
			return nil, err
		}
		doc.Planes = append(doc.Planes, jp)
	}
	for _, s := range w.sketches {
		body, err := s.marshalBody()
		if err != nil {
			return nil, err
		}
		// The shared global parameter table is serialized once at the top level;
		// a world sketch must not also serialize it (that would reintroduce split
		// per-sketch tables on load).
		if s.params == w.params {
			body.Parameters = nil
		}
		pid := 0
		if s.pl != nil {
			pid = s.pl.id
		}
		doc.Sketches = append(doc.Sketches, jsonWorldSketch{jsonSketchBody: body, Plane: &pid})
	}
	return json.Marshal(doc)
}

// UnmarshalJSON implements [json.Unmarshaler], rebuilding the world in place. It
// rejects a non-world document, validates that planes[0:3] are the XY/XZ/YZ
// datums, requires every derived plane's base to reference an earlier plane, and
// requires every sketch's plane id to be in range.
func (w *World) UnmarshalJSON(data []byte) error {
	pf, err := preflight(data)
	if err != nil {
		return err
	}
	if pf.kind != kindWorld {
		return fmt.Errorf("%w: want a world document, got %q", ErrWrongDocumentKind, pf.kind)
	}
	if pf.version > jsonWorldVersion {
		return fmt.Errorf("sketch: unsupported document version %d (this build reads up to %d)", pf.version, jsonWorldVersion)
	}

	var doc jsonWorldDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	if len(doc.Planes) < 3 {
		return fmt.Errorf("sketch: world document needs the three standard datum planes")
	}

	*w = World{}
	for i, jp := range doc.Planes {
		idx := i // capture for the base resolver
		def, err := planeDefFromJSON(jp, func(bid int) (*Plane, error) {
			if bid < 0 || bid >= idx {
				return nil, fmt.Errorf("sketch: plane %d base_id %d must reference an earlier plane", idx, bid)
			}
			return w.planes[bid], nil
		})
		if err != nil {
			return err
		}
		w.planes = append(w.planes, &Plane{def: def, owner: w, id: i, name: jp.Name})
	}
	if w.planes[0].def.kind != planeXY || w.planes[1].def.kind != planeXZ || w.planes[2].def.kind != planeYZ {
		return fmt.Errorf("sketch: world planes[0:3] must be the XY, XZ and YZ datums")
	}

	// The shared global parameter table: a v3 document carries it at the top level;
	// a legacy (v2) world carried a table per sketch, migrated below.
	w.params = doc.Parameters

	var legacy []*param.Table
	for _, jw := range doc.Sketches {
		if jw.Plane == nil {
			return ErrMissingPlane
		}
		pid := *jw.Plane
		if pid < 0 || pid >= len(w.planes) {
			return fmt.Errorf("sketch: sketch plane id %d out of range", pid)
		}
		s := newSketch(w.planes[pid])
		s.world = w
		if err := s.buildFromBody(jw.jsonSketchBody); err != nil {
			return err
		}
		if s.params != nil && s.params != w.params {
			legacy = append(legacy, s.params) // a legacy per-sketch table to migrate
		}
		w.sketches = append(w.sketches, s)
	}

	// Migrate legacy per-sketch tables: a v2 world predates the shared table, so
	// promote them only if they agree; conflicting tables cannot be a single
	// shared table and are rejected rather than silently merged.
	if w.params == nil {
		promoted, err := promoteLegacyTables(legacy)
		if err != nil {
			return err
		}
		w.params = promoted
	}
	if w.params == nil {
		w.params = param.New()
	}
	for _, s := range w.sketches { // every sketch shares the one global table
		s.params = w.params
	}
	return nil
}

// promoteLegacyTables collapses the per-sketch parameter tables of a legacy (v2)
// world document into a single shared table. It returns nil when there are none,
// the common table when they all agree (by serialized form), and an error when
// two differ — a conflict no single shared table can represent.
func promoteLegacyTables(tables []*param.Table) (*param.Table, error) {
	if len(tables) == 0 {
		return nil, nil
	}
	first, err := json.Marshal(tables[0])
	if err != nil {
		return nil, err
	}
	for _, t := range tables[1:] {
		b, err := json.Marshal(t)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(first, b) {
			return nil, fmt.Errorf("sketch: legacy world document has conflicting per-sketch parameter tables; cannot migrate to a shared global table")
		}
	}
	return tables[0], nil
}
