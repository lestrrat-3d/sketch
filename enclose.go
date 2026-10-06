package sketch

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"slices"

	"github.com/lestrrat-3d/units"
	"github.com/lestrrat-go/option/v3"
)

// ErrUncertifiedConstraint is returned (wrapped, naming the constraint kind) by
// [Sketch.Enclose] when the sketch holds a constraint the certified path has
// no certified form for. One such constraint anywhere in the sketch refuses the
// whole sketch, driving or driven, whether or not it touches the loop.
var ErrUncertifiedConstraint = errors.New("sketch: constraint kind has no certified form")

// ErrNotCertified is returned (wrapped, naming the range and the failed test)
// by [Sketch.Enclose] when no enclosure could be proven: the Krawczyk test did
// not close, the Jacobian at a float solution was singular, an angle's
// direction condition failed over a box, a driven dimension could not be
// enclosed, or the piece budget ran out. It is also returned for a call that
// cannot be answered as asked (an invalid range, a driver that is not a
// driving constraint of the sketch, a continuation that does not start where
// the previous enclosure ended). It never carries an enclosure.
var ErrNotCertified = errors.New("sketch: enclosure not certified")

// EncloseOption tunes [Sketch.Enclose].
type EncloseOption interface {
	option.Interface
	encloseOption()
}

type encloseOption struct{ option.Interface }

func (encloseOption) encloseOption() {}

type (
	identContinuation struct{}
	identMaxPieces    struct{}
)

// WithContinuation continues the branch a previous enclosure certified. The
// new call must start where prev ended (lo equal to prev's upper driving
// value, exactly), use the same driver, and run on the same unchanged sketch.
// It is seeded from prev's float solution at that value instead of the
// sketch's current geometry, and it proves that prev's certified box at the
// shared value lies inside the new first piece's uniqueness box, so the two
// enclosures describe one continuous branch.
func WithContinuation(prev *Enclosure) EncloseOption {
	return encloseOption{option.New(identContinuation{}, prev)}
}

// WithMaxPieces caps how many pieces one [Sketch.Enclose] call may split its
// range into before refusing. The default is 4096.
func WithMaxPieces(n int) EncloseOption {
	return encloseOption{option.New(identMaxPieces{}, n)}
}

type encloseConfig struct {
	prev      *Enclosure
	maxPieces int
}

func defaultEncloseConfig() encloseConfig { return encloseConfig{maxPieces: 4096} }

// Enclosure is a certified enclosure of a sketch's exact solution over a range
// of one driving dimension's value, returned by [Sketch.Enclose].
//
// It is a list of pieces, each covering a sub-range of the driving value; the
// pieces cover the whole range in order, adjacent pieces sharing an endpoint.
// For each piece the claims are about the EXACT real solution of the sketch's
// constraint equations (every other dimension reading its float64 target in
// base units, the driver reading every value in the piece's sub-range):
//
//   - for every driving value q in the sub-range there is exactly one solution
//     inside the piece's box, and it moves continuously with q;
//   - every coordinate that solution takes lies in the piece's box, and every
//     driven dimension's measured value lies in the piece's interval for it.
//
// Adjacent pieces are proven to hold the same solution at their shared value,
// so across the whole range the solution is one continuous function of q. The
// whole-range readings ([Enclosure.PointBox], [Enclosure.Driven]) are the hulls
// of the pieces' readings; uniqueness is claimed inside each piece's box, never
// inside the hull.
//
// Every endpoint is outward rounded (see [Interval]). Nothing is claimed about
// solutions outside the boxes.
type Enclosure struct {
	s       *Sketch
	driver  Dimension
	rng     Interval
	pieces  []EnclosurePiece
	fp      uint64
	endVars []float64
	endBox  []Interval
}

// EnclosurePiece is one piece of an [Enclosure]: a sub-range of the driving
// value and the certified box over it.
type EnclosurePiece struct {
	s      *Sketch
	rng    Interval
	box    []Interval
	driven []drivenValue
}

type drivenValue struct {
	d Dimension
	v Interval
}

// Range returns the piece's sub-range of driving values, in base units.
func (p EnclosurePiece) Range() Interval { return p.rng }

// PointBox returns the enclosures of p's x and y coordinates over the piece.
// It reports false for a nil point or one the sketch does not own. A fixed
// point's box is its exact coordinate.
func (p EnclosurePiece) PointBox(pnt *Point) (Interval, Interval, bool) {
	if pnt == nil || p.s == nil || !p.s.owns(pnt) || pnt.xi >= len(p.box) || pnt.yi >= len(p.box) {
		return Interval{}, Interval{}, false
	}
	return p.box[pnt.xi], p.box[pnt.yi], true
}

// Driven returns the enclosure of driven dimension d's measured value over the
// piece, in base units (mm or rad). An angle is a reading mod 2π, shifted by
// whole turns to stay continuous from the dimension's target at the time of
// the call. It reports false for a dimension that was not driven in the
// sketch.
func (p EnclosurePiece) Driven(d Dimension) (Interval, bool) {
	for _, dv := range p.driven {
		if dv.d == d {
			return dv.v, true
		}
	}
	return Interval{}, false
}

// Range returns the driving range the enclosure covers, in base units.
func (e *Enclosure) Range() Interval { return e.rng }

// Pieces returns the enclosure's pieces in driving-value order.
func (e *Enclosure) Pieces() []EnclosurePiece { return slices.Clone(e.pieces) }

// PointBox returns the hull, over every piece, of p's coordinate enclosures:
// every coordinate p's exact solution takes across the whole range lies in it.
// It reports false for a nil point or one the sketch does not own.
func (e *Enclosure) PointBox(p *Point) (Interval, Interval, bool) {
	var x, y Interval
	for i, pc := range e.pieces {
		px, py, ok := pc.PointBox(p)
		if !ok {
			return Interval{}, Interval{}, false
		}
		if i == 0 {
			x, y = px, py
			continue
		}
		x, y = ihull(x, px), ihull(y, py)
	}
	return x, y, len(e.pieces) > 0
}

// Driven returns the hull, over every piece, of driven dimension d's measured
// value. For an angle the pieces are shifted to stay continuous, so the hull's
// width is how far the angle travels across the range.
func (e *Enclosure) Driven(d Dimension) (Interval, bool) {
	var out Interval
	for i, pc := range e.pieces {
		v, ok := pc.Driven(d)
		if !ok {
			return Interval{}, false
		}
		if i == 0 {
			out = v
			continue
		}
		out = ihull(out, v)
	}
	return out, len(e.pieces) > 0
}

// IsStale reports whether the sketch has changed since the enclosure was
// computed: its variables, entities, grounding, constraint set, or any
// dimension's target or driven flag. A stale enclosure describes equations the
// sketch no longer has.
func (e *Enclosure) IsStale() bool { return e.s.encloseFingerprint() != e.fp }

// Enclose certifies the sketch's exact solution while driver's value runs over
// [lo, hi], given in base units (mm for a length, rad for an angle) and read
// as the exact rationals the two float64 values represent; lo == hi asks about
// one value. The driver must be a driving [Angle], [Distance],
// [HorizontalDistance] or [VerticalDistance] committed to this sketch.
//
// The solution followed is the one the solver reaches from the sketch's
// current geometry at lo (or, with [WithContinuation], the one a previous
// enclosure ended on), carried across the range by continuation. The range is
// split into pieces as needed; each piece passes a parametric Krawczyk test in
// outward-rounded interval arithmetic. See [Enclosure] for the claims.
//
// Enclose works on a certified restatement of each constraint, so it supports
// only coincident, horizontal, vertical, horizontal and vertical points,
// parallel, perpendicular, point-on-line, collinear, distance, horizontal and
// vertical distance, and angle constraints; driven dimensions may be angles or
// distances. Any other constraint kind in the sketch refuses with
// [ErrUncertifiedConstraint]. A refusal never carries an enclosure, and its
// cause is a wrapped sentinel:
//
//   - [ErrNotConverged]: the float solve failed at lo.
//   - [ErrUnderconstrained]: degrees of freedom remain once the driver is held.
//   - [ErrRedundant]: there are more equations than unknowns.
//   - [ErrUncertifiedConstraint]: see above.
//   - [ErrNotCertified]: no enclosure could be proven over some sub-range.
//   - [ErrNonFiniteGeometry], [ErrForeignHandle]: the sketch is unreadable.
//   - ctx.Err(): the context ended.
//
// Enclose moves the sketch's variables and the driver's target while it
// works and restores both exactly before returning, so the sketch reads as
// untouched afterwards ([Sketch.Revision] is unchanged); it must not run
// concurrently with any other call on the same sketch. Two calls on the same
// sketch state with the same arguments return bit-identical enclosures.
func (s *Sketch) Enclose(ctx context.Context, driver Dimension, lo, hi float64, options ...EncloseOption) (*Enclosure, error) {
	cfg := defaultEncloseConfig()
	for _, opt := range options {
		switch opt.Ident().(type) {
		case identContinuation:
			cfg.prev = option.MustGet[*Enclosure](opt)
		case identMaxPieces:
			cfg.maxPieces = option.MustGet[int](opt)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if driver == nil || isNilConstraint(driver) {
		return nil, fmt.Errorf("%w: nil driver", ErrNotCertified)
	}
	if !(lo <= hi) || math.IsInf(lo, 0) || math.IsInf(hi, 0) {
		return nil, fmt.Errorf("%w: [%v, %v] is not a finite ordered range", ErrNotCertified, lo, hi)
	}
	for _, c := range s.cons {
		if corruptConstraint(c) {
			return nil, fmt.Errorf("%w: a constraint has a nil operand", ErrNotCertified)
		}
		if s.foreignConstraint(c) {
			return nil, fmt.Errorf("%w: a constraint references another sketch's geometry", ErrForeignHandle)
		}
	}
	if s.hasNonFiniteVars() {
		return nil, fmt.Errorf("%w: the sketch holds a non-finite value", ErrNonFiniteGeometry)
	}
	sys, err := s.certifiedSystem(driver)
	if err != nil {
		return nil, err
	}
	db, err := driverBase(driver, lo, hi)
	if err != nil {
		return nil, err
	}
	if prev := cfg.prev; prev != nil {
		switch {
		case prev.s != s:
			return nil, fmt.Errorf("%w: the continued enclosure belongs to another sketch", ErrNotCertified)
		case prev.driver != driver:
			return nil, fmt.Errorf("%w: the continued enclosure has a different driver", ErrNotCertified)
		case prev.IsStale():
			return nil, fmt.Errorf("%w: the continued enclosure is stale", ErrNotCertified)
		case prev.rng.Hi != lo:
			return nil, fmt.Errorf("%w: the range starts at %v, the continued enclosure ends at %v", ErrNotCertified, lo, prev.rng.Hi)
		}
	}

	fp := s.encloseFingerprint()
	savedVars := slices.Clone(s.vars)
	savedDim := *db
	defer func() {
		copy(s.vars, savedVars)
		*db = savedDim
	}()

	free := s.freeVars()
	col := make([]int, len(s.vars))
	for i := range col {
		col[i] = -1
	}
	for j, vi := range free {
		col[vi] = j
	}
	r := &encloseRun{s: s, sys: sys, db: db, free: free, col: col, maxPieces: cfg.maxPieces}
	e, err := r.run(ctx, lo, hi, cfg.prev)
	if err != nil {
		return nil, err
	}
	e.s, e.driver, e.rng, e.fp = s, driver, Interval{lo, hi}, fp
	return e, nil
}

// driverBase returns the dimension state Enclose rewrites to move the driving
// value, refusing a driver kind or range the certified path cannot take.
func driverBase(driver Dimension, lo, hi float64) (*dimBase, error) {
	switch t := driver.(type) {
	case *Angle:
		if math.Abs(lo) > maxTrigArg || math.Abs(hi) > maxTrigArg {
			return nil, fmt.Errorf("%w: angle range [%v, %v] rad is outside ±%d rad", ErrNotCertified, lo, hi, maxTrigArg)
		}
		return &t.dimBase, nil
	case *Distance:
		if !(lo > 0) {
			return nil, fmt.Errorf("%w: distance range [%v, %v] is not positive", ErrNotCertified, lo, hi)
		}
		return &t.dimBase, nil
	case *HorizontalDistance:
		return &t.dimBase, nil
	case *VerticalDistance:
		return &t.dimBase, nil
	}
	return nil, fmt.Errorf("%w: %s cannot drive an enclosure", ErrUncertifiedConstraint, ConstraintKind(driver))
}

// encloseRun is the per-call state of one Enclose call.
type encloseRun struct {
	s         *Sketch
	sys       *certSystem
	db        *dimBase
	free, col []int
	maxPieces int
}

// setDriver makes the driver's target exactly q in base units.
func (r *encloseRun) setDriver(q float64) {
	u := units.Millimeter
	if r.db.kind == units.Angle {
		u = units.Radian
	}
	r.db.target = units.New(q, u)
}

// solveAt runs the float solver at driving value q from seed and returns the
// solved variables, or an error wrapping ErrNotConverged (or the context's).
func (r *encloseRun) solveAt(ctx context.Context, seed []float64, q float64) ([]float64, error) {
	s := r.s
	copy(s.vars, seed)
	r.setDriver(q)
	if _, err := s.lm(ctx, r.free, s.residuals, localJacobian, 200, 1e-10); err != nil {
		return nil, err
	}
	res := s.residuals(nil)
	norm := math.Sqrt(dot(res, res))
	if !(norm <= 1e-10) {
		return nil, fmt.Errorf("%w: residual %g at driving value %v", ErrNotConverged, norm, q)
	}
	return slices.Clone(s.vars), nil
}

// checkStructure refuses a system that is not square and of full rank at the
// float solution x (solved at q): the Krawczyk test needs exactly as many
// equations as unknowns.
func (r *encloseRun) checkStructure(x []float64, q float64) error {
	s := r.s
	copy(s.vars, x)
	r.setDriver(q)
	n := len(r.free)
	m := len(s.residuals(nil))
	if m != r.sys.m {
		return fmt.Errorf("%w: %d certified equations for %d residual rows", ErrNotCertified, r.sys.m, m)
	}
	rk := 0
	if m > 0 {
		var ok bool
		rk, ok = s.rank(r.free, m)
		if !ok {
			return fmt.Errorf("%w: the sketch holds a non-finite value", ErrNonFiniteGeometry)
		}
	}
	if dof := n - rk; dof > 0 {
		return fmt.Errorf("%w (DOF 0 needed, %d remaining)", ErrUnderconstrained, dof)
	}
	if m > n {
		return fmt.Errorf("%w: %d equations for %d unknowns", ErrRedundant, m, n)
	}
	return nil
}

// run certifies [lo, hi] piece by piece. Each piece [a, b] is solved at its
// midpoint, passes the parametric Krawczyk test over [a, b], and is tied to its
// neighbours: the certified point box at a (carried from the previous piece)
// and the one at b must both lie in the piece's uniqueness box, so the
// solution each neighbour holds at the shared value is the one this piece
// holds. A piece that fails is halved; a piece narrower than 2^-30 of the
// range refuses the call. A piece that succeeds doubles the next attempt.
func (r *encloseRun) run(ctx context.Context, lo, hi float64, prev *Enclosure) (*Enclosure, error) {
	var x0 []float64
	var p0 []Interval
	refs := r.drivenRefs(prev)
	if prev != nil {
		x0, p0 = slices.Clone(prev.endVars), prev.endBox
	} else {
		var err error
		x0, err = r.solveAt(ctx, r.s.vars, lo)
		if err != nil {
			return nil, err
		}
	}
	if err := r.checkStructure(x0, lo); err != nil {
		return nil, err
	}
	if p0 == nil {
		K, _, why := r.sys.krawczyk(x0, pt(lo), lo, r.free, r.col)
		if K == nil {
			return nil, fmt.Errorf("%w: at %v: %s", ErrNotCertified, lo, why)
		}
		p0 = K
	}

	e := &Enclosure{}
	if lo == hi {
		pc, err := r.piece(pt(lo), p0, refs)
		if err != nil {
			return nil, err
		}
		e.pieces, e.endVars, e.endBox = []EnclosurePiece{pc}, x0, p0
		return e, nil
	}

	minStep := (hi - lo) * 0x1p-30
	cur, xcur, pcur := lo, x0, p0
	step := hi - lo
	for cur < hi {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(e.pieces) >= r.maxPieces {
			return nil, fmt.Errorf("%w: piece budget of %d exhausted at %v", ErrNotCertified, r.maxPieces, cur)
		}
		b := cur + step
		if b >= hi || hi-b < minStep {
			b = hi
		}
		K, xb, pb, why, err := r.tryPiece(ctx, cur, b, xcur, pcur)
		if err != nil {
			return nil, err
		}
		if K == nil {
			step = (b - cur) / 2
			if step < minStep || !(cur+step > cur) {
				return nil, fmt.Errorf("%w: over [%v, %v]: %s", ErrNotCertified, cur, b, why)
			}
			continue
		}
		pc, err := r.piece(Interval{cur, b}, K, refs)
		if err != nil {
			return nil, err
		}
		e.pieces = append(e.pieces, pc)
		step = 2 * (b - cur)
		cur, xcur, pcur = b, xb, pb
	}
	e.endVars, e.endBox = xcur, pcur
	return e, nil
}

// tryPiece attempts one piece over [a, b]. It returns the piece's box with the
// float solution and certified point box at b, or a nil box and the reason the
// attempt failed. Only a context error is returned as an error.
func (r *encloseRun) tryPiece(ctx context.Context, a, b float64, xa []float64, pa []Interval) ([]Interval, []float64, []Interval, string, error) {
	mid := a + (b-a)/2
	xm, err := r.solveAt(ctx, xa, mid)
	if err != nil {
		if errors.Is(err, ErrNotConverged) {
			return nil, nil, nil, err.Error(), nil
		}
		return nil, nil, nil, "", err
	}
	K, X, why := r.sys.krawczyk(xm, Interval{a, b}, mid, r.free, r.col)
	if K == nil {
		return nil, nil, nil, why, nil
	}
	if !boxWithin(pa, X) {
		return nil, nil, nil, "the solution at the range start is outside the box", nil
	}
	xb, err := r.solveAt(ctx, xm, b)
	if err != nil {
		if errors.Is(err, ErrNotConverged) {
			return nil, nil, nil, err.Error(), nil
		}
		return nil, nil, nil, "", err
	}
	pb, _, why := r.sys.krawczyk(xb, pt(b), b, r.free, r.col)
	if pb == nil {
		return nil, nil, nil, why, nil
	}
	if !boxWithin(pb, X) {
		return nil, nil, nil, "the solution at the range end is outside the box", nil
	}
	if !r.tightEnough(K, pa, pb, xm) {
		return nil, nil, nil, "the box is looser than the motion it covers", nil
	}
	return K, xb, pb, "", nil
}

// Piece tightness. A Krawczyk box over a sub-range overshoots the path the
// solution actually takes by a term that shrinks with the square of the
// sub-range's width, and a consumer charges the box's whole width as travel.
// So a certified piece is still split when, for some free variable, the box
// reaches beyond the hull of its two endpoint boxes by more than
// pieceSlackRel of that hull's width plus pieceSlackAbs of the sketch's
// coordinate scale. The excess also counts the variable's genuine excursion
// past its endpoints (a follower turning back inside the piece), which shrinks
// the same way, so the split always terminates.
const (
	pieceSlackRel = 0.05
	pieceSlackAbs = 1e-7
)

func (r *encloseRun) tightEnough(K, pa, pb []Interval, xm []float64) bool {
	scale := 1.0
	for _, vi := range r.free {
		scale = max(scale, 1+math.Abs(xm[vi]))
	}
	for _, vi := range r.free {
		h := ihull(pa[vi], pb[vi])
		excess := max(h.Lo-K[vi].Lo, 0) + max(K[vi].Hi-h.Hi, 0)
		if excess > pieceSlackRel*(h.Hi-h.Lo)+pieceSlackAbs*scale {
			return false
		}
	}
	return true
}

func boxWithin(a, b []Interval) bool {
	for i := range a {
		if !a[i].within(b[i]) {
			return false
		}
	}
	return true
}

// drivenRefs seeds the whole-turn reference each driven angle's enclosure is
// shifted toward: the dimension's current target, or the last piece of the
// enclosure being continued.
func (r *encloseRun) drivenRefs(prev *Enclosure) []float64 {
	refs := make([]float64, len(r.sys.driven))
	for i, dv := range r.sys.driven {
		refs[i] = dv.d.base()
		if prev == nil || len(prev.pieces) == 0 {
			continue
		}
		if v, ok := prev.pieces[len(prev.pieces)-1].Driven(dv.d); ok {
			refs[i] = v.mid()
		}
	}
	return refs
}

// piece assembles one EnclosurePiece, enclosing every driven dimension over
// the box and advancing each angle's whole-turn reference.
func (r *encloseRun) piece(q Interval, box []Interval, refs []float64) (EnclosurePiece, error) {
	e := &certEnv{box: box, q: q}
	pc := EnclosurePiece{s: r.s, rng: q, box: box}
	for i, dv := range r.sys.driven {
		v, ok := dv.eval(e)
		if !ok || !v.isFinite() {
			return EnclosurePiece{}, fmt.Errorf("%w: over [%v, %v]: the driven %s could not be enclosed", ErrNotCertified, q.Lo, q.Hi, ConstraintKind(dv.d))
		}
		if _, isAngle := dv.d.(*Angle); isAngle {
			v = shiftNear(v, refs[i])
			refs[i] = v.mid()
		}
		pc.driven = append(pc.driven, drivenValue{d: dv.d, v: v})
	}
	return pc, nil
}

// encloseFingerprint extends [Sketch.Revision] with everything else the
// certified equations read: the grounding flags and, per constraint, its kind,
// the variables its operands name, and a dimension's target and driven flag.
func (s *Sketch) encloseFingerprint() uint64 {
	h := fnv.New64a()
	var buf [8]byte
	write := func(u uint64) {
		binary.LittleEndian.PutUint64(buf[:], u)
		_, _ = h.Write(buf[:])
	}
	writePoint := func(p *Point) {
		if p == nil {
			write(math.MaxUint64)
			return
		}
		write(uint64(p.xi))
		write(uint64(p.yi))
	}
	write(s.Revision())
	for _, f := range s.fixed {
		if f {
			write(1)
			continue
		}
		write(0)
	}
	write(uint64(len(s.cons)))
	for _, c := range s.cons {
		_, _ = h.Write([]byte(ConstraintKind(c)))
		if isNilConstraint(c) {
			continue
		}
		pts, ents := constraintRefs(c)
		for _, p := range pts {
			writePoint(p)
		}
		for _, en := range ents {
			if isNilEntity(en) {
				write(math.MaxUint64)
				continue
			}
			for _, p := range entityPoints(en) {
				writePoint(p)
			}
		}
		if d, ok := c.(Dimension); ok {
			write(math.Float64bits(d.base()))
			if d.Driven() {
				write(1)
				continue
			}
			write(0)
		}
	}
	return h.Sum64()
}
