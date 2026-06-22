package geom

// NURBSBulgeSpanForTest exposes the internal nurbsBulgeSpan to the external test
// package: it computes the signed arc-vs-chord area of the NURBS fragment over
// natural parameters [t0, t1] (t in [0, 1]), evaluating the chord endpoints from
// the curve itself. Test-only.
func NURBSBulgeSpanForTest(c *NURBS, t0, t1 float64) float64 {
	lo, hi := c.domain()
	ax, ay := c.Eval(lo + (hi-lo)*t0)
	ex, ey := c.Eval(lo + (hi-lo)*t1)
	return nurbsBulgeSpan(c, t0, t1, ax, ay, ex, ey)
}
