package geom

import (
	"math"
	"math/big"
)

// The pair ledger decides, for every pair of sources that includes a fit spline,
// whether the two EXACT curves are provably disjoint or provably share only the
// end points they both own. A scene whose every such pair is proven, and whose
// proof the sampled map confirms (auditPairLedger), publishes exact parameter
// bounds although it holds a fit spline. Design in docs/pair-certificate-design.md.
//
// The proof needs no assumption about what the scene depicts: it reads the exact
// rational Bézier pieces of each spline and the exact float64 parameters of each
// line, circle and arc, and it uses no threshold other than the binary64 depth
// limit of ledgerMaxDepth and the vertex-merge distance the planar map already
// welds by.

// ledgerMaxDepth is the subdivision depth limit: the number of mantissa bits of a
// binary64. Two stored curves closer than one part in 2^52 of their extent cannot
// be told apart from the stored coordinates.
const ledgerMaxDepth = 52

type pairVerdict uint8

const (
	// verdictUnproven means the certificate could not decide the pair.
	verdictUnproven pairVerdict = iota
	// verdictDisjoint means the two curves share no point.
	verdictDisjoint
	// verdictEndpointOnly means the curves share only the listed end points.
	verdictEndpointOnly
	// verdictSimple is the self entry of a spline that provably never meets itself.
	verdictSimple
	// verdictCrossing means the curves cross transversally at exactly the listed
	// spline parameters, and nowhere else. The crossings are already injected into
	// the arrangement as analytic events.
	verdictCrossing
)

type pairReason uint8

const (
	reasonNone pairReason = iota
	// reasonResolution: the hulls still overlapped at ledgerMaxDepth. The pair
	// touches, crosses, overlaps or comes closer than binary64 can resolve.
	reasonResolution
	// reasonTangent: no cut of a shared end separated the curves near it.
	reasonTangent
	// reasonBudget: the work limit of the pair was reached.
	reasonBudget
	// reasonSelfContact: the spline's direction test could not show it never meets itself.
	reasonSelfContact
	// reasonDegenerate: a spline piece or target was not finite or had no extent.
	reasonDegenerate
	// reasonAudit: the sampled map recorded a contact the proof does not allow.
	reasonAudit
	// reasonEndMismatch: two splines share an end whose coordinates are not bit-identical.
	reasonEndMismatch
)

type pairEntry struct {
	verdict pairVerdict
	reason  pairReason
	// ends are the shared end points of an endpoint-only verdict.
	ends [][2]float64
	// crossings are the spline parameters of a crossing verdict.
	crossings []float64
}

// ledgerFit is the exact form of one fit spline.
type ledgerFit struct {
	pieces []exactFitPiece
	hulls  []exactFitHull
	bounds exactFitBounds
	fbox   floatBox
	p0, p1 [2]float64
}

// ledgerTarget is the exact form of the other source of a pair.
type ledgerTarget struct {
	kind srcKind
	// line: the segment as a degenerate hull (a, b, a, b).
	seg exactFitHull
	// circle and arc: the full carrier circle.
	cx, cy, r2 *big.Rat
	// arc: the wedge from the centre that holds the arc, widened by arcWedgeSlack
	// on both sides, or nil when the sweep is a half turn or more.
	wedge *ratWedge
	// fit spline.
	fit *ledgerFit
	// fcx, fcy and fr are the centre and radius of a circle or arc as the float64
	// values the exact ones were read from.
	fcx, fcy, fr float64
	// bounds of the whole target, and the float box that holds them.
	bounds exactFitBounds
	fbox   floatBox
}

type pairLedger struct {
	a     *arranger
	merge *big.Rat
	fits  map[int]*ledgerFit
	// targets caches the exact form of each source, built on first use.
	targets map[int]*ledgerTarget
}

// buildPairLedger fills a.ledger when the scene holds fit splines and otherwise
// only line, circle and arc sources, and lifts the whole-scene kind gate for such a
// scene. It does nothing for a scene the narrower certificate already admitted.
func (a *arranger) buildPairLedger() {
	if a.exactAllowed || a.exactVeto {
		return
	}
	nFit := 0
	for i := range a.sources {
		a.budget.step(1)
		switch k := a.sources[i].kind; {
		case k == srcFitSpline:
			nFit++
		case analyticKind(k):
		default:
			return
		}
	}
	if nFit == 0 {
		return
	}
	l := &pairLedger{a: a, merge: rat(a.merge), fits: make(map[int]*ledgerFit, nFit),
		targets: make(map[int]*ledgerTarget, len(a.sources))}
	if l.merge == nil {
		return
	}
	a.ledger = make(map[[2]int]pairEntry)
	for i := range a.sources {
		a.budget.step(1)
		if a.sources[i].kind != srcFitSpline {
			continue
		}
		f := l.fit(i)
		if f == nil {
			a.ledger[[2]int{i, i}] = pairEntry{reason: reasonDegenerate}
			continue
		}
		l.fits[i] = f
	}
	for i := range a.sources {
		a.budget.step(1)
		f, ok := l.fits[i]
		if !ok {
			continue
		}
		a.ledger[[2]int{i, i}] = l.self(f)
		for j := range a.sources {
			a.budget.step(1)
			if j == i {
				continue
			}
			if a.sources[j].kind == srcFitSpline && j < i {
				continue // judged from the lower index
			}
			a.ledger[pairKey(i, j)] = l.judge(i, f, j)
		}
	}
	a.ledgerOn = true
	a.exactAllowed = true
}

func (l *pairLedger) fit(i int) *ledgerFit {
	src := &l.a.sources[i]
	if src.fitEval == nil {
		return nil
	}
	fi := src.fitEval.interpolant()
	if fi == nil {
		return nil
	}
	pieces, ok := memoFitPieces(fitInterpolantKey(fi), fi)
	if !ok || len(pieces) == 0 {
		return nil
	}
	hulls := memoFitHulls(fitInterpolantKey(fi), pieces)
	f := &ledgerFit{pieces: pieces, hulls: hulls, bounds: exactFitHullBounds(hulls), p0: src.at(0), p1: src.at(1)}
	f.fbox = outwardBox(f.bounds)
	if f.bounds.min[0] == nil || !finitePt(f.p0) || !finitePt(f.p1) {
		return nil
	}
	return f
}

func (l *pairLedger) target(j int) *ledgerTarget {
	src := &l.a.sources[j]
	t := &ledgerTarget{kind: src.kind}
	switch src.kind {
	case srcLine:
		ax, ay, bx, by := rat(src.ax), rat(src.ay), rat(src.bx), rat(src.by)
		if ax == nil || ay == nil || bx == nil || by == nil {
			return nil
		}
		t.seg = exactFitHull{points: [4][2]*big.Rat{{ax, ay}, {bx, by}, {ax, ay}, {bx, by}}}
		t.bounds = hullBounds(&t.seg)
	case srcCircle, srcArc:
		cx, cy, r := rat(src.cx), rat(src.cy), rat(src.r)
		if cx == nil || cy == nil || r == nil || r.Sign() <= 0 {
			return nil
		}
		t.cx, t.cy, t.r2 = cx, cy, mul(r, r)
		t.fcx, t.fcy, t.fr = src.cx, src.cy, src.r
		if src.kind == srcArc {
			t.wedge = arcWedge(src, cx, cy)
		}
		t.bounds = exactFitBounds{
			min: [2]*big.Rat{sub(cx, r), sub(cy, r)},
			max: [2]*big.Rat{add(cx, r), add(cy, r)},
		}
		if t.wedge != nil {
			if b, ok := arcBounds(src); ok {
				t.bounds = b
			}
		}
	default:
		return nil
	}
	t.fbox = outwardBox(t.bounds)
	return t
}

// farFromBox reports whether the target provably misses the whole box. The box
// stands for the spline's bounds, so a miss there is a miss for every piece.
func (t *ledgerTarget) farFromBox(b exactFitBounds, fb floatBox) bool {
	if ledgerFloatPrefilter && fb.apart(t.fbox) {
		return true
	}
	if boundsDisjoint(b, t.bounds) {
		return true
	}
	if t.fit != nil {
		return false
	}
	corners := exactFitHull{points: [4][2]*big.Rat{
		{b.min[0], b.min[1]}, {b.max[0], b.min[1]}, {b.max[0], b.max[1]}, {b.min[0], b.max[1]},
	}}
	if t.kind == srcLine {
		return hullsDisjoint(&corners, &t.seg)
	}
	return boxCircleSign(b, t.cx, t.cy, t.r2) != 0 || t.wedge.excludes(&corners)
}

// workMeter limits the work of one pair. Each unit is one exact separation test.
type workMeter struct {
	a    *arranger
	left int
	out  bool
}

func (m *workMeter) spend() bool {
	m.a.budget.step(1)
	if m.left <= 0 {
		m.out = true
		return false
	}
	m.left--
	return true
}

type sepStatus uint8

const (
	sepOK sepStatus = iota
	sepFail
	sepBudget
)

// judge returns the verdict of the pair of fit spline i and source j.
func (l *pairLedger) judge(i int, f *ledgerFit, j int) pairEntry {
	t := l.targetAt(j)
	if t == nil {
		return pairEntry{reason: reasonDegenerate}
	}
	pieces := len(f.hulls)
	if t.fit != nil {
		pieces *= len(t.fit.hulls)
	}
	// One contact region costs at most ledgerMaxDepth levels of four children for
	// each pair of starting pieces.
	meter := &workMeter{a: l.a, left: 4 * ledgerMaxDepth * pieces}

	ends, mismatch := l.sharedEnds(f, j, t)
	if mismatch {
		return pairEntry{reason: reasonEndMismatch}
	}
	if len(ends) == 0 {
		if t.farFromBox(f.bounds, f.fbox) {
			return pairEntry{verdict: verdictDisjoint}
		}
		// A circle has no end, and the exact root count settles every pair with it
		// that is not a touch: no root is disjoint, transversal roots are crossings.
		// It costs one test per piece where the avoidance proof halves a hull to the
		// depth limit around every crossing.
		if t.kind == srcCircle {
			return l.crossCircle(i, f, j, t, pieces)
		}
		switch l.avoidsAll(f.hulls, t, meter) {
		case sepOK:
			return pairEntry{verdict: verdictDisjoint}
		case sepBudget:
			return pairEntry{reason: reasonBudget}
		}
		return pairEntry{reason: reasonResolution}
	}
	return l.endpointOnly(f, t, ends, meter)
}

// crossCircle judges a spline whose hulls the avoidance proof could not separate from
// circle j. An exact root count settles it: no root is a disjoint pair, and a set of
// transversal crossings the sampled map confirms is a crossing pair. Anything else is
// unproven with the reason the avoidance proof already found.
func (l *pairLedger) crossCircle(i int, f *ledgerFit, j int, t *ledgerTarget, pieces int) pairEntry {
	meter := &workMeter{a: l.a, left: 4 * ledgerMaxDepth * pieces}
	ts, ok := l.circleCrossings(f, t, meter)
	switch {
	case meter.out:
		return pairEntry{reason: reasonBudget}
	case !ok:
		return pairEntry{reason: reasonResolution}
	case len(ts) == 0:
		return pairEntry{verdict: verdictDisjoint}
	case !l.a.injectCrossings(i, j, ts):
		return pairEntry{reason: reasonResolution}
	}
	return pairEntry{verdict: verdictCrossing, crossings: ts}
}

func (l *pairLedger) targetAt(j int) *ledgerTarget {
	if t, ok := l.targets[j]; ok {
		return t
	}
	var t *ledgerTarget
	if l.a.sources[j].kind == srcFitSpline {
		if f := l.fits[j]; f != nil {
			t = &ledgerTarget{kind: srcFitSpline, fit: f, bounds: f.bounds, fbox: f.fbox}
		}
	} else {
		t = l.target(j)
	}
	l.targets[j] = t
	return t
}

// sharedEnd names one end of the spline that coincides with one end of the other
// source. aEnd and bEnd are 0 for the start and 1 for the finish.
type sharedEnd struct {
	aEnd, bEnd int
	point      [2]float64
}

// sharedEnds lists the ends the spline shares with source j. Two ends count as
// shared when they lie within the identity distance the planar map already uses.
// A spline against a spline additionally needs the two coordinates to be
// bit-identical, and reports mismatch otherwise.
func (l *pairLedger) sharedEnds(f *ledgerFit, j int, t *ledgerTarget) ([]sharedEnd, bool) {
	src := &l.a.sources[j]
	if src.kind == srcCircle {
		return nil, false
	}
	var out []sharedEnd
	eps := weldIdentEps * l.a.scale
	for ae, p := range [2][2]float64{f.p0, f.p1} {
		for be := range 2 {
			q := src.at(float64(be))
			if math.Hypot(p[0]-q[0], p[1]-q[1]) > eps {
				continue
			}
			if t.fit != nil && (p[0] != q[0] || p[1] != q[1]) {
				return nil, true
			}
			out = append(out, sharedEnd{aEnd: ae, bEnd: be, point: p})
		}
	}
	return out, false
}

// avoidsAll reports whether every hull stays off the target.
func (l *pairLedger) avoidsAll(hulls []exactFitHull, t *ledgerTarget, m *workMeter) sepStatus {
	for k := range hulls {
		if s := l.avoids(hulls[k], t, 0, m); s != sepOK {
			return s
		}
	}
	return sepOK
}

// avoids proves that the curve inside hull h does not meet the target.
func (l *pairLedger) avoids(h exactFitHull, t *ledgerTarget, depth int, m *workMeter) sepStatus {
	if t.fit != nil {
		for k := range t.fit.hulls {
			if s := l.hullsApart(h, t.fit.hulls[k], depth, m); s != sepOK {
				return s
			}
		}
		return sepOK
	}
	if ledgerFloatPrefilter && h.fl.ok && h.fl.box().apart(t.fbox) {
		return sepOK
	}
	hb := hullBounds(&h)
	if boundsDisjoint(hb, t.bounds) {
		return sepOK
	}
	if !m.spend() {
		return sepBudget
	}
	switch t.kind {
	case srcLine:
		if hullsDisjoint(&h, &t.seg) {
			return sepOK
		}
	default:
		if boxCircleSign(hb, t.cx, t.cy, t.r2) != 0 || t.wedge.excludes(&h) ||
			hullCircleSign(&h, t.cx, t.cy, t.r2) != 0 {
			return sepOK
		}
	}
	if depth >= ledgerMaxDepth {
		return sepFail
	}
	left, right := splitHullHalf(h)
	if s := l.avoids(left, t, depth+1, m); s != sepOK {
		return s
	}
	return l.avoids(right, t, depth+1, m)
}

// hullsApart proves that two spline pieces do not meet, splitting both on failure.
func (l *pairLedger) hullsApart(a, b exactFitHull, depth int, m *workMeter) sepStatus {
	if ledgerFloatPrefilter && a.fl.ok && b.fl.ok && a.fl.box().apart(b.fl.box()) {
		return sepOK
	}
	ba, bb := hullBounds(&a), hullBounds(&b)
	if boundsDisjoint(ba, bb) {
		return sepOK
	}
	if !m.spend() {
		return sepBudget
	}
	if hullsDisjoint(&a, &b) {
		return sepOK
	}
	if depth >= ledgerMaxDepth {
		return sepFail
	}
	al, ar := splitHullHalf(a)
	bl, br := splitHullHalf(b)
	for _, pair := range [4][2]exactFitHull{{al, bl}, {al, br}, {ar, bl}, {ar, br}} {
		if s := l.hullsApart(pair[0], pair[1], depth+1, m); s != sepOK {
			return s
		}
	}
	return sepOK
}

// endpointOnly proves that the spline meets the target only at the shared ends.
//
// Around each shared end the spline (and, for a spline target, the target) is cut
// at the parameter 2^-m of the end piece. The part beyond the cut must avoid the
// whole other curve, which hullsApart proves. The part before the cut meets the
// other curve at most once and within the vertex-merge distance of the end, which
// localCarrierOK proves for a line, a circle or an arc and localConesApart proves
// for a spline. The smallest m for which the local proof holds is the largest cut,
// and growing m only shrinks the cut, which can only make the far part harder.
func (l *pairLedger) endpointOnly(f *ledgerFit, t *ledgerTarget, ends []sharedEnd, m *workMeter) pairEntry {
	cutA := [2]bool{}
	cutB := [2]bool{}
	var points [][2]float64
	for _, e := range ends {
		cutA[e.aEnd] = true
		cutB[e.bEnd] = true
		points = append(points, e.point)
	}
	start := 1
	if len(f.hulls) == 1 && cutA[0] && cutA[1] {
		start = 2
	}
	if t.fit != nil && len(t.fit.hulls) == 1 && cutB[0] && cutB[1] {
		start = max(start, 2)
	}
	for k := start; k <= ledgerMaxDepth; k++ {
		if !m.spend() {
			return pairEntry{reason: reasonBudget}
		}
		u0 := new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).Lsh(big.NewInt(1), uint(k)))
		farA, cutsA := cutEnds(f.hulls, cutA, u0)
		if !l.localOK(cutsA, cutA, t, cutB, u0, ends) {
			continue
		}
		if s := l.avoidsAll(farA, t, m); s != sepOK {
			return unprovenFrom(s)
		}
		if t.fit != nil {
			farB, _ := cutEnds(t.fit.hulls, cutB, u0)
			aTarget := &ledgerTarget{kind: srcFitSpline, fit: f, bounds: f.bounds, fbox: f.fbox}
			if s := l.avoidsAll(farB, aTarget, m); s != sepOK {
				return unprovenFrom(s)
			}
		}
		return pairEntry{verdict: verdictEndpointOnly, ends: points}
	}
	return pairEntry{reason: reasonTangent}
}

func unprovenFrom(s sepStatus) pairEntry {
	if s == sepBudget {
		return pairEntry{reason: reasonBudget}
	}
	return pairEntry{reason: reasonResolution}
}

// localOK proves the near-end part of the pair for the cut u0.
func (l *pairLedger) localOK(cutsA [2]*exactFitHull, cutA [2]bool, t *ledgerTarget, cutB [2]bool,
	u0 *big.Rat, ends []sharedEnd) bool {
	if t.fit != nil {
		_, cutsB := cutEnds(t.fit.hulls, cutB, u0)
		for _, e := range ends {
			ca, cb := cutsA[e.aEnd], cutsB[e.bEnd]
			if ca == nil || cb == nil || !conesApart(ca, e.aEnd, cb, e.bEnd) {
				return false
			}
		}
		return true
	}
	for e := range 2 {
		if !cutA[e] {
			continue
		}
		apex := 0
		if e == 1 {
			apex = 3
		}
		if cutsA[e] == nil || !localCarrierOK(cutsA[e], apex, t, l.merge) {
			return false
		}
	}
	return true
}

// cutEnds splits the end pieces at 2^-k. It returns the pieces that remain and the
// near-end sub-curves, indexed by end. A spline of one piece cut at both ends
// requires u0 < 1/2.
func cutEnds(hulls []exactFitHull, cut [2]bool, u0 *big.Rat) ([]exactFitHull, [2]*exactFitHull) {
	var cuts [2]*exactFitHull
	far := make([]exactFitHull, 0, len(hulls))
	last := len(hulls) - 1
	one := rat(1)
	zero := rat(0)
	for i, h := range hulls {
		lo, hi := zero, one
		if i == 0 && cut[0] {
			c := subHull(h, zero, u0)
			cuts[0] = &c
			lo = u0
		}
		if i == last && cut[1] {
			from := sub(one, u0)
			c := subHull(h, from, one)
			cuts[1] = &c
			hi = from
		}
		if lo == zero && hi == one {
			far = append(far, h)
			continue
		}
		far = append(far, subHull(h, lo, hi))
	}
	return far, cuts
}

// self decides whether the spline never meets itself. A spline whose every piece
// advances strictly along one fixed direction cannot meet itself, because the
// coordinate along that direction is strictly increasing.
func (l *pairLedger) self(f *ledgerFit) pairEntry {
	if ledgerFloatPrefilter && advancesAlongChord(f) {
		return pairEntry{verdict: verdictSimple}
	}
	dirs := [][2]*big.Rat{{sub(rat(f.p1[0]), rat(f.p0[0])), sub(rat(f.p1[1]), rat(f.p0[1]))}}
	if dirs[0][0] == nil || dirs[0][1] == nil {
		return pairEntry{reason: reasonDegenerate}
	}
	if dirs[0][0].Sign() == 0 && dirs[0][1].Sign() == 0 {
		return pairEntry{reason: reasonSelfContact}
	}
	for _, d := range dirs {
		ok := true
		for k := range f.hulls {
			h := &f.hulls[k]
			for c := range 3 {
				dx := sub(h.points[c+1][0], h.points[c][0])
				dy := sub(h.points[c+1][1], h.points[c][1])
				if add(mul(dx, d[0]), mul(dy, d[1])).Sign() <= 0 {
					ok = false
				}
			}
		}
		if ok {
			return pairEntry{verdict: verdictSimple}
		}
	}
	return pairEntry{reason: reasonSelfContact}
}

// auditPairLedger demotes every proven pair whose proof the sampled map does not
// confirm. The proof is about the exact curves; the planar map is built from chords.
// A disjoint pair must have no sampled contact, and an endpoint-only pair may have
// sampled contacts only within the vertex-merge distance of a shared end.
func (a *arranger) auditPairLedger() {
	for k, e := range a.ledger {
		a.budget.step(1)
		if k[0] == k[1] || e.verdict == verdictUnproven {
			continue
		}
		contacts := a.sampledContacts[k]
		ok := true
		switch e.verdict {
		case verdictDisjoint:
			ok = len(contacts) == 0
		case verdictEndpointOnly:
			for _, c := range contacts {
				near := false
				for _, end := range e.ends {
					if math.Hypot(c[0]-end[0], c[1]-end[1]) <= a.merge {
						near = true
						break
					}
				}
				ok = ok && near
			}
		}
		if !ok {
			a.ledger[k] = pairEntry{reason: reasonAudit}
		}
	}
}
