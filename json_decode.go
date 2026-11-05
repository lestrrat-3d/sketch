package sketch

import (
	"encoding/json"
	"fmt"

	"github.com/lestrrat-3d/units"
)

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
	// Reject a wrong-kind document before the version gate: a world document
	// (which may use a newer schema version than a sketch) handed to the sketch
	// loader is a kind error, not a version error.
	switch pf.kind {
	case kindWorld:
		return fmt.Errorf("%w: got a world document, want a sketch", ErrWrongDocumentKind)
	case kindSketch, "":
		// handled below
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrWrongDocumentKind, pf.kind)
	}
	if pf.version > jsonMaxVersion {
		return fmt.Errorf("sketch: unsupported document version %d (this build reads up to %d)", pf.version, jsonMaxVersion)
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

	// Every sketch belongs to a world: a single-sketch document loads as an
	// implicit one-sketch world owning the inlined plane.
	w := NewWorld()
	var plane *Plane
	switch pf.kind {
	case kindSketch:
		if doc.Plane == nil {
			return ErrMissingPlane
		}
		plane, err = datumPlaneFromJSON(w, *doc.Plane)
		if err != nil {
			return err
		}
	default: // legacy: a 2D sketch is a world-XY sketch
		plane = w.XY()
	}

	// The entity uid counter is carried ACROSS the in-place reset. It is not
	// document content (never serialized), but it must never rewind: the rebuild
	// replaces every entity instance, so every handle a previously built Profile
	// holds is now dead. Restamping the rebuilt entities from 1 would reproduce
	// the pre-rebuild uids — and, for a reload of the very same bytes, the whole
	// pre-rebuild revision — so a Profile left dangling by the load would read
	// FRESH. Continuing the counter upward makes the rebuilt sketch's revision
	// differ and Profile.IsStale() report true, which is the honest answer.
	nextEntID := s.nextEntID
	// The reset also discards transient source attachments. A loaded reference
	// Source marker cannot inherit a certificate for the previous geometry.
	*s = Sketch{world: w, params: w.params, sys: units.Metric(), pl: plane, nextEntID: nextEntID}
	s.initOrigin() // the reset cleared it; every construction path owes one
	w.sketches = append(w.sketches, s)
	if err := s.buildFromBody(doc.jsonSketchBody); err != nil {
		return err
	}
	// buildFromBody adopts the body's own parameter table when the document
	// carries one; keep the implicit world's shared table pointed at it so
	// w.Params() and s.Params() never diverge.
	w.params = s.params
	return nil
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
		p := s.CreatePoint(jp.X, jp.Y)
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
	elliptical := func(i int) (Elliptical, error) {
		e, ok := s.entByID(i).(Elliptical)
		if !ok {
			return nil, fmt.Errorf("sketch: entity %d is not an ellipse or elliptical arc", i)
		}
		return e, nil
	}

	for _, je := range body.Entities {
		before := len(s.ents)
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
				if _, err := s.CreateReferenceLine(ps[0], ps[1], je.Source); err != nil {
					return err
				}
			case "arc":
				if len(ps) != 3 {
					return fmt.Errorf("sketch: arc needs 3 points, got %d", len(ps))
				}
				if je.Stale {
					return fmt.Errorf("sketch: reference arc staleness is derived, not stored")
				}
				if _, err := s.CreateReferenceArc(ps[0], ps[1], ps[2], je.Source); err != nil {
					return err
				}
			case "circle":
				if len(ps) != 1 {
					return fmt.Errorf("sketch: circle needs 1 point, got %d", len(ps))
				}
				c, err := s.CreateReferenceCircle(ps[0], je.Radius, je.Source)
				if err != nil {
					return err
				}
				c.stale = je.Stale // restore radius staleness
			default:
				return fmt.Errorf("sketch: reference geometry of kind %q is not supported", je.Type)
			}
			if len(s.ents) > before {
				s.ents[before].SetName(je.Name)
			}
			continue
		}
		switch je.Type {
		case "line":
			if len(ps) != 2 {
				return fmt.Errorf("sketch: line needs 2 points, got %d", len(ps))
			}
			s.CreateLine(ps[0], ps[1]).SetConstruction(je.Construction)
		case "circle":
			if len(ps) != 1 {
				return fmt.Errorf("sketch: circle needs 1 point, got %d", len(ps))
			}
			s.CreateCircle(ps[0], je.Radius).SetConstruction(je.Construction)
		case "arc":
			if len(ps) != 3 {
				return fmt.Errorf("sketch: arc needs 3 points, got %d", len(ps))
			}
			s.CreateArc(ps[0], ps[1], ps[2]).SetConstruction(je.Construction)
		case "ellipse":
			if len(ps) != 1 {
				return fmt.Errorf("sketch: ellipse needs 1 point, got %d", len(ps))
			}
			s.CreateEllipse(ps[0], je.Rx, je.Ry, je.Rotation).SetConstruction(je.Construction)
		case "elliptical_arc":
			if len(ps) != 3 {
				return fmt.Errorf("sketch: elliptical arc needs 3 points, got %d", len(ps))
			}
			s.CreateEllipticalArc(ps[0], ps[1], ps[2], je.Rx, je.Ry, je.Rotation).SetConstruction(je.Construction)
		case "spline":
			if je.Degree != 0 && je.Degree != 3 {
				return fmt.Errorf("sketch: unsupported spline degree %d", je.Degree)
			}
			sp, err := s.CreateSpline(ps...) // CreateSpline validates the >= 4 count
			if err != nil {
				return err
			}
			sp.SetConstruction(je.Construction)
		case "closed_spline":
			if je.Degree != 0 && je.Degree != 3 {
				return fmt.Errorf("sketch: unsupported spline degree %d", je.Degree)
			}
			sp, err := s.CreateClosedSpline(ps...) // validates the >= 3 count
			if err != nil {
				return err
			}
			sp.SetConstruction(je.Construction)
		case "fit_spline":
			if je.Degree != 0 && je.Degree != 3 {
				return fmt.Errorf("sketch: unsupported spline degree %d", je.Degree)
			}
			sp, err := s.CreateFitSpline(ps...) // validates the >= 2 count
			if err != nil {
				return err
			}
			sp.SetConstruction(je.Construction)
		case "conic":
			if len(ps) != 3 {
				return fmt.Errorf("sketch: conic needs 3 points, got %d", len(ps))
			}
			c, err := s.CreateConic(ps[0], ps[1], ps[2], je.Rho) // validates rho ∈ (0, 1)
			if err != nil {
				return err
			}
			c.SetConstruction(je.Construction)
		case "nurbs":
			// CreateNURBS validates degree, control count, knot vector and weights;
			// a nil weights slice means non-rational (all weights 1), while an
			// empty non-nil one is a weight-count violation it refuses.
			c, err := s.CreateNURBS(je.Degree, ps, je.Weights, je.Knots)
			if err != nil {
				return err
			}
			c.SetConstruction(je.Construction)
		default:
			return fmt.Errorf("sketch: unknown entity type %q", je.Type)
		}
		if len(s.ents) > before {
			s.ents[before].SetName(je.Name)
		}
	}

	for _, jc := range body.Constraints {
		if err := s.rebuildConstraint(jc, line, circle, circular, ellipse, elliptical); err != nil {
			return err
		}
		// rebuildConstraint appends exactly one public constraint via AddConstraint;
		// restore its optional label.
		if jc.Name != "" && len(s.cons) > 0 {
			s.SetConstraintName(s.cons[len(s.cons)-1], jc.Name)
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

// arcByID resolves entity i as an *Arc for a constraint named name. It first
// goes through the Circular check (preserving its "is not a circle or arc"
// error), then downcasts to *Arc with the type-specific error.
func (s *Sketch) arcByID(i int, name string) (*Arc, error) {
	c, ok := s.entByID(i).(Circular)
	if !ok {
		return nil, fmt.Errorf("sketch: entity %d is not a circle or arc", i)
	}
	arc, ok := c.(*Arc)
	if !ok {
		return nil, fmt.Errorf("sketch: %s requires an arc, got %T", name, c)
	}
	return arc, nil
}

// pointRef returns the point with id i, or an error if i is out of range. The
// v2 decoder validates every reference through this before indexing, so a
// malformed document errors rather than panicking.
func (s *Sketch) pointRef(i int) (*Point, error) {
	// The origin is not in s.points and carries its own reserved id, so it is
	// resolved here rather than positionally (see [Sketch.Origin]).
	if i == originPointID {
		if s.origin == nil {
			return nil, fmt.Errorf("sketch: document references the origin point, which this sketch has none of")
		}
		return s.origin, nil
	}
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
	"horizontal_points": {2, 0}, "vertical_points": {2, 0},
	"parallel": {0, 2}, "perpendicular": {0, 2}, "equal_lines": {0, 2},
	"collinear": {0, 2}, "angle": {0, 2}, "point_on_line": {1, 1}, "point_on_normal": {2, 1},
	"point_on_circle": {1, 1}, "point_on_arc": {1, 1}, "point_on_elliptical_arc": {1, 1},
	"point_on_ellipse": {1, 1}, "point_on_spline": {1, 1}, "point_on_closed_spline": {1, 1},
	"point_on_fit_spline": {1, 1}, "tangent_spline": {0, 2}, "tangent_closed_spline": {0, 2},
	"tangent_fit_spline": {0, 2}, "midpoint": {1, 1},
	"point_on_conic": {1, 1}, "tangent_conic": {0, 2},
	"point_on_nurbs": {1, 1}, "tangent_nurbs": {0, 2},
	"tangent_ellipse_circle": {0, 2}, "tangent_ellipses": {0, 2},
	"midpoint_of": {3, 0},
	"symmetric":   {2, 1}, "symmetric_lines": {0, 3}, "symmetric_circles": {0, 3},
	"symmetric_arcs": {0, 3},
	"concentric":     {0, 2}, "equal_radii": {0, 2}, "equal_line_arc": {0, 2},
	"tangent_line_circle": {0, 2}, "tangent_circles": {0, 2}, "tangent_ellipse": {0, 2},
	"distance": {2, 0}, "hdistance": {2, 0}, "vdistance": {2, 0},
	"distance_point_line": {1, 1}, "distance_lines": {0, 2}, "offset": {0, 2}, "radial_offset": {0, 2},
	"distance_point_circle": {1, 1}, "distance_line_circle": {0, 2},
	"distance_point_arc": {1, 1}, "distance_line_arc": {0, 2},
	"radius": {0, 1}, "diameter": {0, 1}, "arc_length": {0, 1},
	"semi_major": {0, 1}, "semi_minor": {0, 1}, "ellipse_rotation": {0, 1},
}

func (s *Sketch) rebuildConstraint(jc jsonConstraint, line func(int) (*Line, error), circle func(int) (*Circle, error), circular func(int) (Circular, error), ellipse func(int) (*Ellipse, error), elliptical func(int) (Elliptical, error)) error {
	// Validate references before indexing: enough arguments for the type, and
	// every point/entity id in range.
	if a, ok := constraintArity[jc.Type]; ok {
		if len(jc.Points) != a[0] || len(jc.Entities) != a[1] {
			return fmt.Errorf("sketch: constraint %q needs exactly %d point(s) and %d entity(ies), got %d and %d",
				jc.Type, a[0], a[1], len(jc.Points), len(jc.Entities))
		}
	}
	for _, i := range jc.Points {
		if i == originPointID && s.origin != nil {
			continue // the origin's reserved id; resolved by pointRef, not positionally
		}
		if i < 0 || i >= len(s.points) {
			return fmt.Errorf("sketch: constraint %q references point id %d out of range", jc.Type, i)
		}
	}
	for _, i := range jc.Entities {
		if i < 0 || i >= len(s.ents) {
			return fmt.Errorf("sketch: constraint %q references entity id %d out of range", jc.Type, i)
		}
	}

	// pt resolves the constraint's i-th point reference. It goes through pointRef
	// rather than indexing s.points, so the origin's reserved id resolves here the
	// same way it does everywhere else; the loop above already validated every id.
	pt := func(i int) *Point {
		p, _ := s.pointRef(jc.Points[i])
		return p
	}
	// dim restores a dimensional constraint's unit/binding, then commits it.
	dim := func(d Dimension) error {
		if err := restoreDim(d, jc); err != nil {
			return err
		}
		s.AddConstraint(d)
		return nil
	}
	switch jc.Type {
	case "coincident":
		s.AddConstraint(NewCoincident(pt(0), pt(1)))
	case "horizontal_points":
		s.AddConstraint(NewHorizontalPoints(pt(0), pt(1)))
	case "vertical_points":
		s.AddConstraint(NewVerticalPoints(pt(0), pt(1)))
	case "midpoint_of":
		s.AddConstraint(NewMidpointOf(pt(0), pt(1), pt(2)))
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
			if err := dim(NewAngle(l1, l2, jc.Value)); err != nil {
				return err
			}
		}
	case "point_on_line":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		s.AddConstraint(NewPointOnLine(pt(0), l))
	case "point_on_normal":
		if l, err := line(jc.Entities[0]); err == nil {
			s.AddConstraint(NewPointOnNormal(pt(0), l, pt(1)))
		} else {
			c, err := circular(jc.Entities[0])
			if err != nil {
				return err
			}
			a, ok := c.(*Arc)
			if !ok {
				return fmt.Errorf("sketch: point_on_normal entity %d is not a line or an arc", jc.Entities[0])
			}
			s.AddConstraint(NewPointOnNormal(pt(0), a, pt(1)))
		}
	case "point_on_circle":
		c, err := circle(jc.Entities[0])
		if err != nil {
			return err
		}
		s.AddConstraint(NewPointOnCircle(pt(0), c))
	case "point_on_arc":
		arc, err := s.arcByID(jc.Entities[0], "point_on_arc")
		if err != nil {
			return err
		}
		s.AddConstraint(NewPointOnArc(pt(0), arc))
	case "point_on_elliptical_arc":
		ea, ok := s.entByID(jc.Entities[0]).(*EllipticalArc)
		if !ok {
			return fmt.Errorf("sketch: point_on_elliptical_arc requires an elliptical arc")
		}
		s.AddConstraint(NewPointOnEllipticalArc(pt(0), ea))
	case "point_on_ellipse":
		e, err := ellipse(jc.Entities[0])
		if err != nil {
			return err
		}
		s.AddConstraint(NewPointOnEllipse(pt(0), e))
	case "point_on_spline":
		sp, ok := s.entByID(jc.Entities[0]).(*Spline)
		if !ok {
			return fmt.Errorf("sketch: point_on_spline requires a spline")
		}
		s.AddConstraint(NewPointOnSpline(pt(0), sp))
	case "point_on_closed_spline":
		sp, ok := s.entByID(jc.Entities[0]).(*ClosedSpline)
		if !ok {
			return fmt.Errorf("sketch: point_on_closed_spline requires a closed spline")
		}
		s.AddConstraint(NewPointOnClosedSpline(pt(0), sp))
	case "point_on_fit_spline":
		sp, ok := s.entByID(jc.Entities[0]).(*FitSpline)
		if !ok {
			return fmt.Errorf("sketch: point_on_fit_spline requires a fit spline")
		}
		s.AddConstraint(NewPointOnFitSpline(pt(0), sp))
	case "tangent_spline":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		sp, ok := s.entByID(jc.Entities[1]).(*Spline)
		if !ok {
			return fmt.Errorf("sketch: tangent_spline requires a spline")
		}
		s.AddConstraint(NewTangentToSpline(l, sp))
	case "tangent_closed_spline":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		sp, ok := s.entByID(jc.Entities[1]).(*ClosedSpline)
		if !ok {
			return fmt.Errorf("sketch: tangent_closed_spline requires a closed spline")
		}
		s.AddConstraint(NewTangentToClosedSpline(l, sp))
	case "tangent_fit_spline":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		sp, ok := s.entByID(jc.Entities[1]).(*FitSpline)
		if !ok {
			return fmt.Errorf("sketch: tangent_fit_spline requires a fit spline")
		}
		s.AddConstraint(NewTangentToFitSpline(l, sp))
	case "point_on_conic":
		cn, ok := s.entByID(jc.Entities[0]).(*Conic)
		if !ok {
			return fmt.Errorf("sketch: point_on_conic requires a conic")
		}
		s.AddConstraint(NewPointOnConic(pt(0), cn))
	case "tangent_conic":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		cn, ok := s.entByID(jc.Entities[1]).(*Conic)
		if !ok {
			return fmt.Errorf("sketch: tangent_conic requires a conic")
		}
		s.AddConstraint(NewTangentToConic(l, cn))
	case "point_on_nurbs":
		cn, ok := s.entByID(jc.Entities[0]).(*NURBS)
		if !ok {
			return fmt.Errorf("sketch: point_on_nurbs requires a NURBS curve")
		}
		s.AddConstraint(NewPointOnNURBS(pt(0), cn))
	case "tangent_nurbs":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		cn, ok := s.entByID(jc.Entities[1]).(*NURBS)
		if !ok {
			return fmt.Errorf("sketch: tangent_nurbs requires a NURBS curve")
		}
		s.AddConstraint(NewTangentToNURBS(l, cn))
	case "tangent_ellipse_circle":
		e, err := elliptical(jc.Entities[0])
		if err != nil {
			return err
		}
		ci, err := circular(jc.Entities[1])
		if err != nil {
			return err
		}
		s.AddConstraint(NewTangentEllipseCircular(e, ci, jc.Flag))
	case "tangent_ellipses":
		e1, err := elliptical(jc.Entities[0])
		if err != nil {
			return err
		}
		e2, err := elliptical(jc.Entities[1])
		if err != nil {
			return err
		}
		s.AddConstraint(NewTangentEllipses(e1, e2, jc.Flag))
	case "semi_major", "semi_minor", "ellipse_rotation":
		e, ok := s.entByID(jc.Entities[0]).(Elliptical)
		if !ok {
			return fmt.Errorf("sketch: %s requires an ellipse or elliptical arc", jc.Type)
		}
		switch jc.Type {
		case "semi_major":
			if err := dim(NewSemiMajor(e, jc.Value)); err != nil {
				return err
			}
		case "semi_minor":
			if err := dim(NewSemiMinor(e, jc.Value)); err != nil {
				return err
			}
		case "ellipse_rotation":
			if err := dim(NewEllipseRotation(e, jc.Value)); err != nil {
				return err
			}
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
	case "symmetric_lines":
		l1, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		l2, err := line(jc.Entities[1])
		if err != nil {
			return err
		}
		axis, err := line(jc.Entities[2])
		if err != nil {
			return err
		}
		s.AddConstraint(NewSymmetricLines(l1, l2, axis))
	case "symmetric_circles":
		c1, err := circle(jc.Entities[0])
		if err != nil {
			return err
		}
		c2, err := circle(jc.Entities[1])
		if err != nil {
			return err
		}
		axis, err := line(jc.Entities[2])
		if err != nil {
			return err
		}
		s.AddConstraint(NewSymmetricCircles(c1, c2, axis))
	case "symmetric_arcs":
		a1, err := s.arcByID(jc.Entities[0], "symmetric_arcs")
		if err != nil {
			return err
		}
		a2, err := s.arcByID(jc.Entities[1], "symmetric_arcs")
		if err != nil {
			return err
		}
		axis, err := line(jc.Entities[2])
		if err != nil {
			return err
		}
		s.AddConstraint(NewSymmetricArcs(a1, a2, axis))
	case "concentric":
		c1, err := circular(jc.Entities[0])
		if err != nil {
			return err
		}
		c2, err := circular(jc.Entities[1])
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
	case "tangent_ellipse":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		e, err := elliptical(jc.Entities[1])
		if err != nil {
			return err
		}
		s.AddConstraint(NewTangentEllipse(l, e))
	case "distance":
		if err := dim(NewDistance(pt(0), pt(1), jc.Value)); err != nil {
			return err
		}
	case "distance_point_line":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		if err := dim(NewDistancePointLine(pt(0), l, jc.Value)); err != nil {
			return err
		}
	case "distance_point_circle":
		ci, err := circle(jc.Entities[0])
		if err != nil {
			return err
		}
		if err := dim(NewDistancePointCircle(pt(0), ci, jc.Value)); err != nil {
			return err
		}
	case "distance_line_circle":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		ci, err := circle(jc.Entities[1])
		if err != nil {
			return err
		}
		if err := dim(NewDistanceLineCircle(l, ci, jc.Value)); err != nil {
			return err
		}
	case "distance_point_arc":
		arc, err := s.arcByID(jc.Entities[0], "distance_point_arc")
		if err != nil {
			return err
		}
		if err := dim(NewDistancePointArc(pt(0), arc, jc.Value)); err != nil {
			return err
		}
	case "distance_line_arc":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		arc, err := s.arcByID(jc.Entities[1], "distance_line_arc")
		if err != nil {
			return err
		}
		if err := dim(NewDistanceLineArc(l, arc, jc.Value)); err != nil {
			return err
		}
	case "distance_lines":
		l1, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		l2, err := line(jc.Entities[1])
		if err != nil {
			return err
		}
		if err := dim(NewDistanceLines(l1, l2, jc.Value)); err != nil {
			return err
		}
	case "offset":
		src, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		dst, err := line(jc.Entities[1])
		if err != nil {
			return err
		}
		if err := dim(NewOffset(src, dst, jc.Value)); err != nil {
			return err
		}
	case "radial_offset":
		src, err := circular(jc.Entities[0])
		if err != nil {
			return err
		}
		dst, err := circular(jc.Entities[1])
		if err != nil {
			return err
		}
		if err := dim(NewRadialOffset(src, dst, jc.Value)); err != nil {
			return err
		}
	case "hdistance":
		if err := dim(NewHorizontalDistance(pt(0), pt(1), jc.Value)); err != nil {
			return err
		}
	case "vdistance":
		if err := dim(NewVerticalDistance(pt(0), pt(1), jc.Value)); err != nil {
			return err
		}
	case "radius":
		c, err := circular(jc.Entities[0])
		if err != nil {
			return err
		}
		if err := dim(NewRadius(c, jc.Value)); err != nil {
			return err
		}
	case "diameter":
		c, err := circular(jc.Entities[0])
		if err != nil {
			return err
		}
		if err := dim(NewDiameter(c, jc.Value)); err != nil {
			return err
		}
	case "arc_length":
		arc, err := s.arcByID(jc.Entities[0], "arc_length")
		if err != nil {
			return err
		}
		if err := dim(NewArcLength(arc, jc.Value)); err != nil {
			return err
		}
	case "equal_line_arc":
		l, err := line(jc.Entities[0])
		if err != nil {
			return err
		}
		arc, err := s.arcByID(jc.Entities[1], "equal_line_arc")
		if err != nil {
			return err
		}
		s.AddConstraint(NewEqualLineArc(l, arc))
	default:
		return fmt.Errorf("sketch: unknown constraint type %q", jc.Type)
	}
	return nil
}
