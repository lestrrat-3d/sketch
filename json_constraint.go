package sketch

// constraintKind is the single source of truth for a constraint's stable,
// serialized type string. Both marshalConstraint (which adds the operand refs)
// and the public ConstraintKind read it, so the two can never drift — and any
// typo here breaks the round-trip-every-kind serialization test, not just
// introspection. It is deliberately side-effect-free: it switches on the
// constraint's own type (and, for tangentConics, its operand's type) and never
// dereferences operand coordinates, so it is safe on any constructed
// constraint, including one built with nil operands. Internal constraints
// (never serialized) report their own kinds; an unknown type reports "".
func constraintKind(c Constraint) string {
	switch t := c.(type) {
	case *coincident:
		return "coincident"
	case *horizontal:
		return "horizontal"
	case *vertical:
		return "vertical"
	case *horizontalPoints:
		return "horizontal_points"
	case *verticalPoints:
		return "vertical_points"
	case *parallel:
		return "parallel"
	case *perpendicular:
		return "perpendicular"
	case *pointOnLine:
		return "point_on_line"
	case *pointOnNormal:
		return "point_on_normal"
	case *collinear:
		return "collinear"
	case *concentric:
		return "concentric"
	case *pointOnCircle:
		return "point_on_circle"
	case *pointOnArc:
		return "point_on_arc"
	case *pointOnEllipticalArc:
		return "point_on_elliptical_arc"
	case *pointOnEllipse:
		return "point_on_ellipse"
	case *pointOnSpline:
		return "point_on_spline"
	case *pointOnClosedSpline:
		return "point_on_closed_spline"
	case *pointOnFitSpline:
		return "point_on_fit_spline"
	case *tangentToSpline:
		return "tangent_spline"
	case *tangentToClosedSpline:
		return "tangent_closed_spline"
	case *tangentToFitSpline:
		return "tangent_fit_spline"
	case *pointOnConic:
		return "point_on_conic"
	case *tangentToConic:
		return "tangent_conic"
	case *pointOnNURBS:
		return "point_on_nurbs"
	case *tangentToNURBS:
		return "tangent_nurbs"
	case *tangentConics:
		if t != nil {
			switch t.B.(type) {
			case circleConic, arcConic: // B is a circular operand (circle or arc)
				return "tangent_ellipse_circle"
			}
		}
		return "tangent_ellipses"
	case *midpoint:
		return "midpoint"
	case *midpointOf:
		return "midpoint_of"
	case *symmetric:
		return "symmetric"
	case *symmetricLines:
		return "symmetric_lines"
	case *symmetricCircles:
		return "symmetric_circles"
	case *symmetricArcs:
		return "symmetric_arcs"
	case *equalLines:
		return "equal_lines"
	case *equalRadii:
		return "equal_radii"
	case *equalLineArc:
		return "equal_line_arc"
	case *tangentLineCircle:
		return "tangent_line_circle"
	case *tangentCircles:
		return "tangent_circles"
	case *tangentLineEllipse:
		return "tangent_ellipse"
	case *Distance:
		return "distance"
	case *HorizontalDistance:
		return "hdistance"
	case *VerticalDistance:
		return "vdistance"
	case *DistancePointLine:
		return "distance_point_line"
	case *DistancePointCircle:
		return "distance_point_circle"
	case *DistanceLineCircle:
		return "distance_line_circle"
	case *DistancePointArc:
		return "distance_point_arc"
	case *DistanceLineArc:
		return "distance_line_arc"
	case *DistanceLines:
		return "distance_lines"
	case *Offset:
		return "offset"
	case *RadialOffset:
		return "radial_offset"
	case *Radius:
		return "radius"
	case *Diameter:
		return "diameter"
	case *ArcLength:
		return "arc_length"
	case *Angle:
		return "angle"
	case *SemiMajor:
		return "semi_major"
	case *SemiMinor:
		return "semi_minor"
	case *EllipseRotation:
		return "ellipse_rotation"
	// Internal (auto-added) constraints are never serialized, so they have no
	// marshalConstraint case; they still report a stable kind for introspection.
	case *arcRadius:
		return "arc_radius"
	case *ellipticalArcOn:
		return "elliptical_arc_on"
	}
	return ""
}

func marshalConstraint(c Constraint) (jsonConstraint, bool) {
	typ := constraintKind(c)
	switch t := c.(type) {
	case *coincident:
		return jsonConstraint{Type: typ, Points: []int{t.P1.id, t.P2.id}}, true
	case *horizontal:
		return jsonConstraint{Type: typ, Entities: []int{t.L.id}}, true
	case *vertical:
		return jsonConstraint{Type: typ, Entities: []int{t.L.id}}, true
	case *horizontalPoints:
		return jsonConstraint{Type: typ, Points: []int{t.P1.id, t.P2.id}}, true
	case *verticalPoints:
		return jsonConstraint{Type: typ, Points: []int{t.P1.id, t.P2.id}}, true
	case *parallel:
		return jsonConstraint{Type: typ, Entities: []int{t.L1.id, t.L2.id}}, true
	case *perpendicular:
		return jsonConstraint{Type: typ, Entities: []int{t.L1.id, t.L2.id}}, true
	case *pointOnLine:
		return jsonConstraint{Type: typ, Points: []int{t.P.id}, Entities: []int{t.L.id}}, true
	case *pointOnNormal:
		return jsonConstraint{Type: typ, Points: []int{t.P.id, t.At.id}, Entities: []int{t.C.entID()}}, true
	case *collinear:
		return jsonConstraint{Type: typ, Entities: []int{t.L1.id, t.L2.id}}, true
	case *concentric:
		return jsonConstraint{Type: typ, Entities: []int{t.C1.entID(), t.C2.entID()}}, true
	case *pointOnCircle:
		return jsonConstraint{Type: typ, Points: []int{t.P.id}, Entities: []int{t.C.id}}, true
	case *pointOnArc:
		return jsonConstraint{Type: typ, Points: []int{t.P.id}, Entities: []int{t.A.id}}, true
	case *pointOnEllipticalArc:
		return jsonConstraint{Type: typ, Points: []int{t.P.id}, Entities: []int{t.A.id}}, true
	case *pointOnEllipse:
		return jsonConstraint{Type: typ, Points: []int{t.P.id}, Entities: []int{t.E.id}}, true
	case *pointOnSpline:
		return jsonConstraint{Type: typ, Points: []int{t.P.id}, Entities: []int{t.Sp.id}}, true
	case *pointOnClosedSpline:
		return jsonConstraint{Type: typ, Points: []int{t.P.id}, Entities: []int{t.Sp.id}}, true
	case *pointOnFitSpline:
		return jsonConstraint{Type: typ, Points: []int{t.P.id}, Entities: []int{t.Sp.id}}, true
	case *tangentToSpline:
		return jsonConstraint{Type: typ, Entities: []int{t.L.id, t.Sp.id}}, true
	case *tangentToClosedSpline:
		return jsonConstraint{Type: typ, Entities: []int{t.L.id, t.Sp.id}}, true
	case *tangentToFitSpline:
		return jsonConstraint{Type: typ, Entities: []int{t.L.id, t.Sp.id}}, true
	case *pointOnConic:
		return jsonConstraint{Type: typ, Points: []int{t.P.id}, Entities: []int{t.C.id}}, true
	case *tangentToConic:
		return jsonConstraint{Type: typ, Entities: []int{t.L.id, t.C.id}}, true
	case *pointOnNURBS:
		return jsonConstraint{Type: typ, Points: []int{t.P.id}, Entities: []int{t.C.id}}, true
	case *tangentToNURBS:
		return jsonConstraint{Type: typ, Entities: []int{t.L.id, t.C.id}}, true
	case *tangentConics:
		return jsonConstraint{Type: typ, Entities: []int{t.A.ent().entID(), t.B.ent().entID()}, Flag: t.Internal}, true
	case *midpoint:
		return jsonConstraint{Type: typ, Points: []int{t.P.id}, Entities: []int{t.L.id}}, true
	case *midpointOf:
		return jsonConstraint{Type: typ, Points: []int{t.Mid.id, t.P1.id, t.P2.id}}, true
	case *symmetric:
		return jsonConstraint{Type: typ, Points: []int{t.P1.id, t.P2.id}, Entities: []int{t.Axis.id}}, true
	case *symmetricLines:
		return jsonConstraint{Type: typ, Entities: []int{t.L1.id, t.L2.id, t.Axis.id}}, true
	case *symmetricCircles:
		return jsonConstraint{Type: typ, Entities: []int{t.C1.id, t.C2.id, t.Axis.id}}, true
	case *symmetricArcs:
		return jsonConstraint{Type: typ, Entities: []int{t.A1.id, t.A2.id, t.Axis.id}}, true
	case *equalLines:
		return jsonConstraint{Type: typ, Entities: []int{t.L1.id, t.L2.id}}, true
	case *equalRadii:
		return jsonConstraint{Type: typ, Entities: []int{t.C1.entID(), t.C2.entID()}}, true
	case *equalLineArc:
		return jsonConstraint{Type: typ, Entities: []int{t.L.id, t.A.id}}, true
	case *tangentLineCircle:
		return jsonConstraint{Type: typ, Entities: []int{t.L.id, t.C.entID()}}, true
	case *tangentCircles:
		return jsonConstraint{Type: typ, Entities: []int{t.C1.entID(), t.C2.entID()}, Flag: t.Internal}, true
	case *tangentLineEllipse:
		return jsonConstraint{Type: typ, Entities: []int{t.L.id, t.E.entID()}}, true
	case *Distance:
		return dimJSON(typ, t, []int{t.P1.id, t.P2.id}, nil), true
	case *HorizontalDistance:
		return dimJSON(typ, t, []int{t.P1.id, t.P2.id}, nil), true
	case *VerticalDistance:
		return dimJSON(typ, t, []int{t.P1.id, t.P2.id}, nil), true
	case *DistancePointLine:
		return dimJSON(typ, t, []int{t.P.id}, []int{t.L.id}), true
	case *DistancePointCircle:
		return dimJSON(typ, t, []int{t.P.id}, []int{t.C.id}), true
	case *DistanceLineCircle:
		return dimJSON(typ, t, nil, []int{t.L.id, t.C.id}), true
	case *DistancePointArc:
		return dimJSON(typ, t, []int{t.P.id}, []int{t.A.id}), true
	case *DistanceLineArc:
		return dimJSON(typ, t, nil, []int{t.L.id, t.A.id}), true
	case *DistanceLines:
		return dimJSON(typ, t, nil, []int{t.L1.id, t.L2.id}), true
	case *Offset:
		return dimJSON(typ, t, nil, []int{t.Src.id, t.Dst.id}), true
	case *RadialOffset:
		return dimJSON(typ, t, nil, []int{t.Src.entID(), t.Dst.entID()}), true
	case *Radius:
		return dimJSON(typ, t, nil, []int{t.C.entID()}), true
	case *Diameter:
		return dimJSON(typ, t, nil, []int{t.C.entID()}), true
	case *ArcLength:
		return dimJSON(typ, t, nil, []int{t.A.id}), true
	case *Angle:
		return dimJSON(typ, t, nil, []int{t.L1.id, t.L2.id}), true
	case *SemiMajor:
		return dimJSON(typ, t, nil, []int{t.E.entID()}), true
	case *SemiMinor:
		return dimJSON(typ, t, nil, []int{t.E.entID()}), true
	case *EllipseRotation:
		return dimJSON(typ, t, nil, []int{t.E.entID()}), true
	}
	return jsonConstraint{}, false
}
