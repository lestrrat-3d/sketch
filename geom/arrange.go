package geom

import (
	"math"
	"sort"
)

// Regions builds the planar arrangement of the given open curves (lines and
// arcs) and closed primitives (circles and ellipses), splitting every curve at
// its bare crossings with the others, and returns the minimal bounded regions
// — each an outer boundary plus any holes, with a net area and source-curve
// back-references — together with self-intersection signals.
//
// SourceIndex on a returned BoundaryEdge indexes curves for an open curve, or
// len(curves)+k for the k-th entry of closed. The arrangement is built on an
// adaptive polyline sampling of each curve, so a region's topology is exact for
// well-separated geometry; areas of line/arc/circle regions are computed in
// closed form (sampling-independent).
func Regions(curves []Curve, closed []ClosedCurve, opts ...Option) *Arrangement {
	cfg := arrangeConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	a := newArranger(curves, closed, cfg)
	a.densify()
	a.intersect()
	a.split()
	a.prune()
	a.buildGraph()
	return a.extract()
}

// Option configures Regions.
type Option func(*arrangeConfig)

type arrangeConfig struct {
	vertexMerge float64 // 0 => derived from the scene scale
	segsPerTurn int     // 0 => adaptive from the scene scale
}

// WithVertexMerge overrides the distance below which two arrangement points are
// treated as one vertex. Zero (the default) derives it from the scene size.
func WithVertexMerge(eps float64) Option {
	return func(c *arrangeConfig) { c.vertexMerge = eps }
}

// WithSegmentsPerTurn overrides the number of straight segments a full circle
// (or 2π of arc/ellipse) is sampled into. Zero (the default) chooses adaptively.
func WithSegmentsPerTurn(n int) Option {
	return func(c *arrangeConfig) { c.segsPerTurn = n }
}

// source carries enough of an input curve to evaluate a point at a natural
// parameter t∈[0,1] and the exact area contribution of one of its fragments.
type source struct {
	kind   srcKind
	closed bool
	// line
	ax, ay, bx, by float64
	// arc / circle / ellipse
	cx, cy      float64
	r           float64 // arc/circle radius
	phi0, sweep float64 // arc start angle and signed sweep (circle: 0, 2π)
	rx, ry, rot float64 // ellipse
	// elliptical arc: the exact boundary points, used to pin t=0/t=1 (the
	// eccentric-angle sampling would otherwise project an off-ellipse endpoint).
	pinEnds            bool
	e0x, e0y, e1x, e1y float64
	// spline: control-point coordinates for Cox–de Boor evaluation.
	ctrl [][2]float64
	// fit-point spline: a prebuilt natural-cubic interpolant (the tridiagonal
	// solve runs once when the source is created, then is reused per sample).
	fitEval *fitEvaluator
}

type srcKind int

const (
	srcLine srcKind = iota
	srcArc
	srcCircle
	srcEllipse
	srcEllipticalArc // an ellipse restricted to an eccentric-angle sweep
	srcSpline        // a clamped cubic B-spline (open; may self-cross)
	srcClosedSpline  // a periodic cubic B-spline (closed loop; may self-cross)
	srcFitSpline     // a natural-cubic interpolating spline (open; may self-cross)
	srcDegenerate    // unsupported / nil input; contributes no geometry
)

// at returns the source point at natural parameter t.
func (s *source) at(t float64) [2]float64 {
	switch s.kind {
	case srcLine:
		return [2]float64{s.ax + t*(s.bx-s.ax), s.ay + t*(s.by-s.ay)}
	case srcArc:
		ang := s.phi0 + t*s.sweep
		return [2]float64{s.cx + s.r*math.Cos(ang), s.cy + s.r*math.Sin(ang)}
	case srcCircle:
		ang := 2 * math.Pi * t
		return [2]float64{s.cx + s.r*math.Cos(ang), s.cy + s.r*math.Sin(ang)}
	case srcEllipticalArc:
		return s.ellipsePoint(s.phi0 + t*s.sweep)
	case srcSpline:
		x, y := EvalCubicBSpline(s.ctrl, t)
		return [2]float64{x, y}
	case srcClosedSpline:
		x, y := EvalPeriodicCubicBSpline(s.ctrl, t)
		return [2]float64{x, y}
	case srcFitSpline:
		return s.fitEval.at(t)
	default: // ellipse
		return s.ellipsePoint(2 * math.Pi * t)
	}
}

// ellipsePoint evaluates the source's ellipse at eccentric angle ang.
func (s *source) ellipsePoint(ang float64) [2]float64 {
	lx, ly := s.rx*math.Cos(ang), s.ry*math.Sin(ang)
	cosr, sinr := math.Cos(s.rot), math.Sin(s.rot)
	return [2]float64{s.cx + lx*cosr - ly*sinr, s.cy + lx*sinr + ly*cosr}
}

// differential returns the first and second derivatives of the source's position
// with respect to its natural parameter t, for the kinds with a closed-form
// tangent/curvature (line/circle/arc). ok=false for the sampled-only kinds
// (ellipse/spline/elliptical-arc), which keep chord-based half-edge ordering. This
// is the exact local geometry the analytic port ordering needs at a shared vertex,
// where chord directions tie (a tangency) and would branch-swap the face walk.
func (s *source) differential(t float64) (d1, d2 [2]float64, ok bool) {
	switch s.kind {
	case srcLine:
		return [2]float64{s.bx - s.ax, s.by - s.ay}, [2]float64{0, 0}, true
	case srcCircle:
		ang := 2 * math.Pi * t
		w := 2 * math.Pi
		sin, cos := math.Sin(ang), math.Cos(ang)
		d1 = [2]float64{-w * s.r * sin, w * s.r * cos}
		d2 = [2]float64{-w * w * s.r * cos, -w * w * s.r * sin}
		return d1, d2, true
	case srcArc:
		ang := s.phi0 + t*s.sweep
		w := s.sweep
		sin, cos := math.Sin(ang), math.Cos(ang)
		d1 = [2]float64{-w * s.r * sin, w * s.r * cos}
		d2 = [2]float64{-w * w * s.r * cos, -w * w * s.r * sin}
		return d1, d2, true
	}
	return [2]float64{}, [2]float64{}, false
}

// tinySeg is one straight segment of a source's polyline, tagged with the
// source and the natural parameters at its endpoints.
type tinySeg struct {
	src    int
	pa, pb float64
	ax, ay float64
	bx, by float64
	cuts   []cut // segment-local crossings that split it
}

// cut is a crossing that splits a tiny segment at segment-local parameter t, with
// the EXACT crossing point (px,py). A sampled crossing stores the segment
// intersection point (identical to chord interpolation, so the sampled path is
// unchanged); an ANALYTIC crossing stores the exact curve intersection point, so
// two sources cut at the same event canonicalize to ONE vertex (chord
// interpolation of two different sources' params would otherwise miss).
type cut struct {
	t      float64
	px, py float64
}

// arranger holds the working state of one Regions call.
type arranger struct {
	sources []source
	segs    []tinySeg
	cfg     arrangeConfig
	scale   float64
	merge   float64

	verts     vertexTable
	edges     []arrEdge        // undirected arrangement edges
	halfs     []halfEdge       // directed half-edges (two per edge)
	selfX     [][2]float64     // self-intersection points
	selfXc    map[int]struct{} // components that self-intersect
	notSimple map[int]struct{} // core components that are NOT a simple closed loop (some vertex degree != 2)
	core      []bool           // per source: part of the cycle-bearing core (not a dangling spur)
	comp      []int            // per source: core component id, or -1 if not core
	srcCut    []bool           // per source: split by at least one crossing (so its edges are fragments)
	degen     [][2]float64     // points of degenerate (collinear-overlap / unresolvable) conditions
	degenSet  bool

	// Analytic-arrangement state (increment 2): which line/circle/arc source pairs
	// were classified analytically (so the sampled segment loop skips them), and a
	// per-source segment index for mapping an analytic event's source parameter to
	// the tiny segment it cuts.
	handled    map[[2]int]struct{}
	sourceSegs [][]int

	// Certified analytic tangency contacts (increment 3): the exact points where
	// the rotation system must order coincident-tangent ports by curvature instead
	// of by chord direction. Used ONLY at these vertices — at a sampled crossing the
	// edges are chords, so chord ordering (not exact tangents) matches the geometry
	// the face walk traverses.
	exactPortVerts [][2]float64
}

// arrEdge is an undirected arrangement edge between two canonical vertices,
// carrying its source and the natural param range along that source.
type arrEdge struct {
	u, v   int
	src    int
	pu, pv float64
}

func newArranger(curves []Curve, closed []ClosedCurve, cfg arrangeConfig) *arranger {
	a := &arranger{cfg: cfg, selfXc: map[int]struct{}{}}
	a.sources = make([]source, 0, len(curves)+len(closed))

	// Safe endpoints per curve (handles a typed-nil Curve or nil endpoints
	// without dereferencing). ok=false marks an unusable curve.
	ends := make([][2]*Point, len(curves))
	endsOK := make([]bool, len(curves))
	for i, c := range curves {
		p, q, ok := safeEndpoints(c)
		ends[i] = [2]*Point{p, q}
		endsOK[i] = ok && p != nil && q != nil
	}

	// Identify the cycle-bearing "core": iteratively drop curves that have a
	// degree-1 endpoint (dangling spurs and trees). Self-intersection and the
	// simple-loop test are judged on this core, so a bowtie with a spur attached
	// is still recognised as a self-crossing loop once the spur is pruned.
	core := make([]bool, len(curves))
	for i := range curves {
		core[i] = endsOK[i]
	}
	for {
		deg := map[*Point]int{}
		for i := range curves {
			if !core[i] {
				continue
			}
			deg[ends[i][0]]++
			deg[ends[i][1]]++
		}
		changed := false
		for i := range curves {
			if core[i] && (deg[ends[i][0]] <= 1 || deg[ends[i][1]] <= 1) {
				core[i] = false
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	// Connected components + per-point degree over the core only.
	uf := newUnionFind(len(curves))
	coreDeg := map[*Point]int{}
	endpoint := map[*Point]int{}
	for i := range curves {
		if !core[i] {
			continue
		}
		for _, e := range ends[i] {
			coreDeg[e]++
			if j, ok := endpoint[e]; ok {
				uf.union(i, j)
			} else {
				endpoint[e] = i
			}
		}
	}
	// A core component is a simple loop unless one of its vertices has degree != 2.
	notSimple := map[int]struct{}{}
	for i := range curves {
		if !core[i] {
			continue
		}
		if coreDeg[ends[i][0]] != 2 || coreDeg[ends[i][1]] != 2 {
			notSimple[uf.find(i)] = struct{}{}
		}
	}
	a.notSimple = notSimple
	total := len(curves) + len(closed)
	a.core = make([]bool, total)
	a.comp = make([]int, total)
	for i := range curves {
		a.core[i] = core[i]
		if core[i] {
			a.comp[i] = uf.find(i)
		} else {
			a.comp[i] = -1
		}
	}

	for _, c := range curves {
		s := source{}
		switch t := c.(type) {
		case *Line:
			if t == nil || t.Start == nil || t.End == nil {
				a.flagDegenerate(0, 0)
				s.kind = srcDegenerate
				break
			}
			s.kind = srcLine
			s.ax, s.ay, s.bx, s.by = t.Start.X, t.Start.Y, t.End.X, t.End.Y
		case *Arc:
			if t == nil || t.Center == nil || t.Start == nil || t.End == nil {
				a.flagDegenerate(0, 0)
				s.kind = srcDegenerate
				break
			}
			if r := t.Radius(); !posFinite(r) { // start coincides with center
				a.flagDegenerate(t.Center.X, t.Center.Y)
				s.kind = srcDegenerate
				break
			}
			s.kind = srcArc
			s.cx, s.cy = t.Center.X, t.Center.Y
			s.r = t.Radius()
			s.phi0 = t.StartAngle()
			s.sweep = t.Sweep()
		case *EllipticalArc:
			if t == nil || t.Center == nil || t.Start == nil || t.End == nil ||
				!posFinite(t.Rx) || !posFinite(t.Ry) {
				if t != nil && t.Center != nil {
					a.flagDegenerate(t.Center.X, t.Center.Y)
				} else {
					a.flagDegenerate(0, 0)
				}
				s.kind = srcDegenerate
				break
			}
			s.kind = srcEllipticalArc
			s.cx, s.cy = t.Center.X, t.Center.Y
			s.rx, s.ry, s.rot = t.Rx, t.Ry, t.Rotation
			s.phi0 = t.StartParam()
			s.sweep = t.Sweep()
			s.pinEnds = true
			s.e0x, s.e0y, s.e1x, s.e1y = t.Start.X, t.Start.Y, t.End.X, t.End.Y
		case *Spline:
			cc, ok := splineControlCoords(t)
			if !ok {
				a.flagDegenerate(0, 0)
				s.kind = srcDegenerate
				break
			}
			// A spline whose control points are all coincident has no geometric
			// extent — it is a point, not a curve. Flag it rather than silently
			// dropping its collapsed (zero-length) segments.
			if splineExtent(cc) < 1e-9 {
				a.flagDegenerate(cc[0][0], cc[0][1])
				s.kind = srcDegenerate
				break
			}
			s.kind = srcSpline
			s.ctrl = cc
		case *FitSpline:
			coords, ok := fitSplineCoords(t)
			if !ok {
				a.flagDegenerate(0, 0)
				s.kind = srcDegenerate
				break
			}
			if splineExtent(coords) < 1e-9 { // all-coincident fit points: a point
				a.flagDegenerate(coords[0][0], coords[0][1])
				s.kind = srcDegenerate
				break
			}
			s.kind = srcFitSpline
			s.fitEval = newFitEvaluator(coords)
		default:
			a.flagDegenerate(0, 0) // unknown Curve implementation
			s.kind = srcDegenerate
		}
		a.sources = append(a.sources, s)
	}
	base := len(curves)
	for k, cc := range closed {
		s := source{closed: true}
		a.core[base+k] = true
		a.comp[base+k] = base + k // each closed curve is its own component
		switch t := cc.(type) {
		case *Circle:
			if t == nil || t.Center == nil || !posFinite(t.Radius) {
				if t != nil && t.Center != nil {
					a.flagDegenerate(t.Center.X, t.Center.Y)
				} else {
					a.flagDegenerate(0, 0)
				}
				s.kind = srcDegenerate
				break
			}
			s.kind = srcCircle
			s.cx, s.cy, s.r = t.Center.X, t.Center.Y, t.Radius
		case *Ellipse:
			if t == nil || t.Center == nil || !posFinite(t.Rx) || !posFinite(t.Ry) {
				if t != nil && t.Center != nil {
					a.flagDegenerate(t.Center.X, t.Center.Y)
				} else {
					a.flagDegenerate(0, 0)
				}
				s.kind = srcDegenerate
				break
			}
			s.kind = srcEllipse
			s.cx, s.cy = t.Center.X, t.Center.Y
			s.rx, s.ry, s.rot = t.Rx, t.Ry, t.Rotation
		case *ClosedSpline:
			coords, ok := closedSplineControlCoords(t)
			if !ok {
				a.flagDegenerate(0, 0)
				s.kind = srcDegenerate
				break
			}
			if splineExtent(coords) < 1e-9 { // all-coincident controls: a point
				a.flagDegenerate(coords[0][0], coords[0][1])
				s.kind = srcDegenerate
				break
			}
			s.kind = srcClosedSpline
			s.ctrl = coords
		default:
			a.flagDegenerate(0, 0) // unsupported ClosedCurve implementation
			s.kind = srcDegenerate
		}
		a.sources = append(a.sources, s)
	}
	return a
}

// safeEndpoints returns a curve's endpoints, handling a typed-nil or
// unsupported Curve without dereferencing (ok=false then).
func safeEndpoints(c Curve) (*Point, *Point, bool) {
	switch t := c.(type) {
	case *Line:
		if t == nil {
			return nil, nil, false
		}
		return t.Start, t.End, true
	case *Arc:
		if t == nil {
			return nil, nil, false
		}
		return t.Start, t.End, true
	case *EllipticalArc:
		if t == nil {
			return nil, nil, false
		}
		return t.Start, t.End, true
	case *Spline:
		if _, ok := splineControlCoords(t); !ok {
			return nil, nil, false
		}
		return t.Control[0], t.Control[len(t.Control)-1], true
	case *FitSpline:
		if _, ok := fitSplineCoords(t); !ok {
			return nil, nil, false
		}
		return t.Fit[0], t.Fit[len(t.Fit)-1], true
	default:
		return nil, nil, false
	}
}

// fitSplineCoords validates a fit-point spline's points and returns their
// coordinates. ok is false for a typed-nil spline, fewer than two fit points, or
// any nil fit point.
func fitSplineCoords(sp *FitSpline) ([][2]float64, bool) {
	if sp == nil || len(sp.Fit) < 2 {
		return nil, false
	}
	cc := make([][2]float64, len(sp.Fit))
	for i, p := range sp.Fit {
		if p == nil {
			return nil, false
		}
		cc[i] = [2]float64{p.X, p.Y}
	}
	return cc, true
}

// splineControlCoords validates a spline's control points and returns their
// coordinates. ok is false for a typed-nil spline, fewer than four control
// points, or any nil control point — all degenerate inputs the arrangement
// must not dereference.
func splineControlCoords(sp *Spline) ([][2]float64, bool) {
	if sp == nil || len(sp.Control) < 4 {
		return nil, false
	}
	cc := make([][2]float64, len(sp.Control))
	for i, p := range sp.Control {
		if p == nil {
			return nil, false
		}
		cc[i] = [2]float64{p.X, p.Y}
	}
	return cc, true
}

// closedSplineControlCoords validates a closed spline's control points and
// returns their coordinates. ok is false for a typed-nil spline, fewer than
// three control points, or any nil control point.
func closedSplineControlCoords(sp *ClosedSpline) ([][2]float64, bool) {
	if sp == nil || len(sp.Control) < 3 {
		return nil, false
	}
	cc := make([][2]float64, len(sp.Control))
	for i, p := range sp.Control {
		if p == nil {
			return nil, false
		}
		cc[i] = [2]float64{p.X, p.Y}
	}
	return cc, true
}

// splineExtent returns the bounding-box diagonal of the control points; a
// near-zero extent means a degenerate (point-like) spline.
func splineExtent(cc [][2]float64) float64 {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, p := range cc {
		minX, maxX = math.Min(minX, p[0]), math.Max(maxX, p[0])
		minY, maxY = math.Min(minY, p[1]), math.Max(maxY, p[1])
	}
	return math.Hypot(maxX-minX, maxY-minY)
}

// posFinite reports whether v is a positive, finite number — the requirement
// for a usable radius or semi-axis.
func posFinite(v float64) bool { return v > 0 && !math.IsInf(v, 1) }

// flagDegenerate records a degenerate condition at (x,y); the arrangement's
// regions are then not trustworthy.
func (a *arranger) flagDegenerate(x, y float64) {
	a.degenSet = true
	a.degen = append(a.degen, [2]float64{x, y})
}

// densify samples each source into tiny segments and computes the scene scale
// and the vertex-merge tolerance.
func (a *arranger) densify() {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	note := func(p [2]float64) {
		minX, maxX = math.Min(minX, p[0]), math.Max(maxX, p[0])
		minY, maxY = math.Min(minY, p[1]), math.Max(maxY, p[1])
	}
	for si := range a.sources {
		s := &a.sources[si]
		if s.kind == srcDegenerate {
			continue
		}
		params := a.sampleParams(s)
		last := len(params) - 1
		atParam := func(i int) [2]float64 {
			// Pin an elliptical arc's ends to its exact boundary points so it
			// joins its neighbours by shared-endpoint identity (the eccentric
			// sampling would otherwise project an off-ellipse endpoint).
			if s.pinEnds {
				if i == 0 {
					return [2]float64{s.e0x, s.e0y}
				}
				if i == last {
					return [2]float64{s.e1x, s.e1y}
				}
			}
			return s.at(params[i])
		}
		prev := atParam(0)
		note(prev)
		for i := 1; i <= last; i++ {
			cur := atParam(i)
			note(cur)
			a.segs = append(a.segs, tinySeg{
				src: si, pa: params[i-1], pb: params[i],
				ax: prev[0], ay: prev[1], bx: cur[0], by: cur[1],
			})
			prev = cur
		}
	}
	a.scale = math.Max(maxX-minX, maxY-minY)
	if !(a.scale > 0) || math.IsInf(a.scale, 1) {
		a.scale = 1
	}
	a.merge = a.cfg.vertexMerge
	if a.merge <= 0 {
		a.merge = a.scale * 1e-7
	}
	a.verts = newVertexTable(a.merge)
}

// sampleParams returns the natural parameters at which to sample a source.
func (a *arranger) sampleParams(s *source) []float64 {
	switch s.kind {
	case srcLine:
		return []float64{0, 1}
	case srcSpline, srcClosedSpline, srcFitSpline:
		// No analytic crossings: sample densely enough that the polyline tracks
		// the curve and a self-crossing is captured. Scale with control/fit count;
		// an explicit WithSegmentsPerTurn can only raise it. A closed spline
		// closes because at(1) == at(0) (the last sample equals the first).
		var n int
		switch s.kind {
		case srcClosedSpline:
			n = 16 * len(s.ctrl)
		case srcFitSpline:
			n = 16 * len(s.fitEval.x) // active (deduplicated) fit-point count
		default:
			n = 16 * (len(s.ctrl) - 3)
		}
		if n < 64 {
			n = 64
		}
		if a.cfg.segsPerTurn > n {
			n = a.cfg.segsPerTurn
		}
		out := make([]float64, n+1)
		for i := 0; i <= n; i++ {
			out[i] = float64(i) / float64(n)
		}
		return out
	default:
		segs := a.cfg.segsPerTurn
		if segs <= 0 {
			// Adaptive: bound the chord sagitta to ~1e-4 of the scene; capped.
			segs = 256
		}
		var turn float64
		if s.kind == srcArc || s.kind == srcEllipticalArc {
			turn = s.sweep / (2 * math.Pi)
		} else {
			turn = 1
		}
		n := int(math.Ceil(float64(segs) * turn))
		if n < 2 {
			n = 2
		}
		out := make([]float64, n+1)
		for i := 0; i <= n; i++ {
			out[i] = float64(i) / float64(n)
		}
		return out
	}
}

// intersect finds every bare crossing between tiny segments and records the
// split parameters, classifying same-component interior crossings as
// self-intersections.
func (a *arranger) intersect() {
	a.srcCut = make([]bool, len(a.sources))
	a.analyticPrepass()
	n := len(a.segs)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			si, sj := &a.segs[i], &a.segs[j]
			// Supported source pairs (line/circle/arc) were classified analytically
			// in the pre-pass; their crossings are authoritative, so the sampled
			// segment test must not add contradictory ones.
			if si.src != sj.src {
				if _, h := a.handled[pairKey(si.src, sj.src)]; h {
					continue
				}
			}
			sameSpline := false
			if si.src == sj.src {
				// A simple source's own polyline never self-crosses. A spline (open
				// or closed periodic) can, so for a spline source test non-adjacent
				// sampled segments; adjacent ones (j == i+1) merely share a
				// subdivision vertex. The closure seam (first meets last segment) is
				// handled by the param-{0,1} check in the endpoint-meeting branch.
				k := a.sources[si.src].kind
				if (k != srcSpline && k != srcClosedSpline && k != srcFitSpline) || j == i+1 {
					continue
				}
				sameSpline = true
			}
			p, ok := segParams(si, sj)
			if !ok {
				// Parallel: a collinear overlap is a duplicated/coincident edge
				// that corrupts the planar map — flag it rather than miscount.
				if mx, my, over := collinearOverlap(si, sj); over {
					a.flagDegenerate(mx, my)
				}
				continue
			}
			interiorI := p.ti > segEps && p.ti < 1-segEps
			interiorJ := p.tj > segEps && p.tj < 1-segEps
			if !interiorI && !interiorJ {
				// Two segments meeting only at endpoints is normally a join/corner.
				// But two NON-ADJACENT segments of the same spline meeting anywhere
				// means the curve revisits that point — a self-touch we must still
				// flag, since the exact crossing can land on a sample vertex. No cut
				// is recorded (the shared point is already a sample vertex).
				if !sameSpline {
					continue
				}
				// Exception: the natural closure seam of an endpoint-closed spline
				// (S(0) == S(1)) — its first and last sampled segments meet at the
				// shared endpoint. That is the intended closure, not a crossing.
				cpi := si.pa + p.ti*(si.pb-si.pa)
				cpj := sj.pa + p.tj*(sj.pb-sj.pa)
				if lo, hi := math.Min(cpi, cpj), math.Max(cpi, cpj); lo < segEps && hi > 1-segEps {
					continue
				}
			}
			// A near-tangent interior crossing is ill-conditioned at the current
			// sampling (the two curves graze rather than cleanly cross); the
			// region topology there cannot be trusted, so flag it.
			if p.sin < 1e-3 {
				a.flagDegenerate(p.x, p.y)
			}
			if interiorI {
				si.cuts = append(si.cuts, cut{t: p.ti, px: p.x, py: p.y})
				a.srcCut[si.src] = true
			}
			if interiorJ {
				sj.cuts = append(sj.cuts, cut{t: p.tj, px: p.x, py: p.y})
				a.srcCut[sj.src] = true
			}
			// Self-intersection: a single simple closed loop (its core vertices
			// all degree 2) crossing or touching itself away from those vertices.
			// A crossing between two separate boundaries, or within a branched
			// wire (degree > 2 — a legitimate subdivision), is not self-
			// intersection. Judged on the pruned core, so a bowtie with a spur
			// still registers once the spur is pruned away.
			si0, sj0 := si.src, sj.src
			if a.core[si0] && a.core[sj0] {
				ci, cj := a.comp[si0], a.comp[sj0]
				if _, ns := a.notSimple[ci]; ci == cj && !ns {
					a.selfXc[ci] = struct{}{}
					a.selfX = append(a.selfX, [2]float64{p.x, p.y})
				}
			}
		}
	}
}

// analyticPrepass classifies every supported (line/circle/arc) source pair with
// the analytic event kernel and applies the result authoritatively: a transverse
// crossing forces an exact cut on each source; a coincident overlap or an
// unresolvable (ambiguous) classification flags degeneracy; a clean tangency is a
// non-splitting contact that does NOT flag degeneracy — UNLESS it would merge into
// a shared vertex between two cycle-bearing sources (where buildGraph's chord-angle
// sort could branch-swap), which is conservatively flagged degenerate pending the
// exact tangent-port handling of a later increment. Handled pairs are recorded so
// the sampled segment loop skips them.
func (a *arranger) analyticPrepass() {
	a.handled = make(map[[2]int]struct{})
	a.sourceSegs = make([][]int, len(a.sources))
	for i := range a.segs {
		a.sourceSegs[a.segs[i].src] = append(a.sourceSegs[a.segs[i].src], i)
	}
	for i := 0; i < len(a.sources); i++ {
		si := &a.sources[i]
		if !analyticKind(si.kind) {
			continue
		}
		for j := i + 1; j < len(a.sources); j++ {
			sj := &a.sources[j]
			if !analyticKind(sj.kind) {
				continue
			}
			events, ambiguous, ok := analyticEvents(si, sj, a.scale)
			if !ok {
				continue
			}
			nCross := 0
			for _, e := range events {
				if e.kind == evCross {
					nCross++
				}
			}
			// Curve/curve TRANSVERSE crossings (both sources circle/arc) are deferred
			// to the sampled path. The sampled DCEL already resolves their topology
			// correctly (the pre-analytic behaviour); injecting exact cuts buys exact
			// area but, until increment 3's exact tangent-port certificate, can only be
			// admitted by a gate that is either unsound (round-2: equal-count coarse
			// crossings at the wrong locations fuse three regions into one) or so
			// conservative it false-flags well-separated valid crossings (a sampled
			// crossing one chord segment off the analytic param). Both are worse than
			// deferring, so do not take analytic authority here: skip marking handled,
			// flag a genuinely ambiguous verdict, and let the sampled loop run. Line-
			// involved crossings and all tangencies keep analytic authority below.
			if nCross > 0 && isCurvedKind(si.kind) && isCurvedKind(sj.kind) {
				if ambiguous {
					rx, ry := sourceRep(si)
					sx, sy := sourceRep(sj)
					a.flagDegenerate((rx+sx)/2, (ry+sy)/2)
				}
				continue
			}
			a.handled[[2]int{i, j}] = struct{}{}
			// Consistency gate (curved pairs only): the sampled polyline must host
			// the analytic crossings faithfully, or injecting exact cuts would warp
			// the planar map (a vanished disk, a tangled face) while reading clean.
			// Two conditions: (1) the same NUMBER of transverse crossings — a coarse
			// chord that does not reach the true crossing shows too few; (2) each
			// analytic crossing WITNESSED on its own host segment-pair. (The
			// over-conservative branch of incidence only bites curve/curve pairs, which
			// are deferred above; a line-involved curved pair has the exact line as one
			// operand, so its sampled crossing tracks the analytic one.) Failing either,
			// conservatively flag degeneracy. Pure line/line pairs are exact (sample ==
			// geometry), so a clean shallow crossing is never false-flagged.
			if isCurvedKind(si.kind) || isCurvedKind(sj.kind) {
				if a.sampledCrossCount(i, j) != nCross || !a.analyticCrossHosted(i, j, events) {
					rx, ry := sourceRep(si)
					sx, sy := sourceRep(sj)
					a.flagDegenerate((rx+sx)/2, (ry+sy)/2)
				}
			}
			if ambiguous {
				rx, ry := sourceRep(si)
				sx, sy := sourceRep(sj)
				a.flagDegenerate((rx+sx)/2, (ry+sy)/2)
			}
			for _, e := range events {
				switch e.kind {
				case evOverlap:
					a.flagDegenerate(e.x, e.y)
				case evCross:
					a.applyAnalyticCut(i, e.ti, e.x, e.y)
					a.applyAnalyticCut(j, e.tj, e.x, e.y)
					// Two sources meeting only at their endpoints is a normal join /
					// corner, not a self-crossing. Replicate the sampled path, which
					// skips endpoint-endpoint contacts: self-intersection needs at least
					// one interior contact.
					if !atSourceEnd(si, e.ti) || !atSourceEnd(sj, e.tj) {
						a.analyticSelfX(i, j, e.x, e.y)
					}
				case evTangent:
					// A tangency at a SHARED ENDPOINT of both sources is a smooth (G1)
					// join — a slot flank meeting its end cap, a fillet — and is always
					// valid; no cut, no degeneracy.
					if atSourceEnd(si, e.ti) && atSourceEnd(sj, e.tj) {
						break
					}
					// An interior clean contact is no cut, no degeneracy — UNLESS it
					// would canonicalize as a shared vertex between two cycle-bearing
					// sources, where the rotation system must order coincident-tangent
					// ports. buildGraph's exact tangent-port ordering now certifies an
					// EXTERNAL circle/arc tangency there (the two loops separate by
					// opposite curvature sign); internal/containment and line-involved
					// tangencies stay conservatively degenerate pending later increments.
					if a.core[i] && a.core[j] &&
						a.sourceHasVertexNear(i, e.x, e.y) && a.sourceHasVertexNear(j, e.x, e.y) {
						if a.externalCurvedTangency(i, j) {
							// Certify this contact for exact tangent-port ordering at the
							// shared vertex; buildGraph orders its coincident-tangent ports
							// by curvature so the two loops separate.
							a.exactPortVerts = append(a.exactPortVerts, [2]float64{e.x, e.y})
						} else {
							a.flagDegenerate(e.x, e.y)
						}
					}
				}
			}
		}
	}
}

func pairKey(i, j int) [2]int {
	if i < j {
		return [2]int{i, j}
	}
	return [2]int{j, i}
}

// isCurvedKind reports whether an analytic source kind is a curve sampled by
// chords (circle/arc) rather than reproduced exactly (a line). Only curved
// sources can have the sampled polyline disagree with the exact geometry.
func isCurvedKind(k srcKind) bool {
	return k == srcCircle || k == srcArc
}

// sampledCrossCount counts the transverse crossings between two sources'
// sampled polylines: a hit strictly interior to BOTH segments, where the two
// polylines genuinely cross from one side to the other. Requiring both-interior
// (not merely one) excludes a tangential touch at a shared sample vertex — a
// line grazing a circle exactly at a polygon vertex is interior to the line but
// sits at the circle's vertex, a contact the analytic kernel correctly reports
// as a tangency (zero crossings), not a transverse crossing.
func (a *arranger) sampledCrossCount(i, j int) int {
	cnt := 0
	for _, ii := range a.sourceSegs[i] {
		for _, jj := range a.sourceSegs[j] {
			if a.segsCrossInterior(ii, jj) {
				cnt++
			}
		}
	}
	return cnt
}

// segsCrossInterior reports whether two tiny segments cross at a point strictly
// interior to both — the transverse-crossing predicate sampledCrossCount uses.
func (a *arranger) segsCrossInterior(ii, jj int) bool {
	p, ok := segParams(&a.segs[ii], &a.segs[jj])
	if !ok {
		return false
	}
	return p.ti > segEps && p.ti < 1-segEps && p.tj > segEps && p.tj < 1-segEps
}

// segContaining returns the index of the source's tiny segment whose natural
// parameter range contains t, or -1 if none. Segments partition [0,1] in source
// parameter; a closed circle's seam sits at a sample vertex (param 0≡1), so no
// segment wraps it.
func (a *arranger) segContaining(src int, t float64) int {
	for _, si := range a.sourceSegs[src] {
		s := &a.segs[si]
		lo, hi := s.pa, s.pb
		if hi < lo {
			lo, hi = hi, lo
		}
		if t >= lo-segEps && t <= hi+segEps {
			return si
		}
	}
	return -1
}

// analyticCrossHosted reports whether every analytic transverse crossing of the
// pair has a SAMPLED transverse crossing on the very segments that carry its
// source parameters. Equal crossing counts are not enough: at coarse sampling two
// circles can both show two polygon crossings that sit nowhere near the exact
// intersections (the chords cross where the circles do not). Requiring each exact
// crossing to be witnessed on its own host segment-pair certifies the sampled map
// has the SAME crossing incidence the analytic kernel found — the precondition
// buildGraph needs to resolve faces correctly. Any crossing whose host segments do
// not themselves cross means the sampling is too coarse to host it.
func (a *arranger) analyticCrossHosted(i, j int, events []xEvent) bool {
	for _, e := range events {
		if e.kind != evCross {
			continue
		}
		si := a.segContaining(i, e.ti)
		sj := a.segContaining(j, e.tj)
		if si < 0 || sj < 0 || !a.segsCrossInterior(si, sj) {
			return false
		}
	}
	return true
}

// applyAnalyticCut records an exact cut at source-parameter t (event point x,y) on
// the tiny segment of source src that contains t. A cut at a segment boundary or a
// source endpoint reuses the existing vertex (no new record) but still marks the
// source topologically split.
func (a *arranger) applyAnalyticCut(src int, t, x, y float64) {
	if atSourceEnd(&a.sources[src], t) {
		return // a contact at the source's own endpoint does not split it (a join)
	}
	a.srcCut[src] = true
	for _, si := range a.sourceSegs[src] {
		s := &a.segs[si]
		lo, hi := s.pa, s.pb
		if hi < lo {
			lo, hi = hi, lo
		}
		if t < lo-segEps || t > hi+segEps {
			continue
		}
		local := (t - s.pa) / (s.pb - s.pa)
		if local <= segEps || local >= 1-segEps {
			return // interior split, but at an existing sample vertex
		}
		s.cuts = append(s.cuts, cut{t: local, px: x, py: y})
		return
	}
}

// atSourceEnd reports whether a natural source parameter is at a curve endpoint
// (t≈0 or t≈1). A full circle is closed — its seam (t≈0/1) is a topologically
// interior point, not an endpoint — so a crossing there still splits it.
func atSourceEnd(s *source, t float64) bool {
	if s.kind == srcCircle {
		return false
	}
	return t <= sourceEndEps || t >= 1-sourceEndEps
}

const sourceEndEps = 1e-7

// analyticSelfX replicates the sampled self-intersection rule for an analytic
// crossing between two different sources: a crossing within one simple cycle-
// bearing component (not a branched/subdivided wire) is a self-touch.
func (a *arranger) analyticSelfX(i, j int, x, y float64) {
	if !a.core[i] || !a.core[j] {
		return
	}
	ci, cj := a.comp[i], a.comp[j]
	if ci != cj {
		return
	}
	if _, ns := a.notSimple[ci]; ns {
		return
	}
	a.selfXc[ci] = struct{}{}
	a.selfX = append(a.selfX, [2]float64{x, y})
}

// sourceHasVertexNear reports whether source src has a sampled vertex within the
// merge tolerance of (x,y) — i.e. a contact there would canonicalize onto an
// existing vertex of that source.
func (a *arranger) sourceHasVertexNear(src int, x, y float64) bool {
	for _, si := range a.sourceSegs[src] {
		s := &a.segs[si]
		if math.Hypot(s.ax-x, s.ay-y) <= a.merge || math.Hypot(s.bx-x, s.by-y) <= a.merge {
			return true
		}
	}
	return false
}

// sourceRep returns a representative interior point of a source, used only to
// locate a degeneracy flag when the analytic classification is ambiguous.
func sourceRep(s *source) (float64, float64) {
	if s.kind == srcLine {
		return (s.ax + s.bx) / 2, (s.ay + s.by) / 2
	}
	return s.cx, s.cy
}

const segEps = 1e-9

type segHit struct {
	x, y   float64
	ti, tj float64
	sin    float64 // |sin| of the crossing angle (0 = parallel/tangent)
}

// segParams intersects two tiny segments, returning the hit with each segment's
// local parameter and the crossing angle's sine. Endpoints count.
func segParams(s, t *tinySeg) (segHit, bool) {
	x1, y1 := s.ax, s.ay
	d1x, d1y := s.bx-x1, s.by-y1
	x2, y2 := t.ax, t.ay
	d2x, d2y := t.bx-x2, t.by-y2
	den := d1x*d2y - d1y*d2x
	mag := math.Hypot(d1x, d1y) * math.Hypot(d2x, d2y)
	if math.Abs(den) <= 1e-12*mag {
		return segHit{}, false
	}
	ti := ((x2-x1)*d2y - (y2-y1)*d2x) / den
	tj := ((x2-x1)*d1y - (y2-y1)*d1x) / den
	if ti < -segEps || ti > 1+segEps || tj < -segEps || tj > 1+segEps {
		return segHit{}, false
	}
	return segHit{x: x1 + ti*d1x, y: y1 + ti*d1y, ti: ti, tj: tj, sin: math.Abs(den) / mag}, true
}

// collinearOverlap reports whether two segments are collinear and overlap along
// more than a single point (coincident/duplicated edges), returning a
// representative point of the overlap.
func collinearOverlap(s, t *tinySeg) (float64, float64, bool) {
	d1x, d1y := s.bx-s.ax, s.by-s.ay
	d2x, d2y := t.bx-t.ax, t.by-t.ay
	len1 := math.Hypot(d1x, d1y)
	len2 := math.Hypot(d2x, d2y)
	if len1 == 0 || len2 == 0 {
		return 0, 0, false
	}
	if math.Abs(d1x*d2y-d1y*d2x) > 1e-9*len1*len2 {
		return 0, 0, false // not parallel
	}
	// t.Start must lie on s's infinite line (collinear).
	if math.Abs((t.ax-s.ax)*d1y-(t.ay-s.ay)*d1x) > 1e-7*len1*math.Max(len1, len2) {
		return 0, 0, false
	}
	// Project both of t's endpoints onto s, as fractions of len1².
	dd := d1x*d1x + d1y*d1y
	pa := ((t.ax-s.ax)*d1x + (t.ay-s.ay)*d1y) / dd
	pb := ((t.bx-s.ax)*d1x + (t.by-s.ay)*d1y) / dd
	lo, hi := math.Min(pa, pb), math.Max(pa, pb)
	ov0, ov1 := math.Max(0, lo), math.Min(1, hi)
	if ov1-ov0 <= 1e-9 {
		return 0, 0, false // touch at a point or disjoint
	}
	m := (ov0 + ov1) / 2
	return s.ax + m*d1x, s.ay + m*d1y, true
}

// split cuts each tiny segment at its crossing parameters and emits the final
// arrangement edges between canonical vertices.
func (a *arranger) split() {
	for i := range a.segs {
		s := &a.segs[i]
		// Boundaries along the segment: the two endpoints (chord positions) plus
		// every cut, each carrying the EXACT point to canonicalize the vertex at.
		bs := []cut{{t: 0, px: s.ax, py: s.ay}, {t: 1, px: s.bx, py: s.by}}
		bs = append(bs, s.cuts...)
		sort.Slice(bs, func(i, j int) bool { return bs[i].t < bs[j].t })
		// dedup near-equal local params (keep the first, which for an analytic cut at
		// a seg boundary keeps the endpoint's exact point)
		uniq := bs[:0:0]
		for _, b := range bs {
			if len(uniq) == 0 || b.t-uniq[len(uniq)-1].t > segEps {
				uniq = append(uniq, b)
			}
		}
		for k := 1; k < len(uniq); k++ {
			b0, b1 := uniq[k-1], uniq[k]
			u := a.verts.canon(b0.px, b0.py)
			v := a.verts.canon(b1.px, b1.py)
			if u == v {
				continue // collapsed to a point
			}
			p0 := s.pa + b0.t*(s.pb-s.pa)
			p1 := s.pa + b1.t*(s.pb-s.pa)
			a.edges = append(a.edges, arrEdge{u: u, v: v, src: s.src, pu: p0, pv: p1})
		}
	}
}

// prune iteratively drops arrangement edges that have a degree-1 endpoint, so
// dangling spurs and open trees (which bound no region) never enter a face
// boundary. Only edges that lie on a cycle survive.
func (a *arranger) prune() {
	for {
		deg := map[int]int{}
		for _, e := range a.edges {
			deg[e.u]++
			deg[e.v]++
		}
		kept := a.edges[:0:0]
		removed := false
		for _, e := range a.edges {
			if deg[e.u] <= 1 || deg[e.v] <= 1 {
				removed = true
				continue
			}
			kept = append(kept, e)
		}
		a.edges = kept
		if !removed {
			break
		}
	}
}

// halfEdge is a directed traversal of an arrangement edge.
type halfEdge struct {
	from, to int
	edge     int // index into arranger.edges
	forward  bool
	angle    float64 // chord departure angle (the sampled fallback ordering key)
	tx, ty   float64 // exact source-tangent departure direction (when exact)
	kappa    float64 // exact signed curvature in the departure direction (when exact)
	exact    bool    // tx/ty/kappa are valid (a line/circle/arc fragment)
	next     int     // index into arranger.halfs
	visited  bool
}

// portKey returns the exact outgoing tangent direction and signed curvature of a
// source fragment leaving a vertex: at natural parameter t, traversed in the
// direction dir (+1 along increasing param, −1 along decreasing). Reversing
// traversal negates both the tangent direction and the signed curvature.
// ok=false for a sampled-only source (ellipse/spline) or a zero-velocity point.
func (a *arranger) portKey(src int, t, dir float64) (tx, ty, kappa float64, ok bool) {
	d1, d2, ok := a.sources[src].differential(t)
	if !ok {
		return 0, 0, 0, false
	}
	n1 := math.Hypot(d1[0], d1[1])
	if n1 == 0 {
		return 0, 0, 0, false
	}
	kappa = dir * (d1[0]*d2[1] - d1[1]*d2[0]) / (n1 * n1 * n1)
	return dir * d1[0], dir * d1[1], kappa, true
}

// dirHalf splits direction (x,y) into the upper half-plane (0) or lower (1) so
// directions can be CCW-ordered by (half, cross) without an atan2 seam. The +x
// axis is upper, −x is lower — a consistent tie-break on the boundary.
func dirHalf(x, y float64) int {
	if y > 0 || (y == 0 && x >= 0) {
		return 0
	}
	return 1
}

const (
	dirParallelEps  = 1e-9
	kappaCertifyEps = 1e-7
)

// sortExactPorts orders the outgoing half-edges of a certified tangency vertex CCW
// by exact tangent, breaking a shared tangent (a tangency) by signed curvature. To
// keep the comparator a valid strict-weak ordering (an ε-band direction compare is
// intransitive for a chain of near-parallel directions), ports are first CLUSTERED
// into same-ray groups, every member of a group is stamped with ONE shared group
// angle, and the sort then uses only EXACT keys: lexicographic (groupAngle, kappa,
// index). ε appears only in the clustering and the osculation flag, never in an
// ordering decision. A genuine osculation (two same-ray ports with indistinguishable
// scaled curvature) is flagged degenerate.
func (a *arranger) sortExactPorts(v int, ring []int) {
	n := len(ring)
	groupAng := make([]float64, n)
	stamped := make([]bool, n)
	for i := 0; i < n; i++ {
		if stamped[i] {
			continue
		}
		hi := &a.halfs[ring[i]]
		ang := math.Atan2(hi.ty, hi.tx)
		groupAng[i] = ang
		stamped[i] = true
		ni := math.Hypot(hi.tx, hi.ty)
		for j := i + 1; j < n; j++ {
			if stamped[j] {
				continue
			}
			hj := &a.halfs[ring[j]]
			dot := hi.tx*hj.tx + hi.ty*hj.ty
			cr := hi.tx*hj.ty - hi.ty*hj.tx
			if dot > 0 && math.Abs(cr) <= dirParallelEps*ni*math.Hypot(hj.tx, hj.ty) {
				groupAng[j] = ang // same ray → one shared angle for the whole group
				stamped[j] = true
			}
		}
	}
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(x, y int) bool {
		px, py := order[x], order[y]
		if groupAng[px] != groupAng[py] {
			return groupAng[px] < groupAng[py]
		}
		kx, ky := a.halfs[ring[px]].kappa, a.halfs[ring[py]].kappa
		if kx != ky {
			return kx < ky // exact curvature order (a curve bending further CCW comes later)
		}
		return px < py
	})
	sorted := make([]int, n)
	for i, oi := range order {
		sorted[i] = ring[oi]
	}
	// Detect osculation on the reordered ring. A genuine osculation has EXACTLY
	// parallel tangents (cross ≈ 0), so its two ports cluster together and land
	// adjacent after the angle sort; test adjacent pairs by actual tangent direction
	// and flag when the scaled curvatures are indistinguishable. (A pair more than the
	// parallel epsilon apart is not the same ray, hence not an osculation.)
	for i := 0; i < n; i++ {
		hi := &a.halfs[sorted[i]]
		hj := &a.halfs[sorted[(i+1)%n]]
		dot := hi.tx*hj.tx + hi.ty*hj.ty
		cr := hi.tx*hj.ty - hi.ty*hj.tx
		if dot <= 0 || math.Abs(cr) > dirParallelEps*math.Hypot(hi.tx, hi.ty)*math.Hypot(hj.tx, hj.ty) {
			continue // not the same tangent ray
		}
		if math.Abs(hi.kappa-hj.kappa)*a.scale <= kappaCertifyEps {
			vx, vy := a.verts.coord(v)
			a.flagDegenerate(vx, vy)
			break
		}
	}
	copy(ring, sorted)
}

// useExactPorts reports whether vertex v should be ordered by exact tangent ports
// rather than chord direction: only at a certified analytic tangency contact (where
// chord directions tie and would branch-swap) AND only if every incident half-edge
// is an exact line/circle/arc fragment. Everywhere else — sampled crossings, polygon
// corners, ellipse/spline vertices — chord ordering matches the polyline geometry
// the face walk actually traverses, so exact tangents must NOT be used there.
func (a *arranger) useExactPorts(v int, ring []int) bool {
	vx, vy := a.verts.coord(v)
	certified := false
	for _, p := range a.exactPortVerts {
		if math.Hypot(p[0]-vx, p[1]-vy) <= a.merge {
			certified = true
			break
		}
	}
	if !certified {
		return false
	}
	for _, hi := range ring {
		if !a.halfs[hi].exact {
			return false
		}
	}
	return true
}

// externalCurvedTangency reports whether sources i and j are two circle/arc
// carriers in EXTERNAL tangency (centre distance beyond the larger radius). Their
// loops separate cleanly under exact tangent-port ordering (opposite curvature
// sign at the contact), so the merged shared vertex no longer needs a conservative
// degeneracy flag. Internal tangency (containment) still does — its hole
// assignment is not yet certified — so it is excluded here.
func (a *arranger) externalCurvedTangency(i, j int) bool {
	si, sj := &a.sources[i], &a.sources[j]
	if !isCurvedKind(si.kind) || !isCurvedKind(sj.kind) {
		return false
	}
	d := math.Hypot(si.cx-sj.cx, si.cy-sj.cy)
	return d > math.Max(si.r, sj.r)
}

// buildGraph wires the doubly-connected edge list: two half-edges per edge, the
// rotation system at each vertex, and the next pointers (face on the left).
func (a *arranger) buildGraph() {
	a.halfs = make([]halfEdge, 0, len(a.edges)*2)
	for ei, e := range a.edges {
		ux, uy := a.verts.coord(e.u)
		vx, vy := a.verts.coord(e.v)
		// Forward leaves e.u at param e.pu (increasing); backward leaves e.v at
		// param e.pv (decreasing). The exact tangent at the departure point orders
		// the rotation system correctly even where chord directions tie (a tangency).
		ftx, fty, fka, fok := a.portKey(e.src, e.pu, +1)
		btx, bty, bka, bok := a.portKey(e.src, e.pv, -1)
		a.halfs = append(a.halfs, halfEdge{from: e.u, to: e.v, edge: ei, forward: true, angle: math.Atan2(vy-uy, vx-ux), tx: ftx, ty: fty, kappa: fka, exact: fok, next: -1})
		a.halfs = append(a.halfs, halfEdge{from: e.v, to: e.u, edge: ei, forward: false, angle: math.Atan2(uy-vy, ux-vx), tx: btx, ty: bty, kappa: bka, exact: bok, next: -1})
	}
	// Outgoing half-edges per vertex, sorted CCW by departure direction. When every
	// incident half-edge is an exact (line/circle/arc) fragment, order by the exact
	// source tangent and, for ports sharing a tangent (a tangency), by signed
	// curvature — this is what stops a shared tangent vertex from branch-swapping.
	// A vertex with any sampled (ellipse/spline) fragment keeps the chord order.
	out := map[int][]int{}
	for hi := range a.halfs {
		out[a.halfs[hi].from] = append(out[a.halfs[hi].from], hi)
	}
	for v := range out {
		list := out[v]
		if a.useExactPorts(v, list) {
			a.sortExactPorts(v, list)
		} else {
			sort.Slice(list, func(i, j int) bool { return a.halfs[list[i]].angle < a.halfs[list[j]].angle })
		}
		out[v] = list
	}
	pos := map[int]int{} // half-edge -> index within its origin's sorted ring
	for v := range out {
		for idx, hi := range out[v] {
			pos[hi] = idx
		}
	}
	twin := func(hi int) int {
		if hi%2 == 0 {
			return hi + 1
		}
		return hi - 1
	}
	// next(e): at the head of e, take the outgoing edge immediately clockwise
	// from e's twin, so the face stays on the left and bounded faces wind CCW.
	for hi := range a.halfs {
		w := a.halfs[hi].to
		t := twin(hi)
		ring := out[w]
		k := pos[t]
		a.halfs[hi].next = ring[(k-1+len(ring))%len(ring)]
	}
}

// extract walks the next cycles, classifies them into faces and holes, builds
// the regions, and returns the arrangement.
func (a *arranger) extract() *Arrangement {
	var cycles []cycle
	for hi := range a.halfs {
		if a.halfs[hi].visited {
			continue
		}
		var hs []int
		for cur := hi; !a.halfs[cur].visited; cur = a.halfs[cur].next {
			a.halfs[cur].visited = true
			hs = append(hs, cur)
		}
		cycles = append(cycles, a.makeCycle(hs))
	}

	epsArea := a.scale * a.scale * 1e-12
	var faces []*cycle
	var holes []*cycle
	for i := range cycles {
		c := &cycles[i]
		switch {
		case c.area > epsArea:
			faces = append(faces, c)
		case c.area < -epsArea:
			holes = append(holes, c)
		}
	}

	arr := &Arrangement{SelfIntersections: a.selfX, Degenerate: a.degenSet, Degeneracies: a.degen}
	// Assign each hole to the smallest-area face that strictly contains it. The
	// containment probe is a point guaranteed interior to the hole (not a
	// boundary vertex), so a hole touching a face boundary still resolves.
	holeOf := make([][]*cycle, len(faces))
	for _, h := range holes {
		probe := interiorPoint(h.dense)
		best := -1
		for fi, f := range faces {
			if f.area <= -h.area+epsArea {
				continue // not strictly larger than the hole (excludes the twin)
			}
			if !pointInPolygon(probe, f.dense) {
				continue
			}
			if best < 0 || faces[best].area > f.area {
				best = fi
			}
		}
		if best >= 0 {
			holeOf[best] = append(holeOf[best], h)
		}
	}

	for fi, f := range faces {
		reg := &Region{Outer: f.boundary, Area: f.area, SelfIntersecting: f.selfX}
		for _, h := range holeOf[fi] {
			reg.Holes = append(reg.Holes, h.boundary)
			reg.Area -= -h.area // h.area is negative
			if h.selfX {
				reg.SelfIntersecting = true
			}
		}
		arr.Regions = append(arr.Regions, reg)
	}
	return arr
}

// cycle is one next-cycle: its coalesced boundary edges, dense polygon, signed
// area, and whether any contributing source self-intersects.
type cycle struct {
	boundary []BoundaryEdge
	dense    [][2]float64
	area     float64
	selfX    bool
}

// makeCycle coalesces a run of half-edges into BoundaryEdges, builds the dense
// polygon, and computes the exact signed area.
func (a *arranger) makeCycle(hs []int) cycle {
	var c cycle
	// Coalesce consecutive half-edges that share a source into one BoundaryEdge.
	type frag struct {
		src      int
		pStart   float64
		pEnd     float64
		dense    [][2]float64
		reversed bool
	}
	var frags []frag
	for _, hi := range hs {
		h := a.halfs[hi]
		e := a.edges[h.edge]
		var pStart, pEnd float64
		if h.forward {
			pStart, pEnd = e.pu, e.pv
		} else {
			pStart, pEnd = e.pv, e.pu
		}
		fx, fy := a.verts.coord(h.from)
		tx, ty := a.verts.coord(h.to)
		if n := len(frags); n > 0 && frags[n-1].src == e.src && approx(frags[n-1].pEnd, pStart, 1e-9) {
			frags[n-1].pEnd = pEnd
			frags[n-1].dense = append(frags[n-1].dense, [2]float64{tx, ty})
		} else {
			frags = append(frags, frag{src: e.src, pStart: pStart, pEnd: pEnd,
				dense: [][2]float64{{fx, fy}, {tx, ty}}})
		}
		if cm := a.comp[e.src]; cm >= 0 {
			if _, ok := a.selfXc[cm]; ok {
				c.selfX = true
			}
		}
	}
	// A closed loop's first and last fragment may share a source; merge them.
	if n := len(frags); n > 1 && frags[0].src == frags[n-1].src && approx(frags[n-1].pEnd, frags[0].pStart, 1e-9) {
		frags[n-1].pEnd = frags[0].pEnd
		frags[n-1].dense = append(frags[n-1].dense, frags[0].dense[1:]...)
		frags = frags[1:]
	}

	chord := make([][2]float64, 0, len(frags))
	var bulge float64
	for _, f := range frags {
		s := &a.sources[f.src]
		reversed := f.pEnd < f.pStart
		// Whole means the source curve was never split by a crossing — so this
		// edge represents the entire curve (a closed curve's seam is not a split).
		whole := !a.srcCut[f.src]
		c.boundary = append(c.boundary, BoundaryEdge{
			SourceIndex: f.src, Whole: whole, Reversed: reversed, Polyline: f.dense,
		})
		c.dense = append(c.dense, f.dense[:len(f.dense)-1]...)
		chord = append(chord, f.dense[0])
		// Area between this fragment's true curve and its chord. Every curved
		// source contributes an exact, sampling-independent correction: arc/circle
		// and ellipse/elliptical-arc via the closed-form circular/elliptical
		// segment; a spline via the exact ½∫(x·y′−y·x′) integral of its piecewise
		// cubic ([splineBulge], 3-point Gauss–Legendre per knot span). A line is
		// its own chord (zero bulge). The eccentric-angle span of a circular/
		// elliptical fragment is its natural-param fraction times the source sweep.
		switch s.kind {
		case srcArc:
			bulge += chordArcCorrection(s.r, (f.pEnd-f.pStart)*s.sweep)
		case srcCircle:
			bulge += chordArcCorrection(s.r, (f.pEnd-f.pStart)*2*math.Pi)
		case srcEllipse:
			bulge += chordEllipseCorrection(s.rx, s.ry, (f.pEnd-f.pStart)*2*math.Pi)
		case srcEllipticalArc:
			bulge += chordEllipseCorrection(s.rx, s.ry, (f.pEnd-f.pStart)*s.sweep)
		case srcSpline, srcClosedSpline, srcFitSpline:
			a0 := f.dense[0]
			a1 := f.dense[len(f.dense)-1]
			bulge += s.splineBulge(f.pStart, f.pEnd, a0[0], a0[1], a1[0], a1[1])
		}
	}
	c.area = signedPolyArea(chord) + bulge
	return c
}

// chordArcCorrection returns the signed area between an arc's chord and the arc,
// for a fragment of signed subtended angle theta on a circle of radius r. The
// sign follows the walk: a CCW fragment (theta>0) bulges to the left of its
// directed chord and adds positive area.
func chordArcCorrection(r, theta float64) float64 {
	return 0.5 * r * r * (theta - math.Sin(theta))
}

// chordEllipseCorrection returns the exact signed area between an elliptical
// arc's chord and the arc, for a fragment spanning eccentric angle dphi on an
// ellipse with semi-axes rx, ry. In the ellipse's local frame the arc is
// (rx·cosφ, ry·sinφ): the radius sweeps sector area ½·rx·ry·dphi and the chord
// cuts off triangle ½·rx·ry·sin(dphi), so the segment is ½·rx·ry·(dphi −
// sin(dphi)) — the elliptical analog of [chordArcCorrection] (r² → rx·ry). It is
// independent of the ellipse's centre and rotation (area is invariant under
// translation and rotation), so it is exact, not sampled. The sign follows the
// walk via the signed dphi, exactly like the circular case.
func chordEllipseCorrection(rx, ry, dphi float64) float64 {
	return 0.5 * rx * ry * (dphi - math.Sin(dphi))
}

// splineBulge returns the exact signed area between a spline fragment's true
// curve (natural parameters pStart→pEnd, in walk order) and the straight chord
// that closes it — the spline analog of [chordArcCorrection]/
// [chordEllipseCorrection]. (ax,ay) and (ex,ey) are the fragment's chord
// endpoints (the dense polyline's first and last vertex); the chord-closure term
// ½·(ex·ay − ax·ey) matches the implied closing edge of [signedPolyArea], so this
// reproduces signedPolyArea's decomposition with the sampled curve moment
// replaced by the exact integral. The walk direction (and thus the sign) is
// carried by the order of pStart,pEnd, exactly like the arc/ellipse cases.
func (s *source) splineBulge(pStart, pEnd, ax, ay, ex, ey float64) float64 {
	return s.curveMoment(pStart, pEnd) + 0.5*(ex*ay-ax*ey)
}

// curveMoment returns the exact ½∫(x·y′ − y·x′) dt of a spline source over the
// natural-parameter interval pStart→pEnd (signed by direction). A cubic spline is
// piecewise cubic, so the integrand is a degree-5 polynomial on each knot span
// and 3-point Gauss–Legendre integrates it exactly; the interval is split at
// every interior breakpoint so no panel straddles a span boundary (where the
// piecewise polynomial changes and the quadrature would no longer be exact).
func (s *source) curveMoment(pStart, pEnd float64) float64 {
	lo, hi, sign := pStart, pEnd, 1.0
	if lo > hi {
		lo, hi, sign = hi, lo, -1.0
	}
	// Every interior knot strictly inside (lo,hi) must become a panel boundary, or
	// a panel would straddle a span boundary (where the piecewise polynomial
	// changes) and the per-span Gauss–Legendre would no longer be exact. Use a
	// strict open-interval test: a breakpoint coinciding with lo/hi is the boundary
	// already, and an extra split that produces a tiny panel is harmless (the
	// integrand is still a single polynomial there), but dropping a real knot is
	// not. splineBreaks returns the knots in ascending order.
	bounds := []float64{lo}
	for _, b := range s.splineBreaks() {
		if b > lo && b < hi {
			bounds = append(bounds, b)
		}
	}
	bounds = append(bounds, hi)
	var moment float64
	for i := 0; i+1 < len(bounds); i++ {
		moment += s.gaussMoment(bounds[i], bounds[i+1])
	}
	return sign * moment
}

// gauss3 holds the 3-point Gauss–Legendre nodes/weights on [-1,1]; exact for
// polynomials up to degree 5 (the degree of a cubic spline's area integrand).
var gauss3 = struct {
	nodes, weights [3]float64
}{
	nodes:   [3]float64{-0.7745966692414834, 0, 0.7745966692414834}, // ±√(3/5), 0
	weights: [3]float64{5.0 / 9, 8.0 / 9, 5.0 / 9},
}

// gaussMoment integrates ½(x·y′ − y·x′) over a single panel [t0,t1] that lies
// within one polynomial span, by 3-point Gauss–Legendre (exact there).
func (s *source) gaussMoment(t0, t1 float64) float64 {
	half := 0.5 * (t1 - t0)
	mid := 0.5 * (t0 + t1)
	var sum float64
	for k := 0; k < 3; k++ {
		t := mid + half*gauss3.nodes[k]
		p := s.at(t)
		d := s.derivAt(t)
		sum += gauss3.weights[k] * 0.5 * (p[0]*d[1] - p[1]*d[0])
	}
	return sum * half
}

// splineBreaks returns the source's interior knot parameters in (0,1) — the
// span boundaries [curveMoment] must split on. Only the spline kinds have any.
func (s *source) splineBreaks() []float64 {
	switch s.kind {
	case srcSpline: // clamped uniform cubic B-spline: interior knots j/(n-3)
		n := len(s.ctrl)
		spans := n - 3
		if spans < 2 {
			return nil
		}
		out := make([]float64, 0, spans-1)
		for j := 1; j < spans; j++ {
			out = append(out, float64(j)/float64(spans))
		}
		return out
	case srcClosedSpline: // periodic cubic B-spline: span boundaries i/n
		n := len(s.ctrl)
		out := make([]float64, 0, n-1)
		for i := 1; i < n; i++ {
			out = append(out, float64(i)/float64(n))
		}
		return out
	case srcFitSpline:
		return s.fitEval.interiorBreaks()
	}
	return nil
}

// derivAt returns the source's tangent dS/dt at natural parameter t, for the
// spline kinds (the only ones [gaussMoment] evaluates).
func (s *source) derivAt(t float64) [2]float64 {
	switch s.kind {
	case srcSpline:
		dx, dy := EvalCubicBSplineDeriv(s.ctrl, t)
		return [2]float64{dx, dy}
	case srcClosedSpline:
		dx, dy := EvalPeriodicCubicBSplineDeriv(s.ctrl, t)
		return [2]float64{dx, dy}
	case srcFitSpline:
		return s.fitEval.derivAt(t)
	}
	return [2]float64{}
}

func approx(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

// --- vertex table -----------------------------------------------------------

type vertexTable struct {
	merge float64
	cell  float64
	xs    []float64
	ys    []float64
	grid  map[[2]int][]int
}

func newVertexTable(merge float64) vertexTable {
	return vertexTable{merge: merge, cell: math.Max(merge, 1e-300), grid: map[[2]int][]int{}}
}

// canon returns the id of the vertex at (x,y), merging with an existing vertex
// within the merge tolerance (checking the 3×3 neighborhood of grid cells).
func (t *vertexTable) canon(x, y float64) int {
	cx, cy := int(math.Floor(x/t.cell)), int(math.Floor(y/t.cell))
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			for _, id := range t.grid[[2]int{cx + dx, cy + dy}] {
				if math.Hypot(t.xs[id]-x, t.ys[id]-y) <= t.merge {
					return id
				}
			}
		}
	}
	id := len(t.xs)
	t.xs = append(t.xs, x)
	t.ys = append(t.ys, y)
	t.grid[[2]int{cx, cy}] = append(t.grid[[2]int{cx, cy}], id)
	return id
}

func (t *vertexTable) coord(id int) (float64, float64) { return t.xs[id], t.ys[id] }

// --- union-find -------------------------------------------------------------

type unionFind struct{ parent []int }

func newUnionFind(n int) *unionFind {
	p := make([]int, n)
	for i := range p {
		p[i] = i
	}
	return &unionFind{parent: p}
}

func (u *unionFind) find(i int) int {
	for u.parent[i] != i {
		u.parent[i] = u.parent[u.parent[i]]
		i = u.parent[i]
	}
	return i
}

func (u *unionFind) union(i, j int) { u.parent[u.find(i)] = u.find(j) }
