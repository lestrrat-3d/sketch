package examples_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lestrrat-3d/sketch"
)

// Example_sketch_timeout bounds a solve with a context deadline — the defense
// against a hostile sketch (see SECURITY.md). A document parsed from an
// untrusted source may be pathologically expensive to solve, so the solve is
// given a context: cancellation or an expired deadline aborts it at iteration
// granularity, capping the CPU time crafted input can consume. Pairing the
// deadline with WithMaxIterations bounds solve time from both directions.
// Verify (and World.Verify) take a context the same way.
func Example_sketch_timeout() {
	w := sketch.NewWorld()
	s, _ := w.CreateSketch(w.XY())
	a := s.CreatePoint(0, 0)
	b := s.CreatePoint(30, 4)
	a.MoveTo(0, 0)
	s.Fix(a)
	l := s.CreateLine(a, b)
	s.AddConstraint(sketch.NewHorizontal(l))
	s.AddConstraint(sketch.NewDistance(a, b, 30))

	// A well-behaved sketch converges well within a generous deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := s.Solve(ctx, sketch.WithMaxIterations(200))
	fmt.Printf("bounded solve: converged=%t err=%v\n", res.Converged, err)

	// An exhausted deadline aborts the solve with a context error, so hostile
	// input cannot spin the CPU past the budget. The error wraps ctx.Err(), and
	// the analysis fields are marked not-computed (DOF -1) rather than a
	// misleading zero.
	expired, cancelExpired := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer cancelExpired()
	res, err = s.Solve(expired)
	fmt.Printf("expired deadline: deadlineExceeded=%t DOF=%d\n",
		errors.Is(err, context.DeadlineExceeded), res.DOF)

	// Output:
	// bounded solve: converged=true err=<nil>
	// expired deadline: deadlineExceeded=true DOF=-1
}
