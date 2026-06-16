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
	// Probe holds the discrete-ambiguity probe result, populated only when
	// [WithProbe] is passed and the sketch is solvable with DOF 0 (the probe's
	// preconditions). It is nil otherwise; a nil Probe is not a uniqueness
	// claim. See [Sketch.ProbeConfigurations].
	Probe *ProbeResult
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
// the closed profiles, and — with [WithProbe] — discrete configuration
// ambiguity.
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

	rep := &VerificationReport{}

	r := s.residuals(nil)
	rep.Residual = math.Sqrt(dot(r, r))
	rep.Solvable = rep.Residual <= tolerance

	rep.DOF = s.DOF()

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

	rep.FreePoints = s.FreePoints()
	rep.Profiles = s.Profiles()
	rep.Status = classifyStatus(rep)

	// The probe's preconditions are exactly solvable && DOF 0; guarding here
	// keeps the (expensive) probe from running when it would only error.
	if probe && rep.Solvable && rep.DOF == 0 {
		if pr, err := s.ProbeConfigurations(probeOpts...); err == nil {
			rep.Probe = pr
		}
	}

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
