package sketch

import (
	"fmt"
	"math"
	"strings"
)

// DXF renders the sketch as a minimal AutoCAD R12 ASCII DXF document
// containing LINE, CIRCLE and ARC entities. It is intended for interchange
// with CAD tools; only geometry is exported, not constraints.
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
		if e.isConstruction() {
			layer = "CONSTRUCTION"
		}
		switch t := e.(type) {
		case *Line:
			pair(0, "LINE")
			pair(8, layer)
			pairf(10, t.A.x())
			pairf(20, t.A.y())
			pairf(30, 0)
			pairf(11, t.B.x())
			pairf(21, t.B.y())
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
