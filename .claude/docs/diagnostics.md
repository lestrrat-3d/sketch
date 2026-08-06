# Diagnostics and verification — `solver.go`, `diagnose.go`, `verify.go`, `probe.go`

Detail moved out of CLAUDE.md's architecture table. Read before touching rank/DOF analysis, the conflict/redundancy passes, `Verify`'s report or trust verdict, or the ambiguity probe.

## Question router

| Your question | Section |
|---|---|
| What happens on non-finite geometry? | The four primitives that carry the non-finite screen |
| Why does `DOF` return the total variable count? | `DOF` answers with maximum ignorance |
| Why do `FreePoints`/`Diagnose` name everything? | Maximum-ignorance answers for the bare reads |
| What gates `Trustworthy()`? | The trust verdict has one definition and two shapes |
| Which report fields are not verdicts? | `Check` asserts only conditions `Verify` evaluated |
| Why does a per-handle read answer false? | The two per-handle reads |
| Why does `CheckConstraint` refuse a candidate? | `diagnose.go` — overview and the `CheckConstraint` screens |
| Why does the probe refuse or return NaN? | `probe.go` — the ambiguity probe |

Navigation only — the sections below are the authority.

## `solver.go` — solver, Jacobian, rank analysis

### Overview

Levenberg–Marquardt solver, numerical Jacobian, DOF/redundancy (rank) analysis.

### `rank`/`committedRankAnalysis` carry the non-finite screen

**`Sketch.rank`/`Sketch.committedRankAnalysis` are one of the FOUR primitives
that CARRY the non-finite screen in a second return** (see the `verify.go` row):
the analysis is not run and `ok` is false when `hasNonFiniteVars` holds, so a
reader cannot take a rank built from a poisoned Jacobian without writing the
refusal down.

### `Solve` reports DOF/Redundant as the -1 sentinel

**`Solve` translates that into `Result.DOF`/`Result.Redundant` =
`-1`, the not-computed sentinel a cancelled solve already uses, with a NIL
error** — `Converged`, `Residual` and `Iterations` still report what the solver
measured, since they are the solver's own account of the rows it evaluated and
not a verdict built from the Jacobian. A caller gating on `res.DOF == 0 &&
res.Converged` must therefore read `-1` as a refusal, not as a small number.

### The zero-row branch asks the screen directly

**The zero-ROW branch (`mh == 0`) asks the screen DIRECTLY**, because no rank
pass runs there to carry it and the FREE-variable count it would report
collapses to 0 on an all-grounded sketch — the same two-branch shape `DOF` uses.
`rankMargin` reports 0 (maximally fragile) rather than its `+Inf` "vacuously
well-separated" value.

### `DOF` answers with maximum ignorance

**`DOF` has no error return and so cannot refuse on
non-finite geometry** (a NaN/Inf point coordinate, entity shape variable,
dimension target, or constraint-owned auxiliary variable — see `nonfinite.go`,
the `verify.go` row): its rank pass otherwise depends on the partial-pivot
elimination in `rankAnalysisOfMatrix`, which reads a non-finite pivot two
different ways depending on where it lands — never selected (an undercount,
since `v > best` is false against a NaN best) or, once selected, never rejected
either (an overcount, since `best < rankZeroTol` is equally false against a NaN
best) — so no rank it produces is trustworthy in either direction. `DOF` instead
answers with a documented MAXIMUM-IGNORANCE value — the sketch's TOTAL variable
count (`len(s.vars)`), grounded variables INCLUDED, as if neither a constraint
nor a `Fix` had ever been applied — the one direction a caller gating on
`DOF()==0` can never read as falsely fully-constrained; `Sketch.Verify` is the
call that reports the condition itself.

### The free-variable count is not that value

**The FREE-variable count is NOT that
value**: `freeVars` filters on `fixed`, so on an all-grounded sketch it is empty
and the count collapses onto exactly 0 — the reading that says fully constrained
— on exactly the fixture the screen exists for (a line with both endpoints
`Fix`ed beside a `Fix`ed stray point at `(NaN, NaN)`). `Fix` IS a constraint, so
a value that credits it is not maximum ignorance. The total count cannot
collapse that way — every sketch owns an origin contributing two variables — and
it stays a non-negative count inside `DOF`'s documented domain, which a `-1`
sentinel would not: a caller writing `if DOF() > 0 { warn }` reads `-1` as no
warning, the blessing direction again.
`FreePoints`/`Diagnose`/`RedundantConstraints` (`diagnose.go`, `solver.go`) take
the same stance; `Sketch.CheckConstraint` (`diagnose.go`) and `Sketch.Verify`
(`verify.go`) instead refuse outright, since both have an error return / a
report field to record the skip on.

### `pivotAbs` — the one hard stop in both elimination loops

**`pivotAbs` is the ONE hard stop shared by
BOTH elimination loops that decide the rank/DOF/free-point verdicts** —
`rankAnalysisOfMatrix` here and `Sketch.movableVars` (`diagnose.go`) — DEFENCE
IN DEPTH behind `nonFiniteVars` screening the geometry before either loop is
ever reached from a public entry point: it reports 0 for a NaN/Inf matrix entry,
so such an entry reads as the worst possible pivot candidate on both comparisons
a plain `math.Abs` would get backwards, rather than making the result of an
already-poisoned matrix meaningful.

## `diagnose.go` — constraint diagnostics

### Overview and the `CheckConstraint` screens

Constraint diagnostics: `conflictAnalysis` (the shared dependency pass behind
`RedundantConstraints`/`Diagnose`/`Verify`), `Diagnose` (redundant vs
conflicting), `ConflictSet` (a conflicting constraint + the earlier ones it
fights), `CheckConstraint` (pre-commit over-constraint rejection; also the one
place a candidate is screened for ownership and refused with `ErrForeignHandle`,
since probing it would rebind another sketch's constraint — see the aux-var note
under "The parameter model" — and, before that screen even runs, the place a
candidate is screened for CORRUPTION and refused with `ErrCorruptHandle`:
`corruptConstraint` (`parameters.go`, beside `foreignConstraint`) catches a nil
candidate, a typed-nil one (`reflect.ValueOf(c).IsNil()` — the sealed
`Constraint` interface carries no `isNil` method the way `Entity` does, so this
is the one reflection call in the package's production code), and a live one
holding a nil point or entity operand (the same `p == nil`/`isNilEntity` pair
`scanReferenceIntegrity` uses, so the screen cannot diverge from what `Verify`
reports). It runs FIRST because `foreignConstraint` itself dereferences the
candidate through `constraintRefs`, so a foreign-handle screen ahead of it would
panic on exactly the input it exists to catch. Corruption gets its OWN sentinel
because the two defects are INDEPENDENT, never because a corrupt candidate is
always owned: `NewDistance(localPoint, nil, 5)` names only owned geometry, so
`ErrForeignHandle` would be a false statement about it, while
`NewDistance(foreignPoint, nil, 5)` carries BOTH defects and the corrupt screen
— running first — reports it corrupt without ever evaluating the foreign one. So
the single error names the defect that makes the candidate UNREADABLE, never a
claim that it is the only defect the candidate has; re-checking the rebuilt
candidate is what surfaces the ownership one, and `Verify` reports the committed
constraint's foreign handle on its own account (`ForeignHandles`,
`ErrForeignHandle`), so nothing is lost downstream. A caller waiving
`ErrForeignHandle` must not thereby waive a candidate the sketch cannot read at
all, which is the second reason for the split sentinel; `AddConstraint` mirrors
the same split, DROPPING a nil/typed-nil candidate outright (nothing to report)
while still COMMITTING a live candidate with a nil operand, skipping only its
`resolveUnit`/`allocVars` hooks, so `Verify` stays loud about it via
`ErrVerificationIncomplete` — see `constraint_nil_screen_test.go`;
`CheckConstraint` refuses the same way, wrapping `ErrNonFiniteGeometry` before
ever probing the candidate, when the CANDIDATE's own target is non-finite, or
when THIS sketch's own geometry — independent of the candidate — already holds a
non-finite point coordinate, entity shape variable, dimension target, or
constraint-owned auxiliary variable (`Sketch.nonFiniteVars`, `nonfinite.go`):
the probe's rank pass runs the same partial-pivot comparisons
`rankAnalysisOfMatrix` (`solver.go`) does, so a non-finite pivot can rank a
genuine duplicate's row as independent and accept it. The CANDIDATE screen
closes a second, independent false-accept: a `NewDistance(a, b, NaN)` on a
constraint-free sketch passes the probe, since the candidate's own NaN row is
neither correctly selected nor correctly rejected by a plain partial-pivot
comparison — a false bless of a sketch that has no constraints for it to depend
on, with nothing in THIS sketch's geometry poisoned at all. **Both screens sit
BELOW the driven-dimension shortcut and ABOVE the probe**, which is the one
ordering that makes them a statement about a verdict the geometry can reach: the
shortcut returns before `allocVars`, `c.residual` and both `rankAnalysisOf`
calls, and `residuals()` skips a driven dimension, so a driven candidate
contributes no row, is never ranked, and has no verdict for a poisoned pivot to
corrupt — refusing it would report a defect on an answer nothing computed. The
corrupt, foreign-refs and foreign-allocation screens all stay ABOVE the driven
shortcut, and so above both non-finite screens, since those defects are
properties of the candidate itself and hold whatever it drives: a driven
candidate is still refused for them, and a candidate the sketch cannot read or
does not own is named as such rather than reported non-finite.
`TestCheckConstraintDrivenCandidateSkipsNonFiniteScreen`
(`nurbs_nonfinite_test.go`) pins both halves: the driven candidate passes and
the same candidate left driving is still refused),
`FreePoints`/`Point.IsFullyConstrained` (per-point free-DOF attribution) +
`Sketch.EntityIsFullyConstrained` (per-entity: an entity is free when any
defining-point coord OR any intrinsic shape var — whichever ones
`entityShapeVars` in `sketch.go` reports for that type, none for a line, an arc
or the spline families — is in the null-space support). Design in
`docs/diagnostics-design.md`.

### Maximum-ignorance answers for the bare reads

**`FreePoints`, `Diagnose` and
`RedundantConstraints` (`solver.go`) have no error return and so cannot refuse;
on non-finite geometry each answers with a documented MAXIMUM-IGNORANCE value
rather than one computed from a poisoned Jacobian, and the three agree with
`DOF` and with each other.** `FreePoints` reports EVERY point of the sketch,
grounded ones included — mirroring `DOF`'s total variable count (`solver.go`)
exactly — rather than the null-space support `movableVars` would otherwise
compute. Excluding the grounded points is NOT that value, for the same reason
the free-variable count is not `DOF`'s: it collapses to EMPTY on an all-grounded
sketch, the reading a caller gating on `len(FreePoints())==0` takes as fully
constrained, on exactly the fixture the screen exists for. The sketch's own
origin stays excluded, as it is on the analysed path — it is not in `s.points`,
and no public call can move it. `Diagnose` reports EVERY constraint the
dependency analysis could ever flag (`unprovenConstraints`, mirroring
`residuals()`'s iteration so driven dimensions — which contribute no row and so
can never be flagged — are skipped) as `Conflicting`, with `Redundant` empty,
and `RedundantConstraints` returns that same set so the flat list and the
partition `Diagnose` refines it into cannot name two different things. An empty
`Diagnosis` is NOT that value and is the reason the screen is there: a caller
reads two empty lists as "no constraint problem found", the one direction a
poisoned dependency analysis must never report. `Conflicting` rather than
`Redundant` because "removing one changes nothing" is itself a claim about
geometry nothing here can support, while `Conflicting` says only that no
constraint has been proven consistent. All of these are deliberate design
choices, not oversights: `Sketch.Verify` is the call that reports the condition
itself
(`VerificationReport.NonFinitePoints`/`NonFiniteEntities`/`NonFiniteDimensions`/`NonFiniteConstraints`);
these bare reads only guarantee they never answer in the BLESSING direction.

### The two per-handle reads

**The two PER-HANDLE reads — `Point.IsFullyConstrained` and
`Sketch.EntityIsFullyConstrained` — take the same stance and report `false` on
non-finite geometry**, which is the answer that agrees with the aggregate reads
rather than contradicting them: both otherwise answer from the null-space
support the poisoned Jacobian produces, so on the very fixture where
`FreePoints` names every point and `DOF` reports the total variable count, the
per-handle pair certified "nothing here can move" for geometry nothing analysed.
`false` is already each one's documented refusal for a foreign, removed or nil
handle, so the two conditions share one answer. The screen applies uniformly,
`Sketch.Origin()` and origin-drawn geometry included — `FreePoints` excludes the
origin only because it is not in `s.points`, which is a statement about which
slice is walked and not an exemption.
`TestNonFinitePerHandleReadsAreMaximumIgnorance` and its finite control
`TestFinitePerHandleReadsAreUnchanged` (`nurbs_nonfinite_test.go`) pin both
halves over the all-grounded and the constrained fixture.

### `EntityIsFullyConstrained` screens its handle first

**`EntityIsFullyConstrained` SCREENS its handle through `foreignInput` first**,
the same predicate the grounding API and the modification tools use, so this
read cannot diverge from what `Verify` reports. A variable index means something
only in the sketch that allocated it, and this method answers by looking the
handle's indices up in THIS sketch's null-space support — so unscreened, a
foreign entity is judged by unrelated local variables (a genuinely 3-DOF foreign
circle read *fully constrained* through a DOF-0 sketch, whose support is empty),
a removed one by retired variables, and a nil or TYPED-nil one panicked in
`entityPoints` (reachable in one line: `Sketch.EntityByName` returns an untyped
nil on a miss). **Every one of those errs toward BLESSING** a handle the sketch
never examined, the direction an oracle must never fail in. The refusal is
`false` — the only answer that does not certify "nothing here can move", and the
same answer the screened `EntityFixed` already gives for those inputs, so the
two bare-bool entity reads agree.

### `movableVars`/`conflictAnalysis` carry the screen

**`Sketch.movableVars` and
`Sketch.conflictAnalysis` are two of the FOUR primitives that CARRY the
non-finite screen in a final return** (see the `verify.go` row), so every
free-point read — `FreePoints`, both per-handle bools, and the DOF colouring in
`annotate.go` — and every dependency read — `Diagnose`, `RedundantConstraints`,
`Verify`'s `Redundant`/`Conflicts` — is screened by the one call it already
makes rather than by a separate `hasNonFiniteVars` beside it. `conflictAnalysis`
screens ABOVE its own zero-row shortcut, so the branch that builds no Jacobian
is covered by the same return and no caller keeps a direct check.

### `pointMovable` — the one null-space-support definition

**`pointMovable` is the ONE definition of "is this point in the null-space
support"**, the point-level twin of `entityMovable`, read by all three, so a
point cannot read free through one and constrained through another.
**`entityMovable` and `pointMovable` stay deliberately UNSCREENED**: every
caller has already screened the handle and the geometry through `movableVars`'s
own second return. `entity_constrained_ownership_test.go` pins the refusal per
input shape (foreign at both index regimes, removed, nil, typed nil, rewired
foreign point), the agreement with `EntityFixed`, and — the load-bearing half —
the owned answers unchanged, origin-drawn geometry included, since `owns`
carries the origin exception.

### `Point.IsFullyConstrained` carries the same guard

**`Point.IsFullyConstrained` carries the same
guard**, `if p == nil \|\| p.s == nil \|\| !p.s.owns(p) { return false }`: a nil
receiver and the externally-constructible zero value `&Point{}` (every field of
`Point` is unexported, but a caller can still write the literal) both panic on
`p.s.movableVars()` without it, and a removed point's
retired-but-never-reclaimed variables would otherwise read *fully constrained*
while its former sketch still has free DOF — the same blessing direction
`EntityIsFullyConstrained` was corrected for. The `p.s == nil` clause is
required specifically for the zero value, since `owns` dereferences `s.origin`
and a nil-receiver call would trade one panic for another. Unlike the entity
twin there is **no foreign-sketch hole**: the method reads through the point's
own sketch (`p.s.movableVars()`), so a point of another sketch is answered
correctly by its own owner at any index — `owns` here only catches
nil/zero-value/dead handles. `point_constrained_ownership_test.go` pins the
refusals (nil, zero value, `PointByName` miss, removed), agreement with
`EntityIsFullyConstrained` on nil and removed inputs, and the owned answers
unchanged (free, pinned, origin, reference point, and a point of another
sketch).

## `verify.go` — the headless-oracle report

### Overview

`Sketch.Verify(ctx context.Context, ...VerifyOption) *VerificationReport`: the
headless-oracle aggregation layer — one non-mutating call gathering solvability,
DOF, `Status`, redundant constraints, conflict sets, free points, profiles +
their validity (`ProfilesValid`/`InvalidProfiles` — self-intersecting/degenerate
regions gate `Trustworthy()`), stale/broken/foreign reference signals, parameter
unit-kind validity (`ParametersValid`), the **advisory** `RankMargin` (how far
the STRUCTURAL rank/DOF decision sits from the rank-zero cutoff — a fragility
hint; now scale-invariant, computed on the nondimensional Jacobian, but still
does NOT gate `Trustworthy()` — it measures a coarser, different question than
conditioning), the **scale-invariant** `Conditioning` (`conditioning.go`: the
reciprocal condition number of the nondimensionalized Jacobian — this one DOES
gate `Trustworthy()`, below a tolerance-derived `max(1e-6, 4·√tol)` threshold),
`Trustworthy()`, and (opt-in via `WithProbe`) discrete ambiguity. A pure
consumer of the diagnostic building blocks.

### The trust verdict has one definition and two shapes

**The trust verdict has ONE
definition and two shapes**: `Check() Reasons` returns one wrapped sentinel per
failed condition, in emission order (`ErrBrokenReference`, `ErrForeignHandle`,
`ErrNonFiniteGeometry`, `ErrVerificationIncomplete`, `ErrUnsolvable`,
`ErrNotFullyConstrained`, `ErrConflicting`, `ErrRedundant`, `ErrStaleReference`,
`ErrInvalidProfile`, `ErrInvalidParameter`, `ErrNearSingular`,
`ErrProbeIncomplete`, `ErrAmbiguous`), and `Trustworthy()` **is** `Check() ==
nil` rather than a second copy of the condition list — so a condition added to
one is added to both and they cannot drift. `Reasons` is `error` + `Unwrap()
[]error`, so `errors.Is` matches through it AND a caller can waive ONE condition
per reason (the reported case: a sketch built from unsigned constraints is
ambiguous by design, and its consumer must still enforce everything else).

### A new condition goes in `Check`, never in `Trustworthy`

**A
new condition goes in `Check`, never in `Trustworthy`**, and it is fatal by
default for every waiving caller, since it is not in their waiver list — which
is the whole reason a caller must not hand-copy the verdict (a copy silently
stops checking what is added next, and cannot reproduce the conditioning gate at
all: `condGate` is unexported).

### Every reason wraps its sentinel

**Every reason WRAPS its sentinel** —
`fmt.Errorf("%w: …", sentinel)` carrying that condition's own specifics (the
residual, the counts, the probe's error) — never the bare sentinel value, which
answers `errors.Is` by identity while `errors.Unwrap` returns nil, breaking the
`Reasons.Unwrap` contract.

### `Check` asserts only conditions `Verify` evaluated

**`Check` asserts only conditions `Verify` actually
evaluated**: a nil, corrupt or foreign handle stops `Verify` at the
reference-integrity scan (it would panic the residual/profile passes),
non-finite geometry stops it the same way, right beside that scan, and staleness
reports on the skipped path per the `scanReferenceStaleness` note below, which
owns that ordering and its one exception. In every case the fields the skipped
passes never wrote hold zero values that are NOT verdicts, and reporting them
would block a caller waiving `ErrForeignHandle` on failures nobody tested. The
verdict still fails on that path in every case, including the one where a nil
constraint operand is the only finding, since `ErrVerificationIncomplete` is
itself a failed condition. `Check` returns a **literal nil** on the clean path —
never a nil `*reasons` inside a non-nil interface, which would make `err != nil`
true for a sound sketch (`verify_check_test.go` pins it).

### Non-finite geometry is screened on the geometry

**Non-finite geometry
(`nonfinite.go`) is screened on the GEOMETRY, never on a Jacobian built from
it** — a Jacobian-level guard cannot catch the sharpest case, a DOF-0 candidate
whose every point is fixed, which builds a perfectly finite zero-column matrix
and hits `conditioning`'s own `len(free)==0` shortcut (+Inf, maximal trust)
without ever looking at a value. `Sketch.nonFiniteVars` scans `s.vars` directly
for a NaN or infinite point coordinate, entity shape variable, dimension target,
or constraint-owned auxiliary variable (a spline foot parameter, a tangency
slack, a conic-tangency contact witness — enumerated per constraint type by
`auxVars`, `nonfinite.go`, mirroring `entityShapeVars`), regardless of
`s.fixed`: a FIXED poisoned point still poisons the finite-difference centroid
every rank/DOF/conditioning pass subtracts (`positionShift`'s centroid is taken
over ALL points), so it still corrupts the analysis of every OTHER point's
constraints, and the partial-pivot elimination that decides rank
(`rankAnalysisOfMatrix`, `movableVars`) reads a non-finite pivot two different
ways depending on where it lands — never selected (an undercount) or, once
selected, never rejected either (an overcount) — so no rank computed from a
poisoned matrix is trustworthy in either direction. An aux variable is seeded
FINITE at allocation time but is then a free unknown the solver moves like any
other, so it can go non-finite (e.g. an extreme-but-finite `ArcLength` target
driving its unwrapped-sweep `theta` to NaN) with every authored point, entity
and dimension target still finite — a gap the three-source scan alone could not
see. `Verify` runs the scan beside `scanReferenceIntegrity` and takes the SAME
early-out a foreign handle already does: the finding is recorded on
`NonFinitePoints`/`NonFiniteEntities`/`NonFiniteDimensions`/`NonFiniteConstraints`,
`Check` reports it as `ErrNonFiniteGeometry` — the same sentinel the exporters
already return for the analogous defect in their own output (see the
`svg.go`/`png.go`/`dxf.go` row) — ahead of `ErrVerificationIncomplete`, and the
skipped analysis leaves `DOF`/`FreePoints`/`Profiles`/`Conditioning` at their
unevaluated zero values exactly as a foreign handle does.

### `Conditioning` is initialized below the early-out

**`Conditioning` is
initialized BELOW the early-out, never on the report literal, and that placement
is load-bearing on BOTH causes**: `+Inf` is this field's "not applicable" value
and therefore its BEST possible reading, so set on the literal it survives the
skip and has a report that evaluated nothing carry maximal trust in the one
field the gate reads — the blessing direction, and a contradiction of the report
doc's own promise that every unevaluated field holds its zero value.

### `Status` stays at its zero value on the skipped path

**`Status`
leaves at its zero value on the skipped path too, exactly like every other
unevaluated field, on BOTH early-out causes** — a foreign handle and non-finite
geometry alike. `Overconstrained` is not that value: its own doc comment says
redundant or conflicting constraints are present, and `Conflicts`/`Redundant`
both read empty on the same report.
`TestCheckSkippedAnalysisReportsOnlyWhatRan`'s foreign-handle subtest and
`TestAllFixedNaNPointIsNoLongerFalselyBlessed`'s non-finite fixture
(`nurbs_nonfinite_test.go`) both pin `Status` at the zero value,
`Underconstrained`, on the skipped path. `Sketch.CheckConstraint`
(`diagnose.go`) refuses the same candidate-independent way, before ever probing
the candidate, since its rank probe is exactly as vulnerable to a non-finite
pivot; `DOF`, `FreePoints` and `Diagnose` have no error return and so cannot
refuse, and instead answer with a documented MAXIMUM-IGNORANCE value (see the
`solver.go`/`diagnose.go` rows) rather than a number computed from a poisoned
matrix.

### The four primitives that carry the non-finite screen

**THE SCREEN IS CARRIED BY THE FOUR PRIMITIVES EVERY VERDICT IN THIS
FAMILY DERIVES FROM, never by each reader calling it**: `Sketch.movableVars`
(`diagnose.go`), `Sketch.rank`/`committedRankAnalysis` (`solver.go`),
`Sketch.conditioning` (`conditioning.go`) and `Sketch.conflictAnalysis`
(`diagnose.go`, the dependency pass behind
`Diagnose`/`RedundantConstraints`/`Verify`'s `Redundant`/`Conflicts`) each
return a final `ok`, false exactly when `hasNonFiniteVars` holds, so **a new
reader must ACCOUNT for the screen at the call site** instead of never meeting
it — the same enforcement shape the sealed `Entity.isNil` uses at the
`addEntity` funnel, and the only one Go offers, since it can require a second
return be handled but not that a method be called. Its enforcement stops there:
a reader may still discard the screen with the blank identifier, so this is a
chokepoint that makes the screen unmissable, NOT a proof that no reader can
ignore it. Each caller then translates `ok=false` into ITS OWN non-blessing
answer — a refusal where there is an error return (`CheckConstraint`,
`ProbeConfigurations`), the documented not-computed sentinel where there is one
(`Result.DOF`), the maximum-ignorance value where there is neither (`DOF`,
`FreePoints`), `false` for the per-handle bools, everything drawn free for the
DOF colouring — and those answers are legitimately different and deliberately
NOT unified; what is unified is the fact behind them. The ONE branch that must
ask the screen directly is a caller whose rank pass does not run at all (no
residual rows: `DOF`, `Solve`, `ProbeConfigurations`), since there is no second
return to carry it there; `conflictAnalysis` has that branch INSIDE itself and
screens above it, so its callers need no direct check. `conditioning` is the
sharpest of them, since its own "nothing to measure" answer is `+Inf` — its BEST
reading, and what the all-fixed zero-column matrix produces.

### The scan covers every dimension target, driven ones included

**The scan covers
EVERY dimension target, DRIVEN ONES INCLUDED**, which the poisoned-analysis
argument does not imply: `residuals()` skips a driven dimension, so its target
never reaches a Jacobian, but it is still a value no writer can write —
`MarshalJSON` fails on it and the exporters refuse it — and with driven targets
excluded `Verify` reported such a sketch fully constrained and TRUSTWORTHY,
breaking the serialization row's stated invariant that a sketch its report
blesses always marshals. It is also permanent: `refreshDriven` recomputes
`d.base()+r[0]`, both non-finite. The accepted cost is that the bare reads
answer with maximum ignorance, and `Verify` skips its analysis, for a sketch
whose rank pass would in fact have been sound — the conservative direction, and
the alternative (a second, narrower notion of "non-finite") gives one fact two
answers. `CheckConstraint` keeps its driven-CANDIDATE exemption, a DIFFERENT
question: an uncommitted driven candidate is never ranked, so no verdict about
IT is computed.

### `scanReferenceStaleness` runs before the early-out

**`scanReferenceStaleness` runs BEFORE the early-out, on both
non-nil-corrupt skip causes**, since it reads snapshot provenance flags neither
a foreign handle nor a non-finite value can corrupt — and its zero value is the
BLESSING one (`Stale=false` reads "verified against a current 3D snapshot"), so
skipping it turned a genuinely stale reference into a clean one with nothing
left to say otherwise. It still sits behind the NIL-corrupt cause, the one thing
that can panic it (an entity's staleness derives from its defining points), so
on that cause alone the staleness trio is left at its zero value and `Check`
reports no `ErrStaleReference` even against a genuinely stale reference.
`ErrStaleReference` is accordingly in `Check`'s pre-analysis reason group.

### `VerificationReport.Analysed()` — the exported form of the skip

**`VerificationReport.Analysed()` is the exported form of the skip**, since a
report is not self-describing: every field the skipped passes never wrote holds
a zero value that is ALSO a legitimate analysed reading (`DOF` 0 is fully
constrained, `Status` is `Underconstrained`, `Conditioning` 0 is singular), so
an external consumer that renders or gates on any of them has no other way to
know. It is exactly "no `ErrVerificationIncomplete` among the reasons";
`writeStatusBadge` reads it.

## `probe.go` — the ambiguity probe

### Overview

`Sketch.ProbeConfigurations`: multi-solution ambiguity probe — deterministic
multi-start search (structured mirrors + splitmix64 restarts) for the discrete
configurations a DOF-0 sketch admits. A falsifier: ≥2 found proves ambiguity, 1
never proves uniqueness. Design in `docs/ambiguity-probe-design.md`.

### The probe refuses on non-finite geometry

**It
REFUSES with `ErrNonFiniteGeometry` on non-finite geometry**, taking its DOF-0
precondition from `rank`'s screened second return (and asking the screen
directly on the zero-row branch, which never reaches that pass): the
precondition and every configuration the search then accepts are re-solves of
the same poisoned geometry. It HAS an error return, so it refuses the way
`CheckConstraint` does rather than fabricate a verdict — which is also what
makes the direct call agree with `Verify(WithProbe)`, which already never
reaches it on this state.

### `Configuration.PointXY` screens its point

**`Configuration.PointXY` SCREENS its point through
`owns` before indexing the configuration's captured variable vector by its
`xi`/`yi`** — the same predicate the grounding API, `scanReferenceIntegrity` and
`foreignInput` use, so this read cannot diverge from what `Verify` reports, and
it carries the origin exception so `Sketch.Origin()` still reads correctly.
Those indices only mean anything in the sketch the configuration came from:
unscreened, a foreign point can alias one of this sketch's variables at a small
index (silently reading the wrong coordinates — the sharpest case, a foreign
point aliasing a GROUNDED local variable, reads identical across every
configuration, so a consumer diffing two configurations for ambiguity reads a
false "no difference"), run off the captured vector at a large index (a panic in
a documented non-panicking call), or be a point this sketch has since removed
(answering from a retired slot, or from whatever point now occupies its old id —
`owns` catches this by identity, not by index range). The refusal is `(NaN,
NaN)`, the float analog of the bare-bool refusals elsewhere (`EntityFixed`,
`EntityIsFullyConstrained`): every comparison against NaN is false, so a
consumer diffing configurations reads "different", the safe direction, never a
false "identical". `probe_ownership_test.go` pins the refusal at both index
regimes and against a removed point, and — the load-bearing half — an owned
point, `s.Origin()`, and every point of the sketch reading unchanged across
every configuration the probe returns.
