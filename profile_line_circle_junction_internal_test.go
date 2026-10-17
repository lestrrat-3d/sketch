package sketch

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

func TestLineCircleJunctionProofOutcomeLimits(t *testing.T) {
	x := math.Sqrt(96)
	angle := math.Atan2(-2, x) / (2 * math.Pi)
	if angle < 0 {
		angle++
	}
	in := junctionInput{
		line: [4]float64{-12, -2, 12, -2}, circle: [3]float64{0, 0, 10},
		event: [2]float64{x, -2}, branch: 1,
		lineTrim: (x + 12) / 24, circleTrim: angle,
	}
	_, _, err := certifyLineCircleProof(t.Context(), in, standardJunctionLimits)
	require.NoError(t, err)

	overWidth := in
	overWidth.event[0] += 0.001
	_, _, err = certifyLineCircleProof(t.Context(), overWidth, standardJunctionLimits)
	require.ErrorIs(t, err, errJunctionMiss)

	thirdSource := in
	thirdSource.otherBounds = [][4]float64{{9, -3, 10, -1}}
	_, _, err = certifyLineCircleProof(t.Context(), thirdSource, standardJunctionLimits)
	require.ErrorIs(t, err, errJunctionMiss)

	tangent := in
	tangent.line = [4]float64{-12, 10, 12, 10}
	_, _, err = certifyLineCircleProof(t.Context(), tangent, standardJunctionLimits)
	require.ErrorIs(t, err, errJunctionMiss)

	endpoint := in
	endpoint.line = [4]float64{4, 3, 6, 3}
	endpoint.circle = [3]float64{0, 0, 5}
	endpoint.event = [2]float64{4, 3}
	endpoint.lineTrim, endpoint.circleTrim = 0, math.Atan2(3, 4)/(2*math.Pi)
	endpoint.branch = 1
	_, _, err = certifyLineCircleProof(t.Context(), endpoint, standardJunctionLimits)
	require.ErrorIs(t, err, errJunctionMiss)

	seam := in
	seam.line = [4]float64{-12, 0, 12, 0}
	seam.event = [2]float64{10, 0}
	seam.lineTrim, seam.circleTrim = 22.0/24.0, 0
	_, _, err = certifyLineCircleProof(t.Context(), seam, standardJunctionLimits)
	require.ErrorIs(t, err, errJunctionMiss)

	short := standardJunctionLimits
	short.rootRounds = 2
	_, _, err = certifyLineCircleProof(t.Context(), in, short)
	require.ErrorIs(t, err, ErrJunctionWorkLimit)

	short = standardJunctionLimits
	short.intermediateBits = 16
	_, _, err = certifyLineCircleProof(t.Context(), in, short)
	require.ErrorIs(t, err, ErrJunctionWorkLimit)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err = certifyLineCircleProof(ctx, in, standardJunctionLimits)
	require.True(t, errors.Is(err, context.Canceled))
}

type cancelDuringSourceScan struct {
	deadline  func() (time.Time, bool)
	done      <-chan struct{}
	baseErr   func() error
	value     func(any) any
	cancel    context.CancelFunc
	remaining int
	checks    int
}

func (c *cancelDuringSourceScan) Deadline() (time.Time, bool) { return c.deadline() }
func (c *cancelDuringSourceScan) Done() <-chan struct{}       { return c.done }
func (c *cancelDuringSourceScan) Value(key any) any           { return c.value(key) }

func (c *cancelDuringSourceScan) Err() error {
	c.checks++
	c.remaining--
	if c.remaining == 0 {
		c.cancel()
	}
	return c.baseErr()
}

func TestLineCircleJunctionOtherSourceScanBound(t *testing.T) {
	boxes := make([][4]float64, 8)
	for i := range boxes {
		boxes[i] = [4]float64{20, 20, 21, 21}
	}
	px := junctionInterval{ratInt(9), ratInt(10)}
	py := junctionInterval{ratInt(-2), ratInt(-2)}
	base, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx := &cancelDuringSourceScan{
		deadline: base.Deadline, done: base.Done(), baseErr: base.Err,
		value: base.Value, cancel: cancel, remaining: 8,
	}
	m := junctionMath{cancelErr: ctx.Err, limits: standardJunctionLimits}
	require.ErrorIs(t, m.checkOtherSources(px, py, boxes), context.Canceled)
	require.Equal(t, 8, ctx.checks)

	m.cancelErr = t.Context().Err
	boxes[7] = [4]float64{9, -3, 10, -1}
	require.ErrorIs(t, m.checkOtherSources(px, py, boxes), errJunctionMiss)
	boxes = make([][4]float64, maxJunctionOtherSources+1)
	require.ErrorIs(t, m.checkOtherSources(px, py, boxes), ErrJunctionWorkLimit)
}

func TestLineCircleJunctionSourceCopyBound(t *testing.T) {
	w := NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)
	s.CreateCircle(s.CreatePoint(0, 0), 10)
	s.CreateRectangle(-12, -2, 12, 2)
	var cross *geom.LineCircleCrossing
	for _, p := range s.Profiles() {
		for _, edge := range p.Outer {
			if edge.circleEnd != nil {
				cross = edge.circleEnd
				break
			}
		}
		if cross != nil {
			break
		}
	}
	require.NotNil(t, cross)
	_, err = copyJunctionOtherBounds(t.Context(), cross, 1)
	require.ErrorIs(t, err, ErrJunctionWorkLimit)
	boxes, err := copyJunctionOtherBounds(t.Context(), cross, maxJunctionOtherSources)
	require.NoError(t, err)
	require.Len(t, boxes, cross.SourceCount()-2)
	box, ok := cross.SourceBound(0)
	require.True(t, ok)
	box[0] = 12345
	require.Equal(t, 12345.0, box[0])
	again, ok := cross.SourceBound(0)
	require.True(t, ok)
	require.NotEqual(t, 12345.0, again[0])
	_, ok = cross.SourceBound(-1)
	require.False(t, ok)
	_, ok = cross.SourceBound(cross.SourceCount())
	require.False(t, ok)
}
