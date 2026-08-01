package geom

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// White-box tests for the two round-off identity bands the exactness decision rests on,
// vertexCertifies and endpointReproduces. Both ask the same question about ONE source —
// does this source's own evaluation of the reported parameter land on the coordinate the
// arrangement emitted — so both must be answered against that source's own size as well
// as the scene's. Probing them directly pins each decision on its own, where the
// Regions-level regression (TestExactBoundIdentityBandIsSourceLocal) sees only the
// verdict a whole arrangement arrives at.

// bandArranger builds a densified arranger holding a circle of radius r at the origin,
// plus an unrelated line at x=far when far > 0, and returns it with the circle's source
// index. densify is what fills scale, merge, the vertex table and each source's extent.
func bandArranger(r, far float64) (*arranger, int) {
	var curves []Curve
	if far > 0 {
		curves = append(curves, NewLine(NewPoint(far, -1), NewPoint(far, 1)))
	}
	a := newArranger(curves, []ClosedCurve{NewCircle(NewPoint(0, 0), r)}, arrangeConfig{})
	a.densify()
	return a, len(curves)
}

func TestSourceExtentIsTheSourcesOwnSize(t *testing.T) {
	for _, far := range []float64{0, 1e3, 1e6} {
		a, ci := bandArranger(5, far)
		require.InDeltaf(t, 10, a.sources[ci].extent, 1e-9,
			"far=%g: a circle of radius 5 is 10 across whatever else is drawn", far)
		if far > 0 {
			require.Greaterf(t, a.scale, a.sources[ci].extent,
				"far=%g: the fixture is only meaningful while the scene is bigger than the circle", far)
		}
	}
}

// TestExactIdentityBandIsSourceLocal probes vertexCertifies. A bound whose graph vertex
// sits a real distance from the bound's own point is not that point, and how far "real"
// is may not depend on what else was drawn.
func TestExactIdentityBandIsSourceLocal(t *testing.T) {
	const r = 5.0
	// Gaps well above the circle's own band (weldIdentEps·10 = 1e-11) and, at the far
	// scenes, well inside the scene band (weldIdentEps·1e4 = 1e-8) — the window the
	// scene-only band certified.
	for _, gap := range []float64{1e-10, 1e-9, 1e-8} {
		for _, far := range []float64{0, 1e3, 1e4, 1e5} {
			a, ci := bandArranger(r, far)
			src := &a.sources[ci]
			p := src.at(0.125)
			v := a.verts.canon(p[0]+gap, p[1])
			require.Falsef(t, a.vertexCertifies(src, v, p[0], p[1]),
				"gap=%g far=%g: a vertex %g from the bound's own point is a different point",
				gap, far, gap)
		}
	}

	// The band still admits genuine identity: a shared corner reaches the arrangement
	// through each curve's own evaluation, which agree to a few ulps.
	for _, far := range []float64{0, 1e3, 1e6} {
		a, ci := bandArranger(r, far)
		src := &a.sources[ci]
		p := src.at(0.3)
		ulp := math.Nextafter(p[0], math.Inf(1)) - p[0]
		v := a.verts.canon(p[0]+4*ulp, p[1])
		require.Truef(t, a.vertexCertifies(src, v, p[0], p[1]),
			"far=%g: a few-ulp difference is the same point, at every scene size", far)
	}
}

// TestEndpointReproductionBandIsSourceLocal probes endpointReproduces, which decides a
// tiny segment's synthetic endpoint bound: it is exact only when the source's own
// evaluation at the reported parameter lands back on the coordinate densify emitted.
// That miss is a property of the source, so the band it is judged against is too.
func TestEndpointReproductionBandIsSourceLocal(t *testing.T) {
	const r = 5.0
	for _, gap := range []float64{1e-10, 1e-9, 1e-8} {
		for _, far := range []float64{0, 1e3, 1e4, 1e5} {
			a, ci := bandArranger(r, far)
			src := &a.sources[ci]
			q := src.at(0.125)
			require.Falsef(t, a.endpointReproduces(src, 0.125, q[0]+gap, q[1]),
				"gap=%g far=%g: an endpoint the parameter misses by %g is not reproduced",
				gap, far, gap)
			require.Truef(t, a.endpointReproduces(src, 0.125, q[0], q[1]),
				"gap=%g far=%g: the evaluated endpoint reproduces itself", gap, far)
		}
	}
}
