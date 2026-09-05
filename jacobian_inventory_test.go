// This file is an INTERNAL test (package sketch, not sketch_test), against the
// repository convention that tests exercise only the exported API, for the same
// reason solver_matrix_test.go and solver_elimination_test.go are: the claim
// under test is about a private dependency inventory (constraintVarIndices,
// jacobian.go) that no public call exposes. The local residual Jacobian is
// bit-identical to the dense one, so every public read — solved coordinates,
// DOF, conditioning, probe configurations — is identical whether the inventory
// is right or wrong. Exporting the inventory to serve a test would be worse.
package sketch

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// depFixture builds one committed constraint of a single kind on a fresh
// sketch, together with whatever geometry that kind needs. The sketch is the
// unit of the fixture: the completeness check below perturbs EVERY variable the
// sketch holds, so extra geometry is not noise, it is the control that proves
// the constraint ignores what it does not name.
type depFixture struct {
	name  string
	build func(*Sketch) Constraint
}

func newInventorySketch(t *testing.T) *Sketch {
	t.Helper()
	w := NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	return s
}

// depFixtures builds one instance of every constraint kind the package defines
// — every public New… constructor plus the two internal constraints the
// geometry builders add themselves (arc_radius, elliptical_arc_on).
// TestConstraintDependencyTableCoversEveryKind checks the list against the
// constructors declared in constraint.go, so a constructor added later fails
// this file until it is classified here.
func depFixtures() []depFixture {
	return []depFixture{
		{"coincident", func(s *Sketch) Constraint {
			a, b := s.CreatePoint(0, 0), s.CreatePoint(3, 1)
			c := NewCoincident(a, b)
			s.AddConstraint(c)
			return c
		}},
		{"horizontal", func(s *Sketch) Constraint {
			l := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(4, 1))
			c := NewHorizontal(l)
			s.AddConstraint(c)
			return c
		}},
		{"vertical", func(s *Sketch) Constraint {
			l := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(1, 4))
			c := NewVertical(l)
			s.AddConstraint(c)
			return c
		}},
		{"horizontal_points", func(s *Sketch) Constraint {
			c := NewHorizontalPoints(s.CreatePoint(0, 0), s.CreatePoint(4, 1))
			s.AddConstraint(c)
			return c
		}},
		{"vertical_points", func(s *Sketch) Constraint {
			c := NewVerticalPoints(s.CreatePoint(0, 0), s.CreatePoint(1, 4))
			s.AddConstraint(c)
			return c
		}},
		{"parallel", func(s *Sketch) Constraint {
			l1 := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(4, 0))
			l2 := s.CreateLine(s.CreatePoint(0, 2), s.CreatePoint(4, 3))
			c := NewParallel(l1, l2)
			s.AddConstraint(c)
			return c
		}},
		{"perpendicular", func(s *Sketch) Constraint {
			l1 := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(4, 0))
			l2 := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(1, 4))
			c := NewPerpendicular(l1, l2)
			s.AddConstraint(c)
			return c
		}},
		{"point_on_line", func(s *Sketch) Constraint {
			l := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(4, 0))
			c := NewPointOnLine(s.CreatePoint(2, 1), l)
			s.AddConstraint(c)
			return c
		}},
		{"collinear", func(s *Sketch) Constraint {
			l1 := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(4, 0))
			l2 := s.CreateLine(s.CreatePoint(5, 1), s.CreatePoint(9, 1))
			c := NewCollinear(l1, l2)
			s.AddConstraint(c)
			return c
		}},
		{"point_on_circle", func(s *Sketch) Constraint {
			ci := s.CreateCircle(s.CreatePoint(0, 0), 2)
			c := NewPointOnCircle(s.CreatePoint(3, 0), ci)
			s.AddConstraint(c)
			return c
		}},
		{"point_on_arc", func(s *Sketch) Constraint {
			a := s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(2, 0), s.CreatePoint(0, 2))
			c := NewPointOnArc(s.CreatePoint(1.6, 1.6), a)
			s.AddConstraint(c)
			return c
		}},
		{"point_on_elliptical_arc", func(s *Sketch) Constraint {
			ea := s.CreateEllipticalArc(s.CreatePoint(0, 0), s.CreatePoint(3, 0), s.CreatePoint(0, 2), 3, 2, 0)
			c := NewPointOnEllipticalArc(s.CreatePoint(2, 1.4), ea)
			s.AddConstraint(c)
			return c
		}},
		{"point_on_spline", func(s *Sketch) Constraint {
			sp, err := s.CreateSpline(s.CreatePoint(0, 0), s.CreatePoint(1, 2), s.CreatePoint(3, 2), s.CreatePoint(4, 0))
			if err != nil {
				panic(err)
			}
			c := NewPointOnSpline(s.CreatePoint(2, 1.4), sp)
			s.AddConstraint(c)
			return c
		}},
		{"tangent_spline", func(s *Sketch) Constraint {
			sp, err := s.CreateSpline(s.CreatePoint(0, 0), s.CreatePoint(1, 2), s.CreatePoint(3, 2), s.CreatePoint(4, 0))
			if err != nil {
				panic(err)
			}
			l := s.CreateLine(s.CreatePoint(-1, 1.5), s.CreatePoint(5, 1.5))
			c := NewTangentToSpline(l, sp)
			s.AddConstraint(c)
			return c
		}},
		{"point_on_closed_spline", func(s *Sketch) Constraint {
			sp, err := s.CreateClosedSpline(s.CreatePoint(0, 0), s.CreatePoint(2, 1), s.CreatePoint(2, 3), s.CreatePoint(0, 4), s.CreatePoint(-2, 2))
			if err != nil {
				panic(err)
			}
			c := NewPointOnClosedSpline(s.CreatePoint(1.5, 2), sp)
			s.AddConstraint(c)
			return c
		}},
		{"point_on_fit_spline", func(s *Sketch) Constraint {
			sp, err := s.CreateFitSpline(s.CreatePoint(0, 0), s.CreatePoint(2, 2), s.CreatePoint(4, 0))
			if err != nil {
				panic(err)
			}
			c := NewPointOnFitSpline(s.CreatePoint(2, 1.2), sp)
			s.AddConstraint(c)
			return c
		}},
		{"tangent_closed_spline", func(s *Sketch) Constraint {
			sp, err := s.CreateClosedSpline(s.CreatePoint(0, 0), s.CreatePoint(2, 1), s.CreatePoint(2, 3), s.CreatePoint(0, 4), s.CreatePoint(-2, 2))
			if err != nil {
				panic(err)
			}
			l := s.CreateLine(s.CreatePoint(-4, 3.6), s.CreatePoint(4, 3.6))
			c := NewTangentToClosedSpline(l, sp)
			s.AddConstraint(c)
			return c
		}},
		{"tangent_fit_spline", func(s *Sketch) Constraint {
			sp, err := s.CreateFitSpline(s.CreatePoint(0, 0), s.CreatePoint(2, 2), s.CreatePoint(4, 0))
			if err != nil {
				panic(err)
			}
			l := s.CreateLine(s.CreatePoint(-1, 1.9), s.CreatePoint(5, 1.9))
			c := NewTangentToFitSpline(l, sp)
			s.AddConstraint(c)
			return c
		}},
		{"point_on_conic", func(s *Sketch) Constraint {
			co, err := s.CreateConic(s.CreatePoint(0, 0), s.CreatePoint(2, 3), s.CreatePoint(4, 0), 0.5)
			if err != nil {
				panic(err)
			}
			c := NewPointOnConic(s.CreatePoint(2, 1.4), co)
			s.AddConstraint(c)
			return c
		}},
		{"tangent_conic", func(s *Sketch) Constraint {
			co, err := s.CreateConic(s.CreatePoint(0, 0), s.CreatePoint(2, 3), s.CreatePoint(4, 0), 0.5)
			if err != nil {
				panic(err)
			}
			l := s.CreateLine(s.CreatePoint(-1, 1.6), s.CreatePoint(5, 1.6))
			c := NewTangentToConic(l, co)
			s.AddConstraint(c)
			return c
		}},
		{"point_on_nurbs", func(s *Sketch) Constraint {
			cp := []*Point{s.CreatePoint(0, 0), s.CreatePoint(1, 2), s.CreatePoint(3, 2), s.CreatePoint(4, 0)}
			nb, err := s.CreateNURBS(3, cp, nil, ClampedUniformKnots(4, 3))
			if err != nil {
				panic(err)
			}
			c := NewPointOnNURBS(s.CreatePoint(2, 1.4), nb)
			s.AddConstraint(c)
			return c
		}},
		{"tangent_nurbs", func(s *Sketch) Constraint {
			cp := []*Point{s.CreatePoint(0, 0), s.CreatePoint(1, 2), s.CreatePoint(3, 2), s.CreatePoint(4, 0)}
			nb, err := s.CreateNURBS(3, cp, nil, ClampedUniformKnots(4, 3))
			if err != nil {
				panic(err)
			}
			l := s.CreateLine(s.CreatePoint(-1, 1.5), s.CreatePoint(5, 1.5))
			c := NewTangentToNURBS(l, nb)
			s.AddConstraint(c)
			return c
		}},
		{"tangent_ellipse_circle", func(s *Sketch) Constraint {
			e := s.CreateEllipse(s.CreatePoint(0, 0), 4, 2, 0)
			ci := s.CreateCircle(s.CreatePoint(9, 0), 3)
			c := NewTangentEllipseCircular(e, ci, false)
			s.AddConstraint(c)
			return c
		}},
		{"tangent_ellipses", func(s *Sketch) Constraint {
			e1 := s.CreateEllipse(s.CreatePoint(0, 0), 4, 2, 0)
			e2 := s.CreateEllipse(s.CreatePoint(9, 0), 3, 2, 0.2)
			c := NewTangentEllipses(e1, e2, false)
			s.AddConstraint(c)
			return c
		}},
		{"midpoint", func(s *Sketch) Constraint {
			l := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(4, 0))
			c := NewMidpoint(s.CreatePoint(2, 1), l)
			s.AddConstraint(c)
			return c
		}},
		{"midpoint_of", func(s *Sketch) Constraint {
			c := NewMidpointOf(s.CreatePoint(2, 1), s.CreatePoint(0, 0), s.CreatePoint(4, 0))
			s.AddConstraint(c)
			return c
		}},
		{"symmetric", func(s *Sketch) Constraint {
			ax := s.CreateLine(s.CreatePoint(0, -5), s.CreatePoint(0, 5))
			c := NewSymmetric(s.CreatePoint(-2, 1), s.CreatePoint(2.5, 1.2), ax)
			s.AddConstraint(c)
			return c
		}},
		{"symmetric_lines", func(s *Sketch) Constraint {
			ax := s.CreateLine(s.CreatePoint(0, -5), s.CreatePoint(0, 5))
			l1 := s.CreateLine(s.CreatePoint(-3, 0), s.CreatePoint(-1, 2))
			l2 := s.CreateLine(s.CreatePoint(3, 0), s.CreatePoint(1, 2.2))
			c := NewSymmetricLines(l1, l2, ax)
			s.AddConstraint(c)
			return c
		}},
		{"symmetric_circles", func(s *Sketch) Constraint {
			ax := s.CreateLine(s.CreatePoint(0, -5), s.CreatePoint(0, 5))
			c1 := s.CreateCircle(s.CreatePoint(-3, 0), 1)
			c2 := s.CreateCircle(s.CreatePoint(3.2, 0), 1.1)
			c := NewSymmetricCircles(c1, c2, ax)
			s.AddConstraint(c)
			return c
		}},
		{"symmetric_arcs", func(s *Sketch) Constraint {
			ax := s.CreateLine(s.CreatePoint(0, -5), s.CreatePoint(0, 5))
			a1 := s.CreateArc(s.CreatePoint(-3, 0), s.CreatePoint(-1, 0), s.CreatePoint(-3, 2))
			a2 := s.CreateArc(s.CreatePoint(3, 0), s.CreatePoint(1, 0), s.CreatePoint(3, 2.1))
			c := NewSymmetricArcs(a1, a2, ax)
			s.AddConstraint(c)
			return c
		}},
		{"concentric", func(s *Sketch) Constraint {
			c1 := s.CreateCircle(s.CreatePoint(0, 0), 1)
			c2 := s.CreateCircle(s.CreatePoint(0.3, 0.2), 2)
			c := NewConcentric(c1, c2)
			s.AddConstraint(c)
			return c
		}},
		{"equal_lines", func(s *Sketch) Constraint {
			l1 := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(4, 0))
			l2 := s.CreateLine(s.CreatePoint(0, 2), s.CreatePoint(3, 2))
			c := NewEqual(l1, l2)
			s.AddConstraint(c)
			return c
		}},
		{"equal_radii", func(s *Sketch) Constraint {
			c1 := s.CreateCircle(s.CreatePoint(0, 0), 1)
			c2 := s.CreateCircle(s.CreatePoint(5, 0), 2)
			c := NewEqualRadius(c1, c2)
			s.AddConstraint(c)
			return c
		}},
		{"point_on_ellipse", func(s *Sketch) Constraint {
			e := s.CreateEllipse(s.CreatePoint(0, 0), 4, 2, 0.1)
			c := NewPointOnEllipse(s.CreatePoint(4, 1), e)
			s.AddConstraint(c)
			return c
		}},
		{"tangent_line_circle", func(s *Sketch) Constraint {
			a := s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(2, 0), s.CreatePoint(0, 2))
			l := s.CreateLine(s.CreatePoint(-3, 2.2), s.CreatePoint(3, 2.2))
			c := NewTangent(l, a)
			s.AddConstraint(c)
			return c
		}},
		{"tangent_circles", func(s *Sketch) Constraint {
			a1 := s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(2, 0), s.CreatePoint(0, 2))
			a2 := s.CreateArc(s.CreatePoint(5, 0), s.CreatePoint(7, 0), s.CreatePoint(5, 2))
			c := NewTangentCircles(a1, a2, false)
			s.AddConstraint(c)
			return c
		}},
		{"tangent_ellipse", func(s *Sketch) Constraint {
			ea := s.CreateEllipticalArc(s.CreatePoint(0, 0), s.CreatePoint(3, 0), s.CreatePoint(0, 2), 3, 2, 0)
			l := s.CreateLine(s.CreatePoint(-4, 2.2), s.CreatePoint(4, 2.2))
			c := NewTangentEllipse(l, ea)
			s.AddConstraint(c)
			return c
		}},
		{"distance", func(s *Sketch) Constraint {
			c := NewDistance(s.CreatePoint(0, 0), s.CreatePoint(3, 0), 4)
			s.AddConstraint(c)
			return c
		}},
		{"hdistance", func(s *Sketch) Constraint {
			c := NewHorizontalDistance(s.CreatePoint(0, 0), s.CreatePoint(3, 1), 4)
			s.AddConstraint(c)
			return c
		}},
		{"vdistance", func(s *Sketch) Constraint {
			c := NewVerticalDistance(s.CreatePoint(0, 0), s.CreatePoint(1, 3), 4)
			s.AddConstraint(c)
			return c
		}},
		{"distance_point_line", func(s *Sketch) Constraint {
			l := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(4, 0))
			c := NewDistancePointLine(s.CreatePoint(2, 2), l, 3)
			s.AddConstraint(c)
			return c
		}},
		{"distance_point_circle", func(s *Sketch) Constraint {
			ci := s.CreateCircle(s.CreatePoint(0, 0), 2)
			c := NewDistancePointCircle(s.CreatePoint(5, 0), ci, 2)
			s.AddConstraint(c)
			return c
		}},
		{"distance_line_circle", func(s *Sketch) Constraint {
			ci := s.CreateCircle(s.CreatePoint(0, 0), 2)
			l := s.CreateLine(s.CreatePoint(-4, 4), s.CreatePoint(4, 4))
			c := NewDistanceLineCircle(l, ci, 1)
			s.AddConstraint(c)
			return c
		}},
		{"distance_point_arc", func(s *Sketch) Constraint {
			a := s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(2, 0), s.CreatePoint(0, 2))
			c := NewDistancePointArc(s.CreatePoint(3, 3), a, 1)
			s.AddConstraint(c)
			return c
		}},
		{"distance_line_arc", func(s *Sketch) Constraint {
			a := s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(2, 0), s.CreatePoint(0, 2))
			l := s.CreateLine(s.CreatePoint(-3, 3), s.CreatePoint(3, 3))
			c := NewDistanceLineArc(l, a, 1)
			s.AddConstraint(c)
			return c
		}},
		{"distance_lines", func(s *Sketch) Constraint {
			l1 := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(4, 0))
			l2 := s.CreateLine(s.CreatePoint(0, 3), s.CreatePoint(4, 3))
			c := NewDistanceLines(l1, l2, 2)
			s.AddConstraint(c)
			return c
		}},
		{"offset", func(s *Sketch) Constraint {
			l1 := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(4, 0))
			l2 := s.CreateLine(s.CreatePoint(0, 3), s.CreatePoint(4, 3))
			c := NewOffset(l1, l2, 2)
			s.AddConstraint(c)
			return c
		}},
		{"radius", func(s *Sketch) Constraint {
			ci := s.CreateCircle(s.CreatePoint(0, 0), 2)
			c := NewRadius(ci, 3)
			s.AddConstraint(c)
			return c
		}},
		{"diameter", func(s *Sketch) Constraint {
			ci := s.CreateCircle(s.CreatePoint(0, 0), 2)
			c := NewDiameter(ci, 5)
			s.AddConstraint(c)
			return c
		}},
		{"arc_length", func(s *Sketch) Constraint {
			a := s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(2, 0), s.CreatePoint(0, 2))
			c := NewArcLength(a, 4)
			s.AddConstraint(c)
			return c
		}},
		{"equal_line_arc", func(s *Sketch) Constraint {
			a := s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(2, 0), s.CreatePoint(0, 2))
			l := s.CreateLine(s.CreatePoint(5, 0), s.CreatePoint(8, 0))
			c := NewEqualLineArc(l, a)
			s.AddConstraint(c)
			return c
		}},
		{"angle", func(s *Sketch) Constraint {
			l1 := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(4, 0))
			l2 := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(3, 3))
			c := NewAngle(l1, l2, 30)
			s.AddConstraint(c)
			return c
		}},
		{"semi_major", func(s *Sketch) Constraint {
			e := s.CreateEllipse(s.CreatePoint(0, 0), 4, 2, 0)
			c := NewSemiMajor(e, 5)
			s.AddConstraint(c)
			return c
		}},
		{"semi_minor", func(s *Sketch) Constraint {
			e := s.CreateEllipse(s.CreatePoint(0, 0), 4, 2, 0)
			c := NewSemiMinor(e, 1.5)
			s.AddConstraint(c)
			return c
		}},
		{"ellipse_rotation", func(s *Sketch) Constraint {
			e := s.CreateEllipse(s.CreatePoint(0, 0), 4, 2, 0)
			c := NewEllipseRotation(e, 20)
			s.AddConstraint(c)
			return c
		}},
		{"arc_radius", func(s *Sketch) Constraint {
			s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(2, 0), s.CreatePoint(0, 2))
			return findKind(s, "arc_radius")
		}},
		{"elliptical_arc_on", func(s *Sketch) Constraint {
			s.CreateEllipticalArc(s.CreatePoint(0, 0), s.CreatePoint(3, 0), s.CreatePoint(0, 2), 3, 2, 0)
			return findKind(s, "elliptical_arc_on")
		}},
	}
}

// findKind returns the first committed constraint of the given kind — how the
// two internal constraints, which have no public constructor, are reached.
func findKind(s *Sketch, kind string) Constraint {
	for _, c := range s.cons {
		if ConstraintKind(c) == kind {
			return c
		}
	}
	return nil
}

// dependencyRow is one row of the checked-in constraint dependency inventory:
// what a single committed constraint of this kind contributes to residuals(),
// which solver variables its residual can read, and whether the local Jacobian
// evaluator (jacobian.go) supports it or routes it to the dense fallback.
//
// The counts are of DISTINCT variable indices, split by where they come from.
// Point holds coordinate indices, both from point operands and from the
// defining points of entity operands. Shape holds the intrinsic entity
// variables entityShapeVars reports (a circle's radius, an ellipse's semi-axes
// and rotation, a conic's rho). Aux holds the constraint's own auxiliary
// unknowns (auxVars), counted after AddConstraint has allocated them.
type dependencyRow struct {
	kind    string
	rows    int
	point   int
	shape   int
	aux     int
	dynamic string
	local   bool
}

// constraintDependencies is the checked-in inventory. It is regenerated by
// running this test with the table emptied — it prints the observed table on a
// mismatch — but it is checked in rather than computed so a change in a
// constraint's row structure or dependency set has to be reviewed.
//
// The dynamic column records what can change a row: "static" means the counts
// hold for the life of the committed constraint; "aux-gated" means the row and
// aux counts are 0 until AddConstraint allocates the constraint's auxiliary
// variables, and the local evaluator therefore verifies the row count on every
// perturbed evaluation rather than trusting the plan; "shared-endpoint" means
// the row count also depends on whether the two operands share a defining
// point. Parameter binding never appears here: a bound dimension's target is
// refreshed by ApplyParameters BEFORE the solve and is not a solver variable,
// so it moves no dependency.
var constraintDependencies = []dependencyRow{
	{"coincident", 2, 4, 0, 0, "static", true},
	{"horizontal", 1, 4, 0, 0, "static", true},
	{"vertical", 1, 4, 0, 0, "static", true},
	{"horizontal_points", 1, 4, 0, 0, "static", true},
	{"vertical_points", 1, 4, 0, 0, "static", true},
	{"parallel", 1, 8, 0, 0, "static", true},
	{"perpendicular", 1, 8, 0, 0, "static", true},
	{"point_on_line", 1, 6, 0, 0, "static", true},
	{"collinear", 2, 8, 0, 0, "static", true},
	{"point_on_circle", 1, 4, 1, 0, "static", true},
	{"point_on_arc", 2, 8, 0, 1, "aux-gated", true},
	{"point_on_elliptical_arc", 2, 8, 3, 1, "aux-gated", true},
	{"point_on_spline", 4, 10, 0, 3, "aux-gated", true},
	{"tangent_spline", 5, 12, 0, 4, "aux-gated", true},
	{"point_on_closed_spline", 2, 12, 0, 1, "aux-gated", true},
	{"point_on_fit_spline", 4, 8, 0, 3, "aux-gated", true},
	{"tangent_closed_spline", 3, 14, 0, 2, "aux-gated", true},
	{"tangent_fit_spline", 5, 10, 0, 4, "aux-gated", true},
	{"point_on_conic", 4, 8, 1, 3, "aux-gated", true},
	{"tangent_conic", 5, 10, 1, 4, "aux-gated", true},
	{"point_on_nurbs", 4, 10, 0, 3, "aux-gated", true},
	{"tangent_nurbs", 5, 12, 0, 4, "aux-gated", true},
	{"tangent_ellipse_circle", 4, 4, 4, 3, "aux-gated", true},
	{"tangent_ellipses", 4, 4, 6, 3, "aux-gated", true},
	{"midpoint", 2, 6, 0, 0, "static", true},
	{"midpoint_of", 2, 6, 0, 0, "static", true},
	{"symmetric", 2, 8, 0, 0, "static", true},
	{"symmetric_lines", 4, 12, 0, 0, "static", true},
	{"symmetric_circles", 3, 8, 2, 0, "static", true},
	{"symmetric_arcs", 6, 16, 0, 1, "aux-gated", true},
	{"concentric", 2, 4, 2, 0, "static", true},
	{"equal_lines", 1, 8, 0, 0, "static", true},
	{"equal_radii", 1, 4, 2, 0, "static", true},
	{"point_on_ellipse", 1, 4, 3, 0, "static", true},
	{"tangent_line_circle", 2, 10, 0, 1, "aux-gated", true},
	{"tangent_circles", 3, 12, 0, 2, "aux-gated", true},
	{"tangent_ellipse", 2, 10, 3, 1, "aux-gated", true},
	{"distance", 1, 4, 0, 0, "static", true},
	{"hdistance", 1, 4, 0, 0, "static", true},
	{"vdistance", 1, 4, 0, 0, "static", true},
	{"distance_point_line", 1, 6, 0, 0, "static", true},
	{"distance_point_circle", 1, 4, 1, 0, "static", true},
	{"distance_line_circle", 1, 6, 1, 0, "static", true},
	{"distance_point_arc", 2, 8, 0, 1, "aux-gated", true},
	{"distance_line_arc", 2, 10, 0, 1, "aux-gated", true},
	{"distance_lines", 2, 8, 0, 0, "static", true},
	{"offset", 2, 8, 0, 0, "static", true},
	{"radius", 1, 2, 1, 0, "static", true},
	{"diameter", 1, 2, 1, 0, "static", true},
	{"arc_length", 2, 6, 0, 1, "aux-gated", true},
	{"equal_line_arc", 1, 10, 0, 0, "static", true},
	{"angle", 1, 8, 0, 0, "static", true},
	{"semi_major", 1, 2, 3, 0, "static", true},
	{"semi_minor", 1, 2, 3, 0, "static", true},
	{"ellipse_rotation", 1, 2, 3, 0, "static", true},
	{"arc_radius", 1, 6, 0, 0, "static", true},
	{"elliptical_arc_on", 1, 6, 3, 0, "static", true},
}

func TestConstraintDependencyInventory(t *testing.T) {
	fixtures := depFixtures()
	got := make([]dependencyRow, 0, len(fixtures))
	for _, f := range fixtures {
		s := newInventorySketch(t)
		c := f.build(s)
		require.NotNil(t, c, "%s: fixture built no constraint", f.name)
		require.Equal(t, f.name, ConstraintKind(c), "fixture name must be the constraint kind")

		pointIdx, shapeIdx, auxIdx := dependencySources(s, c)
		vars, ok := s.constraintVarIndices(c, nil)
		got = append(got, dependencyRow{
			kind:    f.name,
			rows:    len(c.residual(nil)),
			point:   len(pointIdx),
			shape:   len(shapeIdx),
			aux:     len(auxIdx),
			dynamic: dependencyDynamics(c, auxIdx),
			local:   ok,
		})

		// The merged set the plan uses is exactly the union of the three
		// sources, deduplicated — no fourth source, and nothing dropped.
		if ok {
			want := slices.Compact(slices.Sorted(slices.Values(slices.Concat(pointIdx, shapeIdx, auxIdx))))
			require.Equal(t, want, slices.Compact(slices.Sorted(slices.Values(vars))),
				"%s: constraintVarIndices is not the union of its three sources", f.name)
		}
	}
	if !slices.Equal(constraintDependencies, got) {
		t.Errorf("constraint dependency inventory changed; observed table:\n%s", formatDependencyTable(got))
	}
}

// dependencySources returns the distinct variable indices a constraint reaches,
// split into the three sources the plan reads: point coordinates (operand points
// and entity operands' defining points), entity shape variables, and the
// constraint's own auxiliary variables.
func dependencySources(s *Sketch, c Constraint) ([]int, []int, []int) {
	pts, ents := constraintRefs(c)
	var pointIdx, shapeIdx, auxIdx []int
	for _, p := range pts {
		if p != nil {
			pointIdx = append(pointIdx, p.xi, p.yi)
		}
	}
	for _, e := range ents {
		if isNilEntity(e) {
			continue
		}
		for _, p := range entityPoints(e) {
			if p != nil {
				pointIdx = append(pointIdx, p.xi, p.yi)
			}
		}
		for _, v := range entityShapeVars(e) {
			shapeIdx = append(shapeIdx, v.index)
		}
	}
	if auxOwnerOf(c) == s {
		idx, n := auxVars(c)
		auxIdx = append(auxIdx, idx[:n]...)
	}
	dedup := func(v []int) []int { return slices.Compact(slices.Sorted(slices.Values(v))) }
	return dedup(pointIdx), dedup(shapeIdx), dedup(auxIdx)
}

func dependencyDynamics(c Constraint, auxIdx []int) string {
	if len(auxIdx) > 0 {
		return "aux-gated"
	}
	switch t := c.(type) {
	case *tangentLineCircle:
		if t.shared != nil {
			return "shared-endpoint"
		}
	case *tangentCircles:
		if t.shared != nil {
			return "shared-endpoint"
		}
	case *tangentLineEllipse:
		if t.shared != nil {
			return "shared-endpoint"
		}
	case *tangentConics:
		if t.shared != nil {
			return "shared-endpoint"
		}
	}
	return "static"
}

func formatDependencyTable(rows []dependencyRow) string {
	var b strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&b, "\t{%q, %d, %d, %d, %d, %q, %v},\n",
			r.kind, r.rows, r.point, r.shape, r.aux, r.dynamic, r.local)
	}
	return b.String()
}

// constructorKind maps every public constraint constructor in constraint.go to
// the [ConstraintKind] of what it builds. It is the anchor between the fixture
// list above and the source: a constructor added later has no entry here, and
// TestConstraintDependencyTableCoversEveryKind fails until one is added and a
// fixture built for its kind.
var constructorKind = map[string]string{
	"NewCoincident":             "coincident",
	"NewHorizontal":             "horizontal",
	"NewVertical":               "vertical",
	"NewHorizontalPoints":       "horizontal_points",
	"NewVerticalPoints":         "vertical_points",
	"NewParallel":               "parallel",
	"NewPerpendicular":          "perpendicular",
	"NewPointOnLine":            "point_on_line",
	"NewCollinear":              "collinear",
	"NewPointOnCircle":          "point_on_circle",
	"NewPointOnArc":             "point_on_arc",
	"NewPointOnEllipticalArc":   "point_on_elliptical_arc",
	"NewPointOnSpline":          "point_on_spline",
	"NewTangentToSpline":        "tangent_spline",
	"NewPointOnClosedSpline":    "point_on_closed_spline",
	"NewPointOnFitSpline":       "point_on_fit_spline",
	"NewTangentToClosedSpline":  "tangent_closed_spline",
	"NewTangentToFitSpline":     "tangent_fit_spline",
	"NewPointOnConic":           "point_on_conic",
	"NewTangentToConic":         "tangent_conic",
	"NewPointOnNURBS":           "point_on_nurbs",
	"NewTangentToNURBS":         "tangent_nurbs",
	"NewTangentEllipseCircular": "tangent_ellipse_circle",
	"NewTangentEllipses":        "tangent_ellipses",
	"NewMidpoint":               "midpoint",
	"NewMidpointOf":             "midpoint_of",
	"NewSymmetric":              "symmetric",
	"NewSymmetricLines":         "symmetric_lines",
	"NewSymmetricCircles":       "symmetric_circles",
	"NewSymmetricArcs":          "symmetric_arcs",
	"NewConcentric":             "concentric",
	"NewEqual":                  "equal_lines",
	"NewEqualRadius":            "equal_radii",
	"NewPointOnEllipse":         "point_on_ellipse",
	"NewTangent":                "tangent_line_circle",
	"NewTangentCircles":         "tangent_circles",
	"NewTangentEllipse":         "tangent_ellipse",
	"NewDistance":               "distance",
	"NewHorizontalDistance":     "hdistance",
	"NewVerticalDistance":       "vdistance",
	"NewDistancePointLine":      "distance_point_line",
	"NewDistancePointCircle":    "distance_point_circle",
	"NewDistanceLineCircle":     "distance_line_circle",
	"NewDistancePointArc":       "distance_point_arc",
	"NewDistanceLineArc":        "distance_line_arc",
	"NewDistanceLines":          "distance_lines",
	"NewOffset":                 "offset",
	"NewRadius":                 "radius",
	"NewDiameter":               "diameter",
	"NewArcLength":              "arc_length",
	"NewEqualLineArc":           "equal_line_arc",
	"NewAngle":                  "angle",
	"NewSemiMajor":              "semi_major",
	"NewSemiMinor":              "semi_minor",
	"NewEllipseRotation":        "ellipse_rotation",
}

// TestConstraintDependencyTableCoversEveryKind parses the public constructors
// out of constraint.go the way introspect_nil_contract_test.go does, so a
// constructor added later fails here until its kind is classified in the
// inventory above. Without this anchor a new constraint kind reaches the
// residual plan with nothing recording what its residual reads, and the plan's
// only defence is [Sketch.constraintVarIndices] refusing a kind
// [constraintRefs] does not list — which is itself a checklist item a new
// constraint could satisfy while its residual reads more than it names.
func TestConstraintDependencyTableCoversEveryKind(t *testing.T) {
	built := make(map[string]struct{})
	for _, f := range depFixtures() {
		_, dup := built[f.name]
		require.Falsef(t, dup, "depFixtures lists kind %q twice", f.name)
		built[f.name] = struct{}{}
	}

	declared := publicConstraintConstructors(t)
	require.Equal(t, slices.Sorted(maps.Keys(constructorKind)), declared,
		"constructorKind is out of step with constraint.go's New… constructors")
	for ctor, kind := range constructorKind {
		require.Containsf(t, built, kind, "constructor %s builds kind %q, which has no dependency fixture", ctor, kind)
	}
	// The two internal constraints have no constructor to parse; they are added
	// by CreateArc and CreateEllipticalArc.
	require.Contains(t, built, "arc_radius")
	require.Contains(t, built, "elliptical_arc_on")
}

// publicConstraintConstructors returns the sorted names of the exported
// package-level New… functions constraint.go declares.
func publicConstraintConstructors(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "constraint.go", nil, 0)
	require.NoError(t, err, "failed to parse constraint.go")
	var names []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "New") || !fn.Name.IsExported() {
			continue
		}
		names = append(names, fn.Name.Name)
	}
	require.NotEmpty(t, names, "constraint.go declares no New… constructors — the parse is wrong")
	slices.Sort(names)
	return names
}

// TestConstraintResidualsIgnoreUnlistedVariables is the load-bearing half of
// the inventory: it perturbs every variable a constraint's dependency set does
// NOT name and requires the constraint's residual rows to come back BIT
// IDENTICAL. That is the exact property the local Jacobian relies on — an
// unlisted variable moves no row, so the dense pass's difference for that row is
// an exact zero and the local pass may leave the entry cleared.
//
// It also checks the converse is not vacuous: perturbing the LISTED variables
// moves at least one row for every kind that produces any, so a fixture cannot
// pass by being insensitive to everything.
func TestConstraintResidualsIgnoreUnlistedVariables(t *testing.T) {
	for _, f := range depFixtures() {
		t.Run(f.name, func(t *testing.T) {
			s := newInventorySketch(t)
			c := f.build(s)
			require.NotNil(t, c)
			vars, ok := s.constraintVarIndices(c, nil)
			require.True(t, ok, "every committed kind must be classified")
			listed := make(map[int]struct{}, len(vars))
			for _, v := range vars {
				listed[v] = struct{}{}
			}

			base := c.residual(nil)
			// Steps that span the finite-difference step the solver uses and a
			// step large enough to leave any local flat spot.
			for _, h := range []float64{1e-7, 0.37, -1.9} {
				for i := range s.vars {
					if _, ok := listed[i]; ok {
						continue
					}
					orig := s.vars[i]
					s.vars[i] = orig + h
					got := c.residual(nil)
					s.vars[i] = orig
					require.Len(t, got, len(base), "row count moved on an unlisted variable")
					for k := range base {
						require.Equal(t, math.Float64bits(base[k]), math.Float64bits(got[k]),
							"row %d moved when unlisted variable %d was perturbed by %g", k, i, h)
					}
				}
			}

			if len(base) == 0 {
				return
			}
			moved := false
			for v := range listed {
				orig := s.vars[v]
				s.vars[v] = orig + 0.37
				got := c.residual(nil)
				s.vars[v] = orig
				if len(got) != len(base) {
					moved = true
					break
				}
				for k := range base {
					if math.Float64bits(base[k]) != math.Float64bits(got[k]) {
						moved = true
						break
					}
				}
				if moved {
					break
				}
			}
			require.True(t, moved, "no listed variable moves any row: the fixture proves nothing")
		})
	}
}
