# Exact fit-spline spans

## Purpose

`FitInterpolant.ExactSpans` publishes the exact rational form of a fit spline's cubic pieces. A consumer that
bounds one fit spline against another, such as decad comparing a rotated and refitted spline with its source,
reads the coefficients the arrangement engine's certificates already reason over. The consumer then works from
one definition of the curve.

## API

```go
type ExactFitSpan struct {
	TStart, TEnd float64
	X, Y         [4]*big.Rat
}

func (fi *FitInterpolant) ExactSpans() ([]ExactFitSpan, error)
```

- Span `i` covers the interval between `Points[i]` and `Points[i+1]`, in order, over the same coherent prefix
  `Spans()` reads.
- `X` and `Y` hold power-basis coefficients in the span's local parameter `u` in `[0, 1]`:
  `x(u) = X[0] + u·(X[1] + u·(X[2] + u·X[3]))`. This is the form `FitSpan` documents.
- `TStart` and `TEnd` are the span's bounds in the interpolant's normalized `[0, 1]` parameter, identical to the
  matching `FitSpan` values.
- Each coefficient is the span's formula (`v0`, `(v1-v0) - h²(2m0+m1)/6`, `h²m0/2`, `h²(m1-m0)/6`) evaluated in exact
  arithmetic over the float64 `Params`, `Points` and `SecondDerivs` read as exact rationals. `FitSpan` evaluates the
  same formula in float64, so the two agree to rounding.
- The caller owns the returned `big.Rat` values. Every call builds fresh ones.
- A degenerate interpolant returns `nil, nil`. A non-finite width, point or second derivative returns
  `ErrNonFiniteFitInterpolant`.

## One definition

`ExactSpans` calls `exactFitPieces`, the function `memoFitPieces` and the pair ledger use. A change to the exact
piece formula therefore changes the public coefficients and the certificates together.

## Tests

`TestFitInterpolantExactSpansMatchDefinition` compares every coefficient with a textbook natural-cubic span
written independently in `big.Rat`, and compares each with `FitSpan` to 1e-12 relative. The other two tests check
caller ownership and the degenerate case.
