package geom

import (
	"context"
	"math"
	"math/big"
)

// RegionsTwoRectangleCapsBounded is a deliberately narrow, charged
// arrangement of two separate eight-line scenes. Each scene must contain
// one axis-aligned rectangle crossed at its right wall by another rectangle.
// The first rectangle and the second rectangle's left, lower and upper
// coordinates must agree across scenes. The two results are atomic: a
// refusal or cancellation publishes neither one.
func RegionsTwoRectangleCapsBounded(ctx context.Context, scenes [2][]Curve,
	charge func(uint64) error) ([2]*Arrangement, error) {
	var none [2]*Arrangement
	if ctx == nil || charge == nil {
		return none, ErrUnsupportedLineScene
	}
	take := func(n uint64) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return charge(n)
	}
	var layouts [2]twoRectangleLayout
	for i := range scenes {
		layout, err := certifyTwoRectangles(scenes[i], take)
		if err != nil {
			return none, err
		}
		layouts[i] = layout
	}
	if err := take(1); err != nil {
		return none, err
	}
	a, b := layouts[0], layouts[1]
	if a.prism.box != b.prism.box || a.prism.sides != b.prism.sides ||
		a.loft.box.x0 != b.loft.box.x0 || a.loft.box.y0 != b.loft.box.y0 ||
		a.loft.box.y1 != b.loft.box.y1 || a.loft.sides != b.loft.sides {
		return none, ErrUnsupportedLineScene
	}
	var out [2]*Arrangement
	for i, layout := range layouts {
		arr, err := publishTwoRectangles(layout, take)
		if err != nil {
			return none, err
		}
		out[i] = arr
	}
	if err := ctx.Err(); err != nil {
		return none, err
	}
	return out, nil
}

type twoRectangleBox struct{ x0, y0, x1, y1 float64 }
type twoRectangle struct {
	box   twoRectangleBox
	sides [4]int // left, bottom, right, top; original source indices
}
type twoRectangleLayout struct {
	lines [8]*Line
	prism twoRectangle
	loft  twoRectangle
}

func certifyTwoRectangles(curves []Curve, take func(uint64) error) (twoRectangleLayout, error) {
	var no twoRectangleLayout
	if len(curves) != 8 {
		return no, ErrUnsupportedLineScene
	}
	var layout twoRectangleLayout
	for i, curve := range curves {
		if err := take(1); err != nil {
			return no, err
		}
		line, ok := curve.(*Line)
		if !ok || line == nil || line.Start == nil || line.End == nil {
			return no, ErrUnsupportedLineScene
		}
		layout.lines[i] = line
		for _, p := range []*Point{line.Start, line.End} {
			if err := take(1); err != nil {
				return no, err
			}
			if !twoRectFinite(p.X) || !twoRectFinite(p.Y) {
				return no, ErrUnsupportedLineScene
			}
		}
	}
	var err error
	layout.prism, err = certifyOneRectangle(layout.lines, 0, take)
	if err != nil {
		return no, err
	}
	layout.loft, err = certifyOneRectangle(layout.lines, 4, take)
	if err != nil {
		return no, err
	}
	p, l := layout.prism.box, layout.loft.box
	if err := take(1); err != nil {
		return no, err
	}
	span := math.Max(p.x1-p.x0, p.y1-p.y0)
	span = math.Max(span, l.x1-l.x0)
	margin := math.Max(1e-9, 1e-5*span)
	if !(p.x0+margin < l.x0 && l.x0+margin < p.x1 &&
		p.x1+margin < l.x1 && p.y0+margin < l.y0 &&
		l.y0+margin < l.y1 && l.y1+margin < p.y1) {
		return no, ErrUnsupportedLineScene
	}
	// The certified two rectangular loops imply all 28 pair relations:
	// adjacent within-loop pairs meet only at their shared source point,
	// opposite sides are disjoint, and only prism-right versus loft-bottom
	// and prism-right versus loft-top cross properly. Count the full scan.
	proper := 0
	for i := range layout.lines {
		for j := i + 1; j < len(layout.lines); j++ {
			if err := take(1); err != nil {
				return no, err
			}
			if i == layout.prism.sides[2] &&
				(j == layout.loft.sides[1] || j == layout.loft.sides[3]) {
				proper++
			}
		}
	}
	if proper != 2 {
		return no, ErrUnsupportedLineScene
	}
	return layout, nil
}

func twoRectFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) &&
		math.Abs(v) <= 1e6 && (v == 0 && !math.Signbit(v) ||
		math.Abs(v) >= math.Ldexp(1, -256))
}

func certifyOneRectangle(lines [8]*Line, offset int,
	take func(uint64) error) (twoRectangle, error) {
	var no twoRectangle
	box := twoRectangleBox{x0: math.Inf(1), y0: math.Inf(1),
		x1: math.Inf(-1), y1: math.Inf(-1)}
	for _, line := range lines[offset : offset+4] {
		for _, p := range []*Point{line.Start, line.End} {
			if err := take(1); err != nil {
				return no, err
			}
			box.x0, box.y0 = math.Min(box.x0, p.X), math.Min(box.y0, p.Y)
			box.x1, box.y1 = math.Max(box.x1, p.X), math.Max(box.y1, p.Y)
		}
	}
	if !(box.x0 < box.x1 && box.y0 < box.y1) {
		return no, ErrUnsupportedLineScene
	}
	var corners [4]*Point // lower-left, lower-right, upper-right, upper-left
	result := twoRectangle{box: box}
	var seen [4]bool
	for i := offset; i < offset+4; i++ {
		if err := take(1); err != nil {
			return no, err
		}
		line := lines[i]
		for _, p := range []*Point{line.Start, line.End} {
			corner := -1
			switch {
			case p.X == box.x0 && p.Y == box.y0:
				corner = 0
			case p.X == box.x1 && p.Y == box.y0:
				corner = 1
			case p.X == box.x1 && p.Y == box.y1:
				corner = 2
			case p.X == box.x0 && p.Y == box.y1:
				corner = 3
			}
			if corner < 0 || corners[corner] != nil && corners[corner] != p {
				return no, ErrUnsupportedLineScene
			}
			corners[corner] = p
		}
		side := -1
		a, b := line.Start, line.End
		switch {
		case a.X == box.x0 && b.X == box.x0 && a.Y != b.Y:
			side = 0
		case a.Y == box.y0 && b.Y == box.y0 && a.X != b.X:
			side = 1
		case a.X == box.x1 && b.X == box.x1 && a.Y != b.Y:
			side = 2
		case a.Y == box.y1 && b.Y == box.y1 && a.X != b.X:
			side = 3
		}
		if side < 0 || seen[side] {
			return no, ErrUnsupportedLineScene
		}
		seen[side] = true
		result.sides[side] = i
	}
	for _, p := range corners {
		if err := take(1); err != nil {
			return no, err
		}
		if p == nil {
			return no, ErrUnsupportedLineScene
		}
	}
	return result, nil
}

type twoRectPoint [2]float64
type twoRectFragmentKey struct {
	source int
	lo, hi uint64
}

func publishTwoRectangles(layout twoRectangleLayout,
	take func(uint64) error) (*Arrangement, error) {
	p, l := layout.prism.box, layout.loft.box
	cross := [2]twoRectPoint{{p.x1, l.y0}, {p.x1, l.y1}}
	if err := take(2); err != nil { // allocate and validate two crossing tokens
		return nil, err
	}
	var tokens [2]*LineLineCrossing
	for k, side := range [2]int{1, 3} {
		a, b := layout.prism.sides[2], layout.loft.sides[side]
		if a > b {
			a, b = b, a
		}
		tokens[k] = &LineLineCrossing{sources: [2]int{a, b},
			point: [2]float64(cross[k])}
	}
	// Reserve 16 polygon points, three regions, up to 16 fragment keys,
	// four incidence maps of at most 16 keys each, and five map headers.
	if err := take(104); err != nil {
		return nil, err
	}
	polygons := [3][]twoRectPoint{
		{{p.x0, p.y0}, {p.x1, p.y0}, cross[0], {l.x0, l.y0},
			{l.x0, l.y1}, cross[1], {p.x1, p.y1}, {p.x0, p.y1}},
		{{l.x0, l.y0}, cross[0], cross[1], {l.x0, l.y1}},
		{cross[0], {l.x1, l.y0}, {l.x1, l.y1}, cross[1]},
	}
	arr := &Arrangement{Regions: make([]*Region, 0, 3)}
	uses := make(map[twoRectFragmentKey][]bool, 16)
	incidence := [2][2]map[twoRectFragmentKey]struct{}{}
	for k := range incidence {
		for op := range incidence[k] {
			incidence[k][op] = make(map[twoRectFragmentKey]struct{}, 16)
		}
	}
	for _, polygon := range polygons {
		if err := take(uint64(len(polygon) + 1)); err != nil {
			return nil, err
		}
		region := &Region{Outer: make([]BoundaryEdge, 0, len(polygon))}
		area := new(big.Rat)
		for i, start := range polygon {
			end := polygon[(i+1)%len(polygon)]
			if err := take(4); err != nil { // source scan, fragment, exact area products
				return nil, err
			}
			edge, err := twoRectEdge(layout.lines, start, end, cross, tokens, take)
			if err != nil {
				return nil, err
			}
			region.Outer = append(region.Outer, edge)
			a := new(big.Rat).SetFloat64(start[0])
			b := new(big.Rat).SetFloat64(start[1])
			c := new(big.Rat).SetFloat64(end[0])
			d := new(big.Rat).SetFloat64(end[1])
			term := new(big.Rat).Sub(new(big.Rat).Mul(a, d), new(big.Rat).Mul(b, c))
			area.Add(area, term)
			key := twoRectFragmentKey{edge.SourceIndex, math.Float64bits(edge.TStart),
				math.Float64bits(edge.TEnd)}
			if err := take(1); err != nil {
				return nil, err
			}
			uses[key] = append(uses[key], edge.Reversed)
			for k, token := range tokens {
				if edge.crossStart == token || edge.crossEnd == token {
					op := 0
					if edge.SourceIndex != layout.prism.sides[2] {
						op = 1
					}
					incidence[k][op][key] = struct{}{}
				}
			}
		}
		if err := take(1); err != nil {
			return nil, err
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
	if err := take(uint64(len(uses) + 4)); err != nil {
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
	for k := range incidence {
		if len(incidence[k][0]) != 2 || len(incidence[k][1]) != 2 {
			return nil, ErrUnsupportedLineScene
		}
	}
	return arr, nil
}

func twoRectEdge(lines [8]*Line, start, end twoRectPoint,
	cross [2]twoRectPoint, tokens [2]*LineLineCrossing,
	take func(uint64) error) (BoundaryEdge, error) {
	var no BoundaryEdge
	index := -1
	for i, line := range lines {
		if err := take(1); err != nil {
			return no, err
		}
		a, b := line.Start, line.End
		match := a.X == b.X && start[0] == a.X && end[0] == a.X &&
			start[1] >= math.Min(a.Y, b.Y) && start[1] <= math.Max(a.Y, b.Y) &&
			end[1] >= math.Min(a.Y, b.Y) && end[1] <= math.Max(a.Y, b.Y) ||
			a.Y == b.Y && start[1] == a.Y && end[1] == a.Y &&
				start[0] >= math.Min(a.X, b.X) && start[0] <= math.Max(a.X, b.X) &&
				end[0] >= math.Min(a.X, b.X) && end[0] <= math.Max(a.X, b.X)
		if match {
			if index >= 0 {
				return no, ErrUnsupportedLineScene
			}
			index = i
		}
	}
	if index < 0 || start == end {
		return no, ErrUnsupportedLineScene
	}
	line := lines[index]
	coordinate := func(p twoRectPoint) float64 {
		if line.Start.X == line.End.X {
			return p[1]
		}
		return p[0]
	}
	a, b := line.Start.X, line.End.X
	if line.Start.X == line.End.X {
		a, b = line.Start.Y, line.End.Y
	}
	if err := take(6); err != nil { // exact parameter operations before allocation
		return no, err
	}
	den := new(big.Rat).Sub(new(big.Rat).SetFloat64(b), new(big.Rat).SetFloat64(a))
	param := func(p twoRectPoint) *big.Rat {
		num := new(big.Rat).Sub(new(big.Rat).SetFloat64(coordinate(p)),
			new(big.Rat).SetFloat64(a))
		return num.Quo(num, den)
	}
	t0, t1 := param(start), param(end)
	zero, one := new(big.Rat), big.NewRat(1, 1)
	if t0.Cmp(zero) < 0 || t0.Cmp(one) > 0 || t1.Cmp(zero) < 0 ||
		t1.Cmp(one) > 0 || t0.Cmp(t1) == 0 {
		return no, ErrUnsupportedLineScene
	}
	reversed := t0.Cmp(t1) > 0
	if reversed {
		t0, t1 = t1, t0
	}
	f0, _ := t0.Float64()
	f1, _ := t1.Float64()
	if !(f0 < f1) || a+f0*(b-a) != coordinate(func() twoRectPoint {
		if reversed {
			return end
		}
		return start
	}()) || a+f1*(b-a) != coordinate(func() twoRectPoint {
		if reversed {
			return start
		}
		return end
	}()) {
		return no, ErrUnsupportedLineScene
	}
	edge := BoundaryEdge{SourceIndex: index, Whole: t0.Cmp(zero) == 0 && t1.Cmp(one) == 0,
		Reversed: reversed, TStart: f0, TEnd: f1, TExact: true,
		Polyline: [][2]float64{[2]float64(start), [2]float64(end)}}
	for k, point := range cross {
		if err := take(1); err != nil {
			return no, err
		}
		if index != tokens[k].sources[0] && index != tokens[k].sources[1] {
			continue
		}
		if start == point {
			edge.crossStart = tokens[k]
		}
		if end == point {
			edge.crossEnd = tokens[k]
		}
	}
	return edge, nil
}
