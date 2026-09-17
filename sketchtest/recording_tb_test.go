package sketchtest_test

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type recordingTB struct {
	testing.TB
	mu       sync.Mutex
	messages []string
	failed   bool
}

func (r *recordingTB) Helper() {}

func (r *recordingTB) Fail() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failed = true
}

func (r *recordingTB) Failed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.failed
}

func (r *recordingTB) FailNow() {
	r.Fail()
	runtime.Goexit()
}

func (r *recordingTB) Errorf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, fmt.Sprintf(format, args...))
	r.failed = true
}

func (r *recordingTB) Fatalf(format string, args ...any) {
	r.mu.Lock()
	r.messages = append(r.messages, fmt.Sprintf(format, args...))
	r.mu.Unlock()
	r.FailNow()
}

func (r *recordingTB) output() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.messages, "\n")
}

func captureFailure(t *testing.T, fn func(testing.TB)) string {
	t.Helper()

	recorder := &recordingTB{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(recorder)
	}()
	<-done

	require.True(t, recorder.failed, "the helper did not fail")
	return recorder.output()
}
