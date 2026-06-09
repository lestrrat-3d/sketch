package units_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/units"
	"github.com/stretchr/testify/require"
)

func TestConversionLength(t *testing.T) {
	in, err := units.Millimeters(100).In(units.Inch)
	require.NoError(t, err)
	require.InDelta(t, 100/25.4, in, 1e-9, "100mm in inch")

	m, err := units.Meters(1).In(units.Millimeter)
	require.NoError(t, err)
	require.InDelta(t, 1000, m, 1e-9, "1m in mm")

	ft, err := units.Inches(12).In(units.Foot)
	require.NoError(t, err)
	require.InDelta(t, 1, ft, 1e-9, "12in in ft")

	require.InDelta(t, 0.0254, units.Thous(1).Base(), 1e-9, "1 thou base")
}

func TestConversionAngle(t *testing.T) {
	r, err := units.Degrees(180).In(units.Radian)
	require.NoError(t, err)
	require.InDelta(t, math.Pi, r, 1e-9, "180deg in rad")

	d, err := units.Radians(math.Pi / 2).In(units.Degree)
	require.NoError(t, err)
	require.InDelta(t, 90, d, 1e-9, "pi/2 in deg")
}

func TestIncompatibleKinds(t *testing.T) {
	_, err := units.Millimeters(1).In(units.Degree)
	require.ErrorIs(t, err, units.ErrIncompatible)

	_, err = units.Millimeters(1).Add(units.Degrees(1))
	require.ErrorIs(t, err, units.ErrIncompatible, "add across kinds")
}

func TestArithmetic(t *testing.T) {
	sum, err := units.Millimeters(50).Add(units.Centimeters(5)) // 50mm + 50mm
	require.NoError(t, err)
	require.InDelta(t, 100, sum.Base(), 1e-9, "sum base")
	require.Equal(t, units.Millimeter, sum.Unit(), "sum keeps left-hand unit")

	diff, err := units.Meters(1).Sub(units.Millimeters(250))
	require.NoError(t, err)
	require.InDelta(t, 750, diff.Base(), 1e-9, "diff base")
	require.InDelta(t, 0.75, diff.Mag(), 1e-9, "diff mag (m)")

	require.InDelta(t, 30, units.Millimeters(10).Scale(3).Base(), 1e-9, "scale")
}

func TestFromBaseAndKind(t *testing.T) {
	v := units.FromBase(1000, units.Meter)
	require.InDelta(t, 1, v.Mag(), 1e-9, "from base mag")
	require.Equal(t, units.Length, v.Kind(), "kind")
	require.InDelta(t, 1000, v.Base(), 1e-9, "from base base")
}

func TestString(t *testing.T) {
	require.Equal(t, "100 mm", units.Millimeters(100).String())
	require.Equal(t, "1.5", units.Scalar(1.5).String())
}

func TestEqual(t *testing.T) {
	require.True(t, units.Meters(1).Equal(units.Millimeters(1000), 1e-9), "1m should equal 1000mm")
	require.False(t, units.Millimeters(1).Equal(units.Degrees(1), 1e-9), "length must never equal angle")
}

func TestSystem(t *testing.T) {
	m := units.Metric()
	require.Equal(t, units.Millimeter, m.UnitFor(units.Length), "metric length default")
	require.Equal(t, units.Degree, m.UnitFor(units.Angle), "metric angle default")

	imp := units.Imperial()
	require.InDelta(t, 1, imp.LengthFromBase(25.4).Mag(), 1e-9, "imperial length-from-base") // 25.4mm = 1in
	require.InDelta(t, 180, units.Metric().AngleFromBase(math.Pi).Mag(), 1e-9, "metric angle-from-base")
	require.InDelta(t, 2000, units.Metric().In(units.Meters(2)), 1e-9, "system In") // displayed in mm
}

func TestLookupAndDefine(t *testing.T) {
	u, ok := units.Lookup("mm")
	require.True(t, ok, "lookup mm")
	require.Equal(t, units.Millimeter, u)

	yard := units.Define("yd", units.Length, 914.4)
	u, ok = units.Lookup("yd")
	require.True(t, ok, "lookup yd")
	require.Equal(t, yard, u)

	ft, err := units.New(1, yard).In(units.Foot)
	require.NoError(t, err)
	require.InDelta(t, 3, ft, 1e-9, "1 yd in ft")
}
