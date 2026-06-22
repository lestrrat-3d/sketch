package sketch

import (
	"fmt"
	"math"
	"strings"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/lestrrat-3d/sketch/space"
	"github.com/lestrrat-3d/sketch/units"
	"github.com/lestrrat-go/option/v3"
)

// DXFOption configures [Sketch.DXF]. Construct values with the With… functions.
type DXFOption interface {
	option.Interface
	dxfOption()
}

type dxfOption struct{ option.Interface }

func (dxfOption) dxfOption() {}

type identWorldSpace struct{}

// WithWorldSpace places the exported geometry in 3D world coordinates using the
// sketch's construction-plane frame, instead of the default plane-local 2D
// (z = 0). LINE/SPLINE/ELLIPSE carry true world coordinates; CIRCLE/ARC and the
// sampled (closed/fit) splines carry the plane's extrusion direction and are
// expressed in the entity's object coordinate system (OCS), so a sketch on a
// tilted or offset plane imports at its real 3D placement. A plane-XY sketch is
// unchanged apart from the now-explicit +Z extrusion direction.
func WithWorldSpace(v bool) DXFOption {
	return dxfOption{option.New(identWorldSpace{}, v)}
}

type dxfConfig struct{ worldSpace bool }

func defaultDXFConfig() dxfConfig { return dxfConfig{} }

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
//
// By default geometry is emitted in plane-local 2D (z = 0). Pass
// [WithWorldSpace](true) to place it in 3D world coordinates via the sketch's
// construction-plane frame.
func (s *Sketch) DXF(opts ...DXFOption) (string, error) {
	cfg := defaultDXFConfig()
	for _, o := range opts {
		switch o.Ident().(type) {
		case identWorldSpace:
			cfg.worldSpace = option.MustGet[bool](o)
		}
	}

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

	// World-space placement: resolve the plane frame and the OCS axes the
	// arbitrary-axis algorithm derives from its normal, once. In local mode
	// these stay zero and every put… helper takes its 2D branch.
	var frame space.Frame
	var ax, ay, nrm space.Vec3
	if cfg.worldSpace {
		f, err := s.plane().Frame()
		if err != nil {
			return "", err
		}
		frame = f
		nrm = frame.N()
		ax, ay = arbitraryAxis(nrm)
	}
	// putWCS emits a plane-local point as a world coordinate triple (codes
	// c10 / c10+10 / c10+20); local mode is the bare (x, y, 0).
	putWCS := func(c10 int, lx, ly float64) {
		if cfg.worldSpace {
			w := frame.ToWorldUV(lx, ly)
			pairL(c10, w.X)
			pairL(c10+10, w.Y)
			pairL(c10+20, w.Z)
		} else {
			pairL(c10, lx)
			pairL(c10+10, ly)
			pairf(c10+20, 0)
		}
	}
	// putWCSDir emits a plane-local DIRECTION (no origin translation) as a world
	// vector — the ellipse major-axis endpoint relative to its center.
	putWCSDir := func(c10 int, lx, ly float64) {
		if cfg.worldSpace {
			d := frame.ToWorldUV(lx, ly).Sub(frame.Origin())
			pairL(c10, d.X)
			pairL(c10+10, d.Y)
			pairL(c10+20, d.Z)
		} else {
			pairL(c10, lx)
			pairL(c10+10, ly)
			pairf(c10+20, 0)
		}
	}
	// putOCS emits a plane-local point in the entity's object coordinate system
	// (CIRCLE/ARC), projecting the world point onto the OCS axes.
	putOCS := func(c10 int, lx, ly float64) {
		if cfg.worldSpace {
			w := frame.ToWorldUV(lx, ly)
			pairL(c10, w.Dot(ax))
			pairL(c10+10, w.Dot(ay))
			pairL(c10+20, w.Dot(nrm))
		} else {
			pairL(c10, lx)
			pairL(c10+10, ly)
			pairf(c10+20, 0)
		}
	}
	// putLW emits an LWPOLYLINE 2D vertex (codes 10/20). World mode projects the
	// world point onto the OCS axes; the shared elevation is emitted once via
	// group 38, the extrusion via 210/220/230.
	putLW := func(lx, ly float64) {
		if cfg.worldSpace {
			w := frame.ToWorldUV(lx, ly)
			pairL(10, w.Dot(ax))
			pairL(20, w.Dot(ay))
		} else {
			pairL(10, lx)
			pairL(20, ly)
		}
	}
	// extrusion emits the 210/220/230 extrusion direction (a unit vector, so
	// unitless) in world mode; a no-op in local mode (the implied +Z default).
	extrusion := func() {
		if cfg.worldSpace {
			pairf(210, nrm.X)
			pairf(220, nrm.Y)
			pairf(230, nrm.Z)
		}
	}
	// arcAngles returns the DXF start/end angles (degrees). In world mode they
	// are recomputed in the OCS frame from the world endpoints, since the
	// plane's U/V axes need not coincide with the OCS axes.
	arcAngles := func(t *Arc) (float64, float64) {
		if !cfg.worldSpace {
			return deg(t.StartAngle()), deg(t.EndAngle())
		}
		c := frame.ToWorldUV(t.Center.x(), t.Center.y())
		sp := frame.ToWorldUV(t.Start.x(), t.Start.y()).Sub(c)
		ep := frame.ToWorldUV(t.End.x(), t.End.y()).Sub(c)
		return deg(math.Atan2(sp.Dot(ay), sp.Dot(ax))), deg(math.Atan2(ep.Dot(ay), ep.Dot(ax)))
	}

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
			putWCS(10, t.Start.x(), t.Start.y())
			putWCS(11, t.End.x(), t.End.y())
		case *Circle:
			pair(0, "CIRCLE")
			pair(8, layer)
			putOCS(10, t.Center.x(), t.Center.y())
			pairL(40, t.r())
			extrusion()
		case *Arc:
			pair(0, "ARC")
			pair(8, layer)
			putOCS(10, t.Center.x(), t.Center.y())
			pairL(40, t.R())
			// DXF arc angles are degrees, measured counter-clockwise (in the OCS
			// when placed in world space).
			sa, ea := arcAngles(t)
			pairf(50, sa)
			pairf(51, ea)
			extrusion()
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
			putWCS(10, t.Center.x(), t.Center.y())
			ratio := 1.0
			if major > 0 {
				ratio = minor / major
			}
			putWCSDir(11, major*math.Cos(axis), major*math.Sin(axis))
			pairf(40, ratio)
			pairf(41, 0)
			pairf(42, 2*math.Pi)
			extrusion()
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
			putWCS(10, t.Center.x(), t.Center.y())
			ratio := 1.0
			if major > 0 {
				ratio = minor / major
			}
			putWCSDir(11, major*math.Cos(axis), major*math.Sin(axis))
			pairf(40, ratio)
			pairf(41, startP)
			pairf(42, startP+sweep)
			extrusion()
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
				putWCS(10, c.x(), c.y())
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
			if cfg.worldSpace {
				pairL(38, frame.Origin().Dot(nrm)) // OCS elevation (the loop is planar)
			}
			for _, p := range pts {
				putLW(p[0], p[1])
			}
			extrusion()
		case *FitSpline:
			// The fit spline's control points are a derived interpolation artifact,
			// not clamped-uniform B-spline controls, so emit the sampled
			// interpolating curve as an OPEN LWPOLYLINE rather than a native SPLINE.
			pts := t.Polyline(64)
			pair(0, "LWPOLYLINE")
			pair(8, layer)
			pair(90, fmt.Sprintf("%d", len(pts)))
			pair(70, "0") // open
			if cfg.worldSpace {
				pairL(38, frame.Origin().Dot(nrm)) // OCS elevation (the curve is planar)
			}
			for _, p := range pts {
				putLW(p[0], p[1])
			}
			extrusion()
		case *Conic:
			// A conic is exactly a degree-2 rational Bézier, so emit it as a native
			// (R13+) rational SPLINE: clamped knots [0,0,0,1,1,1], the three control
			// points Start/Apex/End in WCS (ordinary points → the putWCS path), and
			// the rational weights 1, w, 1 (group 41), w = rho/(1−rho). Flags (70):
			// 8 = planar + 4 = rational = 12. A reader honours the weights, so the
			// imported curve is the exact conic, not a sampled polyline.
			w := t.rho() / (1 - t.rho())
			knots := []float64{0, 0, 0, 1, 1, 1}
			weights := []float64{1, w, 1}
			ctrl := []*Point{t.Start, t.Apex, t.End}
			pair(0, "SPLINE")
			pair(8, layer)
			pair(70, "12")
			pair(71, "2")
			pair(72, fmt.Sprintf("%d", len(knots)))
			pair(73, fmt.Sprintf("%d", len(ctrl)))
			pair(74, "0")
			for _, k := range knots {
				pairf(40, k)
			}
			for _, wt := range weights {
				pairf(41, wt)
			}
			for _, c := range ctrl {
				putWCS(10, c.x(), c.y())
			}
		case *NURBS:
			// A general clamped (rational) B-spline maps directly onto a native
			// (R13+) SPLINE: degree (71), the explicit knot vector (40×, count 72),
			// the control points (10/20/30 via the putWCS world-space path, count
			// 73) and — when rational — the per-control weights (41×). Flags (70):
			// 8 = planar, +4 = rational (set only when any weight ≠ 1) so a reader
			// honours the weights and rebuilds the exact curve, not a sampled
			// polyline. Knots and weights are raw (unitless) via pairf.
			flags := 8
			if t.Rational() {
				flags |= 4
			}
			pair(0, "SPLINE")
			pair(8, layer)
			pair(70, fmt.Sprintf("%d", flags))
			pair(71, fmt.Sprintf("%d", t.degree))
			pair(72, fmt.Sprintf("%d", len(t.knots)))
			pair(73, fmt.Sprintf("%d", len(t.Control)))
			pair(74, "0")
			for _, k := range t.knots {
				pairf(40, k)
			}
			if t.Rational() {
				for _, w := range t.weights {
					pairf(41, w)
				}
			}
			for _, c := range t.Control {
				putWCS(10, c.x(), c.y())
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

// arbitraryAxis derives the OCS x/y axes DXF associates with an extrusion
// direction n (assumed unit) via AutoCAD's arbitrary-axis algorithm: it picks a
// reference world axis avoiding near-degeneracy with n, then builds a
// right-handed (ax, ay, n) frame. A CIRCLE/ARC/LWPOLYLINE whose center is given
// in this OCS plus extrusion n round-trips to the correct world placement.
func arbitraryAxis(n space.Vec3) (space.Vec3, space.Vec3) {
	const tol = 1.0 / 64.0
	var a space.Vec3
	if math.Abs(n.X) < tol && math.Abs(n.Y) < tol {
		a = space.NewVec3(0, 1, 0).Cross(n)
	} else {
		a = space.NewVec3(0, 0, 1).Cross(n)
	}
	ax, _ := a.Normalize()
	ay, _ := n.Cross(ax).Normalize()
	return ax, ay
}

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
