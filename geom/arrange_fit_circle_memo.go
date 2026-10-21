package geom

import (
	"encoding/binary"
	"math"
	"sync"
)

// A fit-circle scene is certified once per arrangement, and the same sketch is
// arranged again by every Profiles, Chains, UnionProfiles and Verify call that
// follows. The expensive parts of the certification are pure functions of their
// float64 inputs: exact pieces from an interpolant, the root crossing, the hulls
// and their separation. [fitMemo] keeps each result under a key made of the exact
// bits of every input, so a repeat of the same scene returns what a fresh
// computation would, and a scene that differs in any input bit misses.
//
// A cached value is shared by every caller, so callers MUST treat it as read-only.
// The certification only compares and combines the *big.Rat values it holds,
// building new ones for every result, and never assigns into one.

// fitMemoCap bounds one cache. A full cache is emptied rather than evicted from:
// entries are cheap to rebuild and a long-lived process must not grow without
// limit.
const fitMemoCap = 4096

// fitMemo is a mutex-guarded cache of pure results keyed by input bits.
type fitMemo[V any] struct {
	mu sync.Mutex
	m  map[string]V
}

// get returns the cached value for key, building and storing it on a miss.
// build runs outside the lock, so two concurrent misses may both build; the
// results are equal, and the later store replaces the earlier.
func (c *fitMemo[V]) get(key string, build func() V) V {
	c.mu.Lock()
	v, ok := c.m[key]
	c.mu.Unlock()
	if ok {
		return v
	}
	v = build()
	c.mu.Lock()
	if c.m == nil || len(c.m) >= fitMemoCap {
		c.m = make(map[string]V)
	}
	c.m[key] = v
	c.mu.Unlock()
	return v
}

// fitKey builds a cache key from the exact bits of its inputs.
type fitKey struct{ b []byte }

func (k *fitKey) tag(t byte) *fitKey { k.b = append(k.b, t); return k }

func (k *fitKey) n(v int) *fitKey {
	k.b = binary.LittleEndian.AppendUint64(k.b, uint64(v))
	return k
}

func (k *fitKey) f(vs ...float64) *fitKey {
	for _, v := range vs {
		k.b = binary.LittleEndian.AppendUint64(k.b, math.Float64bits(v))
	}
	return k
}

func (k *fitKey) s(str string) *fitKey { return k.n(len(str)).raw(str) }

func (k *fitKey) raw(str string) *fitKey { k.b = append(k.b, str...); return k }

func (k *fitKey) String() string { return string(k.b) }

// fitInterpolantKey identifies the exact pieces an interpolant yields: they are
// a function of its parameters, points and second derivatives alone.
func fitInterpolantKey(fi *FitInterpolant) string {
	k := (&fitKey{}).tag('I').n(len(fi.Params)).f(fi.Params...).n(len(fi.Points))
	for _, p := range fi.Points {
		k.f(p[0], p[1])
	}
	k.n(len(fi.SecondDerivs))
	for _, d := range fi.SecondDerivs {
		k.f(d[0], d[1])
	}
	return k.String()
}

type fitPiecesResult struct {
	pieces []exactFitPiece
	ok     bool
}

type fitCrossingResult struct {
	t  float64
	ok bool
}

var (
	fitPiecesMemo   fitMemo[fitPiecesResult]
	fitCrossingMemo fitMemo[fitCrossingResult]
	fitCutMemo      fitMemo[bool]
	fitHullsMemo    fitMemo[[]exactFitHull]
	fitSideMemo     fitMemo[int]
	fitSeparateMemo fitMemo[bool]
)

// memoFitPieces is [exactFitPieces] under its interpolant key.
func memoFitPieces(key string, fi *FitInterpolant) ([]exactFitPiece, bool) {
	r := fitPiecesMemo.get(key, func() fitPiecesResult {
		pieces, ok := exactFitPieces(fi)
		return fitPiecesResult{pieces, ok}
	})
	return r.pieces, r.ok
}

// memoFitRadiusCrossing is [fitRadiusCrossing] for the pieces named by key.
func memoFitRadiusCrossing(key string, pieces []exactFitPiece, cx, cy, root, tip, scale float64) (float64, bool) {
	k := (&fitKey{}).tag('R').s(key).f(cx, cy, root, tip, scale).String()
	r := fitCrossingMemo.get(k, func() fitCrossingResult {
		t, ok := fitRadiusCrossing(pieces, cx, cy, root, tip, scale)
		return fitCrossingResult{t, ok}
	})
	return r.t, r.ok
}

// memoFitCutBelowRoot is [fitCutBelowRoot] for the pieces named by key.
func memoFitCutBelowRoot(key string, pieces []exactFitPiece, cut, cx, cy, root, scale float64) bool {
	k := (&fitKey{}).tag('C').s(key).f(cut, cx, cy, root, scale).String()
	return fitCutMemo.get(k, func() bool { return fitCutBelowRoot(pieces, cut, cx, cy, root, scale) })
}

// fitHullsKey and fitHullsAfterKey name the hull sets [exactFitHulls] and
// [exactFitHullsAfter] build for the pieces named by key.
func fitHullsKey(key string) string { return (&fitKey{}).tag('H').s(key).String() }

func fitHullsAfterKey(key string, cut float64) string {
	return (&fitKey{}).tag('A').s(key).f(cut).String()
}

// memoFitHulls is [exactFitHulls] for the pieces named by key.
func memoFitHulls(key string, pieces []exactFitPiece) []exactFitHull {
	k := fitHullsKey(key)
	return fitHullsMemo.get(k, func() []exactFitHull { return exactFitHulls(pieces) })
}

// memoFitHullsAfter is [exactFitHullsAfter] for the pieces named by key.
func memoFitHullsAfter(key string, pieces []exactFitPiece, cut float64) []exactFitHull {
	k := fitHullsAfterKey(key, cut)
	return fitHullsMemo.get(k, func() []exactFitHull { return exactFitHullsAfter(pieces, cut) })
}

// memoFitSide is [fitSide] for the pieces named by key.
func memoFitSide(key string, pieces []exactFitPiece, cx, cy, ax, ay, scale float64) int {
	k := (&fitKey{}).tag('S').s(key).f(cx, cy, ax, ay, scale).String()
	return fitSideMemo.get(k, func() int { return fitSide(pieces, cx, cy, ax, ay, scale) })
}

// memoFitHullsSeparate is [exactFitHullsSeparate] for two hull sets named by
// their keys; the keys MUST be the ones the hulls were built under.
func memoFitHullsSeparate(keyA string, a []exactFitHull, keyB string, b []exactFitHull, scale float64) bool {
	k := (&fitKey{}).tag('P').s(keyA).s(keyB).f(scale).String()
	return fitSeparateMemo.get(k, func() bool { return exactFitHullsSeparate(a, b, scale) })
}
