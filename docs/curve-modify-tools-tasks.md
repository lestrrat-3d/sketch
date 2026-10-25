# Curve Modification Tools — Implementation Tasks

This file turns `docs/curve-modify-tools-design.md` into thirteen PRs, increments 0 to 12. Each section is
self-contained: follow it top to bottom. Every edit below was generated from a prototype copy of this repository in
which all thirteen increments were built in order; `go test ./...`, `go vet ./...` and `golangci-lint run ./...`
passed after each one. Every edit and every `grep -c` check was then replayed, in this file's exact text, three
times: onto a clean copy of `origin/main`, where each increment reproduced the prototype byte for byte; with the six
creation-variants increments landing first; and with them landing between increments 8 and 9. Every anchor matched
exactly as this file says, and `go test ./...` and `golangci-lint` passed on each final tree. Copy the code
exactly. Do not redesign, rename, reorder or "improve" anything.

## Rules for every increment

### Stop and report

Stop and report to whoever gave you the task, instead of improvising, when any of these happens:

- a `grep -c` check prints a number other than the one shown after `# expect`;
- an `Edit` old string does not match the file exactly, or matches more than once;
- a command in this file fails, prints an unexpected result, or a test fails;
- `gofmt -l .` prints a file you did not touch, or `golangci-lint` reports an issue you cannot fix by re-copying the
  code from this file;
- the previous increment in the merge order is not merged yet (Step N.0 checks this);
- anything in the repository contradicts this file.

The report names the increment, the step, the exact command and its output. Do not change tests or expected numbers
to make them pass. Do not skip a step. Do not "fix" an anchor that no longer matches by editing nearby text instead:
another PR changed that text, and the person who gave you the task decides what happens next.

### Paths

- `PROJECT` is `/home/lestrrat/dev/src/github.com/lestrrat-3d/sketch`.
- Each increment works in its own git worktree at `$PROJECT/.worktrees/<branch>`. Never edit the root checkout.
- Pass absolute worktree paths to Read, Edit and Write. A `cd` in Bash does not change where those tools write.
- Store command output you need to re-read in the worktree's `.tmp/` directory (`mkdir -p .tmp` first). Never write
  to `/tmp`.

### Create the worktree

Replace `<branch>` with the increment's branch name:

```sh
git -C /home/lestrrat/dev/src/github.com/lestrrat-3d/sketch fetch origin
git -C /home/lestrrat/dev/src/github.com/lestrrat-3d/sketch worktree add \
  /home/lestrrat/dev/src/github.com/lestrrat-3d/sketch/.worktrees/<branch> -b <branch> origin/main
```

Run every later command from that worktree directory.

### Read before editing

Every increment reads these first, in the worktree:

- `CLAUDE.md`;
- `.claude/docs/sketch-core.md` (the `tools.go` section) and `.claude/docs/constraints.md`;
- `docs/curve-modify-tools-design.md`, the section named in the increment;
- `~/.claude/docs/go.md` and `~/.claude/docs/git-operations.md`.

### How to apply each kind of step

- **`grep -c` checks.** Run each line as shown, from the worktree root. Each prints a count; compare it with the
  number after `# expect`. Run all of a step's checks before its first edit.
- **"Replace exactly:"** Use the Edit tool on the step's file. `old_string` is the content of the first code block
  and `new_string` the content of the block after "with:", character for character, tabs included, without the
  ``` fence lines. An empty block means an empty string. A block whose first or last line is empty includes that
  empty line: copy it too.
- **"Replace exactly (every occurrence; there are exactly K):"** Same, with `replace_all` set to true. Before the edit,
  count the occurrences with the Grep tool on the first line of the block; stop if it is not K.
- **"Run exactly:"** Run each line of the block, in order, from the worktree root.
- **"new file `PATH`":** Use the Write tool to create `PATH` with the block's content followed by one newline.

### Drift: other PRs may land first

The creation-variants increments (`docs/creation-variants-tasks.md` on their branch) edit `compound.go`, `CLAUDE.md`,
`docs/fusion-gap-analysis.md`, `revision_internal_test.go`, `constraint.go`, `conditioning.go` and `geom/modify.go`,
and may merge before or between the increments below. Every edit in this file is anchored on text those PRs do not
change, and this file was replayed on a copy carrying all of them. Still:

1. Start every increment with Step N.0: `git fetch origin` and `git rebase origin/main`.
2. Run each step's `grep -c` checks right before editing that file, after the rebase.
3. If a count differs, or an Edit reports no match or several matches, stop and report. Never edit by line number.

### Verify

Each increment lists its focused test commands. Then run, from the worktree root:

```sh
gofmt -l .
go vet ./...
golangci-lint run ./...
go run ./internal/cmd/genreadme README.md docs/guide/units-and-parameters.md docs/guide/solving.md
git diff --exit-code -- README.md docs/guide
go run ./internal/cmd/genimages docs/images
git status --short docs/images
```

Expected: `gofmt -l .` prints nothing (if it lists a file you touched, run `gofmt -w <file>` once and re-run);
`go vet` prints nothing; `golangci-lint` prints `0 issues.`; the `git diff` exits 0; the last command prints nothing,
except in increment 2, which says what it prints. If `golangci-lint` is not on `PATH`, use
`~/go/bin/golangci-lint`.

Do not run `go generate ./...`. Do not run the full `go test ./...` locally; CI runs it.

### Commit

1. Write the increment's commit message to `.tmp/commit-msg.txt` (one line, as given).
2. `~/.claude/scripts/git-lint-message commit .tmp/commit-msg.txt` must print `ok`.
3. `git add` exactly the files the increment lists, then `git commit -F .tmp/commit-msg.txt`.
4. `git status --short` must show nothing staged or modified (untracked `.tmp/` is ignored).

The message has no body, no `Co-Authored-By` line and no AI or agent attribution of any kind.

### Pull request

1. Write the title (given) to `.tmp/pr-title.txt` and the body (given) to `.tmp/pr-body.md`.
2. `~/.claude/scripts/git-lint-message pr-title .tmp/pr-title.txt` and
   `~/.claude/scripts/git-lint-message pr-body .tmp/pr-body.md` must both print `ok`.
3. `git push -u origin <branch>`.
4. `gh pr create --title "$(cat .tmp/pr-title.txt)" --body-file .tmp/pr-body.md`.
5. Watch CI: `gh pr checks <pr-number> --watch --fail-fast --interval 30` (Bash timeout 600000 ms; if it times out,
   run it again). Any failed check → stop and report, with `gh run view <run-id> --log-failed >
   .tmp/gh-ci-<run-id>.log` saved first.
6. Report the PR URL and the CI result. Do not merge; merging is decided outside this task.

The PR body is 1 to 3 bullets, has no "Test plan" section, and has no AI or agent attribution line.

## Merge order and dependencies

| # | Branch | Merge after |
|---|---|---|
| 0 | `refactor-tools-package-functions` | — |
| 1 | `feat-trim-extend-in-place` | 0 |
| 2 | `feat-fillet-chamfer-keep-legs` | 1 |
| 3 | `feat-trim-pin-to-cutter` | 2 |
| 4 | `feat-point-on-normal` | 3 |
| 5 | `feat-offset-fusion-lines` | 4 |
| 6 | `feat-trim-arc-circle` | 5 |
| 7 | `feat-extend-arc` | 6 |
| 8 | `feat-geom-fillet-curves` | 7 |
| 9 | `feat-fillet-arcs` | 8, and the creation-variants PR `fix rank of shared-endpoint arc tangency` |
| 10 | `feat-radial-offset` | 9 |
| 11 | `feat-offset-circles-arcs` | 10 |
| 12 | `feat-offset-chains` | 11 |

Merge in the order 0 to 12. Start an increment only after every increment before it is merged into `origin/main`:
the edits below are written against the text the previous increment leaves. Step N.0 of each increment checks this
with `git log`.

## Increment 0 — tools become package-level functions

- **Branch:** `refactor-tools-package-functions`
- **Design section:** "Increment 0 — tools become package-level functions".
- **Merge after:** — (first in the order)
- **Files:** `tools.go`, `sketch.go`, `tools_test.go`, `tools_ownership_test.go`, `tools_unsupported_test.go`,
  `tools_fitspline_test.go`, `reference_test.go`, `revision_internal_test.go`,
  `examples/sketch_fillet_example_test.go`, `internal/cmd/genimages/main.go`, `tools_nil_sketch_test.go`, `CLAUDE.md`,
  `.claude/docs/sketch-core.md`, `docs/modification-tools-design.md`, `docs/acceptance-tests.md` (edits;
  `tools_nil_sketch_test.go` is new).

This increment changes about 105 call sites. Do them in the order of the steps: the production code first (Steps 0.1
and 0.2), then one mechanical `gofmt -r` rewrite (Step 0.3), then the hand edits for the calls whose result changed
shape (Steps 0.4 to 0.11), then the new test (Step 0.12) and the docs (Steps 0.13 to 0.16). Until Step 0.11 is done
the test packages do not compile; that is expected.

### Step 0.0 — sync and check the base

From the worktree root:

```sh
git fetch origin
git rebase origin/main
```

If the rebase stops, or a count is 0, stop and report. Then do the steps below in order. Every step starts
with its own `grep` checks: run them before editing that file.

### Step 0.1 — `tools.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '// Sketch-modification tools mutate committed geometry by the build-then-replace' tools.go   # expect 1
grep -c -F -- '// cannot reproduce. Only lines, circles, arcs and fit splines are copied: every' tools.go   # expect 1
grep -c -F -- 'func (s *Sketch) foreignInput(ents ...Entity) bool {' tools.go   # expect 1
grep -c -F -- '// or foreign) or is defined by a point that is not a live point of this sketch.' tools.go   # expect 1
grep -c -F -- 'if param <= epsTool || param >= 1-epsTool {' tools.go   # expect 1
grep -c -F -- 'return l1, l2, true' tools.go   # expect 1
grep -c -F -- 'return nil, nil, false' tools.go   # expect 5
grep -c -F -- '// sketch (nil, dead, or foreign) or has an endpoint that is not a live point of' tools.go   # expect 1
grep -c -F -- '// returning the lengthened line. It returns false (changing nothing) when end' tools.go   # expect 1
grep -c -F -- '// the two shortened legs that meet it, the contact points T1 (on L1) and T2' tools.go   # expect 1
grep -c -F -- '// rounding tangent. The original legs (and any constraints that referenced them' tools.go   # expect 1
grep -c -F -- 'return &Fillet{Arc: nArc, L1: nL1, L2: nL2, T1: t1, T2: t2, Radius: rad}, nil' tools.go   # expect 1
grep -c -F -- '// live entity of this sketch (nil, dead, or foreign) or has an endpoint that is' tools.go   # expect 1
grep -c -F -- '// Mirror groups the geometry created by [Sketch.CreateMirror]: the sour' tools.go   # expect 1
grep -c -F -- '// arcs are reversed to stay counter-clockwise. The sources are left untouched.' tools.go   # expect 1
grep -c -F -- 'return &Mirror{Originals: ents, Copies: grp.Instances, Constraints: grp.' tools.go   # expect 1
grep -c -F -- '// [Sketch.CreatePatternCircular]: the seed entities (left in place), every copy' tools.go   # expect 1
grep -c -F -- '// It returns [ErrForeignEntity] if a seed entity, or a point defining one, is' tools.go   # expect 1
grep -c -F -- '// [ErrForeignEntity] if a seed entity, a point defining one, or the center is' tools.go   # expect 1
grep -c -F -- '// start and end must be distinct, live points of this sketch other than center,' tools.go   # expect 1
grep -c -F -- '// (see [Sketch.CreatePatternCircularRing]).' tools.go   # expect 1
grep -c -F -- '// OffsetGroup groups the geometry created by [Sketch.CreateOffset]: the source' tools.go   # expect 1
grep -c -F -- 'func (s *Sketch) CreateOffset(ents []Entity, d float64) (*OffsetGroup, error) {' tools.go   # expect 1
```

Edit 1. Replace exactly:

```go
// Sketch-modification tools mutate committed geometry by the build-then-replace
// pattern: they read the current geometry via each entity's Geometry snapshot,
// compute the replacement with the geom math layer, build the new geometry from
// sketch points (reusing the originals' surviving points so neighbouring
// geometry stays attached), and retire the originals with RemoveEntity.
// Constraints that referenced a replaced entity by identity are dropped with it
// (the removal cascade); constraints on the surviving shared points are kept.
// Replaced handles are dead.

const epsTool = 1e-9
```

with:

```go
// Sketch-modification tools are package-level functions that take the sketch
// first. They mutate committed geometry by the build-then-replace pattern: they
// read the current geometry via each entity's Geometry snapshot, compute the
// replacement with the geom math layer, build the new geometry from sketch
// points (reusing the originals' surviving points so neighbouring geometry stays
// attached), and retire the originals with RemoveEntity. Constraints that
// referenced a replaced entity by identity are dropped with it (the removal
// cascade); constraints on the surviving shared points are kept. Replaced
// handles are dead. Every tool refuses a nil sketch.

const epsTool = 1e-9

// OpenCurve is the set of curve types the modification tools split, trim,
// extend and fillet: curves with two end points. Use it only as a type
// constraint.
type OpenCurve interface{ *Line | *Arc }

// asEntity returns e as an Entity. A typed-nil e gives a non-nil interface
// holding the nil pointer, which foreignInput refuses.
func asEntity[T OpenCurve](e T) Entity {
	ent, _ := any(e).(Entity)
	return ent
}

// asCurve returns e as the curve type T, or T's zero value when e holds a
// different type.
func asCurve[T OpenCurve](e Entity) T {
	t, _ := any(e).(T)
	return t
}
```

Edit 2. Replace exactly:

```go
	// CreateChamfer, CreatePatternRect, CreatePatternCircular, CreateOffset) when an input is
	// reference geometry, which is externally locked and must not be modified or
	// re-derived. Trim/Extend/Break report it by returning false, and CreateMirror
	// (which has no error return) by returning nil.
	ErrReferenceGeometry = errors.New("sketch: cannot modify reference geometry")
	// ErrUnsupportedEntity is returned by the pattern tools (CreatePatternRect,
	// CreatePatternCircular) when the seed holds an entity kind the copy funnel
	// cannot reproduce. Only lines, circles, arcs and fit splines are copied: every
	// other kind needs the transform's rotation or reflection applied to a shape the
	// point-relinking interface does not carry. CreateMirror (which has no error
	// return) reports it by returning nil.
```

with:

```go
	// CreateChamfer, CreateMirror, CreatePatternRect, CreatePatternCircular,
	// CreateOffset) when an input is reference geometry, which is externally
	// locked and must not be modified or re-derived. Trim/Extend/Break report it
	// by returning false.
	ErrReferenceGeometry = errors.New("sketch: cannot modify reference geometry")
	// ErrUnsupportedEntity is returned by the copy tools (CreateMirror,
	// CreatePatternRect, CreatePatternCircular) when the seed holds an entity kind
	// the copy funnel cannot reproduce, and by CreateFillet for an operand kind it
	// does not fillet. Only lines, circles, arcs and fit splines are copied: every
	// other kind needs the transform's rotation or reflection applied to a shape the
	// point-relinking interface does not carry.
```

Edit 3. Replace exactly:

```go
// a valid tool input.
func (s *Sketch) foreignInput(ents ...Entity) bool {
```

with:

```go
// a valid tool input. A nil sketch owns nothing, so every input is foreign to it.
func (s *Sketch) foreignInput(ents ...Entity) bool {
	if s == nil {
		return true
	}
```

Edit 4. Replace exactly:

```go
// replacing the original with two entities that share a fresh vertex point at
// the split and reporting them. It returns false (changing nothing) when e is
// not a line or arc, or the projection falls outside the segment / arc sweep.
// It also returns false when e is not a live entity of this sketch (nil, dead,
// or foreign) or is defined by a point that is not a live point of this sketch.
// The original entity handle and any constraints that referenced it
// are gone; constraints on the surviving endpoints remain.
func (s *Sketch) Break(e Entity, x, y float64) (Entity, Entity, bool) {
	if s.foreignInput(e) {
		return nil, nil, false // nil, removed, or another sketch's entity
	}
	if e.IsReference() {
		return nil, nil, false // reference geometry is locked; cannot be split
	}
	switch t := e.(type) {
```

with:

```go
// replacing the original with two curves of the same type that share a fresh
// vertex point at the split, and returns them. It returns false (changing
// nothing) when the projection falls outside the segment / arc sweep, when s is
// nil, when e is reference geometry, or when e is not a live entity of s (nil,
// dead, or foreign) or is defined by a point that is not a live point of s. The
// original handle and any constraints that referenced it are gone; constraints
// on the surviving endpoints remain.
func Break[T OpenCurve](s *Sketch, e T, x, y float64) (T, T, bool) {
	var zero T
	ent := asEntity(e)
	if s.foreignInput(ent) {
		return zero, zero, false // nil, removed, or another sketch's entity
	}
	if ent.IsReference() {
		return zero, zero, false // reference geometry is locked; cannot be split
	}
	switch t := ent.(type) {
```

Edit 5. Replace exactly:

```go
		if param <= epsTool || param >= 1-epsTool {
			return nil, nil, false
		}
```

with:

```go
		if param <= epsTool || param >= 1-epsTool {
			return zero, zero, false
		}
```

Edit 6. Replace exactly:

```go
		return l1, l2, true
```

with:

```go
		return asCurve[T](l1), asCurve[T](l2), true
```

Edit 7. Replace exactly:

```go
			return nil, nil, false
```

with:

```go
			return zero, zero, false
```

Edit 8. Replace exactly:

```go
		return a1, a2, true
	}
	return nil, nil, false
}

// Trim shortens line l by removing the portion adjacent to an endpoint up to
// its nearest crossing with another entity, choosing the end nearest the pick
// point (x, y). It returns the shortened replacement line and true, or false
// (changing nothing) when l has no crossing on the picked side, or the pick
// sits on an interior portion bounded by crossings on both sides (which would
// split the line — use Break instead), or when l is not a live line of this
// sketch (nil, dead, or foreign) or has an endpoint that is not a live point of
// this sketch. The original handle is dead.
func (s *Sketch) Trim(l *Line, x, y float64) (*Line, bool) {
	if s.foreignInput(l) {
		return nil, false // nil, removed, or another sketch's line
	}
	if l.IsReference() {
		return nil, false // reference geometry is locked; cannot be trimmed
	}
```

with:

```go
		return asCurve[T](a1), asCurve[T](a2), true
	}
	return zero, zero, false
}

// Trim shortens e by removing the portion adjacent to an endpoint up to its
// nearest crossing with another entity, choosing the end nearest the pick point
// (x, y). It returns the result, of the same type as e, and true. It returns nil
// and false (changing nothing) when s is nil; when e is not a live entity of s
// (nil, dead, or foreign) or has an endpoint that is not a live point of s; when
// e is reference geometry; when e has no crossing on the picked side; or when
// the pick sits on an interior portion bounded by crossings on both sides (use
// [Break]). Lines are handled; an arc returns nil and false. The original handle
// is dead.
func Trim[T OpenCurve](s *Sketch, e T, x, y float64) ([]T, bool) {
	ent := asEntity(e)
	if s.foreignInput(ent) {
		return nil, false // nil, removed, or another sketch's entity
	}
	if ent.IsReference() {
		return nil, false // reference geometry is locked; cannot be trimmed
	}
	l, ok := ent.(*Line)
	if !ok {
		return nil, false // arcs are not trimmed yet
	}
	nl, ok := s.trimLine(l, x, y)
	if !ok {
		return nil, false
	}
	return []T{asCurve[T](nl)}, true
}

// trimLine is Trim for a screened line.
func (s *Sketch) trimLine(l *Line, x, y float64) (*Line, bool) {
```

Edit 9. Replace exactly:

```go
// Extend lengthens line l from the given endpoint (l.Start or l.End) to its
// nearest crossing with another entity beyond that end, replacing l and
// returning the lengthened line. It returns false (changing nothing) when end
// is neither endpoint or no entity lies beyond it, and when l, either of its
// endpoints, or end is not a live handle of this sketch (nil, dead, or
// foreign). The original handle is dead.
func (s *Sketch) Extend(l *Line, end *Point) (*Line, bool) {
	if s.foreignInput(l) || !s.owns(end) {
		return nil, false // nil, removed, or another sketch's handle
	}
	if l.IsReference() {
		return nil, false // reference geometry is locked; cannot be extended
	}
```

with:

```go
// Extend lengthens e from the given endpoint (its Start or End) to its nearest
// crossing with another entity beyond that end, and returns the result, of the
// same type as e. It returns false (changing nothing) when end is neither
// endpoint or no entity lies beyond it; when s is nil; when e is reference
// geometry; and when e, either of its endpoints, or end is not a live handle of
// s (nil, dead, or foreign). Lines are handled; an arc returns false. The
// original handle is dead.
func Extend[T OpenCurve](s *Sketch, e T, end *Point) (T, bool) {
	var zero T
	ent := asEntity(e)
	if s.foreignInput(ent) || !s.owns(end) {
		return zero, false // nil, removed, or another sketch's handle
	}
	if ent.IsReference() {
		return zero, false // reference geometry is locked; cannot be extended
	}
	l, ok := ent.(*Line)
	if !ok {
		return zero, false // arcs are not extended yet
	}
	nl, ok := s.extendLine(l, end)
	if !ok {
		return zero, false
	}
	return asCurve[T](nl), true
}

// extendLine is Extend for a screened line.
func (s *Sketch) extendLine(l *Line, end *Point) (*Line, bool) {
```

Edit 10. Replace exactly:

```go
// Fillet groups the geometry created by [Sketch.CreateFillet]: the rounding arc,
// the two shortened legs that meet it, the contact points T1 (on L1) and T2
// (on L2), and the editable radius dimension.
type Fillet struct {
	Arc    *Arc
	L1, L2 *Line
```

with:

```go
// Fillet groups the geometry created by [CreateFillet]: the rounding arc, the
// two shortened legs that meet it (of the same types as the inputs), the contact
// points T1 (on L1) and T2 (on L2), and the editable radius dimension.
type Fillet[A, B OpenCurve] struct {
	Arc    *Arc
	L1     A
	L2     B
```

Edit 11. Replace exactly:

```go
// CreateFillet rounds the corner shared by lines l1 and l2 with a tangent arc of
// radius r. The shared corner is removed and each leg is shortened to its
// contact point; the arc is held tangent to both legs and its radius pinned by
// an editable dimension, so editing [Fillet.Radius] and re-solving keeps the
// rounding tangent. The original legs (and any constraints that referenced them
// as entities — horizontal, angle, …) are replaced; re-apply such constraints
// to the returned [Fillet.L1]/[Fillet.L2] if needed. Constraints on the
// surviving far endpoints are kept. Returns [ErrForeignEntity] when either line
// is not a live entity of this sketch (nil, dead, or foreign) or has an
// endpoint that is not a live point of this sketch, [ErrNoSharedCorner] or
// [ErrFilletInfeasible] — in each case without modifying the sketch.
func (s *Sketch) CreateFillet(l1, l2 *Line, r float64) (*Fillet, error) {
	if s.foreignInput(l1, l2) {
		return nil, ErrForeignEntity
	}
	if l1.IsReference() || l2.IsReference() {
		return nil, ErrReferenceGeometry
	}
	corner := sharedPoint(l1, l2)
	if corner == nil {
```

with:

```go
// CreateFillet rounds the corner where e1 and e2 meet with a tangent arc of
// radius r. corner must be an endpoint of both curves. The corner is removed and
// each leg is shortened to its contact point; the arc is held tangent to both
// legs and its radius pinned by an editable dimension, so editing
// [Fillet.Radius] and re-solving keeps the rounding tangent. The original legs
// (and any constraints that referenced them as entities — horizontal, angle, …)
// are replaced; re-apply such constraints to the returned [Fillet.L1]/[Fillet.L2]
// if needed. Constraints on the surviving far endpoints are kept. Two lines are
// handled; an arc operand returns [ErrUnsupportedEntity]. Returns
// [ErrForeignEntity] when s is nil or either curve or corner is not a live
// handle of s (nil, dead, or foreign) or a curve has an endpoint that is not a
// live point of s, [ErrReferenceGeometry], [ErrNoSharedCorner] or
// [ErrFilletInfeasible] — in each case without modifying the sketch.
func CreateFillet[A, B OpenCurve](s *Sketch, e1 A, e2 B, corner *Point, r float64) (*Fillet[A, B], error) {
	ent1, ent2 := asEntity(e1), asEntity(e2)
	if s.foreignInput(ent1, ent2) || !s.owns(corner) {
		return nil, ErrForeignEntity
	}
	if ent1.IsReference() || ent2.IsReference() {
		return nil, ErrReferenceGeometry
	}
	l1, ok1 := ent1.(*Line)
	l2, ok2 := ent2.(*Line)
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("%w: CreateFillet does not fillet an arc yet", ErrUnsupportedEntity)
	}
	if sharedPoint(l1, l2) != corner {
```

Edit 12. Replace exactly:

```go
	return &Fillet{Arc: nArc, L1: nL1, L2: nL2, T1: t1, T2: t2, Radius: rad}, nil
}

// Chamfer groups the geometry created by [Sketch.CreateChamfer]: the cut line, the
```

with:

```go
	return &Fillet[A, B]{Arc: nArc, L1: asCurve[A](nL1), L2: asCurve[B](nL2), T1: t1, T2: t2, Radius: rad}, nil
}

// Chamfer groups the geometry created by [CreateChamfer]: the cut line, the
```

Edit 13. Replace exactly:

```go
// [Sketch.CreateFillet]. Returns [ErrForeignEntity] when either line is not a
// live entity of this sketch (nil, dead, or foreign) or has an endpoint that is
// not a live point of this sketch, [ErrNoSharedCorner] or
// [ErrChamferInfeasible] — in each case without modifying the sketch.
func (s *Sketch) CreateChamfer(l1, l2 *Line, d float64) (*Chamfer, error) {
```

with:

```go
// [CreateFillet]. Returns [ErrForeignEntity] when s is nil or either line is not
// a live entity of s (nil, dead, or foreign) or has an endpoint that is not a
// live point of s, [ErrReferenceGeometry], [ErrNoSharedCorner] or
// [ErrChamferInfeasible] — in each case without modifying the sketch.
func CreateChamfer(s *Sketch, l1, l2 *Line, d float64) (*Chamfer, error) {
```

Edit 14. Replace exactly:

```go
// Mirror groups the geometry created by [Sketch.CreateMirror]: the source entities
```

with:

```go
// Mirror groups the geometry created by [CreateMirror]: the source entities
```

Edit 15. Replace exactly:

```go
// copied; it returns nil if the seed holds any other entity kind. Copies share a mirror
// point wherever their sources share a vertex, so a connected source chain
// mirrors to a connected copy. Each source point and its copy are tied with
// [NewSymmetric] (about axis); circles additionally get [NewEqualRadius], and
// arcs are reversed to stay counter-clockwise. The sources are left untouched.
// Returns nil if any source or the axis is reference geometry, is not a live
// entity of this sketch (nil, dead, or foreign), or is defined by a point that
// is not a live point of this sketch.
func (s *Sketch) CreateMirror(ents []Entity, axis *Line) *Mirror {
	if s.foreignInput(ents...) || s.foreignInput(axis) {
		return nil // nil, removed, or another sketch's entity
	}
	if containsReference(ents) || axis.IsReference() {
		return nil // reference geometry is locked; cannot be mirrored
	}
	if _, ok := unsupportedSeed(ents); ok {
		return nil // an entity kind the copy funnel cannot reproduce
```

with:

```go
// copied. Copies share a mirror point wherever their sources share a vertex, so a
// connected source chain mirrors to a connected copy. Each source point and its
// copy are tied with [NewSymmetric] (about axis); circles additionally get
// [NewEqualRadius], and arcs are reversed to stay counter-clockwise. The sources
// are left untouched. It returns [ErrForeignEntity] when s is nil or a source,
// the axis, or a point defining one is not live in s; [ErrReferenceGeometry]
// when a source or the axis is reference geometry; and [ErrUnsupportedEntity]
// when the seed holds any other entity kind — in each case without modifying
// the sketch.
func CreateMirror(s *Sketch, ents []Entity, axis *Line) (*Mirror, error) {
	if s.foreignInput(ents...) || s.foreignInput(axis) {
		return nil, ErrForeignEntity
	}
	if containsReference(ents) || axis.IsReference() {
		return nil, ErrReferenceGeometry
	}
	if e, ok := unsupportedSeed(ents); ok {
		return nil, fmt.Errorf("%w: CreateMirror cannot copy %T; only *sketch.Line, *sketch.Circle, *sketch.Arc and *sketch.FitSpline are copied", ErrUnsupportedEntity, e)
```

Edit 16. Replace exactly:

```go
	return &Mirror{Originals: ents, Copies: grp.Instances, Constraints: grp.Constraints}
```

with:

```go
	return &Mirror{Originals: ents, Copies: grp.Instances, Constraints: grp.Constraints}, nil
```

Edit 17. Replace exactly:

```go
// Pattern groups the geometry created by [Sketch.CreatePatternRect] and
// [Sketch.CreatePatternCircular]: the seed entities (left in place), every copy
```

with:

```go
// Pattern groups the geometry created by [CreatePatternRect] and
// [CreatePatternCircular]: the seed entities (left in place), every copy
```

Edit 18. Replace exactly:

```go
// It returns [ErrForeignEntity] if a seed entity, or a point defining one, is
// not live in this sketch, [ErrInvalidShape] if nx or ny is below 1, and
// [ErrUnsupportedEntity] if the seed holds an entity kind other than a line,
// circle, arc or fit spline — only those four can be copied, so the refusal is
// what keeps the limit from silently shrinking the pattern.
func (s *Sketch) CreatePatternRect(ents []Entity, nx, ny int, dx, dy float64) (*Pattern, error) {
```

with:

```go
// It returns [ErrForeignEntity] if s is nil or a seed entity, or a point
// defining one, is not live in s, [ErrInvalidShape] if nx or ny is below 1, and
// [ErrUnsupportedEntity] if the seed holds an entity kind other than a line,
// circle, arc or fit spline — only those four can be copied, so the refusal is
// what keeps the limit from silently shrinking the pattern.
func CreatePatternRect(s *Sketch, ents []Entity, nx, ny int, dx, dy float64) (*Pattern, error) {
```

Edit 19. Replace exactly:

```go
// [ErrForeignEntity] if a seed entity, a point defining one, or the center is
// not a live handle of this sketch, [ErrInvalidShape] if n is below 2, and
// [ErrUnsupportedEntity] if the seed holds an entity kind other than a line,
// circle, arc or fit spline — only those four can be copied, so the refusal is
// what keeps the limit from silently shrinking the pattern.
func (s *Sketch) CreatePatternCircular(ents []Entity, center *Point, n int) (*Pattern, error) {
	return s.createPatternCircular("CreatePatternCircular", ents, center, n, nil, nil)
}

// CreatePatternCircularRing is [Sketch.CreatePatternCircular] for a seed that is
```

with:

```go
// [ErrForeignEntity] if s is nil or a seed entity, a point defining one, or the
// center is not a live handle of s, [ErrInvalidShape] if n is below 2, and
// [ErrUnsupportedEntity] if the seed holds an entity kind other than a line,
// circle, arc or fit spline — only those four can be copied, so the refusal is
// what keeps the limit from silently shrinking the pattern.
func CreatePatternCircular(s *Sketch, ents []Entity, center *Point, n int) (*Pattern, error) {
	return s.createPatternCircular("CreatePatternCircular", ents, center, n, nil, nil)
}

// CreatePatternCircularRing is [CreatePatternCircular] for a seed that is
```

Edit 20. Replace exactly:

```go
// start and end must be distinct, live points of this sketch other than center,
// and each must define at least one seed entity, else [ErrInvalidShape]. All
// the other errors are CreatePatternCircular's, and n below 2 is refused the
// same way.
func (s *Sketch) CreatePatternCircularRing(ents []Entity, center *Point, n int, start, end *Point) (*Pattern, error) {
	if !s.owns(start) || !s.owns(end) {
```

with:

```go
// start and end must be distinct, live points of s other than center, and each
// must define at least one seed entity, else [ErrInvalidShape]. All the other
// errors are CreatePatternCircular's, and n below 2 is refused the same way.
func CreatePatternCircularRing(s *Sketch, ents []Entity, center *Point, n int, start, end *Point) (*Pattern, error) {
	if s == nil || !s.owns(start) || !s.owns(end) {
```

Edit 21. Replace exactly:

```go
// (see [Sketch.CreatePatternCircularRing]).
```

with:

```go
// (see [CreatePatternCircularRing]).
```

Edit 22. Replace exactly:

```go
// OffsetGroup groups the geometry created by [Sketch.CreateOffset]: the source
```

with:

```go
// OffsetGroup groups the geometry created by [CreateOffset]: the source
```

Edit 23. Replace exactly:

```go
// entities in ents are skipped. It returns [ErrForeignEntity] if a source
// entity, or a point defining one, is not live in this sketch, and
// [ErrInvalidShape] if a source line has zero length (no defined offset
// direction).
func (s *Sketch) CreateOffset(ents []Entity, d float64) (*OffsetGroup, error) {
```

with:

```go
// entities in ents are skipped. It returns [ErrForeignEntity] if s is nil or a
// source entity, or a point defining one, is not live in s, and
// [ErrInvalidShape] if a source line has zero length (no defined offset
// direction).
func CreateOffset(s *Sketch, ents []Entity, d float64) (*OffsetGroup, error) {
```

### Step 0.2 — `sketch.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '// error return report the same condition as [Sketch.Trim], [Sketch.Extend] and' sketch.go   # expect 1
```

Edit 1. Replace exactly:

```go
// error-returning modification tools ([Sketch.CreateFillet],
// [Sketch.CreateChamfer], [Sketch.CreatePatternRect],
// [Sketch.CreatePatternCircular], [Sketch.CreateOffset]) when a handle is nil, a
// removed (dead) handle, or belongs to a different sketch. The tools with no
// error return report the same condition as [Sketch.Trim], [Sketch.Extend] and
// [Sketch.Break] do — false — or, for [Sketch.CreateMirror], nil.
```

with:

```go
// error-returning modification tools ([CreateFillet], [CreateChamfer],
// [CreateMirror], [CreatePatternRect], [CreatePatternCircular],
// [CreatePatternCircularRing], [CreateOffset]) when the sketch is nil or a
// handle is nil, a removed (dead) handle, or belongs to a different sketch. The
// tools with no error return ([Trim], [Extend], [Break]) report the same
// condition by returning false.
```

### Step 0.3 — rewrite the old method calls in five test files

`gofmt -r` rewrites every call `x.Method(args)` to `sketch.Method(x, args)` mechanically. Run it only on the five
files named below; other files are edited by hand in the next steps.

Run exactly:

```sh
gofmt -r 'x.Break(a, b, c) -> sketch.Break(x, a, b, c)' -w tools_test.go tools_ownership_test.go tools_unsupported_test.go tools_fitspline_test.go reference_test.go
gofmt -r 'x.Trim(a, b, c) -> sketch.Trim(x, a, b, c)' -w tools_test.go tools_ownership_test.go tools_unsupported_test.go tools_fitspline_test.go reference_test.go
gofmt -r 'x.Extend(a, b) -> sketch.Extend(x, a, b)' -w tools_test.go tools_ownership_test.go tools_unsupported_test.go tools_fitspline_test.go reference_test.go
gofmt -r 'x.CreateChamfer(a, b, c) -> sketch.CreateChamfer(x, a, b, c)' -w tools_test.go tools_ownership_test.go tools_unsupported_test.go tools_fitspline_test.go reference_test.go
gofmt -r 'x.CreateMirror(a, b) -> sketch.CreateMirror(x, a, b)' -w tools_test.go tools_ownership_test.go tools_unsupported_test.go tools_fitspline_test.go reference_test.go
gofmt -r 'x.CreatePatternRect(a, b, c, d, e) -> sketch.CreatePatternRect(x, a, b, c, d, e)' -w tools_test.go tools_ownership_test.go tools_unsupported_test.go tools_fitspline_test.go reference_test.go
gofmt -r 'x.CreatePatternCircular(a, b, c) -> sketch.CreatePatternCircular(x, a, b, c)' -w tools_test.go tools_ownership_test.go tools_unsupported_test.go tools_fitspline_test.go reference_test.go
gofmt -r 'x.CreatePatternCircularRing(a, b, c, d, e) -> sketch.CreatePatternCircularRing(x, a, b, c, d, e)' -w tools_test.go tools_ownership_test.go tools_unsupported_test.go tools_fitspline_test.go reference_test.go
gofmt -r 'x.CreateOffset(a, b) -> sketch.CreateOffset(x, a, b)' -w tools_test.go tools_ownership_test.go tools_unsupported_test.go tools_fitspline_test.go reference_test.go
```

The commands print nothing. The calls whose result changed shape (`Break`, `Trim`, `CreateMirror`, `CreateFillet`)
still fail to compile; Steps 0.4 to 0.8 fix each one.

### Step 0.4 — `tools_test.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'l1, l2 := e1.(*sketch.Line), e2.(*sketch.Line)' tools_test.go   # expect 1
grep -c -F -- 'e1, e2, ok := sketch.Break(s, arc, 4, 4) // pick near the 45° point' tools_test.go   # expect 1
grep -c -F -- 'e1, _, ok := sketch.Break(s, l, 6, 0)' tools_test.go   # expect 1
grep -c -F -- 'nl, ok := sketch.Trim(s, h, 4, 0) // pick near the right (End) stub' tools_test.go   # expect 1
grep -c -F -- 'nl, ok := sketch.Trim(s, h, -4, 0) // pick near the left (Start) stub' tools_test.go   # expect 1
grep -c -F -- 'f, err := s.CreateFillet(l1, l2, 3)' tools_test.go   # expect 1
grep -c -F -- '_, err := s.CreateFillet(l1, l2, 1)' tools_test.go   # expect 1
grep -c -F -- '_, err := s.CreateFillet(l1, l2, 100) // radius far larger than the legs' tools_test.go   # expect 1
grep -c -F -- '_, err := s.CreateFillet(l1, l2, 3)' tools_test.go   # expect 1
grep -c -F -- 'require.Len(t, m.Copies, 1)' tools_test.go   # expect 1
grep -c -F -- 'm := sketch.CreateMirror(s, []sketch.Entity{src}, axis)' tools_test.go   # expect 2
grep -c -F -- 'd := sketch.NewHorizontalDistance(p1, p2, 3)' tools_test.go   # expect 1
grep -c -F -- 'm := sketch.CreateMirror(s, []sketch.Entity{c}, axis)' tools_test.go   # expect 1
grep -c -F -- 'm := sketch.CreateMirror(s, []sketch.Entity{l1, l2}, axis)' tools_test.go   # expect 1
grep -c -F -- 'sketch.CreateMirror(s, []sketch.Entity{src}, axis)' tools_test.go   # expect 3
```

Edit 1. Replace exactly:

```go
	e1, e2, ok := sketch.Break(s, l, 4, 0)
	require.True(t, ok)
	l1, l2 := e1.(*sketch.Line), e2.(*sketch.Line)
```

with:

```go
	l1, l2, ok := sketch.Break(s, l, 4, 0)
	require.True(t, ok)
```

Edit 2. Replace exactly:

```go
	e1, e2, ok := sketch.Break(s, arc, 4, 4) // pick near the 45° point
	require.True(t, ok)
	a1, a2 := e1.(*sketch.Arc), e2.(*sketch.Arc)
```

with:

```go
	a1, a2, ok := sketch.Break(s, arc, 4, 4) // pick near the 45° point
	require.True(t, ok)
```

Edit 3. Replace exactly:

```go
	e1, _, ok := sketch.Break(s, l, 6, 0)
	require.True(t, ok)
	l1 := e1.(*sketch.Line)
```

with:

```go
	l1, _, ok := sketch.Break(s, l, 6, 0)
	require.True(t, ok)
```

Edit 4. Replace exactly:

```go
	nl, ok := sketch.Trim(s, h, 4, 0) // pick near the right (End) stub
	require.True(t, ok)
```

with:

```go
	res, ok := sketch.Trim(s, h, 4, 0) // pick near the right (End) stub
	require.True(t, ok)
	require.Len(t, res, 1)
	nl := res[0]
```

Edit 5. Replace exactly:

```go
	nl, ok := sketch.Trim(s, h, -4, 0) // pick near the left (Start) stub
	require.True(t, ok)
```

with:

```go
	res, ok := sketch.Trim(s, h, -4, 0) // pick near the left (Start) stub
	require.True(t, ok)
	require.Len(t, res, 1)
	nl := res[0]
```

Edit 6. Replace exactly:

```go
	f, err := s.CreateFillet(l1, l2, 3)
```

with:

```go
	f, err := sketch.CreateFillet(s, l1, l2, corner, 3)
```

Edit 7. Replace exactly:

```go
	_, err := s.CreateFillet(l1, l2, 1)
```

with:

```go
	_, err := sketch.CreateFillet(s, l1, l2, l1.End, 1)
```

Edit 8. Replace exactly:

```go
	_, err := s.CreateFillet(l1, l2, 100) // radius far larger than the legs
```

with:

```go
	_, err := sketch.CreateFillet(s, l1, l2, corner, 100) // radius far larger than the legs
```

Edit 9. Replace exactly:

```go
	_, err := s.CreateFillet(l1, l2, 3)
```

with:

```go
	_, err := sketch.CreateFillet(s, l1, l2, corner, 3)
```

Edit 10. Replace exactly:

```go

	m := sketch.CreateMirror(s, []sketch.Entity{src}, axis)
	require.Len(t, m.Copies, 1)
```

with:

```go

	m, err := sketch.CreateMirror(s, []sketch.Entity{src}, axis)
	require.NoError(t, err)
	require.Len(t, m.Copies, 1)
```

Edit 11. Replace exactly:

```go
	m := sketch.CreateMirror(s, []sketch.Entity{src}, axis)
```

with:

```go
	m, err := sketch.CreateMirror(s, []sketch.Entity{src}, axis)
	require.NoError(t, err)
```

Edit 12. Replace exactly:

```go
	d := sketch.NewHorizontalDistance(p1, p2, 3)
	s.AddConstraint(d)
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

```

with:

```go
	d := sketch.NewHorizontalDistance(p1, p2, 3)
	s.AddConstraint(d)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)

```

Edit 13. Replace exactly:

```go
	m := sketch.CreateMirror(s, []sketch.Entity{c}, axis)
```

with:

```go
	m, err := sketch.CreateMirror(s, []sketch.Entity{c}, axis)
	require.NoError(t, err)
```

Edit 14. Replace exactly:

```go
	m := sketch.CreateMirror(s, []sketch.Entity{l1, l2}, axis)
```

with:

```go
	m, err := sketch.CreateMirror(s, []sketch.Entity{l1, l2}, axis)
	require.NoError(t, err)
```

Edit 15. Replace exactly:

```go
	sketch.CreateMirror(s, []sketch.Entity{src}, axis)
```

with:

```go
	_, err := sketch.CreateMirror(s, []sketch.Entity{src}, axis)
	require.NoError(t, err)
```

### Step 0.5 — `tools_ownership_test.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'require.Nil(t, sketch.CreateMirror(s, []sketch.Entity{fl}, axis))' tools_ownership_test.go   # expect 1
grep -c -F -- 'require.Nil(t, sketch.CreateMirror(s, []sketch.Entity{l}, faxis))' tools_ownership_test.go   # expect 1
grep -c -F -- 'f, err := s.CreateFillet(l1, fl, 2)' tools_ownership_test.go   # expect 1
grep -c -F -- 'require.InDelta(t, 5, nl.End.X(), 1e-9)' tools_ownership_test.go   # expect 2
grep -c -F -- 'm := sketch.CreateMirror(s, []sketch.Entity{l}, axis)' tools_ownership_test.go   # expect 2
grep -c -F -- 'require.NotNil(t, f.Arc)' tools_ownership_test.go   # expect 2
grep -c -F -- 'require.Nil(t, f)' tools_ownership_test.go   # expect 3
grep -c -F -- 'require.Empty(t, s.Constraints(), "no symmetric constraints on a foreign point")' tools_ownership_test.go   # expect 1
grep -c -F -- 'require.Nil(t, sketch.CreateMirror(s, []sketch.Entity{l}, axis))' tools_ownership_test.go   # expect 2
grep -c -F -- 'f, err := s.CreateFillet(l1, l2, 2)' tools_ownership_test.go   # expect 4
grep -c -F -- 'nl, ok := sketch.Trim(s, l, 9, 0)' tools_ownership_test.go   # expect 4
```

Edit 1. Replace exactly:

```go
		require.Nil(t, sketch.CreateMirror(s, []sketch.Entity{fl}, axis))
```

with:

```go
		m, err := sketch.CreateMirror(s, []sketch.Entity{fl}, axis)
		require.ErrorIs(t, err, sketch.ErrForeignEntity)
		require.Nil(t, m)
```

Edit 2. Replace exactly:

```go
		require.Nil(t, sketch.CreateMirror(s, []sketch.Entity{l}, faxis))
```

with:

```go
		m, err := sketch.CreateMirror(s, []sketch.Entity{l}, faxis)
		require.ErrorIs(t, err, sketch.ErrForeignEntity)
		require.Nil(t, m)
```

Edit 3. Replace exactly:

```go
		f, err := s.CreateFillet(l1, fl, 2)
```

with:

```go
		f, err := sketch.CreateFillet(s, l1, fl, corner, 2)
```

Edit 4. Replace exactly:

```go
		s.CreateLine(s.CreatePoint(5, -5), s.CreatePoint(5, 5))

		nl, ok := sketch.Trim(s, l, 9, 0)
		require.True(t, ok)
		require.InDelta(t, 5, nl.End.X(), 1e-9)
	})
```

with:

```go
		s.CreateLine(s.CreatePoint(5, -5), s.CreatePoint(5, 5))

		res, ok := sketch.Trim(s, l, 9, 0)
		require.True(t, ok)
		require.Len(t, res, 1)
		nl := res[0]
		require.InDelta(t, 5, nl.End.X(), 1e-9)
	})
```

Edit 5. Replace exactly:

```go
		axis := s.CreateLine(s.CreatePoint(0, -5), s.CreatePoint(0, 5))
		l := s.CreateLine(s.CreatePoint(2, 0), s.CreatePoint(6, 0))

		m := sketch.CreateMirror(s, []sketch.Entity{l}, axis)
		require.NotNil(t, m)
		require.Len(t, m.Copies, 1)
	})
```

with:

```go
		axis := s.CreateLine(s.CreatePoint(0, -5), s.CreatePoint(0, 5))
		l := s.CreateLine(s.CreatePoint(2, 0), s.CreatePoint(6, 0))

		m, err := sketch.CreateMirror(s, []sketch.Entity{l}, axis)
		require.NoError(t, err)
		require.NotNil(t, m)
		require.Len(t, m.Copies, 1)
	})
```

Edit 6. Replace exactly:

```go
		l2 := s.CreateLine(corner, s.CreatePoint(0, 10))

		f, err := s.CreateFillet(l1, l2, 2)
		require.NoError(t, err)
		require.NotNil(t, f.Arc)
```

with:

```go
		l2 := s.CreateLine(corner, s.CreatePoint(0, 10))

		f, err := sketch.CreateFillet(s, l1, l2, corner, 2)
		require.NoError(t, err)
		require.NotNil(t, f.Arc)
```

Edit 7. Replace exactly:

```go
		l2 := s.CreateLine(corner, s.CreatePoint(0, 10))

		f, err := s.CreateFillet(l1, l2, 2)
		require.ErrorIs(t, err, sketch.ErrForeignEntity)
		require.Nil(t, f)
```

with:

```go
		l2 := s.CreateLine(corner, s.CreatePoint(0, 10))

		f, err := sketch.CreateFillet(s, l1, l2, corner, 2)
		require.ErrorIs(t, err, sketch.ErrForeignEntity)
		require.Nil(t, f)
```

Edit 8. Replace exactly:

```go
		l := s.CreateLine(fl.Start, s.CreatePoint(6, 0))

		require.Nil(t, sketch.CreateMirror(s, []sketch.Entity{l}, axis))
		require.Len(t, s.Entities(), 2, "the axis and l, no copies")
		require.Empty(t, s.Constraints(), "no symmetric constraints on a foreign point")
```

with:

```go
		l := s.CreateLine(fl.Start, s.CreatePoint(6, 0))

		m, err := sketch.CreateMirror(s, []sketch.Entity{l}, axis)
		require.ErrorIs(t, err, sketch.ErrForeignEntity)
		require.Nil(t, m)
		require.Len(t, s.Entities(), 2, "the axis and l, no copies")
		require.Empty(t, s.Constraints(), "no symmetric constraints on a foreign point")
```

Edit 9. Replace exactly:

```go
		require.Nil(t, sketch.CreateMirror(s, []sketch.Entity{l}, axis))
```

with:

```go
		m, err := sketch.CreateMirror(s, []sketch.Entity{l}, axis)
		require.ErrorIs(t, err, sketch.ErrForeignEntity)
		require.Nil(t, m)
```

Edit 10. Replace exactly:

```go

		f, err := s.CreateFillet(l1, l2, 2)
		require.ErrorIs(t, err, sketch.ErrForeignEntity)
```

with:

```go

		f, err := sketch.CreateFillet(s, l1, l2, corner, 2)
		require.ErrorIs(t, err, sketch.ErrForeignEntity)
```

Edit 11. Replace exactly:

```go
		nl, ok := sketch.Trim(s, l, 9, 0)
		require.True(t, ok)
```

with:

```go
		res, ok := sketch.Trim(s, l, 9, 0)
		require.True(t, ok)
		require.Len(t, res, 1)
		nl := res[0]
```

Edit 12. Replace exactly:

```go
		f, err := s.CreateFillet(l1, l2, 2)
```

with:

```go
		f, err := sketch.CreateFillet(s, l1, l2, s.Origin(), 2)
```

Edit 13. Replace exactly:

```go
		m := sketch.CreateMirror(s, []sketch.Entity{l}, axis)
```

with:

```go
		m, err := sketch.CreateMirror(s, []sketch.Entity{l}, axis)
		require.NoError(t, err)
```

### Step 0.6 — `tools_unsupported_test.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'require.Nil(t, sketch.CreateMirror(s, seed, axis))' tools_unsupported_test.go   # expect 2
grep -c -F -- 'm := sketch.CreateMirror(s, []sketch.Entity{a}, axis)' tools_unsupported_test.go   # expect 1
grep -c -F -- 'm := sketch.CreateMirror(s, []sketch.Entity{seed}, axis)' tools_unsupported_test.go   # expect 1
```

Edit 1. Replace exactly:

```go

				require.Nil(t, sketch.CreateMirror(s, seed, axis))
			})
```

with:

```go

				m, err := sketch.CreateMirror(s, seed, axis)
				require.ErrorIs(t, err, sketch.ErrUnsupportedEntity)
				require.Nil(t, m)
			})
```

Edit 2. Replace exactly:

```go
				require.Nil(t, sketch.CreateMirror(s, seed, axis))
```

with:

```go
				m, err := sketch.CreateMirror(s, seed, axis)
				require.ErrorIs(t, err, sketch.ErrUnsupportedEntity)
				require.Nil(t, m)
```

Edit 3. Replace exactly:

```go
	m := sketch.CreateMirror(s, []sketch.Entity{a}, axis)
```

with:

```go
	m, err := sketch.CreateMirror(s, []sketch.Entity{a}, axis)
	require.NoError(t, err)
```

Edit 4. Replace exactly:

```go
		m := sketch.CreateMirror(s, []sketch.Entity{seed}, axis)
```

with:

```go
		m, err := sketch.CreateMirror(s, []sketch.Entity{seed}, axis)
		require.NoError(t, err)
```

### Step 0.7 — `tools_fitspline_test.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'm := sketch.CreateMirror(s, []sketch.Entity{seed}, axis)' tools_fitspline_test.go   # expect 1
```

Edit 1. Replace exactly:

```go
		m := sketch.CreateMirror(s, []sketch.Entity{seed}, axis)
		require.NotNil(t, m)
		_, err := s.Solve(t.Context())
```

with:

```go
		m, err := sketch.CreateMirror(s, []sketch.Entity{seed}, axis)
		require.NoError(t, err)
		require.NotNil(t, m)
		_, err = s.Solve(t.Context())
```

### Step 0.8 — `reference_test.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'require.Nil(t, sketch.CreateMirror(s, []sketch.Entity{l}, axis), "Mirror' reference_test.go   # expect 1
```

Edit 1. Replace exactly:

```go
	require.Nil(t, sketch.CreateMirror(s, []sketch.Entity{l}, axis), "Mirror refuses reference geometry")
```

with:

```go
	_, err = sketch.CreateMirror(s, []sketch.Entity{l}, axis)
	require.ErrorIs(t, err, sketch.ErrReferenceGeometry, "Mirror refuses reference geometry")
```

### Step 0.9 — `revision_internal_test.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '_, err = s.CreateFillet(l1, l2, 2)' revision_internal_test.go   # expect 1
grep -c -F -- 'long := s.CreateLine(s.CreatePoint(0, 60), s.CreatePoint(40, 60))' revision_internal_test.go   # expect 1
grep -c -F -- '_, ok = s.Trim(cut, 5, 80)' revision_internal_test.go   # expect 1
```

Edit 1. Replace exactly:

```go
		_, err = s.CreateFillet(l1, l2, 2)
```

with:

```go
		_, err = CreateFillet(s, l1, l2, o, 2)
```

Edit 2. Replace exactly:

```go
		require.NotNil(t, s.CreateMirror([]Entity{seed}, axis))
		_, err = s.CreatePatternRect([]Entity{seed}, 2, 2, 5, 5)
		require.NoError(t, err)
		_, err = s.CreateOffset([]Entity{seed}, 1)
		require.NoError(t, err)

		long := s.CreateLine(s.CreatePoint(0, 60), s.CreatePoint(40, 60))
		_, _, ok := s.Break(long, 20, 60)
```

with:

```go
		_, err = CreateMirror(s, []Entity{seed}, axis)
		require.NoError(t, err)
		_, err = CreatePatternRect(s, []Entity{seed}, 2, 2, 5, 5)
		require.NoError(t, err)
		_, err = CreateOffset(s, []Entity{seed}, 1)
		require.NoError(t, err)

		long := s.CreateLine(s.CreatePoint(0, 60), s.CreatePoint(40, 60))
		_, _, ok := Break(s, long, 20, 60)
```

Edit 3. Replace exactly:

```go
		_, ok = s.Trim(cut, 5, 80)
```

with:

```go
		_, ok = Trim(s, cut, 5, 80)
```

### Step 0.10 — `examples/sketch_fillet_example_test.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'f, err := s.CreateFillet(l1, l2, 3)' examples/sketch_fillet_example_test.go   # expect 1
```

Edit 1. Replace exactly:

```go
	f, err := s.CreateFillet(l1, l2, 3)
```

with:

```go
	f, err := sketch.CreateFillet(s, l1, l2, corner, 3)
```

### Step 0.11 — `internal/cmd/genimages/main.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'f, err := s.CreateFillet(r.BC, r.CD, 40)' internal/cmd/genimages/main.go   # expect 1
```

Edit 1. Replace exactly:

```go
	f, err := s.CreateFillet(r.BC, r.CD, 40)
```

with:

```go
	f, err := sketch.CreateFillet(s, r.BC, r.CD, r.C, 40)
```

### Step 0.12 — new file `tools_nil_sketch_test.go`

This file must not exist yet: `test -e tools_nil_sketch_test.go && echo EXISTS` prints nothing.

```go
package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// Every modification tool refuses a nil sketch with its refusal shape instead of
// panicking. The handles come from a live sketch so only the nil sketch is wrong.
func TestToolsRefuseNilSketch(t *testing.T) {
	var nilSketch *sketch.Sketch
	live := newSketch(t)
	corner := live.CreatePoint(0, 0)
	l1 := live.CreateLine(corner, live.CreatePoint(10, 0))
	l2 := live.CreateLine(corner, live.CreatePoint(0, 10))
	axis := live.CreateLine(live.CreatePoint(-5, -5), live.CreatePoint(-5, 5))
	seed := []sketch.Entity{l1}

	a, b, ok := sketch.Break(nilSketch, l1, 5, 0)
	require.False(t, ok)
	require.Nil(t, a)
	require.Nil(t, b)

	res, ok := sketch.Trim(nilSketch, l1, 5, 0)
	require.False(t, ok)
	require.Nil(t, res)

	ext, ok := sketch.Extend(nilSketch, l1, l1.End)
	require.False(t, ok)
	require.Nil(t, ext)

	f, err := sketch.CreateFillet(nilSketch, l1, l2, corner, 2)
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
	require.Nil(t, f)

	c, err := sketch.CreateChamfer(nilSketch, l1, l2, 2)
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
	require.Nil(t, c)

	m, err := sketch.CreateMirror(nilSketch, seed, axis)
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
	require.Nil(t, m)

	p, err := sketch.CreatePatternRect(nilSketch, seed, 2, 1, 20, 0)
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
	require.Nil(t, p)

	p, err = sketch.CreatePatternCircular(nilSketch, seed, corner, 3)
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
	require.Nil(t, p)

	p, err = sketch.CreatePatternCircularRing(nilSketch, seed, corner, 3, l1.Start, l1.End)
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
	require.Nil(t, p)

	g, err := sketch.CreateOffset(nilSketch, seed, 2)
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
	require.Nil(t, g)

	require.Len(t, live.Entities(), 3, "the live sketch is untouched")
}

// A typed result keeps its concrete type: no assertion is needed on the halves
// of a broken arc or on the legs of a line fillet.
func TestToolsReturnConcreteTypes(t *testing.T) {
	s := newSketch(t)
	arc := s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(5, 0), s.CreatePoint(0, 5))
	a1, a2, ok := sketch.Break(s, arc, 4, 4)
	require.True(t, ok)
	require.Same(t, a1.End, a2.Start)

	corner := s.CreatePoint(20, 0)
	l1 := s.CreateLine(corner, s.CreatePoint(30, 0))
	l2 := s.CreateLine(corner, s.CreatePoint(20, 10))
	f, err := sketch.CreateFillet(s, l1, l2, corner, 2)
	require.NoError(t, err)
	require.InDelta(t, 22, f.L1.Start.X(), 1e-9, "L1 is a *sketch.Line")

	_, err = sketch.CreateFillet(s, f.L1, a1, f.L1.End, 2)
	require.ErrorIs(t, err, sketch.ErrUnsupportedEntity, "arc fillets are not implemented yet")
}
```

### Step 0.13 — `CLAUDE.md`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '| `tools.go` | Sketch-modification tools on committed geometry (`Trim`/`' CLAUDE.md   # expect 1
```

Edit 1. Replace exactly:

```markdown
| `tools.go` | Sketch-modification tools on committed geometry (`Trim`/`Extend`/`Break`, `CreateFillet`/`CreateChamfer`, `CreateMirror`, `CreatePatternRect`/`CreatePatternCircular`, `CreateOffset`): build-then-replace via the `geom` toolkit + `RemoveEntity`. Design in `docs/modification-tools-design.md`. Every tool screens its inputs and its pattern/mirror seed. | `.claude/docs/sketch-core.md` → "`tools.go` — sketch-modification tools" |
```

with:

```markdown
| `tools.go` | Sketch-modification tools on committed geometry, as package-level functions taking the sketch first (`Trim`/`Extend`/`Break`, `CreateFillet`/`CreateChamfer`, `CreateMirror`, `CreatePatternRect`/`CreatePatternCircular`/`CreatePatternCircularRing`, `CreateOffset`; the curve tools are generic over `OpenCurve`): build-then-replace via the `geom` toolkit + `RemoveEntity`. Design in `docs/modification-tools-design.md`. Every tool screens its inputs and its pattern/mirror seed. | `.claude/docs/sketch-core.md` → "`tools.go` — sketch-modification tools" |
```

Edit 2. Replace exactly:

```markdown
object (the sketch's geometry builders). Load-bearing rules:
```

with:

```markdown
object (the sketch's geometry builders). The sketch-modification tools in
`tools.go` are package-level functions that take the sketch first
(`sketch.Trim(s, l, x, y)`, `sketch.CreateFillet(s, l1, l2, corner, r)`). Load-bearing rules:
```

### Step 0.14 — `.claude/docs/sketch-core.md`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'existing reference-geometry shape — `ErrForeignEntity` from the `Create…` tools,' .claude/docs/sketch-core.md   # expect 1
grep -c -F -- 'from `CreateMirror` (which has no error return, so a caller learns "refused" but' .claude/docs/sketch-core.md   # expect 1
```

Edit 1. Replace exactly:

```markdown
has no equivalent for "a method must call this helper", and a tool is a free
method with an arbitrary signature.) The refusal reuses each signature's
existing reference-geometry shape — `ErrForeignEntity` from the `Create…` tools,
nil from `CreateMirror`, false from `Trim`/`Extend`/`Break` — so no signature
changes. The plain builders (`CreateLine` and friends) still take a foreign
```

with:

```markdown
has no equivalent for "a function must call this helper", and a tool is a
package-level function with an arbitrary signature.) The refusal reuses each
signature's existing reference-geometry shape — `ErrForeignEntity` from the
`Create…` tools, false from `Trim`/`Extend`/`Break` — so no signature changes.
`foreignInput` also refuses a nil sketch, so every tool called with a nil
`*Sketch` returns its refusal instead of panicking. The plain builders (`CreateLine` and friends) still take a foreign
```

Edit 2. Replace exactly:

```markdown
`ErrUnsupportedEntity` from `CreatePatternRect`/`CreatePatternCircular`, nil
from `CreateMirror` (which has no error return, so a caller learns "refused" but
not which kind). `instantiate`'s own switch stays exactly as partial as before —
```

with:

```markdown
`ErrUnsupportedEntity` from `CreateMirror`/`CreatePatternRect`/
`CreatePatternCircular`, wrapped with the refused kind. `instantiate`'s own switch stays exactly as partial as before —
```

### Step 0.15 — `docs/modification-tools-design.md`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'func (s *Sketch) CreateOffset(ents []Entity, d float64) (*OffsetGroup, error)' docs/modification-tools-design.md   # expect 1
```

Edit 1. Replace exactly:

```markdown
// Replace tools (no new constraints): build-then-replace.
func (s *Sketch) Break(e Entity, x, y float64) (Entity, Entity, bool) // line | arc
func (s *Sketch) Trim(l *Line, x, y float64) (*Line, bool)
func (s *Sketch) Extend(l *Line, end *Point) (*Line, bool)

// Corner tools: arc/cut + tangency/coincidence + editable dimension.
func (s *Sketch) CreateFillet(l1, l2 *Line, r float64) (*Fillet, error)
func (s *Sketch) CreateChamfer(l1, l2 *Line, d float64) (*Chamfer, error)

// Copy tools: copies linked to the seed by constraints.
func (s *Sketch) CreateMirror(ents []Entity, axis *Line) *Mirror
func (s *Sketch) CreatePatternRect(ents []Entity, nx, ny int, dx, dy float64) (*Pattern, error)
func (s *Sketch) CreatePatternCircular(ents []Entity, center *Point, n int) (*Pattern, error)
func (s *Sketch) CreateOffset(ents []Entity, d float64) (*OffsetGroup, error)
```

with:

```markdown
// Package-level functions taking the sketch first (docs/curve-modify-tools-design.md).
type OpenCurve interface{ *Line | *Arc }

// Replace tools (no new constraints): build-then-replace.
func Break[T OpenCurve](s *Sketch, e T, x, y float64) (T, T, bool)
func Trim[T OpenCurve](s *Sketch, e T, x, y float64) ([]T, bool)
func Extend[T OpenCurve](s *Sketch, e T, end *Point) (T, bool)

// Corner tools: arc/cut + tangency/coincidence + editable dimension.
func CreateFillet[A, B OpenCurve](s *Sketch, e1 A, e2 B, corner *Point, r float64) (*Fillet[A, B], error)
func CreateChamfer(s *Sketch, l1, l2 *Line, d float64) (*Chamfer, error)

// Copy tools: copies linked to the seed by constraints.
func CreateMirror(s *Sketch, ents []Entity, axis *Line) (*Mirror, error)
func CreatePatternRect(s *Sketch, ents []Entity, nx, ny int, dx, dy float64) (*Pattern, error)
func CreatePatternCircular(s *Sketch, ents []Entity, center *Point, n int) (*Pattern, error)
func CreatePatternCircularRing(s *Sketch, ents []Entity, center *Point, n int, start, end *Point) (*Pattern, error)
func CreateOffset(s *Sketch, ents []Entity, d float64) (*OffsetGroup, error)
```

### Step 0.16 — `docs/acceptance-tests.md`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '| `TestTrimEndStub` / `TestTrimStartStub` / `TestTrimInteriorRejected` |' docs/acceptance-tests.md   # expect 1
```

Edit 1. Replace exactly:

```markdown
| `TestTrimEndStub` / `TestTrimStartStub` / `TestTrimInteriorRejected` | [exists] | `s.Trim(line, x, y)` — committed line crossing a cutter: trim removes the stub on the pick side and replaces the line; a pick bounded by crossings on both sides is rejected (would split — use Break). |
| `TestExtendToCutter` | [exists] | `s.Extend(line, end)` — endpoint extends to the nearest entity beyond it. |
| `TestBreakLine` / `TestBreakArc` | [exists] | `s.Break(ent, x, y)` — one line/arc becomes two sharing the split vertex (and, for arcs, the center); `TestBreakIsParametric` dimensions a half and re-solves. |
| `TestCreateFillet` | [exists] | `s.CreateFillet(l1, l2, r)` — removes the corner, adds the arc with tangent constraints to both legs + a radius dimension; editing `Fillet.Radius` and re-solving keeps tangency (parametric fillet). |
| `TestCreateChamfer` | [exists] | `s.CreateChamfer(l1, l2, d)` — same shape with setback distance dimensions. |
| `TestOffsetLine` / `TestOffsetChainMitresCorner` | [exists] | `s.CreateOffset(entities, d)` — a parallel chain bound by the new `Offset` **constraint**; corners mitre at the offset intersection and `OffsetGroup.Set(d)` + re-solve moves the copy. |
| `TestMirrorLine` / `TestMirrorTracksSource` | [exists] | `s.CreateMirror(entities, axisLine)` — mirrored copies created with symmetric constraints; moving the original re-solves the mirror side. |
| `TestPatternRect` / `TestPatternRectTracksSeed` | [exists] | `s.CreatePatternRect(entities, nx, ny, dx, dy)` — instances rigid-tied to and following the seed. |
| `TestPatternCircular` | [exists] | `s.CreatePatternCircular(entities, center, n)` — equal angular spacing held by construction-spoke constraints. |
```

with:

```markdown
| `TestTrimEndStub` / `TestTrimStartStub` / `TestTrimInteriorRejected` | [exists] | `Trim(s, line, x, y)` — committed line crossing a cutter: trim removes the stub on the pick side and replaces the line; a pick bounded by crossings on both sides is rejected (would split — use Break). |
| `TestExtendToCutter` | [exists] | `Extend(s, line, end)` — endpoint extends to the nearest entity beyond it. |
| `TestBreakLine` / `TestBreakArc` | [exists] | `Break(s, ent, x, y)` — one line/arc becomes two sharing the split vertex (and, for arcs, the center); `TestBreakIsParametric` dimensions a half and re-solves. |
| `TestCreateFillet` | [exists] | `CreateFillet(s, l1, l2, corner, r)` — removes the corner, adds the arc with tangent constraints to both legs + a radius dimension; editing `Fillet.Radius` and re-solving keeps tangency (parametric fillet). |
| `TestCreateChamfer` | [exists] | `CreateChamfer(s, l1, l2, d)` — same shape with setback distance dimensions. |
| `TestOffsetLine` / `TestOffsetChainMitresCorner` | [exists] | `CreateOffset(s, entities, d)` — a parallel chain bound by the new `Offset` **constraint**; corners mitre at the offset intersection and `OffsetGroup.Set(d)` + re-solve moves the copy. |
| `TestMirrorLine` / `TestMirrorTracksSource` | [exists] | `CreateMirror(s, entities, axisLine)` — mirrored copies created with symmetric constraints; moving the original re-solves the mirror side. |
| `TestPatternRect` / `TestPatternRectTracksSeed` | [exists] | `CreatePatternRect(s, entities, nx, ny, dx, dy)` — instances rigid-tied to and following the seed. |
| `TestPatternCircular` | [exists] | `CreatePatternCircular(s, entities, center, n)` — equal angular spacing held by construction-spoke constraints. |
```

### Step 0.17 — verify

Focused tests (all must pass):

```sh
go test -count=1 -run 'Tool|Trim|Extend|Break|Fillet|Chamfer|Mirror|Pattern|Offset|Reference|EntityUID|EntityTypeSwitches' .
go test -count=1 ./examples ./internal/cmd/genimages
```

Prove that no call to an old method is left. Run both lines in one Bash call; they print nothing:

```sh
T='Break|Trim|Extend|CreateFillet|CreateChamfer|CreateMirror|CreatePatternRect|CreatePatternCircular|CreatePatternCircularRing|CreateOffset'
grep -rnE "\.($T)\(" --include=*.go . | grep -vE "sketch\.($T)\(" | grep -vE '(strings|bytes)\.'
```

Compile-driven check: `go vet ./...` prints nothing. If the grep or `go vet` names a file this increment does not
edit, a newer PR added a call to an old method: run the Step 0.3 `gofmt -r` commands on that file, re-run both checks,
and if anything is still reported, stop and report it with the output.

Then the common "Verify" commands.

### Step 0.18 — commit and PR

- **Commit message:** `move modification tools to package functions`
- **Files to add:** `tools.go`, `sketch.go`, `tools_test.go`, `tools_ownership_test.go`, `tools_unsupported_test.go`,
  `tools_fitspline_test.go`, `reference_test.go`, `revision_internal_test.go`,
  `examples/sketch_fillet_example_test.go`, `internal/cmd/genimages/main.go`, `tools_nil_sketch_test.go`, `CLAUDE.md`,
  `.claude/docs/sketch-core.md`, `docs/modification-tools-design.md`, `docs/acceptance-tests.md`
- **PR title:** `move modification tools to package functions`
- **PR body:**

  ```markdown
  - Make the modification tools package-level functions taking the sketch first, so curve tools return concrete types
  - Refuse a nil sketch instead of panicking, and report CreateMirror refusals as errors
  ```

## Increment 1 — trim and extend like Fusion on lines

- **Branch:** `feat-trim-extend-in-place`
- **Design section:** "Modify in place, as Fusion does".
- **Merge after:** 0
- **Files:** `tools.go`, `tools_test.go`, `docs/modification-tools-design.md` (edits); `tools_in_place_test.go` (new).

### Step 1.0 — sync and check the base

From the worktree root:

```sh
git fetch origin
git rebase origin/main
git log --oneline origin/main | grep -c -F -- 'move modification tools to package functions'   # expect 1 or more
```

If the rebase stops, or a count is 0, stop and report. Then do the steps below in order. Every step starts
with its own `grep` checks: run them before editing that file.

### Step 1.1 — `tools.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '// first. They mutate committed geometry by the build-then-replace pattern: they' tools.go   # expect 1
grep -c -F -- '// nearest crossing with another entity, choosing the end nearest the pick point' tools.go   # expect 1
grep -c -F -- 'func (s *Sketch) trimLine(l *Line, x, y float64) (*Line, bool) {' tools.go   # expect 1
grep -c -F -- 'case hiP == nil && loP != nil: // crossing only below the pick: cut the end stub' tools.go   # expect 1
grep -c -F -- '// s (nil, dead, or foreign). Lines are handled; an arc returns false. The' tools.go   # expect 1
grep -c -F -- 'np := s.CreatePoint(best.X, best.Y)' tools.go   # expect 1
```

Edit 1. Replace exactly:

```go
// first. They mutate committed geometry by the build-then-replace pattern: they
// read the current geometry via each entity's Geometry snapshot, compute the
// replacement with the geom math layer, build the new geometry from sketch
// points (reusing the originals' surviving points so neighbouring geometry stays
// attached), and retire the originals with RemoveEntity. Constraints that
// referenced a replaced entity by identity are dropped with it (the removal
// cascade); constraints on the surviving shared points are kept. Replaced
// handles are dead. Every tool refuses a nil sketch.
```

with:

```go
// first. Where Fusion modifies the original curve (trimming or extending an end)
// they move that curve's end point in place, so the handle and every constraint
// that names it survive. Where Fusion deletes the curve (trimming its middle, or
// a curve with no crossing) they build the replacement from sketch points
// (reusing the originals' surviving points so neighbouring geometry stays
// attached) and retire the original with RemoveEntity; constraints that
// referenced it by identity are dropped with it (the removal cascade). Every
// tool refuses a nil sketch.
```

Edit 2. Replace exactly:

```go
// Trim shortens e by removing the portion adjacent to an endpoint up to its
// nearest crossing with another entity, choosing the end nearest the pick point
// (x, y). It returns the result, of the same type as e, and true. It returns nil
// and false (changing nothing) when s is nil; when e is not a live entity of s
// (nil, dead, or foreign) or has an endpoint that is not a live point of s; when
// e is reference geometry; when e has no crossing on the picked side; or when
// the pick sits on an interior portion bounded by crossings on both sides (use
// [Break]). Lines are handled; an arc returns nil and false. The original handle
// is dead.
```

with:

```go
// Trim removes the portion of e between the two crossings with other entities
// that bound the pick point (x, y), as Fusion's trim does; e's own end points
// bound the portion where no crossing does. Picking an end portion moves that end
// of e in place and returns []T{e}: the handle and the constraints that name it
// survive, and the old end point is removed when nothing else uses it. Picking a
// middle portion deletes e and returns the two remaining pieces, which share e's
// far end points. If e crosses nothing, Trim deletes e (and its end points no
// other entity uses) and returns an empty slice. It returns nil and false,
// changing nothing, when s is nil, when e is not a live entity of s (nil, dead,
// or foreign) or has an end point that is not a live point of s, or when e is
// reference geometry. Lines are handled; an arc returns nil and false.
```

Edit 3. Replace exactly:

```go
	nl, ok := s.trimLine(l, x, y)
	if !ok {
		return nil, false
	}
	return []T{asCurve[T](nl)}, true
}

// trimLine is Trim for a screened line.
func (s *Sketch) trimLine(l *Line, x, y float64) (*Line, bool) {
```

with:

```go
	pieces := s.trimLine(l, x, y)
	out := make([]T, len(pieces))
	for i, p := range pieces {
		out[i] = asCurve[T](p)
	}
	return out, true
}

// trimLine is Trim for a screened line.
func (s *Sketch) trimLine(l *Line, x, y float64) []*Line {
```

Edit 4. Replace exactly:

```go
	case loP == nil && hiP != nil: // crossing only above the pick: cut the start stub
		nl := s.CreateLine(s.CreatePoint(hiP.X, hiP.Y), l.End)
		s.RemoveEntity(l)
		return nl, true
	case hiP == nil && loP != nil: // crossing only below the pick: cut the end stub
		nl := s.CreateLine(l.Start, s.CreatePoint(loP.X, loP.Y))
		s.RemoveEntity(l)
		return nl, true
	}
	return nil, false
```

with:

```go
	case loP == nil && hiP == nil: // nothing crosses the picked portion: delete the line
		start, end := l.Start, l.End
		s.RemoveEntity(l)
		s.RemovePoint(start)
		s.RemovePoint(end)
		return []*Line{}
	case loP == nil: // crossing only above the pick: cut the start stub in place
		s.moveEnd(l, l.Start, s.CreatePoint(hiP.X, hiP.Y))
		return []*Line{l}
	case hiP == nil: // crossing only below the pick: cut the end stub in place
		s.moveEnd(l, l.End, s.CreatePoint(loP.X, loP.Y))
		return []*Line{l}
	}
	// Crossings on both sides: delete the line and keep the two outer pieces.
	first := s.CreateLine(l.Start, s.CreatePoint(loP.X, loP.Y))
	second := s.CreateLine(s.CreatePoint(hiP.X, hiP.Y), l.End)
	s.RemoveEntity(l)
	return []*Line{first, second}
}

// replaceEnd points e's end old (its Start or End) at np instead. It is the
// in-place edit Fusion makes when it trims, extends, fillets or chamfers a curve:
// the entity keeps its identity, so every constraint that names it survives.
func replaceEnd(e Entity, old, np *Point) {
	if l, ok := e.(*Line); ok {
		if l.Start == old {
			l.Start = np
		} else {
			l.End = np
		}
		return
	}
	if a, ok := e.(*Arc); ok {
		if a.Start == old {
			a.Start = np
		} else {
			a.End = np
		}
	}
}

// moveEnd moves e's end old to np in place, removes old when no entity uses it
// any more, and refreshes the endpoint tangencies that named old as their contact.
func (s *Sketch) moveEnd(e Entity, old, np *Point) {
	replaceEnd(e, old, np)
	s.RemovePoint(old)
	s.refreshSharedEndpoints()
}

// refreshSharedEndpoints re-derives the shared contact point of every endpoint
// tangency after an in-place end move. A tangency whose operands no longer share
// an end point becomes a tangency anywhere on the two curves: its cached point is
// cleared and allocVars allocates the sweep slack that form needs, so the caller's
// handle stays valid. A conic tangency in that state is removed instead, because
// its other form needs contact-witness variables that only a fresh constraint
// allocates consistently.
func (s *Sketch) refreshSharedEndpoints() {
	var stale []Constraint
	for _, c := range s.cons {
		switch t := c.(type) {
		case *tangentLineCircle:
			if a, ok := t.C.(*Arc); ok && t.shared != nil && sharedPointLineArc(t.L, a) != t.shared {
				t.shared = sharedPointLineArc(t.L, a)
				if t.shared == nil {
					t.allocVars(s)
				}
			}
		case *tangentCircles:
			if t.shared != nil && sharedPointCirculars(t.C1, t.C2) != t.shared {
				t.shared = sharedPointCirculars(t.C1, t.C2)
				if t.shared == nil {
					t.allocVars(s)
				}
			}
		case *tangentLineEllipse:
			if a, ok := t.E.(*EllipticalArc); ok && t.shared != nil && sharedPointLineEllipticalArc(t.L, a) != t.shared {
				t.shared = sharedPointLineEllipticalArc(t.L, a)
				if t.shared == nil {
					t.allocVars(s)
				}
			}
		case *tangentConics:
			if t.shared != nil && sharedConicEndpoint(t.A, t.B) != t.shared {
				stale = append(stale, c)
			}
		}
	}
	for _, c := range stale {
		s.RemoveConstraint(c)
	}
```

Edit 5. Replace exactly:

```go
// s (nil, dead, or foreign). Lines are handled; an arc returns false. The
// original handle is dead.
```

with:

```go
// s (nil, dead, or foreign). As Fusion's extend does, it moves end in place and
// returns e itself: the handle and the constraints that name it survive, and
// the old end point is removed when nothing else uses it. Lines are handled; an
// arc returns false.
```

Edit 6. Replace exactly:

```go
	np := s.CreatePoint(best.X, best.Y)
	var nl *Line
	if fromStart {
		nl = s.CreateLine(np, l.End)
	} else {
		nl = s.CreateLine(l.Start, np)
	}
	s.RemoveEntity(l)
	return nl, true
```

with:

```go
	s.moveEnd(l, end, s.CreatePoint(best.X, best.Y))
	return l, true
```

### Step 1.2 — `tools_test.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'func TestTrimInteriorRejected(t *testing.T) {' tools_test.go   # expect 1
grep -c -F -- 'require.False(t, ok, "trimming a bounded interior portion would split the line")' tools_test.go   # expect 1
grep -c -F -- '_, ok := sketch.Trim(s, h, 2, 0)' tools_test.go   # expect 1
```

Edit 1. Replace exactly:

```go
func TestTrimInteriorRejected(t *testing.T) {
```

with:

```go
func TestTrimInteriorSplits(t *testing.T) {
```

Edit 2. Replace exactly:

```go
	_, ok := sketch.Trim(s, h, 0, 0) // pick on the interior portion
	require.False(t, ok, "trimming a bounded interior portion would split the line")
```

with:

```go
	res, ok := sketch.Trim(s, h, 0, 0) // pick on the interior portion
	require.True(t, ok)
	require.Len(t, res, 2, "Fusion deletes the line and keeps the two outer pieces")
	require.Same(t, h.Start, res[0].Start)
	require.InDelta(t, -2, res[0].End.X(), 1e-9)
	require.InDelta(t, 2, res[1].Start.X(), 1e-9)
	require.Same(t, h.End, res[1].End)
	require.Len(t, s.Entities(), 4, "two cutters and two pieces")
```

Edit 3. Replace exactly:

```go
	_, ok := sketch.Trim(s, h, 2, 0)
	require.False(t, ok)
```

with:

```go
	res, ok := sketch.Trim(s, h, 2, 0)
	require.True(t, ok)
	require.Empty(t, res, "Fusion deletes a curve that crosses nothing")
	require.Empty(t, s.Entities())
	require.Empty(t, s.Points(), "the deleted line's end points go with it")
```

### Step 1.3 — new file `tools_in_place_test.go`

This file must not exist yet: `test -e tools_in_place_test.go && echo EXISTS` prints nothing.

```go
package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// countKind returns how many of the sketch's constraints have the given kind.
func countKind(s *sketch.Sketch, kind string) int {
	n := 0
	for _, c := range s.Constraints() {
		if sketch.ConstraintKind(c) == kind {
			n++
		}
	}
	return n
}

// Trimming an end keeps the line's handle and the constraints that name it, as
// Fusion's trim modifies the original curve; the trimmed-away end point goes.
func TestTrimEndKeepsHandle(t *testing.T) {
	s := newSketch(t)
	oldEnd := s.CreatePoint(5, 0)
	h := s.CreateLine(s.CreatePoint(-5, 0), oldEnd)
	s.CreateLine(s.CreatePoint(2, -5), s.CreatePoint(2, 5)) // vertical cutter at x=2
	s.AddConstraint(sketch.NewHorizontal(h))
	dof := s.DOF()

	res, ok := sketch.Trim(s, h, 4, 0)
	require.True(t, ok)
	require.Equal(t, []*sketch.Line{h}, res, "the same handle comes back")
	require.InDelta(t, 2, h.End.X(), 1e-9)
	require.InDelta(t, 0, h.End.Y(), 1e-9)
	require.NotContains(t, s.Points(), oldEnd, "the trimmed-away end point is removed")
	require.Equal(t, 1, countKind(s, "horizontal"), "the constraint on the line survives")
	require.Equal(t, dof, s.DOF(), "a fresh end replaces the removed one")
}

// Extending keeps the handle too, as Fusion's extend returns the modified curve.
func TestExtendKeepsHandle(t *testing.T) {
	s := newSketch(t)
	l := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(3, 0))
	s.CreateLine(s.CreatePoint(6, -5), s.CreatePoint(6, 5))
	s.AddConstraint(sketch.NewHorizontal(l))

	got, ok := sketch.Extend(s, l, l.End)
	require.True(t, ok)
	require.Same(t, l, got)
	require.InDelta(t, 6, l.End.X(), 1e-9)
	require.Len(t, s.Points(), 4, "the old end point is removed")
	require.Equal(t, 1, countKind(s, "horizontal"))
}

// A tangency whose shared contact point is trimmed off the line becomes a
// tangency anywhere on the two curves: the handle stays and still holds.
func TestTrimRefreshesSharedTangency(t *testing.T) {
	s := newSketch(t)
	p := s.CreatePoint(5, 0)
	arc := s.CreateArc(s.CreatePoint(0, 0), p, s.CreatePoint(0, 5))
	l := s.CreateLine(p, s.CreatePoint(5, -10))
	tan := sketch.NewTangent(l, arc)
	s.AddConstraint(tan)
	require.Len(t, sketch.ConstraintResiduals(tan), 1, "endpoint tangency: one row")
	s.CreateLine(s.CreatePoint(0, -2), s.CreatePoint(10, -2)) // cutter at y=-2

	res, ok := sketch.Trim(s, l, 5, -1)
	require.True(t, ok)
	require.Equal(t, []*sketch.Line{l}, res)
	require.InDelta(t, -2, l.Start.Y(), 1e-9)
	require.Same(t, p, arc.Start, "the arc keeps the old contact point")
	require.Contains(t, s.Constraints(), tan)
	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	rows := sketch.ConstraintResiduals(tan)
	require.Len(t, rows, 2, "tangency anywhere: circle row plus sweep row")
	for _, r := range rows {
		require.InDelta(t, 0, r, 1e-9)
	}
}
```

### Step 1.4 — `docs/modification-tools-design.md`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '(`lineCrossings`), locates the interval around the pick, and replaces the line' docs/modification-tools-design.md   # expect 1
```

Edit 1. Replace exactly:

```markdown
  (`lineCrossings`), locates the interval around the pick, and replaces the line
  with the portion that keeps the far end. A pick on a portion bounded by
  crossings on *both* sides returns false (it would split — use Break).
- **Extend** intersects the infinite line through `l` with the other entities,
  picks the nearest crossing *beyond* the chosen end, and replaces `l` with the
  lengthened segment.
```

with:

```markdown
  (`lineCrossings`) and locates the portion around the pick, bounded by the
  nearest crossings or the line's own ends, as Fusion's trim does. An end portion
  is cut by moving that end of the SAME line in place (the handle and its
  constraints survive; the old end point is removed when unused). A middle
  portion deletes the line and returns the two outer pieces. A line with no
  crossing is deleted.
- **Extend** intersects the infinite line through `l` with the other entities,
  picks the nearest crossing *beyond* the chosen end, and moves that end of `l`
  there in place.
- **In-place edits refresh endpoint tangencies.** An endpoint tangency whose
  shared contact point is moved off one operand becomes a tangency anywhere on the
  two curves (`refreshSharedEndpoints`); a conic tangency in that state is
  removed.
```

### Step 1.5 — verify

Focused tests (all must pass):

```sh
go test -count=1 -run 'Trim|Extend|KeepsHandle|RefreshesShared|Tools|Reference|EntityUID' .
```

Then the common "Verify" commands.

### Step 1.6 — commit and PR

- **Commit message:** `trim and extend lines in place`
- **Files to add:** `tools.go`, `tools_test.go`, `tools_in_place_test.go`, `docs/modification-tools-design.md`
- **PR title:** `trim and extend lines in place`
- **PR body:**

  ```markdown
  - Trim or extend a line's end in place, as Fusion does, so the constraints on the line survive
  - Trim the middle of a line into two pieces and delete a line that crosses nothing, as Fusion does
  ```

## Increment 2 — fillet and chamfer keep their legs

- **Branch:** `feat-fillet-chamfer-keep-legs`
- **Design section:** "Modify in place, as Fusion does".
- **Merge after:** 1
- **Files:** `tools.go`, `internal/cmd/genimages/main.go`, `docs/modification-tools-design.md`,
  `docs/images/fillet-after.svg` (edits); `tools_in_place_corner_test.go` (new).

### Step 2.0 — sync and check the base

From the worktree root:

```sh
git fetch origin
git rebase origin/main
git log --oneline origin/main | grep -c -F -- 'trim and extend lines in place'   # expect 1 or more
```

If the rebase stops, or a count is 0, stop and report. Then do the steps below in order. Every step starts
with its own `grep` checks: run them before editing that file.

### Step 2.1 — `tools.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '// (and any constraints that referenced them as entities — horizontal, angle, …)' tools.go   # expect 1
grep -c -F -- 'ctr := s.CreatePoint(arc.Center.X, arc.Center.Y)' tools.go   # expect 1
grep -c -F -- 'return &Fillet[A, B]{Arc: nArc, L1: asCurve[A](nL1), L2: asCurve[B](nL2)' tools.go   # expect 1
grep -c -F -- '// and each leg shortened to its contact; the contacts are pinned by editable' tools.go   # expect 1
grep -c -F -- 'nL1 := s.orientLeg(l1, corner, far1, t1)' tools.go   # expect 2
grep -c -F -- 'return &Chamfer{Cut: nCut, L1: nL1, L2: nL2, T1: t1, T2: t2, D1: d1, D2:' tools.go   # expect 1
grep -c -F -- '// orientLeg builds the replacement leg for orig, putting the fresh contact' tools.go   # expect 1
```

Edit 1. Replace exactly:

```go
// each leg is shortened to its contact point; the arc is held tangent to both
// legs and its radius pinned by an editable dimension, so editing
// [Fillet.Radius] and re-solving keeps the rounding tangent. The original legs
// (and any constraints that referenced them as entities — horizontal, angle, …)
// are replaced; re-apply such constraints to the returned [Fillet.L1]/[Fillet.L2]
// if needed. Constraints on the surviving far endpoints are kept. Two lines are
```

with:

```go
// each leg is shortened to its contact point in place, as Fusion's fillet does:
// [Fillet.L1] and [Fillet.L2] are e1 and e2 themselves, and the constraints that
// name them (horizontal, angle, …) survive. The arc is held tangent to both legs
// and its radius pinned by an editable dimension, so editing [Fillet.Radius] and
// re-solving keeps the rounding tangent. Constraints on the corner point are
// removed with it; constraints on the far endpoints are kept. Two lines are
```

Edit 2. Replace exactly:

```go
	ctr := s.CreatePoint(arc.Center.X, arc.Center.Y)
	nL1 := s.orientLeg(l1, corner, far1, t1)
	nL2 := s.orientLeg(l2, corner, far2, t2)
	nArc := s.CreateArc(ctr, t1, t2)
```

with:

```go
	ctr := s.CreatePoint(arc.Center.X, arc.Center.Y)
	replaceEnd(l1, corner, t1)
	replaceEnd(l2, corner, t2)
	nArc := s.CreateArc(ctr, t1, t2)
```

Edit 3. Replace exactly:

```go
	s.AddConstraint(NewTangent(nL1, nArc), NewTangent(nL2, nArc))
	rad := NewDistance(nArc.Center, nArc.Start, r)
	s.AddConstraint(rad)

	s.RemoveEntity(l1)
	s.RemoveEntity(l2)
	s.RemovePoint(corner)

	return &Fillet[A, B]{Arc: nArc, L1: asCurve[A](nL1), L2: asCurve[B](nL2), T1: t1, T2: t2, Radius: rad}, nil
```

with:

```go
	s.AddConstraint(NewTangent(l1, nArc), NewTangent(l2, nArc))
	rad := NewDistance(nArc.Center, nArc.Start, r)
	s.AddConstraint(rad)

	s.RemovePoint(corner)
	s.refreshSharedEndpoints()

	return &Fillet[A, B]{Arc: nArc, L1: e1, L2: e2, T1: t1, T2: t2, Radius: rad}, nil
```

Edit 4. Replace exactly:

```go
// and each leg shortened to its contact; the contacts are pinned by editable
// distance dimensions from the far endpoints (D1/D2), so editing them and
// re-solving moves the chamfer parametrically. Constraint handling matches
// [CreateFillet]. Returns [ErrForeignEntity] when s is nil or either line is not
```

with:

```go
// and each leg shortened to its contact in place, as Fusion's chamfer does:
// [Chamfer.L1] and [Chamfer.L2] are l1 and l2 themselves. The contacts are
// pinned by editable distance dimensions from the far endpoints (D1/D2), so
// editing them and re-solving moves the chamfer parametrically. Constraint
// handling matches [CreateFillet]. Returns [ErrForeignEntity] when s is nil or either line is not
```

Edit 5. Replace exactly:

```go
	nL1 := s.orientLeg(l1, corner, far1, t1)
	nL2 := s.orientLeg(l2, corner, far2, t2)
```

with:

```go
	replaceEnd(l1, corner, t1)
	replaceEnd(l2, corner, t2)
```

Edit 6. Replace exactly:

```go
	s.RemoveEntity(l1)
	s.RemoveEntity(l2)
	s.RemovePoint(corner)

	return &Chamfer{Cut: nCut, L1: nL1, L2: nL2, T1: t1, T2: t2, D1: d1, D2: d2}, nil
```

with:

```go
	s.RemovePoint(corner)
	s.refreshSharedEndpoints()

	return &Chamfer{Cut: nCut, L1: l1, L2: l2, T1: t1, T2: t2, D1: d1, D2: d2}, nil
```

Edit 7. Replace exactly:

```go
	return l.Start
}

// orientLeg builds the replacement leg for orig, putting the fresh contact
// point where the corner used to be and reusing the surviving far point.
func (s *Sketch) orientLeg(orig *Line, corner, far, contact *Point) *Line {
	if orig.Start == corner {
		return s.CreateLine(contact, far)
	}
	return s.CreateLine(far, contact)
}
```

with:

```go
	return l.Start
}
```

### Step 2.2 — `internal/cmd/genimages/main.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '// axis-alignment constraints, so re-pin the shortened legs to keep the sketch' internal/cmd/genimages/main.go   # expect 1
```

Edit 1. Replace exactly:

```go
	// The fillet removes edges BC (vertical) and CD (horizontal) with their
	// axis-alignment constraints, so re-pin the shortened legs to keep the sketch
	// fully constrained.
	f, err := sketch.CreateFillet(s, r.BC, r.CD, r.C, 40)
	if err != nil {
		return "", err
	}
	s.AddConstraint(sketch.NewVertical(f.L1), sketch.NewHorizontal(f.L2))
```

with:

```go
	// The fillet shortens edges BC (vertical) and CD (horizontal) in place, so
	// their axis-alignment constraints survive and the sketch stays fully
	// constrained.
	if _, err := sketch.CreateFillet(s, r.BC, r.CD, r.C, 40); err != nil {
		return "", err
	}
```

### Step 2.3 — new file `tools_in_place_corner_test.go`

This file must not exist yet: `test -e tools_in_place_corner_test.go && echo EXISTS` prints nothing.

```go
package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// A fillet shortens its legs in place, as Fusion's fillet does: the legs keep
// their handles and the constraints that name them, so the sketch stays fully
// constrained without re-pinning anything.
func TestFilletKeepsLegs(t *testing.T) {
	s := newSketch(t)
	a, corner, b := s.CreatePoint(0, 10), s.CreatePoint(0, 0), s.CreatePoint(10, 0)
	l1, l2 := s.CreateLine(a, corner), s.CreateLine(corner, b)
	s.AddConstraint(sketch.NewCoincident(corner, s.Origin()), sketch.NewVertical(l1), sketch.NewHorizontal(l2),
		sketch.NewDistance(a, corner, 10), sketch.NewDistance(corner, b, 10))
	require.Equal(t, 0, s.DOF())

	f, err := sketch.CreateFillet(s, l1, l2, corner, 3)
	require.NoError(t, err)
	require.Same(t, l1, f.L1)
	require.Same(t, l2, f.L2)
	require.Same(t, a, l1.Start, "the far end is kept")
	require.Same(t, f.T1, l1.End, "the corner end moved to the contact")
	require.NotContains(t, s.Points(), corner, "the corner point is removed")
	require.Equal(t, 1, countKind(s, "vertical"))
	require.Equal(t, 1, countKind(s, "horizontal"))
	require.Equal(t, 0, countKind(s, "coincident"), "the constraint on the corner goes with it")

	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 0, f.T1.X(), 1e-6, "still on the vertical leg")
	require.InDelta(t, 0, f.T2.Y(), 1e-6, "still on the horizontal leg")
	require.InDelta(t, 3, f.Arc.R(), 1e-6)
}

// A chamfer keeps its legs the same way.
func TestChamferKeepsLegs(t *testing.T) {
	s := newSketch(t)
	a, corner, b := s.CreatePoint(0, 10), s.CreatePoint(0, 0), s.CreatePoint(10, 0)
	l1, l2 := s.CreateLine(a, corner), s.CreateLine(corner, b)
	s.AddConstraint(sketch.NewVertical(l1), sketch.NewHorizontal(l2))

	c, err := sketch.CreateChamfer(s, l1, l2, 3)
	require.NoError(t, err)
	require.Same(t, l1, c.L1)
	require.Same(t, l2, c.L2)
	require.Same(t, c.T1, l1.End)
	require.Same(t, c.T2, l2.Start)
	require.Equal(t, 1, countKind(s, "vertical"))
	require.Equal(t, 1, countKind(s, "horizontal"))
	require.Len(t, s.Entities(), 3, "two legs and the cut")
}
```

### Step 2.4 — `docs/modification-tools-design.md`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'drops constraints that referenced *that line entity* (horizontal, angle, …) via' docs/modification-tools-design.md   # expect 1
grep -c -F -- 'arc/cut reusing the far generics and fresh contact points. The arc/cut shares' docs/modification-tools-design.md   # expect 1
```

Edit 1. Replace exactly:

```markdown
The deliberate limitation: a tool that replaces a line (trim, fillet, chamfer)
drops constraints that referenced *that line entity* (horizontal, angle, …) via
the removal cascade; constraints on its endpoints survive because the points
survive. The returned handle exposes the replacement entities so the caller can
re-apply entity constraints. Constraint transfer is left to the UI layer.
```

with:

```markdown
Trim of an end, extend, fillet and chamfer edit the curve IN PLACE, as Fusion
does, so constraints that reference the curve (horizontal, angle, …) survive. A
tool that deletes a curve (a middle trim, or a curve with no crossing) drops
the constraints that referenced it via the removal cascade; constraints on its
surviving endpoints are kept.
```

Edit 2. Replace exactly:

```markdown
  helper's endpoint-replacement is harmless), then commit the shortened legs +
  arc/cut reusing the far generics and fresh contact points. The arc/cut shares
  its contact points with the legs by pointer (no coincidence constraint
  needed). Fillet adds `NewTangent` on both legs + a `NewDistance(center, T1)`
  radius dimension; Chamfer adds the cut line + two setback `NewDistance`
  dimensions from the far ends. Both remove the originals and the corner point.
```

with:

```markdown
  helper's endpoint-replacement is harmless), then move each leg's corner end to
  a fresh contact point IN PLACE (the legs keep their handles and constraints, as
  Fusion's fillet and chamfer do) and commit the arc/cut. The arc/cut shares its
  contact points with the legs by pointer (no coincidence constraint needed).
  Fillet adds `NewTangent` on both legs + a `NewDistance(center, T1)` radius
  dimension; Chamfer adds the cut line + two setback `NewDistance` dimensions
  from the far ends. Both remove the corner point.
```

### Step 2.5 — regenerate the fillet image

The fillet now keeps its legs, so they stay first in the entity list and the image draws them in a different order.
The picture is the same.

Run exactly:

```sh
go run ./internal/cmd/genimages docs/images
git status --short docs/images
```

The last command prints exactly ` M docs/images/fillet-after.svg` and nothing else.

### Step 2.6 — verify

Focused tests (all must pass):

```sh
go test -count=1 -run 'Fillet|Chamfer|KeepsLegs|Tools' .
go test -count=1 ./examples ./internal/cmd/genimages
```

In this increment the last Verify command prints ` M docs/images/fillet-after.svg` until you commit.

Then the common "Verify" commands.

### Step 2.7 — commit and PR

- **Commit message:** `keep fillet and chamfer legs in place`
- **Files to add:** `tools.go`, `internal/cmd/genimages/main.go`, `tools_in_place_corner_test.go`,
  `docs/modification-tools-design.md`, `docs/images/fillet-after.svg`
- **PR title:** `keep fillet and chamfer legs in place`
- **PR body:**

  ```markdown
  - Shorten the legs of a fillet or chamfer in place, as Fusion does, so their constraints survive
  ```

## Increment 3 — pin trimmed and extended ends to the cutter

- **Branch:** `feat-trim-pin-to-cutter`
- **Design section:** "Pin trimmed and extended ends to the cutter".
- **Merge after:** 2
- **Files:** `tools.go`, `entity_switches_test.go`, `tools_in_place_test.go`, `docs/modification-tools-design.md`
  (edits); `tools_trim_pin_test.go` (new).

### Step 3.0 — sync and check the base

From the worktree root:

```sh
git fetch origin
git rebase origin/main
git log --oneline origin/main | grep -c -F -- 'keep fillet and chamfer legs in place'   # expect 1 or more
```

If the rebase stops, or a count is 0, stop and report. Then do the steps below in order. Every step starts
with its own `grep` checks: run them before editing that file.

### Step 3.1 — `tools.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'var loP, hiP *geom.Point' tools.go   # expect 1
grep -c -F -- 'case hiP == nil: // crossing only below the pick: cut the end stub in place' tools.go   # expect 1
grep -c -F -- 'var best *geom.Point' tools.go   # expect 1
grep -c -F -- '// lineCrossings returns the plane-local intersection points of sl with every' tools.go   # expect 1
grep -c -F -- 'out = append(out, p)' tools.go   # expect 2
grep -c -F -- 'out = append(out, geom.LineCircleIntersections(sl, t.Geometry())...)' tools.go   # expect 1
```

Edit 1. Replace exactly:

```go
	var loP, hiP *geom.Point
	for _, h := range hits {
		_, ht := geom.ClosestPointOnLine(sl, h)
```

with:

```go
	var loP, hiP *crossing
	for i := range hits {
		h := &hits[i]
		_, ht := geom.ClosestPointOnLine(sl, h.at)
```

Edit 2. Replace exactly:

```go
		s.moveEnd(l, l.Start, s.CreatePoint(hiP.X, hiP.Y))
		return []*Line{l}
	case hiP == nil: // crossing only below the pick: cut the end stub in place
		s.moveEnd(l, l.End, s.CreatePoint(loP.X, loP.Y))
		return []*Line{l}
	}
	// Crossings on both sides: delete the line and keep the two outer pieces.
	first := s.CreateLine(l.Start, s.CreatePoint(loP.X, loP.Y))
	second := s.CreateLine(s.CreatePoint(hiP.X, hiP.Y), l.End)
```

with:

```go
		s.moveEnd(l, l.Start, s.pointOnCutter(*hiP))
		return []*Line{l}
	case hiP == nil: // crossing only below the pick: cut the end stub in place
		s.moveEnd(l, l.End, s.pointOnCutter(*loP))
		return []*Line{l}
	}
	// Crossings on both sides: delete the line and keep the two outer pieces.
	first := s.CreateLine(l.Start, s.pointOnCutter(*loP))
	second := s.CreateLine(s.pointOnCutter(*hiP), l.End)
```

Edit 3. Replace exactly:

```go
	var best *geom.Point
	for _, h := range hits {
		_, ht := geom.ClosestPointOnLine(sl, h)
```

with:

```go
	var best *crossing
	for i := range hits {
		h := &hits[i]
		_, ht := geom.ClosestPointOnLine(sl, h.at)
```

Edit 4. Replace exactly:

```go
	s.moveEnd(l, end, s.CreatePoint(best.X, best.Y))
	return l, true
}

// lineCrossings returns the plane-local intersection points of sl with every
// entity except skip. When segment is true, line cutters are intersected as
// bounded segments (for trimming); otherwise the infinite line through sl is
// used and a line cutter contributes a point only where it falls within the
// cutter's own segment (for extending). Circle and arc cutters always use the
// infinite line through sl; the caller filters by position along sl.
func (s *Sketch) lineCrossings(sl *geom.Line, skip Entity, segment bool) []*geom.Point {
	var out []*geom.Point
```

with:

```go
	s.moveEnd(l, end, s.pointOnCutter(*best))
	return l, true
}

// crossing is a point where a trimmed or extended curve meets another entity,
// and that entity (the cutter).
type crossing struct {
	at     *geom.Point
	cutter Entity
}

// pointOnCutter returns the sketch point a trimmed or extended end moves to. A
// crossing on one of the cutter's own end points reuses that point, so the two
// curves share it. Otherwise it creates a point at the crossing and ties it to
// the cutter with a point-on-curve constraint, as Fusion's trim and extend
// create constraints by default.
func (s *Sketch) pointOnCutter(c crossing) *Point {
	for _, p := range cutterEnds(c.cutter) {
		if near(p.Geometry(), c.at) {
			return p
		}
	}
	np := s.CreatePoint(c.at.X, c.at.Y)
	s.pinToCutter(np, c.cutter)
	return np
}

// cutterEnds returns the end points of a line or arc cutter. A circle has none.
func cutterEnds(e Entity) []*Point {
	switch t := e.(type) {
	case *Line:
		return []*Point{t.Start, t.End}
	case *Arc:
		return []*Point{t.Start, t.End}
	}
	return nil
}

// pinToCutter ties p to the cutter it was created on.
func (s *Sketch) pinToCutter(p *Point, cutter Entity) {
	switch t := cutter.(type) {
	case *Line:
		s.AddConstraint(NewPointOnLine(p, t))
	case *Circle:
		s.AddConstraint(NewPointOnCircle(p, t))
	case *Arc:
		s.AddConstraint(NewPointOnArc(p, t))
	}
}

// lineCrossings returns where sl meets every entity except skip, with the
// entity met. When segment is true, line cutters are intersected as bounded
// segments (for trimming); otherwise the infinite line through sl is used and a
// line cutter contributes a point only where it falls within the cutter's own
// segment (for extending). Circle and arc cutters always use the infinite line
// through sl; the caller filters by position along sl.
func (s *Sketch) lineCrossings(sl *geom.Line, skip Entity, segment bool) []crossing {
	var out []crossing
```

Edit 5. Replace exactly:

```go
					out = append(out, p)
```

with:

```go
					out = append(out, crossing{p, t})
```

Edit 6. Replace exactly:

```go
				out = append(out, p)
			}
		case *Circle:
			out = append(out, geom.LineCircleIntersections(sl, t.Geometry())...)
		case *Arc:
			out = append(out, geom.LineArcIntersections(sl, t.Geometry())...)
```

with:

```go
				out = append(out, crossing{p, t})
			}
		case *Circle:
			for _, p := range geom.LineCircleIntersections(sl, t.Geometry()) {
				out = append(out, crossing{p, t})
			}
		case *Arc:
			for _, p := range geom.LineArcIntersections(sl, t.Geometry()) {
				out = append(out, crossing{p, t})
			}
```

### Step 3.2 — `entity_switches_test.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '{"tools.go", "Break"}:                    "splitting is defined for a li' entity_switches_test.go   # expect 1
```

Edit 1. Replace exactly:

```go
	{"tools.go", "Break"}:                    "splitting is defined for a line and an arc only; every other type is the documented false return",
	{"tools.go", "lineCrossings"}:            "only line/circle/arc cutters have a closed-form line intersection in geom; a curve contributes no crossing",
```

with:

```go
	{"tools.go", "Break"}:                    "splitting is defined for a line and an arc only; every other type is the documented false return",
	{"tools.go", "cutterEnds"}:               "only a line or an arc cutter has end points a trimmed end can reuse; a circle has none",
	{"tools.go", "pinToCutter"}:              "only line/circle/arc cutters produce a crossing (lineCrossings), so only they are pinned to",
	{"tools.go", "lineCrossings"}:            "only line/circle/arc cutters have a closed-form line intersection in geom; a curve contributes no crossing",
```

### Step 3.3 — `tools_in_place_test.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'require.Equal(t, dof, s.DOF(), "a fresh end replaces the removed one")' tools_in_place_test.go   # expect 1
```

Edit 1. Replace exactly:

```go
	require.Equal(t, dof, s.DOF(), "a fresh end replaces the removed one")
```

with:

```go
	require.Equal(t, 1, countKind(s, "point_on_line"), "the new end is tied to the cutter")
	require.Equal(t, dof-1, s.DOF(), "a fresh end replaces the removed one, less the pin")
```

### Step 3.4 — new file `tools_trim_pin_test.go`

This file must not exist yet: `test -e tools_trim_pin_test.go && echo EXISTS` prints nothing.

```go
package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// A trimmed end is tied to the cutter it was cut at, one constraint kind per
// cutter kind, so moving the cutter moves the end with it.
func TestTrimPinsEndToCutter(t *testing.T) {
	t.Run("line", func(t *testing.T) {
		s := newSketch(t)
		h := s.CreateLine(s.CreatePoint(-5, 0), s.CreatePoint(5, 0))
		cutter := s.CreateLine(s.CreatePoint(2, -5), s.CreatePoint(2, 5))
		dof := s.DOF()

		_, ok := sketch.Trim(s, h, 4, 0)
		require.True(t, ok)
		require.Equal(t, 1, countKind(s, "point_on_line"))
		require.Equal(t, dof-1, s.DOF())

		s.Fix(cutter.Start)
		s.Fix(cutter.End)
		cutter.Start.MoveTo(3, -5)
		cutter.End.MoveTo(3, 5)
		_, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.InDelta(t, 3, h.End.X(), 1e-6, "the end follows the cutter")
	})

	t.Run("circle", func(t *testing.T) {
		s := newSketch(t)
		h := s.CreateLine(s.CreatePoint(-10, 0), s.CreatePoint(0, 0))
		s.CreateCircle(s.CreatePoint(0, 0), 5) // crosses h at (-5, 0)
		dof := s.DOF()

		_, ok := sketch.Trim(s, h, -2, 0)
		require.True(t, ok)
		require.InDelta(t, -5, h.End.X(), 1e-9)
		require.Equal(t, 1, countKind(s, "point_on_circle"))
		require.Equal(t, dof-1, s.DOF())
	})

	t.Run("arc", func(t *testing.T) {
		s := newSketch(t)
		h := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(10, 0))
		s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(5, -5), s.CreatePoint(5, 5)) // crosses h at (7.07, 0)
		dof := s.DOF()

		_, ok := sketch.Trim(s, h, 9, 0)
		require.True(t, ok)
		require.InDelta(t, 7.0710678118654755, h.End.X(), 1e-9)
		require.Equal(t, 1, countKind(s, "point_on_arc"))
		require.Equal(t, dof-1, s.DOF(), "two rows and one sweep slack variable: one degree of freedom less")
	})

	t.Run("cutter end point is shared", func(t *testing.T) {
		s := newSketch(t)
		h := s.CreateLine(s.CreatePoint(-5, 0), s.CreatePoint(5, 0))
		foot := s.CreatePoint(2, 0)
		s.CreateLine(foot, s.CreatePoint(2, 5)) // a T: the cutter ends on h
		dof := s.DOF()

		_, ok := sketch.Trim(s, h, 4, 0)
		require.True(t, ok)
		require.Same(t, foot, h.End, "the trim reuses the cutter's end point")
		require.Equal(t, 0, countKind(s, "point_on_line"))
		require.Equal(t, dof-2, s.DOF())
	})
}

// An extended end is tied to its cutter the same way.
func TestExtendPinsEndToCutter(t *testing.T) {
	s := newSketch(t)
	l := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(3, 0))
	s.CreateLine(s.CreatePoint(6, -5), s.CreatePoint(6, 5))
	dof := s.DOF()

	_, ok := sketch.Extend(s, l, l.End)
	require.True(t, ok)
	require.Equal(t, 1, countKind(s, "point_on_line"))
	require.Equal(t, dof-1, s.DOF())
}

// A middle trim pins both new ends.
func TestTrimMiddlePinsBothEnds(t *testing.T) {
	s := newSketch(t)
	h := s.CreateLine(s.CreatePoint(-5, 0), s.CreatePoint(5, 0))
	s.CreateLine(s.CreatePoint(-2, -5), s.CreatePoint(-2, 5))
	s.CreateLine(s.CreatePoint(2, -5), s.CreatePoint(2, 5))
	dof := s.DOF()

	res, ok := sketch.Trim(s, h, 0, 0)
	require.True(t, ok)
	require.Len(t, res, 2)
	require.Equal(t, 2, countKind(s, "point_on_line"))
	require.Equal(t, dof+2, s.DOF(), "two fresh points (+4), two pins (-2)")
}
```

### Step 3.5 — `docs/modification-tools-design.md`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'crossing is deleted.' docs/modification-tools-design.md   # expect 1
```

Edit 1. Replace exactly:

```markdown
  crossing is deleted.
```

with:

```markdown
  crossing is deleted. Every end a trim or extend creates on a cutter is tied to
  it, as Fusion's trim and extend create constraints by default: the cutter's own
  end point is reused when the crossing is on it, otherwise a fresh point gets
  `NewPointOnLine`/`NewPointOnCircle`/`NewPointOnArc` to the cutter
  (`pointOnCutter`).
```

### Step 3.6 — verify

Focused tests (all must pass):

```sh
go test -count=1 -run 'Trim|Extend|Pins|KeepsHandle|EntityTypeSwitches|Tools' .
```

Then the common "Verify" commands.

### Step 3.7 — commit and PR

- **Commit message:** `pin trimmed and extended ends to cutters`
- **Files to add:** `tools.go`, `entity_switches_test.go`, `tools_in_place_test.go`, `tools_trim_pin_test.go`,
  `docs/modification-tools-design.md`
- **PR title:** `pin trimmed and extended ends to cutters`
- **PR body:**

  ```markdown
  - Tie every end a trim or extend creates to the curve it was cut at, as Fusion does by default
  ```

## Increment 4 — `PointOnNormal` constraint

- **Branch:** `feat-point-on-normal`
- **Design section:** "The `PointOnNormal` constraint".
- **Merge after:** 3
- **Files:** `constraint.go`, `json.go`, `removal.go`, `conditioning.go`, `annotate.go`,
  `introspect_nil_contract_test.go`, `jacobian_inventory_test.go` (edits); `point_on_normal_test.go` (new).

### Step 4.0 — sync and check the base

From the worktree root:

```sh
git fetch origin
git rebase origin/main
git log --oneline origin/main | grep -c -F -- 'pin trimmed and extended ends to cutters'   # expect 1 or more
```

If the rebase stops, or a count is 0, stop and report. Then do the steps below in order. Every step starts
with its own `grep` checks: run them before editing that file.

### Step 4.1 — `constraint.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'func NewPointOnLine(p *Point, l *Line) Constraint { return &pointOnLine{p, l} }' constraint.go   # expect 1
```

Edit 1. Replace exactly:

```go
func NewPointOnLine(p *Point, l *Line) Constraint { return &pointOnLine{p, l} }

```

with:

```go
func NewPointOnLine(p *Point, l *Line) Constraint { return &pointOnLine{p, l} }

// pointOnNormal holds P on the line through At that is normal to C. C is a
// *Line or an *Arc (the constructor's type constraint admits nothing else).
type pointOnNormal struct {
	P, At *Point
	C     Entity
}

func (c *pointOnNormal) residual(out []float64) []float64 {
	if l, ok := c.C.(*Line); ok {
		// signed distance of P from the perpendicular to l through At (length units)
		ux, uy := l.End.x()-l.Start.x(), l.End.y()-l.Start.y()
		dx, dy := c.P.x()-c.At.x(), c.P.y()-c.At.y()
		return append(out, (dx*ux+dy*uy)/norm(ux, uy))
	}
	if a, ok := c.C.(*Arc); ok {
		// signed distance of P from the radial line through the center and At
		rx, ry := c.At.x()-a.Center.x(), c.At.y()-a.Center.y()
		px, py := c.P.x()-a.Center.x(), c.P.y()-a.Center.y()
		return append(out, (rx*py-ry*px)/norm(rx, ry))
	}
	return append(out, 1)
}

// NewPointOnNormal holds p on the line through at that is normal to c: for a
// line, the perpendicular to its direction through at; for an arc, the radial
// line through its center and at. The residual is the distance of p from that
// line, in length units. The offset tools use it to keep an offset end on the
// source curve's normal at the source end.
func NewPointOnNormal[T OpenCurve](p *Point, c T, at *Point) Constraint {
	return &pointOnNormal{P: p, At: at, C: asEntity(c)}
}

```

### Step 4.2 — `json.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'return "point_on_line"' json.go   # expect 1
grep -c -F -- 'return jsonConstraint{Type: typ, Points: []int{t.P.id}, Entities: []int{t.L.id}}, true' json.go   # expect 2
grep -c -F -- '"collinear": {0, 2}, "angle": {0, 2}, "point_on_line": {1, 1},' json.go   # expect 1
grep -c -F -- 's.AddConstraint(NewPointOnLine(pt(0), l))' json.go   # expect 1
```

Edit 1. Replace exactly:

```go
		return "point_on_line"
	case *collinear:
```

with:

```go
		return "point_on_line"
	case *pointOnNormal:
		return "point_on_normal"
	case *collinear:
```

Edit 2. Replace exactly:

```go
		return jsonConstraint{Type: typ, Points: []int{t.P.id}, Entities: []int{t.L.id}}, true
	case *collinear:
```

with:

```go
		return jsonConstraint{Type: typ, Points: []int{t.P.id}, Entities: []int{t.L.id}}, true
	case *pointOnNormal:
		return jsonConstraint{Type: typ, Points: []int{t.P.id, t.At.id}, Entities: []int{t.C.entID()}}, true
	case *collinear:
```

Edit 3. Replace exactly:

```go
	"collinear": {0, 2}, "angle": {0, 2}, "point_on_line": {1, 1},
```

with:

```go
	"collinear": {0, 2}, "angle": {0, 2}, "point_on_line": {1, 1}, "point_on_normal": {2, 1},
```

Edit 4. Replace exactly:

```go
		s.AddConstraint(NewPointOnLine(pt(0), l))
	case "point_on_circle":
```

with:

```go
		s.AddConstraint(NewPointOnLine(pt(0), l))
	case "point_on_normal":
		if l, err := line(jc.Entities[0]); err == nil {
			s.AddConstraint(NewPointOnNormal(pt(0), l, pt(1)))
		} else {
			c, err := circular(jc.Entities[0])
			if err != nil {
				return err
			}
			a, ok := c.(*Arc)
			if !ok {
				return fmt.Errorf("sketch: point_on_normal entity %d is not a line or an arc", jc.Entities[0])
			}
			s.AddConstraint(NewPointOnNormal(pt(0), a, pt(1)))
		}
	case "point_on_circle":
```

### Step 4.3 — `removal.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'case *collinear:' removal.go   # expect 1
```

Edit 1. Replace exactly:

```go
		return []*Point{t.P}, []Entity{t.L}
	case *collinear:
```

with:

```go
		return []*Point{t.P}, []Entity{t.L}
	case *pointOnNormal:
		return []*Point{t.P, t.At}, []Entity{t.C}
	case *collinear:
```

### Step 4.4 — `conditioning.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'case *pointOnLine:' conditioning.go   # expect 1
```

Edit 1. Replace exactly:

```go
	case *pointOnLine:
		return append(out, rowLength)
```

with:

```go
	case *pointOnLine:
		return append(out, rowLength)
	case *pointOnNormal:
		return append(out, rowLength)
```

### Step 4.5 — `annotate.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'case *pointOnCircle:' annotate.go   # expect 1
```

Edit 1. Replace exactly:

```go
		a.badge(a.scr(t.P), "•")
	case *pointOnCircle:
```

with:

```go
		a.badge(a.scr(t.P), "•")
	case *pointOnNormal:
		a.badge(a.scr(t.P), "⊥")
	case *pointOnCircle:
```

### Step 4.6 — `introspect_nil_contract_test.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '{"NewPointOnLine", "", func() sketch.Constraint { return sketch.NewPoint' introspect_nil_contract_test.go   # expect 1
grep -c -F -- 'for _, param := range fn.Type.Params.List {' introspect_nil_contract_test.go   # expect 1
```

Edit 1. Replace exactly:

```go
	{"NewPointOnLine", "", func() sketch.Constraint { return sketch.NewPointOnLine(nil, nil) }, "ok", "point_on_line", "pts=[nil] ents=[typed-nil(*sketch.Line)]", "panic"},
	{"NewCollinear", "", func() sketch.Constraint { return sketch.NewCollinear(nil, nil) }, "ok", "collinear", "pts=[] ents=[typed-nil(*sketch.Line) typed-nil(*sketch.Line)]", "panic"},
```

with:

```go
	{"NewPointOnLine", "", func() sketch.Constraint { return sketch.NewPointOnLine(nil, nil) }, "ok", "point_on_line", "pts=[nil] ents=[typed-nil(*sketch.Line)]", "panic"},
	{"NewPointOnNormal", "", func() sketch.Constraint { return sketch.NewPointOnNormal[*sketch.Line](nil, nil, nil) }, "ok", "point_on_normal", "pts=[nil nil] ents=[typed-nil(*sketch.Line)]", "panic"},
	{"NewCollinear", "", func() sketch.Constraint { return sketch.NewCollinear(nil, nil) }, "ok", "collinear", "pts=[] ents=[typed-nil(*sketch.Line) typed-nil(*sketch.Line)]", "panic"},
```

Edit 2. Replace exactly:

```go
		for _, param := range fn.Type.Params.List {
			id, ok := param.Type.(*ast.Ident)
			if !ok {
```

with:

```go
		// A type parameter (NewPointOnNormal's T OpenCurve) is constrained to
		// pointer types, so its nil is always a typed nil: one row, like a
		// concrete pointer operand.
		typeParams := map[string]struct{}{}
		if fn.Type.TypeParams != nil {
			for _, f := range fn.Type.TypeParams.List {
				for _, n := range f.Names {
					typeParams[n.Name] = struct{}{}
				}
			}
		}
		for _, param := range fn.Type.Params.List {
			id, ok := param.Type.(*ast.Ident)
			if !ok {
				continue
			}
			if _, tp := typeParams[id.Name]; tp {
```

### Step 4.7 — `jacobian_inventory_test.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'c := NewPointOnLine(s.CreatePoint(2, 1), l)' jacobian_inventory_test.go   # expect 1
grep -c -F -- '{"point_on_line", 1, 6, 0, 0, "static", true},' jacobian_inventory_test.go   # expect 1
grep -c -F -- '"NewPointOnLine":            "point_on_line",' jacobian_inventory_test.go   # expect 1
```

Edit 1. Replace exactly:

```go
			c := NewPointOnLine(s.CreatePoint(2, 1), l)
			s.AddConstraint(c)
```

with:

```go
			c := NewPointOnLine(s.CreatePoint(2, 1), l)
			s.AddConstraint(c)
			return c
		}},
		{"point_on_normal", func(s *Sketch) Constraint {
			l := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(4, 0))
			c := NewPointOnNormal(s.CreatePoint(4, 1), l, s.CreatePoint(4, 0))
			s.AddConstraint(c)
```

Edit 2. Replace exactly:

```go
	{"point_on_line", 1, 6, 0, 0, "static", true},
	{"collinear", 2, 8, 0, 0, "static", true},
```

with:

```go
	{"point_on_line", 1, 6, 0, 0, "static", true},
	{"point_on_normal", 1, 8, 0, 0, "static", true},
	{"collinear", 2, 8, 0, 0, "static", true},
```

Edit 3. Replace exactly:

```go
	"NewPointOnLine":            "point_on_line",
	"NewCollinear":              "collinear",
```

with:

```go
	"NewPointOnLine":            "point_on_line",
	"NewPointOnNormal":          "point_on_normal",
	"NewCollinear":              "collinear",
```

### Step 4.8 — new file `point_on_normal_test.go`

This file must not exist yet: `test -e point_on_normal_test.go && echo EXISTS` prints nothing.

```go
package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// The point is pulled onto the normal of a line at a point: the perpendicular
// to the line through that point.
func TestPointOnNormalLine(t *testing.T) {
	s := newSketch(t)
	a, b := s.CreatePoint(0, 0), s.CreatePoint(10, 0)
	s.Fix(a)
	s.Fix(b)
	l := s.CreateLine(a, b)
	p := s.CreatePoint(7, 3)
	s.AddConstraint(sketch.NewPointOnNormal(p, l, b))

	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 10, p.X(), 1e-6, "on the perpendicular through (10, 0)")
}

// For an arc the normal is the radial line through the center and the point.
func TestPointOnNormalArc(t *testing.T) {
	s := newSketch(t)
	c, st, en := s.CreatePoint(0, 0), s.CreatePoint(5, 0), s.CreatePoint(0, 5)
	s.Fix(c)
	s.Fix(st)
	s.Fix(en)
	arc := s.CreateArc(c, st, en)
	p := s.CreatePoint(7, 2)
	s.AddConstraint(sketch.NewPointOnNormal(p, arc, st))

	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 0, p.Y(), 1e-6, "on the radial line through (5, 0)")
}

// The constraint survives a JSON round trip for both operand kinds, and its row
// is classified for the conditioning gate (an unclassified row reads as NaN).
func TestPointOnNormalRoundTripAndVerify(t *testing.T) {
	s := newSketch(t)
	l := s.CreateLine(s.CreatePoint(0, 0), s.CreatePoint(10, 0))
	arc := s.CreateArc(s.CreatePoint(20, 0), s.CreatePoint(25, 0), s.CreatePoint(20, 5))
	s.AddConstraint(
		sketch.NewPointOnNormal(s.CreatePoint(10, 3), l, l.End),
		sketch.NewPointOnNormal(s.CreatePoint(27, 0), arc, arc.Start),
	)
	require.Equal(t, 2, countKind(s, "point_on_normal"))

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Equal(t, 2, countKind(&s2, "point_on_normal"))

	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	rep := s.Verify(t.Context())
	require.True(t, rep.Analysed())
	require.False(t, math.IsNaN(rep.Conditioning))
}
```

### Step 4.9 — verify

Focused tests (all must pass):

```sh
go test -count=1 -run 'PointOnNormal|Introspection|Dependency|Jacobian|JSON|Conditioning|Annotat' .
```

Then the common "Verify" commands.

### Step 4.10 — commit and PR

- **Commit message:** `add point-on-normal constraint`
- **Files to add:** `constraint.go`, `json.go`, `removal.go`, `conditioning.go`, `annotate.go`,
  `introspect_nil_contract_test.go`, `jacobian_inventory_test.go`, `point_on_normal_test.go`
- **PR title:** `add point-on-normal constraint`
- **PR body:**

  ```markdown
  - Add NewPointOnNormal, which holds a point on a line's or arc's normal at a given point
  ```

## Increment 5 — line offset like Fusion

- **Branch:** `feat-offset-fusion-lines`
- **Design section:** "Offset like Fusion".
- **Merge after:** 4
- **Files:** `tools.go`, `tools_test.go`, `docs/modification-tools-design.md` (edits); `tools_offset_flow_test.go`
  (new).

### Step 5.0 — sync and check the base

From the worktree root:

```sh
git fetch origin
git rebase origin/main
git log --oneline origin/main | grep -c -F -- 'add point-on-normal constraint'   # expect 1 or more
```

If the rebase stops, or a count is 0, stop and report. Then do the steps below in order. Every step starts
with its own `grep` checks: run them before editing that file.

### Step 5.1 — `tools.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '// entities in ents are skipped. It returns [ErrForeignEntity] if s is nil or a' tools.go   # expect 1
grep -c -F -- '// unchanged (no partial geometry committed).' tools.go   # expect 1
grep -c -F -- 'return nil, fmt.Errorf("%w: CreateOffset source line has zero length", E' tools.go   # expect 1
grep -c -F -- 'nx, ny := -dy/n, dx/n   // left normal of the segment direction' tools.go   # expect 1
```

Edit 1. Replace exactly:

```go
// lines (left in place), their offset copies, and the per-segment offset
// constraints. Use [OffsetGroup.Set] to retarget the whole offset distance at
// once and re-solve.
type OffsetGroup struct {
	Source  []Entity
	Copies  []Entity
	Offsets []*Offset
}

// Set retargets every segment's offset distance to d. Call [Sketch.Solve]
// afterwards to apply it.
func (g *OffsetGroup) Set(d float64) {
	for _, o := range g.Offsets {
		o.Set(d)
	}
}

// CreateOffset creates a parallel offset of a chain of lines at signed distance d
// (positive on the left of each segment's start→end direction). Each source
// line gets an offset copy held parallel at distance d by an [Offset]
// constraint; offset segments that meet at a shared source corner share a
// single offset point, which the two constraints pull to the offset
// intersection — so editing the distance keeps a mitred chain. Non-line
// entities in ents are skipped. It returns [ErrForeignEntity] if s is nil or a
// source entity, or a point defining one, is not live in s, and
// [ErrInvalidShape] if a source line has zero length (no defined offset
// direction).
```

with:

```go
// lines (left in place), their offset copies, the per-segment offset
// constraints, and the constraints that hold each free chain end on the source
// normal. Use [OffsetGroup.Set] to retarget the whole offset distance at once
// and re-solve.
type OffsetGroup struct {
	Source      []Entity
	Copies      []Entity
	Offsets     []*Offset
	Constraints []Constraint
	signs       []float64 // per Offset: its target is signs[i]·d
}

// Set retargets the whole offset to distance d (positive to the right of the
// flow, as [CreateOffset] takes it). Call [Sketch.Solve] afterwards to apply it.
func (g *OffsetGroup) Set(d float64) {
	for i, o := range g.Offsets {
		o.Set(g.signs[i] * d)
	}
}

// CreateOffset creates a parallel offset of a chain of lines at signed distance
// d, as Fusion's offset does: positive d is to the right of the flow, negative
// to the left. The flow of a chain runs from its first line in ents toward the
// next line in ents that shares an end point with it; a lone line flows from
// Start to End. Each source line gets an offset copy held parallel by an
// [Offset] constraint; offset segments that meet at a shared source corner share
// a single offset point, which the two constraints pull to the offset
// intersection — so editing the distance keeps a mitred chain. Each free end of
// a chain is held on the source line's normal at the source end
// ([NewPointOnNormal]), so the copy is fully determined by the source and d.
// Non-line entities in ents are skipped. It returns [ErrForeignEntity] if s is
// nil or a source entity, or a point defining one, is not live in s,
// [ErrReferenceGeometry] for reference geometry, and [ErrInvalidShape] if a
// source line has zero length (no defined offset direction) or an end point is
// shared by three or more lines (not a chain).
```

Edit 2. Replace exactly:

```go
	// unchanged (no partial geometry committed).
	for _, e := range ents {
```

with:

```go
	// unchanged (no partial geometry committed).
	var lines []*Line
	for _, e := range ents {
```

Edit 3. Replace exactly:

```go
			return nil, fmt.Errorf("%w: CreateOffset source line has zero length", ErrInvalidShape)
		}
	}
	g := &OffsetGroup{Source: ents}
```

with:

```go
			return nil, fmt.Errorf("%w: CreateOffset source line has zero length", ErrInvalidShape)
		}
		lines = append(lines, l)
	}
	flow, err := offsetFlow(lines)
	if err != nil {
		return nil, err
	}
	g := &OffsetGroup{Source: ents}
```

Edit 4. Replace exactly:

```go
	for _, e := range ents {
		l, ok := e.(*Line)
		if !ok {
			continue
		}
		dx, dy := l.End.x()-l.Start.x(), l.End.y()-l.Start.y()
		n := math.Hypot(dx, dy) // nonzero: validated above
		nx, ny := -dy/n, dx/n   // left normal of the segment direction
		qa, qb := offsetPt(l.Start, nx, ny), offsetPt(l.End, nx, ny)
		dst := s.CreateLine(qa, qb)
		off := NewOffset(l, dst, d)
		s.AddConstraint(off)
		g.Copies = append(g.Copies, dst)
		g.Offsets = append(g.Offsets, off)
	}
	return g, nil
```

with:

```go
	for i, l := range lines {
		// Offset's target is left-positive along the line's own direction; the
		// right of the flow is the left of the line when the flow runs against it.
		sign := -flow[i]
		dx, dy := l.End.x()-l.Start.x(), l.End.y()-l.Start.y()
		n := math.Hypot(dx, dy)         // nonzero: validated above
		nx, ny := -sign*dy/n, sign*dx/n // unit normal toward the offset side
		qa, qb := offsetPt(l.Start, nx, ny), offsetPt(l.End, nx, ny)
		dst := s.CreateLine(qa, qb)
		off := NewOffset(l, dst, sign*d)
		s.AddConstraint(off)
		g.Copies = append(g.Copies, dst)
		g.Offsets = append(g.Offsets, off)
		g.signs = append(g.signs, sign)
	}
	// A free chain end (a source end point no other source line uses) is held on
	// the source normal there, so the copy cannot slide along its own line.
	uses := map[*Point]int{}
	for _, l := range lines {
		uses[l.Start]++
		uses[l.End]++
	}
	for _, l := range lines {
		for _, p := range []*Point{l.Start, l.End} {
			if uses[p] != 1 {
				continue
			}
			pin := NewPointOnNormal(copyOf[p], l, p)
			s.AddConstraint(pin)
			g.Constraints = append(g.Constraints, pin)
		}
	}
	return g, nil
}

// offsetFlow returns, per line, +1 when the offset flow runs along the line's
// own Start→End direction and -1 when it runs against it. Lines sharing end
// points form a chain. A chain's flow runs from its first line (in the given
// order) toward the next line in that order that shares a point with it, as
// Fusion's offset flows from curve 1 to curve 2; a lone line flows from Start to
// End. An end point shared by three or more lines is not a chain:
// [ErrInvalidShape].
func offsetFlow(lines []*Line) ([]float64, error) {
	at := map[*Point][]int{}
	for i, l := range lines {
		at[l.Start] = append(at[l.Start], i)
		at[l.End] = append(at[l.End], i)
	}
	for _, l := range lines {
		for _, p := range []*Point{l.Start, l.End} {
			if len(at[p]) > 2 {
				return nil, fmt.Errorf("%w: CreateOffset: an end point is shared by %d lines, which is not a chain", ErrInvalidShape, len(at[p]))
			}
		}
	}
	// other returns the line other than i that uses p, or -1.
	other := func(i int, p *Point) int {
		for _, j := range at[p] {
			if j != i {
				return j
			}
		}
		return -1
	}
	flow := make([]float64, len(lines))
	// walk signs the lines reached from line `from` through point p: forward
	// means the flow enters the next line at p, backward that it leaves it there.
	walk := func(from int, p *Point, forward bool) {
		for {
			j := other(from, p)
			if j < 0 || flow[j] != 0 {
				return
			}
			lj := lines[j]
			if (forward && lj.Start == p) || (!forward && lj.End == p) {
				flow[j] = 1
			} else {
				flow[j] = -1
			}
			if lj.Start == p {
				p = lj.End
			} else {
				p = lj.Start
			}
			from = j
		}
	}
	for i, l := range lines {
		if flow[i] != 0 {
			continue // already reached from an earlier line of its chain
		}
		toStart, toEnd := other(i, l.Start), other(i, l.End)
		flow[i] = 1
		if toStart >= 0 && (toEnd < 0 || toStart < toEnd) {
			flow[i] = -1 // the earliest neighbour is at Start: the flow leaves through Start
		}
		exit, entry := l.End, l.Start
		if flow[i] < 0 {
			exit, entry = l.Start, l.End
		}
		walk(i, exit, true)
		walk(i, entry, false)
	}
	return flow, nil
```

### Step 5.2 — `tools_test.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '// Positive offset is the left normal of +x, i.e. +y.' tools_test.go   # expect 1
grep -c -F -- 'require.InDelta(t, 2, dst.Start.Y(), 1e-6)' tools_test.go   # expect 1
grep -c -F -- '// Offset of l1 sits at y=2; offset of l2 at x=8; the shared corner is (8,2).' tools_test.go   # expect 1
```

Edit 1. Replace exactly:

```go
	// Positive offset is the left normal of +x, i.e. +y.
```

with:

```go
	// As in Fusion, positive offset is to the right of the flow: right of +x is -y.
```

Edit 2. Replace exactly:

```go
	require.InDelta(t, 2, dst.Start.Y(), 1e-6)
	require.InDelta(t, 2, dst.End.Y(), 1e-6)

	// Editing the distance moves the copy.
	g.Set(5)
```

with:

```go
	require.Equal(t, 0, s.DOF(), "the free ends are held on the source normals")
	require.InDelta(t, 0, dst.Start.X(), 1e-6)
	require.InDelta(t, -2, dst.Start.Y(), 1e-6)
	require.InDelta(t, 10, dst.End.X(), 1e-6)
	require.InDelta(t, -2, dst.End.Y(), 1e-6)

	// Editing the distance moves the copy; a negative distance goes left.
	g.Set(-5)
```

Edit 3. Replace exactly:

```go
	// Offset of l1 sits at y=2; offset of l2 at x=8; the shared corner is (8,2).
	require.InDelta(t, 8, d1.End.X(), 1e-6)
	require.InDelta(t, 2, d1.End.Y(), 1e-6)
```

with:

```go
	// The flow runs l1 then l2, and positive is to its right: the offset of l1
	// sits at y=-2, the offset of l2 at x=12; the shared corner is (12,-2).
	require.InDelta(t, 12, d1.End.X(), 1e-6)
	require.InDelta(t, -2, d1.End.Y(), 1e-6)
	require.Equal(t, 0, s.DOF())
```

### Step 5.3 — new file `tools_offset_flow_test.go`

This file must not exist yet: `test -e tools_offset_flow_test.go && echo EXISTS` prints nothing.

```go
package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// The flow of a chain follows the order of the list, not each line's own
// direction: two lines drawn toward their shared corner still offset to one
// side.
func TestOffsetFlowFollowsListOrder(t *testing.T) {
	s := newSketch(t)
	p0, p1, p2 := s.CreatePoint(0, 0), s.CreatePoint(10, 0), s.CreatePoint(10, 10)
	s.Fix(p0)
	s.Fix(p1)
	s.Fix(p2)
	l1 := s.CreateLine(p0, p1) // runs along the flow
	l2 := s.CreateLine(p2, p1) // drawn against the flow

	g, err := sketch.CreateOffset(s, []sketch.Entity{l1, l2}, 2)
	require.NoError(t, err)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)

	d1, d2 := g.Copies[0].(*sketch.Line), g.Copies[1].(*sketch.Line)
	require.Same(t, d1.End, d2.End, "the copies share the corner point")
	require.InDelta(t, 12, d1.End.X(), 1e-6, "right of the flow p0→p1→p2, as for a chain drawn in order")
	require.InDelta(t, -2, d1.End.Y(), 1e-6)
	require.InDelta(t, 12, d2.Start.X(), 1e-6)
	require.InDelta(t, 10, d2.Start.Y(), 1e-6, "the free end sits on the normal at p2")
	require.Equal(t, 0, s.DOF())

	g.Set(1)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 11, d1.End.X(), 1e-6)
	require.InDelta(t, -1, d1.End.Y(), 1e-6)
}

// A closed loop has no free end, so no end is pinned, and the copy mitres at
// every corner.
func TestOffsetClosedLoop(t *testing.T) {
	s := newSketch(t)
	a, b, c, d := s.CreatePoint(0, 0), s.CreatePoint(10, 0), s.CreatePoint(10, 10), s.CreatePoint(0, 10)
	for _, p := range []*sketch.Point{a, b, c, d} {
		s.Fix(p)
	}
	loop := []sketch.Entity{s.CreateLine(a, b), s.CreateLine(b, c), s.CreateLine(c, d), s.CreateLine(d, a)}

	// The loop runs counter-clockwise, so its right is outside.
	g, err := sketch.CreateOffset(s, loop, 1)
	require.NoError(t, err)
	require.Empty(t, g.Constraints, "a closed loop has no free end")
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.Equal(t, 0, s.DOF())
	first := g.Copies[0].(*sketch.Line)
	require.InDelta(t, -1, first.Start.X(), 1e-6)
	require.InDelta(t, -1, first.Start.Y(), 1e-6)
}

// Three lines meeting at one point are not a chain.
func TestOffsetRefusesBranch(t *testing.T) {
	s := newSketch(t)
	hub := s.CreatePoint(0, 0)
	ents := []sketch.Entity{
		s.CreateLine(hub, s.CreatePoint(10, 0)),
		s.CreateLine(hub, s.CreatePoint(0, 10)),
		s.CreateLine(hub, s.CreatePoint(-10, 0)),
	}
	pts, cons := len(s.Points()), len(s.Constraints())

	_, err := sketch.CreateOffset(s, ents, 1)
	require.ErrorIs(t, err, sketch.ErrInvalidShape)
	require.Len(t, s.Points(), pts)
	require.Len(t, s.Constraints(), cons)
}
```

### Step 5.4 — `docs/modification-tools-design.md`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'the offset intersection — so chains mitre. `OffsetGroup.Set(d)` retargets the' docs/modification-tools-design.md   # expect 1
```

Edit 1. Replace exactly:

```markdown
  the offset intersection — so chains mitre. `OffsetGroup.Set(d)` retargets the
  whole offset.
```

with:

```markdown
  the offset intersection — so chains mitre. As in Fusion, `CreateOffset`'s `d`
  is positive to the RIGHT of the flow, and the flow of a chain runs from its
  first line in the list toward the next line that shares a point with it
  (`offsetFlow`); each `Offset` stores the matching left-positive target. Every
  free chain end is held on the source normal by `NewPointOnNormal`, so an open
  offset is fully determined. `OffsetGroup.Set(d)` retargets the whole offset.
```

### Step 5.5 — verify

Focused tests (all must pass):

```sh
go test -count=1 -run 'Offset|Tools|Reference|EntityUID' .
```

Then the common "Verify" commands.

### Step 5.6 — commit and PR

- **Commit message:** `offset line chains the way Fusion does`
- **Files to add:** `tools.go`, `tools_test.go`, `tools_offset_flow_test.go`, `docs/modification-tools-design.md`
- **PR title:** `offset line chains the way Fusion does`
- **PR body:**

  ```markdown
  - Offset to the right of the flow for positive distances and follow the order of the list, as Fusion does
  - Hold the free ends of an open offset on the source normals so the copy is fully determined
  ```

## Increment 6 — `Trim` on arcs, `TrimCircle`

- **Branch:** `feat-trim-arc-circle`
- **Design section:** "Trim on arcs and circles".
- **Merge after:** 5
- **Files:** `geom/intersect.go`, `tools.go`, `entity_switches_test.go`, `docs/modification-tools-design.md` (edits);
  `geom/arc_angle_test.go`, `tools_trim_arc_test.go` (new).

### Step 6.0 — sync and check the base

From the worktree root:

```sh
git fetch origin
git rebase origin/main
git log --oneline origin/main | grep -c -F -- 'offset line chains the way Fusion does'   # expect 1 or more
```

If the rebase stops, or a count is 0, stop and report. Then do the steps below in order. Every step starts
with its own `grep` checks: run them before editing that file.

### Step 6.1 — `geom/intersect.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'func (a *Arc) Contains(p *Point) bool {' geom/intersect.go   # expect 1
grep -c -F -- 'return d <= a.Sweep()+epsIntersect' geom/intersect.go   # expect 1
```

Edit 1. Replace exactly:

```go
// Contains reports whether p's angle about the arc's center lies within the
// arc's counter-clockwise sweep (endpoints inclusive, with a small angular
// tolerance). The point is not required to lie on the arc's circle.
func (a *Arc) Contains(p *Point) bool {
```

with:

```go
// AngleFromStart returns the counter-clockwise angle from a's start direction to
// p's direction about a's center, in [0, 2π]. p's direction is inside the sweep
// when the result is at most a.Sweep(). The point is not required to lie on the
// arc's circle.
func (a *Arc) AngleFromStart(p *Point) float64 {
```

Edit 2. Replace exactly:

```go
	return d <= a.Sweep()+epsIntersect
```

with:

```go
	return d
}

// Contains reports whether p's angle about the arc's center lies within the
// arc's counter-clockwise sweep (endpoints inclusive, with a small angular
// tolerance). The point is not required to lie on the arc's circle.
func (a *Arc) Contains(p *Point) bool {
	return a.AngleFromStart(p) <= a.Sweep()+epsIntersect
```

### Step 6.2 — new file `geom/arc_angle_test.go`

This file must not exist yet: `test -e geom/arc_angle_test.go && echo EXISTS` prints nothing.

```go
package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func TestArcAngleFromStart(t *testing.T) {
	// A quarter arc from 90° to 180° about (1, 1).
	a := geom.NewArc(geom.NewPoint(1, 1), geom.NewPoint(1, 3), geom.NewPoint(-1, 1))
	require.InDelta(t, 0, a.AngleFromStart(geom.NewPoint(1, 5)), 1e-12, "along the start direction")
	require.InDelta(t, math.Pi/4, a.AngleFromStart(geom.NewPoint(0, 2)), 1e-12)
	require.InDelta(t, math.Pi/2, a.AngleFromStart(geom.NewPoint(-4, 1)), 1e-12, "the end direction")
	require.InDelta(t, 3*math.Pi/2, a.AngleFromStart(geom.NewPoint(2, 1)), 1e-12, "counter-clockwise past the end")
	require.True(t, a.Contains(geom.NewPoint(0, 2)))
	require.False(t, a.Contains(geom.NewPoint(2, 1)))
}
```

### Step 6.3 — `tools.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'import (' tools.go   # expect 1
grep -c -F -- '// reference geometry. Lines are handled; an arc returns nil and false.' tools.go   # expect 1
grep -c -F -- 'return nil, false // arcs are not trimmed yet' tools.go   # expect 1
grep -c -F -- 'return []*Line{first, second}' tools.go   # expect 1
```

Edit 1. Replace exactly:

```go
import (
	"errors"
```

with:

```go
import (
	"cmp"
	"errors"
```

Edit 2. Replace exactly:

```go
// reference geometry. Lines are handled; an arc returns nil and false.
```

with:

```go
// reference geometry. Lines and arcs are both handled; a circle is trimmed by
// [TrimCircle].
```

Edit 3. Replace exactly:

```go
	l, ok := ent.(*Line)
	if !ok {
		return nil, false // arcs are not trimmed yet
	}
	pieces := s.trimLine(l, x, y)
```

with:

```go
	var pieces []Entity
	if l, ok := ent.(*Line); ok {
		for _, p := range s.trimLine(l, x, y) {
			pieces = append(pieces, p)
		}
	} else if a, ok := ent.(*Arc); ok {
		for _, p := range s.trimArc(a, x, y) {
			pieces = append(pieces, p)
		}
	}
```

Edit 4. Replace exactly:

```go
	return []*Line{first, second}
}
```

with:

```go
	return []*Line{first, second}
}

// trimArc is Trim for a screened arc: the same three outcomes as trimLine,
// measured by the angle from the arc's start along its sweep.
func (s *Sketch) trimArc(a *Arc, x, y float64) []*Arc {
	ga := a.Geometry()
	sweep := ga.Sweep()
	pick := ga.AngleFromStart(geom.NewPoint(x, y)) / sweep
	if pick > 1 { // the pick's direction is off the arc: take the nearer end
		if (pick-1)*sweep < 2*math.Pi-pick*sweep {
			pick = 1
		} else {
			pick = 0
		}
	}
	hits := s.circleCrossings(geom.NewCircle(ga.Center, ga.Radius()), a)

	loT, hiT := 0.0, 1.0
	var loP, hiP *crossing
	for i := range hits {
		h := &hits[i]
		ht := ga.AngleFromStart(h.at) / sweep
		if ht <= epsTool || ht >= 1-epsTool {
			continue // an end of the arc, or off the sweep
		}
		if ht < pick && ht > loT {
			loT, loP = ht, h
		}
		if ht > pick && ht < hiT {
			hiT, hiP = ht, h
		}
	}

	switch {
	case loP == nil && hiP == nil: // nothing crosses the picked portion: delete the arc
		center, start, end := a.Center, a.Start, a.End
		s.RemoveEntity(a)
		s.RemovePoint(start)
		s.RemovePoint(end)
		s.RemovePoint(center)
		return []*Arc{}
	case loP == nil: // crossing only after the pick: cut the start stub in place
		s.moveEnd(a, a.Start, s.pointOnCutter(*hiP))
		return []*Arc{a}
	case hiP == nil: // crossing only before the pick: cut the end stub in place
		s.moveEnd(a, a.End, s.pointOnCutter(*loP))
		return []*Arc{a}
	}
	// Crossings on both sides: delete the arc and keep the two outer pieces.
	first := s.CreateArc(a.Center, a.Start, s.pointOnCutter(*loP))
	second := s.CreateArc(a.Center, s.pointOnCutter(*hiP), a.End)
	s.RemoveEntity(a)
	return []*Arc{first, second}
}

// TrimCircle removes the portion of c between the two crossings with other
// entities that bound the pick point (x, y), as Fusion's trim of a closed curve
// does: c is deleted and the remaining arc is returned, both of its ends tied to
// their cutters. The radius, diameter, concentric and equal-radius constraints
// that named c are moved to the arc, so their handles stay valid; every other
// constraint naming c is removed with it. With fewer than two distinct
// crossings c is deleted (with its center point when nothing else uses it) and
// an empty slice returned. It returns nil and false, changing nothing, when s is
// nil, when c or its center is not live in s, when c is reference geometry, or
// when the pick lies exactly on a crossing.
func TrimCircle(s *Sketch, c *Circle, x, y float64) ([]*Arc, bool) {
	if s.foreignInput(c) {
		return nil, false // nil, removed, or another sketch's circle
	}
	if c.IsReference() {
		return nil, false // reference geometry is locked; cannot be trimmed
	}
	gc := c.Geometry()
	angle := func(p *geom.Point) float64 {
		v := math.Atan2(p.Y-gc.Center.Y, p.X-gc.Center.X)
		if v < 0 {
			v += 2 * math.Pi
		}
		return v
	}
	hits := s.circleCrossings(gc, c)
	slices.SortFunc(hits, func(p, q crossing) int { return cmp.Compare(angle(p.at), angle(q.at)) })
	// Merge crossings that coincide (two cutters through one point, a tangent).
	var distinct []crossing
	for _, h := range hits {
		if len(distinct) > 0 && (angle(h.at)-angle(distinct[len(distinct)-1].at))*gc.Radius <= epsTool {
			continue
		}
		distinct = append(distinct, h)
	}
	if n := len(distinct); n > 1 && (angle(distinct[0].at)+2*math.Pi-angle(distinct[n-1].at))*gc.Radius <= epsTool {
		distinct = distinct[:n-1] // the last crossing wraps onto the first
	}
	if len(distinct) < 2 {
		center := c.Center
		s.RemoveEntity(c)
		s.RemovePoint(center)
		return []*Arc{}, true
	}
	pick := angle(geom.NewPoint(x, y))
	n := len(distinct)
	for i := range distinct {
		lo, hi := angle(distinct[i].at), angle(distinct[(i+1)%n].at)
		if math.Abs(pick-lo)*gc.Radius <= epsTool {
			return nil, false // the pick is on a crossing: no portion is picked
		}
		inside := lo < pick && pick < hi
		if i == n-1 { // the portion that wraps through angle 0
			inside = pick > lo || pick < hi
		}
		if !inside {
			continue
		}
		// Keep the arc running counter-clockwise from hi round to lo.
		arc := s.CreateArc(c.Center, s.pointOnCutter(distinct[(i+1)%n]), s.pointOnCutter(distinct[i]))
		s.moveCircleConstraints(c, arc)
		s.RemoveEntity(c)
		return []*Arc{arc}, true
	}
	return nil, false
}

// moveCircleConstraints rewrites to arc the operand of every radius, diameter,
// concentric and equal-radius constraint that names c. None of the four owns an
// auxiliary variable, and none depends on which part of the circle survives.
func (s *Sketch) moveCircleConstraints(c *Circle, arc *Arc) {
	for _, k := range s.cons {
		switch t := k.(type) {
		case *Radius:
			if t.C == Circular(c) {
				t.C = arc
			}
		case *Diameter:
			if t.C == Circular(c) {
				t.C = arc
			}
		case *concentric:
			if t.C1 == Circular(c) {
				t.C1 = arc
			}
			if t.C2 == Circular(c) {
				t.C2 = arc
			}
		case *equalRadii:
			if t.C1 == Circular(c) {
				t.C1 = arc
			}
			if t.C2 == Circular(c) {
				t.C2 = arc
			}
		}
	}
}

// circleCrossings returns where the full circle c meets every entity except
// skip, with the entity met: a line cutter within its own segment, a circle, or
// an arc within its sweep.
func (s *Sketch) circleCrossings(c *geom.Circle, skip Entity) []crossing {
	var out []crossing
	for _, e := range s.ents {
		if e == skip {
			continue
		}
		switch t := e.(type) {
		case *Line:
			ol := t.Geometry()
			for _, p := range geom.LineCircleIntersections(ol, c) {
				if _, u := geom.ClosestPointOnLine(ol, p); u >= -epsTool && u <= 1+epsTool {
					out = append(out, crossing{p, t})
				}
			}
		case *Circle:
			for _, p := range geom.CircleCircleIntersections(c, t.Geometry()) {
				out = append(out, crossing{p, t})
			}
		case *Arc:
			for _, p := range geom.CircleArcIntersections(c, t.Geometry()) {
				out = append(out, crossing{p, t})
			}
		}
	}
	return out
}
```

### Step 6.4 — `entity_switches_test.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '{"tools.go", "pinToCutter"}:              "only line/circle/arc cutters' entity_switches_test.go   # expect 1
```

Edit 1. Replace exactly:

```go
	{"tools.go", "pinToCutter"}:              "only line/circle/arc cutters produce a crossing (lineCrossings), so only they are pinned to",
	{"tools.go", "lineCrossings"}:            "only line/circle/arc cutters have a closed-form line intersection in geom; a curve contributes no crossing",
```

with:

```go
	{"tools.go", "pinToCutter"}:              "only line/circle/arc cutters produce a crossing (lineCrossings), so only they are pinned to",
	{"tools.go", "circleCrossings"}:          "only line/circle/arc cutters have a closed-form circle intersection in geom; a curve contributes no crossing",
	{"tools.go", "lineCrossings"}:            "only line/circle/arc cutters have a closed-form line intersection in geom; a curve contributes no crossing",
```

### Step 6.5 — new file `tools_trim_arc_test.go`

This file must not exist yet: `test -e tools_trim_arc_test.go && echo EXISTS` prints nothing.

```go
package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// Trimming the start stub of an arc moves its Start in place to the crossing,
// keeps the center and End, and ties the new Start to the cutter.
func TestTrimArcEndStub(t *testing.T) {
	s := newSketch(t)
	center, end := s.CreatePoint(0, 0), s.CreatePoint(0, 5)
	arc := s.CreateArc(center, s.CreatePoint(5, 0), end)
	s.CreateLine(s.CreatePoint(3, -1), s.CreatePoint(3, 10)) // crosses the arc at (3, 4)

	res, ok := sketch.Trim(s, arc, 4.9, 1)
	require.True(t, ok)
	require.Equal(t, []*sketch.Arc{arc}, res)
	require.Same(t, center, arc.Center)
	require.Same(t, end, arc.End)
	require.InDelta(t, 3, arc.Start.X(), 1e-9)
	require.InDelta(t, 4, arc.Start.Y(), 1e-9)
	require.Equal(t, 1, countKind(s, "point_on_line"))
}

// Picking between two crossings deletes the arc and keeps the two outer pieces.
func TestTrimArcMiddle(t *testing.T) {
	s := newSketch(t)
	center, start, end := s.CreatePoint(0, 0), s.CreatePoint(5, 0), s.CreatePoint(0, 5)
	arc := s.CreateArc(center, start, end)
	s.CreateLine(s.CreatePoint(0, 2.5), s.CreatePoint(10, 2.5)) // crosses at 30°
	s.CreateLine(s.CreatePoint(2.5, 0), s.CreatePoint(2.5, 10)) // crosses at 60°

	res, ok := sketch.Trim(s, arc, 3.5, 3.5) // pick at 45°
	require.True(t, ok)
	require.Len(t, res, 2)
	require.Same(t, start, res[0].Start)
	require.InDelta(t, 5*math.Cos(math.Pi/6), res[0].End.X(), 1e-9)
	require.InDelta(t, 2.5, res[0].End.Y(), 1e-9)
	require.InDelta(t, 2.5, res[1].Start.X(), 1e-9)
	require.Same(t, end, res[1].End)
	require.Same(t, center, res[0].Center)
	require.Same(t, center, res[1].Center)
	require.NotContains(t, s.Entities(), sketch.Entity(arc))
}

// An arc that crosses nothing is deleted, with its points.
func TestTrimArcNoCrossingDeletes(t *testing.T) {
	s := newSketch(t)
	arc := s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(5, 0), s.CreatePoint(0, 5))

	res, ok := sketch.Trim(s, arc, 4, 3)
	require.True(t, ok)
	require.Empty(t, res)
	require.Empty(t, s.Entities())
	require.Empty(t, s.Points())
}

// Trimming a circle deletes it and returns the remaining arc; its radius
// dimension moves to the arc and keeps driving it.
func TestTrimCircle(t *testing.T) {
	s := newSketch(t)
	center := s.CreatePoint(0, 0)
	c := s.CreateCircle(center, 5)
	radius := sketch.NewRadius(c, 5)
	s.AddConstraint(radius)
	s.CreateLine(s.CreatePoint(3, -10), s.CreatePoint(3, 10)) // crosses at (3, ±4)
	dof := s.DOF()

	res, ok := sketch.TrimCircle(s, c, 5, 0)
	require.True(t, ok)
	require.Len(t, res, 1)
	arc := res[0]
	require.Same(t, center, arc.Center)
	require.InDelta(t, 3, arc.Start.X(), 1e-9)
	require.InDelta(t, 4, arc.Start.Y(), 1e-9)
	require.InDelta(t, 3, arc.End.X(), 1e-9)
	require.InDelta(t, -4, arc.End.Y(), 1e-9)
	require.InDelta(t, 2*math.Pi-2*math.Atan2(4, 3), arc.Sweep(), 1e-9)
	require.Equal(t, 2, countKind(s, "point_on_line"))
	_, ents := sketch.ConstraintRefs(radius)
	require.Equal(t, []sketch.Entity{arc}, ents, "the radius dimension now names the arc")
	require.Equal(t, dof, s.DOF())

	radius.Set(6)
	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 6, arc.R(), 1e-6)
}

// A circle with fewer than two crossings is deleted; a pick on a crossing picks
// nothing.
func TestTrimCircleEdgeCases(t *testing.T) {
	t.Run("tangent cutter deletes", func(t *testing.T) {
		s := newSketch(t)
		c := s.CreateCircle(s.CreatePoint(0, 0), 5)
		s.CreateLine(s.CreatePoint(5, -10), s.CreatePoint(5, 10)) // touches at (5, 0)

		res, ok := sketch.TrimCircle(s, c, -5, 0)
		require.True(t, ok)
		require.Empty(t, res)
		require.Len(t, s.Entities(), 1, "only the cutter is left")
		require.Len(t, s.Points(), 2, "the circle's center is removed")
	})

	t.Run("pick on a crossing", func(t *testing.T) {
		s := newSketch(t)
		c := s.CreateCircle(s.CreatePoint(0, 0), 5)
		s.CreateLine(s.CreatePoint(3, -10), s.CreatePoint(3, 10))

		res, ok := sketch.TrimCircle(s, c, 3, 4)
		require.False(t, ok)
		require.Nil(t, res)
		require.Len(t, s.Entities(), 2, "nothing changed")
	})
}

// Foreign and reference curves are refused with nothing changed.
func TestTrimArcAndCircleRefusals(t *testing.T) {
	other := newSketch(t)
	farc := other.CreateArc(other.CreatePoint(0, 0), other.CreatePoint(5, 0), other.CreatePoint(0, 5))
	fcircle := other.CreateCircle(other.CreatePoint(0, 0), 5)

	s := newSketch(t)
	s.CreateLine(s.CreatePoint(3, -10), s.CreatePoint(3, 10))
	res, ok := sketch.Trim(s, farc, 4.9, 1)
	require.False(t, ok)
	require.Nil(t, res)
	arcs, ok := sketch.TrimCircle(s, fcircle, 5, 0)
	require.False(t, ok)
	require.Nil(t, arcs)

	p1 := s.CreateReferencePoint(0, 0, "c")
	p2 := s.CreateReferencePoint(5, 0, "s")
	p3 := s.CreateReferencePoint(0, 5, "e")
	ref, err := s.CreateReferenceArc(p1, p2, p3, "edge")
	require.NoError(t, err)
	res, ok = sketch.Trim(s, ref, 4.9, 1)
	require.False(t, ok)
	require.Nil(t, res)
	require.Len(t, s.Entities(), 2)
}
```

### Step 6.6 — `docs/modification-tools-design.md`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '(`pointOnCutter`).' docs/modification-tools-design.md   # expect 1
grep -c -F -- '- **Trim/Break of circles** (needs two break points) and ellipse mirroring.' docs/modification-tools-design.md   # expect 1
```

Edit 1. Replace exactly:

```markdown
  (`pointOnCutter`).
```

with:

```markdown
  (`pointOnCutter`). An arc is trimmed the same way, measured along its sweep
  (`trimArc`, `circleCrossings`). `TrimCircle` deletes a circle and returns the
  arc that remains, moving the circle's radius, diameter, concentric and
  equal-radius constraints onto it.
```

Edit 2. Replace exactly:

```markdown
- **Trim/Break of circles** (needs two break points) and ellipse mirroring.
```

with:

```markdown
- **Break of circles** (needs two break points; `TrimCircle` covers the profile
  use) and ellipse mirroring.
```

### Step 6.7 — verify

Focused tests (all must pass):

```sh
go test -count=1 -run 'TrimArc|TrimCircle|Trim|EntityTypeSwitches|Tools' .
go test -count=1 -run 'TestArcAngleFromStart|TestBreak|Contains' ./geom
```

Then the common "Verify" commands.

### Step 6.8 — commit and PR

- **Commit message:** `trim arcs and circles`
- **Files to add:** `geom/intersect.go`, `geom/arc_angle_test.go`, `tools.go`, `entity_switches_test.go`,
  `tools_trim_arc_test.go`, `docs/modification-tools-design.md`
- **PR title:** `trim arcs and circles`
- **PR body:**

  ```markdown
  - Trim arcs the way lines are trimmed, and trim a circle into the arc that remains
  ```

## Increment 7 — `Extend` on arcs

- **Branch:** `feat-extend-arc`
- **Design section:** "Extend on arcs".
- **Merge after:** 6
- **Files:** `tools.go`, `docs/modification-tools-design.md` (edits); `tools_extend_arc_test.go` (new).

### Step 7.0 — sync and check the base

From the worktree root:

```sh
git fetch origin
git rebase origin/main
git log --oneline origin/main | grep -c -F -- 'trim arcs and circles'   # expect 1 or more
```

If the rebase stops, or a count is 0, stop and report. Then do the steps below in order. Every step starts
with its own `grep` checks: run them before editing that file.

### Step 7.1 — `tools.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '// the old end point is removed when nothing else uses it. Lines are handled; an' tools.go   # expect 1
grep -c -F -- 'return zero, false // arcs are not extended yet' tools.go   # expect 1
```

Edit 1. Replace exactly:

```go
// the old end point is removed when nothing else uses it. Lines are handled; an
// arc returns false.
```

with:

```go
// the old end point is removed when nothing else uses it. An arc is extended
// along its own circle; an extension that would reach the arc's other end (and
// close it into a circle, which Fusion returns) is refused with false.
```

Edit 2. Replace exactly:

```go
	l, ok := ent.(*Line)
	if !ok {
		return zero, false // arcs are not extended yet
	}
	nl, ok := s.extendLine(l, end)
	if !ok {
		return zero, false
	}
	return asCurve[T](nl), true
```

with:

```go
	var got Entity
	ok := false
	if l, isLine := ent.(*Line); isLine {
		got, ok = s.extendLine(l, end)
	} else if a, isArc := ent.(*Arc); isArc {
		got, ok = s.extendArc(a, end)
	}
	if !ok {
		return zero, false
	}
	return asCurve[T](got), true
}

// extendArc is Extend for a screened arc: it lengthens a along its own circle
// from end to the nearest crossing beyond it.
func (s *Sketch) extendArc(a *Arc, end *Point) (*Arc, bool) {
	if end != a.Start && end != a.End {
		return nil, false
	}
	ga := a.Geometry()
	sweep := ga.Sweep()
	hits := s.circleCrossings(geom.NewCircle(ga.Center, ga.Radius()), a)

	fromStart := end == a.Start
	bestGap := math.Inf(1)
	var best *crossing
	for i := range hits {
		h := &hits[i]
		phi := ga.AngleFromStart(h.at)
		// The gap is the angle swept beyond the chosen end to reach the hit:
		// clockwise from Start, counter-clockwise from End.
		gap := phi - sweep
		if fromStart {
			gap = 2*math.Pi - phi
		}
		if gap <= epsTool || gap >= 2*math.Pi-sweep-epsTool {
			continue // on the arc itself, or only reachable past the other end
		}
		if gap < bestGap {
			bestGap, best = gap, h
		}
	}
	if best == nil {
		return nil, false
	}
	s.moveEnd(a, end, s.pointOnCutter(*best))
	return a, true
```

### Step 7.2 — new file `tools_extend_arc_test.go`

This file must not exist yet: `test -e tools_extend_arc_test.go && echo EXISTS` prints nothing.

```go
package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// Extending an arc's End moves it in place along the arc's circle to the
// nearest crossing beyond it, tied to the cutter.
func TestExtendArcToCutter(t *testing.T) {
	s := newSketch(t)
	center, start := s.CreatePoint(0, 0), s.CreatePoint(5, 0)
	arc := s.CreateArc(center, start, s.CreatePoint(0, 5))
	s.CreateLine(s.CreatePoint(-3, 0), s.CreatePoint(-3, 10)) // meets the circle at (-3, 4)

	got, ok := sketch.Extend(s, arc, arc.End)
	require.True(t, ok)
	require.Same(t, arc, got)
	require.Same(t, center, arc.Center)
	require.Same(t, start, arc.Start)
	require.InDelta(t, -3, arc.End.X(), 1e-9)
	require.InDelta(t, 4, arc.End.Y(), 1e-9)
	require.Equal(t, 1, countKind(s, "point_on_line"))
}

// Extending the Start runs clockwise.
func TestExtendArcStart(t *testing.T) {
	s := newSketch(t)
	arc := s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(5, 0), s.CreatePoint(0, 5))
	s.CreateLine(s.CreatePoint(0, -3), s.CreatePoint(10, -3)) // meets the circle at (4, -3)

	_, ok := sketch.Extend(s, arc, arc.Start)
	require.True(t, ok)
	require.InDelta(t, 4, arc.Start.X(), 1e-9)
	require.InDelta(t, -3, arc.Start.Y(), 1e-9)
}

// A cutter reachable only by wrapping past the other end is refused: Fusion
// would close the arc into a circle, which Extend cannot return.
func TestExtendArcRefusesWrap(t *testing.T) {
	s := newSketch(t)
	arc := s.CreateArc(s.CreatePoint(0, 0), s.CreatePoint(5, 0), s.CreatePoint(0, 5))
	s.CreateLine(s.CreatePoint(2, 2), s.CreatePoint(6, 6)) // crosses inside the sweep at 45°
	pts, cons := len(s.Points()), len(s.Constraints())

	got, ok := sketch.Extend(s, arc, arc.End)
	require.False(t, ok)
	require.Nil(t, got)
	require.Len(t, s.Points(), pts)
	require.Len(t, s.Constraints(), cons)
	require.InDelta(t, 5, arc.End.Y(), 1e-12, "the arc is unchanged")
}
```

### Step 7.3 — `docs/modification-tools-design.md`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'there in place.' docs/modification-tools-design.md   # expect 1
```

Edit 1. Replace exactly:

```markdown
  there in place.
```

with:

```markdown
  there in place. An arc is extended along its own circle (`extendArc`); a
  crossing reachable only past the arc's other end is refused, where Fusion would
  return a circle.
```

### Step 7.4 — verify

Focused tests (all must pass):

```sh
go test -count=1 -run 'ExtendArc|Extend|Tools' .
```

Then the common "Verify" commands.

### Step 7.5 — commit and PR

- **Commit message:** `extend arcs along their circle`
- **Files to add:** `tools.go`, `tools_extend_arc_test.go`, `docs/modification-tools-design.md`
- **PR title:** `extend arcs along their circle`
- **PR body:**

  ```markdown
  - Extend an arc along its own circle to the nearest crossing beyond the chosen end
  ```

## Increment 8 — `geom` fillet helpers

- **Branch:** `feat-geom-fillet-curves`
- **Design section:** "Fillet on curves", subsection "`geom` helpers".
- **Merge after:** 7
- **Files:** `geom/fillet_curves.go`, `geom/fillet_curves_test.go` (new).

### Step 8.0 — sync and check the base

From the worktree root:

```sh
git fetch origin
git rebase origin/main
git log --oneline origin/main | grep -c -F -- 'extend arcs along their circle'   # expect 1 or more
```

If the rebase stops, or a count is 0, stop and report. Then do the steps below in order. Every step starts
with its own `grep` checks: run them before editing that file.

### Step 8.1 — new file `geom/fillet_curves.go`

This file must not exist yet: `test -e geom/fillet_curves.go && echo EXISTS` prints nothing.

```go
package geom

import "math"

// CornerFillet is the fillet arc that rounds the corner where two curves meet,
// as computed by [FilletLineArc] and [FilletArcArc]. Neither function changes its
// inputs: the caller shortens the curves to T1 and T2.
type CornerFillet struct {
	// Arc is the minor fillet arc. Its Start and End are T1 and T2 themselves
	// (pointer-equal), in counter-clockwise order.
	Arc *Arc
	// T1 and T2 are the contact points on the first and the second curve.
	T1, T2 *Point
	// Inside1 and Inside2 report whether the fillet circle lies inside the first
	// and the second curve's circle (internal tangency). They are always false
	// for a line.
	Inside1, Inside2 bool
}

// filletLeg is one curve of a filleted corner: a line whose End is the corner,
// or an arc whose Start (atStart) or End is the corner.
type filletLeg struct {
	line    *Line
	arc     *Arc
	atStart bool
}

func (g filletLeg) corner() *Point {
	switch {
	case g.line != nil:
		return g.line.End
	case g.atStart:
		return g.arc.Start
	default:
		return g.arc.End
	}
}

// degenerate reports a zero-length line or a zero-radius arc.
func (g filletLeg) degenerate() bool {
	if g.line != nil {
		return g.line.Length() == 0
	}
	return g.arc.Radius() == 0
}

// into returns the unit tangent at the corner pointing into the part of the leg
// that survives the fillet.
func (g filletLeg) into() (float64, float64) {
	c := g.corner()
	if g.line != nil {
		dx, dy := g.line.Start.X-c.X, g.line.Start.Y-c.Y
		n := math.Hypot(dx, dy)
		return dx / n, dy / n
	}
	r := g.arc.Radius()
	tx, ty := -(c.Y-g.arc.Center.Y)/r, (c.X-g.arc.Center.X)/r // counter-clockwise tangent
	if g.atStart {
		return tx, ty
	}
	return -tx, -ty
}

// carrier is a curve every point of which lies at the fillet radius from a leg:
// a line parallel to a line leg, or a circle concentric with an arc leg.
type carrier struct {
	line   *Line
	circle *Circle
	inside bool
}

func (g filletLeg) carriers(r float64) []carrier {
	if g.line != nil {
		dx, dy := g.line.End.X-g.line.Start.X, g.line.End.Y-g.line.Start.Y
		n := math.Hypot(dx, dy)
		nx, ny := -dy/n*r, dx/n*r
		shift := func(sx, sy float64) carrier {
			return carrier{line: NewLine(NewPoint(g.line.Start.X+sx, g.line.Start.Y+sy), NewPoint(g.line.End.X+sx, g.line.End.Y+sy))}
		}
		return []carrier{shift(nx, ny), shift(-nx, -ny)}
	}
	R := g.arc.Radius()
	out := []carrier{{circle: NewCircle(g.arc.Center, R+r)}}
	if R > r {
		out = append(out, carrier{circle: NewCircle(g.arc.Center, R-r), inside: true})
	}
	return out
}

// contact returns where a fillet circle centered at o touches the leg, and
// whether that point lies strictly inside the part of the leg the fillet keeps.
func (g filletLeg) contact(o *Point) (*Point, bool) {
	if g.line != nil {
		p, t := ClosestPointOnLine(g.line, o)
		return p, t > epsIntersect && t < 1-epsIntersect
	}
	a := g.arc
	d := math.Hypot(o.X-a.Center.X, o.Y-a.Center.Y)
	if d == 0 {
		return nil, false
	}
	R := a.Radius()
	p := NewPoint(a.Center.X+R*(o.X-a.Center.X)/d, a.Center.Y+R*(o.Y-a.Center.Y)/d)
	phi, sweep := a.AngleFromStart(p), a.Sweep()
	return p, phi > epsIntersect && phi < sweep-epsIntersect
}

// meet returns the points where two carriers cross.
func meet(c1, c2 carrier) []*Point {
	switch {
	case c1.line != nil && c2.line != nil:
		if p, ok := LineLineIntersection(c1.line, c2.line); ok {
			return []*Point{p}
		}
		return nil
	case c1.line != nil:
		return LineCircleIntersections(c1.line, c2.circle)
	case c2.line != nil:
		return LineCircleIntersections(c2.line, c1.circle)
	default:
		return CircleCircleIntersections(c1.circle, c2.circle)
	}
}

// filletCorner computes the fillet of radius r between two legs meeting at a
// corner. See [FilletLineArc] for the rules.
func filletCorner(g1, g2 filletLeg, r float64) (CornerFillet, bool) {
	if r <= 0 || g1.degenerate() || g2.degenerate() {
		return CornerFillet{}, false
	}
	c1, c2 := g1.corner(), g2.corner()
	if math.Hypot(c1.X-c2.X, c1.Y-c2.Y) > epsIntersect*math.Max(1, math.Hypot(c1.X, c1.Y)) {
		return CornerFillet{}, false // the curves do not meet at one corner
	}
	u1x, u1y := g1.into()
	u2x, u2y := g2.into()
	if math.Abs(u1x*u2y-u1y*u2x) <= epsIntersect {
		return CornerFillet{}, false // tangent at the corner: nothing to round
	}
	best := CornerFillet{}
	bestReach := math.Inf(1)
	for _, k1 := range g1.carriers(r) {
		for _, k2 := range g2.carriers(r) {
			for _, o := range meet(k1, k2) {
				t1, ok1 := g1.contact(o)
				t2, ok2 := g2.contact(o)
				if !ok1 || !ok2 {
					continue
				}
				reach := math.Max(math.Hypot(t1.X-c1.X, t1.Y-c1.Y), math.Hypot(t2.X-c1.X, t2.Y-c1.Y))
				if reach >= bestReach {
					continue
				}
				arc := NewArc(o, t1, t2)
				if arc.Sweep() > math.Pi {
					arc = NewArc(o, t2, t1)
				}
				best = CornerFillet{Arc: arc, T1: t1, T2: t2, Inside1: k1.inside, Inside2: k2.inside}
				bestReach = reach
			}
		}
	}
	return best, best.Arc != nil
}

// FilletLineArc computes the fillet arc of radius r that rounds the corner
// where line l and arc a meet. l.End is the corner and l.Start the end that is
// kept; atStart says whether the corner is a.Start (otherwise a.End). The
// fillet center is r from both curves; among the centers whose contacts lie
// strictly inside the parts of l and a the fillet keeps, the one whose farther
// contact is nearest the corner is chosen, so the fillet removes the least. It
// returns false when r is not positive, a curve is degenerate, the two corner
// coordinates differ, the curves are tangent at the corner, or no fillet fits.
func FilletLineArc(l *Line, a *Arc, atStart bool, r float64) (CornerFillet, bool) {
	return filletCorner(filletLeg{line: l}, filletLeg{arc: a, atStart: atStart}, r)
}

// FilletArcArc is [FilletLineArc] for two arcs: atStart1 and atStart2 say
// whether the corner is the Start of a1 and of a2 (otherwise their End).
func FilletArcArc(a1 *Arc, atStart1 bool, a2 *Arc, atStart2 bool, r float64) (CornerFillet, bool) {
	return filletCorner(filletLeg{arc: a1, atStart: atStart1}, filletLeg{arc: a2, atStart: atStart2}, r)
}
```

### Step 8.2 — new file `geom/fillet_curves_test.go`

This file must not exist yet: `test -e geom/fillet_curves_test.go && echo EXISTS` prints nothing.

```go
package geom_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func TestFilletLineArc(t *testing.T) {
	// Arc about the origin, R=10, from its corner Start (10,0) to (0,10); the line
	// runs from its kept end (20,0) to the same corner.
	a := geom.NewArc(geom.NewPoint(0, 0), geom.NewPoint(10, 0), geom.NewPoint(0, 10))
	l := geom.NewLine(geom.NewPoint(20, 0), geom.NewPoint(10, 0))

	f, ok := geom.FilletLineArc(l, a, true, 2)
	require.True(t, ok)
	require.InDelta(t, math.Sqrt(140), f.Arc.Center.X, 1e-9)
	require.InDelta(t, 2, f.Arc.Center.Y, 1e-9)
	require.InDelta(t, math.Sqrt(140), f.T1.X, 1e-9)
	require.InDelta(t, 0, f.T1.Y, 1e-9)
	require.InDelta(t, 5*math.Sqrt(140)/6, f.T2.X, 1e-9)
	require.InDelta(t, 5.0/3, f.T2.Y, 1e-9)
	require.False(t, f.Inside1)
	require.False(t, f.Inside2, "the fillet sits outside the arc's circle")
	require.InDelta(t, 2, f.Arc.Radius(), 1e-9)
	require.True(t, (f.Arc.Start == f.T1 && f.Arc.End == f.T2) || (f.Arc.Start == f.T2 && f.Arc.End == f.T1))
	require.LessOrEqual(t, f.Arc.Sweep(), math.Pi)
	require.InDelta(t, 20, l.Start.X, 0, "the inputs are not changed")
	require.InDelta(t, 10, a.Start.X, 0)
}

func TestFilletArcArc(t *testing.T) {
	a1 := geom.NewArc(geom.NewPoint(0, 0), geom.NewPoint(10, 0), geom.NewPoint(0, 10))
	h := 10 - 5*math.Sqrt2
	a2 := geom.NewArc(geom.NewPoint(10, 10), geom.NewPoint(h, h), geom.NewPoint(10, 0))

	f, ok := geom.FilletArcArc(a1, true, a2, false, 2)
	require.True(t, ok)
	ox, oy := 5+math.Sqrt(7), 5-math.Sqrt(7)
	require.InDelta(t, ox, f.Arc.Center.X, 1e-9)
	require.InDelta(t, oy, f.Arc.Center.Y, 1e-9)
	require.InDelta(t, ox*10/8, f.T1.X, 1e-9)
	require.InDelta(t, oy*10/8, f.T1.Y, 1e-9)
	require.InDelta(t, 10+(ox-10)*10/8, f.T2.X, 1e-9)
	require.InDelta(t, 10+(oy-10)*10/8, f.T2.Y, 1e-9)
	require.True(t, f.Inside1)
	require.True(t, f.Inside2)
}

func TestFilletCurvesRefuse(t *testing.T) {
	a := geom.NewArc(geom.NewPoint(0, 0), geom.NewPoint(10, 0), geom.NewPoint(0, 10))
	l := geom.NewLine(geom.NewPoint(20, 0), geom.NewPoint(10, 0))

	_, ok := geom.FilletLineArc(l, a, true, 0)
	require.False(t, ok, "radius must be positive")
	_, ok = geom.FilletLineArc(l, a, true, 50)
	require.False(t, ok, "radius too large for the legs")
	tangent := geom.NewLine(geom.NewPoint(10, -10), geom.NewPoint(10, 0)) // continues the arc's tangent
	_, ok = geom.FilletLineArc(tangent, a, true, 2)
	require.False(t, ok, "tangent at the corner: nothing to round")
	elsewhere := geom.NewLine(geom.NewPoint(20, 5), geom.NewPoint(11, 5))
	_, ok = geom.FilletLineArc(elsewhere, a, true, 2)
	require.False(t, ok, "the curves do not meet at the corner")
}
```

### Step 8.3 — verify

Focused tests (all must pass):

```sh
go test -count=1 -run 'TestFillet' ./geom
```

Then the common "Verify" commands.

### Step 8.4 — commit and PR

- **Commit message:** `add line-arc and arc-arc fillet math`
- **Files to add:** `geom/fillet_curves.go`, `geom/fillet_curves_test.go`
- **PR title:** `add line-arc and arc-arc fillet math`
- **PR body:**

  ```markdown
  - Compute the fillet arc for a corner where a line meets an arc or two arcs meet
  ```

## Increment 9 — `CreateFillet` with arcs

- **Branch:** `feat-fillet-arcs`
- **Design section:** "Fillet on curves", subsection "`CreateFillet` with arcs".
- **Merge after:** 8, and the creation-variants PR `fix rank of shared-endpoint arc tangency`
- **Files:** `tools.go`, `tools_nil_sketch_test.go`, `docs/modification-tools-design.md`,
  `docs/fusion-gap-analysis.md` (edits); `tools_fillet_arc_test.go` (new).

This increment needs the creation-variants fix `fix rank of shared-endpoint arc tangency`: without it the tangency
between an arc leg and the fillet arc reads as redundant, and `TestCreateFilletLineArc` fails with DOF off by one.
Step 9.0 checks that the fix is on `origin/main`.

### Step 9.0 — sync and check the base

From the worktree root:

```sh
git fetch origin
git rebase origin/main
git log --oneline origin/main | grep -c -F -- 'add line-arc and arc-arc fillet math'   # expect 1 or more
git log --oneline origin/main | grep -c -F -- 'fix rank of shared-endpoint arc tangency'   # expect 1 or more
```

If the rebase stops, or a count is 0, stop and report. Then do the steps below in order. Every step starts
with its own `grep` checks: run them before editing that file.

### Step 9.1 — `tools.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '// the copy funnel cannot reproduce, and by CreateFillet for an operand kind it' tools.go   # expect 1
grep -c -F -- '// removed with it; constraints on the far endpoints are kept. Two lines are' tools.go   # expect 1
grep -c -F -- 'return nil, fmt.Errorf("%w: CreateFillet does not fillet an arc yet", Er' tools.go   # expect 1
grep -c -F -- 'return &Fillet[A, B]{Arc: nArc, L1: e1, L2: e2, T1: t1, T2: t2, Radius:' tools.go   # expect 1
```

Edit 1. Replace exactly:

```go
	// the copy funnel cannot reproduce, and by CreateFillet for an operand kind it
	// does not fillet. Only lines, circles, arcs and fit splines are copied: every
```

with:

```go
	// the copy funnel cannot reproduce. Only lines, circles, arcs and fit splines
	// are copied: every
```

Edit 2. Replace exactly:

```go
// removed with it; constraints on the far endpoints are kept. Two lines are
// handled; an arc operand returns [ErrUnsupportedEntity]. Returns
```

with:

```go
// removed with it; constraints on the far endpoints are kept. Lines and arcs are
// handled in any combination: a line leg gets [NewTangent], an arc leg
// [NewTangentCircles] (internal when the fillet lies inside the leg's circle),
// both at the shared contact point. Returns
```

Edit 3. Replace exactly:

```go
	l1, ok1 := ent1.(*Line)
	l2, ok2 := ent2.(*Line)
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("%w: CreateFillet does not fillet an arc yet", ErrUnsupportedEntity)
	}
	if sharedPoint(l1, l2) != corner {
		return nil, ErrNoSharedCorner
	}
	far1, far2 := otherSketchEnd(l1, corner), otherSketchEnd(l2, corner)

	cc := geom.NewPoint(corner.x(), corner.y())
	copy1 := geom.NewLine(far1.Geometry(), cc)
	copy2 := geom.NewLine(far2.Geometry(), cc)
	arc, ok := geom.Fillet(copy1, copy2, r)
	if !ok {
		return nil, ErrFilletInfeasible
	}
	c1, c2 := otherEnd(copy1, copy1.Start), otherEnd(copy2, copy2.Start)

	t1, t2 := s.CreatePoint(c1.X, c1.Y), s.CreatePoint(c2.X, c2.Y)
	ctr := s.CreatePoint(arc.Center.X, arc.Center.Y)
	replaceEnd(l1, corner, t1)
	replaceEnd(l2, corner, t2)
	nArc := s.CreateArc(ctr, t1, t2)
	if arc.Start != c1 { // geom.Fillet took the minor arc the other way
		nArc = s.CreateArc(ctr, t2, t1)
	}

	s.AddConstraint(NewTangent(l1, nArc), NewTangent(l2, nArc))
```

with:

```go
	if ent1 == ent2 || !slices.Contains(cutterEnds(ent1), corner) || !slices.Contains(cutterEnds(ent2), corner) {
		return nil, ErrNoSharedCorner
	}
	fg, ok := filletContacts(ent1, ent2, corner, r)
	if !ok {
		return nil, ErrFilletInfeasible
	}

	t1, t2 := s.CreatePoint(fg.p1.X, fg.p1.Y), s.CreatePoint(fg.p2.X, fg.p2.Y)
	ctr := s.CreatePoint(fg.center.X, fg.center.Y)
	replaceEnd(ent1, corner, t1)
	replaceEnd(ent2, corner, t2)
	nArc := s.CreateArc(ctr, t1, t2)
	if !fg.startsAtP1 { // the minor arc runs from the second contact
		nArc = s.CreateArc(ctr, t2, t1)
	}

	s.AddConstraint(tangentToFillet(ent1, nArc, fg.inside1), tangentToFillet(ent2, nArc, fg.inside2))
```

Edit 4. Replace exactly:

```go
	return &Fillet[A, B]{Arc: nArc, L1: e1, L2: e2, T1: t1, T2: t2, Radius: rad}, nil
}
```

with:

```go
	return &Fillet[A, B]{Arc: nArc, L1: e1, L2: e2, T1: t1, T2: t2, Radius: rad}, nil
}

// filletGeometry is a fillet computed on solved-coordinate copies: the contacts
// on the first and second leg, the center, which contact the minor arc starts
// at, and whether the fillet lies inside each leg's circle.
type filletGeometry struct {
	p1, p2, center   *geom.Point
	startsAtP1       bool
	inside1, inside2 bool
}

// filletContacts runs the geom fillet math for two legs (each a *Line or an
// *Arc) meeting at corner: geom.Fillet for two lines, FilletLineArc or
// FilletArcArc otherwise.
func filletContacts(e1, e2 Entity, corner *Point, r float64) (filletGeometry, bool) {
	cc := geom.NewPoint(corner.x(), corner.y())
	// legLine copies a line leg as the geom helpers expect it: kept end first,
	// the corner (one shared geom point) last.
	legLine := func(l *Line) *geom.Line { return geom.NewLine(otherSketchEnd(l, corner).Geometry(), cc) }
	l1, isLine1 := e1.(*Line)
	l2, isLine2 := e2.(*Line)
	if isLine1 && isLine2 {
		copy1, copy2 := legLine(l1), legLine(l2)
		arc, ok := geom.Fillet(copy1, copy2, r)
		if !ok {
			return filletGeometry{}, false
		}
		c1, c2 := otherEnd(copy1, copy1.Start), otherEnd(copy2, copy2.Start)
		return filletGeometry{p1: c1, p2: c2, center: arc.Center, startsAtP1: arc.Start == c1}, true
	}
	a1, _ := e1.(*Arc)
	a2, _ := e2.(*Arc)
	var cf geom.CornerFillet
	var ok bool
	switch {
	case isLine1:
		cf, ok = geom.FilletLineArc(legLine(l1), a2.Geometry(), a2.Start == corner, r)
	case isLine2:
		cf, ok = geom.FilletLineArc(legLine(l2), a1.Geometry(), a1.Start == corner, r)
		cf.T1, cf.T2, cf.Inside1, cf.Inside2 = cf.T2, cf.T1, cf.Inside2, cf.Inside1
	default:
		cf, ok = geom.FilletArcArc(a1.Geometry(), a1.Start == corner, a2.Geometry(), a2.Start == corner, r)
	}
	if !ok {
		return filletGeometry{}, false
	}
	return filletGeometry{
		p1: cf.T1, p2: cf.T2, center: cf.Arc.Center,
		startsAtP1: cf.Arc.Start == cf.T1, inside1: cf.Inside1, inside2: cf.Inside2,
	}, true
}

// tangentToFillet is the endpoint tangency between a fillet arc and one of its
// legs, which share the contact point.
func tangentToFillet(leg Entity, fillet *Arc, inside bool) Constraint {
	if l, ok := leg.(*Line); ok {
		return NewTangent(l, fillet)
	}
	a, _ := leg.(*Arc)
	return NewTangentCircles(a, fillet, inside)
}
```

### Step 9.2 — `tools_nil_sketch_test.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'require.ErrorIs(t, err, sketch.ErrUnsupportedEntity, "arc fillets are no' tools_nil_sketch_test.go   # expect 1
```

Edit 1. Replace exactly:

```go
	require.ErrorIs(t, err, sketch.ErrUnsupportedEntity, "arc fillets are not implemented yet")
```

with:

```go
	require.ErrorIs(t, err, sketch.ErrNoSharedCorner, "the line and the arc share no end point")
```

### Step 9.3 — new file `tools_fillet_arc_test.go`

This file must not exist yet: `test -e tools_fillet_arc_test.go && echo EXISTS` prints nothing.

```go
package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// Filleting the corner where a line meets an arc keeps both legs in place and
// rounds the corner with an arc tangent to both, at the closed-form position.
func TestCreateFilletLineArc(t *testing.T) {
	s := newSketch(t)
	center, corner, far := s.CreatePoint(0, 0), s.CreatePoint(10, 0), s.CreatePoint(0, 10)
	arc := s.CreateArc(center, corner, far)
	lineFar := s.CreatePoint(20, 0)
	line := s.CreateLine(corner, lineFar)
	s.Fix(center)
	s.Fix(far)
	s.Fix(lineFar)
	s.AddConstraint(sketch.NewHorizontal(line))
	dof := s.DOF()

	f, err := sketch.CreateFillet(s, line, arc, corner, 2)
	require.NoError(t, err)
	require.Same(t, line, f.L1)
	require.Same(t, arc, f.L2, "the arc leg keeps its handle and type")
	require.Same(t, f.T1, line.Start)
	require.Same(t, f.T2, arc.Start)
	require.InDelta(t, math.Sqrt(140), f.Arc.Center.X(), 1e-9)
	require.InDelta(t, 2, f.Arc.Center.Y(), 1e-9)
	require.InDelta(t, math.Sqrt(140), f.T1.X(), 1e-9)
	require.InDelta(t, 5*math.Sqrt(140)/6, f.T2.X(), 1e-9)
	require.InDelta(t, 5.0/3, f.T2.Y(), 1e-9)
	require.Equal(t, 1, countKind(s, "tangent_line_circle"))
	require.Equal(t, 1, countKind(s, "tangent_circles"))
	require.Equal(t, 1, countKind(s, "horizontal"), "the constraint on the line leg survives")
	require.Equal(t, dof, s.DOF(), "the fillet adds as many rows as variables")

	f.Radius.Set(3)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 3, f.Arc.R(), 1e-6)
	require.InDelta(t, math.Sqrt(160), f.Arc.Center.X(), 1e-6, "13 from the arc center, 3 above the line")
	require.InDelta(t, 3, f.Arc.Center.Y(), 1e-6)
	for _, c := range s.Constraints() {
		for _, r := range sketch.ConstraintResiduals(c) {
			require.InDelta(t, 0, r, 1e-9)
		}
	}
}

// Filleting two arcs places the fillet inside both circles.
func TestCreateFilletArcArc(t *testing.T) {
	s := newSketch(t)
	corner := s.CreatePoint(10, 0)
	a1 := s.CreateArc(s.CreatePoint(0, 0), corner, s.CreatePoint(0, 10))
	h := 10 - 5*math.Sqrt2
	a2 := s.CreateArc(s.CreatePoint(10, 10), s.CreatePoint(h, h), corner)

	f, err := sketch.CreateFillet(s, a1, a2, corner, 2)
	require.NoError(t, err)
	require.Same(t, a1, f.L1)
	require.Same(t, a2, f.L2)
	require.InDelta(t, 5+math.Sqrt(7), f.Arc.Center.X(), 1e-9)
	require.InDelta(t, 5-math.Sqrt(7), f.Arc.Center.Y(), 1e-9)
	require.Equal(t, 2, countKind(s, "tangent_circles"))
	require.NotContains(t, s.Points(), corner)
}

// Curves that meet tangentially have no corner to round.
func TestCreateFilletArcRefusals(t *testing.T) {
	s := newSketch(t)
	corner := s.CreatePoint(10, 0)
	arc := s.CreateArc(s.CreatePoint(0, 0), corner, s.CreatePoint(0, 10))
	tangent := s.CreateLine(s.CreatePoint(10, -10), corner) // continues the arc's tangent
	pts, cons := len(s.Points()), len(s.Constraints())

	_, err := sketch.CreateFillet(s, tangent, arc, corner, 2)
	require.ErrorIs(t, err, sketch.ErrFilletInfeasible)
	require.Len(t, s.Points(), pts)
	require.Len(t, s.Constraints(), cons)

	_, err = sketch.CreateFillet(s, tangent, arc, tangent.Start, 2)
	require.ErrorIs(t, err, sketch.ErrNoSharedCorner)
}
```

### Step 9.4 — `docs/modification-tools-design.md`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'from the far ends. Both remove the corner point.' docs/modification-tools-design.md   # expect 1
```

Edit 1. Replace exactly:

```markdown
  from the far ends. Both remove the corner point.
```

with:

```markdown
  from the far ends. Both remove the corner point. `CreateFillet` also rounds a
  corner where a line meets an arc or two arcs meet (`geom.FilletLineArc`,
  `geom.FilletArcArc`): an arc leg keeps its center and far end, and gets
  `NewTangentCircles` to the fillet (internal when the fillet lies inside the
  leg's circle).
```

### Step 9.5 — `docs/fusion-gap-analysis.md`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '`CreateMirror`, `CreatePatternRect`, `CreatePatternCircular` and `CreateOffset`.' docs/fusion-gap-analysis.md   # expect 1
grep -c -F -- '- **Trim, extend, fillet and chamfer on curves.** `Trim` and `Extend` take a' docs/fusion-gap-analysis.md   # expect 1
```

Edit 1. Replace exactly:

```markdown
**Closed:** `Trim`, `Extend`, `Break`, `CreateFillet`, `CreateChamfer`,
`CreateMirror`, `CreatePatternRect`, `CreatePatternCircular` and `CreateOffset`.
Design in `docs/modification-tools-design.md`.
```

with:

```markdown
**Closed:** `Trim`, `Extend`, `Break`, `CreateFillet`, `CreateChamfer`,
`CreateMirror`, `CreatePatternRect`, `CreatePatternCircular` and `CreateOffset`,
as package-level functions. `Trim`, `Extend` and `CreateFillet` take lines and
arcs, `TrimCircle` trims a circle, and trim, extend, fillet and chamfer edit the
curve in place as Fusion does. Design in `docs/modification-tools-design.md`
and `docs/curve-modify-tools-design.md`.
```

Edit 2. Replace exactly:

```markdown
- **Trim, extend, fillet and chamfer on curves.** `Trim` and `Extend` take a
  `*Line`, and `CreateFillet` and `CreateChamfer` take two `*Line` operands.
  `Break` accepts any entity.
- **Project, include and intersect.** These stay out by design. The engine verifies
```

with:

```markdown
- **Project, include and intersect.** These stay out by design. The engine verifies
```

### Step 9.6 — verify

Focused tests (all must pass):

```sh
go test -count=1 -run 'Fillet|Tools|TangentCircles' .
```

Then the common "Verify" commands.

### Step 9.7 — commit and PR

- **Commit message:** `fillet corners between lines and arcs`
- **Files to add:** `tools.go`, `tools_nil_sketch_test.go`, `tools_fillet_arc_test.go`,
  `docs/modification-tools-design.md`, `docs/fusion-gap-analysis.md`
- **PR title:** `fillet corners between lines and arcs`
- **PR body:**

  ```markdown
  - Round corners where a line meets an arc or two arcs meet, keeping both legs in place
  ```

## Increment 10 — `RadialOffset` constraint

- **Branch:** `feat-radial-offset`
- **Design section:** "The `RadialOffset` constraint".
- **Merge after:** 9
- **Files:** `constraint.go`, `json.go`, `removal.go`, `conditioning.go`, `annotate.go`, `introspect_test.go`,
  `introspect_nil_contract_test.go`, `jacobian_inventory_test.go` (edits); `radial_offset_test.go` (new).

### Step 10.0 — sync and check the base

From the worktree root:

```sh
git fetch origin
git rebase origin/main
git log --oneline origin/main | grep -c -F -- 'fillet corners between lines and arcs'   # expect 1 or more
```

If the rebase stops, or a count is 0, stop and report. Then do the steps below in order. Every step starts
with its own `grep` checks: run them before editing that file.

### Step 10.1 — `constraint.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '// Radius is an editable radius dimension.' constraint.go   # expect 1
```

Edit 1. Replace exactly:

```go

// Radius is an editable radius dimension.
```

with:

```go

// RadialOffset is an editable radius-difference dimension: it drives Dst's
// radius to Src's radius plus d, so positive d makes Dst larger. It does not hold
// the two curves concentric: [CreateOffset] gives the copy the source's center
// point, and a direct caller shares the center the same way or adds
// [NewConcentric].
type RadialOffset struct {
	dimBase
	Src, Dst Circular
}

func (c *RadialOffset) residual(out []float64) []float64 {
	return append(out, c.Dst.R()-c.Src.R()-c.base()) // length units
}

// NewRadialOffset constrains dst's radius to src's radius plus d (positive d is
// outward). Both operands are circles or arcs; an arc's radius is read from its
// Start, as [Arc.R] does.
func NewRadialOffset(src, dst Circular, d float64) *RadialOffset {
	return &RadialOffset{dimBase: lengthDim(d), Src: src, Dst: dst}
}

// Radius is an editable radius dimension.
```

### Step 10.2 — `json.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'return "offset"' json.go   # expect 1
grep -c -F -- 'return dimJSON(typ, t, nil, []int{t.Src.id, t.Dst.id}), true' json.go   # expect 1
grep -c -F -- '"distance_point_line": {1, 1}, "distance_lines": {0, 2}, "offset": {0, 2},' json.go   # expect 1
grep -c -F -- 'case "hdistance":' json.go   # expect 1
```

Edit 1. Replace exactly:

```go
		return "offset"
	case *Radius:
```

with:

```go
		return "offset"
	case *RadialOffset:
		return "radial_offset"
	case *Radius:
```

Edit 2. Replace exactly:

```go
		return dimJSON(typ, t, nil, []int{t.Src.id, t.Dst.id}), true
	case *Radius:
```

with:

```go
		return dimJSON(typ, t, nil, []int{t.Src.id, t.Dst.id}), true
	case *RadialOffset:
		return dimJSON(typ, t, nil, []int{t.Src.entID(), t.Dst.entID()}), true
	case *Radius:
```

Edit 3. Replace exactly:

```go
	"distance_point_line": {1, 1}, "distance_lines": {0, 2}, "offset": {0, 2},
```

with:

```go
	"distance_point_line": {1, 1}, "distance_lines": {0, 2}, "offset": {0, 2}, "radial_offset": {0, 2},
```

Edit 4. Replace exactly:

```go
		}
	case "hdistance":
```

with:

```go
		}
	case "radial_offset":
		src, err := circular(jc.Entities[0])
		if err != nil {
			return err
		}
		dst, err := circular(jc.Entities[1])
		if err != nil {
			return err
		}
		if err := dim(NewRadialOffset(src, dst, jc.Value)); err != nil {
			return err
		}
	case "hdistance":
```

### Step 10.3 — `removal.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'return nil, []Entity{t.Src, t.Dst}' removal.go   # expect 1
```

Edit 1. Replace exactly:

```go
		return nil, []Entity{t.Src, t.Dst}
	case *Radius:
```

with:

```go
		return nil, []Entity{t.Src, t.Dst}
	case *RadialOffset:
		return nil, []Entity{t.Src, t.Dst}
	case *Radius:
```

### Step 10.4 — `conditioning.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'case *Radius:' conditioning.go   # expect 1
```

Edit 1. Replace exactly:

```go
		return append(out, rowLength, rowLength)
	case *Radius:
```

with:

```go
		return append(out, rowLength, rowLength)
	case *RadialOffset:
		return append(out, rowLength)
	case *Radius:
```

### Step 10.5 — `annotate.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'a.lineToLine(t.Src, t.Dst, a.dimText("", t), t.Driven())' annotate.go   # expect 1
```

Edit 1. Replace exactly:

```go
		a.lineToLine(t.Src, t.Dst, a.dimText("", t), t.Driven())
	case *Radius:
```

with:

```go
		a.lineToLine(t.Src, t.Dst, a.dimText("", t), t.Driven())
	case *RadialOffset:
		a.leaderToCircular(a.scr(t.Src.centerPt()), t.Dst, a.dimText("", t), t.Driven())
	case *Radius:
```

### Step 10.6 — `introspect_test.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '{"Offset", (*sketch.Offset)(nil), "offset"},' introspect_test.go   # expect 1
```

Edit 1. Replace exactly:

```go
			{"Offset", (*sketch.Offset)(nil), "offset"},
			{"Radius", (*sketch.Radius)(nil), "radius"},
```

with:

```go
			{"Offset", (*sketch.Offset)(nil), "offset"},
			{"RadialOffset", (*sketch.RadialOffset)(nil), "radial_offset"},
			{"Radius", (*sketch.Radius)(nil), "radius"},
```

### Step 10.7 — `introspect_nil_contract_test.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '{"NewOffset", "", func() sketch.Constraint { return sketch.NewOffset(nil' introspect_nil_contract_test.go   # expect 1
```

Edit 1. Replace exactly:

```go
	{"NewOffset", "", func() sketch.Constraint { return sketch.NewOffset(nil, nil, 0) }, "ok", "offset", "pts=[] ents=[typed-nil(*sketch.Line) typed-nil(*sketch.Line)]", "panic"},
	{"NewArcLength", "", func() sketch.Constraint { return sketch.NewArcLength(nil, 1) }, "ok", "arc_length", "pts=[] ents=[typed-nil(*sketch.Arc)]", "panic"},
```

with:

```go
	{"NewOffset", "", func() sketch.Constraint { return sketch.NewOffset(nil, nil, 0) }, "ok", "offset", "pts=[] ents=[typed-nil(*sketch.Line) typed-nil(*sketch.Line)]", "panic"},
	{"NewRadialOffset", "typed", func() sketch.Constraint {
		return sketch.NewRadialOffset((*sketch.Circle)(nil), (*sketch.Circle)(nil), 0)
	}, "ok", "radial_offset", "pts=[] ents=[typed-nil(*sketch.Circle) typed-nil(*sketch.Circle)]", "panic"},
	{"NewRadialOffset", "untyped", func() sketch.Constraint { return sketch.NewRadialOffset(nil, nil, 0) }, "ok", "radial_offset", "pts=[] ents=[untyped-nil untyped-nil]", "panic"},
	{"NewArcLength", "", func() sketch.Constraint { return sketch.NewArcLength(nil, 1) }, "ok", "arc_length", "pts=[] ents=[typed-nil(*sketch.Arc)]", "panic"},
```

### Step 10.8 — `jacobian_inventory_test.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '{"radius", func(s *Sketch) Constraint {' jacobian_inventory_test.go   # expect 1
grep -c -F -- '{"offset", 2, 8, 0, 0, "static", true},' jacobian_inventory_test.go   # expect 1
grep -c -F -- '"NewOffset":                 "offset",' jacobian_inventory_test.go   # expect 1
```

Edit 1. Replace exactly:

```go
		}},
		{"radius", func(s *Sketch) Constraint {
```

with:

```go
		}},
		{"radial_offset", func(s *Sketch) Constraint {
			c1 := s.CreateCircle(s.CreatePoint(0, 0), 2)
			c2 := s.CreateCircle(s.CreatePoint(1, 0), 3)
			c := NewRadialOffset(c1, c2, 1)
			s.AddConstraint(c)
			return c
		}},
		{"radius", func(s *Sketch) Constraint {
```

Edit 2. Replace exactly:

```go
	{"offset", 2, 8, 0, 0, "static", true},
	{"radius", 1, 2, 1, 0, "static", true},
```

with:

```go
	{"offset", 2, 8, 0, 0, "static", true},
	{"radial_offset", 1, 4, 2, 0, "static", true},
	{"radius", 1, 2, 1, 0, "static", true},
```

Edit 3. Replace exactly:

```go
	"NewOffset":                 "offset",
	"NewRadius":                 "radius",
```

with:

```go
	"NewOffset":                 "offset",
	"NewRadialOffset":           "radial_offset",
	"NewRadius":                 "radius",
```

### Step 10.9 — new file `radial_offset_test.go`

This file must not exist yet: `test -e radial_offset_test.go && echo EXISTS` prints nothing.

```go
package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// Two concentric circles sharing a center: the radial offset drives the copy's
// radius to the source's radius plus d.
func TestRadialOffsetCircles(t *testing.T) {
	s := newSketch(t)
	center := s.CreatePoint(0, 0)
	src := s.CreateCircle(center, 10)
	dst := s.CreateCircle(center, 11)
	off := sketch.NewRadialOffset(src, dst, 2)
	s.AddConstraint(sketch.NewRadius(src, 10), off)

	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 12, dst.R(), 1e-6)

	off.Set(-3)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 7, dst.R(), 1e-6)
}

// An arc operand reads its radius from its Start.
func TestRadialOffsetArcs(t *testing.T) {
	s := newSketch(t)
	center := s.CreatePoint(0, 0)
	s.Fix(center)
	srcStart, srcEnd := s.CreatePoint(10, 0), s.CreatePoint(0, 10)
	s.Fix(srcStart)
	s.Fix(srcEnd)
	src := s.CreateArc(center, srcStart, srcEnd)
	dst := s.CreateArc(center, s.CreatePoint(11, 0), s.CreatePoint(0, 11))
	s.AddConstraint(sketch.NewRadialOffset(src, dst, 2))

	_, err := s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 12, dst.R(), 1e-6)
}

// The dimension survives a JSON round trip with its kind and signed target, and
// its row is classified for the conditioning gate.
func TestRadialOffsetRoundTrip(t *testing.T) {
	s := newSketch(t)
	center := s.CreatePoint(0, 0)
	src := s.CreateCircle(center, 10)
	dst := s.CreateCircle(center, 7)
	s.AddConstraint(sketch.NewRadialOffset(src, dst, -3))

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var s2 sketch.Sketch
	require.NoError(t, json.Unmarshal(data, &s2))
	require.Equal(t, 1, countKind(&s2, "radial_offset"))
	for _, c := range s2.Constraints() {
		if ro, ok := c.(*sketch.RadialOffset); ok {
			require.InDelta(t, -3, ro.Target().Base(), 1e-12)
		}
	}

	rep := s.Verify(t.Context())
	require.True(t, rep.Analysed())
	require.False(t, math.IsNaN(rep.Conditioning))
}
```

### Step 10.10 — verify

Focused tests (all must pass):

```sh
go test -count=1 -run 'RadialOffset|Introspection|Dependency|Jacobian|JSON|Conditioning|Annotat' .
```

Then the common "Verify" commands.

### Step 10.11 — commit and PR

- **Commit message:** `add radial offset dimension`
- **Files to add:** `constraint.go`, `json.go`, `removal.go`, `conditioning.go`, `annotate.go`, `introspect_test.go`,
  `introspect_nil_contract_test.go`, `jacobian_inventory_test.go`, `radial_offset_test.go`
- **PR title:** `add radial offset dimension`
- **PR body:**

  ```markdown
  - Add NewRadialOffset, which holds one circle or arc's radius at another's plus a distance
  ```

## Increment 11 — `CreateOffset` for circles and lone arcs

- **Branch:** `feat-offset-circles-arcs`
- **Design section:** "Offset of arcs and circles", subsection "Circles and lone arcs".
- **Merge after:** 10
- **Files:** `tools.go`, `entity_switches_test.go`, `docs/modification-tools-design.md` (edits);
  `tools_offset_curve_test.go` (new).

### Step 11.0 — sync and check the base

From the worktree root:

```sh
git fetch origin
git rebase origin/main
git log --oneline origin/main | grep -c -F -- 'add radial offset dimension'   # expect 1 or more
```

If the rebase stops, or a count is 0, stop and report. Then do the steps below in order. Every step starts
with its own `grep` checks: run them before editing that file.

### Step 11.1 — `tools.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '// constraints, and the constraints that hold each free chain end on the source' tools.go   # expect 1
grep -c -F -- 'signs       []float64 // per Offset: its target is signs[i]·d' tools.go   # expect 1
grep -c -F -- 'o.Set(g.signs[i] * d)' tools.go   # expect 1
grep -c -F -- '// Non-line entities in ents are skipped. It returns [ErrForeignEntity] if s is' tools.go   # expect 1
grep -c -F -- '// Validate every source line up front so a failure leaves the sketch' tools.go   # expect 1
grep -c -F -- 'return g, nil' tools.go   # expect 1
```

Edit 1. Replace exactly:

```go
// lines (left in place), their offset copies, the per-segment offset
// constraints, and the constraints that hold each free chain end on the source
// normal. Use [OffsetGroup.Set] to retarget the whole offset distance at once
// and re-solve.
```

with:

```go
// entities (left in place), their offset copies (the line copies in order, then
// the arc and circle copies in order), the per-line offset constraints, the
// per-arc/circle radial offsets, and the constraints that hold each free end on
// the source normal. Use [OffsetGroup.Set] to retarget the whole offset distance
// at once and re-solve.
```

Edit 2. Replace exactly:

```go
	Constraints []Constraint
	signs       []float64 // per Offset: its target is signs[i]·d
```

with:

```go
	Radials     []*RadialOffset
	Constraints []Constraint
	signs       []float64 // per Offset: its target is signs[i]·d
	radialSigns []float64 // per RadialOffset: its target is radialSigns[i]·d
```

Edit 3. Replace exactly:

```go
		o.Set(g.signs[i] * d)
	}
```

with:

```go
		o.Set(g.signs[i] * d)
	}
	for i, r := range g.Radials {
		r.Set(g.radialSigns[i] * d)
	}
```

Edit 4. Replace exactly:

```go
// Non-line entities in ents are skipped. It returns [ErrForeignEntity] if s is
// nil or a source entity, or a point defining one, is not live in s,
// [ErrReferenceGeometry] for reference geometry, and [ErrInvalidShape] if a
// source line has zero length (no defined offset direction) or an end point is
// shared by three or more lines (not a chain).
```

with:

```go
// A circle, and an arc that shares no end point with another member, flows
// counter-clockwise: its copy shares the source's center, a [RadialOffset] holds
// its radius at the source radius plus d (positive d is outward, as Fusion
// offsets a circle), and an arc copy's ends are held on the source normals. It
// returns [ErrForeignEntity] if s is nil or a source entity, or a point
// defining one, is not live in s, [ErrReferenceGeometry] for reference
// geometry, [ErrUnsupportedEntity] for an entity other than a line, an arc or a
// circle, and [ErrInvalidShape] if a source line has zero length (no defined
// offset direction), an end point is shared by three or more lines (not a
// chain), a copy's radius would not be positive, or an arc shares an end point
// with another member (joined arcs are not offset yet).
```

Edit 5. Replace exactly:

```go
	if containsReference(ents) {
		return nil, ErrReferenceGeometry
	}
	// Validate every source line up front so a failure leaves the sketch
```

with:

```go
	if containsReference(ents) {
		return nil, ErrReferenceGeometry
	}
	if e, ok := unsupportedOffset(ents); ok {
		return nil, fmt.Errorf("%w: CreateOffset cannot offset %T; only *sketch.Line, *sketch.Arc and *sketch.Circle are offset", ErrUnsupportedEntity, e)
	}
	if err := checkOffsetCurves(ents, d); err != nil {
		return nil, err
	}
	// Validate every source line up front so a failure leaves the sketch
```

Edit 6. Replace exactly:

```go
	return g, nil
```

with:

```go
	s.offsetCurves(g, ents, d)
	return g, nil
}

// unsupportedOffset reports the first entity in ents CreateOffset cannot
// offset. Like unsupportedSeed it is an allow-list with a refusing default, so
// an entity type added later is refused rather than silently skipped.
func unsupportedOffset(ents []Entity) (Entity, bool) {
	for _, e := range ents {
		switch e.(type) {
		case *Line, *Arc, *Circle:
		default:
			return e, true
		}
	}
	return nil, false
}

// checkOffsetCurves refuses, before anything is committed, a circle or arc whose
// copy would have a radius of zero or less, and an arc that shares an end point
// with another member.
func checkOffsetCurves(ents []Entity, d float64) error {
	ends := map[*Point]int{}
	for _, e := range ents {
		for _, p := range cutterEnds(e) {
			ends[p]++
		}
	}
	for _, e := range ents {
		if c, ok := e.(*Circle); ok && c.r()+d <= 0 {
			return fmt.Errorf("%w: CreateOffset: the offset circle's radius would be %g", ErrInvalidShape, c.r()+d)
		}
		a, ok := e.(*Arc)
		if !ok {
			continue
		}
		if a.R()+d <= 0 {
			return fmt.Errorf("%w: CreateOffset: the offset arc's radius would be %g", ErrInvalidShape, a.R()+d)
		}
		if ends[a.Start] > 1 || ends[a.End] > 1 {
			return fmt.Errorf("%w: CreateOffset does not offset an arc joined to another member yet", ErrInvalidShape)
		}
	}
	return nil
}

// offsetCurves adds the copies of the circles and lone arcs in ents: each shares
// its source's center, a RadialOffset holds its radius at the source radius plus
// d, and an arc copy's ends are held on the source normals at the source ends.
func (s *Sketch) offsetCurves(g *OffsetGroup, ents []Entity, d float64) {
	for _, e := range ents {
		if c, ok := e.(*Circle); ok {
			dst := s.CreateCircle(c.Center, c.r()+d)
			ro := NewRadialOffset(c, dst, d)
			s.AddConstraint(ro)
			g.Copies = append(g.Copies, dst)
			g.Radials = append(g.Radials, ro)
			g.radialSigns = append(g.radialSigns, 1)
			continue
		}
		a, ok := e.(*Arc)
		if !ok {
			continue
		}
		k := (a.R() + d) / a.R()
		at := func(p *Point) *Point {
			return s.CreatePoint(a.Center.x()+k*(p.x()-a.Center.x()), a.Center.y()+k*(p.y()-a.Center.y()))
		}
		dst := s.CreateArc(a.Center, at(a.Start), at(a.End))
		ro := NewRadialOffset(a, dst, d)
		pins := []Constraint{NewPointOnNormal(dst.Start, a, a.Start), NewPointOnNormal(dst.End, a, a.End)}
		s.AddConstraint(ro)
		s.AddConstraint(pins...)
		g.Copies = append(g.Copies, dst)
		g.Radials = append(g.Radials, ro)
		g.radialSigns = append(g.radialSigns, 1)
		g.Constraints = append(g.Constraints, pins...)
	}
```

### Step 11.2 — `entity_switches_test.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '{"tools.go", "circleCrossings"}:          "only line/circle/arc cutters' entity_switches_test.go   # expect 1
```

Edit 1. Replace exactly:

```go
	{"tools.go", "circleCrossings"}:          "only line/circle/arc cutters have a closed-form circle intersection in geom; a curve contributes no crossing",
	{"tools.go", "lineCrossings"}:            "only line/circle/arc cutters have a closed-form line intersection in geom; a curve contributes no crossing",
```

with:

```go
	{"tools.go", "circleCrossings"}:          "only line/circle/arc cutters have a closed-form circle intersection in geom; a curve contributes no crossing",
	{"tools.go", "unsupportedOffset"}:        "an allow-list with a refusing default: only line, arc and circle are offset, and an entity type not named here is REFUSED by CreateOffset rather than skipped",
	{"tools.go", "lineCrossings"}:            "only line/circle/arc cutters have a closed-form line intersection in geom; a curve contributes no crossing",
```

### Step 11.3 — new file `tools_offset_curve_test.go`

This file must not exist yet: `test -e tools_offset_curve_test.go && echo EXISTS` prints nothing.

```go
package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// A circle offsets to a concentric copy sharing its center; positive is outward,
// as in Fusion, and Set retargets the copy.
func TestOffsetCircle(t *testing.T) {
	s := newSketch(t)
	center := s.CreatePoint(0, 0)
	s.Fix(center)
	src := s.CreateCircle(center, 10)
	s.AddConstraint(sketch.NewRadius(src, 10))

	g, err := sketch.CreateOffset(s, []sketch.Entity{src}, 2)
	require.NoError(t, err)
	dst := g.Copies[0].(*sketch.Circle)
	require.Same(t, center, dst.Center)
	require.Len(t, g.Radials, 1)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.Equal(t, 0, s.DOF())
	require.InDelta(t, 12, dst.R(), 1e-6)

	g.Set(-3)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 7, dst.R(), 1e-6)
}

// A lone arc offsets to a concentric arc whose ends stay on the source normals.
func TestOffsetLoneArc(t *testing.T) {
	s := newSketch(t)
	center, start, end := s.CreatePoint(0, 0), s.CreatePoint(10, 0), s.CreatePoint(0, 10)
	for _, p := range []*sketch.Point{center, start, end} {
		s.Fix(p)
	}
	src := s.CreateArc(center, start, end)

	g, err := sketch.CreateOffset(s, []sketch.Entity{src}, 2)
	require.NoError(t, err)
	dst := g.Copies[0].(*sketch.Arc)
	require.Same(t, center, dst.Center)
	require.Len(t, g.Constraints, 2, "both ends are held on the source normals")
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.Equal(t, 0, s.DOF())
	require.InDelta(t, 12, dst.Start.X(), 1e-6)
	require.InDelta(t, 0, dst.Start.Y(), 1e-6)
	require.InDelta(t, 0, dst.End.X(), 1e-6)
	require.InDelta(t, 12, dst.End.Y(), 1e-6)

	g.Set(1)
	_, err = s.Solve(t.Context())
	require.NoError(t, err)
	require.InDelta(t, 11, dst.R(), 1e-6)
	require.InDelta(t, 11, dst.Start.X(), 1e-6)
}

// Refusals leave the sketch unchanged.
func TestOffsetCurveRefusals(t *testing.T) {
	cases := []struct {
		name  string
		build func(s *sketch.Sketch) []sketch.Entity
		want  error
	}{
		{"ellipse", func(s *sketch.Sketch) []sketch.Entity {
			return []sketch.Entity{s.CreateEllipse(s.CreatePoint(0, 0), 5, 3, 0)}
		}, sketch.ErrUnsupportedEntity},
		{"radius would vanish", func(s *sketch.Sketch) []sketch.Entity {
			return []sketch.Entity{s.CreateCircle(s.CreatePoint(0, 0), 2)}
		}, sketch.ErrInvalidShape},
		{"joined arc", func(s *sketch.Sketch) []sketch.Entity {
			p := s.CreatePoint(10, 0)
			arc := s.CreateArc(s.CreatePoint(0, 0), p, s.CreatePoint(0, 10))
			return []sketch.Entity{s.CreateLine(s.CreatePoint(20, 0), p), arc}
		}, sketch.ErrInvalidShape},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSketch(t)
			ents := tc.build(s)
			pts, n, cons := len(s.Points()), len(s.Entities()), len(s.Constraints())

			_, err := sketch.CreateOffset(s, ents, -3)
			require.ErrorIs(t, err, tc.want)
			require.Len(t, s.Points(), pts)
			require.Len(t, s.Entities(), n)
			require.Len(t, s.Constraints(), cons)
		})
	}
}
```

### Step 11.4 — `docs/modification-tools-design.md`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '- **Offset of arcs/circles** as a chain (concentric offset) — `CreateOff' docs/modification-tools-design.md   # expect 1
```

Edit 1. Replace exactly:

```markdown
- **Offset of arcs/circles** as a chain (concentric offset) — `CreateOffset` skips
  non-line entities; a single circle offset is already expressible with
  `NewConcentric` + `NewRadius`.
```

with:

```markdown
- **Offset of arcs joined into a chain** — `CreateOffset` offsets circles and
  lone arcs (a concentric copy held by `RadialOffset`) and refuses an arc that
  shares an end point with another member; any kind other than line, arc and
  circle returns `ErrUnsupportedEntity`.
```

### Step 11.5 — verify

Focused tests (all must pass):

```sh
go test -count=1 -run 'Offset|EntityTypeSwitches|Tools' .
```

Then the common "Verify" commands.

### Step 11.6 — commit and PR

- **Commit message:** `offset circles and lone arcs`
- **Files to add:** `tools.go`, `entity_switches_test.go`, `tools_offset_curve_test.go`,
  `docs/modification-tools-design.md`
- **PR title:** `offset circles and lone arcs`
- **PR body:**

  ```markdown
  - Offset circles and lone arcs concentrically, and refuse entity kinds the tool cannot offset instead of skipping them
  ```

## Increment 12 — `CreateOffset` for chains of lines and arcs

- **Branch:** `feat-offset-chains`
- **Design section:** "Offset of arcs and circles", subsection "Offset chains".
- **Merge after:** 11
- **Files:** `tools.go`, `tools_offset_curve_test.go`, `docs/modification-tools-design.md`,
  `docs/fusion-gap-analysis.md`, `compound.go` (edits); `tools_offset_chain_test.go` (new).

Step 12.6 fixes two doc links in `compound.go` that increment 0 left pointing at the removed methods. It is done last
because the creation-variants increment 2 rewrites the same comment; its anchors are the two link texts alone, which
both versions of the comment contain.

### Step 12.0 — sync and check the base

From the worktree root:

```sh
git fetch origin
git rebase origin/main
git log --oneline origin/main | grep -c -F -- 'offset circles and lone arcs'   # expect 1 or more
```

If the rebase stops, or a count is 0, stop and report. Then do the steps below in order. Every step starts
with its own `grep` checks: run them before editing that file.

### Step 12.1 — `tools.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '// entities (left in place), their offset copies (the line copies in order, then' tools.go   # expect 1
grep -c -F -- '// CreateOffset creates a parallel offset of a chain of lines at signed distance' tools.go   # expect 1
grep -c -F -- 'return nil, fmt.Errorf("%w: CreateOffset source line has zero length", E' tools.go   # expect 1
grep -c -F -- '// A free chain end (a source end point no other source line uses) is held on' tools.go   # expect 1
grep -c -F -- '// copy would have a radius of zero or less, and an arc that shares an end point' tools.go   # expect 1
grep -c -F -- '// means the flow enters the next line at p, backward that it leaves it there.' tools.go   # expect 1
grep -c -F -- 'if (forward && lj.Start == p) || (!forward && lj.End == p) {' tools.go   # expect 1
grep -c -F -- 'if lj.Start == p {' tools.go   # expect 1
grep -c -F -- 'continue // already reached from an earlier line of its chain' tools.go   # expect 1
grep -c -F -- 'exit, entry := l.End, l.Start' tools.go   # expect 1
```

Edit 1. Replace exactly:

```go
// entities (left in place), their offset copies (the line copies in order, then
// the arc and circle copies in order), the per-line offset constraints, the
// per-arc/circle radial offsets, and the constraints that hold each free end on
// the source normal. Use [OffsetGroup.Set] to retarget the whole offset distance
// at once and re-solve.
```

with:

```go
// entities (left in place), their offset copies (the line and arc copies in
// the order of the source, then the circle copies in order), the per-line
// offset constraints, the per-arc/circle radial offsets, and the constraints
// that hold the copy's ends and tangent joints in place (point-on-normal pins
// and line–arc tangencies). Use [OffsetGroup.Set] to retarget the whole offset
// distance at once and re-solve.
```

Edit 2. Replace exactly:

```go
// CreateOffset creates a parallel offset of a chain of lines at signed distance
// d, as Fusion's offset does: positive d is to the right of the flow, negative
// to the left. The flow of a chain runs from its first line in ents toward the
// next line in ents that shares an end point with it; a lone line flows from
// Start to End. Each source line gets an offset copy held parallel by an
// [Offset] constraint; offset segments that meet at a shared source corner share
// a single offset point, which the two constraints pull to the offset
// intersection — so editing the distance keeps a mitred chain. Each free end of
// a chain is held on the source line's normal at the source end
// ([NewPointOnNormal]), so the copy is fully determined by the source and d.
// A circle, and an arc that shares no end point with another member, flows
// counter-clockwise: its copy shares the source's center, a [RadialOffset] holds
// its radius at the source radius plus d (positive d is outward, as Fusion
// offsets a circle), and an arc copy's ends are held on the source normals. It
// returns [ErrForeignEntity] if s is nil or a source entity, or a point
// defining one, is not live in s, [ErrReferenceGeometry] for reference
// geometry, [ErrUnsupportedEntity] for an entity other than a line, an arc or a
// circle, and [ErrInvalidShape] if a source line has zero length (no defined
// offset direction), an end point is shared by three or more lines (not a
// chain), a copy's radius would not be positive, or an arc shares an end point
// with another member (joined arcs are not offset yet).
```

with:

```go
// CreateOffset creates the offset of a chain of lines and arcs, and of
// circles, at signed distance d, as Fusion's offset does: positive d is to the
// right of the flow, negative to the left. The flow of a chain runs from its
// first member in ents toward the next member in ents that shares an end point
// with it; a lone line or arc flows from Start to End, and a circle flows
// counter-clockwise, so positive d offsets a circle outward.
//
// Each line copy is held parallel by an [Offset] and each arc or circle copy
// shares its source's center and is held at the source radius plus or minus d
// by a [RadialOffset]. Copies that meet at a shared source point share one copy
// point. Where two members meet at an angle, that point is pulled to where the
// two offset curves cross (a mitre). Where they meet tangentially and one of
// them is an arc, the point is held on the arc's normal at the source point
// ([NewPointOnNormal]); the line beside it is held tangent to the arc copy
// instead of by an [Offset], and the second arc of a tangent arc–arc joint
// takes its radius from the joint, so no joint is constrained twice. Each free
// end of a chain is held on the source normal at the source end. The copy is
// thus fully determined by the source and d.
//
// It returns [ErrForeignEntity] if s is nil or a source entity, or a point
// defining one, is not live in s, [ErrReferenceGeometry] for reference
// geometry, [ErrUnsupportedEntity] for an entity other than a line, an arc or a
// circle, and [ErrInvalidShape] if a source line has zero length, an end point
// is shared by three or more members (not a chain), the chain folds back on
// itself at a joint (a cusp), a copy's radius would not be positive, the offset
// curves do not cross near a corner, or a closed chain has only tangent arc–arc
// joints (offset the circle instead) — in each case without modifying the
// sketch.
```

Edit 3. Replace exactly:

```go
	if err := checkOffsetCurves(ents, d); err != nil {
		return nil, err
	}
	// Validate every source line up front so a failure leaves the sketch
	// unchanged (no partial geometry committed).
	var lines []*Line
	for _, e := range ents {
		l, ok := e.(*Line)
		if !ok {
			continue
		}
		if math.Hypot(l.End.x()-l.Start.x(), l.End.y()-l.Start.y()) == 0 {
			return nil, fmt.Errorf("%w: CreateOffset source line has zero length", ErrInvalidShape)
		}
		lines = append(lines, l)
	}
	flow, err := offsetFlow(lines)
```

with:

```go
	// Plan everything up front so a failure leaves the sketch unchanged (no
	// partial geometry committed).
	plan, err := planOffset(ents, d)
```

Edit 4. Replace exactly:

```go
	offsetPt := func(src *Point, nx, ny float64) *Point {
		if cp, ok := copyOf[src]; ok {
			return cp
		}
		cp := s.CreatePoint(src.x()+d*nx, src.y()+d*ny)
		copyOf[src] = cp
		return cp
	}
	for i, l := range lines {
		// Offset's target is left-positive along the line's own direction; the
		// right of the flow is the left of the line when the flow runs against it.
		sign := -flow[i]
		dx, dy := l.End.x()-l.Start.x(), l.End.y()-l.Start.y()
		n := math.Hypot(dx, dy)         // nonzero: validated above
		nx, ny := -sign*dy/n, sign*dx/n // unit normal toward the offset side
		qa, qb := offsetPt(l.Start, nx, ny), offsetPt(l.End, nx, ny)
		dst := s.CreateLine(qa, qb)
		off := NewOffset(l, dst, sign*d)
		s.AddConstraint(off)
		g.Copies = append(g.Copies, dst)
		g.Offsets = append(g.Offsets, off)
		g.signs = append(g.signs, sign)
	}
	// A free chain end (a source end point no other source line uses) is held on
	// the source normal there, so the copy cannot slide along its own line.
	uses := map[*Point]int{}
	for _, l := range lines {
		uses[l.Start]++
		uses[l.End]++
	}
	for _, l := range lines {
		for _, p := range []*Point{l.Start, l.End} {
			if uses[p] != 1 {
				continue
			}
			pin := NewPointOnNormal(copyOf[p], l, p)
			s.AddConstraint(pin)
			g.Constraints = append(g.Constraints, pin)
		}
	}
	s.offsetCurves(g, ents, d)
	return g, nil
```

with:

```go
	for _, q := range plan.points {
		at := plan.seeds[q]
		copyOf[q] = s.CreatePoint(at.X, at.Y)
	}
	dsts := make([]Entity, len(plan.members))
	for i, m := range plan.members {
		if m.line != nil {
			dsts[i] = s.CreateLine(copyOf[m.line.Start], copyOf[m.line.End])
		} else {
			dsts[i] = s.CreateArc(m.arc.Center, copyOf[m.arc.Start], copyOf[m.arc.End])
		}
		g.Copies = append(g.Copies, dsts[i])
	}
	add := func(c Constraint) {
		s.AddConstraint(c)
		g.Constraints = append(g.Constraints, c)
	}
	for i, m := range plan.members {
		if m.arc != nil {
			if plan.radiusFromJoint[i] {
				continue // a tangent arc–arc joint before it already fixes its radius
			}
			dst, _ := dsts[i].(*Arc)
			ro := NewRadialOffset(m.arc, dst, m.flow*d)
			s.AddConstraint(ro)
			g.Radials = append(g.Radials, ro)
			g.radialSigns = append(g.radialSigns, m.flow)
			continue
		}
		dst, _ := dsts[i].(*Line)
		switch arcs := plan.tangentArcs[i]; len(arcs) {
		case 0:
			// Offset's target is left-positive along the line's own direction;
			// the right of the flow is the left of the line when the flow runs
			// against it.
			off := NewOffset(m.line, dst, -m.flow*d)
			s.AddConstraint(off)
			g.Offsets = append(g.Offsets, off)
			g.signs = append(g.signs, -m.flow)
		case 1:
			arcDst, _ := dsts[arcs[0]].(*Arc)
			add(NewTangent(dst, arcDst))
		}
	}
	for _, pin := range plan.pins {
		add(plan.members[pin.member].pin(copyOf[pin.at], pin.at))
	}
	s.offsetCircles(g, ents, d)
	return g, nil
}

// offsetMember is one line or arc of an offset chain and its flow: +1 when the
// flow runs along the member's own Start→End direction, -1 against it.
type offsetMember struct {
	e    Entity
	line *Line
	arc  *Arc
	flow float64
}

// pin holds cp on the member's normal at its source point at.
func (m offsetMember) pin(cp, at *Point) Constraint {
	if m.line != nil {
		return NewPointOnNormal(cp, m.line, at)
	}
	return NewPointOnNormal(cp, m.arc, at)
}

func (m offsetMember) ends() [2]*Point {
	if m.line != nil {
		return [2]*Point{m.line.Start, m.line.End}
	}
	return [2]*Point{m.arc.Start, m.arc.End}
}

// flowStart and flowEnd are the end points the flow enters and leaves by.
func (m offsetMember) flowStart() *Point {
	e := m.ends()
	if m.flow > 0 {
		return e[0]
	}
	return e[1]
}

func (m offsetMember) flowEnd() *Point {
	e := m.ends()
	if m.flow > 0 {
		return e[1]
	}
	return e[0]
}

// tangent returns the unit tangent at q (a point of the member) along the flow.
func (m offsetMember) tangent(q *Point) (float64, float64) {
	if m.line != nil {
		dx, dy := m.line.End.x()-m.line.Start.x(), m.line.End.y()-m.line.Start.y()
		n := math.Hypot(dx, dy)
		return m.flow * dx / n, m.flow * dy / n
	}
	rx, ry := q.x()-m.arc.Center.x(), q.y()-m.arc.Center.y()
	r := math.Hypot(rx, ry)
	return m.flow * -ry / r, m.flow * rx / r // counter-clockwise tangent, along the flow
}

// carrier returns the member's offset curve at distance d to the right of the
// flow: a line shifted sideways, or a concentric circle.
func (m offsetMember) carrier(d float64) (*geom.Line, *geom.Circle) {
	if m.line != nil {
		tx, ty := m.tangent(m.line.Start)
		nx, ny := ty*d, -tx*d // right of the flow
		return geom.NewLine(geom.NewPoint(m.line.Start.x()+nx, m.line.Start.y()+ny),
			geom.NewPoint(m.line.End.x()+nx, m.line.End.y()+ny)), nil
	}
	return nil, geom.NewCircle(m.arc.Center.Geometry(), m.arc.R()+m.flow*d)
}

// offsetPin holds the copy of the source point at on the normal of member at.
type offsetPin struct {
	member int
	at     *Point
}

// offsetPlan is everything CreateOffset commits, computed without touching the
// sketch.
type offsetPlan struct {
	members         []offsetMember
	points          []*Point               // source end points, in first-seen order
	seeds           map[*Point]*geom.Point // where each copy point starts
	tangentArcs     [][]int                // per line member: the arc members it meets tangentially
	radiusFromJoint []bool                 // per arc member: its radius comes from a tangent arc–arc joint
	pins            []offsetPin
}

// planOffset validates the chain members of ents and plans their copies.
func planOffset(ents []Entity, d float64) (*offsetPlan, error) {
	p := &offsetPlan{seeds: map[*Point]*geom.Point{}}
	for _, e := range ents {
		if l, ok := e.(*Line); ok {
			if math.Hypot(l.End.x()-l.Start.x(), l.End.y()-l.Start.y()) == 0 {
				return nil, fmt.Errorf("%w: CreateOffset source line has zero length", ErrInvalidShape)
			}
			p.members = append(p.members, offsetMember{e: e, line: l})
		}
		if a, ok := e.(*Arc); ok {
			p.members = append(p.members, offsetMember{e: e, arc: a})
		}
		if c, ok := e.(*Circle); ok && c.r()+d <= 0 {
			return nil, fmt.Errorf("%w: CreateOffset: the offset circle's radius would be %g", ErrInvalidShape, c.r()+d)
		}
	}
	ends := make([][2]*Point, len(p.members))
	for i, m := range p.members {
		ends[i] = m.ends()
	}
	flow, err := offsetFlow(ends)
	if err != nil {
		return nil, err
	}
	at := map[*Point][]int{}
	for i := range p.members {
		p.members[i].flow = flow[i]
		for _, q := range ends[i] {
			if len(at[q]) == 0 {
				p.points = append(p.points, q)
			}
			at[q] = append(at[q], i)
		}
	}
	for _, m := range p.members {
		if m.arc != nil && m.arc.R()+m.flow*d <= 0 {
			return nil, fmt.Errorf("%w: CreateOffset: the offset arc's radius would be %g", ErrInvalidShape, m.arc.R()+m.flow*d)
		}
	}
	p.tangentArcs = make([][]int, len(p.members))
	p.radiusFromJoint = make([]bool, len(p.members))
	for _, q := range p.points {
		ms := at[q]
		if len(ms) == 1 { // a free end: on the source normal
			m := p.members[ms[0]]
			tx, ty := m.tangent(q)
			p.seeds[q] = geom.NewPoint(q.x()+ty*d, q.y()-tx*d)
			p.pins = append(p.pins, offsetPin{member: ms[0], at: q})
			continue
		}
		in, out := ms[0], ms[1] // in arrives at q, out leaves it
		if p.members[in].flowEnd() != q {
			in, out = out, in
		}
		a, b := p.members[in], p.members[out]
		ax, ay := a.tangent(q)
		bx, by := b.tangent(q)
		cross, dot := ax*by-ay*bx, ax*bx+ay*by
		if math.Abs(cross) <= 1e-6 {
			if dot < 0 {
				return nil, fmt.Errorf("%w: CreateOffset: the chain folds back on itself at (%g, %g)", ErrInvalidShape, q.x(), q.y())
			}
			// Tangent joint: both offset curves touch at the shared normal.
			p.seeds[q] = geom.NewPoint(q.x()+ay*d, q.y()-ax*d)
			switch {
			case a.arc != nil && b.arc != nil:
				p.radiusFromJoint[out] = true
				p.pins = append(p.pins, offsetPin{member: in, at: q})
			case a.arc != nil:
				p.tangentArcs[out] = append(p.tangentArcs[out], in)
				p.pins = append(p.pins, offsetPin{member: in, at: q})
			case b.arc != nil:
				p.tangentArcs[in] = append(p.tangentArcs[in], out)
				p.pins = append(p.pins, offsetPin{member: out, at: q})
			}
			continue
		}
		seed, ok := mitre(a, b, q, d)
		if !ok {
			return nil, fmt.Errorf("%w: CreateOffset: the offset curves do not cross near (%g, %g)", ErrInvalidShape, q.x(), q.y())
		}
		p.seeds[q] = seed
	}
	if allTangentArcLoop(p, at) {
		return nil, fmt.Errorf("%w: CreateOffset: a closed chain of arcs joined only tangentially has no free radius; offset the circle instead", ErrInvalidShape)
	}
	return p, nil
}

// mitre returns where the offset curves of a (arriving) and b (leaving) cross,
// nearest the point a's offset reaches at the source joint q.
func mitre(a, b offsetMember, q *Point, d float64) (*geom.Point, bool) {
	ax, ay := a.tangent(q)
	near := geom.NewPoint(q.x()+ay*d, q.y()-ax*d)
	la, ca := a.carrier(d)
	lb, cb := b.carrier(d)
	var hits []*geom.Point
	switch {
	case la != nil && lb != nil:
		if h, ok := geom.LineLineIntersection(la, lb); ok {
			hits = []*geom.Point{h}
		}
	case la != nil:
		hits = geom.LineCircleIntersections(la, cb)
	case lb != nil:
		hits = geom.LineCircleIntersections(lb, ca)
	default:
		hits = geom.CircleCircleIntersections(ca, cb)
	}
	var best *geom.Point
	for _, h := range hits {
		if best == nil || math.Hypot(h.X-near.X, h.Y-near.Y) < math.Hypot(best.X-near.X, best.Y-near.Y) {
			best = h
		}
	}
	return best, best != nil
}

// allTangentArcLoop reports a closed chain whose every joint is a tangent
// arc–arc joint. Around such a loop the arc radii fix one another, so one of
// the arcs' internal radius rows is always dependent.
func allTangentArcLoop(p *offsetPlan, at map[*Point][]int) bool {
	seen := map[int]bool{}
	for start := range p.members {
		if seen[start] {
			continue
		}
		closed, allTangent := true, true
		stack := []int{start}
		seen[start] = true
		for len(stack) > 0 {
			i := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if p.members[i].arc == nil || !p.radiusFromJoint[i] {
				allTangent = false
			}
			for _, q := range p.members[i].ends() {
				if len(at[q]) < 2 {
					closed = false
				}
				for _, j := range at[q] {
					if !seen[j] {
						seen[j] = true
						stack = append(stack, j)
					}
				}
			}
		}
		if closed && allTangent {
			return true
		}
	}
	return false
```

Edit 5. Replace exactly:

```go
// checkOffsetCurves refuses, before anything is committed, a circle or arc whose
// copy would have a radius of zero or less, and an arc that shares an end point
// with another member.
func checkOffsetCurves(ents []Entity, d float64) error {
	ends := map[*Point]int{}
	for _, e := range ents {
		for _, p := range cutterEnds(e) {
			ends[p]++
		}
	}
	for _, e := range ents {
		if c, ok := e.(*Circle); ok && c.r()+d <= 0 {
			return fmt.Errorf("%w: CreateOffset: the offset circle's radius would be %g", ErrInvalidShape, c.r()+d)
		}
		a, ok := e.(*Arc)
		if !ok {
			continue
		}
		if a.R()+d <= 0 {
			return fmt.Errorf("%w: CreateOffset: the offset arc's radius would be %g", ErrInvalidShape, a.R()+d)
		}
		if ends[a.Start] > 1 || ends[a.End] > 1 {
			return fmt.Errorf("%w: CreateOffset does not offset an arc joined to another member yet", ErrInvalidShape)
		}
	}
	return nil
}

// offsetCurves adds the copies of the circles and lone arcs in ents: each shares
// its source's center, a RadialOffset holds its radius at the source radius plus
// d, and an arc copy's ends are held on the source normals at the source ends.
func (s *Sketch) offsetCurves(g *OffsetGroup, ents []Entity, d float64) {
	for _, e := range ents {
		if c, ok := e.(*Circle); ok {
			dst := s.CreateCircle(c.Center, c.r()+d)
			ro := NewRadialOffset(c, dst, d)
			s.AddConstraint(ro)
			g.Copies = append(g.Copies, dst)
			g.Radials = append(g.Radials, ro)
			g.radialSigns = append(g.radialSigns, 1)
			continue
		}
		a, ok := e.(*Arc)
		if !ok {
			continue
		}
		k := (a.R() + d) / a.R()
		at := func(p *Point) *Point {
			return s.CreatePoint(a.Center.x()+k*(p.x()-a.Center.x()), a.Center.y()+k*(p.y()-a.Center.y()))
		}
		dst := s.CreateArc(a.Center, at(a.Start), at(a.End))
		ro := NewRadialOffset(a, dst, d)
		pins := []Constraint{NewPointOnNormal(dst.Start, a, a.Start), NewPointOnNormal(dst.End, a, a.End)}
		s.AddConstraint(ro)
		s.AddConstraint(pins...)
		g.Copies = append(g.Copies, dst)
		g.Radials = append(g.Radials, ro)
		g.radialSigns = append(g.radialSigns, 1)
		g.Constraints = append(g.Constraints, pins...)
	}
}

// offsetFlow returns, per line, +1 when the offset flow runs along the line's
// own Start→End direction and -1 when it runs against it. Lines sharing end
// points form a chain. A chain's flow runs from its first line (in the given
// order) toward the next line in that order that shares a point with it, as
// Fusion's offset flows from curve 1 to curve 2; a lone line flows from Start to
// End. An end point shared by three or more lines is not a chain:
// [ErrInvalidShape].
func offsetFlow(lines []*Line) ([]float64, error) {
	at := map[*Point][]int{}
	for i, l := range lines {
		at[l.Start] = append(at[l.Start], i)
		at[l.End] = append(at[l.End], i)
	}
	for _, l := range lines {
		for _, p := range []*Point{l.Start, l.End} {
			if len(at[p]) > 2 {
				return nil, fmt.Errorf("%w: CreateOffset: an end point is shared by %d lines, which is not a chain", ErrInvalidShape, len(at[p]))
			}
		}
	}
	// other returns the line other than i that uses p, or -1.
```

with:

```go
// offsetCircles adds the copies of the circles in ents: each shares its
// source's center and a RadialOffset holds its radius at the source radius plus
// d (a circle flows counter-clockwise, so positive d is outward).
func (s *Sketch) offsetCircles(g *OffsetGroup, ents []Entity, d float64) {
	for _, e := range ents {
		c, ok := e.(*Circle)
		if !ok {
			continue
		}
		dst := s.CreateCircle(c.Center, c.r()+d)
		ro := NewRadialOffset(c, dst, d)
		s.AddConstraint(ro)
		g.Copies = append(g.Copies, dst)
		g.Radials = append(g.Radials, ro)
		g.radialSigns = append(g.radialSigns, 1)
	}
}

// offsetFlow returns, per member (given by its two end points), +1 when the
// offset flow runs along the member's own Start→End direction and -1 when it
// runs against it. Members sharing end points form a chain. A chain's flow runs
// from its first member (in the given order) toward the next member in that
// order that shares a point with it, as Fusion's offset flows from curve 1 to
// curve 2; a lone member flows from Start to End. An end point shared by three
// or more members is not a chain: [ErrInvalidShape].
func offsetFlow(ends [][2]*Point) ([]float64, error) {
	at := map[*Point][]int{}
	for i, e := range ends {
		at[e[0]] = append(at[e[0]], i)
		at[e[1]] = append(at[e[1]], i)
	}
	for _, e := range ends {
		for _, p := range e {
			if len(at[p]) > 2 {
				return nil, fmt.Errorf("%w: CreateOffset: an end point is shared by %d members, which is not a chain", ErrInvalidShape, len(at[p]))
			}
		}
	}
	// other returns the member other than i that uses p, or -1.
```

Edit 6. Replace exactly:

```go
	flow := make([]float64, len(lines))
	// walk signs the lines reached from line `from` through point p: forward
	// means the flow enters the next line at p, backward that it leaves it there.
```

with:

```go
	flow := make([]float64, len(ends))
	// walk signs the members reached from member `from` through point p: forward
	// means the flow enters the next member at p, backward that it leaves it there.
```

Edit 7. Replace exactly:

```go
			lj := lines[j]
			if (forward && lj.Start == p) || (!forward && lj.End == p) {
```

with:

```go
			start, end := ends[j][0], ends[j][1]
			if (forward && start == p) || (!forward && end == p) {
```

Edit 8. Replace exactly:

```go
			if lj.Start == p {
				p = lj.End
			} else {
				p = lj.Start
```

with:

```go
			if start == p {
				p = end
			} else {
				p = start
```

Edit 9. Replace exactly:

```go
	for i, l := range lines {
		if flow[i] != 0 {
			continue // already reached from an earlier line of its chain
		}
		toStart, toEnd := other(i, l.Start), other(i, l.End)
```

with:

```go
	for i, e := range ends {
		if flow[i] != 0 {
			continue // already reached from an earlier member of its chain
		}
		toStart, toEnd := other(i, e[0]), other(i, e[1])
```

Edit 10. Replace exactly:

```go
		exit, entry := l.End, l.Start
		if flow[i] < 0 {
			exit, entry = l.Start, l.End
```

with:

```go
		exit, entry := e[1], e[0]
		if flow[i] < 0 {
			exit, entry = e[0], e[1]
```

### Step 12.2 — `tools_offset_curve_test.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'return []sketch.Entity{s.CreateLine(s.CreatePoint(20, 0), p), arc}' tools_offset_curve_test.go   # expect 1
```

Edit 1. Replace exactly:

```go
		{"joined arc", func(s *sketch.Sketch) []sketch.Entity {
			p := s.CreatePoint(10, 0)
			arc := s.CreateArc(s.CreatePoint(0, 0), p, s.CreatePoint(0, 10))
			return []sketch.Entity{s.CreateLine(s.CreatePoint(20, 0), p), arc}
```

with:

```go
		{"cusp", func(s *sketch.Sketch) []sketch.Entity {
			p := s.CreatePoint(10, 0)
			return []sketch.Entity{s.CreateLine(s.CreatePoint(0, 0), p), s.CreateLine(p, s.CreatePoint(5, 0))}
		}, sketch.ErrInvalidShape},
		{"closed loop of tangent arcs", func(s *sketch.Sketch) []sketch.Entity {
			c, a, b := s.CreatePoint(0, 0), s.CreatePoint(5, 0), s.CreatePoint(-5, 0)
			return []sketch.Entity{s.CreateArc(c, a, b), s.CreateArc(c, b, a)}
```

### Step 12.3 — new file `tools_offset_chain_test.go`

This file must not exist yet: `test -e tools_offset_chain_test.go && echo EXISTS` prints nothing.

```go
package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// fixAll grounds every point the sketch holds, so the DOF that remains belongs
// to the offset copy alone. The sources carry no constraints of their own; only
// a source arc's internal radius row, its points grounded, reads as redundant.
func fixAll(s *sketch.Sketch) {
	for _, p := range s.Points() {
		s.Fix(p)
	}
}

// requireDeterminedOffset solves, then requires the copy to be fully
// determined with no redundant constraint of its own, before and after Set(d2).
func requireDeterminedOffset(t *testing.T, s *sketch.Sketch, g *sketch.OffsetGroup, d2 float64) {
	t.Helper()
	sources := map[sketch.Entity]struct{}{}
	for _, e := range g.Source {
		sources[e] = struct{}{}
	}
	check := func() {
		_, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.Equal(t, 0, s.DOF())
		for _, c := range s.RedundantConstraints() {
			_, ents := sketch.ConstraintRefs(c)
			require.True(t, sketch.IsInternal(c), "only a grounded source arc's radius row may read redundant")
			require.Contains(t, sources, ents[0])
		}
	}
	check()
	g.Set(d2)
	check()
}

// A slot: two lines and two half circles, every joint tangent.
func TestOffsetChainSlot(t *testing.T) {
	s := newSketch(t)
	c1, c2 := s.CreatePoint(0, 0), s.CreatePoint(10, 0)
	p1l, p1r := s.CreatePoint(0, 2), s.CreatePoint(0, -2)
	p2l, p2r := s.CreatePoint(10, 2), s.CreatePoint(10, -2)
	bottom := s.CreateLine(p1r, p2r)
	right := s.CreateArc(c2, p2r, p2l)
	top := s.CreateLine(p2l, p1l)
	left := s.CreateArc(c1, p1l, p1r)
	fixAll(s)

	// The loop runs counter-clockwise, so positive is outside.
	g, err := sketch.CreateOffset(s, []sketch.Entity{bottom, right, top, left}, 1)
	require.NoError(t, err)
	require.Empty(t, g.Offsets, "both lines take their place from the arcs")
	require.Len(t, g.Radials, 2)
	requireDeterminedOffset(t, s, g, 2)

	cb, cr := g.Copies[0].(*sketch.Line), g.Copies[1].(*sketch.Arc)
	require.InDelta(t, -4, cb.Start.Y(), 1e-6, "bottom copy at distance 2")
	require.InDelta(t, -4, cb.End.Y(), 1e-6)
	require.InDelta(t, 4, cr.R(), 1e-6)
	require.Same(t, c2, cr.Center)
}

// A rounded rectangle: four lines and four fillet arcs, every joint tangent.
func TestOffsetChainRoundedRectangle(t *testing.T) {
	s := newSketch(t)
	pt := s.CreatePoint
	b0, b1 := pt(2, 0), pt(18, 0)
	r0, r1 := pt(20, 2), pt(20, 18)
	t0, t1 := pt(18, 20), pt(2, 20)
	l0, l1 := pt(0, 18), pt(0, 2)
	ents := []sketch.Entity{
		s.CreateLine(b0, b1), s.CreateArc(pt(18, 2), b1, r0),
		s.CreateLine(r0, r1), s.CreateArc(pt(18, 18), r1, t0),
		s.CreateLine(t0, t1), s.CreateArc(pt(2, 18), t1, l0),
		s.CreateLine(l0, l1), s.CreateArc(pt(2, 2), l1, b0),
	}
	fixAll(s)

	g, err := sketch.CreateOffset(s, ents, 1)
	require.NoError(t, err)
	requireDeterminedOffset(t, s, g, 1)

	bottom := g.Copies[0].(*sketch.Line)
	require.InDelta(t, -1, bottom.Start.Y(), 1e-6)
	require.InDelta(t, 2, bottom.Start.X(), 1e-6)
	require.InDelta(t, 3, g.Copies[1].(*sketch.Arc).R(), 1e-6)
}

// A rectangle with one filleted corner: the two lines beside the fillet are
// held tangent to its copy, the other two by Offset, and the corners mitre.
func TestOffsetChainOneFilletedCorner(t *testing.T) {
	s := newSketch(t)
	pt := s.CreatePoint
	o, b1, r0, tr, tl := pt(0, 0), pt(18, 0), pt(20, 2), pt(20, 20), pt(0, 20)
	ents := []sketch.Entity{
		s.CreateLine(o, b1), s.CreateArc(pt(18, 2), b1, r0),
		s.CreateLine(r0, tr), s.CreateLine(tr, tl), s.CreateLine(tl, o),
	}
	fixAll(s)

	g, err := sketch.CreateOffset(s, ents, 1)
	require.NoError(t, err)
	require.Len(t, g.Offsets, 2)
	require.Len(t, g.Radials, 1)
	require.Equal(t, 2, countKind(s, "tangent_line_circle"))
	requireDeterminedOffset(t, s, g, 1)

	top := g.Copies[3].(*sketch.Line)
	require.InDelta(t, 21, top.Start.X(), 1e-6, "the top-right corner mitres at (21, 21)")
	require.InDelta(t, 21, top.Start.Y(), 1e-6)
	require.InDelta(t, -1, top.End.X(), 1e-6)
	require.InDelta(t, 21, top.End.Y(), 1e-6)
}

// An open chain of a line and an arc meeting tangentially: both free ends are
// held on the source normals, so the copy is determined.
func TestOffsetChainOpenLineArc(t *testing.T) {
	s := newSketch(t)
	joint := s.CreatePoint(10, 0)
	line := s.CreateLine(s.CreatePoint(0, 0), joint)
	arc := s.CreateArc(s.CreatePoint(10, 5), joint, s.CreatePoint(15, 5))
	fixAll(s)

	g, err := sketch.CreateOffset(s, []sketch.Entity{line, arc}, 1)
	require.NoError(t, err)
	requireDeterminedOffset(t, s, g, 1)

	cl, ca := g.Copies[0].(*sketch.Line), g.Copies[1].(*sketch.Arc)
	require.InDelta(t, 0, cl.Start.X(), 1e-6)
	require.InDelta(t, -1, cl.Start.Y(), 1e-6)
	require.Same(t, cl.End, ca.Start)
	require.InDelta(t, 6, ca.R(), 1e-6)
	require.InDelta(t, 16, ca.End.X(), 1e-6)
	require.InDelta(t, 5, ca.End.Y(), 1e-6)
}
```

### Step 12.4 — `docs/modification-tools-design.md`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'offset is fully determined. `OffsetGroup.Set(d)` retargets the whole offset.' docs/modification-tools-design.md   # expect 1
grep -c -F -- '- **Offset of arcs joined into a chain** — `CreateOffset` offsets circles and' docs/modification-tools-design.md   # expect 1
```

Edit 1. Replace exactly:

```markdown
  offset is fully determined. `OffsetGroup.Set(d)` retargets the whole offset.
```

with:

```markdown
  offset is fully determined. Arcs and circles are offset concentrically
  (`RadialOffset`), alone or in a chain with lines: a corner joint mitres where
  the two offset curves cross; a tangent joint touching an arc is held on the
  arc's normal (`NewPointOnNormal`), the line beside it is held tangent to the
  arc copy instead of by `Offset`, and the second arc of a tangent arc–arc joint
  takes its radius from the joint (`planOffset`). `OffsetGroup.Set(d)` retargets
  the whole offset.
```

Edit 2. Replace exactly:

```markdown
- **Offset of arcs joined into a chain** — `CreateOffset` offsets circles and
  lone arcs (a concentric copy held by `RadialOffset`) and refuses an arc that
  shares an end point with another member; any kind other than line, arc and
  circle returns `ErrUnsupportedEntity`.
```

with:

```markdown
- **Offset of a line–line tangent joint** (collinear consecutive lines) keeps
  both `Offset` constraints, so that joint carries one redundant row and one
  sliding degree of freedom. Any kind other than line, arc and circle returns
  `ErrUnsupportedEntity` from `CreateOffset`.
```

### Step 12.5 — `docs/fusion-gap-analysis.md`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- 'curve in place as Fusion does. Design in `docs/modification-tools-design.md`' docs/fusion-gap-analysis.md   # expect 1
grep -c -F -- '- **Offset of arcs and circles.** `CreateOffset` skips every entity that' docs/fusion-gap-analysis.md   # expect 1
```

Edit 1. Replace exactly:

```markdown
curve in place as Fusion does. Design in `docs/modification-tools-design.md`
```

with:

```markdown
curve in place as Fusion does. `CreateOffset` offsets circles and chains of
lines and arcs. Design in `docs/modification-tools-design.md`
```

Edit 2. Replace exactly:

```markdown
- **Offset of arcs and circles.** `CreateOffset` skips every entity that is not a
  line.
- **Project, include and intersect.** These stay out by design. The engine verifies
```

with:

```markdown
- **Project, include and intersect.** These stay out by design. The engine verifies
```

### Step 12.6 — `compound.go`

Check first (each count must print exactly as shown, else stop and report):

```sh
grep -c -F -- '[Sketch.CreatePatternRect]' compound.go   # expect 1
grep -c -F -- '[Sketch.CreatePatternCircular]' compound.go   # expect 1
```

Edit 1. Replace exactly:

```go
[Sketch.CreatePatternRect]
```

with:

```go
[CreatePatternRect]
```

Edit 2. Replace exactly:

```go
[Sketch.CreatePatternCircular]
```

with:

```go
[CreatePatternCircular]
```

### Step 12.7 — verify

Focused tests (all must pass):

```sh
go test -count=1 -run 'Offset|Tools|Reference|EntityUID' .
```

Then the common "Verify" commands.

### Step 12.8 — commit and PR

- **Commit message:** `offset chains of lines and arcs`
- **Files to add:** `tools.go`, `tools_offset_curve_test.go`, `tools_offset_chain_test.go`,
  `docs/modification-tools-design.md`, `docs/fusion-gap-analysis.md`, `compound.go`
- **PR title:** `offset chains of lines and arcs`
- **PR body:**

  ```markdown
  - Offset chains that mix lines and arcs, mitring corners and keeping tangent joints tangent
  ```
