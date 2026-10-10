package sketch

import (
	"fmt"
	"math"
	"slices"
)

// UnionProfiles joins selected regions from the current Profiles snapshot.
// Indices are positions in Sketch.Profiles, and the result is one connected
// profile. Shared entity intervals cancel even when one region publishes a
// whole circle and its neighbour publishes only part of that circle. An
// unmatched split on a non-circular entity is refused.
//
// The returned profile stores the selected indices so a consumer can rebuild
// and authenticate it against the same sketch revision. Its boundary edges
// retain their original entities and exactness; this method never upgrades a
// sampled trim to an exact one.
func (s *Sketch) UnionProfiles(indices ...int) (*Profile, error) {
	if len(indices) < 2 {
		return nil, fmt.Errorf("sketch: a profile union needs at least two regions")
	}
	profiles := s.Profiles()
	selected := append([]int(nil), indices...)
	slices.Sort(selected)
	byEntity := make(map[Entity][]BoundaryEdge)
	var area float64
	for i, index := range selected {
		if index < 0 || index >= len(profiles) || (i > 0 && index == selected[i-1]) {
			return nil, fmt.Errorf("sketch: invalid profile union index %d", index)
		}
		p := profiles[index]
		if !p.Valid {
			return nil, fmt.Errorf("sketch: profile union region %d is invalid", index)
		}
		area += p.Area
		for _, edge := range p.Outer {
			byEntity[edge.Entity] = append(byEntity[edge.Entity], edge)
		}
		for _, hole := range p.Holes {
			for _, edge := range hole {
				byEntity[edge.Entity] = append(byEntity[edge.Entity], edge)
			}
		}
	}

	var exposed []BoundaryEdge
	for entity, edges := range byEntity {
		parts, err := unionEntityIntervals(entity, edges)
		if err != nil {
			return nil, err
		}
		exposed = append(exposed, parts...)
	}
	if len(exposed) == 0 {
		return nil, fmt.Errorf("sketch: profile union has no boundary")
	}
	loops, err := unionBoundaryLoops(exposed)
	if err != nil {
		return nil, err
	}
	for i := range loops {
		loops[i] = canonicalUnionLoop(loops[i])
	}
	slices.SortFunc(loops, func(a, b []BoundaryEdge) int {
		return compareUnionPoints(a[0].Polyline[0], b[0].Polyline[0])
	})
	out := &Profile{Area: area, Valid: true, sketch: s, revision: s.Revision(),
		unionRegionIndices: selected}
	for _, loop := range loops {
		if unionLoopArea(loop) > 0 {
			if out.Outer != nil {
				return nil, fmt.Errorf("sketch: profile union is disconnected")
			}
			out.Outer = loop
		} else {
			out.Holes = append(out.Holes, loop)
		}
	}
	if out.Outer == nil {
		return nil, fmt.Errorf("sketch: profile union has no outer boundary")
	}
	seen := make(map[Entity]struct{})
	for _, edge := range out.Outer {
		if _, ok := seen[edge.Entity]; ok {
			continue
		}
		seen[edge.Entity] = struct{}{}
		out.Entities = append(out.Entities, edge.Entity)
	}
	return out, nil
}

func unionEntityIntervals(entity Entity, edges []BoundaryEdge) ([]BoundaryEdge, error) {
	breaks := make([]float64, 0, 2*len(edges))
	for _, edge := range edges {
		if edge.Entity != entity || math.IsNaN(edge.TStart) || math.IsInf(edge.TStart, 0) ||
			math.IsNaN(edge.TEnd) || math.IsInf(edge.TEnd, 0) ||
			edge.TStart < 0 || edge.TEnd > 1 || edge.TStart >= edge.TEnd {
			return nil, fmt.Errorf("sketch: profile union has an invalid edge range")
		}
		breaks = append(breaks, edge.TStart, edge.TEnd)
	}
	slices.Sort(breaks)
	breaks = slices.Compact(breaks)
	var out []BoundaryEdge
	for i := 0; i+1 < len(breaks); i++ {
		lo, hi := breaks[i], breaks[i+1]
		mid := lo + (hi-lo)/2
		if !(lo < mid && mid < hi) {
			return nil, fmt.Errorf("sketch: profile union has an unresolved interval")
		}
		balance := 0
		var contributor *BoundaryEdge
		for j := range edges {
			edge := &edges[j]
			if !(edge.TStart <= mid && mid <= edge.TEnd) {
				continue
			}
			if edge.Reversed {
				balance--
			} else {
				balance++
			}
			contributor = edge
		}
		if balance == 0 {
			continue
		}
		if balance < -1 || balance > 1 {
			return nil, fmt.Errorf("sketch: profile union overlaps an entity interval")
		}
		for j := range edges {
			edge := &edges[j]
			if edge.TStart <= mid && mid <= edge.TEnd && edge.Reversed == (balance < 0) {
				contributor = edge
				break
			}
		}
		if contributor == nil {
			return nil, fmt.Errorf("sketch: profile union cannot name an exposed interval")
		}
		if contributor.TStart == lo && contributor.TEnd == hi {
			out = append(out, *contributor)
			continue
		}
		circle, ok := entity.(*Circle)
		if !ok {
			return nil, fmt.Errorf("sketch: profile union cannot split %T", entity)
		}
		start, startExact := unionCircleEndpoint(edges, lo)
		end, endExact := unionCircleEndpoint(edges, hi)
		if !startExact || !endExact {
			return nil, fmt.Errorf("sketch: profile union cannot certify a circle split")
		}
		part := *contributor
		part.TStart, part.TEnd = lo, hi
		part.Partial = true
		part.TExact = contributor.TExact && startExact && endExact
		part.Polyline = unionCirclePolyline(circle, lo, hi, start, end, part.Reversed)
		out = append(out, part)
	}
	return out, nil
}

func unionCircleEndpoint(edges []BoundaryEdge, t float64) ([2]float64, bool) {
	for _, edge := range edges {
		if len(edge.Polyline) == 0 || !edge.TExact {
			continue
		}
		if edge.TStart == t {
			if edge.Reversed {
				return edge.Polyline[len(edge.Polyline)-1], true
			}
			return edge.Polyline[0], true
		}
		if edge.TEnd == t {
			if edge.Reversed {
				return edge.Polyline[0], true
			}
			return edge.Polyline[len(edge.Polyline)-1], true
		}
	}
	return [2]float64{}, false
}

func unionCirclePolyline(circle *Circle, lo, hi float64, start, end [2]float64, reversed bool) [][2]float64 {
	count := max(2, int(math.Ceil((hi-lo)*128))+1)
	points := make([][2]float64, count)
	cx, cy := circle.Center.X(), circle.Center.Y()
	radius := circle.R()
	for i := range points {
		t := lo + (hi-lo)*float64(i)/float64(count-1)
		angle := 2 * math.Pi * t
		points[i] = [2]float64{cx + radius*math.Cos(angle), cy + radius*math.Sin(angle)}
	}
	points[0], points[len(points)-1] = start, end
	if reversed {
		slices.Reverse(points)
	}
	return points
}

type unionPointKey struct{ x, y uint64 }

func unionKey(p [2]float64) unionPointKey {
	if p[0] == 0 {
		p[0] = 0
	}
	if p[1] == 0 {
		p[1] = 0
	}
	return unionPointKey{math.Float64bits(p[0]), math.Float64bits(p[1])}
}

func unionBoundaryLoops(edges []BoundaryEdge) ([][]BoundaryEdge, error) {
	outgoing := make(map[unionPointKey]int, len(edges))
	incoming := make(map[unionPointKey]int, len(edges))
	for i, edge := range edges {
		if len(edge.Polyline) < 2 {
			return nil, fmt.Errorf("sketch: profile union has an edge without endpoints")
		}
		start := unionKey(edge.Polyline[0])
		end := unionKey(edge.Polyline[len(edge.Polyline)-1])
		if _, exists := outgoing[start]; exists {
			return nil, fmt.Errorf("sketch: profile union has a branched boundary")
		}
		if _, exists := incoming[end]; exists {
			return nil, fmt.Errorf("sketch: profile union has a branched boundary")
		}
		outgoing[start], incoming[end] = i, i
	}
	if len(incoming) != len(outgoing) {
		return nil, fmt.Errorf("sketch: profile union has an open boundary")
	}
	seen := make([]bool, len(edges))
	var loops [][]BoundaryEdge
	for first := range edges {
		if seen[first] {
			continue
		}
		var loop []BoundaryEdge
		at := first
		for {
			if seen[at] {
				if at != first {
					return nil, fmt.Errorf("sketch: profile union has an open boundary")
				}
				break
			}
			seen[at] = true
			loop = append(loop, edges[at])
			end := unionKey(edges[at].Polyline[len(edges[at].Polyline)-1])
			next, ok := outgoing[end]
			if !ok {
				return nil, fmt.Errorf("sketch: profile union has an open boundary")
			}
			at = next
		}
		loops = append(loops, loop)
	}
	return loops, nil
}

func unionLoopArea(loop []BoundaryEdge) float64 {
	var twice float64
	for _, edge := range loop {
		for i := 1; i < len(edge.Polyline); i++ {
			a, b := edge.Polyline[i-1], edge.Polyline[i]
			twice += a[0]*b[1] - b[0]*a[1]
		}
	}
	return twice / 2
}

func canonicalUnionLoop(loop []BoundaryEdge) []BoundaryEdge {
	first := 0
	for i := 1; i < len(loop); i++ {
		if compareUnionPoints(loop[i].Polyline[0], loop[first].Polyline[0]) < 0 {
			first = i
		}
	}
	return slices.Concat(loop[first:], loop[:first])
}

func compareUnionPoints(a, b [2]float64) int {
	if a[0] < b[0] {
		return -1
	}
	if a[0] > b[0] {
		return 1
	}
	if a[1] < b[1] {
		return -1
	}
	if a[1] > b[1] {
		return 1
	}
	return 0
}
