package sketch

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/lestrrat-3d/sketch/param"
	"github.com/lestrrat-3d/units"
)

// On-disk representation. Points and entities are referenced by their stable
// creation index, which is reproduced exactly when the sketch is rebuilt.

type jsonPoint struct {
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	Fixed        bool    `json:"fixed,omitempty"`
	Name         string  `json:"name,omitempty"`
	Construction bool    `json:"construction,omitempty"`
	Reference    bool    `json:"reference,omitempty"`
	Source       string  `json:"source,omitempty"`
	Stale        bool    `json:"stale,omitempty"`
}

type jsonEntity struct {
	Type         string    `json:"type"` // "line" | "circle" | "arc" | "ellipse" | "spline"
	Points       []int     `json:"points"`
	Radius       float64   `json:"radius,omitempty"`
	Rx           float64   `json:"rx,omitempty"`       // ellipse semi-axis (local x)
	Ry           float64   `json:"ry,omitempty"`       // ellipse semi-axis (local y)
	Rotation     float64   `json:"rotation,omitempty"` // ellipse frame rotation, radians
	Rho          float64   `json:"rho,omitempty"`      // conic fullness parameter, in (0, 1)
	Degree       int       `json:"degree,omitempty"`   // spline degree (always 3 today); NURBS degree
	Knots        []float64 `json:"knots,omitempty"`    // NURBS knot vector
	Weights      []float64 `json:"weights,omitempty"`  // NURBS per-control weights
	Name         string    `json:"name,omitempty"`     // optional entity label
	Construction bool      `json:"construction,omitempty"`
	Reference    bool      `json:"reference,omitempty"`
	Source       string    `json:"source,omitempty"`
	Stale        bool      `json:"stale,omitempty"` // reference circle radius freshness only
}

type jsonConstraint struct {
	Type     string  `json:"type"`
	Name     string  `json:"name,omitempty"` // optional constraint label
	Points   []int   `json:"points,omitempty"`
	Entities []int   `json:"entities,omitempty"`
	Value    float64 `json:"value,omitempty"`
	Unit     string  `json:"unit,omitempty"`   // dimension's unit symbol
	Expr     string  `json:"expr,omitempty"`   // parameter binding on a dimension
	Driven   bool    `json:"driven,omitempty"` // reference dimension flag
	Flag     bool    `json:"flag,omitempty"`
}

type jsonSystem struct {
	Length string `json:"length"`
	Angle  string `json:"angle"`
}

// jsonVersion is the BASELINE standalone-sketch document version. Version-2
// documents carry an explicit "kind" ("sketch" or "world") and placement. Legacy
// documents (version absent/0/1, no "kind") still load as world-XY sketches;
// documents from a newer schema are rejected rather than mis-loaded.
const jsonVersion = 2

// jsonOriginVersion is the version a document of EITHER kind declares once it
// references the sketch origin ([Sketch.Origin]) — from a constraint operand or
// from an entity's defining points, both of which serialize a point by id. Such
// a document carries a point reference no older reader resolves, so it must not
// be handed to one.
//
// The version is stamped ON DEMAND rather than unconditionally: a document that
// never touches the origin is byte-identical to what earlier builds wrote and
// stays readable by them. The number a document declares is the OLDEST reader
// that can read it faithfully, not the newest writer that produced it.
const jsonOriginVersion = 4

// jsonMaxVersion is the newest schema THIS build reads, for either kind. Both
// loaders reject anything above it rather than mis-load.
const jsonMaxVersion = jsonOriginVersion

// referencesOrigin reports whether the document this sketch serializes to carries
// a reference to the sketch's origin point, which is what forces the higher
// document version. BOTH shapes of point reference count: an ENTITY's defining
// points and a constraint's operands. An entity writes its points' ids exactly as
// a constraint does, so a line drawn from the origin puts the reserved id in the
// document with no constraint involved.
//
// Each half reads through the very accessor marshalBody serializes from —
// [entityPoints] for entities, constraintRefs for constraints — so a type
// that can reach the origin cannot be written there while being missed here.
func (s *Sketch) referencesOrigin() bool {
	if s.origin == nil {
		return false
	}
	for _, e := range s.ents {
		if slices.Contains(entityPoints(e), s.origin) {
			return true
		}
	}
	for _, c := range s.cons {
		pts, _ := constraintRefs(c)
		if slices.Contains(pts, s.origin) {
			return true
		}
	}
	return false
}

// checkNoForeignRefs rejects a point or entity reference this sketch does not
// own. Every reference is serialized as a bare id and the loader resolves that id
// against the RECEIVING sketch, so writing a foreign one launders it into a local
// one across a round trip: the reloaded document holds an ordinary local relation
// where the original had the cross-sketch reference [Sketch.Verify] reports as
// [ErrForeignHandle]. The reload is CLEAN — nothing is left for the oracle to
// flag — so the round trip would turn a rejected sketch into a blessed one. That
// is the whole reason marshalling refuses instead of writing.
//
// The origin is the sharpest case, and the one that needs no id collision at all:
// it carries the reserved id originPointID, which [Sketch.pointRef] resolves to
// the READER's own origin, so a borrowed origin always comes back rebound. An
// ordinary foreign point instead rebinds whenever its positional id happens to
// name a local point — the likely case, since small ids are the common ones.
//
// Ownership is decided by [Sketch.owns] — the SAME predicate scanReferenceIntegrity
// uses to set ForeignHandles, so marshal and [Sketch.Verify] cannot diverge. A
// weaker screen on the point's own sketch pointer (p.s != s) would pass a DEAD
// point of this sketch, one [Sketch.RemovePoint] spliced out: its s still names
// this sketch while its id is stale, so the document would write the id a
// DIFFERENT live point has since inherited and the reload would bind the
// reference to that point — silently, with nothing left to flag — or, when the
// removed point was the last one, write an id out of range and produce a document
// that marshals cleanly and then fails to load. `owns` also carries the origin
// exception (it is deliberately absent from s.points, so a positional check alone
// would call it foreign). The entity half reuses [Sketch.ownsEntity], which is
// positional in the same way.
//
// A nil point is screened out first rather than reported here: [Sketch.Verify]
// splits a nil operand out as a corrupt reference, not a foreign one, and the
// entity half skips a nil operand for the same reason.
func (s *Sketch) checkNoForeignRefs(pts []*Point, ents []Entity, what string) error {
	for _, p := range pts {
		if p != nil && !s.owns(p) {
			return fmt.Errorf("%w: %s references a point this sketch does not own", ErrForeignHandle, what)
		}
	}
	for _, e := range ents {
		if !isNilEntity(e) && !s.ownsEntity(e) {
			return fmt.Errorf("%w: %s references an entity this sketch does not own", ErrForeignHandle, what)
		}
	}
	return nil
}

// docVersion is the version a document containing this sketch must declare: the
// given baseline for its kind, raised when the sketch uses a feature an older
// reader cannot resolve.
func (s *Sketch) docVersion(baseline int) int {
	if s.referencesOrigin() {
		return jsonOriginVersion
	}
	return baseline
}

// Document kind discriminators (the top-level "kind" field).
const (
	kindSketch = "sketch"
	kindWorld  = "world"
)

// jsonSketchBody is the kind/version-less payload shared by a standalone sketch
// document and a sketch element inside a world document. Decoding goes through
// (*Sketch).buildFromBody for both, so reference validation and constraint
// reconstruction live in exactly one place.
type jsonSketchBody struct {
	Points      []jsonPoint      `json:"points"`
	Entities    []jsonEntity     `json:"entities"`
	Constraints []jsonConstraint `json:"constraints"`
	Units       *jsonSystem      `json:"units,omitempty"`
	Parameters  *param.Table     `json:"parameters,omitempty"`
}

// jsonSketchDoc is a standalone (engine-only) sketch document: the shared body
// plus a kind/version wrapper and an inline world-frame datum plane.
type jsonSketchDoc struct {
	Kind    string `json:"kind,omitempty"`
	Version int    `json:"version"`
	jsonSketchBody
	Plane *jsonPlane `json:"plane,omitempty"`
}

// pointIDs collects the serialized ids of a point slice, preserving order.
func pointIDs(ps []*Point) []int {
	ids := make([]int, len(ps))
	for k, p := range ps {
		ids[k] = p.id
	}
	return ids
}

// dimJSON builds the serialized form of a dimensional constraint.
func dimJSON(typ string, d Dimension, points, entities []int) jsonConstraint {
	t := d.Target()
	return jsonConstraint{
		Type: typ, Points: points, Entities: entities,
		Value: t.Mag(), Unit: t.Unit().Symbol(), Expr: d.driverExpr(), Driven: d.Driven(),
	}
}

// dimUnit resolves a stored unit symbol for a dimension of the given kind,
// falling back to the kind's base unit. A dimension kind is always
// length or angle, both of which have a base unit, so the fallback never
// fails in practice; the error is returned rather than panicked to honour the
// no-panic contract.
func dimUnit(symbol string, kind units.Kind) (units.Unit, error) {
	if u, ok := units.Lookup(symbol); ok && u.Kind() == kind {
		return u, nil
	}
	bu, ok := units.BaseUnit(kind)
	if !ok {
		return units.Unit{}, fmt.Errorf("sketch: %s dimension has no base unit", kind)
	}
	return bu, nil
}

// restoreDim reinstates a deserialized dimension's unit, parameter binding and
// driven flag.
func restoreDim(d Dimension, jc jsonConstraint) error {
	u, err := dimUnit(jc.Unit, d.Kind())
	if err != nil {
		return err
	}
	d.restore(jc.Value, u)
	d.setDriverExpr(jc.Expr)
	d.SetDriven(jc.Driven)
	return nil
}

// MarshalJSON implements [json.Marshaler], producing a portable, reloadable
// standalone sketch document (kind "sketch") with the sketch's plane inlined.
// The plane must be a world-frame datum; a sketch on a derived (world-owned)
// plane must be serialized through its [World] instead.
//
// It fails with an error wrapping [ErrForeignHandle] when a constraint operand or
// an entity's defining point belongs to another sketch. A reference serializes as
// a bare id that the loader resolves against the receiving sketch, so writing a
// foreign one would silently rebind it to a local point or entity and the reload
// would read clean. This is exactly the sketch [Sketch.Verify] already reports as
// ForeignHandles, so a sketch its report calls trustworthy always marshals.
func (s *Sketch) MarshalJSON() ([]byte, error) {
	body, err := s.marshalBody()
	if err != nil {
		return nil, err
	}
	jp, err := inlinePlaneJSON(s.plane())
	if err != nil {
		return nil, err
	}
	return json.Marshal(jsonSketchDoc{
		Kind: kindSketch, Version: s.docVersion(jsonVersion),
		jsonSketchBody: body, Plane: jp,
	})
}

// marshalBody builds the shared, placement-free payload of a sketch.
func (s *Sketch) marshalBody() (jsonSketchBody, error) {
	body := jsonSketchBody{
		Points:      make([]jsonPoint, 0, len(s.points)),
		Entities:    make([]jsonEntity, 0, len(s.ents)),
		Constraints: make([]jsonConstraint, 0, len(s.cons)),
	}

	for _, p := range s.points {
		body.Points = append(body.Points, jsonPoint{
			X: p.x(), Y: p.y(), Fixed: p.IsFixed(),
			Name: p.name, Construction: p.construction,
			Reference: p.reference, Source: p.source, Stale: p.stale,
		})
	}

	for _, e := range s.ents {
		// Every entity's point references come from entityPoints — the same
		// accessor referencesOrigin and checkNoForeignRefs read — so an entity type
		// cannot be serialized here while those two miss it. The order it returns is
		// the order each type's document form declares.
		epts := entityPoints(e)
		if err := s.checkNoForeignRefs(epts, nil, fmt.Sprintf("entity %T", e)); err != nil {
			return jsonSketchBody{}, err
		}
		pts := pointIDs(epts)
		before := len(body.Entities)
		switch t := e.(type) {
		case *Line:
			body.Entities = append(body.Entities, jsonEntity{
				Type: "line", Points: pts, Construction: t.construction,
				Reference: t.reference, Source: t.source,
			})
		case *Circle:
			body.Entities = append(body.Entities, jsonEntity{
				Type: "circle", Points: pts, Radius: t.r(), Construction: t.construction,
				Reference: t.reference, Source: t.source, Stale: t.stale, // circle: radius freshness
			})
		case *Arc:
			body.Entities = append(body.Entities, jsonEntity{
				Type: "arc", Points: pts, Construction: t.construction,
				Reference: t.reference, Source: t.source,
			})
		case *Ellipse:
			body.Entities = append(body.Entities, jsonEntity{
				Type: "ellipse", Points: pts,
				Rx: t.rx(), Ry: t.ry(), Rotation: t.rot(), Construction: t.construction,
			})
		case *EllipticalArc:
			body.Entities = append(body.Entities, jsonEntity{
				Type: "elliptical_arc", Points: pts,
				Rx: t.rx(), Ry: t.ry(), Rotation: t.rot(), Construction: t.construction,
			})
		case *Spline:
			body.Entities = append(body.Entities, jsonEntity{
				Type: "spline", Degree: 3, Construction: t.construction,
				Points: pts,
			})
		case *ClosedSpline:
			// A distinct type (not a "closed" flag on "spline") so an older reader
			// rejects it as unknown rather than silently loading it as an open spline.
			body.Entities = append(body.Entities, jsonEntity{
				Type: "closed_spline", Degree: 3, Construction: t.construction,
				Points: pts,
			})
		case *FitSpline:
			// Distinct type: the points are FIT points (interpolated), not control
			// points; the interpolant is recomputed on load, never serialized.
			body.Entities = append(body.Entities, jsonEntity{
				Type: "fit_spline", Degree: 3, Construction: t.construction,
				Points: pts,
			})
		case *Conic:
			body.Entities = append(body.Entities, jsonEntity{
				Type: "conic", Points: pts,
				Rho: t.rho(), Construction: t.construction,
			})
		case *NURBS:
			body.Entities = append(body.Entities, jsonEntity{
				Type: "nurbs", Degree: t.degree, Construction: t.construction,
				Knots:   append([]float64(nil), t.knots...),
				Weights: append([]float64(nil), t.weights...),
				Points:  pts,
			})
		}
		// Attach the optional label to whichever entity this iteration appended.
		if len(body.Entities) > before {
			body.Entities[before].Name = e.Name()
		}
	}

	for _, c := range s.cons {
		if _, ok := c.(internalConstraint); ok {
			continue // recreated automatically on load
		}
		// marshalConstraint writes each operand's raw id — points AND entities — so
		// both halves are screened here, through the same constraintRefs the removal
		// cascade and the integrity scan read.
		cpts, cents := constraintRefs(c)
		if err := s.checkNoForeignRefs(cpts, cents, fmt.Sprintf("constraint %q", constraintKind(c))); err != nil {
			return jsonSketchBody{}, err
		}
		jc, ok := marshalConstraint(c)
		if !ok {
			return jsonSketchBody{}, fmt.Errorf("sketch: cannot serialize constraint %T", c)
		}
		jc.Name = s.conNames[c] // optional label lives on the sketch, not the constraint
		body.Constraints = append(body.Constraints, jc)
	}

	body.Units = &jsonSystem{Length: s.sys.Length.Symbol(), Angle: s.sys.Angle.Symbol()}

	if s.params != nil && len(s.params.Names()) > 0 {
		body.Parameters = s.params
	}

	return body, nil
}
