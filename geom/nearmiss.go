package geom

import "math"

// Chord-deviation bounds and the near-miss degeneracy guard.
//
// The sampled arrangement reaches every curve through chords, and a chord is not
// the curve: between two consecutive samples the curve bows away from it, and a
// crossing that lives entirely inside that bow is absent from the planar map
// before any pair test runs. Nothing downstream sees it — the regions it would
// have separated are simply fused, and every soundness signal reads clean.
//
// This file closes that hole from the conservative side. Each tiny segment
// carries a PROVEN upper bound on how far its source departs from its own chord
// over that segment's parameter span (arranger.segDev, filled by densify). Two
// segments whose chords approach within the SUM of their two bounds may hide a
// crossing between them; where the sampled map recorded no contact between the
// two sources anywhere near that approach, the arrangement cannot rule a crossing
// out and says so with Degenerate.
//
// Load-bearing: a bound is a BOUND, never a sample. A deviation read off the
// span's midpoint — or off any finite set of points on it — says nothing about a
// lobe elsewhere in the span, and using one as a reach was measured here at a
// 22195x underestimate of the true maximum on the same span. Every bound below is
// either a closed form for the whole span (circle/arc sagitta, the second-
// derivative interpolation bound for the ellipse family) or the convex hull of
// the sub-span's own control polygon (the conic and the whole spline family). A
// family that could supply neither would report ok=false and be treated as
// unbounded — flagged rather than guessed.

// chordDeviation returns a proven upper bound on the distance from any point of
// source s over its natural-parameter span [p0,p1] to the straight chord
// (ax,ay)→(bx,by) the sampler put there. ok is false when no bound can be proven
// for the family, in which case the span is unbounded.
func chordDeviation(s *source, p0, p1, ax, ay, bx, by float64) (float64, bool) {
	dev, ok := spanDeviation(s, p0, p1, ax, ay, bx, by)
	if !ok || math.IsNaN(dev) {
		return math.Inf(1), false
	}
	// The sampler may place a chord endpoint somewhere other than the curve point
	// at that parameter — an elliptical arc pins its ends to its exact boundary
	// points. Displacing an endpoint by d moves every point of the chord by at
	// most d, so the bound carries it additively.
	q0, q1 := s.at(p0), s.at(p1)
	slip := math.Max(math.Hypot(q0[0]-ax, q0[1]-ay), math.Hypot(q1[0]-bx, q1[1]-by))
	return dev + slip, true
}

// spanDeviation is chordDeviation without the endpoint-slip correction: the
// per-family bound on the curve's departure from the chord.
func spanDeviation(s *source, p0, p1, ax, ay, bx, by float64) (float64, bool) {
	switch s.kind {
	case srcLine:
		// A line's polyline IS the line.
		return 0, true
	case srcCircle:
		return circularSagitta(s.r, 2*math.Pi*math.Abs(p1-p0)), true
	case srcArc:
		return circularSagitta(s.r, math.Abs(s.sweep)*math.Abs(p1-p0)), true
	case srcEllipse:
		return chordInterpBound(2*math.Pi*math.Abs(p1-p0), math.Max(s.rx, s.ry)), true
	case srcEllipticalArc:
		return chordInterpBound(math.Abs(s.sweep)*math.Abs(p1-p0), math.Max(s.rx, s.ry)), true
	case srcConic:
		hull := conicSpanHull(s, p0, p1)
		return hullDeviation(hull, ax, ay, bx, by), true
	case srcSpline:
		hull, ok := bsplineSpanHull(3, s.ctrl, nil, ClampedKnots(len(s.ctrl)), p0, p1)
		if !ok {
			return 0, false
		}
		return hullDeviation(hull, ax, ay, bx, by), true
	case srcNURBS:
		nb := s.nurbs
		lo, hi := nb.domain()
		ctrl := make([][2]float64, len(nb.Control))
		for i, p := range nb.Control {
			ctrl[i] = [2]float64{p.X, p.Y}
		}
		hull, ok := bsplineSpanHull(nb.Degree, ctrl, nb.Weights, nb.Knots, lo+(hi-lo)*p0, lo+(hi-lo)*p1)
		if !ok {
			return 0, false
		}
		return hullDeviation(hull, ax, ay, bx, by), true
	case srcClosedSpline:
		hull, ok := closedSplineSpanHull(s.ctrl, p0, p1)
		if !ok {
			return 0, false
		}
		return hullDeviation(hull, ax, ay, bx, by), true
	case srcFitSpline:
		hull, ok := fitSplineSpanHull(s.fitEval, p0, p1)
		if !ok {
			return 0, false
		}
		return hullDeviation(hull, ax, ay, bx, by), true
	}
	return 0, false
}

// circularSagitta is the exact maximum distance from a circular arc of radius r
// sweeping theta to its own chord: r·(1−cos(theta/2)). densify floors every
// source at two segments, so theta never exceeds pi and the maximum is the
// midpoint sagitta.
func circularSagitta(r, theta float64) float64 {
	if theta <= 0 {
		return 0
	}
	if theta > math.Pi {
		theta = math.Pi
	}
	return r * (1 - math.Cos(theta/2))
}

// chordInterpBound is the standard linear-interpolation error bound h²·M/8 for a
// curve whose second derivative with respect to the span parameter is bounded by
// M over a span of length h. It holds for the vector-valued case with the vector
// norm: P(t)−chord(t) is the Green's-function integral of the second derivative
// against a kernel whose L1 norm peaks at h²/8.
//
// The ellipse family uses it because its second derivative has a closed-form
// bound: with C + rx·cos(phi)·u + ry·sin(phi)·v and u,v orthonormal, the second
// derivative in the eccentric angle is −rx·cos(phi)·u − ry·sin(phi)·v, whose norm
// is at most max(rx,ry) everywhere.
func chordInterpBound(h, m2 float64) float64 {
	if h <= 0 || m2 <= 0 {
		return 0
	}
	return h * h * m2 / 8
}

// hullDeviation bounds a curve's departure from the chord (ax,ay)→(bx,by) by the
// largest distance from a point of hull to that chord SEGMENT.
//
// It is sound because the curve over the span lies inside the convex hull of
// those points, and the distance to a segment is a convex function — so its
// maximum over a convex hull is attained at a hull point, and every point of the
// hull is at most that far from the chord.
func hullDeviation(hull [][2]float64, ax, ay, bx, by float64) float64 {
	worst := 0.0
	for _, q := range hull {
		if d := distPointSeg(q[0], q[1], ax, ay, bx, by); d > worst {
			worst = d
		}
	}
	return worst
}

// distPointSeg is the distance from (px,py) to the segment (ax,ay)→(bx,by).
func distPointSeg(px, py, ax, ay, bx, by float64) float64 {
	ex, ey := bx-ax, by-ay
	l2 := ex*ex + ey*ey
	if l2 <= 0 {
		return math.Hypot(px-ax, py-ay)
	}
	t := ((px-ax)*ex + (py-ay)*ey) / l2
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return math.Hypot(px-(ax+t*ex), py-(ay+t*ey))
}

// ---------------------------------------------------------------------------
// convex-hull enclosures per sub-span
// ---------------------------------------------------------------------------

// conicSpanHull returns the three control points of the rational quadratic Bézier
// restricted to [p0,p1] — the sub-curve's own control polygon, whose convex hull
// contains it (positive weights make every curve point a convex combination of
// them).
func conicSpanHull(s *source, p0, p1 float64) [][2]float64 {
	cp := [][3]float64{
		{s.conStart[0], s.conStart[1], 1},
		{s.conApex[0] * s.conW, s.conApex[1] * s.conW, s.conW},
		{s.conEnd[0], s.conEnd[1], 1},
	}
	return projectHom(bezierSubSpan(cp, p0, p1))
}

// closedSplineSpanHull returns control points whose convex hull contains the
// periodic uniform cubic B-spline over [p0,p1].
//
// Each unit span i of the periodic basis is the cubic Bézier of its four cyclic
// controls (the standard uniform B-spline-to-Bézier conversion), so the sub-span
// falls out of de Casteljau. A span [p0,p1] that straddles several of them
// contributes each piece's hull; their union still encloses the curve.
func closedSplineSpanHull(ctrl [][2]float64, p0, p1 float64) ([][2]float64, bool) {
	n := len(ctrl)
	if n < 3 {
		return nil, false
	}
	u0, u1 := p0*float64(n), p1*float64(n)
	if u1 < u0 {
		u0, u1 = u1, u0
	}
	lo := int(math.Floor(u0))
	hi := int(math.Ceil(u1)) - 1
	if hi < lo {
		hi = lo
	}
	var out [][2]float64
	for i := lo; i <= hi; i++ {
		if i < 0 || i >= n {
			continue
		}
		p := [4][2]float64{ctrl[i], ctrl[(i+1)%n], ctrl[(i+2)%n], ctrl[(i+3)%n]}
		cp := [][3]float64{
			{(p[0][0] + 4*p[1][0] + p[2][0]) / 6, (p[0][1] + 4*p[1][1] + p[2][1]) / 6, 1},
			{(2*p[1][0] + p[2][0]) / 3, (2*p[1][1] + p[2][1]) / 3, 1},
			{(p[1][0] + 2*p[2][0]) / 3, (p[1][1] + 2*p[2][1]) / 3, 1},
			{(p[1][0] + 4*p[2][0] + p[3][0]) / 6, (p[1][1] + 4*p[2][1] + p[3][1]) / 6, 1},
		}
		lv := math.Max(u0-float64(i), 0)
		hv := math.Min(u1-float64(i), 1)
		if hv < lv {
			continue
		}
		out = append(out, projectHom(bezierSubSpan(cp, lv, hv))...)
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// fitSplineSpanHull returns control points whose convex hull contains the
// natural-cubic interpolant over the normalized chord range [s0,s1].
//
// Each natural-cubic piece is a cubic polynomial, converted to Bézier form from
// its endpoint values and tangents (the Hermite-to-Bézier identity B1 = B0 + h·d0/3,
// B2 = B3 − h·d1/3), so the sub-span again falls out of de Casteljau.
func fitSplineSpanHull(e *fitEvaluator, s0, s1 float64) ([][2]float64, bool) {
	if e == nil {
		return nil, false
	}
	k := len(e.x)
	if k == 0 {
		return nil, false
	}
	if k == 1 {
		return [][2]float64{{e.x[0], e.y[0]}}, true
	}
	total := e.t[k-1]
	if !(total > 0) {
		return [][2]float64{{e.x[0], e.y[0]}}, true
	}
	p0, p1 := s0*total, s1*total
	if p1 < p0 {
		p0, p1 = p1, p0
	}
	var out [][2]float64
	for i := 0; i < k-1; i++ {
		lo, hi := e.t[i], e.t[i+1]
		h := hi - lo
		if !(h > 0) || hi < p0 || lo > p1 {
			continue
		}
		b0 := [2]float64{e.x[i], e.y[i]}
		b3 := [2]float64{e.x[i+1], e.y[i+1]}
		d0 := [2]float64{derivCubicSpan(e.t, e.x, e.mx, i, lo), derivCubicSpan(e.t, e.y, e.my, i, lo)}
		d1 := [2]float64{derivCubicSpan(e.t, e.x, e.mx, i, hi), derivCubicSpan(e.t, e.y, e.my, i, hi)}
		cp := [][3]float64{
			{b0[0], b0[1], 1},
			{b0[0] + h*d0[0]/3, b0[1] + h*d0[1]/3, 1},
			{b3[0] - h*d1[0]/3, b3[1] - h*d1[1]/3, 1},
			{b3[0], b3[1], 1},
		}
		lv := math.Max((p0-lo)/h, 0)
		hv := math.Min((p1-lo)/h, 1)
		if hv < lv {
			continue
		}
		out = append(out, projectHom(bezierSubSpan(cp, lv, hv))...)
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// bsplineSpanHull returns control points whose convex hull contains a clamped
// (possibly rational) B-spline of the given degree over the knot interval
// [u0,u1].
//
// Both bounds are inserted into the knot vector to full multiplicity p, after
// which the control points supporting [u0,u1] are exactly those of the Bézier
// pieces it spans. Insertion runs on the HOMOGENEOUS control points; the hull
// property then holds for the rational curve itself, since every point of it is a
// convex combination of the projected controls (weights are positive by the
// NURBS validity check).
func bsplineSpanHull(degree int, ctrl [][2]float64, weights, knots []float64, u0, u1 float64) ([][2]float64, bool) {
	p := degree
	if p < 1 || len(ctrl) < p+1 || len(knots) != len(ctrl)+p+1 {
		return nil, false
	}
	if u1 < u0 {
		u0, u1 = u1, u0
	}
	lo, hi := knots[p], knots[len(ctrl)]
	if !(hi > lo) {
		return nil, false
	}
	u0 = math.Max(u0, lo)
	u1 = math.Min(u1, hi)
	pw := make([][3]float64, len(ctrl))
	for i, c := range ctrl {
		w := 1.0
		if i < len(weights) {
			w = weights[i]
		}
		if !(w > 0) {
			return nil, false
		}
		pw[i] = [3]float64{w * c[0], w * c[1], w}
	}
	u := append([]float64(nil), knots...)
	for _, x := range [2]float64{u0, u1} {
		if x <= lo || x >= hi {
			continue // a clamped end already carries multiplicity p+1
		}
		for m := knotMultiplicity(u, x); m < p; m++ {
			u, pw = insertKnotHom(p, u, pw, x)
		}
	}
	// Span k of the refined curve is supported by control points k−p..k. The
	// sub-curve runs from the span STARTING at u0 to the one ENDING at u1, so the
	// supporting set is [findSpan(u0)−p, findSpan(u1)−p] — the upper bound reads a
	// span index, not a control index, because u1 carries multiplicity p and
	// findSpan lands on the last of its copies.
	n := len(pw) - 1
	first := findSpan(n, p, u0, u) - p
	last := n
	if u1 < hi {
		last = findSpan(n, p, u1, u) - p
	}
	if first < 0 {
		first = 0
	}
	// Never below the full support of the span STARTING at u0: dropping part of it
	// would leave the hull enclosing less than the curve, which is the one direction
	// this must not err in.
	if last < first+p {
		last = first + p
	}
	if last > n {
		last = n
	}
	out := make([][2]float64, 0, last-first+1)
	for i := first; i <= last; i++ {
		w := pw[i][2]
		if !(w > 0) {
			return nil, false
		}
		out = append(out, [2]float64{pw[i][0] / w, pw[i][1] / w})
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// knotMultiplicity counts how many times x appears in the knot vector.
func knotMultiplicity(u []float64, x float64) int {
	m := 0
	for _, k := range u {
		if k == x {
			m++
		}
	}
	return m
}

// insertKnotHom inserts x once into the degree-p knot vector u with homogeneous
// control points pw (Boehm's algorithm), returning the refined pair. The curve is
// unchanged; only its control representation is refined.
func insertKnotHom(p int, u []float64, pw [][3]float64, x float64) ([]float64, [][3]float64) {
	n := len(pw) - 1
	k := findSpan(n, p, x, u)
	s := 0
	for i := k; i >= 0 && u[i] == x; i-- {
		s++
	}
	q := make([][3]float64, n+2)
	for i := 0; i <= k-p; i++ {
		q[i] = pw[i]
	}
	for i := k - s + 1; i <= n+1; i++ {
		q[i] = pw[i-1]
	}
	for i := k - p + 1; i <= k-s; i++ {
		alpha := 0.0
		if den := u[i+p] - u[i]; den > 0 {
			alpha = (x - u[i]) / den
		}
		q[i] = [3]float64{
			alpha*pw[i][0] + (1-alpha)*pw[i-1][0],
			alpha*pw[i][1] + (1-alpha)*pw[i-1][1],
			alpha*pw[i][2] + (1-alpha)*pw[i-1][2],
		}
	}
	nu := make([]float64, 0, len(u)+1)
	nu = append(nu, u[:k+1]...)
	nu = append(nu, x)
	nu = append(nu, u[k+1:]...)
	return nu, q
}

// bezierSubSpan returns the homogeneous control points of the Bézier restricted
// to [v0,v1] ⊆ [0,1], by two de Casteljau subdivisions.
func bezierSubSpan(cp [][3]float64, v0, v1 float64) [][3]float64 {
	if v0 < 0 {
		v0 = 0
	}
	if v1 > 1 {
		v1 = 1
	}
	if v1 <= v0 {
		// A zero-length sub-span is one point of the curve, not the whole piece.
		// Returning the piece's own polygon here is what once let a sample span
		// ending exactly on a knot pull in the neighbouring piece's full hull,
		// inflating that segment's bound by three orders of magnitude.
		_, right := deCasteljauSplit(cp, v0)
		return right[:1]
	}
	if v0 > 0 {
		_, cp = deCasteljauSplit(cp, v0)
		v1 = (v1 - v0) / (1 - v0)
	}
	if v1 < 1 {
		cp, _ = deCasteljauSplit(cp, v1)
	}
	return cp
}

// deCasteljauSplit subdivides a Bézier (homogeneous control points) at parameter
// t, returning the control points of the left and right pieces.
func deCasteljauSplit(cp [][3]float64, t float64) ([][3]float64, [][3]float64) {
	n := len(cp)
	tmp := append([][3]float64(nil), cp...)
	left := make([][3]float64, n)
	right := make([][3]float64, n)
	left[0] = tmp[0]
	right[n-1] = tmp[n-1]
	for k := 1; k < n; k++ {
		for i := 0; i < n-k; i++ {
			tmp[i] = [3]float64{
				tmp[i][0] + t*(tmp[i+1][0]-tmp[i][0]),
				tmp[i][1] + t*(tmp[i+1][1]-tmp[i][1]),
				tmp[i][2] + t*(tmp[i+1][2]-tmp[i][2]),
			}
		}
		left[k] = tmp[0]
		right[n-1-k] = tmp[n-1-k]
	}
	return left, right
}

// projectHom divides homogeneous control points through by their weight.
func projectHom(cp [][3]float64) [][2]float64 {
	out := make([][2]float64, 0, len(cp))
	for _, c := range cp {
		if c[2] == 0 {
			continue
		}
		out = append(out, [2]float64{c[0] / c[2], c[1] / c[2]})
	}
	return out
}

// ---------------------------------------------------------------------------
// the guard
// ---------------------------------------------------------------------------

// nearMissGuard flags the arrangement degenerate wherever the sampled path cannot
// rule out a crossing it may have missed.
//
// Two chords whose separation is no more than the sum of their sources' proven
// chord-deviation bounds may have the true curves crossing between them — twice,
// so the chords need not cross at all and nothing is recorded. Where the sampled
// map made no contact between those two sources near that approach, the region
// set it publishes may be missing a genuine crossing, and Degenerate is the only
// honest verdict. Raising WithSegmentsPerTurn shrinks every bound quadratically,
// so a caller who needs the region set can buy it with density.
//
// Deliberately scoped to a pair with at least one FREE-FORM source. Line, circle
// and arc pairs are classified by the closed-form kernel, which finds their
// crossings exactly and needs no band; an all-analytic scene is untouched by this
// pass, and pays nothing for it either — the loop never reaches its segments.
//
// The excusing contact is read from the sampled map's own record (sampledContacts:
// a chord/chord crossing, or a weld of two sample vertices), so an ordinary join —
// a spline meeting a line at a shared endpoint, where the chords touch and the
// separation is zero — is explained by the vertex that is already there rather
// than flagged.
func (a *arranger) nearMissGuard() {
	for i := range a.sources {
		ki := a.sources[i].kind
		if ki == srcDegenerate {
			continue
		}
		for j := i + 1; j < len(a.sources); j++ {
			kj := a.sources[j].kind
			if kj == srcDegenerate {
				continue
			}
			if analyticKind(ki) && analyticKind(kj) {
				continue
			}
			if x, y, miss := a.pairNearMiss(i, j); miss {
				a.flagDegenerate(x, y, i, j)
			}
		}
	}
}

// pairNearMiss reports the first approach between the two sources' chords that
// their deviation bounds cannot separate and no recorded contact explains.
func (a *arranger) pairNearMiss(i, j int) (float64, float64, bool) {
	contacts := a.sampledContacts[pairKey(i, j)]
	for _, ii := range a.sourceSegs[i] {
		si := &a.segs[ii]
		di := a.segDev[ii]
		for _, jj := range a.sourceSegs[j] {
			sj := &a.segs[jj]
			band := di + a.segDev[jj]
			if !(band > 0) {
				continue
			}
			if boxesApart(si, sj, band) {
				continue
			}
			d, x, y := segSegDistance(si, sj)
			if d > band {
				continue
			}
			// A contact EXPLAINS this approach when it sits within the BAND of it —
			// the same length the approach was judged by, so the window is exactly
			// the reach a hidden crossing has and no wider, and it shrinks with
			// density along with the band. The merge tolerance floors it, for a
			// contact the sampling recorded as a weld rather than as a crossing.
			//
			// One CURVED CHORD — the window sampledCrossingsExplained and
			// sampledRepresents use — was measured as far too generous here. Those two
			// ask whether a contact the kernel already found is REPRESENTED in the
			// sampled map, where a chord-length window is the error of the thing being
			// located; this asks whether anything is recorded at all near an approach
			// nothing has certified. On a chord window a grazing lens narrower than one
			// chord answered for itself: over the hider x partner matrix a chord window
			// left the wrong region count unflagged in 30 of 45 combinations, and the
			// band window flags every one of them with no measured cost on ordinary
			// geometry.
			tol := math.Max(band, a.merge)
			explained := false
			for _, c := range contacts {
				if math.Hypot(c[0]-x, c[1]-y) <= tol {
					explained = true
					break
				}
			}
			if !explained {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

// boxesApart is the cheap reject: the two chords' bounding boxes are separated by
// more than band, so no point of one is within band of the other.
func boxesApart(si, sj *tinySeg, band float64) bool {
	iloX, ihiX := math.Min(si.ax, si.bx), math.Max(si.ax, si.bx)
	iloY, ihiY := math.Min(si.ay, si.by), math.Max(si.ay, si.by)
	jloX, jhiX := math.Min(sj.ax, sj.bx), math.Max(sj.ax, sj.bx)
	jloY, jhiY := math.Min(sj.ay, sj.by), math.Max(sj.ay, sj.by)
	return jloX-ihiX > band || iloX-jhiX > band || jloY-ihiY > band || iloY-jhiY > band
}

// segSegDistance returns the minimum distance between two chords together with a
// witness point midway between the closest pair — the location a hidden crossing
// would have to sit near.
func segSegDistance(si, sj *tinySeg) (float64, float64, float64) {
	if p, ok := segParams(si, sj); ok && p.ti >= 0 && p.ti <= 1 && p.tj >= 0 && p.tj <= 1 {
		return 0, p.x, p.y
	}
	best := math.Inf(1)
	var bx, by float64
	note := func(px, py, qx, qy float64) {
		if d := math.Hypot(px-qx, py-qy); d < best {
			best, bx, by = d, (px+qx)/2, (py+qy)/2
		}
	}
	for _, e := range [2][2]float64{{si.ax, si.ay}, {si.bx, si.by}} {
		fx, fy := footOnSeg(e[0], e[1], sj.ax, sj.ay, sj.bx, sj.by)
		note(e[0], e[1], fx, fy)
	}
	for _, e := range [2][2]float64{{sj.ax, sj.ay}, {sj.bx, sj.by}} {
		fx, fy := footOnSeg(e[0], e[1], si.ax, si.ay, si.bx, si.by)
		note(e[0], e[1], fx, fy)
	}
	return best, bx, by
}

// footOnSeg returns the point of the segment (ax,ay)→(bx,by) closest to (px,py).
func footOnSeg(px, py, ax, ay, bx, by float64) (float64, float64) {
	ex, ey := bx-ax, by-ay
	l2 := ex*ex + ey*ey
	if l2 <= 0 {
		return ax, ay
	}
	t := ((px-ax)*ex + (py-ay)*ey) / l2
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return ax + t*ex, ay + t*ey
}
