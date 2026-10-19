package sketch_test

import (
	"context"
	"math"
	"math/big"
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/stretchr/testify/require"
)

type quarterJunctionTestWork struct {
	cancelErr func() error
	used      int
	limit     int
	onStep    func(sketch.JunctionCharge)
	decline   sketch.JunctionDecline
}

func (w *quarterJunctionTestWork) Step(kind sketch.JunctionCharge) error {
	if err := w.cancelErr(); err != nil {
		return err
	}
	w.used++
	if w.used > w.limit {
		return sketch.ErrJunctionWorkLimit
	}
	if w.onStep != nil {
		w.onStep(kind)
	}
	return nil
}

func (w *quarterJunctionTestWork) Err() error { return w.cancelErr() }

func (w *quarterJunctionTestWork) JunctionDeclined(d sketch.JunctionDecline) {
	w.decline = d
}

func quarterJunctionRealProfiles(t *testing.T) (*sketch.Sketch, []*sketch.Profile) {
	t.Helper()
	s := newSketch(t)
	center := s.CreatePoint(0, 0)
	s.Fix(center)
	right := s.CreatePoint(5, 0)
	top := s.CreatePoint(0, 5)
	s.CreateLine(center, right)
	s.CreateArc(center, right, top)
	s.CreateLine(top, center)
	rect := s.CreateRectangle(2, 1, 6, 3)
	s.Fix(rect.A)
	_, err := s.Solve(t.Context())
	require.NoError(t, err)

	profiles := s.Profiles()
	require.NotEmpty(t, profiles)
	return s, profiles
}

func TestQuarterArcJunctionRealProfiles(t *testing.T) {
	s, profiles := quarterJunctionRealProfiles(t)
	work := &quarterJunctionTestWork{cancelErr: t.Context().Err, limit: 4096}
	junctions, ok, err := s.CertifyQuarterArcJunctions(t.Context(), profiles, work)
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, junctions, 2)
	arcFirst, lineFirst := 0, 0
	for index, junction := range junctions {
		sources := junction.Sources()
		require.NotEmpty(t, junction.Occurrences())
		if sources[0].Kind == sketch.JunctionArc {
			arcFirst++
		} else {
			lineFirst++
		}
		xBracket, yBracket := junction.Point()
		xLo, xHi := xBracket.Bounds()
		yLo, yHi := yBracket.Bounds()
		require.NotNil(t, xLo)
		require.NotNil(t, xHi)
		require.NotNil(t, yLo)
		require.NotNil(t, yHi)
		var y float64
		switch {
		case yLo.Cmp(yHi) == 0 && yLo.Sign() > 0:
			y, _ = yLo.Float64()
		default:
			lower, _ := yLo.Float64()
			upper, _ := yHi.Float64()
			y = (lower + upper) / 2
		}
		require.True(t, math.Abs(y-1) < 1e-12 || math.Abs(y-3) < 1e-12,
			"unexpected crossing y=%g", y)
		wantSquare := big.NewRat(24, 1)
		if math.Abs(y-3) < 1e-12 {
			wantSquare = big.NewRat(16, 1)
		}
		require.LessOrEqual(t, new(big.Rat).Mul(xLo, xLo).Cmp(wantSquare), 0)
		require.GreaterOrEqual(t, new(big.Rat).Mul(xHi, xHi).Cmp(wantSquare), 0)
		left, right := junction.Parameters()
		for i, param := range [2]sketch.RationalBracket{left, right} {
			lo, hi := param.Bounds()
			require.NotNil(t, lo)
			require.NotNil(t, hi)
			l, _ := lo.Float64()
			h, _ := hi.Float64()
			var want float64
			switch sources[i].Kind {
			case sketch.JunctionLine:
				want = (math.Sqrt(25-y*y) - 2) / 4
			case sketch.JunctionArc:
				want = math.Asin(y/5) / (math.Pi / 2)
			default:
				t.Fatalf("junction %d source %d kind=%d", index, i, sources[i].Kind)
			}
			require.InDelta(t, want, (l+h)/2, 1e-12,
				"junction %d source %d parameter", index, i)
		}
	}
	require.Equal(t, 1, arcFirst)
	require.Equal(t, 1, lineFirst)
}

func TestQuarterArcJunctionRejectsSketchMutationDuringProof(t *testing.T) {
	s, profiles := quarterJunctionRealProfiles(t)
	changed := false
	work := &quarterJunctionTestWork{cancelErr: t.Context().Err, limit: 4096}
	work.onStep = func(kind sketch.JunctionCharge) {
		if kind != sketch.JunctionRootProof || changed {
			return
		}
		changed = true
		s.CreateLine(s.CreatePoint(20, 20), s.CreatePoint(21, 21))
	}
	junctions, ok, err := s.CertifyQuarterArcJunctions(t.Context(), profiles, work)
	require.True(t, changed)
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, junctions)
	require.LessOrEqual(t, work.used, work.limit)
	require.Equal(t, "publication.stale", work.decline.Predicate)
}

func TestQuarterArcJunctionRejectsFinalRevisionCallbackMutation(t *testing.T) {
	s, profiles := quarterJunctionRealProfiles(t)
	baseline := &quarterJunctionTestWork{cancelErr: t.Context().Err, limit: 4096}
	_, ok, err := s.CertifyQuarterArcJunctions(t.Context(), profiles, baseline)
	require.NoError(t, err)
	require.True(t, ok)

	first := s.Entities()[0].(*sketch.Line)
	changed := false
	work := &quarterJunctionTestWork{cancelErr: t.Context().Err, limit: 4096}
	work.onStep = func(sketch.JunctionCharge) {
		if work.used == baseline.used {
			changed = true
			s.CreateLine(first.Start, first.End)
		}
	}
	junctions, ok, err := s.CertifyQuarterArcJunctions(t.Context(), profiles, work)
	require.True(t, changed)
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, junctions)
	require.Equal(t, "publication.stale", work.decline.Predicate)
}

func TestQuarterArcJunctionRejectsFinalRevisionSameCountMutation(t *testing.T) {
	s, profiles := quarterJunctionRealProfiles(t)
	baseline := &quarterJunctionTestWork{cancelErr: t.Context().Err, limit: 4096}
	_, ok, err := s.CertifyQuarterArcJunctions(t.Context(), profiles, baseline)
	require.NoError(t, err)
	require.True(t, ok)

	first := s.Entities()[0].(*sketch.Line)
	changed := false
	work := &quarterJunctionTestWork{cancelErr: t.Context().Err, limit: 4096}
	work.onStep = func(sketch.JunctionCharge) {
		if work.used == baseline.used {
			changed = true
			require.True(t, s.RemoveEntity(first))
			s.CreateLine(first.Start, first.End)
		}
	}
	junctions, ok, err := s.CertifyQuarterArcJunctions(t.Context(), profiles, work)
	require.True(t, changed)
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, junctions)
	require.Equal(t, "publication.stale", work.decline.Predicate)
}

func TestQuarterArcJunctionChecksFinalRevisionCancellation(t *testing.T) {
	s, profiles := quarterJunctionRealProfiles(t)
	baseline := &quarterJunctionTestWork{cancelErr: t.Context().Err, limit: 4096}
	_, ok, err := s.CertifyQuarterArcJunctions(t.Context(), profiles, baseline)
	require.NoError(t, err)
	require.True(t, ok)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	canceled := false
	work := &quarterJunctionTestWork{cancelErr: ctx.Err, limit: 4096}
	work.onStep = func(sketch.JunctionCharge) {
		if work.used == baseline.used {
			canceled = true
			cancel()
		}
	}
	junctions, ok, err := s.CertifyQuarterArcJunctions(ctx, profiles, work)
	require.True(t, canceled)
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, ok)
	require.Empty(t, junctions)
}

func TestQuarterArcJunctionChargesRevisionScan(t *testing.T) {
	s, profiles := quarterJunctionRealProfiles(t)
	work := &quarterJunctionTestWork{cancelErr: t.Context().Err, limit: 10}
	junctions, ok, err := s.CertifyQuarterArcJunctions(t.Context(), profiles, work)
	require.ErrorIs(t, err, sketch.ErrJunctionWorkLimit)
	require.False(t, ok)
	require.Empty(t, junctions)
	require.Equal(t, work.limit+1, work.used)
}

func TestQuarterArcJunctionRefusesUnchargedNURBSRevision(t *testing.T) {
	s, _ := quarterJunctionRealProfiles(t)
	control := []*sketch.Point{
		s.CreatePoint(20, 20), s.CreatePoint(21, 22), s.CreatePoint(22, 20),
	}
	_, err := s.CreateNURBS(2, control, nil, sketch.ClampedUniformKnots(3, 2))
	require.NoError(t, err)
	profiles := s.Profiles()
	require.NotEmpty(t, profiles)
	work := &quarterJunctionTestWork{cancelErr: t.Context().Err, limit: 4096}
	junctions, ok, err := s.CertifyQuarterArcJunctions(t.Context(), profiles, work)
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, junctions)
	require.Equal(t, "sketch.nurbs", work.decline.Predicate)
	require.LessOrEqual(t, work.used, work.limit)
}

func TestQuarterArcJunctionRefusesNURBSAddedByWorkCallback(t *testing.T) {
	s, profiles := quarterJunctionRealProfiles(t)
	added := false
	work := &quarterJunctionTestWork{cancelErr: t.Context().Err, limit: 4096}
	work.onStep = func(sketch.JunctionCharge) {
		if added {
			return
		}
		added = true
		control := []*sketch.Point{
			s.CreatePoint(20, 20), s.CreatePoint(21, 22), s.CreatePoint(22, 20),
		}
		_, err := s.CreateNURBS(2, control, nil, sketch.ClampedUniformKnots(3, 2))
		require.NoError(t, err)
	}
	junctions, ok, err := s.CertifyQuarterArcJunctions(t.Context(), profiles, work)
	require.True(t, added)
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, junctions)
	require.Equal(t, "sketch.nurbs", work.decline.Predicate)
}

func TestQuarterArcJunctionEmptyForUnsupportedArc(t *testing.T) {
	s := newSketch(t)
	center := s.CreatePoint(0, 0)
	right := s.CreatePoint(5, 0)
	left := s.CreatePoint(-5, 0)
	s.CreateArc(center, right, left)
	s.CreateLine(left, right)
	profiles := s.Profiles()
	require.Len(t, profiles, 1)
	work := &quarterJunctionTestWork{cancelErr: t.Context().Err, limit: 4096}
	junctions, ok, err := s.CertifyQuarterArcJunctions(t.Context(), profiles, work)
	require.NoError(t, err)
	require.True(t, ok)
	require.Empty(t, junctions)
	require.Positive(t, work.used)
}

func TestQuarterArcJunctionNoEndpointTangentToken(t *testing.T) {
	s := newSketch(t)
	center := s.CreatePoint(0, 0)
	right := s.CreatePoint(5, 0)
	top := s.CreatePoint(0, 5)
	s.CreateLine(center, right)
	s.CreateArc(center, right, top)
	s.CreateLine(top, center)
	s.CreateLine(s.CreatePoint(-1, 5), s.CreatePoint(1, 5))
	profiles := s.Profiles()
	require.NotEmpty(t, profiles)
	work := &quarterJunctionTestWork{cancelErr: t.Context().Err, limit: 4096}
	junctions, _, err := s.CertifyQuarterArcJunctions(t.Context(), profiles, work)
	require.NoError(t, err)
	require.Empty(t, junctions)
}

func TestQuarterArcJunctionRejectsMixedPublication(t *testing.T) {
	s, profiles := quarterJunctionRealProfiles(t)
	fresh := s.Profiles()
	require.Len(t, fresh, len(profiles))
	profiles[1] = fresh[1]
	work := &quarterJunctionTestWork{cancelErr: t.Context().Err, limit: 4096}
	junctions, ok, err := s.CertifyQuarterArcJunctions(t.Context(), profiles, work)
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, junctions)
	require.Equal(t, "profile.publication", work.decline.Predicate)
}

func TestQuarterArcJunctionRejectsForeignEdge(t *testing.T) {
	s, profiles := quarterJunctionRealProfiles(t)
	foreign := newSketch(t)
	profiles[0].Outer[0].Entity = foreign.CreateLine(
		foreign.CreatePoint(0, 0), foreign.CreatePoint(1, 0))
	work := &quarterJunctionTestWork{cancelErr: t.Context().Err, limit: 4096}
	junctions, ok, err := s.CertifyQuarterArcJunctions(t.Context(), profiles, work)
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, junctions)
	require.Equal(t, "edge.source_entity", work.decline.Predicate)
}

func TestQuarterArcJunctionPreflightsRawLimits(t *testing.T) {
	for _, tc := range []struct {
		name, predicate string
		mutate          func([]*sketch.Profile)
	}{
		{
			name: "holes", predicate: "profile.hole_limit",
			mutate: func(profiles []*sketch.Profile) {
				profiles[0].Holes = make([][]sketch.BoundaryEdge, 128)
			},
		},
		{
			name: "edges", predicate: "loop.edge_limit",
			mutate: func(profiles []*sketch.Profile) {
				profiles[0].Outer = append(profiles[0].Outer,
					make([]sketch.BoundaryEdge, 129)...)
			},
		},
		{
			name: "points", predicate: "edge.point_limit",
			mutate: func(profiles []*sketch.Profile) {
				profiles[0].Outer[0].Polyline = make([][2]float64, 1025)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, profiles := quarterJunctionRealProfiles(t)
			tc.mutate(profiles)
			work := &quarterJunctionTestWork{cancelErr: t.Context().Err, limit: 4096}
			junctions, ok, err := s.CertifyQuarterArcJunctions(t.Context(), profiles, work)
			require.NoError(t, err)
			require.False(t, ok)
			require.Empty(t, junctions)
			require.Equal(t, tc.predicate, work.decline.Predicate)
			require.LessOrEqual(t, work.used, work.limit)
		})
	}
}

func TestQuarterArcJunctionCancelsDuringRootProof(t *testing.T) {
	s, profiles := quarterJunctionRealProfiles(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	canceled := false
	work := &quarterJunctionTestWork{cancelErr: ctx.Err, limit: 4096}
	work.onStep = func(kind sketch.JunctionCharge) {
		if kind == sketch.JunctionRootProof && !canceled {
			canceled = true
			cancel()
		}
	}
	junctions, ok, err := s.CertifyQuarterArcJunctions(ctx, profiles, work)
	require.True(t, canceled)
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, ok)
	require.Empty(t, junctions)
}

func TestQuarterArcJunctionDeclinesThreeSourceContact(t *testing.T) {
	s := newSketch(t)
	center := s.CreatePoint(0, 0)
	right := s.CreatePoint(5, 0)
	top := s.CreatePoint(0, 5)
	s.CreateLine(center, right)
	s.CreateArc(center, right, top)
	s.CreateLine(top, center)
	s.CreateRectangle(2, 1, 6, 3)
	s.CreateLine(s.CreatePoint(4, 0), s.CreatePoint(4, 5))
	profiles := s.Profiles()
	require.NotEmpty(t, profiles)
	work := &quarterJunctionTestWork{cancelErr: t.Context().Err, limit: 4096}
	junctions, ok, err := s.CertifyQuarterArcJunctions(t.Context(), profiles, work)
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, junctions, 1)
	three := big.NewRat(3, 1)
	for _, junction := range junctions {
		_, y := junction.Point()
		lo, hi := y.Bounds()
		require.NotNil(t, lo)
		require.NotNil(t, hi)
		require.False(t, lo.Cmp(three) <= 0 && hi.Cmp(three) >= 0,
			"a three-source contact cannot carry a quarter certificate")
	}
}
