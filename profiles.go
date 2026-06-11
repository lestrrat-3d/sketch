package sketch

import "github.com/lestrrat-3d/sketch/geom"

// Profile is a closed region boundary detected in a sketch: either a chain of
// lines and arcs connected end-to-end through shared points, or a single
// closed primitive (circle or ellipse). Profiles are what downstream
// operations (eventually extrude/revolve) consume.
type Profile struct {
	Entities []Entity // boundary entities, in walk order for chains
}

// Profiles detects the closed profiles formed by the sketch's
// non-construction entities. Every circle and ellipse is a profile on its
// own; every closed loop of lines and arcs — connected by shared points, the
// natural state for geometry built from shared [geom.Point]s — contributes
// one profile. Open chains contribute nothing. Boundaries that merely cross
// without sharing a point are not subdivided into regions; splitting curves
// at bare intersections is a future extension (the intersection math already
// lives in [geom]).
func (s *Sketch) Profiles() []*Profile {
	var profiles []*Profile
	var curves []geom.Curve
	owner := map[geom.Curve]Entity{}
	// Loop detection keys on shared *geom.Point identity, so map each shared
	// sketch point to a single geom point: entities meeting at a point then
	// share that geom point and form a closed chain for geom.Loops.
	gpt := map[*Point]*geom.Point{}
	pt := func(p *Point) *geom.Point {
		if g, ok := gpt[p]; ok {
			return g
		}
		g := p.Geometry()
		gpt[p] = g
		return g
	}
	for _, e := range s.ents {
		if e.IsConstruction() {
			continue
		}
		switch t := e.(type) {
		case *Circle:
			profiles = append(profiles, &Profile{Entities: []Entity{t}})
		case *Ellipse:
			profiles = append(profiles, &Profile{Entities: []Entity{t}})
		case *Line:
			c := geom.NewLine(pt(t.Start), pt(t.End))
			curves = append(curves, c)
			owner[c] = t
		case *Arc:
			c := geom.NewArc(pt(t.Center), pt(t.Start), pt(t.End))
			curves = append(curves, c)
			owner[c] = t
		}
	}
	for _, loop := range geom.Loops(curves) {
		p := &Profile{}
		for _, c := range loop.Curves {
			p.Entities = append(p.Entities, owner[c])
		}
		profiles = append(profiles, p)
	}
	return profiles
}
