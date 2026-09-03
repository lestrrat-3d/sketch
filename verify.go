package sketch

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

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
//
// One path leaves most of the report unevaluated. A nil, corrupt or foreign
// handle would panic the residual, rank, profile and parameter passes, and
// non-finite geometry (a NaN or infinite point coordinate, entity shape
// variable, dimension target, or constraint-owned auxiliary variable) would
// corrupt them silently instead — no rank computed from a poisoned Jacobian is
// trustworthy in either direction, see [Sketch.nonFiniteVars] — so
// [Sketch.Verify] reports what it established BEFORE the analysis and stops
// there: BrokenReferences, ForeignHandles, NonFinitePoints, NonFiniteEntities,
// NonFiniteDimensions, NonFiniteConstraints carry a finding, and every
// other field — Status included — holds its zero value. The
// Stale/StaleReferences/StaleReferencePoints trio is the one exception with
// only PARTIAL coverage on this path: staleness reads snapshot provenance
// flags, which a foreign handle or non-finite geometry cannot corrupt, so the
// trio still carries a finding on those two causes — but a nil or corrupt
// point CAN corrupt it (an entity's staleness is derived from its defining
// points), so on THAT cause alone the trio is left at its zero value like
// every other unevaluated field. Those zero values are
// not verdicts — a false Solvable on that path means "never evaluated", not
// "does not solve", and the zero-value Status ([Underconstrained]) means the
// same thing rather than "no remaining degrees of freedom".
// [VerificationReport.Check] reports such a report as [ErrVerificationIncomplete]
// and asserts none of the conditions the skipped passes decide.
//
// [VerificationReport.Analysed] is how a consumer tells the two paths apart. A
// report is not self-describing: every unevaluated field holds a value that is
// also a legitimate analysed one, so a reader that renders or gates on DOF,
// Status, Solvable, Residual, Conditioning, RankMargin, FreePoints, Profiles,
// InvalidProfiles, ProfilesValid, Probe, ProbeIncomplete, Redundant, Conflicts
// or the parameter fields must ask Analysed first.
type VerificationReport struct {
	// Solvable reports whether every (non-driven) constraint holds within the
	// tolerance at the current configuration (the same default as [Sketch.Solve],
	// overridable with [WithTolerance]). Verify does not move geometry, so call
	// [Sketch.Solve] first: after a converged solve this is the solvability
	// verdict; before any solve, or after one that failed, it reflects the
	// current — possibly unsolved — state. Meaningful only when
	// [VerificationReport.Analysed] is true: the unevaluated zero value is
	// false, which reads as unsolvable.
	Solvable bool
	// Residual is the Euclidean norm of the constraint residual vector at the
	// current configuration (base units; 0 when fully satisfied). Meaningful only
	// when [VerificationReport.Analysed] is true: the unevaluated zero value is
	// 0, which reads as fully satisfied.
	Residual float64
	// DOF is the number of remaining degrees of freedom (0 == fully constrained).
	// Meaningful only when [VerificationReport.Analysed] is true: the unevaluated
	// zero value is 0, which reads as fully constrained.
	DOF int
	// RankMargin is an ADVISORY diagnostic: the multiplicative distance of the
	// STRUCTURAL rank decision from the rank-zero cutoff at the current
	// configuration (near 1 means the rank — and therefore the DOF / redundancy
	// verdict — was decided by a near-cutoff pivot and could flip under a tiny
	// perturbation; +Inf when there are no constraint rows). It is computed on the
	// same nondimensional Jacobian as the rank/DOF analysis, so — unlike before — it
	// is scale- and unit-invariant. It still does NOT gate
	// [VerificationReport.Trustworthy]: it measures the margin of the STRUCTURAL
	// rank decision (could DOF flip), a coarser, different question than
	// [VerificationReport.Conditioning] (how near-singular a full-rank system is),
	// which is the designated trust gate. Meaningful only when
	// [VerificationReport.Analysed] is true: the unevaluated zero value is 0,
	// which reads as a maximally fragile margin rather than as "not computed".
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
	//
	// On the skipped path described above it holds its zero value like every
	// other unevaluated field, NOT +Inf: +Inf is this field's best possible
	// reading, so leaving it there would have a report that evaluated nothing
	// carry the most trusting conditioning value it can report. Ask
	// [VerificationReport.Analysed] before reading it — a 0 here is otherwise
	// indistinguishable from a genuinely singular system.
	Conditioning float64
	// condGate is the tolerance-derived threshold Conditioning was gated against
	// (see [conditioningGate]); read by Trustworthy.
	condGate float64
	// analysisSkipped records that Verify stopped after the reference-integrity
	// and non-finite-geometry scans, so every field the residual/rank/profile/
	// parameter passes fill holds an unevaluated zero value. Check reads it to
	// report only what ran.
	analysisSkipped bool
	// Status is the single-value severity summary (see [Status]). Meaningful only
	// when [VerificationReport.Analysed] is true: the unevaluated zero value is
	// [Underconstrained], which is a real status a caller cannot tell apart from
	// a computed one.
	Status Status
	// Redundant lists constraints that contribute a dependent but satisfied
	// equation — consistent duplicates whose removal changes nothing. Mirrors
	// the redundant half of [Sketch.Diagnose]. Meaningful only when
	// [VerificationReport.Analysed] is true: the unevaluated nil value reads the
	// same as a genuine finding of no redundant constraints.
	Redundant []Constraint
	// Conflicts lists the conflicting constraints — dependent and violated —
	// each with the earlier constraints it fights (see [ConflictSet]). Empty
	// when the sketch is solvable. Meaningful only when
	// [VerificationReport.Analysed] is true: the unevaluated nil value reads the
	// same as a sketch with no conflicts.
	Conflicts []ConflictSet
	// FreePoints lists the points that can still move under some
	// constraint-preserving motion, in id order (the under-constrained
	// remainder). Mirrors [Sketch.FreePoints]. Meaningful only when
	// [VerificationReport.Analysed] is true: nil reads as "fully constrained",
	// but that reading is legitimate ONLY on the analysed path — on the skipped
	// path nil means nothing ran, not that no point remains free.
	FreePoints []*Point
	// Profiles lists the closed-region boundaries detected in the sketch's
	// non-construction geometry (see [Sketch.Profiles]). Meaningful only when
	// [VerificationReport.Analysed] is true: the unevaluated nil value reads the
	// same as a sketch with no closed regions.
	Profiles []*Profile
	// InvalidProfiles lists the detected profiles that failed region validity —
	// self-intersecting, zero-area, or reached by an unresolvable arrangement
	// condition. A condition is reached when it involves one of the region's own
	// boundary curves, or when no curve could be blamed for it at all (an unusable
	// input dropped before it reached the arrangement), which reaches every detected
	// region and so lists them all. A subset of Profiles. Such a region
	// cannot be extruded. It can be empty while ProfilesValid is false: a condition
	// that produced no region, or one that destroyed one, has no profile to list.
	// Meaningful only when [VerificationReport.Analysed] is true: the unevaluated
	// nil value reads the same as a sketch with no invalid profiles.
	InvalidProfiles []*Profile
	// ProfilesValid is true when every detected region is a valid profile and the
	// arrangement resolved cleanly. It is vacuously true when no geometry forms a
	// region (an open sketch has no regions, which is not itself invalid), but
	// false when the arrangement was degenerate even if that produced no region.
	// Mirrors the Stale trust-signal shape. Meaningful only when
	// [VerificationReport.Analysed] is true: the unevaluated zero value is false,
	// which reads as an invalid arrangement rather than as "not evaluated".
	ProfilesValid bool
	// Probe holds the discrete-ambiguity probe result, populated only when
	// [WithProbe] is passed and the sketch is solvable with DOF 0 (the probe's
	// preconditions). It is nil otherwise; a nil Probe is not a uniqueness
	// claim, and — on the skipped path, see [VerificationReport.Analysed] — not
	// even a claim the probe ran at all. See [Sketch.ProbeConfigurations].
	Probe *ProbeResult
	// ProbeIncomplete is true when [WithProbe] was requested and the probe's
	// preconditions held, but the probe could not finish (e.g. ctx was
	// cancelled), so Probe is nil for lack of a result rather than because no
	// probe was asked for. It fails [VerificationReport.Trustworthy]: the
	// requested ambiguity check did not run, so the sketch must not be blessed.
	// Meaningful only when [VerificationReport.Analysed] is true: the unevaluated
	// zero value is false, which reads the same as a probe that was never
	// requested rather than one that was requested but never ran.
	ProbeIncomplete bool
	// probeErr is the error that ended the probe run behind a true
	// ProbeIncomplete; Check reports it as that reason's specifics.
	probeErr error
	// StaleReferences and StaleReferencePoints list the reference geometry whose
	// 3D source has changed since its snapshot was taken (see [Sketch.MarkStale]).
	// Points are tracked separately because a pierce point is not an [Entity].
	// Populated even when [VerificationReport.Analysed] is false, UNLESS the
	// report's analysis was skipped for a nil or corrupt handle rather than a
	// foreign one or non-finite geometry: staleness is established before the
	// analysis passes because it reads snapshot provenance flags a foreign
	// handle or non-finite geometry cannot corrupt, but a nil or corrupt point
	// would panic the same scan (an entity's staleness is derived from its
	// defining points), so on that one cause these fields are left at their
	// zero values — nil — like every other unevaluated field.
	StaleReferences      []Entity
	StaleReferencePoints []*Point
	// Stale is true when any reference geometry is stale — verifying against an
	// outdated snapshot is untrustworthy. Carries the same partial coverage as
	// StaleReferences/StaleReferencePoints above: populated on a report whose
	// analysis was skipped for a foreign handle or non-finite geometry, but left
	// at its zero value (false) when the skip cause was a nil or corrupt handle
	// instead — a false reading that, on that one cause, means "not evaluated"
	// rather than "verified against a current 3D snapshot".
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
	// NonFinitePoints lists points holding a NaN or infinite x or y coordinate,
	// regardless of whether the point is grounded ([Sketch.Fix]). NonFiniteEntities
	// lists entities whose own intrinsic shape variable (a circle's radius, an
	// ellipse's semi-axes, …) is NaN or infinite. NonFiniteDimensions lists
	// dimension constraints whose target is NaN or infinite — DRIVEN ones
	// included, even though a driven dimension contributes no residual row and so
	// poisons no Jacobian: it still cannot be serialized or exported, so a report
	// that blessed it would name a sketch no writer can write (see
	// [Sketch.nonFiniteVars]). NonFiniteConstraints lists constraints holding a
	// NaN or infinite LIVE auxiliary solver variable (a spline foot parameter, a
	// tangency slack, a conic-tangency contact witness — see [auxVars]): an aux
	// var is seeded finite at commit time but is then a free unknown the solver
	// moves like any other, so it can go non-finite with every authored point,
	// entity and dimension target still finite.
	// Any of the four stops Verify at the reference-integrity scan exactly like
	// ForeignHandles: no rank/DOF/conditioning verdict built from such geometry
	// is trustworthy in either direction (see [Sketch.nonFiniteVars]), so the
	// analysis below is skipped rather than reporting a fabricated number.
	NonFinitePoints []*Point
	// NonFiniteEntities is documented on NonFinitePoints above.
	NonFiniteEntities []Entity
	// NonFiniteDimensions is documented on NonFinitePoints above.
	NonFiniteDimensions []Dimension
	// NonFiniteConstraints is documented on NonFinitePoints above.
	NonFiniteConstraints []Constraint
	// ParametersValid is true when every parameter-bound dimension's expression
	// evaluates with consistent unit kinds and a kind matching the dimension. It
	// is false when an expression mixes kinds (e.g. a length plus an angle) or
	// drives a dimension of the wrong kind — a soundness bug a magnitude-only
	// evaluation would silently accept. Meaningful only when
	// [VerificationReport.Analysed] is true: the unevaluated zero value is
	// false, which reads as an invalid parameter rather than as "not evaluated".
	ParametersValid bool
	// ParameterErrors lists the per-dimension parameter-evaluation errors behind a
	// false ParametersValid (each wraps [param.ErrIncompatibleKind] or names the
	// kind mismatch), in constraint order. Meaningful only when
	// [VerificationReport.Analysed] is true: the unevaluated nil value reads the
	// same as a sketch with no parameter errors.
	ParameterErrors []error
}

// The conditions [VerificationReport.Check] reports, one sentinel per condition.
// Each is wrapped in an error carrying the specifics (which constraints, how far
// off), so a caller matches the condition with [errors.Is] and reads the detail
// from the message or the report's own fields.
var (
	// ErrUnsolvable: the sketch does not solve to tolerance.
	ErrUnsolvable = errors.New("sketch: does not solve to tolerance")
	// ErrNotFullyConstrained: the sketch has remaining degrees of freedom, or is
	// over-constrained. [VerificationReport.Status] says which.
	ErrNotFullyConstrained = errors.New("sketch: not fully constrained")
	// ErrConflicting: constraints conflict — see [VerificationReport.Conflicts].
	ErrConflicting = errors.New("sketch: conflicting constraints")
	// ErrRedundant: dependent-but-satisfied constraints are present — see
	// [VerificationReport.Redundant].
	ErrRedundant = errors.New("sketch: redundant constraints")
	// ErrStaleReference: reference geometry is stale, so the sketch is being
	// verified against an outdated 3D snapshot.
	ErrStaleReference = errors.New("sketch: stale reference geometry")
	// ErrBrokenReference: reference geometry failed its lock-integrity check —
	// see [VerificationReport.BrokenReferences].
	ErrBrokenReference = errors.New("sketch: broken reference geometry")
	// ErrForeignHandle: a point or entity reachable from this sketch is not
	// live-owned by it (cross-sketch references are unsupported).
	ErrForeignHandle = errors.New("sketch: foreign handle")
	// ErrVerificationIncomplete: the reference-integrity or non-finite-geometry
	// scan found a nil, corrupt, foreign, or non-finite handle, so [Sketch.Verify]
	// stopped before the solvability, rank, profile and parameter passes could
	// run on that geometry. The conditions those passes decide are UNKNOWN for
	// this report rather than passed, which is why the verdict fails: repair the
	// handles the accompanying reasons name and verify again.
	ErrVerificationIncomplete = errors.New("sketch: verification incomplete")
	// ErrInvalidProfile: the region set cannot be trusted as extrudable profiles —
	// see [VerificationReport.InvalidProfiles], which can be empty when the
	// arrangement was unresolvable without producing a region.
	ErrInvalidProfile = errors.New("sketch: invalid profiles")
	// ErrInvalidParameter: a parameter-bound dimension's expression is not
	// unit-kind-consistent — see [VerificationReport.ParameterErrors].
	ErrInvalidParameter = errors.New("sketch: invalid parameter expression")
	// ErrNearSingular: the constraint system is numerically near-singular — see
	// [VerificationReport.Conditioning].
	ErrNearSingular = errors.New("sketch: near-singular constraint system")
	// ErrProbeIncomplete: the ambiguity probe was requested and its preconditions
	// held, but it could not finish, so ambiguity is unknown rather than absent.
	ErrProbeIncomplete = errors.New("sketch: ambiguity probe did not finish")
	// ErrAmbiguous: the probe found several discrete configurations satisfying the
	// same constraints — see [VerificationReport.Probe].
	ErrAmbiguous = errors.New("sketch: sketch admits several configurations")
)

// Reasons is the error [VerificationReport.Check] returns: one wrapped sentinel
// per failed condition, so a caller decides per reason rather than over the whole
// verdict.
//
// Unwrap exposes the reasons as data. [errors.Is] also matches through it, so a
// caller that cares about only one condition can ask directly without ranging.
type Reasons interface {
	error
	// Unwrap returns one error per failed condition, each wrapping the sentinel
	// that names it. It is never empty: a report with nothing to report yields a
	// nil Reasons instead.
	Unwrap() []error
}

// reasons is the concrete [Reasons]. It is never returned empty — Check returns a
// literal nil interface instead, so `Check() != nil` cannot be true for a
// trustworthy report through a non-nil interface holding a nil pointer.
type reasons struct{ errs []error }

func (r *reasons) Error() string {
	msgs := make([]string, len(r.errs))
	for i, e := range r.errs {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "\n")
}

func (r *reasons) Unwrap() []error { return r.errs }

// Check reports every condition of the oracle verdict that the sketch fails,
// returning nil when it passes them all.
//
// It is the granular form of [VerificationReport.Trustworthy], which is defined as
// `Check() == nil` — so the two can never disagree, and a condition added here is
// added to both. Gate on Trustworthy when the whole verdict is what matters; use
// Check when a diagnostic needs to name what failed, or when the caller
// legitimately disagrees with ONE condition:
//
//	if err := rep.Check(); err != nil {
//		for _, reason := range err.Unwrap() {
//			// A sketch built from unsigned constraints is ambiguous BY DESIGN;
//			// every other condition still gates.
//			if !errors.Is(reason, sketch.ErrAmbiguous) {
//				return reason
//			}
//		}
//	}
//
// That waiver is per reason and stays honest as the engine grows: a condition
// added to the verdict later arrives as another element and is fatal by default,
// because it is not in the caller's waiver list. Reimplementing the verdict by
// copying its conditions is the alternative this exists to remove — such a copy
// silently stops checking whatever is added next, and cannot reproduce the
// conditioning gate at all, since its threshold is not exported.
//
// The reasons appear in a fixed order: the reference-integrity, non-finite-
// geometry and staleness conditions come first, being the ones [Sketch.Verify]
// establishes before it analyses anything, then the analysis conditions, most
// fundamental first — an unsolvable sketch is reported before the properties
// that only make sense once it solves. A report whose analysis was skipped
// carries every reason established before analysis — reference-integrity and
// non-finite-geometry always, and staleness too UNLESS the skip cause was a
// nil or corrupt handle rather than a foreign one or non-finite geometry: a
// nil or corrupt point would panic the staleness scan itself, so on that one
// cause it never ran and [ErrStaleReference] cannot be among the reasons even
// when the underlying reference geometry is in fact stale — plus
// [ErrVerificationIncomplete], and nothing else: asserting a condition nobody
// evaluated would invent a failure, and would leave a caller who deliberately
// waives the handle condition blocked by reasons that were never tested.
func (r *VerificationReport) Check() Reasons {
	var errs []error
	add := func(err error) { errs = append(errs, err) }

	// The reference-integrity and non-finite-geometry scans run before every
	// other pass, so their conditions are the ones a report carries on either
	// path.
	if n := len(r.BrokenReferences); n > 0 {
		add(fmt.Errorf("%w: %d entities", ErrBrokenReference, n))
	}
	if r.ForeignHandles {
		add(fmt.Errorf("%w: a reachable point or entity is not owned by this sketch", ErrForeignHandle))
	}
	if n := len(r.NonFinitePoints) + len(r.NonFiniteEntities) + len(r.NonFiniteDimensions) + len(r.NonFiniteConstraints); n > 0 {
		add(fmt.Errorf("%w: %d points, %d entities, %d dimensions, %d constraints", ErrNonFiniteGeometry,
			len(r.NonFinitePoints), len(r.NonFiniteEntities), len(r.NonFiniteDimensions), len(r.NonFiniteConstraints)))
	}
	// Staleness sits in this group because [Sketch.Verify] establishes it here:
	// it reads snapshot provenance flags, not the analysis, so it is reported on
	// the foreign-handle and non-finite skip paths too rather than dropped for a
	// condition it does not depend on. A NIL-CORRUPT handle is the one exception —
	// it would panic the staleness scan itself, so that scan does not run and
	// r.Stale is left at its zero value, reporting nothing here.
	if r.Stale {
		add(fmt.Errorf("%w: %d entities, %d points", ErrStaleReference,
			len(r.StaleReferences), len(r.StaleReferencePoints)))
	}
	if r.analysisSkipped {
		// Everything below reads a field the skipped passes never wrote, so it
		// would report a failure nobody tested. Name the missing analysis
		// instead — the verdict still fails, on a condition that is true.
		add(fmt.Errorf("%w: solvability, degrees of freedom, profiles and parameters were not analysed",
			ErrVerificationIncomplete))
		return &reasons{errs: errs}
	}

	if !r.Solvable {
		add(fmt.Errorf("%w: residual %g", ErrUnsolvable, r.Residual))
	}
	if r.Status != FullyConstrained {
		add(fmt.Errorf("%w: %s (DOF %d)", ErrNotFullyConstrained, r.Status, r.DOF))
	}
	if n := len(r.Conflicts); n > 0 {
		add(fmt.Errorf("%w: %d", ErrConflicting, n))
	}
	if n := len(r.Redundant); n > 0 {
		add(fmt.Errorf("%w: %d", ErrRedundant, n))
	}
	if !r.ProfilesValid {
		add(fmt.Errorf("%w: %d of %d regions", ErrInvalidProfile,
			len(r.InvalidProfiles), len(r.Profiles)))
	}
	if !r.ParametersValid {
		add(fmt.Errorf("%w: %d dimensions", ErrInvalidParameter, len(r.ParameterErrors)))
	}
	if !(r.Conditioning >= r.condGate) { // NaN fails closed
		// Printing condGate here exposes nothing: the threshold is already public
		// information, stated verbatim as max(1e-6, 4·√tolerance) in the Conditioning
		// field's own doc above, and it is a function of the tolerance THIS CALLER
		// passed to Verify — so a caller can compute it without reading this message.
		// The field stays unexported and gains no accessor; a number in an error
		// string is not API surface. Without it the reason reads "conditioning 3e-08
		// is below" and cannot be acted on.
		add(fmt.Errorf("%w: conditioning %g is below %g", ErrNearSingular, r.Conditioning, r.condGate))
	}
	if r.ProbeIncomplete {
		if r.probeErr != nil {
			add(fmt.Errorf("%w: %s", ErrProbeIncomplete, r.probeErr))
		} else {
			add(fmt.Errorf("%w: ambiguity is unknown", ErrProbeIncomplete))
		}
	}
	if r.Probe != nil && r.Probe.Ambiguous() {
		add(fmt.Errorf("%w: %d", ErrAmbiguous, len(r.Probe.Configurations)))
	}

	if len(errs) == 0 {
		return nil // a literal nil, never a nil *reasons in a non-nil interface
	}
	return &reasons{errs: errs}
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
//
// A report whose analysis was skipped never passes either, and for a different
// reason: a nil, corrupt, foreign, or non-finite handle stopped [Sketch.Verify]
// before those conditions could be established at all, so the verdict fails on
// [ErrVerificationIncomplete] alongside the handle reason.
//
// It is exactly [VerificationReport.Check] returning nil, and is defined that way
// rather than restated, so the boolean and the reasons cannot drift apart. Use
// Check to learn WHICH condition failed, or to waive one deliberately.
func (r *VerificationReport) Trustworthy() bool { return r.Check() == nil }

// Analysed reports whether [Sketch.Verify] ran its analysis passes — solvability,
// rank/DOF, conditioning, the constraint dependency analysis, profiles and
// parameters — or stopped at the scans that precede them because a nil, corrupt
// or foreign handle, or non-finite geometry, would have made those passes panic
// or answer from poisoned input.
//
// It is false exactly when [ErrVerificationIncomplete] is among the reasons, and
// it is the ONLY way to tell the two paths apart from a report alone: every
// field the skipped passes never wrote holds its zero value, and each of those
// zero values is also a legitimate analysed reading — DOF 0 is fully
// constrained, Status is [Underconstrained], Solvable is false, ProfilesValid is
// false, Conditioning is 0. A consumer that renders, logs or gates on any of
// them must branch on this first; the fields' own doc comments say which ones.
//
// [VerificationReport.Trustworthy] already fails on that path, so a caller that
// only gates on the verdict needs nothing here. This is for the reader that
// wants the numbers.
func (r *VerificationReport) Analysed() bool { return !r.analysisSkipped }

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

// dof derives the DOF count from a [committedJacobian] and its [rankAnalysis]
// — the same two-branch shape [Sketch.DOF] uses: the free-variable count
// directly when there are no residual rows (no elimination ran), else the free
// count minus the rank, floored at 0. Used only by [Sketch.Verify], which
// builds cj and ra once and shares them across this and [margin] rather than
// calling [Sketch.DOF] (which would rebuild its own Jacobian).
func dof(cj committedJacobian, ra rankAnalysis) int {
	if cj.m == 0 {
		return len(cj.free)
	}
	d := len(cj.free) - ra.rank
	if d < 0 {
		return 0
	}
	return d
}

// margin derives the rank-decision margin from a [committedJacobian] and its
// [rankAnalysis]: +Inf when there are no residual rows (vacuously
// well-separated), else the analysis' own margin. Used only by [Sketch.Verify];
// see [dof].
func margin(cj committedJacobian, ra rankAnalysis) float64 {
	if cj.m == 0 {
		return math.Inf(1)
	}
	return ra.margin()
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
//
// The ctx argument bounds any probe run triggered by [WithProbe] (the only
// potentially expensive, re-solving work Verify performs); pass
// context.Background() when no bound is needed.
func (s *Sketch) Verify(ctx context.Context, options ...VerifyOption) *VerificationReport {
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

	rep := &VerificationReport{condGate: conditioningGate(tolerance)}

	// Reference integrity + reachability first: it is nil-safe, and a nil/corrupt
	// or foreign operand would otherwise panic the residual/profile/staleness
	// analysis below (a foreign entity such as &Line{} can have nil endpoints).
	// Such a sketch is untrustworthy regardless, so report the broken/foreign
	// handles and skip the analysis. The skip is recorded: the fields those
	// passes would have filled keep their zero values, and Check must report the
	// missing analysis rather than read them as findings.
	nilCorrupt := s.scanReferenceIntegrity(rep)

	// Non-finite geometry next. nonFiniteVars reads s.vars directly rather than
	// following a point's coordinate accessor, so — like the scan above — it
	// never panics on nil/corrupt topology and can run regardless of what that
	// scan found. A NaN/Inf point coordinate, entity shape variable, dimension
	// target or constraint-owned auxiliary variable poisons every pass below it
	// in a way that reads as either too much or too little constraint, never a
	// sound answer either way (see [Sketch.nonFiniteVars]) — and a Jacobian-level
	// guard could not catch the sharpest case, a DOF-0 candidate whose every
	// point is fixed, which builds a perfectly finite zero-column matrix and hits
	// [Sketch.conditioningOn]'s own len(free)==0 shortcut (+Inf, maximal trust)
	// without ever looking at a value. So this screens the geometry, not the
	// matrix, and takes the same early-out ForeignHandles already does.
	nf := s.nonFiniteVars()
	rep.NonFinitePoints = nf.points
	rep.NonFiniteEntities = nf.entities
	rep.NonFiniteDimensions = nf.dims
	rep.NonFiniteConstraints = nf.cons

	// Reference staleness is established BEFORE the early-out, not after the
	// analysis, because it is not part of the analysis: it reads the boolean
	// provenance flags a snapshot carries and nothing a non-finite value or a
	// foreign handle can corrupt. Skipping it bought nothing and lost a real
	// finding whose ZERO VALUE is the blessing one — Stale=false reads as
	// "verified against a current 3D snapshot", so one poisoned point beside a
	// genuinely stale reference turned Stale=true into Stale=false with nothing
	// left to say otherwise. It still must not run on NIL-corrupt topology: an
	// entity's staleness is derived from its defining points (Line.IsStale reads
	// Start and End), so a nil point panics here — which is the whole reason the
	// scan sat below the early-out to begin with.
	if !nilCorrupt {
		s.scanReferenceStaleness(rep)
	}

	if nilCorrupt || rep.ForeignHandles || nf.found() {
		rep.analysisSkipped = true
		return rep
	}

	// Conditioning's "not applicable" value is +Inf, its best possible reading,
	// so it is initialized only once the analysis is actually going to run. Set
	// on the report literal it would survive the early-out above and have a
	// report that evaluated nothing carry maximal trust in the one field a
	// caller reads as "how far from singular is this" — the blessing direction,
	// on both early-out causes (a foreign handle as much as non-finite
	// geometry). Below the early-out the skipped path keeps the zero value the
	// report's own doc comment promises for every unevaluated field.
	rep.Conditioning = math.Inf(1)

	r := s.residuals(nil)
	rep.Residual = math.Sqrt(dot(r, r))
	rep.Solvable = rep.Residual <= tolerance

	// One committed Jacobian serves every analysis below — rank/DOF, conditioning,
	// conflict/redundancy, free points — instead of each rebuilding its own (see
	// committedJacobian's doc comment in conditioning.go for why sharing it is
	// sound within this one call, and only within it). ok is unreachable false:
	// the early-out above already stopped on the same non-finite-geometry
	// condition buildCommittedJacobian screens.
	cj, ok := s.buildCommittedJacobian()
	if !ok {
		rep.analysisSkipped = true
		return rep
	}
	ra := s.rankAnalysisOn(cj) // one elimination serves both DOF and RankMargin
	rep.DOF = dof(cj, ra)
	rep.RankMargin = margin(cj, ra) // advisory; does not gate Trustworthy (scale-dependent)
	// The conditioning measure is meaningful only for a DOF-0 candidate: an
	// under-constrained sketch is genuinely singular by its free DOF (a separate
	// verdict), so leave Conditioning at +Inf (not applicable) there.
	if rep.DOF == 0 {
		rep.Conditioning = s.conditioningOn(cj)
	}

	flagged, conflicts := s.conflictAnalysisOn(cj)
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

	movable := s.movableVarsOn(cj)
	rep.FreePoints = freePoints(s.points, movable) // same loop Sketch.FreePoints uses

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
	// keeps the (expensive) probe from running when it would only error. ctx
	// bounds it: a cancelled/failed probe leaves Probe nil but marks the report
	// incomplete, so Trustworthy() does not pass as if the requested ambiguity
	// check had run and found nothing.
	if probe && rep.Solvable && rep.DOF == 0 {
		if pr, err := s.ProbeConfigurations(ctx, probeOpts...); err == nil {
			rep.Probe = pr
		} else {
			rep.ProbeIncomplete = true
			rep.probeErr = err
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
