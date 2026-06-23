package examples_test

import (
	"fmt"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_reference shows the sketch/3D separation keystone: the layer
// above hands in 3D-derived geometry (a projected edge) as a locked reference
// snapshot with a source id, the sketch is verified *against* it, and staleness
// makes the oracle refuse to bless a sketch built on an out-of-date snapshot
// until the 3D layer re-feeds it.
func Example_sketch_reference() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())

	// A projected edge of a 3D body, handed in as plane-local coordinates plus a
	// source id. Reference geometry is locked — the solver never moves it.
	a := s.AddReferencePoint(0, 0, "body1.edge7")
	b := s.AddReferencePoint(10, 0, "body1.edge7")
	if _, err := s.AddReferenceLine(a, b, "body1.edge7"); err != nil {
		fmt.Println(err)
		return
	}

	// A sketch point pierced to the projected vertex (coincident), fully pinning
	// the sketch against the reference.
	tip := s.AddPoint(4, 1)
	s.AddConstraint(sketch.NewCoincident(tip, a))
	if _, err := s.Solve(); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("tip pierced to (%.0f, %.0f)\n", tip.X(), tip.Y())

	rep := s.Verify()
	fmt.Printf("trustworthy=%t stale=%t\n", rep.Trustworthy(), rep.Stale)

	// The 3D body changes: the snapshot is now stale, so the verdict is no longer
	// trustworthy even though the sketch is still fully constrained and solvable.
	s.MarkStale("body1.edge7")
	rep = s.Verify()
	fmt.Printf("after change: trustworthy=%t stale=%t stale-references=%d\n",
		rep.Trustworthy(), rep.Stale, len(rep.StaleReferences))

	// The 3D layer re-feeds the snapshot; trust is restored.
	s.RefreshReference(a, 0, 0)
	s.RefreshReference(b, 10, 0)
	rep = s.Verify()
	fmt.Printf("after refresh: trustworthy=%t stale=%t\n", rep.Trustworthy(), rep.Stale)

	// Output:
	// tip pierced to (0, 0)
	// trustworthy=true stale=false
	// after change: trustworthy=false stale=true stale-references=1
	// after refresh: trustworthy=true stale=false
}
