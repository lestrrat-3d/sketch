// Package space is a self-contained 3D coordinate-math layer: a [Vec3] vector
// type and an orthonormal right-handed [Frame] that carries the bidirectional
// transform between a plane's local (u, v, w) coordinates and world (x, y, z).
//
// It is to 3D what the geom package is to 2D — pure coordinate math with no
// document state — and the meeting point between the 2D sketch engine and the
// world: a solved local point (u, v) becomes a world [Vec3] via
// [Frame.ToWorldUV]. The package depends only on the standard library and is
// intended to be independently extractable.
//
// # Invariants
//
// A [Frame] is ALWAYS orthonormal and right-handed. The only way to obtain one
// is [NewFrame], which orthonormalizes its axes and rejects degenerate input;
// the zero value Frame{} is invalid and is reported as such by [Frame.IsValid]
// so callers that accept a frame from outside can reject it.
// Because the axes are orthonormal, the inverse transform [Frame.ToLocal] is
// three dot products (the transpose), never a matrix solve.
package space
