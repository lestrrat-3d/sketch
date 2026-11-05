package examples_test

import (
	"fmt"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_fitSplineInterpolant shows how to read the curve a fit spline
// IS, rather than points on it. Eval and Polyline sample the curve; a consumer
// that has to integrate it exactly (a solid modeller, an exact-area pass, an
// exporter emitting a native curve) needs the interpolant's own coefficients,
// which Interpolant returns as the natural-cubic data plus per-span polynomials.
func Example_sketch_fitSplineInterpolant() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())

	// A flank drawn the way a spec gives it: sample points the curve runs through.
	flank, err := s.CreateFitSpline(
		s.CreatePoint(0, 0),
		s.CreatePoint(2, 3),
		s.CreatePoint(5, 4),
		s.CreatePoint(8, 0),
	)
	if err != nil {
		fmt.Printf("failed to create the fit spline: %s\n", err)
		return
	}

	fi, err := flank.Interpolant()
	if err != nil {
		fmt.Printf("failed to read the interpolant: %s\n", err)
		return
	}
	total := fi.Params[len(fi.Params)-1]
	fmt.Printf("interpolated points: %d, cubic spans: %d, chord length: %.4f\n",
		len(fi.Points), len(fi.Spans()), total)
	// A natural cubic: the second derivative vanishes at both ends.
	fmt.Printf("end second derivatives: %v %v\n", fi.SecondDerivs[0], fi.SecondDerivs[len(fi.SecondDerivs)-1])

	// Each span is an ordinary cubic in its own normalized parameter
	// u = (p - PStart)/(PEnd - PStart), running over [0, 1] across the span, so a
	// consumer can integrate or differentiate it in closed form. Normalized is what
	// keeps every coefficient on the scale of the curve itself: in the absolute
	// p - PStart the cubic term carries 1/h², which underflows to zero on a wide
	// span and drops the term without any coefficient going non-finite.
	span := fi.Spans()[1]
	h := span.PEnd - span.PStart
	u := 0.5
	x := span.X[0] + u*(span.X[1]+u*(span.X[2]+u*span.X[3]))
	y := span.Y[0] + u*(span.Y[1]+u*(span.Y[2]+u*span.Y[3]))
	fmt.Printf("span 1 spans t %.4f..%.4f\n", span.TStart, span.TEnd)
	fmt.Printf("reconstructed midpoint: (%.6f, %.6f)\n", x, y)

	// The same point the sampling API reports, because it is the same curve.
	ex, ey := flank.Eval((span.PStart + u*h) / total)
	fmt.Printf("Eval at the same parameter: (%.6f, %.6f)\n", ex, ey)

	// Output:
	// interpolated points: 4, cubic spans: 3, chord length: 11.7678
	// end second derivatives: [0 0] [0 0]
	// span 1 spans t 0.3064..0.5751
	// reconstructed midpoint: (3.472116, 3.826510)
	// Eval at the same parameter: (3.472116, 3.826510)
}
