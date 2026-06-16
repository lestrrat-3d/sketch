package space_test

import (
	"math"
	"testing"

	"github.com/lestrrat-3d/sketch/space"
	"github.com/stretchr/testify/require"
)

func vecEqual(t *testing.T, want, got space.Vec3) {
	t.Helper()
	const tol = 1e-12
	require.InDelta(t, want.X, got.X, tol)
	require.InDelta(t, want.Y, got.Y, tol)
	require.InDelta(t, want.Z, got.Z, tol)
}

func mkFrame(t *testing.T, origin, u, v space.Vec3) space.Frame {
	t.Helper()
	f, err := space.NewFrame(origin, u, v)
	require.NoError(t, err)
	return f
}

func TestVec3Ops(t *testing.T) {
	a := space.NewVec3(1, 2, 3)
	b := space.NewVec3(4, 5, 6)
	vecEqual(t, space.NewVec3(5, 7, 9), a.Add(b))
	vecEqual(t, space.NewVec3(-3, -3, -3), a.Sub(b))
	vecEqual(t, space.NewVec3(2, 4, 6), a.Scale(2))
	require.InDelta(t, 32, a.Dot(b), 1e-12)
	// x × y = z (right-handed).
	vecEqual(t, space.NewVec3(0, 0, 1), space.NewVec3(1, 0, 0).Cross(space.NewVec3(0, 1, 0)))

	zero := space.Vec3{}
	if _, ok := zero.Normalize(); ok {
		t.Fatal("normalizing the zero vector must report false")
	}
	u, ok := space.NewVec3(0, 3, 0).Normalize()
	require.True(t, ok)
	vecEqual(t, space.NewVec3(0, 1, 0), u)
}

func TestNewFrameOrthonormalizes(t *testing.T) {
	// Skewed, non-unit axes must come back orthonormal and right-handed.
	f, err := space.NewFrame(space.NewVec3(1, 1, 1), space.NewVec3(0, 2, 0), space.NewVec3(3, 3, 0))
	require.NoError(t, err)
	require.True(t, f.IsValid())
	require.InDelta(t, 1, f.U().Len(), 1e-12)
	require.InDelta(t, 1, f.V().Len(), 1e-12)
	require.InDelta(t, 0, f.U().Dot(f.V()), 1e-12)
	// N == U × V and is unit.
	vecEqual(t, f.U().Cross(f.V()), f.N())
	require.InDelta(t, 1, f.N().Len(), 1e-12)
}

func TestNewFrameDegenerate(t *testing.T) {
	_, err := space.NewFrame(space.Vec3{}, space.Vec3{}, space.NewVec3(0, 1, 0))
	require.ErrorIs(t, err, space.ErrDegenerateFrame)
	// Collinear axes: v parallel to u leaves no perpendicular component.
	_, err = space.NewFrame(space.Vec3{}, space.NewVec3(1, 0, 0), space.NewVec3(2, 0, 0))
	require.ErrorIs(t, err, space.ErrDegenerateFrame)
}

func TestZeroFrameInvalid(t *testing.T) {
	require.False(t, space.Frame{}.IsValid())
}

func TestFrameRoundTrip(t *testing.T) {
	f := mkFrame(t, space.NewVec3(10, -5, 2), space.NewVec3(1, 1, 0), space.NewVec3(-1, 1, 0))
	for _, w := range []space.Vec3{
		space.NewVec3(0, 0, 0),
		space.NewVec3(3, 4, 5),
		space.NewVec3(-7, 2, 9),
	} {
		vecEqual(t, w, f.ToWorld(f.ToLocal(w)))
	}
}

func TestKnownMapsXZ(t *testing.T) {
	// The XZ datum: U = +X, V = +Z, N = −Y.
	xz := mkFrame(t, space.Vec3{}, space.NewVec3(1, 0, 0), space.NewVec3(0, 0, 1))
	vecEqual(t, space.NewVec3(1, 0, 0), xz.ToWorldUV(1, 0))
	vecEqual(t, space.NewVec3(0, 0, 1), xz.ToWorldUV(0, 1))
	vecEqual(t, space.NewVec3(0, -1, 0), xz.N())
}

func TestOffsetAlongNormal(t *testing.T) {
	// Shifting the XY origin along +N (=+Z) moves world z, leaving x, y.
	xy := mkFrame(t, space.Vec3{}, space.NewVec3(1, 0, 0), space.NewVec3(0, 1, 0))
	d := 7.5
	shifted := mkFrame(t, xy.Origin().Add(xy.N().Scale(d)), xy.U(), xy.V())
	w := shifted.ToWorldUV(3, 4)
	vecEqual(t, space.NewVec3(3, 4, d), w)
	require.InDelta(t, math.Abs(d), math.Abs(w.Z), 1e-12)
}
