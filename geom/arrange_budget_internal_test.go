package geom

import (
	"context"
	"errors"
	"math"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWireGraphPreflightsHalfEdgePositions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cancel bool
	}{
		{name: "work refusal"},
		{name: "cancellation", cancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			refused := errors.New("position index work refused")
			var want error = refused
			if tc.cancel {
				want = context.Canceled
			}
			var twoRecordCharges int
			budget := &arrangeBudget{contextErr: ctx.Err, charge: func(n uint64) error {
				if n != 2 {
					return nil
				}
				twoRecordCharges++
				if twoRecordCharges == 1 {
					return nil // the half-edge storage preflight
				}
				if tc.cancel {
					cancel()
				}
				return want // the position-index preflight must refuse before make
			}}
			a := &arranger{
				budget:  budget,
				sources: []source{{kind: srcLine, ax: 0, ay: 0, bx: 1, by: 0}},
				verts:   vertexTable{xs: []float64{0, 1}, ys: []float64{0, 0}},
				edges:   []arrEdge{{u: 0, v: 1, src: 0, pu: 0, pv: 1}},
			}
			var stopped any
			func() {
				defer func() { stopped = recover() }()
				a.wireGraph()
			}()
			abort, ok := stopped.(arrangeAbort)
			require.True(t, ok)
			require.ErrorIs(t, abort.err, want)
			require.Equal(t, 2, twoRecordCharges)
			require.Len(t, a.halfs, 2)
		})
	}
}

func TestArrangementHardCapsCoverGroupReplayUsage(t *testing.T) {
	require.GreaterOrEqual(t, maxArrangementWork, 1_603_678)
	require.GreaterOrEqual(t, maxArrangementRecords, 19_811)
	require.GreaterOrEqual(t, maxArrangementBytes, 7_679_504)
}

func TestArrangeBudgetStepPreservesBoundedOrder(t *testing.T) {
	t.Run("cancellation before zero step", func(t *testing.T) {
		canceled := errors.New("canceled")
		budget := &arrangeBudget{
			contextErr: func() error { return canceled },
			charge: func(uint64) error {
				t.Fatal("zero step reached charge")
				return nil
			},
		}

		var stopped any
		func() {
			defer func() { stopped = recover() }()
			budget.step(0)
		}()

		abort, ok := stopped.(arrangeAbort)
		require.True(t, ok)
		require.ErrorIs(t, abort.err, canceled)
	})

	t.Run("cap before charge", func(t *testing.T) {
		calls := 0
		budget := &arrangeBudget{
			contextErr: func() error {
				calls++
				return nil
			},
			charge: func(uint64) error {
				t.Fatal("over-cap step reached charge")
				return nil
			},
		}

		var stopped any
		func() {
			defer func() { stopped = recover() }()
			budget.step(257)
		}()

		abort, ok := stopped.(arrangeAbort)
		require.True(t, ok)
		require.ErrorIs(t, abort.err, ErrUnsupportedLineScene)
		require.Equal(t, 1, calls)
		require.Zero(t, budget.spent)
	})

	t.Run("charge before spend", func(t *testing.T) {
		refused := errors.New("charge refused")
		budget := &arrangeBudget{
			contextErr: func() error { return nil },
			charge:     func(uint64) error { return refused },
		}

		var stopped any
		func() {
			defer func() { stopped = recover() }()
			budget.step(3)
		}()

		abort, ok := stopped.(arrangeAbort)
		require.True(t, ok)
		require.ErrorIs(t, abort.err, refused)
		require.Zero(t, budget.spent)
	})
}

func TestSplitFragmentsKeepsNaNMergeInsertionPath(t *testing.T) {
	a := newArranger(nil, []ClosedCurve{NewCircle(NewPoint(0, 0), 10)},
		arrangeConfig{vertexMerge: math.NaN()})
	a.densify()
	nBoundaries := 2 * len(a.segs)

	frags := a.splitFragments()

	require.Len(t, frags, len(a.segs))
	require.Len(t, a.verts.xs, nBoundaries+2*len(frags))
}

func TestDedupBoundaryPointsKeepsBitwiseDistinctPointsAndChargesWork(t *testing.T) {
	negZero := math.Copysign(0, -1)
	nan1 := math.Float64frombits(0x7ff8000000000001)
	nan2 := math.Float64frombits(0x7ff8000000000002)
	points := [][2]float64{
		{0, 0},
		{negZero, 0},
		{0, negZero},
		{nan1, 1},
		{nan2, 1},
		{nan1, 1},
		{0, 0},
	}
	var charged, reservedBytes uint64
	budget := &arrangeBudget{
		contextErr: func() error { return nil },
		charge: func(n uint64) error {
			charged += n
			return nil
		},
		resources: func(_ uint64, bytes uint64) error {
			reservedBytes += bytes
			return nil
		},
	}

	unique := dedupBoundaryPoints(budget, points)
	require.Len(t, unique, 5)
	require.Equal(t, uint64(len(points)), budget.spent)
	require.Equal(t, budget.spent, charged)
	require.Equal(t, uint64(len(points))*32, reservedBytes)
	require.True(t, samePointBits(unique[0], [2]float64{0, 0}))
	require.True(t, samePointBits(unique[1], [2]float64{negZero, 0}))
	require.True(t, samePointBits(unique[2], [2]float64{0, negZero}))
	require.True(t, samePointBits(unique[3], [2]float64{nan1, 1}))
	require.True(t, samePointBits(unique[4], [2]float64{nan2, 1}))

	slices.SortFunc(unique, canonPointCompare)
	require.Less(t, canonPointCompare(unique[0], unique[1]), 0,
		"cmp.Compare keeps NaNs before finite x values and payload bits break the NaN tie")
	require.Less(t, canonPointCompare(unique[3], unique[4]), 0,
		"raw coordinate bits keep the two signed-zero points distinct in canonical order")
}
