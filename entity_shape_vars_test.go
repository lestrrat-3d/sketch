package sketch_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// entityBuilder builds one entity of a given type, on its own fresh points, so
// several of them can share a sketch without interfering.
type entityBuilder struct {
	name  string
	build func(*testing.T, *sketch.Sketch) sketch.Entity
}

// entityBuilders builds one entity of every type. The list is asserted against
// entityTypeNames below rather than trusted, so a new entity type trips this
// test too.
var entityBuilders = []entityBuilder{
	{"Line", func(t *testing.T, s *sketch.Sketch) sketch.Entity {
		return s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(10, 0))
	}},
	{"Circle", func(t *testing.T, s *sketch.Sketch) sketch.Entity {
		return s.CreateCircle(s.CreatePoint(0, 20), 4)
	}},
	{"Arc", func(t *testing.T, s *sketch.Sketch) sketch.Entity {
		return s.CreateArc(s.CreatePoint(0, 40), s.CreatePoint(5, 40), s.CreatePoint(0, 45))
	}},
	{"Ellipse", func(t *testing.T, s *sketch.Sketch) sketch.Entity {
		return s.CreateEllipse(s.CreatePoint(0, 60), 5, 3, 0)
	}},
	{"EllipticalArc", func(t *testing.T, s *sketch.Sketch) sketch.Entity {
		center := s.CreatePoint(0, 80)
		return s.CreateEllipticalArc(center, s.CreatePoint(5, 80), s.CreatePoint(0, 83), 5, 3, 0)
	}},
	{"Conic", func(t *testing.T, s *sketch.Sketch) sketch.Entity {
		c, err := s.CreateConic(s.CreatePoint(0, 100), s.CreatePoint(2, 105), s.CreatePoint(6, 100), 0.5)
		require.NoError(t, err)
		return c
	}},
	{"Spline", func(t *testing.T, s *sketch.Sketch) sketch.Entity {
		sp, err := s.CreateSpline(s.CreatePoint(0, 120), s.CreatePoint(2, 124), s.CreatePoint(5, 118), s.CreatePoint(8, 122))
		require.NoError(t, err)
		return sp
	}},
	{"ClosedSpline", func(t *testing.T, s *sketch.Sketch) sketch.Entity {
		sp, err := s.CreateClosedSpline(s.CreatePoint(0, 140), s.CreatePoint(4, 144), s.CreatePoint(8, 140))
		require.NoError(t, err)
		return sp
	}},
	{"FitSpline", func(t *testing.T, s *sketch.Sketch) sketch.Entity {
		sp, err := s.CreateFitSpline(s.CreatePoint(0, 160), s.CreatePoint(4, 164), s.CreatePoint(8, 160))
		require.NoError(t, err)
		return sp
	}},
	{"NURBS", func(t *testing.T, s *sketch.Sketch) sketch.Entity {
		c, err := s.CreateNURBS(2,
			[]*sketch.Point{s.CreatePoint(0, 180), s.CreatePoint(4, 184), s.CreatePoint(8, 180)},
			nil, sketch.ClampedUniformKnots(3, 2))
		require.NoError(t, err)
		return c
	}},
}

// TestFixEntityGroundsEveryVariable pins that grounding an entity leaves it with
// no degree of freedom at all — its defining points AND every scalar variable it
// owns beyond them (a circle's radius, an ellipse's or elliptical arc's
// semi-axes and rotation, a conic's rho). DOF() answers from the Jacobian's free
// columns rather than from the grounding API, so a variable missing from the one
// definition of an entity's shape variables (entityShapeVars) shows up here as a
// remaining degree of freedom, while FixEntity/EntityFixed — which read that
// same definition — would agree with each other and report nothing.
func TestFixEntityGroundsEveryVariable(t *testing.T) {
	t.Run("the builders cover every entity type", func(t *testing.T) {
		s := newSketch(t)
		var built []string
		for _, b := range entityBuilders {
			e := b.build(t, s)
			name := strings.TrimPrefix(fmt.Sprintf("%T", e), "*sketch.")
			require.Equal(t, b.name, name, `the builder named %s builds a %s`, b.name, name)
			built = append(built, name)
		}
		sort.Strings(built)
		require.Equal(t, entityTypeNames, built,
			`the Entity set moved: add the new type to entityBuilders so its shape variables are covered here too`)
	})

	for _, b := range entityBuilders {
		t.Run(b.name, func(t *testing.T) {
			s := newSketch(t)
			e := b.build(t, s)
			require.Greater(t, s.DOF(), 0, `an ungrounded %s is free to move`, b.name)

			s.FixEntity(e)
			require.Equal(t, 0, s.DOF(),
				`grounding a %s must leave no free variable; a shape variable missing from entityShapeVars stays free here`, b.name)
			require.True(t, s.EntityFixed(e))
		})
	}

	t.Run("one sketch holding every entity type", func(t *testing.T) {
		s := newSketch(t)
		ents := make([]sketch.Entity, 0, len(entityBuilders))
		for _, b := range entityBuilders {
			ents = append(ents, b.build(t, s))
		}
		require.Greater(t, s.DOF(), 0, `the ungrounded sketch is free to move`)

		for _, e := range ents {
			s.FixEntity(e)
		}
		require.Equal(t, 0, s.DOF(), `grounding every entity must leave no free variable`)
		for i, e := range ents {
			require.Truef(t, s.EntityFixed(e), `%s reads grounded`, entityBuilders[i].name)
		}
	})
}

// TestRemoveEntityRetiresShapeVariables pins the other side of the one
// definition: removal retires the scalar variables the removed entity owned, so
// they do not linger as free degrees of freedom of a sketch that no longer holds
// the entity.
func TestRemoveEntityRetiresShapeVariables(t *testing.T) {
	for _, b := range entityBuilders {
		t.Run(b.name, func(t *testing.T) {
			s := newSketch(t)
			e := b.build(t, s)
			for _, p := range s.Points() {
				s.Fix(p)
			}
			// Only the entity's own scalar variables are still free.
			require.True(t, s.RemoveEntity(e))
			require.Equal(t, 0, s.DOF(),
				`removing a %s must retire every variable it owned; one missed by entityShapeVars stays free here`, b.name)
		})
	}
}
