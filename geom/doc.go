// Package geom holds context-agnostic 2D geometry: plain [Point], [Line],
// [Circle] and [Arc] definitions with no notion of a sketch, solver or
// constraints.
//
// These types are reusable templates. The same generic geometry can be
// committed into several independent sketches (see the parent sketch package's
// Add methods), each of which builds its own solver-bound instance from it.
// Generic geometry holds only its defining coordinates and metadata; it is
// never mutated by solving.
//
// Points are referenced by pointer so that geometry sharing an endpoint
// (two lines meeting at a vertex) shares one [Point], and a sketch can map each
// distinct generic point to a single solver point.
//
// The package depends only on the standard library and is intended to be
// reusable on its own.
package geom
