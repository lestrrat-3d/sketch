package sketch

import "math"

// Result reports the outcome of a [Sketch.Solve] call.
type Result struct {
	Converged  bool    // every residual is within Tolerance
	Iterations int     // outer Levenberg–Marquardt iterations performed
	Residual   float64 // Euclidean norm of the final residual vector
	DOF        int     // remaining degrees of freedom (0 == fully constrained)
	Redundant  int     // number of redundant/conflicting constraint equations
}

// SolveOptions tunes the constraint solver. Use [DefaultSolveOptions] as a
// starting point.
type SolveOptions struct {
	MaxIterations int     // outer iteration budget
	Tolerance     float64 // convergence threshold on the residual norm
}

// DefaultSolveOptions returns reasonable solver settings.
func DefaultSolveOptions() SolveOptions {
	return SolveOptions{MaxIterations: 200, Tolerance: 1e-10}
}

// Solve runs the constraint solver, moving non-grounded geometry until all
// constraints are satisfied. Optional settings override [DefaultSolveOptions].
//
// Solve returns [ErrNotConverged] (along with the partial [Result]) if the
// residuals cannot be driven below the tolerance within the iteration budget,
// which usually means the sketch is over-constrained or contradictory.
func (s *Sketch) Solve(opts ...SolveOptions) (*Result, error) {
	o := DefaultSolveOptions()
	if len(opts) > 0 {
		o = opts[0]
	}

	free := s.freeVars()
	n := len(free)

	r := s.residuals(nil)
	m := len(r)

	res := &Result{}
	if m == 0 {
		res.Converged = true
		res.DOF = n
		return res, nil
	}

	cost := dot(r, r) // sum of squared residuals
	lambda := 1e-3
	var iter int
	for iter = 0; iter < o.MaxIterations; iter++ {
		if math.Sqrt(cost) <= o.Tolerance {
			break
		}
		if n == 0 {
			break // nothing free to move
		}

		J := s.jacobian(free, m)
		// Normal equations: A = JᵀJ, g = Jᵀr.
		A := make([][]float64, n)
		g := make([]float64, n)
		for i := 0; i < n; i++ {
			A[i] = make([]float64, n)
			for j := 0; j < n; j++ {
				var sum float64
				for k := 0; k < m; k++ {
					sum += J[k][i] * J[k][j]
				}
				A[i][j] = sum
			}
			var gs float64
			for k := 0; k < m; k++ {
				gs += J[k][i] * r[k]
			}
			g[i] = gs
		}

		// Absolute damping scale. Using λ·max(diag) rather than λ·A[i][i]
		// regularizes every direction by the same amount, which keeps the
		// step well behaved (minimum-norm) for rank-deficient / under-
		// constrained systems where some diagonal entries are tiny.
		maxDiag := 0.0
		for i := 0; i < n; i++ {
			if A[i][i] > maxDiag {
				maxDiag = A[i][i]
			}
		}
		if maxDiag == 0 {
			maxDiag = 1
		}

		// Inner loop: adapt the damping λ until a step reduces the cost.
		improved := false
		for try := 0; try < 40; try++ {
			mu := lambda * maxDiag
			damped := make([][]float64, n)
			rhs := make([]float64, n)
			for i := 0; i < n; i++ {
				damped[i] = make([]float64, n)
				copy(damped[i], A[i])
				damped[i][i] += mu + 1e-12 // Levenberg damping + numerical floor
				rhs[i] = -g[i]
			}
			delta, ok := solveLinear(damped, rhs)
			if !ok {
				lambda *= 10
				continue
			}
			// Apply the trial step.
			for j, vi := range free {
				s.vars[vi] += delta[j]
			}
			rNew := s.residuals(nil)
			costNew := dot(rNew, rNew)
			if costNew < cost {
				cost = costNew
				r = rNew
				lambda *= 0.5
				improved = true
				break
			}
			// Reject: undo and increase damping.
			for j, vi := range free {
				s.vars[vi] -= delta[j]
			}
			lambda *= 10
			if lambda > 1e12 {
				break
			}
		}
		if !improved {
			break
		}
	}

	res.Iterations = iter
	res.Residual = math.Sqrt(cost)
	res.Converged = res.Residual <= o.Tolerance

	rank := s.rank(free, m)
	res.DOF = n - rank
	if res.DOF < 0 {
		res.DOF = 0
	}
	res.Redundant = m - rank
	if res.Redundant < 0 {
		res.Redundant = 0
	}

	if !res.Converged {
		return res, ErrNotConverged
	}
	return res, nil
}

// DOF reports the remaining degrees of freedom of the sketch at its current
// configuration (0 means fully constrained). It does not move any geometry.
func (s *Sketch) DOF() int {
	free := s.freeVars()
	m := len(s.residuals(nil))
	if m == 0 {
		return len(free)
	}
	d := len(free) - s.rank(free, m)
	if d < 0 {
		return 0
	}
	return d
}

func (s *Sketch) freeVars() []int {
	idx := make([]int, 0, len(s.vars))
	for i := range s.vars {
		if !s.fixed[i] {
			idx = append(idx, i)
		}
	}
	return idx
}

// residuals evaluates every constraint into a fresh slice (reusing buf's
// backing array when possible).
func (s *Sketch) residuals(buf []float64) []float64 {
	buf = buf[:0]
	for _, c := range s.cons {
		buf = c.residual(buf)
	}
	return buf
}

// jacobian computes the m×n Jacobian of the residuals w.r.t. the free
// variables using central finite differences.
func (s *Sketch) jacobian(free []int, m int) [][]float64 {
	n := len(free)
	J := make([][]float64, m)
	for i := range J {
		J[i] = make([]float64, n)
	}
	for j, vi := range free {
		orig := s.vars[vi]
		h := 1e-7 * (1 + math.Abs(orig))
		s.vars[vi] = orig + h
		rp := s.residuals(nil)
		s.vars[vi] = orig - h
		rm := s.residuals(nil)
		s.vars[vi] = orig
		inv := 1.0 / (2 * h)
		for i := 0; i < m; i++ {
			J[i][j] = (rp[i] - rm[i]) * inv
		}
	}
	return J
}

// rank estimates the rank of the Jacobian at the current configuration via
// Gaussian elimination with partial pivoting.
func (s *Sketch) rank(free []int, m int) int {
	J := s.jacobian(free, m)
	n := len(free)
	const eps = 1e-9
	row := 0
	for col := 0; col < n && row < m; col++ {
		// Find a pivot in this column at or below the current row.
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
		for r := 0; r < m; r++ {
			if r == row {
				continue
			}
			f := J[r][col] / J[row][col]
			if f == 0 {
				continue
			}
			for c := col; c < n; c++ {
				J[r][c] -= f * J[row][c]
			}
		}
		row++
	}
	return row
}

// solveLinear solves A·x = b for a square matrix using Gaussian elimination
// with partial pivoting. A and b are not modified. ok is false if A is
// singular.
func solveLinear(A [][]float64, b []float64) (x []float64, ok bool) {
	n := len(b)
	M := make([][]float64, n)
	for i := range M {
		M[i] = make([]float64, n+1)
		copy(M[i], A[i])
		M[i][n] = b[i]
	}
	for col := 0; col < n; col++ {
		piv := col
		best := math.Abs(M[col][col])
		for r := col + 1; r < n; r++ {
			if v := math.Abs(M[r][col]); v > best {
				best = v
				piv = r
			}
		}
		if best < 1e-15 {
			return nil, false
		}
		M[col], M[piv] = M[piv], M[col]
		for r := col + 1; r < n; r++ {
			f := M[r][col] / M[col][col]
			for c := col; c <= n; c++ {
				M[r][c] -= f * M[col][c]
			}
		}
	}
	x = make([]float64, n)
	for i := n - 1; i >= 0; i-- {
		sum := M[i][n]
		for c := i + 1; c < n; c++ {
			sum -= M[i][c] * x[c]
		}
		x[i] = sum / M[i][i]
	}
	return x, true
}

func dot(a, b []float64) float64 {
	var s float64
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}
