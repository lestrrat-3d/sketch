package geom

import (
	"math"
	"testing"
)

func approx(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func TestLineLength(t *testing.T) {
	l := NewLine(NewPoint(0, 0), NewPoint(3, 4))
	approx(t, "length", l.Length(), 5)
}

func TestArcMetrics(t *testing.T) {
	a := NewArc(NewPoint(0, 0), NewPoint(5, 0), NewPoint(0, 5))
	approx(t, "radius", a.Radius(), 5)
	approx(t, "start angle", a.StartAngle(), 0)
	approx(t, "end angle", a.EndAngle(), math.Pi/2)
	approx(t, "sweep", a.Sweep(), math.Pi/2)
}

func TestSharedPointIdentity(t *testing.T) {
	v := NewPoint(1, 1)
	l1 := NewLine(NewPoint(0, 0), v)
	l2 := NewLine(v, NewPoint(2, 2))
	if l1.End != l2.Start {
		t.Error("shared vertex should be the same *Point")
	}
}

func TestFullCircleSweep(t *testing.T) {
	// start == end means a full turn, reported as 2π rather than 0.
	a := NewArc(NewPoint(0, 0), NewPoint(1, 0), NewPoint(1, 0))
	approx(t, "full sweep", a.Sweep(), 2*math.Pi)
}
