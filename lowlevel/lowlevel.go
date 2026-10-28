//go:build (cgo && (linux || darwin)) || (windows && amd64)

package lowlevel

import (
	"errors"
	"runtime"
	"unsafe"
)

var (
	ErrClosed           = errors.New("gone_v8: resource is closed")
	ErrReentrant        = errors.New("gone_v8: cannot call queued Engine methods from its owning thread")
	ErrWrongThread      = errors.New("gone_v8: operation must run on the owning OS thread")
	ErrScopeClosed      = errors.New("gone_v8: handle scope is closed")
	ErrScopeNotCurrent  = errors.New("gone_v8: only the innermost handle scope may be used")
	ErrWrongIsolate     = errors.New("gone_v8: resource belongs to another isolate")
	ErrWrongScope       = errors.New("gone_v8: value belongs to another scope")
	ErrInvalidValue     = errors.New("gone_v8: invalid value")
	ErrType             = errors.New("gone_v8: value has an unexpected type")
	ErrCopiedResource   = errors.New("gone_v8: resource must not be copied")
	errNativeAllocation = errors.New("gone_v8: native allocation failed")
)

var emptyNativeText byte

// Native string consumers copy synchronously and never retain this pointer.
func nativeTextPointer(text string) *byte {
	if len(text) == 0 {
		return &emptyNativeText
	}
	return unsafe.StringData(text)
}

// Isolate is a low-level, thread-affine V8 isolate with one retained context.
// Obtain it through WithIsolate or Engine.Do. It is not goroutine-safe, must not
// be copied, and cannot be used after the callback that owns it has returned.
// Methods call V8 directly, with no worker queue or implicit string conversion.
type Isolate struct {
	identity *Isolate
	vm       *nativeVM
	thread   uintptr
	closed   bool
	current  *Scope
	scripts  map[*Script]struct{}
}

func newIsolate(vm *nativeVM) *Isolate {
	i := &Isolate{vm: vm, thread: nativeThreadID()}
	i.identity = i
	return i
}

// WithIsolate pins the calling goroutine to its OS thread, creates an isolate,
// calls fn synchronously and releases all native resources before returning.
// Cleanup also runs if fn panics. No worker goroutine or channel is involved.
func WithIsolate(fn func(*Isolate) error) error {
	if fn == nil {
		return errors.New("gone_v8: nil isolate callback")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	vm, err := newNativeVM()
	if err != nil {
		return err
	}
	i := newIsolate(vm)
	defer i.close()
	return fn(i)
}

func (i *Isolate) check() error {
	if i == nil || i.vm == nil || i.thread == 0 {
		return ErrClosed
	}
	if i.identity != i {
		return ErrCopiedResource
	}
	// Check the immutable owner token before touching thread-confined state.
	if nativeThreadID() != i.thread {
		return ErrWrongThread
	}
	if i.closed {
		return ErrClosed
	}
	return nil
}

func (i *Isolate) close() {
	for script := range i.scripts {
		script.native.close()
		script.closed = true
	}
	i.scripts = nil
	i.closed = true
	i.vm.close()
}

// WithScope opens a native V8 HandleScope. Values created in fn expire when fn
// returns, including on panic. Nested scopes are allowed, but only the innermost
// scope may be operated on. Use short scopes to bound native handle retention.
func (i *Isolate) WithScope(fn func(*Scope) error) error {
	if err := i.check(); err != nil {
		return err
	}
	if fn == nil {
		return errors.New("gone_v8: nil scope callback")
	}
	native, err := i.vm.openScope()
	if err != nil {
		return err
	}
	s := &Scope{owner: i, native: native, active: true}
	parent := i.current
	i.current = s
	defer func() {
		s.active = false
		s.native.close()
		i.current = parent
	}()
	return fn(s)
}

// Compile compiles once, without executing. The script survives handle scopes
// and is tied to this isolate/context. Close it when done; isolate teardown
// also frees any scripts that were not explicitly closed.
func (i *Isolate) Compile(source string) (*Script, error) {
	var script *Script
	err := i.WithScope(func(s *Scope) error {
		var err error
		script, err = s.Compile(source)
		return err
	})
	return script, err
}

// Scope owns local Values. Do not copy it or use it outside its callback.
type Scope struct {
	owner  *Isolate
	native nativeScope
	active bool
}

func (s *Scope) check() error {
	if s == nil {
		return ErrScopeClosed
	}
	i := s.owner
	if i == nil || i.vm == nil || i.thread == 0 {
		return ErrClosed
	}
	if i.identity != i {
		return ErrCopiedResource
	}
	// The native check only compares the scope pointer; it never dereferences
	// it, so it is safe even if the scope has already been closed. Checking
	// the thread before accessing thread-confined state preserves safety.
	current := s.native.checkThreadCurrent(i.thread)
	if current < 0 {
		return ErrWrongThread
	}
	if i.closed {
		return ErrClosed
	}
	if !s.active {
		return ErrScopeClosed
	}
	if i.current != s || current == 0 {
		return ErrScopeNotCurrent
	}
	return nil
}

// Eval compiles and executes source, returning a local Value without ToString.
func (s *Scope) Eval(source string) (Value, error) {
	if err := s.check(); err != nil {
		return Value{}, err
	}
	h, err := s.native.evalValue(source)
	return s.value(h, err)
}

func (s *Scope) value(id uintptr, err error) (Value, error) {
	if err != nil {
		return Value{}, err
	}
	return Value{scope: s, id: id}, nil
}

// Compile creates a persistent compiled script without executing it.
func (s *Scope) Compile(source string) (*Script, error) {
	if err := s.check(); err != nil {
		return nil, err
	}
	native, err := s.native.compile(source)
	if err != nil {
		return nil, err
	}
	script := &Script{owner: s.owner, native: native}
	script.identity = script
	if s.owner.scripts == nil {
		s.owner.scripts = make(map[*Script]struct{})
	}
	s.owner.scripts[script] = struct{}{}
	return script, nil
}

// Script is a compiled, context-bound script, reusable across scopes in its
// owning isolate. It is thread-affine, must not be copied, and holds native
// resources until Close or isolate teardown. It is not a serialized code cache.
type Script struct {
	identity *Script
	owner    *Isolate
	native   nativeScript
	closed   bool
}

// Run executes the compiled script and creates a Value in scope.
func (script *Script) Run(scope *Scope) (Value, error) {
	if err := scope.check(); err != nil {
		return Value{}, err
	}
	if script == nil || script.owner == nil {
		return Value{}, ErrClosed
	}
	if script.identity != script {
		return Value{}, ErrCopiedResource
	}
	if script.owner != scope.owner {
		return Value{}, ErrWrongIsolate
	}
	if script.closed {
		return Value{}, ErrClosed
	}
	h, err := scope.native.run(script.native)
	return scope.value(h, err)
}

// Close releases the persistent script. Call on the owning thread while its
// isolate is alive. Repeated calls on that thread are harmless.
func (script *Script) Close() error {
	if script == nil {
		return nil
	}
	if script.owner == nil {
		return ErrClosed
	}
	if script.identity != script {
		return ErrCopiedResource
	}
	if err := script.owner.check(); err != nil {
		return err
	}
	if !script.closed {
		script.native.close()
		script.closed = true
		delete(script.owner.scripts, script)
	}
	return nil
}
