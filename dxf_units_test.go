package sketch_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/units"
	"github.com/stretchr/testify/require"
)

// dxfGroup returns the value string of the first occurrence of group code at or
// after the byte index where marker first appears, walking the code/value pair
// stream a DXF document is built from.
func dxfGroup(t *testing.T, dxf, marker, code string) string {
	t.Helper()
	lines := strings.Split(dxf, "\n")
	start := 0
	if marker != "" {
		for i, ln := range lines {
			if ln == marker {
				start = i + 1 // marker is a value line; codes follow it
				break
			}
		}
	}
	for i := start; i+1 < len(lines); i += 2 {
		if lines[i] == code {
			return lines[i+1]
		}
	}
	t.Fatalf("group code %q not found after marker %q", code, marker)
	return ""
}

// A metric sketch emits the $INSUNITS/$MEASUREMENT/$EXT* header and leaves the
// millimetre geometry untouched — coordinates are already in base units.
func TestDXFMetricHeaderAndExtents(t *testing.T) {
	s := sketch.New() // defaults to Metric (mm)
	o := s.AddPoint(5, 5)
	s.AddCircle(o, 3)

	dxf, err := s.DXF()
	require.NoError(t, err)

	require.Contains(t, dxf, "$INSUNITS\n70\n4\n", "millimetre → INSUNITS 4")
	require.Contains(t, dxf, "$MEASUREMENT\n70\n1\n", "metric measurement flag")
	require.Contains(t, dxf, "$EXTMIN", "extents written when geometry present")
	require.Contains(t, dxf, "$EXTMAX")

	// Radius stays 3 mm; extents span the circle.
	require.InDelta(t, 3, mustFloat(t, dxfGroup(t, dxf, "CIRCLE", "40")), 1e-9)
	require.InDelta(t, 2, mustFloat(t, dxfGroup(t, dxf, "$EXTMIN", "10")), 1e-9) // 5-3
	require.InDelta(t, 8, mustFloat(t, dxfGroup(t, dxf, "$EXTMAX", "10")), 1e-9) // 5+3
}

// An imperial sketch reports $INSUNITS 1 (inches) and emits every length-valued
// field converted from base millimetres into inches — angles and the unitless
// fields are unaffected.
func TestDXFImperialLengthsConverted(t *testing.T) {
	s := sketch.New()
	s.SetUnits(units.Imperial()) // inch / degree

	// Author at base millimetres: center 5 in, radius 3 in.
	o := s.AddPoint(127, 127) // 5 in
	s.AddCircle(o, 76.2)      // 3 in

	dxf, err := s.DXF()
	require.NoError(t, err)

	require.Contains(t, dxf, "$INSUNITS\n70\n1\n", "inch → INSUNITS 1")
	require.Contains(t, dxf, "$MEASUREMENT\n70\n0\n", "imperial measurement flag")

	require.InDelta(t, 5, mustFloat(t, dxfGroup(t, dxf, "CIRCLE", "10")), 1e-9, "center x in inches")
	require.InDelta(t, 5, mustFloat(t, dxfGroup(t, dxf, "CIRCLE", "20")), 1e-9, "center y in inches")
	require.InDelta(t, 3, mustFloat(t, dxfGroup(t, dxf, "CIRCLE", "40")), 1e-9, "radius in inches")
	require.InDelta(t, 2, mustFloat(t, dxfGroup(t, dxf, "$EXTMIN", "10")), 1e-9, "extents converted too")
}

// An arc's center/radius convert with the display unit, but its sweep angles
// (codes 50/51) are always degrees regardless of the length unit.
func TestDXFArcAnglesStayDegrees(t *testing.T) {
	s := sketch.New()
	s.SetUnits(units.Imperial())
	o := s.AddPoint(0, 0)
	st := s.AddPoint(25.4, 0) // 1 in to the right → start angle 0°
	en := s.AddPoint(0, 25.4) // 1 in up        → end angle 90°
	s.AddArc(o, st, en)

	dxf, err := s.DXF()
	require.NoError(t, err)

	require.InDelta(t, 1, mustFloat(t, dxfGroup(t, dxf, "ARC", "40")), 1e-9, "radius converted to inches")
	require.InDelta(t, 0, mustFloat(t, dxfGroup(t, dxf, "ARC", "50")), 1e-6, "start angle in degrees")
	require.InDelta(t, 90, mustFloat(t, dxfGroup(t, dxf, "ARC", "51")), 1e-6, "end angle in degrees")
}

func mustFloat(t *testing.T, s string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(s, 64)
	require.NoErrorf(t, err, "parse %q", s)
	return v
}
