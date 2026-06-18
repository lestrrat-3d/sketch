package geom

// Curve is an open curve bounded by two endpoints: a *Line, an *Arc or an
// *EllipticalArc. Closed primitives (Circle, Ellipse) form loops on their own
// and do not participate in chain detection.
type Curve interface {
	Endpoints() (*Point, *Point)
}

// Endpoints returns the line's endpoints.
func (l *Line) Endpoints() (*Point, *Point) { return l.Start, l.End }

// Endpoints returns the arc's endpoints (start and end; the center is not an
// endpoint).
func (a *Arc) Endpoints() (*Point, *Point) { return a.Start, a.End }

// Loop is a closed chain of curves: consecutive curves share an endpoint
// *Point and the last curve closes back to the first. Curves appear in walk
// order.
type Loop struct {
	Curves []Curve
}

// Loops finds closed loops among open curves. Connectivity is by endpoint
// identity — two curves connect when they share the same *Point, which is the
// natural state for sketch geometry built with shared points (coincidence by
// coordinates, without a shared point, does not connect; splitting curves at
// bare crossings is a future extension). Each curve belongs to at most one
// loop. At a junction used by more than two curves the walk follows the first
// unused curve, so ambiguous subdivisions resolve deterministically (input
// order) but arbitrarily.
func Loops(curves []Curve) []*Loop {
	adj := map[*Point][]Curve{}
	for _, c := range curves {
		a, b := c.Endpoints()
		if a == nil || b == nil || a == b {
			continue // degenerate curve
		}
		adj[a] = append(adj[a], c)
		adj[b] = append(adj[b], c)
	}

	used := map[Curve]struct{}{}
	var loops []*Loop
	for _, start := range curves {
		if _, ok := used[start]; ok {
			continue
		}
		home, at := start.Endpoints()
		if home == nil || at == nil || home == at {
			continue
		}
		chain := []Curve{start}
		visited := map[Curve]struct{}{start: {}}
		for at != home {
			var next Curve
			for _, c := range adj[at] {
				if _, ok := used[c]; ok {
					continue
				}
				if _, ok := visited[c]; ok {
					continue
				}
				next = c
				break
			}
			if next == nil {
				chain = nil // dead end: no loop through start
				break
			}
			chain = append(chain, next)
			visited[next] = struct{}{}
			na, nb := next.Endpoints()
			if na == at {
				at = nb
			} else {
				at = na
			}
		}
		if chain == nil {
			continue
		}
		loops = append(loops, &Loop{Curves: chain})
		for _, c := range chain {
			used[c] = struct{}{}
		}
	}
	return loops
}
