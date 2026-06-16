package space

import "math"

// zeroLen is the threshold below which a vector is treated as having no
// direction. It is a divide-by-zero guard for Normalize, not a geometric
// tolerance.
const zeroLen = 1e-12

// Vec3 is a 3D vector (or point): pure transient coordinate math, the 3D analog
// of geom.Point. It carries no document state.
type Vec3 struct {
	X, Y, Z float64
}

// NewVec3 returns the vector (x, y, z).
func NewVec3(x, y, z float64) Vec3 { return Vec3{X: x, Y: y, Z: z} }

// Add returns v + o.
func (v Vec3) Add(o Vec3) Vec3 { return Vec3{v.X + o.X, v.Y + o.Y, v.Z + o.Z} }

// Sub returns v − o.
func (v Vec3) Sub(o Vec3) Vec3 { return Vec3{v.X - o.X, v.Y - o.Y, v.Z - o.Z} }

// Scale returns v scaled by s.
func (v Vec3) Scale(s float64) Vec3 { return Vec3{v.X * s, v.Y * s, v.Z * s} }

// Dot returns the dot product v · o.
func (v Vec3) Dot(o Vec3) float64 { return v.X*o.X + v.Y*o.Y + v.Z*o.Z }

// Cross returns the cross product v × o.
func (v Vec3) Cross(o Vec3) Vec3 {
	return Vec3{
		v.Y*o.Z - v.Z*o.Y,
		v.Z*o.X - v.X*o.Z,
		v.X*o.Y - v.Y*o.X,
	}
}

// Len returns the Euclidean length of v.
func (v Vec3) Len() float64 { return math.Sqrt(v.Dot(v)) }

// Normalize returns the unit vector along v and true, or the zero vector and
// false when v is (near-)zero. The boolean is deliberate: unlike a
// floor-against-zero helper, Normalize never fabricates a non-unit direction
// from a zero vector — callers must handle the false case.
func (v Vec3) Normalize() (Vec3, bool) {
	l := v.Len()
	if l < zeroLen {
		return Vec3{}, false
	}
	return v.Scale(1 / l), true
}
