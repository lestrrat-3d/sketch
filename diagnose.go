package sketch

import (
	"errors"
	"fmt"
	"math"
)

// ErrOverconstrained is returned (wrapped) by [Sketch.CheckConstraint] when a
// candidate constraint would not add an independent equation — committing it
// would make the sketch redundant or conflicting.
var ErrOverconstrained = errors.New("sketch: constraint would over-constrain the sketch")

// conflictTol separates a satisfied residual from a violated one when
// partitioning dependent constraints in [Sketch.Diagnose]. Residuals are
// unit-normalized (length units or dimensionless), and a converged solve
// leaves them at or below the solver tolerance (1e-10), so anything above
// this is a genuine violation rather than numerical noise.
const conflictTol = 1e-8

// Diagnosis partitions the constraints that contribute dependent equations at
// the current configuration. See [Sketch.Diagnose].
type Diagnosis struct {
	// Redundant constraints duplicate information the rest of the sketch
	// already pins down, and currently hold: removing one changes nothing.
	Redundant []Constraint
	// Conflicting constraints are dependent and violated: they fight other
	// constraints over the same degrees of freedom and the solver cannot
	// satisfy everything at once. At least one constraint in each conflict
	// must be removed or edited.
	Conflicting []Constraint
}

// Diagnose classifies the sketch's dependent constraints as redundant
// (consistent duplicates) or conflicting (mutually unsatisfiable), refining
// the flat list returned by [Sketch.RedundantConstraints]. Like that method
// it analyses the call-time configuration and follows creation order — of two
// constraints fighting over the same equation, the later-added one is
// reported. Call it after [Sketch.Solve]: a converged solve leaves dependent-
// but-satisfied constraints (redundant), a failed solve leaves dependent
// constraints with residuals the solver could not remove (conflicting).
func (s *Sketch) Diagnose() *Diagnosis {
	d := &Diagnosis{}
	for _, c := range s.RedundantConstraints() {
		if maxAbsResidual(c) > conflictTol {
			d.Conflicting = append(d.Conflicting, c)
			continue
		}
		d.Redundant = append(d.Redundant, c)
	}
	return d
}

func maxAbsResidual(c Constraint) float64 {
	var worst float64
	for _, r := range c.residual(nil) {
		if v := math.Abs(r); v > worst {
			worst = v
		}
	}
	return worst
}

// CheckConstraint reports whether committing c would over-constrain the
// sketch, without committing anything. It returns nil when every equation c
// contributes is independent of the existing constraints at the current
// configuration, and an error wrapping [ErrOverconstrained] otherwise — both
// a consistent duplicate and a contradiction are rejected, since neither adds
// an equation the sketch can use. Driven dimensions contribute no equations
// and always pass.
//
// Like [Sketch.DOF], the analysis is local to the call-time configuration;
// check against solved geometry (after [Sketch.Solve]) for the most reliable
// verdict. A caller that wants Fusion's behavior — refuse the gesture, leave
// the sketch untouched — calls this before [Sketch.AddConstraint].
func (s *Sketch) CheckConstraint(c Constraint) error {
	if d, ok := c.(Dimension); ok && d.Driven() {
		return nil // measures the geometry, constrains nothing
	}
	k := len(c.residual(nil))
	if k == 0 {
		return nil
	}
	free := s.freeVars()
	m0 := len(s.residuals(nil))
	var r0 int
	if m0 > 0 {
		r0 = s.rankOf(free, m0, s.residuals)
	}
	aug := func(buf []float64) []float64 { return c.residual(s.residuals(buf)) }
	r1 := s.rankOf(free, m0+k, aug)
	if r1 < r0+k {
		return fmt.Errorf("%w: %d of its %d equations depend on existing constraints", ErrOverconstrained, r0+k-r1, k)
	}
	return nil
}

// FreePoints reports which points can still move — the under-constrained
// remainder of the sketch, in id order. A point is free when some first-order
// motion compatible with every constraint displaces it: the union of supports
// of a null-space basis of the constraint Jacobian at the current
// configuration. Grounded points are never free. An empty result on a solved
// sketch means it is fully constrained (DOF 0); this is the engine-level
// answer to "which geometry would a GUI color as under-constrained".
func (s *Sketch) FreePoints() []*Point {
	movable := s.movableVars()
	var out []*Point
	for _, p := range s.points {
		if _, ok := movable[p.xi]; ok {
			out = append(out, p)
			continue
		}
		if _, ok := movable[p.yi]; ok {
			out = append(out, p)
		}
	}
	return out
}

// IsFullyConstrained reports whether the point cannot move without violating
// a constraint (or is grounded). It is the per-point view of
// [Sketch.FreePoints].
func (p *Point) IsFullyConstrained() bool {
	movable := p.s.movableVars()
	if _, ok := movable[p.xi]; ok {
		return false
	}
	_, ok := movable[p.yi]
	return !ok
}

// movableVars identifies the free variables with a nonzero component in some
// null-space direction of the constraint Jacobian — the variables a
// constraint-preserving motion can change. Computed by reducing the Jacobian
// to reduced row-echelon form: each non-pivot column seeds a null-space basis
// vector with support on itself and on every pivot column its elimination
// touches.
func (s *Sketch) movableVars() map[int]struct{} {
	free := s.freeVars()
	movable := make(map[int]struct{})
	m := len(s.residuals(nil))
	if m == 0 {
		for _, vi := range free {
			movable[vi] = struct{}{}
		}
		return movable
	}

	J := s.jacobian(free, m, s.residuals)
	n := len(free)
	const eps = 1e-9
	isPivot := make([]bool, n)
	var pivotCols []int // pivotCols[r] = pivot column of row r
	row := 0
	for col := 0; col < n && row < m; col++ {
		piv := row
		best := math.Abs(J[row][col])
		for r := row + 1; r < m; r++ {
			if v := math.Abs(J[r][col]); v > best {
				best = v
				piv = r
			}
		}
		if best < eps {
			continue
		}
		J[row], J[piv] = J[piv], J[row]
		inv := 1 / J[row][col]
		for c := col; c < n; c++ {
			J[row][c] *= inv
		}
		for r := 0; r < m; r++ {
			if r == row {
				continue
			}
			f := J[r][col]
			if f == 0 {
				continue
			}
			for c := col; c < n; c++ {
				J[r][c] -= f * J[row][c]
			}
		}
		isPivot[col] = true
		pivotCols = append(pivotCols, col)
		row++
	}

	for j := 0; j < n; j++ {
		if isPivot[j] {
			continue
		}
		// Null vector: x[j] = 1, x[pivotCols[r]] = -J[r][j].
		movable[free[j]] = struct{}{}
		for r := 0; r < row; r++ {
			if math.Abs(J[r][j]) > eps {
				movable[free[pivotCols[r]]] = struct{}{}
			}
		}
	}
	return movable
}
