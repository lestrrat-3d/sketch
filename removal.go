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
				delete(s.conNames, c) // purge the optional label with the last occurrence
			}
			return true
		}
	}
	return false
}

// retireConstraintVars grounds any auxiliary variables a constraint owns (e.g.
// an arc tangency's sweep slack) when its last occurrence is removed, mirroring
// how entity-owned variables are retired.
//
// It is screened on the ALLOCATING sketch: the variables are retired only when
// this sketch is the one that allocated them, read through the allocatedBy
// accessor the auxOwner field carries (allocVars records the sketch before it
// allocates anything). Retiring on any other sketch's behalf would ground an
// unrelated variable of THIS sketch — or run off its vector — while resetting
// the owner's indices, silently dropping a row from a constraint that sketch
// still holds. A constraint no sketch has parameterized reads nil and is
// likewise skipped, its indices being unallocated already.
//
// That is a DIFFERENT question from the one the other two doors ask.
// [Sketch.AddConstraint] and [Sketch.CheckConstraint] screen on reference
// ownership (foreignConstraint), which is right THERE: they decide whether to
// parameterize at all, and the constraint has no allocating sketch yet. Here the
// allocation has already happened, and reference ownership does not answer who
// performed it. The two disagree in both directions for a constraint whose
// exported operand field was rewired after it was committed: rewired to another
// sketch's handle (or to a dead one of this sketch, which owns reports the same
// way), a constraint THIS sketch parameterized reads foreign and would leak its
// variable forever — a phantom free DOF this sketch's own report cannot see.
// Removal reaches that state through [Sketch.RemoveConstraint] and through the
// [Sketch.RemoveEntity]/[Sketch.RemovePoint] cascade alike, since the cascade
// matches on one operand while another may be the rewired one.
func (s *Sketch) retireConstraintVars(c Constraint) {
	r, ok := c.(interface{ retireVars(*Sketch) })
	if !ok {
		return // owns no auxiliary variables
	}
	a, ok := c.(interface{ allocatedBy() *Sketch })
	if !ok || a.allocatedBy() != s {
		return
	}
	r.retireVars(s)
	// The variables are back, so the ownership they recorded ends with them.
	// Keeping the pointer would leave the constraint reading as allocated by this
	// sketch, and the two parameterizing doors refuse such a candidate — so a
	// remove-here / add-there sequence, which is a legitimate way to move a
	// constraint between sketches, would be refused for an allocation that no
	// longer exists.
	clearAuxOwnerOf(c)
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
// present. The entity's own scalar variables are retired: whichever ones
// entityShapeVars reports for its type, a circle's radius being one example.
// Its points are NOT removed — points are
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
	s.dropLoopsNaming(e)
	delete(s.refSeals, e) // drop any reference topology seal
	// Drop the entity's instance identity. The uid counter is NOT rewound, so a
	// re-created identical entity lands at the same position and id but gets a
	// FRESH uid — which is what makes the revision (and any Profile holding the
	// dead handle) change. See Sketch.addEntity.
	delete(s.entUIDs, e)
	// Retire scalar variables owned by the entity itself, read from the one
	// definition of what those are (entityShapeVars). A line, an arc and the
	// spline families own none — their coordinates belong to their points, which
	// survive.
	for _, v := range entityShapeVars(e) {
		s.retireVar(v.index)
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
		if s.entityUsesPoint(e, p) {
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
			delete(s.conNames, c) // purge the optional label with the last occurrence
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
	case *ClosedSpline:
		t.id = id
	case *FitSpline:
		t.id = id
	case *Conic:
		t.id = id
	case *NURBS:
		t.id = id
	}
}

// entityUsesPoint reports whether the entity references p as an endpoint,
// center or control point. It is a membership check over the entity's defining
// points — the single source of truth is [entityPoints].
func (s *Sketch) entityUsesPoint(e Entity, p *Point) bool {
	for _, q := range entityPoints(e) {
		if q == p {
			return true
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
	return constraintRefsInto(nil, nil, c)
}

// constraintRefsInto appends a constraint's point and entity operands to the
// provided buffers. The same switch serves the removal cascade and callers
// that can keep the reference lists in stack storage.
func constraintRefsInto(points []*Point, entities []Entity, c Constraint) ([]*Point, []Entity) {
	switch t := c.(type) {
	case *coincident:
		return append(points, t.P1, t.P2), entities
	case *horizontal:
		return points, append(entities, t.L)
	case *vertical:
		return points, append(entities, t.L)
	case *horizontalPoints:
		return append(points, t.P1, t.P2), entities
	case *verticalPoints:
		return append(points, t.P1, t.P2), entities
	case *parallel:
		return points, append(entities, t.L1, t.L2)
	case *perpendicular:
		return points, append(entities, t.L1, t.L2)
	case *pointOnLine:
		return append(points, t.P), append(entities, t.L)
	case *pointOnNormal:
		return append(points, t.P, t.At), append(entities, t.C)
	case *collinear:
		return points, append(entities, t.L1, t.L2)
	case *concentric:
		return points, append(entities, t.C1, t.C2)
	case *pointOnCircle:
		return append(points, t.P), append(entities, t.C)
	case *pointOnArc:
		return append(points, t.P), append(entities, t.A)
	case *pointOnEllipticalArc:
		return append(points, t.P), append(entities, t.A)
	case *pointOnEllipse:
		return append(points, t.P), append(entities, t.E)
	case *pointOnSpline:
		return append(points, t.P), append(entities, t.Sp)
	case *pointOnClosedSpline:
		return append(points, t.P), append(entities, t.Sp)
	case *pointOnFitSpline:
		return append(points, t.P), append(entities, t.Sp)
	case *tangentToSpline:
		return points, append(entities, t.L, t.Sp)
	case *tangentToClosedSpline:
		return points, append(entities, t.L, t.Sp)
	case *tangentToFitSpline:
		return points, append(entities, t.L, t.Sp)
	case *pointOnConic:
		return append(points, t.P), append(entities, t.C)
	case *tangentToConic:
		return points, append(entities, t.L, t.C)
	case *pointOnNURBS:
		return append(points, t.P), append(entities, t.C)
	case *tangentToNURBS:
		return points, append(entities, t.L, t.C)
	case *tangentConics:
		// The cached shared endpoint (shared) is a point this constraint reads, so
		// it must be enumerated for the removal cascade and reachability check.
		if t.shared != nil {
			return append(points, t.shared), append(entities, t.A.ent(), t.B.ent())
		}
		return points, append(entities, t.A.ent(), t.B.ent())
	case *midpoint:
		return append(points, t.P), append(entities, t.L)
	case *midpointOf:
		return append(points, t.Mid, t.P1, t.P2), entities
	case *symmetric:
		return append(points, t.P1, t.P2), append(entities, t.Axis)
	case *symmetricLines:
		return points, append(entities, t.L1, t.L2, t.Axis)
	case *symmetricCircles:
		return points, append(entities, t.C1, t.C2, t.Axis)
	case *symmetricArcs:
		return points, append(entities, t.A1, t.A2, t.Axis)
	case *equalLines:
		return points, append(entities, t.L1, t.L2)
	case *equalRadii:
		return points, append(entities, t.C1, t.C2)
	case *equalLineArc:
		return points, append(entities, t.L, t.A)
	case *tangentLineCircle:
		// The cached endpoint-tangency contact (shared) is a point this
		// constraint reads, so it must be enumerated for the removal cascade and
		// the Verify reachability check.
		if t.shared != nil {
			return append(points, t.shared), append(entities, t.L, t.C)
		}
		return points, append(entities, t.L, t.C)
	case *tangentLineEllipse:
		if t.shared != nil {
			return append(points, t.shared), append(entities, t.L, t.E)
		}
		return points, append(entities, t.L, t.E)
	case *tangentCircles:
		if t.shared != nil {
			return append(points, t.shared), append(entities, t.C1, t.C2)
		}
		return points, append(entities, t.C1, t.C2)
	case *arcRadius:
		return points, append(entities, t.a)
	case *ellipticalArcOn:
		return append(points, t.p), append(entities, t.ea)
	case *Distance:
		return append(points, t.P1, t.P2), entities
	case *HorizontalDistance:
		return append(points, t.P1, t.P2), entities
	case *VerticalDistance:
		return append(points, t.P1, t.P2), entities
	case *DistancePointLine:
		return append(points, t.P), append(entities, t.L)
	case *DistancePointCircle:
		return append(points, t.P), append(entities, t.C)
	case *DistanceLineCircle:
		return points, append(entities, t.L, t.C)
	case *DistancePointArc:
		return append(points, t.P), append(entities, t.A)
	case *DistanceLineArc:
		return points, append(entities, t.L, t.A)
	case *DistanceLines:
		return points, append(entities, t.L1, t.L2)
	case *Offset:
		return points, append(entities, t.Src, t.Dst)
	case *RadialOffset:
		return points, append(entities, t.Src, t.Dst)
	case *Radius:
		return points, append(entities, t.C)
	case *Diameter:
		return points, append(entities, t.C)
	case *ArcLength:
		return points, append(entities, t.A)
	case *Angle:
		return points, append(entities, t.L1, t.L2)
	case *SemiMajor:
		return points, append(entities, t.E)
	case *SemiMinor:
		return points, append(entities, t.E)
	case *EllipseRotation:
		return points, append(entities, t.E)
	}
	return points, entities
}
