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
//
// # Validation
//
// Constructors are value holders; the arrangement engine ([Regions]) validates
// at the point of use. Of the twelve package-level New… constructors, eight —
// [NewPoint], [NewLine], [NewCircle], [NewEllipse], [NewArc],
// [NewEllipticalArc], [NewConic] and [NewNURBS] — validate nothing and have no
// error return: a nil point, a non-finite coordinate or a degenerate radius
// all construct cleanly. The other four are narrower than general input
// validation, not an exception to it: [NewSpline], [NewClosedSpline] and
// [NewFitSpline] check only the point count their kernel needs
// ([ErrTooFewControlPoints]/[ErrTooFewClosedControlPoints]/
// [ErrTooFewFitPoints]) — a precondition the evaluator itself cannot express,
// never a check on a point's coordinates — and [NewFitInterpolant] validates a
// built interpolant's finiteness ([ErrNonFiniteFitInterpolant]), not its input
// fit coordinates. Everything else — a nil point, a non-finite coordinate, a
// degenerate radius — is caught later, when [Regions] builds the arrangement:
// a usable-radius guard for a circle/ellipse/arc family, a NURBS structure
// guard, a fit-spline point guard (screened before the fit evaluator's own
// coincidence filter can silently drop a non-finite point), a degenerate
// (all-coincident) control/fit-point guard, and a finiteness check over every
// evaluated sample as the last net.
package geom
