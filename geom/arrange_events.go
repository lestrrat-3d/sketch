package geom

import "math"

// Analytic crossing events between two arrangement sources — the exact
// alternative to the polyline segment-intersection heuristic, for the source
// kinds that have a closed-form intersection (line / circle / arc). The
// arrangement uses these to classify a pair's contact precisely: a transverse
// crossing forces a topology split; a clean TANGENCY is a contact that does NOT
// make the arrangement degenerate (two tangent circles bound two clean disk
// regions); a coincident OVERLAP, or any case the analytic test cannot certify,
// is reported so the arrangement flags it degenerate. Curve pairs with no
// closed form (anything involving an ellipse or spline) return ok=false and the
// caller keeps the sampled fallback.

// eventKind classifies one analytic contact between two sources.
type eventKind uint8

const (
	evCross   eventKind = iota // transverse intersection (distinct, well-separated roots)
	evTangent                  // tangential contact (a double root — curves touch, do not cross)
	evOverlap                  // coincident / collinear overlap (a degenerate, unresolvable map)
)

// xEvent is one analytic contact: its point, the natural parameter t∈[0,1] on
// each source (so the caller can place a cut on the right fragment), and the
// kind. The arrangement classifies a contact by kind (a transverse Cross splits,
// a Tangent is a non-splitting contact, an Overlap is degenerate) — there is no
// reliance on a crossing-angle magnitude.
type xEvent struct {
	x, y   float64
	ti, tj float64
	kind   eventKind
}

// analyticEvents returns the exact contact events between sources si and sj when
// both are line/circle/arc kinds (ok=true). ambiguous is set when the
// classification cannot be certified at the given scale (a near-tangency or
// near-overlap that the closed form cannot resolve cleanly) — the caller treats
// that like a degeneracy. For an unsupported kind it returns ok=false and the
// caller falls back to the sampled segment test. scale is the scene size, used to
// make the classification thresholds scale-relative.
func analyticEvents(si, sj *source, scale float64) (events []xEvent, ambiguous, ok bool) {
	if !analyticKind(si.kind) || !analyticKind(sj.kind) {
		return nil, false, false
	}
	// Reduce to (line|circle-with-sweep) operands: an arc is its full circle plus
	// a sweep filter; a circle is a full sweep.
	a := operandOf(si)
	b := operandOf(sj)
	switch {
	case a.isLine && b.isLine:
		events, ambiguous = lineLineEvents(a, b, scale)
	case a.isLine && !b.isLine:
		events, ambiguous = lineCircleEvents(a, b, scale)
	case !a.isLine && b.isLine:
		events, ambiguous = lineCircleEvents(b, a, scale)
		for i := range events {
			events[i].ti, events[i].tj = events[i].tj, events[i].ti
		}
	default:
		events, ambiguous = circleCircleEvents(a, b, scale)
	}
	// Confine each event to the swept portion of an arc operand (a full circle's
	// sweep is the whole turn). An event off either sweep is not a contact of the
	// actual arc.
	kept := events[:0]
	for _, e := range events {
		if a.inSweep(e.ti) && b.inSweep(e.tj) {
			kept = append(kept, e)
		}
	}
	return kept, ambiguous, true
}

func analyticKind(k srcKind) bool {
	return k == srcLine || k == srcCircle || k == srcArc
}

// operand is a line or a circle-with-sweep, the normalized form an arc/circle/
// line reduces to for the closed-form intersection.
type operand struct {
	isLine         bool
	ax, ay, bx, by float64 // line endpoints (also used as the source param frame)
	cx, cy, r      float64 // circle center + radius
	phi0, sweep    float64 // arc start angle + signed sweep; full circle: 0, 2π
	fullCircle     bool
}

func operandOf(s *source) operand {
	switch s.kind {
	case srcLine:
		return operand{isLine: true, ax: s.ax, ay: s.ay, bx: s.bx, by: s.by}
	case srcCircle:
		return operand{cx: s.cx, cy: s.cy, r: s.r, phi0: 0, sweep: 2 * math.Pi, fullCircle: true}
	default: // srcArc
		return operand{cx: s.cx, cy: s.cy, r: s.r, phi0: s.phi0, sweep: s.sweep}
	}
}

// lineParam returns the natural parameter t of point (x,y) on the line operand
// (t=0 at A, t=1 at B), by projection onto the segment direction.
func (o operand) lineParam(x, y float64) float64 {
	dx, dy := o.bx-o.ax, o.by-o.ay
	d2 := dx*dx + dy*dy
	if d2 == 0 {
		return 0
	}
	return ((x-o.ax)*dx + (y-o.ay)*dy) / d2
}

// circleParam returns the natural parameter t∈[0,1) of point (x,y) on the circle/
// arc operand: the fraction of the (signed) sweep from phi0 to the point's angle.
// For a full circle the sweep is 2π so t is angle/2π. The signed-sweep handling is
// symmetric so that the START of the arc (angle == phi0) maps to t=0 for BOTH a CCW
// (sweep>0) and a CW (sweep<0) arc — never wrapping a start contact to t≈1.
func (o operand) circleParam(x, y float64) float64 {
	ang := math.Atan2(y-o.cy, x-o.cx)
	d := math.Mod(ang-o.phi0, 2*math.Pi) // d ∈ (-2π, 2π)
	if o.sweep < 0 {
		if d > 0 {
			d -= 2 * math.Pi // CW: map to (-2π, 0]
		}
	} else if d < 0 {
		d += 2 * math.Pi // CCW: map to [0, 2π)
	}
	t := d / o.sweep
	// Numerically nudge a t just past the seam back into [0,1) for a full circle.
	if o.fullCircle {
		t -= math.Floor(t)
	}
	return t
}

// inSweep reports whether the source natural parameter t lies on the actual
// (extent-clipped) operand. A finite line segment and a swept arc both confine
// t to [0,1] (a small epsilon admits an exact endpoint contact); only a full
// circle accepts any wrapped parameter. Clipping lines is what keeps a carrier-line
// crossing that falls OFF the segment from being reported as a contact.
func (o operand) inSweep(t float64) bool {
	if o.fullCircle {
		return true
	}
	return t >= -arcParamEps && t <= 1+arcParamEps
}

const arcParamEps = 1e-9

// lineLineEvents: two infinite lines either cross once, are coincident (overlap),
// or are parallel and disjoint (no event). The arrangement's edge clipping decides
// whether the crossing lands on the segments; here we report the carrier crossing.
func lineLineEvents(a, b operand, scale float64) ([]xEvent, bool) {
	d1x, d1y := a.bx-a.ax, a.by-a.ay
	d2x, d2y := b.bx-b.ax, b.by-b.ay
	den := d1x*d2y - d1y*d2x
	l1 := math.Hypot(d1x, d1y)
	l2 := math.Hypot(d2x, d2y)
	if l1 == 0 || l2 == 0 {
		return nil, true // a zero-length "line" is degenerate
	}
	if sin := math.Abs(den) / (l1 * l2); sin < lineParallelEps {
		// Parallel. Distinct carriers never meet. A shared carrier is a degenerate
		// duplicate edge ONLY where the two segments actually overlap; collinear but
		// disjoint segments do not meet.
		perp := math.Abs((b.ax-a.ax)*d1y-(b.ay-a.ay)*d1x) / l1
		if perp >= scale*mergeEps {
			return nil, false // parallel, distinct carriers
		}
		tb0, tb1 := a.lineParam(b.ax, b.ay), a.lineParam(b.bx, b.by)
		lo, hi := math.Min(tb0, tb1), math.Max(tb0, tb1)
		ov0, ov1 := math.Max(0, lo), math.Min(1, hi)
		if ov1-ov0 <= arcParamEps {
			// Disjoint, or touching only at a shared endpoint — that is a normal join
			// (a corner), not a degenerate overlap.
			return nil, false
		}
		mid := (ov0 + ov1) / 2 // a point inside the positive-length overlap, in a's param
		x, y := a.ax+mid*d1x, a.ay+mid*d1y
		return []xEvent{{x: x, y: y, ti: mid, tj: b.lineParam(x, y), kind: evOverlap}}, false
	}
	t := ((b.ax-a.ax)*d2y - (b.ay-a.ay)*d2x) / den
	x, y := a.ax+t*d1x, a.ay+t*d1y
	return []xEvent{{
		x: x, y: y,
		ti:   a.lineParam(x, y),
		tj:   b.lineParam(x, y),
		kind: evCross,
	}}, false
}

// lineCircleEvents: substitute the line into the circle to get a quadratic in the
// line parameter. Two distinct roots → two transverse crossings; a double root →
// a tangency; no real root → a miss. The near-double band is ambiguous.
func lineCircleEvents(line, circ operand, scale float64) ([]xEvent, bool) {
	dx, dy := line.bx-line.ax, line.by-line.ay
	dlen := math.Hypot(dx, dy)
	if dlen == 0 {
		return nil, true
	}
	// Signed perpendicular distance from the circle center to the carrier line.
	h := ((circ.cx-line.ax)*dy - (circ.cy-line.ay)*dx) / dlen
	gap := math.Abs(h) - circ.r // <0 secant, =0 tangent, >0 miss
	certify := scale * tangentCertify
	band := scale * tangentBand
	switch {
	case math.Abs(gap) <= certify:
		// Certified tangency (double root): the foot of the perpendicular is the
		// contact, essentially on the circle.
		fx, fy := footOnLine(line, circ.cx, circ.cy)
		return []xEvent{{x: fx, y: fy, ti: line.lineParam(fx, fy), tj: circ.circleParam(fx, fy), kind: evTangent}}, false
	case gap > band:
		return nil, false // clean miss
	case gap < -band:
		// Secant: two roots at the foot ± half-chord along the line direction.
		half := math.Sqrt(circ.r*circ.r - h*h)
		fx, fy := footOnLine(line, circ.cx, circ.cy)
		ux, uy := dx/dlen, dy/dlen
		var out []xEvent
		for _, s := range []float64{-half, half} {
			x, y := fx+s*ux, fy+s*uy
			out = append(out, xEvent{
				x: x, y: y,
				ti:   line.lineParam(x, y),
				tj:   circ.circleParam(x, y),
				kind: evCross,
			})
		}
		return out, false
	default:
		// Transition zone (|gap| between certify and band): a near-tangency the
		// closed form cannot resolve cleanly → ambiguous (the caller flags it).
		return nil, true
	}
}

// circleCircleEvents: classify by center distance vs r1±r2. Two intersection
// points (secant), one (tangent, internal or external), coincident (overlap), or
// none. The near-tangent bands are ambiguous only when uncertain; a certified
// double contact is a clean tangency.
func circleCircleEvents(a, b operand, scale float64) ([]xEvent, bool) {
	dx, dy := b.cx-a.cx, b.cy-a.cy
	d := math.Hypot(dx, dy)
	certify := scale * tangentCertify
	band := scale * tangentBand
	if d < band {
		// Near-coincident centers. Certify only EXACT coincidence (same center AND
		// radius) as a degenerate overlap. Clearly different radii are concentric —
		// a clean miss (an annulus). Anything between is genuinely ambiguous (it
		// could be a sliver two-crossing).
		switch {
		case d <= certify && math.Abs(a.r-b.r) <= certify:
			return []xEvent{{x: a.cx, y: a.cy, kind: evOverlap}}, false
		case math.Abs(a.r-b.r) > band:
			return nil, false
		default:
			return nil, true
		}
	}
	sum := a.r + b.r
	diff := math.Abs(a.r - b.r)
	ux, uy := dx/d, dy/d
	switch {
	case math.Abs(d-sum) <= certify:
		// Certified external tangency: contact on the center line at radius a.r
		// toward b.
		x, y := a.cx+a.r*ux, a.cy+a.r*uy
		return []xEvent{{x: x, y: y, ti: a.circleParam(x, y), tj: b.circleParam(x, y), kind: evTangent}}, false
	case math.Abs(d-diff) <= certify:
		// Certified internal tangency: the larger circle contains the smaller; the
		// contact is at radius a.r on the side toward b when a is the larger, away
		// when a is the smaller.
		sgn := 1.0
		if a.r < b.r {
			sgn = -1.0
		}
		x, y := a.cx+sgn*a.r*ux, a.cy+sgn*a.r*uy
		return []xEvent{{x: x, y: y, ti: a.circleParam(x, y), tj: b.circleParam(x, y), kind: evTangent}}, false
	case d > sum+band || d < diff-band:
		return nil, false // clean separation / containment → miss
	case d > diff+band && d < sum-band:
		// Clean secant: two symmetric points about the center line.
		aDist := (d*d + a.r*a.r - b.r*b.r) / (2 * d) // signed distance from a's center to the radical line
		hh := a.r*a.r - aDist*aDist
		if hh < 0 {
			return nil, true // numerically inconsistent → ambiguous
		}
		half := math.Sqrt(hh)
		mx, my := a.cx+aDist*ux, a.cy+aDist*uy
		nx, ny := -uy, ux // perpendicular to the center line
		var out []xEvent
		for _, s := range []float64{-half, half} {
			x, y := mx+s*nx, my+s*ny
			out = append(out, xEvent{
				x: x, y: y,
				ti:   a.circleParam(x, y),
				tj:   b.circleParam(x, y),
				kind: evCross,
			})
		}
		return out, false
	default:
		// Transition zone near an external or internal tangency → ambiguous.
		return nil, true
	}
}

// footOnLine returns the foot of the perpendicular from (px,py) to the carrier
// line of the operand.
func footOnLine(line operand, px, py float64) (float64, float64) {
	dx, dy := line.bx-line.ax, line.by-line.ay
	d2 := dx*dx + dy*dy
	t := ((px-line.ax)*dx + (py-line.ay)*dy) / d2
	return line.ax + t*dx, line.ay + t*dy
}

// Classification thresholds, all scale-relative. tangentCertify is the tight band
// in which a center-distance / perpendicular gap is CERTIFIED to be a double-root
// tangency (a solved-exact tangency sits well inside it); tangentBand is the wider
// band outside of which a contact is a clean secant or a clean miss — the zone
// between certify and band is unresolved and reported ambiguous. tangentCertify
// MUST be < tangentBand. lineParallelEps is the dimensionless sine below which two
// lines are parallel. mergeEps matches the arrangement's vertex merge for
// coincidence tests.
const (
	tangentCertify  = 1e-9
	tangentBand     = 1e-6
	lineParallelEps = 1e-9
	mergeEps        = 1e-7
)
