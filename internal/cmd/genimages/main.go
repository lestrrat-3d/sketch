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
	"quickstart":           quickstart,
	"hexagon":              hexagon,
	"dof-underconstrained": dofUnder,
	"dof-constrained":      dofFull,
	"conflict":             conflict,
	"parametric-before":    func() (string, error) { return parametric(120) },
	"parametric-after":     func() (string, error) { return parametric(200) },
	"profiles":             profiles,
	"fillet-before":        filletBefore,
	"fillet-after":         filletAfter,
}

// annStyle is the shared annotation styling for the gallery.
var annStyle = []sketch.SVGOption{sketch.WithMargin(8)}

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
	s, _ := groundedRect(40, 24)
	if _, err := s.Solve(); err != nil {
		return "", err
	}
	return s.SVG(withAnn(sketch.WithDimensions(true), sketch.WithConstraints(true))...)
}

func hexagon() (string, error) {
	world := sketch.NewWorld()
	s, _ := world.CreateSketch(world.XY())
	poly, err := s.CreatePolygon(0, 0, 6, 24)
	if err != nil {
		return "", err
	}
	poly.Center.MoveTo(0, 0)
	s.Fix(poly.Center)
	if _, err := s.Solve(); err != nil {
		return "", err
	}
	return s.SVG(withAnn(sketch.WithConstraints(true))...)
}

// underRect builds a rectangle whose top-right corner is left free.
func underRect() *sketch.Sketch {
	world := sketch.NewWorld()
	s, _ := world.CreateSketch(world.XY())
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(40, 0)
	c := s.CreatePoint(40, 24)
	d := s.CreatePoint(0, 24)
	ab := s.CreateLine(a, b)
	s.CreateLine(b, c)
	s.CreateLine(c, d)
	da := s.CreateLine(d, a)
	a.MoveTo(0, 0)
	s.Fix(a)
	s.AddConstraint(sketch.NewHorizontal(ab), sketch.NewVertical(da), sketch.NewDistance(a, b, 40))
	s.Solve()
	return s
}

func dofUnder() (string, error) {
	return underRect().SVG(withAnn(sketch.WithDOFColoring(true), sketch.WithStatusBadge(true))...)
}

func dofFull() (string, error) {
	s, _ := groundedRect(40, 24)
	if _, err := s.Solve(); err != nil {
		return "", err
	}
	return s.SVG(withAnn(sketch.WithDOFColoring(true), sketch.WithStatusBadge(true))...)
}

func conflict() (string, error) {
	world := sketch.NewWorld()
	s, _ := world.CreateSketch(world.XY())
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(40, 0)
	a.MoveTo(0, 0)
	s.Fix(a)
	l := s.CreateLine(a, b)
	s.AddConstraint(sketch.NewHorizontal(l))
	// Two distances fight over the same span.
	s.AddConstraint(sketch.NewDistance(a, b, 40), sketch.NewDistance(a, b, 28))
	s.Solve() // will not converge; render the conflict anyway
	return s.SVG(withAnn(sketch.WithConflicts(true), sketch.WithStatusBadge(true))...)
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
	if _, err := s.Solve(); err != nil {
		return "", err
	}
	return s.SVG(withAnn(sketch.WithDimensions(true))...)
}

func profiles() (string, error) {
	world := sketch.NewWorld()
	s, _ := world.CreateSketch(world.XY())
	r := s.CreateRectangle(0, 0, 40, 24)
	o := s.CreatePoint(20, 12)
	hole := s.CreateCircle(o, 6)
	r.A.MoveTo(0, 0)
	s.Fix(r.A)
	s.AddConstraint(
		sketch.NewDistance(r.A, r.B, 40),
		sketch.NewDistance(r.A, r.D, 24),
		sketch.NewHorizontalDistance(r.A, o, 20),
		sketch.NewVerticalDistance(r.A, o, 12),
		sketch.NewRadius(hole, 6),
	)
	if _, err := s.Solve(); err != nil {
		return "", err
	}
	return s.SVG(withAnn(sketch.WithProfileFill(true))...)
}

func filletBefore() (string, error) {
	s, _ := groundedRect(40, 24)
	if _, err := s.Solve(); err != nil {
		return "", err
	}
	return s.SVG(annStyle...)
}

func filletAfter() (string, error) {
	s, r := groundedRect(40, 24)
	if _, err := s.Solve(); err != nil {
		return "", err
	}
	if _, err := s.CreateFillet(r.BC, r.CD, 8); err != nil {
		return "", err
	}
	if _, err := s.Solve(); err != nil {
		return "", err
	}
	return s.SVG(annStyle...)
}
