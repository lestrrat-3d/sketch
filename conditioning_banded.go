package sketch

import (
	"cmp"
	"math"
	"slices"
)

// The banded singular-value path behind [singularValueExtremes].
//
// The dense kernel is O(m·n²) in the number of free variables n, which is what
// made a ring-shaped sketch (a gear profile: thousands of points, each tied to
// its neighbours) cost minutes in [Sketch.Verify]. Those Jacobians are sparse,
// and once the columns are renumbered by a bandwidth-reducing ordering they are
// narrow-banded: a row only touches a few neighbouring points. Singular values
// are unchanged by a column permutation and by orthogonal transforms, so the
// extremes can be read off a banded reduction of the same matrix at
// O(m·β² + n²·β) for bandwidth β, followed by the same Sturm bisection the dense
// kernel ends with.
//
// The path is an optimization of the dense one, never a different measure. It
// declines (second result false) wherever the dense kernel must answer: a
// non-finite entry, fewer rows than columns, a matrix too small or too wide-banded
// for the saving to matter. Small sketches therefore keep the dense kernel's
// results bit for bit.

const (
	// bandedMinCols is the smallest column count the banded path takes. Below it
	// the dense kernel is fast enough that keeping its exact results matters more.
	bandedMinCols = 256
	// bandedMaxRowNNZ bounds a row's nonzero count. The column graph links every
	// pair of columns a row touches, so a dense row would cost its square.
	bandedMaxRowNNZ = 64
	// bandedWidthDiv requires the band (β+1) to be at most 1/bandedWidthDiv of the
	// column count, which is where the banded reduction clearly beats the dense one.
	bandedWidthDiv = 8
	// bandedDropTol is the relative size below which a row's remainder, after it has
	// been rotated against the factor, is treated as rounding noise of a dependent
	// row and dropped. The change to any singular value is at most this multiple of
	// the row norm, the same order as the backward error of the reduction itself.
	bandedDropTol = 16 * dblEps
)

// bandedSingularValueExtremes returns σ_max and σ_min of A exactly as
// [singularValueExtremes] defines them, for a sparse, narrow-banded A with
// m ≥ n ≥ [bandedMinCols]. The third result is false when A does not qualify;
// the caller then uses the dense kernel. A is not modified.
//
// The pipeline is: order the columns by reverse Cuthill–McKee on the column
// graph (two columns are adjacent when a row touches both), sort the rows by
// their first column, triangularize with Givens rotations into a banded R, reduce
// that band to bidiagonal form by bulge chasing, and bisect.
func bandedSingularValueExtremes(A [][]float64) (float64, float64, bool) {
	m := len(A)
	if m == 0 || len(A[0]) < bandedMinCols || m < len(A[0]) {
		return 0, 0, false
	}
	n := len(A[0])

	// Row patterns. A non-finite entry is the dense kernel's NaN path.
	start := make([]int, m+1)
	var cols []int
	for i, row := range A {
		for j, v := range row {
			if v == 0 {
				continue
			}
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return 0, 0, false
			}
			cols = append(cols, j)
		}
		start[i+1] = len(cols)
		if start[i+1]-start[i] > bandedMaxRowNNZ {
			return 0, 0, false
		}
	}

	adj := make([][]int, n)
	for i := 0; i < m; i++ {
		rc := cols[start[i]:start[i+1]]
		for _, a := range rc {
			for _, b := range rc {
				if a != b {
					adj[a] = append(adj[a], b)
				}
			}
		}
	}
	for j := range adj {
		slices.Sort(adj[j])
		adj[j] = slices.Compact(adj[j])
	}
	pos := reverseCuthillMcKee(adj)

	type rowSpan struct{ lo, hi, idx int }
	spans := make([]rowSpan, 0, m)
	beta := 0
	for i := 0; i < m; i++ {
		rc := cols[start[i]:start[i+1]]
		if len(rc) == 0 {
			continue
		}
		lo, hi := n, -1
		for _, c := range rc {
			lo, hi = min(lo, pos[c]), max(hi, pos[c])
		}
		spans = append(spans, rowSpan{lo, hi, i})
		beta = max(beta, hi-lo)
	}
	// Fewer nonzero rows than columns is rank deficient: the dense kernel reports
	// that case, and it is not the one this path exists for.
	if len(spans) < n || (beta+1)*bandedWidthDiv > n {
		return 0, 0, false
	}
	slices.SortStableFunc(spans, func(a, b rowSpan) int {
		if c := cmp.Compare(a.lo, b.lo); c != 0 {
			return c
		}
		return cmp.Compare(a.hi, b.hi)
	})

	// Givens QR. R row c keeps columns c..c+β (window width w). A row is rotated
	// against R rows from its first column on; it ends where it meets a column no
	// earlier row has claimed, becoming that R row. Its support never passes
	// c+β when it reaches column c, because it starts within β of its first column
	// and each rotation only mixes in a window that ends at c+β.
	w := beta + 1
	R := make([]float64, n*w)
	occupied := make([]bool, n)
	t := make([]float64, n+w)
	for _, sp := range spans {
		row := A[sp.idx]
		var norm2 float64
		for _, c := range cols[start[sp.idx]:start[sp.idx+1]] {
			v := row[c]
			t[pos[c]] = v
			norm2 += v * v
		}
		tol := bandedDropTol * math.Sqrt(norm2)
		hi := sp.hi
		for c := sp.lo; c <= hi; c++ {
			x := t[c]
			if x == 0 {
				continue
			}
			end := min(c+beta, n-1)
			rrow := R[c*w : c*w+end-c+1]
			if !occupied[c] {
				copy(rrow, t[c:end+1])
				clear(t[c : hi+1])
				occupied[c] = true
				break
			}
			rho := math.Hypot(rrow[0], x)
			cs, sn := rrow[0]/rho, x/rho
			rrow[0], t[c] = rho, 0
			for k := 1; k < len(rrow); k++ {
				rj, tj := rrow[k], t[c+k]
				rrow[k] = cs*rj + sn*tj
				t[c+k] = cs*tj - sn*rj
			}
			hi = max(hi, end)
			var rest float64
			for _, v := range t[c+1 : hi+1] {
				rest = max(rest, math.Abs(v))
			}
			if rest <= tol {
				clear(t[c+1 : hi+1])
				break
			}
		}
	}

	d, e := bandToBidiagonal(R, n, beta)
	smax, smin := bidiagonalExtremes(d, e)
	return smax, smin, true
}

// reverseCuthillMcKee returns, for each node of the undirected graph adj (sorted,
// deduplicated neighbour lists), its position in a bandwidth-reducing order. Each
// connected component is entered at a pseudo-peripheral node, visited breadth
// first with neighbours taken lowest degree first, and the whole sequence is then
// reversed. A ring numbered around its circumference folds into a band about
// twice as wide as one row's reach, which is the case this order exists for.
// Ties break by node index, so the order is deterministic.
func reverseCuthillMcKee(adj [][]int) []int {
	n := len(adj)
	mark := make([]int, n)
	gen := 0
	queue := make([]int, 0, n)
	// farthest returns a lowest-degree node on the last breadth-first level from
	// root, and that level's depth.
	farthest := func(root int) (int, int) {
		gen++
		queue = append(queue[:0], root)
		mark[root] = gen
		levelStart, depth := 0, 0
		for {
			levelEnd := len(queue)
			for _, u := range queue[levelStart:levelEnd] {
				for _, v := range adj[u] {
					if mark[v] != gen {
						mark[v] = gen
						queue = append(queue, v)
					}
				}
			}
			if len(queue) == levelEnd {
				best := queue[levelStart]
				for _, u := range queue[levelStart:levelEnd] {
					if len(adj[u]) < len(adj[best]) {
						best = u
					}
				}
				return best, depth
			}
			levelStart = levelEnd
			depth++
		}
	}

	visited := make([]bool, n)
	order := make([]int, 0, n)
	var nb []int
	for s := 0; s < n; s++ {
		if visited[s] {
			continue
		}
		root := s
		far, ecc := farthest(root)
		for it := 0; it < 8; it++ {
			far2, ecc2 := farthest(far)
			root = far
			if ecc2 <= ecc {
				break
			}
			far, ecc = far2, ecc2
		}
		head := len(order)
		visited[root] = true
		order = append(order, root)
		for ; head < len(order); head++ {
			nb = nb[:0]
			for _, v := range adj[order[head]] {
				if !visited[v] {
					visited[v] = true
					nb = append(nb, v)
				}
			}
			slices.SortFunc(nb, func(a, b int) int {
				if c := cmp.Compare(len(adj[a]), len(adj[b])); c != 0 {
					return c
				}
				return cmp.Compare(a, b)
			})
			order = append(order, nb...)
		}
	}
	pos := make([]int, n)
	for k, node := range order {
		pos[node] = n - 1 - k
	}
	return pos
}

// bandToBidiagonal reduces the n×n upper-triangular band R (row c holds columns
// c..c+beta in R[c*(beta+1):], columns past n−1 unused) to upper bidiagonal form
// with Givens rotations, returning the diagonal d and superdiagonal e (len n,
// e[n−1] = 0) in the shape [bidiagonalExtremes] reads. Outermost superdiagonal
// first, each entry (i, i+k) is zeroed by a rotation of columns (i+k−1, i+k),
// which leaves one entry below the diagonal; a rotation of two rows removes that
// and pushes an entry out to superdiagonal k+1 further down, and the chase
// repeats until it leaves the matrix. Orthogonal transforms, so the singular
// values are those of R.
func bandToBidiagonal(R []float64, n, beta int) ([]float64, []float64) {
	// Row i keeps columns i−1..i+beta+1: the one entry below the diagonal and the
	// one beyond the band that the chase uses as scratch.
	bw := beta + 3
	B := make([]float64, n*bw)
	idx := func(i, j int) int { return i*bw + j - i + 1 }
	w := beta + 1
	for i := 0; i < n; i++ {
		for k := 0; k <= min(beta, n-1-i); k++ {
			B[idx(i, i+k)] = R[i*w+k]
		}
	}
	for k := beta; k >= 2; k-- {
		for i := 0; i+k < n; i++ {
			row, col := i, i+k
			for col < n {
				y := B[idx(row, col)]
				if y == 0 {
					break
				}
				x := B[idx(row, col-1)]
				rho := bandHypot(x, y)
				cs, sn := x/rho, y/rho
				for r := row; r <= col; r++ {
					a, b := B[idx(r, col-1)], B[idx(r, col)]
					B[idx(r, col-1)] = cs*a + sn*b
					B[idx(r, col)] = cs*b - sn*a
				}
				B[idx(row, col-1)], B[idx(row, col)] = rho, 0

				z := B[idx(col, col-1)]
				if z == 0 {
					break
				}
				p := B[idx(col-1, col-1)]
				rho = bandHypot(p, z)
				cs, sn = p/rho, z/rho
				for j := col - 1; j <= min(n-1, col+k); j++ {
					a, b := B[idx(col-1, j)], B[idx(col, j)]
					B[idx(col-1, j)] = cs*a + sn*b
					B[idx(col, j)] = cs*b - sn*a
				}
				B[idx(col-1, col-1)], B[idx(col, col-1)] = rho, 0
				row, col = col-1, col+k
			}
		}
	}
	d := make([]float64, n)
	e := make([]float64, n)
	for i := 0; i < n; i++ {
		d[i] = B[idx(i, i)]
		if i+1 < n {
			e[i] = B[idx(i, i+1)]
		}
	}
	return d, e
}

// bandHypot uses the hardware square-root path for the ordinary finite range
// produced by a scaled Jacobian. Outside that range it keeps Hypot's overflow,
// underflow and non-finite handling.
func bandHypot(x, y float64) float64 {
	ax, ay := math.Abs(x), math.Abs(y)
	max := math.Max(ax, ay)
	if max >= 1e-150 && max <= 1e150 {
		return math.Sqrt(x*x + y*y)
	}
	return math.Hypot(x, y)
}
