package units

import (
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

func TestConversionLength(t *testing.T) {
	w := Millimeters(100)
	in, err := w.In(Inch)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "100mm in inch", in, 100/25.4)

	m, _ := Meters(1).In(Millimeter)
	approx(t, "1m in mm", m, 1000)

	ft, _ := Inches(12).In(Foot)
	approx(t, "12in in ft", ft, 1)

	approx(t, "1 thou base", Thous(1).Base(), 0.0254)
}

func TestConversionAngle(t *testing.T) {
	r, err := Degrees(180).In(Radian)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "180deg in rad", r, math.Pi)
	d, _ := Radians(math.Pi / 2).In(Degree)
	approx(t, "pi/2 in deg", d, 90)
}

func TestIncompatibleKinds(t *testing.T) {
	if _, err := Millimeters(1).In(Degree); !errors.Is(err, ErrIncompatible) {
		t.Fatalf("expected ErrIncompatible, got %v", err)
	}
	if _, err := Millimeters(1).Add(Degrees(1)); !errors.Is(err, ErrIncompatible) {
		t.Fatalf("expected ErrIncompatible on add, got %v", err)
	}
}

func TestArithmetic(t *testing.T) {
	sum, err := Millimeters(50).Add(Centimeters(5)) // 50mm + 50mm
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "sum base", sum.Base(), 100)
	if sum.Unit() != Millimeter {
		t.Errorf("sum unit = %v, want mm", sum.Unit())
	}

	diff, _ := Meters(1).Sub(Millimeters(250))
	approx(t, "diff base", diff.Base(), 750)
	approx(t, "diff mag (m)", diff.Mag(), 0.75)

	approx(t, "scale", Millimeters(10).Scale(3).Base(), 30)
}

func TestFromBaseAndKind(t *testing.T) {
	v := FromBase(1000, Meter)
	approx(t, "from base mag", v.Mag(), 1)
	if v.Kind() != Length {
		t.Errorf("kind = %v, want length", v.Kind())
	}
	approx(t, "from base base", v.Base(), 1000)
}

func TestString(t *testing.T) {
	if got := Millimeters(100).String(); got != "100 mm" {
		t.Errorf("String = %q, want %q", got, "100 mm")
	}
	if got := Scalar(1.5).String(); got != "1.5" {
		t.Errorf("scalar String = %q, want %q", got, "1.5")
	}
}

func TestEqual(t *testing.T) {
	if !Meters(1).Equal(Millimeters(1000), 1e-9) {
		t.Error("1m should equal 1000mm")
	}
	if Millimeters(1).Equal(Degrees(1), 1e-9) {
		t.Error("length must never equal angle")
	}
}

func TestSystem(t *testing.T) {
	m := Metric()
	if m.UnitFor(Length) != Millimeter || m.UnitFor(Angle) != Degree {
		t.Error("metric defaults wrong")
	}
	imp := Imperial()
	approx(t, "imperial length-from-base", imp.LengthFromBase(25.4).Mag(), 1) // 25.4mm = 1in
	approx(t, "metric angle-from-base", Metric().AngleFromBase(math.Pi).Mag(), 180)
	approx(t, "system In", Metric().In(Meters(2)), 2000) // displayed in mm
}

func TestLookupAndDefine(t *testing.T) {
	if u, ok := Lookup("mm"); !ok || u != Millimeter {
		t.Error("lookup mm failed")
	}
	yard := Define("yd", Length, 914.4)
	if u, ok := Lookup("yd"); !ok || u != yard {
		t.Error("define/lookup yard failed")
	}
	approx(t, "1 yd in ft", func() float64 { v, _ := New(1, yard).In(Foot); return v }(), 3)
}
