package sketch

import (
	"context"
	"errors"
	"math"
	"math/big"

	"github.com/lestrrat-3d/sketch/geom"
)

// JunctionCharge identifies the shared source-wall work done by the batch proof.
type JunctionCharge uint8

const (
	JunctionRawVisit JunctionCharge = iota + 1
	JunctionRootProof
)

// JunctionWork charges both raw publication checks and exact root operations.
type JunctionWork interface {
	Step(JunctionCharge) error
	Err() error
}

// JunctionDecline records the first raw-publication check that refused a batch.
// Source indices are arrangement positions; Entity handles identify the callers'
// original tagged sources. The diagnostic does not change proof admission.
type JunctionDecline struct {
	Predicate                         string
	Profile, Loop, Edge               int
	LeftSourceIndex, RightSourceIndex int
	LeftEntity, RightEntity           Entity
}

// JunctionDeclineSink is an optional bounded diagnostic observer.
type JunctionDeclineSink interface{ JunctionDeclined(JunctionDecline) }

func quarterDecline(work JunctionWork, predicate string, profile, loop, edge int,
	left, right BoundaryEdge) ([]QuarterArcJunction, bool, error) {
	if sink, ok := work.(JunctionDeclineSink); ok {
		sink.JunctionDeclined(JunctionDecline{
			Predicate: predicate, Profile: profile, Loop: loop, Edge: edge,
			LeftSourceIndex: left.sourceIndex, RightSourceIndex: right.sourceIndex,
			LeftEntity: left.Entity, RightEntity: right.Entity,
		})
	}
	return nil, false, nil
}

// JunctionOccurrence locates one walked raw-profile appearance of a contact.
// Loop is -1 for Outer; otherwise it is a zero-based hole index.
type JunctionOccurrence struct {
	Profile, Loop, LeftEdge, RightEdge int
}

// QuarterArcJunction encloses one authenticated Line/Arc contact.
type QuarterArcJunction struct {
	sources     [2]JunctionSource
	point       [2]RationalBracket
	params      [2]RationalBracket // walked source order, in natural domains
	branch      int8
	occurrences []JunctionOccurrence
}

func (w QuarterArcJunction) Sources() [2]JunctionSource { return w.sources }
func (w QuarterArcJunction) Point() (RationalBracket, RationalBracket) {
	return w.point[0], w.point[1]
}

// Parameters returns exact enclosures in the walked Sources order.
func (w QuarterArcJunction) Parameters() (RationalBracket, RationalBracket) {
	return w.params[0], w.params[1]
}
func (w QuarterArcJunction) Branch() int8 { return w.branch }
func (w QuarterArcJunction) Occurrences() []JunctionOccurrence {
	return append([]JunctionOccurrence(nil), w.occurrences...)
}

type quarterCandidate struct {
	cross       *geom.LineArcCrossing
	pair        [2]int // line, then arc, independent of the walked order
	left, right BoundaryEdge
	occurrences []JunctionOccurrence
}

// CertifyQuarterArcJunctions proves at most two retained, interior Line/Arc
// contacts from one complete, live Profiles publication. An empty result
// proves no contact; it does not prove that no unsupported arc crosses a
// source. Callers must check the number and roles of contacts they require.
// The caller must keep raw profiles unchanged during this call.
// This work limit starts after Profiles has built the publication.
// A sketch containing NURBS declines because its revision has variable-length
// knot and weight scans that the current revision charge cannot bound.
func (s *Sketch) CertifyQuarterArcJunctions(ctx context.Context, raw []*Profile,
	work JunctionWork) ([]QuarterArcJunction, bool, error) {
	if ctx == nil || work == nil {
		return nil, false, ErrInvalidJunctionContext
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if err := work.Err(); err != nil {
		return nil, false, err
	}
	if s == nil || len(raw) == 0 || raw[0] == nil ||
		raw[0].junctionPublication == nil ||
		len(raw) != raw[0].junctionPublication.count {
		return quarterDecline(work, "publication", -1, -1, -1,
			BoundaryEdge{}, BoundaryEdge{})
	}
	publication := raw[0].junctionPublication
	chargeRaw := func(n uint64) error {
		for i := uint64(0); i < n; i++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := work.Step(JunctionRawVisit); err != nil {
				return err
			}
		}
		return nil
	}
	boundedRevision := func() (uint64, error) {
		return s.revisionWithChargeGuard(chargeRaw, func(entity Entity) error {
			if _, ok := entity.(*NURBS); ok {
				return errJunctionMiss
			}
			return nil
		})
	}
	revision, err := boundedRevision()
	if errors.Is(err, errJunctionMiss) {
		return quarterDecline(work, "sketch.nurbs", -1, -1, -1,
			BoundaryEdge{}, BoundaryEdge{})
	}
	if errors.Is(err, errRevisionChanged) {
		return quarterDecline(work, "publication.stale", -1, -1, -1,
			BoundaryEdge{}, BoundaryEdge{})
	}
	if err != nil {
		return nil, false, err
	}
	candidates := make(map[*geom.LineArcCrossing]*quarterCandidate)
	ordered := make([]*quarterCandidate, 0, 2)
	totalEdges, totalPoints := 0, 0
	for pi, p := range raw {
		if p == nil {
			return quarterDecline(work, "profile.nil", pi, -1, -1,
				BoundaryEdge{}, BoundaryEdge{})
		}
		if p.sketch != s {
			return quarterDecline(work, "profile.sketch", pi, -1, -1,
				BoundaryEdge{}, BoundaryEdge{})
		}
		if p.revision != revision {
			return quarterDecline(work, "profile.revision", pi, -1, -1,
				BoundaryEdge{}, BoundaryEdge{})
		}
		if p.junctionPublication != publication {
			return quarterDecline(work,
				"profile.publication", pi, -1, -1, BoundaryEdge{}, BoundaryEdge{})
		}
		if p.junctionProfileIndex != pi {
			return quarterDecline(work,
				"profile.index", pi, -1, -1, BoundaryEdge{}, BoundaryEdge{})
		}
		if !p.Valid {
			return quarterDecline(work, "profile.valid", pi, -1, -1,
				BoundaryEdge{}, BoundaryEdge{})
		}
		if len(p.Holes) >= 128 {
			return quarterDecline(work, "profile.hole_limit", pi, -1, -1,
				BoundaryEdge{}, BoundaryEdge{})
		}
		loops := append([][]BoundaryEdge{p.Outer}, p.Holes...)
		for li, loop := range loops {
			loopIndex := li - 1
			if len(loop) == 0 {
				return quarterDecline(work, "loop.empty", pi,
					loopIndex, -1, BoundaryEdge{}, BoundaryEdge{})
			}
			if len(loop) > 128-totalEdges {
				return quarterDecline(work, "loop.edge_limit", pi,
					loopIndex, -1, BoundaryEdge{}, BoundaryEdge{})
			}
			totalEdges += len(loop)
			walk := loop[0].junctionWalk
			if walk == nil {
				return quarterDecline(work, "loop.walk_nil", pi,
					loopIndex, -1, loop[0], BoundaryEdge{})
			}
			if !walk.closed {
				return quarterDecline(work, "loop.walk_open", pi,
					loopIndex, -1, loop[0], BoundaryEdge{})
			}
			if walk.length != len(loop) {
				return quarterDecline(work, "loop.walk_length", pi,
					loopIndex, -1, loop[0], BoundaryEdge{})
			}
			if walk.owner != p {
				return quarterDecline(work, "loop.walk_owner", pi,
					loopIndex, -1, loop[0], BoundaryEdge{})
			}
			if walk.loop != loopIndex {
				return quarterDecline(work, "loop.walk_index", pi,
					loopIndex, -1, loop[0], BoundaryEdge{})
			}
			for ei, e := range loop {
				if err := ctx.Err(); err != nil {
					return nil, false, err
				}
				if err := work.Step(JunctionRawVisit); err != nil {
					return nil, false, err
				}
				if len(e.Polyline) > 1024-totalPoints {
					return quarterDecline(work,
						"edge.point_limit", pi, loopIndex, ei, e, BoundaryEdge{})
				}
				totalPoints += len(e.Polyline)
				for range e.Polyline {
					if err := work.Step(JunctionRawVisit); err != nil {
						return nil, false, err
					}
				}
				if e.sketch != s {
					return quarterDecline(work, "edge.sketch",
						pi, loopIndex, ei, e, BoundaryEdge{})
				}
				if e.revision != p.revision {
					return quarterDecline(work,
						"edge.revision", pi, loopIndex, ei, e, BoundaryEdge{})
				}
				if e.junctionWalk != walk {
					return quarterDecline(work,
						"edge.walk", pi, loopIndex, ei, e, BoundaryEdge{})
				}
				if e.junctionIndex != ei {
					return quarterDecline(work,
						"edge.index", pi, loopIndex, ei, e, BoundaryEdge{})
				}
				if e.Entity != e.sourceEntity {
					return quarterDecline(work,
						"edge.source_entity", pi, loopIndex, ei, e, BoundaryEdge{})
				}
				if !s.ownsEntity(e.Entity) {
					return quarterDecline(work,
						"edge.ownership", pi, loopIndex, ei, e, BoundaryEdge{})
				}
				if (e.arcStart != nil || e.arcEnd != nil) && !e.matchesSnapshot() {
					return quarterDecline(work,
						"edge.snapshot", pi, loopIndex, ei, e, BoundaryEdge{})
				}
				next := loop[(ei+1)%len(loop)]
				if err := work.Step(JunctionRawVisit); err != nil {
					return nil, false, err
				}
				if e.arcEnd == nil && next.arcStart == nil {
					continue
				}
				if e.arcEnd == nil {
					return quarterDecline(work,
						"junction.left_token_nil", pi, loopIndex, ei, e, next)
				}
				if e.arcEnd != next.arcStart {
					return quarterDecline(work,
						"junction.token_mismatch", pi, loopIndex, ei, e, next)
				}
				if !e.Partial {
					return quarterDecline(work,
						"junction.left_whole", pi, loopIndex, ei, e, next)
				}
				if !next.Partial {
					return quarterDecline(work,
						"junction.right_whole", pi, loopIndex, ei, e, next)
				}
				if !e.TExact {
					return quarterDecline(work,
						"junction.left_inexact", pi, loopIndex, ei, e, next)
				}
				if !next.TExact {
					return quarterDecline(work,
						"junction.right_inexact", pi, loopIndex, ei, e, next)
				}
				if len(e.Polyline) < 2 {
					return quarterDecline(work,
						"junction.left_polyline", pi, loopIndex, ei, e, next)
				}
				if len(next.Polyline) < 2 {
					return quarterDecline(work,
						"junction.right_polyline", pi, loopIndex, ei, e, next)
				}
				cross := e.arcEnd
				pair := cross.SourceIndices()
				if e.sourceIndex == next.sourceIndex {
					return quarterDecline(work,
						"junction.same_source", pi, loopIndex, ei, e, next)
				}
				if e.sourceIndex != pair[0] && e.sourceIndex != pair[1] {
					return quarterDecline(work, "junction.left_source",
						pi, loopIndex, ei, e, next)
				}
				if next.sourceIndex != pair[0] && next.sourceIndex != pair[1] {
					return quarterDecline(work, "junction.right_source",
						pi, loopIndex, ei, e, next)
				}
				if e.Polyline[len(e.Polyline)-1] != cross.Point() {
					return quarterDecline(work, "junction.left_vertex",
						pi, loopIndex, ei, e, next)
				}
				if next.Polyline[0] != cross.Point() {
					return quarterDecline(work, "junction.right_vertex",
						pi, loopIndex, ei, e, next)
				}
				_, eLine := e.Entity.(*Line)
				_, eArc := e.Entity.(*Arc)
				_, nLine := next.Entity.(*Line)
				_, nArc := next.Entity.(*Arc)
				validKinds := eLine && nArc || eArc && nLine
				if !validKinds {
					return quarterDecline(work,
						"junction.source_kinds", pi, loopIndex, ei, e, next)
				}
				canonical := [2]int{e.sourceIndex, next.sourceIndex}
				if eArc {
					canonical = [2]int{next.sourceIndex, e.sourceIndex}
				}
				if canonical != pair {
					return quarterDecline(work, "junction.canonical_pair",
						pi, loopIndex, ei, e, next)
				}
				c := candidates[cross]
				if c == nil {
					if len(ordered) >= 2 {
						return quarterDecline(work,
							"junction.distinct_limit", pi, loopIndex, ei, e, next)
					}
					c = &quarterCandidate{cross: cross, pair: canonical,
						left: e, right: next}
					candidates[cross] = c
					ordered = append(ordered, c)
				} else if c.pair != canonical ||
					cross.Branch() != c.cross.Branch() {
					return quarterDecline(work, "junction.group_consistency",
						pi, loopIndex, ei, e, next)
				}
				c.occurrences = append(c.occurrences,
					JunctionOccurrence{pi, loopIndex, ei, (ei + 1) % len(loop)})
			}
		}
	}
	result := make([]QuarterArcJunction, 0, len(ordered))
	for _, c := range ordered {
		w, err := s.certifyQuarterCandidate(ctx, work, c)
		if err != nil {
			if errors.Is(err, errJunctionMiss) {
				return nil, false, nil
			}
			return nil, false, err
		}
		result = append(result, w)
	}
	if err := work.Err(); err != nil {
		return nil, false, err
	}
	// Charge the complete final read before it starts. Work callbacks may edit
	// the sketch, so a scan that calls Step while hashing cannot seal success:
	// its last callback can change state that the scan has already read.
	entityCount := len(s.ents)
	if err := chargeRaw(uint64(entityCount)); err != nil {
		return nil, false, err
	}
	if len(s.ents) != entityCount {
		return quarterDecline(work, "publication.stale", -1, -1, -1,
			BoundaryEdge{}, BoundaryEdge{})
	}
	revisionCost := func() (uint64, error) {
		cost := uint64(len(s.vars)) + uint64(len(s.points)) +
			1 + uint64(len(s.ents))
		for _, entity := range s.ents {
			if _, ok := entity.(*NURBS); ok {
				return 0, errJunctionMiss
			}
			cost += uint64(len(entityPoints(entity)))
		}
		return cost, nil
	}
	cost, err := revisionCost()
	if err == nil {
		err = chargeRaw(cost)
	}
	if err == nil && len(s.ents) != entityCount {
		err = errRevisionChanged
	}
	if err == nil {
		var after uint64
		after, err = revisionCost()
		if err == nil && after > cost {
			err = errRevisionChanged
		}
	}
	var current uint64
	if err == nil {
		current, err = s.revisionWithChargeGuard(nil, func(entity Entity) error {
			if _, ok := entity.(*NURBS); ok {
				return errJunctionMiss
			}
			return nil
		})
	}
	if errors.Is(err, errJunctionMiss) {
		return quarterDecline(work, "sketch.nurbs", -1, -1, -1,
			BoundaryEdge{}, BoundaryEdge{})
	}
	if errors.Is(err, errRevisionChanged) {
		return quarterDecline(work, "publication.stale", -1, -1, -1,
			BoundaryEdge{}, BoundaryEdge{})
	}
	if err != nil {
		return nil, false, err
	}
	if current != revision {
		return quarterDecline(work, "publication.stale", -1, -1, -1,
			BoundaryEdge{}, BoundaryEdge{})
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if err := work.Err(); err != nil {
		return nil, false, err
	}
	return result, true, nil
}

func (s *Sketch) certifyQuarterCandidate(ctx context.Context, work JunctionWork,
	c *quarterCandidate) (QuarterArcJunction, error) {
	left, right := c.left, c.right
	lineEdge, arcEdge := left, right
	if _, ok := left.Entity.(*Arc); ok {
		lineEdge, arcEdge = right, left
	}
	line, lineOK := lineEdge.Entity.(*Line)
	arc, arcOK := arcEdge.Entity.(*Arc)
	if !lineOK || !arcOK {
		return QuarterArcJunction{}, errJunctionMiss
	}
	if !s.ownsEntity(line) || !s.ownsEntity(arc) {
		return QuarterArcJunction{}, errJunctionMiss
	}
	// These float operations must denote the same exact dyadic quarter carrier.
	vals := [6]float64{arc.Center.X(), arc.Center.Y(), arc.Start.X(),
		arc.Start.Y(), arc.End.X(), arc.End.Y()}
	for _, v := range vals {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return QuarterArcJunction{}, errJunctionMiss
		}
	}
	cx, cy := new(big.Rat).SetFloat64(vals[0]), new(big.Rat).SetFloat64(vals[1])
	sx, sy := new(big.Rat).SetFloat64(vals[2]), new(big.Rat).SetFloat64(vals[3])
	ex, ey := new(big.Rat).SetFloat64(vals[4]), new(big.Rat).SetFloat64(vals[5])
	radius := new(big.Rat).Sub(sx, cx)
	if sy.Cmp(cy) != 0 || ex.Cmp(cx) != 0 || radius.Sign() <= 0 ||
		radius.Cmp(new(big.Rat).Sub(ey, cy)) != 0 ||
		!quarterCoordinateFits(vals[:]) ||
		arc.R() != vals[2]-vals[0] ||
		arc.StartAngle() != 0 || arc.EndAngle() != math.Pi/2 ||
		arc.Sweep() != math.Pi/2 {
		return QuarterArcJunction{}, errJunctionMiss
	}
	// A float subtraction that lost an exact radius bit cannot name this carrier.
	roundedRadius := new(big.Rat).SetFloat64(vals[2] - vals[0])
	if roundedRadius.Cmp(radius) != 0 {
		return QuarterArcJunction{}, errJunctionMiss
	}
	counter := quarterRootCounter{cancelErr: ctx.Err, work: work}
	m := junctionMath{
		cancelErr: counter.step,
		limits: junctionLimits{rootRounds: 128, atanTerms: 24,
			endpointBits: 256, intermediateBits: 8192},
	}
	if err := counter.step(); err != nil {
		return QuarterArcJunction{}, err
	}
	pair := c.cross.SourceIndices()
	if pair[0] != lineEdge.sourceIndex || pair[1] != arcEdge.sourceIndex ||
		(c.cross.Branch() != -1 && c.cross.Branch() != 1) {
		return QuarterArcJunction{}, errJunctionMiss
	}
	bounds, err := quarterOtherBounds(&counter, c.cross)
	if err != nil {
		return QuarterArcJunction{}, err
	}
	in := junctionInput{
		line:   [4]float64{line.Start.X(), line.Start.Y(), line.End.X(), line.End.Y()},
		circle: [3]float64{vals[0], vals[1], vals[2] - vals[0]},
		event:  c.cross.Point(), branch: c.cross.Branch(), otherBounds: bounds,
		lineTrim:   walkedJunctionParam(lineEdge, lineEdge.sourceIndex == left.sourceIndex),
		circleTrim: walkedJunctionParam(arcEdge, arcEdge.sourceIndex == left.sourceIndex),
	}
	point, params, err := certifyQuarterRoot(m, in)
	if err != nil {
		return QuarterArcJunction{}, err
	}
	if _, arcFirst := left.Entity.(*Arc); arcFirst {
		params[0], params[1] = params[1], params[0]
	}
	source := func(e BoundaryEdge, atEnd bool) JunctionSource {
		kind := JunctionLine
		if _, ok := e.Entity.(*Arc); ok {
			kind = JunctionArc
		}
		return JunctionSource{Index: e.sourceIndex, Kind: kind,
			TStart: e.TStart, TEnd: e.TEnd, Reversed: e.Reversed, AtWalkEnd: atEnd}
	}
	return QuarterArcJunction{
		sources: [2]JunctionSource{source(left, true), source(right, false)},
		point:   point, params: params, branch: c.cross.Branch(),
		occurrences: append([]JunctionOccurrence(nil), c.occurrences...),
	}, nil
}

func quarterCoordinateFits(vals []float64) bool {
	for _, v := range vals {
		r := new(big.Rat).SetFloat64(v)
		if r == nil || r.Num().BitLen() > 16 || r.Denom().BitLen() > 16 {
			return false
		}
	}
	return true
}

type quarterRootCounter struct {
	cancelErr func() error
	work      JunctionWork
	used      int
}

func (c *quarterRootCounter) step() error {
	if err := c.cancelErr(); err != nil {
		return err
	}
	if err := c.work.Err(); err != nil {
		return err
	}
	c.used++
	if c.used > 512 {
		return ErrJunctionWorkLimit
	}
	return c.work.Step(JunctionRootProof)
}

func quarterOtherBounds(c *quarterRootCounter,
	cross *geom.LineArcCrossing) ([][4]float64, error) {
	pair, count := cross.SourceIndices(), cross.SourceCount()
	if count < 2 || count > 10 || pair[0] < 0 || pair[1] < 0 ||
		pair[0] == pair[1] || pair[0] >= count || pair[1] >= count {
		return nil, errJunctionMiss
	}
	out := make([][4]float64, 0, count-2)
	for i := 0; i < count; i++ {
		if err := c.step(); err != nil {
			return nil, err
		}
		if i == pair[0] || i == pair[1] {
			continue
		}
		box, ok := cross.SourceBound(i)
		if !ok {
			return nil, errJunctionMiss
		}
		out = append(out, box)
	}
	return out, nil
}

var quarterPi = func() junctionInterval {
	den := new(big.Int).Lsh(big.NewInt(1), 128)
	lo, _ := new(big.Int).SetString("1069028584064966747859680373161870783300", 10)
	hi, _ := new(big.Int).SetString("1069028584064966747859680373161870783301", 10)
	return junctionInterval{new(big.Rat).SetFrac(lo, new(big.Int).Set(den)),
		new(big.Rat).SetFrac(hi, new(big.Int).Set(den))}
}()

// certifyQuarterRoot uses one 128-bit integer square root and two 24-term
// endpoint atan series. Every checked arithmetic call charges the root wall.
func certifyQuarterRoot(m junctionMath, in junctionInput) (
	[2]RationalBracket, [2]RationalBracket, error) {
	var noPoint, noParams [2]RationalBracket
	if in.branch != -1 && in.branch != 1 {
		return noPoint, noParams, errJunctionMiss
	}
	for _, v := range append(in.line[:], in.circle[:]...) {
		r := new(big.Rat).SetFloat64(v)
		if r == nil || r.Num().BitLen() > 16 || r.Denom().BitLen() > 16 {
			return noPoint, noParams, errJunctionMiss
		}
	}
	var r [7]*big.Rat
	for i, v := range [7]float64{in.line[0], in.line[1], in.line[2],
		in.line[3], in.circle[0], in.circle[1], in.circle[2]} {
		var err error
		r[i], err = m.finiteRat(v)
		if err != nil {
			return noPoint, noParams, err
		}
	}
	if r[6].Sign() <= 0 {
		return noPoint, noParams, errJunctionMiss
	}
	var failed error
	op := func(fn func(*big.Rat, *big.Rat) (*big.Rat, error),
		a, b *big.Rat) *big.Rat {
		if failed != nil {
			return ratInt(0)
		}
		v, err := fn(a, b)
		if err != nil {
			failed = err
			return ratInt(0)
		}
		return v
	}
	dx := op(m.sub, r[2], r[0])
	dy := op(m.sub, r[3], r[1])
	ox := op(m.sub, r[0], r[4])
	oy := op(m.sub, r[1], r[5])
	a := op(m.add, op(m.mul, dx, dx), op(m.mul, dy, dy))
	b := op(m.mul, ratInt(2), op(m.add,
		op(m.mul, dx, ox), op(m.mul, dy, oy)))
	c := op(m.sub, op(m.add, op(m.mul, ox, ox), op(m.mul, oy, oy)),
		op(m.mul, r[6], r[6]))
	discriminant := op(m.sub, op(m.mul, b, b),
		op(m.mul, ratInt(4), op(m.mul, a, c)))
	twoA := op(m.mul, ratInt(2), a)
	if failed != nil {
		return noPoint, noParams, failed
	}
	if a.Sign() <= 0 || discriminant.Sign() <= 0 {
		return noPoint, noParams, errJunctionMiss
	}
	sqrt, err := m.sqrtInterval(discriminant, 128)
	if err != nil {
		return noPoint, noParams, err
	}
	selected, err := m.quadraticRoot(b, twoA, sqrt, in.branch)
	if err != nil {
		return noPoint, noParams, err
	}
	other, err := m.quadraticRoot(b, twoA, sqrt, -in.branch)
	if err != nil {
		return noPoint, noParams, err
	}
	zero, one := ratInt(0), ratInt(1)
	if selected.hi.Cmp(zero) <= 0 || selected.lo.Cmp(one) >= 0 {
		return noPoint, noParams, errJunctionMiss
	}
	if selected.lo.Cmp(zero) <= 0 || selected.hi.Cmp(one) >= 0 ||
		selected.lo.Cmp(other.hi) <= 0 && selected.hi.Cmp(other.lo) >= 0 {
		return noPoint, noParams, ErrJunctionWorkLimit
	}
	paramLimit := new(big.Rat).SetFrac(ratInt(1).Num(),
		new(big.Int).Lsh(big.NewInt(1), 48))
	scale := ratInt(1)
	for _, v := range r {
		abs := new(big.Rat).Abs(v)
		if abs.Cmp(scale) > 0 {
			scale = abs
		}
	}
	pointLimit, err := m.div(scale,
		new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), 48)))
	if err != nil {
		return noPoint, noParams, err
	}
	lineParam, err := m.widthCheck(selected, in.lineTrim, paramLimit)
	if err != nil {
		return noPoint, noParams, quarterRefinement(err)
	}
	px, err := m.affine(r[0], dx, selected)
	if err != nil {
		return noPoint, noParams, err
	}
	py, err := m.affine(r[1], dy, selected)
	if err != nil {
		return noPoint, noParams, err
	}
	px, err = m.widthCheck(px, in.event[0], pointLimit)
	if err != nil {
		return noPoint, noParams, quarterRefinement(err)
	}
	py, err = m.widthCheck(py, in.event[1], pointLimit)
	if err != nil {
		return noPoint, noParams, quarterRefinement(err)
	}
	if err := m.checkOtherSources(px, py, in.otherBounds); err != nil {
		return noPoint, noParams, err
	}
	otherX, err := m.affine(r[0], dx, other)
	if err != nil {
		return noPoint, noParams, err
	}
	otherY, err := m.affine(r[1], dy, other)
	if err != nil {
		return noPoint, noParams, err
	}
	if px.lo.Cmp(otherX.hi) <= 0 && px.hi.Cmp(otherX.lo) >= 0 &&
		py.lo.Cmp(otherY.hi) <= 0 && py.hi.Cmp(otherY.lo) >= 0 {
		return noPoint, noParams, ErrJunctionWorkLimit
	}
	arcParam, err := quarterArcParameter(m, px, py, r[4], r[5], r[6])
	if err != nil {
		return noPoint, noParams, quarterRefinement(err)
	}
	arcParam, err = m.widthCheck(arcParam, in.circleTrim, paramLimit)
	if err != nil {
		return noPoint, noParams, quarterRefinement(err)
	}
	return [2]RationalBracket{intervalBracket(px), intervalBracket(py)},
		[2]RationalBracket{intervalBracket(lineParam), intervalBracket(arcParam)}, nil
}

func quarterRefinement(err error) error {
	if errors.Is(err, errJunctionNeedMore) {
		return ErrJunctionWorkLimit
	}
	return err
}

func quarterArcParameter(m junctionMath, px, py junctionInterval,
	cx, cy, radius *big.Rat) (junctionInterval, error) {
	x, err := m.subInterval(px, junctionInterval{cx, cx})
	if err != nil {
		return junctionInterval{}, err
	}
	y, err := m.subInterval(py, junctionInterval{cy, cy})
	if err != nil {
		return junctionInterval{}, err
	}
	if x.hi.Sign() <= 0 || y.hi.Sign() <= 0 ||
		x.lo.Cmp(radius) >= 0 || y.lo.Cmp(radius) >= 0 {
		return junctionInterval{}, errJunctionMiss
	}
	if x.lo.Sign() <= 0 || y.lo.Sign() <= 0 ||
		x.hi.Cmp(radius) >= 0 || y.hi.Cmp(radius) >= 0 {
		return junctionInterval{}, errJunctionNeedMore
	}
	swapped := false
	var ratio junctionInterval
	switch {
	case x.hi.Cmp(y.lo) < 0:
		swapped = true
		ratio, err = m.divPositive(x, y, 128)
	case y.hi.Cmp(x.lo) < 0:
		ratio, err = m.divPositive(y, x, 128)
	default:
		return junctionInterval{}, errJunctionNeedMore
	}
	if err != nil {
		return junctionInterval{}, err
	}
	half := new(big.Rat).SetFrac64(1, 2)
	piHalf, err := m.divPositive(quarterPi,
		junctionInterval{ratInt(2), ratInt(2)}, 128)
	if err != nil {
		return junctionInterval{}, err
	}
	var theta junctionInterval
	switch {
	case ratio.hi.Cmp(half) < 0:
		theta, err = quarterAtanEndpoints(m, ratio)
	case ratio.lo.Cmp(half) > 0:
		one := junctionInterval{ratInt(1), ratInt(1)}
		num, e := m.subInterval(one, ratio)
		if e != nil {
			return junctionInterval{}, e
		}
		den, e := m.addInterval(one, ratio)
		if e != nil {
			return junctionInterval{}, e
		}
		reduced, e := m.divPositive(num, den, 128)
		if e != nil {
			return junctionInterval{}, e
		}
		atan, e := quarterAtanEndpoints(m, reduced)
		if e != nil {
			return junctionInterval{}, e
		}
		piQuarter, e := m.divPositive(quarterPi,
			junctionInterval{ratInt(4), ratInt(4)}, 128)
		if e != nil {
			return junctionInterval{}, e
		}
		theta, err = m.subInterval(piQuarter, atan)
	default:
		return junctionInterval{}, errJunctionNeedMore
	}
	if err != nil {
		return junctionInterval{}, err
	}
	if swapped {
		theta, err = m.subInterval(piHalf, theta)
		if err != nil {
			return junctionInterval{}, err
		}
	}
	param, err := m.divPositive(theta, piHalf, 128)
	if err != nil {
		return junctionInterval{}, err
	}
	if param.lo.Sign() <= 0 || param.hi.Cmp(ratInt(1)) >= 0 {
		return junctionInterval{}, errJunctionNeedMore
	}
	return param, nil
}

func quarterAtanEndpoints(m junctionMath, q junctionInterval) (junctionInterval, error) {
	low, err := m.atanSeries(q.lo)
	if err != nil {
		return junctionInterval{}, err
	}
	high, err := m.atanSeries(q.hi)
	if err != nil {
		return junctionInterval{}, err
	}
	return junctionInterval{low.lo, high.hi}, nil
}
