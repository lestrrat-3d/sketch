package param_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch/param"
	"github.com/lestrrat-3d/sketch/units"
	"github.com/stretchr/testify/require"
)

func kindTable(t *testing.T) *param.Table {
	t.Helper()
	tbl := param.New()
	require.NoError(t, tbl.SetValue("width", units.Millimeters(100)))
	require.NoError(t, tbl.SetValue("height", units.Millimeters(50)))
	require.NoError(t, tbl.SetValue("theta", units.Degrees(30)))
	require.NoError(t, tbl.SetValue("ratio", units.Scalar(2)))
	return tbl
}

func TestKindValidExpressions(t *testing.T) {
	tbl := kindTable(t)
	cases := []struct {
		expr string
		kind units.Kind
	}{
		{"width * 2", units.Length},
		{"2 * width", units.Length},
		{"width / 2", units.Length},
		{"width / height", units.Dimensionless}, // a ratio
		{"width + height", units.Length},
		{"width - height", units.Length},
		{"width * ratio", units.Length},
		{"theta + theta", units.Angle},
		{"theta + pi/2", units.Angle}, // radians are dimensionless: angle + number → angle
		{"theta - pi", units.Angle},   // angle - dimensionless → angle
		{"max(theta, pi/4)", units.Angle},
		{"theta % tau", units.Angle}, // wrap an angle by a dimensionless period
		{"sin(theta)", units.Dimensionless},
		{"atan(ratio)", units.Angle},
		{"abs(width)", units.Length},
		{"max(width, height)", units.Length},
		{"hypot(width, height)", units.Length},
		{"rad(45)", units.Angle},
		{"2 + 3", units.Dimensionless},
	}
	for _, tc := range cases {
		v, err := tbl.EvalValue(tc.expr)
		require.NoErrorf(t, err, "expr %q should be valid", tc.expr)
		require.Equalf(t, tc.kind, v.Kind(), "expr %q kind", tc.expr)
	}
}

func TestKindIncompatibleExpressions(t *testing.T) {
	tbl := kindTable(t)
	bad := []string{
		"width + theta",     // length + angle
		"width + 5",         // length + dimensionless (use a typed parameter)
		"width * height",    // no area unit
		"1 / width",         // inverse length
		"width / theta",     // length / angle
		"sqrt(width)",       // sqrt of a length
		"sin(width)",        // trig of a length
		"width ^ 2",         // power of a dimensioned value
		"min(width, theta)", // mixed kinds
	}
	for _, expr := range bad {
		_, err := tbl.EvalValue(expr)
		require.ErrorIsf(t, err, param.ErrIncompatibleKind, "expr %q should be a kind error", expr)
		_, err = tbl.Eval(expr)
		require.ErrorIsf(t, err, param.ErrIncompatibleKind, "Eval(%q) should also reject", expr)
	}
}

func TestKindParameterDefinitionRejected(t *testing.T) {
	// A parameter DEFINED with a mixed-kind expression errors when evaluated.
	tbl := kindTable(t)
	require.NoError(t, tbl.SetExpr("bad", "width + theta", units.Millimeter))
	_, err := tbl.Get("bad")
	require.ErrorIs(t, err, param.ErrIncompatibleKind)
	require.ErrorIs(t, tbl.Validate(), param.ErrIncompatibleKind)
}

func TestKindDeclaredUnitMismatchRejected(t *testing.T) {
	// A parameter declared as a length but DEFINED by an angle expression must be
	// rejected — otherwise it would masquerade as a length (its declared kind) and
	// smuggle an angle into a length dimension.
	tbl := kindTable(t)
	require.NoError(t, tbl.SetExpr("w", "theta", units.Millimeter)) // declared length, expr angle
	_, err := tbl.Get("w")
	require.ErrorIs(t, err, param.ErrIncompatibleKind)
	require.ErrorIs(t, tbl.Validate(), param.ErrIncompatibleKind)

	// A length parameter declared with a length expression is fine.
	require.NoError(t, tbl.SetExpr("ok", "width * 2", units.Millimeter))
	_, err = tbl.Get("ok")
	require.NoError(t, err)
	// A dimensionless expression is still interpreted in the declared unit.
	require.NoError(t, tbl.SetExpr("scaled", "3", units.Millimeter))
	got, err := tbl.Get("scaled")
	require.NoError(t, err)
	require.InDelta(t, 3, got, 1e-9)
}

func TestKindValidParameterChainSolves(t *testing.T) {
	tbl := kindTable(t)
	require.NoError(t, tbl.SetExpr("total", "width + height", units.Millimeter))
	got, err := tbl.Get("total")
	require.NoError(t, err)
	require.InDelta(t, 150, got, 1e-9) // 100mm + 50mm in base units
}
