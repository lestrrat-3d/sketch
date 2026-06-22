package sketch

import (
	"fmt"
	"math"
	"strings"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/lestrrat-3d/sketch/units"
)

// DXF renders the sketch as a minimal AutoCAD R12 ASCII DXF document
// containing LINE, CIRCLE, ARC, ELLIPSE and SPLINE entities (ELLIPSE and
// SPLINE are formally R13+, but widely accepted). It is intended for
// interchange with CAD tools; only geometry is exported, not constraints.
//
// Lengths are emitted in the sketch's display length unit (see
// [Sketch.SetUnits]) — a metric (millimetre) sketch is unchanged, while an
// imperial sketch exports inch-valued coordinates — and the HEADER carries the
// matching $INSUNITS / $MEASUREMENT so a CAD importer reads the drawing at the
// correct scale instead of guessing. Angles (degrees), the ellipse axis ratio
// and spline knots are unitless and emitted as-is. The drawing extents
// ($EXTMIN/$EXTMAX) are written when the sketch has geometry.
func (s *Sketch) DXF() (string, error) {
	var sb strings.Builder

	// Minimal but valid R12 header.
	pair := func(code int, value string) {
		fmt.Fprintf(&sb, "%d\n%s\n", code, value)
	}
	pairf := func(code int, value float64) {
		fmt.Fprintf(&sb, "%d\n%s\n", code, dxff(value))
	}
	// lengthMag converts a base-unit (millimetre) length into the sketch's
	// display length unit through the units library — never by relabelling a
	// magnitude. pairL emits a length-valued group code so converted; angles,
	// ratios, knots and eccentric params stay raw via pairf.
	lengthMag := func(base float64) float64 {
		return units.FromBase(base, s.sys.Length).Mag()
	}
	pairL := func(code int, base float64) { pairf(code, lengthMag(base)) }

	pair(0, "SECTION")
	pair(2, "HEADER")
	pair(9, "$ACADVER")
	pair(1, "AC1009")
	pair(9, "$INSUNITS")
	pair(70, fmt.Sprintf("%d", dxfInsUnits(s.sys.Length)))
	pair(9, "$MEASUREMENT")
	pair(70, fmt.Sprintf("%d", dxfMeasurement(s.sys.Length)))
	if b, ok := s.bounds(); ok {
		pair(9, "$EXTMIN")
		pairL(10, b.minX)
		pairL(20, b.minY)
		pairf(30, 0)
		pair(9, "$EXTMAX")
		pairL(10, b.maxX)
		pairL(20, b.maxY)
		pairf(30, 0)
	}
	pair(0, "ENDSEC")

	pair(0, "SECTION")
	pair(2, "ENTITIES")

	for _, e := range s.ents {
		layer := "0"
		switch {
		case e.IsReference():
			layer = "REFERENCE"
		case e.IsConstruction():
			layer = "CONSTRUCTION"
		}
		switch t := e.(type) {
		case *Line:
			pair(0, "LINE")
			pair(8, layer)
			pairL(10, t.Start.x())
			pairL(20, t.Start.y())
			pairf(30, 0)
			pairL(11, t.End.x())
			pairL(21, t.End.y())
			pairf(31, 0)
		case *Circle:
			pair(0, "CIRCLE")
			pair(8, layer)
			pairL(10, t.Center.x())
			pairL(20, t.Center.y())
			pairf(30, 0)
			pairL(40, t.r())
		case *Arc:
			pair(0, "ARC")
			pair(8, layer)
			pairL(10, t.Center.x())
			pairL(20, t.Center.y())
			pairf(30, 0)
			pairL(40, t.R())
			// DXF arc angles are degrees, measured counter-clockwise.
			pairf(50, deg(t.StartAngle()))
			pairf(51, deg(t.EndAngle()))
		case *Ellipse:
			// ELLIPSE is an R13+ entity; most modern tools accept it in this
			// otherwise-R12 stream. Codes: 11/21 = major-axis endpoint
			// relative to the center, 40 = minor/major ratio (must be ≤ 1, so
			// pick the longer semi-axis as major), 41/42 = full sweep.
			major, minor, axis := t.rx(), t.ry(), t.rot()
			if minor > major {
				major, minor = minor, major
				axis += math.Pi / 2
			}
			pair(0, "ELLIPSE")
			pair(8, layer)
			pairL(10, t.Center.x())
			pairL(20, t.Center.y())
			pairf(30, 0)
			ratio := 1.0
			if major > 0 {
				ratio = minor / major
			}
			pairL(11, major*math.Cos(axis))
			pairL(21, major*math.Sin(axis))
			pairf(31, 0)
			pairf(40, ratio)
			pairf(41, 0)
			pairf(42, 2*math.Pi)
		case *EllipticalArc:
			// Like ELLIPSE above, but 41/42 carry the real eccentric-angle sweep.
			// DXF measures the parameter from the major-axis endpoint (11/21), so
			// when ry is the longer semi-axis the axis is rotated +90° and the
			// start parameter shifts by −90° to keep the same point.
			major, minor, axis := t.rx(), t.ry(), t.rot()
			startP, sweep := t.StartParam(), t.Sweep()
			if minor > major {
				major, minor = minor, major
				axis += math.Pi / 2
				startP -= math.Pi / 2
			}
			pair(0, "ELLIPSE")
			pair(8, layer)
			pairL(10, t.Center.x())
			pairL(20, t.Center.y())
			pairf(30, 0)
			ratio := 1.0
			if major > 0 {
				ratio = minor / major
			}
			pairL(11, major*math.Cos(axis))
			pairL(21, major*math.Sin(axis))
			pairf(31, 0)
			pairf(40, ratio)
			pairf(41, startP)
			pairf(42, startP+sweep)
		case *Spline:
			// SPLINE is an R13+ entity, like ELLIPSE above. Flags (70): 8 =
			// planar. Degree 3, clamped uniform knots, then the control
			// points.
			n := len(t.Control)
			knots := geom.ClampedKnots(n)
			pair(0, "SPLINE")
			pair(8, layer)
			pair(70, "8")
			pair(71, "3")
			pair(72, fmt.Sprintf("%d", len(knots)))
			pair(73, fmt.Sprintf("%d", n))
			pair(74, "0")
			for _, k := range knots {
				pairf(40, k)
			}
			for _, c := range t.Control {
				pairL(10, c.x())
				pairL(20, c.y())
				pairf(30, 0)
			}
		case *ClosedSpline:
			// No periodic SPLINE form is honored uniformly across readers; emit a
			// closed LWPOLYLINE of the sampled ring (the same loop SVG/PNG draw).
			pts := t.Polyline(64)
			if len(pts) > 1 {
				pts = pts[:len(pts)-1] // drop the duplicate closing vertex; flag 70=1 wraps
			}
			pair(0, "LWPOLYLINE")
			pair(8, layer)
			pair(90, fmt.Sprintf("%d", len(pts)))
			pair(70, "1") // closed
			for _, p := range pts {
				pairL(10, p[0])
				pairL(20, p[1])
			}
		case *FitSpline:
			// The fit spline's control points are a derived interpolation artifact,
			// not clamped-uniform B-spline controls, so emit the sampled
			// interpolating curve as an OPEN LWPOLYLINE rather than a native SPLINE.
			pts := t.Polyline(64)
			pair(0, "LWPOLYLINE")
			pair(8, layer)
			pair(90, fmt.Sprintf("%d", len(pts)))
			pair(70, "0") // open
			for _, p := range pts {
				pairL(10, p[0])
				pairL(20, p[1])
			}
		}
	}

	pair(0, "ENDSEC")
	pair(0, "EOF")
	return sb.String(), nil
}

func deg(rad float64) float64 {
	d := math.Mod(rad*180/math.Pi, 360)
	if d < 0 {
		d += 360
	}
	return d
}

func dxff(v float64) string { return trimFloat(v, 6) }

// dxfInsUnits maps a length unit to its AutoCAD $INSUNITS code so an importer
// scales the drawing correctly. An unrecognised unit is reported unitless (0).
func dxfInsUnits(u units.Unit) int {
	switch u.Symbol() {
	case "mm":
		return 4
	case "cm":
		return 5
	case "m":
		return 6
	case "in":
		return 1
	case "ft":
		return 2
	default:
		return 0
	}
}

// dxfMeasurement maps a length unit to the $MEASUREMENT flag (1 metric, 0
// imperial) that selects the linetype/hatch tables a reader loads. Anything
// other than the imperial units defaults to metric.
func dxfMeasurement(u units.Unit) int {
	switch u.Symbol() {
	case "in", "ft":
		return 0
	default:
		return 1
	}
}
