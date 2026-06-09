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
}

type jsonEntity struct {
	Type         string  `json:"type"` // "line" | "circle" | "arc"
	Points       []int   `json:"points"`
	Radius       float64 `json:"radius,omitempty"`
	Construction bool    `json:"construction,omitempty"`
}

type jsonConstraint struct {
	Type     string  `json:"type"`
	Points   []int   `json:"points,omitempty"`
	Entities []int   `json:"entities,omitempty"`
	Value    float64 `json:"value,omitempty"`
	Unit     string  `json:"unit,omitempty"` // dimension's unit symbol
	Expr     string  `json:"expr,omitempty"` // parameter binding on a dimension
	Flag     bool    `json:"flag,omitempty"`
}

type jsonSystem struct {
	Length string `json:"length"`
	Angle  string `json:"angle"`
}

type jsonSketch struct {
	Points      []jsonPoint      `json:"points"`
	Entities    []jsonEntity     `json:"entities"`
	Constraints []jsonConstraint `json:"constraints"`
	Units       *jsonSystem      `json:"units,omitempty"`
	Parameters  *param.Table     `json:"parameters,omitempty"`
}

// dimJSON builds the serialized form of a dimensional constraint.
func dimJSON(typ string, d Dimension, points, entities []int) jsonConstraint {
	t := d.Target()
	return jsonConstraint{
		Type: typ, Points: points, Entities: entities,
		Value: t.Mag(), Unit: t.Unit().Symbol(), Expr: d.driverExpr(),
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

// restoreDim reinstates a deserialized dimension's unit and parameter binding.
func restoreDim(d Dimension, jc jsonConstraint) {
	d.restore(jc.Value, dimUnit(jc.Unit, d.Kind()))
	d.setDriverExpr(jc.Expr)
}

// MarshalJSON implements [json.Marshaler], producing a portable, reloadable
// description of the sketch.
func (s *Sketch) MarshalJSON() ([]byte, error) {
	var js jsonSketch

	for _, p := range s.points {
		js.Points = append(js.Points, jsonPoint{
			X: p.x(), Y: p.y(), Fixed: p.IsFixed(),
			Name: p.Name, Construction: p.Construction,
		})
	}

	for _, e := range s.ents {
		switch t := e.(type) {
		case *Line:
			js.Entities = append(js.Entities, jsonEntity{
				Type: "line", Points: []int{t.A.id, t.B.id}, Construction: t.Construction,
			})
		case *Circle:
			js.Entities = append(js.Entities, jsonEntity{
				Type: "circle", Points: []int{t.Center.id}, Radius: t.r(), Construction: t.Construction,
			})
		case *Arc:
			js.Entities = append(js.Entities, jsonEntity{
				Type: "arc", Points: []int{t.Center.id, t.Start.id, t.End.id}, Construction: t.Construction,
			})
		}
	}

	for _, c := range s.cons {
		if _, ok := c.(internalConstraint); ok {
			continue // recreated automatically on load
		}
		jc, ok := marshalConstraint(c)
		if !ok {
			return nil, fmt.Errorf("sketch: cannot serialize constraint %T", c)
		}
		js.Constraints = append(js.Constraints, jc)
	}

	js.Units = &jsonSystem{Length: s.sys.Length.Symbol(), Angle: s.sys.Angle.Symbol()}

	if s.params != nil && len(s.params.Names()) > 0 {
		js.Parameters = s.params
	}

	return json.Marshal(js)
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
	case *midpoint:
		return jsonConstraint{Type: "midpoint", Points: []int{t.P.id}, Entities: []int{t.L.id}}, true
	case *symmetric:
		return jsonConstraint{Type: "symmetric", Points: []int{t.P1.id, t.P2.id}, Entities: []int{t.Axis.id}}, true
	case *equalLines:
		return jsonConstraint{Type: "equal_lines", Entities: []int{t.L1.id, t.L2.id}}, true
	case *equalRadii:
		return jsonConstraint{Type: "equal_radii", Entities: []int{t.C1.id, t.C2.id}}, true
	case *tangentLineCircle:
		return jsonConstraint{Type: "tangent_line_circle", Entities: []int{t.L.id, t.C.id}}, true
	case *tangentCircles:
		return jsonConstraint{Type: "tangent_circles", Entities: []int{t.C1.id, t.C2.id}, Flag: t.Internal}, true
	case *Distance:
		return dimJSON("distance", t, []int{t.P1.id, t.P2.id}, nil), true
	case *HorizontalDistance:
		return dimJSON("hdistance", t, []int{t.P1.id, t.P2.id}, nil), true
	case *VerticalDistance:
		return dimJSON("vdistance", t, []int{t.P1.id, t.P2.id}, nil), true
	case *Radius:
		return dimJSON("radius", t, nil, []int{t.C.id}), true
	case *Diameter:
		return dimJSON("diameter", t, nil, []int{t.C.id}), true
	case *Angle:
		return dimJSON("angle", t, nil, []int{t.L1.id, t.L2.id}), true
	}
	return jsonConstraint{}, false
}

// UnmarshalJSON implements [json.Unmarshaler], rebuilding the sketch in place.
func (s *Sketch) UnmarshalJSON(data []byte) error {
	var js jsonSketch
	if err := json.Unmarshal(data, &js); err != nil {
		return err
	}

	*s = Sketch{sys: units.Metric()}
	if js.Units != nil {
		if lu, ok := units.Lookup(js.Units.Length); ok && lu.Kind() == units.Length {
			s.sys.Length = lu
		}
		if au, ok := units.Lookup(js.Units.Angle); ok && au.Kind() == units.Angle {
			s.sys.Angle = au
		}
	}

	for _, jp := range js.Points {
		p := s.AddPoint(NewPoint(jp.X, jp.Y))
		p.Name = jp.Name
		p.Construction = jp.Construction
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

	for _, je := range js.Entities {
		switch je.Type {
		case "line":
			if len(je.Points) != 2 {
				return fmt.Errorf("sketch: line needs 2 points, got %d", len(je.Points))
			}
			l := s.AddLine(NewLine(s.points[je.Points[0]], s.points[je.Points[1]]))
			l.Construction = je.Construction
		case "circle":
			if len(je.Points) != 1 {
				return fmt.Errorf("sketch: circle needs 1 point, got %d", len(je.Points))
			}
			c := s.AddCircle(NewCircle(s.points[je.Points[0]], je.Radius))
			c.Construction = je.Construction
		case "arc":
			if len(je.Points) != 3 {
				return fmt.Errorf("sketch: arc needs 3 points, got %d", len(je.Points))
			}
			a := s.AddArc(NewArc(s.points[je.Points[0]], s.points[je.Points[1]], s.points[je.Points[2]]))
			a.Construction = je.Construction
		default:
			return fmt.Errorf("sketch: unknown entity type %q", je.Type)
		}
	}

	for _, jc := range js.Constraints {
		if err := s.rebuildConstraint(jc, line, circle); err != nil {
			return err
		}
	}

	if js.Parameters != nil {
		s.params = js.Parameters
	}
	return nil
}

func (s *Sketch) entByID(i int) Entity {
	if i < 0 || i >= len(s.ents) {
		return nil
	}
	return s.ents[i]
}

func (s *Sketch) rebuildConstraint(jc jsonConstraint, line func(int) (*Line, error), circle func(int) (*Circle, error)) error {
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
		c1, err := circle(jc.Entities[0])
		if err != nil {
			return err
		}
		c2, err := circle(jc.Entities[1])
		if err != nil {
			return err
		}
		s.AddConstraint(NewEqualRadius(c1, c2))
	case "tangent_line_circle":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		c, err := circle(jc.Entities[1])
		if err != nil {
			return err
		}
		s.AddConstraint(NewTangent(l, c))
	case "tangent_circles":
		c1, err := circle(jc.Entities[0])
		if err != nil {
			return err
		}
		c2, err := circle(jc.Entities[1])
		if err != nil {
			return err
		}
		s.AddConstraint(NewTangentCircles(c1, c2, jc.Flag))
	case "distance":
		dim(NewDistance(pt(0), pt(1), jc.Value))
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
