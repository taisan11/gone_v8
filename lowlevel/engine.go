//go:build (cgo && (linux || darwin)) || (windows && amd64)

package lowlevel

import (
	"errors"
	"runtime"
	"sync"
)

type evaluation struct {
	source string
	do     func(*Isolate) error
	answer chan result
}

type result struct {
	text string
	err  error
}

// Engine holds one V8 isolate and one persistent Context.
// Its native calls are serialized on a dedicated locked OS thread.
// It is safe to call Eval from multiple goroutines; JavaScript runs serially.
// You must call Close when finished. Do not copy Engine.
type Engine struct {
	mu     sync.Mutex
	closed bool
	jobs   chan evaluation
	answer chan result // Reused while mu serializes submitters.
	done   chan struct{}
	thread uintptr // Immutable after New returns.
}

// New starts an OS-thread-affine V8 engine.
func New() (*Engine, error) {
	e := &Engine{jobs: make(chan evaluation), done: make(chan struct{})}
	started := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(e.done)
		vm, err := newNativeVM()
		if err != nil {
			started <- err
			return
		}
		isolate := newIsolate(vm)
		e.thread = isolate.thread
		defer isolate.close()
		started <- nil
		for task := range e.jobs {
			if task.do != nil {
				task.answer <- invokeIsolateCallback(isolate, task.do)
			} else {
				task.answer <- vm.eval(task.source)
			}
		}
	}()
	if err := <-started; err != nil {
		<-e.done
		return nil, err
	}
	return e, nil
}

// Eval executes JavaScript in the persistent context and returns JS ToString output.
// For example, Eval("[1,2].join('-')") returns "1-2".
func (e *Engine) Eval(source string) (string, error) {
	value := e.submit(evaluation{source: source})
	return value.text, value.err
}

func (e *Engine) submit(task evaluation) result {
	if e == nil {
		return result{err: errors.New("gone_v8: nil engine")}
	}
	if e.onWorkerThread() {
		return result{err: ErrReentrant}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return result{err: ErrClosed}
	}
	if e.answer == nil {
		e.answer = make(chan result, 1)
	}
	task.answer = e.answer
	select {
	case e.jobs <- task:
	case <-e.done:
		return result{err: ErrClosed}
	}
	select {
	case value := <-task.answer:
		return value
	case <-e.done:
		return result{err: ErrClosed}
	}
}

// Do enters the engine's owning OS thread once. Within fn, low-level methods
// operate directly; batch work here to avoid a queue round-trip per operation.
// Do not use the Isolate/Scope/Value from another goroutine, call methods on
// this Engine (including Close/Compile/CompiledScript.Run) from fn, or wait for
// another goroutine that needs this Engine. Reentrant calls return ErrReentrant;
// Close panics with ErrReentrant. Waiting for a different caller can deadlock.
// Panics are rethrown in the caller after scope cleanup; the engine remains usable.
func (e *Engine) Do(fn func(*Isolate) error) error {
	if fn == nil {
		return errors.New("gone_v8: nil isolate callback")
	}
	r := e.submit(evaluation{do: fn})
	if p, ok := r.err.(*isolateCallbackPanic); ok {
		panic(p.value)
	}
	return r.err
}

type isolateCallbackPanic struct{ value any }

func (*isolateCallbackPanic) Error() string { return "gone_v8: isolate callback panicked" }

func invokeIsolateCallback(i *Isolate, fn func(*Isolate) error) (r result) {
	returned := false
	defer func() {
		if !returned {
			r.err = &isolateCallbackPanic{value: recover()}
		}
	}()
	r.err = fn(i)
	returned = true
	return r
}

func (e *Engine) onWorkerThread() bool {
	if e.thread == 0 || nativeThreadID() != e.thread {
		return false
	}
	select {
	case <-e.done:
		return false
	default:
		return true
	}
}

// Close destroys the native Context and isolate on their owning OS thread.
// Close is idempotent, but must not be called from an engine callback.
func (e *Engine) Close() {
	if e == nil {
		return
	}
	if e.onWorkerThread() {
		panic(ErrReentrant)
	}
	e.mu.Lock()
	if !e.closed {
		e.closed = true
		close(e.jobs)
	}
	e.mu.Unlock()
	<-e.done
}
