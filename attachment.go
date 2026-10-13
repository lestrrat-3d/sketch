package sketch

// AttachmentKey identifies transient consumer data stored on a Sketch. Each
// key returned by NewAttachmentKey has a different pointer identity. Keep a
// key private when its value is a source certificate that callers must not
// create or replace.
type AttachmentKey[T any] struct {
	identity byte // nonzero size keeps distinct allocations at distinct addresses
}

// NewAttachmentKey creates a key for one type of transient Sketch attachment.
func NewAttachmentKey[T any]() *AttachmentKey[T] {
	return &AttachmentKey[T]{identity: 1}
}

type attachment struct {
	owner    *Sketch
	revision uint64
	value    any
}

// Attach stores value on s until its geometry changes or it is unmarshaled in
// place. It returns false for a nil key or Sketch. Attachments are not included
// in Sketch JSON and do not change Sketch.Revision.
func (k *AttachmentKey[T]) Attach(s *Sketch, value T) bool {
	if k == nil || s == nil {
		return false
	}
	if s.attachments == nil {
		s.attachments = make(map[any]attachment)
	}
	s.attachments[k] = attachment{owner: s, revision: s.Revision(), value: value}
	return true
}

// Load returns the value attached under k only for the original Sketch
// pointer and its attached geometry revision. Copying a Sketch by value or
// changing its geometry cannot carry a live attachment forward.
func (k *AttachmentKey[T]) Load(s *Sketch) (T, bool) {
	var zero T
	if k == nil || s == nil {
		return zero, false
	}
	a, ok := s.attachments[k]
	if !ok || a.owner != s || a.revision != s.Revision() {
		return zero, false
	}
	value, ok := a.value.(T)
	if !ok {
		return zero, false
	}
	return value, true
}
