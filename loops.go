package sketch

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/lestrrat-3d/sketch/geom"
)

// ErrInvalidLoop is wrapped by every error [Sketch.DeclareLoop] returns, so
// errors.Is(err, ErrInvalidLoop) holds for all of them. Each cause also has its own
// sentinel, which errors.Is tells apart.
var ErrInvalidLoop = errors.New("sketch: invalid loop")

var (
	// ErrLoopEmpty: DeclareLoop was given no entity.
	ErrLoopEmpty = fmt.Errorf("%w: no entity", ErrInvalidLoop)
	// ErrLoopNilEntity: DeclareLoop was given a nil entity.
	ErrLoopNilEntity = fmt.Errorf("%w: nil entity", ErrInvalidLoop)
	// ErrLoopForeignEntity: an entity is not in this sketch, or was removed from it.
	ErrLoopForeignEntity = fmt.Errorf("%w: entity not in this sketch", ErrInvalidLoop)
	// ErrLoopDuplicateEntity: an entity is named twice in one loop.
	ErrLoopDuplicateEntity = fmt.Errorf("%w: entity named twice", ErrInvalidLoop)
	// ErrLoopSpanRange: a span of DeclareLoopSpans has a parameter that is not a number
	// in [0, 1], or its two parameters are equal.
	ErrLoopSpanRange = fmt.Errorf("%w: span parameters out of range", ErrInvalidLoop)
)

// Loop is a declared closed chain of entities of one [Sketch]. It is a handle: declare
// it with [Sketch.DeclareLoop], and read the verdict of each profile against it with
// [Profile.Containment]. Loops are transient state of the sketch. They are not
// serialized and are not part of [Sketch.Revision].
type Loop struct {
	sketch *Sketch
	// entities lists the entity of every element, in declaration order. An entity
	// repeats when the loop holds several spans of it.
	entities []Entity
	elems    []loopElem
	removed  bool
}

// loopElem is one element of a loop: a whole entity, or the part of it between two
// of its parameters (lo < hi, in the entity's natural parameter).
type loopElem struct {
	entity Entity
	span   bool
	lo, hi float64
}

// LoopSpan is one element of a loop declared with [Sketch.DeclareLoopSpans]: the part
// of Entity between the parameters TStart and TEnd. The parameters are the ones
// [BoundaryEdge.TStart] and [BoundaryEdge.TEnd] report for a fragment of the entity.
// They may be given in either order; the span is the same.
//
// A span with parameters 0 and 1 is the whole entity.
type LoopSpan struct {
	Entity       Entity
	TStart, TEnd float64
}

// Spans returns the loop's elements as spans, in the order they were declared. A whole
// entity reads as the span from 0 to 1. A span given in reverse order reads in natural
// order.
func (l *Loop) Spans() []LoopSpan {
	if l == nil {
		return nil
	}
	out := make([]LoopSpan, len(l.elems))
	for i, el := range l.elems {
		out[i] = LoopSpan{Entity: el.entity, TStart: el.lo, TEnd: el.hi}
		if !el.span {
			out[i].TStart, out[i].TEnd = 0, 1
		}
	}
	return out
}

// Entities returns the loop's entities in the order they were declared.
func (l *Loop) Entities() []Entity {
	if l == nil {
		return nil
	}
	return slices.Clone(l.entities)
}

// Removed reports whether the loop was removed, by [Sketch.RemoveLoop] or because
// [Sketch.RemoveEntity] removed one of its entities.
func (l *Loop) Removed() bool { return l == nil || l.removed }

// ContainmentState is the answer of [Profile.Containment].
type ContainmentState uint8

const (
	// ContainmentUndeclared is the zero value. It is returned when the loop was not
	// declared when the profile was built, is from another sketch, was removed, is nil,
	// or the profile was not made by [Sketch.Profiles]. It is never an answer about
	// the geometry.
	ContainmentUndeclared ContainmentState = iota
	// ContainmentInside means the profile's region lies inside the loop.
	ContainmentInside
	// ContainmentOutside means the profile's region lies outside the loop.
	ContainmentOutside
	// ContainmentUnproven means no certificate was found; see [Containment.Reason].
	ContainmentUnproven
)

// ContainmentReason says why a verdict is [ContainmentUnproven].
type ContainmentReason uint8

const (
	// ReasonNone accompanies every state but [ContainmentUnproven].
	ReasonNone ContainmentReason = iota
	// ReasonGap: the chain does not close, two consecutive entities do not share an
	// end coordinate exactly, or an edge of the loop bounds no face.
	ReasonGap
	// ReasonSelfCrossing: two edges of the loop cross or touch, or two entities of the
	// loop draw one span.
	ReasonSelfCrossing
	// ReasonKind: an entity of the loop is not a line, circle, arc or fit spline, or is
	// not drawn (a construction entity).
	ReasonKind
	// ReasonComponent: the region lies in a part of the arrangement whose map is not
	// certified, or is reachable from the unbounded face only through such parts.
	ReasonComponent
	// ReasonCoincident: a coincident-carrier overlap that involves the loop was refused.
	ReasonCoincident
	// ReasonInconsistent: the parity of the loop's edges around some cycle is odd.
	ReasonInconsistent
	// ReasonInvalid: the profile itself is not valid (see [Profile.Valid]).
	ReasonInvalid
	// ReasonStale: the profile no longer describes its sketch.
	ReasonStale
	// ReasonSpanBound: an end of a span is not an exact bound of the arrangement edges
	// of its entity, or the span holds no edge. The scene's bounds may be inexact, the
	// span may start inside an edge, or its parameters may not come from a fragment of
	// this sketch.
	ReasonSpanBound
)

// Containment is the verdict for one (profile, loop) pair.
type Containment struct {
	State ContainmentState
	// Reason is non-zero only when State is [ContainmentUnproven].
	Reason ContainmentReason
}

func unproven(r ContainmentReason) Containment {
	return Containment{State: ContainmentUnproven, Reason: r}
}

// DeclareLoop declares a closed chain of entities, given in chain order, and returns
// its handle. A sketch holds any number of loops; an entity may belong to several, and
// two loops may draw the same span with different entities.
//
// It returns an error wrapping [ErrInvalidLoop] for an empty list ([ErrLoopEmpty]), a nil
// entity ([ErrLoopNilEntity]), an entity that is not in this sketch
// ([ErrLoopForeignEntity]), or an entity named twice ([ErrLoopDuplicateEntity]). It does
// not check that the chain closes: the sketch is solved later, so every [Sketch.Profiles]
// call judges the chain from the coordinates it finds. A chain that does not close reads
// [ReasonGap] in every profile.
//
// The verdict is available on profiles built after the declaration. A sketch that
// declares no loops does the same work and returns the same values as before.
func (s *Sketch) DeclareLoop(entities ...Entity) (*Loop, error) {
	if len(entities) == 0 {
		return nil, ErrLoopEmpty
	}
	seen := map[Entity]struct{}{}
	for _, e := range entities {
		if e == nil {
			return nil, ErrLoopNilEntity
		}
		if !s.ownsEntity(e) {
			return nil, ErrLoopForeignEntity
		}
		if _, dup := seen[e]; dup {
			return nil, ErrLoopDuplicateEntity
		}
		seen[e] = struct{}{}
	}
	elems := make([]loopElem, len(entities))
	for i, e := range entities {
		elems[i] = loopElem{entity: e}
	}
	l := &Loop{sketch: s, entities: slices.Clone(entities), elems: elems}
	s.loops = append(s.loops, l)
	return l, nil
}

// DeclareLoopSpans declares a closed loop whose elements are parts of entities, so a
// loop can follow a fragment of a circle, or the part of a fit spline beyond a crossing.
// It is the sub-range form of [Sketch.DeclareLoop] and the sketch holds both kinds of
// loop together. Pass the parameters of a fragment as [BoundaryEdge] reports them for a
// profile of this sketch, or of a sketch that holds the same entities.
//
// It returns an error wrapping [ErrInvalidLoop] for an empty list ([ErrLoopEmpty]), a nil
// entity ([ErrLoopNilEntity]), an entity that is not in this sketch ([ErrLoopForeignEntity]),
// the same span twice ([ErrLoopDuplicateEntity]), or a parameter that is not a number in
// [0, 1] or two equal parameters ([ErrLoopSpanRange]). A span may be given with TStart
// above TEnd. A span from 0 to 1 is the whole entity.
//
// The verdict of a loop with a span follows the arrangement, not coordinates. The two
// ends of every span must be exact bounds of arrangement edges of the span's entity, and the
// edges between them must run without a break. The elements together must cover edges
// that form one cycle, joined at arrangement vertices; the order the spans are given in
// does not change the verdict. Otherwise every profile reads [ContainmentUnproven] with
// [ReasonSpanBound] (an end is not an exact bound) or [ReasonGap] (a break, or more than
// one cycle). A span covers only the edges its own entity draws: a stretch that another
// entity draws in its place, as the lower-indexed one of two coincident curves does, is
// not covered.
func (s *Sketch) DeclareLoopSpans(spans ...LoopSpan) (*Loop, error) {
	if len(spans) == 0 {
		return nil, ErrLoopEmpty
	}
	type key struct {
		e      Entity
		lo, hi float64
	}
	seen := map[key]struct{}{}
	elems := make([]loopElem, 0, len(spans))
	entities := make([]Entity, 0, len(spans))
	for _, sp := range spans {
		if sp.Entity == nil {
			return nil, ErrLoopNilEntity
		}
		if !s.ownsEntity(sp.Entity) {
			return nil, ErrLoopForeignEntity
		}
		lo, hi := sp.TStart, sp.TEnd
		if hi < lo {
			lo, hi = hi, lo
		}
		if math.IsNaN(lo) || math.IsNaN(hi) || lo < 0 || hi > 1 || lo == hi {
			return nil, ErrLoopSpanRange
		}
		if _, dup := seen[key{sp.Entity, lo, hi}]; dup {
			return nil, ErrLoopDuplicateEntity
		}
		seen[key{sp.Entity, lo, hi}] = struct{}{}
		elems = append(elems, loopElem{entity: sp.Entity, span: !(lo == 0 && hi == 1), lo: lo, hi: hi})
		entities = append(entities, sp.Entity)
	}
	l := &Loop{sketch: s, entities: entities, elems: elems}
	s.loops = append(s.loops, l)
	return l, nil
}

// Loops returns the live loops in declaration order. The slice is a copy; the
// elements are the live handles.
func (s *Sketch) Loops() []*Loop { return slices.Clone(s.loops) }

// RemoveLoop removes a loop and reports whether it was declared. Profiles built
// earlier read [ContainmentUndeclared] for it afterwards.
func (s *Sketch) RemoveLoop(l *Loop) bool {
	i := slices.Index(s.loops, l)
	if l == nil || i < 0 {
		return false
	}
	s.loops = slices.Delete(s.loops, i, i+1)
	l.removed = true
	return true
}

// dropLoopsNaming removes every loop that names the entity.
func (s *Sketch) dropLoopsNaming(e Entity) {
	kept := s.loops[:0]
	for _, l := range s.loops {
		if slices.Contains(l.entities, e) {
			l.removed = true
			continue
		}
		kept = append(kept, l)
	}
	for i := len(kept); i < len(s.loops); i++ {
		s.loops[i] = nil
	}
	s.loops = kept
}

// Containment returns the verdict of this profile against the loop. A profile built
// before the loop was declared, one from another sketch, a removed or nil loop, and a
// profile made by [Sketch.UnionProfiles] all read [ContainmentUndeclared].
func (p *Profile) Containment(l *Loop) Containment {
	if p == nil || l == nil || p.containment == nil {
		return Containment{}
	}
	c, ok := p.containment[l]
	if !ok || l.removed {
		return Containment{}
	}
	if p.IsStale() {
		return unproven(ReasonStale)
	}
	return c
}

// Containments returns the verdict against every live loop of the sketch, in the order
// [Sketch.Loops] returns them now. It is Containment called once per loop.
func (p *Profile) Containments() []Containment {
	if p == nil || p.sketch == nil {
		return nil
	}
	out := make([]Containment, len(p.sketch.loops))
	for i, l := range p.sketch.loops {
		out[i] = p.Containment(l)
	}
	return out
}

// loopEnds returns the two end coordinates of an entity a loop may chain, and false for
// an entity that is a closed curve on its own or of a kind a loop does not take.
func loopEnds(e Entity) (a, b [2]float64, ok bool) {
	pt := func(p *Point) [2]float64 { return [2]float64{p.X(), p.Y()} }
	switch t := e.(type) {
	case *Line:
		return pt(t.Start), pt(t.End), true
	case *Arc:
		return pt(t.Start), pt(t.End), true
	case *FitSpline:
		if len(t.Fit) < 2 {
			return a, b, false
		}
		return pt(t.Fit[0]), pt(t.Fit[len(t.Fit)-1]), true
	}
	return a, b, false
}

// loopKindOK reports whether a loop may hold the entity.
func loopKindOK(e Entity) bool {
	switch e.(type) {
	case *Line, *Arc, *FitSpline, *Circle:
		return !e.IsConstruction()
	}
	return false
}

// loopClosed reports whether the entities, in order, form one closed chain whose
// consecutive ends are bit-identical. A circle is a closed chain on its own.
func loopClosed(ents []Entity) bool {
	if len(ents) == 1 {
		_, isCircle := ents[0].(*Circle)
		return isCircle
	}
	type ends struct{ a, b [2]float64 }
	es := make([]ends, len(ents))
	for i, e := range ents {
		a, b, ok := loopEnds(e)
		if !ok || a == b {
			return false
		}
		es[i] = ends{a, b}
	}
	try := func(entry [2]float64) bool {
		cur := entry
		for i := range es {
			var exit [2]float64
			switch cur {
			case es[i].a:
				exit = es[i].b
			case es[i].b:
				exit = es[i].a
			default:
				return false
			}
			cur = exit
		}
		return cur == entry
	}
	// The entry of the first entity is whichever of its ends the last entity's exit
	// meets; try both.
	return try(es[0].a) || try(es[0].b)
}

// containmentFor computes the verdicts of every live loop against the arrangement's
// regions. srcOf maps an entity to its source index in the arrangement.
func (s *Sketch) containmentFor(arr *geom.Arrangement, profiles []*Profile, srcOf map[Entity]int) {
	loops := slices.Clone(s.loops)
	if len(loops) == 0 || arr.FaceGraph == nil {
		return
	}
	type prepared struct {
		reason ContainmentReason
	}
	prep := make([]prepared, len(loops))
	var judged [][]geom.LoopElement
	var judgedAt []int
	for k, l := range loops {
		ok := true
		hasSpan := false
		var elems []geom.LoopElement
		for _, el := range l.elems {
			if !loopKindOK(el.entity) {
				prep[k].reason, ok = ReasonKind, false
				break
			}
			src, drawn := srcOf[el.entity]
			if !drawn {
				prep[k].reason, ok = ReasonKind, false
				break
			}
			hasSpan = hasSpan || el.span
			elems = append(elems, geom.LoopElement{Source: src, Span: el.span, Lo: el.lo, Hi: el.hi})
		}
		// A span ends where the arrangement cut its entity, so its neighbour's end
		// coordinate need not be bit-identical. The walk judges such a loop by its
		// vertices.
		if ok && !hasSpan && !loopClosed(l.entities) {
			prep[k].reason, ok = ReasonGap, false
		}
		if ok {
			judged = append(judged, elems)
			judgedAt = append(judgedAt, k)
		}
	}
	walks := arr.FaceGraph.WalkLoopElements(judged)
	walkOf := map[int]geom.LoopWalk{}
	for i, k := range judgedAt {
		walkOf[k] = walks[i]
	}
	for fi, p := range profiles {
		p.containment = make(map[*Loop]Containment, len(loops))
		for k, l := range loops {
			var c Containment
			switch w, walked := walkOf[k]; {
			case !p.Valid:
				c = unproven(ReasonInvalid)
			case !walked:
				c = unproven(prep[k].reason)
			case w.Defect != geom.LoopDefectNone:
				c = unproven(walkDefectReason(w.Defect, arr.Degenerate))
			case w.Side[fi] < 0:
				c = unproven(ReasonComponent)
			case w.Side[fi] == 1:
				c = Containment{State: ContainmentInside}
			default:
				c = Containment{State: ContainmentOutside}
			}
			p.containment[l] = c
		}
	}
}

func walkDefectReason(d geom.LoopDefect, degenerate bool) ContainmentReason {
	switch d {
	case geom.LoopDefectGap:
		return ReasonGap
	case geom.LoopDefectSpanBound:
		return ReasonSpanBound
	case geom.LoopDefectSelfCrossing:
		if degenerate {
			return ReasonCoincident
		}
		return ReasonSelfCrossing
	}
	if degenerate {
		return ReasonCoincident
	}
	return ReasonInconsistent
}
