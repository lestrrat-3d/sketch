package sketch

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExportSamplesPreserveBoundsAndPolylineOutput(t *testing.T) {
	w := NewWorld()
	s, err := w.CreateSketch(w.XY())
	require.NoError(t, err)

	o := s.CreatePoint(0, 0)
	start := s.CreatePoint(8, 0)
	end := s.CreatePoint(0, 8)
	s.CreateArc(o, start, end)
	s.CreateEllipticalArc(o, start, end, 10, 5, 0.2)

	control := []*Point{
		s.CreatePoint(-4, -2),
		s.CreatePoint(-2, 5),
		s.CreatePoint(3, 6),
		s.CreatePoint(7, 1),
	}
	_, err = s.CreateSpline(control...)
	require.NoError(t, err)
	_, err = s.CreateClosedSpline(control...)
	require.NoError(t, err)
	_, err = s.CreateFitSpline(control...)
	require.NoError(t, err)
	_, err = s.CreateConic(control[0], control[1], control[3], 0.7)
	require.NoError(t, err)
	_, err = s.CreateNURBS(3, append(control, s.CreatePoint(9, -3)), nil, ClampedUniformKnots(5, 3))
	require.NoError(t, err)

	samples := make([][][2]float64, len(s.ents))
	for i, e := range s.ents {
		samples[i] = sampleEntityPolyline(e, exportSegments)
		require.Equal(t, sampleEntityPolyline(e, exportSegments), svgSamples(samples, i, e, exportSegments))
		for j, p := range sampleEntityPolyline(e, boundsSegments) {
			require.Equal(t, p, samples[i][j*2], "32-segment sample %d for %T", j, e)
		}
	}

	wantBounds, ok := s.bounds()
	require.True(t, ok)
	gotBounds, ok := s.boundsWithSamples(samples)
	require.True(t, ok)
	require.Equal(t, wantBounds, gotBounds, "every other export sample must preserve the 32-segment bounds")

	svg, err := s.SVG()
	require.NoError(t, err)
	require.Equal(t, 7, strings.Count(svg, "<path "))
	require.Equal(t, 7*exportSegments, strings.Count(svg, " L"), "each output curve must retain 64 segments")
	for _, pts := range samples {
		var path strings.Builder
		path.WriteString(`<path d="`)
		for i, p := range pts {
			if i == 0 {
				path.WriteString("M")
			} else {
				path.WriteString(" L")
			}
			path.WriteString(trimFloat(p[0]-wantBounds.minX+10, 4))
			path.WriteByte(' ')
			path.WriteString(trimFloat(wantBounds.maxY-p[1]+10, 4))
		}
		require.Contains(t, svg, path.String(), "SVG path must match the direct 64-segment samples")
	}

	dxf, err := s.DXF()
	require.NoError(t, err)
	minX := appendTrimFloat(make([]byte, 0, 32), wantBounds.minX, 6)
	minY := appendTrimFloat(make([]byte, 0, 32), wantBounds.minY, 6)
	maxX := appendTrimFloat(make([]byte, 0, 32), wantBounds.maxX, 6)
	maxY := appendTrimFloat(make([]byte, 0, 32), wantBounds.maxY, 6)
	require.Contains(t, dxf, fmt.Sprintf("$EXTMIN\n10\n%s\n20\n%s\n", minX, minY))
	require.Contains(t, dxf, fmt.Sprintf("$EXTMAX\n10\n%s\n20\n%s\n", maxX, maxY))
	require.Contains(t, dxf, "LWPOLYLINE\n8\n0\n90\n64\n70\n1\n", "closed spline output keeps 64 segments")
	require.Contains(t, dxf, "LWPOLYLINE\n8\n0\n90\n65\n70\n0\n", "fit spline output keeps 64 segments")
}
