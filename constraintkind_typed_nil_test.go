package sketch_test

import (
	"reflect"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// TestConstraintKindTangentConicsTypedNil pins the one input shape
// TestIntrospectionNilOperandOutcomes cannot express: a typed nil of the
// unexported tangent-conics type (the dynamic type NewTangentEllipseCircular
// and NewTangentEllipses return). That table is anchored against
// constraint.go's public New… constructors and both of those constructors
// return a non-nil pointer on any input, including nil operands — so no call
// through the public API ever produces this value. Building it needs
// reflect.Zero over the dynamic type of a real constraint, which is why this
// sits in its own test rather than as a row in that table.
func TestConstraintKindTangentConicsTypedNil(t *testing.T) {
	s := newSketch(t)
	ec := s.CreatePoint(0, 0)
	e := s.CreateEllipse(ec, 4, 2, 0)
	cc := s.CreatePoint(10, 0)
	c := s.CreateCircle(cc, 3)
	live := sketch.NewTangentEllipseCircular(e, c, false)

	typedNil := reflect.Zero(reflect.TypeOf(live)).Interface().(sketch.Constraint)

	require.Equal(t, "tangent_ellipses", sketch.ConstraintKind(typedNil))
}
