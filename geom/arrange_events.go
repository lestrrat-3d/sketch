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
	// overlap carries the SECOND boundary of a coincident-carrier evOverlap event
	// this repository can RESOLVE (see coincident-carrier-resolution-design.md) — x
	// /y/ti/tj above already carry the first. nil for every other event, and for an
	// evOverlap this design leaves out of scope (a coincident LINE carrier, two
	// fully-coincident full circles, or a multi-window overlap), which callers still
	// treat as an unconditional degeneracy.
	overlap *overlapExtent
}

// overlapExtent carries BOTH boundaries of a resolvable coincident-carrier
// overlap window explicitly — lo (the same point xEvent.x/y/ti/tj already
// carries, repeated here so resolveCoincidentOverlap has both ends in one
// place) and hi (the second boundary) — plus each source's natural parameter
// at each, and the window's absolute angular extent on the shared carrier
// (angLo, hi's own absolute angle minus lo's — i.e. width — around the shared
// center). Angle space is used because it needs no per-source
// natural-parameter sign/wrap bookkeeping: the two sources can have
// independent sweep directions, but they share one physical center.
type overlapExtent struct {
	loX, loY     float64
	loTi, loTj   float64
	hiX, hiY     float64
	hiTi, hiTj   float64
	angLo, width float64
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

// arcSpan returns the operand's covered angular interval as a CCW arc
// [start, start+length] with start ∈ [0,2π) and length ∈ [0,2π]. The covered SET
// is independent of sweep direction (a CW arc covers the same angles as its CCW
// mirror); a full circle spans the whole turn.
func (o operand) arcSpan() (start, length float64) {
	if o.fullCircle {
		return 0, 2 * math.Pi
	}
	length = math.Abs(o.sweep)
	start = o.phi0
	if o.sweep < 0 {
		start = o.phi0 + o.sweep
	}
	start = math.Mod(start, 2*math.Pi)
	if start < 0 {
		start += 2 * math.Pi
	}
	return start, length
}

// arcOverlap is the result of testing two coincident-carrier operands' swept arcs
// for a positive-length angular overlap.
type arcOverlap struct {
	// midX/midY is a point strictly inside the overlap — any window, even one of
	// several disjoint ones — used only to locate a degeneracy flag, exactly like
	// the single-window case's own boundary points are used for resolution.
	midX, midY float64
	// over is true whenever the sweeps overlap with positive length, in one window
	// or more — the "is this a degenerate overlap at all" question.
	over bool
	// single is true only when the overlap is exactly ONE contiguous window (never
	// for a multi-window overlap — see "Scope" in
	// docs/coincident-carrier-resolution-design.md) — in which case loX/loY/hiX/hiY/
	// angLo/width describe it exactly. Each boundary angle is, by construction, one
	// operand's own domain end (an arc's phi0 or phi0+sweep) or the other's — never
	// an iteratively solved root.
	single             bool
	loX, loY, hiX, hiY float64
	angLo, width       float64
}

// coincidentArcOverlap tests two coincident-carrier operands' swept arcs for a
// positive-length angular overlap. Caller has already established the carriers are
// the same circle.
func coincidentArcOverlap(a, b operand) arcOverlap {
	// A full circle covers every angle, so intersecting with one always yields the
	// OTHER operand's entire own domain — computed directly, bypassing the
	// wraparound-offset split below. That split represents "the other operand's
	// domain, relative to a's own reference frame" as [rb,rb+lb] cut at rb; for a
	// full circle b, rb is b's own arbitrary phi0=0 reference re-expressed in a's
	// frame, not a real boundary of anything, so the loop would artificially
	// bisect a's single true window into two ADJACENT (not disjoint) pieces at rb
	// — indistinguishable, by length alone, from a genuine multi-window overlap.
	switch {
	case a.fullCircle && b.fullCircle:
		// Two fully-coincident full circles: an overlap "window" has no meaning (the
		// whole circle is shared), so report just enough to flag the degeneracy —
		// resolution is out of scope for this pair regardless (both fullCircle; see
		// the a.fullCircle==b.fullCircle exclusion at the call site).
		return arcOverlap{midX: a.cx + a.r, midY: a.cy, over: true}
	case a.fullCircle:
		return fullCircleOverlap(b)
	case b.fullCircle:
		return fullCircleOverlap(a)
	}
	sa, la := a.arcSpan()
	sb, lb := b.arcSpan()
	rb := math.Mod(sb-sa, 2*math.Pi)
	if rb < 0 {
		rb += 2 * math.Pi
	}
	// Arc a is [0,la]; arc b is [rb, rb+lb] and its wrapped copy [rb-2π, rb+lb-2π].
	// The two offsets are the SAME angular relationship represented twice (to catch
	// b wrapping across a's 0), so ordinarily at most one has positive length — but
	// when both arcs cover nearly the whole circle, the pair can genuinely overlap
	// in two disjoint windows, one from each offset; that is the multi-window case.
	var lens, los, his [2]float64
	for k, off := range [2]float64{rb, rb - 2*math.Pi} {
		lo := math.Max(0, off)
		hi := math.Min(la, off+lb)
		if hi > lo {
			lens[k], los[k], his[k] = hi-lo, lo, hi
		}
	}
	bestK := 0
	if lens[1] > lens[0] {
		bestK = 1
	}
	if lens[bestK] <= arcParamEps {
		return arcOverlap{} // disjoint, or endpoint-only touch
	}
	angLo, angHi := sa+los[bestK], sa+his[bestK]
	res := arcOverlap{
		midX: a.cx + a.r*math.Cos((angLo+angHi)/2),
		midY: a.cy + a.r*math.Sin((angLo+angHi)/2),
		over: true,
	}
	if otherK := 1 - bestK; lens[otherK] > arcParamEps {
		return res // a second, disjoint window: over, but not a single resolvable one
	}
	res.single = true
	res.angLo, res.width = angLo, angHi-angLo
	res.loX, res.loY = a.cx+a.r*math.Cos(angLo), a.cy+a.r*math.Sin(angLo)
	res.hiX, res.hiY = a.cx+a.r*math.Cos(angHi), a.cy+a.r*math.Sin(angHi)
	return res
}

// fullCircleOverlap returns the overlap between a full circle and the given (non-
// full) arc operand: the arc's entire own domain, since a full circle covers every
// angle. Used by coincidentArcOverlap when exactly one operand is a full circle.
func fullCircleOverlap(arc operand) arcOverlap {
	sa, la := arc.arcSpan()
	angLo, angHi := sa, sa+la
	res := arcOverlap{
		midX: arc.cx + arc.r*math.Cos((angLo+angHi)/2),
		midY: arc.cy + arc.r*math.Sin((angLo+angHi)/2),
		over: true, single: true,
		angLo: angLo, width: la,
	}
	res.loX, res.loY = arc.cx+arc.r*math.Cos(angLo), arc.cy+arc.r*math.Sin(angLo)
	res.hiX, res.hiY = arc.cx+arc.r*math.Cos(angHi), arc.cy+arc.r*math.Sin(angHi)
	return res
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
			// Coincident carrier circles. They are a degenerate overlap ONLY where the
			// two swept arcs actually coincide; same-carrier arcs whose sweeps are
			// disjoint, or meet only at a shared endpoint, do not overlap (mirrors the
			// collinear-line case); no overlap returns no event, leaving disjoint arcs
			// clean. A SINGLE-window overlap with at least one ARC operand (a finite
			// domain to bound the resolution) is RESOLVABLE (see
			// docs/coincident-carrier-resolution-design.md's "Scope"): both boundary
			// points are reported so analyticPrepass can cut both sources and suppress
			// the losing one over the shared span. Two fully-coincident FULL circles and
			// a multi-window overlap stay an unconditional degeneracy — the event's
			// overlap field is left nil, and the caller flags it exactly as before.
			ov := coincidentArcOverlap(a, b)
			if !ov.over {
				return nil, false
			}
			e := xEvent{x: ov.midX, y: ov.midY, ti: a.circleParam(ov.midX, ov.midY), tj: b.circleParam(ov.midX, ov.midY), kind: evOverlap}
			// ov.single is never true when both operands are full circles —
			// coincidentArcOverlap's own a.fullCircle&&b.fullCircle case always returns
			// single=false — so this already excludes that out-of-scope pair; no
			// separate check needed here.
			if ov.single {
				e.overlap = &overlapExtent{
					loX: ov.loX, loY: ov.loY, loTi: a.circleParam(ov.loX, ov.loY), loTj: b.circleParam(ov.loX, ov.loY),
					hiX: ov.hiX, hiY: ov.hiY, hiTi: a.circleParam(ov.hiX, ov.hiY), hiTj: b.circleParam(ov.hiX, ov.hiY),
					angLo: ov.angLo, width: ov.width,
				}
			}
			return []xEvent{e}, false
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
