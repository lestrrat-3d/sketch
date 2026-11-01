package geom

import "errors"

// ErrUnsupportedLineScene reports that a bounded line arrangement cannot
// account for the supplied scene. It never denotes an exhausted work budget.
var ErrUnsupportedLineScene = unsupportedLineSceneError{}

// ErrArrangementLimit reports that a bounded arrangement exceeded its finite
// work, record, or memory allowance.
var ErrArrangementLimit = errors.New("geom: bounded arrangement limit exceeded")

type unsupportedLineSceneError struct{}

func (unsupportedLineSceneError) Error() string { return "unsupported bounded line scene" }

type arrangeBudget struct {
	contextErr func() error
	charge     func(uint64) error
	resources  func(uint64, uint64) error
	limitErr   error
	spent      uint64
	records    uint64
	bytes      uint64
}

const maxLineArrangementWork = 1 << 20
const maxLineArrangementRecords = 1 << 15
const maxArrangementBytes = 1 << 26

type arrangeAbort struct{ err error }

func (b *arrangeBudget) step(n uint64) {
	if b == nil {
		return
	}
	if err := b.contextErr(); err != nil {
		panic(arrangeAbort{err})
	}
	if n == 0 {
		return
	}
	if n > 256 {
		panic(arrangeAbort{ErrUnsupportedLineScene})
	}
	if n > maxLineArrangementWork-b.spent {
		if b.limitErr != nil {
			panic(arrangeAbort{b.limitErr})
		}
		panic(arrangeAbort{ErrUnsupportedLineScene})
	}
	if err := b.charge(n); err != nil {
		panic(arrangeAbort{err})
	}
	b.spent += n
}

func (b *arrangeBudget) preflight(n int) int {
	if b != nil {
		if n < 0 || n > maxLineArrangementRecords {
			panic(arrangeAbort{b.limitError()})
		}
		b.reserveResources(uint64(n))
		b.reserve(uint64(n))
	}
	return n
}

func (b *arrangeBudget) limitError() error {
	if b.limitErr != nil {
		return b.limitErr
	}
	return ErrUnsupportedLineScene
}

func (b *arrangeBudget) reserveResources(n uint64) {
	if b == nil || n == 0 {
		return
	}
	const bytesPerRecord = 512
	if n > maxLineArrangementRecords-b.records || n > (maxArrangementBytes-b.bytes)/bytesPerRecord {
		panic(arrangeAbort{b.limitError()})
	}
	if b.resources != nil {
		if err := b.resources(n, n*bytesPerRecord); err != nil {
			panic(arrangeAbort{err})
		}
	}
	b.records += n
	b.bytes += n * bytesPerRecord
}

func (b *arrangeBudget) reserve(n uint64) {
	if b == nil {
		return
	}
	for n > 0 {
		part := min(n, 256)
		b.step(part)
		n -= part
	}
}

// reserveSort charges a conservative bound before a comparator that cannot
// return an error. All bounded line-scene sort inputs are records derived from
// at most 64 lines; the separate size guard avoids integer overflow here.
func (b *arrangeBudget) reserveSort(n int) {
	if b == nil || n < 2 {
		return
	}
	if n > maxLineArrangementRecords {
		panic(arrangeAbort{ErrUnsupportedLineScene})
	}
	b.reserve(uint64(n) * uint64(n))
}

func firstBudget(budgets []*arrangeBudget) *arrangeBudget {
	if len(budgets) == 0 {
		return nil
	}
	return budgets[0]
}

func chargedAppend[T any](b *arrangeBudget, dst []T, values ...T) []T {
	if b == nil {
		return append(dst, values...)
	}
	if len(dst) > maxLineArrangementRecords || len(values) > maxLineArrangementRecords-len(dst) {
		panic(arrangeAbort{b.limitError()})
	}
	for len(values) > 0 {
		n := min(len(values), 256)
		b.reserveResources(uint64(n))
		b.step(uint64(n))
		dst = append(dst, values[:n]...)
		values = values[n:]
	}
	return dst
}
