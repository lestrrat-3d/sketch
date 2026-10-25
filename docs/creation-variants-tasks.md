# Creation Variants — Implementation Tasks

This file turns `docs/creation-variants-design.md` into six PRs, increments 0 to 5. Each section is
self-contained: follow it top to bottom. Every code block below was compiled and tested in a prototype copy of
this repository; `go test ./...`, `go vet ./...` and `golangci-lint run ./...` passed on the result. Copy the
code exactly. Do not redesign, rename, reorder or "improve" anything.

## Rules for every increment

### Stop and report

Stop and report to whoever gave you the task, instead of improvising, when any of these happens:

- an `Edit` old string does not match the file exactly;
- a command in this file fails, prints an unexpected result, or a test fails;
- `gofmt -l .` prints a file you did not touch, or `golangci-lint` reports an issue you cannot fix by
  re-copying the code from this file;
- the previous increment in the merge order is not merged yet;
- anything in the repository contradicts this file.

The report names the increment, the step, the exact command and its output. Do not change tests or expected
numbers to make them pass. Do not skip a step.

### Paths

- `PROJECT` is `/home/lestrrat/dev/src/github.com/lestrrat-3d/sketch`.
- Each increment works in its own git worktree at `$PROJECT/.worktrees/<branch>`. Never edit the root checkout.
- Pass absolute worktree paths to Read, Edit and Write. A `cd` in Bash does not change where those tools write.
- Store command output you need to re-read in the worktree's `.tmp/` directory (`mkdir -p .tmp` first). Never
  write to `/tmp`.

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
- `.claude/docs/sketch-core.md` and `.claude/docs/constraints.md`;
- `docs/creation-variants-design.md`, the section named in the increment;
- `~/.claude/docs/go.md` and `~/.claude/docs/git-operations.md`.

### Verify

Each increment lists its focused test command. Then run, from the worktree root:

```sh
gofmt -l .
go vet ./...
golangci-lint run ./...
go run ./internal/cmd/genreadme README.md docs/guide/units-and-parameters.md docs/guide/solving.md
git diff --exit-code -- README.md docs/guide
```

Expected: `gofmt -l .` prints nothing (if it lists a file you touched, run `gofmt -w <file>` once and re-run);
`go vet` prints nothing; `golangci-lint` prints `0 issues.`; the last command exits 0. If `golangci-lint` is
not on `PATH`, use `~/go/bin/golangci-lint`.

Do not run `go generate ./...`: it also rewrites the images under `docs/images`, whose watermark changes on
every commit. Do not run the full `go test ./...` locally; CI runs it.

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
5. Watch CI: `gh pr checks <pr-number> --watch --fail-fast --interval 30` (Bash timeout 600000 ms; if it times
   out, run it again). Any failed check → stop and report, with
   `gh run view <run-id> --log-failed > .tmp/gh-ci-<run-id>.log` saved first.
6. Report the PR URL and the CI result. Do not merge; merging is decided outside this task.

The PR body is 1 to 3 bullets, has no "Test plan" section, and has no AI or agent attribution line.

## Merge order and dependencies

| # | Branch | Depends on |
|---|---|---|
| 0 | `fix-tangent-circles-shared-endpoint` | — |
| 1 | `docs-shape-recipes-example` | — |
| 2 | `feat-arc-three-points` | — |
| 3 | `feat-arc-tangent` | 2 |
| 4 | `feat-circle-three-tangents` | 2 |
| 5 | `feat-arc-slot` | 0 and 2 |

Merge in the order 0, 1, 2, 3, 4, 5. Start an increment only after every increment before it in that order is
merged into `origin/main`: the `CLAUDE.md` and `docs/fusion-gap-analysis.md` edits below are written against
the text the previous increment leaves.

## Increment 0 — repair shared-endpoint `NewTangentCircles`

- **Branch:** `fix-tangent-circles-shared-endpoint`
- **Design section:** "Increment 0 — shared-endpoint arc tangency".
- **Files:** `constraint.go` (edit), `conditioning.go` (edit), `tangent_shared_endpoint_test.go` (new).

### Step 0.1 — `constraint.go`, residual head

Replace exactly:

```go
func (c *tangentCircles) residual(out []float64) []float64 {
	o1, o2 := c.C1.centerPt(), c.C2.centerPt()
	r1, r2 := c.C1.R(), c.C2.R()
```

with:

```go
func (c *tangentCircles) residual(out []float64) []float64 {
	o1, o2 := c.C1.centerPt(), c.C2.centerPt()

	// Endpoint tangency: the shared point is the contact (an arc endpoint, on
	// both circles), and the circles are tangent there exactly when both centers
	// lie on one line through it — on the same side for internal tangency, on
	// opposite sides for external. One dimensionless row measures that as an
	// angle. The center-distance row below is NOT usable here: with the shared
	// point on both circles the triangle inequality makes it non-negative, so
	// tangency is its minimum and its gradient vanishes at the solution — the
	// row adds no rank and reads as redundant. The contact is in both sweeps by
	// inclusivity, so no sweep slack row is needed.
	if c.shared != nil {
		return append(out, sharedTangentAngle(c.shared, o1, o2, c.Internal))
	}

	r1, r2 := c.C1.R(), c.C2.R()
```

### Step 0.2 — `constraint.go`, remove the old endpoint return

Replace exactly:

```go
	out = append(out, base)

	// Endpoint tangency: the shared point is the contact (an arc endpoint, on
	// both circles). The base residual alone — which already honors internal vs
	// external — pins both the tangency and the side there, and the contact is
	// in the sweep by inclusivity, so no sweep slack row is needed.
	if c.shared != nil {
		return out
	}

```

with:

```go
	out = append(out, base)

```

### Step 0.3 — `constraint.go`, add the helper

Insert this block immediately above the line `// arcInSweepExcess returns dot(contactDir, midDir) − cos(sweep/2) for the unit`:

```go
// sharedTangentAngle is the endpoint-tangency residual of two circular
// entities that share the contact point x: the signed angle, wrapped into
// (−π, π], from the radius direction x−o1 to x−o2 (internal) or to o2−x
// (external). It is zero exactly when the two circles are tangent at x on the
// requested side, and ±π on the wrong side. A zero-length radius or coincident
// centers have no tangent direction, so they return 1 rather than an angle that
// could read as satisfied.
func sharedTangentAngle(x, o1, o2 *Point, internal bool) float64 {
	u1x, u1y := x.x()-o1.x(), x.y()-o1.y()
	u2x, u2y := x.x()-o2.x(), x.y()-o2.y()
	if math.Hypot(u1x, u1y) < 1e-9 || math.Hypot(u2x, u2y) < 1e-9 ||
		math.Hypot(o2.x()-o1.x(), o2.y()-o1.y()) < 1e-9 {
		return 1
	}
	if !internal {
		u2x, u2y = -u2x, -u2y
	}
	return math.Atan2(u1x*u2y-u1y*u2x, u1x*u2x+u1y*u2y)
}

```

### Step 0.4 — `conditioning.go`

Replace exactly:

```go
	case *tangentCircles:
		out = append(out, rowLength) // center-distance tangency
		if t.shared != nil {
			return out // endpoint: no sweep rows
		}
```

with:

```go
	case *tangentCircles:
		if t.shared != nil {
			return append(out, rowDimensionless) // endpoint: wrapped angle between the radii, no sweep rows
		}
		out = append(out, rowLength) // center-distance tangency
```

### Step 0.5 — new file `tangent_shared_endpoint_test.go`

```go
package sketch_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// Two arcs tangent at a shared endpoint: the tangency must remove exactly one
// degree of freedom and must not read as redundant.
func TestTangentCirclesSharedEndpointRank(t *testing.T) {
	cases := []struct {
		name     string
		internal bool
		c2x      float64
		e2x      float64
		e2y      float64
	}{
		{"external", false, 10, 10, 5},
		{"internal", true, 3, 3, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSketch(t)
			c1 := s.CreatePoint(0, 0)
			s1 := s.CreatePoint(0, 5)
			p := s.CreatePoint(5, 0) // the shared endpoint, already tangent
			a1 := s.CreateArc(c1, s1, p)
			c2 := s.CreatePoint(tc.c2x, 0)
			e2 := s.CreatePoint(tc.e2x, tc.e2y)
			a2 := s.CreateArc(c2, p, e2)
			require.Equal(t, 8, s.DOF(), "two free arcs sharing one point")

			s.AddConstraint(sketch.NewTangentCircles(a1, a2, tc.internal))
			require.Equal(t, 7, s.DOF(), "tangency removes one degree of freedom")
			require.Empty(t, s.RedundantConstraints(), "tangency is not redundant")
		})
	}
}

// A shared-endpoint tangency drives a free arc onto the tangent configuration
// on the requested side.
func TestTangentCirclesSharedEndpointSolves(t *testing.T) {
	cases := []struct {
		name     string
		internal bool
		seedC2   [2]float64
		seedE2   [2]float64
		wantC2Y  float64
		wantE2   [2]float64
	}{
		{"external", false, [2]float64{1, 9}, [2]float64{3.5, 8.5}, 8, [2]float64{3, 8}},
		{"internal", true, [2]float64{1, 2.5}, [2]float64{3.5, 2.5}, 2, [2]float64{3, 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSketch(t)
			c1 := s.CreatePoint(0, 0)
			s1 := s.CreatePoint(5, 0)
			p := s.CreatePoint(0, 5)
			a1 := s.CreateArc(c1, s1, p)
			s.AddConstraint(
				sketch.NewCoincident(c1, s.Origin()),
				sketch.NewHorizontalPoints(c1, s1),
				sketch.NewDistance(c1, s1, 5),
				sketch.NewVerticalPoints(c1, p),
			)

			c2 := s.CreatePoint(tc.seedC2[0], tc.seedC2[1])
			e2 := s.CreatePoint(tc.seedE2[0], tc.seedE2[1])
			a2 := s.CreateArc(c2, p, e2)
			s.AddConstraint(
				sketch.NewTangentCircles(a1, a2, tc.internal),
				sketch.NewDistance(c2, p, 3),
				sketch.NewHorizontalPoints(c2, e2),
			)

			res, err := s.Solve(t.Context())
			require.NoError(t, err)
			require.Equal(t, 0, res.DOF, "fully constrained")
			require.Empty(t, s.RedundantConstraints())
			require.InDelta(t, 0, c2.X(), 1e-6, "c2 on the line through c1 and p")
			require.InDelta(t, tc.wantC2Y, c2.Y(), 1e-6, "c2 on the requested side of p")
			require.InDelta(t, tc.wantE2[0], e2.X(), 1e-6, "e2.X")
			require.InDelta(t, tc.wantE2[1], e2.Y(), 1e-6, "e2.Y")
			require.InDelta(t, 3, a2.R(), 1e-6, "a2 radius")
			require.InDelta(t, 5, a1.R(), 1e-6, "a1 radius")
		})
	}
}

// A degenerate shared-endpoint tangency (coincident centers) is never reported
// as satisfied.
func TestTangentCirclesSharedEndpointCoincidentCenters(t *testing.T) {
	s := newSketch(t)
	c := s.CreatePoint(0, 0)
	p := s.CreatePoint(5, 0)
	a1 := s.CreateArc(c, s.CreatePoint(0, 5), p)
	a2 := s.CreateArc(c, p, s.CreatePoint(-5, 0))
	tan := sketch.NewTangentCircles(a1, a2, true)
	s.AddConstraint(tan)
	res := sketch.ConstraintResiduals(tan)
	require.Len(t, res, 1)
	require.InDelta(t, 1, math.Abs(res[0]), 1e-12, "coincident centers keep the residual at 1")
}
```

### Step 0.6 — verify

Focused tests (all must pass):

```sh
go test -count=1 -run 'TestTangentCirclesSharedEndpoint|TestTangentSharedArcsRespectInternal|TestTangent|Conditioning|Jacobian|Introspect|JSON' .
go test -count=1 ./examples
```

Then the common "Verify" commands.

### Step 0.7 — commit and PR

- **Commit message:** `fix rank of shared-endpoint arc tangency`
- **Files to add:** `constraint.go`, `conditioning.go`, `tangent_shared_endpoint_test.go`
- **PR title:** `fix rank of shared-endpoint arc tangency`
- **PR body:**

  ```markdown
  - Make tangency between two arcs that share an endpoint remove one degree of freedom instead of reading as redundant
  - Unblock builders that join arcs end to end (arc slot)
  ```

## Increment 1 — shape recipes example

- **Branch:** `docs-shape-recipes-example`
- **Design section:** "Recipes: polygon and slot variants".
- **Files:** `examples/sketch_shape_recipes_example_test.go` (new), `docs/fusion-gap-analysis.md` (edit).

### Step 1.1 — new file `examples/sketch_shape_recipes_example_test.go`

```go
package examples_test

import (
	"context"
	"fmt"
	"math"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_polygonScribed builds Fusion's inscribed and circumscribed
// polygons from CreatePolygon. The two differ only in which size is
// dimensioned: an inscribed polygon fixes the center-to-corner distance, a
// circumscribed one the center-to-side distance. CreatePolygon takes a
// circumradius, so the circumscribed one is seeded at a / cos(π/n).
func Example_sketch_polygonScribed() {
	const n, size = 6, 10.0
	for _, inscribed := range []bool{true, false} {
		w := sketch.NewWorld()
		s, _ := w.CreateSketch(w.XY())

		seed := size
		if !inscribed {
			seed = size / math.Cos(math.Pi/n)
		}
		p, err := s.CreatePolygon(0, 0, n, seed)
		if err != nil {
			fmt.Printf("failed to create polygon: %s\n", err)
			return
		}
		// Ground the center on the origin and put the first corner on +x.
		s.AddConstraint(
			sketch.NewCoincident(p.Center, s.Origin()),
			sketch.NewHorizontalPoints(p.Center, p.Vertices[0]),
		)
		if inscribed {
			s.AddConstraint(sketch.NewDistance(p.Center, p.Vertices[0], size))
		} else {
			s.AddConstraint(sketch.NewDistancePointLine(p.Center, p.Sides[0], size))
		}

		res, err := s.Solve(context.Background())
		if err != nil {
			fmt.Printf("failed to solve: %s\n", err)
			return
		}
		name := "circumscribed"
		if inscribed {
			name = "inscribed"
		}
		fmt.Printf("%s: first corner (%.4f, %.4f), side %.4f, DOF %d\n",
			name, p.Vertices[0].X(), p.Vertices[0].Y(), p.Sides[0].Length(), res.DOF)
	}

	// Output:
	// inscribed: first corner (10.0000, 0.0000), side 10.0000, DOF 0
	// circumscribed: first corner (11.5470, 0.0000), side 11.5470, DOF 0
}

// Example_sketch_slotOverall dimensions a slot by its overall length, as
// Fusion's overall slot does, and centers it on the origin, as Fusion's
// center-point slot does. CreateSlot builds the slot from its cap centers; two
// tip points, each held on the slot axis and on its cap, give the overall
// length a dimension of its own. Each cap sweeps only the half facing away from
// the slot, so the point-on-arc constraint admits only the far tip. (Without tip
// points, bind the center distance to "L - W" and the cap radius to "W / 2"
// through the parameter table instead.)
func Example_sketch_slotOverall() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())

	sl, err := s.CreateSlot(-15, 0, 15, 0, 5)
	if err != nil {
		fmt.Printf("failed to create slot: %s\n", err)
		return
	}
	axis := s.CreateLine(sl.C1, sl.C2)
	axis.SetConstruction(true)
	t1, t2 := s.CreatePoint(-20, 0), s.CreatePoint(20, 0)
	mid := s.CreatePoint(0, 0)
	overall := sketch.NewDistance(t1, t2, 40)
	s.AddConstraint(
		sketch.NewPointOnLine(t1, axis), sketch.NewPointOnArc(t1, sl.A1),
		sketch.NewPointOnLine(t2, axis), sketch.NewPointOnArc(t2, sl.A2),
		// Center-point placement: the axis midpoint sits on the origin.
		sketch.NewMidpoint(mid, axis), sketch.NewCoincident(mid, s.Origin()),
		sketch.NewHorizontal(axis),
		overall,
		sketch.NewDistance(sl.C1, sl.L1.Start, 5), // cap radius: width 10
	)

	report := func() {
		res, err := s.Solve(context.Background())
		if err != nil {
			fmt.Printf("failed to solve: %s\n", err)
			return
		}
		fmt.Printf("overall %.1f: tips %.1f..%.1f, centers %.1f..%.1f, DOF %d\n",
			overall.Target().Base(), t1.X(), t2.X(), sl.C1.X(), sl.C2.X(), res.DOF)
	}
	report()
	overall.Set(50)
	report()

	// Output:
	// overall 40.0: tips -20.0..20.0, centers -15.0..15.0, DOF 0
	// overall 50.0: tips -25.0..25.0, centers -20.0..20.0, DOF 0
}
```

### Step 1.2 — `docs/fusion-gap-analysis.md`

Replace exactly:

```markdown
- ~~**Rectangle / polygon constructors**~~ — *closed*: `CreateRectangle`
  (H/V constraints) and `CreatePolygon` (equal sides + equal construction spokes).
```

with:

```markdown
- ~~**Rectangle / polygon constructors**~~ — *closed*: `CreateRectangle`
  (H/V constraints) and `CreatePolygon` (equal sides + equal construction spokes).
- **Creation variants** — design in `docs/creation-variants-design.md`. Recipes:
  inscribed/circumscribed polygon and overall-length/center-point slot
  (`examples/sketch_shape_recipes_example_test.go`).
  Builders: none yet.
```

### Step 1.3 — verify

```sh
go test -count=1 -run 'Example_sketch_(polygonScribed|slotOverall)$' -v ./examples
```

Both examples must print `--- PASS`. Then the common "Verify" commands.

### Step 1.4 — commit and PR

- **Commit message:** `add polygon and slot recipe examples`
- **Files to add:** `examples/sketch_shape_recipes_example_test.go`, `docs/fusion-gap-analysis.md`
- **PR title:** `add polygon and slot recipe examples`
- **PR body:**

  ```markdown
  - Show Fusion's inscribed/circumscribed polygon and overall-length/center-point slot as recipes on existing builders
  ```

## Increment 2 — `geom.ArcThroughPoints` and `CreateArcThreePoints`

- **Branch:** `feat-arc-three-points`
- **Design section:** "`CreateArcThreePoints` — three-point arc".
- **Extra read:** `.claude/docs/profiles-geom.md`, section "The `geom` package (slated for extraction)"
  (this increment edits `geom/`).
- **Files:** `geom/modify.go`, `geom/toolkit_test.go`, `compound.go`, `revision_internal_test.go`,
  `CLAUDE.md`, `docs/fusion-gap-analysis.md` (edits); `arc_three_points_test.go`,
  `examples/sketch_arc_three_points_example_test.go` (new).

### Step 2.1 — `geom/modify.go`

Append at the end of the file:

```go

// ArcThroughPoints returns the arc through start, mid and end, swept
// counter-clockwise. Its Center is a new point at the circumcenter of the three
// points. When mid lies on the counter-clockwise path from start to end, the
// returned arc's Start and End are the start and end pointers passed in;
// otherwise they are swapped (Start is end, End is start), so the arc still
// passes through mid. ok is false when a point is nil or has a non-finite
// coordinate, when two of the points coincide, or when the three points are
// collinear.
func ArcThroughPoints(start, mid, end *Point) (*Arc, bool) {
	for _, p := range []*Point{start, mid, end} {
		if p == nil || math.IsNaN(p.X) || math.IsNaN(p.Y) || math.IsInf(p.X, 0) || math.IsInf(p.Y, 0) {
			return nil, false
		}
	}
	bx, by := end.X-start.X, end.Y-start.Y
	mx, my := mid.X-start.X, mid.Y-start.Y
	lb, lm := math.Hypot(bx, by), math.Hypot(mx, my)
	cross := bx*my - by*mx
	// Coincident points make lb or lm zero (or, for mid == end, cross zero), so
	// this one test also refuses them.
	if lb == 0 || lm == 0 || math.Abs(cross) <= 1e-9*lb*lm {
		return nil, false
	}
	bb, mm := bx*bx+by*by, mx*mx+my*my
	d := 2 * cross
	center := NewPoint(start.X+(my*bb-by*mm)/d, start.Y+(bx*mm-mx*bb)/d)
	if cross < 0 { // start → mid → end runs counter-clockwise
		return NewArc(center, start, end), true
	}
	return NewArc(center, end, start), true
}
```

### Step 2.2 — `geom/toolkit_test.go`

Append at the end of the file:

```go

func TestArcThroughPoints(t *testing.T) {
	t.Run("counter-clockwise", func(t *testing.T) {
		start, mid, end := geom.NewPoint(10, 0), geom.NewPoint(0, 10), geom.NewPoint(-10, 0)
		arc, ok := geom.ArcThroughPoints(start, mid, end)
		require.True(t, ok)
		require.Same(t, start, arc.Start, "start kept as Start")
		require.Same(t, end, arc.End, "end kept as End")
		require.InDelta(t, 0, arc.Center.X, 1e-12, "center x")
		require.InDelta(t, 0, arc.Center.Y, 1e-12, "center y")
		require.InDelta(t, 10, arc.Radius(), 1e-12, "radius")
		require.InDelta(t, math.Pi, arc.Sweep(), 1e-12, "half turn")
	})
	t.Run("clockwise mid swaps the ends", func(t *testing.T) {
		start, mid, end := geom.NewPoint(10, 0), geom.NewPoint(0, -10), geom.NewPoint(-10, 0)
		arc, ok := geom.ArcThroughPoints(start, mid, end)
		require.True(t, ok)
		require.Same(t, end, arc.Start, "end becomes Start")
		require.Same(t, start, arc.End, "start becomes End")
		require.InDelta(t, 0, arc.Center.X, 1e-12, "center x")
		require.InDelta(t, 0, arc.Center.Y, 1e-12, "center y")
		require.InDelta(t, math.Pi, arc.Sweep(), 1e-12, "half turn through (0, -10)")
	})
	t.Run("off-center", func(t *testing.T) {
		arc, ok := geom.ArcThroughPoints(geom.NewPoint(4, 2), geom.NewPoint(1, 5), geom.NewPoint(-2, 2))
		require.True(t, ok)
		require.InDelta(t, 1, arc.Center.X, 1e-12, "center x")
		require.InDelta(t, 2, arc.Center.Y, 1e-12, "center y")
		require.InDelta(t, 3, arc.Radius(), 1e-12, "radius")
	})
	t.Run("refusals", func(t *testing.T) {
		_, ok := geom.ArcThroughPoints(geom.NewPoint(0, 0), geom.NewPoint(5, 0), geom.NewPoint(10, 0))
		require.False(t, ok, "collinear")
		_, ok = geom.ArcThroughPoints(geom.NewPoint(0, 0), geom.NewPoint(0, 0), geom.NewPoint(10, 0))
		require.False(t, ok, "mid on start")
		_, ok = geom.ArcThroughPoints(geom.NewPoint(0, 0), geom.NewPoint(5, 5), geom.NewPoint(0, 0))
		require.False(t, ok, "start on end")
		_, ok = geom.ArcThroughPoints(geom.NewPoint(0, 0), geom.NewPoint(10, 0), geom.NewPoint(10, 0))
		require.False(t, ok, "mid on end")
		_, ok = geom.ArcThroughPoints(geom.NewPoint(0, 0), geom.NewPoint(math.NaN(), 5), geom.NewPoint(10, 0))
		require.False(t, ok, "NaN")
		_, ok = geom.ArcThroughPoints(geom.NewPoint(0, 0), geom.NewPoint(5, math.Inf(1)), geom.NewPoint(10, 0))
		require.False(t, ok, "Inf")
		_, ok = geom.ArcThroughPoints(nil, geom.NewPoint(5, 5), geom.NewPoint(10, 0))
		require.False(t, ok, "nil")
	})
}
```

### Step 2.3 — `compound.go`, imports

Replace exactly:

```go
import (
	"errors"
	"fmt"
	"math"
)
```

with:

```go
import (
	"errors"
	"fmt"
	"math"

	"github.com/lestrrat-3d/sketch/geom"
)
```

### Step 2.4 — `compound.go`, `ErrInvalidShape` doc comment

Replace exactly:

```go
// ErrInvalidShape is returned by the compound shape builders ([Sketch.CreatePolygon],
// [Sketch.CreateSlot]) and the pattern tools ([Sketch.CreatePatternRect],
// [Sketch.CreatePatternCircular]) when given counts or coordinates that cannot form
// the requested shape.
```

with:

```go
// ErrInvalidShape is returned by the compound shape builders and creation
// variants in this file (for example [Sketch.CreatePolygon], [Sketch.CreateSlot]
// and [Sketch.CreateArcThreePoints]) and by the pattern tools
// ([Sketch.CreatePatternRect], [Sketch.CreatePatternCircular]) when given counts
// or coordinates that cannot form the requested shape.
```

### Step 2.5 — `compound.go`, builder

Append at the end of the file:

```go

// allFinite reports whether every value is neither NaN nor ±Inf.
func allFinite(vs ...float64) bool {
	for _, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// CreateArcThreePoints adds an arc from start to end that passes through
// (mx, my), the counterpart of Fusion's three-point arc. start and end are
// existing points, so the arc can share them with other geometry; (mx, my) only
// places the arc and is not kept as a point. A new center point is created at
// the circumcenter. Arcs sweep counter-clockwise, so when (mx, my) lies
// clockwise of the path from start to end, the returned arc's Start is end and
// its End is start. To keep a point on the arc, create it and add
// [NewPointOnArc]. Beyond its two endpoints the arc adds one degree of freedom
// (its bulge). It returns [ErrForeignEntity] if start or end is not a live point
// of this sketch, and [ErrInvalidShape] if start and end are the same point, mx
// or my is not finite, or the three positions coincide or are collinear — in
// each case without modifying the sketch.
func (s *Sketch) CreateArcThreePoints(start *Point, mx, my float64, end *Point) (*Arc, error) {
	if !s.owns(start) || !s.owns(end) {
		return nil, ErrForeignEntity
	}
	if start == end {
		return nil, fmt.Errorf("%w: CreateArcThreePoints requires distinct start and end points", ErrInvalidShape)
	}
	if !allFinite(mx, my) {
		return nil, fmt.Errorf("%w: CreateArcThreePoints requires a finite middle position", ErrInvalidShape)
	}
	sg, eg := start.Geometry(), end.Geometry()
	g, ok := geom.ArcThroughPoints(sg, geom.NewPoint(mx, my), eg)
	if !ok {
		return nil, fmt.Errorf("%w: CreateArcThreePoints requires three distinct, finite, non-collinear positions", ErrInvalidShape)
	}
	center := s.CreatePoint(g.Center.X, g.Center.Y)
	if g.Start == sg {
		return s.CreateArc(center, start, end), nil
	}
	return s.CreateArc(center, end, start), nil
}
```

### Step 2.6 — new file `arc_three_points_test.go`

```go
package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestCreateArcThreePoints(t *testing.T) {
	t.Run("counter-clockwise placement", func(t *testing.T) {
		s := newSketch(t)
		start, end := s.CreatePoint(10, 0), s.CreatePoint(-10, 0)
		arc, err := s.CreateArcThreePoints(start, 0, 10, end)
		require.NoError(t, err)
		require.Same(t, start, arc.Start, "start kept as Start")
		require.Same(t, end, arc.End, "end kept as End")
		require.InDelta(t, 0, arc.Center.X(), 1e-9, "center x")
		require.InDelta(t, 0, arc.Center.Y(), 1e-9, "center y")
		require.InDelta(t, 10, arc.R(), 1e-9, "radius")
		require.InDelta(t, math.Pi, arc.Sweep(), 1e-9, "sweep")
		require.Len(t, s.Points(), 3, "start, end and the new center")
		require.Equal(t, 5, s.DOF(), "a free arc")
	})

	t.Run("clockwise mid swaps the ends", func(t *testing.T) {
		s := newSketch(t)
		start, end := s.CreatePoint(10, 0), s.CreatePoint(-10, 0)
		arc, err := s.CreateArcThreePoints(start, 0, -10, end)
		require.NoError(t, err)
		require.Same(t, end, arc.Start, "end becomes Start")
		require.Same(t, start, arc.End, "start becomes End")
		require.InDelta(t, math.Pi, arc.Sweep(), 1e-9, "sweep through (0, -10)")
		require.InDelta(t, -math.Pi/2, arc.StartAngle()+arc.Sweep()/2-2*math.Pi, 1e-9, "midpoint of the sweep at -90 degrees")
	})

	t.Run("fully constrained and parametric", func(t *testing.T) {
		s := newSketch(t)
		start, end := s.CreatePoint(10, 0), s.CreatePoint(-10, 0)
		arc, err := s.CreateArcThreePoints(start, 0, 10, end)
		require.NoError(t, err)
		radius := sketch.NewRadius(arc, 12)
		s.AddConstraint(
			sketch.NewCoincident(arc.Center, s.Origin()),
			radius,
			sketch.NewHorizontalPoints(arc.Center, start),
			sketch.NewHorizontalPoints(arc.Center, end),
		)
		res, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.Equal(t, 0, res.DOF, "fully constrained")
		require.Empty(t, s.RedundantConstraints())
		require.InDelta(t, 12, start.X(), 1e-6, "start x")
		require.InDelta(t, 0, start.Y(), 1e-6, "start y")
		require.InDelta(t, -12, end.X(), 1e-6, "end x")
		require.InDelta(t, 0, end.Y(), 1e-6, "end y")
		require.Nil(t, s.Verify(t.Context()).Check())

		radius.Set(15)
		_, err = s.Solve(t.Context())
		require.NoError(t, err)
		require.InDelta(t, 15, start.X(), 1e-6, "start follows the radius")
		require.InDelta(t, -15, end.X(), 1e-6, "end follows the radius")

		data, err := json.Marshal(s)
		require.NoError(t, err)
		var s2 sketch.Sketch
		require.NoError(t, json.Unmarshal(data, &s2))
		require.Len(t, s2.Entities(), len(s.Entities()))
		require.Len(t, s2.Constraints(), len(s.Constraints()))
		res2, err := s2.Solve(t.Context())
		require.NoError(t, err)
		require.Equal(t, 0, res2.DOF, "reloaded DOF")
	})

	t.Run("origin as start", func(t *testing.T) {
		s := newSketch(t)
		end := s.CreatePoint(10, 0)
		arc, err := s.CreateArcThreePoints(s.Origin(), 5, 5, end)
		require.NoError(t, err)
		require.Same(t, end, arc.Start, "(5, 5) lies clockwise of origin -> (10, 0)")
		require.Same(t, s.Origin(), arc.End)
		require.InDelta(t, 5, arc.Center.X(), 1e-9, "center x")
		require.InDelta(t, 0, arc.Center.Y(), 1e-9, "center y")
	})

	t.Run("refusals", func(t *testing.T) {
		s := newSketch(t)
		a, b := s.CreatePoint(0, 0), s.CreatePoint(10, 0)
		counts := func() [3]int { return [3]int{len(s.Points()), len(s.Entities()), len(s.Constraints())} }
		before := counts()
		cases := []struct {
			name   string
			start  *sketch.Point
			mx, my float64
			end    *sketch.Point
			want   error
		}{
			{"foreign start", foreignPoint(t, 0, 0, 0), 5, 5, b, sketch.ErrForeignEntity},
			{"foreign end", a, 5, 5, foreignPoint(t, 0, 10, 0), sketch.ErrForeignEntity},
			{"nil end", a, 5, 5, nil, sketch.ErrForeignEntity},
			{"same start and end", a, 5, 5, a, sketch.ErrInvalidShape},
			{"collinear", a, 5, 0, b, sketch.ErrInvalidShape},
			{"mid on start", a, 0, 0, b, sketch.ErrInvalidShape},
			{"mid on end", a, 10, 0, b, sketch.ErrInvalidShape},
			{"NaN mid", a, math.NaN(), 5, b, sketch.ErrInvalidShape},
			{"Inf mid", a, 5, math.Inf(1), b, sketch.ErrInvalidShape},
		}
		for _, tc := range cases {
			arc, err := s.CreateArcThreePoints(tc.start, tc.mx, tc.my, tc.end)
			require.ErrorIs(t, err, tc.want, tc.name)
			require.Nil(t, arc, tc.name)
			require.Equal(t, before, counts(), "%s: the sketch is unchanged", tc.name)
		}
	})
}
```

### Step 2.7 — `revision_internal_test.go`

In `TestEntityUIDInvariant`, find `t.Run("compound builders", ...)`. Insert these lines inside that subtest,
immediately above its last line `requireStamped(t, s)`:

```go
		_, err = s.CreateArcThreePoints(s.CreatePoint(70, 0), 60, 10, s.CreatePoint(50, 0))
		require.NoError(t, err)
```

### Step 2.8 — new file `examples/sketch_arc_three_points_example_test.go`

```go
package examples_test

import (
	"context"
	"fmt"
	"math"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_arcThreePoints draws an arc through three positions, as
// Fusion's three-point arc does. The middle position only places the arc: the
// builder creates the center at the circumcenter, and the arc keeps its two
// endpoints. Because arcs sweep counter-clockwise, a middle position on the
// clockwise side makes the given end the arc's Start.
func Example_sketch_arcThreePoints() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())

	start, end := s.CreatePoint(10, 0), s.CreatePoint(-10, 0)
	arc, err := s.CreateArcThreePoints(start, 0, 10, end)
	if err != nil {
		fmt.Printf("failed to create arc: %s\n", err)
		return
	}
	fmt.Printf("center (%.1f, %.1f), radius %.1f, sweep %.0f deg, start kept: %v\n",
		arc.Center.X(), arc.Center.Y(), arc.R(), arc.Sweep()*180/math.Pi, arc.Start == start)

	radius := sketch.NewRadius(arc, 12)
	s.AddConstraint(
		sketch.NewCoincident(arc.Center, s.Origin()),
		radius,
		sketch.NewHorizontalPoints(arc.Center, start),
		sketch.NewHorizontalPoints(arc.Center, end),
	)
	res, err := s.Solve(context.Background())
	if err != nil {
		fmt.Printf("failed to solve: %s\n", err)
		return
	}
	fmt.Printf("radius 12: start (%.1f, %.1f), end (%.1f, %.1f), DOF %d\n",
		start.X(), start.Y(), end.X(), end.Y(), res.DOF)

	// Output:
	// center (0.0, 0.0), radius 10.0, sweep 180 deg, start kept: true
	// radius 12: start (12.0, 0.0), end (-12.0, 0.0), DOF 0
}
```

### Step 2.9 — `CLAUDE.md`

Edit 1. Replace exactly:

```markdown
| `compound.go` | Compound shape builders (`CreateRectangle`/`CreatePolygon`/`CreateSlot`): primitives + shape-holding constraints, returned as a grouping handle (handle itself is not serialized). | — |
```

with:

```markdown
| `compound.go` | Compound shape builders (`CreateRectangle`/`CreatePolygon`/`CreateSlot`) and creation variants (`CreateArcThreePoints`): primitives + shape-holding constraints, returned as the entity or a grouping handle (handle itself is not serialized). Design of the variants in `docs/creation-variants-design.md`. | — |
```

Edit 2. Replace exactly:

```markdown
  The shape/pattern builders (`CreatePolygon`/`CreateSlot`/`CreateSpline`/
  `CreatePatternRect`/`CreatePatternCircular` → `ErrInvalidShape`),
```

with:

```markdown
  The shape/pattern builders (`CreatePolygon`/`CreateSlot`/`CreateSpline`/
  `CreatePatternRect`/`CreatePatternCircular` and the creation variants
  `CreateArcThreePoints` → `ErrInvalidShape`),
```

### Step 2.10 — `docs/fusion-gap-analysis.md`

Replace exactly:

```markdown
  Builders: none yet.
```

with:

```markdown
  Builders: `CreateArcThreePoints`.
```

### Step 2.11 — verify

```sh
go test -count=1 -run 'TestArcThroughPoints|TestFillet' ./geom
go test -count=1 -run 'TestCreateArcThreePoints|TestEntityUIDInvariant' .
go test -count=1 -run 'Example_sketch_arcThreePoints$' -v ./examples
```

Then the common "Verify" commands.

### Step 2.12 — commit and PR

- **Commit message:** `add three-point arc builder`
- **Files to add:** `geom/modify.go`, `geom/toolkit_test.go`, `compound.go`, `arc_three_points_test.go`,
  `revision_internal_test.go`, `examples/sketch_arc_three_points_example_test.go`, `CLAUDE.md`,
  `docs/fusion-gap-analysis.md`
- **PR title:** `add three-point arc builder`
- **PR body:**

  ```markdown
  - Let callers draw an arc through three positions the way Fusion's three-point arc does
  - Keep the arc counter-clockwise without the caller working out which end comes first
  ```

## Increment 3 — `CreateArcTangent`

- **Branch:** `feat-arc-tangent`
- **Design section:** "`CreateArcTangent` — tangent arc from a line endpoint".
- **Requires:** increment 2 merged (`allFinite` and the `geom` import exist in `compound.go`).
- **Files:** `compound.go`, `revision_internal_test.go`, `CLAUDE.md`, `docs/fusion-gap-analysis.md` (edits);
  `arc_tangent_test.go`, `examples/sketch_arc_tangent_example_test.go` (new).

### Step 3.1 — `compound.go`

Append at the end of the file:

```go

// CreateArcTangent adds an arc that starts at at, an endpoint of l, leaves it
// tangent to l in the direction l runs toward at, and ends at a new point
// (x, y). It is the counterpart of Fusion's tangent arc. A new center point is
// created, and [NewTangent] holds the arc tangent to l at the shared point, so
// the arc stays tangent when either is re-dimensioned. Arcs sweep
// counter-clockwise: an arc that turns left has at as its Start, and one that
// turns right has at as its End. Beyond the line, the arc adds two degrees of
// freedom (its end point). It returns [ErrForeignEntity] if l is not a live line
// of this sketch or has an endpoint that is not a live point of it, and
// [ErrInvalidShape] if at is not an endpoint of l, l has zero length, x or y is
// not finite, or (x, y) lies on the infinite line through l (no arc tangent at
// at reaches it) — in each case without modifying the sketch.
func (s *Sketch) CreateArcTangent(l *Line, at *Point, x, y float64) (*Arc, error) {
	if s.foreignInput(l) {
		return nil, ErrForeignEntity
	}
	var other *Point
	switch at {
	case l.End:
		other = l.Start
	case l.Start:
		other = l.End
	default:
		return nil, fmt.Errorf("%w: CreateArcTangent requires at to be an endpoint of l", ErrInvalidShape)
	}
	if !allFinite(x, y) {
		return nil, fmt.Errorf("%w: CreateArcTangent requires a finite end point", ErrInvalidShape)
	}
	dx, dy := at.x()-other.x(), at.y()-other.y()
	dl := math.Hypot(dx, dy)
	if dl <= epsTool {
		return nil, fmt.Errorf("%w: CreateArcTangent requires a line of nonzero length", ErrInvalidShape)
	}
	vx, vy := x-at.x(), y-at.y()
	cross := dx*vy - dy*vx
	if math.Abs(cross) <= epsTool*dl*math.Hypot(vx, vy) {
		return nil, fmt.Errorf("%w: CreateArcTangent requires an end point off the line", ErrInvalidShape)
	}
	nx, ny := -dy/dl, dx/dl // left normal of the travel direction
	k := (vx*vx + vy*vy) / (2 * (nx*vx + ny*vy))
	end := s.CreatePoint(x, y)
	center := s.CreatePoint(at.x()+k*nx, at.y()+k*ny)
	var a *Arc
	if cross > 0 {
		a = s.CreateArc(center, at, end)
	} else {
		a = s.CreateArc(center, end, at)
	}
	s.AddConstraint(NewTangent(l, a))
	return a, nil
}
```

### Step 3.2 — new file `arc_tangent_test.go`

```go
package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// arcLeavesAlong reports the dot product of the arc's unit tangent at p (the
// direction the arc moves away from p) with the unit direction (dx, dy).
func arcLeavesAlong(a *sketch.Arc, p *sketch.Point, dx, dy float64) float64 {
	rx, ry := p.X()-a.Center.X(), p.Y()-a.Center.Y()
	// Counter-clockwise travel direction at p is the radius turned left.
	tx, ty := -ry, rx
	if p == a.End { // leaving from End means travelling clockwise
		tx, ty = ry, -rx
	}
	tl, dl := math.Hypot(tx, ty), math.Hypot(dx, dy)
	return (tx*dx + ty*dy) / (tl * dl)
}

func TestCreateArcTangent(t *testing.T) {
	cases := []struct {
		name      string
		ey        float64
		wantStart bool // at is the arc's Start (left turn)
		centerY   float64
		solvedY   float64 // center and end y after the radius is driven to 8
	}{
		{"left turn", 5, true, 5, 8},
		{"right turn", -5, false, -5, -8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSketch(t)
			p0, p1 := s.CreatePoint(0, 0), s.CreatePoint(10, 0)
			l := s.CreateLine(p0, p1)
			s.AddConstraint(
				sketch.NewCoincident(p0, s.Origin()),
				sketch.NewHorizontal(l),
				sketch.NewDistance(p0, p1, 10),
			)
			require.Equal(t, 0, s.DOF(), "the line is fully constrained")

			arc, err := s.CreateArcTangent(l, p1, 15, tc.ey)
			require.NoError(t, err)
			if tc.wantStart {
				require.Same(t, p1, arc.Start)
			} else {
				require.Same(t, p1, arc.End)
			}
			require.InDelta(t, 10, arc.Center.X(), 1e-9, "center x")
			require.InDelta(t, tc.centerY, arc.Center.Y(), 1e-9, "center y")
			require.InDelta(t, 5, arc.R(), 1e-9, "radius")
			require.InDelta(t, math.Pi/2, arc.Sweep(), 1e-9, "quarter turn")
			require.InDelta(t, 1, arcLeavesAlong(arc, p1, 1, 0), 1e-9, "leaves p1 along +x")
			require.Equal(t, 2, s.DOF(), "the end point is free")

			end := arc.End
			if !tc.wantStart {
				end = arc.Start
			}
			radius := sketch.NewRadius(arc, 8)
			s.AddConstraint(radius, sketch.NewHorizontalPoints(arc.Center, end))
			res, err := s.Solve(t.Context())
			require.NoError(t, err)
			require.Equal(t, 0, res.DOF, "fully constrained")
			require.Empty(t, s.RedundantConstraints())
			require.InDelta(t, 10, arc.Center.X(), 1e-6, "center x")
			require.InDelta(t, tc.solvedY, arc.Center.Y(), 1e-6, "center y")
			require.InDelta(t, 18, end.X(), 1e-6, "end x")
			require.InDelta(t, tc.solvedY, end.Y(), 1e-6, "end y")
			require.Greater(t, arcLeavesAlong(arc, p1, 1, 0), 0.999999, "still a smooth continuation, not a cusp")
			require.Nil(t, s.Verify(t.Context()).Check())

			data, err := json.Marshal(s)
			require.NoError(t, err)
			var s2 sketch.Sketch
			require.NoError(t, json.Unmarshal(data, &s2))
			require.Len(t, s2.Entities(), len(s.Entities()))
			require.Len(t, s2.Constraints(), len(s.Constraints()))
			res2, err := s2.Solve(t.Context())
			require.NoError(t, err)
			require.Equal(t, 0, res2.DOF, "reloaded DOF")
		})
	}

	t.Run("from the line's start", func(t *testing.T) {
		s := newSketch(t)
		p0, p1 := s.CreatePoint(0, 0), s.CreatePoint(10, 0)
		l := s.CreateLine(p0, p1)
		arc, err := s.CreateArcTangent(l, p0, -5, -5)
		require.NoError(t, err)
		require.Same(t, p0, arc.Start, "travelling -x, (-5, -5) is a left turn")
		require.InDelta(t, 0, arc.Center.X(), 1e-9, "center x")
		require.InDelta(t, -5, arc.Center.Y(), 1e-9, "center y")
		require.InDelta(t, 1, arcLeavesAlong(arc, p0, -1, 0), 1e-9, "leaves p0 along -x")
	})

	t.Run("refusals", func(t *testing.T) {
		s := newSketch(t)
		p0, p1 := s.CreatePoint(0, 0), s.CreatePoint(10, 0)
		l := s.CreateLine(p0, p1)
		q := s.CreatePoint(3, 3)
		z := s.CreateLine(q, q) // zero length
		counts := func() [3]int { return [3]int{len(s.Points()), len(s.Entities()), len(s.Constraints())} }
		before := counts()
		fl := foreignLine(t, 0, 0, 10, 0)
		cases := []struct {
			name string
			l    *sketch.Line
			at   *sketch.Point
			x, y float64
			want error
		}{
			{"foreign line", fl, fl.End, 15, 5, sketch.ErrForeignEntity},
			{"nil line", nil, p1, 15, 5, sketch.ErrForeignEntity},
			{"at not an endpoint", l, q, 15, 5, sketch.ErrInvalidShape},
			{"zero-length line", z, q, 15, 5, sketch.ErrInvalidShape},
			{"end ahead on the line", l, p1, 20, 0, sketch.ErrInvalidShape},
			{"end behind on the line", l, p1, -5, 0, sketch.ErrInvalidShape},
			{"end on at", l, p1, 10, 0, sketch.ErrInvalidShape},
			{"NaN end", l, p1, math.NaN(), 5, sketch.ErrInvalidShape},
			{"Inf end", l, p1, 15, math.Inf(-1), sketch.ErrInvalidShape},
		}
		for _, tc := range cases {
			arc, err := s.CreateArcTangent(tc.l, tc.at, tc.x, tc.y)
			require.ErrorIs(t, err, tc.want, tc.name)
			require.Nil(t, arc, tc.name)
			require.Equal(t, before, counts(), "%s: the sketch is unchanged", tc.name)
		}
	})
}
```

### Step 3.3 — `revision_internal_test.go`

In `TestEntityUIDInvariant`, inside `t.Run("compound builders", ...)`, insert immediately above its last line
`requireStamped(t, s)`:

```go
		tl := s.CreateLine(s.CreatePoint(0, 70), s.CreatePoint(10, 70))
		_, err = s.CreateArcTangent(tl, tl.End, 15, 75)
		require.NoError(t, err)
```

### Step 3.4 — new file `examples/sketch_arc_tangent_example_test.go`

```go
package examples_test

import (
	"context"
	"fmt"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_arcTangent continues a line with a tangent arc, as Fusion's
// tangent arc does. The arc leaves the line's end in the line's direction and
// stays tangent to it when re-dimensioned: driving the radius from 5 to 8 moves
// the center along the line's normal.
func Example_sketch_arcTangent() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())

	p0, p1 := s.CreatePoint(0, 0), s.CreatePoint(10, 0)
	l := s.CreateLine(p0, p1)
	s.AddConstraint(
		sketch.NewCoincident(p0, s.Origin()),
		sketch.NewHorizontal(l),
		sketch.NewDistance(p0, p1, 10),
	)

	arc, err := s.CreateArcTangent(l, p1, 15, 5) // a left turn up to (15, 5)
	if err != nil {
		fmt.Printf("failed to create arc: %s\n", err)
		return
	}
	fmt.Printf("center (%.1f, %.1f), radius %.1f, DOF %d\n", arc.Center.X(), arc.Center.Y(), arc.R(), s.DOF())

	s.AddConstraint(sketch.NewRadius(arc, 8), sketch.NewHorizontalPoints(arc.Center, arc.End))
	res, err := s.Solve(context.Background())
	if err != nil {
		fmt.Printf("failed to solve: %s\n", err)
		return
	}
	fmt.Printf("radius 8: center (%.1f, %.1f), end (%.1f, %.1f), DOF %d\n",
		arc.Center.X(), arc.Center.Y(), arc.End.X(), arc.End.Y(), res.DOF)

	// Output:
	// center (10.0, 5.0), radius 5.0, DOF 2
	// radius 8: center (10.0, 8.0), end (18.0, 8.0), DOF 0
}
```

### Step 3.5 — `CLAUDE.md`

Edit 1. Replace exactly:

```markdown
creation variants (`CreateArcThreePoints`):
```

with:

```markdown
creation variants (`CreateArcThreePoints`/`CreateArcTangent`):
```

Edit 2. Replace exactly:

```markdown
  `CreateArcThreePoints` → `ErrInvalidShape`),
```

with:

```markdown
  `CreateArcThreePoints`/`CreateArcTangent` → `ErrInvalidShape`),
```

### Step 3.6 — `docs/fusion-gap-analysis.md`

Replace exactly:

```markdown
  Builders: `CreateArcThreePoints`.
```

with:

```markdown
  Builders: `CreateArcThreePoints`, `CreateArcTangent`.
```

### Step 3.7 — verify

```sh
go test -count=1 -run 'TestCreateArcTangent|TestEntityUIDInvariant' .
go test -count=1 -run 'Example_sketch_arcTangent$' -v ./examples
```

Then the common "Verify" commands.

### Step 3.8 — commit and PR

- **Commit message:** `add tangent arc builder`
- **Files to add:** `compound.go`, `arc_tangent_test.go`, `revision_internal_test.go`,
  `examples/sketch_arc_tangent_example_test.go`, `CLAUDE.md`, `docs/fusion-gap-analysis.md`
- **PR title:** `add tangent arc builder`
- **PR body:**

  ```markdown
  - Let callers continue a line with a tangent arc the way Fusion's tangent arc does
  - Keep the arc tangent through later dimension edits instead of only at creation
  ```

## Increment 4 — `CreateCircleThreeTangents`

- **Branch:** `feat-circle-three-tangents`
- **Design section:** "`CreateCircleThreeTangents` — three-tangent circle".
- **Requires:** increment 2 merged (`allFinite` exists in `compound.go`).
- **Files:** `compound.go`, `revision_internal_test.go`, `CLAUDE.md`, `docs/fusion-gap-analysis.md` (edits);
  `circle_three_tangents_test.go`, `examples/sketch_circle_three_tangents_example_test.go` (new).

### Step 4.1 — `compound.go`

Append at the end of the file:

```go

// CreateCircleThreeTangents adds a circle tangent to the infinite lines through
// l1, l2 and l3, the counterpart of Fusion's three-tangent circle. Three lines
// that form a triangle have four such circles (the incircle and three
// excircles); the hint (hx, hy) picks the one whose region contains it: inside
// the triangle for the incircle, across one side for that side's excircle. A new
// center point and the circle are created, held by one [NewTangent] per line.
// The circle adds no degree of freedom: the lines determine it. It returns
// [ErrForeignEntity] if a line is not a live line of this sketch or has an
// endpoint that is not a live point of it, and [ErrInvalidShape] if two
// arguments are the same line, a line has zero length, two lines are parallel,
// hx or hy is not finite, the hint lies on one of the lines, or no tangent
// circle lies in the hint's region (a hint across a vertex, or three lines
// through one point) — in each case without modifying the sketch.
func (s *Sketch) CreateCircleThreeTangents(l1, l2, l3 *Line, hx, hy float64) (*Circle, error) {
	if s.foreignInput(l1, l2, l3) {
		return nil, ErrForeignEntity
	}
	if l1 == l2 || l1 == l3 || l2 == l3 {
		return nil, fmt.Errorf("%w: CreateCircleThreeTangents requires three distinct lines", ErrInvalidShape)
	}
	if !allFinite(hx, hy) {
		return nil, fmt.Errorf("%w: CreateCircleThreeTangents requires a finite hint", ErrInvalidShape)
	}
	lines := [3]*Line{l1, l2, l3}
	var m [3][3]float64 // rows: nx·cx + ny·cy − σ·ρ = d
	var rhs [3]float64
	for i, l := range lines {
		dx, dy := l.End.x()-l.Start.x(), l.End.y()-l.Start.y()
		dl := math.Hypot(dx, dy)
		if dl <= epsTool {
			return nil, fmt.Errorf("%w: CreateCircleThreeTangents requires lines of nonzero length", ErrInvalidShape)
		}
		nx, ny := -dy/dl, dx/dl
		d := nx*l.Start.x() + ny*l.Start.y()
		side := nx*hx + ny*hy - d
		if math.Abs(side) <= epsTool {
			return nil, fmt.Errorf("%w: CreateCircleThreeTangents requires a hint off every line", ErrInvalidShape)
		}
		sigma := 1.0
		if side < 0 {
			sigma = -1
		}
		m[i] = [3]float64{nx, ny, -sigma}
		rhs[i] = d
	}
	for i := 0; i < 3; i++ {
		for j := i + 1; j < 3; j++ {
			if math.Abs(m[i][0]*m[j][1]-m[i][1]*m[j][0]) <= epsTool {
				return nil, fmt.Errorf("%w: CreateCircleThreeTangents requires non-parallel lines", ErrInvalidShape)
			}
		}
	}
	det := det3(m)
	if math.Abs(det) <= epsTool {
		return nil, fmt.Errorf("%w: CreateCircleThreeTangents found no tangent circle", ErrInvalidShape)
	}
	var sol [3]float64
	for k := 0; k < 3; k++ {
		mk := m
		for i := 0; i < 3; i++ {
			mk[i][k] = rhs[i]
		}
		sol[k] = det3(mk) / det
	}
	if sol[2] <= epsTool {
		return nil, fmt.Errorf("%w: CreateCircleThreeTangents found no tangent circle in the hint's region", ErrInvalidShape)
	}
	c := s.CreateCircle(s.CreatePoint(sol[0], sol[1]), sol[2])
	s.AddConstraint(NewTangent(l1, c), NewTangent(l2, c), NewTangent(l3, c))
	return c, nil
}

// det3 returns the determinant of a 3×3 matrix.
func det3(m [3][3]float64) float64 {
	return m[0][0]*(m[1][1]*m[2][2]-m[1][2]*m[2][1]) -
		m[0][1]*(m[1][0]*m[2][2]-m[1][2]*m[2][0]) +
		m[0][2]*(m[1][0]*m[2][1]-m[1][1]*m[2][0])
}
```

### Step 4.2 — new file `circle_three_tangents_test.go`

```go
package sketch_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestCreateCircleThreeTangents(t *testing.T) {
	t.Run("incircle, fully constrained and parametric", func(t *testing.T) {
		s := newSketch(t)
		a, b, c := s.CreatePoint(0, 0), s.CreatePoint(12, 0), s.CreatePoint(0, 9)
		ab, bc, ca := s.CreateLine(a, b), s.CreateLine(b, c), s.CreateLine(c, a)
		base := sketch.NewDistance(a, b, 12)
		s.AddConstraint(
			sketch.NewCoincident(a, s.Origin()),
			sketch.NewHorizontal(ab),
			sketch.NewVertical(ca),
			base,
			sketch.NewDistance(a, c, 9),
		)
		require.Equal(t, 0, s.DOF(), "the triangle is fully constrained")

		circle, err := s.CreateCircleThreeTangents(ab, bc, ca, 2, 2)
		require.NoError(t, err)
		require.InDelta(t, 3, circle.Center.X(), 1e-9, "center x")
		require.InDelta(t, 3, circle.Center.Y(), 1e-9, "center y")
		require.InDelta(t, 3, circle.R(), 1e-9, "inradius (12 + 9 - 15) / 2")
		require.Equal(t, 0, s.DOF(), "the circle adds no freedom")

		res, err := s.Solve(t.Context())
		require.NoError(t, err)
		require.Equal(t, 0, res.DOF)
		require.Empty(t, s.RedundantConstraints())
		require.Nil(t, s.Verify(t.Context()).Check())
		require.Len(t, s.Profiles(), 2, "the disk and the triangle around it")

		base.Set(16)
		_, err = s.Solve(t.Context())
		require.NoError(t, err)
		want := (16 + 9 - math.Hypot(16, 9)) / 2 // 3.3212...
		require.InDelta(t, want, circle.R(), 1e-6, "inradius follows the triangle")
		require.InDelta(t, want, circle.Center.X(), 1e-6, "center x")
		require.InDelta(t, want, circle.Center.Y(), 1e-6, "center y")

		data, err := json.Marshal(s)
		require.NoError(t, err)
		var s2 sketch.Sketch
		require.NoError(t, json.Unmarshal(data, &s2))
		require.Len(t, s2.Entities(), len(s.Entities()))
		require.Len(t, s2.Constraints(), len(s.Constraints()))
		res2, err := s2.Solve(t.Context())
		require.NoError(t, err)
		require.Equal(t, 0, res2.DOF, "reloaded DOF")
	})

	t.Run("hint picks an excircle", func(t *testing.T) {
		s := newSketch(t)
		a, b, c := s.CreatePoint(0, 0), s.CreatePoint(12, 0), s.CreatePoint(0, 9)
		ab, bc, ca := s.CreateLine(a, b), s.CreateLine(b, c), s.CreateLine(c, a)
		circle, err := s.CreateCircleThreeTangents(ab, bc, ca, 10, 10)
		require.NoError(t, err)
		require.InDelta(t, 18, circle.Center.X(), 1e-9, "center x")
		require.InDelta(t, 18, circle.Center.Y(), 1e-9, "center y")
		require.InDelta(t, 18, circle.R(), 1e-9, "exradius across BC")
	})

	t.Run("refusals", func(t *testing.T) {
		s := newSketch(t)
		a, b, c := s.CreatePoint(0, 0), s.CreatePoint(12, 0), s.CreatePoint(0, 9)
		ab, bc, ca := s.CreateLine(a, b), s.CreateLine(b, c), s.CreateLine(c, a)
		top := s.CreateLine(s.CreatePoint(0, 20), s.CreatePoint(12, 20)) // parallel to ab
		q := s.CreatePoint(5, 5)
		z := s.CreateLine(q, q) // zero length
		// Three lines through (0, 0).
		d1 := s.CreateLine(s.CreatePoint(-5, -5), s.CreatePoint(5, 5))
		counts := func() [3]int { return [3]int{len(s.Points()), len(s.Entities()), len(s.Constraints())} }
		before := counts()
		fl := foreignLine(t, 0, 0, 12, 0)
		cases := []struct {
			name       string
			l1, l2, l3 *sketch.Line
			hx, hy     float64
			want       error
		}{
			{"foreign line", fl, bc, ca, 2, 2, sketch.ErrForeignEntity},
			{"nil line", ab, nil, ca, 2, 2, sketch.ErrForeignEntity},
			{"same line twice", ab, ab, ca, 2, 2, sketch.ErrInvalidShape},
			{"zero-length line", ab, z, ca, 2, 2, sketch.ErrInvalidShape},
			{"parallel pair", ab, top, ca, 2, 2, sketch.ErrInvalidShape},
			{"hint across a vertex", ab, bc, ca, -1, -1, sketch.ErrInvalidShape},
			{"hint on a line", ab, bc, ca, 5, 0, sketch.ErrInvalidShape},
			{"concurrent lines", ab, ca, d1, 3, 1, sketch.ErrInvalidShape},
			{"NaN hint", ab, bc, ca, math.NaN(), 2, sketch.ErrInvalidShape},
			{"Inf hint", ab, bc, ca, 2, math.Inf(1), sketch.ErrInvalidShape},
		}
		for _, tc := range cases {
			circle, err := s.CreateCircleThreeTangents(tc.l1, tc.l2, tc.l3, tc.hx, tc.hy)
			require.ErrorIs(t, err, tc.want, tc.name)
			require.Nil(t, circle, tc.name)
			require.Equal(t, before, counts(), "%s: the sketch is unchanged", tc.name)
		}
	})
}
```

### Step 4.3 — `revision_internal_test.go`

In `TestEntityUIDInvariant`, inside `t.Run("compound builders", ...)`, insert immediately above its last line
`requireStamped(t, s)`:

```go
		ta, tb, tc := s.CreatePoint(40, 60), s.CreatePoint(52, 60), s.CreatePoint(40, 69)
		_, err = s.CreateCircleThreeTangents(s.CreateLine(ta, tb), s.CreateLine(tb, tc), s.CreateLine(tc, ta), 42, 62)
		require.NoError(t, err)
```

### Step 4.4 — new file `examples/sketch_circle_three_tangents_example_test.go`

```go
package examples_test

import (
	"context"
	"fmt"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_circleThreeTangents inscribes a circle in a triangle, as
// Fusion's three-tangent circle does. The hint point picks the circle: inside
// the triangle gives the incircle, across a side gives that side's excircle.
// The circle adds no degree of freedom, so it follows the triangle when a side
// is re-dimensioned.
func Example_sketch_circleThreeTangents() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())

	a, b, c := s.CreatePoint(0, 0), s.CreatePoint(12, 0), s.CreatePoint(0, 9)
	ab, bc, ca := s.CreateLine(a, b), s.CreateLine(b, c), s.CreateLine(c, a)
	base := sketch.NewDistance(a, b, 12)
	s.AddConstraint(
		sketch.NewCoincident(a, s.Origin()),
		sketch.NewHorizontal(ab),
		sketch.NewVertical(ca),
		base,
		sketch.NewDistance(a, c, 9),
	)

	circle, err := s.CreateCircleThreeTangents(ab, bc, ca, 2, 2)
	if err != nil {
		fmt.Printf("failed to create circle: %s\n", err)
		return
	}
	report := func(label string) {
		res, err := s.Solve(context.Background())
		if err != nil {
			fmt.Printf("failed to solve: %s\n", err)
			return
		}
		fmt.Printf("%s: center (%.4f, %.4f), radius %.4f, DOF %d\n",
			label, circle.Center.X(), circle.Center.Y(), circle.R(), res.DOF)
	}
	report("base 12")
	base.Set(16)
	report("base 16")

	// Output:
	// base 12: center (3.0000, 3.0000), radius 3.0000, DOF 0
	// base 16: center (3.3212, 3.3212), radius 3.3212, DOF 0
}
```

### Step 4.5 — `CLAUDE.md`

Edit 1. Replace exactly:

```markdown
creation variants (`CreateArcThreePoints`/`CreateArcTangent`):
```

with:

```markdown
creation variants (`CreateArcThreePoints`/`CreateArcTangent`/`CreateCircleThreeTangents`):
```

Edit 2. Replace exactly:

```markdown
  `CreateArcThreePoints`/`CreateArcTangent` → `ErrInvalidShape`),
```

with:

```markdown
  `CreateArcThreePoints`/`CreateArcTangent`/`CreateCircleThreeTangents` →
  `ErrInvalidShape`),
```

### Step 4.6 — `docs/fusion-gap-analysis.md`

Replace exactly:

```markdown
  Builders: `CreateArcThreePoints`, `CreateArcTangent`.
```

with:

```markdown
  Builders: `CreateArcThreePoints`, `CreateArcTangent`,
  `CreateCircleThreeTangents`.
```

### Step 4.7 — verify

```sh
go test -count=1 -run 'TestCreateCircleThreeTangents|TestEntityUIDInvariant' .
go test -count=1 -run 'Example_sketch_circleThreeTangents$' -v ./examples
```

Then the common "Verify" commands.

### Step 4.8 — commit and PR

- **Commit message:** `add three-tangent circle builder`
- **Files to add:** `compound.go`, `circle_three_tangents_test.go`, `revision_internal_test.go`,
  `examples/sketch_circle_three_tangents_example_test.go`, `CLAUDE.md`, `docs/fusion-gap-analysis.md`
- **PR title:** `add three-tangent circle builder`
- **PR body:**

  ```markdown
  - Let callers place a circle tangent to three lines the way Fusion's three-tangent circle does
  - Pick the incircle or an excircle by hint point instead of by solver luck
  ```

## Increment 5 — `ArcSlot` and `CreateArcSlot`

- **Branch:** `feat-arc-slot`
- **Design section:** "`CreateArcSlot` — arc slot".
- **Requires:** increments 0 and 2 merged. Check increment 0 first: `grep -n sharedTangentAngle constraint.go`
  must print a match. If it prints nothing, stop and report.
- **Files:** `compound.go`, `revision_internal_test.go`, `CLAUDE.md`, `docs/fusion-gap-analysis.md`,
  `docs/creation-variants-design.md` (edits); `arc_slot_test.go`, `examples/sketch_arc_slot_example_test.go`
  (new).

### Step 5.1 — `compound.go`

Append at the end of the file:

```go

// ArcSlot groups the geometry created by [Sketch.CreateArcSlot].
type ArcSlot struct {
	Center       *Point  // center of the slot's centerline arc
	C1, C2       *Point  // cap centers: the centerline's start and end
	Outer, Inner *Arc    // concentric flanks about Center
	A1, A2       *Arc    // semicircular end caps about C1 and C2
	Spokes       []*Line // construction lines Center→C1 and Center→C2, for dimensioning
}

// CreateArcSlot builds an arc slot, the counterpart of Fusion's center-point
// arc slot: a slot of cap radius r (half the slot width) whose centerline is the
// arc about (cx, cy) running counter-clockwise from (x1, y1) toward the
// direction of (x2, y2). As with [Sketch.CreateArc], (x2, y2) sets only the end
// direction; the centerline radius is the distance from the center to
// (x1, y1). Swap the two points for the clockwise slot. Two concentric flank
// arcs about Center are joined by two semicircular caps about C1 and C2; each
// cap is held tangent to both flanks at the endpoints it shares with them
// ([NewTangentCircles]: internally to Outer, externally to Inner), which also
// keeps both caps the same radius and both cap centers on the centerline. The
// construction Spokes carry no constraint; they exist so the sweep can be
// dimensioned with [NewAngle] and the centerline radius with [NewDistance] on
// Center and C1. The slot keeps 6 degrees of freedom (center, centerline
// radius, start angle, sweep, cap radius). It returns [ErrInvalidShape] if an
// argument is not finite, (x1, y1) or (x2, y2) coincides with the center,
// r <= 0 or r is not less than the centerline radius, the sweep is a full turn,
// or the caps would overlap across the gap of a sweep above 180°.
func (s *Sketch) CreateArcSlot(cx, cy, x1, y1, x2, y2, r float64) (*ArcSlot, error) {
	if !allFinite(cx, cy, x1, y1, x2, y2, r) {
		return nil, fmt.Errorf("%w: CreateArcSlot requires finite arguments", ErrInvalidShape)
	}
	rc := math.Hypot(x1-cx, y1-cy)
	if rc <= epsTool || math.Hypot(x2-cx, y2-cy) <= epsTool {
		return nil, fmt.Errorf("%w: CreateArcSlot requires both points away from the center", ErrInvalidShape)
	}
	if r <= 0 || r >= rc {
		return nil, fmt.Errorf("%w: CreateArcSlot requires 0 < r < centerline radius", ErrInvalidShape)
	}
	th1 := math.Atan2(y1-cy, x1-cx)
	sweep := math.Mod(math.Atan2(y2-cy, x2-cx)-th1, 2*math.Pi)
	if sweep < 0 {
		sweep += 2 * math.Pi
	}
	if sweep <= epsTool || sweep >= 2*math.Pi-epsTool {
		return nil, fmt.Errorf("%w: CreateArcSlot requires a sweep below a full turn", ErrInvalidShape)
	}
	if sweep > math.Pi && rc*math.Sin(sweep/2) <= r {
		return nil, fmt.Errorf("%w: CreateArcSlot caps would overlap across the gap", ErrInvalidShape)
	}
	th2 := th1 + sweep
	at := func(th, rad float64) *Point { return s.CreatePoint(cx+rad*math.Cos(th), cy+rad*math.Sin(th)) }
	center := s.CreatePoint(cx, cy)
	c1, c2 := at(th1, rc), at(th2, rc)
	p1o, p1i := at(th1, rc+r), at(th1, rc-r)
	p2o, p2i := at(th2, rc+r), at(th2, rc-r)
	sl := &ArcSlot{
		Center: center, C1: c1, C2: c2,
		Outer: s.CreateArc(center, p1o, p2o),
		Inner: s.CreateArc(center, p1i, p2i),
		// Each cap's half-turn bulges away from the slot body.
		A1: s.CreateArc(c1, p1i, p1o),
		A2: s.CreateArc(c2, p2o, p2i),
	}
	for _, c := range []*Point{c1, c2} {
		sp := s.CreateLine(center, c)
		sp.SetConstruction(true)
		sl.Spokes = append(sl.Spokes, sp)
	}
	s.AddConstraint(
		NewTangentCircles(sl.A1, sl.Outer, true),
		NewTangentCircles(sl.A1, sl.Inner, false),
		NewTangentCircles(sl.A2, sl.Outer, true),
		NewTangentCircles(sl.A2, sl.Inner, false),
	)
	return sl, nil
}
```

### Step 5.2 — new file `arc_slot_test.go`

```go
package sketch_test

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

func TestCreateArcSlot(t *testing.T) {
	for _, sweep := range []float64{30, 90, 270} {
		t.Run(fmt.Sprintf("sweep %v", sweep), func(t *testing.T) {
			s := newSketch(t)
			rad := sweep * math.Pi / 180
			sl, err := s.CreateArcSlot(0, 0, 20, 0, 20*math.Cos(rad), 20*math.Sin(rad), 3)
			require.NoError(t, err)

			// Exact placement.
			require.InDelta(t, 20, sl.C1.X(), 1e-9, "C1 x")
			require.InDelta(t, 0, sl.C1.Y(), 1e-9, "C1 y")
			require.InDelta(t, 20*math.Cos(rad), sl.C2.X(), 1e-9, "C2 x")
			require.InDelta(t, 20*math.Sin(rad), sl.C2.Y(), 1e-9, "C2 y")
			require.InDelta(t, 23, sl.Outer.R(), 1e-9, "outer radius")
			require.InDelta(t, 17, sl.Inner.R(), 1e-9, "inner radius")
			require.InDelta(t, 3, sl.A1.R(), 1e-9, "cap 1 radius")
			require.InDelta(t, 3, sl.A2.R(), 1e-9, "cap 2 radius")
			require.InDelta(t, rad, sl.Outer.Sweep(), 1e-9, "outer sweep")
			require.InDelta(t, math.Pi, sl.A1.Sweep(), 1e-9, "cap 1 is a half turn")
			require.InDelta(t, math.Pi, sl.A2.Sweep(), 1e-9, "cap 2 is a half turn")
			require.Len(t, sl.Spokes, 2)
			require.True(t, sl.Spokes[0].IsConstruction())
			require.True(t, sl.Spokes[1].IsConstruction())
			require.Equal(t, 6, s.DOF(), "center, centerline radius, start angle, sweep, cap radius")
			require.Empty(t, s.RedundantConstraints())

			// Ground through the origin and dimension the five remaining freedoms.
			radius := sketch.NewDistance(sl.Center, sl.C1, 20)
			capR := sketch.NewRadius(sl.A1, 3)
			angle := sketch.NewAngle(sl.Spokes[0], sl.Spokes[1], sweep)
			s.AddConstraint(
				sketch.NewCoincident(sl.Center, s.Origin()),
				sketch.NewHorizontal(sl.Spokes[0]),
				angle, radius, capR,
			)
			res, err := s.Solve(t.Context())
			require.NoError(t, err)
			require.Equal(t, 0, res.DOF, "fully constrained")
			require.Empty(t, s.RedundantConstraints())
			require.Nil(t, s.Verify(t.Context()).Check())
			profiles := s.Profiles()
			require.Len(t, profiles, 1)
			require.InDelta(t, rad*20*6+math.Pi*9, profiles[0].Area, 1e-4, "sweep*R*2r + pi*r^2")

			// Parametric: radius, width and sweep all reflow.
			radius.Set(25)
			capR.Set(4)
			_, err = s.Solve(t.Context())
			require.NoError(t, err)
			require.InDelta(t, 25*math.Cos(rad), sl.C2.X(), 1e-6, "C2 x after R=25")
			require.InDelta(t, 25*math.Sin(rad), sl.C2.Y(), 1e-6, "C2 y after R=25")
			require.InDelta(t, 29, sl.Outer.R(), 1e-6, "outer radius")
			require.InDelta(t, 21, sl.Inner.R(), 1e-6, "inner radius")
			require.InDelta(t, 4, sl.A2.R(), 1e-6, "cap 2 follows cap 1")

			angle.Set(sweep + 20)
			_, err = s.Solve(t.Context())
			require.NoError(t, err)
			require.InDelta(t, rad+20*math.Pi/180, sl.Outer.Sweep(), 1e-6, "sweep follows the angle")

			data, err := json.Marshal(s)
			require.NoError(t, err)
			var s2 sketch.Sketch
			require.NoError(t, json.Unmarshal(data, &s2))
			require.Len(t, s2.Entities(), len(s.Entities()))
			require.Len(t, s2.Constraints(), len(s.Constraints()))
			res2, err := s2.Solve(t.Context())
			require.NoError(t, err)
			require.Equal(t, 0, res2.DOF, "reloaded DOF")
		})
	}

	t.Run("refusals", func(t *testing.T) {
		s := newSketch(t)
		c350 := 20 * math.Cos(350*math.Pi/180)
		s350 := 20 * math.Sin(350*math.Pi/180)
		cases := []struct {
			name                      string
			cx, cy, x1, y1, x2, y2, r float64
		}{
			{"start on the center", 0, 0, 0, 0, 0, 20, 3},
			{"end on the center", 0, 0, 20, 0, 0, 0, 3},
			{"zero cap radius", 0, 0, 20, 0, 0, 20, 0},
			{"negative cap radius", 0, 0, 20, 0, 0, 20, -1},
			{"cap radius equal to the centerline radius", 0, 0, 20, 0, 0, 20, 20},
			{"full turn", 0, 0, 20, 0, 30, 0, 3},
			{"caps overlap across the gap", 0, 0, 20, 0, c350, s350, 3},
			{"NaN", 0, 0, math.NaN(), 0, 0, 20, 3},
			{"Inf", 0, 0, 20, 0, 0, 20, math.Inf(1)},
		}
		for _, tc := range cases {
			sl, err := s.CreateArcSlot(tc.cx, tc.cy, tc.x1, tc.y1, tc.x2, tc.y2, tc.r)
			require.ErrorIs(t, err, sketch.ErrInvalidShape, tc.name)
			require.Nil(t, sl, tc.name)
			require.Empty(t, s.Points(), "%s: no point created", tc.name)
			require.Empty(t, s.Entities(), "%s: no entity created", tc.name)
			require.Empty(t, s.Constraints(), "%s: no constraint created", tc.name)
		}
	})
}
```

### Step 5.3 — `revision_internal_test.go`

In `TestEntityUIDInvariant`, inside `t.Run("compound builders", ...)`, insert immediately above its last line
`requireStamped(t, s)`:

```go
		_, err = s.CreateArcSlot(80, 80, 100, 80, 80, 100, 3)
		require.NoError(t, err)
```

### Step 5.4 — new file `examples/sketch_arc_slot_example_test.go`

```go
package examples_test

import (
	"context"
	"fmt"
	"math"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/geom"
)

// Example_sketch_arcSlot builds Fusion's center-point arc slot and then its
// three-point form. CreateArcSlot takes the centerline's center, its start and
// a point in its end direction, plus the cap radius (half the width). Five
// dimensions fully constrain it; the three-point form is geom.ArcThroughPoints
// feeding the same builder.
func Example_sketch_arcSlot() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())

	sl, err := s.CreateArcSlot(0, 0, 20, 0, 0, 20, 3) // 90° about the origin, width 6
	if err != nil {
		fmt.Printf("failed to create arc slot: %s\n", err)
		return
	}
	fmt.Printf("created: DOF %d\n", s.DOF())

	radius := sketch.NewDistance(sl.Center, sl.C1, 20)
	s.AddConstraint(
		sketch.NewCoincident(sl.Center, s.Origin()),
		sketch.NewHorizontal(sl.Spokes[0]),
		sketch.NewAngle(sl.Spokes[0], sl.Spokes[1], 90),
		radius,
		sketch.NewRadius(sl.A1, 3),
	)
	report := func(label string) {
		res, err := s.Solve(context.Background())
		if err != nil {
			fmt.Printf("failed to solve: %s\n", err)
			return
		}
		fmt.Printf("%s: outer %.1f, inner %.1f, area %.4f, DOF %d\n",
			label, sl.Outer.R(), sl.Inner.R(), s.Profiles()[0].Area, res.DOF)
	}
	report("R 20")
	radius.Set(25)
	report("R 25")

	// Three-point form: the centerline through (20, 0), (0, 20) and (-20, 0).
	s2, _ := w.CreateSketch(w.XY())
	g, ok := geom.ArcThroughPoints(geom.NewPoint(20, 0), geom.NewPoint(0, 20), geom.NewPoint(-20, 0))
	if !ok {
		fmt.Println("collinear centerline")
		return
	}
	sl2, err := s2.CreateArcSlot(g.Center.X, g.Center.Y, g.Start.X, g.Start.Y, g.End.X, g.End.Y, 3)
	if err != nil {
		fmt.Printf("failed to create arc slot: %s\n", err)
		return
	}
	fmt.Printf("three-point: center (%.1f, %.1f), sweep %.0f deg\n",
		sl2.Center.X(), sl2.Center.Y(), sl2.Outer.Sweep()*180/math.Pi)

	// Output:
	// created: DOF 6
	// R 20: outer 23.0, inner 17.0, area 216.7699, DOF 0
	// R 25: outer 28.0, inner 22.0, area 263.8938, DOF 0
	// three-point: center (0.0, 0.0), sweep 180 deg
}
```

### Step 5.5 — `CLAUDE.md`

Edit 1. Replace exactly:

```markdown
creation variants (`CreateArcThreePoints`/`CreateArcTangent`/`CreateCircleThreeTangents`):
```

with:

```markdown
creation variants (`CreateArcThreePoints`/`CreateArcTangent`/`CreateCircleThreeTangents`/`CreateArcSlot`):
```

Edit 2. Replace exactly:

```markdown
  `CreateArcThreePoints`/`CreateArcTangent`/`CreateCircleThreeTangents` →
  `ErrInvalidShape`),
```

with:

```markdown
  `CreateArcThreePoints`/`CreateArcTangent`/`CreateCircleThreeTangents`/
  `CreateArcSlot` → `ErrInvalidShape`),
```

### Step 5.6 — `docs/fusion-gap-analysis.md`

Edit 1. Replace exactly:

```markdown
  tangent constraint does not). Arc slot still open.
```

with:

```markdown
  tangent constraint does not). Arc slot: `CreateArcSlot` (see Creation variants).
```

Edit 2. Replace exactly:

```markdown
  `CreateCircleThreeTangents`.
```

with:

```markdown
  `CreateCircleThreeTangents`, `CreateArcSlot`.
```

### Step 5.7 — `docs/creation-variants-design.md`

Replace exactly:

```markdown
Status: **proposed**, decisions recorded.
```

with:

```markdown
Status: **implemented**.
```

### Step 5.8 — verify

```sh
go test -count=1 -run 'TestCreateArcSlot|TestEntityUIDInvariant|TestTangentCirclesSharedEndpoint' .
go test -count=1 -run 'Example_sketch_arcSlot$' -v ./examples
```

Then the common "Verify" commands.

### Step 5.9 — commit and PR

- **Commit message:** `add arc slot builder`
- **Files to add:** `compound.go`, `arc_slot_test.go`, `revision_internal_test.go`,
  `examples/sketch_arc_slot_example_test.go`, `CLAUDE.md`, `docs/fusion-gap-analysis.md`,
  `docs/creation-variants-design.md`
- **PR title:** `add arc slot builder`
- **PR body:**

  ```markdown
  - Let callers build Fusion's center-point and three-point arc slots in one call
  - Close the last open slot variant in the Fusion gap analysis
  ```
