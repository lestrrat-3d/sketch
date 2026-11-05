package sketch

// transposeInto fills jt with the column-major transpose of the m×n row-major
// matrix j: jt[c*m+r] holds j[r][c], so column c of j lands in the contiguous
// run jt[c*m:(c+1)*m]. jt must hold at least m*n entries and j at least m rows
// of n columns; [lmWorkspace.alloc] sizes both from the same m and n.
//
// It only MOVES float64 values, so it introduces no arithmetic and cannot
// perturb a single bit of what the caller then computes from jt.
func transposeInto(jt []float64, j [][]float64, m, n int) {
	for r := 0; r < m; r++ {
		row := j[r]
		for c := 0; c < n; c++ {
			jt[c*m+r] = row[c]
		}
	}
}

// normalEquationsInto accumulates the normal equations of the least-squares
// step into the caller's buffers: a = JᵀJ (n×n) and g = Jᵀr (n), read from the
// column-major transpose jt that [transposeInto] produced and the residual
// vector r. a is fully overwritten and g likewise, so neither needs clearing
// between iterations.
//
// Every entry is a dot product of two Jacobian COLUMNS, and jt holds each
// column as one contiguous run — the reason the transpose is worth its own
// pass. Reading the columns out of the row-major J instead walks m separately
// allocated rows per entry, which is a cache miss per term rather than per line
// and is what dominated the solver profile.
//
// The arithmetic is deliberately unchanged from the row-major double loop this
// replaces, term for term and in the same ascending k: same two factors, same
// running sum from positive zero, no compensated summation, no skipped zero
// terms, no reassociation. Float addition is neither associative nor
// commutative, so any of those would move result bits; the equivalence is
// pinned bit for bit by TestNormalEquationsColumnLayoutMatchesReference.
//
// a is symmetric, so only its upper triangle is accumulated and each sum is
// mirrored into the lower one. IEEE multiplication is commutative and the
// order over k does not depend on i or j, so the mirrored entry is bit-
// identical to what a full double loop would have computed for (j, i).
//
// [Sketch.lm] no longer calls this directly: [normalEquationsSparseInto] does
// the same accumulation over the Jacobian's nonzeros and falls back here on
// non-finite input. This stays the definition of the result both must produce,
// and the "no skipped zero terms" rule above is a rule about THIS loop — the
// sparse kernel's doc comment says on what grounds it may skip.
func normalEquationsInto(a [][]float64, g []float64, jt []float64, r []float64, m, n int) {
	for i := 0; i < n; i++ {
		ci := jt[i*m : (i+1)*m]
		for j := i; j < n; j++ {
			cj := jt[j*m : (j+1)*m]
			var sum float64
			for k := 0; k < m; k++ {
				sum += ci[k] * cj[k]
			}
			a[i][j] = sum
			a[j][i] = sum
		}
		var gs float64
		for k := 0; k < m; k++ {
			gs += ci[k] * r[k]
		}
		g[i] = gs
	}
}

// normalEquationsSparseInto accumulates the same a = JᵀJ and g = Jᵀr that
// [normalEquationsInto] does, over the STRUCTURAL NONZEROS of each Jacobian
// column only, and produces bit-identical results on finite input. A sketch
// Jacobian is sparse — a constraint's residual row touches only the few
// variables of the geometry it references — so most of the products the dense
// kernel forms are exact zeros it spends a multiply and an add on anyway.
//
// The equivalence is exact, not approximate, and rests on two facts. Adding an
// exact zero to a float64 that is not negative zero returns that float64
// unchanged, for every finite value and for both infinities. And the running
// sum starts at positive zero and can never BECOME negative zero: round-to-
// nearest returns positive zero for every exact cancellation, and positive zero
// plus either signed zero is positive zero. So a term whose product is exactly
// zero contributes nothing to the sum it would have joined, and dropping it
// leaves every remaining term in its original ascending-k position. That is why
// this kernel may skip terms where the doc comment on normalEquationsInto
// forbids it: skipping is not a reassociation.
//
// A product is exactly zero only when one factor is zero and the OTHER IS
// FINITE — zero times infinity is NaN, which the dense kernel would propagate
// and this one would drop. Hence the finiteness guard: a NaN or infinity
// anywhere in jt or r sends the whole call to the dense kernel, whose result is
// then the one bit pattern both paths agree on. The `!= 0` tests treat negative
// zero as zero, which is correct — its products are exact zeros too.
//
// The pattern is rebuilt from scratch on every call. It is NOT carried over
// from the previous iteration: the Jacobian is a finite-difference
// approximation, so an entry that was exactly zero for one variable position
// can be nonzero at the next, and a stale pattern would silently drop a real
// term.
func normalEquationsSparseInto(ws *lmWorkspace, m, n int) {
	jt, r := ws.JT, ws.r
	nonzeros := 0
	for i := 0; i < m*n; i++ {
		if nonFinite(jt[i]) {
			normalEquationsInto(ws.A, ws.g, jt, r, m, n)
			return
		}
		if jt[i] != 0 {
			nonzeros++
		}
	}
	for k := 0; k < m; k++ {
		if nonFinite(r[k]) {
			normalEquationsInto(ws.A, ws.g, jt, r, m, n)
			return
		}
	}
	// Dense columns do not benefit from building row lists and bitmaps. The
	// finite-value scan above already counts their structural nonzeros, so use
	// the reference kernel once at least half the matrix is populated.
	if nonzeros*2 >= m*n {
		normalEquationsInto(ws.A, ws.g, jt, r, m, n)
		return
	}

	// Column i's nonzero rows, read off the contiguous column runs of jt.
	colRows, colStart := ws.colRows[:0], ws.colStart[:n+1]
	for i := 0; i < n; i++ {
		colStart[i] = len(colRows)
		ci := jt[i*m : (i+1)*m]
		for k := 0; k < m; k++ {
			if ci[k] != 0 {
				colRows = append(colRows, k)
			}
		}
	}
	colStart[n] = len(colRows)
	ws.colRows = colRows

	// The same pattern as one bitmap per column, so the "share a row?" question
	// below costs a handful of word ANDs rather than a walk over either column.
	words := ws.colWords
	colBits := ws.colBits[:n*words]
	clear(colBits)
	for i := 0; i < n; i++ {
		bits := colBits[i*words : (i+1)*words]
		for _, k := range colRows[colStart[i]:colStart[i+1]] {
			bits[k/64] |= 1 << (k % 64)
		}
	}

	for i := 0; i < n; i++ {
		ci := jt[i*m : (i+1)*m]
		rowsI := colRows[colStart[i]:colStart[i+1]]
		bi := colBits[i*words : (i+1)*words]

		for j := i; j < n; j++ {
			// Every slot this column owns is written on both sides of the
			// diagonal, whether or not the pair contributes anything: the mirror
			// a[j][i] is written nowhere else, since column j only ever reaches
			// a[j][c] and a[c][j] for c >= j. Skipping the write on a pair that
			// shares no row would leave the previous iteration's sum in place.
			bj := colBits[j*words : (j+1)*words]
			shares := false
			for w := range bi {
				if bi[w]&bj[w] != 0 {
					shares = true
					break
				}
			}
			if !shares {
				// Every term of this sum is an exact zero added to a sum that
				// started at positive zero, so the dense kernel writes positive
				// zero here too.
				ws.A[i][j] = 0
				ws.A[j][i] = 0
				continue
			}

			// Only column i's nonzero rows are walked, in ascending order. Where
			// column j is zero at one of them the product is an exact zero, so
			// including it is the identity this whole kernel rests on — testing
			// for it would cost more than the multiply it saves.
			//
			// A column with no zero at all takes the straight-through loop
			// instead: the same terms in the same order, read without the index
			// indirection. A real sketch produces such a column whenever one
			// variable is touched by every residual row.
			cj := jt[j*m : (j+1)*m]
			var sum float64
			if len(rowsI) == m {
				for k := 0; k < m; k++ {
					sum += ci[k] * cj[k]
				}
			} else {
				for _, k := range rowsI {
					sum += ci[k] * cj[k]
				}
			}
			ws.A[i][j] = sum
			ws.A[j][i] = sum
		}
		// The diagonal needs no special case: a column with any nonzero shares a
		// row with itself, and an all-zero column takes the positive-zero branch,
		// which is what the dense kernel computes for it.

		var gs float64
		for _, k := range rowsI {
			gs += ci[k] * r[k]
		}
		ws.g[i] = gs
	}
}

// buildStructuralSupport records every residual row a free variable's
// constraints own. A successful local Jacobian may still contain numeric zeros
// in these slots, but it writes exact zeros outside them. The support therefore
// remains valid as the numeric values change during this one lm call.
func buildStructuralSupport(ws *lmWorkspace, plan *residualPlan, free []int, m, n int) bool {
	ws.structuralReady = false
	ws.structPairCols = ws.structPairCols[:0]
	rows := ws.structRows[:0]
	starts := ws.structStart[:n+1]
	for col, vi := range free {
		if vi < 0 || vi+1 >= len(plan.varStart) {
			return false
		}
		starts[col] = len(rows)
		for _, k := range plan.consFor(vi) {
			if k < 0 || k >= len(plan.offset) || k >= len(plan.count) {
				return false
			}
			off, count := plan.offset[k], plan.count[k]
			if off < 0 || count < 0 || off+count > m {
				return false
			}
			for row := off; row < off+count; row++ {
				rows = append(rows, row)
			}
		}
	}
	starts[n] = len(rows)
	ws.structRows = rows
	ws.structStart = starts
	ws.structDensity = len(rows)

	words := ws.colWords
	bits := ws.structBits[:n*words]
	clear(bits)
	for col := 0; col < n; col++ {
		colBits := bits[col*words : (col+1)*words]
		for _, row := range rows[starts[col]:starts[col+1]] {
			colBits[row/64] |= 1 << (row % 64)
		}
	}
	ws.structuralReady = true
	if ws.structDensity*2 >= m*n {
		return true
	}

	pairCols, pairStart := ws.structPairCols[:0], ws.structPairStart
	for i := 0; i < n; i++ {
		pairStart[i] = len(pairCols)
		bi := bits[i*words : (i+1)*words]
		for j := i; j < n; j++ {
			bj := bits[j*words : (j+1)*words]
			for w := range bi {
				if bi[w]&bj[w] != 0 {
					pairCols = append(pairCols, j)
					break
				}
			}
		}
	}
	pairStart[n] = len(pairCols)
	ws.structPairCols = pairCols
	return true
}

// normalEquationsStructuralInto uses the structural superset built from a
// successful local residual plan. It checks every potentially nonzero Jacobian
// slot and every residual for nonfinite values before doing arithmetic; an
// exceptional input keeps the existing dense fallback. Every row inside the
// superset is accumulated, including finite numeric zeros, in ascending row
// order so the result matches normalEquationsInto bit for bit. Supported pairs
// are precomputed once from structBits; the kernel clears A and visits only
// those pairs in ascending order.
func normalEquationsStructuralInto(ws *lmWorkspace, m, n int) {
	jt, r := ws.JT, ws.r
	rows, starts := ws.structRows, ws.structStart
	for col := 0; col < n; col++ {
		for _, row := range rows[starts[col]:starts[col+1]] {
			if nonFinite(jt[col*m+row]) {
				normalEquationsInto(ws.A, ws.g, jt, r, m, n)
				return
			}
		}
	}
	for row := 0; row < m; row++ {
		if nonFinite(r[row]) {
			normalEquationsInto(ws.A, ws.g, jt, r, m, n)
			return
		}
	}
	if ws.structDensity*2 >= m*n {
		normalEquationsInto(ws.A, ws.g, jt, r, m, n)
		return
	}

	clear(ws.aBack[:n*n])
	pairCols, pairStart := ws.structPairCols, ws.structPairStart
	for i := 0; i < n; i++ {
		ci := jt[i*m : (i+1)*m]
		rowsI := rows[starts[i]:starts[i+1]]
		for _, j := range pairCols[pairStart[i]:pairStart[i+1]] {
			cj := jt[j*m : (j+1)*m]
			var sum float64
			for _, row := range rowsI {
				sum += ci[row] * cj[row]
			}
			ws.A[i][j] = sum
			ws.A[j][i] = sum
		}
		var gs float64
		for _, row := range rowsI {
			gs += ci[row] * r[row]
		}
		ws.g[i] = gs
	}
}
