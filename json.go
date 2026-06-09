package sketch

import (
	"encoding/json"
	"fmt"

	"github.com/lestrrat-3d/sketch/param"
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
	Expr     string  `json:"expr,omitempty"` // parameter binding on a dimension
	Flag     bool    `json:"flag,omitempty"`
}

type jsonSketch struct {
	Points      []jsonPoint      `json:"points"`
	Entities    []jsonEntity     `json:"entities"`
	Constraints []jsonConstraint `json:"constraints"`
	Parameters  *param.Table     `json:"parameters,omitempty"`
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
		return jsonConstraint{Type: "distance", Points: []int{t.P1.id, t.P2.id}, Value: t.Value, Expr: t.Expr}, true
	case *HorizontalDistance:
		return jsonConstraint{Type: "hdistance", Points: []int{t.P1.id, t.P2.id}, Value: t.Value, Expr: t.Expr}, true
	case *VerticalDistance:
		return jsonConstraint{Type: "vdistance", Points: []int{t.P1.id, t.P2.id}, Value: t.Value, Expr: t.Expr}, true
	case *Radius:
		return jsonConstraint{Type: "radius", Entities: []int{t.C.id}, Value: t.Value, Expr: t.Expr}, true
	case *Diameter:
		return jsonConstraint{Type: "diameter", Entities: []int{t.C.id}, Value: t.Value, Expr: t.Expr}, true
	case *Angle:
		return jsonConstraint{Type: "angle", Entities: []int{t.L1.id, t.L2.id}, Value: t.Value, Expr: t.Expr}, true
	}
	return jsonConstraint{}, false
}

// UnmarshalJSON implements [json.Unmarshaler], rebuilding the sketch in place.
func (s *Sketch) UnmarshalJSON(data []byte) error {
	var js jsonSketch
	if err := json.Unmarshal(data, &js); err != nil {
		return err
	}

	*s = Sketch{}

	for _, jp := range js.Points {
		p := s.AddPoint(jp.X, jp.Y)
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
			l := s.AddLine(s.points[je.Points[0]], s.points[je.Points[1]])
			l.Construction = je.Construction
		case "circle":
			if len(je.Points) != 1 {
				return fmt.Errorf("sketch: circle needs 1 point, got %d", len(je.Points))
			}
			c := s.AddCircle(s.points[je.Points[0]], je.Radius)
			c.Construction = je.Construction
		case "arc":
			if len(je.Points) != 3 {
				return fmt.Errorf("sketch: arc needs 3 points, got %d", len(je.Points))
			}
			a := s.AddArc(s.points[je.Points[0]], s.points[je.Points[1]], s.points[je.Points[2]])
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
	switch jc.Type {
	case "coincident":
		s.Coincident(pt(0), pt(1))
	case "horizontal":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		s.Horizontal(l)
	case "vertical":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		s.Vertical(l)
	case "parallel", "perpendicular", "equal_lines", "angle":
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
			s.Parallel(l1, l2)
		case "perpendicular":
			s.Perpendicular(l1, l2)
		case "equal_lines":
			s.Equal(l1, l2)
		case "angle":
			s.Angle(l1, l2, jc.Value).setDriverExpr(jc.Expr)
		}
	case "point_on_line":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		s.PointOnLine(pt(0), l)
	case "point_on_circle":
		c, err := circle(jc.Entities[0])
		if err != nil {
			return err
		}
		s.PointOnCircle(pt(0), c)
	case "midpoint":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		s.Midpoint(pt(0), l)
	case "symmetric":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		s.Symmetric(pt(0), pt(1), l)
	case "equal_radii":
		c1, err := circle(jc.Entities[0])
		if err != nil {
			return err
		}
		c2, err := circle(jc.Entities[1])
		if err != nil {
			return err
		}
		s.EqualRadius(c1, c2)
	case "tangent_line_circle":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		c, err := circle(jc.Entities[1])
		if err != nil {
			return err
		}
		s.Tangent(l, c)
	case "tangent_circles":
		c1, err := circle(jc.Entities[0])
		if err != nil {
			return err
		}
		c2, err := circle(jc.Entities[1])
		if err != nil {
			return err
		}
		s.TangentCircles(c1, c2, jc.Flag)
	case "distance":
		s.Distance(pt(0), pt(1), jc.Value).setDriverExpr(jc.Expr)
	case "hdistance":
		s.HorizontalDistance(pt(0), pt(1), jc.Value).setDriverExpr(jc.Expr)
	case "vdistance":
		s.VerticalDistance(pt(0), pt(1), jc.Value).setDriverExpr(jc.Expr)
	case "radius":
		c, err := circle(jc.Entities[0])
		if err != nil {
			return err
		}
		s.Radius(c, jc.Value).setDriverExpr(jc.Expr)
	case "diameter":
		c, err := circle(jc.Entities[0])
		if err != nil {
			return err
		}
		s.Diameter(c, jc.Value).setDriverExpr(jc.Expr)
	default:
		return fmt.Errorf("sketch: unknown constraint type %q", jc.Type)
	}
	return nil
}
