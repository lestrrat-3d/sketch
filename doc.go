// Package sketch is a standalone, fully programmable parametric 2D sketch
// engine in the spirit of the sketch environment in Autodesk Fusion.
//
// A [Sketch] is built from geometric primitives — [Point], [Line], [Circle]
// and [Arc] — that are tied together with geometric and dimensional
// constraints (coincident, horizontal, vertical, parallel, perpendicular,
// tangent, equal, concentric, symmetric, distance, angle, radius, …). A
// numerical Levenberg–Marquardt constraint solver then moves the geometry so
// that every constraint is satisfied simultaneously, exactly the way a
// parametric CAD sketcher does.
//
// Because dimensional constraints are ordinary, editable values, sketches are
// fully parametric: change a dimension and call [Sketch.Solve] again and the
// geometry updates.
//
// # Example
//
//	// Every sketch belongs to a World; create one on a datum plane.
//	w := sketch.NewWorld()
//	s, _ := w.CreateSketch(w.XY())
//	// Geometry is authored from points; sharing a point ties entities together.
//	a := s.CreatePoint(0, 0)
//	b := s.CreatePoint(7, 2)
//	d := s.CreatePoint(-1, 8)
//	ab := s.CreateLine(a, b)
//	ad := s.CreateLine(a, d)
//
//	s.Fix(a) // ground the shared origin corner
//	wd := sketch.NewDistance(a, b, 100) // driving dimension
//	h := sketch.NewDistance(a, d, 60)
//	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewVertical(ad), wd, h)
//
//	res, err := s.Solve()
//	if err != nil { /* ... */ }
//	fmt.Println(res.DOF, "degrees of freedom remaining")
//	svg, _ := s.SVG() // or s.SVG(sketch.WithMargin(20), sketch.WithShowPoints(false))
//
// The geom package is the transient math / snapshot layer (an entity's
// Geometry method returns a geom value at the current solved coordinates); it
// is never committed as sketch input.
//
// # Orientation and sign conventions
//
// A sketch's coordinates are plane-local (u, v): Y-up and angles
// counterclockwise-positive, in the frame of the construction plane the sketch
// is drawn on (the world XY datum by default — see [Plane] and [World]). World
// (x, y, z) coordinates are a derived read-out via [Point.World]. Every
// directional convention in the package derives from the plane-local frame:
// "the left of a line" means the left of its start→end direction, and a
// positive angle turns counterclockwise.
//
// Constraints divide into two groups by how they treat sides and branches:
//
//   - Signed constraints pin a branch. [NewAngle] (counterclockwise from l1's
//     direction to l2's), [NewOffset] (positive to the left of src), and
//     [NewHorizontalDistance]/[NewVerticalDistance] (Δx/Δy) each admit a single
//     configuration per target value; the mirrored configuration is selected
//     by negating the value, not by moving the geometry.
//   - Unsigned and side-relative constraints leave the branch to the geometry:
//     [NewTangent], [NewDistancePointLine], [NewDistanceLines] and
//     [NewSymmetric] are satisfied on either side, and the solver keeps
//     whichever side the geometry starts on. The initial positions (the seed —
//     see [Sketch.Solve] and [Point.MoveTo]) decide the branch.
//
// A fully constrained sketch (DOF 0) built from unsigned constraints can
// therefore still admit several discrete configurations;
// [Sketch.ProbeConfigurations] searches for such alternatives.
package sketch
