// Package param is a small, self-contained parameter and expression engine.
//
// A [Table] holds named parameters. Each parameter is either a literal number
// or an expression that may reference other parameters, call functions and use
// constants. Expressions are evaluated lazily with dependency resolution, so
// parameters may be defined in any order (forward references are allowed) and
// cyclic references are reported as errors rather than looping.
//
//	t := param.New()
//	t.Set("height", "60")
//	t.Set("width", "height * 1.5")
//	t.Set("area", "width * height")
//	w, _ := t.Get("width") // 90
//	a, _ := t.Get("area")  // 5400
//
//	t.Set("height", "40")  // edit and everything downstream follows
//	a, _ = t.Get("area")   // 2400
//
// The expression language supports +, -, *, /, % and ^ (right-associative
// exponentiation), unary +/-, parentheses, numeric literals (including
// scientific notation), identifiers (parameter or constant references) and
// function calls. Constants pi, tau, e and phi are predefined; a set of math
// functions is registered by default and more can be added with
// [Table.SetFunc].
//
// This package has no dependencies outside the standard library and no
// knowledge of the rest of the repository; it is intended to be extracted into
// its own module in the future.
package param
