package param_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/param"
	"github.com/lestrrat-3d/sketch/units"
	"github.com/stretchr/testify/require"
)

func TestLiteralAndExpression(t *testing.T) {
	tb := param.New()
	require.NoError(t, tb.Set("height", "60"))
	require.NoError(t, tb.Set("width", "height * 1.5"))
	require.NoError(t, tb.Set("area", "width * height"))

	require.InDelta(t, 60, tb.MustGet("height"), 1e-9, "height")
	require.InDelta(t, 90, tb.MustGet("width"), 1e-9, "width")
	require.InDelta(t, 5400, tb.MustGet("area"), 1e-9, "area")
}

func TestParametricUpdate(t *testing.T) {
	tb := param.New()
	require.NoError(t, tb.Set("height", "60"))
	require.NoError(t, tb.Set("width", "height * 1.5"))
	require.NoError(t, tb.Set("area", "width * height"))
	require.InDelta(t, 5400, tb.MustGet("area"), 1e-9, "area")

	require.NoError(t, tb.Set("height", "40")) // edit propagates downstream
	require.InDelta(t, 60, tb.MustGet("width"), 1e-9, "width after edit")
	require.InDelta(t, 2400, tb.MustGet("area"), 1e-9, "area after edit")
}

func TestForwardReference(t *testing.T) {
	tb := param.New()
	require.NoError(t, tb.Set("a", "b + 1")) // b defined later
	require.NoError(t, tb.SetNumber("b", 2))
	require.InDelta(t, 3, tb.MustGet("a"), 1e-9, "a")
}

func TestTypedValues(t *testing.T) {
	tb := param.New()
	require.NoError(t, tb.SetValue("width", units.Meters(1))) // 1 m
	require.NoError(t, tb.SetExpr("half", "width / 2", units.Millimeter))

	// Get returns the base-unit (mm) magnitude.
	require.InDelta(t, 1000, tb.MustGet("width"), 1e-9, "width base")
	require.InDelta(t, 500, tb.MustGet("half"), 1e-9, "half base")

	// GetValue carries the declared unit.
	w, err := tb.GetValue("width")
	require.NoError(t, err)
	require.Equal(t, units.Meter, w.Unit(), "width unit")
	require.InDelta(t, 1, w.Mag(), 1e-9, "width mag (m)")

	h, err := tb.GetValue("half")
	require.NoError(t, err)
	require.InDelta(t, 500, h.Mag(), 1e-9, "half mag (mm)")
	require.Equal(t, units.Length, h.Kind(), "half kind")
}

func TestTypedAngle(t *testing.T) {
	tb := param.New()
	require.NoError(t, tb.SetValue("a", units.Degrees(90)))
	require.InDelta(t, math.Pi/2, tb.MustGet("a"), 1e-9, "90deg base (rad)")
	v, err := tb.GetValue("a")
	require.NoError(t, err)
	require.InDelta(t, 90, v.Mag(), 1e-9, "mag in deg")
}

func TestUnitMethod(t *testing.T) {
	tb := param.New()
	require.NoError(t, tb.SetValue("len", units.Inches(2)))
	u, ok := tb.Unit("len")
	require.True(t, ok, "Unit(len) ok")
	require.Equal(t, units.Inch, u)
}

func TestJSONRoundTripUnits(t *testing.T) {
	tb := param.New()
	require.NoError(t, tb.SetValue("width", units.Meters(2)))
	require.NoError(t, tb.SetExpr("height", "width / 4", units.Millimeter))
	require.NoError(t, tb.Set("ratio", "1.5"))

	data, err := json.Marshal(tb)
	require.NoError(t, err)

	var tb2 param.Table
	require.NoError(t, json.Unmarshal(data, &tb2))

	w, err := tb2.GetValue("width")
	require.NoError(t, err)
	require.Equal(t, units.Meter, w.Unit(), "reloaded width unit")
	require.InDelta(t, 2000, tb2.MustGet("width"), 1e-9, "reloaded width base")
	require.InDelta(t, 500, tb2.MustGet("height"), 1e-9, "reloaded height base")
	require.InDelta(t, 1.5, tb2.MustGet("ratio"), 1e-9, "reloaded ratio")
}

func TestPrecedenceAndAssociativity(t *testing.T) {
	tb := param.New()
	cases := map[string]float64{
		"2 + 3 * 4":   14,
		"(2 + 3) * 4": 20,
		"2 ^ 3 ^ 2":   512, // right associative
		"-2 ^ 2":      -4,  // unary minus looser than ^
		"2 ^ -3":      0.125,
		"10 % 3":      1,
		"7 / 2":       3.5,
		"-(3 - 5)":    2,
		"1e3 + 0.5":   1000.5,
	}
	for expr, want := range cases {
		got, err := tb.Eval(expr)
		require.NoErrorf(t, err, "eval %q", expr)
		require.InDelta(t, want, got, 1e-9, expr)
	}
}

func TestFunctionsAndConstants(t *testing.T) {
	tb := param.New()
	require.InDelta(t, 1, mustEval(t, tb, "sin(pi/2)"), 1e-9, "sin(pi/2)")
	require.InDelta(t, 2, mustEval(t, tb, "sqrt(2)^2"), 1e-9, "sqrt(2)^2")
	require.InDelta(t, 7, mustEval(t, tb, "max(1, 7, 3)"), 1e-9, "max(1,7,3)")
	require.InDelta(t, 1, mustEval(t, tb, "min(1, 7, 3)"), 1e-9, "min(1,7,3)")
	require.InDelta(t, 5, mustEval(t, tb, "hypot(3, 4)"), 1e-9, "hypot(3,4)")
	require.InDelta(t, 10, mustEval(t, tb, "clamp(12, 0, 10)"), 1e-9, "clamp(12,0,10)")
	require.InDelta(t, 180, mustEval(t, tb, "deg(pi)"), 1e-9, "deg(pi)")
	require.InDelta(t, math.Pi/4, mustEval(t, tb, "atan2(1, 1)"), 1e-9, "atan2(1,1)")
}

func TestCustomFunc(t *testing.T) {
	tb := param.New()
	tb.SetFunc("double", func(a []float64) (float64, error) { return a[0] * 2, nil })
	require.InDelta(t, 42, mustEval(t, tb, "double(21)"), 1e-9, "double(21)")
}

func TestCycleDetection(t *testing.T) {
	tb := param.New()
	require.NoError(t, tb.Set("a", "b + 1"))
	require.NoError(t, tb.Set("b", "a + 1"))
	_, err := tb.Get("a")
	require.ErrorIs(t, err, param.ErrCycle)
}

func TestSelfReferenceCycle(t *testing.T) {
	tb := param.New()
	require.NoError(t, tb.Set("a", "a + 1"))
	_, err := tb.Get("a")
	require.ErrorIs(t, err, param.ErrCycle)
}

func TestUndefinedReference(t *testing.T) {
	tb := param.New()
	require.NoError(t, tb.Set("a", "missing + 1"))
	_, err := tb.Get("a")
	require.ErrorIs(t, err, param.ErrUndefined)
}

func TestDivisionByZero(t *testing.T) {
	tb := param.New()
	_, err := tb.Eval("1 / 0")
	require.Error(t, err, "expected division by zero error")
}

func TestSyntaxError(t *testing.T) {
	tb := param.New()
	err := tb.Set("a", "2 +")
	var pe *param.ParseError
	require.ErrorAs(t, err, &pe)
}

func TestInvalidName(t *testing.T) {
	tb := param.New()
	err := tb.Set("2bad", "1")
	require.ErrorIs(t, err, param.ErrInvalidName)
}

func TestDependencies(t *testing.T) {
	tb := param.New()
	require.NoError(t, tb.Set("a", "1"))
	require.NoError(t, tb.Set("b", "2"))
	require.NoError(t, tb.Set("c", "a + b * pi")) // pi is a constant, excluded
	deps, err := tb.Dependencies("c")
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, deps)
}

func TestValidate(t *testing.T) {
	tb := param.New()
	require.NoError(t, tb.Set("a", "b"))
	require.NoError(t, tb.Set("b", "c")) // c undefined
	require.Error(t, tb.Validate(), "expected validation error for undefined reference")

	tb2 := param.New()
	require.NoError(t, tb2.Set("a", "1"))
	require.NoError(t, tb2.Set("b", "a + 1"))
	require.NoError(t, tb2.Validate())
}

func TestParamShadowsConstant(t *testing.T) {
	tb := param.New()
	require.NoError(t, tb.Set("pi", "3")) // parameter named pi shadows the constant
	require.InDelta(t, 3, tb.MustGet("pi"), 1e-9, "pi param")
	require.InDelta(t, 6, mustEval(t, tb, "pi * 2"), 1e-9, "uses shadow")
}

func TestDeleteAndOrder(t *testing.T) {
	tb := param.New()
	require.NoError(t, tb.Set("a", "1"))
	require.NoError(t, tb.Set("b", "2"))
	require.NoError(t, tb.Set("c", "3"))
	tb.Delete("b")
	require.Equal(t, []string{"a", "c"}, tb.Names(), "Names after delete")
	require.False(t, tb.Has("b"), "b should be deleted")
}

func TestJSONRoundTrip(t *testing.T) {
	tb := param.New()
	require.NoError(t, tb.Set("height", "60"))
	require.NoError(t, tb.Set("width", "height * 1.5"))
	require.NoError(t, tb.Set("corner_r", "min(width, height) / 8"))

	data, err := json.Marshal(tb)
	require.NoError(t, err)

	var tb2 param.Table
	require.NoError(t, json.Unmarshal(data, &tb2))

	require.Equal(t, []string{"height", "width", "corner_r"}, tb2.Names(), "order preserved")
	require.InDelta(t, 90, tb2.MustGet("width"), 1e-9, "reloaded width")
	require.InDelta(t, 7.5, tb2.MustGet("corner_r"), 1e-9, "reloaded corner_r")
}

// --- helpers ---------------------------------------------------------------

func mustEval(t *testing.T, tb *param.Table, expr string) float64 {
	t.Helper()
	v, err := tb.Eval(expr)
	require.NoErrorf(t, err, "eval %q", expr)
	return v
}
