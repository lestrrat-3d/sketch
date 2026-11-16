package geom_test

import (
	"slices"
	"testing"

	"github.com/lestrrat-3d/sketch/geom"
	"github.com/stretchr/testify/require"
)

// leverForkScene is the fork end of a watch lever and the notch cut into it. The
// fork's gap round G and the notch semicircle B both end at a = (r, ky), where G
// touches B from inside; the corner round K touches B from outside there. All
// three centres lie on y = ky, so all three curves are vertical at a, and G and
// B leave a along one ray. The notch's straight walls start at a too.
func leverForkScene() []geom.Curve {
	const (
		ky  = 38.874650636459869
		r   = 1.6676711944252536
		gc  = 1.1338355972126268
		dr  = 0.59999999999999998
		tip = 42.149999999999999
		kc  = 2.3338355972126268
	)
	p := geom.NewPoint
	a0, a1, a2 := p(3, 30), p(3, ky), p(r, ky)
	b0, b1, b2 := p(-3, 30), p(-3, ky), p(-r, ky)
	d1, d2, d3, d4 := p(dr, ky), p(dr, tip), p(-dr, tip), p(-dr, ky)
	u1, u2 := p(1, 44), p(-1, 44)
	return []geom.Curve{
		geom.NewLine(a0, a1),
		geom.NewArc(p(kc, ky), a1, a2),
		geom.NewArc(p(gc, ky), d1, a2),
		geom.NewLine(d1, d2),
		geom.NewArc(p(0, tip), d2, d3),
		geom.NewLine(d3, d4),
		geom.NewArc(p(-gc, ky), b2, d4),
		geom.NewArc(p(-kc, ky), b2, b1),
		geom.NewLine(b1, b0),
		geom.NewLine(b0, a0),
		geom.NewArc(p(0, ky), b2, a2),
		geom.NewLine(a2, u1),
		geom.NewLine(u1, u2),
		geom.NewLine(u2, b2),
	}
}

// TestSharedTangentEndKeepsCells pins the three cells of the fork and notch: the
// fork less the notch, their overlap (the dart and two crescents that narrow to
// a and its mirror), and the notch less the fork. Before the shared same-ray end
// was certified for exact port order, the face walk returned one cell with the
// union's area, reported valid and not degenerate.
func TestSharedTangentEndKeepsCells(t *testing.T) {
	scene := leverForkScene()
	for _, reversed := range []bool{false, true} {
		in := slices.Clone(scene)
		if reversed {
			slices.Reverse(in)
		}
		arr := geom.Regions(in, nil)
		require.Falsef(t, arr.Degenerate, "reversed=%v", reversed)
		require.Lenf(t, arr.Regions, 3, "reversed=%v", reversed)
		areas := make([]float64, 0, len(arr.Regions))
		for _, region := range arr.Regions {
			areas = append(areas, region.Area)
		}
		slices.Sort(areas)
		require.InDeltaf(t, 7.969198, areas[0], 1e-5, "reversed=%v", reversed)
		require.InDeltaf(t, 10.072133, areas[1], 1e-5, "reversed=%v", reversed)
		require.InDeltaf(t, 50.273480, areas[2], 1e-5, "reversed=%v", reversed)
	}
}

// TestSharedEndLineArcSameRayIsDegenerate pins the conservative verdict for a
// line and an arc leaving a shared endpoint along one ray: exact port order is
// certified for two curved sources only, so the scene is flagged rather than
// walked by chord order.
func TestSharedEndLineArcSameRayIsDegenerate(t *testing.T) {
	p := geom.NewPoint
	// The quarter arc from (1, 0) to (0, 1) about the origin leaves (1, 0) along
	// +y, as does the line from (1, 0) to (1, 2).
	curves := []geom.Curve{
		geom.NewArc(p(0, 0), p(1, 0), p(0, 1)),
		geom.NewLine(p(1, 0), p(1, 2)),
		geom.NewLine(p(1, 2), p(0, 1)),
	}
	arr := geom.Regions(curves, nil)
	require.True(t, arr.Degenerate)
}
