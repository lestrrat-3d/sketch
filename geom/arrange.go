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
	// conic: the rational-quadratic-Bézier control points and apex weight
	// w = rho/(1−rho). The bulge uses these directly via the closed form.
	conStart, conApex, conEnd [2]float64
	conW                      float64
	// spline: control-point coordinates for Cox–de Boor evaluation.
	ctrl [][2]float64
	// fit-point spline: a prebuilt natural-cubic interpolant (the tridiagonal
	// solve runs once when the source is created, then is reused per sample).
	fitEval *fitEvaluator
	// NURBS: the general rational B-spline (degree/knots/weights/control). The
	// natural parameter t in [0, 1] maps linearly across the knot domain.
	nurbs *NURBS
	// extent is THIS source's own bounding-box extent, measured over its sampled
	// polyline by densify with the same formula the scene scale uses. It is the
	// SOURCE-LOCAL yardstick the identity band is stated in (vertexCertifies,
	// endpointReproduces), so that band is a property of the curve under test and
	// cannot be widened by geometry drawn somewhere else. Zero for a source that
	// samples to a single point (a degenerate one contributes no segments), which
	// admits only an exact match — the same conservative choice polylineChordAt
	// makes for a vertex with no chord.
	extent float64
}

type srcKind int

const (
	srcLine srcKind = iota
	srcArc
	srcCircle
	srcEllipse
	srcEllipticalArc // an ellipse restricted to an eccentric-angle sweep
	srcConic         // a rational quadratic Bézier (open; convex-hull-bounded, cannot self-cross)
	srcSpline        // a clamped cubic B-spline (open; may self-cross)
	srcClosedSpline  // a periodic cubic B-spline (closed loop; may self-cross)
	srcFitSpline     // a natural-cubic interpolating spline (open; may self-cross)
	srcNURBS         // a general clamped rational B-spline (open; may self-cross)
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
	case srcConic:
		return s.conicPoint(t)
	case srcSpline:
		// control-point count is guaranteed valid by the source builder.
		x, y, _ := EvalCubicBSpline(s.ctrl, t)
		return [2]float64{x, y}
	case srcClosedSpline:
		x, y, _ := EvalPeriodicCubicBSpline(s.ctrl, t)
		return [2]float64{x, y}
	case srcFitSpline:
		return s.fitEval.at(t)
	case srcNURBS:
		lo, hi := s.nurbs.domain()
		x, y := s.nurbs.Eval(lo + (hi-lo)*t)
		return [2]float64{x, y}
	default: // ellipse
		return s.ellipsePoint(2 * math.Pi * t)
	}
}

// ellipsePoint evaluates the source's ellipse at eccentric angle ang.
func (s *source) ellipsePoint(ang float64) [2]float64 {
	return ellipsePointAt(s.cx, s.cy, s.rx, s.ry, s.rot, ang)
}

// conicPoint evaluates the source's rational quadratic Bézier at parameter t.
func (s *source) conicPoint(t float64) [2]float64 {
	return conicEvalRaw(s.conStart, s.conApex, s.conEnd, s.conW, t)
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
	// exact is true when this cut came from the closed-form kernel (an analytic
	// crossing), so t is the true source parameter and px,py the true intersection
	// point. It is false for a SAMPLED cut, whose t is interpolated between two
	// sample params and whose point is the chord-chord intersection — off the true
	// curve by the chord sagitta. It propagates to BoundaryEdge.TExact so a consumer
	// can tell a trustworthy parameter range from a sampling-accurate one.
	exact bool
	// srcEnd is the boundary's PROVENANCE: true only for a bound that IS the source
	// curve's own domain end (an open curve's endpoint, or a closed curve's seam),
	// false for a bound a cut or a distance weld put there. It propagates to
	// BoundaryEdge.Whole, which is exactly "both of my bounds are the curve's own
	// ends" — a structural fact known when the boundary is built, never re-derived by
	// comparing a parameter against 0/1 afterwards (no float compare can tell "this
	// bound IS the endpoint" from "this bound is a crossing that landed very near
	// it"). Only split's two synthetic segment-endpoint bounds can carry it; every
	// real cut leaves it false.
	srcEnd bool
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
	degen     []degenRecord    // degenerate (collinear-overlap / unresolvable) conditions
	degenSet  bool

	// Analytic-arrangement state (increment 2): which line/circle/arc source pairs
	// were classified analytically (so the sampled segment loop skips them), the
	// events that classification found (so a distance-weld between the pair's sample
	// vertices can be audited against them), and a per-source segment index for
	// mapping an analytic event's source parameter to the tiny segment it cuts.
	handled    map[[2]int]struct{}
	events     map[[2]int][]xEvent
	sourceSegs [][]int

	// Curve/curve crossings the incidence certificate REFUSED, so the pair fell back
	// to the sampled path (see analyticCrossingsCertified), and the contacts the
	// SAMPLED loop actually recorded between two sources — a crossing it found, or a
	// weld of their sample vertices. The fallback is only sound while the sampled map
	// represents those crossings; where it does not, the map is fused and no fragment
	// of the affected component may report an exact parameter. refuseExactOnFusedMap
	// compares the two and fills exactRefused.
	deferredCross   map[[2]int][]xEvent
	sampledContacts map[[2]int][][2]float64

	// exactRefused withdraws exact authority per SOURCE: split() forces every bound of
	// such a source to exact:false, whatever its own cut records say. Nil when nothing
	// is refused (the common case).
	exactRefused []bool

	// exactAllowed gates EVERY exact bound this arrangement emits, ahead of any
	// per-source or per-pair reasoning: it is true only when EVERY source is a line,
	// circle or arc (analyticKind). A scene holding any free-form source — ellipse,
	// elliptical arc, conic, spline, closed spline, fit spline or NURBS — publishes NO
	// exact bound anywhere, including on the lines, circles and arcs that share the
	// scene with it, and including a free-form curve's own uncut whole edge.
	//
	// A free-form source reaches the planar map only as chords, and nothing bounds how
	// far one of them leaves its chord: the sampler places vertices, it does not certify
	// a deviation, and no per-family enclosure is computed here. A curve with a lobe
	// between two consecutive samples can therefore cross another curve entirely between
	// them (measured on a knot-clustered degree-3 NURBS at the default sampling: a
	// midpoint-sampled deviation of 2.1e-05 against a true 4.7e-01 maximum deviation on
	// the same segment). That crossing is missing from the map, the regions it separates
	// FUSE, and the analytic pairs of the same scene — certified on their own merits and
	// cut exactly — then publish the fused map with every bound exact. Gating on the
	// SOURCE KINDS answers that without a threshold, a per-family deviation bound, or an
	// estimate of any kind; in an all-analytic scene there is no sampled-only pair to
	// reason about at all, and the only remaining reconciliation is the refused-crossing
	// one below, whose bound errs toward withdrawing exactness.
	//
	// What it costs is exactness on the analytic sources sharing a scene with a
	// free-form one. Topology, areas, degeneracy and the reported parameter ranges are
	// untouched, and an all-analytic scene is unaffected. Lifting it needs a sampler
	// that certifies its own deviation per source — a separate change to densify, not a
	// wider estimate here.
	exactAllowed bool

	// Certified analytic tangency contacts (increment 3): the exact points where
	// the rotation system must order coincident-tangent ports by curvature instead
	// of by chord direction. Used ONLY at these vertices — at a sampled crossing the
	// edges are chords, so chord ordering (not exact tangents) matches the geometry
	// the face walk traverses.
	exactPortVerts [][2]float64

	// overlaps holds one record per resolved coincident-carrier overlap (see
	// resolveCoincidentOverlap and docs/coincident-carrier-resolution-design.md), and
	// suppressed indexes them by LOSING source so split() can find a fragment's
	// windows in one lookup. A single source can lose against several different named
	// sources (e.g. one hub circle coincident with every tooth's root arc in a gear),
	// so each source maps to a slice of records.
	overlaps   []coincidentOverlap
	suppressed map[int][]int
}

// coincidentOverlap is one resolved coincident-carrier overlap: the pair, the
// window's two boundary points (where BOTH sources were cut), the angular window
// suppressed on the losing source, and a locator for the degeneracy flag should the
// resolution be withdrawn.
//
// refused is written by certifySuppression, which runs inside split() with the
// emitted fragments in hand: the window is a CLAIM at analytic-pass time, and only
// the fragments split actually produces can settle whether it holds. A refused
// record suppresses nothing, so the losing source emits its coincident span exactly
// as it did before this design, and the pair is flagged degenerate like any other
// out-of-scope overlap.
type coincidentOverlap struct {
	named, losing int
	loX, loY      float64
	hiX, hiY      float64
	// repX/repY is the event's own window MIDPOINT — never a cut site, only where
	// flagDegenerate points when the resolution is withdrawn.
	repX, repY float64
	win        angularWindow
	refused    bool
}

// angularWindow is a suppression range on a source that shares a coincident carrier
// with a lower-indexed source: the portion of the source's own sweep, expressed as
// an absolute angle around the shared carrier's center (so it needs no per-source
// natural-parameter sign/wrap bookkeeping — the two sources sweep independently and
// a closed carrier's parameter range wraps through its seam, while the angle is one
// quantity both agree on; see step 3 of "Resolution" in
// docs/coincident-carrier-resolution-design.md), to omit from split()'s emitted
// edges.
type angularWindow struct {
	cx, cy       float64
	angLo, width float64
}

// contains reports whether the point (x,y) — evaluated on the shared carrier —
// falls inside the window, wrapping by a full turn.
//
// The window is tested EXACTLY, with no outward slop, and the asymmetry is
// load-bearing. A fragment that genuinely lies inside the window is separated from
// either end of it by at least half its own angular extent, because both window
// boundaries are boundaries of the losing source's own emitted fragments, so no
// emitted fragment straddles one — and it therefore needs no margin to be
// recognised. That premise is not predicted, it is CHECKED against the emitted
// fragments themselves: certifySuppression withdraws any window whose two
// boundaries did not survive split's dedup as distinct shared fragment bounds, so a
// window that survives to be tested here has the property this test needs.
// A fragment OUTSIDE the window, by contrast, can sit arbitrarily close to it: the
// losing source's own gap beyond the overlap is a real span of any width, and it is
// the only thing left to close a region when the overlap covers nearly the whole
// carrier. An outward slop of arcParamEps deleted exactly that fragment for every
// gap up to twice it, taking the region with it and leaving nothing flagged.
//
// The point handed in must be the source EVALUATED at the fragment's parameter
// midpoint, never the midpoint of the fragment's chord: a fragment spanning half a
// circle has its chord midpoint AT the carrier centre, where the angle this test
// reads is meaningless (see fragmentSuppressed).
func (w angularWindow) contains(x, y float64) bool {
	ang := math.Atan2(y-w.cy, x-w.cx)
	d := math.Mod(ang-w.angLo, 2*math.Pi)
	if d < 0 {
		d += 2 * math.Pi
	}
	return d <= w.width
}

// arrEdge is an undirected arrangement edge between two canonical vertices,
// carrying its source and the natural param range along that source.
type arrEdge struct {
	u, v   int
	src    int
	pu, pv float64
	// exactU/exactV report whether the param at that end is the true source
	// parameter (an analytic cut, a sample vertex, or the curve's own endpoint)
	// rather than a sampled crossing's interpolated one. See cut.exact — and note
	// split ANDs in vertexCertifies, so a bound whose graph vertex is not where the
	// bound says it is (a distance weld, however it was chained) is never exact.
	exactU, exactV bool
	// endU/endV report whether that end is the source curve's own domain end (or a
	// closed curve's seam) rather than a cut/weld. See cut.srcEnd.
	endU, endV bool
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
		case *Conic:
			if t == nil || t.Start == nil || t.Apex == nil || t.End == nil ||
				!(t.Rho > 0 && t.Rho < 1) {
				a.flagDegenerate(0, 0)
				s.kind = srcDegenerate
				break
			}
			s.kind = srcConic
			s.conStart = [2]float64{t.Start.X, t.Start.Y}
			s.conApex = [2]float64{t.Apex.X, t.Apex.Y}
			s.conEnd = [2]float64{t.End.X, t.End.Y}
			s.conW = t.Rho / (1 - t.Rho)
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
		case *NURBS:
			nb, ok := nurbsSnapshot(t)
			if !ok {
				a.flagDegenerate(0, 0)
				s.kind = srcDegenerate
				break
			}
			if nurbsExtent(nb) < 1e-9 { // all-coincident controls: a point
				a.flagDegenerate(nb.Control[0].X, nb.Control[0].Y)
				s.kind = srcDegenerate
				break
			}
			s.kind = srcNURBS
			s.nurbs = nb
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
	// Decided once, over the whole scene, before anything is sampled or classified:
	// a free-form (or unusable) source anywhere withholds exact bounds everywhere.
	a.exactAllowed = true
	for i := range a.sources {
		if !analyticKind(a.sources[i].kind) {
			a.exactAllowed = false
			break
		}
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
	case *Conic:
		if t == nil || t.Start == nil || t.End == nil {
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
	case *NURBS:
		if !nurbsValid(t) {
			return nil, nil, false
		}
		return t.Control[0], t.Control[len(t.Control)-1], true
	default:
		return nil, nil, false
	}
}

// nurbsValid reports whether a NURBS is structurally well-formed enough for the
// arrangement to evaluate: non-nil, degree >= 1, at least degree+1 control
// points (none nil), and a clamped non-decreasing knot vector of the right
// length. The sketch entity's CreateNURBS validates the same conditions up front, so
// this is a defensive guard against a hand-built or typed-nil snapshot.
func nurbsValid(c *NURBS) bool {
	if c == nil || c.Degree < 1 || len(c.Control) < c.Degree+1 {
		return false
	}
	if len(c.Knots) != len(c.Control)+c.Degree+1 {
		return false
	}
	for _, p := range c.Control {
		if p == nil {
			return false
		}
	}
	for i := 1; i < len(c.Knots); i++ {
		if c.Knots[i] < c.Knots[i-1] {
			return false
		}
	}
	p := c.Degree
	n := len(c.Control) - 1
	for i := 0; i <= p; i++ {
		if c.Knots[i] != c.Knots[0] || c.Knots[len(c.Knots)-1-i] != c.Knots[len(c.Knots)-1] {
			return false
		}
	}
	if c.Knots[p] >= c.Knots[n+1] { // empty domain
		return false
	}
	if c.Weights != nil {
		if len(c.Weights) != len(c.Control) {
			return false
		}
		for _, w := range c.Weights {
			if !(w > 0) {
				return false
			}
		}
	}
	return true
}

// nurbsSnapshot validates a NURBS and returns a defensive copy (control points,
// knots and weights) the arranger can hold without aliasing the caller's slices.
// ok is false for any degenerate/typed-nil input.
func nurbsSnapshot(c *NURBS) (*NURBS, bool) {
	if !nurbsValid(c) {
		return nil, false
	}
	ctrl := make([]*Point, len(c.Control))
	for i, p := range c.Control {
		ctrl[i] = &Point{X: p.X, Y: p.Y}
	}
	knots := append([]float64(nil), c.Knots...)
	var weights []float64
	if c.Weights != nil {
		weights = append([]float64(nil), c.Weights...)
	}
	return &NURBS{Degree: c.Degree, Control: ctrl, Knots: knots, Weights: weights}, true
}

// nurbsExtent returns the largest pairwise control-point separation along either
// axis — used to reject an all-coincident (zero-extent) NURBS that is a point,
// not a curve.
func nurbsExtent(c *NURBS) float64 {
	minX, minY := c.Control[0].X, c.Control[0].Y
	maxX, maxY := minX, minY
	for _, p := range c.Control {
		minX, maxX = math.Min(minX, p.X), math.Max(maxX, p.X)
		minY, maxY = math.Min(minY, p.Y), math.Max(maxY, p.Y)
	}
	return math.Max(maxX-minX, maxY-minY)
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

// degenRecord is one degenerate condition: a representative point and the sources
// it involves. An EMPTY srcs means the condition could not be attributed to any
// geometry that reaches the arrangement — an unusable input curve, dropped before
// it could form an edge — and every region carries it, since whatever that curve
// would have subdivided is unknown.
type degenRecord struct {
	x, y float64
	srcs []int
}

// flagDegenerate records a degenerate condition at (x,y) involving the given
// sources; the arrangement's regions are then not trustworthy.
//
// Pass every source the condition involves. A region is reported degenerate when
// its boundary uses one of them (see Region.Degenerate), so omitting them widens
// the blame to the whole arrangement — which is what a source-level failure (an
// unusable input curve, contributing no edge at all) wants and nothing else does.
func (a *arranger) flagDegenerate(x, y float64, srcs ...int) {
	a.degenSet = true
	a.degen = append(a.degen, degenRecord{x: x, y: y, srcs: srcs})
}

// densify samples each source into tiny segments and computes the scene scale,
// each source's own extent and the vertex-merge tolerance.
func (a *arranger) densify() {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	// Per-source bounds, accumulated from the same sample points as the scene's, so
	// source.extent is the scene scale's formula applied to ONE source (see the field).
	var sMinX, sMinY, sMaxX, sMaxY float64
	note := func(p [2]float64) {
		minX, maxX = math.Min(minX, p[0]), math.Max(maxX, p[0])
		minY, maxY = math.Min(minY, p[1]), math.Max(maxY, p[1])
		sMinX, sMaxX = math.Min(sMinX, p[0]), math.Max(sMaxX, p[0])
		sMinY, sMaxY = math.Min(sMinY, p[1]), math.Max(sMaxY, p[1])
	}
	for si := range a.sources {
		s := &a.sources[si]
		if s.kind == srcDegenerate {
			continue
		}
		sMinX, sMinY = math.Inf(1), math.Inf(1)
		sMaxX, sMaxY = math.Inf(-1), math.Inf(-1)
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
		s.extent = math.Max(sMaxX-sMinX, sMaxY-sMinY)
		if !(s.extent > 0) || math.IsInf(s.extent, 1) {
			s.extent = 0
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
	case srcSpline, srcClosedSpline, srcFitSpline, srcNURBS:
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
		case srcNURBS:
			n = 16 * len(s.nurbs.Control)
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
	a.analyticPrepass()
	n := len(a.segs)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			si, sj := &a.segs[i], &a.segs[j]
			if si.src != sj.src {
				// The planar map canonicalizes vertices by DISTANCE (a.merge), while the
				// crossing test below accepts a contact only inside a parametric window
				// (segEps). Two sample vertices of different sources that merge into one
				// graph vertex therefore split both sources even when the segment test
				// declines the pair — a near-miss just outside the window, or a parallel
				// pair that never reaches the test at all. Taint on the map's own merge
				// rule so no split can hide in that gap.
				if _, h := a.handled[pairKey(si.src, sj.src)]; h {
					// Supported source pairs (line/circle/arc) were classified analytically
					// in the pre-pass; their crossings are authoritative, so the sampled
					// segment test must not add contradictory ones. But the vertex table
					// still welds their sample vertices by DISTANCE, and a weld the kernel
					// found no event for is a split nothing exact explains — so audit the
					// welds against the pair's events instead of trusting the pair
					// wholesale (see auditMergedEndpoints).
					a.auditMergedEndpoints(si, sj)
					continue
				}
				a.taintMergedEndpoints(si, sj)
			}
			sameSpline := false
			if si.src == sj.src {
				// A simple source's own polyline never self-crosses. A spline (open
				// or closed periodic) can, so for a spline source test non-adjacent
				// sampled segments; adjacent ones (j == i+1) merely share a
				// subdivision vertex. The closure seam (first meets last segment) is
				// handled by the param-{0,1} check in the endpoint-meeting branch.
				k := a.sources[si.src].kind
				if (k != srcSpline && k != srcClosedSpline && k != srcFitSpline && k != srcNURBS) || j == i+1 {
					continue
				}
				sameSpline = true
			}
			p, ok := segParams(si, sj)
			if !ok {
				// Parallel: a collinear overlap is a duplicated/coincident edge
				// that corrupts the planar map — flag it rather than miscount.
				if mx, my, over := collinearOverlap(si, sj); over {
					a.flagDegenerate(mx, my, si.src, sj.src)
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
					// Between two DIFFERENT sources this is a join only where each side
					// sits at its SOURCE curve's own endpoint. A T-junction — one side at
					// its curve's endpoint, the other at an interior SAMPLE VERTEX of a
					// free-form curve — DOES split that curve in the graph (the shared
					// vertex reaches degree > 2, so its fragments stop coalescing). No cut
					// record belongs here either way (the vertex already exists), and the
					// split itself is recorded by taintMergedEndpoints above, which runs
					// on the vertex table's own merge rule before this branch is reached.
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
				a.flagDegenerate(p.x, p.y, si.src, sj.src)
			}
			a.noteSampledContact(si.src, sj.src, p.x, p.y)
			// exact:false — a sampled chord-chord crossing. Both the parameter and
			// the point are approximations that converge with sampling density.
			//
			// A crossing that lands ON a sample vertex (a tiny-segment boundary) needs
			// no new cut record — the vertex already exists — but the boundary's
			// parameter there must still read as sampled (see taintSampledVertex).
			if interiorI {
				si.cuts = append(si.cuts, cut{t: p.ti, px: p.x, py: p.y, exact: false})
			} else {
				a.taintSampledVertex(si.src, si.pa+p.ti*(si.pb-si.pa))
			}
			if interiorJ {
				sj.cuts = append(sj.cuts, cut{t: p.tj, px: p.x, py: p.y, exact: false})
			} else {
				a.taintSampledVertex(sj.src, sj.pa+p.tj*(sj.pb-sj.pa))
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
	a.refuseExactOnFusedMap()
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
	a.events = make(map[[2]int][]xEvent)
	a.deferredCross = make(map[[2]int][]xEvent)
	a.sampledContacts = make(map[[2]int][][2]float64)
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
			nCross, nTangent := 0, 0
			for _, e := range events {
				if e.kind == evCross {
					nCross++
				}
				if e.kind == evTangent {
					nTangent++
				}
			}
			// An INTERNAL curved tangency (one circle/arc inside the other, touching
			// at one point) is blessed via exact containment: the inner is a hole of
			// the outer, resolved by the exact point-in-region test in hole assignment.
			// The inner's chord polygon pokes outside the outer near the contact (so the
			// sampled count gate would flag it, and a sampled containment would miss the
			// hole) — both artifacts the exact path is immune to. So neither flag it nor
			// certify a port vertex; let the separate inner/outer loops + exact hole
			// assignment produce the annulus.
			internalTan := nTangent > 0 && a.internalCurvedTangency(i, j)
			// A curve/curve TRANSVERSE crossing (both sources circle/arc) takes analytic
			// authority only when the incidence certificate below passes. Unlike a
			// line-involved pair — whose line operand is reproduced exactly, so its
			// sampled crossing tracks the analytic one — BOTH polylines here are chord
			// approximations, and an exact cut bumps each of them outward by up to its
			// own sagitta. That is safe exactly when the bumped polylines still meet
			// only at the injected points, and cross there; when they do not, the map
			// would fuse regions (the round-2 bug) while reading clean.
			//
			// The fallback is the SAMPLED path, not a degeneracy: leave the pair
			// unhandled and let the sampled loop resolve it exactly as before the lift.
			// What the lift buys is the exact cut parameter (BoundaryEdge.TExact) and the
			// exact area, so a pair too coarsely sampled to certify loses only that, and
			// no caller that was blessed before the lift is refused after it.
			//
			// The sampled path resolves such a pair correctly wherever it finds the
			// crossing at all — and below that density it does not find it, fusing the
			// regions the crossing separates. That fusion is the sampled path's own
			// pre-existing density limit, but the OTHER pairs of the same component,
			// certified on their own merits, would then hand out exact bounds describing
			// the fused map. So each refused crossing is recorded here and reconciled
			// against the sampled map after the sampled loop (refuseExactOnFusedMap).
			curveCrossPair := nCross > 0 && isCurvedKind(si.kind) && isCurvedKind(sj.kind)
			if curveCrossPair && !a.analyticCrossingsCertified(i, j, events) {
				if ambiguous {
					rx, ry := sourceRep(si)
					sx, sy := sourceRep(sj)
					a.flagDegenerate((rx+sx)/2, (ry+sy)/2, i, j)
				}
				var crossings []xEvent
				for _, e := range events {
					if e.kind == evCross {
						crossings = append(crossings, e)
					}
				}
				a.deferredCross[pairKey(i, j)] = crossings
				continue
			}
			a.handled[[2]int{i, j}] = struct{}{}
			a.events[[2]int{i, j}] = events
			// Consistency gate (curved pairs only): the sampled polyline must host
			// the analytic contacts faithfully, or injecting exact cuts would warp
			// the planar map (a vanished disk, a tangled face) while reading clean.
			// Two conditions, one per kind of contact. (1) A crossing that INSERTS a
			// vertex on both sources must be WITNESSED on its own host segment-pair —
			// a coarse chord that never reaches the true crossing hosts nothing
			// (analyticCrossHosted). (2) A contact the sampled map already carries a
			// vertex for needs no witness, and cannot have one, but two of them inside
			// ONE chord put a whole cap below the sampling (contactsResolved). Failing
			// either, conservatively flag degeneracy. Pure line/line pairs are exact
			// (sample == geometry), so a clean shallow crossing is never false-flagged.
			// A coincident-carrier OVERLAP pair carries no crossing to witness — the two
			// sources' sampled polylines cross each other all along the shared arc, an
			// artifact of the coincidence itself, not something this gate can or should
			// judge — so it is exempted here; resolveCoincidentOverlap below is its own,
			// separate, sample-density-independent soundness argument (see "Determinism"
			// in docs/coincident-carrier-resolution-design.md).
			// A certified curve/curve crossing pair is exempt: analyticCrossingsCertified
			// already answered the same question directly, on the geometry the cuts
			// actually produce, and answered it without the strict host-segment-pair
			// requirement that a second, independently sampled curve makes fragile.
			isOverlapPair := len(events) == 1 && events[0].kind == evOverlap
			if !curveCrossPair && (isCurvedKind(si.kind) || isCurvedKind(sj.kind)) && !internalTan && !isOverlapPair {
				if !a.analyticCrossHosted(i, j, events) ||
					!a.contactsResolved(i, j, events) ||
					!a.sampledCrossingsExplained(i, j, events) {
					rx, ry := sourceRep(si)
					sx, sy := sourceRep(sj)
					a.flagDegenerate((rx+sx)/2, (ry+sy)/2, i, j)
				}
			}
			if ambiguous {
				rx, ry := sourceRep(si)
				sx, sy := sourceRep(sj)
				a.flagDegenerate((rx+sx)/2, (ry+sy)/2, i, j)
			}
			for _, e := range events {
				switch e.kind {
				case evOverlap:
					// A certified, single-window, at-least-one-arc coincidence (see
					// docs/coincident-carrier-resolution-design.md) is RESOLVED — cut,
					// suppress the losing source's edges over the shared span, and mark
					// handled rather than degenerate. Everything else that reaches this
					// arm (both operands covering the full turn, multi-window, a
					// coincident LINE carrier, or carriers equal only within the
					// classification band rather than at round-off) keeps the original
					// unconditional flag — e.overlap is nil there (populated only by
					// circleCircleEvents' in-scope branch). Whether the resolution
					// actually holds is NOT decided here: certifySuppression settles it
					// in split(), against the fragments split emits, and flags the pair
					// degenerate there if the window's boundaries did not survive.
					if e.overlap == nil {
						a.flagDegenerate(e.x, e.y, i, j)
						break
					}
					a.resolveCoincidentOverlap(i, j, e)
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
						switch {
						case internalTan || a.externalCurvedTangency(i, j):
							// Certify this shared contact for exact tangent-port ordering;
							// buildGraph orders its coincident-tangent ports by curvature so
							// the loops separate (external → two disks; internal → the outer
							// cycle and the inner cycle, which exact hole assignment nests
							// into an annulus + inner disk).
							a.exactPortVerts = append(a.exactPortVerts, [2]float64{e.x, e.y})
						default:
							a.flagDegenerate(e.x, e.y, i, j)
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

// sampledCrossingsExplained reports whether every crossing of the two sampled
// polylines answers to an analytic contact — the count half of the gate.
//
// The exact map is built from the kernel's contacts, so a sampled crossing with no
// contact behind it is the chord approximation disagreeing with the geometry: two
// arcs that merely touch slicing through each other, or a coarse chord cutting a
// line where the true curve does not. The face walk has no vertex for it, so it
// must not be blessed.
//
// A crossing is explained when it sits within one CURVED chord of a contact. That
// bound is where the chord approximation's own error lives — a curve's chord runs
// inside it, so a line reaching a contact ON the curve starts in the sliver between
// chord and arc and has to cross that chord to leave it (the inward gear spur is
// exactly this: one contact at the line's endpoint, one sampled crossing beside it
// that no analytic crossing corresponds to). A line contributes no bound at all:
// its polyline IS the line, so it deviates nowhere. The bound shrinks with sampling
// density, so this only ever forgives what the sampling itself produced.
func (a *arranger) sampledCrossingsExplained(i, j int, events []xEvent) bool {
	for _, ii := range a.sourceSegs[i] {
		for _, jj := range a.sourceSegs[j] {
			p, ok := a.segsCrossInteriorAt(ii, jj)
			if !ok {
				continue
			}
			var tol float64
			if isCurvedKind(a.sources[i].kind) {
				tol = math.Max(tol, a.segLen(ii))
			}
			if isCurvedKind(a.sources[j].kind) {
				tol = math.Max(tol, a.segLen(jj))
			}
			explained := false
			for _, e := range events {
				if math.Hypot(e.x-p.x, e.y-p.y) <= tol {
					explained = true
					break
				}
			}
			if !explained {
				return false
			}
		}
	}
	return true
}

// segLen is a tiny segment's chord length.
func (a *arranger) segLen(si int) float64 {
	s := &a.segs[si]
	return math.Hypot(s.bx-s.ax, s.by-s.ay)
}

// contactsResolved reports whether the sampling separates the pair's contacts on
// each curved source: two of them falling inside ONE chord of a curve mean the
// sampling cannot resolve the geometry between them, so whatever the exact cuts
// produce there cannot be blessed.
//
// That stretch is a sub-sample cap. The plainest failure is a line whose BOTH ends
// lie on a circle within one sampled chord: neither contact needs a sampled witness
// (each sits at the line's own endpoint), so the host check passes vacuously, yet
// the whole line runs through the sliver OUTSIDE the sampled polygon and the planar
// map collapses — the disk vanishes while the arrangement reads clean. Contacts
// that DO need a witness are held to this too: hosting says the chords cross where
// the curves do, not that the cap between two crossings is resolved at all. A
// caller who wants such a contact blessed raises WithSegmentsPerTurn.
//
// Only CURVED sources are checked. A line is reproduced exactly by its single
// segment, so two contacts sharing it (an ordinary secant) say nothing about
// resolution.
func (a *arranger) contactsResolved(i, j int, events []xEvent) bool {
	for _, src := range [2]int{i, j} {
		if !isCurvedKind(a.sources[src].kind) {
			continue
		}
		seen := map[int]struct{}{}
		for _, e := range events {
			t := e.ti
			if src == j {
				t = e.tj
			}
			seg := a.segContaining(src, t)
			if seg < 0 {
				continue
			}
			if _, dup := seen[seg]; dup {
				return false
			}
			seen[seg] = struct{}{}
		}
	}
	return true
}

// segsCrossInterior reports whether two tiny segments cross at a point strictly
// interior to both — the transverse-crossing predicate the consistency gate uses.
// Requiring both-interior (not merely one) excludes a tangential touch at a shared
// sample vertex — a line grazing a circle exactly at a polygon vertex is interior
// to the line but sits at the circle's vertex, a contact the analytic kernel
// correctly reports as a tangency, not a transverse crossing.
func (a *arranger) segsCrossInterior(ii, jj int) bool {
	_, ok := a.segsCrossInteriorAt(ii, jj)
	return ok
}

// segsCrossInteriorAt is segsCrossInterior with the hit itself, for a caller that
// needs where the two chords crossed.
func (a *arranger) segsCrossInteriorAt(ii, jj int) (segHit, bool) {
	p, ok := segParams(&a.segs[ii], &a.segs[jj])
	if !ok {
		return segHit{}, false
	}
	if p.ti <= segEps || p.ti >= 1-segEps || p.tj <= segEps || p.tj >= 1-segEps {
		return segHit{}, false
	}
	return p, true
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
		// A crossing the sampled map already carries a vertex for needs no
		// transverse witness — see crossNeedsSampledWitness.
		if !a.crossNeedsSampledWitness(i, j, e) {
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

// analyticCrossingsCertified reports whether the pair's exact transverse crossings
// can be injected into the sampled map without changing its topology — the
// incidence certificate for a CURVE/CURVE crossing, where analyticCrossHosted's
// strict host-segment-pair witness does not carry.
//
// It asks the question directly, on the geometry the cut phase actually emits:
// splice each exact crossing point into BOTH sources' polylines at the site
// applyAnalyticCut would use (postCutPolyline, so the gate can never test something
// the cut phase does not do), then require
//
//   - the two spliced polylines to meet ONLY at those points — every contact between
//     them IS an injected crossing point (polylinesMeetOnlyAtContacts). A leftover
//     contact has no vertex in the planar map, because a handled pair's sampled
//     crossings are never recorded, so the face walk would run two edges through each
//     other and fuse the regions on either side. That is the round-2 failure exactly.
//     The membership is decided by IDENTITY with an injected point, never by where the
//     contact sits along its host segments: an arc's own endpoint resting on the
//     interior of the other source's chord, and a transverse pass-through that lands on
//     a sample vertex of one polyline, are both contacts with no node — and a
//     segment-endpoint band admits both of them.
//   - each contact to sit AT the polyline vertex it was mapped to (contactIsVertex).
//     A spliced point satisfies this by construction; a contact postCutPolyline mapped
//     onto an EXISTING vertex need not, because that mapping is decided in the source's
//     parameter and a parameter that close still admits a position gap far above
//     round-off. Where the gap is real the vertex, not the contact, is what the
//     remaining checks and the emitted parameter would describe; where the contact IS
//     the vertex the two are one point and the bound is the true crossing parameter.
//   - the four chord departures at each injected point to ALTERNATE between the two
//     sources (portsCross), in the same rotation order buildGraph sorts by. Meeting
//     at a point is not crossing at it: if both of one source's chords leave on the
//     same side of the other's, the loops touch, and the face walk pairs the wrong
//     half-edges. A contact at an open source's own ENDPOINT has only three
//     departures and so can never pass — the sampled fallback owns that case.
//
// Together those are the polygonal statement of "the sampled map has the same
// crossing incidence as the exact geometry", which is what injecting an exact cut
// needs and all it needs. The third is threshold-free — no tolerance, no chord-length
// bound, no crossing-angle floor — so a shallow crossing is judged by whether the
// chords actually resolve it, not by how shallow it is. The first two decide only
// "one point or two", and both decide it with the SAME identity band vertexCertifies
// uses: the first to ask whether a contact IS an injected crossing point, the second
// whether it IS the vertex it was mapped to (there additionally bounded by the vertex's
// own chord, as vertexCertifies is by its source's own extent, so a distant object
// cannot widen it). No crossing-angle or chord-length threshold enters either verdict.
//
// Why analyticCrossHosted is not that statement for this pair: it requires the
// crossing to be witnessed on the very segment pair carrying its two source
// parameters. The sampled crossing sits off the exact one by roughly the sagitta
// divided by the sine of the crossing angle, so whenever that offset carries it into
// a neighbouring segment the witness is looked for in the wrong place. With a line
// operand only one grid can be off (the line's polyline IS the line, one segment
// covering it); with two sampled curves both can, and the miss rate stops falling
// with density in any useful way — it aliases against the two sampling grids.
func (a *arranger) analyticCrossingsCertified(i, j int, events []xEvent) bool {
	var ti, tj []float64
	var pts [][2]float64
	for _, e := range events {
		if e.kind != evCross {
			continue
		}
		ti = append(ti, e.ti)
		tj = append(tj, e.tj)
		pts = append(pts, [2]float64{e.x, e.y})
	}
	if len(pts) == 0 {
		return true
	}
	pi, ci := a.postCutPolyline(i, ti, pts)
	pj, cj := a.postCutPolyline(j, tj, pts)
	if pi == nil || pj == nil {
		return false // a contact the sampled polyline has no place for
	}
	if !polylinesMeetOnlyAtContacts(pi, pj, pts, weldIdentEps*a.scale) {
		return false
	}
	for k := range pts {
		if !a.contactIsVertex(pi, ci[k], pts[k]) || !a.contactIsVertex(pj, cj[k], pts[k]) {
			return false
		}
		if !portsCross(pi, ci[k], a.sources[i].closed, pj, cj[k], a.sources[j].closed) {
			return false
		}
	}
	return true
}

// contactIsVertex reports whether the polyline vertex a contact was mapped to IS the
// contact point, at the round-off identity band — bounded by TWO yardsticks at once, the
// scene's own scale and the CHORD the vertex sits on, exactly as carriersIdentical bounds
// a carrier match by both the scene band and the carrier-local one.
//
// For a spliced contact this holds by construction — the vertex is the contact point, at
// zero distance. It is load-bearing for a contact cutSite reported as already carrying a
// vertex: that decision is made in the source's PARAMETER (within segEps of a segment
// boundary), and a parameter that close still admits a POSITION gap of up to a few times
// segEps·segment length, orders of magnitude above the identity band. Certifying a contact
// with a real gap hands the sampled vertex's own parameter — a plain sample fraction i/n —
// out as an exact bound, describing a point the crossing is not at. A curve/curve pair has
// no second safety net here: certification exempts it from the taint passes that would
// otherwise mark a weld inexact, so the certificate itself has to refuse. Refusing means
// the sampled fallback, where the parameter is reported inexact and the topology is
// unchanged. A contact that IS the vertex passes and keeps its exact bound: the sample
// fraction and the true crossing parameter are then the same number, and evaluating it
// reproduces the emitted point, which is all TExact claims.
//
// The CHORD-LOCAL band is what keeps the question about THIS contact. a.scale is the whole
// scene's bounding-box extent, so an unrelated object far away widens it: with a circle of
// radius 5 the verdict flips at a scene extent of about 24.5·r, so ONE construction line
// parked 100 units away turns a contact 1.1e-10 off its vertex — a gap the same pair
// correctly refuses when drawn alone — into a certified one, publishing the vertex's own
// sample fraction as an exact crossing parameter. Nothing about the pair changed; only the
// scene did. The gap is measured against the chord the vertex belongs to, which is the
// only length the mapping decision (a segEps window on that chord's parameter) is stated
// in, and no distant object can inflate it.
func (a *arranger) contactIsVertex(p [][2]float64, k int, pt [2]float64) bool {
	if k < 0 || k >= len(p) {
		return false
	}
	gap := math.Hypot(p[k][0]-pt[0], p[k][1]-pt[1])
	return gap <= weldIdentEps*a.scale && gap <= weldIdentEps*polylineChordAt(p, k)
}

// polylineChordAt returns the longer of the chords meeting at the polyline's k-th vertex —
// the local length scale of the mapping that put a contact there. The longer of the two is
// the one whose segEps parameter window admits the widest position gap, so it is the bound
// the check has to be stated against; a lone vertex (no chord) has no local scale and
// admits only an exact match.
func polylineChordAt(p [][2]float64, k int) float64 {
	out := 0.0
	if k > 0 {
		out = math.Max(out, math.Hypot(p[k][0]-p[k-1][0], p[k][1]-p[k-1][1]))
	}
	if k+1 < len(p) {
		out = math.Max(out, math.Hypot(p[k+1][0]-p[k][0], p[k+1][1]-p[k][1]))
	}
	return out
}

// noteSampledContact records that the SAMPLED loop put a contact between two DIFFERENT
// sources at (x,y): a chord/chord crossing, or a weld of their sample vertices (the
// shape a crossing takes when it lands on a sample vertex of both). It is the evidence
// refuseExactOnFusedMap reconciles a refused analytic crossing against
// (sampledRepresents).
func (a *arranger) noteSampledContact(i, j int, x, y float64) {
	if i == j {
		return
	}
	k := pairKey(i, j)
	a.sampledContacts[k] = append(a.sampledContacts[k], [2]float64{x, y})
}

// refuseExactOnFusedMap withdraws exact authority from every source of a connected
// component whose planar map may be FUSED — one carrying a crossing that is missing
// from the sampled map the pair was handed back to.
//
// The pair handed back is a curve/curve crossing the incidence certificate REFUSED
// (deferredCross). It rests on the sampled path, which is sound only while that path
// RESOLVES the crossing. Below the density where the two chord polylines meet at all,
// it does not: the crossing disappears, the regions it separates fuse, and nothing
// flags it (this is the sampled path's own pre-existing sampling limit — the region
// count changes with WithSegmentsPerTurn either way, and repairing that is not what
// this pass is for). What this pass prevents is the fused map being published as EXACT.
// The other pairs of the same component are certified on their own merits and cut
// exactly, so every surviving fragment reports TExact — describing a topology that is
// wrong. Three r=5 circles in general position show it directly: two pairs certify, the
// third pair's shallow crossing is below the sampling, and the arrangement reports five
// regions with every bound exact where seven is the truth.
//
// A SAMPLED-ONLY pair — one involving an ellipse, conic, spline or NURBS — can hide a
// crossing the same way, and is answered a whole level up instead, by the exactAllowed
// scene gate: such a pair only exists in a scene that publishes no exact bound at all.
// Nothing here estimates how far a chord runs from its curve.
//
// The unit is the CONNECTED COMPONENT, not the offending pair: a fused crossing moves
// the face boundaries of every cycle it takes part in, so a fragment of ANY source
// reachable through contacts is describing the fused map. Sources are joined here by
// CONTACT — an analytic event of a handled pair, a refused crossing, or a sampled
// contact. A handled pair with NO event is not a contact and must not join: the kernel
// classified those two sources and found they never meet, so an untouched cluster
// elsewhere in the scene keeps its exactness however busy the rest of the drawing is.
//
// Refusal costs only the exactness FLAG. Topology, areas and degeneracy are untouched,
// and the reported ranges stay the sampled ones, which is exactly what the same
// geometry reported before curve/curve crossings could certify at all.
func (a *arranger) refuseExactOnFusedMap() {
	if len(a.deferredCross) == 0 {
		return
	}
	uf := newUnionFind(len(a.sources))
	for k, evs := range a.events {
		if len(evs) == 0 {
			continue // classified and found not to meet — no contact, no component edge
		}
		uf.union(k[0], k[1])
	}
	for k := range a.deferredCross {
		uf.union(k[0], k[1])
	}
	for k := range a.sampledContacts {
		uf.union(k[0], k[1])
	}
	fused := map[int]struct{}{}
	for k, evs := range a.deferredCross {
		for _, e := range evs {
			if a.sampledRepresents(k[0], k[1], e) {
				continue
			}
			fused[uf.find(k[0])] = struct{}{}
			break
		}
	}
	if len(fused) == 0 {
		return
	}
	a.exactRefused = make([]bool, len(a.sources))
	for s := range a.sources {
		if _, bad := fused[uf.find(s)]; bad {
			a.exactRefused[s] = true
		}
	}
}

// sampledRepresents reports whether the sampled map carries a contact for a refused
// analytic crossing of the pair.
//
// The sampled crossing does not sit ON the exact one — it is displaced by roughly the
// chord sagitta divided by the sine of the crossing angle — so the question is asked
// within one chord of the two host segments (the same bound sampledCrossingsExplained
// uses, and where the chord approximation's own error lives), floored at the vertex
// merge tolerance for a contact the sampling recorded as a weld. Judging a represented
// crossing unrepresented costs only the exactness flag on that component; the reverse
// would publish a fused map as exact, so the bound is deliberately the tight one.
func (a *arranger) sampledRepresents(i, j int, e xEvent) bool {
	tol := a.merge
	if si := a.segContaining(i, e.ti); si >= 0 {
		tol = math.Max(tol, a.segLen(si))
	}
	if sj := a.segContaining(j, e.tj); sj >= 0 {
		tol = math.Max(tol, a.segLen(sj))
	}
	for _, p := range a.sampledContacts[pairKey(i, j)] {
		if math.Hypot(p[0]-e.x, p[1]-e.y) <= tol {
			return true
		}
	}
	return false
}

// postCutPolyline returns source src's sampled polyline with the given exact contact
// points spliced in — the geometry split() emits once applyAnalyticCut has run — plus
// the index each contact occupies in it.
//
// The splice sites come from cutSite, the same call applyAnalyticCut acts on, so the
// polyline this builds is the one the cut phase produces and not an idealization of
// it: a contact the sampled map already has a vertex for (a source endpoint, a sample
// vertex) is NOT inserted — it maps to that existing vertex, exactly as
// applyAnalyticCut records nothing there, and WHICH vertex is read off cutSite's local
// parameter. A nil polyline means some contact sits on no sampled segment at all,
// which no splice can represent. The mapped vertex is not assumed to BE the contact:
// the caller checks that (contactIsVertex), because the parameter cutSite decided on
// bounds no position gap.
func (a *arranger) postCutPolyline(src int, ts []float64, pts [][2]float64) ([][2]float64, []int) {
	segs := a.sourceSegs[src]
	if len(segs) == 0 {
		return nil, nil
	}
	type site struct {
		seg   int     // index into a.segs
		pos   int     // that segment's position along the source
		local float64 // local chord parameter within it
		atVtx bool    // the sampled map already carries a vertex here
		k     int     // index into the caller's contact list
	}
	pos := make(map[int]int, len(segs))
	for n, si := range segs {
		pos[si] = n
	}
	sites := make([]site, 0, len(ts))
	for k, t := range ts {
		seg, local, atVtx := a.cutSite(src, t)
		n, ok := pos[seg]
		if seg < 0 || !ok {
			return nil, nil
		}
		sites = append(sites, site{seg: seg, pos: n, local: local, atVtx: atVtx, k: k})
	}
	sort.Slice(sites, func(x, y int) bool {
		if sites[x].pos != sites[y].pos {
			return sites[x].pos < sites[y].pos
		}
		return sites[x].local < sites[y].local
	})
	out := make([][2]float64, 0, len(segs)+1+len(sites))
	at := make([]int, len(ts))
	segStart := make([]int, len(segs))
	for n, si := range segs {
		s := &a.segs[si]
		segStart[n] = len(out)
		out = append(out, [2]float64{s.ax, s.ay})
		for _, c := range sites {
			if c.pos == n && !c.atVtx {
				out = append(out, pts[c.k])
				at[c.k] = len(out) - 1
			}
		}
	}
	end := &a.segs[segs[len(segs)-1]]
	lastIdx := len(out)
	out = append(out, [2]float64{end.bx, end.by})
	// A contact already carrying a vertex maps to it: the segment's own start when
	// its local parameter sits at 0, else the vertex that begins the next segment
	// (the last segment's end being the polyline's own end).
	for _, c := range sites {
		if !c.atVtx {
			continue
		}
		switch {
		case c.local <= 0.5:
			at[c.k] = segStart[c.pos]
		case c.pos+1 < len(segs):
			at[c.k] = segStart[c.pos+1]
		default:
			at[c.k] = lastIdx
		}
	}
	return out, at
}

// polylinesMeetOnlyAtContacts reports whether the two spliced polylines meet ONLY at
// the injected contact points pts — every contact between a segment of one and a
// segment of the other IS one of those points, within the round-off identity band eps
// (weldIdentEps·scale, the scene half of the band contactIsVertex and vertexCertifies
// decide "one point or two" by; each of those adds a local yardstick of its own, and
// this predicate has no single source or vertex to state one against).
//
// Membership is decided by IDENTITY with an injected point. Deciding it by POSITION
// ALONG THE HOST SEGMENTS instead — excluding a contact whose segment parameter sits
// within segEps of either segment's end, which is what this predicate used to do — is a
// different question, and the gap between the two admits contacts that carry no node in
// the planar map:
//
//   - a source's own ENDPOINT resting on the INTERIOR of the other source's chord. The
//     endpoint is one segment's parameter 1, so the band excused it, while the contact
//     is a real meeting of the two polylines that the exact geometry does not have.
//   - a genuine transverse PASS-THROUGH that happens to land on a sample vertex of one
//     of the polylines. It is the same crossing the gate exists to refuse, waved
//     through for sitting at a vertex of one source rather than being a vertex of both.
//
// PARALLEL pairs never reached the parameter test at all: segParams rejects a pair by
// its determinant before any range test, so a collinear OVERLAP — two chords sharing a
// whole span rather than a point — arrived as silence and was accepted with no
// tolerance applied to it whatever. collinearOverlap is the same coincident-edge test
// the sampled loop uses, and an overlap of positive length is never a transverse
// crossing, so it is refused outright.
//
// Refusing costs the sampled fallback, which keeps the correct sampled topology and
// reports TExact=false; accepting one of these publishes a fused map as exact.
func polylinesMeetOnlyAtContacts(pi, pj, pts [][2]float64, eps float64) bool {
	injected := func(x, y float64) bool {
		for _, p := range pts {
			if math.Hypot(p[0]-x, p[1]-y) <= eps {
				return true
			}
		}
		return false
	}
	for x := 0; x+1 < len(pi); x++ {
		si := tinySeg{ax: pi[x][0], ay: pi[x][1], bx: pi[x+1][0], by: pi[x+1][1]}
		for y := 0; y+1 < len(pj); y++ {
			sj := tinySeg{ax: pj[y][0], ay: pj[y][1], bx: pj[y+1][0], by: pj[y+1][1]}
			if _, _, ok := collinearOverlap(&si, &sj); ok {
				return false
			}
			p, ok := segParams(&si, &sj)
			if !ok {
				continue
			}
			if !injected(p.x, p.y) {
				return false
			}
		}
	}
	return true
}

// portsCross reports whether the two polylines genuinely cross at the vertex each
// carries the contact at: their four chord departures must alternate between the
// sources in angular order — the rotation order buildGraph itself sorts half-edges
// by, so this is the map's own criterion, not a proxy for it.
//
// Anything that is not four departures fails. An index of -1 (no such vertex) has
// none. A vertex with a SINGLE departure is an open curve's ENDPOINT: the curve stops
// there, so it contributes one direction, the four cannot alternate, and there is no
// crossing here to certify — the contact is a T-junction, which the exact geometry may
// well have but which this predicate has no evidence about. Passing it certified the
// injected cut against nothing, and the cut then bent the other source's chord through
// the endpoint and silently erased a region; such a contact takes the sampled fallback
// instead. A CLOSED source's seam is not the endpoint case and must not be read as one
// — hence the per-polyline closed flag.
func portsCross(pi [][2]float64, a int, aClosed bool, pj [][2]float64, b int, bClosed bool) bool {
	di := polylinePorts(pi, a, aClosed)
	dj := polylinePorts(pj, b, bClosed)
	if len(di) != 2 || len(dj) != 2 {
		return false
	}
	angs := [4]float64{
		math.Atan2(di[0][1], di[0][0]), math.Atan2(di[1][1], di[1][0]),
		math.Atan2(dj[0][1], dj[0][0]), math.Atan2(dj[1][1], dj[1][0]),
	}
	src := [4]int{0, 0, 1, 1}
	for x := 1; x < 4; x++ {
		for y := x; y > 0 && angs[y] < angs[y-1]; y-- {
			angs[y], angs[y-1] = angs[y-1], angs[y]
			src[y], src[y-1] = src[y-1], src[y]
		}
	}
	return src[0] != src[1] && src[1] != src[2] && src[2] != src[3]
}

// polylinePorts returns the chord departure vectors at the polyline's k-th vertex.
// An interior vertex has two and an OPEN curve's endpoint has one. A CLOSED source's
// polyline repeats its seam vertex at index 0 and at the end, so either index names
// the one seam vertex, whose two departures are its two distinct neighbours.
func polylinePorts(p [][2]float64, k int, closed bool) [][2]float64 {
	if k < 0 || k >= len(p) {
		return nil
	}
	if closed && len(p) >= 3 && (k == 0 || k == len(p)-1) {
		return [][2]float64{
			{p[1][0] - p[0][0], p[1][1] - p[0][1]},
			{p[len(p)-2][0] - p[len(p)-1][0], p[len(p)-2][1] - p[len(p)-1][1]},
		}
	}
	var out [][2]float64
	if k > 0 {
		out = append(out, [2]float64{p[k-1][0] - p[k][0], p[k-1][1] - p[k][1]})
	}
	if k+1 < len(p) {
		out = append(out, [2]float64{p[k+1][0] - p[k][0], p[k+1][1] - p[k][1]})
	}
	return out
}

// crossNeedsSampledWitness reports whether an analytic transverse crossing has to
// be witnessed by a sampled interior crossing of the two polylines.
//
// A witness is needed only when the crossing inserts a NEW vertex on both sources.
// That is the case the gate exists for: the injected point sits on the true curve,
// off the chord by up to the sagitta, so at coarse sampling it can bend the polygon
// through the other curve the wrong way (the vanished disk) — and the sampled map
// hosting the same crossing is the evidence it does not.
//
// A crossing that lands where the sampled map ALREADY has a vertex inserts nothing
// there (cutSite says so, and applyAnalyticCut acts on exactly that answer), so
// there is nothing to bend and nothing to witness. Two ordinary arrangements reach
// this, and both used to be flagged degenerate:
//
//   - a contact at a source's own ENDPOINT — a corner join between two curves, or a
//     spur ending on a circle (the gear flank meeting its root circle). It splits
//     only the other source, at the exact point the endpoint's own vertex welds to.
//   - a contact at an interior SAMPLE VERTEX — the sample vertex IS the true
//     crossing point (the vertex is the source evaluated at that parameter, and the
//     parameters agree), so the polyline already passes through it.
//
// The sampled interior predicate can never host either one (a contact at a segment
// boundary is not interior to that segment), so requiring a witness there asks for
// evidence that cannot exist. What such contacts get instead is contactsResolved,
// which checks the sampling is fine enough to separate them.
func (a *arranger) crossNeedsSampledWitness(i, j int, e xEvent) bool {
	segI, _, vertI := a.cutSite(i, e.ti)
	segJ, _, vertJ := a.cutSite(j, e.tj)
	if segI < 0 || segJ < 0 {
		// The crossing parameter sits on no tiny segment at all. Demand a witness so
		// analyticCrossHosted rejects it, rather than blessing a contact the sampled
		// map has no place for.
		return true
	}
	return !vertI && !vertJ
}

// cutSite locates the tiny segment of source src carrying an analytic contact at
// source parameter t, returning that segment's index and the contact's local chord
// parameter. The index is -1 when no segment covers t.
//
// atVertex reports that the contact needs no cut because the sampled map already
// has a vertex there: the source's own endpoint (a join, which never splits it), or
// a tiny-segment boundary (a sample vertex, which is already at the true parameter).
// It is the single place that decision is made — applyAnalyticCut acts on it, and
// crossNeedsSampledWitness reads it — so what the gate expects can never drift from
// what the cut phase does.
//
// The local parameter is reported for a SOURCE END too, and it is the real one — 0
// at the source's start, 1 at an open source's end — not a placeholder. postCutPolyline
// reads it to decide WHICH sampled vertex the contact occupies, and a hardcoded 0 named
// the far end of the last segment for a contact at t=1: the certificate then ran on a
// vertex a whole chord away from the contact, which has the two departures the real
// endpoint does not, and blessed a crossing the injected cut went on to erase a region
// over.
func (a *arranger) cutSite(src int, t float64) (int, float64, bool) {
	if atSourceEnd(&a.sources[src], t) {
		si := a.segContaining(src, t)
		if si < 0 {
			return -1, 0, true
		}
		return si, segLocal(&a.segs[si], t), true
	}
	for _, si := range a.sourceSegs[src] {
		s := &a.segs[si]
		lo, hi := s.pa, s.pb
		if hi < lo {
			lo, hi = hi, lo
		}
		if t < lo-segEps || t > hi+segEps {
			continue
		}
		local := segLocal(s, t)
		return si, local, local <= segEps || local >= 1-segEps
	}
	return -1, 0, false
}

// segLocal maps a source parameter onto the segment's local chord parameter, clamped
// to [0,1] — a source parameter reaches this a hair outside the segment's own span
// (both the atSourceEnd band and the segEps slack in the scan admit that), and a local
// parameter outside [0,1] names no point of the chord.
func segLocal(s *tinySeg, t float64) float64 {
	if s.pb == s.pa {
		return 0
	}
	local := (t - s.pa) / (s.pb - s.pa)
	return math.Min(1, math.Max(0, local))
}

// applyAnalyticCut records an exact cut at source-parameter t (event point x,y) on
// the tiny segment of source src that contains t. A cut at a segment boundary or a
// source endpoint reuses the existing vertex and records nothing — the vertex is
// already there, at the true parameter, so the fragment bounds it yields stay exact.
func (a *arranger) applyAnalyticCut(src int, t, x, y float64) {
	si, local, atVertex := a.cutSite(src, t)
	if si < 0 || atVertex {
		return
	}
	a.segs[si].cuts = append(a.segs[si].cuts, cut{t: local, px: x, py: y, exact: true})
}

// resolveCoincidentOverlap cuts both sources of a certified, single-window
// coincident-carrier overlap at its two boundary points and records a
// suppression window on the LOSING source (j, always the higher of the pair's
// indices — analyticPrepass's own i<j iteration order — see "The SourceIndex
// decision" in docs/coincident-carrier-resolution-design.md), so split() omits
// its edges over the shared span; the NAMED source i represents it instead.
//
// Both boundary points are exact — each is one operand's own domain end or the
// other's, never a solved root — and both cuts are stamped exact:true on BOTH
// sources even though the points are computed on ONE operand's carrier. That is
// sound only because the event is emitted only for carriers identical at round-off
// (carriersIdentical), which bounds how far the point can sit off the other
// operand's own curve.
//
// Recording the window is a CLAIM, not the resolution: nothing here predicts what
// the cut phase and split's per-segment dedup will make of these two boundaries.
// certifySuppression settles that inside split(), with the emitted fragments in
// hand, and withdraws the record — flagging the pair degenerate, exactly as every
// other out-of-scope overlap is flagged — when the boundaries did not survive as
// distinct fragment bounds shared by both sources.
func (a *arranger) resolveCoincidentOverlap(i, j int, e xEvent) {
	ov := e.overlap
	a.applyAnalyticCut(i, ov.loTi, ov.loX, ov.loY)
	a.applyAnalyticCut(i, ov.hiTi, ov.hiX, ov.hiY)
	a.applyAnalyticCut(j, ov.loTj, ov.loX, ov.loY)
	a.applyAnalyticCut(j, ov.hiTj, ov.hiX, ov.hiY)
	if a.suppressed == nil {
		a.suppressed = map[int][]int{}
	}
	a.suppressed[j] = append(a.suppressed[j], len(a.overlaps))
	a.overlaps = append(a.overlaps, coincidentOverlap{
		named: i, losing: j,
		loX: ov.loX, loY: ov.loY, hiX: ov.hiX, hiY: ov.hiY,
		repX: e.x, repY: e.y,
		win: angularWindow{cx: a.sources[j].cx, cy: a.sources[j].cy, angLo: ov.angLo, width: ov.width},
	})
}

// certifySuppression is the POSTCONDITION each recorded coincident-carrier
// suppression window must clear before split() acts on it, and the reason the
// window's boundaries are no longer predicted anywhere earlier.
//
// The window is stated at analytic-pass time in exact angles, but suppression is
// only sound while both of its boundaries really are boundaries of the losing
// source's emitted fragments AND of the named source's — that is what makes "no
// emitted fragment straddles a window boundary" (angularWindow.contains) true, and
// what lets the named source's surviving edge attach where the losing source's kept
// fragments end. Every earlier attempt to predict that outcome disagreed with it by
// a different route: applyAnalyticCut records nothing when cutSite judges — in
// PARAMETER space — that a vertex is already there, and split's per-segment dedup
// then drops any boundary a COMPETING cut lands within segEps of, a global decision
// over every cut on that segment that no per-boundary precondition can see.
//
// So the check is made where the outcome is known: over the fragments split has
// already built and canonicalized. Both boundaries must resolve to a graph vertex
// that bounds a fragment of the losing source AND a fragment of the named source —
// the SAME vertex on both, since a vertex is exactly what the two sources have to
// share for the loop to close — and the two must be DISTINCT vertices, or the span
// between them was never emitted. Identity is decided by vertex, never by distance;
// a.merge only locates which vertex a boundary point belongs to, which is the
// welding radius by definition.
//
// A window that fails is withdrawn rather than repaired: the losing source then
// emits its coincident span exactly as it did before this design, and the pair is
// flagged degenerate — the conservative verdict a caller can act on, in place of a
// silently deleted region.
func (a *arranger) certifySuppression(frags []splitFrag) {
	if len(a.overlaps) == 0 {
		return
	}
	bounds := make([]map[int]struct{}, len(a.sources))
	for _, f := range frags {
		src := a.segs[f.seg].src
		if bounds[src] == nil {
			bounds[src] = map[int]struct{}{}
		}
		bounds[src][f.u] = struct{}{}
		bounds[src][f.v] = struct{}{}
	}
	for k := range a.overlaps {
		o := &a.overlaps[k]
		vLoN, okLoN := a.boundVertexAt(bounds[o.named], o.loX, o.loY)
		vHiN, okHiN := a.boundVertexAt(bounds[o.named], o.hiX, o.hiY)
		vLoL, okLoL := a.boundVertexAt(bounds[o.losing], o.loX, o.loY)
		vHiL, okHiL := a.boundVertexAt(bounds[o.losing], o.hiX, o.hiY)
		if okLoN && okHiN && okLoL && okHiL && vLoN == vLoL && vHiN == vHiL && vLoL != vHiL {
			continue
		}
		o.refused = true
		a.flagDegenerate(o.repX, o.repY, o.named, o.losing)
	}
}

// boundVertexAt returns the graph vertex, among those bounding a source's emitted
// fragments, that the point (x,y) belongs to: the nearest one within the vertex
// table's own merge tolerance, which is precisely the radius at which canon would
// have welded the point onto it. Ties break to the lower vertex id so the answer
// does not depend on map iteration order.
func (a *arranger) boundVertexAt(verts map[int]struct{}, x, y float64) (int, bool) {
	best, bestD := -1, math.Inf(1)
	for v := range verts {
		vx, vy := a.verts.coord(v)
		d := math.Hypot(vx-x, vy-y)
		if d > a.merge {
			continue
		}
		if d < bestD || (d == bestD && v < best) {
			best, bestD = v, d
		}
	}
	return best, best >= 0
}

// fragmentSuppressed reports whether the fragment of source src spanning natural
// parameters p0..p1 falls inside a suppression window still standing for src (one
// certifySuppression withdrew suppresses nothing).
//
// The fragment is classified by the source EVALUATED at its parameter midpoint — a
// point on the source by construction, whatever the fragment's extent. Its CHORD
// midpoint is not a substitute: for a fragment spanning half a circle the chord is a
// diameter and its midpoint is the carrier centre itself, where the window's angle
// test reads atan2(0,0) = 0 and suppresses (or keeps) the fragment for a reason that
// has nothing to do with where it lies. A tiny segment never wraps a closed source's
// seam, so the midpoint parameter is genuinely between p0 and p1.
func (a *arranger) fragmentSuppressed(src int, p0, p1 float64) bool {
	ks := a.suppressed[src]
	if len(ks) == 0 {
		return false
	}
	p := a.sources[src].at((p0 + p1) / 2)
	for _, k := range ks {
		if !a.overlaps[k].refused && a.overlaps[k].win.contains(p[0], p[1]) {
			return true
		}
	}
	return false
}

// taintMergedEndpoints taints the sample vertices of two DIFFERENT sources whose
// tiny-segment endpoints canonicalize to the same graph vertex (they lie within the
// vertex-merge tolerance). Such a vertex is shared, so every source incident to it
// at an interior sample vertex is split there — and the contact was found by the
// sampled polyline, not the closed-form kernel, so its parameter must read sampled.
//
// The merge tolerance is the SAME one buildGraph's vertex table uses, which is what
// makes this exhaustive: the sampled crossing test can only see a contact inside its
// parametric window, so it alone cannot guarantee every merged vertex is accounted
// for. taintSampledVertex no-ops at a source's own endpoint, so an ordinary
// end-to-end join between two curves stays a join.
func (a *arranger) taintMergedEndpoints(si, sj *tinySeg) {
	a.forEachMergedEnd(si, sj, func(e mergedEnd) {
		// A weld IS a contact the sampled map made between the two sources — the
		// shape a crossing takes when it lands on a sample vertex of both rather
		// than interior to a chord of each. refuseExactOnFusedMap reads it as such.
		a.noteSampledContact(si.src, sj.src, e.xi, e.yi)
		a.taintSampledVertex(si.src, e.ti)
		a.taintSampledVertex(sj.src, e.tj)
	})
}

// auditMergedEndpoints is taintMergedEndpoints for a pair the analytic pre-pass
// HANDLED. Being handled means the closed-form kernel is authoritative about where
// the two sources meet — but the vertex table still welds sample vertices by
// DISTANCE, and the two rules do not agree: a line whose true circle intersections
// sit just OUTSIDE its segment produces no analytic event at all, yet its endpoint
// can still land within the merge tolerance of a circle sample vertex and weld. The
// weld splits the circle in the graph all the same, and nothing exact explains where.
//
// So each weld is audited against the pair's own events: a weld an analytic event
// sits at keeps its honest exactness (the kernel found a real contact there, and the
// parameter at that vertex is the true one — an exact cut must not be laundered into
// a sampled one), while a weld NO event explains is a distance-only split and is
// tainted, exactly like the sampled path. Never the reverse: a distance weld is not
// evidence of an analytic contact.
func (a *arranger) auditMergedEndpoints(si, sj *tinySeg) {
	events := a.events[pairKey(si.src, sj.src)]
	a.forEachMergedEnd(si, sj, func(e mergedEnd) {
		if a.eventExplains(events, e) {
			return
		}
		a.taintSampledVertex(si.src, e.ti)
		a.taintSampledVertex(sj.src, e.tj)
	})
}

// eventExplains reports whether some analytic event of the pair IS the vertex the two
// sample endpoints welded at — i.e. whether the event canonicalizes to that same graph
// vertex.
//
// The predicate mirrors vertexTable.canon, which decides identity by DISTANCE to an
// existing vertex's stored coordinates (<= a.merge), and stores the coordinates of
// whichever point reached the table first. The welded vertex is therefore located at
// ONE of the two endpoints — which one depends on insertion order, which is not known
// here — so an event canonicalizes to it only if it lies within a.merge of that
// representative. Since either endpoint may be the representative, requiring the event
// to be within a.merge of BOTH is the sound rule: it holds exactly when canon(event)
// would return that vertex whichever endpoint happens to represent it.
//
// A looser window (a multiple of a.merge, or a distance to the endpoints' midpoint)
// approximates canonicalization rather than mirroring it, and would let an unrelated
// analytic event merely NEAR the weld — but a separate graph vertex of its own —
// suppress the taint, leaving a bound that really came from a distance weld wearing
// exact:true. The failure of that direction is a false certification, so the predicate
// must be the canonicalization rule itself.
//
// An event's CONTACT POINTS are what must be tested, and for a resolvable
// coincident-carrier overlap those are the window's two BOUNDARY points, not e.x/e.y:
// the boundary points are where the two carriers exactly touch, and where
// resolveCoincidentOverlap cuts both sources when it takes the resolution, while
// e.x/e.y is only the window midpoint, a locator for the degeneracy flag and never a
// cut site. Testing the midpoint alone tainted the very cuts the resolution had just
// made exact, whenever an overlap boundary happened to weld onto a sample vertex.
func (a *arranger) eventExplains(events []xEvent, m mergedEnd) bool {
	for _, e := range events {
		for _, p := range eventContacts(e) {
			if math.Hypot(p[0]-m.xi, p[1]-m.yi) <= a.merge && math.Hypot(p[0]-m.xj, p[1]-m.yj) <= a.merge {
				return true
			}
		}
	}
	return false
}

// eventContacts returns the points an analytic event places a contact at: the
// event's own point, plus — for a resolvable coincident-carrier overlap — the two
// boundary points of its window (see resolveCoincidentOverlap). They are contacts of
// the geometry, so they answer the weld audit whether or not the resolution was
// ultimately taken.
func eventContacts(e xEvent) [][2]float64 {
	if e.overlap == nil {
		return [][2]float64{{e.x, e.y}}
	}
	return [][2]float64{
		{e.x, e.y},
		{e.overlap.loX, e.overlap.loY},
		{e.overlap.hiX, e.overlap.hiY},
	}
}

// mergedEnd is one weld: the tiny-segment endpoints of two DIFFERENT sources that the
// vertex table canonicalizes into a single graph vertex, with each side's natural
// source parameter (ti, tj) and its own coordinates (xi,yi / xj,yj). The two
// coordinates are kept separate — not averaged — because vertex identity is decided by
// distance to one of them, never to their midpoint.
type mergedEnd struct {
	ti, tj         float64
	xi, yi, xj, yj float64
}

// forEachMergedEnd calls fn for every pair of tiny-segment endpoints — one from each
// of two DIFFERENT sources — that the vertex table would canonicalize into a single
// graph vertex (they lie within the merge tolerance).
func (a *arranger) forEachMergedEnd(si, sj *tinySeg, fn func(mergedEnd)) {
	type end struct{ x, y, t float64 }
	iEnds := [2]end{{si.ax, si.ay, si.pa}, {si.bx, si.by, si.pb}}
	jEnds := [2]end{{sj.ax, sj.ay, sj.pa}, {sj.bx, sj.by, sj.pb}}
	for _, ei := range iEnds {
		for _, ej := range jEnds {
			if math.Hypot(ei.x-ej.x, ei.y-ej.y) > a.merge {
				continue
			}
			fn(mergedEnd{ti: ei.t, tj: ej.t, xi: ei.x, yi: ei.y, xj: ej.x, yj: ej.y})
		}
	}
}

// taintSampledVertex records a contact that landed ON an existing sample vertex of
// source src (a tiny-segment boundary at source parameter t) rather than interior to
// a segment, and that NO closed-form event certifies — a sampled crossing, or a
// distance-weld the vertex table made on its own. No cut record is needed there (the
// vertex already exists), but the boundary parameter at that vertex must read as
// SAMPLED, not exact: a fragment bounded by it would otherwise report TExact,
// blessing an approximate range as certified.
//
// It says nothing about whether the source is split — that is read off each emitted
// fragment's own range in makeCycle, so a contact whose partner is later pruned away
// cannot leave a phantom Partial behind.
//
// A contact at the source's OWN endpoint is a join, not a split — the same rule
// applyAnalyticCut applies — so no marker is pushed there: a marker at an endpoint
// would AND away that bound's source-end PROVENANCE (cut.srcEnd) in split's dedup and
// falsely demote a whole curve to a fragment. Endpoint EXACTNESS is a separate
// question, and it is NOT decided here: split audits every bound — endpoints included
// — against the vertex it actually canonicalized to (vertexCertifies), which is the only
// place that can see a weld chained through a third vertex. So an endpoint dragged
// onto another curve's vertex loses its exactness there while keeping its provenance,
// and one this pass cannot even see (canon is not transitive) is caught all the same.
func (a *arranger) taintSampledVertex(src int, t float64) {
	s := &a.sources[src]
	if atSourceEnd(s, t) {
		return
	}
	a.taintSegBoundary(src, t)
	// A closed source's seam is a single vertex reachable as both t≈0 and t≈1, so
	// taint both incident segments: a fragment ending there from either side must
	// read as sampled.
	if s.closed {
		switch {
		case t <= segEps:
			a.taintSegBoundary(src, 1)
		case t >= 1-segEps:
			a.taintSegBoundary(src, 0)
		}
	}
}

// taintSegBoundary marks the sample vertex at source parameter t inexact on every
// tiny segment of src incident to it (the two segments meeting there, or the one at
// a closed source's seam end).
//
// The marker is a cut at that segment's own boundary (local param exactly 0 or 1)
// carrying the SEGMENT ENDPOINT's coordinates, so split's dedup collapses it into
// that endpoint: no new vertex, no extra or zero-length edge, identical edge
// parameters. The only thing that moves is the ANDed exactness of the surviving
// boundary.
func (a *arranger) taintSegBoundary(src int, t float64) {
	for _, si := range a.sourceSegs[src] {
		s := &a.segs[si]
		lo, hi := s.pa, s.pb
		if hi < lo {
			lo, hi = hi, lo
		}
		if t < lo-segEps || t > hi+segEps {
			continue
		}
		switch local := (t - s.pa) / (s.pb - s.pa); {
		case local <= segEps:
			s.cuts = append(s.cuts, cut{t: 0, px: s.ax, py: s.ay, exact: false})
		case local >= 1-segEps:
			s.cuts = append(s.cuts, cut{t: 1, px: s.bx, py: s.by, exact: false})
		}
	}
}

// atSourceEnd reports whether a natural source parameter is at a curve endpoint
// (t≈0 or t≈1). A closed source (circle, ellipse, closed spline) has no endpoint —
// its seam (t≈0/1) is a topologically interior point — so a crossing there still
// splits it.
func atSourceEnd(s *source, t float64) bool {
	if s.closed {
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

// atDomainEnd reports whether a SAMPLE parameter — a tiny segment's own endpoint,
// never a cut's — sits at the source's domain boundary: an open curve's endpoint, or
// a closed curve's seam (which is a domain end of the PARAMETERIZATION even though it
// is topologically interior, so a fragment bounded by it on both sides still covers
// the whole curve). Sample params are the exact fractions i/n (see sampleParams), so
// only the first and last can be within an epsilon of 0/1 — this is a structural test
// on the sampling grid, NOT a numeric tolerance applied to a crossing parameter.
func atDomainEnd(t float64) bool {
	return t <= segEps || t >= 1-segEps
}

// param maps a tiny segment's local chord parameter to its source's natural
// parameter. At the segment's own endpoints the answer IS the sample param, returned
// verbatim: interpolating there would only add rounding, and a whole curve's reported
// range must be its exact domain [0,1], not [0, 1-1ulp].
func (s *tinySeg) param(t float64) float64 {
	switch t {
	case 0:
		return s.pa
	case 1:
		return s.pb
	}
	return s.pa + t*(s.pb-s.pa)
}

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

// splitFrag is one fragment split built before any suppression decision: the tiny
// segment it came from, its two deduped boundaries, and the canonical vertices those
// boundaries landed on.
//
// Building every fragment first is what lets certifySuppression judge a suppression
// window against the outcome instead of predicting it. Canonicalization is
// unconditional — a boundary is welded into the vertex table whether or not its
// fragment is ultimately emitted — so the vertex table, and every vertex identity the
// certification reasons about, is exactly what it would be with no suppression at all.
type splitFrag struct {
	seg    int
	b0, b1 cut
	u, v   int
}

// split cuts each tiny segment at its crossing parameters and emits the final
// arrangement edges between canonical vertices, minus the fragments a still-standing
// coincident-carrier suppression window covers.
func (a *arranger) split() {
	frags := a.splitFragments()
	a.certifySuppression(frags)
	for _, f := range frags {
		s := &a.segs[f.seg]
		// A fragment inside a resolved coincident-carrier overlap's suppression
		// window belongs to the LOSING source (see resolveCoincidentOverlap): the
		// NAMED source's own edge over the identical span already covers it, so
		// this one is omitted rather than emitted as a duplicate boundary. The two
		// boundary points are canonicalized regardless (both sources were cut at the
		// SAME exact event point), so the named source's edge still has valid
		// endpoints to attach to. The fragment is classified by the source evaluated
		// at its PARAMETER midpoint — never its chord midpoint, which for a
		// half-circle fragment is the carrier centre itself (see fragmentSuppressed)
		// — and testing that ONE point answers for the whole fragment because
		// certifySuppression has already confirmed both window boundaries are
		// distinct shared fragment bounds, so no emitted fragment straddles one (see
		// angularWindow.contains, which is why the window is tested with no slop).
		if a.fragmentSuppressed(s.src, s.param(f.b0.t), s.param(f.b1.t)) {
			continue
		}
		// Exactness is decided HERE, against the vertex the boundary actually
		// canonicalized to — see vertexCertifies. A bound whose graph vertex sits
		// somewhere else, with nothing exact to explain the move, cannot carry an
		// exact parameter, whatever its own record says. Provenance (srcEnd) is NOT
		// audited: it is a fact about the source's parameterization ("this bound IS
		// the curve's domain end"), which a weld does not change, and Whole must not
		// be lost to one.
		//
		// Two withdrawals, both applied to every bound of this fragment. The SCENE gate
		// (exactAllowed) withholds exactness from the whole arrangement when any source
		// is free-form; the per-source one (exactRefused) withholds it from a component
		// whose map may be fused (refuseExactOnFusedMap). Either is applied wholesale,
		// cut records and vertex identity notwithstanding: the bounds are individually
		// right about their own curve and may collectively describe a topology that is
		// missing a crossing. Neither changes what is emitted — only whether its
		// parameter is published as exact.
		refused := !a.exactAllowed || (a.exactRefused != nil && a.exactRefused[s.src])
		a.edges = append(a.edges, arrEdge{u: f.u, v: f.v, src: s.src,
			pu: s.param(f.b0.t), pv: s.param(f.b1.t),
			exactU: !refused && f.b0.exact && a.vertexCertifies(&a.sources[s.src], f.u, f.b0.px, f.b0.py),
			exactV: !refused && f.b1.exact && a.vertexCertifies(&a.sources[s.src], f.v, f.b1.px, f.b1.py),
			endU:   f.b0.srcEnd, endV: f.b1.srcEnd})
	}
}

// splitFragments dedups each tiny segment's boundaries and canonicalizes every
// surviving fragment's two bounds into graph vertices, returning the fragments split
// would emit if nothing were suppressed.
func (a *arranger) splitFragments() []splitFrag {
	var frags []splitFrag
	for i := range a.segs {
		s := &a.segs[i]
		// Boundaries along the segment: the two endpoints (chord positions) plus
		// every cut, each carrying the EXACT point to canonicalize the vertex at.
		// A segment endpoint is exact only when evaluating its source at the endpoint's
		// reported parameter reproduces the emitted coordinate — the general form that
		// makes TExact's meaning ("eval(reported param) == emitted polyline endpoint")
		// hold BY CONSTRUCTION for every source, evaluated or pinned. For all but one
		// source densify stored the endpoint AS s.at(param), so the reproduction is
		// bit-exact; but an elliptical arc PINS its ends to their sketch Start/End
		// points, which sit off the parametric ellipse by solver tolerance, so
		// s.at(param) does NOT reproduce them — identity of the welded vertex to the
		// pinned coordinate would then pass vertexCertifies while the reported parameter
		// misses the endpoint, the round-8 false certification this test guards against.
		// The two endpoints also carry the source-end PROVENANCE (cut.srcEnd) when the
		// segment endpoint is the source's own domain end — the fact Whole is read from
		// (unchanged by this: Whole is topology, not parameter reproduction).
		src := &a.sources[s.src]
		bs := []cut{
			{t: 0, px: s.ax, py: s.ay, exact: a.endpointReproduces(src, s.param(0), s.ax, s.ay), srcEnd: atDomainEnd(s.pa)},
			{t: 1, px: s.bx, py: s.by, exact: a.endpointReproduces(src, s.param(1), s.bx, s.by), srcEnd: atDomainEnd(s.pb)},
		}
		bs = append(bs, s.cuts...)
		sort.Slice(bs, func(i, j int) bool { return bs[i].t < bs[j].t })
		// dedup near-equal local params (keep the first, which for an analytic cut at
		// a seg boundary keeps the endpoint's exact point). Exactness and source-end
		// provenance are ANDed into the survivor: a boundary coincident with a sampled
		// cut is only as trustworthy as that cut, and a domain end a cut lands on is a
		// cut — so the merge never launders inexact into exact, nor a cut into a
		// curve's own end.
		uniq := bs[:0:0]
		for _, b := range bs {
			if len(uniq) == 0 || b.t-uniq[len(uniq)-1].t > segEps {
				uniq = append(uniq, b)
				continue
			}
			last := &uniq[len(uniq)-1]
			last.exact = last.exact && b.exact
			last.srcEnd = last.srcEnd && b.srcEnd
		}
		for k := 1; k < len(uniq); k++ {
			b0, b1 := uniq[k-1], uniq[k]
			u := a.verts.canon(b0.px, b0.py)
			v := a.verts.canon(b1.px, b1.py)
			if u == v {
				continue // collapsed to a point
			}
			frags = append(frags, splitFrag{seg: i, b0: b0, b1: b1, u: u, v: v})
		}
	}
	return frags
}

// vertexCertifies reports whether the canonical vertex v that boundary point (px,py)
// of source src landed on is one the bound's parameter may be certified against: the
// vertex must BE that point (coordinate identity). Exactness means the reported
// parameter reproduces the emitted geometry — and the bound's own point IS the
// evaluation of its reported parameter (split sets pu = s.param(b.t) alongside
// px,py = the point at b.t). So "the vertex equals (px,py)" is precisely "eval(the
// reported parameter) equals the emitted polyline endpoint", which is the definition
// of exact. Anything looser certifies the wrong question.
//
// This is the exactness audit against what the vertex table ACTUALLY did, and it is
// needed because vertexTable.canon is NOT transitive: it welds a point onto the first
// vertex within a.merge of it and keeps THAT vertex's coordinates, so two points
// farther apart than merge can still land on one vertex through a third one inserted
// first. No pairwise reasoning over the cuts — which endpoints are within merge of
// which, which analytic event explains which weld (eventExplains) — can see that
// chain; only the vertex table knows where the vertex ended up. So the last word on
// exactness is taken here, after canonicalization: an unexplained move of the vertex
// away from the bound's own point means the reported parameter does not describe the
// emitted geometry, and the bound is not exact. This is also what covers the one bound
// the taint passes deliberately leave alone — a source's OWN endpoint, which is never
// cut (an end-to-end contact is a join, not a split) yet can still be dragged onto
// another curve's vertex by a weld, chained or not.
//
// It is NOT enough for the vertex to sit at an analytic CONTACT of src: a sample-param
// bound (its parameter a sample fraction i/n) whose sample point welds onto a nearby
// analytic contact reports the SAMPLE parameter, which evaluates back to the sample
// point — not to the contact the vertex moved to. The vertex being a certified contact
// says nothing about whether the bound's OWN parameter reproduces it, so the only sound
// test is identity between the vertex and the bound's own point. (For a genuine analytic
// CUT the two coincide anyway: the cut's point IS the contact, so identity holds by
// construction and needs no separate contact list.)
//
// Identity is tested at round-off, NOT at the merge tolerance: a genuine shared
// endpoint (the only way this engine expresses topology — two curves holding the same
// Point) reaches the arrangement through each curve's own evaluation of that
// coordinate, so the two agree to within evaluation round-off (a line reproduces the
// point exactly; an arc/ellipse/spline rebuilds it through trig or basis functions, a
// few ulps out). weldIdentEps·scale is ~1e4 ulps of the scene — comfortably above that
// round-off, and five orders of magnitude BELOW the default merge (1e-7·scale), so no
// distance weld (a gap the caller's TOLERANCE forgave, not a coincidence) can pass as
// identity. A weld tighter than round-off is identity.
//
// The band is bounded by TWO yardsticks at once — the scene's extent and the SOURCE's
// own — the same two-band shape carriersIdentical uses for a coincident carrier and for
// the same reason. a.scale is the whole scene's bounding-box extent, so on the scene
// band alone an object drawn somewhere else widens what counts as identity here: a
// circle of radius 5 whose crossing welds 1e-9 away from a sample vertex correctly
// reports that bound inexact when drawn with its chord alone, and ONE unrelated line
// parked at x=1000 flips it to exact — reachable through Sketch.Profiles() with no
// options at all, publishing a parameter that misses the emitted polyline endpoint by
// the whole 1e-9. Nothing about the source changed; only the scene did. The gap is a
// displacement along the curve whose parameter is being certified, so the curve's own
// size is the length it has to be judged against, and no distant object can inflate it
// (TestExactBoundIdentityBandIsSourceLocal).
func (a *arranger) vertexCertifies(src *source, v int, px, py float64) bool {
	vx, vy := a.verts.coord(v)
	gap := math.Hypot(vx-px, vy-py)
	return gap <= weldIdentEps*a.scale && gap <= weldIdentEps*src.extent
}

// endpointReproduces reports whether evaluating source src at parameter p reproduces
// the emitted polyline coordinate (x,y) within the identity band. It is the exactness
// test for a tiny segment's synthetic endpoint bound: TExact certifies that eval(the
// reported parameter) equals the emitted polyline endpoint, so a bound may only be
// exact when the source's own evaluation at its reported parameter lands back on the
// coordinate densify actually emitted. For an evaluated endpoint (densify stored
// s.at(p) verbatim) this is bit-exact; for a PINNED endpoint — an elliptical arc's
// Start/End are pinned to sketch points off the parametric ellipse — s.at(p) misses
// the pinned coordinate, so the bound is correctly inexact. The band is the same
// round-off identity band vertexCertifies uses (five orders below the merge tolerance),
// so a tolerance gap can never pass as exact reproduction — including its SOURCE-LOCAL
// half, which this test needs for the same reason and measures the same way: the
// quantity here is how far this source's own evaluation lands from this source's own
// emitted endpoint, so geometry drawn elsewhere must not decide whether that miss is
// round-off (TestEndpointReproductionBandIsSourceLocal).
func (a *arranger) endpointReproduces(src *source, p, x, y float64) bool {
	q := src.at(p)
	gap := math.Hypot(q[0]-x, q[1]-y)
	return gap <= weldIdentEps*a.scale && gap <= weldIdentEps*src.extent
}

// weldIdentEps is the round-off band, as a fraction of a length, within which two
// coordinates are the SAME point rather than two points a tolerance welded together.
// Every user states it against TWO lengths and requires both: the scene's scale, and a
// length local to the geometry the decision is about — the source's own extent
// (vertexCertifies, endpointReproduces), the vertex's chord (contactIsVertex), the
// carriers' radii (carriersIdentical). The scene band alone lets an unrelated distant
// object widen a verdict about geometry it has nothing to do with.
const weldIdentEps = 1e-12

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
			a.flagDegenerate(vx, vy, a.edges[hi.edge].src, a.edges[hj.edge].src)
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
// internalCurvedTangency reports whether sources i and j are two circle/arc
// carriers in INTERNAL tangency — one inside the other (centre distance below the
// larger radius), touching at one point (d = |R−r|). The inner is a hole of the
// outer, resolved by exact containment in hole assignment.
func (a *arranger) internalCurvedTangency(i, j int) bool {
	si, sj := &a.sources[i], &a.sources[j]
	if !isCurvedKind(si.kind) || !isCurvedKind(sj.kind) {
		return false
	}
	d := math.Hypot(si.cx-sj.cx, si.cy-sj.cy)
	return d < math.Max(si.r, sj.r)
}

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

	arr := &Arrangement{SelfIntersections: a.selfX, Degenerate: a.degenSet}
	for _, d := range a.degen {
		arr.Degeneracies = append(arr.Degeneracies, [2]float64{d.x, d.y})
	}
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
			if !a.exactPointInRegion(probe, f) {
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
		reg.Degenerate = a.regionDegenerate(reg)
		arr.Regions = append(arr.Regions, reg)
	}
	return arr
}

// regionDegenerate reports whether any recorded degenerate condition reaches this
// region: one involving a curve the region's own boundary is built from, or one
// that could not be attributed to any curve at all.
//
// Attribution is by SOURCE, not by where the condition's representative point
// landed. The point is only a locator — several are a midpoint between two sources
// rather than the contact itself — while the curve identity is exact, and a
// condition can only corrupt the faces its curves bound. A region built from
// entirely different curves is unaffected, which is why a spur touching one circle
// no longer invalidates a second circle across the sketch.
//
// The arrangement-wide [Arrangement.Degenerate] is unchanged: a condition that
// produces no region at all (or destroys one) still has to be reported somewhere,
// so a consumer gating on trustworthiness reads that, and this only refines WHICH
// regions are implicated.
func (a *arranger) regionDegenerate(reg *Region) bool {
	if len(a.degen) == 0 {
		return false
	}
	srcs := map[int]struct{}{}
	for _, e := range reg.Outer {
		srcs[e.SourceIndex] = struct{}{}
	}
	for _, h := range reg.Holes {
		for _, e := range h {
			srcs[e.SourceIndex] = struct{}{}
		}
	}
	for _, d := range a.degen {
		if len(d.srcs) == 0 {
			return true // unattributable: every region carries it
		}
		for _, s := range d.srcs {
			if _, ok := srcs[s]; ok {
				return true
			}
		}
	}
	return false
}

// cycle is one next-cycle: its coalesced boundary edges, dense polygon, signed
// area, and whether any contributing source self-intersects.
type cycle struct {
	boundary []BoundaryEdge
	dense    [][2]float64
	frags    []cycFrag // source + natural-param range of each boundary fragment
	area     float64
	selfX    bool
}

// cycFrag is one boundary fragment of a cycle: its source and the natural-param
// range it spans (pStart→pEnd, reversed if pEnd<pStart). It carries the exact
// geometry needed for the exact point-in-region (winding/ray-cast) containment
// test, independent of how finely the fragment was densified.
type cycFrag struct {
	src          int
	pStart, pEnd float64
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
		// exactStart/exactEnd track the trustworthiness of pStart/pEnd, and
		// endStart/endEnd their source-end provenance. Only the fragment's two OUTER
		// bounds matter: an interior boundary coalesced away is not reported, so
		// neither its exactness nor its provenance is folded in.
		exactStart, exactEnd bool
		endStart, endEnd     bool
	}
	var frags []frag
	for _, hi := range hs {
		h := a.halfs[hi]
		e := a.edges[h.edge]
		var pStart, pEnd float64
		var exStart, exEnd bool
		var enStart, enEnd bool
		if h.forward {
			pStart, pEnd = e.pu, e.pv
			exStart, exEnd = e.exactU, e.exactV
			enStart, enEnd = e.endU, e.endV
		} else {
			pStart, pEnd = e.pv, e.pu
			exStart, exEnd = e.exactV, e.exactU
			enStart, enEnd = e.endV, e.endU
		}
		fx, fy := a.verts.coord(h.from)
		tx, ty := a.verts.coord(h.to)
		if n := len(frags); n > 0 && frags[n-1].src == e.src && approx(frags[n-1].pEnd, pStart, 1e-9) {
			frags[n-1].pEnd = pEnd
			frags[n-1].exactEnd = exEnd
			frags[n-1].endEnd = enEnd
			frags[n-1].dense = append(frags[n-1].dense, [2]float64{tx, ty})
		} else {
			frags = append(frags, frag{src: e.src, pStart: pStart, pEnd: pEnd,
				exactStart: exStart, exactEnd: exEnd,
				endStart: enStart, endEnd: enEnd,
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
		frags[n-1].exactEnd = frags[0].exactEnd
		frags[n-1].endEnd = frags[0].endEnd
		frags[n-1].dense = append(frags[n-1].dense, frags[0].dense[1:]...)
		frags = frags[1:]
	}

	chord := make([][2]float64, 0, len(frags))
	var bulge float64
	for _, f := range frags {
		s := &a.sources[f.src]
		reversed := f.pEnd < f.pStart
		// TStart/TEnd are reported in the source's NATURAL parameter direction, so
		// TStart < TEnd always; Reversed (above) is what says the walk traverses the
		// fragment backwards. Both bounds must be trustworthy for TExact.
		tStart, tEnd := f.pStart, f.pEnd
		exStart, exEnd := f.exactStart, f.exactEnd
		if reversed {
			tStart, tEnd = tEnd, tStart
			exStart, exEnd = exEnd, exStart
		}
		// Whole is read off the fragment's OWN surviving bounds — the one thing that
		// is actually true of the edge being emitted — not off a per-source "was it cut
		// anywhere" flag (which outlives pruning and reports a phantom fragment on a
		// whole curve), and NOT off a numeric comparison of the range against [0,1]
		// (which cannot tell a bound that IS the curve's end from a crossing that
		// landed 1e-10 away from it, and so would bless a sampled-bounded fragment as
		// the whole curve — the unsafe direction).
		//
		// Instead each bound carries its PROVENANCE (cut.srcEnd → arrEdge.endU/endV):
		// it is either the source curve's own domain end, or a cut/weld. The edge is
		// the whole curve exactly when BOTH of its bounds are the curve's own ends.
		// Deciding here — after pruning and after the coalescing above — is what makes
		// that agree with the emitted geometry: a contact whose partner was pruned
		// away, or a split vertex the walk runs straight through, leaves a degree-2
		// vertex the fragments coalesce back across, so the curve's own ends are the
		// surviving bounds again and it correctly reads whole. A CLOSED source cut once
		// coalesces the same way (the walk leaves the contact and returns to it), and
		// the surviving bounds are its seam — the curve's own domain ends — so it too
		// reads whole. The lone conservative corner is a closed source whose single cut
		// lands ON the seam: both bounds are then cuts, so it reads as a fragment
		// spanning [0,1]. That errs toward Partial (a consumer re-derives the same
		// curve from the range either way) and never toward a false Whole, which is the
		// only direction that can mislead.
		whole := f.endStart && f.endEnd
		c.boundary = append(c.boundary, BoundaryEdge{
			SourceIndex: f.src, Whole: whole, Reversed: reversed, Polyline: f.dense,
			TStart: tStart, TEnd: tEnd, TExact: exStart && exEnd,
		})
		c.frags = append(c.frags, cycFrag{src: f.src, pStart: f.pStart, pEnd: f.pEnd})
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
		case srcConic:
			// Exact, sampling-independent area between the conic fragment
			// (parameters f.pStart→f.pEnd in walk order) and its chord. The closed
			// form is signed by the parameter order, exactly like the arc/ellipse
			// cases (a reversed fragment has pEnd < pStart, flipping the sign).
			bulge += conicBulgeSpan(s.conStart, s.conApex, s.conEnd, s.conW, f.pStart, f.pEnd)
		case srcSpline, srcClosedSpline, srcFitSpline:
			a0 := f.dense[0]
			a1 := f.dense[len(f.dense)-1]
			bulge += s.splineBulge(f.pStart, f.pEnd, a0[0], a0[1], a1[0], a1[1])
		case srcNURBS:
			// Exact (non-rational) / numerically exact (rational) area between the
			// NURBS fragment and its chord, integrated on the true curve.
			a0 := f.dense[0]
			a1 := f.dense[len(f.dense)-1]
			bulge += nurbsBulgeSpan(s.nurbs, f.pStart, f.pEnd, a0[0], a0[1], a1[0], a1[1])
		}
	}
	c.area = signedPolyArea(chord) + bulge
	return c
}

// exactPointInRegion reports whether q is inside the cycle, by ray-casting a
// horizontal +x ray and counting EXACT crossings with each boundary fragment
// (closed-form for line/circle/arc). This is immune to the chord poke-out that
// defeats the sampled pointInPolygon at a tangency (the inner circle's chord
// polygon dips outside the outer's near the contact). Falls back to the chord
// polygon when any boundary fragment is an ellipse/spline (no closed-form ray
// crossing here, and those are not the poke-out case).
func (a *arranger) exactPointInRegion(q [2]float64, c *cycle) bool {
	for _, f := range c.frags {
		switch a.sources[f.src].kind {
		case srcLine, srcCircle, srcArc:
		default:
			return pointInPolygon(q, c.dense)
		}
	}
	// Perturb the probe by a tiny GENERIC offset — far above the float-rounding floor
	// (~ULP·scale ≈ scale·1e-16) yet far below any real feature (and below the
	// strictly-interior margin of the hole the probe came from). This stops the
	// horizontal ray from crossing a circle exactly at its param seam (angle 0),
	// where the seam param rounds to the fragment-endpoint boundary and the half-open
	// test drops the crossing — the gap that double-counted a hole near angle 0. The
	// offset's irrational ratio avoids re-aligning with another source's seam.
	q = [2]float64{q[0] + a.scale*4.131e-9, q[1] + a.scale*9.073e-9}
	crossings := 0
	for _, f := range c.frags {
		crossings += a.rayFragCrossings(q, f)
	}
	return crossings%2 == 1
}

// rayFragCrossings counts how many times the horizontal +x ray from q crosses the
// line/circle/arc fragment f. Endpoints use a half-open convention (the lower-y
// endpoint counts, the upper-y one does not) so a ray through a shared vertex is
// counted once; a ray grazing a circle tangentially (zero discriminant) counts 0.
func (a *arranger) rayFragCrossings(q [2]float64, f cycFrag) int {
	s := &a.sources[f.src]
	if s.kind == srcLine {
		A := s.at(f.pStart)
		B := s.at(f.pEnd)
		return raySegCrossings(q, A, B)
	}
	// circle / arc: the ray's carrier line y=q.y meets the circle at
	// x = cx ± sqrt(r² − (q.y−cy)²); keep roots to the right of q within the sweep.
	dy := q[1] - s.cy
	disc := s.r*s.r - dy*dy
	if disc <= 0 {
		return 0 // miss or tangential graze (no net crossing)
	}
	sq := math.Sqrt(disc)
	count := 0
	for _, x := range [2]float64{s.cx - sq, s.cx + sq} {
		if x <= q[0] {
			continue
		}
		ang := math.Atan2(dy, x-s.cx)
		if a.angInFragment(s, f, ang) {
			count++
		}
	}
	return count
}

// raySegCrossings is the standard half-open horizontal-ray vs segment test.
func raySegCrossings(q, A, B [2]float64) int {
	if (A[1] > q[1]) == (B[1] > q[1]) {
		return 0
	}
	x := A[0] + (q[1]-A[1])/(B[1]-A[1])*(B[0]-A[0])
	if x > q[0] {
		return 1
	}
	return 0
}

// angInFragment reports whether circle/arc angle ang lies on the fragment's swept
// natural-param range [pStart,pEnd] (in either direction). Uses a half-open
// convention at the param endpoints so a ray through a fragment boundary vertex is
// counted by exactly one of the two adjoining fragments.
func (a *arranger) angInFragment(s *source, f cycFrag, ang float64) bool {
	// natural param of this angle on the source
	var t float64
	if s.kind == srcCircle {
		t = ang / (2 * math.Pi)
	} else { // srcArc
		if s.sweep == 0 {
			return false
		}
		t = (ang - s.phi0) / s.sweep
	}
	lo, hi := f.pStart, f.pEnd
	if hi < lo {
		lo, hi = hi, lo
	}
	// A whole, uncut circle (the fragment spans the full period) has no endpoint —
	// every angle is on it.
	if s.kind == srcCircle && hi-lo >= 1-1e-9 {
		return true
	}
	// bring t into [lo, lo+1) by whole turns (param period is 1), then test [lo,hi).
	// The caller perturbs the probe generically so a ray never crosses exactly at a
	// fragment endpoint (the seam), keeping this half-open test off the float boundary.
	t -= math.Floor((t - lo))
	return t >= lo && t < hi
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

// conicBulge returns the exact signed area between a conic (rational quadratic
// Bézier with control points start/apex/end and apex weight w = rho/(1−rho)) and
// its chord Start→End, over the whole curve t ∈ [0, 1] — the conic analog of
// chordArcCorrection / chordEllipseCorrection. With Start at the origin, a =
// Apex−Start, b = End−Start, the bulge is w·(a×b)·∫₀¹ t²/W(t)² dt with W = 2c·t²
// − 2c·t + 1, c = 1−w; the rational integral has a closed form. At w = 1 (the
// parabola) this is (a×b)/3, the known quadratic-Bézier result. Verified against
// fine numerical integration to ~1e-13 across the ellipse/parabola/hyperbola
// range. It is exact and sampling-independent.
func conicBulge(start, apex, end [2]float64, w float64) float64 {
	return conicBulgeSpan(start, apex, end, w, 0, 1)
}

// conicBulgeSpan returns the exact signed area between a conic FRAGMENT — the
// curve restricted to parameters [t0, t1] in walk order — and the straight chord
// closing it, the conic analog of the per-fragment arc/ellipse/spline bulge so a
// conic split by a crossing still contributes exact area. The whole-curve
// conicBulge is the [0, 1] case. With a = apex−start, b = end−start, the moment
// swept FROM start over the fragment is w·(a×b)·∫_{t0}^{t1} t²/W(t)² dt, W =
// 2c·t² − 2c·t + 1, c = 1−w, via the closed-form antiderivative of t²/W²
// (substitution u = t − ½, W = α·u² + k, α = 2c, k = (1+w)/2; the α→0 parabola
// branch uses the polynomial antiderivative, avoiding the 1/α singularity).
// That moment is the area between the arc and the two radii start→P(t0)→P(t1);
// subtracting the triangle (start, P(t0), P(t1)) leaves the area between the arc
// and ITS chord P(t0)→P(t1) — the correction makeCycle adds to
// signedPolyArea(chord). For the whole curve t0 = 0 ⇒ P(t0) = start ⇒ the
// triangle vanishes. Both terms are antisymmetric in (t0, t1), so a reversed
// fragment flips sign, exactly like the other curved cases.
func conicBulgeSpan(start, apex, end [2]float64, w, t0, t1 float64) float64 {
	ax, ay := apex[0]-start[0], apex[1]-start[1]
	bx, by := end[0]-start[0], end[1]-start[1]
	crossAB := ax*by - ay*bx
	c := 1 - w
	k := (1 + w) / 2
	alpha := 2 * c
	i := conicMomentIndef(t1-0.5, alpha, k) - conicMomentIndef(t0-0.5, alpha, k)
	moment := w * crossAB * i
	p0 := conicEvalRaw(start, apex, end, w, t0)
	p1 := conicEvalRaw(start, apex, end, w, t1)
	tri := ((p0[0]-start[0])*(p1[1]-start[1]) - (p1[0]-start[0])*(p0[1]-start[1])) / 2
	return moment - tri
}

// conicEvalRaw evaluates the rational quadratic Bézier (control points
// start/apex/end, apex weight w) at parameter t — the same map as
// source.conicPoint, on bare coordinates for the area helpers.
func conicEvalRaw(start, apex, end [2]float64, w, t float64) [2]float64 {
	u := 1 - t
	b0, b1, b2 := u*u, 2*u*t*w, t*t
	den := b0 + b1 + b2
	return [2]float64{
		(b0*start[0] + b1*apex[0] + b2*end[0]) / den,
		(b0*start[1] + b1*apex[1] + b2*end[1]) / den,
	}
}

// conicMomentIndef is the antiderivative, evaluated at u = t − ½, of
// (u² + u + ¼)/(α·u² + k)² — i.e. of t²/W(t)² in the shifted variable. It sums
// the three standard moment antiderivatives J2 + J1 + ¼·J0 of 1/(α·u²+k)². For
// |α| ≈ 0 (parabola) it returns the exact polynomial antiderivative
// (u³/3 + u²/2 + u/4)/k², the removable-singularity limit.
func conicMomentIndef(u, alpha, k float64) float64 {
	if math.Abs(alpha) < 1e-9 {
		return (u*u*u/3 + u*u/2 + u/4) / (k * k)
	}
	den := alpha*u*u + k
	f := conicF(u, alpha, k)             // ∫ du/(α·u²+k)
	j0 := u/(2*k*den) + f/(2*k)          // ∫ du/(α·u²+k)²
	j1 := -1 / (2 * alpha * den)         // ∫ u du/(α·u²+k)²
	j2 := -u/(2*alpha*den) + f/(2*alpha) // ∫ u² du/(α·u²+k)²
	return j2 + j1 + 0.25*j0
}

// conicF is the antiderivative ∫ du/(α·u² + k): atan for α > 0 (rho < ½, the
// ellipse arc), atanh for α < 0 (rho > ½, the hyperbola arc). k = (1+w)/2 > 0
// always, so the radicands are well-defined on their respective branches. The
// α ≈ 0 (parabola) case never reaches here — conicMomentIndef short-circuits it.
func conicF(u, alpha, k float64) float64 {
	if alpha > 0 {
		s := math.Sqrt(alpha * k)
		return math.Atan(u*math.Sqrt(alpha/k)) / s
	}
	s := math.Sqrt(-alpha * k)
	return math.Atanh(u*math.Sqrt(-alpha/k)) / s
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
	return s.curveMoment(pStart, pEnd) + chordClosure(ax, ay, ex, ey)
}

// curveMoment returns the exact ½∫(x·y′ − y·x′) dt of a spline source over the
// natural-parameter interval pStart→pEnd (signed by direction). A cubic spline is
// piecewise cubic, so the integrand is a degree-5 polynomial on each knot span
// and 3-point Gauss–Legendre integrates it exactly; the interval is split at
// every interior breakpoint so no panel straddles a span boundary (where the
// piecewise polynomial changes and the quadrature would no longer be exact).
func (s *source) curveMoment(pStart, pEnd float64) float64 {
	// Every interior knot strictly inside (lo,hi) must become a panel boundary, or
	// a panel would straddle a span boundary (where the piecewise polynomial
	// changes) and the per-span Gauss–Legendre would no longer be exact. A
	// breakpoint coinciding with lo/hi is the boundary already, and an extra split
	// that produces a tiny panel is harmless (the integrand is still a single
	// polynomial there), but dropping a real knot is not. splineBreaks returns the
	// knots in ascending order; momentOverBreaks splits strictly inside (lo,hi).
	return momentOverBreaks(pStart, pEnd, s.splineBreaks(), s.gaussMoment)
}

// gauss3 holds the 3-point Gauss–Legendre rule on [-1,1]; exact for polynomials
// up to degree 5 (the degree of a cubic spline's area integrand).
var gauss3 = gaussRule{
	nodes:   []float64{-0.7745966692414834, 0, 0.7745966692414834}, // ±√(3/5), 0
	weights: []float64{5.0 / 9, 8.0 / 9, 5.0 / 9},
}

// gaussMoment integrates ½(x·y′ − y·x′) over a single panel [t0,t1] that lies
// within one polynomial span, by 3-point Gauss–Legendre (exact there).
func (s *source) gaussMoment(t0, t1 float64) float64 {
	return gaussPanel(gauss3, t0, t1, func(t float64) float64 {
		p := s.at(t)
		d := s.derivAt(t)
		return 0.5 * (p[0]*d[1] - p[1]*d[0])
	})
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
		dx, dy, _ := EvalCubicBSplineDeriv(s.ctrl, t)
		return [2]float64{dx, dy}
	case srcClosedSpline:
		dx, dy, _ := EvalPeriodicCubicBSplineDeriv(s.ctrl, t)
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
