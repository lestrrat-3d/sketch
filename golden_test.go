package sketch_test

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// This file captures the CURRENT, untouched behaviour of the engine as a
// byte/value baseline (.tmp/perf-plan.md section 1.3), so later performance
// work can prove it changed nothing observable. Regeneration is gated on
// SKETCH_UPDATE_GOLDEN=1; a normal run compares against the committed files
// under testdata/golden/. Floats are compared with tolerance, not bit-exact,
// because CI's arm64 job fuses FMA and the last bit legitimately differs from
// the amd64 machine that generated the files; exporter text compares
// byte-exact.

const updateGoldenEnv = "SKETCH_UPDATE_GOLDEN"

func goldenUpdate() bool { return os.Getenv(updateGoldenEnv) == "1" }

func goldenPath(name string) string { return filepath.Join("testdata", "golden", name) }

// compareOrUpdateText writes got as the golden when SKETCH_UPDATE_GOLDEN=1,
// otherwise compares it byte-exact against the committed file.
func compareOrUpdateText(t *testing.T, name, got string) {
	t.Helper()
	path := goldenPath(name)
	if goldenUpdate() {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "golden %s missing; run with SKETCH_UPDATE_GOLDEN=1", name)
	require.Equal(t, string(want), got, "golden %s changed", name)
}

// loadOrStoreJSON writes actual as the golden (as JSON) when
// SKETCH_UPDATE_GOLDEN=1 and returns it unchanged, so the caller's assertions
// still run (trivially) on that path; otherwise it loads and returns the
// committed golden for the caller to compare actual against.
func loadOrStoreJSON[T any](t *testing.T, name string, actual T) T {
	t.Helper()
	path := goldenPath(name)
	if goldenUpdate() {
		data, err := json.MarshalIndent(actual, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, data, 0o644))
		return actual
	}
	data, err := os.ReadFile(path)
	require.NoError(t, err, "golden %s missing; run with SKETCH_UPDATE_GOLDEN=1", name)
	var want T
	require.NoError(t, json.Unmarshal(data, &want))
	return want
}

// requireRelClose asserts want and got agree to a relative tolerance rel,
// scaled by the larger magnitude (both zero always passes).
func requireRelClose(t *testing.T, want, got, rel float64, msg string) {
	t.Helper()
	if want == 0 && got == 0 {
		return
	}
	scale := math.Max(math.Abs(want), math.Abs(got))
	require.InDelta(t, want, got, rel*scale, msg)
}

// solveFixtures lists the six TestGoldenSolve/TestGoldenVerify fixtures: small,
// large, hexagon-style, an over-constrained conflicting one, one with a driven
// dimension, and one with an arc-tangency auxiliary variable.
func solveFixtures() []struct {
	name  string
	build func(testing.TB) *sketch.Sketch
} {
	return []struct {
		name  string
		build func(testing.TB) *sketch.Sketch
	}{
		{"small", func(tb testing.TB) *sketch.Sketch { s, _ := buildSmallFixture(tb); return s }},
		{"large", func(tb testing.TB) *sketch.Sketch { s, _ := buildLargeFixture(tb); return s }},
		{"hexagon", buildHexagonFixture},
		{"conflict", buildConflictFixture},
		{"drivenDim", buildDrivenDimFixture},
		{"tangentAux", buildTangentAuxFixture},
	}
}

// --- TestGoldenExport --------------------------------------------------

// TestGoldenExport pins the exporters' byte output for the gallery, 12-circle
// and spline fixtures.
func TestGoldenExport(t *testing.T) {
	gallery := buildGalleryFixture(t)
	plain, err := gallery.SVG()
	require.NoError(t, err)
	compareOrUpdateText(t, "gallery.svg", plain)

	annotated, err := gallery.SVG(
		sketch.WithDimensions(true), sketch.WithConstraints(true),
		sketch.WithShowPoints(true), sketch.WithStatusBadge(true),
	)
	require.NoError(t, err)
	compareOrUpdateText(t, "gallery_annotated.svg", annotated)

	dxfLocal, err := gallery.DXF()
	require.NoError(t, err)
	compareOrUpdateText(t, "gallery.dxf", dxfLocal)

	dxfWorld, err := gallery.DXF(sketch.WithWorldSpace(true))
	require.NoError(t, err)
	compareOrUpdateText(t, "gallery_world.dxf", dxfWorld)

	circles := buildCircles12Fixture(t)
	circlesSVG, err := circles.SVG()
	require.NoError(t, err)
	compareOrUpdateText(t, "circles12.svg", circlesSVG)

	splines := buildSplinesFixture(t)
	splinesSVG, err := splines.SVG()
	require.NoError(t, err)
	compareOrUpdateText(t, "splines.svg", splinesSVG)
}

// --- TestGoldenSolve -----------------------------------------------------

type pointXY struct{ X, Y float64 }

type solveSnapshot struct {
	Points     []pointXY
	Converged  bool
	Iterations int
	Residual   float64
	DOF        int
	Redundant  int
}

func snapshotSolve(s *sketch.Sketch, res *sketch.Result) solveSnapshot {
	pts := s.Points()
	out := solveSnapshot{
		Points:     make([]pointXY, len(pts)),
		Converged:  res.Converged,
		Iterations: res.Iterations,
		Residual:   res.Residual,
		DOF:        res.DOF,
		Redundant:  res.Redundant,
	}
	for i, p := range pts {
		out.Points[i] = pointXY{p.X(), p.Y()}
	}
	return out
}

// TestGoldenSolve pins Solve's outcome (every point's coordinates, Result) for
// the six fixtures in solveFixtures.
func TestGoldenSolve(t *testing.T) {
	for _, f := range solveFixtures() {
		t.Run(f.name, func(t *testing.T) {
			s := f.build(t)
			res, err := s.Solve(t.Context())
			if err != nil {
				require.ErrorIs(t, err, sketch.ErrNotConverged, "the only expected Solve failure is non-convergence")
			}
			got := snapshotSolve(s, res)
			want := loadOrStoreJSON(t, "solve_"+f.name+".json", got)

			require.Equal(t, want.Converged, got.Converged)
			require.Equal(t, want.Iterations, got.Iterations)
			require.Equal(t, want.DOF, got.DOF)
			require.Equal(t, want.Redundant, got.Redundant)
			require.InDelta(t, want.Residual, got.Residual, 1e-9, "Residual")
			require.Equal(t, len(want.Points), len(got.Points), "point count")
			for i := range want.Points {
				require.InDelta(t, want.Points[i].X, got.Points[i].X, 1e-9, "point %d X", i)
				require.InDelta(t, want.Points[i].Y, got.Points[i].Y, 1e-9, "point %d Y", i)
			}
		})
	}
}

// --- TestGoldenVerify ------------------------------------------------------

// constraintIndex returns c's position in s.Constraints(), or -1 if it is not
// found (e.g. an internal constraint never surfaced there).
func constraintIndex(s *sketch.Sketch, c sketch.Constraint) int {
	for i, cc := range s.Constraints() {
		if cc == c {
			return i
		}
	}
	return -1
}

type conflictSnapshot struct {
	ConstraintIdx int
	WithIdx       []int
}

type verifySnapshot struct {
	DOF             int
	Status          int
	Solvable        bool
	Residual        float64
	RankMarginInf   bool
	RankMargin      float64
	ConditioningInf bool
	Conditioning    float64
	FreePointIDs    []int
	RedundantCount  int
	Conflicts       []conflictSnapshot
	ProfilesValid   bool
	ProfileCount    int
}

func snapshotVerify(s *sketch.Sketch, rep *sketch.VerificationReport) verifySnapshot {
	out := verifySnapshot{
		DOF:            rep.DOF,
		Status:         int(rep.Status),
		Solvable:       rep.Solvable,
		Residual:       rep.Residual,
		RedundantCount: len(rep.Redundant),
		ProfilesValid:  rep.ProfilesValid,
		ProfileCount:   len(rep.Profiles),
	}
	if math.IsInf(rep.RankMargin, 1) {
		out.RankMarginInf = true
	} else {
		out.RankMargin = rep.RankMargin
	}
	if math.IsInf(rep.Conditioning, 1) {
		out.ConditioningInf = true
	} else {
		out.Conditioning = rep.Conditioning
	}
	for _, p := range rep.FreePoints {
		out.FreePointIDs = append(out.FreePointIDs, p.ID())
	}
	for _, cs := range rep.Conflicts {
		cf := conflictSnapshot{ConstraintIdx: constraintIndex(s, cs.Constraint)}
		for _, w := range cs.With {
			cf.WithIdx = append(cf.WithIdx, constraintIndex(s, w))
		}
		out.Conflicts = append(out.Conflicts, cf)
	}
	return out
}

// TestGoldenVerify pins Verify's report for the same six fixtures TestGoldenSolve
// uses, after the same Solve call.
func TestGoldenVerify(t *testing.T) {
	for _, f := range solveFixtures() {
		t.Run(f.name, func(t *testing.T) {
			s := f.build(t)
			_, err := s.Solve(t.Context())
			if err != nil {
				require.ErrorIs(t, err, sketch.ErrNotConverged)
			}
			rep := s.Verify(t.Context())
			require.True(t, rep.Analysed(), "fixtures carry no non-finite/foreign geometry")
			got := snapshotVerify(s, rep)
			want := loadOrStoreJSON(t, "verify_"+f.name+".json", got)

			require.Equal(t, want.DOF, got.DOF)
			require.Equal(t, want.Status, got.Status)
			require.Equal(t, want.Solvable, got.Solvable)
			require.InDelta(t, want.Residual, got.Residual, 1e-9, "Residual")
			require.Equal(t, want.RankMarginInf, got.RankMarginInf)
			if !want.RankMarginInf {
				requireRelClose(t, want.RankMargin, got.RankMargin, 1e-8, "RankMargin")
			}
			require.Equal(t, want.ConditioningInf, got.ConditioningInf)
			if !want.ConditioningInf {
				requireRelClose(t, want.Conditioning, got.Conditioning, 1e-8, "Conditioning")
			}
			require.Equal(t, want.FreePointIDs, got.FreePointIDs)
			require.Equal(t, want.RedundantCount, got.RedundantCount)
			require.Equal(t, want.Conflicts, got.Conflicts)
			require.Equal(t, want.ProfilesValid, got.ProfilesValid)
			require.Equal(t, want.ProfileCount, got.ProfileCount)
		})
	}
}

// --- TestGoldenProfiles ----------------------------------------------------

// entityIndex returns e's position in s.Entities(), which — per CLAUDE.md's
// serialization invariant — is also its id.
func entityIndex(s *sketch.Sketch, e sketch.Entity) int {
	for i, ee := range s.Entities() {
		if ee == e {
			return i
		}
	}
	return -1
}

type edgeSnapshot struct {
	EntityIdx int
	Partial   bool
	Reversed  bool
	TStart    float64
	TEnd      float64
	TExact    bool
}

type regionSnapshot struct {
	Area             float64
	Valid            bool
	SelfIntersecting bool
	Outer            []edgeSnapshot
	Holes            [][]edgeSnapshot
}

func snapshotEdges(s *sketch.Sketch, edges []sketch.BoundaryEdge) []edgeSnapshot {
	out := make([]edgeSnapshot, len(edges))
	for i, e := range edges {
		out[i] = edgeSnapshot{
			EntityIdx: entityIndex(s, e.Entity),
			Partial:   e.Partial,
			Reversed:  e.Reversed,
			TStart:    e.TStart,
			TEnd:      e.TEnd,
			TExact:    e.TExact,
		}
	}
	return out
}

func snapshotProfiles(s *sketch.Sketch, profiles []*sketch.Profile) []regionSnapshot {
	out := make([]regionSnapshot, len(profiles))
	for i, p := range profiles {
		rs := regionSnapshot{
			Area:             p.Area,
			Valid:            p.Valid,
			SelfIntersecting: p.SelfIntersecting,
			Outer:            snapshotEdges(s, p.Outer),
		}
		for _, h := range p.Holes {
			rs.Holes = append(rs.Holes, snapshotEdges(s, h))
		}
		out[i] = rs
	}
	return out
}

// profilesFixtures covers a representative subset of the section-4.2 fixture
// family that has a natural Sketch-level builder (the arrangement's own full
// fixture table lives in geom/arrange_golden_test.go's TestGoldenArrangement,
// which owns the near-miss/tangency/self-crossing/etc. cases below the
// sketch's Curve/ClosedCurve seam).
func profilesFixtures() []struct {
	name  string
	build func(testing.TB) *sketch.Sketch
} {
	return []struct {
		name  string
		build func(testing.TB) *sketch.Sketch
	}{
		{"small", func(tb testing.TB) *sketch.Sketch { s, _ := buildSmallFixture(tb); return s }},
		{"hexagon", buildHexagonFixture},
		{"circles12", buildCircles12Fixture},
		{"splines", buildSplinesFixture},
	}
}

// TestGoldenProfiles pins Sketch.Profiles' region/edge structure for a
// representative subset of the section-4.2 fixtures.
func TestGoldenProfiles(t *testing.T) {
	for _, f := range profilesFixtures() {
		t.Run(f.name, func(t *testing.T) {
			s := f.build(t)
			profiles := s.Profiles()
			got := snapshotProfiles(s, profiles)
			want := loadOrStoreJSON(t, "profiles_"+f.name+".json", got)

			require.Equal(t, len(want), len(got), "region count")
			for i := range want {
				require.Equal(t, want[i].Valid, got[i].Valid, "region %d Valid", i)
				require.Equal(t, want[i].SelfIntersecting, got[i].SelfIntersecting, "region %d SelfIntersecting", i)
				require.InDelta(t, want[i].Area, got[i].Area, 1e-12, "region %d Area", i)
				requireEdgesEqual(t, want[i].Outer, got[i].Outer, i, "Outer")
				require.Equal(t, len(want[i].Holes), len(got[i].Holes), "region %d hole count", i)
				for j := range want[i].Holes {
					requireEdgesEqual(t, want[i].Holes[j], got[i].Holes[j], i, "Holes")
				}
			}
		})
	}
}

func requireEdgesEqual(t *testing.T, want, got []edgeSnapshot, region int, label string) {
	t.Helper()
	require.Equal(t, len(want), len(got), "region %d %s edge count", region, label)
	for k := range want {
		require.Equal(t, want[k].EntityIdx, got[k].EntityIdx, "region %d %s edge %d EntityIdx", region, label, k)
		require.Equal(t, want[k].Partial, got[k].Partial, "region %d %s edge %d Partial", region, label, k)
		require.Equal(t, want[k].Reversed, got[k].Reversed, "region %d %s edge %d Reversed", region, label, k)
		require.Equal(t, want[k].TExact, got[k].TExact, "region %d %s edge %d TExact", region, label, k)
		require.InDelta(t, want[k].TStart, got[k].TStart, 1e-12, "region %d %s edge %d TStart", region, label, k)
		require.InDelta(t, want[k].TEnd, got[k].TEnd, 1e-12, "region %d %s edge %d TEnd", region, label, k)
	}
}
