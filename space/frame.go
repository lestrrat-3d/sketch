package space

import (
	"errors"
	"math"
)

// ErrDegenerateFrame is returned by [NewFrame] when its axes are zero or
// collinear, so no orthonormal frame can be built from them.
var ErrDegenerateFrame = errors.New("space: degenerate frame (zero or collinear axes)")

// orthoTol is how far the stored axes may drift from unit length / mutual
// orthogonality before [Frame.IsValid] rejects them.
const orthoTol = 1e-9

// Frame is a right-handed orthonormal coordinate frame in world space: an
// origin plus two in-plane unit axes U and V. The normal N is always U × V and
// is derived, never stored, so the frame cannot disagree with its own normal.
//
// The fields are unexported: a Frame can only be created through [NewFrame],
// which enforces orthonormality. The zero value Frame{} is invalid (see
// [Frame.IsValid]); a caller that receives a Frame from outside must validate it
// before trusting it.
type Frame struct {
	origin, u, v Vec3
}

// NewFrame returns an orthonormal right-handed frame at origin whose first axis
// is along u and whose second axis lies in the u–v plane. The axes are
// orthonormalized with Gram–Schmidt (u is kept; v is made perpendicular to u;
// both are normalized). It returns [ErrDegenerateFrame] when u is zero or when
// v is collinear with u (the perpendicular component vanishes).
func NewFrame(origin, u, v Vec3) (Frame, error) {
	un, ok := u.Normalize()
	if !ok {
		return Frame{}, ErrDegenerateFrame
	}
	// Remove the u-component of v, leaving the in-plane perpendicular.
	vp := v.Sub(un.Scale(v.Dot(un)))
	vn, ok := vp.Normalize()
	if !ok {
		return Frame{}, ErrDegenerateFrame
	}
	return Frame{origin: origin, u: un, v: vn}, nil
}

// Origin returns the world position of the frame's local (0, 0, 0).
func (f Frame) Origin() Vec3 { return f.origin }

// U returns the frame's first in-plane unit axis.
func (f Frame) U() Vec3 { return f.u }

// V returns the frame's second in-plane unit axis.
func (f Frame) V() Vec3 { return f.v }

// N returns the frame's unit normal, U × V. It is derived on every call, never
// stored.
func (f Frame) N() Vec3 { return f.u.Cross(f.v) }

// IsValid reports whether the frame's axes are unit length and mutually
// orthogonal. The zero value Frame{} is not valid. Use it to vet a frame
// supplied by a caller before building geometry on it.
func (f Frame) IsValid() bool {
	if math.Abs(f.u.Len()-1) > orthoTol {
		return false
	}
	if math.Abs(f.v.Len()-1) > orthoTol {
		return false
	}
	return math.Abs(f.u.Dot(f.v)) <= orthoTol
}

// ToWorld maps a local coordinate (u along U, v along V, w along N) to world
// space.
func (f Frame) ToWorld(local Vec3) Vec3 {
	return f.origin.
		Add(f.u.Scale(local.X)).
		Add(f.v.Scale(local.Y)).
		Add(f.N().Scale(local.Z))
}

// ToWorldUV maps an in-plane 2D point (u, v) — the currency of a sketch — to
// world space (w = 0).
func (f Frame) ToWorldUV(u, v float64) Vec3 {
	return f.origin.Add(f.u.Scale(u)).Add(f.v.Scale(v))
}

// ToLocal maps a world point to local coordinates. The third component is the
// signed distance off the plane (along N). It is the exact inverse of ToWorld
// (the transpose), valid because the frame is orthonormal.
func (f Frame) ToLocal(world Vec3) Vec3 {
	d := world.Sub(f.origin)
	return Vec3{d.Dot(f.u), d.Dot(f.v), d.Dot(f.N())}
}
