package sketchtest

import (
	"fmt"
	"math"
	"testing"

	"github.com/lestrrat-3d/r3"
	"github.com/lestrrat-3d/sketch"
)

// Measures fails tb unless got is within the configured absolute or relative
// tolerance of want. Exactly one tolerance form is required; a later option
// replaces an earlier one.
func Measures(tb testing.TB, what string, got, want float64, opts ...Option) {
	tb.Helper()

	cfg := resolveOptions(opts)
	if cfg.nilOption {
		tb.Fatalf("%s: a nil Option was passed", what)
		return
	}
	if !cfg.set {
		tb.Fatalf("%s: pass Within or WithinRel", what)
		return
	}
	if math.IsNaN(cfg.value) || math.IsInf(cfg.value, 0) || cfg.value < 0 {
		tb.Fatalf("%s: tolerance must be finite and non-negative, got %g", what, cfg.value)
		return
	}
	if math.IsNaN(got) || math.IsInf(got, 0) || math.IsNaN(want) || math.IsInf(want, 0) {
		tb.Fatalf("%s: got and want must be finite, got %g and want %g", what, got, want)
		return
	}

	tolerance := cfg.value
	kind := "absolute"
	if cfg.relative {
		tolerance *= math.Abs(want)
		kind = "relative"
	}
	difference := math.Abs(got - want)
	if difference <= tolerance {
		return
	}
	tb.Fatalf("%s: got %g, want %g, difference %g exceeds %s tolerance %g",
		what, got, want, difference, kind, tolerance)
}

// MeasuresPoint fails tb unless point's local X and Y coordinates measure x
// and y with the configured tolerance. point MUST NOT be nil.
func MeasuresPoint(tb testing.TB, point *sketch.Point, x, y float64, opts ...Option) {
	tb.Helper()

	if point == nil {
		tb.Fatalf("sketchtest.MeasuresPoint: point must not be nil")
		return
	}
	name := pointName(point)
	Measures(tb, name+" x", point.X(), x, opts...)
	Measures(tb, name+" y", point.Y(), y, opts...)
}

// MeasuresWorldPoint fails tb unless point's world X, Y, and Z coordinates
// measure want with the configured tolerance. point MUST NOT be nil.
func MeasuresWorldPoint(tb testing.TB, point *sketch.Point, want r3.Vec, opts ...Option) {
	tb.Helper()

	if point == nil {
		tb.Fatalf("sketchtest.MeasuresWorldPoint: point must not be nil")
		return
	}
	if err := point.WorldErr(); err != nil {
		tb.Fatalf("%s world coordinates: reading them failed: %v", pointName(point), err)
		return
	}
	got := point.World()
	name := pointName(point) + " world"
	Measures(tb, name+" x", got.X, want.X, opts...)
	Measures(tb, name+" y", got.Y, want.Y, opts...)
	Measures(tb, name+" z", got.Z, want.Z, opts...)
}

// MeasuresProfileArea fails tb unless profile's area measures want with the
// configured tolerance. profile MUST NOT be nil.
func MeasuresProfileArea(tb testing.TB, profile *sketch.Profile, want float64, opts ...Option) {
	tb.Helper()

	if profile == nil {
		tb.Fatalf("sketchtest.MeasuresProfileArea: profile must not be nil")
		return
	}
	Measures(tb, "profile area", profile.Area, want, opts...)
}

// Satisfies fails tb unless every residual of a committed constraint measures
// zero with the configured tolerance. constraint MUST come from
// Sketch.Constraints and MUST NOT be nil.
func Satisfies(tb testing.TB, constraint sketch.Constraint, opts ...Option) {
	tb.Helper()

	if constraint == nil {
		tb.Fatalf("sketchtest.Satisfies: constraint must not be nil")
		return
	}
	residuals := sketch.ConstraintResiduals(constraint)
	if len(residuals) == 0 {
		tb.Fatalf("%s: constraint has no residuals; pass a committed constraint from Sketch.Constraints",
			constraintName(constraint))
		return
	}
	for i, residual := range residuals {
		Measures(tb, fmt.Sprintf("%s residual[%d]", constraintName(constraint), i), residual, 0, opts...)
	}
}
