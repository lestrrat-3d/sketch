package geom

import (
	"context"
	"math"
	"math/big"
)

// RegionsTwoConvexLineCapsBounded arranges two four-line convex-loop pairs
// when each pair crosses one carrier twice and has no other contact. It
// returns no arrangement for any unproved shape or refused charge. Other
// line scenes can use RegionsLinesBounded.
func RegionsTwoConvexLineCapsBounded(ctx context.Context, scenes [2][]Curve,
	charge func(uint64) error) ([2]*Arrangement, error) {
	var none [2]*Arrangement
	if ctx == nil || charge == nil {
		return none, ErrUnsupportedLineScene
	}
	take := func(n uint64) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if n > 256 {
			return ErrUnsupportedLineScene
		}
		return charge(n)
	}
	var layouts [2]convexLineCap
	for cap, curves := range scenes {
		layout, err := certifyConvexLineCap(curves, take)
		if err != nil {
			return none, err
		}
		layouts[cap] = layout
	}
	if err := take(8); err != nil {
		return none, err
	}
	if layouts[0].carrier != layouts[1].carrier ||
		layouts[0].exit != layouts[1].exit || layouts[0].enter != layouts[1].enter {
		return none, ErrUnsupportedLineScene
	}
	for i, point := range layouts[0].points[0] {
		if point.xy != layouts[1].points[0][i].xy {
			return none, ErrUnsupportedLineScene
		}
	}
	var result [2]*Arrangement
	for cap, layout := range layouts {
		arr, err := publishConvexLineCap(layout, take)
		if err != nil {
			return none, err
		}
		result[cap] = arr
	}
	if err := ctx.Err(); err != nil {
		return none, err
	}
	return result, nil
}

type convexCapPoint struct {
	xy  [2]float64
	rat [2]*big.Rat
}

type convexLineCap struct {
	lines   [8]*Line
	points  [2][4]convexCapPoint
	carrier int
	exit    int               // Loft edge crossing from prism interior to exterior
	enter   int               // Loft edge crossing from prism exterior to interior
	cuts    [2]convexCapPoint // ordered along the prism carrier
}

func capRat(v float64) *big.Rat { return new(big.Rat).SetFloat64(v) }

func capSub(a, b *big.Rat) *big.Rat { return new(big.Rat).Sub(a, b) }

func capMul(a, b *big.Rat) *big.Rat { return new(big.Rat).Mul(a, b) }

func capOrientation(a, b, c convexCapPoint) *big.Rat {
	return capSub(capMul(capSub(b.rat[0], a.rat[0]), capSub(c.rat[1], a.rat[1])),
		capMul(capSub(b.rat[1], a.rat[1]), capSub(c.rat[0], a.rat[0])))
}

func certifyConvexLineCap(curves []Curve, take func(uint64) error) (convexLineCap, error) {
	var no convexLineCap
	if len(curves) != 8 {
		return no, ErrUnsupportedLineScene
	}
	// Reserve all source-point rationals and the fixed loop/pair ledgers before
	// allocating. Coordinates share the existing rectangle producer's finite
	// magnitude screen, so exact products have a fixed binary-size ceiling.
	if err := take(128); err != nil {
		return no, err
	}
	var layout convexLineCap
	for i, curve := range curves {
		if err := take(3); err != nil {
			return no, err
		}
		line, ok := curve.(*Line)
		if !ok || line == nil || line.Start == nil || line.End == nil {
			return no, ErrUnsupportedLineScene
		}
		layout.lines[i] = line
		for _, v := range [...]float64{line.Start.X, line.Start.Y, line.End.X, line.End.Y} {
			if !twoRectFinite(v) {
				return no, ErrUnsupportedLineScene
			}
		}
	}
	for loop := range 2 {
		for i := range 4 {
			if err := take(4); err != nil {
				return no, err
			}
			line := layout.lines[4*loop+i]
			if line.End != layout.lines[4*loop+(i+1)%4].Start {
				return no, ErrUnsupportedLineScene
			}
			p := line.Start
			layout.points[loop][i] = convexCapPoint{
				xy:  [2]float64{p.X, p.Y},
				rat: [2]*big.Rat{capRat(p.X), capRat(p.Y)},
			}
		}
		for i := range 4 {
			if err := take(2); err != nil {
				return no, err
			}
			p := layout.points[loop]
			if capOrientation(p[i], p[(i+1)%4], p[(i+2)%4]).Sign() <= 0 {
				return no, ErrUnsupportedLineScene
			}
		}
	}
	// Convexity proves that nonadjacent edges of each loop cannot meet.
	// Check every cross-loop pair exactly, including endpoint contact, so the
	// two carrier crossings below are the complete source arrangement.
	var pairCrossings [4]int
	for i := range 4 {
		for j := range 4 {
			if err := take(4); err != nil {
				return no, err
			}
			pa, pb := layout.points[0][i], layout.points[0][(i+1)%4]
			la, lb := layout.points[1][j], layout.points[1][(j+1)%4]
			p0, p1 := capOrientation(pa, pb, la).Sign(), capOrientation(pa, pb, lb).Sign()
			l0, l1 := capOrientation(la, lb, pa).Sign(), capOrientation(la, lb, pb).Sign()
			if p0 == 0 && convexCapOnSegment(pa, pb, la) ||
				p1 == 0 && convexCapOnSegment(pa, pb, lb) ||
				l0 == 0 && convexCapOnSegment(la, lb, pa) ||
				l1 == 0 && convexCapOnSegment(la, lb, pb) {
				return no, ErrUnsupportedLineScene
			}
			if p0 != p1 && l0 != l1 {
				pairCrossings[i]++
			}
		}
	}
	// Every Loft vertex lies strictly inside the other three prism
	// halfplanes. On the carrier, its vertices lie on both sides and exactly
	// two Loft edges cross it properly.
	layout.carrier, layout.exit, layout.enter = -1, -1, -1
	for i := range 4 {
		inside, outside := 0, 0
		for _, q := range layout.points[1] {
			if err := take(2); err != nil {
				return no, err
			}
			p := layout.points[0]
			side := capOrientation(p[i], p[(i+1)%4], q).Sign()
			if side == 0 {
				return no, ErrUnsupportedLineScene
			}
			if side > 0 {
				inside++
			} else {
				outside++
			}
		}
		if outside == 0 {
			continue
		}
		if layout.carrier >= 0 || inside == 0 {
			return no, ErrUnsupportedLineScene
		}
		layout.carrier = i
	}
	if layout.carrier < 0 {
		return no, ErrUnsupportedLineScene
	}
	for i, count := range pairCrossings {
		if i == layout.carrier && count != 2 || i != layout.carrier && count != 0 {
			return no, ErrUnsupportedLineScene
		}
	}
	carrier := layout.carrier
	p := layout.points[0]
	l := layout.points[1]
	var crossing [2]convexCapPoint
	for j := range 4 {
		if err := take(8); err != nil {
			return no, err
		}
		a := capOrientation(p[carrier], p[(carrier+1)%4], l[j]).Sign()
		b := capOrientation(p[carrier], p[(carrier+1)%4], l[(j+1)%4]).Sign()
		if a == b {
			continue
		}
		point, ok := convexCapCross(p[carrier], p[(carrier+1)%4], l[j], l[(j+1)%4])
		if !ok {
			return no, ErrUnsupportedLineScene
		}
		if a > 0 {
			if layout.exit >= 0 {
				return no, ErrUnsupportedLineScene
			}
			layout.exit = j
			crossing[0] = point
		} else {
			if layout.enter >= 0 {
				return no, ErrUnsupportedLineScene
			}
			layout.enter = j
			crossing[1] = point
		}
	}
	if layout.exit < 0 || layout.enter < 0 || layout.exit == layout.enter {
		return no, ErrUnsupportedLineScene
	}
	t0, ok0 := convexCapParam(p[carrier], p[(carrier+1)%4], crossing[0])
	t1, ok1 := convexCapParam(p[carrier], p[(carrier+1)%4], crossing[1])
	if !ok0 || !ok1 || t0.Cmp(t1) >= 0 {
		return no, ErrUnsupportedLineScene
	}
	layout.cuts = crossing
	return layout, nil
}

func convexCapOnSegment(a, b, p convexCapPoint) bool {
	for axis := range 2 {
		lo, hi := a.rat[axis], b.rat[axis]
		if lo.Cmp(hi) > 0 {
			lo, hi = hi, lo
		}
		if p.rat[axis].Cmp(lo) < 0 || p.rat[axis].Cmp(hi) > 0 {
			return false
		}
	}
	return true
}

func convexCapCross(a, b, c, d convexCapPoint) (convexCapPoint, bool) {
	var no convexCapPoint
	ux, uy := capSub(b.rat[0], a.rat[0]), capSub(b.rat[1], a.rat[1])
	vx, vy := capSub(d.rat[0], c.rat[0]), capSub(d.rat[1], c.rat[1])
	den := capSub(capMul(ux, vy), capMul(uy, vx))
	if den.Sign() == 0 {
		return no, false
	}
	wx, wy := capSub(c.rat[0], a.rat[0]), capSub(c.rat[1], a.rat[1])
	t := new(big.Rat).Quo(capSub(capMul(wx, vy), capMul(wy, vx)), den)
	u := new(big.Rat).Quo(capSub(capMul(wx, uy), capMul(wy, ux)), den)
	zero, one := new(big.Rat), big.NewRat(1, 1)
	if t.Cmp(zero) <= 0 || t.Cmp(one) >= 0 || u.Cmp(zero) <= 0 || u.Cmp(one) >= 0 {
		return no, false
	}
	x := new(big.Rat).Add(a.rat[0], capMul(t, ux))
	y := new(big.Rat).Add(a.rat[1], capMul(t, uy))
	fx, exactX := x.Float64()
	fy, exactY := y.Float64()
	if !exactX || !exactY || !twoRectFinite(fx) || !twoRectFinite(fy) {
		return no, false
	}
	return convexCapPoint{xy: [2]float64{fx, fy}, rat: [2]*big.Rat{x, y}}, true
}

func convexCapParam(a, b, p convexCapPoint) (*big.Rat, bool) {
	dx, dy := capSub(b.rat[0], a.rat[0]), capSub(b.rat[1], a.rat[1])
	den, num := dx, capSub(p.rat[0], a.rat[0])
	if den.Sign() == 0 {
		den, num = dy, capSub(p.rat[1], a.rat[1])
	}
	if den.Sign() == 0 {
		return nil, false
	}
	t := new(big.Rat).Quo(num, den)
	if new(big.Rat).Add(a.rat[0], capMul(t, dx)).Cmp(p.rat[0]) != 0 ||
		new(big.Rat).Add(a.rat[1], capMul(t, dy)).Cmp(p.rat[1]) != 0 {
		return nil, false
	}
	return t, true
}

type convexCapFragment struct {
	start, end convexCapPoint
	source     int
}

func convexCapLoftPath(layout convexLineCap, first, last int,
	start, end convexCapPoint, take func(uint64) error) ([]convexCapFragment, error) {
	if err := take(5); err != nil {
		return nil, err
	}
	path := make([]convexCapFragment, 0, 4)
	current := start
	for edge := first; ; edge = (edge + 1) % 4 {
		if err := take(2); err != nil {
			return nil, err
		}
		next := end
		if edge != last {
			next = layout.points[1][(edge+1)%4]
		}
		path = append(path, convexCapFragment{current, next, 4 + edge})
		if edge == last {
			return path, nil
		}
		current = next
	}
}

func publishConvexLineCap(layout convexLineCap, take func(uint64) error) (*Arrangement, error) {
	carrier, c0, c1 := layout.carrier, layout.cuts[0], layout.cuts[1]
	outside, err := convexCapLoftPath(layout, layout.exit, layout.enter, c0, c1, take)
	if err != nil {
		return nil, err
	}
	inside, err := convexCapLoftPath(layout, layout.enter, layout.exit, c1, c0, take)
	if err != nil {
		return nil, err
	}
	// Reserve the three region headers, at most 18 fragments, the source-use
	// table, the bounded endpoint cache, and two crossing tokens before publication.
	if err := take(132); err != nil {
		return nil, err
	}
	var paths [3][]convexCapFragment
	p := layout.points[0]
	paths[0] = make([]convexCapFragment, 0, 5+len(inside))
	paths[0] = append(paths[0], convexCapFragment{c1, p[(carrier+1)%4], carrier})
	for k := 1; k <= 3; k++ {
		i := (carrier + k) % 4
		paths[0] = append(paths[0], convexCapFragment{p[i], p[(i+1)%4], i})
	}
	paths[0] = append(paths[0], convexCapFragment{p[carrier], c0, carrier})
	for i := len(inside) - 1; i >= 0; i-- {
		f := inside[i]
		paths[0] = append(paths[0], convexCapFragment{f.end, f.start, f.source})
	}
	paths[1] = make([]convexCapFragment, 0, 1+len(inside))
	paths[1] = append(paths[1], convexCapFragment{c0, c1, carrier})
	paths[1] = append(paths[1], inside...)
	paths[2] = make([]convexCapFragment, 0, 1+len(outside))
	paths[2] = append(paths[2], outside...)
	paths[2] = append(paths[2], convexCapFragment{c1, c0, carrier})
	var tokens [2]*LineLineCrossing
	for k, source := range [2]int{layout.exit, layout.enter} {
		a, b := carrier, 4+source
		if a > b {
			a, b = b, a
		}
		tokens[k] = &LineLineCrossing{sources: [2]int{a, b}, point: layout.cuts[k].xy}
	}
	arr := &Arrangement{Regions: make([]*Region, 0, 3)}
	type key struct {
		source int
		lo, hi uint64
	}
	uses := make(map[key][]bool, 12)
	verifiedEndpoints := make(map[convexCapEndpointKey]struct{}, 36)
	for _, path := range paths {
		if err := take(uint64(len(path) + 1)); err != nil {
			return nil, err
		}
		region := &Region{Outer: make([]BoundaryEdge, 0, len(path))}
		area := new(big.Rat)
		for _, fragment := range path {
			if err := take(8); err != nil {
				return nil, err
			}
			edge, err := convexCapEdge(layout.lines, fragment, layout.cuts, tokens,
				verifiedEndpoints, take)
			if err != nil {
				return nil, err
			}
			region.Outer = append(region.Outer, edge)
			term := capSub(capMul(fragment.start.rat[0], fragment.end.rat[1]),
				capMul(fragment.start.rat[1], fragment.end.rat[0]))
			area.Add(area, term)
			k := key{edge.SourceIndex, math.Float64bits(edge.TStart), math.Float64bits(edge.TEnd)}
			uses[k] = append(uses[k], edge.Reversed)
		}
		if area.Sign() <= 0 {
			return nil, ErrUnsupportedLineScene
		}
		area.Quo(area, big.NewRat(2, 1))
		region.Area, _ = area.Float64()
		if math.IsInf(region.Area, 0) || region.Area <= 0 {
			return nil, ErrUnsupportedLineScene
		}
		arr.Regions = append(arr.Regions, region)
	}
	if err := take(uint64(len(uses))); err != nil {
		return nil, err
	}
	if len(uses) != 12 {
		return nil, ErrUnsupportedLineScene
	}
	for _, directions := range uses {
		if len(directions) < 1 || len(directions) > 2 ||
			len(directions) == 2 && directions[0] == directions[1] {
			return nil, ErrUnsupportedLineScene
		}
	}
	return arr, nil
}

// convexCapEndpointReproduces checks the reported binary64 parameter against
// the exact point on this source. The carrier and crossing are already proved
// with rationals. A rounded parameter may evaluate a few ulps differently
// when the compiler fuses multiply-add. Check the exact dyadic evaluation and
// both IEEE evaluation paths against the same source-local identity band used
// by ordinary arrangement TExact. The source is part of the scene, so its
// extent is no larger than the scene extent and this also implies that band.
func convexCapEndpointReproduces(a, b, point convexCapPoint, t float64,
	take func(uint64) error) (bool, error) {
	// Covers the local and per-axis roundoff bands, exact evaluation, both
	// rounded evaluation paths, and their bounded comparisons.
	if err := take(40); err != nil {
		return false, err
	}
	tRat := capRat(t)
	if tRat == nil || t < 0 || t > 1 {
		return false, nil
	}
	var delta [2]*big.Rat
	var roundBand [2]*big.Rat
	extent := new(big.Rat)
	for axis := range 2 {
		if point.rat[axis] == nil || point.rat[axis].Cmp(capRat(point.xy[axis])) != 0 {
			return false, nil
		}
		delta[axis] = capSub(b.rat[axis], a.rat[axis])
		size := new(big.Rat).Abs(delta[axis])
		if size.Cmp(extent) > 0 {
			extent = size
		}
		scale := size
		for _, coord := range [2]*big.Rat{a.rat[axis], b.rat[axis]} {
			magnitude := new(big.Rat).Abs(coord)
			if magnitude.Cmp(scale) > 0 {
				scale = magnitude
			}
		}
		roundBand[axis] = capMul(capRat(8*unitRoundoff), scale)
	}
	if extent.Sign() <= 0 {
		return false, nil
	}
	band := capMul(capRat(weldIdentEps), extent)
	bandSquared := capMul(band, band)
	var exact, fused, separate [2]*big.Rat
	for axis := range 2 {
		exact[axis] = new(big.Rat).Add(a.rat[axis], capMul(tRat, delta[axis]))
		av, bv := a.xy[axis], b.xy[axis]
		roundedDelta := bv - av
		fused[axis] = capRat(math.FMA(t, roundedDelta, av))
		roundedProduct, _ := capMul(tRat, capRat(roundedDelta)).Float64()
		roundedSum, _ := new(big.Rat).Add(capRat(av),
			capRat(roundedProduct)).Float64()
		separate[axis] = capRat(roundedSum)
		if fused[axis] == nil || separate[axis] == nil {
			return false, nil
		}
	}
	for _, evaluated := range [3][2]*big.Rat{exact, fused, separate} {
		var gap [2]*big.Rat
		for axis := range 2 {
			gap[axis] = new(big.Rat).Abs(capSub(evaluated[axis], point.rat[axis]))
			if gap[axis].Cmp(roundBand[axis]) > 0 {
				return false, nil
			}
		}
		gapSquared := new(big.Rat).Add(capMul(gap[0], gap[0]),
			capMul(gap[1], gap[1]))
		if gapSquared.Cmp(bandSquared) > 0 {
			return false, nil
		}
	}
	return true, nil
}

type convexCapEndpointKey struct {
	source      int
	endpoint    uint8 // 0=cut, 1=exact source start, 2=exact source end
	param, x, y uint64
}

// A profile may walk the same source endpoint in two cells. Charge each lookup
// and prove each unchanged (source, parameter, point) only once per cap.
func convexCapCachedEndpoint(a, b, point convexCapPoint, exactT *big.Rat,
	t float64, source int, verified map[convexCapEndpointKey]struct{},
	take func(uint64) error) (bool, error) {
	if err := take(1); err != nil {
		return false, err
	}
	if exactT == nil {
		return false, nil
	}
	wholeStart := exactT.Sign() == 0
	wholeEnd := exactT.Cmp(big.NewRat(1, 1)) == 0
	var endpoint uint8
	if wholeStart {
		endpoint = 1
	} else if wholeEnd {
		endpoint = 2
	}
	key := convexCapEndpointKey{source: source, endpoint: endpoint,
		param: math.Float64bits(t), x: math.Float64bits(point.xy[0]),
		y: math.Float64bits(point.xy[1])}
	if _, ok := verified[key]; ok {
		return true, nil
	}
	if key.endpoint != 0 {
		// Exact source-domain identity: two parameter checks, two rational
		// coordinate checks, two binary64 bit checks, and cache publication.
		if err := take(8); err != nil {
			return false, err
		}
		original := a
		expectedT := float64(0)
		if wholeEnd {
			original, expectedT = b, 1
		}
		if math.Float64bits(t) != math.Float64bits(expectedT) {
			return false, nil
		}
		for axis := range 2 {
			if point.rat[axis] == nil || original.rat[axis] == nil ||
				point.rat[axis].Cmp(original.rat[axis]) != 0 ||
				math.Float64bits(point.xy[axis]) !=
					math.Float64bits(original.xy[axis]) {
				return false, nil
			}
		}
	} else {
		ok, err := convexCapEndpointReproduces(a, b, point, t, take)
		if err != nil || !ok {
			return ok, err
		}
	}
	verified[key] = struct{}{}
	return true, nil
}

func convexCapEdge(lines [8]*Line, fragment convexCapFragment,
	cuts [2]convexCapPoint, tokens [2]*LineLineCrossing,
	verified map[convexCapEndpointKey]struct{},
	take func(uint64) error) (BoundaryEdge, error) {
	var no BoundaryEdge
	line := lines[fragment.source]
	a := convexCapPoint{xy: [2]float64{line.Start.X, line.Start.Y},
		rat: [2]*big.Rat{capRat(line.Start.X), capRat(line.Start.Y)}}
	b := convexCapPoint{xy: [2]float64{line.End.X, line.End.Y},
		rat: [2]*big.Rat{capRat(line.End.X), capRat(line.End.Y)}}
	t0, ok0 := convexCapParam(a, b, fragment.start)
	t1, ok1 := convexCapParam(a, b, fragment.end)
	if !ok0 || !ok1 || t0.Cmp(t1) == 0 {
		return no, ErrUnsupportedLineScene
	}
	zero, one := new(big.Rat), big.NewRat(1, 1)
	if t0.Cmp(zero) < 0 || t0.Cmp(one) > 0 || t1.Cmp(zero) < 0 || t1.Cmp(one) > 0 {
		return no, ErrUnsupportedLineScene
	}
	reversed := t0.Cmp(t1) > 0
	if reversed {
		t0, t1 = t1, t0
	}
	start, _ := t0.Float64()
	end, _ := t1.Float64()
	if !(start < end) {
		return no, ErrUnsupportedLineScene
	}
	lo, hi := fragment.start, fragment.end
	if reversed {
		lo, hi = hi, lo
	}
	reproduced, err := convexCapCachedEndpoint(a, b, lo, t0, start,
		fragment.source, verified, take)
	if err != nil {
		return no, err
	}
	if !reproduced {
		return no, ErrUnsupportedLineScene
	}
	reproduced, err = convexCapCachedEndpoint(a, b, hi, t1, end,
		fragment.source, verified, take)
	if err != nil {
		return no, err
	}
	if !reproduced {
		return no, ErrUnsupportedLineScene
	}
	// Charge the two-point polyline before allocation. The exact rational
	// source parameters and both stored endpoints were checked above.
	if err := take(2); err != nil {
		return no, err
	}
	edge := BoundaryEdge{SourceIndex: fragment.source,
		Whole:    t0.Cmp(zero) == 0 && t1.Cmp(one) == 0,
		Reversed: reversed, TStart: start, TEnd: end, TExact: true,
		Polyline: [][2]float64{fragment.start.xy, fragment.end.xy}}
	for k, cut := range cuts {
		if err := take(1); err != nil {
			return no, err
		}
		if fragment.source != tokens[k].sources[0] && fragment.source != tokens[k].sources[1] {
			continue
		}
		if fragment.start.xy == cut.xy {
			edge.crossStart = tokens[k]
		}
		if fragment.end.xy == cut.xy {
			edge.crossEnd = tokens[k]
		}
	}
	return edge, nil
}
