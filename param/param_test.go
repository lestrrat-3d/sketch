package param

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
)

func approx(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func TestLiteralAndExpression(t *testing.T) {
	tb := New()
	must(t, tb.Set("height", "60"))
	must(t, tb.Set("width", "height * 1.5"))
	must(t, tb.Set("area", "width * height"))

	approx(t, "height", tb.MustGet("height"), 60)
	approx(t, "width", tb.MustGet("width"), 90)
	approx(t, "area", tb.MustGet("area"), 5400)
}

func TestParametricUpdate(t *testing.T) {
	tb := New()
	must(t, tb.Set("height", "60"))
	must(t, tb.Set("width", "height * 1.5"))
	must(t, tb.Set("area", "width * height"))
	approx(t, "area", tb.MustGet("area"), 5400)

	must(t, tb.Set("height", "40")) // edit propagates downstream
	approx(t, "width after edit", tb.MustGet("width"), 60)
	approx(t, "area after edit", tb.MustGet("area"), 2400)
}

func TestForwardReference(t *testing.T) {
	tb := New()
	must(t, tb.Set("a", "b + 1")) // b defined later
	must(t, tb.SetValue("b", 2))
	approx(t, "a", tb.MustGet("a"), 3)
}

func TestPrecedenceAndAssociativity(t *testing.T) {
	tb := New()
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
		if err != nil {
			t.Errorf("%q: %v", expr, err)
			continue
		}
		approx(t, expr, got, want)
	}
}

func TestFunctionsAndConstants(t *testing.T) {
	tb := New()
	approx(t, "sin(pi/2)", mustEval(t, tb, "sin(pi/2)"), 1)
	approx(t, "sqrt(2)^2", mustEval(t, tb, "sqrt(2)^2"), 2)
	approx(t, "max(1,7,3)", mustEval(t, tb, "max(1, 7, 3)"), 7)
	approx(t, "min(1,7,3)", mustEval(t, tb, "min(1, 7, 3)"), 1)
	approx(t, "hypot(3,4)", mustEval(t, tb, "hypot(3, 4)"), 5)
	approx(t, "clamp(12,0,10)", mustEval(t, tb, "clamp(12, 0, 10)"), 10)
	approx(t, "deg(pi)", mustEval(t, tb, "deg(pi)"), 180)
	approx(t, "atan2(1,1)", mustEval(t, tb, "atan2(1, 1)"), math.Pi/4)
}

func TestCustomFunc(t *testing.T) {
	tb := New()
	tb.SetFunc("double", func(a []float64) (float64, error) { return a[0] * 2, nil })
	approx(t, "double(21)", mustEval(t, tb, "double(21)"), 42)
}

func TestCycleDetection(t *testing.T) {
	tb := New()
	must(t, tb.Set("a", "b + 1"))
	must(t, tb.Set("b", "a + 1"))
	_, err := tb.Get("a")
	if !errors.Is(err, ErrCycle) {
		t.Fatalf("expected ErrCycle, got %v", err)
	}
}

func TestSelfReferenceCycle(t *testing.T) {
	tb := New()
	must(t, tb.Set("a", "a + 1"))
	if _, err := tb.Get("a"); !errors.Is(err, ErrCycle) {
		t.Fatalf("expected ErrCycle, got %v", err)
	}
}

func TestUndefinedReference(t *testing.T) {
	tb := New()
	must(t, tb.Set("a", "missing + 1"))
	if _, err := tb.Get("a"); !errors.Is(err, ErrUndefined) {
		t.Fatalf("expected ErrUndefined, got %v", err)
	}
}

func TestDivisionByZero(t *testing.T) {
	tb := New()
	if _, err := tb.Eval("1 / 0"); err == nil {
		t.Fatal("expected division by zero error")
	}
}

func TestSyntaxError(t *testing.T) {
	tb := New()
	err := tb.Set("a", "2 +")
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %v", err)
	}
}

func TestInvalidName(t *testing.T) {
	tb := New()
	if err := tb.Set("2bad", "1"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("expected ErrInvalidName, got %v", err)
	}
}

func TestDependencies(t *testing.T) {
	tb := New()
	must(t, tb.Set("a", "1"))
	must(t, tb.Set("b", "2"))
	must(t, tb.Set("c", "a + b * pi")) // pi is a constant, excluded
	deps, err := tb.Dependencies("c")
	must(t, err)
	if len(deps) != 2 || deps[0] != "a" || deps[1] != "b" {
		t.Fatalf("deps = %v, want [a b]", deps)
	}
}

func TestValidate(t *testing.T) {
	tb := New()
	must(t, tb.Set("a", "b"))
	must(t, tb.Set("b", "c")) // c undefined
	if err := tb.Validate(); err == nil {
		t.Fatal("expected validation error for undefined reference")
	}

	tb2 := New()
	must(t, tb2.Set("a", "1"))
	must(t, tb2.Set("b", "a + 1"))
	if err := tb2.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestParamShadowsConstant(t *testing.T) {
	tb := New()
	must(t, tb.Set("pi", "3")) // parameter named pi shadows the constant
	approx(t, "pi param", tb.MustGet("pi"), 3)
	approx(t, "uses shadow", mustEval(t, tb, "pi * 2"), 6)
}

func TestDeleteAndOrder(t *testing.T) {
	tb := New()
	must(t, tb.Set("a", "1"))
	must(t, tb.Set("b", "2"))
	must(t, tb.Set("c", "3"))
	tb.Delete("b")
	got := tb.Names()
	if len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Fatalf("Names after delete = %v, want [a c]", got)
	}
	if tb.Has("b") {
		t.Fatal("b should be deleted")
	}
}

func TestJSONRoundTrip(t *testing.T) {
	tb := New()
	must(t, tb.Set("height", "60"))
	must(t, tb.Set("width", "height * 1.5"))
	must(t, tb.Set("corner_r", "min(width, height) / 8"))

	data, err := json.Marshal(tb)
	must(t, err)

	var tb2 Table
	must(t, json.Unmarshal(data, &tb2))

	if got := tb2.Names(); len(got) != 3 || got[0] != "height" || got[2] != "corner_r" {
		t.Fatalf("order not preserved: %v", got)
	}
	approx(t, "reloaded width", tb2.MustGet("width"), 90)
	approx(t, "reloaded corner_r", tb2.MustGet("corner_r"), 7.5)
}

// --- helpers ---------------------------------------------------------------

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func mustEval(t *testing.T, tb *Table, expr string) float64 {
	t.Helper()
	v, err := tb.Eval(expr)
	if err != nil {
		t.Fatalf("eval %q: %v", expr, err)
	}
	return v
}
