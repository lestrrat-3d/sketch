package sketch

import (
	"math"

	"github.com/lestrrat-go/option/v3"
)

// Status summarizes a sketch's constraint state in a single value. The full
// picture lives in the [VerificationReport] fields (DOF, Redundant, Conflicts);
// Status applies a fixed severity precedence so one check can gate on the
// dominant condition:
//
//   - [Overconstrained] if any constraint conflicts (the sketch is unsolvable),
//   - else [Underconstrained] if degrees of freedom remain,
//   - else [Overconstrained] if a consistent redundant constraint is present,
//     or the sketch is DOF-0 yet unsatisfiable (e.g. distances that violate the
//     triangle inequality — independent constraints with no real solution, which
//     rank analysis cannot localize),
//   - else [FullyConstrained].
//
// A conflict outranks remaining DOF because it makes the sketch unsolvable;
// remaining DOF outranks consistent redundancy because the sketch is not yet
// determined. [FullyConstrained] is only ever returned for a *solvable* sketch,
// so the status never reads as "valid" for one whose constraints do not hold.
// The zero value is [Underconstrained] — a safe default that never reads as
// "valid" either.
type Status int

const (
	// Underconstrained: the sketch has remaining degrees of freedom.
	Underconstrained Status = iota
	// FullyConstrained: DOF 0 with no redundant or conflicting constraints.
	FullyConstrained
	// Overconstrained: redundant or conflicting constraints are present.
	Overconstrained
)

// String returns a lowercase human-readable name for the status.
func (st Status) String() string {
	switch st {
	case Underconstrained:
		return "underconstrained"
	case FullyConstrained:
		return "fully constrained"
	case Overconstrained:
		return "overconstrained"
	default:
		return "unknown"
	}
}

// VerificationReport aggregates the trust signals an agent needs to decide a
// sketch is correct before executing the equivalent work in CAD software. It is
// produced by [Sketch.Verify] and is a read-only snapshot of the call-time
// configuration; it holds no live link to the sketch.
type VerificationReport struct {
	// Solvable reports whether every (non-driven) constraint holds within the
	// tolerance at the current configuration (the same default as [Sketch.Solve],
	// overridable with [WithTolerance]). Verify does not move geometry, so call
	// [Sketch.Solve] first: after a converged solve this is the solvability
	// verdict; before any solve, or after one that failed, it reflects the
	// current — possibly unsolved — state.
	Solvable bool
	// Residual is the Euclidean norm of the constraint residual vector at the
	// current configuration (base units; 0 when fully satisfied).
	Residual float64
	// DOF is the number of remaining degrees of freedom (0 == fully constrained).
	DOF int
	// RankMargin is an ADVISORY diagnostic: the multiplicative distance of the
	// constraint Jacobian's closest rank decision from the hard pivot threshold at
	// the current configuration (near 1 means the rank — and therefore the DOF /
	// redundancy verdict — was decided by a near-threshold pivot and could flip
	// under a tiny perturbation; +Inf when there are no constraint rows). It is
	// NOT a unit-invariant conditioning measure: the raw pivots scale with geometry
	// (e.g. angle-constraint derivatives grow with line length), so it is not
	// comparable across sketches of different scale and must NOT be thresholded as
	// a pass/fail gate — it does not gate [VerificationReport.Trustworthy].
	RankMargin float64
	// Conditioning is the SCALE- AND UNIT-INVARIANT near-singularity measure that
	// DOES gate [VerificationReport.Trustworthy]: the reciprocal condition number
	// σ_min/σ_max of the nondimensionalized constraint Jacobian (A = Drow·J·Dcol,
	// length rows/cols scaled by the bounding-box diagonal so every entry is
	// dimensionless). Unlike RankMargin it is comparable across sketches of any
	// scale or unit. A small value means the DOF-0 verdict is decided by a
	// near-dependent constraint set (e.g. a point pinned by two nearly-parallel
	// lines, or a tangency at a near-degenerate contact) and is too fragile to
	// bless; below the trust threshold it fails the gate. That threshold is
	// tolerance-derived — max(1e-6, 4·√tolerance) — so a slack-encoded inequality
	// resting at its active boundary (where the slack only resolves to ≈√tolerance)
	// cannot slip a near-singular system through. It is computed only for an
	// otherwise fully-constrained candidate (DOF 0); an under-constrained sketch is
	// genuinely singular by its free DOF — a separate, already-reported verdict —
	// so Conditioning is left +Inf (not applicable) there.
	Conditioning float64
	// condGate is the tolerance-derived threshold Conditioning was gated against
	// (see [conditioningGate]); read by Trustworthy.
	condGate float64
	// Status is the single-value severity summary (see [Status]).
	Status Status
	// Redundant lists constraints that contribute a dependent but satisfied
	// equation — consistent duplicates whose removal changes nothing. Mirrors
	// the redundant half of [Sketch.Diagnose].
	Redundant []Constraint
	// Conflicts lists the conflicting constraints — dependent and violated —
	// each with the earlier constraints it fights (see [ConflictSet]). Empty
	// when the sketch is solvable.
	Conflicts []ConflictSet
	// FreePoints lists the points that can still move under some
	// constraint-preserving motion, in id order (the under-constrained
	// remainder). Nil when the sketch is fully constrained. Mirrors
	// [Sketch.FreePoints].
	FreePoints []*Point
	// Profiles lists the closed-region boundaries detected in the sketch's
	// non-construction geometry (see [Sketch.Profiles]).
	Profiles []*Profile
	// InvalidProfiles lists the detected profiles that failed region validity —
	// self-intersecting, degenerate (zero-area), or produced by an unresolvable
	// arrangement. A subset of Profiles. Such a region cannot be extruded.
	InvalidProfiles []*Profile
	// ProfilesValid is true when every detected region is a valid profile and the
	// arrangement resolved cleanly. It is vacuously true when no geometry forms a
	// region (an open sketch has no regions, which is not itself invalid), but
	// false when the arrangement was degenerate even if that produced no region.
	// Mirrors the Stale trust-signal shape.
	ProfilesValid bool
	// Probe holds the discrete-ambiguity probe result, populated only when
	// [WithProbe] is passed and the sketch is solvable with DOF 0 (the probe's
	// preconditions). It is nil otherwise; a nil Probe is not a uniqueness
	// claim. See [Sketch.ProbeConfigurations].
	Probe *ProbeResult
	// StaleReferences and StaleReferencePoints list the reference geometry whose
	// 3D source has changed since its snapshot was taken (see [Sketch.MarkStale]).
	// Points are tracked separately because a pierce point is not an [Entity].
	StaleReferences      []Entity
	StaleReferencePoints []*Point
	// Stale is true when any reference geometry is stale — verifying against an
	// outdated snapshot is untrustworthy.
	Stale bool
	// BrokenReferences lists entities failing the reference lock-integrity check:
	// a reference entity whose defining points were rewired, are not all
	// reference-locked, or whose owned vars are not fixed — plus any entity (even
	// a normal one) whose defining point is a foreign/dead handle.
	BrokenReferences []Entity
	// ForeignHandles is true when any point or entity reachable from the sketch's
	// entities or constraints is not live-owned by this sketch (e.g. a constraint
	// to a reference point of another sketch). Cross-sketch references are
	// unsupported; this surfaces them rather than silently trusting them.
	ForeignHandles bool
	// ParametersValid is true when every parameter-bound dimension's expression
	// evaluates with consistent unit kinds and a kind matching the dimension. It
	// is false when an expression mixes kinds (e.g. a length plus an angle) or
	// drives a dimension of the wrong kind — a soundness bug a magnitude-only
	// evaluation would silently accept.
	ParametersValid bool
	// ParameterErrors lists the per-dimension parameter-evaluation errors behind a
	// false ParametersValid (each wraps [param.ErrIncompatibleKind] or names the
	// kind mismatch), in constraint order.
	ParameterErrors []error
}

// Trustworthy reports the canonical oracle verdict: the sketch is solvable, fully
// constrained, free of conflicting and redundant constraints, has no stale or
// broken reference geometry, no foreign handles, every detected region is a
// valid profile, every parameter expression is unit-kind-consistent, its
// constraint system is not numerically near-singular (the scale-invariant
// [VerificationReport.Conditioning] is at or above its threshold), and — if the
// ambiguity probe ran — is not ambiguous. It is the single check an agent should
// gate on; a stale, broken-reference, self-intersecting, or near-singular sketch
// never reads as a clean pass through it, even when [VerificationReport.Status] is
// [FullyConstrained]. (The advisory [VerificationReport.RankMargin] is reported
// separately; being scale-dependent, it does not gate this verdict — Conditioning
// is the unit-invariant gating measure.)
func (r *VerificationReport) Trustworthy() bool {
	return r.Solvable &&
		r.Status == FullyConstrained &&
		len(r.Conflicts) == 0 &&
		len(r.Redundant) == 0 &&
		!r.Stale &&
		len(r.BrokenReferences) == 0 &&
		!r.ForeignHandles &&
		r.ProfilesValid &&
		r.ParametersValid &&
		r.Conditioning >= r.condGate &&
		(r.Probe == nil || !r.Probe.Ambiguous())
}

// VerifyOption tunes [Sketch.Verify]. Construct values with the With… helpers.
type VerifyOption interface {
	option.Interface
	verifyOption()
}

type verifyOption struct{ option.Interface }

func (verifyOption) verifyOption() {}

type identProbe struct{}

// WithProbe enables the discrete-ambiguity probe ([Sketch.ProbeConfigurations])
// as part of verification, populating [VerificationReport.Probe]. The probe is
// expensive — it re-solves the sketch from many perturbations — so it is off by
// default. Any [ProbeOption] values passed here are forwarded to the probe.
//
// The probe only runs when its preconditions hold (the sketch is solvable with
// DOF 0); otherwise Probe is left nil, and the report's Solvable/DOF fields
// explain why.
func WithProbe(opts ...ProbeOption) VerifyOption {
	return verifyOption{option.New(identProbe{}, opts)}
}

// Verify aggregates the sketch's verification signals into a single
// [VerificationReport]: solvability, degrees of freedom, the redundant and
// conflicting constraints (with each conflict's set), the still-free points,
// the closed profiles and their validity (self-intersecting / degenerate
// regions are reported and gate [VerificationReport.Trustworthy]), and — with
// [WithProbe] — discrete configuration ambiguity.
//
// Like [Sketch.DOF] and [Sketch.Diagnose], Verify analyses the call-time
// configuration and does not move any geometry; call [Sketch.Solve] first so
// the report reflects the solved sketch. It recomputes the constraint Jacobian
// at the current configuration (never reusing a solve's stale one), so the
// counts are consistent with the geometry as it stands.
func (s *Sketch) Verify(options ...VerifyOption) *VerificationReport {
	var probe bool
	var probeOpts []ProbeOption
	tolerance := defaultSolveConfig().tolerance
	for _, opt := range options {
		switch opt.Ident().(type) {
		case identProbe:
			probe = true
			probeOpts = option.MustGet[[]ProbeOption](opt)
		case identTolerance:
			tolerance = option.MustGet[float64](opt)
		}
	}

	rep := &VerificationReport{Conditioning: math.Inf(1), condGate: conditioningGate(tolerance)}

	// Reference integrity + reachability first: it is nil-safe, and a nil/corrupt
	// or foreign operand would otherwise panic the residual/profile/staleness
	// analysis below (a foreign entity such as &Line{} can have nil endpoints).
	// Such a sketch is untrustworthy regardless, so report the broken/foreign
	// handles and skip the analysis.
	if nilCorrupt := s.scanReferenceIntegrity(rep); nilCorrupt || rep.ForeignHandles {
		rep.Status = Overconstrained
		return rep
	}

	r := s.residuals(nil)
	rep.Residual = math.Sqrt(dot(r, r))
	rep.Solvable = rep.Residual <= tolerance

	rep.DOF = s.DOF()
	rep.RankMargin = s.rankMargin() // advisory; does not gate Trustworthy (scale-dependent)
	// The conditioning measure is meaningful only for a DOF-0 candidate: an
	// under-constrained sketch is genuinely singular by its free DOF (a separate
	// verdict), so leave Conditioning at +Inf (not applicable) there.
	if rep.DOF == 0 {
		rep.Conditioning = s.conditioning()
	}

	flagged, conflicts := s.conflictAnalysis()
	rep.Conflicts = conflicts
	if len(conflicts) < len(flagged) {
		bad := make(map[Constraint]struct{}, len(conflicts))
		for _, cs := range conflicts {
			bad[cs.Constraint] = struct{}{}
		}
		for _, c := range flagged {
			if _, isBad := bad[c]; !isBad {
				rep.Redundant = append(rep.Redundant, c)
			}
		}
	}

	rep.ParametersValid = true
	if s.params != nil {
		for _, c := range s.cons {
			d, ok := c.(Dimension)
			if !ok || d.Driven() {
				continue // a driven dimension measures; an expression cannot drive it
			}
			expr := d.driverExpr()
			if expr == "" {
				continue // a literal-valued dimension carries no expression to kind-check
			}
			if _, err := s.evalDimension(d, expr); err != nil {
				rep.ParametersValid = false
				rep.ParameterErrors = append(rep.ParameterErrors, err)
			}
		}
	}

	rep.FreePoints = s.FreePoints()
	profiles, degenerate, _ := s.buildProfiles()
	rep.Profiles = profiles
	rep.ProfilesValid = !degenerate
	for _, p := range profiles {
		if !p.Valid {
			rep.InvalidProfiles = append(rep.InvalidProfiles, p)
			rep.ProfilesValid = false
		}
	}
	rep.Status = classifyStatus(rep)

	// The probe's preconditions are exactly solvable && DOF 0; guarding here
	// keeps the (expensive) probe from running when it would only error.
	if probe && rep.Solvable && rep.DOF == 0 {
		if pr, err := s.ProbeConfigurations(probeOpts...); err == nil {
			rep.Probe = pr
		}
	}

	s.scanReferenceStaleness(rep)

	return rep
}

// classifyStatus applies the severity precedence documented on [Status].
func classifyStatus(r *VerificationReport) Status {
	if len(r.Conflicts) > 0 {
		return Overconstrained
	}
	if r.DOF > 0 {
		return Underconstrained
	}
	// A DOF-0 sketch that is redundant, or unsatisfiable in a way rank analysis
	// cannot localize (e.g. the triangle inequality), is over-constrained — and
	// must never report FullyConstrained while !Solvable, or the status would
	// read as "valid" for an unsolved sketch.
	if len(r.Redundant) > 0 || !r.Solvable {
		return Overconstrained
	}
	return FullyConstrained
}
