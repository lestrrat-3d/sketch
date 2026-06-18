package sketch

// Removal support. Sketches are no longer append-only: constraints, entities
// and (unused) points can be removed. Ids equal current slice position, so
// removal splices and renumbers; solver variables owned by removed geometry
// are retired (marked fixed and abandoned) rather than reclaimed — see
// docs/removal-design.md. A removed handle is dead: using it afterward is
// undefined.

// RemoveConstraint removes a previously added constraint and reports whether
// it was present. Internal (auto-added) constraints can be removed too; they
// are only ever recreated by their entity's Add… constructor.
func (s *Sketch) RemoveConstraint(c Constraint) bool {
	for i, cc := range s.cons {
		if cc == c {
			s.cons = append(s.cons[:i], s.cons[i+1:]...)
			// Retire aux vars only once the last occurrence is gone: a handle
			// committed more than once shares a single set of aux variables.
			if !containsConstraint(s.cons, c) {
				s.retireConstraintVars(c)
			}
			return true
		}
	}
	return false
}

// retireConstraintVars grounds any auxiliary variables a constraint owns (e.g.
// an arc tangency's sweep slack) when its last occurrence is removed, mirroring
// how entity-owned variables are retired.
func (s *Sketch) retireConstraintVars(c Constraint) {
	if r, ok := c.(interface{ retireVars(*Sketch) }); ok {
		r.retireVars(s)
	}
}

func containsConstraint(cs []Constraint, c Constraint) bool {
	for _, cc := range cs {
		if cc == c {
			return true
		}
	}
	return false
}

// RemoveEntity removes an entity and every constraint referencing it
// (including auto-added internal ones), and reports whether the entity was
// present. The entity's own scalar variables (circle radius, ellipse
// axes/rotation) are retired. Its points are NOT removed — points are
// first-class and may be shared; remove orphans explicitly with
// [Sketch.RemovePoint]. Re-adding the same generic geometry afterwards
// creates a fresh, independent instance.
func (s *Sketch) RemoveEntity(e Entity) bool {
	idx := -1
	for i, ee := range s.ents {
		if ee == e {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false
	}
	s.removeConstraintsReferencing(nil, e)
	delete(s.refSeals, e) // drop any reference topology seal
	// Retire scalar variables owned by the entity itself. Line/Arc/Spline own
	// none — their coordinates belong to their points, which survive.
	switch t := e.(type) {
	case *Circle:
		s.retireVar(t.ri)
	case *Ellipse:
		s.retireVar(t.rxi)
		s.retireVar(t.ryi)
		s.retireVar(t.roti)
	case *EllipticalArc:
		s.retireVar(t.rxi)
		s.retireVar(t.ryi)
		s.retireVar(t.roti)
	}
	s.ents = append(s.ents[:idx], s.ents[idx+1:]...)
	for i := idx; i < len(s.ents); i++ {
		renumberEntity(s.ents[i], i)
	}
	return true
}

// RemovePoint removes a point that no entity uses, along with every
// constraint referencing it, and reports success. It returns false — and
// changes nothing — while any entity still uses the point as an endpoint,
// center or spline control point: removing load-bearing geometry implicitly
// would corrupt the entity. Remove the entity first.
func (s *Sketch) RemovePoint(p *Point) bool {
	idx := -1
	for i, pp := range s.points {
		if pp == p {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false
	}
	for _, e := range s.ents {
		if entityUsesPoint(e, p) {
			return false
		}
	}
	s.removeConstraintsReferencing(p, nil)
	s.retireVar(p.xi)
	s.retireVar(p.yi)
	s.points = append(s.points[:idx], s.points[idx+1:]...)
	for i := idx; i < len(s.points); i++ {
		s.points[i].id = i
	}
	return true
}

// retireVar grounds a variable slot owned by removed geometry. Retired slots
// are invisible to the solver and DOF analysis (only free variables count)
// and are never reclaimed; reloading from JSON naturally compacts them away.
func (s *Sketch) retireVar(i int) { s.fixed[i] = true }

// removeConstraintsReferencing drops every constraint whose references
// include the given point or entity (either may be nil).
func (s *Sketch) removeConstraintsReferencing(p *Point, e Entity) {
	kept := s.cons[:0]
	var removed []Constraint
	for _, c := range s.cons {
		pts, ents := constraintRefs(c)
		hit := false
		if p != nil {
			for _, rp := range pts {
				if rp == p {
					hit = true
					break
				}
			}
		}
		if !hit && e != nil {
			for _, re := range ents {
				if re == e {
					hit = true
					break
				}
			}
		}
		if !hit {
			kept = append(kept, c)
			continue
		}
		removed = append(removed, c)
	}
	s.cons = kept
	// Retire aux vars of fully-removed constraints (a duplicate handle whose
	// other occurrence survives in kept keeps its aux vars).
	for _, c := range removed {
		if !containsConstraint(s.cons, c) {
			s.retireConstraintVars(c)
		}
	}
}

// renumberEntity rewrites an entity's id after a splice.
func renumberEntity(e Entity, id int) {
	switch t := e.(type) {
	case *Line:
		t.id = id
	case *Circle:
		t.id = id
	case *Arc:
		t.id = id
	case *Ellipse:
		t.id = id
	case *EllipticalArc:
		t.id = id
	case *Spline:
		t.id = id
	}
}

// entityUsesPoint reports whether the entity references p as an endpoint,
// center or control point. The spline check scans Control — the same point
// may legally appear more than once.
func entityUsesPoint(e Entity, p *Point) bool {
	switch t := e.(type) {
	case *Line:
		return t.Start == p || t.End == p
	case *Circle:
		return t.Center == p
	case *Arc:
		return t.Center == p || t.Start == p || t.End == p
	case *Ellipse:
		return t.Center == p
	case *EllipticalArc:
		return t.Center == p || t.Start == p || t.End == p
	case *Spline:
		for _, c := range t.Control {
			if c == p {
				return true
			}
		}
	}
	return false
}

// constraintRefs enumerates the points and entities a constraint references,
// by concrete pointer identity. It is consulted by the removal cascade and
// mirrors the per-type switches in json.go — a new constraint type MUST add a
// case here (see the checklist in CLAUDE.md). The internal *arcRadius case is
// load-bearing: it is what removes the radius-consistency residual together
// with its arc.
func constraintRefs(c Constraint) ([]*Point, []Entity) {
	switch t := c.(type) {
	case *coincident:
		return []*Point{t.P1, t.P2}, nil
	case *horizontal:
		return nil, []Entity{t.L}
	case *vertical:
		return nil, []Entity{t.L}
	case *horizontalPoints:
		return []*Point{t.P1, t.P2}, nil
	case *verticalPoints:
		return []*Point{t.P1, t.P2}, nil
	case *parallel:
		return nil, []Entity{t.L1, t.L2}
	case *perpendicular:
		return nil, []Entity{t.L1, t.L2}
	case *pointOnLine:
		return []*Point{t.P}, []Entity{t.L}
	case *collinear:
		return nil, []Entity{t.L1, t.L2}
	case *concentric:
		return nil, []Entity{t.C1, t.C2}
	case *pointOnCircle:
		return []*Point{t.P}, []Entity{t.C}
	case *pointOnArc:
		return []*Point{t.P}, []Entity{t.A}
	case *pointOnEllipticalArc:
		return []*Point{t.P}, []Entity{t.A}
	case *pointOnEllipse:
		return []*Point{t.P}, []Entity{t.E}
	case *pointOnSpline:
		return []*Point{t.P}, []Entity{t.Sp}
	case *tangentToSpline:
		return nil, []Entity{t.L, t.Sp}
	case *tangentConics:
		return nil, []Entity{t.A.ent(), t.B.ent()}
	case *midpoint:
		return []*Point{t.P}, []Entity{t.L}
	case *midpointOf:
		return []*Point{t.Mid, t.P1, t.P2}, nil
	case *symmetric:
		return []*Point{t.P1, t.P2}, []Entity{t.Axis}
	case *symmetricLines:
		return nil, []Entity{t.L1, t.L2, t.Axis}
	case *symmetricCircles:
		return nil, []Entity{t.C1, t.C2, t.Axis}
	case *equalLines:
		return nil, []Entity{t.L1, t.L2}
	case *equalRadii:
		return nil, []Entity{t.C1, t.C2}
	case *tangentLineCircle:
		// The cached endpoint-tangency contact (shared) is a point this
		// constraint reads, so it must be enumerated for the removal cascade and
		// the Verify reachability check.
		if t.shared != nil {
			return []*Point{t.shared}, []Entity{t.L, t.C}
		}
		return nil, []Entity{t.L, t.C}
	case *tangentLineEllipse:
		if t.shared != nil {
			return []*Point{t.shared}, []Entity{t.L, t.E}
		}
		return nil, []Entity{t.L, t.E}
	case *tangentCircles:
		if t.shared != nil {
			return []*Point{t.shared}, []Entity{t.C1, t.C2}
		}
		return nil, []Entity{t.C1, t.C2}
	case *arcRadius:
		return nil, []Entity{t.a}
	case *ellipticalArcOn:
		return []*Point{t.p}, []Entity{t.ea}
	case *Distance:
		return []*Point{t.P1, t.P2}, nil
	case *HorizontalDistance:
		return []*Point{t.P1, t.P2}, nil
	case *VerticalDistance:
		return []*Point{t.P1, t.P2}, nil
	case *DistancePointLine:
		return []*Point{t.P}, []Entity{t.L}
	case *DistanceLines:
		return nil, []Entity{t.L1, t.L2}
	case *Offset:
		return nil, []Entity{t.Src, t.Dst}
	case *Radius:
		return nil, []Entity{t.C}
	case *Diameter:
		return nil, []Entity{t.C}
	case *ArcLength:
		return nil, []Entity{t.A}
	case *Angle:
		return nil, []Entity{t.L1, t.L2}
	case *SemiMajor:
		return nil, []Entity{t.E}
	case *SemiMinor:
		return nil, []Entity{t.E}
	case *EllipseRotation:
		return nil, []Entity{t.E}
	}
	return nil, nil
}
