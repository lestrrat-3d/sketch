package geom

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

// mapVertexTable is the vertex table as it was kept before the grid became an
// open-addressing table: a map from grid cell to the ids in it, in insertion
// order. The grid table must weld every point exactly as this one does.
type mapVertexTable struct {
	merge, cell float64
	xs, ys      []float64
	grid        map[[2]int][]int
}

func (t *mapVertexTable) canon(x, y float64) int {
	cx, cy := int(math.Floor(x/t.cell)), int(math.Floor(y/t.cell))
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			for _, id := range t.grid[[2]int{cx + dx, cy + dy}] {
				if math.Hypot(t.xs[id]-x, t.ys[id]-y) <= t.merge {
					return id
				}
			}
		}
	}
	id := len(t.xs)
	t.xs = append(t.xs, x)
	t.ys = append(t.ys, y)
	t.grid[[2]int{cx, cy}] = append(t.grid[[2]int{cx, cy}], id)
	return id
}

func TestVertexTableMatchesMapTable(t *testing.T) {
	rng := rand.New(rand.NewPCG(0x56455254, 9))
	for trial := 0; trial < 40; trial++ {
		merge := math.Pow(10, -float64(1+rng.IntN(6)))
		ref := &mapVertexTable{merge: merge, cell: merge, grid: map[[2]int][]int{}}
		got := newVertexTable(merge)
		if trial%2 == 0 {
			got.reserveCells(200)
		}
		var pts [][2]float64
		for range 600 {
			// Clusters of near-coincident points, plus repeats and negatives.
			switch {
			case len(pts) > 0 && rng.IntN(3) == 0:
				p := pts[rng.IntN(len(pts))]
				pts = append(pts, [2]float64{p[0] + (rng.Float64()-0.5)*2*merge, p[1] + (rng.Float64()-0.5)*2*merge})
			default:
				pts = append(pts, [2]float64{(rng.Float64() - 0.5) * 50, (rng.Float64() - 0.5) * 50})
			}
		}
		for i, p := range pts {
			require.Equal(t, ref.canon(p[0], p[1]), got.canon(p[0], p[1]), "trial %d point %d", trial, i)
		}
		require.Equal(t, ref.xs, got.xs)
		require.Equal(t, ref.ys, got.ys)
	}
}
