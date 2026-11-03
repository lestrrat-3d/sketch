package geom

import (
	"context"
	"errors"
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
