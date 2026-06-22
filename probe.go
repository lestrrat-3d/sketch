package sketch

import (
	"errors"
	"fmt"
	"math"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/lestrrat-go/option/v3"
)

// ErrUnderconstrained is returned (wrapped) by [Sketch.ProbeConfigurations]
// when the sketch has remaining degrees of freedom: an under-constrained
// sketch admits a continuum of configurations, so probing for discrete
// alternatives is meaningless. Fully constrain the sketch (DOF 0) first.
var ErrUnderconstrained = errors.New("sketch: ambiguity probe requires a fully constrained sketch")

// separationTol decides when two converged configurations are the same
// solution. Distances are relative (coordinates and radii to the bounding-box
// diagonal, angles to π). A converged solve (residual ≤ 1e-10) leaves
// coordinate noise below ~1e-8 relative, while genuinely distinct branches
// are separated at feature scale (O(1) relative — a flipped apex moves by the
// size of the triangle). 1e-6 sits far above the noise and far below any real
// branch gap.
const separationTol = 1e-6

// ProbeOption tunes [Sketch.ProbeConfigurations]. Construct values with the
// With… helpers; any option left unset falls back to a sensible default.
type ProbeOption interface {
	option.Interface
	probeOption()
}

type probeOption struct{ option.Interface }

func (probeOption) probeOption() {}

type (
	identRestarts struct{}
	identSeed     struct{}
)

// WithRestarts sets the number of pseudo-random restarts performed in addition
// to the structured (mirror/flip) probes. Zero is legal — structured probes
// only. More restarts search more basins at the cost of one solve each.
func WithRestarts(n int) ProbeOption { return probeOption{option.New(identRestarts{}, n)} }

// WithSeed selects the deterministic pseudo-random stream used for the
// restarts. The probe is fully deterministic for a given sketch state and
// option set; vary the seed to explore differently, e.g. when hunting for a
// suspected additional configuration.
func WithSeed(v uint64) ProbeOption { return probeOption{option.New(identSeed{}, v)} }

// probeConfig holds the resolved probe options.
type probeConfig struct {
	restarts int
	seed     uint64
}

func defaultProbeConfig() probeConfig {
	return probeConfig{restarts: 12, seed: 1}
}

// Configuration is one converged configuration found by
// [Sketch.ProbeConfigurations]: a snapshot of the sketch's variable vector at
// which every constraint holds. It stays valid until geometry is added to or
// removed from the sketch.
type Configuration struct {
	s    *Sketch
	vars []float64
}

// PointXY reports p's coordinates in this configuration without touching the
// sketch.
func (c *Configuration) PointXY(p *Point) (float64, float64) {
	return c.vars[p.xi], c.vars[p.yi]
}

// Apply writes this configuration's values into the sketch, like a batch of
// [Point.MoveTo] seeds. The configuration is already converged, so the sketch
// is left at a valid solved state; call [Sketch.Solve] afterwards if you also
// want driven dimensions refreshed. Typical use: Apply each configuration in
// turn and export via [Sketch.SVG] or [Sketch.PNG] to compare the
// alternatives visually.
//
// Only free variables are restored: fixed/grounded values (including locked
// reference geometry that may have been refreshed since the probe ran) are left
// as they are, so an old configuration can never revert reference coordinates.
func (c *Configuration) Apply() {
	for i := range c.vars {
		if i < len(c.s.fixed) && c.s.fixed[i] {
			continue
		}
		c.s.vars[i] = c.vars[i]
	}
}

// ProbeResult is the outcome of [Sketch.ProbeConfigurations].
type ProbeResult struct {
	// Configurations holds the distinct configurations found: the baseline
	// (the call-time configuration, converged) first, then any alternatives in
	// deterministic probe order. Its length is a lower bound on the true
	// number of configurations — the probe can miss basins, so a length of 1
	// is not proof of uniqueness.
	Configurations []*Configuration
}

// Ambiguous reports whether the probe found more than one configuration. True
// proves the sketch is configuration-ambiguous (pin the intended branch with a
// signed constraint — see "Orientation and sign conventions" in the package
// doc); false only means no alternative was found within the probe budget.
func (r *ProbeResult) Ambiguous() bool { return len(r.Configurations) > 1 }

// ProbeConfigurations searches for distinct configurations that satisfy every
// constraint of a fully constrained sketch, by re-solving from structured
// (mirror/flip) and pseudo-random perturbations of the current geometry. A
// sketch with zero remaining degrees of freedom can still admit several
// discrete configurations — mirror images, side flips, branch choices — and
// [Sketch.DOF], [Sketch.Diagnose] and [Sketch.CheckConstraint] are blind to
// that; this probe is the diagnostic for it.
//
// The probe is a falsifier, not a certifier: finding two or more
// configurations proves the sketch is ambiguous, but finding exactly one
// never proves uniqueness — only that no alternative was found within the
// probe budget. The search is fully deterministic for a given sketch state
// and option set.
//
// Like [Sketch.Diagnose], the analysis is local to the call-time configuration
// and dimension targets; call it after [Sketch.Solve]. The sketch is not
// mutated: its variables are restored on return regardless of outcome,
// parameter bindings are not re-evaluated, and driven dimensions are not
// refreshed. The only sanctioned way to adopt a found configuration is the
// explicit [Configuration.Apply].
//
// It returns [ErrNotConverged] if the sketch cannot be solved from its current
// state, and an error wrapping [ErrUnderconstrained] if degrees of freedom
// remain (a continuum of configurations has no discrete branches to probe).
func (s *Sketch) ProbeConfigurations(options ...ProbeOption) (*ProbeResult, error) {
	cfg := defaultProbeConfig()
	for _, opt := range options {
		switch opt.Ident().(type) {
		case identRestarts:
			cfg.restarts = option.MustGet[int](opt)
		case identSeed:
			cfg.seed = option.MustGet[uint64](opt)
		}
	}

	entry := append([]float64(nil), s.vars...)
	defer copy(s.vars, entry)

	free := s.freeVars()
	sc := defaultSolveConfig()

	// Baseline: solve from the current seed. The probe deliberately calls lm
	// directly rather than Solve so that parameter bindings are not
	// re-evaluated and refreshDriven never writes a probed configuration's
	// measurements into driven dimensions.
	s.lm(free, s.residuals, sc.maxIterations, sc.tolerance)
	r := s.residuals(nil)
	if math.Sqrt(dot(r, r)) > sc.tolerance {
		return nil, ErrNotConverged
	}

	// DOF at the baseline-converged point, computed fresh per the rank
	// invariant (never reuse an earlier Jacobian).
	dof := len(free)
	if m := len(r); m > 0 {
		dof = len(free) - s.rank(free, m)
	}
	if dof > 0 {
		return nil, fmt.Errorf("%w (DOF 0 needed, %d remaining)", ErrUnderconstrained, dof)
	}

	baseline := append([]float64(nil), s.vars...)
	result := &ProbeResult{Configurations: []*Configuration{{s: s, vars: baseline}}}

	// Perturbation scale and flip center come from the baseline bounding box.
	b, ok := s.bounds()
	diag := 1.0
	cx, cy := 0.0, 0.0
	if ok {
		cx, cy = (b.minX+b.maxX)/2, (b.minY+b.maxY)/2
		if h := math.Hypot(b.maxX-b.minX, b.maxY-b.minY); h > 1e-12 {
			diag = h
		}
	}
	kinds := s.varKinds()

	// try re-solves from one perturbation of the baseline and keeps the result
	// if it converged to a configuration not seen before. Acceptance order is
	// probe order, so the result is deterministic.
	try := func(perturb func()) {
		copy(s.vars, baseline)
		perturb()
		s.lm(free, s.residuals, sc.maxIterations, sc.tolerance)
		rr := s.residuals(nil)
		if math.Sqrt(dot(rr, rr)) > sc.tolerance {
			return
		}
		cand := append([]float64(nil), s.vars...)
		for _, c := range result.Configurations {
			if configSep(c.vars, cand, free, kinds, diag) < separationTol {
				return
			}
		}
		result.Configurations = append(result.Configurations, &Configuration{s: s, vars: cand})
	}

	// Structured probes, tier 1: reflect every free point across each
	// candidate mirror axis. Axes are the infinite lines through every line
	// entity and through every pair of fixed points — mirror branches reflect
	// across constraint-defined axes (a triangle apex flips across the line
	// through its fixed base points even when no line entity joins them; a
	// tangent circle's center flips across the tangent line). The axis list is
	// capped so pathological fixed-point counts cannot blow up the probe
	// budget.
	for _, axis := range s.probeAxes(baseline) {
		try(func() {
			for _, p := range s.points {
				m := geom.MirrorPoint(geom.NewPoint(baseline[p.xi], baseline[p.yi]), axis)
				if !s.fixed[p.xi] {
					s.vars[p.xi] = m.X
				}
				if !s.fixed[p.yi] {
					s.vars[p.yi] = m.Y
				}
			}
		})
	}

	// Structured probes, tier 2: global flips of the free point coordinates
	// about the bounding-box center — catches mirror branches not aligned with
	// any line entity.
	for _, f := range [][2]bool{{true, false}, {false, true}, {true, true}} {
		try(func() {
			for _, p := range s.points {
				if f[0] && !s.fixed[p.xi] {
					s.vars[p.xi] = 2*cx - baseline[p.xi]
				}
				if f[1] && !s.fixed[p.yi] {
					s.vars[p.yi] = 2*cy - baseline[p.yi]
				}
			}
		})
	}

	// Pseudo-random restarts: every free variable is offset from its baseline
	// value by a deterministic stream, with the amplitude cycling through a
	// quarter, a half and the whole bounding-box diagonal for multi-scale
	// basin coverage. Each restart perturbs from the baseline, never from the
	// previous restart, so every round is independent of acceptance history.
	amps := [...]float64{0.25, 0.5, 1.0}
	for k := 0; k < cfg.restarts; k++ {
		amp := amps[k%len(amps)] * diag
		try(func() {
			for _, vi := range free {
				u := probeUnit(cfg.seed, k, vi)
				switch kinds[vi] {
				case varAngle:
					// Angles wrap; offset within ±π rather than by length.
					s.vars[vi] = baseline[vi] + math.Pi*(2*u-1)
				case varRadius:
					// Keep radius seeds positive: a negative radius fights
					// norm()'s floor instead of exploring a basin.
					v := math.Abs(baseline[vi] + amp*(2*u-1))
					if v < 1e-9*diag {
						v = 1e-9 * diag
					}
					s.vars[vi] = v
				case varDimensionless:
					// A conic's rho lives in (0, 1); seed across that range rather
					// than by scene length, clamped away from the open bounds.
					v := 0.05 + 0.9*u
					s.vars[vi] = v
				default:
					s.vars[vi] = baseline[vi] + amp*(2*u-1)
				}
			}
		})
	}

	return result, nil
}

// probeMaxAxes caps the structured mirror-axis list: line entities first, then
// fixed-point pairs. The cap only matters for sketches with very many fixed
// points or lines, where the pseudo-random restarts carry the search instead.
const probeMaxAxes = 64

// probeAxes collects the candidate mirror axes for the structured probes at
// the baseline coordinates: the infinite line through every line entity, then
// through every pair of fixed points. Degenerate (zero-length) axes are
// skipped.
func (s *Sketch) probeAxes(baseline []float64) []*geom.Line {
	var axes []*geom.Line
	add := func(x1, y1, x2, y2 float64) bool {
		if len(axes) >= probeMaxAxes {
			return false
		}
		if math.Hypot(x2-x1, y2-y1) < 1e-9 {
			return true
		}
		axes = append(axes, geom.NewLine(geom.NewPoint(x1, y1), geom.NewPoint(x2, y2)))
		return true
	}
	for _, e := range s.ents {
		l, isLine := e.(*Line)
		if !isLine {
			continue
		}
		if !add(baseline[l.Start.xi], baseline[l.Start.yi], baseline[l.End.xi], baseline[l.End.yi]) {
			return axes
		}
	}
	var fixedPts []*Point
	for _, p := range s.points {
		if s.fixed[p.xi] && s.fixed[p.yi] {
			fixedPts = append(fixedPts, p)
		}
	}
	for i, p1 := range fixedPts {
		for _, p2 := range fixedPts[i+1:] {
			if !add(baseline[p1.xi], baseline[p1.yi], baseline[p2.xi], baseline[p2.yi]) {
				return axes
			}
		}
	}
	return axes
}

// varKind classifies a solver variable for perturbation and distance purposes.
type varKind uint8

const (
	varCoordinate    varKind = iota // point x/y (the default)
	varRadius                       // circle radius, ellipse semi-axes
	varAngle                        // ellipse rotation
	varDimensionless                // a bounded ratio: a conic's fullness rho ∈ (0, 1)
)

// varKinds classifies every variable index by walking the entities that own
// non-coordinate variables. Variables retired by removal are fixed and never
// free, so their (stale) classification is never read.
func (s *Sketch) varKinds() []varKind {
	kinds := make([]varKind, len(s.vars))
	for _, e := range s.ents {
		switch t := e.(type) {
		case *Circle:
			kinds[t.ri] = varRadius
		case *Ellipse:
			kinds[t.rxi] = varRadius
			kinds[t.ryi] = varRadius
			kinds[t.roti] = varAngle
		case *EllipticalArc:
			kinds[t.rxi] = varRadius
			kinds[t.ryi] = varRadius
			kinds[t.roti] = varAngle
		case *Conic:
			kinds[t.rhoi] = varDimensionless
		}
	}
	return kinds
}

// configSep is the distance between two configurations: the maximum over the
// free variables of the per-variable relative separation. Coordinates and
// radii are relative to the bounding-box diagonal; angles are wrapped into
// (-π, π], folded by π (an ellipse rotated by π is the same point set) and
// relative to π; a bounded ratio (a conic's rho) is relative to its unit range.
func configSep(a, b []float64, free []int, kinds []varKind, diag float64) float64 {
	worst := 0.0
	for _, vi := range free {
		var sep float64
		switch kinds[vi] {
		case varAngle:
			delta := math.Abs(wrapPi(a[vi] - b[vi]))
			folded := math.Abs(wrapPi(a[vi] - b[vi] - math.Pi))
			sep = math.Min(delta, folded) / math.Pi
		case varDimensionless:
			sep = math.Abs(a[vi] - b[vi]) // already in [0, 1)
		default:
			sep = math.Abs(a[vi]-b[vi]) / diag
		}
		if sep > worst {
			worst = sep
		}
	}
	return worst
}

// wrapPi wraps an angle into (-π, π].
func wrapPi(a float64) float64 {
	a = math.Mod(a, 2*math.Pi)
	if a > math.Pi {
		a -= 2 * math.Pi
	} else if a <= -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

// splitmix64 is the public-domain SplitMix64 mixer (Steele, Lea & Flood). A
// self-contained six-line generator keeps the probe's determinism self-evident
// and independent of any random-stream stability guarantees.
func splitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

// probeUnit maps (seed, restart, variable index) to a deterministic uniform
// value in [0, 1).
func probeUnit(seed uint64, k, vi int) float64 {
	h := splitmix64(splitmix64(seed^uint64(k+1)) + uint64(vi+1))
	return float64(h>>11) / (1 << 53)
}
