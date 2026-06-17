package sketch

import (
	"fmt"
	"math"
	"strings"

	"github.com/lestrrat-3d/sketch/geom"
)

// DXF renders the sketch as a minimal AutoCAD R12 ASCII DXF document
// containing LINE, CIRCLE, ARC, ELLIPSE and SPLINE entities (ELLIPSE and
// SPLINE are formally R13+, but widely accepted). It is intended for
// interchange with CAD tools; only geometry is exported, not constraints.
func (s *Sketch) DXF() (string, error) {
	var sb strings.Builder

	// Minimal but valid R12 header.
	pair := func(code int, value string) {
		fmt.Fprintf(&sb, "%d\n%s\n", code, value)
	}
	pairf := func(code int, value float64) {
		fmt.Fprintf(&sb, "%d\n%s\n", code, dxff(value))
	}

	pair(0, "SECTION")
	pair(2, "HEADER")
	pair(9, "$ACADVER")
	pair(1, "AC1009")
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
			pairf(10, t.Start.x())
			pairf(20, t.Start.y())
			pairf(30, 0)
			pairf(11, t.End.x())
			pairf(21, t.End.y())
			pairf(31, 0)
		case *Circle:
			pair(0, "CIRCLE")
			pair(8, layer)
			pairf(10, t.Center.x())
			pairf(20, t.Center.y())
			pairf(30, 0)
			pairf(40, t.r())
		case *Arc:
			pair(0, "ARC")
			pair(8, layer)
			pairf(10, t.Center.x())
			pairf(20, t.Center.y())
			pairf(30, 0)
			pairf(40, t.R())
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
			pairf(10, t.Center.x())
			pairf(20, t.Center.y())
			pairf(30, 0)
			ratio := 1.0
			if major > 0 {
				ratio = minor / major
			}
			pairf(11, major*math.Cos(axis))
			pairf(21, major*math.Sin(axis))
			pairf(31, 0)
			pairf(40, ratio)
			pairf(41, 0)
			pairf(42, 2*math.Pi)
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
				pairf(10, c.x())
				pairf(20, c.y())
				pairf(30, 0)
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
