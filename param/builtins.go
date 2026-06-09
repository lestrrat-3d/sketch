package param

import (
	"fmt"
	"math"
)

func defaultConsts() map[string]float64 {
	return map[string]float64{
		"pi":  math.Pi,
		"tau": 2 * math.Pi,
		"e":   math.E,
		"phi": math.Phi,
	}
}

// mod and powFn are used by the evaluator so that % and ^ behave like the math
// package rather than panicking or truncating.
func mod(a, b float64) float64   { return math.Mod(a, b) }
func powFn(a, b float64) float64 { return math.Pow(a, b) }

func arityErr(name string, want, got int) error {
	return fmt.Errorf("param: %s expects %d argument(s), got %d", name, want, got)
}

func fn1(name string, f func(float64) float64) Func {
	return func(a []float64) (float64, error) {
		if len(a) != 1 {
			return 0, arityErr(name, 1, len(a))
		}
		return f(a[0]), nil
	}
}

func fn2(name string, f func(float64, float64) float64) Func {
	return func(a []float64) (float64, error) {
		if len(a) != 2 {
			return 0, arityErr(name, 2, len(a))
		}
		return f(a[0], a[1]), nil
	}
}

func defaultFuncs() map[string]Func {
	m := map[string]Func{
		"sin":   fn1("sin", math.Sin),
		"cos":   fn1("cos", math.Cos),
		"tan":   fn1("tan", math.Tan),
		"asin":  fn1("asin", math.Asin),
		"acos":  fn1("acos", math.Acos),
		"atan":  fn1("atan", math.Atan),
		"atan2": fn2("atan2", math.Atan2),
		"sqrt":  fn1("sqrt", math.Sqrt),
		"cbrt":  fn1("cbrt", math.Cbrt),
		"abs":   fn1("abs", math.Abs),
		"floor": fn1("floor", math.Floor),
		"ceil":  fn1("ceil", math.Ceil),
		"round": fn1("round", math.Round),
		"trunc": fn1("trunc", math.Trunc),
		"sign": fn1("sign", func(x float64) float64 {
			switch {
			case x > 0:
				return 1
			case x < 0:
				return -1
			default:
				return 0
			}
		}),
		"exp":   fn1("exp", math.Exp),
		"ln":    fn1("ln", math.Log),
		"log":   fn1("log", math.Log),
		"log10": fn1("log10", math.Log10),
		"log2":  fn1("log2", math.Log2),
		"deg":   fn1("deg", func(r float64) float64 { return r * 180 / math.Pi }),
		"rad":   fn1("rad", func(d float64) float64 { return d * math.Pi / 180 }),
		"pow":   fn2("pow", math.Pow),
		"mod":   fn2("mod", math.Mod),
		"min":   variadic("min", math.Min),
		"max":   variadic("max", math.Max),
	}
	m["hypot"] = func(a []float64) (float64, error) {
		if len(a) < 1 {
			return 0, fmt.Errorf("param: hypot expects at least 1 argument")
		}
		var sum float64
		for _, v := range a {
			sum += v * v
		}
		return math.Sqrt(sum), nil
	}
	m["clamp"] = func(a []float64) (float64, error) {
		if len(a) != 3 {
			return 0, arityErr("clamp", 3, len(a))
		}
		x, lo, hi := a[0], a[1], a[2]
		if lo > hi {
			lo, hi = hi, lo
		}
		return math.Max(lo, math.Min(hi, x)), nil
	}
	return m
}

// variadic builds a reducer like min/max that accepts one or more arguments.
func variadic(name string, reduce func(float64, float64) float64) Func {
	return func(a []float64) (float64, error) {
		if len(a) == 0 {
			return 0, fmt.Errorf("param: %s expects at least 1 argument", name)
		}
		acc := a[0]
		for _, v := range a[1:] {
			acc = reduce(acc, v)
		}
		return acc, nil
	}
}
