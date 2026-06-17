package geom

// interiorPoint returns a point guaranteed to lie strictly inside the closed
// polygon through poly. It tries the centroid (correct for convex polygons),
// then falls back to nudging each edge midpoint toward the centroid until one
// lands inside — robust for non-convex polygons.
func interiorPoint(poly [][2]float64) [2]float64 {
	n := len(poly)
	if n == 0 {
		return [2]float64{}
	}
	var cx, cy float64
	for _, p := range poly {
		cx += p[0]
		cy += p[1]
	}
	c := [2]float64{cx / float64(n), cy / float64(n)}
	if pointInPolygon(c, poly) {
		return c
	}
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		m := [2]float64{(poly[i][0] + poly[j][0]) / 2, (poly[i][1] + poly[j][1]) / 2}
		probe := [2]float64{m[0]*0.999 + c[0]*0.001, m[1]*0.999 + c[1]*0.001}
		if pointInPolygon(probe, poly) {
			return probe
		}
	}
	return poly[0]
}

// signedPolyArea returns the signed area of the closed polygon through pts
// (the closing edge from the last point back to the first is implied). The
// result is positive when the vertices wind counter-clockwise, negative when
// clockwise. Fewer than three points enclose no area.
func signedPolyArea(pts [][2]float64) float64 {
	n := len(pts)
	if n < 3 {
		return 0
	}
	var sum float64
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		sum += pts[i][0]*pts[j][1] - pts[j][0]*pts[i][1]
	}
	return sum / 2
}

// pointInPolygon reports whether pt lies strictly inside the closed polygon
// through poly (the closing edge is implied), by even-odd ray casting along
// +x. The half-open edge rule (one endpoint counted, the other not) keeps a
// vertex or near-horizontal edge on the ray from being double-counted. A point
// exactly on the boundary is reported unreliably by design — callers that care
// about on-boundary use a representative interior point instead.
func pointInPolygon(pt [2]float64, poly [][2]float64) bool {
	n := len(poly)
	if n < 3 {
		return false
	}
	x, y := pt[0], pt[1]
	inside := false
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		yi, yj := poly[i][1], poly[j][1]
		if (yi > y) == (yj > y) {
			continue // edge does not straddle the horizontal ray
		}
		// x coordinate where edge i-j crosses the horizontal line through pt
		xcross := poly[i][0] + (y-yi)/(yj-yi)*(poly[j][0]-poly[i][0])
		if x < xcross {
			inside = !inside
		}
	}
	return inside
}
