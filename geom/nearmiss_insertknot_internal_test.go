package geom

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

// referenceInsertKnotHom is insertKnotHom as it stood when it built a fresh
// control array and knot vector per insertion. The in-place refinement must
// reproduce it bit for bit.
func referenceInsertKnotHom(p int, u []float64, pw [][3]float64, x float64) ([]float64, [][3]float64) {
	n := len(pw) - 1
	k := findSpan(n, p, x, u)
	s := 0
	for i := k; i >= 0 && u[i] == x; i-- {
		s++
	}
	q := make([][3]float64, n+2)
	for i := 0; i <= k-p; i++ {
		q[i] = pw[i]
	}
	for i := k - s + 1; i <= n+1; i++ {
		q[i] = pw[i-1]
	}
	for i := k - p + 1; i <= k-s; i++ {
		alpha := 0.0
		if den := u[i+p] - u[i]; den > 0 {
			alpha = (x - u[i]) / den
		}
		q[i] = [3]float64{
			alpha*pw[i][0] + (1-alpha)*pw[i-1][0],
			alpha*pw[i][1] + (1-alpha)*pw[i-1][1],
			alpha*pw[i][2] + (1-alpha)*pw[i-1][2],
		}
	}
	nu := make([]float64, 0, len(u)+1)
	nu = append(nu, u[:k+1]...)
	nu = append(nu, x)
	nu = append(nu, u[k+1:]...)
	return nu, q
}

func TestInsertKnotHomMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewPCG(0x4b4e4f54, 3))
	for trial := 0; trial < 400; trial++ {
		p := 1 + rng.IntN(4)
		n := p + rng.IntN(8) // last control index
		// A clamped knot vector with random interior knots, some repeated.
		interior := n - p
		u := make([]float64, 0, n+p+2)
		for range p + 1 {
			u = append(u, 0)
		}
		prev := 0.0
		for range interior {
			if rng.IntN(4) != 0 {
				prev += rng.Float64()
			}
			u = append(u, prev)
		}
		top := prev + 1
		for range p + 1 {
			u = append(u, top)
		}
		pw := make([][3]float64, n+1)
		for i := range pw {
			w := 0.5 + rng.Float64()
			pw[i] = [3]float64{w * rng.NormFloat64(), w * rng.NormFloat64(), w}
		}
		x := u[p] + rng.Float64()*(u[n+1]-u[p])
		if rng.IntN(3) == 0 && interior > 0 {
			x = u[p+1+rng.IntN(interior)] // an existing knot: raises multiplicity
		}

		// The caller inserts only strictly inside the domain and only below
		// multiplicity p.
		if x <= u[p] || x >= u[n+1] || knotMultiplicity(u, x) >= p {
			continue
		}

		wantU, wantQ := referenceInsertKnotHom(p, u, pw, x)

		gotU := append(make([]float64, 0, len(u)+4), u...)
		gotP := append(make([][3]float64, 0, len(pw)+4), pw...)
		gotU, gotQ := insertKnotHom(p, gotU, gotP, x)

		require.Len(t, gotU, len(wantU), "trial %d knots", trial)
		require.Len(t, gotQ, len(wantQ), "trial %d controls", trial)
		for i := range wantU {
			require.Equal(t, math.Float64bits(wantU[i]), math.Float64bits(gotU[i]), "trial %d knot %d", trial, i)
		}
		for i := range wantQ {
			for c := range 3 {
				require.Equal(t, math.Float64bits(wantQ[i][c]), math.Float64bits(gotQ[i][c]),
					"trial %d control %d[%d]", trial, i, c)
			}
		}
	}
}
