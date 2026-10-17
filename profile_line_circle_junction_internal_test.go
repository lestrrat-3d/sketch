package sketch

import (
	"context"
	"errors"
	"math"
	"testing"

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
