package sketch_test

import (
	"testing"

	"github.com/lestrrat-3d/sketch"
	"github.com/lestrrat-3d/sketch/space"
	"github.com/stretchr/testify/require"
)

func worldVecEqual(t *testing.T, want, got space.Vec3) {
	t.Helper()
	const tol = 1e-9
	require.InDelta(t, want.X, got.X, tol)
	require.InDelta(t, want.Y, got.Y, tol)
	require.InDelta(t, want.Z, got.Z, tol)
}

func TestNewWorldSeedsDatums(t *testing.T) {
	w := sketch.NewWorld()
	require.Len(t, w.Planes(), 3)
	require.Equal(t, 0, w.XY().ID())
	require.Equal(t, 1, w.XZ().ID())
	require.Equal(t, 2, w.YZ().ID())

	f, err := w.XZ().Frame()
	require.NoError(t, err)
	worldVecEqual(t, space.NewVec3(1, 0, 0), f.ToWorldUV(1, 0))
	worldVecEqual(t, space.NewVec3(0, 0, 1), f.ToWorldUV(0, 1))
}

func TestNewEqualsNewOnWorldXY(t *testing.T) {
	s := sketch.New()
	p := s.AddPoint(3, 4)
	// A bare sketch is a world-XY sketch: world == (x, y, 0).
	worldVecEqual(t, space.NewVec3(3, 4, 0), p.World())
	require.NoError(t, p.WorldErr())
}

func TestSketchOnXZWorldCoords(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.Sketch(w.XZ())
	require.NoError(t, err)
	// A unit square on XZ: local (u, v) → world (u, 0, v).
	corners := [][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	for _, c := range corners {
		p := s.AddPoint(c[0], c[1])
		worldVecEqual(t, space.NewVec3(c[0], 0, c[1]), p.World())
	}
}

func TestOffsetPlaneShiftsWorldZ(t *testing.T) {
	w := sketch.NewWorld()
	off, err := w.OffsetPlane(w.XY(), 5)
	require.NoError(t, err)
	s, err := w.Sketch(off)
	require.NoError(t, err)
	p := s.AddPoint(3, 4)
	worldVecEqual(t, space.NewVec3(3, 4, 5), p.World())
}

func TestPlaneFromPoints(t *testing.T) {
	w := sketch.NewWorld()
	// Three points in the world z = 2 plane; normal should be +Z.
	p, err := w.PlaneFromPoints(space.NewVec3(0, 0, 2), space.NewVec3(1, 0, 2), space.NewVec3(0, 1, 2))
	require.NoError(t, err)
	f, err := p.Frame()
	require.NoError(t, err)
	worldVecEqual(t, space.NewVec3(0, 0, 1), f.N())

	_, err = w.PlaneFromPoints(space.NewVec3(0, 0, 0), space.NewVec3(1, 0, 0), space.NewVec3(2, 0, 0))
	require.ErrorIs(t, err, space.ErrDegenerateFrame)
}

func TestPlaneFromFrameRejectsInvalid(t *testing.T) {
	_, err := sketch.PlaneFromFrame(space.Frame{})
	require.ErrorIs(t, err, space.ErrDegenerateFrame)
	w := sketch.NewWorld()
	_, err = w.PlaneFromFrame(space.Frame{})
	require.ErrorIs(t, err, space.ErrDegenerateFrame)
}

func TestForeignPlaneRejected(t *testing.T) {
	w1 := sketch.NewWorld()
	w2 := sketch.NewWorld()
	_, err := w1.Sketch(w2.XY())
	require.ErrorIs(t, err, sketch.ErrForeignPlane)
	_, err = w1.OffsetPlane(w2.XY(), 1)
	require.ErrorIs(t, err, sketch.ErrForeignPlane)
	// A standalone plane is also foreign to a world.
	_, err = w1.Sketch(sketch.WorldXY())
	require.ErrorIs(t, err, sketch.ErrForeignPlane)
}

func TestNewOnRejectsWorldOwnedPlane(t *testing.T) {
	w := sketch.NewWorld()
	_, err := sketch.NewOn(w.XY())
	require.ErrorIs(t, err, sketch.ErrWorldOwnedPlane)
	// Owner-less planes are fine.
	_, err = sketch.NewOn(sketch.WorldXY())
	require.NoError(t, err)
}

func TestRemovePlane(t *testing.T) {
	w := sketch.NewWorld()

	// Standard datums cannot be removed.
	require.ErrorIs(t, w.RemovePlane(w.XY()), sketch.ErrStandardDatum)

	// A plane a sketch is placed on is in use.
	used, err := w.OffsetPlane(w.XY(), 1)
	require.NoError(t, err)
	_, err = w.Sketch(used)
	require.NoError(t, err)
	require.ErrorIs(t, w.RemovePlane(used), sketch.ErrPlaneInUse)

	// A plane used as a base is in use.
	base, err := w.OffsetPlane(w.XY(), 2)
	require.NoError(t, err)
	_, err = w.OffsetPlane(base, 3)
	require.NoError(t, err)
	require.ErrorIs(t, w.RemovePlane(base), sketch.ErrPlaneInUse)

	// A free plane removes, renumbers densely, and tombstones.
	free, err := w.OffsetPlane(w.XY(), 9)
	require.NoError(t, err)
	freeID := free.ID()
	require.NoError(t, w.RemovePlane(free))
	require.Equal(t, -1, free.ID())
	for i, p := range w.Planes() {
		require.Equal(t, i, p.ID(), "ids stay dense and equal to position")
	}
	// The removed (tombstoned) handle fails the liveness check.
	_, err = w.OffsetPlane(free, 1)
	require.ErrorIs(t, err, sketch.ErrForeignPlane)
	require.NotEqual(t, freeID, -2) // freeID was a real id before removal
}

func TestWorldPolylineLiftsToWorld(t *testing.T) {
	w := sketch.NewWorld()
	s, err := w.Sketch(w.XZ())
	require.NoError(t, err)
	a := s.AddPoint(0, 0)
	b := s.AddPoint(2, 0)
	line := s.AddLine(a, b)
	pts, err := s.WorldPolyline(line)
	require.NoError(t, err)
	require.Len(t, pts, 2)
	worldVecEqual(t, space.NewVec3(0, 0, 0), pts[0])
	worldVecEqual(t, space.NewVec3(2, 0, 0), pts[1])
}

func TestWorldPolylineRejectsForeignEntity(t *testing.T) {
	w := sketch.NewWorld()
	s1, err := w.Sketch(w.XY())
	require.NoError(t, err)
	s2, err := w.Sketch(w.XZ())
	require.NoError(t, err)

	foreign := s1.AddLine(s1.AddPoint(0, 0), s1.AddPoint(1, 0))
	_, err = s2.WorldPolyline(foreign) // entity of s1 lifted through s2's plane
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
	_, err = s2.WorldPolyline(nil)
	require.ErrorIs(t, err, sketch.ErrForeignEntity)

	// A removed entity is a dead handle.
	line := s1.AddLine(s1.AddPoint(2, 0), s1.AddPoint(3, 0))
	require.True(t, s1.RemoveEntity(line))
	_, err = s1.WorldPolyline(line)
	require.ErrorIs(t, err, sketch.ErrForeignEntity)
}

func TestNewOnNilEqualsNew(t *testing.T) {
	s, err := sketch.NewOn(nil)
	require.NoError(t, err)
	// A nil placement normalizes to a stable world-XY plane (not a fresh one
	// each call), so Plane() identity is consistent.
	require.Same(t, s.Plane(), s.Plane())
	p := s.AddPoint(3, 4)
	worldVecEqual(t, space.NewVec3(3, 4, 0), p.World())
}
