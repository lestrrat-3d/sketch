// Command genimages regenerates the annotated SVG hero images embedded in the
// README gallery. It builds a fixed set of sketches in-process, solves them,
// renders each with the annotation options that tell its story (dimensions,
// constraint glyphs, DOF coloring, conflict highlighting, profile fill, status
// badge), and writes the results under a target directory (default docs/images).
//
// Output is deterministic — no timestamps, no randomness, geometry walked in
// slice order — so a matching in-sync test can byte-compare a regeneration
// against the committed files. Regenerate with:
//
//	go generate ./...                     # via the //go:generate directive
//	go run ./internal/cmd/genimages docs/images
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/lestrrat-3d/sketch"
)

func main() {
	dir := "docs/images"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := run(dir); err != nil {
		fmt.Fprintln(os.Stderr, "genimages:", err)
		os.Exit(1)
	}
}

// run regenerates every hero image into dir.
func run(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	images, err := build()
	if err != nil {
		return err
	}
	names := make([]string, 0, len(images))
	for name := range images {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(dir, name+".svg")
		if err := os.WriteFile(path, []byte(images[name]), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}
	return nil
}

// build constructs every hero and returns name -> SVG. It is exported to the
// package (lowercase) so the in-sync test can compare against the committed
// files without shelling out.
func build() (map[string]string, error) {
	out := make(map[string]string)
	for name, fn := range builders {
		svg, err := fn()
		if err != nil {
			return nil, fmt.Errorf("building %s: %w", name, err)
		}
		out[name] = svg
	}
	return out, nil
}

// builders maps each hero image name to its generator.
var builders = map[string]func() (string, error){
	"banner":               banner,
	"quickstart":           quickstart,
	"hexagon":              hexagon,
	"dof-underconstrained": dofUnder,
	"dof-constrained":      dofFull,
	"conflict-valid":       conflictValid,
	"conflict":             conflict,
	"parametric-before":    func() (string, error) { return parametric(120) },
	"parametric-after":     func() (string, error) { return parametric(200) },
	"profiles":             profiles,
	"profiles-subdivision": profilesSubdivision,
	"fillet-before":        filletBefore,
	"fillet-after":         filletAfter,
	"geometry-primitives":  geometryPrimitives,
	"compound-shapes":      compoundShapes,
	"constraint-showcase":  constraintShowcase,
	"dimension-showcase":   dimensionShowcase,
	"driven-dimension":     drivenDimension,
	"goal-drag":            goalDrag,
}

// annStyle is the shared styling for the gallery. Geometry is authored at a
// ~200-unit scale so the default stroke/point sizes read as clean CAD
// proportions; WithPixelWidth scales the whole drawing up to a legible embed
// size while the viewBox stays in geometry units. A wide margin keeps dimension
// labels inside the window frame. Every hero is framed, gridded and watermarked.
var annStyle = []sketch.SVGOption{
	sketch.WithMargin(34),
	sketch.WithPixelWidth(560),
	sketch.WithFrame(true),
	sketch.WithGrid(true),
}

func withAnn(opts ...sketch.SVGOption) []sketch.SVGOption {
	return append(append([]sketch.SVGOption{}, annStyle...), opts...)
}

// groundedRect builds an axis-aligned rectangle grounded at the origin with
// width/height driving distances, fully constrained.
func groundedRect(w, h float64) (*sketch.Sketch, *sketch.Rectangle) {
	world := sketch.NewWorld()
	s, _ := world.CreateSketch(world.XY())
	r := s.CreateRectangle(0, 0, w, h)
	r.A.MoveTo(0, 0)
	s.Fix(r.A)
	s.AddConstraint(sketch.NewDistance(r.A, r.B, w), sketch.NewDistance(r.A, r.D, h))
	return s, r
}

func quickstart() (string, error) {
	s, _ := groundedRect(200, 120)
	if _, err := s.Solve(context.Background()); err != nil {
		return "", err
	}
	return s.SVG(withAnn(
		sketch.WithDimensions(true),
		sketch.WithConstraints(true),
		sketch.WithDOFColoring(true),
		sketch.WithStatusBadge(true),
	)...)
}

func hexagon() (string, error) {
	world := sketch.NewWorld()
	s, _ := world.CreateSketch(world.XY())
	poly, err := s.CreatePolygon(0, 0, 6, 110)
	if err != nil {
		return "", err
	}
	// CreatePolygon holds regularity (equal sides + equal spokes) but leaves
	// position, rotation and size free. Ground it fully: fix the center
	// (position), make a long diagonal horizontal (rotation), and dimension the
	// circumradius (size) — DOF 0.
	poly.Center.MoveTo(0, 0)
	s.Fix(poly.Center)
	diagonal := s.CreateLine(poly.Vertices[0], poly.Vertices[3])
	diagonal.SetConstruction(true)
	s.AddConstraint(sketch.NewHorizontal(diagonal))
	s.AddConstraint(sketch.NewDistance(poly.Center, poly.Vertices[0], 110))
	if _, err := s.Solve(context.Background()); err != nil {
		return "", err
	}
	return s.SVG(withAnn(
		sketch.WithConstraints(true),
		sketch.WithDOFColoring(true),
		sketch.WithStatusBadge(true),
	)...)
}

// underRect builds a rectangle whose top-right corner is left free.
func underRect() *sketch.Sketch {
	world := sketch.NewWorld()
	s, _ := world.CreateSketch(world.XY())
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(200, 0)
	c := s.CreatePoint(200, 120)
	d := s.CreatePoint(0, 120)
	ab := s.CreateLine(a, b)
	s.CreateLine(b, c)
	s.CreateLine(c, d)
	da := s.CreateLine(d, a)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewVertical(da), sketch.NewDistance(a, b, 200))
	s.Solve(context.Background())
	return s
}

func dofUnder() (string, error) {
	return underRect().SVG(withAnn(sketch.WithDOFColoring(true), sketch.WithStatusBadge(true))...)
}

func dofFull() (string, error) {
	s, _ := groundedRect(200, 120)
	if _, err := s.Solve(context.Background()); err != nil {
		return "", err
	}
	return s.SVG(withAnn(sketch.WithDOFColoring(true), sketch.WithStatusBadge(true))...)
}

// triangleOpts is the shared render styling for the works/doesn't-work triangle
// pair: dimensions, DOF coloring, conflict highlighting and a status badge, with
// a wide margin so the side dimensions stay inside the frame. DOF coloring is on
// so the "works" triangle reads as fully constrained (black) rather than the
// default stroke, which the gallery's color language would misread as free.
func triangleOpts() []sketch.SVGOption {
	return withAnn(
		sketch.WithMargin(64),
		sketch.WithDimensions(true),
		sketch.WithDOFColoring(true),
		sketch.WithConflicts(true),
		sketch.WithStatusBadge(true),
	)
}

// conflictValid is the "works" half of the pair: a 3-4-5 right triangle built
// from its three side lengths (SSS). The sides are consistent, so it solves and
// reports fully constrained.
func conflictValid() (string, error) {
	world := sketch.NewWorld()
	s, _ := world.CreateSketch(world.XY())
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(400, 0)
	c := s.CreatePoint(0, 300)
	ab := s.CreateLine(a, b)
	s.CreateLine(b, c)
	s.CreateLine(c, a)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab))
	s.AddConstraint(
		sketch.NewDistance(a, b, 400),
		sketch.NewDistance(c, a, 300),
		sketch.NewDistance(b, c, 500), // 3-4-5: consistent
	)
	s.Solve(context.Background())
	return s.SVG(triangleOpts()...)
}

// conflict is the "doesn't work" half: the same right triangle, whose legs (400,
// 300) and right angle force the hypotenuse to 500 — then the hypotenuse is
// dimensioned 600. We solve the consistent right triangle first (so the geometry
// is a clean, properly grounded right triangle — one anchor, no pinned interior
// points), then add the conflicting hypotenuse dimension. Rendering without a
// re-solve is the verification-oracle view: the intended geometry, with the
// unsatisfiable dimension flagged red, rather than a shape the solver has warped
// trying to reconcile the impossible. The badge reports overconstrained /
// unsolvable.
func conflict() (string, error) {
	world := sketch.NewWorld()
	s, _ := world.CreateSketch(world.XY())
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(400, 0)
	c := s.CreatePoint(0, 300)
	ab := s.CreateLine(a, b)
	s.CreateLine(b, c)
	ca := s.CreateLine(c, a)
	a.MoveTo(0, 0)
	s.Fix(a) // single grounding anchor + orientation; legs and angle are driven
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewPerpendicular(ab, ca))
	s.AddConstraint(
		sketch.NewDistance(a, b, 400),
		sketch.NewDistance(c, a, 300),
	)
	s.Solve(context.Background())                  // clean 3-4-5 right triangle, hypotenuse = 500
	s.AddConstraint(sketch.NewDistance(b, c, 600)) // real hypotenuse is 500 → conflict
	return s.SVG(triangleOpts()...)
}

// parametric builds a plate with a centered hole at the given width and renders
// it with dimensions, so before/after widths tell the parametric story.
func parametric(width float64) (string, error) {
	height := width * 0.6
	world := sketch.NewWorld()
	s, _ := world.CreateSketch(world.XY())
	r := s.CreateRectangle(0, 0, width, height)
	o := s.CreatePoint(width/2, height/2)
	hole := s.CreateCircle(o, width/8)
	r.A.MoveTo(0, 0)
	s.Fix(r.A)
	s.AddConstraint(
		sketch.NewDistance(r.A, r.B, width),
		sketch.NewDistance(r.A, r.D, height),
		sketch.NewHorizontalDistance(r.A, o, width/2),
		sketch.NewVerticalDistance(r.A, o, height/2),
		sketch.NewRadius(hole, width/8),
	)
	if _, err := s.Solve(context.Background()); err != nil {
		return "", err
	}
	return s.SVG(withAnn(sketch.WithDimensions(true))...)
}

func profiles() (string, error) {
	world := sketch.NewWorld()
	s, _ := world.CreateSketch(world.XY())
	r := s.CreateRectangle(0, 0, 200, 120)
	o := s.CreatePoint(100, 60)
	hole := s.CreateCircle(o, 32)
	r.A.MoveTo(0, 0)
	s.Fix(r.A)
	s.AddConstraint(
		sketch.NewDistance(r.A, r.B, 200),
		sketch.NewDistance(r.A, r.D, 120),
		sketch.NewHorizontalDistance(r.A, o, 100),
		sketch.NewVerticalDistance(r.A, o, 60),
		sketch.NewRadius(hole, 32),
	)
	if _, err := s.Solve(context.Background()); err != nil {
		return "", err
	}
	return s.SVG(withAnn(
		sketch.WithProfileFill(true),
		sketch.WithDOFColoring(true),
		sketch.WithStatusBadge(true),
	)...)
}

func filletBefore() (string, error) {
	s, _ := groundedRect(200, 120)
	if _, err := s.Solve(context.Background()); err != nil {
		return "", err
	}
	return s.SVG(withAnn(sketch.WithDOFColoring(true), sketch.WithStatusBadge(true))...)
}

func filletAfter() (string, error) {
	s, r := groundedRect(200, 120)
	if _, err := s.Solve(context.Background()); err != nil {
		return "", err
	}
	// The fillet removes edges BC (vertical) and CD (horizontal) with their
	// axis-alignment constraints, so re-pin the shortened legs to keep the sketch
	// fully constrained.
	f, err := s.CreateFillet(r.BC, r.CD, 40)
	if err != nil {
		return "", err
	}
	s.AddConstraint(sketch.NewVertical(f.L1), sketch.NewHorizontal(f.L2))
	if _, err := s.Solve(context.Background()); err != nil {
		return "", err
	}
	return s.SVG(withAnn(sketch.WithDOFColoring(true), sketch.WithStatusBadge(true))...)
}
