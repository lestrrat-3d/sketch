package geom

import "testing"

// TestPolylinesMeetOnlyAtContacts pins the incidence certificate's first condition —
// "the two spliced polylines meet ONLY at the injected crossing points" — against the
// three inputs an earlier form accepted. That form asked a DIFFERENT question, whether a
// contact sits within segEps of either host segment's end, and each case below is a
// contact with no node in the planar map that the segment-end band waved through.
//
// This is one of the package's few internal tests: the predicate takes the spliced
// polylines, which are built inside the arranger from the sampled map and are not
// reachable from the exported API. The scene-level consequences are covered from outside
// by TestAnalyticCurveCrossingEndpointOnChordNotCertified and
// TestAnalyticCurveCrossingAtSampleVertexNotCertified.
func TestPolylinesMeetOnlyAtContacts(t *testing.T) {
	const eps = 1e-12

	t.Run("injected crossing accepted", func(t *testing.T) {
		// The four segment pairs around an injected point all report the contact AT that
		// point, so a point-based whitelist passes every one of them. This is the case
		// the certificate exists to admit, and the guard against over-refusal.
		pi := [][2]float64{{-1, -1}, {0, 0}, {1, 1}}
		pj := [][2]float64{{-1, 1}, {0, 0}, {1, -1}}
		if !polylinesMeetOnlyAtContacts(pi, pj, [][2]float64{{0, 0}}, eps) {
			t.Fatal("a crossing at the injected point is the certificate's own case and must be accepted")
		}
	})

	t.Run("endpoint on the interior of the other chord", func(t *testing.T) {
		// pi ENDS at (5,0), which lies halfway along pj's last segment. The contact is
		// real — the two polylines meet there — but it is not an injected crossing, so
		// the planar map gets no vertex for it once the pair takes analytic authority.
		// Its host parameters are ti=1, tj=0.5, so the old segment-end band excused it.
		pi := [][2]float64{{-3, -3}, {0, 0}, {5, 0}}
		pj := [][2]float64{{-3, 3}, {0, 0}, {3, -1}, {7, 1}}
		if polylinesMeetOnlyAtContacts(pi, pj, [][2]float64{{0, 0}}, eps) {
			t.Fatal("an endpoint resting on the interior of the other polyline's chord is a contact with no node")
		}
	})

	t.Run("pass-through at a vertex", func(t *testing.T) {
		// A genuine transverse crossing at (2,0), with no vertex shared: (2,0) is a
		// vertex of pi and interior to pj's only segment. Nothing is injected anywhere
		// near it, so the map has no node for it and the face walk runs the two edges
		// through each other.
		pi := [][2]float64{{0, -1}, {2, 0}, {4, 1}}
		pj := [][2]float64{{2, -3}, {2, 3}}
		if polylinesMeetOnlyAtContacts(pi, pj, nil, eps) {
			t.Fatal("a transverse pass-through at a vertex of one polyline is still a crossing with no node")
		}
		// The discriminator: the same crossing moved OFF pi's vertex is interior to a
		// segment of each, which the old form did refuse. Identity with an injected
		// point answers both placements the same way, which is the repair.
		pjOff := [][2]float64{{1, -3}, {1, 3}}
		if polylinesMeetOnlyAtContacts(pi, pjOff, nil, eps) {
			t.Fatal("an interior/interior crossing must be refused too")
		}
	})

	t.Run("collinear overlap", func(t *testing.T) {
		// segParams rejects a parallel pair on its determinant before any range test, so
		// a collinear OVERLAP — two chords sharing a whole span rather than a point —
		// reached the old predicate as silence and was accepted with no tolerance applied
		// to it at all. An overlap of positive length is never a transverse crossing.
		pi := [][2]float64{{0, 0}, {10, 0}}
		pj := [][2]float64{{3, 0}, {13, 0}}
		if polylinesMeetOnlyAtContacts(pi, pj, nil, eps) {
			t.Fatal("a collinear overlap is a shared span, not a crossing at an injected point")
		}
	})
}
