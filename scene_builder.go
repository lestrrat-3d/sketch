package sketch

import (
	"context"
	"errors"
)

var (
	// ErrInvalidSceneContext reports a nil context passed to bounded scene creation.
	ErrInvalidSceneContext = errors.New("sketch: nil scene construction context")
	// ErrSceneBuilderClosed reports use of a scene builder after construction ends.
	ErrSceneBuilderClosed = errors.New("sketch: scene builder is closed")
)

// SketchSceneBuilder creates points, lines, circles, and arcs for one sketch.
// Its methods are available only during [World.CreateSketchContext].
type SketchSceneBuilder struct {
	contextErr func() error
	budget     ProfileBudget
	sketch     *Sketch
	work       uint64
	records    uint64
	bytes      uint64
	err        error
	closed     bool
}

// CreateSketchContext constructs a sketch with bounded scene primitives and
// publishes it to the world only after build returns successfully. build may be
// nil to create an empty sketch. The context, budget, and plane MUST NOT be nil
// and plane MUST belong to this world. A refusal or cancellation returns no
// sketch and leaves the world unchanged.
func (w *World) CreateSketchContext(ctx context.Context, plane *Plane, budget ProfileBudget,
	build func(*SketchSceneBuilder) error) (*Sketch, error) {
	if w == nil || !w.owns(plane) {
		return nil, ErrForeignPlane
	}
	if ctx == nil {
		return nil, ErrInvalidSceneContext
	}
	if budget == nil {
		return nil, ErrProfileBudgetExceeded
	}
	builder := &SketchSceneBuilder{contextErr: ctx.Err, budget: budget}
	if err := builder.charge(1, 4, 1024); err != nil {
		return nil, err
	}
	s := newSketch(plane)
	s.world = w
	s.params = w.params
	builder.sketch = s
	if build != nil {
		if err := build(builder); err != nil {
			builder.close(err)
			return nil, err
		}
	}
	if builder.err != nil {
		builder.close(builder.err)
		return nil, builder.err
	}
	if err := builder.contextErr(); err != nil {
		builder.close(err)
		return nil, err
	}
	if err := builder.charge(1, 1, 16); err != nil {
		builder.close(err)
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		builder.close(err)
		return nil, err
	}
	w.sketches = append(w.sketches, s)
	builder.close(nil)
	return s, nil
}

func (b *SketchSceneBuilder) charge(work, records, bytes uint64) error {
	if b.err != nil {
		return b.err
	}
	if b.closed {
		return ErrSceneBuilderClosed
	}
	if err := b.contextErr(); err != nil {
		b.close(err)
		return err
	}
	if work > maxBoundedSketchWork-b.work || records > maxBoundedSketchRecords-b.records ||
		bytes > maxBoundedSketchBytes-b.bytes {
		b.close(ErrProfileBudgetExceeded)
		return ErrProfileBudgetExceeded
	}
	if err := b.budget.Charge(work, records, bytes); err != nil {
		b.close(err)
		return err
	}
	if err := b.contextErr(); err != nil {
		b.close(err)
		return err
	}
	b.work += work
	b.records += records
	b.bytes += bytes
	return nil
}

func (b *SketchSceneBuilder) close(err error) {
	if b.err == nil {
		b.err = err
	}
	b.closed = true
}

func (b *SketchSceneBuilder) usable() error {
	if b.err != nil {
		return b.err
	}
	if b.closed {
		return ErrSceneBuilderClosed
	}
	return nil
}

// CreatePoint adds a point to the pending sketch after charging its solver and
// point storage.
func (b *SketchSceneBuilder) CreatePoint(x, y float64) (*Point, error) {
	if err := b.usable(); err != nil {
		return nil, err
	}
	if err := b.charge(1, 5, 256); err != nil {
		return nil, err
	}
	return b.sketch.CreatePoint(x, y), nil
}

// CreateLine adds a line whose endpoints belong to the pending sketch.
func (b *SketchSceneBuilder) CreateLine(start, end *Point) (*Line, error) {
	if err := b.usable(); err != nil {
		return nil, err
	}
	if !b.sketch.owns(start) || !b.sketch.owns(end) {
		return nil, ErrForeignPoint
	}
	if err := b.charge(1, 2, 512); err != nil {
		return nil, err
	}
	return b.sketch.CreateLine(start, end), nil
}

// CreateCircle adds a circle whose center belongs to the pending sketch.
func (b *SketchSceneBuilder) CreateCircle(center *Point, radius float64) (*Circle, error) {
	if err := b.usable(); err != nil {
		return nil, err
	}
	if !b.sketch.owns(center) {
		return nil, ErrForeignPoint
	}
	if err := b.charge(1, 4, 640); err != nil {
		return nil, err
	}
	return b.sketch.CreateCircle(center, radius), nil
}

// CreateArc adds an arc whose defining points belong to the pending sketch,
// together with its internal radius constraint.
func (b *SketchSceneBuilder) CreateArc(center, start, end *Point) (*Arc, error) {
	if err := b.usable(); err != nil {
		return nil, err
	}
	if !b.sketch.owns(center) || !b.sketch.owns(start) || !b.sketch.owns(end) {
		return nil, ErrForeignPoint
	}
	if err := b.charge(1, 3, 768); err != nil {
		return nil, err
	}
	return b.sketch.CreateArc(center, start, end), nil
}
