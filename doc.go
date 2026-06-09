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
//	s := sketch.New()
//	a := s.AddPoint(0, 0)
//	b := s.AddPoint(7, 2)
//	c := s.AddPoint(6, 9)
//	d := s.AddPoint(-1, 8)
//	s.AddLine(a, b)
//	s.AddLine(b, c)
//	s.AddLine(c, d)
//	s.AddLine(d, a)
//
//	s.Lock(a, 0, 0)            // ground the origin corner
//	s.Horizontal(s.AddLine(a, b))
//	s.Vertical(s.AddLine(a, d))
//	w := s.Distance(a, b, 100) // driving dimension
//	h := s.Distance(a, d, 60)
//	_ = w; _ = h
//
//	res, err := s.Solve()
//	if err != nil { /* ... */ }
//	fmt.Println(res.DOF, "degrees of freedom remaining")
//	svg, _ := s.SVG(sketch.DefaultSVGOptions())
//
// The package has no external dependencies.
package sketch
