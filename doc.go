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
//	// Generic geometry lives in the geom package and is committed into a sketch.
//	ga := geom.NewPoint(0, 0)
//	gb := geom.NewPoint(7, 2)
//	gd := geom.NewPoint(-1, 8)
//
//	s := sketch.New()
//	ab := s.AddLine(geom.NewLine(ga, gb)) // commits the line and its points
//	ad := s.AddLine(geom.NewLine(ga, gd))
//
//	s.Lock(ab.A, 0, 0) // ground the origin corner
//	w := sketch.NewDistance(ab.A, ab.B, 100) // driving dimension
//	h := sketch.NewDistance(ad.A, ad.B, 60)
//	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewVertical(ad), w, h)
//
//	res, err := s.Solve()
//	if err != nil { /* ... */ }
//	fmt.Println(res.DOF, "degrees of freedom remaining")
//	svg, _ := s.SVG(sketch.DefaultSVGOptions())
//
// The package has no external dependencies.
package sketch
