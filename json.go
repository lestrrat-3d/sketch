package sketch

import (
	"encoding/json"
	"fmt"

	"github.com/lestrrat-3d/sketch/param"
	"github.com/lestrrat-3d/sketch/units"
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
	Type         string  `json:"type"` // "line" | "circle" | "arc" | "ellipse" | "spline"
	Points       []int   `json:"points"`
	Radius       float64 `json:"radius,omitempty"`
	Rx           float64 `json:"rx,omitempty"`       // ellipse semi-axis (local x)
	Ry           float64 `json:"ry,omitempty"`       // ellipse semi-axis (local y)
	Rotation     float64 `json:"rotation,omitempty"` // ellipse frame rotation, radians
	Degree       int     `json:"degree,omitempty"`   // spline degree (always 3 today)
	Construction bool    `json:"construction,omitempty"`
	Reference    bool    `json:"reference,omitempty"`
	Source       string  `json:"source,omitempty"`
	Stale        bool    `json:"stale,omitempty"` // reference circle radius freshness only
}

type jsonConstraint struct {
	Type     string  `json:"type"`
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

// jsonVersion is the current document schema version. Version-2 documents carry
// an explicit "kind" ("sketch" or "world") and placement. Legacy documents
// (version absent/0/1, no "kind") still load as world-XY sketches; documents
// from a newer schema are rejected rather than mis-loaded.
const jsonVersion = 2

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

// dimJSON builds the serialized form of a dimensional constraint.
func dimJSON(typ string, d Dimension, points, entities []int) jsonConstraint {
	t := d.Target()
	return jsonConstraint{
		Type: typ, Points: points, Entities: entities,
		Value: t.Mag(), Unit: t.Unit().Symbol(), Expr: d.driverExpr(), Driven: d.Driven(),
	}
}

// dimUnit resolves a stored unit symbol for a dimension of the given kind,
// falling back to the kind's base unit.
func dimUnit(symbol string, kind units.Kind) units.Unit {
	if u, ok := units.Lookup(symbol); ok && u.Kind() == kind {
		return u
	}
	return units.BaseUnit(kind)
}

// restoreDim reinstates a deserialized dimension's unit, parameter binding and
// driven flag.
func restoreDim(d Dimension, jc jsonConstraint) {
	d.restore(jc.Value, dimUnit(jc.Unit, d.Kind()))
	d.setDriverExpr(jc.Expr)
	d.SetDriven(jc.Driven)
}

// MarshalJSON implements [json.Marshaler], producing a portable, reloadable
// standalone sketch document (kind "sketch") with the sketch's plane inlined.
// The plane must be a world-frame datum; a sketch on a derived (world-owned)
// plane must be serialized through its [World] instead.
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
		Kind: kindSketch, Version: jsonVersion,
		jsonSketchBody: body, Plane: jp,
	})
}

// marshalBody builds the shared, placement-free payload of a sketch.
func (s *Sketch) marshalBody() (jsonSketchBody, error) {
	var body jsonSketchBody

	for _, p := range s.points {
		body.Points = append(body.Points, jsonPoint{
			X: p.x(), Y: p.y(), Fixed: p.IsFixed(),
			Name: p.name, Construction: p.construction,
			Reference: p.reference, Source: p.source, Stale: p.stale,
		})
	}

	for _, e := range s.ents {
		switch t := e.(type) {
		case *Line:
			body.Entities = append(body.Entities, jsonEntity{
				Type: "line", Points: []int{t.Start.id, t.End.id}, Construction: t.construction,
				Reference: t.reference, Source: t.source,
			})
		case *Circle:
			body.Entities = append(body.Entities, jsonEntity{
				Type: "circle", Points: []int{t.Center.id}, Radius: t.r(), Construction: t.construction,
				Reference: t.reference, Source: t.source, Stale: t.stale, // circle: radius freshness
			})
		case *Arc:
			body.Entities = append(body.Entities, jsonEntity{
				Type: "arc", Points: []int{t.Center.id, t.Start.id, t.End.id}, Construction: t.construction,
				Reference: t.reference, Source: t.source,
			})
		case *Ellipse:
			body.Entities = append(body.Entities, jsonEntity{
				Type: "ellipse", Points: []int{t.Center.id},
				Rx: t.rx(), Ry: t.ry(), Rotation: t.rot(), Construction: t.construction,
			})
		case *Spline:
			je := jsonEntity{Type: "spline", Degree: 3, Construction: t.construction}
			for _, c := range t.Control {
				je.Points = append(je.Points, c.id)
			}
			body.Entities = append(body.Entities, je)
		}
	}

	for _, c := range s.cons {
		if _, ok := c.(internalConstraint); ok {
			continue // recreated automatically on load
		}
		jc, ok := marshalConstraint(c)
		if !ok {
			return jsonSketchBody{}, fmt.Errorf("sketch: cannot serialize constraint %T", c)
		}
		body.Constraints = append(body.Constraints, jc)
	}

	body.Units = &jsonSystem{Length: s.sys.Length.Symbol(), Angle: s.sys.Angle.Symbol()}

	if s.params != nil && len(s.params.Names()) > 0 {
		body.Parameters = s.params
	}

	return body, nil
}

func marshalConstraint(c Constraint) (jsonConstraint, bool) {
	switch t := c.(type) {
	case *coincident:
		return jsonConstraint{Type: "coincident", Points: []int{t.P1.id, t.P2.id}}, true
	case *horizontal:
		return jsonConstraint{Type: "horizontal", Entities: []int{t.L.id}}, true
	case *vertical:
		return jsonConstraint{Type: "vertical", Entities: []int{t.L.id}}, true
	case *parallel:
		return jsonConstraint{Type: "parallel", Entities: []int{t.L1.id, t.L2.id}}, true
	case *perpendicular:
		return jsonConstraint{Type: "perpendicular", Entities: []int{t.L1.id, t.L2.id}}, true
	case *pointOnLine:
		return jsonConstraint{Type: "point_on_line", Points: []int{t.P.id}, Entities: []int{t.L.id}}, true
	case *collinear:
		return jsonConstraint{Type: "collinear", Entities: []int{t.L1.id, t.L2.id}}, true
	case *concentric:
		return jsonConstraint{Type: "concentric", Entities: []int{t.C1.id, t.C2.id}}, true
	case *pointOnCircle:
		return jsonConstraint{Type: "point_on_circle", Points: []int{t.P.id}, Entities: []int{t.C.id}}, true
	case *pointOnEllipse:
		return jsonConstraint{Type: "point_on_ellipse", Points: []int{t.P.id}, Entities: []int{t.E.id}}, true
	case *midpoint:
		return jsonConstraint{Type: "midpoint", Points: []int{t.P.id}, Entities: []int{t.L.id}}, true
	case *symmetric:
		return jsonConstraint{Type: "symmetric", Points: []int{t.P1.id, t.P2.id}, Entities: []int{t.Axis.id}}, true
	case *equalLines:
		return jsonConstraint{Type: "equal_lines", Entities: []int{t.L1.id, t.L2.id}}, true
	case *equalRadii:
		return jsonConstraint{Type: "equal_radii", Entities: []int{t.C1.entID(), t.C2.entID()}}, true
	case *tangentLineCircle:
		return jsonConstraint{Type: "tangent_line_circle", Entities: []int{t.L.id, t.C.entID()}}, true
	case *tangentCircles:
		return jsonConstraint{Type: "tangent_circles", Entities: []int{t.C1.entID(), t.C2.entID()}, Flag: t.Internal}, true
	case *Distance:
		return dimJSON("distance", t, []int{t.P1.id, t.P2.id}, nil), true
	case *HorizontalDistance:
		return dimJSON("hdistance", t, []int{t.P1.id, t.P2.id}, nil), true
	case *VerticalDistance:
		return dimJSON("vdistance", t, []int{t.P1.id, t.P2.id}, nil), true
	case *DistancePointLine:
		return dimJSON("distance_point_line", t, []int{t.P.id}, []int{t.L.id}), true
	case *DistanceLines:
		return dimJSON("distance_lines", t, nil, []int{t.L1.id, t.L2.id}), true
	case *Offset:
		return dimJSON("offset", t, nil, []int{t.Src.id, t.Dst.id}), true
	case *Radius:
		return dimJSON("radius", t, nil, []int{t.C.id}), true
	case *Diameter:
		return dimJSON("diameter", t, nil, []int{t.C.id}), true
	case *Angle:
		return dimJSON("angle", t, nil, []int{t.L1.id, t.L2.id}), true
	case *SemiMajor:
		return dimJSON("semi_major", t, nil, []int{t.E.id}), true
	case *SemiMinor:
		return dimJSON("semi_minor", t, nil, []int{t.E.id}), true
	case *EllipseRotation:
		return dimJSON("ellipse_rotation", t, nil, []int{t.E.id}), true
	}
	return jsonConstraint{}, false
}

// UnmarshalJSON implements [json.Unmarshaler], rebuilding the sketch in place
// from a standalone sketch document. It rejects a world document and a
// missing-kind document carrying v2-only keys, and requires a plane for a v2
// "sketch" document; a legacy document (no kind, version absent/0/1) loads as a
// world-XY sketch.
func (s *Sketch) UnmarshalJSON(data []byte) error {
	pf, err := preflight(data)
	if err != nil {
		return err
	}
	if pf.version > jsonVersion {
		return fmt.Errorf("sketch: unsupported document version %d (this build reads up to %d)", pf.version, jsonVersion)
	}
	switch pf.kind {
	case kindWorld:
		return fmt.Errorf("%w: got a world document, want a sketch", ErrWrongDocumentKind)
	case kindSketch, "":
		// handled below
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrWrongDocumentKind, pf.kind)
	}
	if pf.version >= 2 && pf.kind == "" {
		return fmt.Errorf("%w: a version %d document requires a \"kind\"", ErrWrongDocumentKind, pf.version)
	}
	if pf.kind == "" && (pf.has("plane") || pf.has("planes") || pf.has("sketches")) {
		return fmt.Errorf("%w: a legacy (kind-less) document must not carry a v2-only key", ErrWrongDocumentKind)
	}

	var doc jsonSketchDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}

	var plane *Plane
	switch pf.kind {
	case kindSketch:
		if doc.Plane == nil {
			return ErrMissingPlane
		}
		plane, err = standalonePlaneFromJSON(*doc.Plane)
		if err != nil {
			return err
		}
	default: // legacy: a 2D sketch is a world-XY sketch
		plane = WorldXY()
	}

	*s = Sketch{sys: units.Metric(), pl: plane}
	return s.buildFromBody(doc.jsonSketchBody)
}

// buildFromBody rebuilds the sketch's geometry, constraints, units and
// parameters from a decoded body. The sketch's vars/points/ents/cons slices
// must already be empty (a fresh *s); placement and version are handled by the
// caller.
func (s *Sketch) buildFromBody(body jsonSketchBody) error {
	if body.Units != nil {
		if lu, ok := units.Lookup(body.Units.Length); ok && lu.Kind() == units.Length {
			s.sys.Length = lu
		}
		if au, ok := units.Lookup(body.Units.Angle); ok && au.Kind() == units.Angle {
			s.sys.Angle = au
		}
	}

	for _, jp := range body.Points {
		p := s.AddPoint(jp.X, jp.Y)
		p.SetName(jp.Name)
		if jp.Reference {
			if jp.Construction {
				return fmt.Errorf("sketch: point cannot be both reference and construction")
			}
			p.reference = true
			p.source = jp.Source
			p.stale = jp.Stale
			s.fixed[p.xi] = true // reference geometry is locked
			s.fixed[p.yi] = true
			continue
		}
		p.SetConstruction(jp.Construction)
		if jp.Fixed {
			s.Fix(p)
		}
	}

	line := func(i int) (*Line, error) {
		l, ok := s.entByID(i).(*Line)
		if !ok {
			return nil, fmt.Errorf("sketch: entity %d is not a line", i)
		}
		return l, nil
	}
	circle := func(i int) (*Circle, error) {
		c, ok := s.entByID(i).(*Circle)
		if !ok {
			return nil, fmt.Errorf("sketch: entity %d is not a circle", i)
		}
		return c, nil
	}
	circular := func(i int) (Circular, error) {
		c, ok := s.entByID(i).(Circular)
		if !ok {
			return nil, fmt.Errorf("sketch: entity %d is not a circle or arc", i)
		}
		return c, nil
	}
	ellipse := func(i int) (*Ellipse, error) {
		e, ok := s.entByID(i).(*Ellipse)
		if !ok {
			return nil, fmt.Errorf("sketch: entity %d is not an ellipse", i)
		}
		return e, nil
	}

	for _, je := range body.Entities {
		ps, err := s.pointsRef(je.Points)
		if err != nil {
			return err
		}
		if je.Reference {
			if je.Construction {
				return fmt.Errorf("sketch: entity cannot be both reference and construction")
			}
			// The reference constructors require reference-locked points and seal
			// the topology, so a corrupt document (reference entity on free points)
			// is rejected here.
			switch je.Type {
			case "line":
				if len(ps) != 2 {
					return fmt.Errorf("sketch: line needs 2 points, got %d", len(ps))
				}
				if je.Stale {
					return fmt.Errorf("sketch: reference line staleness is derived, not stored")
				}
				if _, err := s.AddReferenceLine(ps[0], ps[1], je.Source); err != nil {
					return err
				}
			case "arc":
				if len(ps) != 3 {
					return fmt.Errorf("sketch: arc needs 3 points, got %d", len(ps))
				}
				if je.Stale {
					return fmt.Errorf("sketch: reference arc staleness is derived, not stored")
				}
				if _, err := s.AddReferenceArc(ps[0], ps[1], ps[2], je.Source); err != nil {
					return err
				}
			case "circle":
				if len(ps) != 1 {
					return fmt.Errorf("sketch: circle needs 1 point, got %d", len(ps))
				}
				c, err := s.AddReferenceCircle(ps[0], je.Radius, je.Source)
				if err != nil {
					return err
				}
				c.stale = je.Stale // restore radius staleness
			default:
				return fmt.Errorf("sketch: reference geometry of kind %q is not supported", je.Type)
			}
			continue
		}
		switch je.Type {
		case "line":
			if len(ps) != 2 {
				return fmt.Errorf("sketch: line needs 2 points, got %d", len(ps))
			}
			s.AddLine(ps[0], ps[1]).SetConstruction(je.Construction)
		case "circle":
			if len(ps) != 1 {
				return fmt.Errorf("sketch: circle needs 1 point, got %d", len(ps))
			}
			s.AddCircle(ps[0], je.Radius).SetConstruction(je.Construction)
		case "arc":
			if len(ps) != 3 {
				return fmt.Errorf("sketch: arc needs 3 points, got %d", len(ps))
			}
			s.AddArc(ps[0], ps[1], ps[2]).SetConstruction(je.Construction)
		case "ellipse":
			if len(ps) != 1 {
				return fmt.Errorf("sketch: ellipse needs 1 point, got %d", len(ps))
			}
			s.AddEllipse(ps[0], je.Rx, je.Ry, je.Rotation).SetConstruction(je.Construction)
		case "spline":
			if je.Degree != 0 && je.Degree != 3 {
				return fmt.Errorf("sketch: unsupported spline degree %d", je.Degree)
			}
			sp, err := s.AddSpline(ps...) // AddSpline validates the >= 4 count
			if err != nil {
				return err
			}
			sp.SetConstruction(je.Construction)
		default:
			return fmt.Errorf("sketch: unknown entity type %q", je.Type)
		}
	}

	for _, jc := range body.Constraints {
		if err := s.rebuildConstraint(jc, line, circle, circular, ellipse); err != nil {
			return err
		}
	}

	if body.Parameters != nil {
		s.params = body.Parameters
	}
	return nil
}

func (s *Sketch) entByID(i int) Entity {
	if i < 0 || i >= len(s.ents) {
		return nil
	}
	return s.ents[i]
}

// pointRef returns the point with id i, or an error if i is out of range. The
// v2 decoder validates every reference through this before indexing, so a
// malformed document errors rather than panicking.
func (s *Sketch) pointRef(i int) (*Point, error) {
	if i < 0 || i >= len(s.points) {
		return nil, fmt.Errorf("sketch: point id %d out of range", i)
	}
	return s.points[i], nil
}

// pointsRef resolves a list of point ids, validating each.
func (s *Sketch) pointsRef(ids []int) ([]*Point, error) {
	out := make([]*Point, len(ids))
	for k, i := range ids {
		p, err := s.pointRef(i)
		if err != nil {
			return nil, err
		}
		out[k] = p
	}
	return out, nil
}

// constraintArity is the exact {points, entities} a serialized constraint of
// each type must carry. The decoder validates argument counts against it before
// indexing, so a malformed document (too few or too many refs) errors instead
// of panicking or silently dropping extras. A type missing here simply skips the
// count check (and is caught by the switch's default); the round-trip test
// exercises every kind, so a stale entry surfaces there.
var constraintArity = map[string][2]int{
	"coincident": {2, 0}, "horizontal": {0, 1}, "vertical": {0, 1},
	"parallel": {0, 2}, "perpendicular": {0, 2}, "equal_lines": {0, 2},
	"collinear": {0, 2}, "angle": {0, 2}, "point_on_line": {1, 1},
	"point_on_circle": {1, 1}, "point_on_ellipse": {1, 1}, "midpoint": {1, 1},
	"symmetric": {2, 1}, "concentric": {0, 2}, "equal_radii": {0, 2},
	"tangent_line_circle": {0, 2}, "tangent_circles": {0, 2},
	"distance": {2, 0}, "hdistance": {2, 0}, "vdistance": {2, 0},
	"distance_point_line": {1, 1}, "distance_lines": {0, 2}, "offset": {0, 2},
	"radius": {0, 1}, "diameter": {0, 1},
	"semi_major": {0, 1}, "semi_minor": {0, 1}, "ellipse_rotation": {0, 1},
}

func (s *Sketch) rebuildConstraint(jc jsonConstraint, line func(int) (*Line, error), circle func(int) (*Circle, error), circular func(int) (Circular, error), ellipse func(int) (*Ellipse, error)) error {
	// Validate references before indexing: enough arguments for the type, and
	// every point/entity id in range.
	if a, ok := constraintArity[jc.Type]; ok {
		if len(jc.Points) != a[0] || len(jc.Entities) != a[1] {
			return fmt.Errorf("sketch: constraint %q needs exactly %d point(s) and %d entity(ies), got %d and %d",
				jc.Type, a[0], a[1], len(jc.Points), len(jc.Entities))
		}
	}
	for _, i := range jc.Points {
		if i < 0 || i >= len(s.points) {
			return fmt.Errorf("sketch: constraint %q references point id %d out of range", jc.Type, i)
		}
	}
	for _, i := range jc.Entities {
		if i < 0 || i >= len(s.ents) {
			return fmt.Errorf("sketch: constraint %q references entity id %d out of range", jc.Type, i)
		}
	}

	pt := func(i int) *Point { return s.points[jc.Points[i]] }
	// dim restores a dimensional constraint's unit/binding, then commits it.
	dim := func(d Dimension) {
		restoreDim(d, jc)
		s.AddConstraint(d)
	}
	switch jc.Type {
	case "coincident":
		s.AddConstraint(NewCoincident(pt(0), pt(1)))
	case "horizontal":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		s.AddConstraint(NewHorizontal(l))
	case "vertical":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		s.AddConstraint(NewVertical(l))
	case "parallel", "perpendicular", "equal_lines", "collinear", "angle":
		l1, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		l2, err := line(jc.Entities[1])
		if err != nil {
			return err
		}
		switch jc.Type {
		case "parallel":
			s.AddConstraint(NewParallel(l1, l2))
		case "perpendicular":
			s.AddConstraint(NewPerpendicular(l1, l2))
		case "equal_lines":
			s.AddConstraint(NewEqual(l1, l2))
		case "collinear":
			s.AddConstraint(NewCollinear(l1, l2))
		case "angle":
			dim(NewAngle(l1, l2, jc.Value))
		}
	case "point_on_line":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		s.AddConstraint(NewPointOnLine(pt(0), l))
	case "point_on_circle":
		c, err := circle(jc.Entities[0])
		if err != nil {
			return err
		}
		s.AddConstraint(NewPointOnCircle(pt(0), c))
	case "point_on_ellipse":
		e, err := ellipse(jc.Entities[0])
		if err != nil {
			return err
		}
		s.AddConstraint(NewPointOnEllipse(pt(0), e))
	case "semi_major", "semi_minor", "ellipse_rotation":
		e, err := ellipse(jc.Entities[0])
		if err != nil {
			return err
		}
		switch jc.Type {
		case "semi_major":
			dim(NewSemiMajor(e, jc.Value))
		case "semi_minor":
			dim(NewSemiMinor(e, jc.Value))
		case "ellipse_rotation":
			dim(NewEllipseRotation(e, jc.Value))
		}
	case "midpoint":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		s.AddConstraint(NewMidpoint(pt(0), l))
	case "symmetric":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		s.AddConstraint(NewSymmetric(pt(0), pt(1), l))
	case "concentric":
		c1, err := circle(jc.Entities[0])
		if err != nil {
			return err
		}
		c2, err := circle(jc.Entities[1])
		if err != nil {
			return err
		}
		s.AddConstraint(NewConcentric(c1, c2))
	case "equal_radii":
		c1, err := circular(jc.Entities[0])
		if err != nil {
			return err
		}
		c2, err := circular(jc.Entities[1])
		if err != nil {
			return err
		}
		s.AddConstraint(NewEqualRadius(c1, c2))
	case "tangent_line_circle":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		c, err := circular(jc.Entities[1])
		if err != nil {
			return err
		}
		s.AddConstraint(NewTangent(l, c))
	case "tangent_circles":
		c1, err := circular(jc.Entities[0])
		if err != nil {
			return err
		}
		c2, err := circular(jc.Entities[1])
		if err != nil {
			return err
		}
		s.AddConstraint(NewTangentCircles(c1, c2, jc.Flag))
	case "distance":
		dim(NewDistance(pt(0), pt(1), jc.Value))
	case "distance_point_line":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		dim(NewDistancePointLine(pt(0), l, jc.Value))
	case "distance_lines":
		l1, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		l2, err := line(jc.Entities[1])
		if err != nil {
			return err
		}
		dim(NewDistanceLines(l1, l2, jc.Value))
	case "offset":
		src, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		dst, err := line(jc.Entities[1])
		if err != nil {
			return err
		}
		dim(NewOffset(src, dst, jc.Value))
	case "hdistance":
		dim(NewHorizontalDistance(pt(0), pt(1), jc.Value))
	case "vdistance":
		dim(NewVerticalDistance(pt(0), pt(1), jc.Value))
	case "radius":
		c, err := circle(jc.Entities[0])
		if err != nil {
			return err
		}
		dim(NewRadius(c, jc.Value))
	case "diameter":
		c, err := circle(jc.Entities[0])
		if err != nil {
			return err
		}
		dim(NewDiameter(c, jc.Value))
	default:
		return fmt.Errorf("sketch: unknown constraint type %q", jc.Type)
	}
	return nil
}
