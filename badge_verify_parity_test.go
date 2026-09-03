package sketch_test

import (
	"fmt"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

// TestStatusBadgeMatchesVerify pins the status badge's own cheaper computation
// (writeStatusBadge renders badgeVerify, not a full Sketch.Verify) against the
// full report on every Status the badge can display: under-constrained, fully
// constrained, over-constrained by a conflict, over-constrained by a redundant
// but consistent constraint (Status alone separates it from the conflicting
// case), and a sketch whose profiles are invalid (which Verify still analyses
// fully — profile validity does not gate the DOF/Status/Solvable pass the badge
// reads). For every fixture the rendered card must carry the exact
// "DOF %d · %s · solvable=%t" text a full [sketch.Sketch.Verify] call would
// produce. The skipped-analysis card, which neither path computes a verdict
// for, is pinned separately by TestStatusBadgeSkippedAnalysis.
func TestStatusBadgeMatchesVerify(t *testing.T) {
	tests := []struct {
		name  string
		build func(t *testing.T) *sketch.Sketch
	}{
		{"under-constrained", func(t *testing.T) *sketch.Sketch {
			s := newSketch(t)
			a := s.CreatePoint(0, 0)
			b := s.CreatePoint(20, 0)
			s.CreateLine(a, b)
			s.AddConstraint(sketch.NewCoincident(a, s.Origin()))
			return s
		}},
		{"fully constrained", func(t *testing.T) *sketch.Sketch {
			s := newSketch(t)
			a := s.CreatePoint(0, 0)
			b := s.CreatePoint(20, 0)
			c := s.CreatePoint(20, 12)
			d := s.CreatePoint(0, 12)
			ab := s.CreateLine(a, b)
			bc := s.CreateLine(b, c)
			cd := s.CreateLine(c, d)
			da := s.CreateLine(d, a)
			s.AddConstraint(
				sketch.NewCoincident(a, s.Origin()),
				sketch.NewHorizontal(ab),
				sketch.NewHorizontal(cd),
				sketch.NewVertical(bc),
				sketch.NewVertical(da),
				sketch.NewDistance(a, b, 20),
				sketch.NewDistance(a, d, 12),
			)
			_, err := s.Solve(t.Context())
			require.NoError(t, err)
			return s
		}},
		{"over-constrained conflicting", func(t *testing.T) *sketch.Sketch {
			s := newSketch(t)
			a := s.CreatePoint(0, 0)
			b := s.CreatePoint(20, 0)
			s.CreateLine(a, b)
			s.AddConstraint(sketch.NewCoincident(a, s.Origin()))
			s.AddConstraint(sketch.NewDistance(a, b, 20))
			s.AddConstraint(sketch.NewDistance(a, b, 30))
			_, _ = s.Solve(t.Context())
			return s
		}},
		{"over-constrained redundant", func(t *testing.T) *sketch.Sketch {
			// A duplicated but CONSISTENT dimension: solvable and DOF 0, so only
			// the redundancy makes it over-constrained. It separates the two
			// classifyStatus branches the conflicting fixture cannot.
			s := newSketch(t)
			a := s.CreatePoint(0, 0)
			b := s.CreatePoint(20, 0)
			ab := s.CreateLine(a, b)
			s.AddConstraint(
				sketch.NewCoincident(a, s.Origin()),
				sketch.NewHorizontal(ab),
				sketch.NewDistance(a, b, 20),
				sketch.NewDistance(a, b, 20),
			)
			_, err := s.Solve(t.Context())
			require.NoError(t, err)
			return s
		}},
		{"invalid profiles", func(t *testing.T) *sketch.Sketch {
			// A control polygon whose periodic closed-spline loop crosses
			// itself: Verify still runs its full analysis (DOF/Status/Solvable
			// are unaffected), it just also reports ProfilesValid=false, which
			// the badge does not read.
			s := newSketch(t)
			a := s.CreatePoint(0, 0)
			b := s.CreatePoint(4, 3)
			c := s.CreatePoint(0, 3)
			d := s.CreatePoint(4, 0)
			_, err := s.CreateClosedSpline(a, b, c, d)
			require.NoError(t, err)
			return s
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.build(t)

			rep := s.Verify(t.Context())
			require.True(t, rep.Analysed(), "fixture must reach the analysed path")
			want := fmt.Sprintf("DOF %d · %s · solvable=%t", rep.DOF, rep.Status, rep.Solvable)

			out, err := s.SVG(sketch.WithStatusBadge(true))
			require.NoError(t, err)
			require.Contains(t, out, want)
		})
	}
}
